package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// now is the time every test runs at; an accept entry whose last day is 2026-10-12 has expired.
var now = time.Date(2026, 10, 13, 15, 4, 5, 0, time.UTC)

const (
	sdkModule   = "github.com/permitio/permit-golang/v2\n"
	toolsModule = "github.com/permitio/permit-golang/v2/tools\ngo.yaml.in/yaml/v3\n"
	plantModule = "example.com/plant/xnet\ngolang.org/x/net\n"
	// ceiling is the -ceiling directory, relative to the temporary repository root.
	ceiling = "ceiling"
	// listed is git ls-files -z output for a repository with the root and tools modules.
	listed = "go.mod\x00tools/go.mod\x00"
)

// result is what a fake command returns.
type result struct {
	out output
	err error
}

// call is one command a fake ran.
type call struct {
	dir, name string
	args      []string
}

// fake stands in for git, go list and govulncheck. It answers git with git, go with goList by
// directory, and every other program, which is govulncheck, with scans by directory, and records
// every command it gets.
type fake struct {
	git    result
	goList map[string]result
	scans  map[string]result
	calls  []call
}

func (f *fake) command(_ context.Context, dir, name string, args ...string) (output, error) {
	f.calls = append(f.calls, call{dir: dir, name: name, args: args})
	r, ok := f.git, true
	switch name {
	case "git":
	case "go":
		r, ok = f.goList[dir]
	default:
		r, ok = f.scans[dir]
	}
	if !ok {
		return output{stderr: []byte("no fake result for " + dir), code: 1}, nil
	}
	return r.out, r.err
}

// stdout returns a result whose command printed data and exited 0.
func stdout(data []byte) result {
	return result{out: output{stdout: data}}
}

// testdata is the absolute path of the fixtures, taken before a test changes the working directory.
var testdata = func() string {
	dir, err := filepath.Abs("testdata")
	if err != nil {
		panic(err)
	}
	return dir
}()

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdata, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// rewrite decodes a govulncheck JSON stream and encodes it again with each message passed through
// change, which edits the message in place or returns false to drop it.
func rewrite(t *testing.T, data []byte, change func(message map[string]any) bool) []byte {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	for {
		var message map[string]any
		err := decoder.Decode(&message)
		if errors.Is(err, io.EOF) {
			return out.Bytes()
		}
		if err != nil {
			t.Fatal(err)
		}
		if change(message) {
			if err := encoder.Encode(message); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// withoutFinding returns the fixture without the findings of the vulnerability id.
func withoutFinding(t *testing.T, name, id string) []byte {
	t.Helper()
	return rewrite(t, fixture(t, name), func(message map[string]any) bool {
		f, ok := message["finding"].(map[string]any)
		return !ok || f["osv"] != id
	})
}

// newFake returns a fake for a repository whose three trees are clean: the floor and the ceiling
// have only a standard library finding, and tools two.
func newFake(t *testing.T) *fake {
	t.Helper()
	clean := fixture(t, "floor-clean.jsonl")
	return &fake{
		git: stdout([]byte(listed)),
		goList: map[string]result{
			".":     stdout([]byte(sdkModule)),
			ceiling: stdout([]byte(sdkModule)),
			"tools": stdout([]byte(toolsModule)),
		},
		scans: map[string]result{
			".":     stdout(clean),
			ceiling: stdout(clean),
			"tools": stdout(fixture(t, "tools-clean.jsonl")),
		},
	}
}

// withPlant makes the floor the planted module that requires golang.org/x/net v0.23.0.
func (f *fake) withPlant(t *testing.T) *fake {
	t.Helper()
	f.goList["."] = stdout([]byte(plantModule))
	f.scans["."] = stdout(fixture(t, "floor-xnet.jsonl"))
	return f
}

// repository writes the files vulnscan reads into a new directory and makes it the working
// directory: the ceiling's go.mod, a govulncheck binary, and the accept and trees files with the
// given lines. It returns the path of the govulncheck binary.
func repository(t *testing.T, accept, trees []string) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	files := map[string]string{
		"ceiling/go.mod":              "module github.com/permitio/permit-golang/v2\n",
		"bin/govulncheck":             "#!/bin/sh\nexit 99\n",
		".github/vulnscan-accept.txt": "# accepted\n\n" + strings.Join(accept, "\n") + "\n",
		".github/vulnscan-trees.txt":  "# not scanned\n\n" + strings.Join(trees, "\n") + "\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(root, "bin", "govulncheck")
}

// runWith runs vulnscan with the fake and returns its exit code and output.
func runWith(t *testing.T, f *fake, args ...string) (int, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	code := run(t.Context(), args, &out, &stderr, system{now: now, command: f.command})
	t.Logf("exit %d, output:\n%s%s", code, out.String(), stderr.String())
	return code, out.String()
}

// verdicts is the last line vulnscan prints for each exit code.
var verdicts = map[int]string{
	exitPassed:    "vulnscan: PASS\n",
	exitFailed:    "vulnscan: FAIL\n",
	exitDidNotRun: "vulnscan: DID NOT RUN\n",
}

func assertOutput(t *testing.T, code, wantCode int, out string, want ...string) {
	t.Helper()
	if code != wantCode {
		t.Errorf("exit %d, want %d", code, wantCode)
	}
	if !strings.HasSuffix(out, verdicts[wantCode]) {
		t.Errorf("output does not end with %q", verdicts[wantCode])
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output does not contain %q", w)
		}
	}
}

func TestScanWithoutFindingsPasses(t *testing.T) {
	govulncheck := repository(t, nil, nil)
	f := newFake(t)

	code, out := runWith(t, f, "-ceiling", ceiling)

	assertOutput(t, code, exitPassed, out,
		"vulnscan: floor: 2 modules, among them the 1 that go list -deps ./... lists; "+
			"govulncheck v1.8.0, go1.27.1, database of 2026-10-08T22:31:09Z\n",
		"vulnscan: tools: 3 modules, among them the 2 that go list -deps ./... tool lists;",
		"\nGO-2026-6599  reported only, the standard library, required, not imported\n"+
			"    stdlib in floor go1.27.1, fixed in go1.27.2; ceiling go1.27.1, fixed in "+
			"go1.27.2; tools go1.27.1, fixed in go1.27.2\n",
		"\nGO-2026-6604  reported only, the standard library, called\n"+
			"    stdlib in tools go1.27.1, fixed in go1.27.2\n",
		"    calls: os.doInRoot[int], os.doInRoot[os.FileInfo]\n",
		"\nvulnscan: 0 to fix, 0 accepted, 0 expired and 0 stale accept entries, 2 in the "+
			"standard library (reported only); 3 of 3 trees scanned\nvulnscan: PASS\n")
	// The tools tree adds the tool pattern, the programs its tool directives pin. The root and
	// the ceiling have no tool directive, so they never get it.
	golist := []string{"list", "-deps", "-f", "{{with .Module}}{{.Path}}{{end}}", "./..."}
	scan := []string{"-mode", "source", "-scan", "symbol", "-format", "json", "./..."}
	want := []call{
		{dir: ".", name: "git", args: []string{"ls-files", "-z", "--", "go.mod", "*/go.mod"}},
		{dir: ".", name: "go", args: golist},
		{dir: ".", name: govulncheck, args: scan},
		{dir: ceiling, name: "go", args: golist},
		{dir: ceiling, name: govulncheck, args: scan},
		{dir: "tools", name: "go", args: append(slices.Clone(golist), "tool")},
		{dir: "tools", name: govulncheck, args: append(slices.Clone(scan), "tool")},
	}
	if !slices.EqualFunc(f.calls, want, func(a, b call) bool {
		return a.dir == b.dir && a.name == b.name && slices.Equal(a.args, b.args)
	}) {
		t.Errorf("commands:\n%q\nwant:\n%q", f.calls, want)
	}
}

func TestFindings(t *testing.T) {
	const (
		reason   = "no package imports the vulnerable code"
		until    = "2026-12-31"
		accept03 = "GO-2025-3503 golang.org/x/net " + reason + " " + until
		accept66 = "GO-2026-6603 golang.org/x/net " + reason + " " + until
		xnetHTML = "    Non-linear parsing of case-insensitive content in golang.org/x/net/html\n"
	)
	withoutCalled := func(t *testing.T) []byte {
		t.Helper()
		return withoutFinding(t, "floor-xnet.jsonl", "GO-2024-3333")
	}
	clean := func(t *testing.T) []byte {
		t.Helper()
		return fixture(t, "floor-clean.jsonl")
	}
	tests := []struct {
		name string
		// floor is the floor's govulncheck output, with its exit code, when set; the planted
		// module's otherwise. modules is what go list prints for the floor when set. tools is
		// the tools tree's govulncheck output when set.
		floor    func(t *testing.T) []byte
		code     int
		modules  string
		tools    func(t *testing.T) []byte
		accept   []string
		wantExit int
		want     []string
	}{
		{
			name: "every finding outside the standard library fails", wantExit: exitFailed,
			want: []string{
				"\nGO-2024-3333  FAIL, called, not accepted\n" +
					"    golang.org/x/net in floor v0.23.0, fixed in v0.33.0\n" + xnetHTML +
					"    calls: html.Parse\n",
				"\nGO-2025-3503  FAIL, required, not imported, not accepted\n" +
					"    golang.org/x/net in floor v0.23.0, fixed in v0.36.0\n",
				"\nGO-2026-6603  FAIL, required, not imported, not accepted\n" +
					"    golang.org/x/net in floor v0.23.0, fixed in v0.60.0\n",
				"\nGO-2026-6603  reported only, the standard library, required, not imported\n" +
					"    stdlib in floor go1.26.5, fixed in go1.26.9\n",
				"\nGO-2026-6604  reported only, the standard library, called\n" +
					"    stdlib in floor go1.26.5, fixed in go1.26.9; tools go1.27.1, fixed in " +
					"go1.27.2\n",
				"\nvulnscan: 3 to fix, 0 accepted, 0 expired and 0 stale accept entries, 3 in " +
					"the standard library (reported only); 3 of 3 trees scanned\n",
			},
		},
		{
			name: "accepted findings are printed with their reasons", floor: withoutCalled,
			accept: []string{accept03, accept66}, wantExit: exitPassed,
			want: []string{
				"\nGO-2025-3503  accepted, required, not imported\n" +
					"    golang.org/x/net in floor v0.23.0, fixed in v0.36.0\n" +
					"    HTTP Proxy bypass using IPv6 Zone IDs in golang.org/x/net\n" +
					"    accepted until 2026-12-31: " + reason + "\n",
				"\nGO-2026-6603  accepted, required, not imported\n",
				"vulnscan: 0 to fix, 2 accepted, 0 expired and 0 stale accept entries",
			},
		},
		{
			name:   "called code cannot be accepted",
			accept: []string{"GO-2024-3333 golang.org/x/net " + reason + " " + until},
			want: []string{
				"\nGO-2024-3333  FAIL, called; the accept entry does not apply: called code " +
					"cannot be accepted\n",
				"vulnscan: 3 to fix, 0 accepted, 0 expired and 0 stale accept entries",
			},
			wantExit: exitFailed,
		},
		{
			// The trees are scanned in the order floor, ceiling, tools: a later tree that only
			// requires the module must not lower the level an earlier tree found.
			name: "code called in one tree and only required in another cannot be accepted",
			tools: func(t *testing.T) []byte {
				t.Helper()
				return append(fixture(t, "tools-clean.jsonl"), `{"finding":{"osv":"GO-2024-3333",`+
					`"fixed_version":"v0.33.0","trace":[{"module":"golang.org/x/net",`+
					`"version":"v0.23.0"}]}}`+"\n"...)
			},
			accept: []string{
				accept03, accept66, "GO-2024-3333 golang.org/x/net " + reason + " " + until,
			},
			wantExit: exitFailed,
			want: []string{
				"\nGO-2024-3333  FAIL, called; the accept entry does not apply: called code " +
					"cannot be accepted\n" +
					"    golang.org/x/net in floor v0.23.0, fixed in v0.33.0; tools v0.23.0, " +
					"fixed in v0.33.0\n" + xnetHTML + "    calls: html.Parse\n",
				"vulnscan: 1 to fix, 2 accepted, 0 expired and 0 stale accept entries",
			},
		},
		{
			name: "an entry for another module accepts nothing and is stale", floor: withoutCalled,
			accept:   []string{accept03, "GO-2026-6603 golang.org/x/text " + reason + " " + until},
			wantExit: exitFailed,
			want: []string{
				"\nGO-2026-6603  FAIL, required, not imported, not accepted\n",
				"\nGO-2026-6603  FAIL, stale accept entry: no tree reports it in " +
					"golang.org/x/text; remove it from .github/vulnscan-accept.txt\n",
				"vulnscan: 1 to fix, 1 accepted, 0 expired and 1 stale accept entries",
			},
		},
		{
			name: "an entry whose last day was yesterday fails", floor: withoutCalled,
			accept: []string{
				"GO-2025-3503 golang.org/x/net " + reason + " 2026-10-12", accept66,
			},
			wantExit: exitFailed,
			want: []string{
				"\nGO-2025-3503  FAIL, required, not imported, not accepted: its accept entry " +
					"expired on 2026-10-12\n",
				"\nGO-2025-3503  FAIL, the accept entry for golang.org/x/net expired on " +
					"2026-10-12; fix the vulnerability, or renew the entry with a reason that " +
					"still holds\n",
				"vulnscan: 1 to fix, 1 accepted, 1 expired and 0 stale accept entries",
			},
		},
		{
			name: "an entry whose last day is today applies", floor: withoutCalled,
			accept: []string{
				"GO-2025-3503 golang.org/x/net " + reason + " 2026-10-13", accept66,
			},
			wantExit: exitPassed,
			want:     []string{"accepted until 2026-10-13: " + reason + "\n"},
		},
		{
			name:  "an expired entry whose vulnerability is gone fails as expired",
			floor: clean, modules: sdkModule,
			accept:   []string{"GO-2025-3503 golang.org/x/net " + reason + " 2026-10-01"},
			wantExit: exitFailed,
			want: []string{
				"GO-2025-3503  FAIL, the accept entry for golang.org/x/net expired on 2026-10-01",
				"0 to fix, 0 accepted, 1 expired and 0 stale accept entries",
			},
		},
		{
			name: "an imported module that is not called is not accepted",
			floor: func(t *testing.T) []byte {
				t.Helper()
				return rewrite(
					t,
					fixture(t, "floor-xnet.jsonl"),
					func(message map[string]any) bool {
						f, ok := message["finding"].(map[string]any)
						if !ok {
							return true
						}
						trace, _ := f["trace"].([]any)
						top, _ := trace[0].(map[string]any)
						delete(top, "function")
						return true
					},
				)
			},
			accept:   []string{accept03, accept66},
			wantExit: exitFailed,
			want: []string{
				"\nGO-2024-3333  FAIL, imported, not called, not accepted\n" +
					"    golang.org/x/net in floor v0.23.0, fixed in v0.33.0\n" + xnetHTML + "\n",
				"\nGO-2026-6603  accepted, required, not imported\n",
				"1 to fix, 2 accepted, 0 expired and 0 stale accept entries",
			},
		},
		{
			name: "govulncheck exit 3 fails", wantExit: exitFailed, code: govulncheckFound,
			floor: clean, modules: sdkModule,
			want: []string{
				"vulnscan: floor: govulncheck exit 3: vulnerabilities found\n",
				"0 to fix, 0 accepted, 0 expired and 0 stale accept entries, 2 in the standard " +
					"library (reported only); 3 of 3 trees scanned\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository(t, tt.accept, nil)
			f := newFake(t).withPlant(t)
			if tt.floor != nil {
				f.scans["."] = result{out: output{stdout: tt.floor(t), code: tt.code}}
			}
			if tt.modules != "" {
				f.goList["."] = stdout([]byte(tt.modules))
			}
			if tt.tools != nil {
				f.scans["tools"] = stdout(tt.tools(t))
			}

			code, out := runWith(t, f, "-ceiling", ceiling)

			assertOutput(t, code, tt.wantExit, out, tt.want...)
		})
	}
}

func TestStaleEntry(t *testing.T) {
	const entry = "GO-2025-3503 golang.org/x/net fixed in a later commit 2026-12-31"
	t.Run("when every scan completed", func(t *testing.T) {
		repository(t, []string{entry}, nil)

		code, out := runWith(t, newFake(t), "-ceiling", ceiling)

		assertOutput(t, code, exitFailed, out,
			"\nGO-2025-3503  FAIL, stale accept entry: no tree reports it in golang.org/x/net; "+
				"remove it from .github/vulnscan-accept.txt\n",
			"0 to fix, 0 accepted, 0 expired and 1 stale accept entries")
	})
	t.Run("not when a scan did not complete", func(t *testing.T) {
		repository(t, []string{entry}, nil)
		f := newFake(t)
		f.scans[ceiling] = result{out: output{stderr: []byte("loading packages: boom"), code: 1}}

		code, out := runWith(t, f, "-ceiling", ceiling)

		assertOutput(t, code, exitDidNotRun, out,
			"vulnscan: ceiling: govulncheck exit 1: loading packages: boom\n",
			"\nvulnscan: accept entries not checked for staleness: not every scan completed\n",
			"0 to fix, 0 accepted, 0 expired and 0 stale accept entries, 2 in the standard "+
				"library (reported only); 2 of 3 trees scanned\n")
		if strings.Contains(out, "stale accept entry:") {
			t.Error("output calls an accept entry stale after a scan that did not complete")
		}
	})
}

func TestScanThatDidNotCompleteExits2(t *testing.T) {
	clean := func(t *testing.T) []byte {
		t.Helper()
		return fixture(t, "floor-clean.jsonl")
	}
	// Each of these returns the clean floor's output with one change.
	configAnd := func(keep string) func(t *testing.T) result {
		return func(t *testing.T) result {
			t.Helper()
			return stdout(rewrite(t, clean(t), func(message map[string]any) bool {
				_, isConfig := message["config"]
				_, isKept := message[keep]
				return isConfig || isKept
			}))
		}
	}
	setConfig := func(key, value string) func(t *testing.T) result {
		return func(t *testing.T) result {
			t.Helper()
			return stdout(rewrite(t, clean(t), func(message map[string]any) bool {
				if config, ok := message["config"].(map[string]any); ok {
					config[key] = value
				}
				return true
			}))
		}
	}
	findingWithout := func(field string) func(t *testing.T) result {
		return func(t *testing.T) result {
			t.Helper()
			return stdout(rewrite(t, clean(t), func(message map[string]any) bool {
				if f, ok := message["finding"].(map[string]any); ok {
					delete(f, field)
				}
				return true
			}))
		}
	}
	tests := []struct {
		name string
		// scan is the floor's govulncheck result, and list its go list result, when set.
		scan func(t *testing.T) result
		list *result
		want string
	}{
		{
			name: "govulncheck failed",
			scan: func(*testing.T) result {
				return result{out: output{stderr: []byte("loading packages: boom\n"), code: 1}}
			},
			want: "vulnscan: floor: govulncheck exit 1: loading packages: boom\n",
		},
		{
			name: "govulncheck could not be run",
			scan: func(*testing.T) result {
				return result{err: errors.New("signal: killed")}
			},
			want: "vulnscan: floor: could not run govulncheck: signal: killed\n",
		},
		{
			name: "empty output",
			scan: func(*testing.T) result { return result{} },
			want: "vulnscan: floor: govulncheck output has no config: it is empty or not -format " +
				"json\n",
		},
		{
			name: "truncated output",
			scan: func(t *testing.T) result {
				t.Helper()
				data := clean(t)
				return stdout(data[:len(data)/2])
			},
			want: "vulnscan: floor: govulncheck output is not complete JSON: unexpected EOF\n",
		},
		{
			name: "text output",
			scan: func(*testing.T) result { return stdout([]byte("No vulnerabilities found.\n")) },
			want: "vulnscan: floor: govulncheck output is not complete JSON: invalid character",
		},
		{
			name: "another scan mode",
			scan: setConfig("scan_mode", "binary"),
			want: "vulnscan: floor: govulncheck output is of scan mode \"binary\", want source\n",
		},
		{
			name: "another scan level",
			scan: setConfig("scan_level", "module"),
			want: "vulnscan: floor: govulncheck output is of scan level \"module\", want symbol\n",
		},
		{
			name: "no module list",
			scan: configAnd("osv"),
			want: "vulnscan: floor: govulncheck output has no module list: it was cut short\n",
		},
		{
			name: "no vulnerability database entries",
			scan: configAnd("SBOM"),
			want: "vulnscan: floor: govulncheck output has no vulnerability database entries: it " +
				"was cut short\n",
		},
		{
			name: "a finding without an ID",
			scan: findingWithout("osv"),
			want: "vulnscan: floor: govulncheck output has a finding without an ID or a trace\n",
		},
		{
			name: "a finding without a trace",
			scan: findingWithout("trace"),
			want: "vulnscan: floor: govulncheck output has a finding without an ID or a trace\n",
		},
		{
			name: "a module of the build missing from the module list",
			list: &result{out: output{stdout: []byte(sdkModule + "golang.org/x/text\n" +
				sdkModule + "golang.org/x/sys\n")}},
			want: "vulnscan: floor: govulncheck found 2 modules, but its module list lacks 2 of " +
				"the 3 that go list -deps ./... lists: golang.org/x/sys, golang.org/x/text\n",
		},
		{
			name: "go list failed",
			list: &result{out: output{stderr: []byte("go: updates to go.mod needed\n"), code: 1}},
			want: "vulnscan: floor: go list -deps ./... could not list the modules of the build: " +
				"exit 1: go: updates to go.mod needed\n",
		},
		{
			name: "go list could not be run",
			list: &result{err: errors.New(`exec: "go": executable file not found in $PATH`)},
			want: "vulnscan: floor: go list -deps ./... could not list the modules of the build: " +
				"exec: \"go\": executable file not found in $PATH\n",
		},
		{
			name: "go list lists no module",
			list: &result{out: output{stdout: []byte("\n\n")}},
			want: "vulnscan: floor: go list -deps ./... lists no module: the tree has no " +
				"packages to scan\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository(t, nil, nil)
			f := newFake(t)
			if tt.scan != nil {
				f.scans["."] = tt.scan(t)
			}
			if tt.list != nil {
				f.goList["."] = *tt.list
			}

			code, out := runWith(t, f, "-ceiling", ceiling)

			assertOutput(t, code, exitDidNotRun, out, tt.want, "2 of 3 trees scanned\n")
		})
	}
}

func TestIncompleteScanOutranksFindings(t *testing.T) {
	repository(t, nil, nil)
	f := newFake(t).withPlant(t)
	f.scans["tools"] = result{out: output{code: 2, stderr: []byte("bad flag")}}

	code, out := runWith(t, f, "-ceiling", ceiling)

	assertOutput(t, code, exitDidNotRun, out, "GO-2024-3333  FAIL, called, not accepted",
		"vulnscan: tools: govulncheck exit 2: bad flag\n", "3 to fix")
}

func TestModules(t *testing.T) {
	const testOnly = "examples/consumer a test-only module that never ships"
	tests := []struct {
		name     string
		git      result
		trees    []string
		wantExit int
		want     []string
	}{
		{
			name: "a module listed in the trees file", wantExit: exitPassed,
			git:   stdout([]byte(listed + "examples/consumer/go.mod\x00")),
			trees: []string{testOnly},
			want: []string{
				"vulnscan: modules: examples/consumer is not scanned: a test-only module that " +
					"never ships\n",
			},
		},
		{
			name: "a module under testdata", wantExit: exitPassed,
			git: stdout([]byte(listed + "tools/cmd/x/testdata/m/go.mod\x00testdata/go.mod\x00")),
		},
		{
			name: "a module that is neither scanned nor listed", wantExit: exitDidNotRun,
			git: stdout([]byte(listed + "examples/consumer/go.mod\x00")),
			want: []string{
				"vulnscan: modules: examples/consumer has a go.mod, but vulnscan does not scan " +
					"it and .github/vulnscan-trees.txt does not list it; list it there with the " +
					"reason it never ships, or scan it\n",
			},
		},
		{
			name: "a listed directory without a module", wantExit: exitFailed,
			trees: []string{testOnly},
			want: []string{
				"vulnscan: modules: FAIL, .github/vulnscan-trees.txt lists examples/consumer, " +
					"which has no go.mod that git tracks; remove the entry\n",
			},
		},
		{
			name: "no go.mod at the root", wantExit: exitDidNotRun,
			git: stdout([]byte("tools/go.mod\x00")),
			want: []string{
				"vulnscan: modules: git tracks no go.mod; vulnscan scans the root and tools/\n",
			},
		},
		{
			name: "no go.mod in tools", wantExit: exitDidNotRun,
			git: stdout([]byte("go.mod\x00")),
			want: []string{
				"vulnscan: modules: git tracks no tools/go.mod; vulnscan scans the root and " +
					"tools/\n",
			},
		},
		{
			name: "git failed", wantExit: exitDidNotRun,
			git: result{out: output{stderr: []byte("fatal: not a git repository\n"), code: 128}},
			want: []string{
				"vulnscan: modules: git ls-files could not list the go.mod files: exit 128: " +
					"fatal: not a git repository\n",
				"3 of 3 trees scanned",
			},
		},
		{
			name: "git could not be run", wantExit: exitDidNotRun,
			git: result{err: errors.New(`exec: "git": executable file not found in $PATH`)},
			want: []string{
				"vulnscan: modules: git ls-files could not list the go.mod files: exec: " +
					"\"git\": executable file not found in $PATH\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository(t, nil, tt.trees)
			f := newFake(t)
			if tt.git.out.stdout != nil || tt.git.out.code != 0 || tt.git.err != nil {
				f.git = tt.git
			}

			code, out := runWith(t, f, "-ceiling", ceiling)

			assertOutput(t, code, tt.wantExit, out, tt.want...)
		})
	}
}

func TestBadInputExits2WithoutScanning(t *testing.T) {
	const reason = "a reason 2026-12-31"
	tests := []struct {
		name string
		args []string
		// accept and trees are the lines of the accept and trees files.
		accept, trees []string
		// remove names a file of the repository to delete first.
		remove string
		want   string
	}{
		{name: "an argument", args: []string{"./..."}, want: `unexpected arguments ["./..."]`},
		{name: "an unknown flag", args: []string{"-mode", "binary"}, want: "-mode"},
		{name: "no ceiling", args: []string{}, want: "-ceiling is missing"},
		{
			name: "a ceiling without go.mod", args: []string{"-ceiling", "bin"},
			want: "-ceiling bin has no go.mod",
		},
		{
			name: "no govulncheck", remove: "bin/govulncheck",
			want: "govulncheck bin/govulncheck is missing; build it with go -C tools build -o " +
				"../bin/govulncheck golang.org/x/vuln/cmd/govulncheck",
		},
		{
			name: "no accept file", remove: ".github/vulnscan-accept.txt",
			want: "open .github/vulnscan-accept.txt: no such file or directory",
		},
		{
			name: "an accept entry without a last day", accept: []string{"GO-2026-1234 m why"},
			want: ".github/vulnscan-accept.txt:3: want an ID, a module, a reason and the last day",
		},
		{
			name: "an accept entry with a CVE", accept: []string{"CVE-2026-1 m " + reason},
			want: `"CVE-2026-1" is not a Go vulnerability ID such as GO-2026-1234`,
		},
		{
			name:   "an accept entry for the standard library",
			accept: []string{"GO-2026-6599 stdlib " + reason},
			want: "GO-2026-6599 names the standard library, whose findings never fail; remove " +
				"the entry",
		},
		{
			name: "an accept entry without a date", accept: []string{"GO-2026-1234 m why never"},
			want: `the last field, "never", is not a date such as 2026-12-31`,
		},
		{
			name:   "an accept entry listed twice",
			accept: []string{"GO-2026-1234 m " + reason, "GO-2026-1234 m again " + reason},
			want:   ".github/vulnscan-accept.txt:4: GO-2026-1234 in m is listed twice",
		},
		{
			name: "no trees file", remove: ".github/vulnscan-trees.txt",
			want: "open .github/vulnscan-trees.txt: no such file or directory",
		},
		{
			name: "a trees entry without a reason", trees: []string{"examples/consumer"},
			want: ".github/vulnscan-trees.txt:3: want a directory and the reason its module is " +
				"not scanned",
		},
		{
			name: "an absolute trees entry", trees: []string{"/consumer why"},
			want: `"/consumer" is not a clean path relative to the repository root`,
		},
		{
			name: "a trees entry that is not clean", trees: []string{"a/../b why"},
			want: `"a/../b" is not a clean path`,
		},
		{
			name: "a trees entry outside the repository", trees: []string{"../other why"},
			want: `"../other" is not a clean path`,
		},
		{
			name: "a trees entry for the root", trees: []string{". why"},
			want: ". is a tree vulnscan scans; remove the entry",
		},
		{
			name: "a trees entry for tools", trees: []string{"tools why"},
			want: "tools is a tree vulnscan scans; remove the entry",
		},
		{
			name: "a trees entry listed twice", trees: []string{"a why", "a again"},
			want: ".github/vulnscan-trees.txt:4: a is listed twice",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository(t, tt.accept, tt.trees)
			if tt.remove != "" {
				if err := os.Remove(filepath.FromSlash(tt.remove)); err != nil {
					t.Fatal(err)
				}
			}
			args := tt.args
			if args == nil {
				args = []string{"-ceiling", ceiling}
			}
			f := newFake(t)

			code, out := runWith(t, f, args...)

			assertOutput(t, code, exitDidNotRun, out, "vulnscan: DID NOT RUN: ", tt.want)
			if len(f.calls) != 0 {
				t.Errorf("vulnscan ran %d commands, want none", len(f.calls))
			}
		})
	}
}

func TestRunCommand(t *testing.T) {
	dir := t.TempDir()
	out, err := runCommand(t.Context(), dir, "sh", "-c", "pwd; echo err >&2; exit 3")
	if err != nil {
		t.Fatal(err)
	}
	wantDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotDir, err := filepath.EvalSymlinks(strings.TrimSpace(string(out.stdout)))
	if err != nil {
		t.Fatal(err)
	}
	if gotDir != wantDir || string(out.stderr) != "err\n" || out.code != 3 {
		t.Errorf("output %q, %q, exit %d; want %s, err, exit 3", out.stdout, out.stderr,
			out.code, wantDir)
	}

	if _, killed := runCommand(t.Context(), dir, "sh", "-c", "kill -9 $$"); killed == nil {
		t.Error("a killed command returned no error")
	}
	_, err = runCommand(t.Context(), dir, filepath.Join(dir, "missing"))
	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("a missing program returned %v, want a not-found error", err)
	}
}

// TestRepositoryFiles checks that the accept and trees files of this repository can be read.
func TestRepositoryFiles(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	if _, err := readAcceptFile(filepath.Join(root, ".github", "vulnscan-accept.txt")); err != nil {
		t.Error(err)
	}
	if _, err := readTreesFile(filepath.Join(root, ".github", "vulnscan-trees.txt")); err != nil {
		t.Error(err)
	}
}
