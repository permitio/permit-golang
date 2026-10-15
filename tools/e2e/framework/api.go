package framework

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// maxReplyBytes bounds the body of a reply that API reads.
const maxReplyBytes = 8 << 20

// retryPolicy says how API retries a request that the API answers with status 429: this retry is
// the test layer's own, apart from the SDK's, so that the scenarios survive the rate limits of a
// shared project.
type retryPolicy struct {
	// Attempts is the most attempts of one request, the first one included.
	Attempts int

	// Base is the backoff after the first 429 whose reply has no Retry-After header. It doubles
	// after each 429, up to Max, and each wait is half of it plus a random part of the other half.
	Base time.Duration

	// Max bounds each wait, also one that a Retry-After header asks for.
	Max time.Duration
}

// defaultRetry is the retryPolicy of an API that NewAPI returns.
var defaultRetry = retryPolicy{Attempts: 6, Base: time.Second, Max: 30 * time.Second}

// API is a client of the Permit API in plain net/http, which sends each request with its own API
// key. It is safe for concurrent use.
type API struct {
	base   string
	key    string
	client *http.Client
	retry  retryPolicy

	// interval is the wait between two tries of a check that Absence returns.
	interval time.Duration

	// sleep waits d, or returns the error of ctx when ctx ends first.
	sleep func(ctx context.Context, d time.Duration) error

	// jitter returns a random number in [0, 1).
	jitter func() float64
}

// NewAPI returns an API that sends its requests with client to baseURL, an http or https URL
// without a query or a fragment, such as https://api.permit.io, and authenticates them with key.
func NewAPI(baseURL, key string, client *http.Client) (*API, error) {
	u, err := url.Parse(baseURL)
	switch {
	case err != nil:
		return nil, fmt.Errorf("framework: the API URL: %w", err)
	case u.Scheme != "http" && u.Scheme != "https" || u.Host == "":
		return nil, fmt.Errorf("framework: the API URL %q is not an http or https URL with a "+
			"host", baseURL)
	case u.RawQuery != "" || u.Fragment != "" || u.User != nil:
		return nil, fmt.Errorf("framework: the API URL %q has a query, a fragment or a user",
			baseURL)
	case key == "":
		return nil, errors.New("framework: the API needs an API key")
	case client == nil:
		return nil, errors.New("framework: the API needs an HTTP client")
	}
	return &API{
		base: u.String(), key: key, client: client, retry: defaultRetry, interval: time.Second,
		sleep: sleep, jitter: rand.Float64,
	}, nil
}

// sleep waits d, or returns the error of ctx when ctx ends first.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Request is a request to the API.
type Request struct {
	// Method is the HTTP method, such as POST.
	Method string

	// Path is the path of the request with its parameters escaped, such as /v2/projects/p/envs.
	Path string

	// Body is sent as JSON, or nothing is sent when it is nil.
	Body any
}

// String returns the method and the path of r, such as POST /v2/projects/p/envs.
func (r Request) String() string {
	return r.Method + " " + r.Path
}

// Response is the reply to a request.
type Response struct {
	// Status is the status code.
	Status int

	// Body is the body, up to 8 MiB.
	Body []byte
}

// Do sends r and returns the reply. After a reply with status 429 it waits as the reply's
// Retry-After header says, in seconds or as a date, or else for a backoff with jitter that
// starts at a second and doubles, each wait at most 30 seconds, and sends r again, up to 6
// attempts in all. It returns an error when the body cannot be encoded, no reply came, ctx
// ended, or the last attempt got 429.
func (a *API) Do(ctx context.Context, r Request) (Response, error) {
	var body []byte
	if r.Body != nil {
		var err error
		if body, err = json.Marshal(r.Body); err != nil {
			return Response{}, fmt.Errorf("framework: %s: encoding the body: %w", r, err)
		}
	}
	backoff := a.retry.Base
	for attempt := 1; ; attempt++ {
		reply, header, err := a.send(ctx, r, body)
		if err != nil {
			return Response{}, err
		}
		if reply.Status != http.StatusTooManyRequests {
			return reply, nil
		}
		if attempt >= a.retry.Attempts {
			return reply, fmt.Errorf("framework: %s: still 429 after %d attempts", r, attempt)
		}
		wait, ok := retryAfter(header.Get("Retry-After"), time.Now())
		if !ok {
			wait = backoff/2 + time.Duration(a.jitter()*float64(backoff/2))
			backoff = min(2*backoff, a.retry.Max)
		}
		if err := a.sleep(ctx, min(wait, a.retry.Max)); err != nil {
			return Response{}, fmt.Errorf("framework: %s: waiting to retry a 429: %w", r, err)
		}
	}
}

// send sends one attempt of r with body, and returns its reply and the reply's header.
func (a *API) send(ctx context.Context, r Request, body []byte) (Response, http.Header, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, a.base+r.Path, reader)
	if err != nil {
		return Response{}, nil, fmt.Errorf("framework: %s: %w", r, err)
	}
	req.Header.Set("Authorization", "Bearer "+a.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return Response{}, nil, fmt.Errorf("framework: %s: %w", r, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReplyBytes))
	if err != nil {
		return Response{}, nil, fmt.Errorf("framework: %s: reading the reply: %w", r, err)
	}
	return Response{Status: resp.StatusCode, Body: data}, resp.Header, nil
}

// retryAfter returns the wait that value, a Retry-After header, asks for at now: a number of
// seconds, or the time until a date; false when value is neither. A date in the past is no wait.
func retryAfter(value string, now time.Time) (time.Duration, bool) {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(date.Sub(now), 0), true
	}
	return 0, false
}

// Call sends r as Do does, and decodes the body of a reply with a status in 200-299 into out,
// unless out is nil. Another status is an error that names r, the status and the start of the
// body.
func (a *API) Call(ctx context.Context, r Request, out any) error {
	reply, err := a.Do(ctx, r)
	if err != nil {
		return err
	}
	if reply.Status < 200 || reply.Status > 299 {
		return statusError(r, reply)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(reply.Body, out); err != nil {
		return fmt.Errorf("framework: %s: decoding the reply: %w", r, err)
	}
	return nil
}

// statusError returns the error of reply, the reply to r with an unexpected status.
func statusError(r Request, reply Response) error {
	body := reply.Body
	if len(body) > 300 {
		body = append(body[:300:300], "..."...)
	}
	return fmt.Errorf("framework: %s answered %d: %s", r, reply.Status, body)
}

// Remover returns a teardown that sends DELETE to path and succeeds on a reply with status 200,
// 204 or 404: an object that is gone already is removed.
func (a *API) Remover(path string) func(context.Context) error {
	return func(ctx context.Context) error {
		r := Request{Method: http.MethodDelete, Path: path}
		reply, err := a.Do(ctx, r)
		if err != nil {
			return err
		}
		switch reply.Status {
		case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
			return nil
		}
		return statusError(r, reply)
	}
}

// Absence returns a check that sends GET to path until the reply has status 404, for at most
// timeout, and fails on a status other than 200 and 404, as a check that an object is gone that
// does not trust the reply to its deletion.
func (a *API) Absence(path string, timeout time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		r := Request{Method: http.MethodGet, Path: path}
		return Poll(ctx, timeout, a.interval, func(ctx context.Context) (bool, error) {
			reply, err := a.Do(ctx, r)
			switch {
			case err != nil:
				return false, err
			case reply.Status == http.StatusNotFound:
				return true, nil
			case reply.Status == http.StatusOK:
				return false, nil
			}
			return false, statusError(r, reply)
		})
	}
}
