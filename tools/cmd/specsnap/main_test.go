package main

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	// pinnedDigest and newDigest are made-up image digests.
	pinnedDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	newDigest    = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	pinnedImage  = "permitio/pdp-v2:0.9.16@" + pinnedDigest
	// containerNote is the note the tests record for the container PDP's spec.
	containerNote = "saved from a test container"
)

// now is the clock of the tests: late on 2026-10-11 in New York, already 2026-10-12 in UTC.
var now = time.Date(2026, 10, 11, 23, 30, 0, 0, time.FixedZone("EDT", -4*60*60))

// reply is what the fake web answers on a path.
type reply struct {
	status int
	body   []byte
}

// web is an httptest server that stands in for the published specs and Docker Hub.
type web struct {
	server  *httptest.Server
	mu      sync.Mutex
	replies map[string]reply
	hits    map[string]int
}

// newWeb starts a web that serves the fixtures and a Docker Hub whose latest is the pinned image.
func newWeb(t *testing.T) *web {
	t.Helper()
	w := &web{replies: map[string]reply{}, hits: map[string]int{}}
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.hits[r.URL.Path]++
		if r.Method != http.MethodGet || r.Header.Get("Accept") != "application/json" {
			t.Errorf("request %s %s with Accept %q, want GET with application/json", r.Method,
				r.URL, r.Header.Get("Accept"))
		}
		if r.URL.Path == "/hub" && r.URL.RawQuery != "page_size=100&ordering=last_updated" {
			t.Errorf("Docker Hub query = %q", r.URL.RawQuery)
		}
		got, ok := w.replies[r.URL.Path]
		if !ok {
			http.NotFound(rw, r)
			return
		}
		rw.WriteHeader(got.status)
		_, _ = rw.Write(got.body)
	}))
	t.Cleanup(w.server.Close)
	w.serve("/control-plane", http.StatusOK, readFixture(t, "control-plane.json"))
	w.serve("/pdp-cloud", http.StatusOK, readFixture(t, "pdp-cloud.json"))
	w.serve("/hub", http.StatusOK, hubReply(t, map[string]string{
		"0.9.17-rc1": newDigest, "latest": pinnedDigest, "0.9.16": pinnedDigest,
	}))
	return w
}

// serve sets the reply on a path.
func (w *web) serve(path string, status int, body []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.replies[path] = reply{status: status, body: body}
}

// hitCount returns the number of requests on a path.
func (w *web) hitCount(path string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hits[path]
}

// sys returns a system whose specs and Docker Hub are on the web.
func (w *web) sys() system {
	return system{
		sources: []source{
			{
				name: "control-plane", url: w.server.URL + "/control-plane", minOperations: 3,
				patches: controlPlanePatches,
			},
			{name: "pdp-cloud", url: w.server.URL + "/pdp-cloud", minOperations: 1},
			{name: "pdp-container", minOperations: 2},
		},
		hubURL:  w.server.URL + "/hub?page_size=100&ordering=last_updated",
		client:  w.server.Client(),
		maxBody: 1 << 20,
		now:     func() time.Time { return now },
	}
}

// hubReply returns Docker Hub's tag list with the tags and digests, in no particular order.
func hubReply(t *testing.T, tags map[string]string) []byte {
	t.Helper()
	type tag struct {
		Name   string `json:"name"`
		Digest string `json:"digest"`
	}
	list := struct {
		Count   int   `json:"count"`
		Results []tag `json:"results"`
	}{Count: len(tags)}
	for name, digest := range tags {
		list.Results = append(list.Results, tag{Name: name, Digest: digest})
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// runSpecsnap runs specsnap and returns its exit code and output.
func runSpecsnap(t *testing.T, sys system, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), args, &stdout, &stderr, sys)
	return code, stdout.String() + stderr.String()
}

// lastLine returns the last line of the output, the verdict.
func lastLine(output string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	return lines[len(lines)-1]
}

// writeFile writes a file in dir and returns its path.
func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// refreshAll writes the three snapshots into a new directory and returns it.
func refreshAll(t *testing.T, sys system) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"-dir", dir, "-refresh", "control-plane"},
		{"-dir", dir, "-refresh", "pdp-cloud", "-fetched", "2026-10-01"},
		{
			"-dir", dir, "-refresh", "pdp-container", "-image", pinnedImage, "-fetched",
			"2026-09-30", "-note", containerNote, "-from",
			filepath.Join("testdata", "pdp-container.json"),
		},
	} {
		if code, out := runSpecsnap(t, sys, args...); code != exitPassed {
			t.Fatalf("specsnap %q exited %d:\n%s", args, code, out)
		}
	}
	return dir
}

// readLockFile reads the lock in dir.
func readLockFile(t *testing.T, dir string) map[string]lockEntry {
	t.Helper()
	lock, err := readLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

// writeLockFile writes the lock in dir.
func writeLockFile(t *testing.T, dir string, lock map[string]lockEntry) {
	t.Helper()
	data, err := encodeLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, lockFile, data)
}

func TestRefresh(t *testing.T) {
	w := newWeb(t)
	sys := w.sys()
	dir := t.TempDir()
	code, out := runSpecsnap(t, sys, "-dir", dir, "-refresh", "control-plane")
	if code != exitPassed {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if w.hitCount("/control-plane") != 1 {
		t.Errorf("the spec was fetched %d times, want 1", w.hitCount("/control-plane"))
	}
	want := mustCanonical(t, func() map[string]any {
		doc := controlPlane(t)
		if _, err := prefix(doc, controlPlanePatches); err != nil {
			t.Fatal(err)
		}
		return doc
	}())
	got, err := os.ReadFile(filepath.Join(dir, "control-plane.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("control-plane.json is not the pre-fixed canonical spec:\n%s", got)
	}
	wantEntry := lockEntry{
		File:         "control-plane.json",
		URL:          w.server.URL + "/control-plane",
		Fetched:      "2026-10-12",
		SourceSHA256: sha256Hex(readFixture(t, "control-plane.json")),
		SHA256:       sha256Hex(want),
		Operations:   3,
	}
	if lock := readLockFile(t, dir); !reflect.DeepEqual(lock,
		map[string]lockEntry{"control-plane": wantEntry}) {
		t.Errorf("lock.json = %+v, want only %+v", lock, wantEntry)
	}
	for _, line := range []string{
		`control-plane: pre-fix "ProxyConfig secret types": fixed 9 places`,
		`control-plane: pre-fix "repeated Bulk Operations tag": fixed 1 place`,
		"PASS: wrote " + filepath.Join(dir, "control-plane.json") + ": 3 operations, sha256 " +
			wantEntry.SHA256,
	} {
		if !strings.Contains(out, line+"\n") {
			t.Errorf("output lacks %q:\n%s", line, out)
		}
	}
}

// TestRefreshKeepsTheOtherEntries checks the lock after three refreshes, and that a refresh from
// a file records the URL, the day given and the note.
func TestRefreshKeepsTheOtherEntries(t *testing.T) {
	w := newWeb(t)
	sys := w.sys()
	dir := refreshAll(t, sys)
	cloudFile := writeFile(t, t.TempDir(), "cloud.json", readFixture(t, "pdp-cloud.json"))
	code, out := runSpecsnap(t, sys, "-dir", dir, "-refresh", "pdp-cloud", "-from", cloudFile,
		"-fetched", "2026-10-02", "-note", "from a file")
	if code != exitPassed {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if w.hitCount("/pdp-cloud") != 1 {
		t.Errorf("the cloud spec was fetched %d times, want once, by refreshAll",
			w.hitCount("/pdp-cloud"))
	}
	lock := readLockFile(t, dir)
	if len(lock) != 3 {
		t.Fatalf("lock.json has %d entries, want 3: %+v", len(lock), lock)
	}
	cloud := lock["pdp-cloud"]
	if cloud.URL != w.server.URL+"/pdp-cloud" || cloud.Fetched != "2026-10-02" ||
		cloud.Note != "from a file" || cloud.Image != "" || cloud.Digest != "" {
		t.Errorf("pdp-cloud entry = %+v", cloud)
	}
	container := lock["pdp-container"]
	wantContainer := lockEntry{
		File:         "pdp-container.json",
		Image:        "permitio/pdp-v2:0.9.16",
		Digest:       pinnedDigest,
		Note:         containerNote,
		Fetched:      "2026-09-30",
		SourceSHA256: sha256Hex(readFixture(t, "pdp-container.json")),
		SHA256:       container.SHA256,
		Operations:   2,
	}
	if container != wantContainer {
		t.Errorf("pdp-container entry = %+v, want %+v", container, wantContainer)
	}
	if code, out := runSpecsnap(t, sys, "-dir", dir, "-verify"); code != exitPassed {
		t.Errorf("-verify after the refreshes exited %d:\n%s", code, out)
	}
	names := slices.Sorted(maps.Keys(dirContents(t, dir)))
	if want := []string{
		"control-plane.json", lockFile, "pdp-cloud.json", "pdp-container.json",
	}; !slices.Equal(names, want) {
		t.Errorf("the refreshes left %q, want %q", names, want)
	}
	data, err := os.ReadFile(filepath.Join(dir, lockFile))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(data, []byte("}\n")) || !bytes.HasPrefix(data,
		[]byte("{\n  \"control-plane\": {\n    \"file\": \"control-plane.json\",\n")) {
		t.Errorf("lock.json is not indented by two spaces with its keys in order:\n%s", data)
	}
}

// TestRefreshDidNotRun checks the refreshes that cannot write a snapshot: they exit 2, or 1
// for a pre-fix patch that is no longer needed, and write nothing.
func TestRefreshDidNotRun(t *testing.T) {
	fixtureDoc := string(readFixture(t, "control-plane.json"))
	noTuple := strings.Replace(fixtureDoc, `{"items": [{"type": "string"}, `,
		`{"prefixItems": [{"type": "string"}, `, 1)
	if noTuple == fixtureDoc {
		t.Fatal("the fixture has no tuple to replace")
	}
	for _, tc := range []struct {
		name   string
		status int
		body   string
		from   string
		code   int
		want   string
	}{
		{
			name: "not found", status: http.StatusNotFound, body: "no",
			code: exitDidNotRun, want: "DID NOT RUN: reading the control-plane spec: fetching ",
		},
		{
			name: "server error", status: http.StatusInternalServerError, body: fixtureDoc,
			code: exitDidNotRun, want: "/control-plane: HTTP 500",
		},
		{
			name: "empty", status: http.StatusOK, body: "",
			code: exitDidNotRun,
			want: "DID NOT RUN: the control-plane spec: it is not a JSON object",
		},
		{
			name: "cut short", status: http.StatusOK, body: fixtureDoc[:len(fixtureDoc)/2],
			code: exitDidNotRun, want: "it is not a JSON object: unexpected EOF",
		},
		{
			name: "too few operations", status: http.StatusOK,
			body: `{"openapi": "3.1.0", "paths": {"/a": {"get": {}, "post": {}}}}`,
			code: exitDidNotRun,
			want: "DID NOT RUN: the control-plane spec: it has 2 operations, and a complete " +
				"control-plane spec has at least 3: it is cut short",
		},
		{
			name: "too large", status: http.StatusOK, body: strings.Repeat(" ", 1<<20+1),
			code: exitDidNotRun, want: "/control-plane answered with more than 1048576 bytes",
		},
		{
			name: "fix no longer needed", status: http.StatusOK, body: noTuple,
			code: exitFailed,
			want: `FAIL: the control-plane spec: pre-fix "OPALUpdateCallback tuple items": fix ` +
				"no longer needed",
		},
		{
			name: "missing file", from: "testdata/no-such-file.json",
			code: exitDidNotRun, want: "DID NOT RUN: reading the control-plane spec: open ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWeb(t)
			w.serve("/control-plane", tc.status, []byte(tc.body))
			dir := t.TempDir()
			args := []string{"-dir", dir, "-refresh", "control-plane"}
			if tc.from != "" {
				args = append(args, "-from", tc.from)
			}
			code, out := runSpecsnap(t, w.sys(), args...)
			if code != tc.code || !strings.Contains(lastLine(out), tc.want) {
				t.Errorf("exit %d, want %d, with a last line containing %q:\n%s", code, tc.code,
					tc.want, out)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Errorf("the refresh wrote %d files", len(entries))
			}
		})
	}
}

// TestRefreshCannotWrite checks a refresh whose lock or directory is unusable.
func TestRefreshCannotWrite(t *testing.T) {
	w := newWeb(t)
	sys := w.sys()
	t.Run("bad lock", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, lockFile, []byte(`{"control-plane": {"file": 1}}`))
		code, out := runSpecsnap(t, sys, "-dir", dir, "-refresh", "control-plane")
		if code != exitDidNotRun || !strings.Contains(out, "DID NOT RUN: parsing lock.json") {
			t.Errorf("exit %d:\n%s", code, out)
		}
		if w.hitCount("/control-plane") != 0 {
			t.Error("specsnap fetched the spec before it read the lock")
		}
	})
	t.Run("lock is null", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, lockFile, []byte("null\n"))
		code, out := runSpecsnap(t, sys, "-dir", dir, "-refresh", "control-plane")
		if code != exitDidNotRun ||
			!strings.Contains(out, "DID NOT RUN: parsing lock.json: it is not a JSON object") {
			t.Errorf("exit %d:\n%s", code, out)
		}
	})
	t.Run("no directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "missing")
		code, out := runSpecsnap(t, sys, "-dir", dir, "-refresh", "control-plane")
		if code != exitDidNotRun ||
			!strings.Contains(out, "DID NOT RUN: writing control-plane.json") {
			t.Errorf("exit %d:\n%s", code, out)
		}
	})
	t.Run("lock is a directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, lockFile), 0o750); err != nil {
			t.Fatal(err)
		}
		code, out := runSpecsnap(t, sys, "-dir", dir, "-refresh", "control-plane")
		if code != exitDidNotRun || !strings.Contains(out, "DID NOT RUN: reading lock.json") {
			t.Errorf("exit %d:\n%s", code, out)
		}
	})
	t.Run("lock cannot be written", func(t *testing.T) {
		changed := newWeb(t)
		dir := refreshAll(t, changed.sys())
		changed.serve("/pdp-cloud", http.StatusOK, bytes.Replace(readFixture(t, "pdp-cloud.json"),
			[]byte("allowed"), []byte("denied"), 1))
		if err := os.Mkdir(filepath.Join(dir, lockFile+".tmp"), 0o750); err != nil {
			t.Fatal(err)
		}
		before := dirContents(t, dir)
		code, out := runSpecsnap(t, changed.sys(), "-dir", dir, "-refresh", "pdp-cloud")
		if code != exitDidNotRun || !strings.Contains(out, "DID NOT RUN: writing lock.json") {
			t.Errorf("exit %d:\n%s", code, out)
		}
		if diff := changedFiles(t, dir, before); len(diff) > 0 {
			t.Errorf("a refresh that could not write lock.json changed %q", diff)
		}
	})
	t.Run("snapshot cannot be replaced", func(t *testing.T) {
		dir := refreshAll(t, sys)
		snapshot := filepath.Join(dir, "pdp-cloud.json")
		if err := os.Remove(snapshot); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(snapshot, 0o750); err != nil {
			t.Fatal(err)
		}
		before := dirContents(t, dir)
		code, out := runSpecsnap(t, sys, "-dir", dir, "-refresh", "pdp-cloud")
		if code != exitDidNotRun || !strings.Contains(out, "DID NOT RUN: writing pdp-cloud.json") {
			t.Errorf("exit %d:\n%s", code, out)
		}
		if diff := changedFiles(t, dir, before); len(diff) > 0 {
			t.Errorf("a refresh that could not replace the snapshot changed %q", diff)
		}
	})
}

// dirContents returns the content of each file in dir by name, and "directory" for each
// directory in it.
func dirContents(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			contents[entry.Name()] = "directory"
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		contents[entry.Name()] = string(data)
	}
	return contents
}

// changedFiles returns the names of the entries of dir that were added, removed or changed
// since dirContents returned before, sorted.
func changedFiles(t *testing.T, dir string, before map[string]string) []string {
	t.Helper()
	after := dirContents(t, dir)
	var changed []string
	for name, content := range after {
		if old, ok := before[name]; !ok || old != content {
			changed = append(changed, name)
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			changed = append(changed, name)
		}
	}
	slices.Sort(changed)
	return changed
}

// TestVerify checks -verify on snapshots that match and, one change at a time, on snapshots or
// a lock that do not.
func TestVerify(t *testing.T) {
	w := newWeb(t)
	sys := w.sys()
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, dir string)
		code   int
		want   []string
	}{
		{
			name:   "match",
			change: func(*testing.T, string) {},
			code:   exitPassed,
			want: []string{
				"control-plane: PASS: matches lock.json", "pdp-container: PASS: matches lock.json",
				"PASS: the 3 snapshots match lock.json",
			},
		},
		{
			name: "a snapshot changed by hand",
			change: func(t *testing.T, dir string) {
				t.Helper()
				path := filepath.Join(dir, "pdp-cloud.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, dir, "pdp-cloud.json", bytes.Replace(data, []byte("allowed"),
					[]byte("denied"), 1))
			},
			code: exitFailed,
			want: []string{
				"pdp-cloud: FAIL: pdp-cloud.json has sha256 ",
				"refresh it with specsnap -refresh, never by hand",
				"control-plane: PASS", "FAIL: failed for pdp-cloud",
			},
		},
		{
			name: "operations in the lock",
			change: func(t *testing.T, dir string) {
				t.Helper()
				lock := readLockFile(t, dir)
				entry := lock["control-plane"]
				entry.Operations = 4
				lock["control-plane"] = entry
				writeLockFile(t, dir, lock)
			},
			code: exitFailed,
			want: []string{"control-plane: FAIL: control-plane.json has 3 operations, and " +
				"lock.json records 4"},
		},
		{
			name: "not canonical",
			change: func(t *testing.T, dir string) {
				t.Helper()
				data := []byte(`{"paths": {"/allowed": {"post": {}}}}` + "\n")
				relock(t, dir, "pdp-cloud", data, 1)
			},
			code: exitFailed,
			want: []string{"pdp-cloud: FAIL: pdp-cloud.json is not in canonical form"},
		},
		{
			name: "a snapshot with too few operations",
			change: func(t *testing.T, dir string) {
				t.Helper()
				relock(t, dir, "pdp-container", []byte(`{}`+"\n"), 0)
			},
			code: exitDidNotRun,
			want: []string{
				"pdp-container: DID NOT RUN: pdp-container.json has 0 operations, and a " +
					"complete pdp-container spec has at least 2",
				"DID NOT RUN: did not complete for pdp-container",
			},
		},
		{
			name: "a snapshot that is not JSON",
			change: func(t *testing.T, dir string) {
				t.Helper()
				relock(t, dir, "pdp-cloud", []byte("not json\n"), 0)
			},
			code: exitDidNotRun,
			want: []string{"pdp-cloud: DID NOT RUN: parsing pdp-cloud.json: it is not a JSON"},
		},
		{
			name: "a missing snapshot and a changed one",
			change: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.Remove(filepath.Join(dir, "control-plane.json")); err != nil {
					t.Fatal(err)
				}
				writeFile(t, dir, "pdp-cloud.json", []byte("{}\n"))
			},
			code: exitDidNotRun,
			want: []string{
				"control-plane: DID NOT RUN: reading the snapshot: open ",
				"DID NOT RUN: did not complete for control-plane; failed for pdp-cloud",
			},
		},
		{
			name: "the lock names another file",
			change: func(t *testing.T, dir string) {
				t.Helper()
				lock := readLockFile(t, dir)
				entry := lock["pdp-cloud"]
				entry.File = "control-plane.json"
				lock["pdp-cloud"] = entry
				writeLockFile(t, dir, lock)
			},
			code: exitDidNotRun,
			want: []string{`pdp-cloud: DID NOT RUN: lock.json names the file ` +
				`"control-plane.json", not "pdp-cloud.json"`},
		},
		{
			name: "the lock has no entry for a spec",
			change: func(t *testing.T, dir string) {
				t.Helper()
				lock := readLockFile(t, dir)
				delete(lock, "pdp-cloud")
				writeLockFile(t, dir, lock)
			},
			code: exitDidNotRun,
			want: []string{"DID NOT RUN: lock.json has no entry for pdp-cloud: write it with " +
				"specsnap -refresh pdp-cloud"},
		},
		{
			name: "the lock has an entry for an unknown spec",
			change: func(t *testing.T, dir string) {
				t.Helper()
				lock := readLockFile(t, dir)
				lock["pdp-edge"] = lock["pdp-cloud"]
				writeLockFile(t, dir, lock)
			},
			code: exitDidNotRun,
			want: []string{`DID NOT RUN: lock.json has an entry for "pdp-edge", which is not ` +
				"one of control-plane, pdp-cloud, pdp-container"},
		},
		{
			name: "the lock has an unknown field",
			change: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, dir, lockFile, []byte(`{"pdp-cloud": {"files": "x"}}`))
			},
			code: exitDidNotRun,
			want: []string{`DID NOT RUN: parsing lock.json: json: unknown field "files"`},
		},
		{
			name: "the lock has more data after its object",
			change: func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, lockFile))
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, dir, lockFile, append(data, "{}\n"...))
			},
			code: exitDidNotRun,
			want: []string{
				"DID NOT RUN: parsing lock.json: it has more data after its JSON object",
			},
		},
		{
			name: "no lock",
			change: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.Remove(filepath.Join(dir, lockFile)); err != nil {
					t.Fatal(err)
				}
			},
			code: exitDidNotRun,
			want: []string{"DID NOT RUN: reading lock.json: open "},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := refreshAll(t, sys)
			tc.change(t, dir)
			code, out := runSpecsnap(t, sys, "-verify", "-dir", dir)
			if code != tc.code {
				t.Errorf("exit %d, want %d:\n%s", code, tc.code, out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			if !strings.HasPrefix(lastLine(out), verdictWord(tc.code)+": ") {
				t.Errorf("the last line is not the verdict %s:\n%s", verdictWord(tc.code), out)
			}
		})
	}
}

// relock writes a snapshot and gives its lock entry the snapshot's sha256 and the operations.
func relock(t *testing.T, dir, name string, data []byte, operations int) {
	t.Helper()
	writeFile(t, dir, name+".json", data)
	lock := readLockFile(t, dir)
	entry := lock[name]
	entry.SHA256 = sha256Hex(data)
	entry.Operations = operations
	lock[name] = entry
	writeLockFile(t, dir, lock)
}

func TestBadArgs(t *testing.T) {
	sys := newWeb(t).sys()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "give exactly one of -refresh, -verify, -check-live and -check-pdp"},
		{[]string{"-verify", "-check-live"}, "give exactly one of"},
		{[]string{"-verify", "-refresh", "pdp-cloud"}, "give exactly one of"},
		{[]string{"-refresh", "pdp-cloud", "-check-live"}, "give exactly one of"},
		{[]string{"-verify", "extra"}, `unexpected arguments ["extra"]`},
		{[]string{"-no-such-flag"}, "flag provided but not defined: -no-such-flag"},
		{[]string{"-verify", "-out", "x"}, "-verify does not take -out"},
		{[]string{"-verify", "-from", "x", "-note", "y"}, "-verify does not take -from, -note"},
		{[]string{"-check-live", "-fetched", "2026-10-11"}, "-check-live does not take -fetched"},
		{[]string{"-check-live", "-image", pinnedImage}, "-check-live does not take -image"},
		{[]string{"-refresh", "pdp-cloud", "-out", "x"}, "-refresh does not take -out"},
		{[]string{"-verify", "-dir", ""}, "-dir is empty"},
		{
			[]string{"-refresh", "pdp-edge"},
			`-refresh "pdp-edge" is not one of control-plane, pdp-cloud, pdp-container`,
		},
		{
			[]string{"-refresh", "pdp-container", "-image", pinnedImage},
			"-refresh pdp-container needs -from, the spec saved from the container's " +
				"GET /openapi.json, and -image",
		},
		{[]string{"-refresh", "pdp-container", "-from", "x.json"}, "needs -from"},
		{
			[]string{"-refresh", "pdp-container", "-from", "x", "-image", "permitio/pdp-v2:0.9.16"},
			`-image "permitio/pdp-v2:0.9.16" is not permitio/pdp-v2:<tag>@sha256:<64 hex digits>`,
		},
		{
			[]string{
				"-refresh", "pdp-container", "-from", "x",
				"-image", "permitio/pdp:0.9.16@" + pinnedDigest,
			},
			"is not permitio/pdp-v2:<tag>@sha256",
		},
		{
			[]string{
				"-refresh", "pdp-container", "-from", "x",
				"-image", "permitio/pdp-v2:0.9.16@" + pinnedDigest + "0",
			},
			"is not permitio/pdp-v2:<tag>@sha256",
		},
		{
			[]string{"-refresh", "pdp-container", "-from", "x", "-image", "x" + pinnedImage},
			"is not permitio/pdp-v2:<tag>@sha256",
		},
		{
			[]string{"-refresh", "control-plane", "-image", pinnedImage},
			"-image is only for the container PDP's spec, not control-plane",
		},
		{[]string{"-refresh", "pdp-cloud", "-note", "two\nlines"}, "-note must be one line"},
		{[]string{"-refresh", "pdp-cloud", "-note", "a\rb"}, "-note must be one line"},
		{
			[]string{"-refresh", "pdp-cloud", "-fetched", "2026-02-30"},
			`-fetched "2026-02-30" is not a day as YYYY-MM-DD`,
		},
		{[]string{"-refresh", "pdp-cloud", "-fetched", "11/10/2026"}, "is not a day"},
	} {
		code, out := runSpecsnap(t, sys, tc.args...)
		if code != exitDidNotRun || !strings.Contains(out, tc.want) ||
			!strings.Contains(out, "DID NOT RUN: ") {
			t.Errorf("specsnap %q: exit %d, want 2 and %q:\n%s", tc.args, code, tc.want, out)
		}
	}
}

func TestExitCode(t *testing.T) {
	err := failure{os.ErrNotExist}
	for _, tc := range []struct {
		err  error
		want int
	}{
		{err, exitFailed},
		{wrapError(err), exitFailed},
		{os.ErrNotExist, exitDidNotRun},
	} {
		if got := exitCode(tc.err); got != tc.want {
			t.Errorf("exitCode(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
	for code, want := range map[int]string{0: "PASS", 1: "FAIL", 2: "DID NOT RUN"} {
		if got := verdictWord(code); got != want {
			t.Errorf("verdictWord(%d) = %q, want %q", code, got, want)
		}
	}
}

// wrapError wraps an error once.
func wrapError(err error) error {
	return &wrapped{err}
}

type wrapped struct {
	err error
}

func (w *wrapped) Error() string { return "wrapped: " + w.err.Error() }

func (w *wrapped) Unwrap() error { return w.err }
