package permit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/testkit/concurrent"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// unreadBody is the body of the replies of the keep-alive test, which the client does not read in
// full: 100 KiB, more than the client reads of an error reply, which it drains.
var unreadBody = `{"message":"` + strings.Repeat("x", 100<<10) + `"}`

// TestKeepAlive sends 50 requests, one after the other, whose replies mix 200, 404 and 500 with
// bodies that the client does not read in full, and checks that they all go over one
// connection: the client drains each body before it closes it. Without the drain, on Go 1.26,
// each request opens a connection of its own.
func TestKeepAlive(t *testing.T) {
	const requests = 50
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	var fired, deleted, failed []mockpermit.Reply
	for i := range requests {
		switch i % 3 {
		case 0:
			fired = append(fired, mockpermit.Reply{Body: unreadBody})
		case 1:
			deleted = append(deleted, mockpermit.Reply{Status: 404, Body: unreadBody})
		default:
			failed = append(failed, mockpermit.Reply{Status: 500, Body: unreadBody})
		}
	}
	api.Handle(route(opFire), fired...)
	api.Handle(route(opDeleteSelf), deleted...)
	api.Handle(route(opGet), failed...)
	c := wireClient(t, api, pdp)
	for i := range requests {
		var err error
		switch i % 3 {
		case 0:
			err = c.doNoValue(t.Context(), request{op: opFire.ID, target: apiTarget})
		case 1:
			err = c.doNoValue(t.Context(), request{
				op: opDeleteSelf.ID, target: apiTarget, params: []string{"a"},
			})
		default:
			_, err = do[testObject](t.Context(), c, get("a"))
			if e, ok := errors.AsType[*Error](err); ok &&
				e.StatusCode == http.StatusInternalServerError {
				err = nil
			}
		}
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if got := api.Connections(); got != 1 {
		t.Errorf("%d requests opened %d connections, want 1", requests, got)
	}
}

// TestConcurrentCalls makes 64 calls at once on one client, under -race, and checks that each
// call returns the reply to its own request within the deadline.
func TestConcurrentCalls(t *testing.T) {
	const calls = 64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+mockpermit.APIKey {
			http.Error(w, "the Authorization header is wrong", http.StatusUnauthorized)
			return
		}
		key := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		_, _ = io.WriteString(w, `{"key":"`+key+`","n":`+key+`}`)
	}))
	t.Cleanup(srv.Close)
	c := urlClient(t, mockpermit.APIKey, srv.URL)
	results := make([]*testObject, calls)
	err := concurrent.Run(t, calls, 30*time.Second, func(ctx context.Context, i int) error {
		got, err := do[testObject](ctx, c, get(strconv.Itoa(i)))
		results[i] = got
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, got := range results {
		if want := (testObject{Key: strconv.Itoa(i), N: i}); got == nil || *got != want {
			t.Errorf("call %d returned %v, want %v", i, got, want)
		}
	}
}
