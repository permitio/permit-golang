package permit

import (
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/permitio/permit-golang/v2/internal/logging"
	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/scope"
	"github.com/permitio/permit-golang/v2/internal/transport"
	"github.com/permitio/permit-golang/v2/internal/validate"
)

// maxIdleConnsPerHost is how many idle connections to each host the transport of a client's own
// http.Client keeps.
const maxIdleConnsPerHost = 64

// Client is a client of the Permit API and of the policy decision point (PDP). Build one with
// [New]; the zero Client is not valid. A Client is safe for concurrent use. Call [Client.Close]
// when you are done with it.
//
// A Client prints its settings with a marker in place of its API key: [Client.String],
// [Client.GoString], [Client.Format] with every verb and [Client.LogValue] all give the same
// text.
//
// The calls of the Permit API are grouped by what they act on, in the services that the fields
// of a Client hold, such as [Client.Projects]; the decisions of the PDP are methods of the
// Client itself, such as [Client.Check]. A Client encodes as the empty JSON object: it holds
// no data.
type Client struct {
	// APIKeys reads the API key of an environment, and lists, reads, creates, deletes and
	// rotates the API keys of the organization.
	APIKeys *APIKeysService `json:"-"`

	// Projects lists, reads, creates, changes and deletes the projects of the organization.
	Projects *ProjectsService `json:"-"`

	// Environments lists, reads, creates, changes, copies and deletes the environments of a
	// project, and reads their statistics.
	Environments *EnvironmentsService `json:"-"`

	// Resources lists, reads, creates, replaces, changes and deletes the resources of the
	// environment of the client's scope.
	Resources *ResourcesService `json:"-"`

	// Tenants lists, reads, creates, changes and deletes the tenants of an environment, and
	// lists, adds and removes the users in a tenant.
	Tenants *TenantsService `json:"-"`

	// ResourceActions lists, reads, creates, changes and deletes the actions of a resource of
	// the environment of the client's scope.
	ResourceActions *ResourceActionsService `json:"-"`
	// ResourceActionGroups lists, reads, creates, changes and deletes the action groups of the
	// resources of the environment of the client's scope.
	ResourceActionGroups *ResourceActionGroupsService `json:"-"`

	// ResourceAttributes lists, reads, creates, changes and deletes the attributes of a resource
	// of the environment of the client's scope.
	ResourceAttributes *ResourceAttributesService `json:"-"`

	// ResourceRelations lists, reads, creates and deletes the relations of the resources of the
	// environment of the client's scope.
	ResourceRelations *ResourceRelationsService `json:"-"`

	// Roles lists, reads, creates, changes and deletes the roles of the environment of the
	// client's scope, and changes their permissions.
	Roles *RolesService `json:"-"`

	// ResourceRoles lists, reads, creates, changes and deletes the roles of the resources of the
	// environment of the client's scope, and changes their permissions.
	ResourceRoles *ResourceRolesService `json:"-"`

	// ImplicitGrants creates and deletes the implicit grants of the roles of the resources of the
	// environment of the client's scope, and sets their conditions.
	ImplicitGrants *ImplicitGrantsService `json:"-"`

	// UserAttributes lists, reads, creates, changes and deletes the attributes of the users of
	// the environment of the client's scope: the schema of what a user may hold, not its values.
	UserAttributes *UserAttributesService `json:"-"`

	// ConditionSets lists, reads, creates, changes and deletes the condition sets of the
	// environment of the client's scope, its user sets and resource sets, and lists their
	// ancestors and descendants.
	ConditionSets *ConditionSetsService `json:"-"`

	// ConditionSetRules lists, creates and deletes the condition set rules of the environment of
	// the client's scope, each of which grants the users of a user set a permission on the
	// resources of a resource set.
	ConditionSetRules *ConditionSetRulesService `json:"-"`

	// Users lists, reads, creates, syncs, changes and deletes the users of an environment, and
	// assigns and unassigns their roles.
	Users *UsersService `json:"-"`

	// RoleAssignments lists, assigns and unassigns the roles that the users of an environment
	// have in its tenants and on its resource instances.
	RoleAssignments *RoleAssignmentsService `json:"-"`

	// ResourceInstances lists, reads, creates, changes and deletes the resource instances of an
	// environment, and lists them with their relationships.
	ResourceInstances *ResourceInstancesService `json:"-"`

	// RelationshipTuples lists, creates and deletes the relationship tuples of an environment,
	// the relations between its resource instances.
	RelationshipTuples *RelationshipTuplesService `json:"-"`

	// ProxyConfigs lists, reads, creates, changes and deletes the proxy configs of the
	// environment of the client's scope: the credentials that the Permit proxy adds to the
	// requests that it sends on, and the mapping rules of the URLs that CheckURL checks.
	ProxyConfigs *ProxyConfigsService `json:"-"`

	// Groups lists, reads, creates and deletes the groups of the environment of the client's
	// scope, and assigns, lists and removes the users of a group.
	Groups *GroupsService `json:"-"`

	// PDPs asks the PDPs of the environment of the client's scope to read their data again.
	PDPs *PDPsService `json:"-"`

	// AuditLogs lists and reads the audit logs of the environment of the client's scope, the
	// decisions that its PDPs made.
	AuditLogs *AuditLogsService `json:"-"`

	apiKey  redact.Secret
	apiURL  string
	pdpURL  string
	timeout time.Duration

	// secrets holds the API key when it has at least 16 bytes, and each credential of at least 8
	// bytes that the client's calls return, or send in the secret of a proxy config, which the
	// client's errors never hold.
	secrets *redact.Set

	httpClient *http.Client

	// transport is the transport of httpClient when New created it, which Close closes, and nil
	// when the http.Client came from WithHTTPClient.
	transport *http.Transport

	// sender sends each attempt of a request through httpClient, with the API key.
	sender *transport.Sender

	// retry decides whether a call sends its request again after an attempt that failed.
	retry transport.Policy

	// logger writes the client's log lines, with the members of secrets replaced.
	logger *logging.Logger

	// registry holds the operations that the client can send: rest.Default, and in a test
	// client that export_test.go builds, the test's test-only operations too.
	registry *rest.Registry

	// scope gives the project and the environment that the client's calls act on: the scope that
	// WithScope gave, or the scope of the API key, which it looks up once.
	scope *scope.Resolver

	// factsViaPDP says that the calls of the facts API go to the PDP container, as
	// WithFactsViaPDP says.
	factsViaPDP bool

	closed atomic.Bool
}

// New returns a Client configured by opts. A setting that no option gives comes from its
// environment variable, when the variable is set and not empty, and else from its default:
//
//   - the API key, [WithAPIKey], from PERMIT_API_KEY, without a default: New fails without a key;
//   - the API URL, [WithAPIURL], from PERMIT_API_URL, by default https://api.permit.io;
//   - the PDP URL, [WithPDPURL], from PERMIT_PDP_URL, by default http://localhost:7766;
//   - the *http.Client, [WithHTTPClient], by default one of the Client's own;
//   - the timeout of a call, [WithTimeout], by default 30 seconds;
//   - the retries of a call, [WithRetry], by default up to 3;
//   - the logger, [WithLogger], by default none: the client logs nothing;
//   - the scope, [WithScope], by default the scope of the API key, which the client looks up
//     once, as [Client.Scope] describes;
//   - where the calls of the facts API go, [WithFactsViaPDP], by default to the Permit API.
//
// New sends no request. It returns an error that matches [ErrInvalidConfig], and no Client, when
// the configuration breaks a rule that an option's documentation states, such as a URL without
// http:// or https://, or when an Option is the zero Option. The error names the option or the
// environment variable, and never holds the API key. New logs a warning when the API URL uses
// http:// to a host that is not loopback, and when WithFactsViaPDP sends facts to the cloud PDP,
// as WithLogger describes.
func New(opts ...Option) (*Client, error) {
	var s settings
	for i, opt := range opts {
		if opt.apply == nil {
			return nil, &configError{
				setting: "option " + strconv.Itoa(i+1) + " of New",
				rule:    validate.OptionZero,
			}
		}
		if err := opt.apply(&s); err != nil {
			return nil, err
		}
	}
	if err := s.resolve(); err != nil {
		return nil, err
	}
	c := &Client{
		apiKey:     s.apiKey,
		apiURL:     s.apiURL,
		pdpURL:     s.pdpURL,
		timeout:    s.timeout,
		retry:      *s.retry,
		secrets:    redact.NewSet(s.apiKey.Value()),
		httpClient: s.httpClient,
		registry:   rest.Default(),
	}
	c.APIKeys = &APIKeysService{client: c}
	c.Projects = &ProjectsService{client: c}
	c.Environments = &EnvironmentsService{client: c}
	c.Resources = &ResourcesService{client: c}
	c.Tenants = &TenantsService{facts: facts{client: c}}
	c.ResourceActions = &ResourceActionsService{client: c}
	c.ResourceActionGroups = &ResourceActionGroupsService{client: c}
	c.ResourceAttributes = &ResourceAttributesService{client: c}
	c.ResourceRelations = &ResourceRelationsService{client: c}
	c.Roles = &RolesService{client: c}
	c.ResourceRoles = &ResourceRolesService{client: c}
	c.ImplicitGrants = &ImplicitGrantsService{client: c}
	c.UserAttributes = &UserAttributesService{client: c}
	c.ConditionSets = &ConditionSetsService{client: c}
	c.ConditionSetRules = &ConditionSetRulesService{client: c}
	c.Users = &UsersService{facts: facts{client: c}}
	c.RoleAssignments = &RoleAssignmentsService{facts: facts{client: c}}
	c.ResourceInstances = &ResourceInstancesService{facts: facts{client: c}}
	c.RelationshipTuples = &RelationshipTuplesService{facts: facts{client: c}}
	c.ProxyConfigs = &ProxyConfigsService{client: c}
	c.Groups = &GroupsService{client: c}
	c.PDPs = &PDPsService{client: c}
	c.AuditLogs = &AuditLogsService{client: c}
	c.factsViaPDP = s.factsViaPDP
	if s.scope != nil {
		c.scope = scope.Known(*s.scope)
	} else {
		c.scope = scope.New(c.lookupScope)
	}
	c.logger = logging.New(s.logger, c.secrets)
	if plainHTTPToRemote(c.apiURL) {
		c.logger.PlainHTTP(c.apiURL)
	}
	if c.factsViaPDP && isCloudPDP(c.pdpURL) {
		c.logger.FactsViaCloudPDP(c.pdpURL)
	}
	if c.httpClient == nil {
		c.httpClient, c.transport = ownHTTPClient()
	}
	c.sender = transport.New(c.httpClient, c.apiKey, Version)
	if c.transport != nil {
		// The cleanup gets the transport, not c, so that it does not keep c reachable. It runs
		// after Close too, for the connections that calls still in flight at Close returned.
		runtime.AddCleanup(c, (*http.Transport).CloseIdleConnections, c.transport)
	}
	return c, nil
}

// plainHTTPToRemote reports whether apiURL, a URL that baseURL normalized, uses http to a host
// that is not loopback: neither localhost, in any case, nor an address of 127.0.0.0/8 or ::1,
// also as an IPv4-mapped IPv6 address. Requests to such a host carry the API key unencrypted
// across the network.
func plainHTTPToRemote(apiURL string) bool {
	u, err := url.Parse(apiURL)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return false
	}
	addr, err := netip.ParseAddr(host)
	return err != nil || !addr.IsLoopback()
}

// ownHTTPClient returns an http.Client for one Client, and its transport: a copy of
// http.DefaultTransport that keeps maxIdleConnsPerHost idle connections to each host. When
// http.DefaultTransport is not an *http.Transport, as when a test replaced it, the http.Client
// sends through it, and the transport is nil.
func ownHTTPClient() (*http.Client, *http.Transport) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{Transport: http.DefaultTransport}, nil
	}
	own := base.Clone()
	own.MaxIdleConnsPerHost = maxIdleConnsPerHost
	return &http.Client{Transport: own}, own
}

// Close closes the idle connections of the client's own *http.Client, and leaves those of an
// *http.Client that [WithHTTPClient] gave open. A call that starts after Close fails with an
// error that matches [ErrClosed] and sends no request; a call in flight when Close is called
// runs on, and returns its result or its error, but sends no other attempt: a retry it waits for
// returns the error of its last attempt. Close also ends the lookup of the scope that
// [Client.Scope] describes when one is in flight, and the calls that wait for it return an error
// that matches ErrClosed. Close always returns nil, also when it is called again, and is safe to
// call from many goroutines. A Client that is garbage collected without Close has the idle
// connections of its own *http.Client closed too.
func (c *Client) Close() error {
	if c == nil || !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	if c.scope != nil {
		c.scope.Stop()
	}
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
	return nil
}

// checkOpen returns ErrClosed once Close was called, and else nil. A call checks it before it
// sends a request.
func (c *Client) checkOpen() error {
	if c.closed.Load() {
		return ErrClosed
	}
	return nil
}

// String returns the client's settings on one line: its API URL, its PDP URL and its timeout,
// with a marker in place of its API key, and in place of the key in a URL when the key has at
// least 16 bytes or is a Permit API key; the Example of [New] shows one. A nil *Client is <nil>.
func (c *Client) String() string {
	if c == nil {
		return "<nil>"
	}
	return "permit.Client{APIKey: " + strconv.Quote(c.apiKey.String()) +
		", APIURL: " + strconv.Quote(redact.ScrubString(c.secrets, c.apiURL)) +
		", PDPURL: " + strconv.Quote(redact.ScrubString(c.secrets, c.pdpURL)) +
		", Timeout: " + c.timeout.String() + "}"
}

// GoString returns what [Client.String] returns, for the %#v verb.
func (c *Client) GoString() string { return c.String() }

// Format writes what [Client.String] returns as fmt writes a string with the same verb, flags and
// width, such as quoted for %q, and as it is for %#v, so that no verb prints the client's fields.
func (c *Client) Format(f fmt.State, verb rune) { formatText(f, verb, c.String()) }

// formatText writes text, what a value's String method returns, as fmt writes a string with verb
// and the flags and width of f, and as it is for %#v.
func formatText(f fmt.State, verb rune, text string) {
	if verb == 'v' && f.Flag('#') {
		_, _ = io.WriteString(f, text)
		return
	}
	_, _ = fmt.Fprintf(f, fmt.FormatString(f, verb), text)
}
