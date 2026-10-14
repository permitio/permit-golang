package permit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/permitio/permit-golang/v2/internal/logging"
	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testhook"
	"github.com/permitio/permit-golang/v2/internal/transport"
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// target is the server that a call's request goes to.
type target int

const (
	// The zero target is no server, so that a request that names none fails.
	_ target = iota

	// apiTarget is the Permit API, at the client's API URL.
	apiTarget

	// pdpTarget is the PDP, at the client's PDP URL.
	pdpTarget
)

// request is the request of one call of an operation, as a method of the client gives it.
type request struct {
	// op is the ID of the operation in the client's registry, such as get_user.
	op string

	// target is the server that the request goes to.
	target target

	// params fill the placeholders of the operation's route, in order.
	params []string

	// query is the request's query, or nil when it has none.
	query *wire.Query

	// body is the request's body, which wire.Encode writes, or nil when it has none.
	body any

	// header holds the headers of this call alone, such as its wait for sync, or is nil. No
	// other call shares it.
	header http.Header
}

// listShape is the shape of the body of a reply that holds a list, as the operation's route
// answers it.
type listShape int

const (
	// The zero listShape is no shape, which no body fits.
	_ listShape = iota

	// arrayShape is a JSON array of the items.
	arrayShape

	// pageShape is the paginated object that holds the items in its data array.
	pageShape

	// arrayOrPageShape is either, as some routes of the API answer.
	arrayOrPageShape
)

// fits reports whether a body whose first byte that is not white space is first has the shape.
func (s listShape) fits(first byte) bool {
	switch s {
	case arrayShape:
		return first == '['
	case pageShape:
		return first == '{'
	case arrayOrPageShape:
		return first == '[' || first == '{'
	default:
		return false
	}
}

// doNoValue makes a call whose reply holds no value that the method returns, such as a delete,
// and reads no body from a reply in 200-299. A 404 to an operation whose path names the object
// it deletes (rest.Op.DeleteSelf) means that the object is gone already, and doNoValue returns
// nil for it. It returns the errors that exchange returns.
func (c *Client) doNoValue(ctx context.Context, r request) error {
	_, _, err := c.exchange(ctx, r, false)
	return err
}

// do makes a call whose reply holds an object, a JSON object that it decodes into a new T, and
// returns it. A reply in 200-299 whose body is empty, null or not an object, or does not decode
// into a T, is a *Error that names the model, never a nil T without an error. It returns the
// errors that exchange returns.
func do[T any](ctx context.Context, c *Client, r request) (*T, error) {
	op, reply, err := c.exchange(ctx, r, true)
	if err != nil {
		return nil, err
	}
	value := new(T)
	if firstByte(reply.Body) != '{' || wire.Decode(reply.Body, value) != nil {
		return nil, opCall(op).decodeError(reply.Status, modelName(op, "JSON object"))
	}
	return value, nil
}

// doList makes a call whose reply holds a list in shape, and returns the list's items and the
// counts of a paginated reply. A reply in 200-299 whose body is empty or has another shape, such
// as null or an object where an array was expected, or does not decode into items of T, is a
// *Error that names the model, never an empty list. It returns the errors that exchange returns.
func doList[T any](ctx context.Context, c *Client, r request, shape listShape) (wire.List[T],
	error,
) {
	op, reply, err := c.exchange(ctx, r, true)
	if err != nil {
		return wire.List[T]{}, err
	}
	list, err := wire.DecodeList[T](reply.Body)
	if err != nil || !shape.fits(firstByte(reply.Body)) {
		return wire.List[T]{}, opCall(op).decodeError(reply.Status,
			"list of "+modelName(op, "items"))
	}
	return list, nil
}

// exchange sends the request of a call, and again as the client's retry policy allows, and
// returns its operation and its reply in 200-299, whose body it reads when readBody is set. A
// Facts operation that goes to the PDP goes to its route under /facts: the operation that
// rest.Op.ViaPDP gives, which exchange returns and its errors name. Before it sends anything, it
// returns ErrClosed after Close, and an error that matches ErrInvalidConfig for a path parameter
// that is empty, . or .., naming its placeholder, and for a body that cannot be JSON. It bounds
// the call, every attempt and every wait between them, by the client's timeout when ctx has no
// deadline, and writes one Debug line for each attempt.
//
// The reply's credentials, the values that redact.Set.AddFields finds in a body it reads under a
// credential name or one of the operation's SensitiveBodyFields, join the client's Set, unless
// the operation is NoReplyCredentials, as a decision of the PDP is. exchange returns a *Error for
// a reply whose status is not in 200-299 and for a request without a complete reply, with these
// exceptions: an Error that wraps ErrContainerPDPOnly, not ErrNotFound, for a 404 with an empty
// body to a ContainerOnly operation, which the cloud PDP does not serve, as a Facts operation
// sent to the PDP is; and otherwise nil for a 404 to a DeleteSelf operation when readBody is not
// set, as the object is gone already. A call that fails returns the error of its last attempt,
// or, when ctx ends while it waits to send the request again, a *Error that wraps the error of
// ctx.
func (c *Client) exchange(ctx context.Context, r request, readBody bool) (rest.Op,
	transport.Reply, error,
) {
	if err := c.checkOpen(); err != nil {
		return rest.Op{}, transport.Reply{}, err
	}
	op, ok := c.registry.Op(r.op)
	if !ok {
		return rest.Op{}, transport.Reply{}, fmt.Errorf("permit: the operation %s is not in "+
			"the client's operation registry", r.op)
	}
	if r.target == pdpTarget {
		op = op.ViaPDP()
	}
	attempt, err := c.attempt(op, r, readBody)
	if err != nil {
		return op, transport.Reply{}, err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	reply, err := c.sendWithRetries(ctx, op, attempt, readBody)
	if err != nil {
		return op, reply, err
	}
	if readBody && !op.NoReplyCredentials {
		c.secrets.AddFields(reply.Body, op.SensitiveBodyFields...)
	}
	return op, reply, nil
}

// sendWithRetries sends r, the request of a call of op, and sends it again after each attempt
// that failed for as long as the client's retry policy allows, waiting before each retry as the
// policy says. It writes one Debug line for each attempt, and returns the reply of the last
// attempt and its error, as attemptError returns it; or a *Error that wraps the error of ctx when
// ctx ends while it waits.
func (c *Client) sendWithRetries(ctx context.Context, op rest.Op, r transport.Request,
	readBody bool,
) (transport.Reply, error) {
	for attempt := 1; ; attempt++ {
		reply, sendErr := c.sender.Send(ctx, r)
		err := c.attemptError(op, reply, sendErr, readBody)
		c.logger.Request(ctx, logging.Request{
			Route:    op.Route,
			Status:   reply.Status,
			Attempt:  attempt,
			Duration: reply.Duration,
			Err:      err,
		})
		if err == nil {
			return reply, nil
		}
		wait, again := c.retry.Next(ctx, op, attempt, reply, sendErr)
		if !again {
			return reply, err
		}
		if waitErr := transport.Sleep(ctx, wait); waitErr != nil {
			return reply, opCall(op).transportError(waitErr, c.secrets)
		}
		// A client closed while the call waited sends no other request.
		if c.checkOpen() != nil {
			return reply, err
		}
	}
}

// attempt returns the attempt of r, a call of op, or the error of a request that the client
// cannot send.
func (c *Client) attempt(op rest.Op, r request, readBody bool) (transport.Request, error) {
	path, err := wire.Path(op.Route, r.params...)
	if pathErr, ok := errors.AsType[*wire.PathError](err); ok {
		return transport.Request{}, &configError{
			setting: op.ID + ": {" + pathErr.Placeholder + "} of " + op.Route,
			rule:    pathErr.Rule,
		}
	}
	if err != nil {
		return transport.Request{}, fmt.Errorf("permit: %s: %w", op.ID, err)
	}
	var body []byte
	if r.body != nil {
		if body, err = wire.Encode(r.body); err != nil {
			return transport.Request{}, fmt.Errorf("%w: %s: the request body cannot be "+
				"written as JSON: %w", ErrInvalidConfig, op.ID, err)
		}
	}
	var query string
	if r.query != nil {
		query = r.query.Encode()
	}
	attempt := transport.Request{
		Method:   op.Method,
		Path:     path,
		Query:    query,
		Template: op.Route,
		Body:     body,
		Header:   r.header,
		ReadBody: readBody,
	}
	switch r.target {
	case apiTarget:
		attempt.BaseURL, attempt.Target = c.apiURL, testhook.API
	case pdpTarget:
		attempt.BaseURL, attempt.Target = c.pdpURL, testhook.PDP
	default:
		return transport.Request{}, fmt.Errorf("permit: %s: the call names no server to send "+
			"its request to", op.ID)
	}
	return attempt, nil
}

// attemptError returns the error of an attempt of op that got reply, or got no complete reply
// when sendErr is not nil, as exchange describes, or nil.
func (c *Client) attemptError(op rest.Op, reply transport.Reply, sendErr error,
	readBody bool,
) error {
	failed := opCall(op)
	// The checks are if statements, not the cases of a switch, so that coverage, and with it
	// the mutation tests, reach each condition.
	if sendErr != nil {
		e := failed.transportError(sendErr, c.secrets)
		e.StatusCode = reply.Status
		return e
	}
	if isSuccess(reply.Status) {
		return nil
	}
	containerOnly := reply.Status == http.StatusNotFound && op.ContainerOnly &&
		len(bytes.TrimSpace(reply.Body)) == 0
	if reply.Status == http.StatusNotFound && op.DeleteSelf && !readBody && !containerOnly {
		return nil
	}
	e := failed.replyError(reply.Status, reply.Body, c.secrets, op.SensitiveBodyFields)
	if containerOnly {
		e.Err = ErrContainerPDPOnly
	}
	return e
}

// opCall returns the call of op, for the errors of its calls.
func opCall(op rest.Op) call {
	return call{op: op.ID, method: op.Method, route: op.Route}
}

// modelName returns the name of the model that the replies of op hold, such as models.UserRead,
// for the error of a reply that is not one, or unnamed when op names no model.
func modelName(op rest.Op, unnamed string) string {
	if op.ResponseModel == "" {
		return unnamed
	}
	return "models." + op.ResponseModel
}

// firstByte returns the first byte of body that is not JSON white space, or 0 when there is none.
func firstByte(body []byte) byte {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) == 0 {
		return 0
	}
	return trimmed[0]
}
