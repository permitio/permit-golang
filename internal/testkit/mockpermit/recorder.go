package mockpermit

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testhook"
)

// RecordEnv names the environment variable that turns the request record on. When it names a
// file, RunRecorded writes the record of the requests that the servers receive there, for
// tools/cmd/apicoverage.
const RecordEnv = "PERMIT_API_COVERAGE_RECORD"

// recording is the record that the servers of this test binary write to while RunRecorded runs
// the tests, or nil.
var recording struct {
	mu   sync.Mutex
	file *testhook.File
}

// RunRecorded runs the tests of a package with run, which returns their exit code, as its
// TestMain does, and returns the exit code for TestMain to pass to os.Exit:
//
//	func TestMain(m *testing.M) {
//		os.Exit(mockpermit.RunRecorded(m.Run))
//	}
//
// A package with Examples runs its tests through ExampleServers.Run inside it:
//
//	os.Exit(mockpermit.RunRecorded(func() int { return servers.Run(m) }))
//
// When the environment variable PERMIT_API_COVERAGE_RECORD names a file, which must not exist,
// RunRecorded writes the request record to it, in the format that internal/testhook describes:
// every request that a server of NewAPI or NewPDP receives while the tests run, in the mode
// wire, then the session trailer with the exit code of the tests and the number of tests that
// took part. A test takes part when it starts a server or calls RecordTest. The servers of
// StartExample do not record. RunRecorded returns the exit code of the tests, or 1 when they
// passed but the record could not be written in full; it runs no test when it cannot create the
// file.
func RunRecorded(run func() int) int {
	//nolint:forbidigo // A setting of the test run, which no client reads.
	path := os.Getenv(RecordEnv)
	return runRecorded(run, path, os.Stderr)
}

// runRecorded runs the tests with run, and writes the record to path unless it is empty. It
// reports to stderr why the record failed.
func runRecorded(run func() int, path string, stderr io.Writer) int {
	if path == "" {
		return run()
	}
	file, err := testhook.Create(path)
	if err != nil {
		reportf(stderr, "mockpermit: %s: %v\n", RecordEnv, err)
		return 1
	}
	setRecording(file)
	code := run()
	setRecording(nil)
	if err := file.Close(code); err != nil {
		reportf(stderr, "mockpermit: the request record %s is not complete: %v\n", path, err)
		if code == 0 {
			return 1
		}
	}
	return code
}

// reportf writes a line to stderr. The exit code reports the failure even when stderr cannot
// take it, so a write error is left out.
func reportf(stderr io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(stderr, format, args...)
}

// setRecording sets the record that new servers write to.
func setRecording(file *testhook.File) {
	recording.mu.Lock()
	defer recording.mu.Unlock()
	recording.file = file
}

// currentRecording returns the record that new servers write to, or nil.
func currentRecording() *testhook.File {
	recording.mu.Lock()
	defer recording.mu.Unlock()
	return recording.file
}

// RecordTest counts the test among the tests that took part in the request record, when
// RunRecorded writes one, and does nothing otherwise. A server counts the test that starts it.
// A test that takes part without a server calls RecordTest, such as the test of the operation
// map that tools/cmd/apicoverage reads with the record.
func RecordTest(tb testing.TB) {
	tb.Helper()
	if file := currentRecording(); file != nil {
		file.AddTest(tb.Name())
	}
}

// recordAs makes the server write the requests it receives to the record that RunRecorded
// writes, as those of the test tb, and counts the test. It does nothing when there is no
// record. The caller has not started the server yet.
func (s *Server) recordAs(tb testing.TB) {
	tb.Helper()
	file := currentRecording()
	if file == nil {
		return
	}
	s.record, s.test = file, tb.Name()
	file.AddTest(s.test)
}

// recordRequest writes a request that the server received to its record, if it has one.
// template is the path of the route that answered it, or empty.
func (s *Server) recordRequest(r *http.Request, template, rawPath string, status int) {
	if s.record == nil {
		return
	}
	s.record.Write(testhook.Request{
		Mode:     testhook.Wire,
		Target:   s.target,
		Method:   r.Method,
		Template: template,
		Path:     rawPath,
		Status:   status,
		Test:     s.test,
	})
}

// statusWriter keeps the status that the server answers a request with.
type statusWriter struct {
	http.ResponseWriter

	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
