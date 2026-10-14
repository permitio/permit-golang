// Package mockpermit serves strict fakes of the Permit API and the PDP for the SDK's offline wire
// tests. It is for tests only.
//
// NewAPI and NewPDP each start a local HTTP server. A server answers only the routes that a test
// registers with [Server.Handle], with the replies the test scripts for each, and records every
// request, so that the test can check what went over the wire with [Request.ExpectJSON] and
// [Request.ExpectBytes]. Each of these fails the test:
//
//   - a request whose Authorization header is not exactly "Bearer " and [APIKey], such as one
//     that spells it "bearer";
//   - a path with an empty or dot segment, which usually means an empty key or ID;
//   - a request to a route the test did not register, or with a method the route does not have;
//   - more requests to a route than it has replies;
//   - when the test ends, a reply that no request used, and a fault from [Server.FailRequests]
//     that answered no request.
//
// The API and the PDP are separate servers, so that a test can check which one a request went
// to, and with [Server.AssertUntouched] that the other one got nothing. NewPDP(t, Cloud) fakes
// the cloud PDP, which answers 404 with an empty body on the routes that only the PDP container
// serves. StartExample starts the servers for the Examples of a package, which have no
// testing.TB.
//
// The strict route matching, the request recording, the scripted faults with their check that
// they fired, and the canonical Bearer check are adapted from the offline fake of the Permit API
// written for the Terraform provider for Permit.io.
package mockpermit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode"
)

// APIKey is the API key the servers accept: every request must carry the header
// "Authorization: Bearer " + APIKey. It is canary.Key("mockpermit").
//
//nolint:gosec // G101: a fake key that only these servers accept.
const APIKey = "permit_key_FAKEmockpermit000000000000000000000000000000000000000"

// PDPKind says which PDP NewPDP fakes.
type PDPKind int

const (
	// Container is the PDP container that an application runs. It serves the routes the test
	// registers.
	Container PDPKind = iota

	// Cloud is the cloud PDP. It answers every request to a route that only the PDP container
	// serves with 404 and an empty body, as the cloud PDP does, and that request does not fail
	// the test. A test cannot register such a route.
	Cloud
)

// Reply is the answer to one request.
type Reply struct {
	// Status is the HTTP status code; 0 means 200.
	Status int

	// Header holds the response headers, such as Retry-After. Content-Type is
	// application/json when Body is not empty and Header does not set it.
	Header http.Header

	// Body is the response body, sent as it is. A reply with status 204 or 304 has none.
	Body string
}

// Request is one HTTP request that a server received.
type Request struct {
	// Method is the request method, such as POST.
	Method string

	// RawPath is the path exactly as the client sent it, with its escapes, such as
	// /v2/facts/proj/env/users/a%2Fb.
	RawPath string

	// RawQuery is the query exactly as the client sent it, without the "?".
	RawQuery string

	// Header holds the request headers.
	Header http.Header

	// Body is the request body.
	Body []byte
}

// reporter receives the failures a server finds: the testing.TB of the test, or the collector
// of StartExample.
type reporter interface {
	Helper()
	Errorf(format string, args ...any)
}

// Server is a running fake of the Permit API or of the PDP. Its methods are safe for concurrent
// use.
type Server struct {
	// URL is the server's base URL, such as http://127.0.0.1:50123, without a trailing slash.
	URL string

	name   string
	report reporter
	// cloud makes the server answer like the cloud PDP.
	cloud bool
	// cycle starts a route's replies over after the last one, and keeps finish from reporting
	// replies that no request used, for the servers of StartExample.
	cycle bool
	srv   *httptest.Server

	mu          sync.Mutex
	routes      []*route
	faults      []*fault
	requests    []Request
	connections int
}

// route is a route a server serves, and its replies.
type route struct {
	// pattern is the route as the test wrote it, such as "GET /v2/users/{user_id}".
	pattern string
	method  string
	// segments are the segments of the route's path; a {name} segment fits any segment.
	segments []string
	replies  []Reply
	// calls counts the requests that got a reply, or would have had the replies not run out.
	calls int
}

// refusal is why a server cannot answer a request: the status it answers with and the message
// it fails the test with.
type refusal struct {
	status  int
	message string
}

// NewAPI starts a fake Permit API that fails the test on every request it cannot answer, and
// stops it when the test ends.
func NewAPI(tb testing.TB) *Server {
	tb.Helper()
	s := &Server{name: "API", report: tb}
	s.start()
	tb.Cleanup(s.finish)
	return s
}

// NewPDP starts a fake PDP that fails the test on every request it cannot answer, and stops it
// when the test ends. It fakes the PDP container, or the cloud PDP with NewPDP(tb, Cloud).
func NewPDP(tb testing.TB, kind ...PDPKind) *Server {
	tb.Helper()
	s := &Server{name: "PDP", report: tb}
	switch {
	case len(kind) > 1:
		tb.Fatalf("mockpermit: NewPDP takes at most one PDPKind, got %v", kind)
	case len(kind) == 1 && kind[0] == Cloud:
		s.name, s.cloud = "cloud PDP", true
	case len(kind) == 1 && kind[0] != Container:
		tb.Fatalf(
			"mockpermit: NewPDP got PDPKind %d, which is neither Container nor Cloud",
			kind[0],
		)
	}
	s.start()
	tb.Cleanup(s.finish)
	return s
}

// start starts the server. Its settings must not change after.
func (s *Server) start() {
	s.srv = httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	s.srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			s.mu.Lock()
			s.connections++
			s.mu.Unlock()
		}
	}
	s.srv.Start()
	s.URL = s.srv.URL
}

// finish stops the server, then fails the test for each route with replies that no request used
// and for each fault that is still set and answered no request.
func (s *Server) finish() {
	s.report.Helper()
	s.srv.Close()
	s.mu.Lock()
	var problems []string
	for _, rt := range s.routes {
		if unused := len(rt.replies) - rt.calls; unused > 0 && !s.cycle {
			problems = append(problems, fmt.Sprintf("%s: %d of its %d replies were never used",
				rt.pattern, unused, len(rt.replies)))
		}
	}
	for _, f := range s.faults {
		if !f.fired {
			problems = append(problems, f.unfired())
		}
	}
	s.faults = nil
	s.mu.Unlock()
	for _, problem := range problems {
		s.report.Errorf("mockpermit %s: %s", s.name, problem)
	}
}

// Handle serves route with replies: the first request to the route gets the first reply, the
// next request the next reply, and a request after the last reply fails the test. route is a
// method and a path, such as "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}". The method is
// GET, POST, PUT, PATCH or DELETE. A path segment written {name} fits any one segment, as the
// client escaped it; every other segment must be the same as the client sent it, so a space or
// a control character in it is written escaped.
//
// Handle fails the test when route is malformed, could fit a path that a route with the same
// method already fits, or is a route that the cloud PDP does not serve; and when replies is
// empty, has a status other than 0 or 200 to 599, or has a body with status 204 or 304.
func (s *Server) Handle(route string, replies ...Reply) {
	s.report.Helper()
	if err := s.handle(route, replies); err != nil {
		s.report.Errorf("mockpermit %s: Handle(%q): %v", s.name, route, err)
	}
}

func (s *Server) handle(pattern string, replies []Reply) error {
	rt, err := parseRoute(pattern)
	if err != nil {
		return err
	}
	if len(replies) == 0 {
		return errors.New("no replies; give one for each request the route should answer")
	}
	for i, reply := range replies {
		if reply.Status != 0 && (reply.Status < 200 || reply.Status > 599) {
			return fmt.Errorf("reply %d has status %d; want 0 for 200, or 200 to 599", i,
				reply.Status)
		}
		noBody := reply.Status == http.StatusNoContent || reply.Status == http.StatusNotModified
		if noBody && reply.Body != "" {
			return fmt.Errorf("reply %d has status %d and a body, which an answer with that "+
				"status cannot carry", i, reply.Status)
		}
	}
	if s.cloud && containerOnly(rt.method, rt.segments, overlaps) {
		return errors.New("only the PDP container serves the route; the cloud PDP answers it " +
			"with 404 and an empty body without a route")
	}
	rt.replies = slices.Clone(replies)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, other := range s.routes {
		if other.method == rt.method && overlaps(other.segments, rt.segments) {
			return fmt.Errorf("a path can fit both the route and %q, which the server serves",
				other.pattern)
		}
	}
	s.routes = append(s.routes, rt)
	return nil
}

// Requests returns the requests the server has received, oldest first, also those it failed.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

// Connections returns how many connections clients have opened to the server. A client that
// keeps its connections alive sends many requests over one.
func (s *Server) Connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections
}

// AssertUntouched fails the test when a client opened a connection to the server or sent it a
// request. A routing test calls it on the server that a request must not go to.
func (s *Server) AssertUntouched() {
	s.report.Helper()
	s.mu.Lock()
	connections := s.connections
	var sent []string
	for _, r := range s.requests {
		sent = append(sent, r.Method+" "+r.RawPath)
	}
	s.mu.Unlock()
	if connections > 0 || len(sent) > 0 {
		s.report.Errorf("mockpermit %s: want no connection and no request; connections: %d, "+
			"requests: %q", s.name, connections, sent)
	}
}

// serve records a request, then answers it with its route's next reply, or fails the test and
// answers with an error.
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	rawPath, rawQuery, _ := strings.Cut(r.RequestURI, "?")
	body, readErr := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, Request{
		Method:   r.Method,
		RawPath:  rawPath,
		RawQuery: rawQuery,
		Header:   r.Header.Clone(),
		Body:     body,
	})
	s.mu.Unlock()

	request := r.Method + " " + r.RequestURI
	if readErr != nil {
		s.refuse(w, refusal{http.StatusBadRequest, "reading the body: " + readErr.Error()},
			request)
		return
	}
	if auth := r.Header.Values("Authorization"); len(auth) != 1 || auth[0] != "Bearer "+APIKey {
		s.refuse(w, refusal{http.StatusUnauthorized, fmt.Sprintf(
			"the Authorization header is %q, want exactly [%q]", auth, "Bearer "+APIKey)}, request)
		return
	}
	segments, ok := splitPath(rawPath)
	if !ok {
		s.refuse(w, refusal{http.StatusBadRequest, "the path has an empty, dot or badly " +
			"escaped segment, which usually means an empty key or ID"}, request)
		return
	}
	if s.cloud && containerOnly(r.Method, segments, matches) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	reply, refused := s.pick(r.Method, segments)
	if refused != nil {
		s.refuse(w, *refused, request)
		return
	}
	writeReply(w, reply)
}

// pick returns the reply to a request, or why the server cannot answer it.
func (s *Server) pick(method string, segments []string) (Reply, *refusal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var others []string
	for _, rt := range s.routes {
		if !matches(rt.segments, segments) {
			continue
		}
		if rt.method != method {
			others = append(others, rt.method)
			continue
		}
		if f := s.firingFault(rt); f != nil {
			return Reply{Status: f.status, Body: faultBody}, nil
		}
		return rt.next(s.cycle)
	}
	if len(others) > 0 {
		return Reply{}, &refusal{http.StatusMethodNotAllowed, fmt.Sprintf(
			"the route has no %s, only %s", method, strings.Join(others, " and "))}
	}
	return Reply{}, &refusal{
		http.StatusNotImplemented,
		"no route; register it with Handle if the SDK should send this request",
	}
}

// next returns the route's next reply. With cycle, the replies start over after the last one.
func (rt *route) next(cycle bool) (Reply, *refusal) {
	i := rt.calls
	rt.calls++
	if cycle {
		i %= len(rt.replies)
	}
	if i >= len(rt.replies) {
		return Reply{}, &refusal{http.StatusNotImplemented, fmt.Sprintf(
			"%s has %d replies, and this is request %d to it", rt.pattern, len(rt.replies),
			rt.calls)}
	}
	return rt.replies[i], nil
}

// refuse fails the test with the reason the server cannot answer a request, and answers it with
// the status and an error body that gives the reason.
func (s *Server) refuse(w http.ResponseWriter, why refusal, request string) {
	message := fmt.Sprintf("mockpermit %s: %s: %s", s.name, request, why.message)
	s.report.Errorf("%s", message)
	body, err := json.Marshal(errorBody{
		ErrorCode: "MOCKPERMIT",
		Title:     http.StatusText(why.status),
		Message:   message,
	})
	if err != nil {
		s.report.Errorf("mockpermit %s: encoding the error body: %v", s.name, err)
	}
	writeReply(w, Reply{Status: why.status, Body: string(body)})
}

// errorBody is the body of an answer that refuses a request, shaped like the Permit API's error
// body.
type errorBody struct {
	ErrorCode string `json:"error_code"`
	Title     string `json:"title"`
	Message   string `json:"message"`
}

func writeReply(w http.ResponseWriter, reply Reply) {
	for name, values := range reply.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	if reply.Body != "" && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	status := reply.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	// A client that gave up on the request, such as in a timeout test, does not read the body,
	// and the test decides from what the client did.
	_, _ = io.WriteString(w, reply.Body)
}

// methods are the methods a route can have: those the Permit API and the PDP use.
var methods = []string{
	http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
}

// parseRoute parses a route written "METHOD /path/{param}".
func parseRoute(pattern string) (*route, error) {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok || !slices.Contains(methods, method) {
		return nil, fmt.Errorf("want one of the methods %s, a space and a path",
			strings.Join(methods, ", "))
	}
	segments, ok := splitPath(path)
	if !ok {
		return nil, errors.New("the path must start with / and have no empty, dot or badly " +
			"escaped segment")
	}
	for _, segment := range segments {
		if strings.ContainsFunc(segment, isSpaceOrControl) {
			return nil, fmt.Errorf("segment %q has a space or a control character, which a "+
				"client sends escaped; write it escaped, such as %%20 for a space", segment)
		}
		if strings.ContainsAny(segment, "{}?#") && !isParam(segment) {
			return nil, fmt.Errorf("segment %q: a parameter is a whole segment, such as "+
				"{user_id}, and a route has no query", segment)
		}
	}
	return &route{pattern: pattern, method: method, segments: segments}, nil
}

// isSpaceOrControl reports whether r is a space or a control character, which no request path
// holds unescaped.
func isSpaceOrControl(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r)
}

// isParam reports whether a route's path segment is a parameter, such as {user_id}.
func isParam(segment string) bool {
	name, ok := strings.CutPrefix(segment, "{")
	name, closed := strings.CutSuffix(name, "}")
	return ok && closed && name != "" && !strings.ContainsAny(name, "{}?#")
}

// splitPath returns the segments of a path that starts with / and has no empty, "." or ".."
// segment, also when escaped, and false for any other path.
func splitPath(path string) ([]string, bool) {
	segments := strings.Split(path, "/")
	if len(segments) < 2 || segments[0] != "" {
		return nil, false
	}
	segments = segments[1:]
	for _, segment := range segments {
		unescaped, err := url.PathUnescape(segment)
		if err != nil || unescaped == "" || unescaped == "." || unescaped == ".." {
			return nil, false
		}
	}
	return segments, true
}

// matches reports whether path, the segments of a request's path, fits template, the segments
// of a route's path: a {name} segment of template fits any segment, and every other one must be
// the same.
func matches(template, path []string) bool {
	if len(template) != len(path) {
		return false
	}
	for i, segment := range template {
		if segment != path[i] && !isParam(segment) {
			return false
		}
	}
	return true
}

// overlaps reports whether a path can fit both templates, the segments of two routes' paths.
func overlaps(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] && !isParam(a[i]) && !isParam(b[i]) {
			return false
		}
	}
	return true
}
