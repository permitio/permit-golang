package scripts

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// platforms are the platforms every tools/binaries.lock entry has a sha256 for.
var platforms = []string{"darwin/arm64", "darwin/amd64", "linux/amd64", "linux/arm64"}

// thisPlatform is the platform fetch-tool.sh downloads for on the machine that runs the tests.
var thisPlatform = runtime.GOOS + "/" + runtime.GOARCH

// releaseNames are the os.* and arch.* lines lockEntry writes. They give every Go OS and
// architecture a release name that differs from its Go name and from the other release names.
const releaseNames = "os.darwin = release-darwin\nos.linux = release-linux\n" +
	"arch.amd64 = release-amd64\narch.arm64 = release-arm64\n"

// releaseName returns the name releaseNames gives a Go OS or architecture.
func releaseName(goName string) string {
	return "release-" + goName
}

// tarGz returns a tar.gz archive that holds one file. The file's mode is 0644, so a test can
// tell that fetch-tool.sh made the binary executable.
func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	header := &tar.Header{
		Name: name, Mode: 0o644, Size: int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// lockEntry returns a tools/binaries.lock block for fake-tool 1.2.3 with the given url and
// binary, the lines in releaseNames, and the sha256 sum for each platform in sums.
func lockEntry(url, binary string, sums map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[fake-tool]\nversion = 1.2.3\nurl = %s\narchive = tar.gz\nbinary = %s\n%s",
		url, binary, releaseNames)
	for _, platform := range platforms {
		if sum, ok := sums[platform]; ok {
			fmt.Fprintf(&b, "sha256.%s = %s\n", platform, sum)
		}
	}
	return b.String()
}

// sumsFor returns sum as the sha256 for this machine's platform and a different sha256 for each
// other platform, and leaves out the platforms in without.
func sumsFor(sum string, without ...string) map[string]string {
	sums := map[string]string{}
	for _, platform := range platforms {
		switch {
		case slices.Contains(without, platform):
		case platform == thisPlatform:
			sums[platform] = sum
		default:
			sums[platform] = sha256Hex([]byte("the archive for " + platform))
		}
	}
	return sums
}

// otherPlatform returns a platform in platforms that is not this machine's.
func otherPlatform() string {
	if platforms[0] == thisPlatform {
		return platforms[1]
	}
	return platforms[0]
}

// newRoot lays out a repository in a new directory, with fetch-tool.sh and lock as
// tools/binaries.lock, and returns the directory.
func newRoot(t *testing.T, lock string) string {
	t.Helper()
	root := t.TempDir()
	linkScripts(t, root, "fetch-tool.sh")
	writeFiles(t, root, map[string]string{"tools/binaries.lock": lock})
	return root
}

func TestFetchTool(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Fatalf("fetch-tool.sh supports darwin and linux; this test runs on %s", runtime.GOOS)
	}
	content := []byte("#!/bin/sh\necho fake-tool 1.2.3\n")
	osName, archName := releaseName(runtime.GOOS), releaseName(runtime.GOARCH)
	file := fmt.Sprintf("fake-tool_1.2.3_%s_%s.tar.gz", osName, archName)
	member := fmt.Sprintf("fake-tool-1.2.3-%s-%s", osName, archName)
	archive := tarGz(t, member+"/fake-tool", content)
	goFile := fmt.Sprintf("fake-tool_1.2.3_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	goArchive := tarGz(t,
		fmt.Sprintf("fake-tool-1.2.3-%s-%s/fake-tool", runtime.GOOS, runtime.GOARCH), content)
	tampered := bytes.Clone(archive)
	tampered[len(tampered)/2] ^= 0xff
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.2.3/"+file {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(archive)
	}))
	t.Cleanup(plain.Close)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request,
	) {
		switch r.URL.Path {
		case "/v1.2.3/" + file:
			_, _ = w.Write(archive)
		case "/v1.2.3/" + goFile:
			_, _ = w.Write(goArchive)
		case "/tampered/" + file:
			_, _ = w.Write(tampered)
		case "/redirect/" + file:
			http.Redirect(w, r, "/v1.2.3/"+file, http.StatusFound)
		case "/redirect-to-http/" + file:
			http.Redirect(w, r, plain.URL+"/v1.2.3/"+file, http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caFile, ca, 0o600); err != nil {
		t.Fatal(err)
	}

	repositoryLock, err := os.ReadFile(filepath.Join("..", "binaries.lock"))
	if err != nil {
		t.Fatal(err)
	}
	good := sha256Hex(archive)
	urlIn := func(dir string) string {
		return server.URL + "/" + dir + "/fake-tool_{version}_{os}_{arch}.tar.gz"
	}
	template := urlIn("v{version}")
	binary := "fake-tool-{version}-{os}-{arch}/fake-tool"
	tests := []struct {
		name       string
		lock       string
		setup      func(t *testing.T, root string)
		args       []string
		wantExit   int
		wantStderr string
	}{
		{
			name: "a correct download", lock: lockEntry(template, binary, sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 0,
			wantStderr: "fetch-tool: installed fake-tool 1.2.3 for " + thisPlatform,
		},
		{
			name: "a correct download after other entries and comments",
			lock: "# comment\n\n[other-tool]\nversion = 9\nunknown = ignored in other entries\n\n" +
				lockEntry(template, binary, sumsFor(good)) + "\n[last-tool]\nversion = 1\n",
			args: []string{"fake-tool"}, wantExit: 0,
		},
		{
			name: "a correct download with the Go names for the OS and architecture",
			lock: strings.Replace(lockEntry(template, binary, sumsFor(sha256Hex(goArchive))),
				releaseNames, "", 1),
			args: []string{"fake-tool"}, wantExit: 0,
		},
		{
			name: "indented comments, blank lines with spaces and trailing spaces",
			lock: "  # indented comment\n \t\n" + strings.Replace(
				lockEntry(template, binary, sumsFor(good)), "version = 1.2.3\n",
				"version = 1.2.3 \t\n", 1),
			args: []string{"fake-tool"}, wantExit: 0,
		},
		{
			name: "a correct download over an installed version",
			lock: lockEntry(template, binary, sumsFor(good)),
			setup: func(t *testing.T, root string) {
				t.Helper()
				bin := filepath.Join(root, "bin")
				if err := os.MkdirAll(bin, 0o755); err != nil {
					t.Fatal(err)
				}
				old := []byte("#!/bin/sh\necho fake-tool 1.2.2\n")
				if err := os.WriteFile(filepath.Join(bin, "fake-tool"), old, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			args: []string{"fake-tool"}, wantExit: 0,
		},
		{
			name: "a download that redirects", lock: lockEntry(urlIn("redirect"), binary,
				sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 0,
		},
		{
			name: "a download that redirects to http",
			lock: lockEntry(urlIn("redirect-to-http"), binary, sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: downloading " + server.URL + "/redirect-to-http/" + file +
				" failed",
		},
		{
			name: "a tampered download",
			lock: lockEntry(urlIn("tampered"), binary, sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: sha256 mismatch for " + server.URL + "/tampered/" + file +
				": got " + sha256Hex(tampered) + ", want " + good,
		},
		{
			name: "an unknown name", lock: lockEntry(template, binary, sumsFor(good)),
			args: []string{"other-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: tools/binaries.lock has no [other-tool] entry",
		},
		{
			name: "an unknown name in the repository's lock", lock: string(repositoryLock),
			args: []string{"no-such-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: tools/binaries.lock has no [no-such-tool] entry",
		},
		{
			name: "no lock",
			lock: lockEntry(template, binary, sumsFor(good)),
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Remove(filepath.Join(root, "tools", "binaries.lock")); err != nil {
					t.Fatal(err)
				}
			},
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: tools/binaries.lock is missing or not readable",
		},
		{
			name: "no sha256 for this platform",
			lock: lockEntry(template, binary, sumsFor(good, thisPlatform)),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: [fake-tool] has no sha256." + thisPlatform,
		},
		{
			name: "no sha256 for another platform",
			lock: lockEntry(template, binary, sumsFor(good, otherPlatform())),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: [fake-tool] has no sha256." + otherPlatform(),
		},
		{
			name: "a platform the script does not know",
			lock: lockEntry(template, binary, sumsFor(good)) + "sha256.windows/amd64 = " + good,
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: tools/binaries.lock:14: unknown key sha256.windows/amd64",
		},
		{
			name: "a sha256 that is not 64 lowercase hex digits",
			lock: lockEntry(template, binary, sumsFor(strings.ToUpper(good))),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "is not 64 lowercase hex digits",
		},
		{
			name: "a download that fails",
			lock: lockEntry(server.URL+"/missing/{version}.tar.gz", binary, sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: downloading " + server.URL + "/missing/1.2.3.tar.gz failed",
		},
		{
			name: "a URL that is not https",
			lock: lockEntry(strings.Replace(template, "https://", "http://", 1), binary,
				sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "is not an https URL",
		},
		{
			name: "an unknown placeholder in the URL",
			lock: lockEntry(template+"?{tag}", binary, sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: [fake-tool] url " + server.URL + "/v1.2.3/" + file +
				"?{tag} has an unknown placeholder",
		},
		{
			name: "an unknown placeholder in the binary path",
			lock: lockEntry(template, binary+"{tag}", sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: [fake-tool] binary " + member +
				"/fake-tool{tag} has an unknown placeholder",
		},
		{
			name: "a binary path the archive does not have",
			lock: lockEntry(template, "fake-tool", sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: extracting fake-tool from",
		},
		{
			name: "a binary path that names a directory",
			lock: lockEntry(template, "fake-tool-{version}-{os}-{arch}", sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: extracting " + member + " from " + server.URL + "/v1.2.3/" +
				file + " failed: not a regular file",
		},
		{
			name: "a bin/fake-tool that is a directory",
			lock: lockEntry(template, binary, sumsFor(good)),
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(root, "bin", "fake-tool"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: bin/fake-tool exists and is not a regular file",
		},
		{
			name: "an archive type the script does not unpack",
			lock: strings.Replace(lockEntry(template, binary, sumsFor(good)),
				"archive = tar.gz\n", "archive = zip\n", 1),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: [fake-tool] archive zip is not supported; want tar.gz",
		},
		{
			name: "an entry without a version",
			lock: strings.Replace(lockEntry(template, binary, sumsFor(good)), "version = 1.2.3\n",
				"", 1),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: [fake-tool] has no version",
		},
		{
			name: "a key set twice",
			lock: lockEntry(template, binary, sumsFor(good)) + "version = 1.2.4\n",
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: tools/binaries.lock:14: version is set twice in [fake-tool]",
		},
		{
			name: "an entry named twice",
			lock: lockEntry(template, binary, sumsFor(good)) + "[fake-tool]\n",
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: tools/binaries.lock:14: [fake-tool] appears twice",
		},
		{
			name: "a key outside an entry",
			lock: "version = 1\n" + lockEntry(template, binary, sumsFor(good)),
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: tools/binaries.lock:1: version = 1 is outside an entry",
		},
		{
			name: "a line that is not an entry or a key",
			lock: lockEntry(template, binary, sumsFor(good)) + "not a key\n",
			args: []string{"fake-tool"}, wantExit: 2,
			wantStderr: "fetch-tool: tools/binaries.lock:14: want [name] or key = value",
		},
		{
			name: "no name", lock: lockEntry(template, binary, sumsFor(good)), wantExit: 2,
			wantStderr: "fetch-tool: usage: fetch-tool.sh <name>",
		},
		{
			name: "a name that is a path", lock: lockEntry(template, binary, sumsFor(good)),
			args: []string{"../fake-tool"}, wantExit: 2,
			wantStderr: `fetch-tool: "../fake-tool" is not a tool name`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := newRoot(t, tt.lock)
			if tt.setup != nil {
				tt.setup(t, root)
			}
			tmp := t.TempDir()
			cmd := exec.CommandContext(t.Context(),
				filepath.Join(root, ".github", "scripts", "fetch-tool.sh"), tt.args...)
			cmd.Env = append(os.Environ(), "CURL_CA_BUNDLE="+caFile, "TMPDIR="+tmp,
				"NO_PROXY=127.0.0.1", "no_proxy=127.0.0.1")
			var stdout, stderr strings.Builder
			cmd.Stdout, cmd.Stderr = &stdout, &stderr

			err := cmd.Run()

			exit := 0
			if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
				exit = exitErr.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			if exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stderr:\n%s", exit, tt.wantExit, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr does not contain %q:\n%s", tt.wantStderr, stderr.String())
			}
			installed := filepath.Join(root, "bin", "fake-tool")
			if tt.wantExit == 0 {
				assertInstalled(t, installed, content)
				if want := installed + "\n"; stdout.String() != want {
					t.Errorf("stdout = %q, want %q", stdout.String(), want)
				}
			} else {
				assertNothingInstalled(t, filepath.Join(root, "bin"))
				if stdout.Len() > 0 {
					t.Errorf("stdout = %q, want nothing after a failed fetch", stdout.String())
				}
			}
			if left, _ := os.ReadDir(tmp); len(left) > 0 {
				t.Errorf("fetch-tool.sh left %d files in TMPDIR", len(left))
			}
		})
	}
}

func assertInstalled(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s holds %q, want %q", path, got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 != 0o111 {
		t.Errorf("%s has mode %v, want it executable", path, info.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("bin/ holds %d files, want only the installed binary", len(entries))
	}
}

// assertNothingInstalled checks that the bin directory, if there is one, holds no files.
func assertNothingInstalled(t *testing.T, bin string) {
	t.Helper()
	err := filepath.WalkDir(bin, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			t.Errorf("%s exists after a failed fetch", path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}
