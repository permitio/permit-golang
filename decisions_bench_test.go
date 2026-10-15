package permit

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
)

// The benchmarks of the decision calls, the SDK's hot path, measure what the client adds to each
// call, without a network: each client sends its requests through a decisionTransport, and logs
// at slog.LevelDebug, through the client's redacting handler, to a JSON handler that writes to
// io.Discard. Their ns/op and allocs/op count encoding the checks, the retry wrapper, the Debug
// line of each attempt and decoding the decisions. CONTRIBUTING.md ("Benchmarks") says how to
// run them and how CI compares them with the previous result.

// decisionTransport is an http.RoundTripper that answers each request to path with a 200 whose
// JSON body is reply, without a connection. It reads and closes the body of the request, as a
// transport that sends it does. A request to another path gets an error, so that a benchmark
// measures decisions and not an error.
type decisionTransport struct {
	path  string
	reply []byte
}

func (d decisionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_, err := io.Copy(io.Discard, req.Body)
		if closeErr := req.Body.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, err
		}
	}
	if req.URL.Path != d.path {
		return nil, fmt.Errorf("the benchmark answers requests to %s, not to %s", d.path,
			req.URL.Path)
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(d.reply)),
		ContentLength: int64(len(d.reply)),
		Request:       req,
	}, nil
}

// benchClient returns a client that sends its requests through a decisionTransport that answers
// route, a route of the PDP such as /allowed, with reply, and logs at slog.LevelDebug to a JSON
// handler that writes to io.Discard. It closes the client when the benchmark ends.
func benchClient(b *testing.B, route string, reply []byte) *Client {
	b.Helper()
	pdp, err := url.Parse(codeURL)
	if err != nil {
		b.Fatal(err)
	}
	transport := decisionTransport{path: pdp.Path + route, reply: reply}
	// slog.DiscardHandler is never enabled, so the client would not write the line to measure.
	handler := slog.NewJSONHandler( //nolint:sloglint // The Debug line must be written.
		io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	c := mustNew(b, WithAPIKey(canary.Key("bench")), WithAPIURL(codeURL), WithPDPURL(codeURL),
		WithHTTPClient(&http.Client{Transport: transport}), WithLogger(slog.New(handler)))
	b.Cleanup(func() { _ = c.Close() })
	return c
}

// benchUser is the user of every check of the benchmarks, with attributes, as an application
// that uses attribute-based policies sends it.
var benchUser = User{
	Key:        "user-4711",
	Email:      "user-4711@example.com",
	Attributes: map[string]any{"department": "finance", "level": 3, "admin": false},
}

// benchResource returns the resource of check i of a benchmark: a document of a tenant, with
// attributes.
func benchResource(i int) Resource {
	return Resource{
		Type: "document", Key: "report-" + strconv.Itoa(i), Tenant: "tenant-a",
		Attributes: map[string]any{"owner": "user-4711", "classification": "internal"},
	}
}

// bulkReply returns the reply of the PDP to n checks in one request: a decision for each, which
// allows the even ones and denies the odd ones.
func bulkReply(n int) []byte {
	var reply strings.Builder
	reply.WriteString(`{"allow":[`)
	for i := range n {
		if i > 0 {
			reply.WriteByte(',')
		}
		fmt.Fprintf(&reply, `{"allow":%t}`, i%2 == 0)
	}
	reply.WriteString(`]}`)
	return []byte(reply.String())
}

// BenchmarkCheck measures Client.Check: one check of a resource instance, POST /allowed.
func BenchmarkCheck(b *testing.B) {
	c := benchClient(b, "/allowed", []byte(`{"allow":true}`))
	resource := benchResource(0)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		allowed, err := c.Check(ctx, benchUser, "read", resource)
		if err != nil || !allowed {
			b.Fatalf("Check returned %t, %v; want true, nil", allowed, err)
		}
	}
}

// BenchmarkBulkCheck measures Client.BulkCheck of 10 and of 100 checks in one request, POST
// /allowed/bulk.
func BenchmarkBulkCheck(b *testing.B) {
	for _, n := range []int{10, 100} {
		b.Run("checks="+strconv.Itoa(n), func(b *testing.B) {
			c := benchClient(b, "/allowed/bulk", bulkReply(n))
			checks := make([]CheckRequest, n)
			for i := range checks {
				checks[i] = CheckRequest{
					User:     benchUser,
					Action:   "read",
					Resource: benchResource(i),
				}
			}
			ctx := b.Context()
			b.ReportAllocs()
			for b.Loop() {
				decisions, err := c.BulkCheck(ctx, checks)
				if err != nil || len(decisions) != n {
					b.Fatalf("BulkCheck returned %d decisions and %v; want %d and nil",
						len(decisions), err, n)
				}
			}
		})
	}
}

// BenchmarkFilterObjects measures Client.FilterObjects of 100 resources, which it checks in one
// request, POST /allowed/bulk, and of which the PDP allows half.
func BenchmarkFilterObjects(b *testing.B) {
	const n = 100
	b.Run("resources="+strconv.Itoa(n), func(b *testing.B) {
		c := benchClient(b, "/allowed/bulk", bulkReply(n))
		resources := make([]Resource, n)
		for i := range resources {
			resources[i] = benchResource(i)
		}
		ctx := b.Context()
		b.ReportAllocs()
		for b.Loop() {
			kept, err := c.FilterObjects(ctx, benchUser, "read", resources)
			if err != nil || len(kept) != n/2 {
				b.Fatalf("FilterObjects kept %d resources and returned %v; want %d and nil",
					len(kept), err, n/2)
			}
		}
	})
}
