package permit

import (
	"context"
	"iter"
	"slices"

	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the proxy configs of an environment.
const (
	opListProxyConfigs  = "list_proxy_configs"
	opCreateProxyConfig = "create_proxy_config"
	opGetProxyConfig    = "get_proxy_config"
	opUpdateProxyConfig = "update_proxy_config"
	opDeleteProxyConfig = "delete_proxy_config"
)

// ProxyConfigsService lists, reads, creates, changes and deletes the proxy configs of an
// environment; [Client.ProxyConfigs] holds it. A proxy config holds the credential that the
// Permit proxy adds to the requests that it sends on to a backend service, its secret, and the
// mapping rules that map a URL and an HTTP method to a resource and an action, which
// [Client.CheckURL] decides on. A method that acts on one config takes its key or its ID, such as
// payments, and sends it as one segment of the path, escaped: a key that holds a / or a % names
// that config, not another route.
//
// The secret is a [models.ProxySecret]: a Basic credential, a Bearer token or a set of headers,
// as the config's [models.AuthMechanism] says. Create and Update send its credentials, which
// [models.Secret.Value] returns, with the auth mechanism of the branch that it sets when the
// call's AuthMechanism is empty, and each reply holds the credentials of the secret. The SDK
// keeps them out of what it prints and logs: a ProxySecret prints the marker in place of each
// credential, and the client adds each credential that a call sends or reads, the value of each
// header of a secret of headers too, to the credentials that its errors and its log lines replace
// with the marker, wherever they appear, for as long as the client lives, as it does its API key.
// So a secret of headers should hold only credentials, as [models.ProxySecret] says.
//
// The routes of the configs start with /v2/facts/, as those of the facts API do, but only the API
// serves them: a client with [WithFactsViaPDP] sends them to the API too, with the project and the
// environment of its scope, and never to the PDP.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type ProxyConfigsService struct {
	client *Client
}

// proxyConfigCreateBody is the body of a create of a proxy config: the fields of the model, with
// the credentials of its secret in place of the markers that the model's secret encodes.
type proxyConfigCreateBody struct {
	models.ProxyConfigCreate

	// Secret is the secret as the request sends it, which proxySecretBody returns.
	Secret any `json:"secret"`
}

// proxyConfigUpdateBody is the body of an update of a proxy config: the fields of the model, with
// the credentials of its secret in place of the markers that the model's secret encodes.
type proxyConfigUpdateBody struct {
	models.ProxyConfigUpdate

	// Secret is the secret as the request sends it, which proxySecretBody returns.
	Secret any `json:"secret"`
}

// proxySecretBranch is a branch of a ProxySecret that is set: the auth mechanism whose
// credential it is, the JSON value that a request sends for it, and its credentials.
type proxySecretBranch struct {
	mechanism   models.AuthMechanism
	value       any
	credentials []string
}

// proxySecretBranches returns the branches that secret sets: Basic and Bearer when they are not
// nil, as a string each, and Headers when it is not nil, as a map of strings by the names of the
// headers.
func proxySecretBranches(secret models.ProxySecret) []proxySecretBranch {
	var branches []proxySecretBranch
	if secret.Basic != nil {
		value := secret.Basic.Value()
		branches = append(branches,
			proxySecretBranch{models.AuthMechanismBasic, value, []string{value}})
	}
	if secret.Bearer != nil {
		value := secret.Bearer.Value()
		branches = append(branches,
			proxySecretBranch{models.AuthMechanismBearer, value, []string{value}})
	}
	if secret.Headers != nil {
		headers := make(map[string]string, len(secret.Headers))
		credentials := make([]string, 0, len(secret.Headers))
		for name, value := range secret.Headers {
			headers[name] = value.Value()
			credentials = append(credentials, value.Value())
		}
		branches = append(branches,
			proxySecretBranch{models.AuthMechanismHeaders, headers, credentials})
	}
	return branches
}

// proxySecretBody returns the JSON value that the body of a call of the operation op sends for
// secret, the credential of Basic or of Bearer as a string, or the credentials of Headers as a
// map of strings, by the names of the headers; and the auth mechanism that the body sends with
// it: mechanism, or, when it is empty, the mechanism of the branch that secret sets. It adds
// each credential to the client's Set, so that the client's errors and log lines replace it from
// then on, in the error of the call too, whose body may echo it.
//
// It returns an error that matches ErrInvalidConfig and names op, and adds nothing, when secret
// sets more than one branch, when it holds no credential, as it sets none or the one that it sets
// is empty or has an empty value, and when mechanism is one that this version of the SDK knows
// and names another branch. A mechanism that it does not know is sent as it is, for the API to
// decide on.
func (c *Client) proxySecretBody(op string, mechanism models.AuthMechanism,
	secret models.ProxySecret,
) (any, models.AuthMechanism, error) {
	branches := proxySecretBranches(secret)
	if len(branches) > 1 {
		return nil, "", &configError{setting: op + ": secret", rule: validate.ProxyConfigSecretMany}
	}
	if len(branches) == 0 || len(branches[0].credentials) == 0 ||
		slices.Contains(branches[0].credentials, "") {
		return nil, "", &configError{setting: op + ": secret", rule: validate.ProxyConfigSecretNone}
	}
	branch := branches[0]
	if mechanism == "" {
		mechanism = branch.mechanism
	}
	if mechanism.Known() && mechanism != branch.mechanism {
		return nil, "", &configError{
			setting: op + ": auth_mechanism", rule: validate.ProxyConfigAuthMechanismMismatch,
		}
	}
	c.secrets.Add(branch.credentials...)
	return branch.value, mechanism, nil
}

// call makes the call of r, a request of one proxy config, in the environment of the client's
// scope, and returns the config of its reply.
func (s *ProxyConfigsService) call(ctx context.Context, r request,
) (*models.ProxyConfigRead, error) {
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return do[models.ProxyConfigRead](ctx, s.client, r)
}

// List returns one page of the proxy configs of the environment, in the order of the API's
// reply, with GET /v2/facts/{proj_id}/{env_id}/proxy_configs. A nil opts, like the zero
// ListOptions, asks for the first page, with the API's default of 30 configs on it; the API gives
// at most 100 on a page. The request's query holds page and per_page when opts sets them, and
// nothing else. The reply reports no counts, so the TotalCount and the PageCount of the page are
// nil. Each config holds its secret, which prints the marker in place of each credential.
// [ProxyConfigsService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ProxyConfigsService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a scope without an environment: an error that matches [ErrInvalidConfig], and no request
//     for the list; without WithScope, the lookup of the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the list;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply, with the marker in place of each credential;
//   - a reply in 200-299 that is not a JSON array of configs: an Error that says so, and holds
//     nothing of the reply's body;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ProxyConfigsService) List(ctx context.Context, opts *ListOptions,
) (*Page[models.ProxyConfigRead], error) {
	r, err := s.client.inScope(ctx, request{op: opListProxyConfigs, target: apiTarget})
	if err != nil {
		return nil, err
	}
	return listPage[models.ProxyConfigRead](ctx, s.client, r, arrayShape, listOptions(opts))
}

// All returns an iterator over the proxy configs of the environment, for a range loop. It reads
// the pages in turn, as [ProxyConfigsService.List] reads one, from the page of opts.Page on, or
// the first, each of opts.PerPage configs, or else of 100, the most that the API gives on a page.
// It stops after the first page without configs, and never after a page with fewer configs than
// it asked for. It asks for the next page only when the loop wants a config past the last of a
// page, so a loop that stops early sends no other request. It reads opts when it is called: a
// later change to opts does not change the iterator. Each loop over the iterator reads the
// configs again, from the first page that opts asks for. The pages are requests of their own, so
// a config created or deleted during a loop can move the others from one page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// ProxyConfigRead, and stops.
func (s *ProxyConfigsService) All(ctx context.Context, opts *ListOptions,
) iter.Seq2[models.ProxyConfigRead, error] {
	r := request{op: opListProxyConfigs, target: apiTarget}
	return allInScope[models.ProxyConfigRead](ctx, s.client, r, arrayShape, listOptions(opts))
}

// Get returns the proxy config whose key or ID is proxyConfigKey, with its secret, with
// GET /v2/facts/{proj_id}/{env_id}/proxy_configs/{proxy_config_id}. The secret is read as
// [models.ProxyConfigRead.UnmarshalJSON] says: Headers for a JSON object, and Basic or Bearer for
// a JSON string, as the config's auth mechanism names it.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ProxyConfigsService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no config:
//
//   - for a scope without an environment, or a proxyConfigKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the config; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     with the marker in place of each credential, such as one that matches [ErrNotFound] when
//     the environment has no such config;
//   - for a reply in 200-299 that is not a JSON object of a config, such as one whose secret is a
//     number: an Error that says so, and holds nothing of the reply's body;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ProxyConfigsService) Get(ctx context.Context, proxyConfigKey string,
) (*models.ProxyConfigRead, error) {
	return s.call(ctx, request{
		op: opGetProxyConfig, target: apiTarget, params: []string{proxyConfigKey},
	})
}

// Create creates a proxy config in the environment with
// POST /v2/facts/{proj_id}/{env_id}/proxy_configs, and returns it as the API stores it, with its
// ID and its secret. The request holds the key, the name, the auth mechanism and the secret of
// config, and its mapping rules when they are set, as [models.ProxyConfigCreate] says. It sends
// the secret's credentials, not the markers that the model's own JSON holds: the credential of
// Basic or of Bearer as a JSON string, and Headers as a JSON object of strings. It sends the
// auth mechanism of config, or, when it is empty, the mechanism of the branch that the secret
// sets.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ProxyConfigsService] says. It is sent again only after a 429, which the API
// did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// created the config, so Create returns that error, and [ProxyConfigsService.Get] of the key
// tells whether the config exists.
//
// Create returns an error, and no config:
//
//   - for a secret that holds no credential, as it sets none of Basic, Bearer and Headers, such
//     as the zero ProxySecret, or the one that it sets is empty or has an empty value; for a
//     secret that sets more than one; and for an auth mechanism that names another branch than
//     the one that the secret sets: an error that matches [ErrInvalidConfig], and no request at
//     all, not even the lookup of the scope;
//   - for a scope without an environment: an error that matches ErrInvalidConfig, and no
//     request for the config; without WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     with the marker in place of each credential, also where the body echoes the secret, such
//     as one that matches [ErrConflict] when the environment has a config with the key already,
//     or one whose Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a config: an Error that says so, and
//     holds nothing of the reply's body;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ProxyConfigsService) Create(ctx context.Context, config models.ProxyConfigCreate,
) (*models.ProxyConfigRead, error) {
	if err := s.client.checkOpen(); err != nil {
		return nil, err
	}
	secret, mechanism, err := s.client.proxySecretBody(opCreateProxyConfig, config.AuthMechanism,
		config.Secret)
	if err != nil {
		return nil, err
	}
	config.AuthMechanism = mechanism
	return s.call(ctx, request{
		op: opCreateProxyConfig, target: apiTarget,
		body: proxyConfigCreateBody{ProxyConfigCreate: config, Secret: secret},
	})
}

// Update changes the proxy config whose key or ID is proxyConfigKey with
// PATCH /v2/facts/{proj_id}/{env_id}/proxy_configs/{proxy_config_id}, and returns it as the API
// stores it. The request always holds the secret of update, as the API answers 422 to an update
// without it, and the auth mechanism, as Create does; it holds the name and the mapping rules of
// update only when they are set, as [models.ProxyConfigUpdate] says. It sends the secret's
// credentials, as Create does. The API adds the mapping rules of update to those that it stores,
// and removes a rule only when its ShouldDelete is set.
//
// Update is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ProxyConfigsService] says. It is sent again only after a 429, which the API
// did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// changed the config, and a second request could add or remove a mapping rule again, so Update
// returns that error, and [ProxyConfigsService.Get] tells what the config holds.
//
// Update returns an error, and no config:
//
//   - for a secret that holds no credential, or sets more than one branch, and for an auth
//     mechanism that names another branch than the one that the secret sets, as for Create: an
//     error that matches [ErrInvalidConfig], and no request at all, not even the lookup of the
//     scope;
//   - for a scope without an environment, or a proxyConfigKey that is empty, . or ..: an error
//     that matches ErrInvalidConfig, and no request for the config; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     with the marker in place of each credential, also where the body echoes the secret, such
//     as one that matches [ErrNotFound] when the environment has no such config, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a config: an Error that says so, and
//     holds nothing of the reply's body;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ProxyConfigsService) Update(ctx context.Context, proxyConfigKey string,
	update models.ProxyConfigUpdate,
) (*models.ProxyConfigRead, error) {
	if err := s.client.checkOpen(); err != nil {
		return nil, err
	}
	secret, mechanism, err := s.client.proxySecretBody(opUpdateProxyConfig, update.AuthMechanism,
		update.Secret)
	if err != nil {
		return nil, err
	}
	update.AuthMechanism = mechanism
	return s.call(ctx, request{
		op: opUpdateProxyConfig, target: apiTarget, params: []string{proxyConfigKey},
		body: proxyConfigUpdateBody{ProxyConfigUpdate: update, Secret: secret},
	})
}

// Delete deletes the proxy config whose key or ID is proxyConfigKey, with all its data, with
// DELETE /v2/facts/{proj_id}/{env_id}/proxy_configs/{proxy_config_id}. It returns nil when the API
// answers 404, as the config is gone already, such as when an earlier attempt of the call deleted
// it; so a mistyped key returns nil too, and deletes nothing.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ProxyConfigsService] says. Deleting a config twice leaves it deleted, so the
// call sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as
// after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a proxyConfigKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the config; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     config;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ProxyConfigsService) Delete(ctx context.Context, proxyConfigKey string) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteProxyConfig, target: apiTarget, params: []string{proxyConfigKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}
