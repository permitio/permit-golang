package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// otherImage is an image of the container PDP other than the one of the snapshot.
const otherImage = "permitio/pdp-v2:0.9.17@" + newDigest

// TestCheckPDP checks -check-pdp: it passes when the container PDP serves the spec of its
// snapshot, also when only the prose differs and with the snapshot's image, and fails, saying
// where the two first differ, when the PDP's spec or the snapshot changed, or when the PDP runs
// another image.
func TestCheckPDP(t *testing.T) {
	fixture := string(readFixture(t, "pdp-container.json"))
	for _, tc := range []struct {
		name string

		// spec is what the PDP serves at /pdp/openapi.json.
		spec string

		// snapshot changes the committed snapshot, unless it is nil.
		snapshot func(data string) string

		// args are the flags beside -dir and -check-pdp.
		args []string

		code int
		want []string
	}{
		{
			name: "the same spec", spec: fixture, code: exitPassed,
			want: []string{
				"PASS: the PDP at {url} serves the spec of pdp-container.json, the snapshot of " +
					pinnedImage + "\n",
			},
		},
		{
			name: "the image of the snapshot", spec: fixture, args: []string{"-image", pinnedImage},
			code: exitPassed, want: []string{"PASS: the PDP at {url} serves the spec of"},
		},
		{
			name: "only the prose differs",
			spec: strings.Replace(fixture, `"Decision"`, `"The decision"`, 1), code: exitPassed,
			want: []string{"PASS: the PDP at {url} serves the spec of"},
		},
		{
			name: "the PDP serves another spec",
			spec: strings.Replace(fixture, `"/user-tenants"`, `"/tenants"`, 1), code: exitFailed,
			want: []string{
				"FAIL: the spec at {url}/openapi.json is not pdp-container.json, the snapshot " +
					"of " + pinnedImage + ": its snapshot has sha256 ",
				" and 2 operations, and pdp-container.json has sha256 ",
				`; they first differ at line 15, "\"/tenants\": {" in the PDP's and ` +
					`"\"/user-tenants\": {" in pdp-container.json` + "\n",
			},
		},
		{
			name: "the snapshot was changed", spec: fixture,
			snapshot: func(data string) string {
				return strings.Replace(data, `"operationId": "allowed"`,
					`"operationId": "is_allowed"`, 1)
			},
			code: exitFailed,
			want: []string{
				`they first differ at line 9, "\"operationId\": \"allowed\"," in the PDP's ` +
					`and "\"operationId\": \"is_allowed\"," in pdp-container.json`,
			},
		},
		{
			name: "the snapshot lost its last line", spec: fixture,
			snapshot: func(data string) string { return strings.TrimSuffix(data, "\n") },
			code:     exitFailed,
			want:     []string{`"" in the PDP's and "<end of file>" in pdp-container.json`},
		},
		{
			name: "another image", spec: fixture, args: []string{"-image", otherImage},
			code: exitFailed,
			want: []string{
				"FAIL: the PDP runs " + otherImage + ", and pdp-container.json is the snapshot " +
					"of " + pinnedImage + "\n",
			},
		},
		{
			name: "another image and another spec",
			spec: strings.Replace(fixture, `"/allowed"`, `"/allow"`, 1),
			args: []string{"-image", otherImage}, code: exitFailed,
			want: []string{
				"FAIL: the PDP runs " + otherImage + ", and pdp-container.json is the snapshot " +
					"of " + pinnedImage + "; the spec at {url}/openapi.json is not",
			},
		},
		{
			name: "a spec cut short", spec: `{"paths": {"/allowed": {"post": {}}}}`,
			code: exitDidNotRun,
			want: []string{
				"DID NOT RUN: the spec at {url}/openapi.json: it has 1 operation, and a " +
					"complete pdp-container spec has at least 2",
			},
		},
		{
			name: "no spec", code: exitDidNotRun,
			want: []string{"DID NOT RUN: reading the PDP's spec: fetching {url}/openapi.json: " +
				"HTTP 404"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWeb(t)
			sys := w.sys()
			dir := refreshAll(t, sys)
			if tc.spec != "" {
				w.serve("/pdp/openapi.json", http.StatusOK, []byte(tc.spec))
			}
			if tc.snapshot != nil {
				path := filepath.Join(dir, "pdp-container.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, dir, "pdp-container.json", []byte(tc.snapshot(string(data))))
			}
			url := w.server.URL + "/pdp"
			args := append([]string{"-dir", dir, "-check-pdp", url}, tc.args...)

			code, output := runSpecsnap(t, sys, args...)

			if code != tc.code {
				t.Errorf("exit %d, want %d:\n%s", code, tc.code, output)
			}
			for _, want := range tc.want {
				if want = strings.ReplaceAll(want, "{url}", url); !strings.Contains(output, want) {
					t.Errorf("output lacks %q:\n%s", want, output)
				}
			}
			if lines := strings.Count(output, "\n"); lines != 1 {
				t.Errorf("output has %d lines, want the verdict alone:\n%s", lines, output)
			}
			if got := w.hitCount("/pdp/openapi.json"); got != 1 {
				t.Errorf("the PDP's spec was fetched %d times, want once", got)
			}
		})
	}
}

// TestCheckPDPURL checks that -check-pdp asks for /openapi.json under the PDP's base URL, with
// or without a slash at its end.
func TestCheckPDPURL(t *testing.T) {
	w := newWeb(t)
	sys := w.sys()
	dir := refreshAll(t, sys)
	w.serve("/pdp/openapi.json", http.StatusOK, readFixture(t, "pdp-container.json"))
	for _, url := range []string{w.server.URL + "/pdp", w.server.URL + "/pdp/"} {
		if code, output := runSpecsnap(t, sys, "-dir", dir, "-check-pdp", url); code !=
			exitPassed {
			t.Errorf("-check-pdp %s: exit %d:\n%s", url, code, output)
		}
	}
	if got := w.hitCount("/pdp/openapi.json"); got != 2 {
		t.Errorf("the PDP's spec was fetched %d times, want twice", got)
	}
}

// TestCheckPDPDidNotRun checks the cases where -check-pdp cannot compare: no lock, no entry of
// the container PDP's spec in it, a snapshot that is missing or not JSON, and a specsnap that
// keeps no container PDP's spec. It fetches nothing then.
func TestCheckPDPDidNotRun(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, dir string, sys *system)
		want  string
	}{
		{
			name: "no lock",
			setup: func(t *testing.T, dir string, _ *system) {
				t.Helper()
				if err := os.Remove(filepath.Join(dir, lockFile)); err != nil {
					t.Fatal(err)
				}
			},
			want: "DID NOT RUN: reading lock.json: ",
		},
		{
			name: "no entry",
			setup: func(t *testing.T, dir string, _ *system) {
				t.Helper()
				lock := readLockFile(t, dir)
				delete(lock, "pdp-container")
				writeLockFile(t, dir, lock)
			},
			want: "DID NOT RUN: lock.json has no entry for pdp-container\n",
		},
		{
			name: "no snapshot",
			setup: func(t *testing.T, dir string, _ *system) {
				t.Helper()
				if err := os.Remove(filepath.Join(dir, "pdp-container.json")); err != nil {
					t.Fatal(err)
				}
			},
			want: "DID NOT RUN: reading the snapshot: open ",
		},
		{
			name: "a snapshot that is not JSON",
			setup: func(t *testing.T, dir string, _ *system) {
				t.Helper()
				writeFile(t, dir, "pdp-container.json", []byte("{"))
			},
			want: "DID NOT RUN: parsing pdp-container.json: ",
		},
		{
			name: "no container PDP's spec",
			setup: func(_ *testing.T, _ string, sys *system) {
				sys.sources = sys.sources[:2]
			},
			want: "DID NOT RUN: specsnap keeps no pdp-container spec\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWeb(t)
			sys := w.sys()
			dir := refreshAll(t, sys)
			w.serve("/pdp/openapi.json", http.StatusOK, readFixture(t, "pdp-container.json"))
			tc.setup(t, dir, &sys)

			code, output := runSpecsnap(t, sys, "-dir", dir, "-check-pdp", w.server.URL+"/pdp")

			if code != exitDidNotRun || !strings.Contains(output, tc.want) {
				t.Errorf("exit %d, want 2 with %q:\n%s", code, tc.want, output)
			}
			if got := w.hitCount("/pdp/openapi.json"); got != 0 {
				t.Errorf("the PDP's spec was fetched %d times, want none", got)
			}
		})
	}
}

// TestCheckPDPBadArgs checks the command lines of -check-pdp that specsnap does not run.
func TestCheckPDPBadArgs(t *testing.T) {
	sys := newWeb(t).sys()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-check-pdp", "127.0.0.1:7766"}, `-check-pdp "127.0.0.1:7766" is not the ` +
			"base URL of a PDP, such as http://127.0.0.1:7766"},
		{[]string{"-check-pdp", "ftp://127.0.0.1"}, "is not the base URL of a PDP"},
		{[]string{"-check-pdp", "http://"}, "is not the base URL of a PDP"},
		{[]string{"-check-pdp", "http://127.0.0.1:7766?x=1"}, "is not the base URL of a PDP"},
		{[]string{"-check-pdp", "http://127.0.0.1:7766#x"}, "is not the base URL of a PDP"},
		{[]string{"-check-pdp", "http://u:p@127.0.0.1:7766"}, "is not the base URL of a PDP"},
		{[]string{"-check-pdp", "http://[::1"}, "is not the base URL of a PDP"},
		{
			[]string{"-check-pdp", "http://127.0.0.1:7766", "-image", "permitio/pdp-v2:0.9.16"},
			`-image "permitio/pdp-v2:0.9.16" is not permitio/pdp-v2:<tag>@sha256:<64 hex digits>`,
		},
		{[]string{"-check-pdp", "http://127.0.0.1:7766", "-out", "x"}, "-check-pdp does not " +
			"take -out"},
		{[]string{"-check-pdp", "http://127.0.0.1:7766", "-check-live"}, "give exactly one of " +
			"-refresh, -verify, -check-live and -check-pdp"},
		{[]string{"-check-pdp", "http://127.0.0.1:7766", "-verify"}, "give exactly one of"},
	} {
		code, out := runSpecsnap(t, sys, tc.args...)
		if code != exitDidNotRun || !strings.Contains(out, "DID NOT RUN: ") ||
			!strings.Contains(out, tc.want) {
			t.Errorf("specsnap %q: exit %d, want 2 and %q:\n%s", tc.args, code, tc.want, out)
		}
	}
}

// TestCheckPDPCommittedSnapshot checks -check-pdp with the specs that specsnap keeps and the
// committed snapshots: a PDP that serves pdp-container.json, and runs the image that lock.json
// records for it, passes.
func TestCheckPDPCommittedSnapshot(t *testing.T) {
	w := newWeb(t)
	sys := w.sys()
	sys.sources = sources
	snapshot, err := os.ReadFile(filepath.Join(specsDir, "pdp-container.json"))
	if err != nil {
		t.Fatal(err)
	}
	w.serve("/pdp/openapi.json", http.StatusOK, snapshot)
	entry := readLockFile(t, specsDir)["pdp-container"]

	code, output := runSpecsnap(t, sys, "-dir", specsDir, "-check-pdp", w.server.URL+"/pdp",
		"-image", entry.Image+"@"+entry.Digest)

	if code != exitPassed || !strings.HasPrefix(output, "PASS: ") {
		t.Errorf("exit %d, want 0:\n%s", code, output)
	}
}

// TestFirstDifference checks the line in which two snapshots first differ.
func TestFirstDifference(t *testing.T) {
	for _, tc := range []struct {
		a, b     string
		line     int
		aAt, bAt string
	}{
		{a: "{\n  \"a\": 1\n}\n", b: "{\n  \"a\": 1\n}\n", line: 0},
		{a: "{\n  \"a\": 1\n}\n", b: "{\n  \"a\": 2\n}\n", line: 2, aAt: `"a": 1`, bAt: `"a": 2`},
		{a: "x\n", b: "x", line: 2, aAt: "", bAt: endOfFile},
		{a: "x", b: "x\ny", line: 2, aAt: endOfFile, bAt: "y"},
		{a: "x\ny\n", b: "x\n", line: 2, aAt: "y", bAt: ""},
		{a: "", b: "x\n", line: 1, aAt: "", bAt: "x"},
	} {
		line, aAt, bAt := firstDifference([]byte(tc.a), []byte(tc.b))
		if line != tc.line || aAt != tc.aAt || bAt != tc.bAt {
			t.Errorf("firstDifference(%q, %q) = %d, %q, %q; want %d, %q, %q", tc.a, tc.b, line,
				aAt, bAt, tc.line, tc.aAt, tc.bAt)
		}
	}
}
