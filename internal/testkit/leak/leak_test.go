package leak

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
)

// blockUntilClosed runs until ch is closed. Its name shows in the stack Check reports.
func blockUntilClosed(ch <-chan struct{}) { <-ch }

// thisPackage is the import path of this package, as stacks name its functions.
const thisPackage = "github.com/permitio/permit-golang/v2/internal/testkit/leak"

func TestCheckReportsAGoroutineLeftRunning(t *testing.T) {
	ft := faketb.New(t)
	wait := 200 * time.Millisecond
	check(ft, wait)
	release := make(chan struct{})
	defer close(release)
	go blockUntilClosed(release)

	start := time.Now()
	ft.End()
	elapsed := time.Since(start)
	errs := ft.Take()

	if len(errs) != 1 {
		t.Fatalf("Check reported %q, want one error", errs)
	}
	for _, want := range []string{
		"leak: 1 goroutines that the test started still run 200ms after it ended:\n\ngoroutine ",
		"leak.blockUntilClosed(",
		"created by github.com/permitio/permit-golang/v2/internal/testkit/leak.",
	} {
		if !strings.Contains(errs[0], want) {
			t.Errorf("Check reported %q, want it to contain %q", errs[0], want)
		}
	}
	if elapsed < wait {
		t.Errorf("Check reported the goroutine after %v, before its wait of %v ended", elapsed,
			wait)
	}
}

func TestCheckReportsAServerLeftRunning(t *testing.T) {
	ft := faketb.New(t)
	check(ft, 100*time.Millisecond)
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	ft.End()
	errs := ft.Take()

	if len(errs) != 1 || !strings.Contains(errs[0], "net/http.(*Server).Serve") {
		t.Errorf("Check reported %q, want the server's goroutine", errs)
	}
}

func TestCheckWaitsForStoppingGoroutines(t *testing.T) {
	ft := faketb.New(t)
	check(ft, 2*time.Second)
	go time.Sleep(200 * time.Millisecond)

	ft.End()
	errs := ft.Take()

	if len(errs) != 0 {
		t.Errorf("Check reported %q for a goroutine that ended within the wait", errs)
	}
}

// TestCheckWaitsTheGracePeriod checks that Check gives goroutines time to stop, so a server
// whose connections are still closing passes.
func TestCheckWaitsTheGracePeriod(t *testing.T) {
	ft := faketb.New(t)
	Check(ft)
	go time.Sleep(500 * time.Millisecond)

	ft.End()
	errs := ft.Take()

	if len(errs) != 0 {
		t.Errorf("Check reported %q for a goroutine that ended after 500ms", errs)
	}
}

func TestCheckIgnoresGoroutinesStartedBefore(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	go blockUntilClosed(release)
	ft := faketb.New(t)
	check(ft, 50*time.Millisecond)

	ft.End()
	errs := ft.Take()

	if len(errs) != 0 {
		t.Errorf("Check reported %q for a goroutine started before it", errs)
	}
}

// TestCheckPassesAClosedServer is how a wire test uses Check: first, so that its check runs after
// the server is closed and the client's connections are closed.
func TestCheckPassesAClosedServer(t *testing.T) {
	Check(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)
	client := &http.Client{Transport: &http.Transport{}}
	t.Cleanup(client.CloseIdleConnections)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// TestGoroutinesGrowsItsBuffer counts only the goroutines it starts, by the "created by" line of
// their stacks: another goroutine can start or exit between two calls of goroutines(), and a
// goroutine that has not run yet does not show blockUntilClosed in its stack.
func TestGoroutinesGrowsItsBuffer(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	const started = 100
	for range started {
		go blockUntilClosed(release)
	}
	after, err := goroutines()
	if err != nil {
		t.Fatal(err)
	}
	mine := 0
	for _, stack := range after {
		if strings.Contains(stack, "\ncreated by "+thisPackage+".TestGoroutinesGrowsItsBuffer ") {
			mine++
		}
	}
	if mine != started {
		t.Errorf("goroutines() found %d goroutines created by this test after it started %d",
			mine, started)
	}
	for id, stack := range after {
		if !strings.HasPrefix(stack, fmt.Sprintf("goroutine %d [", id)) {
			t.Errorf("goroutines()[%d] = %q, want the stack of goroutine %d", id, stack, id)
		}
	}
}

func TestParseStacks(t *testing.T) {
	first := "goroutine 1 [running]:\nmain.main()\n\t/src/main.go:5 +0x1d"
	second := "goroutine 22 [chan receive, 2 minutes]:\nmain.wait()\n\t/src/main.go:9 +0x2a\n" +
		"created by main.main in goroutine 1\n\t/src/main.go:6 +0x3b"
	got, err := parseStacks(first + "\n\n" + second + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1] != first || got[22] != second {
		t.Errorf("parseStacks() = %v, want goroutines 1 and 22", got)
	}
}

func TestParseStacksRejectsAnUnknownFormat(t *testing.T) {
	for _, dump := range []string{
		"",
		"thread 1 [running]:\nmain.main()",
		"goroutine one [running]:\nmain.main()",
		"goroutine 1 [running]:\nmain.main()\n\ngoroutine [running]:\nmain.f()",
		"Goroutine 1 [running]:",
	} {
		if got, err := parseStacks(dump); err == nil {
			t.Errorf("parseStacks(%q) = %v, want an error", dump, got)
		} else if !strings.Contains(err.Error(), "does not start with \"goroutine <ID> \"") {
			t.Errorf("parseStacks(%q) error = %v, want one about the stack's first line", dump,
				err)
		}
	}
}
