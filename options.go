package permit

import (
	"cmp"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/scope"
	"github.com/permitio/permit-golang/v2/internal/transport"
	"github.com/permitio/permit-golang/v2/internal/validate"
)

// The environment variables that [New] reads for a setting that no option gives. A variable
// that is set to the empty string counts as unset.
const (
	envAPIKey = "PERMIT_API_KEY" //nolint:gosec // G101: the name of a variable, not a credential
	envAPIURL = "PERMIT_API_URL"
	envPDPURL = "PERMIT_PDP_URL"
)

// The defaults of the settings that neither an option nor an environment variable gives.
const (
	defaultAPIURL     = "https://api.permit.io"
	defaultPDPURL     = "http://localhost:7766"
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
	defaultMaxBackoff = 5 * time.Second
)

// Option configures a [Client]: pass options to [New]. Build an Option with a With function,
// such as [WithAPIKey]; the zero Option is not valid, and New rejects it. When options set the
// same setting, the last one wins.
//
// An Option prints as the call that builds it, such as permit.WithTimeout(10s), with a marker in
// place of the API key: [Option.String], [Option.GoString], [Option.Format] with every verb and
// [Option.LogValue] all give that text.
type Option struct {
	// name is the name of the With function that built the option.
	name string

	// shown is the argument of the With function, as the option prints it.
	shown string

	// apply sets the option's setting in s, or returns the error of an argument that New
	// rejects.
	apply func(s *settings) error
}

// settings holds what the options set. A field that no option set holds its zero value.
type settings struct {
	apiKey     redact.Secret
	apiURL     string
	pdpURL     string
	httpClient *http.Client
	timeout    time.Duration
	logger     *slog.Logger

	// retry is the retry policy, with the default MaxBackoff in place of 0, or nil when no option
	// gave one.
	retry *transport.Policy

	// scope is the scope that WithScope gave, or nil when the client looks up the scope of its
	// API key.
	scope *scope.Scope
}

// WithAPIKey sets the Permit API key that the client sends with each request. Without it, [New]
// reads the key from the environment variable PERMIT_API_KEY, and fails without a key. An empty
// key is an error that matches [ErrInvalidConfig]: leave the option out to read the variable.
func WithAPIKey(apiKey string) Option {
	shown := strconv.Quote(apiKey)
	if apiKey != "" {
		shown = redact.Marker
	}
	return Option{name: "WithAPIKey", shown: shown, apply: func(s *settings) error {
		if apiKey == "" {
			return &configError{setting: "WithAPIKey", rule: validate.APIKeyEmpty}
		}
		s.apiKey = redact.NewSecret(apiKey)
		return nil
	}}
}

// WithAPIURL sets the URL of the Permit API, such as https://api.permit.io, the default.
// Without it, [New] reads the URL from the environment variable PERMIT_API_URL, and uses the
// default when the variable is unset or empty.
//
// The URL must use http or https, in any case, and have a host. It must not have a user name or
// a password, a query or a fragment, and it must not be empty. A path, such as /permit in
// https://proxy.example.com/permit, is kept and starts every route; one slash at the end is
// removed. New returns an error that matches [ErrInvalidConfig] for a URL that breaks these
// rules.
func WithAPIURL(apiURL string) Option {
	return urlOption("WithAPIURL", apiURL, func(s *settings, normalized string) {
		s.apiURL = normalized
	})
}

// WithPDPURL sets the URL of the policy decision point (PDP) that the client asks for
// decisions, such as http://localhost:7766, the default: a PDP container that runs next to the
// application. Without it, [New] reads the URL from the environment variable PERMIT_PDP_URL, and
// uses the default when the variable is unset or empty. The URL follows the rules of
// [WithAPIURL].
func WithPDPURL(pdpURL string) Option {
	return urlOption("WithPDPURL", pdpURL, func(s *settings, normalized string) {
		s.pdpURL = normalized
	})
}

// urlOption returns the option named name that sets a URL with set, or that New rejects when
// raw breaks a rule of a base URL. The option prints the URL that it sets, quoted and with the
// Permit API keys in it replaced, and a marker in place of a URL that New rejects, as such a URL
// can hold a credential in a part that New does not accept.
func urlOption(name, raw string, set func(s *settings, normalized string)) Option {
	normalized, err := baseURL(name, raw)
	shown := strconv.Quote(redact.ScrubString(nil, normalized))
	if err != nil && raw != "" {
		shown = redact.Marker
	}
	return Option{name: name, shown: shown, apply: func(s *settings) error {
		if err != nil {
			return err
		}
		set(s, normalized)
		return nil
	}}
}

// WithHTTPClient sets the *http.Client that the client sends its requests with. The client uses
// it as it is: it changes none of its fields, such as Timeout or Transport, and
// [Client.Close] leaves its connections open; close them yourself when you are done with it.
// A nil *http.Client is an error that matches [ErrInvalidConfig].
//
// Without the option, each Client has an *http.Client of its own, whose transport is a copy of
// [http.DefaultTransport] as the Client was built: it uses the proxy that the environment
// variables HTTPS_PROXY, HTTP_PROXY and NO_PROXY name, the system's TLS settings, and keeps up to
// 64 idle connections to each host. Close closes those connections. When http.DefaultTransport
// is not an *http.Transport, as when a test replaced it, the client sends its requests through
// it, and Close leaves it alone.
func WithHTTPClient(httpClient *http.Client) Option {
	shown := "nil"
	if httpClient != nil {
		shown = fmt.Sprintf("%p", httpClient)
	}
	return Option{name: "WithHTTPClient", shown: shown, apply: func(s *settings) error {
		if httpClient == nil {
			return &configError{setting: "WithHTTPClient", rule: validate.HTTPClientNil}
		}
		s.httpClient = httpClient
		return nil
	}}
}

// WithTimeout sets the longest that a call of the client may take, from its start to its
// result, across all the requests it sends; the default is 30 seconds. A timeout of zero or less
// is an error that matches [ErrInvalidConfig]. When the context of a call has a deadline, the
// deadline bounds the call instead of the timeout. A call that runs out of time returns an error
// that matches [context.DeadlineExceeded].
//
// The client keeps the timeout to itself: it does not change the Timeout of the *http.Client
// that [WithHTTPClient] gives.
func WithTimeout(timeout time.Duration) Option {
	return Option{name: "WithTimeout", shown: timeout.String(), apply: func(s *settings) error {
		if timeout <= 0 {
			return &configError{setting: "WithTimeout", rule: validate.TimeoutNotPositive}
		}
		s.timeout = timeout
		return nil
	}}
}

// RetryPolicy says how many times a call sends its request again after an attempt that failed,
// and how long it waits at most before it does; [WithRetry] takes it, and describes when a call
// sends its request again. The zero RetryPolicy sends each request once.
type RetryPolicy struct {
	// MaxRetries is how many times, at most, a call sends its request again after its first
	// attempt: 0 sends each request once. The default, without WithRetry, is 3.
	MaxRetries int

	// MaxBackoff is the longest wait before a retry when the reply has no Retry-After header: 0
	// is the default, 5 seconds.
	MaxBackoff time.Duration
}

// WithRetry sets how many times a call sends its request again after an attempt that failed,
// and how long it waits at most before it does. Without it, a call sends its request again up to
// 3 times, and waits up to 5 seconds before each retry when the reply has no Retry-After header.
// policy replaces both: WithRetry(permit.RetryPolicy{}) sends each request once. A MaxRetries or
// a MaxBackoff less than zero is an error that matches [ErrInvalidConfig].
//
// A call sends its request again:
//
//   - after a reply with status 429 Too Many Requests, to any operation, as the server did not
//     act on the request, except an operation whose documentation says that it is sent once;
//   - after a reply with status 500, 502, 503 or 504, or when the request got no complete reply,
//     such as when the connection failed, only to an operation that does the same when it is
//     sent twice, such as a GET, a PUT or a DELETE. A request that the server may have acted on
//     and that would repeat what it did, such as a POST that creates an object, is not sent
//     again after these replies; only a 429 makes the client send it again.
//
// Any other status whose reply arrived complete, such as 400, 401, 404 or 409, ends the call,
// as does a reply in 200-299. A reply whose body was cut short counts as no complete reply.
// A call in flight when [Client.Close] is called sends no other attempt.
//
// Before a retry, the call waits for what the reply's Retry-After header asks, in seconds or as
// an HTTP-date. Without the header, it waits for a random time from 0 to 200 milliseconds before
// the first retry, and from 0 to twice the limit of the wait before each later one, up to
// MaxBackoff. The waits count toward the call's timeout, [WithTimeout], or the deadline of its
// context: a call never waits past it, nor for a Retry-After of more than 60 seconds. It returns
// the error of its last attempt at once instead, which matches [ErrRateLimited] for a 429. A call
// whose context ends while it waits returns an error that matches the context's error, such as
// [context.Canceled]. The client writes the Debug line of each attempt, as [WithLogger]
// describes, with the attempt's number, from 1, in attempt.
func WithRetry(policy RetryPolicy) Option {
	shown := "permit.RetryPolicy{MaxRetries: " + strconv.Itoa(policy.MaxRetries) +
		", MaxBackoff: " + policy.MaxBackoff.String() + "}"
	return Option{name: "WithRetry", shown: shown, apply: func(s *settings) error {
		if policy.MaxRetries < 0 {
			return &configError{setting: "WithRetry", rule: validate.RetryMaxRetriesNegative}
		}
		if policy.MaxBackoff < 0 {
			return &configError{setting: "WithRetry", rule: validate.RetryMaxBackoffNegative}
		}
		s.retry = &transport.Policy{
			MaxRetries: policy.MaxRetries,
			MaxBackoff: cmp.Or(policy.MaxBackoff, defaultMaxBackoff),
		}
		return nil
	}}
}

// WithScope sets the project and the environment that the client's calls act on, each by its key
// or its ID, such as WithScope("my-project", "production"). The client then sends no request for
// the scope of its API key, and [Client.Scope] returns what the option gives. Without it, the
// client looks up the scope of its API key once, as Client.Scope describes: an
// organization-level API key has neither a project nor an environment, and a project-level key
// has no environment.
//
// A call that acts on an environment, such as one of the facts API, needs both, and one that acts
// on a project needs the project. A call that lacks one returns an error that matches
// [ErrInvalidConfig], names its operation and WithScope, and sends no request for the operation;
// without WithScope, the client may first look up the scope of its API key. Give an empty
// environment to a client that acts on a project only. New returns an error that matches
// ErrInvalidConfig for an environment without a project, an empty project, and a project or an
// environment that is . or .., which would change the route of every call.
func WithScope(project, environment string) Option {
	shown := strconv.Quote(redact.ScrubString(nil, project)) + ", " +
		strconv.Quote(redact.ScrubString(nil, environment))
	return Option{name: "WithScope", shown: shown, apply: func(s *settings) error {
		if err := checkScope(project, environment); err != nil {
			return err
		}
		s.scope = &scope.Scope{Project: project, Environment: environment}
		return nil
	}}
}

// checkScope returns the error of a scope that New rejects: an environment without a project, an
// empty project, and a project or an environment that is . or .., which would change the route
// of every call that fills it in.
func checkScope(project, environment string) error {
	if project == "" && environment != "" {
		return &configError{setting: "WithScope", rule: validate.ScopeEnvironmentWithoutProject}
	}
	if project == "" {
		return &configError{setting: "WithScope", rule: validate.ScopeProjectEmpty}
	}
	if isDotSegment(project) {
		return &configError{setting: "WithScope: project", rule: validate.PathSegmentDot}
	}
	if isDotSegment(environment) {
		return &configError{setting: "WithScope: environment", rule: validate.PathSegmentDot}
	}
	return nil
}

// isDotSegment reports whether s is . or .., which a server or a proxy may resolve as a step in
// a path.
func isDotSegment(s string) bool {
	return s == "." || s == ".."
}

// WithLogger sets the *slog.Logger that the client logs to. By default the client logs nothing,
// and so does a nil logger, or one whose handler is nil, such as the zero slog.Logger.
//
// Each line of the client has a fixed message that starts with "permit: ". The client writes:
//
//   - at [slog.LevelDebug], one line for each attempt of a request, with the attributes route,
//     the route template such as /v2/facts/{proj_id}/{env_id}/users, status, which is 0 when no
//     reply came, attempt, from 1, duration and, when the attempt failed, error; never a header
//     or a body. The line reports the HTTP attempt: a reply in 200-299 that the SDK cannot
//     decode is the error of the call, and not of the line, which has its status and no error;
//   - at [slog.LevelWarn], warnings about the configuration, each once, from [New]: when the
//     API URL uses http:// to a host that is not loopback (localhost, 127.0.0.0/8 or ::1), with
//     the attribute url. A PDP URL over http:// gets no warning, as a PDP often runs next to the
//     application.
//
// The handler of the logger decides which of them it keeps, by its level: a handler at
// slog.LevelInfo, the default of slog's handlers, keeps the warnings only.
//
// Before a line reaches the handler, the client replaces its credentials in it with a marker:
// its API key, the credentials its calls return, every Permit API key and the value of an
// Authorization header. It replaces them in the message, in the keys and string values of
// attributes, in groups at any depth and in what a [slog.LogValuer] returns. An error reaches the
// handler as an error that holds its text, with the credentials replaced, and nothing else: no
// cause to unwrap. A value of another type, such as a struct, reaches it as its text, as fmt
// prints it with %+v. The attributes that the application gave the logger itself, such as with
// [slog.Logger.With], are its own, and the client leaves them as they are.
//
// To tell the client's lines apart from the application's, give it a logger with an attribute
// of its own:
//
//	permit.WithLogger(logger.With("component", "permit"))
//
// An application that logs with zap can give the client a logger through zapslog, the package
// go.uber.org/zap/exp/zapslog of the module go.uber.org/zap/exp, v0.3.0, an experimental
// module that needs go.uber.org/zap v1.26.0 or later:
//
//	zapLogger, err := zap.NewProduction()
//	if err != nil {
//		return err
//	}
//	client, err := permit.New(
//		permit.WithLogger(slog.New(zapslog.NewHandler(zapLogger.Core()))),
//	)
//
// zapslog adds a stack trace to each record at slog.LevelError and above by default; its option
// zapslog.AddStacktraceAt changes the level.
func WithLogger(logger *slog.Logger) Option {
	shown := "nil"
	if logger != nil {
		shown = fmt.Sprintf("%p", logger)
	}
	return Option{name: "WithLogger", shown: shown, apply: func(s *settings) error {
		s.logger = logger
		return nil
	}}
}

// String returns the call that builds the option, such as permit.WithTimeout(10s), with a marker
// in place of the API key, of a URL that New rejects, which can hold a credential, and of a
// Permit API key in a URL. The zero Option is permit.Option{}.
func (o Option) String() string {
	if o.apply == nil {
		return "permit.Option{}"
	}
	return "permit." + o.name + "(" + o.shown + ")"
}

// GoString returns what [Option.String] returns, for the %#v verb.
func (o Option) GoString() string { return o.String() }

// Format writes what [Option.String] returns as fmt writes a string with the same verb, flags and
// width, such as quoted for %q, and as it is for %#v, so that no verb prints the option's fields.
func (o Option) Format(f fmt.State, verb rune) { formatText(f, verb, o.String()) }

// LogValue returns what [Option.String] returns as a string value, so that a logger never
// writes the API key.
func (o Option) LogValue() slog.Value { return slog.StringValue(o.String()) }

// LogValue returns what [Client.String] returns as a string value, so that a logger never
// writes the API key. It is declared here, as options.go is the one file of the package that
// may import log/slog.
func (c *Client) LogValue() slog.Value { return slog.StringValue(c.String()) }

// resolve fills in each setting that no option gave: from its environment variable when the
// variable is set and not empty, and else from its default. It returns the error of a missing
// API key, or of a URL in a variable that New rejects.
func (s *settings) resolve() error {
	if s.apiKey.Value() == "" {
		apiKey := os.Getenv(envAPIKey)
		if apiKey == "" {
			return &configError{rule: validate.APIKeyMissing}
		}
		s.apiKey = redact.NewSecret(apiKey)
	}
	var err error
	if s.apiURL, err = resolveURL(s.apiURL, envAPIURL, defaultAPIURL); err != nil {
		return err
	}
	if s.pdpURL, err = resolveURL(s.pdpURL, envPDPURL, defaultPDPURL); err != nil {
		return err
	}
	if s.timeout == 0 {
		s.timeout = defaultTimeout
	}
	if s.retry == nil {
		s.retry = &transport.Policy{MaxRetries: defaultMaxRetries, MaxBackoff: defaultMaxBackoff}
	}
	return nil
}

// resolveURL returns fromOption, the URL that an option gave, or else the URL in the environment
// variable env, normalized, or else fallback.
func resolveURL(fromOption, env, fallback string) (string, error) {
	if fromOption != "" {
		return fromOption, nil
	}
	if raw := os.Getenv(env); raw != "" {
		return baseURL(env, raw)
	}
	return fallback, nil
}

// baseURL returns raw, the URL of the Permit API or of a PDP that setting gives, normalized: the
// scheme in lower case and one slash at the end removed, so that a route can follow it. It
// returns a *configError for setting when raw is empty or does not parse, when its scheme is not
// http or https, when it has no host, and when it has a user name or a password, a query or a
// fragment. The error never holds raw, which can hold a credential.
func baseURL(setting, raw string) (string, error) {
	broken := func(rule validate.Rule) (string, error) {
		return "", &configError{setting: setting, rule: rule}
	}
	if raw == "" {
		return broken(validate.URLEmpty)
	}
	// The checks are if statements, not the cases of a switch, so that coverage, and with it
	// the mutation tests, reach each condition.
	u, err := url.Parse(raw)
	if err != nil {
		// The parser's error quotes raw, so the rule's message stands in for it.
		return broken(validate.URLParse)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return broken(validate.URLScheme)
	}
	if u.Hostname() == "" {
		return broken(validate.URLHost)
	}
	if u.User != nil {
		return broken(validate.URLUserinfo)
	}
	if u.RawQuery != "" || u.ForceQuery {
		return broken(validate.URLQuery)
	}
	if strings.Contains(raw, "#") {
		return broken(validate.URLFragment)
	}
	// url.URL.String escapes the host again, so an IPv6 zone (%25 in the URL) survives.
	base := url.URL{
		Scheme:  u.Scheme,
		Host:    u.Host,
		Path:    strings.TrimSuffix(u.Path, "/"),
		RawPath: strings.TrimSuffix(u.RawPath, "/"),
	}
	return base.String(), nil
}

// configError is the error of a configuration that breaks a validation rule. It matches
// [ErrInvalidConfig].
type configError struct {
	// setting is the option or the environment variable that gave what breaks the rule, such as
	// WithAPIURL or PERMIT_API_URL; a part of an option, such as WithScope: project or
	// WithScope: environment; the operation whose call needs the scope, for the rules of the
	// scope of a call; or empty for a rule of no one setting.
	setting string

	rule validate.Rule
}

// Error returns the text of ErrInvalidConfig, the setting and the rule's message.
func (e *configError) Error() string {
	if e.setting == "" {
		return ErrInvalidConfig.Error() + ": " + e.rule.Message
	}
	return ErrInvalidConfig.Error() + ": " + e.setting + ": " + e.rule.Message
}

// Unwrap returns ErrInvalidConfig, so that errors.Is matches it.
func (e *configError) Unwrap() error { return ErrInvalidConfig }
