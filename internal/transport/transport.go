// Package transport sends one attempt of a request of the SDK to the Permit API or to a PDP, and
// reads the reply. It decodes nothing: the permit package decides what a reply means. Its
// [Policy] decides whether a call sends its request again after an attempt that failed, by the
// facts of the call's operation, and how long the call waits before it does.
//
// Every request carries the client's API key in Authorization, the SDK's name and version in
// User-Agent, X-Permit-Sdk-Language and X-Permit-Sdk-Version, Accept: application/json and the
// headers of its [Request.Header]; a request with a body also carries Content-Type:
// application/json. Send reads a reply's body only as far as the caller needs it, then reads and
// discards up to [MaxDrain] bytes of the rest, and closes the body on every path, so that the
// connection can carry the next request. After each attempt, Send writes it to the request record
// of the [testhook.Recorder] that the context carries, when a test put one there.
package transport

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/testhook"
)

// The headers that Send sets on requests or reads from replies, in the canonical form that
// net/http uses. The exported ones and their values are what tests outside the package check
// requests against.
const (
	HeaderAccept        = "Accept"
	headerAuthorization = "Authorization"
	HeaderContentType   = "Content-Type"
	headerRetryAfter    = "Retry-After"
	HeaderSDKLanguage   = "X-Permit-Sdk-Language"
	HeaderSDKVersion    = "X-Permit-Sdk-Version"
	HeaderUserAgent     = "User-Agent"
)

// JSONType is the media type of the bodies that the SDK sends and reads.
const JSONType = "application/json"

// Language is the value of X-Permit-Sdk-Language, which the PDP counts the SDK's requests by.
const Language = "go"

// MaxDrain is the most bytes of what is left of a reply's body, after what the caller needs of
// it, that Send reads and discards, so that the connection can carry the next request. A body
// with more left is closed with its connection, rather than read to its end.
const MaxDrain = 1 << 20

// MaxErrorBody is the most bytes of the body of a reply whose status is not in 200-299 that Send
// reads: more than the error bodies of the API and the PDP hold. An error keeps a part of it.
const MaxErrorBody = 64 << 10

// Sender sends the requests of one client. Build one with [New]. It is safe for concurrent use.
type Sender struct {
	http      *http.Client
	apiKey    redact.Secret
	version   string
	userAgent string
}

// New returns a Sender that sends its requests through hc with apiKey, and names the SDK at
// version in its headers.
func New(hc *http.Client, apiKey redact.Secret, version string) *Sender {
	return &Sender{http: hc, apiKey: apiKey, version: version, userAgent: UserAgent(version)}
}

// UserAgent returns the User-Agent of the SDK at version: permit-golang/, the version, and the
// version of Go that built the program in parentheses, such as permit-golang/2.0.0 (go1.27.1).
func UserAgent(version string) string {
	return "permit-golang/" + version + " (" + runtime.Version() + ")"
}

// Request is one attempt of a request.
type Request struct {
	// Method is the HTTP method, such as GET.
	Method string

	// BaseURL is the URL of the API or of the PDP, such as https://api.permit.io, without a
	// slash at its end.
	BaseURL string

	// Path follows BaseURL: the path of the operation's route with its parameters escaped, such
	// as /v2/facts/p/e/users/a%2Fb.
	Path string

	// Query is the query without the "?", or empty when the request has none.
	Query string

	// Template is the route template, such as /v2/facts/{proj_id}/{env_id}/users/{user_id}, for
	// the request record.
	Template string

	// Target is the server the request goes to, for the request record.
	Target testhook.Target

	// Body is the JSON body, or nil when the request has none.
	Body []byte

	// Header holds the headers of this request alone, such as the wait for sync of a facts
	// write, or is nil. Send does not change it, and a header that Send sets itself, such as
	// Authorization, replaces one of the same name here.
	Header http.Header

	// ReadBody says that the caller reads the body of a reply in 200-299. Send reads the body of
	// a reply with another status in any case, up to MaxErrorBody bytes.
	ReadBody bool
}

// Reply is what an attempt got back.
type Reply struct {
	// Status is the status of the reply, or 0 when no reply came.
	Status int

	// Body is the reply's body as far as Send read it: all of it for a reply in 200-299 when
	// Request.ReadBody is set, at most MaxErrorBody bytes for a reply whose status is not in
	// 200-299, and nil for any other reply.
	Body []byte

	// Duration is how long the attempt took, from its start until the body was closed or the
	// request failed.
	Duration time.Duration

	// RetryAfter is the reply's Retry-After header, or empty when it has none or no reply came.
	RetryAfter string
}

// Send sends one attempt of r, which ctx bounds, and returns the reply. It returns an error when
// the attempt got no complete reply, such as when ctx ended or the connection failed: then the
// Reply's Status is the status of a reply whose body Send could not read, or 0 when no reply
// came. Send then writes the attempt to the request record of the testhook.Recorder that ctx
// carries, if any, with the path of the route and without BaseURL's path, the query or the body.
func (s *Sender) Send(ctx context.Context, r Request) (Reply, error) {
	start := time.Now()
	reply, err := s.send(ctx, r)
	reply.Duration = time.Since(start)
	testhook.Record(ctx, testhook.Attempt{
		Target:   r.Target,
		Method:   r.Method,
		Template: r.Template,
		Path:     r.Path,
		Status:   reply.Status,
	})
	return reply, err
}

// send sends r with ctx and reads the reply's body as Reply says.
func (s *Sender) send(ctx context.Context, r Request) (Reply, error) {
	var body io.Reader
	if r.Body != nil {
		body = bytes.NewReader(r.Body)
	}
	target := r.BaseURL + r.Path
	if r.Query != "" {
		target += "?" + r.Query
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, target, body)
	if err != nil {
		return Reply{}, err
	}
	for name, values := range r.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	s.setHeaders(req.Header, r.Body != nil)
	resp, err := s.http.Do(req)
	if err != nil {
		// net/http closes the body of the reply it returns with an error.
		return Reply{}, err
	}
	defer func() {
		drain(resp.Body)
		// The close of a reply's body reports nothing that the caller could act on.
		_ = resp.Body.Close()
	}()
	reply := Reply{Status: resp.StatusCode, RetryAfter: resp.Header.Get(headerRetryAfter)}
	// The checks are if statements, not the cases of a switch, so that coverage, and with it
	// the mutation tests, reach each condition.
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		reply.Body, err = io.ReadAll(io.LimitReader(resp.Body, MaxErrorBody))
	} else if r.ReadBody {
		reply.Body, err = io.ReadAll(resp.Body)
	}
	return reply, err
}

// setHeaders sets the headers of a request in h; hasBody says that the request has a body.
func (s *Sender) setHeaders(h http.Header, hasBody bool) {
	h.Set(headerAuthorization, "Bearer "+s.apiKey.Value())
	h.Set(HeaderUserAgent, s.userAgent)
	h.Set(HeaderSDKLanguage, Language)
	h.Set(HeaderSDKVersion, s.version)
	h.Set(HeaderAccept, JSONType)
	if hasBody {
		h.Set(HeaderContentType, JSONType)
	}
}

// drain reads and discards up to MaxDrain bytes of what is left of body, the body of a reply, so
// that its connection goes back to the idle connections of the transport when the body is closed,
// when that was all of it. The close of a body with more left closes the connection.
func drain(body io.Reader) {
	// A read that fails leaves the connection to the close, which then closes it.
	_, _ = io.CopyN(io.Discard, body, MaxDrain)
}
