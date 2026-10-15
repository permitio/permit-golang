package permit

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// The sentinel errors. Check for one with [errors.Is], which also finds it in an error that wraps
// it. An [Error] matches the sentinel of its status, as [Error.Is] says.
var (
	// ErrNotFound means that the object does not exist. An Error with status 404 matches it,
	// unless it wraps [ErrContainerPDPOnly]: a route that only the PDP container serves is not a
	// missing object.
	ErrNotFound = errors.New("permit: not found")

	// ErrConflict means that the object exists already, or that the request conflicts with
	// another object. An Error with status 409 matches it.
	ErrConflict = errors.New("permit: conflict")

	// ErrUnauthorized means that the API or the PDP did not accept the API key. An Error with
	// status 401 matches it.
	ErrUnauthorized = errors.New("permit: unauthorized")

	// ErrForbidden means that the API key may not do what the request asks. An Error with status
	// 403 matches it.
	ErrForbidden = errors.New("permit: forbidden")

	// ErrRateLimited means that the API or the PDP got too many requests and refused this one. An
	// Error with status 429 matches it.
	ErrRateLimited = errors.New("permit: rate limited")

	// ErrContainerPDPOnly means that only the PDP container serves the request's route, and the
	// PDP that the client calls, such as the cloud PDP, does not.
	ErrContainerPDPOnly = errors.New("permit: only the PDP container serves this route")

	// ErrInvalidConfig means that an option of the client or an argument of a call is not one the
	// SDK can use. The call sends no request.
	ErrInvalidConfig = errors.New("permit: invalid configuration")

	// ErrMalformedDecision means that the PDP answered a check with a reply that is not a valid
	// decision, or a call for the permissions of a user, for the users that may do an action or
	// for tenants with a reply that is not of the shape the call reads.
	ErrMalformedDecision = errors.New("permit: malformed decision")

	// ErrClosed means that the call started after the client was closed.
	ErrClosed = errors.New("permit: the client is closed")
)

// Error is the error of a call whose request failed: the API or the PDP answered with an error
// status, the SDK could not decode a reply, or the request got no reply. Such a call returns it,
// or an error that wraps it. Check what failed with [errors.Is] and a sentinel such as
// [ErrNotFound], and read the fields with [errors.AsType] and the type *Error, or with
// [errors.As].
//
// When the SDK creates an Error, it replaces each credential it finds in the fields that come
// from the reply and in the text of Err with a marker: the client's API key, the credentials that
// the client's calls returned, Permit API keys, Authorization header values, and the values of
// JSON fields that name a credential, such as secret, token, password or api_key. Then it cuts
// each of Code, Title, Detail and RequestID to at most 1 KiB: a longer one is cut where a
// character starts and ends with "...".
type Error struct {
	// Op is the ID of the operation that failed, as the Permit API's specification names it,
	// such as get_user.
	Op string

	// Method is the HTTP method of the request, such as GET.
	Method string

	// Route is the request's route template, such as /v2/facts/{proj_id}/{env_id}/users/{user_id}:
	// its path before the SDK filled in the parameters, which can be the keys of your objects.
	Route string

	// StatusCode is the HTTP status of the reply, or 0 when the request got no reply.
	StatusCode int

	// Code is the error code of the API's error body, such as NOT_FOUND.
	Code string

	// Title is the title of the API's error body.
	Title string

	// Detail is what went wrong, as the reply says: the message of the API's error body, the
	// detail string of a body such as {"detail": "Not Found"}, or the first 10 fields that failed
	// validation, each with its message, and how many more failed, joined by "; ". It is empty
	// when the body says none of these, such as an HTML page.
	Detail string

	// RequestID is the id field of the API's error body.
	RequestID string

	// Body is the body of a reply whose status is not in 200-299, cut to its first 8 KiB after
	// its credentials were replaced. It is empty for a reply in 200-299 and for a request that got
	// no reply. Error never prints it.
	Body []byte

	// Err is the cause, or nil: the error of a request that got no reply, such as a canceled
	// context or a refused connection, or the reason the SDK could not decode a reply. Check it
	// with errors.Is and errors.As on the Error: its text may differ from the cause's.
	Err error
}

// Error returns the operation, the method and route of the request, the status and code of the
// reply, the reply's detail or else its title, and the cause, each that is set, after "permit:".
// It never returns the body.
func (e *Error) Error() string {
	if e == nil {
		return "permit: <nil>"
	}
	var b strings.Builder
	b.WriteString("permit")
	if e.Op != "" {
		b.WriteString(": " + e.Op)
	}
	if request := strings.TrimSpace(e.Method + " " + e.Route); request != "" {
		b.WriteString(": " + request)
	}
	var status string
	if e.StatusCode != 0 {
		status = "status " + strconv.Itoa(e.StatusCode)
	}
	if reply := strings.TrimSpace(status + " " + e.Code); reply != "" {
		b.WriteString(": " + reply)
	}
	if detail := cmp.Or(e.Detail, e.Title); detail != "" {
		b.WriteString(": " + detail)
	}
	if e.Err != nil {
		b.WriteString(": " + e.Err.Error())
	}
	return b.String()
}

// Unwrap returns Err, so that [errors.Is] and [errors.As] reach the cause.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Is reports whether target is the sentinel of e's status: [ErrNotFound] for 404, unless Err is
// or wraps [ErrContainerPDPOnly], [ErrConflict] for 409, [ErrUnauthorized] for 401,
// [ErrForbidden] for 403 and [ErrRateLimited] for 429. [errors.Is] calls it, and then checks Err.
// Any other status, such as 400, 422 or 500, matches no sentinel: read StatusCode.
func (e *Error) Is(target error) bool {
	if e == nil {
		return false
	}
	switch target {
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound && !errors.Is(e.Err, ErrContainerPDPOnly)
	case ErrConflict:
		return e.StatusCode == http.StatusConflict
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized
	case ErrForbidden:
		return e.StatusCode == http.StatusForbidden
	case ErrRateLimited:
		return e.StatusCode == http.StatusTooManyRequests
	default:
		return false
	}
}

// maxErrorBody is the most bytes of a reply's body that an Error keeps.
const maxErrorBody = 8 << 10

// maxErrorField is the most bytes of each text that an Error reads from a reply's body.
const maxErrorField = 1 << 10

// cutMarker ends a text that an Error cut.
const cutMarker = "..."

// call names the request of a call, for the Error that the call returns.
type call struct {
	// op is the operation's ID in the API's specification.
	op string

	// method is the HTTP method.
	method string

	// route is the route template, never the path with the parameters filled in.
	route string
}

// newError returns an Error for c with status.
func (c call) newError(status int) *Error {
	return &Error{Op: c.op, Method: c.method, Route: c.route, StatusCode: status}
}

// replyError returns the Error for a reply to c with status and body. When status is not in
// 200-299, it reads Code, Title, Detail and RequestID from the body, each cut to 1 KiB, and keeps
// a copy of the body in Body, cut to 8 KiB; it replaces the credentials of secrets, which may be
// nil, and those that redact.Scrub finds, in all of them before it cuts them, and in Body also the
// values of sensitiveFields, the fields of the route's body that hold credentials. A status of 0
// or in 200-299 keeps nothing of the body.
func (c call) replyError(status int, body []byte, secrets *redact.Set,
	sensitiveFields []string,
) *Error {
	e := c.newError(status)
	if status == 0 || isSuccess(status) {
		return e
	}
	if details, ok := wire.DecodeErrorBody(body); ok {
		e.Code = cut(redact.ScrubString(secrets, details.ErrorCode))
		e.Title = cut(redact.ScrubString(secrets, details.Title))
		e.Detail = cut(redact.ScrubString(secrets, details.Summary()))
		e.RequestID = cut(redact.ScrubString(secrets, details.ID))
	}
	scrubbed := redact.Scrub(secrets, body, sensitiveFields...)
	e.Body = bytes.Clone(scrubbed[:min(len(scrubbed), maxErrorBody)])
	return e
}

// cut returns text, or, when text has more than maxErrorField bytes, its start, cut where a
// character starts, and cutMarker, which together have at most maxErrorField bytes.
func cut(text string) string {
	if len(text) <= maxErrorField {
		return text
	}
	end := maxErrorField - len(cutMarker)
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + cutMarker
}

// decodeError returns the Error for a reply to c with status whose body is not a valid model,
// such as models.UserRead. The decoder's error can quote the body, so the Error names the model
// only and keeps nothing of the body.
func (c call) decodeError(status int, model string) *Error {
	e := c.newError(status)
	e.Err = errors.New("the reply is not a valid " + model)
	return e
}

// malformedDecision returns the Error for a reply to c, a decision of the PDP, with status in
// 200-299, that is not a valid decision, as why says. It matches ErrMalformedDecision, and keeps
// nothing of the body, as decodeError does.
func (c call) malformedDecision(status int, why string) *Error {
	e := c.newError(status)
	e.Err = fmt.Errorf("%w: %s", ErrMalformedDecision, why)
	return e
}

// transportError returns the Error for a request of c that got no reply, such as one whose
// connection failed or whose context ended. Err wraps cause, so that [errors.Is] finds
// context.DeadlineExceeded and [errors.As] finds a *url.Error. The text of Err is the text of
// cause, without the URL of a *url.Error, whose path can hold the keys of the caller's objects,
// and with the credentials of secrets, which may be nil, and those that redact.Scrub finds
// replaced.
func (c call) transportError(cause error, secrets *redact.Set) *Error {
	e := c.newError(0)
	if cause == nil {
		return e
	}
	e.Err = &scrubbedError{text: redact.ScrubString(secrets, causeText(cause)), cause: cause}
	return e
}

// isSuccess reports whether status is in 200-299.
func isSuccess(status int) bool {
	return status >= 200 && status <= 299
}

// causeText returns the text of err, or, when err is or wraps a *url.Error, a text without its
// URL: the text of the error that the *url.Error wraps, else its Op, else "the request failed".
func causeText(err error) string {
	urlErr, ok := errors.AsType[*url.Error](err)
	switch {
	case !ok:
		return err.Error()
	case urlErr.Err != nil:
		return urlErr.Err.Error()
	default:
		return cmp.Or(urlErr.Op, "the request failed")
	}
}

// scrubbedError wraps an error with a text that has its credentials replaced.
type scrubbedError struct {
	text  string
	cause error
}

func (e *scrubbedError) Error() string { return e.text }

func (e *scrubbedError) Unwrap() error { return e.cause }
