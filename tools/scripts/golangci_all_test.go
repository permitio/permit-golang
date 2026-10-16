package scripts

import (
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// golangciStub is a stand-in for golangci-lint. It appends its directory and arguments to the
// file STUB_LOG names, prints its version, 2.14.0, for version --short, and exits with the code
// that STUB_EXITS gives the step, as space-separated "<step>=<code>" pairs. A step is "verify",
// "fmt", or "run:<directory name>".
const golangciStub = `#!/bin/sh
printf '%s|%s\n' "$PWD" "$*" >>"$STUB_LOG"
case "$1" in
version) echo 2.14.0; exit 0 ;;
config) step=verify ;;
fmt) step=fmt ;;
run) step="run:${PWD##*/}" ;;
*) exit 99 ;;
esac
for pair in $STUB_EXITS; do
	if [ "${pair%%=*}" = "$step" ]; then exit "${pair#*=}"; fi
done
exit 0
`

func TestGolangciAll(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Fatalf("fetch-tool.sh supports darwin and linux; this test runs on %s", runtime.GOOS)
	}
	archiveName := fmt.Sprintf("golangci-lint-2.14.0-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	binary := fmt.Sprintf("golangci-lint-2.14.0-%s-%s/golangci-lint", runtime.GOOS, runtime.GOARCH)
	archive := tarGz(t, binary, []byte(golangciStub))
	// otherArchive is laid out like the 2.14.0 release, but its binary reports 2.13.0.
	otherArchive := tarGz(t, binary,
		[]byte(strings.Replace(golangciStub, "echo 2.14.0", "echo 2.13.0", 1)))
	archives := map[string][]byte{"/" + archiveName: archive, "/other.tar.gz": otherArchive}
	lockFor := func(url string, release []byte) string {
		var b strings.Builder
		fmt.Fprintf(&b, "[golangci-lint]\nversion = 2.14.0\nurl = %s\narchive = tar.gz\n"+
			"binary = golangci-lint-{version}-{os}-{arch}/golangci-lint\n", url)
		for platform, sum := range sumsFor(sha256Hex(release)) {
			fmt.Fprintf(&b, "sha256.%s = %s\n", platform, sum)
		}
		return b.String()
	}
	// releaseLock is the lock of the 2.14.0 release that the archive server at url serves.
	releaseLock := func(url string) string {
		return lockFor(url+"/golangci-lint-{version}-{os}-{arch}.tar.gz", archive)
	}
	// modules are the tracked go.mod files of a repository with a nested module, and a module
	// under testdata that the script must not lint.
	modules := map[string]string{
		"go.mod":                        "module example.com/root\n",
		"tools/go.mod":                  "module example.com/root/tools\n",
		"tools/testdata/fixture/go.mod": "module example.com/fixture\n",
	}
	// installed are the stubs that a repository links as bin/golangci-lint, by their version.
	// fetch-tool.sh replaces the link, not the stub, when it installs another binary.
	installed := map[string]string{}
	for _, version := range []string{"2.14.0", "2.13.0"} {
		installed[version] = writeStub(t, "golangci-lint",
			strings.Replace(golangciStub, "echo 2.14.0", "echo "+version, 1))
	}
	stubInstalled := func(version string) func(t *testing.T, root string) {
		return func(t *testing.T, root string) {
			t.Helper()
			linkFile(t, root, "bin/golangci-lint", installed[version])
		}
	}
	// allSteps are the steps the script runs on the modules above, in order.
	allSteps := []string{
		"version --short", "config verify", "fmt --diff", "run ./...",
		"tools: run ./...",
	}
	tests := []struct {
		name string
		// files are written into the repository; the lock, modules and .golangci.yml are added
		// unless noDefaults is set. lock returns the lock for the URL of the subtest's archive
		// server, releaseLock when nil.
		files      map[string]string
		lock       func(url string) string
		noDefaults bool
		// noRepository leaves out git init, and untracked names files git must not track.
		noRepository  bool
		untracked     []string
		setup         func(t *testing.T, root string)
		args          []string
		exits         string
		wantExit      int
		wantStderr    []string
		wantSteps     []string
		wantDownloads int32
	}{
		{
			name: "every step passes", setup: stubInstalled("2.14.0"), wantExit: 0,
			wantStderr: []string{
				"golangci-all: golangci-lint 2.14.0: verifying .golangci.yml",
				"golangci-all: linting the module in tools",
				"golangci-all: linted 2 modules; exit 0",
			},
			wantSteps: allSteps,
		},
		{
			name: "no installed golangci-lint", wantExit: 0, wantDownloads: 1,
			wantStderr: []string{"golangci-all: linted 2 modules; exit 0"},
			wantSteps:  allSteps,
		},
		{
			name: "an installed golangci-lint of another version", setup: stubInstalled("2.13.0"),
			wantExit: 0, wantDownloads: 1,
			wantStderr: []string{"golangci-all: linted 2 modules; exit 0"},
			wantSteps:  slices.Concat([]string{"version --short"}, allSteps),
		},
		{
			name: "an issue in the nested module", setup: stubInstalled("2.14.0"),
			exits: "run:tools=1", wantExit: 1,
			wantStderr: []string{"golangci-all: linted 2 modules; exit 1"},
			wantSteps:  allSteps,
		},
		{
			name: "a file that is not formatted", setup: stubInstalled("2.14.0"), exits: "fmt=1",
			wantExit: 1, wantSteps: allSteps,
		},
		{
			name: "a module with no Go files", setup: stubInstalled("2.14.0"), exits: "run:tools=5",
			wantExit: 2,
			wantStderr: []string{
				"golangci-all: golangci-lint run ./... in tools exited 5, so it did not finish",
				"golangci-all: linted 2 modules; exit 2",
			},
			wantSteps: allSteps,
		},
		{
			name:  "an issue in one module and a failure in the other",
			setup: stubInstalled("2.14.0"), exits: "fmt=3 run:tools=1", wantExit: 2,
			wantSteps: allSteps,
		},
		{
			name:     "a configuration that does not verify",
			setup:    stubInstalled("2.14.0"),
			exits:    "verify=1",
			wantExit: 2,
			wantStderr: []string{
				"golangci-all: golangci-lint config verify rejected .golangci.yml",
			},
			wantSteps: allSteps[:2],
		},
		{
			name:  "a lock version with no spaces around = and spaces after it",
			setup: stubInstalled("2.14.0"),
			lock: func(url string) string {
				return strings.Replace(releaseLock(url), "version = 2.14.0\n",
					"version=2.14.0  \n", 1)
			},
			wantExit:   0,
			wantStderr: []string{"golangci-all: golangci-lint 2.14.0: verifying .golangci.yml"},
			wantSteps:  allSteps,
		},
		{
			name:     "a download that fails",
			wantExit: 2,
			lock: func(url string) string {
				return lockFor(url+"/missing.tar.gz", archive)
			},
			wantStderr: []string{"golangci-all: installing golangci-lint 2.14.0 with " +
				".github/scripts/fetch-tool.sh failed"},
		},
		{
			name: "a download that reports another version", wantExit: 2, wantDownloads: 1,
			lock: func(url string) string {
				return lockFor(url+"/other.tar.gz", otherArchive)
			},
			wantStderr: []string{"golangci-all: bin/golangci-lint is version 2.13.0, want " +
				"2.14.0 from tools/binaries.lock"},
			wantSteps: []string{"version --short"},
		},
		{
			name:  "a module that git tracks and the working tree does not have",
			files: map[string]string{"examples/demo/go.mod": "module example.com/demo\n"},
			setup: func(t *testing.T, root string) {
				t.Helper()
				stubInstalled("2.14.0")(t, root)
				if err := os.RemoveAll(filepath.Join(root, "examples")); err != nil {
					t.Fatal(err)
				}
			},
			wantExit: 2,
			wantStderr: []string{"golangci-all: git tracks examples/demo/go.mod, but the " +
				"working tree does not have it"},
			wantSteps: allSteps[:1],
		},
		{
			name: "no go.mod at the root that git tracks", setup: stubInstalled("2.14.0"),
			untracked: []string{"go.mod"}, wantExit: 2,
			wantStderr: []string{"golangci-all: git lists no go.mod at the repository root"},
			wantSteps:  allSteps[:1],
		},
		{
			name: "not a git repository", setup: stubInstalled("2.14.0"), noRepository: true,
			wantExit:   2,
			wantStderr: []string{"golangci-all: listing the go.mod files git tracks failed"},
			wantSteps:  allSteps[:1],
		},
		{
			name: "no .golangci.yml", noDefaults: true, setup: stubInstalled("2.14.0"),
			files: map[string]string{
				"go.mod":              modules["go.mod"],
				"tools/binaries.lock": releaseLock("https://127.0.0.1:9"),
			},
			wantExit:   2,
			wantStderr: []string{"golangci-all: .golangci.yml is missing"},
		},
		{
			name:     "no golangci-lint in the lock",
			setup:    stubInstalled("2.14.0"),
			files:    map[string]string{"tools/binaries.lock": "[other]\nversion = 2.14.0\n"},
			wantExit: 2,
			wantStderr: []string{
				"golangci-all: tools/binaries.lock has no version for golangci-lint",
			},
		},
		{
			name: "no lock", noDefaults: true, setup: stubInstalled("2.14.0"),
			files:      map[string]string{".golangci.yml": "version: \"2\"\n"},
			wantExit:   2,
			wantStderr: []string{"golangci-all: reading tools/binaries.lock failed"},
		},
		{
			name: "arguments", setup: stubInstalled("2.14.0"), args: []string{"./..."},
			wantExit:   2,
			wantStderr: []string{"golangci-all: usage: golangci-all.sh (it takes no arguments)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := startArchiveServer(t, archives)
			lock := releaseLock
			if tt.lock != nil {
				lock = tt.lock
			}
			root := t.TempDir()
			linkScripts(t, root, "golangci-all.sh", "fetch-tool.sh")
			if !tt.noDefaults {
				writeFiles(t, root, modules)
				writeFiles(t, root, map[string]string{
					".golangci.yml": "version: \"2\"\n", "tools/binaries.lock": lock(server.url),
				})
			}
			writeFiles(t, root, tt.files)
			if !tt.noRepository {
				gitInit(t, root)
			}
			for _, name := range tt.untracked {
				result := runScript(t, root, gitEnv(root), "git", "rm", "--cached", "--quiet", name)
				if result.exit != 0 {
					t.Fatalf("git rm --cached %s: %s", name, result.stderr)
				}
			}
			if tt.setup != nil {
				tt.setup(t, root)
			}
			stubLog := filepath.Join(t.TempDir(), "stub.log")
			env := slices.Concat(gitEnv(root), []string{
				"STUB_LOG=" + stubLog, "STUB_EXITS=" + tt.exits, "CURL_CA_BUNDLE=" + server.caFile,
				"TMPDIR=" + t.TempDir(), "NO_PROXY=127.0.0.1", "no_proxy=127.0.0.1",
			})

			result := runScript(t, root, env,
				filepath.Join(root, ".github", "scripts", "golangci-all.sh"), tt.args...)

			if result.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stderr:\n%s", result.exit, tt.wantExit, result.stderr)
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(result.stderr, want) {
					t.Errorf("stderr does not contain %q:\n%s", want, result.stderr)
				}
			}
			if got := stubSteps(t, stubLog, root); !slices.Equal(got, tt.wantSteps) {
				t.Errorf("golangci-lint ran\n%q\nwant\n%q", got, tt.wantSteps)
			}
			if got := server.downloads.Load(); got != tt.wantDownloads {
				t.Errorf("downloaded golangci-lint %d times, want %d", got, tt.wantDownloads)
			}
		})
	}
}

// archiveServer is a TLS server of release archives: its URL, the file of its certificate for
// CURL_CA_BUNDLE, and how many archives it served.
type archiveServer struct {
	url, caFile string
	downloads   atomic.Int32
}

// startArchiveServer starts an archiveServer that serves archives, a map from a URL path to an
// archive, until the test ends.
func startArchiveServer(t *testing.T, archives map[string][]byte) *archiveServer {
	t.Helper()
	s := &archiveServer{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request,
	) {
		served, ok := archives[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.downloads.Add(1)
		_, _ = w.Write(served)
	}))
	t.Cleanup(server.Close)
	s.url = server.URL
	s.caFile = filepath.Join(t.TempDir(), "ca.pem")
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(s.caFile, ca, 0o600); err != nil {
		t.Fatal(err)
	}
	return s
}

// stubSteps reads the stub's log and returns each call as its arguments without the --config
// flag, prefixed with "<dir>: " when it ran in a directory other than root. It fails the test
// when a call other than version --short does not end with --config and root's .golangci.yml.
func stubSteps(t *testing.T, stubLog, root string) []string {
	t.Helper()
	data, err := os.ReadFile(stubLog)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	config := " --config " + filepath.Join(root, ".golangci.yml")
	var steps []string
	for line := range strings.Lines(string(data)) {
		dir, args, _ := strings.Cut(strings.TrimSuffix(line, "\n"), "|")
		step, hasConfig := strings.CutSuffix(args, config)
		if !hasConfig && step != "version --short" {
			t.Errorf("golangci-lint %s ran without%s", args, config)
		}
		if dir, err = filepath.EvalSymlinks(dir); err != nil {
			t.Fatal(err)
		}
		if rel, err := filepath.Rel(realRoot, dir); err == nil && rel != "." {
			step = rel + ": " + step
		}
		steps = append(steps, step)
	}
	return steps
}
