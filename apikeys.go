package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the API keys.
//
//nolint:gosec // G101: operation IDs, no credentials
const (
	opGetEnvironmentAPIKey = "get_environment_api_key"
	opListAPIKeys          = "list_api_keys"
	opCreateAPIKey         = "create_api_key"
	opGetAPIKey            = "get_api_key"
	opDeleteAPIKey         = "delete_api_key"
	opRotateAPIKey         = "rotate_api_key"
)

// apiKeyScopeID is the API key ID that the methods of one key reject: GET /v2/api-key/scope is
// the route of the scope of the API key, not of a key.
const apiKeyScopeID = "scope"

// APIKeysService reads the API key of an environment, and lists, reads, creates, deletes and
// rotates the API keys of the organization of the client's API key; [Client.APIKeys] holds it.
// A method that acts on one key takes its ID, and sends it as one segment of the path, escaped.
//
// Its calls need no scope: they send no lookup of the scope of the API key, and fill in no
// project and no environment from [WithScope]. Each method says the level of API key that it
// needs; the API answers a key of a narrower level with an error status, which the call returns
// as an [Error], such as one that matches [ErrForbidden].
//
// The secret of each key that a call returns is a [models.Secret], which prints a marker in
// place of the credential: read it with its Value method. The client adds each secret that its
// calls return to the credentials that it keeps out of its errors and its log lines, as it does
// its own API key, so an error body or a log line of a later call of the client that holds the
// secret shows the marker in its place.
type APIKeysService struct {
	client *Client
}

// APIKeysListOptions are the page and the filters of [APIKeysService.List] and
// [APIKeysService.All]. Each filter that is not empty keeps only the keys that match it, and the
// call sends it as it is given, in the query parameter that its doc comment names. An empty
// filter, as each is in the zero APIKeysListOptions, is not sent and keeps every key.
type APIKeysListOptions struct {
	// ListOptions picks the page and the number of keys on it, sent in page and per_page. With a
	// PerPage of 0, List leaves the number to the API, which gives 30, and All asks for 100, the
	// most that the API gives.
	ListOptions

	// ObjectType keeps the keys of this level, such as [models.MemberAccessObjEnv] for the keys
	// of environments. Its query parameter is object_type.
	ObjectType models.MemberAccessObj

	// Project names the project whose keys to keep. It is sent as it is given in the query
	// parameter proj_id.
	Project string
}

// checkKeyID returns the error that a call of op on the API key whose ID is apiKeyID returns
// before any request: ErrClosed after Close, then an error that matches ErrInvalidConfig for the
// ID scope, which names the route of the scope of the API key, not a key. The call itself
// rejects the other IDs that it cannot send, such as an empty one.
func (s *APIKeysService) checkKeyID(op, apiKeyID string) error {
	if err := s.client.checkOpen(); err != nil {
		return err
	}
	if apiKeyID != apiKeyScopeID {
		return nil
	}
	o, _ := s.client.registry.Op(op)
	return &configError{
		setting: op + ": {api_key_id} of " + o.Route, rule: validate.PathAPIKeyIDScope,
	}
}

// apiKeysRequest returns the request of the keys that opts filters, with a parameter in its
// query for each filter of opts that is not empty, and the page of opts. A nil opts is the zero
// APIKeysListOptions. The request holds copies of the values of opts, so a later change to opts
// does not change it.
func apiKeysRequest(opts *APIKeysListOptions) (request, ListOptions) {
	var o APIKeysListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	for _, filter := range []struct{ name, value string }{
		{"object_type", string(o.ObjectType)},
		{"proj_id", o.Project},
	} {
		if filter.value != "" {
			query.AddString(filter.name, &filter.value)
		}
	}
	return request{op: opListAPIKeys, target: apiTarget, query: query}, o.ListOptions
}

// GetEnvironmentKey returns the API key of the environment whose key or ID is environmentKey, in
// the project whose key or ID is projectKey, with GET /v2/api-key/{proj_id}/{env_id}. Its
// [models.APIKeyRead.Secret] is the API key of the environment, which a client that acts on the
// environment, such as one that checks permissions in it, takes with [WithAPIKey]. The client
// adds the secret to the credentials that it keeps out of its errors and log lines, as
// [APIKeysService] says.
//
// GetEnvironmentKey needs a project-level API key of the project, or higher. It needs no scope:
// it reads the project and the environment from its parameters, not from [WithScope]. The call
// changes nothing, so it sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// GetEnvironmentKey returns an error, and no key:
//
//   - for a projectKey or an environmentKey that is empty, . or ..: an error that matches
//     [ErrInvalidConfig], and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the project has no such environment;
//   - for a reply in 200-299 that is not a JSON object of an API key: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *APIKeysService) GetEnvironmentKey(ctx context.Context, projectKey,
	environmentKey string,
) (*models.APIKeyRead, error) {
	r := request{
		op: opGetEnvironmentAPIKey, target: apiTarget, params: []string{projectKey, environmentKey},
	}
	return do[models.APIKeyRead](ctx, s.client, r)
}

// List returns one page of the API keys of the organization, in the order of the API's reply,
// those that match each filter of opts, with GET /v2/api-key. A nil opts, like the zero
// APIKeysListOptions, asks for the first page of every key, with the API's default of 30 keys on
// it; the API gives at most 100 on a page. The page holds the counts that the reply reports.
// [APIKeysService.All] reads every page.
//
// List needs an organization-level API key, and no scope. The call changes nothing, so it sends
// its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero: an error that matches [ErrInvalidConfig], and no
//     request;
//   - a reply with an error status, such as 403 for a key that may not list the API keys, or 422
//     for a PerPage above 100: an [Error] with the status and the body of the reply;
//   - a reply in 200-299 that is not the paginated JSON object of API keys: an Error that says
//     so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *APIKeysService) List(ctx context.Context, opts *APIKeysListOptions,
) (*Page[models.APIKeyRead], error) {
	r, page := apiKeysRequest(opts)
	return listPage[models.APIKeyRead](ctx, s.client, r, pageShape, page)
}

// All returns an iterator over the API keys of the organization that match each filter of opts,
// for a range loop. It reads the pages in turn, as [APIKeysService.List] reads one, from the
// page of opts.Page on, or the first, each of opts.PerPage keys, or else of 100, the most that
// the API gives on a page. It stops after the last page that the replies count, or after the
// first page without keys, and never after a page with fewer keys than it asked for. It asks for
// the next page only when the loop wants a key past the last of a page, so a loop that stops
// early sends no other request. It reads opts when it is called: a later change to opts does not
// change the iterator. Each loop over the iterator reads the keys again, from the first page
// that opts asks for. The pages are requests of their own, so a key created or deleted during a
// loop can move the others from one page to the next.
//
// All needs an organization-level API key, and no scope, as List does. Each page is a call of
// its own, with its own timeout and retries. When a page fails, with one of the errors that List
// returns, the iterator yields its error once, with the zero APIKeyRead, and stops.
func (s *APIKeysService) All(ctx context.Context, opts *APIKeysListOptions,
) iter.Seq2[models.APIKeyRead, error] {
	r, page := apiKeysRequest(opts)
	return all[models.APIKeyRead](ctx, s.client, r, pageShape, page)
}

// Get returns the API key whose ID is apiKeyID, with GET /v2/api-key/{api_key_id}.
//
// Get needs an organization-level API key, and no scope. The call changes nothing, so it sends
// its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no key:
//
//   - for an apiKeyID that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request;
//   - for the apiKeyID scope, which names the route of the scope of the API key, not a key: an
//     error that matches ErrInvalidConfig and says so, and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the organization has no such key;
//   - for a reply in 200-299 that is not a JSON object of an API key: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *APIKeysService) Get(ctx context.Context, apiKeyID string) (*models.APIKeyRead, error) {
	if err := s.checkKeyID(opGetAPIKey, apiKeyID); err != nil {
		return nil, err
	}
	r := request{op: opGetAPIKey, target: apiTarget, params: []string{apiKeyID}}
	return do[models.APIKeyRead](ctx, s.client, r)
}

// Create creates an API key in the organization with POST /v2/api-key, and returns it as the API
// stores it, with its ID and its secret. The request holds the fields of key that are set, as
// [models.APIKeyCreate] says. The client adds the secret to the credentials that it keeps out of
// its errors and log lines, as [APIKeysService] says.
//
// Create needs an organization-level API key, and no scope. It is sent again only after a 429,
// which the API did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the
// API may have created the key, so Create returns that error.
//
// Create returns an error, and no key:
//
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     whose Detail names each field that the API rejects for a 422, such as an ID that is not a
//     UUID;
//   - for a reply in 200-299 that is not a JSON object of an API key: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *APIKeysService) Create(ctx context.Context, key models.APIKeyCreate,
) (*models.APIKeyRead, error) {
	r := request{op: opCreateAPIKey, target: apiTarget, body: key}
	return do[models.APIKeyRead](ctx, s.client, r)
}

// Delete deletes the API key whose ID is apiKeyID, with DELETE /v2/api-key/{api_key_id}. It
// returns nil when the API answers 404, as the key is gone already, such as when an earlier
// attempt of the call deleted it; so a mistyped ID returns nil too, and deletes nothing.
//
// Delete needs an organization-level API key, and no scope. Deleting a key twice leaves it
// deleted, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// Delete returns an error:
//
//   - for an apiKeyID that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request;
//   - for the apiKeyID scope, which names the route of the scope of the API key, not a key: an
//     error that matches ErrInvalidConfig and says so, and no request;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete keys;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *APIKeysService) Delete(ctx context.Context, apiKeyID string) error {
	if err := s.checkKeyID(opDeleteAPIKey, apiKeyID); err != nil {
		return err
	}
	r := request{op: opDeleteAPIKey, target: apiTarget, params: []string{apiKeyID}}
	return s.client.doNoValue(ctx, r)
}

// RotateSecret gives the API key whose ID is apiKeyID a new secret, with
// POST /v2/api-key/{api_key_id}/rotate-secret, and returns the key with the new secret. The
// client adds the new secret to the credentials that it keeps out of its errors and log lines,
// as [APIKeysService] says. A client keeps the API key that it was built with: a client whose
// own key is rotated keeps sending the secret that [WithAPIKey] gave it, so build a new client
// with the new secret.
//
// RotateSecret needs an organization-level API key, and no scope. It is sent once: the call
// never sends its request again. After a 500, 502, 503 or 504 reply or a failed connection, the
// API may have rotated the secret, and a second rotation would replace the secret that the first
// returned, so the call returns the error, and cannot tell whether the key has a new secret.
// After a 429, which the API did not act on, the call also returns the error, and leaves the
// retry to the caller.
//
// RotateSecret returns an error, and no key:
//
//   - for an apiKeyID that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request;
//   - for the apiKeyID scope, which names the route of the scope of the API key, not a key: an
//     error that matches ErrInvalidConfig and says so, and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the organization has no such key, and [ErrRateLimited]
//     for a 429;
//   - for a reply in 200-299 that is not a JSON object of an API key: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *APIKeysService) RotateSecret(ctx context.Context, apiKeyID string,
) (*models.APIKeyRead, error) {
	if err := s.checkKeyID(opRotateAPIKey, apiKeyID); err != nil {
		return nil, err
	}
	r := request{op: opRotateAPIKey, target: apiTarget, params: []string{apiKeyID}}
	return do[models.APIKeyRead](ctx, s.client, r)
}
