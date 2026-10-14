package mockpermit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
)

// recordLine is a line of a request record, as tools/cmd/apicoverage reads it.
type recordLine struct {
	Type       string `json:"type"`
	Format     int    `json:"format,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Target     string `json:"target,omitempty"`
	Method     string `json:"method,omitempty"`
	Template   string `json:"template,omitempty"`
	Path       string `json:"path,omitempty"`
	Status     *int   `json:"status,omitempty"`
	Test       string `json:"test,omitempty"`
	ExitStatus *int   `json:"exit_status,omitempty"`
	Tests      *int   `json:"tests,omitempty"`
}

// readRecord returns the lines of the record at path.
func readRecord(t *testing.T, path string) []recordLine {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []recordLine
	for text := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "\n") {
		dec := json.NewDecoder(strings.NewReader(text))
		dec.DisallowUnknownFields()
		var l recordLine
		if err := dec.Decode(&l); err != nil {
			t.Fatalf("the record line %q: %v", text, err)
		}
		lines = append(lines, l)
	}
	return lines
}

func TestRunRecordedWritesTheRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.jsonl")
	ft := faketb.New(t)
	var stderr bytes.Buffer
	code := runRecorded(func() int {
		client := newClient(t)
		api := NewAPI(ft)
		api.Handle("GET /v2/facts/{proj_id}/{env_id}/users/{user_id}", Reply{Body: `{}`})
		api.Handle("POST /v2/facts/{proj_id}/{env_id}/users", Reply{Body: `{}`})
		api.Handle("GET /__test/echo", Reply{Status: http.StatusNoContent})
		stop := api.FailRequests("POST /v2/facts/{proj_id}/{env_id}/users", 503, 0)
		pdp := NewPDP(ft)
		pdp.Handle("POST /allowed", Reply{Body: `{"allow":true}`})
		cloud := NewPDP(ft, Cloud)
		(call{method: http.MethodPost, path: "/v2/facts/p/e/users"}).send(t, client, api)
		stop()
		for _, c := range []struct {
			server *Server
			call   call
		}{
			{api, call{method: http.MethodGet, path: "/v2/facts/p/e/users/a%2Fb?x=1"}},
			{api, call{method: http.MethodPost, path: "/v2/facts/p/e/users"}},
			{api, call{method: http.MethodGet, path: "/__test/echo"}},
			{api, call{method: http.MethodDelete, path: "/v2/nothing"}},
			{pdp, call{method: http.MethodPost, path: "/allowed"}},
			{cloud, call{method: http.MethodPost, path: "/user-tenants"}},
		} {
			c.call.send(t, client, c.server)
		}
		t.Run("subtest", func(t *testing.T) {
			RecordTest(t)
			RecordTest(t)
		})
		return 0
	}, path, &stderr)
	ft.End()
	wantFailures(t, ft, "DELETE /v2/nothing: no route")
	if code != 0 || stderr.Len() > 0 {
		t.Errorf("runRecorded returned %d and wrote %q, want 0 and nothing", code, stderr.String())
	}
	request := func(target, method, template, path string, status int) recordLine {
		return recordLine{
			Type: "request", Mode: "wire", Target: target, Method: method, Template: template,
			Path: path, Status: new(status), Test: t.Name(),
		}
	}
	want := []recordLine{
		{Type: "header", Format: 1},
		request("api", "POST", "/v2/facts/{proj_id}/{env_id}/users", "/v2/facts/p/e/users", 503),
		request("api", "GET", "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
			"/v2/facts/p/e/users/a%2Fb", 200),
		request("api", "POST", "/v2/facts/{proj_id}/{env_id}/users", "/v2/facts/p/e/users", 200),
		request("api", "DELETE", "", "/v2/nothing", 501),
		request("pdp", "POST", "/allowed", "/allowed", 200),
		request("pdp", "POST", "", "/user-tenants", 404),
		{Type: "session", ExitStatus: new(0), Tests: new(2)},
	}
	if diff := cmp.Diff(want, readRecord(t, path)); diff != "" {
		t.Errorf("the record differs (-want +got):\n%s", diff)
	}
	if currentRecording() != nil {
		t.Error("the record is still set after the run")
	}
}

// TestRunRecordedReadsTheEnvironment runs a function through RunRecorded, as a TestMain that
// also runs ExampleServers.Run does, with and without PERMIT_API_COVERAGE_RECORD.
func TestRunRecordedReadsTheEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.jsonl")
	t.Setenv(RecordEnv, path)
	if code := RunRecorded(func() int { RecordTest(t); return 0 }); code != 0 {
		t.Errorf("RunRecorded returned %d, want 0", code)
	}
	want := []recordLine{
		{Type: "header", Format: 1},
		{Type: "session", ExitStatus: new(0), Tests: new(1)},
	}
	if diff := cmp.Diff(want, readRecord(t, path)); diff != "" {
		t.Errorf("the record differs (-want +got):\n%s", diff)
	}
	t.Setenv(RecordEnv, "")
	if code := RunRecorded(func() int { return 3 }); code != 3 {
		t.Errorf("RunRecorded without a record returned %d, want 3", code)
	}
}

func TestRunRecordedWithoutARecord(t *testing.T) {
	var stderr bytes.Buffer
	runs := 0
	code := runRecorded(func() int {
		runs++
		ft := faketb.New(t)
		api := NewAPI(ft)
		RecordTest(t)
		if api.record != nil || currentRecording() != nil {
			t.Error("a server records without a record file")
		}
		ft.End()
		return 3
	}, "", &stderr)
	if code != 3 || runs != 1 || stderr.Len() > 0 {
		t.Errorf("runRecorded returned %d after %d runs and wrote %q; want 3, 1 run and nothing",
			code, runs, stderr.String())
	}
}

func TestRunRecordedPassesTheExitCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.jsonl")
	var stderr bytes.Buffer
	if code := runRecorded(func() int { RecordTest(t); return 1 }, path,
		&stderr); code != 1 || stderr.Len() > 0 {
		t.Errorf("runRecorded returned %d and wrote %q, want 1 and nothing", code, stderr.String())
	}
	got := readRecord(t, path)
	want := recordLine{Type: "session", ExitStatus: new(1), Tests: new(1)}
	if diff := cmp.Diff(want, got[len(got)-1]); diff != "" {
		t.Errorf("the trailer differs (-want +got):\n%s", diff)
	}
}

func TestRunRecordedCannotCreateTheRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.jsonl")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	runs := 0
	code := runRecorded(func() int { runs++; return 0 }, path, &stderr)
	if code != 1 || runs != 0 {
		t.Errorf("runRecorded returned %d after %d runs, want 1 and no run", code, runs)
	}
	if !strings.Contains(stderr.String(), "mockpermit: PERMIT_API_COVERAGE_RECORD: testhook: "+
		"creating the request record") {
		t.Errorf("runRecorded wrote %q, want why it could not create the record", stderr.String())
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "old\n" {
		t.Errorf("the existing file holds %q, %v; want it unchanged", data, err)
	}
}

func TestRunRecordedReportsAnIncompleteRecord(t *testing.T) {
	for _, tc := range []struct {
		name     string
		testCode int
		want     int
	}{
		{"tests that passed", 0, 1},
		{"tests that failed", 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "record.jsonl")
			var stderr bytes.Buffer
			code := runRecorded(func() int {
				currentRecording().AddTest("")
				return tc.testCode
			}, path, &stderr)
			if code != tc.want {
				t.Errorf("runRecorded returned %d, want %d", code, tc.want)
			}
			if !strings.Contains(stderr.String(), "mockpermit: the request record "+path+
				" is not complete: testhook: a test without a name") {
				t.Errorf("runRecorded wrote %q, want why the record is not complete",
					stderr.String())
			}
		})
	}
}

func TestServersStartedBeforeTheRecordDoNotRecord(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle("GET /v2/x", Reply{Body: `{}`})
	path := filepath.Join(t.TempDir(), "record.jsonl")
	runRecorded(func() int {
		(call{method: http.MethodGet, path: "/v2/x"}).send(t, newClient(t), api)
		return 0
	}, path, &bytes.Buffer{})
	ft.End()
	wantFailures(t, ft)
	want := []recordLine{
		{Type: "header", Format: 1},
		{Type: "session", ExitStatus: new(0), Tests: new(0)},
	}
	if diff := cmp.Diff(want, readRecord(t, path)); diff != "" {
		t.Errorf("the record differs (-want +got):\n%s", diff)
	}
}
