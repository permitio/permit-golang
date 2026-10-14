package mockpermit

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
	"github.com/permitio/permit-golang/v2/internal/testkit/leak"
)

const usersRoute = "POST /v2/facts/{proj_id}/{env_id}/users"

func TestAPIKeyIsACanaryKey(t *testing.T) {
	if want := canary.Key("mockpermit"); APIKey != want {
		t.Errorf("APIKey = %q, want canary.Key(\"mockpermit\") = %q", APIKey, want)
	}
}

func TestServesTheRepliesInOrder(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	client := newClient(t)
	api.Handle(usersRoute,
		Reply{Status: http.StatusCreated, Body: `{"key":"a"}`},
		Reply{
			Status: http.StatusTooManyRequests,
			Header: http.Header{"retry-after": {"2"}, "X-Extra": {"1", "2"}},
			Body:   `{"error_code":"TOO_MANY_REQUESTS"}`,
		},
		Reply{},
		Reply{
			Status: http.StatusOK, Header: http.Header{"Content-Type": {"text/plain"}},
			Body: "ok",
		},
	)
	post := call{method: http.MethodPost, path: "/v2/facts/proj/env/users", body: `{}`}

	tests := []struct {
		status      int
		contentType string
		retryAfter  string
		extra       []string
		body        string
	}{
		{status: 201, contentType: "application/json", body: `{"key":"a"}`},
		{
			status: 429, contentType: "application/json", retryAfter: "2",
			extra: []string{"1", "2"}, body: `{"error_code":"TOO_MANY_REQUESTS"}`,
		},
		{status: 200},
		{status: 200, contentType: "text/plain", body: "ok"},
	}
	for i, want := range tests {
		got := post.send(t, client, api)
		if got.status != want.status || got.body != want.body ||
			got.header.Get("Content-Type") != want.contentType ||
			got.header.Get("Retry-After") != want.retryAfter ||
			strings.Join(got.header.Values("X-Extra"), ",") != strings.Join(want.extra, ",") {
			t.Errorf("request %d: got %d %v %q, want %d, Content-Type %q, Retry-After %q, "+
				"X-Extra %q and %q", i+1, got.status, got.header, got.body, want.status,
				want.contentType, want.retryAfter, want.extra, want.body)
		}
	}
	ft.End()
	wantFailures(t, ft)
}

func TestRecordsEveryRequest(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	client := newClient(t)
	api.Handle("PUT /v2/facts/{proj_id}/{env_id}/users/{user_id}", Reply{})

	call{
		method: http.MethodPut, path: "/v2/facts/proj/env/users/a%2Fb?page=2&per_page=10",
		body: `{"key": "a/b"}`,
	}.send(t, client, api)
	call{method: http.MethodGet, path: "/unknown?x"}.send(t, client, api)

	got := api.Requests()
	if len(got) != 2 {
		t.Fatalf("Requests() = %d requests, want 2", len(got))
	}
	put := got[0]
	if put.Method != http.MethodPut || put.RawPath != "/v2/facts/proj/env/users/a%2Fb" ||
		put.RawQuery != "page=2&per_page=10" || string(put.Body) != `{"key": "a/b"}` ||
		put.Header.Get("Authorization") != "Bearer "+APIKey {
		t.Errorf("Requests()[0] = %+v, want the PUT as it was sent", put)
	}
	if unknown := got[1]; unknown.Method != http.MethodGet || unknown.RawPath != "/unknown" ||
		unknown.RawQuery != "x" || len(unknown.Body) != 0 {
		t.Errorf("Requests()[1] = %+v, want the GET the mock refused", unknown)
	}
	wantFailures(t, ft, "mockpermit API: GET /unknown?x: no route")
}

func TestUnknownRequestsFailTheTest(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantError  string
	}{
		{
			name: "unknown path", method: http.MethodGet, path: "/v2/facts/proj/env/tenants",
			wantStatus: http.StatusNotImplemented,
			wantError: "mockpermit API: GET /v2/facts/proj/env/tenants: no route; register " +
				"it with Handle if the SDK should send this request",
		},
		{
			name: "one segment more", method: http.MethodPost, path: "/v2/facts/proj/env/users/x",
			wantStatus: http.StatusNotImplemented, wantError: "no route",
		},
		{
			name: "one segment less", method: http.MethodPost, path: "/v2/facts/proj/env",
			wantStatus: http.StatusNotImplemented, wantError: "no route",
		},
		{
			name: "unexpected method", method: http.MethodPut, path: "/v2/facts/proj/env/users",
			wantStatus: http.StatusMethodNotAllowed,
			wantError: "mockpermit API: PUT /v2/facts/proj/env/users: the route has no PUT, " +
				"only POST and GET",
		},
		{
			name: "HEAD of a GET route", method: http.MethodHead, path: "/v2/facts/proj/env/users",
			wantStatus: http.StatusMethodNotAllowed, wantError: "the route has no HEAD",
		},
		{
			name: "a literal segment that differs", method: http.MethodGet,
			path:       "/v2/facts/proj/env/user",
			wantStatus: http.StatusNotImplemented, wantError: "no route",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := faketb.New(t)
			api := NewAPI(ft)
			api.Handle(usersRoute, Reply{})
			api.Handle("GET /v2/facts/{proj_id}/{env_id}/users", Reply{})

			got := call{method: tt.method, path: tt.path}.send(t, newClient(t), api)

			if got.status != tt.wantStatus {
				t.Errorf("status = %d, want %d", got.status, tt.wantStatus)
			}
			wantFailures(t, ft, tt.wantError)
			if tt.method == http.MethodHead {
				return
			}
			var body errorBody
			if err := json.Unmarshal([]byte(got.body), &body); err != nil ||
				body.ErrorCode != "MOCKPERMIT" || body.Title != http.StatusText(tt.wantStatus) ||
				!strings.Contains(body.Message, tt.wantError) {
				t.Errorf("body = %s, want an error body that says why", got.body)
			}
		})
	}
}

func TestServersAreIndependent(t *testing.T) {
	ft := faketb.New(t)
	api, pdp := NewAPI(ft), NewPDP(ft)
	api.Handle("POST /allowed", Reply{Body: `{"allow":true}`})
	client := newClient(t)

	got := call{method: http.MethodPost, path: "/allowed"}.send(t, client, pdp)

	if got.status != http.StatusNotImplemented {
		t.Errorf("the PDP answered %d to a route only the API serves, want 501", got.status)
	}
	wantFailures(t, ft, "mockpermit PDP: POST /allowed: no route")
	call{method: http.MethodPost, path: "/allowed"}.send(t, client, api)
	if len(api.Requests()) != 1 || len(pdp.Requests()) != 1 {
		t.Errorf("the API got %d requests and the PDP %d, want 1 each", len(api.Requests()),
			len(pdp.Requests()))
	}
}

func TestAuthorizationMustBeExact(t *testing.T) {
	right := "Bearer " + APIKey
	for _, auth := range [][]string{
		{},
		{"bearer " + APIKey},
		{"BEARER " + APIKey},
		{"Bearer  " + APIKey},
		{"Bearer " + canary.Key("other")},
		{"Basic " + APIKey},
		{APIKey},
		{right, right},
	} {
		t.Run(strings.Join(auth, "|"), func(t *testing.T) {
			ft := faketb.New(t)
			api := NewAPI(ft)
			api.Handle(usersRoute, Reply{})

			got := call{method: http.MethodPost, path: "/v2/facts/p/e/users", auth: auth}.send(t,
				newClient(t), api)

			if got.status != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", got.status)
			}
			wantFailures(t, ft, fmt.Sprintf("mockpermit API: POST /v2/facts/p/e/users: the "+
				"Authorization header is %q, want exactly [%q]", auth, right))
		})
	}
}

func TestNonCanonicalPathsFailTheTest(t *testing.T) {
	for _, path := range []string{
		"/v2/facts/p//users",
		"/v2/facts/p/e/users/",
		"/v2/facts/p/./users",
		"/v2/facts/p/e/../users",
		"/v2/facts/p/%2e/users",
		"/v2/facts/p/e/%2E%2E",
	} {
		t.Run(path, func(t *testing.T) {
			ft := faketb.New(t)
			api := NewAPI(ft)
			api.Handle("POST /v2/facts/{proj_id}/{env_id}/{collection}", Reply{})

			got := call{method: http.MethodPost, path: path}.send(t, newClient(t), api)

			if got.status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", got.status)
			}
			wantFailures(t, ft, "mockpermit API: POST "+path+": the path has an empty, dot or "+
				"badly escaped segment, which usually means an empty key or ID")
		})
	}
}

// TestRequestsThatClientsCannotSend checks requests that a Go client does not send, by calling
// the server's handler directly.
func TestRequestsThatClientsCannotSend(t *testing.T) {
	tests := []struct {
		name       string
		requestURI string
		body       *errorReader
		wantStatus int
		wantError  string
	}{
		{
			name: "bad escape", requestURI: "/v2/a%zz", wantStatus: http.StatusBadRequest,
			wantError: "mockpermit API: POST /v2/a%zz: the path has an empty, dot or badly " +
				"escaped segment",
		},
		{
			name: "asterisk", requestURI: "*", wantStatus: http.StatusBadRequest,
			wantError: "mockpermit API: POST *: the path has an empty, dot or badly",
		},
		{
			name: "unreadable body", requestURI: "/v2/a", body: &errorReader{},
			wantStatus: http.StatusBadRequest,
			wantError:  "mockpermit API: POST /v2/a: reading the body: the body broke",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := faketb.New(t)
			api := NewAPI(ft)
			api.Handle("POST /v2/{x}", Reply{})
			req := &http.Request{
				Method:     http.MethodPost,
				RequestURI: tt.requestURI,
				URL:        &url.URL{Path: tt.requestURI},
				Header:     http.Header{"Authorization": {"Bearer " + APIKey}},
				Body:       http.NoBody,
			}
			if tt.body != nil {
				req.Body = tt.body
			}
			w := httptest.NewRecorder()

			api.serve(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			wantFailures(t, ft, tt.wantError)
		})
	}
}

// errorReader is a request body that cannot be read.
type errorReader struct{}

func (*errorReader) Read([]byte) (int, error) { return 0, errors.New("the body broke") }

func (*errorReader) Close() error { return nil }

func TestRepliesRunOut(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle(usersRoute, Reply{Body: `{}`})
	client := newClient(t)
	post := call{method: http.MethodPost, path: "/v2/facts/p/e/users"}

	post.send(t, client, api)
	wantFailures(t, ft)
	for _, request := range []string{"2", "3"} {
		if got := post.send(t, client, api); got.status != http.StatusNotImplemented {
			t.Errorf("request %s: status = %d, want 501", request, got.status)
		}
		wantFailures(t, ft, "mockpermit API: POST /v2/facts/p/e/users: "+usersRoute+
			" has 1 replies, and this is request "+request+" to it")
	}
	ft.End()
	wantFailures(t, ft)
}

func TestUnusedRepliesFailTheTest(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle(usersRoute, Reply{}, Reply{}, Reply{})
	api.Handle("GET /v2/api-key/scope", Reply{})
	api.Handle("DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}", Reply{})
	client := newClient(t)
	call{method: http.MethodPost, path: "/v2/facts/p/e/users"}.send(t, client, api)
	call{method: http.MethodDelete, path: "/v2/facts/p/e/users/u"}.send(t, client, api)

	ft.End()

	wantFailures(t, ft,
		"mockpermit API: "+usersRoute+": 2 of its 3 replies were never used",
		"mockpermit API: GET /v2/api-key/scope: 1 of its 1 replies were never used")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, api.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := newClient(t).Do(req); err == nil {
		_ = resp.Body.Close()
		t.Error("the server still answers after the test ended")
	}
}

func TestHandleRejectsBadRoutes(t *testing.T) {
	for _, tt := range []struct {
		route string
		want  string
	}{
		{"GET", "want one of the methods GET, POST, PUT, PATCH, DELETE, a space and a path"},
		{"FETCH /v2/x", "want one of the methods"},
		{"get /v2/x", "want one of the methods"},
		{"HEAD /v2/x", "want one of the methods"},
		{"GET v2/x", "the path must start with / and have no empty, dot or badly escaped segment"},
		{"GET /", "the path must start with /"},
		{"GET ", "the path must start with /"},
		{"GET /v2//x", "the path must start with /"},
		{"GET /v2/x/", "the path must start with /"},
		{"GET /v2/./x", "the path must start with /"},
		{"GET /v2/%zz", "the path must start with /"},
		{"GET /v2/{", `segment "{": a parameter is a whole segment, such as {user_id}`},
		{"GET /v2/{}", `segment "{}"`},
		{"GET /v2/x{id}", `segment "x{id}"`},
		{"GET /v2/{id}x", `segment "{id}x"`},
		{"GET /v2/{{id}}", `segment "{{id}}"`},
		{"GET /v2/{i?d}", `segment "{i?d}"`},
		{"GET /v2/x?page=1", `segment "x?page=1": a parameter is a whole segment, such as ` +
			"{user_id}, and a route has no query"},
		{"GET /v2/x#top", `segment "x#top"`},
		{"GET /v2/x ", `segment "x " has a space or a control character, which a client sends ` +
			"escaped; write it escaped, such as %20 for a space"},
		{"GET /v2/a b/c", `segment "a b" has a space`},
		{"GET /v2/{a b}", `segment "{a b}" has a space`},
		{"GET /v2/a\u00a0b", `segment "a\u00a0b" has a space`},
		{"GET /v2/a\tb", `segment "a\tb" has a space or a control character`},
		{"GET /v2/a\x7fb", `segment "a\x7fb" has a space or a control character`},
		{"GET /v2/a\u0085b", `segment "a\u0085b" has a space or a control character`},
	} {
		t.Run(tt.route, func(t *testing.T) {
			ft := faketb.New(t)
			api := NewAPI(ft)

			api.Handle(tt.route, Reply{})

			wantFailures(t, ft, fmt.Sprintf("mockpermit API: Handle(%q): %s", tt.route, tt.want))
			ft.End()
			wantFailures(t, ft)
		})
	}
}

func TestHandleRejectsBadReplies(t *testing.T) {
	for _, tt := range []struct {
		name    string
		replies []Reply
		want    string
	}{
		{
			name: "no replies",
			want: "no replies; give one for each request the route should answer",
		},
		{
			name: "status 199", replies: []Reply{{}, {Status: 199}},
			want: "reply 1 has status 199; want 0 for 200, or 200 to 599",
		},
		{name: "status 600", replies: []Reply{{Status: 600}}, want: "reply 0 has status 600"},
		{name: "status -1", replies: []Reply{{Status: -1}}, want: "reply 0 has status -1"},
		{
			name: "a body with 204", replies: []Reply{{}, {Status: 204, Body: `{}`}},
			want: "reply 1 has status 204 and a body, which an answer with that status cannot " +
				"carry",
		},
		{
			name: "a body with 304", replies: []Reply{{Status: 304, Body: " "}},
			want: "reply 0 has status 304 and a body",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ft := faketb.New(t)
			api := NewAPI(ft)

			api.Handle("GET /v2/x", tt.replies...)

			wantFailures(t, ft, "mockpermit API: Handle(\"GET /v2/x\"): "+tt.want)
			got := call{method: http.MethodGet, path: "/v2/x"}.send(t, newClient(t), api)
			if got.status != http.StatusNotImplemented {
				t.Errorf("a rejected route answered %d, want 501", got.status)
			}
			wantFailures(t, ft, "no route")
		})
	}
}

func TestHandleStatusBoundaries(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle("GET /v2/x", Reply{Status: 200}, Reply{Status: 599})
	client := newClient(t)
	for _, want := range []int{200, 599} {
		if got := (call{method: http.MethodGet, path: "/v2/x"}).send(t, client, api); got.status !=
			want {
			t.Errorf("status = %d, want %d", got.status, want)
		}
	}
	ft.End()
	wantFailures(t, ft)
}

// TestHandleServesRepliesWithoutABody checks that a 204 or 304 reply without a body is served as
// it was given, without a Content-Type.
func TestHandleServesRepliesWithoutABody(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle("DELETE /v2/x", Reply{Status: http.StatusNoContent},
		Reply{Status: http.StatusNotModified, Header: http.Header{"Etag": {`"1"`}}})
	client := newClient(t)
	for _, want := range []int{http.StatusNoContent, http.StatusNotModified} {
		got := call{method: http.MethodDelete, path: "/v2/x"}.send(t, client, api)
		if got.status != want || got.body != "" || got.header.Get("Content-Type") != "" {
			t.Errorf("got %d, Content-Type %q, body %q; want %d without a Content-Type or a body",
				got.status, got.header.Get("Content-Type"), got.body, want)
		}
	}
	ft.End()
	wantFailures(t, ft)
}

func TestHandleRejectsOverlappingRoutes(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle("GET /v2/users/{user_id}", Reply{})
	for _, route := range []string{
		"GET /v2/users/me",
		"GET /v2/users/{id}",
		"GET /v2/{collection}/me",
		"GET /{version}/{collection}/{key}",
	} {
		api.Handle(route, Reply{})
		wantFailures(t, ft, "mockpermit API: Handle(\""+route+"\"): a path can fit both the "+
			"route and \"GET /v2/users/{user_id}\", which the server serves")
	}
	for _, route := range []string{
		"DELETE /v2/users/{user_id}",
		"GET /v2/users",
		"GET /v2/users/{user_id}/roles",
		"GET /v3/users/me",
		"GET /v2/tenants/me",
	} {
		api.Handle(route, Reply{})
		wantFailures(t, ft)
	}
	ft.End()
	wantFailures(t, ft,
		"GET /v2/users/{user_id}: 1 of its 1 replies were never used",
		"DELETE /v2/users/{user_id}: 1 of its 1",
		"GET /v2/users: 1 of its 1",
		"GET /v2/users/{user_id}/roles: 1 of its 1",
		"GET /v3/users/me: 1 of its 1",
		"GET /v2/tenants/me: 1 of its 1")
}

func TestConnectionsCountsNewOnes(t *testing.T) {
	for _, tt := range []struct {
		name      string
		keepAlive bool
		want      int
	}{
		{name: "keep-alive", keepAlive: true, want: 1},
		{name: "a connection per request", keepAlive: false, want: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ft := faketb.New(t)
			api := NewAPI(ft)
			api.Handle("GET /v2/x", Reply{}, Reply{}, Reply{})
			transport := &http.Transport{DisableKeepAlives: !tt.keepAlive}
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport}

			if got := api.Connections(); got != 0 {
				t.Errorf("Connections() = %d before any request, want 0", got)
			}
			for range 3 {
				call{method: http.MethodGet, path: "/v2/x"}.send(t, client, api)
			}

			if got := api.Connections(); got != tt.want {
				t.Errorf("Connections() = %d after 3 requests, want %d", got, tt.want)
			}
			ft.End()
			wantFailures(t, ft)
		})
	}
}

func TestAssertUntouched(t *testing.T) {
	ft := faketb.New(t)
	api, pdp := NewAPI(ft), NewPDP(ft)
	api.Handle("GET /v2/x", Reply{})
	call{method: http.MethodGet, path: "/v2/x"}.send(t, newClient(t), api)

	pdp.AssertUntouched()
	wantFailures(t, ft)
	api.AssertUntouched()
	wantFailures(t, ft, `mockpermit API: want no connection and no request; connections: 1, `+
		`requests: ["GET /v2/x"]`)
}

// TestAssertUntouchedBareConnection checks that a connection with no request counts.
func TestAssertUntouchedBareConnection(t *testing.T) {
	ft := faketb.New(t)
	pdp := NewPDP(ft)
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp",
		strings.TrimPrefix(pdp.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	for deadline := time.Now().Add(5 * time.Second); pdp.Connections() == 0; {
		if time.Now().After(deadline) {
			t.Fatal("the server did not count the connection within 5s")
		}
		time.Sleep(time.Millisecond)
	}

	pdp.AssertUntouched()

	wantFailures(t, ft, "mockpermit PDP: want no connection and no request; connections: 1, "+
		"requests: []")
}

func TestNewPDPKinds(t *testing.T) {
	for _, tt := range []struct {
		name  string
		kinds []PDPKind
		cloud bool
	}{
		{name: "default"},
		{name: "Container", kinds: []PDPKind{Container}},
		{name: "Cloud", kinds: []PDPKind{Cloud}, cloud: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ft := faketb.New(t)
			pdp := NewPDP(ft, tt.kinds...)
			got := call{method: http.MethodPost, path: "/user-tenants"}.send(t, newClient(t), pdp)
			if wantCloud := got.status == http.StatusNotFound; wantCloud != tt.cloud {
				t.Errorf("POST /user-tenants: status %d; want the cloud PDP's 404: %t",
					got.status, tt.cloud)
			}
			if tt.cloud {
				wantFailures(t, ft)
			} else {
				wantFailures(t, ft, "mockpermit PDP: POST /user-tenants: no route")
			}
		})
	}
}

func TestNewPDPRejectsBadKinds(t *testing.T) {
	for _, tt := range []struct {
		kinds []PDPKind
		want  string
	}{
		{
			kinds: []PDPKind{Cloud, Container},
			want:  "mockpermit: NewPDP takes at most one PDPKind, got [1 0]",
		},
		{
			kinds: []PDPKind{7},
			want:  "mockpermit: NewPDP got PDPKind 7, which is neither Container nor Cloud",
		},
	} {
		ft := faketb.New(t)
		if ft.Run(func() { NewPDP(ft, tt.kinds...) }) {
			t.Errorf("NewPDP(%v) returned, want it to end the test with Fatalf", tt.kinds)
		}
		wantFailures(t, ft, tt.want)
	}
}

// TestServersStopWhenTheTestEnds checks that NewAPI and NewPDP leave no goroutine running once
// the test that started them ends.
func TestServersStopWhenTheTestEnds(t *testing.T) {
	leak.Check(t)
	api, pdp := NewAPI(t), NewPDP(t)
	api.Handle("GET /v2/x", Reply{})
	pdp.Handle("POST /allowed", Reply{Body: `{"allow":true}`})
	client := newClient(t)
	call{method: http.MethodGet, path: "/v2/x"}.send(t, client, api)
	call{method: http.MethodPost, path: "/allowed"}.send(t, client, pdp)
}

func TestSplitPath(t *testing.T) {
	for _, tt := range []struct {
		path string
		want []string
	}{
		{"/a", []string{"a"}},
		{"/a/b%2Fc/{d}", []string{"a", "b%2Fc", "{d}"}},
		{"/a/.b/..c/...", []string{"a", ".b", "..c", "..."}},
		{"", nil},
		{"/", nil},
		{"a", nil},
		{"a/b", nil},
		{"//a", nil},
		{"/a//b", nil},
		{"/a/", nil},
		{"/./a", nil},
		{"/a/..", nil},
		{"/a/%2e", nil},
		{"/a/%2E%2e", nil},
		{"/a/%", nil},
		{"/a/%zz", nil},
	} {
		got, ok := splitPath(tt.path)
		if ok != (tt.want != nil) || strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("splitPath(%q) = %q, %t, want %q", tt.path, got, ok, tt.want)
		}
	}
}

func TestIsParam(t *testing.T) {
	for segment, want := range map[string]bool{
		"{a}": true, "{user_id}": true, "{}": false, "{": false, "}": false, "a": false,
		"{a": false, "a}": false, "{{a}}": false, "{a}b": false, "b{a}": false, "{a?}": false,
		"{a#}": false, "": false,
	} {
		if got := isParam(segment); got != want {
			t.Errorf("isParam(%q) = %t, want %t", segment, got, want)
		}
	}
}

func TestMatchesAndOverlaps(t *testing.T) {
	for _, tt := range []struct {
		a, b              string
		matches, overlaps bool
	}{
		{a: "/a/b", b: "/a/b", matches: true, overlaps: true},
		{a: "/a/{x}", b: "/a/b", matches: true, overlaps: true},
		{a: "/a/b", b: "/a/{x}", matches: false, overlaps: true},
		{a: "/{x}/b", b: "/a/{y}", matches: false, overlaps: true},
		{a: "/a/b", b: "/a/c", matches: false, overlaps: false},
		{a: "/a/{x}", b: "/b/{x}", matches: false, overlaps: false},
		{a: "/a/{x}", b: "/a/b/c", matches: false, overlaps: false},
		{a: "/a/b/c", b: "/a/{x}", matches: false, overlaps: false},
	} {
		a, _ := splitPath(tt.a)
		b, _ := splitPath(tt.b)
		if got := matches(a, b); got != tt.matches {
			t.Errorf("matches(%s, %s) = %t, want %t", tt.a, tt.b, got, tt.matches)
		}
		if got := overlaps(a, b); got != tt.overlaps {
			t.Errorf("overlaps(%s, %s) = %t, want %t", tt.a, tt.b, got, tt.overlaps)
		}
	}
}
