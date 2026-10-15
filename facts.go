package permit

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/permitio/permit-golang/v2/internal/validate"
)

// cloudPDPHost is the host of the cloud PDP, which serves the decisions of the PDP and none of the
// routes of facts that the PDP container serves under /facts.
const cloudPDPHost = "cloudpdp.api.permit.io"

// The headers of a facts call that waits for sync, which the PDP container reads.
const (
	headerWaitTimeout   = "X-Wait-Timeout"
	headerTimeoutPolicy = "X-Timeout-Policy"
)

// WithFactsViaPDP sends the calls of the facts API, those that write and read users, tenants and
// the users in them, role assignments, resource instances and relationship tuples, one at a time
// or in bulk, to the PDP container at the PDP URL, [WithPDPURL], instead of to the Permit API. The
// container serves them under /facts, sends each on to the API, and can hold its answer to a
// write until the write reaches its policy, as the section Facts and wait-for-sync of the
// package documentation says. Every other call goes where it goes without the option: those of
// set rules, user invites, proxy configs, the schema, projects and environments to the API.
//
// A call through the PDP container acts on the environment of the container's own API key: it
// fills in no project and no environment, and sends no request for the scope of the client's API
// key. Only the PDP container serves these routes: the cloud PDP answers them with 404 and an
// empty body, which a call returns as an error that matches [ErrContainerPDPOnly]. [New] logs a
// warning when the option is given with the URL of the cloud PDP, https://cloudpdp.api.permit.io,
// as [WithLogger] describes.
func WithFactsViaPDP() Option {
	return Option{name: "WithFactsViaPDP", apply: func(s *settings) error {
		s.factsViaPDP = true
		return nil
	}}
}

// isCloudPDP reports whether pdpURL, a URL that baseURL normalized, names the cloud PDP: its host
// is cloudPDPHost in any case, with or without a dot at its end.
func isCloudPDP(pdpURL string) bool {
	u, err := url.Parse(pdpURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), cloudPDPHost)
}

// SyncPolicy is what the PDP container does when the write of a call that waits for sync does
// not reach its policy within the timeout of the wait. [WithSyncPolicy] sends it as the header
// X-Timeout-Policy.
type SyncPolicy string

const (
	// SyncPolicyFail makes the container answer with an error.
	SyncPolicyFail SyncPolicy = "fail"

	// SyncPolicyIgnore makes the container answer with the result of the write.
	SyncPolicyIgnore SyncPolicy = "ignore"
)

// SyncOption is an option of a wait for sync, which the WaitForSync method of a facts service
// takes, such as [TenantsService.WaitForSync]. Build one with [WithSyncPolicy]. The zero
// SyncOption is not valid: each call through the copy of the service that gets it, in any place
// among its options, returns an error that matches [ErrInvalidConfig] and sends no request.
type SyncOption struct {
	// policy is the policy that the option gives the wait.
	policy SyncPolicy
}

// WithSyncPolicy makes the calls that wait for sync send policy as the header X-Timeout-Policy,
// so that the PDP container does what policy says when a write does not reach its policy within
// the timeout of the wait. Without the option, a call sends no X-Timeout-Policy, and the
// container decides. A policy other than [SyncPolicyFail] and [SyncPolicyIgnore] is not valid:
// each call through the copy of the service that gets it, in any place among its options,
// returns an error that matches [ErrInvalidConfig] and sends no request. When a wait gets the
// option more than once, each with a valid policy, the last one wins.
func WithSyncPolicy(policy SyncPolicy) SyncOption {
	return SyncOption{policy: policy}
}

// syncWait is the wait for sync of a facts call: how long the PDP container may hold its answer
// until the call's write reaches its policy, and what it does when the time runs out.
type syncWait struct {
	// timeout is the longest that the container waits. Zero is a wait of zero, which the
	// container gets as X-Wait-Timeout: 0, not a wait left out.
	timeout time.Duration

	// policy is what the container does when the timeout passes, or empty to send no
	// X-Timeout-Policy, which leaves it to the container.
	policy SyncPolicy

	// policyInvalid says that a SyncOption gave a policy other than SyncPolicyFail and
	// SyncPolicyIgnore, as the zero SyncOption does, which checkWait rejects.
	policyInvalid bool
}

// newSyncWait returns the wait of timeout and opts, the arguments of the WaitForSync method of a
// facts service: the policy of the last of opts, when there is one. It checks each of opts, and
// records whether any of them gave a policy that is not valid, for checkWait to reject.
func newSyncWait(timeout time.Duration, opts []SyncOption) syncWait {
	w := syncWait{timeout: timeout}
	for _, o := range opts {
		if o.policy != SyncPolicyFail && o.policy != SyncPolicyIgnore {
			w.policyInvalid = true
		}
		w.policy = o.policy
	}
	return w
}

// header returns the headers that ask the PDP container for w: X-Wait-Timeout, the timeout in
// decimal seconds, and X-Timeout-Policy when w has a policy. It returns a new map on each call,
// so that no two requests share one.
func (w syncWait) header() http.Header {
	h := http.Header{}
	h.Set(headerWaitTimeout, waitSeconds(w.timeout))
	if w.policy != "" {
		h.Set(headerTimeoutPolicy, string(w.policy))
	}
	return h
}

// waitSeconds returns d, which is not negative, as a decimal number of seconds with every digit
// of its nanoseconds and without trailing zeros: 0, 0.5, 1.5 or 10.
func waitSeconds(d time.Duration) string {
	seconds := strconv.FormatInt(int64(d/time.Second), 10)
	fraction := d % time.Second
	if fraction == 0 {
		return seconds
	}
	return seconds + "." + strings.TrimRight(fmt.Sprintf("%09d", fraction), "0")
}

// facts makes the calls of the facts API for a facts service. A facts does not change once built:
// withWait returns a copy, so that the wait of one copy never reaches the calls of another, which
// other goroutines may make at the same time.
type facts struct {
	client *Client

	// wait is the wait for sync of the calls, or nil when they do not wait.
	wait *syncWait
}

// withWait returns a copy of f whose calls wait for sync as w says.
func (f facts) withWait(w syncWait) facts {
	f.wait = &w
	return f
}

// prepare returns r, a call of a Facts operation, ready to send: as direct returns it, and, for a
// call to the API, with the project and the environment of the client's scope before the params
// of r, which the client's inScope puts there, and whose errors it returns. The params of r fill
// the placeholders of the operation's route after {proj_id} and {env_id}.
func (f facts) prepare(ctx context.Context, r request) (request, error) {
	r, err := f.direct(r)
	if err != nil || r.target == pdpTarget {
		return r, err
	}
	return f.client.inScope(ctx, r)
}

// direct returns r, a call of a Facts operation, with the server that it goes to and the headers
// of f's wait, if f has one. With [WithFactsViaPDP], the call goes to the PDP container under
// /facts, which needs no scope; else it goes to the API, under the project and the environment
// of the client's scope, which prepare, or allInScope for the pages of a list, puts in.
//
// Before it sends anything, it returns ErrClosed after Close. Then it returns an error for an
// operation that the client's registry does not hold or that is not a Facts operation; and an
// error that matches ErrInvalidConfig and names the operation for a wait of a client without
// WithFactsViaPDP, as only the PDP container waits; for a wait of an operation that the
// container does not wait on, one that is not WaitsForSync; for a wait whose timeout is less than
// zero; and for a wait whose SyncOptions gave a policy that is not valid.
func (f facts) direct(r request) (request, error) {
	c := f.client
	if err := c.checkOpen(); err != nil {
		return request{}, err
	}
	op, ok := c.registry.Op(r.op)
	if !ok {
		return request{}, fmt.Errorf("permit: the operation %s is not in the client's "+
			"operation registry", r.op)
	}
	if !op.Facts {
		return request{}, fmt.Errorf("permit: %s is not an operation of the facts API; send it "+
			"to its own server", op.ID)
	}
	if f.wait != nil {
		if err := checkWait(op.ID, *f.wait, c.factsViaPDP, op.WaitsForSync); err != nil {
			return request{}, err
		}
		r.header = f.wait.header()
	}
	r.target = apiTarget
	if c.factsViaPDP {
		r.target = pdpTarget
	}
	return r, nil
}

// factsAll returns the iterator of the All method of a facts service whose calls f makes, over
// the list that r requests, whose replies have shape, from the page of opts on. Each loop gets r
// ready with facts.direct before its first page, and yields its error, such as ErrClosed after
// Close or that of a wait that the call cannot make, once, with the zero T, and stops; else it
// yields the items of the pages of all for the PDP container, which needs no scope, or of
// allInScope for the API, which gets the scope before the first page. Nothing is sent until a
// loop asks for an item.
func factsAll[T any](ctx context.Context, f facts, r request, shape listShape,
	opts ListOptions,
) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		direct, err := f.direct(r)
		if err != nil {
			var zero T
			yield(zero, err)
			return
		}
		pages := allInScope[T]
		if direct.target == pdpTarget {
			pages = all[T]
		}
		for item, pageErr := range pages(ctx, f.client, direct, shape, opts) {
			if !yield(item, pageErr) {
				return
			}
		}
	}
}

// checkWait returns the error of w, the wait for sync of a call of the operation id, when the
// call cannot wait: when the client does not send facts to the PDP container, viaPDP, or the
// container does not wait on the operation, waits, or the timeout is less than zero, or a
// SyncOption of the wait gave a policy other than SyncPolicyFail and SyncPolicyIgnore.
func checkWait(id string, w syncWait, viaPDP, waits bool) error {
	if !viaPDP {
		return &configError{setting: id, rule: validate.WaitForSyncFactsViaAPI}
	}
	if !waits {
		return &configError{setting: id, rule: validate.WaitForSyncNotWaiting}
	}
	if w.timeout < 0 {
		return &configError{setting: id, rule: validate.WaitForSyncTimeoutNegative}
	}
	if w.policyInvalid {
		return &configError{setting: id, rule: validate.WaitForSyncPolicyUnknown}
	}
	return nil
}
