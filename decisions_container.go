package permit

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/transport"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// The IDs of the operations of the PDP that only the PDP container serves: the tenants in which a
// check is allowed, the tenants of a user, and the check of a URL.
const (
	opAllTenantsCheck = "is_allowed_all_tenants_allowed_all_tenants_post"
	opUserTenants     = "Get_User_Tenants_user_tenants_post"
	opCheckURL        = "is_allowed_url_allowed_url_post"
)

// The keys of the replies of the tenants that the SDK checks: the list of the decisions of a
// reply of AllTenantsCheck, and the key of a tenant.
const (
	allowedTenantsKey = "allowed_tenants"
	tenantKeyKey      = "key"
)

// AllTenantsCheck asks the PDP in which tenants user may do action on resource, such as in which
// tenants the user alice may read documents, and returns those tenants, in the order of the
// PDP's reply, or an empty list when the PDP allows none. It sends the check as [Client.Check]
// does: a resource with a Key is checked as that instance of its type, and one without as its
// type, and a resource without a Tenant is sent with the tenant default. With
// [WithDecisionContext], the check carries a context that the policy reads.
//
// AllTenantsCheck is container PDP only: it sends one request, POST /allowed/all-tenants, to the
// PDP at the PDP URL, [WithPDPURL], which only the PDP container serves. The cloud PDP answers it
// with 404 and an empty body, which the call returns, after that one request, as an [Error] that
// matches [ErrContainerPDPOnly] and names the route. A later version of the PDP may drop the
// route: [Client.UserPermissions] returns the permissions of a user in each tenant, and every PDP
// serves it. AllTenantsCheck needs an environment-level API key. The call changes nothing, so it
// sends its request again as Check does.
//
// AllTenantsCheck returns every failure as an error, and never a list of tenants with it:
//
//   - a user without a key, a resource without a type, or the zero CheckOption: an error that
//     matches [ErrInvalidConfig], and no request;
//   - a reply with an error status, such as 404 or 422, or 429 after the retries: an [Error] with
//     the status and the body of the reply;
//   - a reply in 200-299 that is not a JSON object with one key allowed_tenants, spelled so, whose
//     value is a list of decisions, each a JSON object with one key allow, whose value is true or
//     false, and one key tenant, whose value is a JSON object with one field key whose value is a
//     string, and attributes, when present, a JSON object or null: an Error that matches
//     [ErrMalformedDecision], also when only a decision that denies is malformed;
//   - a request that got no reply, such as one whose connection was refused: an Error whose Err
//     wraps the cause, such as a *url.Error.
func (c *Client) AllTenantsCheck(ctx context.Context, user User, action string, resource Resource,
	opts ...CheckOption,
) ([]TenantDetails, error) {
	s, err := settingsOf("AllTenantsCheck", opts)
	if err != nil {
		return nil, err
	}
	if err = userError("AllTenantsCheck: user", user); err != nil {
		return nil, err
	}
	if err = resourceError("AllTenantsCheck: resource", resource); err != nil {
		return nil, err
	}
	check := CheckRequest{User: user, Action: action, Resource: resource, Context: s.context}
	op, reply, err := c.exchange(ctx, request{
		op: opAllTenantsCheck, target: pdpTarget, body: check.sent(),
	}, true)
	if err != nil {
		return nil, err
	}
	return readAllowedTenants(op, reply)
}

// readAllowedTenants returns the tenants that reply, the reply in 200-299 to a call of
// AllTenantsCheck of op, allows, in order: a JSON object whose one key allowed_tenants, as fieldOf
// reads it, is a list of decisions, each a JSON object with one key allow, whose value is true or
// false, and one key tenant, whose value is a tenant as tenantOf reads it. It keeps the tenant of
// each decision whose allow is true. Any other reply, also one in which only a decision whose
// allow is false is malformed, is an Error that matches ErrMalformedDecision. Other fields are
// ignored.
func readAllowedTenants(op rest.Op, reply transport.Reply) ([]TenantDetails, error) {
	failed := opCall(op)
	fields, ok := membersOf(reply.Body)
	list, hasList := fieldOf(fields, allowedTenantsKey)
	var decisions []json.RawMessage
	if !ok || !hasList || firstByte(list) != '[' || wire.Decode(list, &decisions) != nil {
		return nil, failed.malformedDecision(reply.Status, "the reply is not a JSON object "+
			"with one key allowed_tenants whose value is a list of decisions")
	}
	tenants := make([]TenantDetails, 0, len(decisions))
	for i, decision := range decisions {
		tenant, allowed, ok := tenantDecisionOf(decision)
		if !ok {
			return nil, failed.malformedDecision(reply.Status, "decision "+strconv.Itoa(i+1)+
				" of the reply is not a JSON object with one key allow whose value is true or "+
				"false and one key tenant whose value is a tenant")
		}
		if allowed {
			tenants = append(tenants, tenant)
		}
	}
	return tenants, nil
}

// tenantDecisionOf returns the tenant and the decision that decision, an item of the list of a
// reply of AllTenantsCheck, holds, and false when it is not a JSON object with one key allow, as
// fieldOf reads it, whose value is true or false, and one key tenant whose value is a tenant as
// tenantOf reads it.
func tenantDecisionOf(decision json.RawMessage) (TenantDetails, bool, bool) {
	fields, ok := membersOf(decision)
	if !ok {
		return TenantDetails{}, false, false
	}
	allow, hasAllow := fieldOf(fields, allowKey)
	allowed, isBool := boolOf(allow)
	value, hasTenant := fieldOf(fields, tenantKey)
	tenant, isTenant := tenantOf(value)
	if !hasAllow || !isBool || !hasTenant || !isTenant {
		return TenantDetails{}, false, false
	}
	return tenant, allowed, true
}

// tenantOf returns the tenant that value, a JSON value of a reply, is, and false when it is not a
// JSON object with one field key, as fieldOf reads it, whose value is a string, or does not decode
// into a TenantDetails. The other fields decode as encoding/json reads them, and those that
// TenantDetails does not have are ignored.
func tenantOf(value json.RawMessage) (TenantDetails, bool) {
	fields, ok := membersOf(value)
	key, hasKey := fieldOf(fields, tenantKeyKey)
	var tenant TenantDetails
	if !ok || !hasKey || firstByte(key) != '"' || wire.Decode(value, &tenant) != nil {
		return TenantDetails{}, false
	}
	return tenant, true
}

// UserTenantsRequest is the body of the request that [Client.UserTenants] sends to the PDP, which
// the method builds from its user and its options, and which no method takes: it shows what each
// argument sends.
type UserTenantsRequest struct {
	// User is the user whose tenants the request asks for.
	User User `json:"user"`

	// Context is the context of the request, of [WithDecisionContext], which the PDP passes to
	// the policy: any JSON values, which reach the PDP as they are. Nil or empty sends none.
	Context map[string]any `json:"context,omitzero"`
}

// sent returns r as the body of a request holds it: with each of its maps of attributes or of
// context that is empty left out, as a nil one is, rather than sent as {}.
func (r UserTenantsRequest) sent() UserTenantsRequest {
	r.User.Attributes = nonEmpty(r.User.Attributes)
	r.Context = nonEmpty(r.Context)
	return r
}

// UserTenants returns the tenants of user that the PDP finds, each with its key and its
// attributes, in the order of the PDP's reply, or an empty list for a user without tenants. The
// PDP finds the tenants in which the user has a role: a user who is a member of a tenant without
// a role in it is not listed. With [WithDecisionContext], the call carries a context that the
// policy reads.
//
// UserTenants is container PDP only: it sends one request, POST /user-tenants, to the PDP at the
// PDP URL, [WithPDPURL], which only the PDP container serves. The cloud PDP answers it with 404
// and an empty body, which the call returns, after that one request, as an [Error] that matches
// [ErrContainerPDPOnly] and names the route. UserTenants needs an environment-level API key. The
// call changes nothing, so it sends its request again as [Client.Check] does.
//
// UserTenants returns every failure as an error, and never a list of tenants with it:
//
//   - a user without a key, or the zero CheckOption: an error that matches [ErrInvalidConfig],
//     and no request;
//   - a reply with an error status, such as 404 or 422, or 429 after the retries: an [Error] with
//     the status and the body of the reply;
//   - a reply in 200-299 that is not a JSON array, such as null or the tenants inside another
//     object, or that holds a tenant that is not a JSON object with one field key, spelled so,
//     whose value is a string, and attributes, when present, a JSON object or null: an Error that
//     matches [ErrMalformedDecision];
//   - a request that got no reply, such as one whose connection was refused: an Error whose Err
//     wraps the cause, such as a *url.Error.
func (c *Client) UserTenants(ctx context.Context, user User, opts ...CheckOption,
) ([]TenantDetails, error) {
	s, err := settingsOf("UserTenants", opts)
	if err != nil {
		return nil, err
	}
	if err = userError("UserTenants: user", user); err != nil {
		return nil, err
	}
	body := UserTenantsRequest{User: user, Context: s.context}
	op, reply, err := c.exchange(ctx, request{
		op: opUserTenants, target: pdpTarget, body: body.sent(),
	}, true)
	if err != nil {
		return nil, err
	}
	return readTenants(op, reply)
}

// readTenants returns the tenants that reply, the reply in 200-299 to a call of UserTenants of
// op, holds, in order: a JSON array whose every item is a tenant, as tenantOf reads it. Any other
// reply is an Error that matches ErrMalformedDecision.
func readTenants(op rest.Op, reply transport.Reply) ([]TenantDetails, error) {
	failed := opCall(op)
	var items []json.RawMessage
	if firstByte(reply.Body) != '[' || wire.Decode(reply.Body, &items) != nil {
		return nil, failed.malformedDecision(reply.Status, "the reply is not a JSON array of "+
			"tenants")
	}
	tenants := make([]TenantDetails, len(items))
	for i, item := range items {
		tenant, ok := tenantOf(item)
		if !ok {
			return nil, failed.malformedDecision(reply.Status, "tenant "+strconv.Itoa(i+1)+
				" of the reply is not a JSON object with one field key whose value is a string")
		}
		tenants[i] = tenant
	}
	return tenants, nil
}

// CheckURLRequest is the body of the request that [Client.CheckURL] sends to the PDP, which the
// method builds from its arguments and its options, and which no method takes: it shows what
// each argument sends.
type CheckURLRequest struct {
	// User is the user that the check asks about.
	User User `json:"user"`

	// HTTPMethod is the HTTP method of the request that the check asks about, such as GET.
	HTTPMethod string `json:"http_method"`

	// URL is the URL of the request that the check asks about.
	URL string `json:"url"`

	// Tenant is the key of the tenant of the check: default when the call is given none.
	Tenant string `json:"tenant"`

	// Context is the context of the check, of [WithDecisionContext], which the PDP passes to the
	// policy: any JSON values, which reach the PDP as they are. Nil or empty sends none.
	Context map[string]any `json:"context,omitzero"`
}

// sent returns r as the body of a request holds it: with the tenant default when it has no
// tenant, and with each of its maps of attributes or of context that is empty left out, as a
// nil one is, rather than sent as {}.
func (r CheckURLRequest) sent() CheckURLRequest {
	if r.Tenant == "" {
		r.Tenant = defaultTenant
	}
	r.User.Attributes = nonEmpty(r.User.Attributes)
	r.Context = nonEmpty(r.Context)
	return r
}

// CheckURL asks the PDP whether user may send a request with the HTTP method method, such as
// GET, to url, such as https://app.example.com/documents/2026-report, in the tenant whose key is
// tenant, and returns true when the PDP allows it and false when it denies it. The PDP decides by
// the mapping rules of the environment, which map a URL and an HTTP method to a resource and an
// action. An empty tenant sends default, the key of the default tenant of an environment, as a
// check of a resource without a tenant does. The method and the URL are sent as they are given:
// the SDK checks only that the URL is not empty. With [WithDecisionContext], the check carries a
// context that the policy reads.
//
// CheckURL is container PDP only: it sends one request, POST /allowed_url, to the PDP at the PDP
// URL, [WithPDPURL], which only the PDP container serves. The cloud PDP answers it with 404 and
// an empty body, which the call returns, after that one request, as an [Error] that matches
// [ErrContainerPDPOnly] and names the route. CheckURL needs an environment-level API key. The
// call changes nothing, so it sends its request again as [Client.Check] does.
//
// CheckURL returns false only for a decision that denies; it returns every failure as an error:
//
//   - a user without a key, an empty URL, or the zero CheckOption: an error that matches
//     [ErrInvalidConfig], and no request;
//   - a reply with an error status, such as 404 or 422, or 429 after the retries: an [Error] with
//     the status and the body of the reply;
//   - a reply in 200-299 that is not a decision, a JSON object with one key allow, spelled so,
//     whose value is true or false, as Check reads it: an Error that matches
//     [ErrMalformedDecision];
//   - a request that got no reply, such as one whose connection was refused: an Error whose Err
//     wraps the cause, such as a *url.Error.
func (c *Client) CheckURL(ctx context.Context, user User, method, url, tenant string,
	opts ...CheckOption,
) (bool, error) {
	s, err := settingsOf("CheckURL", opts)
	if err != nil {
		return false, err
	}
	if err = userError("CheckURL: user", user); err != nil {
		return false, err
	}
	if url == "" {
		return false, &configError{setting: "CheckURL: url", rule: validate.DecisionURLEmpty}
	}
	body := CheckURLRequest{
		User: user, HTTPMethod: method, URL: url, Tenant: tenant, Context: s.context,
	}
	op, reply, err := c.exchange(ctx, request{
		op: opCheckURL, target: pdpTarget, body: body.sent(),
	}, true)
	if err != nil {
		return false, err
	}
	return readDecision(op, reply)
}
