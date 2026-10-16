// Package testhook writes the request record that tools/cmd/apicoverage reads to measure which
// operations of the Permit API and the PDP the tests reach. It is for tests only: Record is for
// the SDK's transport to call after each attempt at a request, and does nothing unless the
// context carries a Recorder, which only a test puts there. It keeps no global state.
//
// A record is a file of JSON lines. The first is the header, {"type": "header", "format": 1}.
// Each request is a line such as
//
//	{"type": "request", "mode": "wire", "target": "api", "method": "GET",
//	 "template": "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
//	 "path": "/v2/facts/p/e/users/a%2Fb", "status": 200, "test": "TestUsers/get"}
//
// written on one line, with the route's path, its parameters escaped, after any path of the base
// URL, and without the query, the headers or the body. The template is the route the request was
// made for, or empty when the writer knows none, and the status is 0 when no response came. The
// last line is the session trailer that the run's TestMain writes:
//
//	{"type": "session", "exit_status": 0, "tests": 12}
//
// with the exit code of the tests and the number of tests that took part in the record. A
// record without its trailer is from a run that did not finish. A request to a test-only route,
// whose path starts with /__test/, is never written.
//
// The mode says who sent the request: wire for the offline wire tests against
// internal/testkit/mockpermit, dryrun for the end-to-end scenarios run offline, live for the
// end-to-end scenarios against the Permit cloud, and local for a full local stack.
package testhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Format is the version of the record format that the header names.
const Format = 1

// Mode says who sent a request.
type Mode string

const (
	// Wire is a request of an offline wire test, which internal/testkit/mockpermit records.
	Wire Mode = "wire"

	// DryRun is a request of an end-to-end scenario run offline, which proves that the scenario
	// sends it but not that the Permit cloud answers it.
	DryRun Mode = "dryrun"

	// Live is a request of an end-to-end scenario to the Permit cloud.
	Live Mode = "live"

	// Local is a request of a run against a full local stack.
	Local Mode = "local"
)

// Target is the server a request went to.
type Target string

const (
	// API is the Permit API, the control plane.
	API Target = "api"

	// PDP is a PDP, the container or the cloud PDP.
	PDP Target = "pdp"
)

// TestPrefix starts the path of every test-only route. A request to such a route is never
// written.
const TestPrefix = "/__test/"

// Request is one request that a record holds.
type Request struct {
	// Mode says who sent the request.
	Mode Mode

	// Target is the server the request went to.
	Target Target

	// Method is the request method, such as GET.
	Method string

	// Template is the path of the route the request was made for, such as
	// /v2/facts/{proj_id}/{env_id}/users/{user_id}, or empty when the writer knows none.
	Template string

	// Path is the route's path with its parameters escaped: the part of the request's path after
	// any path of the base URL, without the query.
	Path string

	// Status is the status of the response, or 0 when none came.
	Status int

	// Test is the name of the test or scenario that sent the request.
	Test string
}

// line is a line of a record. Every line has a type; the other fields depend on it.
type line struct {
	Type       string `json:"type"`
	Format     int    `json:"format,omitempty"`
	Mode       Mode   `json:"mode,omitempty"`
	Target     Target `json:"target,omitempty"`
	Method     string `json:"method,omitempty"`
	Template   string `json:"template,omitempty"`
	Path       string `json:"path,omitempty"`
	Status     *int   `json:"status,omitempty"`
	Test       string `json:"test,omitempty"`
	ExitStatus *int   `json:"exit_status,omitempty"`
	Tests      *int   `json:"tests,omitempty"`
}

// File is a record being written. Its methods are safe for concurrent use. A write that fails
// does not fail the request or the test that made it; Close returns the first such error.
type File struct {
	mu sync.Mutex
	w  io.WriteCloser
	// late receives the requests and tests that come after Close, which the record can no
	// longer hold: standard error, where go test shows it.
	late   io.Writer
	tests  map[string]bool
	err    error
	closed bool
}

// Create creates the record file at path and writes its header. It fails when the file exists,
// so that two runs never write one record.
func Create(path string) (*File, error) {
	//nolint:gosec // G304: a test names the record file it writes.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("testhook: creating the request record: %w; remove the file or "+
			"name another", err)
	}
	return newFile(f, os.Stderr)
}

// newFile writes the header of a record to w, and reports to late what comes after Close.
func newFile(w io.WriteCloser, late io.Writer) (*File, error) {
	file := &File{w: w, late: late, tests: map[string]bool{}}
	if err := file.write(line{Type: "header", Format: Format}); err != nil {
		_ = w.Close() // The write error is the one to report.
		return nil, err
	}
	return file, nil
}

// AddTest counts the test or scenario among the tests that took part in the record, once
// however often it is added.
func (f *File) AddTest(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case name == "":
		f.keep(errors.New("testhook: a test without a name took part in the record"))
		return
	case f.closed:
		f.tooLate(fmt.Sprintf("the test %q took part", name))
		return
	}
	f.tests[name] = true
}

// Write writes a request to the record and counts its test, unless its path starts with
// TestPrefix. A request that a record cannot hold is not written, and Close returns the error.
func (f *File) Write(r Request) {
	if strings.HasPrefix(r.Path, TestPrefix) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		f.tooLate(fmt.Sprintf("%s %s was sent by %q", r.Method, r.Path, r.Test))
		return
	}
	if err := check(r); err != nil {
		f.keep(err)
		return
	}
	f.tests[r.Test] = true
	status := r.Status
	f.keep(f.write(line{
		Type:     "request",
		Mode:     r.Mode,
		Target:   r.Target,
		Method:   r.Method,
		Template: r.Template,
		Path:     r.Path,
		Status:   &status,
		Test:     r.Test,
	}))
}

// Close writes the session trailer with the exit code of the tests and the number of tests that
// took part, and closes the file. It returns the first error of a write or of a request that a
// record cannot hold. A request or a test after Close is not in the record; File writes it to
// standard error.
func (f *File) Close(exitStatus int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("testhook: the record is closed already")
	}
	f.closed = true
	tests := len(f.tests)
	f.keep(f.write(line{Type: "session", ExitStatus: &exitStatus, Tests: &tests}))
	if err := f.w.Close(); err != nil {
		f.keep(fmt.Errorf("testhook: closing the request record: %w", err))
	}
	return f.err
}

// tooLate reports what came after Close. The caller holds f.mu.
func (f *File) tooLate(what string) {
	// Close has returned, and standard error is the last place to report to.
	_, _ = fmt.Fprintf(f.late, "testhook: %s after the request record was closed, so the "+
		"record does not hold it\n", what)
}

// keep keeps err when it is the first error. The caller holds f.mu.
func (f *File) keep(err error) {
	if f.err == nil {
		f.err = err
	}
}

// write writes l as one line. The caller holds f.mu, or is the only user of f.
func (f *File) write(l line) error {
	data, err := json.Marshal(l)
	if err != nil {
		return fmt.Errorf("testhook: encoding a line of the request record: %w", err)
	}
	if _, err := f.w.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("testhook: writing the request record: %w", err)
	}
	return nil
}

// modes are the modes a record can hold.
var modes = []Mode{Wire, DryRun, Live, Local}

// requestMethod matches a request method, as tools/cmd/apicoverage reads it.
var requestMethod = regexp.MustCompile(`^[A-Z]+$`)

// check returns an error when a record cannot hold r: tools/cmd/apicoverage reads every line
// that it lets through.
func check(r Request) error {
	switch {
	case !slices.Contains(modes, r.Mode):
		return fmt.Errorf("testhook: %s %s has the mode %q; want wire, dryrun, live or local",
			r.Method, r.Path, r.Mode)
	case r.Target != API && r.Target != PDP:
		return fmt.Errorf("testhook: %s %s has the target %q; want api or pdp", r.Method, r.Path,
			r.Target)
	case !requestMethod.MatchString(r.Method):
		return fmt.Errorf("testhook: the request to %s has the method %q; want upper-case "+
			"letters, such as GET", r.Path, r.Method)
	case !strings.HasPrefix(r.Path, "/"):
		return fmt.Errorf("testhook: %s %q is not a path that starts with /", r.Method, r.Path)
	case strings.ContainsAny(r.Path, "?#"):
		return fmt.Errorf("testhook: %s %q holds a query or a fragment; give the path without "+
			"them", r.Method, r.Path)
	case r.Status != 0 && (r.Status < 100 || r.Status > 599):
		return fmt.Errorf("testhook: %s %s has the status %d; want 0, for no response, or 100 "+
			"to 599", r.Method, r.Path, r.Status)
	case r.Test == "":
		return fmt.Errorf("testhook: %s %s names no test", r.Method, r.Path)
	}
	return nil
}

// Recorder writes the requests of one test or scenario to a record. A test puts it in the
// context it passes the SDK with WithRecorder.
type Recorder struct {
	file *File
	mode Mode
	test string
}

// NewRecorder returns a Recorder that writes the requests of the test or scenario named test to
// file, in the mode dryrun or live, and counts the test in the record.
func NewRecorder(file *File, mode Mode, test string) (*Recorder, error) {
	switch {
	case file == nil:
		return nil, errors.New("testhook: NewRecorder needs a record file")
	case mode != DryRun && mode != Live:
		return nil, fmt.Errorf("testhook: a Recorder records in the mode dryrun or live, not %q; "+
			"mockpermit records the wire tests", mode)
	case test == "":
		return nil, errors.New("testhook: NewRecorder needs the name of the test or scenario")
	}
	file.AddTest(test)
	return &Recorder{file: file, mode: mode, test: test}, nil
}

// recorderKey is the context key of the Recorder.
type recorderKey struct{}

// WithRecorder returns a copy of ctx that carries r.
func WithRecorder(ctx context.Context, r *Recorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, r)
}

// Attempt is one attempt at a request, as the SDK's transport made it.
type Attempt struct {
	// Target is the server the request went to.
	Target Target

	// Method is the request method.
	Method string

	// Template is the path of the operation's route, such as
	// /v2/facts/{proj_id}/{env_id}/users/{user_id}.
	Template string

	// Path is the route's path with its parameters escaped: the part of the request's path after
	// any path of the base URL, without the query.
	Path string

	// Status is the status of the response, or 0 when none came.
	Status int
}

// Record writes the attempt to the record of the Recorder that ctx carries, and does nothing
// when ctx carries none.
func Record(ctx context.Context, a Attempt) {
	r, ok := ctx.Value(recorderKey{}).(*Recorder)
	if !ok || r == nil {
		return
	}
	r.file.Write(Request{
		Mode:     r.mode,
		Target:   a.Target,
		Method:   a.Method,
		Template: a.Template,
		Path:     a.Path,
		Status:   a.Status,
		Test:     r.test,
	})
}
