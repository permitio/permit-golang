package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckLive checks -check-live when nothing changed: it passes and writes the live
// snapshots, which equal the committed ones.
func TestCheckLive(t *testing.T) {
	w := newWeb(t)
	sys := w.sys()
	dir := refreshAll(t, sys)
	out := filepath.Join(t.TempDir(), "live", "specs")
	code, output := runSpecsnap(t, sys, "-dir", dir, "-check-live", "-out", out)
	if code != exitPassed {
		t.Fatalf("exit %d:\n%s", code, output)
	}
	for _, want := range []string{
		"control-plane: PASS: the live spec matches the snapshot\n",
		"pdp-cloud: PASS: the live spec matches the snapshot\n",
		"pdp-container: PASS: Docker Hub's latest permitio/pdp-v2 is permitio/pdp-v2:0.9.16\n",
		"PASS: the 3 live specs and images match lock.json\n",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output lacks %q:\n%s", want, output)
		}
	}
	for _, name := range []string{"control-plane.json", "pdp-cloud.json"} {
		live, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		committed, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(live, committed) {
			t.Errorf("the live %s differs from the snapshot", name)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "pdp-container.json")); !os.IsNotExist(err) {
		t.Errorf("-check-live wrote a container snapshot: %v", err)
	}
	if code, output := runSpecsnap(t, sys, "-dir", dir, "-check-live"); code != exitPassed {
		t.Errorf("without -out: exit %d:\n%s", code, output)
	}
}

// TestCheckLiveDrift checks -check-live when a live spec or Docker Hub's latest image differs
// from its snapshot, and when the check cannot complete.
func TestCheckLiveDrift(t *testing.T) {
	fixtureDoc := string(readFixture(t, "control-plane.json"))
	for _, tc := range []struct {
		name  string
		path  string
		reply reply
		lock  func(lock map[string]lockEntry)
		code  int
		want  []string
	}{
		{
			name: "the control plane changed",
			path: "/control-plane",
			reply: reply{http.StatusOK, []byte(strings.Replace(fixtureDoc,
				`"required": ["callbacks"]`, `"required": []`, 1))},
			code: exitFailed,
			want: []string{
				"control-plane: FAIL: the live spec changed: its snapshot has sha256 ",
				" and 3 operations; control-plane.json has sha256 ",
				"and 3 operations, fetched 2026-10-12",
				"pdp-cloud: PASS", "pdp-container: PASS",
				"FAIL: drift in control-plane\n",
			},
		},
		{
			name: "only the prose changed",
			path: "/control-plane",
			reply: reply{http.StatusOK, []byte(strings.Replace(fixtureDoc, "List things",
				"List all the things", 1))},
			code: exitPassed,
			want: []string{"control-plane: PASS", "PASS: the 3 live specs and images match"},
		},
		{
			name: "the API fixed a defect",
			path: "/control-plane",
			reply: reply{http.StatusOK, []byte(strings.Replace(fixtureDoc,
				"{\"name\": \"Bulk Operations\", \"description\": \"None\"}\n  ]",
				"{\"name\": \"Other\"}\n  ]", 1))},
			code: exitFailed,
			want: []string{`control-plane: FAIL: pre-fix "repeated Bulk Operations tag": fix ` +
				"no longer needed"},
		},
		{
			name:  "a new image is latest",
			path:  "/hub",
			reply: reply{http.StatusOK, nil},
			code:  exitFailed,
			want: []string{
				"pdp-container: FAIL: latest permitio/pdp-v2 on Docker Hub is " + newDigest +
					" (also tagged 0.9.17, v0.9.17), and the snapshot was taken from " +
					pinnedImage,
				"FAIL: drift in pdp-container\n",
			},
		},
		{
			name: "latest has no other tag",
			path: "/hub",
			reply: reply{http.StatusOK, []byte(`{"results": [{"name": "latest", "digest": "` +
				newDigest + `"}, {"name": "0.9.16", "digest": "` + pinnedDigest + `"}]}`)},
			code: exitFailed,
			want: []string{"is " + newDigest + " (no other tag), and the snapshot"},
		},
		{
			name:  "the live spec is cut short",
			path:  "/pdp-cloud",
			reply: reply{http.StatusOK, []byte(`{"openapi": "3.1.0", "paths": {}}`)},
			code:  exitDidNotRun,
			want: []string{
				"pdp-cloud: DID NOT RUN: it has 0 operations, and a complete pdp-cloud spec has " +
					"at least 1",
				"DID NOT RUN: did not complete for pdp-cloud\n",
			},
		},
		{
			name:  "a spec cannot be fetched while another drifted",
			path:  "/pdp-cloud",
			reply: reply{http.StatusBadGateway, []byte("down")},
			lock: func(lock map[string]lockEntry) {
				entry := lock["control-plane"]
				entry.SHA256 = strings.Repeat("0", 64)
				lock["control-plane"] = entry
			},
			code: exitDidNotRun,
			want: []string{
				"pdp-cloud: DID NOT RUN: fetching ",
				"/pdp-cloud: HTTP 502",
				"DID NOT RUN: did not complete for pdp-cloud; drift in control-plane\n",
			},
		},
		{
			name:  "Docker Hub fails",
			path:  "/hub",
			reply: reply{http.StatusTooManyRequests, []byte("slow down")},
			code:  exitDidNotRun,
			want: []string{
				"pdp-container: DID NOT RUN: fetching ",
				"/hub?page_size=100&ordering=last_updated: HTTP 429",
			},
		},
		{
			name:  "Docker Hub answers with something else",
			path:  "/hub",
			reply: reply{http.StatusOK, []byte(`<html>`)},
			code:  exitDidNotRun,
			want: []string{
				"pdp-container: DID NOT RUN: parsing Docker Hub's tags of permitio/pdp-v2: ",
			},
		},
		{
			name: "Docker Hub has no latest",
			path: "/hub",
			reply: reply{http.StatusOK, []byte(`{"results": [{"name": "0.9.16", "digest": "` +
				pinnedDigest + `"}]}`)},
			code: exitDidNotRun,
			want: []string{
				"pdp-container: DID NOT RUN: the newest 100 tags of permitio/pdp-v2 on Docker " +
					"Hub have no latest",
			},
		},
		{
			name:  "Docker Hub has no tags",
			path:  "/hub",
			reply: reply{http.StatusOK, []byte(`{"results": []}`)},
			code:  exitDidNotRun,
			want:  []string{"have no latest"},
		},
		{
			name: "latest has no digest",
			path: "/hub",
			reply: reply{
				http.StatusOK,
				[]byte(`{"results": [{"name": "latest", "digest": "sha256:abc"}]}`),
			},
			code: exitDidNotRun,
			want: []string{
				`the latest tag on Docker Hub has the digest "sha256:abc", which is not ` +
					"sha256:<64 hex digits>",
			},
		},
		{
			name: "the lock has no digest",
			lock: func(lock map[string]lockEntry) {
				entry := lock["pdp-container"]
				entry.Digest = ""
				lock["pdp-container"] = entry
			},
			code: exitDidNotRun,
			want: []string{
				`pdp-container: DID NOT RUN: lock.json records the digest "", which is not ` +
					"sha256:<64 hex digits>",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWeb(t)
			sys := w.sys()
			dir := refreshAll(t, sys)
			if tc.path == "/hub" && tc.reply.body == nil {
				tc.reply.body = hubReply(t, map[string]string{
					"0.9.17": newDigest, "v0.9.17": newDigest, "latest": newDigest,
					"0.9.16": pinnedDigest,
				})
			}
			if tc.path != "" {
				w.serve(tc.path, tc.reply.status, tc.reply.body)
			}
			if tc.lock != nil {
				lock := readLockFile(t, dir)
				tc.lock(lock)
				writeLockFile(t, dir, lock)
			}
			code, output := runSpecsnap(t, sys, "-dir", dir, "-check-live", "-out", t.TempDir())
			if code != tc.code {
				t.Errorf("exit %d, want %d:\n%s", code, tc.code, output)
			}
			for _, want := range tc.want {
				if !strings.Contains(output, want) {
					t.Errorf("output lacks %q:\n%s", want, output)
				}
			}
			if !strings.HasPrefix(lastLine(output), verdictWord(tc.code)+": ") {
				t.Errorf("the last line is not the verdict %s:\n%s", verdictWord(tc.code), output)
			}
		})
	}
}

// TestCheckLiveWritesTheChangedSnapshot checks that the snapshot of a live spec that changed is
// written to -out, for the pull request that refreshes it.
func TestCheckLiveWritesTheChangedSnapshot(t *testing.T) {
	w := newWeb(t)
	sys := w.sys()
	dir := refreshAll(t, sys)
	changed := []byte(`{"paths": {"/allowed": {"post": {}}, "/allowed/bulk": {"post": {}}}}`)
	w.serve("/pdp-cloud", http.StatusOK, changed)
	out := t.TempDir()
	if code, output := runSpecsnap(t, sys, "-dir", dir, "-check-live", "-out", out); code !=
		exitFailed {
		t.Fatalf("exit %d:\n%s", code, output)
	}
	got, err := os.ReadFile(filepath.Join(out, "pdp-cloud.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := mustCanonical(t, mustParse(t, changed)); !bytes.Equal(got, want) {
		t.Errorf("the live snapshot is\n%s\nwant\n%s", got, want)
	}
}

// TestCheckLiveDidNotRun checks the cases where -check-live cannot start: no usable lock, or an
// -out that cannot be created.
func TestCheckLiveDidNotRun(t *testing.T) {
	w := newWeb(t)
	sys := w.sys()
	dir := refreshAll(t, sys)
	notADir := writeFile(t, t.TempDir(), "file", []byte("x"))
	for name, args := range map[string][]string{
		"no lock":          {"-dir", t.TempDir(), "-check-live"},
		"out is a file":    {"-dir", dir, "-check-live", "-out", notADir},
		"out is in a file": {"-dir", dir, "-check-live", "-out", filepath.Join(notADir, "x")},
	} {
		code, output := runSpecsnap(t, sys, args...)
		if code != exitDidNotRun || !strings.HasPrefix(lastLine(output), "DID NOT RUN: ") {
			t.Errorf("%s: exit %d:\n%s", name, code, output)
		}
	}
	readOnly := t.TempDir()
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	code, output := runSpecsnap(t, sys, "-dir", dir, "-check-live", "-out", readOnly)
	if code != exitDidNotRun || !strings.Contains(output,
		"control-plane: DID NOT RUN: writing the live snapshot: open ") {
		t.Errorf("a read-only -out: exit %d:\n%s", code, output)
	}
	lock := readLockFile(t, dir)
	delete(lock, "pdp-container")
	writeLockFile(t, dir, lock)
	code, output = runSpecsnap(t, sys, "-dir", dir, "-check-live")
	if code != exitDidNotRun || !strings.Contains(output, "lock.json has no entry for "+
		"pdp-container") {
		t.Errorf("a lock without pdp-container: exit %d:\n%s", code, output)
	}
	if w.hitCount("/hub") != 1 {
		t.Errorf("Docker Hub was asked %d times, want once, with the read-only -out",
			w.hitCount("/hub"))
	}
}

// TestFetchLimit checks that a body of exactly maxBody bytes is read, and one byte more is not.
func TestFetchLimit(t *testing.T) {
	body := []byte(`{"paths": {}}`)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write(body)
	}))
	t.Cleanup(server.Close)
	sys := system{client: server.Client(), maxBody: int64(len(body))}
	if got, err := fetch(t.Context(), sys, server.URL); err != nil || !bytes.Equal(got, body) {
		t.Errorf("a body at the limit: %q, %v", got, err)
	}
	sys.maxBody--
	if _, err := fetch(t.Context(), sys, server.URL); err == nil ||
		!strings.Contains(err.Error(), "answered with more than 12 bytes") {
		t.Errorf("a body over the limit: %v", err)
	}
}

// TestFetchFails checks the requests that fail before or while the body is read.
func TestFetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Length", "100")
		_, _ = rw.Write([]byte(`{"paths"`))
	}))
	t.Cleanup(server.Close)
	sys := system{client: server.Client(), maxBody: 1 << 20}
	if _, err := fetch(t.Context(), sys, server.URL); err == nil ||
		!strings.Contains(err.Error(), "reading "+server.URL+": unexpected EOF") {
		t.Errorf("a body cut short: %v", err)
	}
	if _, err := fetch(t.Context(), sys, "http://[::1"); err == nil ||
		!strings.Contains(err.Error(), `building the request for http://[::1: `) {
		t.Errorf("a bad URL: %v", err)
	}
}
