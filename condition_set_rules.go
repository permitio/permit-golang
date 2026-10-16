package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the condition set rules of an environment.
//
//nolint:gosec // G101: operation IDs, no credentials
const (
	opListSetPermissions     = "list_set_permissions"
	opAssignSetPermissions   = "assign_set_permissions"
	opUnassignSetPermissions = "unassign_set_permissions"
)

// ConditionSetRulesService lists, creates and deletes the condition set rules of an environment;
// [Client.ConditionSetRules] holds it. A rule grants the users of a user set a permission on the
// resources of a resource set, such as the permission document:read to the users older than 18
// on the documents that are not private: the two sets are condition sets of the environment,
// which [ConditionSetsService] creates. A rule has no key of its own in a request: a create names
// the user set, the permission and the resource set, and a delete names the same three.
//
// The routes of the rules start with /v2/facts/, as those of the facts API do, but only the API
// serves them: a client with [WithFactsViaPDP] sends them to the API too, with the project and
// the environment of its scope, and never to the PDP.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type ConditionSetRulesService struct {
	client *Client
}

// ConditionSetRulesListOptions are the page and the filters of [ConditionSetRulesService.List]
// and [ConditionSetRulesService.All]. Each filter that is set is sent as it is given, and keeps
// the rules that match it; a filter that is empty is left out of the query.
type ConditionSetRulesListOptions struct {
	// ListOptions picks the page and the number of rules on it, sent in page and per_page. With
	// a PerPage of 0, List leaves the number to the API, which gives 30, and All asks for 100,
	// the most that the API gives.
	ListOptions

	// UserSet keeps the rules that grant a permission to the user set, sent in the query
	// parameter user_set, such as adults.
	UserSet string

	// Permission keeps the rules that grant the permission, sent in the query parameter
	// permission.
	Permission string

	// ResourceSet keeps the rules that grant a permission on the resource set, sent in the query
	// parameter resource_set, such as public_docs.
	ResourceSet string
}

// conditionSetRulesRequest returns the request of the condition set rules that opts filters,
// with user_set, permission and resource_set in its query when opts sets them, and the page of
// opts. A nil opts is the zero ConditionSetRulesListOptions. The request holds copies of the
// values of opts, so a later change to opts does not change it.
func conditionSetRulesRequest(opts *ConditionSetRulesListOptions) (request, ListOptions) {
	var o ConditionSetRulesListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	for _, filter := range []struct{ name, value string }{
		{"user_set", o.UserSet}, {"permission", o.Permission}, {"resource_set", o.ResourceSet},
	} {
		if filter.value != "" {
			query.AddString(filter.name, &filter.value)
		}
	}
	return request{op: opListSetPermissions, target: apiTarget, query: query}, o.ListOptions
}

// List returns one page of the condition set rules of the environment that match the filters of
// opts, in the order of the API's reply, with GET /v2/facts/{proj_id}/{env_id}/set_rules. A nil
// opts, like the zero ConditionSetRulesListOptions, asks for the first page of every rule, with
// the API's default of 30 rules on it, and sends no filter; the API gives at most 100 rules on a
// page. The reply is a JSON array of rules, which reports no counts, so the TotalCount and the
// PageCount of the page are nil. [ConditionSetRulesService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ConditionSetRulesService] says. The call changes nothing, so it sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a scope without an environment: an error that matches [ErrInvalidConfig], and no request
//     for the list; without WithScope, the lookup of the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the list;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not a JSON array of rules, such as an empty body, null or an
//     object: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ConditionSetRulesService) List(ctx context.Context, opts *ConditionSetRulesListOptions,
) (*Page[models.ConditionSetRuleRead], error) {
	r, page := conditionSetRulesRequest(opts)
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.ConditionSetRuleRead](ctx, s.client, r, arrayShape, page)
}

// All returns an iterator over the condition set rules of the environment that match the filters
// of opts, for a range loop. It reads the pages in turn, as [ConditionSetRulesService.List] reads
// one, from the page of opts.Page on, or the first, each of opts.PerPage rules, or else of 100,
// the most that the API gives on a page. It stops after the first page without rules, and never
// after a page with fewer rules than it asked for. It asks for the next page only when the loop
// wants a rule past the last of a page, so a loop that stops early sends no other request. It
// reads opts when it is called: a later change to opts does not change the iterator. Each loop
// over the iterator reads the rules again, from the first page that opts asks for. The pages are
// requests of their own, so a rule created or deleted during a loop can move the others from one
// page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// ConditionSetRuleRead, and stops.
func (s *ConditionSetRulesService) All(ctx context.Context, opts *ConditionSetRulesListOptions,
) iter.Seq2[models.ConditionSetRuleRead, error] {
	r, page := conditionSetRulesRequest(opts)
	return allInScope[models.ConditionSetRuleRead](ctx, s.client, r, arrayShape, page)
}

// Create grants the users of the user set of rule its permission on the resources of its
// resource set, with POST /v2/facts/{proj_id}/{env_id}/set_rules, and returns the rules of the
// API's reply, a JSON array of rules, in its order; an empty array returns an empty slice, not
// nil. The request holds the user set, the permission and the resource set of rule, and its
// flags that are set, as [models.ConditionSetRuleCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ConditionSetRulesService] says. The API's documentation says that a
// permission that the user set has on the resource set already is skipped, so sending the create
// twice grants the permission once, and the call sends its request again after a 500, 502, 503
// or 504 reply or a failed connection, as after a 429.
//
// Create returns an error, and no rules:
//
//   - for a scope without an environment: an error that matches [ErrInvalidConfig], and no
//     request for the rule; without WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one whose Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON array of rules, such as an empty body, null or
//     an object: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ConditionSetRulesService) Create(ctx context.Context, rule models.ConditionSetRuleCreate,
) ([]models.ConditionSetRuleRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opAssignSetPermissions, target: apiTarget, body: rule,
	})
	if err != nil {
		return nil, err
	}
	list, err := doList[models.ConditionSetRuleRead](ctx, s.client, r, arrayShape)
	if err != nil {
		return nil, err
	}
	return list.Data, nil
}

// Delete takes the permission of rule away from the users of its user set on the resources of
// its resource set, with DELETE /v2/facts/{proj_id}/{env_id}/set_rules and a body that holds the
// user set, the permission and the resource set of rule, and its flags that are set, as
// [models.ConditionSetRuleRemove] says. The body, not the path, names what the call deletes, so
// a 404 can mean a mistyped key: Delete returns it as an [Error] that matches [ErrNotFound],
// never nil.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ConditionSetRulesService] says. The API's documentation says that a
// permission that the user set does not have on the resource set is skipped, so deleting a rule
// twice leaves it deleted, and the call sends its request again after a 500, 502, 503 or 504
// reply or a failed connection, as after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment: an error that matches [ErrInvalidConfig], and no
//     request for the rule; without WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an Error with the status and the body of the reply,
//     such as one that matches ErrNotFound for a 404, or [ErrForbidden] for a key that may not
//     delete the rule;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ConditionSetRulesService) Delete(ctx context.Context,
	rule models.ConditionSetRuleRemove,
) error {
	r, err := s.client.inScope(ctx, request{
		op: opUnassignSetPermissions, target: apiTarget, body: rule,
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}
