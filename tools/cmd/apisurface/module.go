package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// maxZipFile is the most bytes apisurface reads from one file of the module zip; the zip's own
// limit is 500 MB in all, and v1's largest Go file is under 200 KB.
const maxZipFile = 8 << 20

// download is what go mod download -json prints for a module version.
type download struct {
	Path    string
	Version string
	Zip     string
	Sum     string
	Error   string
}

// verificationOff lists the environment variables that would let the go command skip the
// checksum database, or fetch insecurely; apisurface runs it without them. It also runs it with
// GOENV=off, so that the same settings in the user's go env file do not apply either.
var verificationOff = []string{
	"GONOSUMDB", "GOPRIVATE", "GONOPROXY", "GOINSECURE", "GOFLAGS", "GOENV", "GOMODCACHE",
}

// generate writes the v1 surface of a module version: it downloads the module's zip with
// go mod download, which verifies it against the checksum database, reads blocks a, b and c from
// its files and writes them to opts.out.
func generate(ctx context.Context, opts options, stdout io.Writer, sys system) error {
	modulePath, version, err := splitModule(opts.module)
	if err != nil {
		return err
	}
	specContent, err := os.ReadFile(opts.spec)
	if err != nil {
		return fmt.Errorf("reading the API spec: %w", err)
	}
	spec, err := parseSpec(specContent)
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "apisurface-download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	got, err := downloadModule(ctx, sys, work, modulePath, version)
	if err != nil {
		return err
	}
	files, err := readModuleZip(got.Zip, modulePath, version)
	if err != nil {
		return err
	}
	items, err := extractSurface(files, spec)
	if err != nil {
		return fmt.Errorf("reading the surface of %s@%s: %w", modulePath, version, err)
	}
	content := writeSurface(modulePath, version, got.Sum, items)
	if err := writeFileAtomic(opts.out, content); err != nil {
		return err
	}
	counts := blockCounts(items)
	fmt.Fprintf(stdout, "%s@%s %s: %d identifiers (a), %d operations (b), %d models types (c)\n",
		modulePath, version, got.Sum, counts[blockIdentifiers], counts[blockOperations],
		counts[blockModels])
	fmt.Fprintf(stdout, "PASS: wrote %s\n", opts.out)
	return nil
}

// splitModule splits <module path>@<version> and checks both parts.
func splitModule(arg string) (modulePath, version string, err error) {
	modulePath, version, ok := strings.Cut(arg, "@")
	if !ok {
		return "", "", fmt.Errorf("-module %q is not <module path>@<version>", arg)
	}
	if err := module.Check(modulePath, version); err != nil {
		return "", "", fmt.Errorf("-module %q: %w", arg, err)
	}
	if semver.Canonical(version) != version {
		return "", "", fmt.Errorf("-module %q: the version must be a canonical semantic version, "+
			"such as v1.2.8", arg)
	}
	return modulePath, version, nil
}

// downloadModule downloads a module version with go mod download -json, run in the empty
// directory work with a module cache of its own under it, and returns what it printed. The go
// command checks the zip against the checksum database, GOSUMDB; apisurface refuses to run with
// GOSUMDB=off and clears the variables that would skip the check for some modules.
func downloadModule(ctx context.Context, sys system, work, modulePath, version string,
) (download, error) {
	env := sys.environ()
	for _, kv := range env {
		if name, value, _ := strings.Cut(kv, "="); name == "GOSUMDB" && value == "off" {
			return download{}, errors.New("GOSUMDB=off: apisurface reads only a module zip that " +
				"the checksum database verified; unset GOSUMDB")
		}
	}
	env = withoutVars(env, verificationOff...)
	env = append(env, "GOENV=off", "GOMODCACHE="+filepath.Join(work, "modcache"),
		"GOFLAGS=-modcacherw")
	out, stderr, err := sys.runIn(ctx, work, env, "go", "mod", "download", "-json",
		modulePath+"@"+version)
	var got download
	//nolint:musttag // go mod download -json names its JSON fields as the Go fields are named.
	if jsonErr := json.Unmarshal(out, &got); jsonErr != nil {
		return download{}, fmt.Errorf("go mod download %s@%s: %w: %s", modulePath, version,
			errors.Join(err, jsonErr), strings.TrimSpace(string(stderr)))
	}
	if got.Error != "" || err != nil {
		return download{}, fmt.Errorf("go mod download %s@%s: %s %s", modulePath, version,
			got.Error, strings.TrimSpace(string(stderr)))
	}
	if got.Path != modulePath || got.Version != version || got.Zip == "" ||
		!strings.HasPrefix(got.Sum, "h1:") {
		return download{}, fmt.Errorf("go mod download %s@%s printed %s@%s with zip %q and hash "+
			"%q", modulePath, version, got.Path, got.Version, got.Zip, got.Sum)
	}
	return got, nil
}

// withoutVars returns env without the variables named.
func withoutVars(env []string, names ...string) []string {
	var kept []string
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(names, name) {
			kept = append(kept, kv)
		}
	}
	return kept
}

// readModuleZip returns the Go files of the module zip, with their slash paths in the module.
// Every file of a module zip is under <module path>@<version>/.
func readModuleZip(name, modulePath, version string) ([]sourceFile, error) {
	r, err := zip.OpenReader(name)
	if err != nil {
		return nil, fmt.Errorf("opening the module zip: %w", err)
	}
	defer r.Close()
	prefix := modulePath + "@" + version + "/"
	var files []sourceFile
	for _, f := range r.File {
		rel, ok := strings.CutPrefix(f.Name, prefix)
		if !ok {
			return nil, fmt.Errorf("the module zip holds %s, outside %s", f.Name, prefix)
		}
		if path.Ext(rel) != ".go" {
			continue
		}
		content, err := readZipFile(f)
		if err != nil {
			return nil, err
		}
		files = append(files, sourceFile{name: rel, content: content})
	}
	return files, nil
}

// readZipFile returns the content of a file of a zip, up to maxZipFile bytes.
func readZipFile(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("opening %s in the module zip: %w", f.Name, err)
	}
	defer r.Close()
	content, err := io.ReadAll(io.LimitReader(r, maxZipFile+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s in the module zip: %w", f.Name, err)
	}
	if len(content) > maxZipFile {
		return nil, fmt.Errorf("%s in the module zip is over %d bytes", f.Name, maxZipFile)
	}
	return content, nil
}

// writeFileAtomic writes content to name through a temporary file beside it, so that a failed
// write leaves the old file in place.
func writeFileAtomic(name string, content []byte) error {
	tmp := name + ".tmp"
	//nolint:gosec // G306: the files apisurface writes are committed, and anyone may read them.
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, name); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}

// blockCounts returns the number of items in each block.
func blockCounts(items []item) map[string]int {
	counts := map[string]int{}
	for _, it := range items {
		counts[it.block]++
	}
	return counts
}
