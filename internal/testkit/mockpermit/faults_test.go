package mockpermit

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
)

const userRoute = "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}"

func TestFailRequests(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle(userRoute, Reply{Body: `{"key":"first"}`}, Reply{Body: `{"key":"second"}`})
	api.Handle(usersRoute, Reply{Body: `{"key":"created"}`})
	client := newClient(t)
	get := call{method: http.MethodGet, path: "/v2/facts/p/e/users/u"}

	stop := api.FailRequests(userRoute, http.StatusServiceUnavailable, 1)
	answers := []response{get.send(t, client, api), get.send(t, client, api)}
	other := call{method: http.MethodPost, path: "/v2/facts/p/e/users"}.send(t, client, api)
	answers = append(answers, get.send(t, client, api))
	stop()
	answers = append(answers, get.send(t, client, api))

	want := []struct {
		status int
		body   string
	}{
		{http.StatusOK, `{"key":"first"}`},
		{http.StatusServiceUnavailable, faultBody},
		{http.StatusServiceUnavailable, faultBody},
		{http.StatusOK, `{"key":"second"}`},
	}
	for i, got := range answers {
		if got.status != want[i].status || got.body != want[i].body {
			t.Errorf("request %d: got %d %s, want %d %s", i+1, got.status, got.body,
				want[i].status, want[i].body)
		}
	}
	if other.status != http.StatusOK || other.body != `{"key":"created"}` {
		t.Errorf("a request to another route got %d %s, want its reply", other.status, other.body)
	}
	if ct := answers[1].header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("a failed request's Content-Type is %q, want application/json", ct)
	}
	ft.End()
	wantFailures(t, ft)
}

func TestFaultThatNeverFiresFails(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle(userRoute, Reply{}, Reply{})
	get := call{method: http.MethodGet, path: "/v2/facts/p/e/users/u"}

	api.FailRequests(userRoute, http.StatusInternalServerError, 0)()
	wantFailures(t, ft, `mockpermit API: FailRequests("`+userRoute+`", 500, 0) answered no `+
		`request; the route got 0 requests while it was set`)

	stop := api.FailRequests(userRoute, http.StatusNotFound, 2)
	get.send(t, newClient(t), api)
	get.send(t, newClient(t), api)
	stop()
	wantFailures(t, ft, `FailRequests("`+userRoute+`", 404, 2) answered no request; the route `+
		`got 2 requests while it was set`)
	stop()
	wantFailures(t, ft)
}

func TestFaultStillSetAtTheEnd(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle(userRoute, Reply{})
	api.Handle(usersRoute, Reply{})
	api.FailRequests(userRoute, http.StatusBadGateway, 0)
	api.FailRequests(usersRoute, http.StatusBadGateway, 0)
	call{method: http.MethodPost, path: "/v2/facts/p/e/users"}.send(t, newClient(t), api)

	ft.End()

	wantFailures(t, ft,
		"mockpermit API: "+userRoute+": 1 of its 1 replies were never used",
		"mockpermit API: "+usersRoute+": 1 of its 1 replies were never used",
		`mockpermit API: FailRequests("`+userRoute+`", 502, 0) answered no request`)
}

func TestFailRequestsFirstFaultDecides(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle(userRoute, Reply{})
	client := newClient(t)
	get := call{method: http.MethodGet, path: "/v2/facts/p/e/users/u"}

	stopFirst := api.FailRequests(userRoute, http.StatusInternalServerError, 0)
	stopSecond := api.FailRequests(userRoute, http.StatusBadGateway, 0)
	first := get.send(t, client, api)
	stopFirst()
	second := get.send(t, client, api)
	stopSecond()
	served := get.send(t, client, api)

	if first.status != http.StatusInternalServerError || second.status != http.StatusBadGateway ||
		served.status != http.StatusOK {
		t.Errorf("statuses %d, %d, %d; want 500 from the first fault, then 502, then 200",
			first.status, second.status, served.status)
	}
	ft.End()
	wantFailures(t, ft)
}

func TestFailRequestsRejectsBadArguments(t *testing.T) {
	for _, tt := range []struct {
		route  string
		status int
		after  int
		want   string
	}{
		{userRoute, 399, 0, "status 399 is not an error status, 400 to 599"},
		{userRoute, 600, 0, "status 600 is not an error status"},
		{userRoute, http.StatusOK, 0, "status 200 is not an error status"},
		{userRoute, 500, -1, "after is negative"},
		{
			"GET /v2/facts/{proj_id}/{env_id}/users/{id}", 500, 0,
			"the server does not serve the route; register it with Handle first",
		},
		{
			"DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}", 500, 0,
			"the server does not serve the route",
		},
		{"GET /v2/facts/p/e/users/u", 500, 0, "the server does not serve the route"},
		{"GET v2", 500, 0, "the path must start with /"},
	} {
		ft := faketb.New(t)
		api := NewAPI(ft)
		api.Handle(userRoute, Reply{})

		stop := api.FailRequests(tt.route, tt.status, tt.after)
		stop()

		wantFailures(t, ft, fmt.Sprintf("mockpermit API: FailRequests(%q, %d, %d): %s", tt.route,
			tt.status, tt.after, tt.want))
		got := call{method: http.MethodGet, path: "/v2/facts/p/e/users/u"}.send(t,
			newClient(t), api)
		if got.status != http.StatusOK {
			t.Errorf("%s: a rejected fault answered the route with %d", tt.route, got.status)
		}
		ft.End()
		wantFailures(t, ft)
	}
}

func TestFailRequestsStatusBoundaries(t *testing.T) {
	ft := faketb.New(t)
	api := NewAPI(ft)
	api.Handle(userRoute, Reply{})
	get := call{method: http.MethodGet, path: "/v2/facts/p/e/users/u"}
	for _, status := range []int{400, 599} {
		stop := api.FailRequests(userRoute, status, 0)
		if got := get.send(t, newClient(t), api); got.status != status {
			t.Errorf("status = %d, want %d", got.status, status)
		}
		stop()
	}
	get.send(t, newClient(t), api)
	ft.End()
	wantFailures(t, ft)
}
