package mockpermit

import (
	"errors"
	"fmt"
	"slices"
)

// faultBody is the body of every answer to a request that FailRequests fails. It says the object
// was not found whatever the status, so that a client that decides from the body instead of the
// status is caught.
const faultBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
	`"message":"The requested object was not found"}`

// fault is a set fault of FailRequests.
type fault struct {
	call   string // how FailRequests was called, for messages
	route  *route
	status int
	// after is how many requests to the route are served before the fault answers.
	after int
	seen  int
	// fired says the fault answered a request.
	fired bool
}

// FailRequests answers the requests to route with status and an error body instead of the
// route's replies, until the returned stop function is called. route is a route the server
// serves, written as it was given to Handle. The first after requests to the route are served as
// usual. A failed request does not use one of the route's replies, and gets a body that says the
// object was not found whatever the status, so that a client that decides from the body instead
// of the status is caught.
//
// stop fails the test when the fault answered no request, so that a test cannot pass without
// reaching it, and so does the end of the test while the fault is still set. FailRequests fails
// the test when the server does not serve route, status is not 400 to 599, or after is negative.
func (s *Server) FailRequests(route string, status, after int) (stop func()) {
	s.report.Helper()
	call := fmt.Sprintf("FailRequests(%q, %d, %d)", route, status, after)
	f, err := s.addFault(call, route, status, after)
	if err != nil {
		s.report.Errorf("mockpermit %s: %s: %v", s.name, call, err)
		return func() {}
	}
	return func() {
		s.report.Helper()
		s.mu.Lock()
		i := slices.Index(s.faults, f)
		if i >= 0 {
			s.faults = slices.Delete(s.faults, i, i+1)
		}
		unfired := i >= 0 && !f.fired
		s.mu.Unlock()
		if unfired {
			s.report.Errorf("mockpermit %s: %s", s.name, f.unfired())
		}
	}
}

// addFault sets a fault; call is how FailRequests was called.
func (s *Server) addFault(call, pattern string, status, after int) (*fault, error) {
	if status < 400 || status > 599 {
		return nil, fmt.Errorf("status %d is not an error status, 400 to 599", status)
	}
	if after < 0 {
		return nil, errors.New("after is negative")
	}
	want, err := parseRoute(pattern)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rt := range s.routes {
		if rt.method == want.method && slices.Equal(rt.segments, want.segments) {
			f := &fault{call: call, route: rt, status: status, after: after}
			s.faults = append(s.faults, f)
			return f, nil
		}
	}
	return nil, errors.New("the server does not serve the route; register it with Handle first")
}

// firingFault returns the fault that answers the next request to rt, or nil when the route
// serves it. The fault set first for the route decides. The caller holds s.mu.
func (s *Server) firingFault(rt *route) *fault {
	for _, f := range s.faults {
		if f.route != rt {
			continue
		}
		f.seen++
		if f.seen <= f.after {
			return nil
		}
		f.fired = true
		return f
	}
	return nil
}

// unfired says that the fault answered no request. The caller holds s.mu, or the fault is no
// longer set.
func (f *fault) unfired() string {
	return fmt.Sprintf("%s answered no request; the route got %d requests while it was set",
		f.call, f.seen)
}
