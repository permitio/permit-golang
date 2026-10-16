package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/mod/sumdb"
	"golang.org/x/mod/sumdb/dirhash"
	"golang.org/x/mod/sumdb/note"
)

const (
	// fixturePath and fixtureVersion name the fixture module on the fake proxy.
	fixturePath    = "example.com/sdk"
	fixtureVersion = "v1.2.3"
	// fixtureGoMod is the fixture module's go.mod.
	fixtureGoMod = "module example.com/sdk\n\ngo 1.19\n"
)

// moduleZip returns a module zip of the files, a map from slash path to content, for
// fixturePath at fixtureVersion. go.mod is added.
func moduleZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	all := maps.Clone(files)
	all["go.mod"] = fixtureGoMod
	for _, name := range slices.Sorted(maps.Keys(all)) {
		f, err := w.Create(fixturePath + "@" + fixtureVersion + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, all[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// zipHash returns the h1: hash of a module zip, as go.sum and the checksum database hold it.
func zipHash(t *testing.T, content []byte) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "m.zip")
	if err := os.WriteFile(name, content, 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := dirhash.HashZip(name, dirhash.Hash1)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// fakeGo is a fake Go module proxy and checksum database that serve one module version.
type fakeGo struct {
	proxy, sumdb *httptest.Server
	// verifierKey is the checksum database's public key.
	verifierKey string
	// zipRequests and lookups count the zip downloads and the checksum database's lookups.
	zipRequests, lookups atomic.Int32
}

// newFakeGo starts a proxy that serves served as the fixture module's zip and a checksum
// database that holds the hash of signed. They differ when the test tampers with the zip.
func newFakeGo(t *testing.T, served, signed []byte) *fakeGo {
	t.Helper()
	f := &fakeGo{}
	skey, vkey, err := note.GenerateKey(rand.Reader, "sum.example.test")
	if err != nil {
		t.Fatal(err)
	}
	f.verifierKey = vkey
	goModHash, err := dirhash.Hash1([]string{"go.mod"}, func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(fixtureGoMod)), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	signedHash := zipHash(t, signed)
	db := sumdb.NewServer(sumdb.NewTestServer(skey, func(path, version string) ([]byte, error) {
		if path != fixturePath || version != fixtureVersion {
			return nil, fmt.Errorf("unknown module %s@%s", path, version)
		}
		return fmt.Appendf(nil, "%s %s %s\n%s %s/go.mod %s\n", path, version, signedHash, path,
			version, goModHash), nil
	}))
	f.sumdb = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/lookup/") {
			f.lookups.Add(1)
		}
		db.ServeHTTP(w, r)
	}))
	t.Cleanup(f.sumdb.Close)
	prefix := "/" + fixturePath + "/@v/" + fixtureVersion
	f.proxy = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case prefix + ".info":
			fmt.Fprintf(w, `{"Version":%q,"Time":"2026-01-02T03:04:05Z"}`, fixtureVersion)
		case prefix + ".mod":
			_, _ = io.WriteString(w, fixtureGoMod)
		case prefix + ".zip":
			f.zipRequests.Add(1)
			_, _ = w.Write(served)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.proxy.Close)
	return f
}

// sys returns a system whose go command uses the fake proxy and checksum database, with GOPATH
// in a temporary directory, so that the checksum database's state stays out of the user's.
// extra is added after them.
func (f *fakeGo) sys(t *testing.T, extra ...string) system {
	t.Helper()
	return system{extraEnv: append([]string{
		"GOPROXY=" + f.proxy.URL,
		"GOSUMDB=" + f.verifierKey + " " + f.sumdb.URL,
		"GOPATH=" + t.TempDir(),
	}, extra...)}
}

// generateArgs returns the command line that writes the fixture's surface to a file in a
// temporary directory, whose name it also returns.
func generateArgs(t *testing.T) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.json")
	if err := os.WriteFile(spec, []byte(fixtureSpec), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "v1-surface.txt")
	return []string{"-module", fixturePath + "@" + fixtureVersion, "-spec", spec, "-out", out}, out
}

func TestGenerate(t *testing.T) {
	content := moduleZip(t, fixtureModule())
	f := newFakeGo(t, content, content)
	args, out := generateArgs(t)
	code, output := runTool(t, f.sys(t), args...)
	wantRun(t, code, output, exitPassed,
		"example.com/sdk@v1.2.3 "+zipHash(t, content)+": 16 identifiers (a), 3 operations (b), "+
			"2 models types (c)\n", "PASS: wrote "+out)
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	header := "\n# module example.com/sdk v1.2.3 " + zipHash(t, content) + "\n"
	before, items, ok := strings.Cut(string(got), header)
	if !ok || !strings.HasPrefix(before, "# The v1 API that v2 replaces or drops.") {
		t.Fatalf("the file does not start with the header and %q:\n%s", header, got)
	}
	if want := strings.Join(fixtureSurface, "\n") + "\n"; items != want {
		t.Errorf("the file lists\n%s\nwant\n%s", items, want)
	}
	if f.lookups.Load() == 0 {
		t.Error("the checksum database was never asked for the module")
	}
}

func TestGenerateFailsOnATamperedZip(t *testing.T) {
	signed := moduleZip(t, fixtureModule())
	tampered := moduleZip(t, maps.Collect(func(yield func(string, string) bool) {
		for name, content := range fixtureModule() {
			if !yield(name, strings.ReplaceAll(content, "Users", "Admins")) {
				return
			}
		}
	}))
	// A go env file that would let the go command skip the checksum database for the module.
	goEnv := filepath.Join(t.TempDir(), "go.env")
	if err := os.WriteFile(goEnv, []byte("GONOSUMDB=example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, extra := range map[string][]string{
		"in a plain environment": nil,
		// Each of these would let the go command skip the checksum database for the module, or
		// fetch it from elsewhere; apisurface clears them, so the tampered zip is still caught.
		"with GONOSUMDB":  {"GONOSUMDB=example.com"},
		"with GOPRIVATE":  {"GOPRIVATE=example.com"},
		"with GOFLAGS":    {"GOFLAGS=-insecure"},
		"with a GOENV":    {"GOENV=" + goEnv},
		"with GONOPROXY":  {"GONOPROXY=example.com", "GOINSECURE=example.com"},
		"with GOMODCACHE": {"GOMODCACHE=" + t.TempDir()},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeGo(t, tampered, signed)
			args, out := generateArgs(t)
			code, output := runTool(t, f.sys(t, extra...), args...)
			wantRun(t, code, output, exitDidNotRun, "DID NOT RUN: go mod download "+
				"example.com/sdk@v1.2.3", "SECURITY ERROR")
			if _, err := os.Stat(out); err == nil {
				t.Error("the surface file was written from a zip that failed verification")
			}
			if f.zipRequests.Load() == 0 {
				t.Error("the zip was never downloaded from the fake proxy")
			}
		})
	}
}

func TestGenerateRefusesGOSUMDBOff(t *testing.T) {
	content := moduleZip(t, fixtureModule())
	f := newFakeGo(t, content, content)
	args, _ := generateArgs(t)
	code, output := runTool(t, f.sys(t, "GOSUMDB=off"), args...)
	wantRun(t, code, output, exitDidNotRun, "DID NOT RUN: GOSUMDB=off: apisurface reads only "+
		"a module zip that the checksum database verified")
	if f.zipRequests.Load() != 0 {
		t.Error("the zip was downloaded with GOSUMDB=off")
	}
}

func TestGenerateDidNotRun(t *testing.T) {
	content := moduleZip(t, fixtureModule())
	broken := fixtureModule()
	delete(broken, "pkg/permit/permit.go")
	for name, tc := range map[string]struct {
		zip  []byte
		args func(args []string) []string
		want string
	}{
		"a version the proxy does not have": {
			zip: content,
			args: func(args []string) []string {
				return slices.Concat([]string{"-module", fixturePath + "@v1.2.4"}, args[2:])
			},
			want: "DID NOT RUN: go mod download example.com/sdk@v1.2.4",
		},
		"a module without one of the packages": {
			zip: moduleZip(t, broken),
			want: "DID NOT RUN: reading the surface of example.com/sdk@v1.2.3: the module has " +
				"no Go file in pkg/permit",
		},
		"a missing spec": {
			zip: content,
			args: func(args []string) []string {
				return slices.Concat(args[:3], []string{filepath.Join(t.TempDir(), "none.json")},
					args[4:])
			},
			want: "DID NOT RUN: reading the API spec:",
		},
		"an output directory that does not exist": {
			zip: content,
			args: func(args []string) []string {
				return slices.Concat(args[:5], []string{filepath.Join(t.TempDir(), "no", "x")})
			},
			want: "DID NOT RUN: open ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			signed := tc.zip
			f := newFakeGo(t, tc.zip, signed)
			args, _ := generateArgs(t)
			if tc.args != nil {
				args = tc.args(args)
			}
			code, output := runTool(t, f.sys(t), args...)
			wantRun(t, code, output, exitDidNotRun, tc.want)
		})
	}
}

func TestGenerateRejectsABadSpec(t *testing.T) {
	content := moduleZip(t, fixtureModule())
	f := newFakeGo(t, content, content)
	args, _ := generateArgs(t)
	if err := os.WriteFile(args[3], []byte(`{"paths": {}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, output := runTool(t, f.sys(t), args...)
	wantRun(t, code, output, exitDidNotRun, "DID NOT RUN: the API spec has no operation")
	if f.zipRequests.Load() != 0 {
		t.Error("the zip was downloaded for a spec that cannot be read")
	}
}

func TestSplitModule(t *testing.T) {
	path, version, err := splitModule("github.com/permitio/permit-golang@v1.2.8")
	if err != nil || path != "github.com/permitio/permit-golang" || version != "v1.2.8" {
		t.Errorf("splitModule = %q, %q, %v", path, version, err)
	}
	for arg, want := range map[string]string{
		"github.com/permitio/permit-golang":        "is not <module path>@<version>",
		"github.com/permitio/permit-golang@v1.2":   "must be a canonical semantic version",
		"github.com/permitio/permit-golang@1.2.8":  "@1.2.8\": ",
		"github.com/permitio/permit-golang@v2.0.0": "-module",
		"Bad Path@v1.0.0":                          "-module",
	} {
		_, _, err := splitModule(arg)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("splitModule(%q) error = %v, want one that contains %q", arg, err, want)
		}
	}
}

func TestReadModuleZip(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, files map[string]string) string {
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		for _, n := range slices.Sorted(maps.Keys(files)) {
			f, err := w.Create(n)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(f, files[n]); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	prefix := fixturePath + "@" + fixtureVersion + "/"
	files, err := readModuleZip(write("ok.zip", map[string]string{
		prefix + "go.mod": fixtureGoMod, prefix + "a/a.go": "package a\n",
		prefix + "README.md": "x\n",
	}), fixturePath, fixtureVersion)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].name != "a/a.go" || string(files[0].content) != "package a\n" {
		t.Errorf("readModuleZip = %+v, want a/a.go alone", files)
	}
	for name, tc := range map[string]struct {
		files map[string]string
		want  string
	}{
		"a file outside the module's directory": {
			files: map[string]string{"other@v1.0.0/a.go": "package a\n"},
			want:  "the module zip holds other@v1.0.0/a.go, outside example.com/sdk@v1.2.3/",
		},
		"a file over the limit": {
			files: map[string]string{prefix + "big.go": strings.Repeat("a", maxZipFile+1)},
			want:  "big.go in the module zip is over 8388608 bytes",
		},
	} {
		_, zipErr := readModuleZip(write(strings.ReplaceAll(name, " ", "-")+".zip", tc.files),
			fixturePath, fixtureVersion)
		if zipErr == nil || !strings.Contains(zipErr.Error(), tc.want) {
			t.Errorf("%s: readModuleZip error = %v, want one that contains %q", name, zipErr,
				tc.want)
		}
	}
	files, err = readModuleZip(write("limit.zip", map[string]string{
		prefix + "big.go": strings.Repeat("a", maxZipFile),
	}), fixturePath, fixtureVersion)
	if err != nil || len(files) != 1 || len(files[0].content) != maxZipFile {
		t.Errorf("a file of exactly the limit: %d files, %v", len(files), err)
	}
	_, err = readModuleZip(filepath.Join(dir, "none.zip"), fixturePath, fixtureVersion)
	if err == nil || !strings.Contains(err.Error(), "opening the module zip: ") {
		t.Errorf("readModuleZip of a missing file: error %v, want one that it cannot open it",
			err)
	}
}

func TestWithoutVars(t *testing.T) {
	got := withoutVars([]string{"A=1", "GONOSUMDB=x", "B=", "GOFLAGS=-mod=mod", "AB=2"},
		"GONOSUMDB", "GOFLAGS", "A")
	if want := []string{"B=", "AB=2"}; !slices.Equal(got, want) {
		t.Errorf("withoutVars = %q, want %q", got, want)
	}
}
