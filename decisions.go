package permit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/transport"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// The IDs of the operations of the PDP's decisions: one check, and many checks in one request.
const (
	opCheck     = "is_allowed_allowed_post"
	opBulkCheck = "is_allowed_bulk_allowed_bulk_post"
)

// defaultTenant is the tenant that a check sends for a resource without one: the key of the
// default tenant of an environment, which the other SDKs of Permit send too.
const defaultTenant = "default"

// User is the user that a check, or [Client.UserPermissions], asks about, as the PDP gets it.
// Build it as a struct literal, such as permit.User{Key: "alice"}. The PDP decides for the user
// by its key, and passes the rest to the policy.
type User struct {
	// Key is the key of the user, as Permit knows the user. It must not be empty: a call for a
	// user without a key returns an error that matches [ErrInvalidConfig] and sends no request.
	Key string `json:"key"`

	// FirstName is the first name of the user, or empty to send none. It is sent as first_name,
	// which the PDP container and the cloud PDP both read.
	FirstName string `json:"first_name,omitzero"`

	// LastName is the last name of the user, or empty to send none. It is sent as last_name, which
	// the PDP container and the cloud PDP both read.
	LastName string `json:"last_name,omitzero"`

	// Email is the email address of the user, or empty to send none.
	Email string `json:"email,omitzero"`

	// Attributes are the attributes of the user for the check, such as for the conditions of an
	// attribute-based policy: any JSON values, such as numbers, booleans, null and nested objects
	// and arrays, which reach the PDP as they are. Nil or empty sends none.
	Attributes map[string]any `json:"attributes,omitzero"`
}

// Resource is the resource that a check, or [Client.AuthorizedUsers], asks about, as the PDP gets
// it: a resource type, and for a check of one instance of the type, the instance's key. Build it
// as a struct literal, such as permit.Resource{Type: "document", Key: "2026-report", Tenant:
// "tenant-a"}.
type Resource struct {
	// Type is the key of the resource type, such as document. It must not be empty: a call for a
	// resource without a type returns an error that matches [ErrInvalidConfig] and sends no
	// request.
	Type string `json:"type"`

	// Key is the key of the resource instance, for a check of that instance, which the roles
	// and the relationships of the instance can decide. Empty sends none, and the check is one
	// of the resource type.
	Key string `json:"key,omitzero"`

	// Tenant is the key of the tenant of the resource. Empty sends default, the key of the
	// default tenant of an environment, as the other SDKs of Permit do, so that a check always
	// names a tenant. [Client.FilterObjects] returns a resource without a tenant as it was given.
	Tenant string `json:"tenant,omitzero"`

	// Attributes are the attributes of the resource for the check: any JSON values, which reach
	// the PDP as they are. Nil or empty sends none.
	Attributes map[string]any `json:"attributes,omitzero"`

	// Context is the context of the resource for the check: any JSON values, which reach the PDP
	// as they are. Nil or empty sends none. The PDP container reads it; the spec of the cloud PDP
	// has no context in a resource, so give the context that the cloud PDP reads with
	// [WithDecisionContext] or [CheckRequest.Context].
	Context map[string]any `json:"context,omitzero"`
}

// CheckRequest is one check of [Client.BulkCheck]: whether User may do Action on Resource, with
// the Context of the check. Build it as a struct literal.
type CheckRequest struct {
	// User is the user that the check asks about.
	User User `json:"user"`

	// Action is the key of the action, such as read.
	Action string `json:"action"`

	// Resource is the resource that the check asks about.
	Resource Resource `json:"resource"`

	// Context is the context of the check, which the PDP passes to the policy: any JSON values,
	// such as numbers, booleans, null and nested objects and arrays, which reach the PDP as they
	// are, as [WithDecisionContext] gives it to [Client.Check]. Nil or empty sends none.
	Context map[string]any `json:"context,omitzero"`
}

// sent returns r as the body of a request holds it: the tenant default for a resource without a
// tenant, and each of its maps of attributes or of context that is empty left out, as a nil one
// is, rather than sent as {}. No PDP takes null for one.
func (r CheckRequest) sent() CheckRequest {
	r.User.Attributes = nonEmpty(r.User.Attributes)
	r.Resource = r.Resource.sent()
	r.Context = nonEmpty(r.Context)
	return r
}

// sent returns r as the body of a request holds it: with the tenant default when it has no
// tenant, and with each of its maps of attributes or of context that is empty left out.
func (r Resource) sent() Resource {
	if r.Tenant == "" {
		r.Tenant = defaultTenant
	}
	r.Attributes = nonEmpty(r.Attributes)
	r.Context = nonEmpty(r.Context)
	return r
}

// nonEmpty returns m, or nil when m is empty.
func nonEmpty(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	return m
}

// CheckOption changes a call that asks the PDP for decisions: [Client.Check],
// [Client.FilterObjects] and [Client.AuthorizedUsers], and, as a [PermissionsOption],
// [Client.UserPermissions]. Build one with [WithDecisionContext]. The zero CheckOption is not
// valid: a call that gets one returns an error that matches [ErrInvalidConfig] and sends no
// request.
type CheckOption struct {
	// apply sets what the option changes in s.
	apply func(s *checkSettings)
}

// checkSettings holds what the options of a call that makes checks set.
type checkSettings struct {
	// context is the context of each check of the call, or nil.
	context map[string]any
}

// WithDecisionContext gives the checks of a call the context decisionContext, which the PDP
// passes to the policy beside the user, the action and the resource, such as the time of the
// request: a map of any JSON values, such as numbers, booleans, null and nested objects and
// arrays, which reach the PDP as they are. It gives [Client.AuthorizedUsers] and
// [Client.UserPermissions] their context too. Nil or an empty map sends none. When a call gets
// the option more than once, the last one wins.
func WithDecisionContext(decisionContext map[string]any) CheckOption {
	return CheckOption{apply: func(s *checkSettings) { s.context = decisionContext }}
}

// settingsOf returns the settings that opts, the options of a call of method, give, or the error
// of the first that is the zero CheckOption, which names method and the option's position.
func settingsOf(method string, opts []CheckOption) (checkSettings, error) {
	var s checkSettings
	for i, opt := range opts {
		if opt.apply == nil {
			return checkSettings{}, &configError{
				setting: method + ": option " + strconv.Itoa(i+1),
				rule:    validate.DecisionOptionZero,
			}
		}
		opt.apply(&s)
	}
	return s, nil
}

// userError returns the error of user, the argument that setting names, when it has no key, as no
// PDP decides for a user without one; or nil.
func userError(setting string, user User) error {
	if user.Key == "" {
		return &configError{setting: setting, rule: validate.DecisionUserKeyEmpty}
	}
	return nil
}

// resourceError returns the error of resource, the argument that setting names, when it has no
// type, as no PDP decides on a resource without one; or nil.
func resourceError(setting string, resource Resource) error {
	if resource.Type == "" {
		return &configError{setting: setting, rule: validate.DecisionResourceTypeEmpty}
	}
	return nil
}

// Check asks the PDP whether user may do action on resource, such as whether the user alice may
// read the document 2026-report, and returns true when the PDP allows it and false when it denies
// it. A resource with a Key is checked as that instance of its type, and one without as its type;
// a resource without a Tenant is checked in the tenant default. With [WithDecisionContext], the
// check carries a context that the policy reads.
//
// Check sends one request, POST /allowed, to the PDP at the PDP URL, [WithPDPURL]: the PDP
// container and the cloud PDP both serve it. It needs an environment-level API key, the key of
// the environment whose policy the PDP decides on. The PDP's decisions change nothing, so a call
// sends its request again as [WithRetry] describes: after a 429, and after a 500, 502, 503 or 504
// reply or a failed connection too.
//
// Check returns false only for a decision that denies; it returns every failure as an error:
//
//   - a user without a key, a resource without a type, or the zero CheckOption: an error that
//     matches [ErrInvalidConfig], and no request;
//   - a reply with an error status, such as 400, 421 or 422, or 429 after the retries: an
//     [Error] with the status and the body of the reply;
//   - a reply in 200-299 that is not a decision, such as {}, {"allow": null},
//     {"allow": "true"} or {"Allow": true}, which is not a JSON object with one key allow, spelled
//     so, whose value is true or false: an Error that matches [ErrMalformedDecision];
//   - a request that got no reply, such as one whose connection was refused: an Error whose Err
//     wraps the cause, such as a *url.Error.
func (c *Client) Check(ctx context.Context, user User, action string, resource Resource,
	opts ...CheckOption,
) (bool, error) {
	s, err := settingsOf("Check", opts)
	if err != nil {
		return false, err
	}
	if err = userError("Check: user", user); err != nil {
		return false, err
	}
	if err = resourceError("Check: resource", resource); err != nil {
		return false, err
	}
	check := CheckRequest{User: user, Action: action, Resource: resource, Context: s.context}
	op, reply, err := c.exchange(ctx, request{
		op: opCheck, target: pdpTarget, body: check.sent(),
	}, true)
	if err != nil {
		return false, err
	}
	return readDecision(op, reply)
}

// BulkCheck asks the PDP for the decision of each of checks, and returns the decisions in the
// order of checks: true where the PDP allows the check, and false where it denies it. Each check
// has its own user, action, resource and context, and the resources may be of different tenants.
//
// BulkCheck sends one request for all the checks, POST /allowed/bulk, to the PDP at the PDP URL,
// [WithPDPURL], and sends none for no checks: the PDP container and the cloud PDP both serve it.
// It needs an environment-level API key. It sends its request again as [Client.Check] does, and
// returns the errors that Check returns, naming the check, such as checks[2].user, of a user
// without a key or a resource without a type. A reply that does not hold a decision, true or
// false, for each check, in a JSON object whose allow is a list, each under one key allow as
// Check reads it, is an [Error] that matches [ErrMalformedDecision].
func (c *Client) BulkCheck(ctx context.Context, checks []CheckRequest) ([]bool, error) {
	for i, check := range checks {
		at := "BulkCheck: checks[" + strconv.Itoa(i) + "]."
		if err := userError(at+"user", check.User); err != nil {
			return nil, err
		}
		if err := resourceError(at+"resource", check.Resource); err != nil {
			return nil, err
		}
	}
	return c.decide(ctx, checks)
}

// FilterObjects returns the resources of resources on which the PDP allows user to do action, in
// their order, each as it was given. It checks each resource as [Client.Check] does, with its
// key, tenant, attributes and context, so that the instances of a type each get their own
// decision. With [WithDecisionContext], each check carries the context.
//
// FilterObjects sends one request for all the resources, POST /allowed/bulk, to the PDP at the
// PDP URL, [WithPDPURL], and sends none for no resources: the PDP container and the cloud PDP both
// serve it. It needs an environment-level API key. It returns the errors that [Client.BulkCheck]
// returns, naming the resource, such as resources[2], that has no type.
func (c *Client) FilterObjects(ctx context.Context, user User, action string,
	resources []Resource, opts ...CheckOption,
) ([]Resource, error) {
	s, err := settingsOf("FilterObjects", opts)
	if err != nil {
		return nil, err
	}
	if err = userError("FilterObjects: user", user); err != nil {
		return nil, err
	}
	checks := make([]CheckRequest, len(resources))
	for i, resource := range resources {
		at := "FilterObjects: resources[" + strconv.Itoa(i) + "]"
		if err = resourceError(at, resource); err != nil {
			return nil, err
		}
		checks[i] = CheckRequest{User: user, Action: action, Resource: resource, Context: s.context}
	}
	allowed, err := c.decide(ctx, checks)
	if err != nil {
		return nil, err
	}
	kept := make([]Resource, 0, len(resources))
	for i, resource := range resources {
		if allowed[i] {
			kept = append(kept, resource)
		}
	}
	return kept, nil
}

// decide sends checks to the PDP in one request, POST /allowed/bulk, and returns the decision of
// each, in order. For no checks, it sends nothing and returns an empty list, or ErrClosed after
// Close, as every call that starts after Close does.
func (c *Client) decide(ctx context.Context, checks []CheckRequest) ([]bool, error) {
	if len(checks) == 0 {
		if err := c.checkOpen(); err != nil {
			return nil, err
		}
		return []bool{}, nil
	}
	body := make([]CheckRequest, len(checks))
	for i, check := range checks {
		body[i] = check.sent()
	}
	op, reply, err := c.exchange(ctx, request{op: opBulkCheck, target: pdpTarget, body: body}, true)
	if err != nil {
		return nil, err
	}
	return readDecisions(op, reply, len(checks))
}

// allowKey is the key of the decision in a reply of the PDP, and in each item of a bulk reply.
const allowKey = "allow"

// allowOf returns the value of the one key allow of data, a JSON object, as fieldOf finds it, and
// false when data is not one JSON object with exactly one key allow.
func allowOf(data []byte) (json.RawMessage, bool) {
	members, ok := membersOf(data)
	if !ok {
		return nil, false
	}
	return fieldOf(members, allowKey)
}

// member is a member of a JSON object of a reply: its key, after its escapes are read, as JSON
// defines it, and its value as the reply spells it.
type member struct {
	key   string
	value json.RawMessage
}

// membersOf returns the members of data, one JSON object, in their order, a key that is there
// twice too, and false when data is not one JSON object.
func membersOf(data []byte) ([]member, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if open, err := dec.Token(); err != nil || open != json.Delim('{') {
		return nil, false
	}
	var members []member
	for dec.More() {
		token, err := dec.Token()
		var value json.RawMessage
		if err != nil || dec.Decode(&value) != nil {
			return nil, false
		}
		// The token of a key of an object that Token returns without an error is a string.
		key, _ := token.(string)
		members = append(members, member{key: key, value: value})
	}
	// More is false, so the next token is the } that ends the object, or an error.
	if _, err := dec.Token(); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return members, true
}

// fieldOf returns the value of the one member of members whose key is name, spelled so, and false
// when no member or more than one has the key name. A key that differs from name only in case,
// such as Allow for allow, which encoding/json would read as name, and a second key name, of which
// encoding/json would keep the last, make the object ambiguous, so fieldOf returns false for both.
func fieldOf(members []member, name string) (json.RawMessage, bool) {
	var value json.RawMessage
	found := 0
	for _, m := range members {
		switch {
		case m.key == name:
			value, found = m.value, found+1
		case strings.EqualFold(m.key, name):
			return nil, false
		}
	}
	return value, found == 1
}

// boolOf returns the boolean that value, a JSON value as a reply spells it, is, and false when
// value is not the literal true or false.
func boolOf(value json.RawMessage) (allowed, ok bool) {
	switch string(value) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

// readDecision returns the decision that reply, the reply in 200-299 to a check of op, holds: the
// value of its one key allow, as allowOf reads it, which must be true or false. Any other reply
// is an Error that matches ErrMalformedDecision. Fields other than allow are ignored.
func readDecision(op rest.Op, reply transport.Reply) (bool, error) {
	allow, ok := allowOf(reply.Body)
	allowed, isBool := boolOf(allow)
	if !ok || !isBool {
		return false, opCall(op).malformedDecision(reply.Status, "the reply is not a JSON "+
			"object with one key allow whose value is true or false")
	}
	return allowed, nil
}

// readDecisions returns the decisions that reply, the reply in 200-299 to n checks of op, holds,
// in order: a JSON object whose one key allow, as allowOf reads it, is a list with a decision for
// each check, each a JSON object whose one key allow is true or false. Any other reply is an
// Error that matches ErrMalformedDecision; an allow that is null holds no decisions. Fields other
// than allow are ignored.
func readDecisions(op rest.Op, reply transport.Reply, n int) ([]bool, error) {
	failed := opCall(op)
	allow, ok := allowOf(reply.Body)
	var items []json.RawMessage
	if !ok || wire.Decode(allow, &items) != nil {
		return nil, failed.malformedDecision(reply.Status, "the reply is not a JSON object "+
			"with one key allow whose value is a list of decisions, each a JSON object with one "+
			"key allow whose value is true or false")
	}
	if len(items) != n {
		return nil, failed.malformedDecision(reply.Status, "the number of decisions in the "+
			"reply, "+strconv.Itoa(len(items))+", is not the number of checks, "+strconv.Itoa(n))
	}
	allowed := make([]bool, n)
	for i, item := range items {
		value, ok := allowOf(item)
		decision, isBool := boolOf(value)
		if !ok || !isBool {
			return nil, failed.malformedDecision(reply.Status, "decision "+strconv.Itoa(i+1)+
				" of the reply is not a JSON object with one key allow whose value is true or "+
				"false")
		}
		allowed[i] = decision
	}
	return allowed, nil
}
