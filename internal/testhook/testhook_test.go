package testhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// buffer is an io.WriteCloser that keeps what it is given, and can fail its writes or its close.
type buffer struct {
	mu        sync.Mutex
	data      bytes.Buffer
	writeErr  error
	failAfter int // the number of writes that succeed before writeErr; 0 fails the first
	writes    int
	closeErr  error
	closed    bool
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.writes++
	if b.writeErr != nil && b.writes > b.failAfter {
		return 0, b.writeErr
	}
	return b.data.Write(p)
}

func (b *buffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return b.closeErr
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}

// newTestFile returns a File that writes to a buffer and reports what comes late to another.
func newTestFile(t *testing.T) (*File, *buffer, *bytes.Buffer) {
	t.Helper()
	out := &buffer{}
	late := &bytes.Buffer{}
	f, err := newFile(out, late)
	if err != nil {
		t.Fatal(err)
	}
	return f, out, late
}

func TestFileWritesTheRecord(t *testing.T) {
	f, out, late := newTestFile(t)
	f.Write(Request{
		Mode: Wire, Target: API, Method: "GET",
		Template: "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		Path:     "/v2/facts/p/e/users/a%2Fb", Status: 200, Test: "TestUsers/get",
	})
	f.Write(Request{
		Mode: Live, Target: PDP, Method: "POST", Path: "/allowed", Status: 0, Test: "Scenario",
	})
	f.AddTest("TestOperationMap")
	f.AddTest("TestUsers/get")
	if err := f.Close(3); err != nil {
		t.Fatalf("Close returned %v", err)
	}
	want := `{"type":"header","format":1}
{"type":"request","mode":"wire","target":"api","method":"GET",` +
		`"template":"/v2/facts/{proj_id}/{env_id}/users/{user_id}",` +
		`"path":"/v2/facts/p/e/users/a%2Fb","status":200,"test":"TestUsers/get"}
{"type":"request","mode":"live","target":"pdp","method":"POST","path":"/allowed",` +
		`"status":0,"test":"Scenario"}
{"type":"session","exit_status":3,"tests":3}
`
	if got := out.String(); got != want {
		t.Errorf("the record is\n%s\nwant\n%s", got, want)
	}
	if !out.closed {
		t.Error("Close did not close the file")
	}
	if late.Len() > 0 {
		t.Errorf("the record reported %q as late", late.String())
	}
}

// TestFileWritesTheGoldenRecord checks the record format against testdata/record.jsonl, which
// the tests of tools/cmd/apicoverage read, so that the writer and the reader agree.
func TestFileWritesTheGoldenRecord(t *testing.T) {
	f, out, _ := newTestFile(t)
	f.Write(Request{
		Mode: Wire, Target: API, Method: "GET",
		Template: "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		Path:     "/v2/facts/p/e/users/a%2Fb", Status: 200, Test: "TestUsers/get",
	})
	f.Write(Request{
		Mode: DryRun, Target: API, Method: "DELETE",
		Template: "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		Path:     "/v2/facts/p/e/users/u", Status: 204, Test: "TestE2EUsers/delete",
	})
	f.Write(Request{
		Mode: Live, Target: PDP, Method: "POST", Template: "/allowed", Path: "/allowed",
		Test: "TestLiveCheck",
	})
	f.Write(Request{
		Mode: Local, Target: PDP, Method: "POST", Path: "/facts/users", Status: 200,
		Test: "pdp.facts",
	})
	if err := f.Close(0); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "record.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != string(want) {
		t.Errorf("the record is\n%s\nwant testdata/record.jsonl:\n%s", got, want)
	}
}

func TestFileOfNoRequests(t *testing.T) {
	f, out, _ := newTestFile(t)
	if err := f.Close(0); err != nil {
		t.Fatal(err)
	}
	want := "{\"type\":\"header\",\"format\":1}\n" +
		"{\"type\":\"session\",\"exit_status\":0,\"tests\":0}\n"
	if got := out.String(); got != want {
		t.Errorf("the record is %q, want %q", got, want)
	}
}

func TestWriteLeavesOutTestRoutes(t *testing.T) {
	f, out, _ := newTestFile(t)
	f.Write(Request{Mode: Wire, Target: API, Method: "GET", Path: "/__test/echo", Test: "TestA"})
	f.Write(Request{Mode: Wire, Target: API, Method: "GET", Path: "/__test/", Test: "TestB"})
	f.Write(Request{Mode: Wire, Target: API, Method: "GET", Path: "/__testing", Test: "TestC"})
	if err := f.Close(0); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], `"path":"/__testing"`) {
		t.Fatalf("the record holds %q, want the header, /__testing and the trailer", lines)
	}
	if !strings.Contains(lines[2], `"tests":1`) {
		t.Errorf("the trailer is %s, want 1 test: the tests of test routes do not count", lines[2])
	}
}

func TestWriteRejectsWhatARecordCannotHold(t *testing.T) {
	valid := Request{Mode: DryRun, Target: API, Method: "GET", Path: "/v2/x", Test: "TestX"}
	for _, tc := range []struct {
		name   string
		change func(*Request)
		want   string
	}{
		{"no mode", func(r *Request) { r.Mode = "" }, `has the mode ""`},
		{"an unknown mode", func(r *Request) { r.Mode = "cloud" }, `has the mode "cloud"`},
		{"no target", func(r *Request) { r.Target = "" }, `has the target ""`},
		{"an unknown target", func(r *Request) { r.Target = "opa" }, `has the target "opa"`},
		{"no method", func(r *Request) { r.Method = "" }, `has the method ""`},
		{"a lower-case method", func(r *Request) { r.Method = "get" }, `has the method "get"`},
		{"a method with a space", func(r *Request) { r.Method = "GET " }, `has the method "GET "`},
		{"a relative path", func(r *Request) { r.Path = "v2/x" }, `"v2/x" is not a path`},
		{"no path", func(r *Request) { r.Path = "" }, `"" is not a path`},
		{"a query", func(r *Request) { r.Path = "/v2/x?y=1" }, `"/v2/x?y=1" holds a query`},
		{"a fragment", func(r *Request) { r.Path = "/v2/x#y" }, `"/v2/x#y" holds a query`},
		{"a status of 99", func(r *Request) { r.Status = 99 }, "has the status 99"},
		{"a status of 600", func(r *Request) { r.Status = 600 }, "has the status 600"},
		{"a negative status", func(r *Request) { r.Status = -1 }, "has the status -1"},
		{"no test", func(r *Request) { r.Test = "" }, "names no test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, out, _ := newTestFile(t)
			r := valid
			tc.change(&r)
			f.Write(r)
			f.Write(valid)
			err := f.Close(0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Close returned %v, want an error that contains %q", err, tc.want)
			}
			if got := strings.Count(out.String(), `"type":"request"`); got != 1 {
				t.Errorf("the record holds %d requests, want only the valid one", got)
			}
		})
	}
	for _, mode := range modes {
		f, _, _ := newTestFile(t)
		r := valid
		r.Mode = mode
		f.Write(r)
		if err := f.Close(0); err != nil {
			t.Errorf("a request in the mode %s: Close returned %v", mode, err)
		}
	}
	for _, status := range []int{0, 100, 599} {
		f, _, _ := newTestFile(t)
		r := valid
		r.Status = status
		f.Write(r)
		if err := f.Close(0); err != nil {
			t.Errorf("a request with the status %d: Close returned %v", status, err)
		}
	}
}

func TestCloseReturnsTheFirstError(t *testing.T) {
	f, _, _ := newTestFile(t)
	f.AddTest("")
	f.Write(Request{Mode: Wire, Target: API, Method: "GET", Path: "/x"})
	err := f.Close(0)
	if err == nil || !strings.Contains(err.Error(), "a test without a name") {
		t.Errorf("Close returned %v, want the first error, the test without a name", err)
	}
	if err := f.Close(0); err == nil || !strings.Contains(err.Error(), "closed already") {
		t.Errorf("a second Close returned %v, want an error", err)
	}
}

func TestAfterCloseIsReportedLate(t *testing.T) {
	f, out, late := newTestFile(t)
	if err := f.Close(0); err != nil {
		t.Fatal(err)
	}
	before := out.String()
	f.Write(Request{Mode: Wire, Target: API, Method: "GET", Path: "/v2/x", Test: "TestLate"})
	f.AddTest("TestAfter")
	if out.String() != before {
		t.Errorf("the record changed after Close:\n%s", out.String())
	}
	for _, want := range []string{
		`testhook: GET /v2/x was sent by "TestLate" after the request record was closed`,
		`testhook: the test "TestAfter" took part after the request record was closed`,
	} {
		if !strings.Contains(late.String(), want) {
			t.Errorf("standard error got %q, want it to contain %q", late.String(), want)
		}
	}
}

func TestWriteErrors(t *testing.T) {
	failing := errors.New("disk full")
	out := &buffer{writeErr: failing, failAfter: 1}
	f, err := newFile(out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	f.Write(Request{Mode: Wire, Target: API, Method: "GET", Path: "/v2/x", Test: "TestX"})
	if closeErr := f.Close(0); !errors.Is(closeErr, failing) ||
		!strings.Contains(closeErr.Error(), "writing the request record") {
		t.Errorf("Close returned %v, want the write error", closeErr)
	}

	header := &buffer{writeErr: failing}
	if _, headerErr := newFile(header, &bytes.Buffer{}); !errors.Is(headerErr, failing) {
		t.Errorf("newFile returned %v, want the error of the header's write", headerErr)
	}
	if !header.closed {
		t.Error("newFile left the file open after the header failed")
	}

	closing := &buffer{closeErr: failing}
	f, err = newFile(closing, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := f.Close(0); !errors.Is(closeErr, failing) ||
		!strings.Contains(closeErr.Error(), "closing the request record") {
		t.Errorf("Close returned %v, want the close error", closeErr)
	}

	trailer := &buffer{writeErr: failing, failAfter: 1}
	f, err = newFile(trailer, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(0); !errors.Is(err, failing) {
		t.Errorf("Close returned %v, want the error of the trailer's write", err)
	}
	if !trailer.closed {
		t.Error("Close left the file open after the trailer failed")
	}
}

func TestCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.jsonl")
	f, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Write(Request{Mode: Wire, Target: PDP, Method: "POST", Path: "/allowed", Test: "TestA"})
	if closeErr := f.Close(0); closeErr != nil {
		t.Fatal(closeErr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(data), "\n"); lines != 3 {
		t.Errorf("the file holds %d lines, want 3:\n%s", lines, data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the file's mode is %v, want 0600", mode)
	}
	if _, err := Create(path); err == nil || !strings.Contains(err.Error(),
		"remove the file or name another") {
		t.Errorf("Create of an existing file returned %v, want an error", err)
	}
	if _, err := Create(filepath.Join(t.TempDir(), "missing", "record.jsonl")); err == nil {
		t.Error("Create in a missing directory returned no error")
	}
}

func TestRecord(t *testing.T) {
	f, out, _ := newTestFile(t)
	Record(t.Context(), Attempt{Target: API, Method: "GET", Path: "/v2/nobody"})
	r, err := NewRecorder(f, DryRun, "TestE2EUsers/delete")
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithRecorder(t.Context(), r)
	Record(ctx, Attempt{
		Target: API, Method: "DELETE", Template: "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		Path: "/v2/facts/p/e/users/u", Status: 204,
	})
	Record(context.WithValue(ctx, recorderKey{}, (*Recorder)(nil)),
		Attempt{Target: API, Method: "GET", Path: "/v2/nil"})
	if err := f.Close(0); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("the record holds %q, want the header, one request and the trailer", lines)
	}
	var got line
	if err := json.Unmarshal([]byte(lines[1]), &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != DryRun || got.Test != "TestE2EUsers/delete" || got.Method != http.MethodDelete ||
		got.Template != "/v2/facts/{proj_id}/{env_id}/users/{user_id}" ||
		got.Path != "/v2/facts/p/e/users/u" || got.Status == nil || *got.Status != 204 ||
		got.Target != API {
		t.Errorf("the request line is %s", lines[1])
	}
	if lines[2] != `{"type":"session","exit_status":0,"tests":1}` {
		t.Errorf("the trailer is %s, want the scenario counted", lines[2])
	}
}

func TestNewRecorderRejects(t *testing.T) {
	f, _, _ := newTestFile(t)
	for _, tc := range []struct {
		name string
		file *File
		mode Mode
		test string
		want string
	}{
		{"no file", nil, Live, "S", "needs a record file"},
		{"the wire mode", f, Wire, "S", `not "wire"`},
		{"the local mode", f, Local, "S", `not "local"`},
		{"no mode", f, "", "S", `not ""`},
		{"no test", f, Live, "", "needs the name of the test"},
	} {
		r, err := NewRecorder(tc.file, tc.mode, tc.test)
		if r != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: NewRecorder returned %v, %v; want an error that contains %q", tc.name,
				r, err, tc.want)
		}
	}
	for _, mode := range []Mode{DryRun, Live} {
		if _, err := NewRecorder(f, mode, "S"); err != nil {
			t.Errorf("NewRecorder in the mode %s returned %v", mode, err)
		}
	}
}

func TestFileIsSafeForConcurrentUse(t *testing.T) {
	f, out, _ := newTestFile(t)
	const writers, each = 8, 50
	var wg sync.WaitGroup
	for w := range writers {
		wg.Go(func() {
			for range each {
				f.Write(Request{
					Mode: Wire, Target: API, Method: "GET", Path: "/v2/x", Status: 200,
					Test: "TestW" + string(rune('A'+w)),
				})
			}
		})
	}
	wg.Wait()
	if err := f.Close(0); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != writers*each+2 {
		t.Fatalf("the record holds %d lines, want %d", len(lines), writers*each+2)
	}
	for _, l := range lines {
		if !json.Valid([]byte(l)) {
			t.Fatalf("the line %q is not JSON", l)
		}
	}
	if want := `{"type":"session","exit_status":0,"tests":8}`; lines[len(lines)-1] != want {
		t.Errorf("the trailer is %s, want %s", lines[len(lines)-1], want)
	}
}
