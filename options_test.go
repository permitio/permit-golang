package permit

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/surface"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/validate"
)

// The URLs that the tests give in code and in the environment. Nothing listens on port 1, and
// New sends no request.
const (
	codeURL = "http://127.0.0.1:1/code"
	envURL  = "http://127.0.0.1:1/env"
)

// configEnv are the environment variables that New reads.
var configEnv = []string{envAPIKey, envAPIURL, envPDPURL}

// clearEnv unsets the environment variables that New reads until the test ends, so that the
// environment the tests run in does not change what New builds.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range configEnv {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

// mustNew returns New(opts...), and fails the test when New returns an error.
func mustNew(t *testing.T, opts ...Option) *Client {
	t.Helper()
	c, err := New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// localOptions are options that point both URLs at a local address and give a key, so that a
// test that builds a client can add the option it tests.
func localOptions() []Option {
	return []Option{
		WithAPIKey(canary.Key("local")),
		WithAPIURL(codeURL),
		WithPDPURL(codeURL),
	}
}

// optionEffect is what an option changes in the client New builds.
type optionEffect struct {
	// option is the option, with a value that differs from the default of its setting.
	option Option

	// setting returns the value of the option's setting in a client.
	setting func(c *Client) any

	// want is the value of the setting in a client built with option.
	want any

	// unset are options of the same With function that leave the setting as it is without the
	// option, such as a nil logger.
	unset []Option
}

// effectHTTPClient is the *http.Client of the effect of WithHTTPClient.
var effectHTTPClient = &http.Client{}

// effectLogHandler is the handler of the *slog.Logger of the effect of WithLogger.
var effectLogHandler = slog.NewTextHandler(new(strings.Builder), nil)

// optionEffects holds the effect of each exported option, by the name of its With function.
// TestOptionsHaveEffect fails when an exported With function has no entry here, or an entry names
// no such function: add the effect of a new option here.
var optionEffects = map[string]optionEffect{
	"WithAPIKey": {
		option:  WithAPIKey(canary.Key("effect")),
		setting: func(c *Client) any { return c.apiKey.Value() },
		want:    canary.Key("effect"),
	},
	"WithAPIURL": {
		option:  WithAPIURL("http://127.0.0.1:1/api/"),
		setting: func(c *Client) any { return c.apiURL },
		want:    "http://127.0.0.1:1/api",
	},
	"WithPDPURL": {
		option:  WithPDPURL("http://127.0.0.1:1/pdp/"),
		setting: func(c *Client) any { return c.pdpURL },
		want:    "http://127.0.0.1:1/pdp",
	},
	"WithHTTPClient": {
		option:  WithHTTPClient(effectHTTPClient),
		setting: func(c *Client) any { return c.httpClient },
		want:    effectHTTPClient,
	},
	"WithLogger": {
		option:  WithLogger(slog.New(effectLogHandler)),
		setting: func(c *Client) any { return c.logger.Output() },
		want:    effectLogHandler,
		// A nil logger, and the zero slog.Logger, whose handler is nil, discard, as the
		// default does.
		unset: []Option{WithLogger(nil), WithLogger(&slog.Logger{})},
	},
	"WithRetry": {
		option:  WithRetry(RetryPolicy{MaxRetries: 5, MaxBackoff: 2 * time.Second}),
		setting: retryPolicy,
		want:    RetryPolicy{MaxRetries: 5, MaxBackoff: 2 * time.Second},
		// The default policy, also with the MaxBackoff of 0 that stands for the default.
		unset: []Option{
			WithRetry(RetryPolicy{MaxRetries: 3, MaxBackoff: 5 * time.Second}),
			WithRetry(RetryPolicy{MaxRetries: 3}),
		},
	},
	"WithTimeout": {
		option:  WithTimeout(7 * time.Second),
		setting: func(c *Client) any { return c.timeout },
		want:    7 * time.Second,
	},
	"WithScope": {
		option:  WithScope("effect-project", "effect-environment"),
		setting: knownScope,
		want:    Scope{Project: "effect-project", Environment: "effect-environment"},
	},
}

// knownScope returns the scope that c has without a lookup, or the error of the lookup that it
// would need. It calls Scope with a context that has ended, so that it sends no request.
func knownScope(c *Client) any {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, err := c.Scope(ctx)
	if err != nil {
		return err.Error()
	}
	return s
}

// exportedOptions returns the names of the exported functions of the package whose names start
// with With, sorted: its options.
func exportedOptions(t *testing.T) []string {
	t.Helper()
	p, err := surface.Read(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, item := range p.Items {
		if item.Kind == surface.Func && strings.HasPrefix(item.Name, "With") {
			names = append(names, item.Name)
		}
	}
	slices.Sort(names)
	return names
}

// TestOptionsHaveEffect checks that every exported option changes the client New builds. Its
// count sentinel fails when the options and optionEffects differ.
func TestOptionsHaveEffect(t *testing.T) {
	options := exportedOptions(t)
	if len(options) == 0 {
		t.Fatal("the package exports no With function; the options were not read")
	}
	for _, name := range options {
		effect, ok := optionEffects[name]
		if !ok {
			t.Errorf("%s has no entry in optionEffects; add what it changes", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			// Without options, the key and the URLs come from the environment, local URLs so
			// that no client of this test can reach the Permit cloud, and the rest are defaults.
			clearEnv(t)
			t.Setenv(envAPIKey, canary.Key("effectenv"))
			t.Setenv(envAPIURL, envURL)
			t.Setenv(envPDPURL, envURL)
			without, with := mustNew(t), mustNew(t, effect.option)
			if got := effect.setting(with); got != effect.want {
				t.Errorf("with %v, the setting is %v, want %v", effect.option, got, effect.want)
			}
			if got := effect.setting(without); got == effect.want {
				t.Errorf("without %v, the setting is %v already, so the test shows no effect; "+
					"give the option another value", effect.option, got)
			}
			for _, option := range effect.unset {
				got, want := effect.setting(mustNew(t, option)), effect.setting(without)
				if got != want {
					t.Errorf("with %v, the setting is %v, want %v as without the option",
						option, got, want)
				}
			}
		})
	}
	for name := range optionEffects {
		if !slices.Contains(options, name) {
			t.Errorf("optionEffects has %s, which the package does not export", name)
		}
	}
	if len(options) != len(optionEffects) {
		t.Errorf("count sentinel: the package exports %d options, %q, and optionEffects has %d",
			len(options), options, len(optionEffects))
	}
}

func TestAPIKeyPrecedence(t *testing.T) {
	codeKey, envKey := canary.Key("precedencecode"), canary.Key("precedenceenv")
	for _, tc := range []struct {
		name string
		// env is the value of PERMIT_API_KEY, unset when nil.
		env  *string
		opts []Option
		// want is the key of the client, or "" when New must fail for a missing key.
		want string
	}{
		{
			name: "code over the environment", env: &envKey, opts: []Option{WithAPIKey(codeKey)},
			want: codeKey,
		},
		{
			name: "code over an empty variable", env: new(""), opts: []Option{WithAPIKey(codeKey)},
			want: codeKey,
		},
		{name: "code alone", opts: []Option{WithAPIKey(codeKey)}, want: codeKey},
		{
			name: "the last option", opts: []Option{WithAPIKey(envKey), WithAPIKey(codeKey)},
			want: codeKey,
		},
		{name: "the environment", env: &envKey, want: envKey},
		{name: "an empty variable is unset", env: new("")},
		{name: "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			if tc.env != nil {
				t.Setenv(envAPIKey, *tc.env)
			}
			c, err := New(append(tc.opts, WithAPIURL(codeURL), WithPDPURL(codeURL))...)
			if tc.want == "" {
				checkConfigError(t, c, err, "", validate.APIKeyMissing)
				return
			}
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got := c.apiKey.Value(); got != tc.want {
				t.Errorf("the key is %q, want %q", got, tc.want)
			}
			if !c.secrets.Contains(tc.want) {
				t.Error("the client's redact.Set does not hold the key")
			}
		})
	}
}

func TestURLPrecedence(t *testing.T) {
	for _, setting := range []struct {
		env    string
		option func(string) Option
		// url and other return the URL of the setting and the other URL of a client.
		url, other func(*Client) string
		// def and otherDef are their defaults.
		def, otherDef string
	}{
		{
			env: envAPIURL, option: WithAPIURL,
			url: func(c *Client) string { return c.apiURL }, def: defaultAPIURL,
			other: func(c *Client) string { return c.pdpURL }, otherDef: defaultPDPURL,
		},
		{
			env: envPDPURL, option: WithPDPURL,
			url: func(c *Client) string { return c.pdpURL }, def: defaultPDPURL,
			other: func(c *Client) string { return c.apiURL }, otherDef: defaultAPIURL,
		},
	} {
		for _, tc := range []struct {
			name string
			// env is the value of the variable, unset when nil.
			env  *string
			code bool
			want string
		}{
			{name: "code over the environment", env: new(envURL), code: true, want: codeURL},
			{name: "code over an empty variable", env: new(""), code: true, want: codeURL},
			{name: "code over an invalid variable", env: new("ftp://"), code: true, want: codeURL},
			{name: "code alone", code: true, want: codeURL},
			{name: "the environment", env: new(envURL), want: envURL},
			{name: "an empty variable is unset", env: new(""), want: setting.def},
			{name: "the default", want: setting.def},
		} {
			t.Run(setting.env+"/"+tc.name, func(t *testing.T) {
				clearEnv(t)
				if tc.env != nil {
					t.Setenv(setting.env, *tc.env)
				}
				opts := []Option{WithAPIKey(canary.Key("precedence"))}
				if tc.code {
					opts = append(opts, setting.option(codeURL))
				}
				c := mustNew(t, opts...)
				if got := setting.url(c); got != tc.want {
					t.Errorf("the URL is %q, want %q", got, tc.want)
				}
				if got := setting.other(c); got != setting.otherDef {
					t.Errorf("the other URL is %q, want its default %q", got, setting.otherDef)
				}
			})
		}
	}
}

func TestDefaultTimeout(t *testing.T) {
	clearEnv(t)
	if c := mustNew(t, localOptions()...); c.timeout != 30*time.Second {
		t.Errorf("the default timeout is %v, want 30s", c.timeout)
	}
}

// retryPolicy returns the retry policy of c as a RetryPolicy.
func retryPolicy(c *Client) any {
	return RetryPolicy{MaxRetries: c.retry.MaxRetries, MaxBackoff: c.retry.MaxBackoff}
}

// TestRetryPolicies checks the retry policy of a client: up to 3 retries with waits of up to 5
// seconds by default; the policy that WithRetry gives, with 5 seconds for a MaxBackoff of 0; and
// a client's own random waits.
func TestRetryPolicies(t *testing.T) {
	clearEnv(t)
	for _, tc := range []struct {
		opts []Option
		want RetryPolicy
	}{
		{nil, RetryPolicy{MaxRetries: 3, MaxBackoff: 5 * time.Second}},
		{[]Option{WithRetry(RetryPolicy{})}, RetryPolicy{MaxBackoff: 5 * time.Second}},
		{
			[]Option{WithRetry(RetryPolicy{MaxRetries: 1, MaxBackoff: time.Nanosecond})},
			RetryPolicy{MaxRetries: 1, MaxBackoff: time.Nanosecond},
		},
		{
			[]Option{WithRetry(RetryPolicy{MaxRetries: 9}), WithRetry(RetryPolicy{MaxRetries: 2})},
			RetryPolicy{MaxRetries: 2, MaxBackoff: 5 * time.Second},
		},
	} {
		c := mustNew(t, append(localOptions(), tc.opts...)...)
		if got := retryPolicy(c); got != tc.want {
			t.Errorf("with %v, the retry policy is %+v, want %+v", tc.opts, got, tc.want)
		}
		if c.retry.Jitter != nil {
			t.Errorf("with %v, the client's waits are not random", tc.opts)
		}
	}
}

// TestURLNormalization checks the URLs that New accepts, from an option and from the
// environment, and how it writes them.
func TestURLNormalization(t *testing.T) {
	for raw, want := range map[string]string{
		"http://127.0.0.1:7766":           "http://127.0.0.1:7766",
		"http://127.0.0.1:7766/":          "http://127.0.0.1:7766",
		"https://127.0.0.1:7766/":         "https://127.0.0.1:7766",
		"HTTP://127.0.0.1:7766/":          "http://127.0.0.1:7766",
		"HtTpS://LocalHost:7766":          "https://LocalHost:7766",
		"http://127.0.0.1:7766/prefix":    "http://127.0.0.1:7766/prefix",
		"http://127.0.0.1:7766/prefix/":   "http://127.0.0.1:7766/prefix",
		"http://127.0.0.1:7766/a/b/":      "http://127.0.0.1:7766/a/b",
		"http://127.0.0.1:7766/a%2Fb/":    "http://127.0.0.1:7766/a%2Fb",
		"http://127.0.0.1:7766//":         "http://127.0.0.1:7766/",
		"http://[::1]:7766/":              "http://[::1]:7766",
		"http://localhost":                "http://localhost",
		"http://localhost:7766/v2/prefix": "http://localhost:7766/v2/prefix",
		"http://[fe80::1%25en0]:7766/":    "http://[fe80::1%25en0]:7766",
		"http://localhost/€/":             "http://localhost/%E2%82%AC",
	} {
		t.Run(raw, func(t *testing.T) {
			clearEnv(t)
			c := mustNew(t, WithAPIKey(canary.Key("normal")), WithAPIURL(raw), WithPDPURL(raw))
			if c.apiURL != want || c.pdpURL != want {
				t.Errorf("from the options, the API URL is %q and the PDP URL %q, want %q",
					c.apiURL, c.pdpURL, want)
			}
			if _, err := New(WithAPIKey(canary.Key("normal")), WithAPIURL(want),
				WithPDPURL(want)); err != nil {
				t.Errorf("New rejects its own result %q: %v", want, err)
			}
			t.Setenv(envAPIURL, raw)
			t.Setenv(envPDPURL, raw)
			c = mustNew(t, WithAPIKey(canary.Key("normal")))
			if c.apiURL != want || c.pdpURL != want {
				t.Errorf("from the environment, the API URL is %q and the PDP URL %q, want %q",
					c.apiURL, c.pdpURL, want)
			}
		})
	}
}

// checkConfigError checks that New returned no client and an error that matches
// ErrInvalidConfig, for the rule and the setting, the option or variable it names, which is
// empty for a rule of no one setting.
func checkConfigError(t *testing.T, c *Client, err error, setting string, rule validate.Rule) {
	t.Helper()
	if c != nil {
		t.Errorf("New returned a client, %v, with the error", c)
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("New returned %v, want an error that matches ErrInvalidConfig", err)
	}
	ce, ok := errors.AsType[*configError](err)
	if !ok {
		t.Fatalf("New returned %v, which is no *configError", err)
	}
	if ce.rule != rule || ce.setting != setting {
		t.Errorf("New broke the rule %s of %q, want %s of %q", ce.rule.ID, ce.setting, rule.ID,
			setting)
	}
	if !slices.Contains(validate.Rules(), rule) {
		t.Errorf("validate.Rules does not list the rule %s", rule.ID)
	}
	want := "permit: invalid configuration: " + rule.Message
	if setting != "" {
		want = "permit: invalid configuration: " + setting + ": " + rule.Message
	}
	if got := err.Error(); got != want {
		t.Errorf("the error is\n%s\nwant\n%s", got, want)
	}
}

// TestInvalidConfiguration checks each configuration that New rejects: it returns an error that
// matches ErrInvalidConfig, names the option or the variable and never holds the key, and no
// request reaches the servers that the valid settings point at.
func TestInvalidConfiguration(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	key := canary.Key("invalid")
	hostPort := strings.TrimPrefix(api.URL, "http://")
	scanner := canary.NewScanner(key, "userpart")
	for _, tc := range []struct {
		name    string
		env     map[string]string
		opts    []Option
		setting string
		rule    validate.Rule
	}{
		{name: "no key", rule: validate.APIKeyMissing},
		{
			name: "an empty key variable", env: map[string]string{envAPIKey: ""},
			rule: validate.APIKeyMissing,
		},
		{
			name: "an empty key from code", env: map[string]string{envAPIKey: key},
			opts: []Option{WithAPIKey("")}, setting: "WithAPIKey", rule: validate.APIKeyEmpty,
		},
		{
			name: "an empty API URL from code", opts: []Option{WithAPIURL("")},
			setting: "WithAPIURL", rule: validate.URLEmpty,
		},
		{
			name: "an empty PDP URL from code", opts: []Option{WithPDPURL("")},
			setting: "WithPDPURL", rule: validate.URLEmpty,
		},
		{
			name: "no scheme", opts: []Option{WithAPIURL("api.permit.io")},
			setting: "WithAPIURL", rule: validate.URLScheme,
		},
		{
			name: "no scheme before a port", opts: []Option{WithPDPURL(hostPort)},
			setting: "WithPDPURL", rule: validate.URLParse,
		},
		{
			name: "localhost and a port", opts: []Option{WithPDPURL("localhost:7766")},
			setting: "WithPDPURL", rule: validate.URLScheme,
		},
		{
			name: "4 characters", opts: []Option{WithAPIURL("http")},
			setting: "WithAPIURL", rule: validate.URLScheme,
		},
		{
			name: "another scheme", opts: []Option{WithAPIURL("ftp://" + hostPort)},
			setting: "WithAPIURL", rule: validate.URLScheme,
		},
		{
			name: "a scheme alone", opts: []Option{WithAPIURL("http://")},
			setting: "WithAPIURL", rule: validate.URLHost,
		},
		{
			name: "no host before a path", opts: []Option{WithAPIURL("http:///v2")},
			setting: "WithAPIURL", rule: validate.URLHost,
		},
		{
			name: "an opaque URL", opts: []Option{WithAPIURL("http:" + hostPort)},
			setting: "WithAPIURL", rule: validate.URLHost,
		},
		{
			name: "a port without a host", opts: []Option{WithAPIURL("http://:7766")},
			setting: "WithAPIURL", rule: validate.URLHost,
		},
		{
			name:    "the key as the user",
			opts:    []Option{WithAPIURL("http://" + key + "@" + hostPort)},
			setting: "WithAPIURL", rule: validate.URLUserinfo,
		},
		{
			name:    "the key as the password",
			opts:    []Option{WithPDPURL("http://userpart:" + key + "@" + hostPort)},
			setting: "WithPDPURL", rule: validate.URLUserinfo,
		},
		{
			name: "an empty user", opts: []Option{WithAPIURL("http://@" + hostPort)},
			setting: "WithAPIURL", rule: validate.URLUserinfo,
		},
		{
			name: "a query", opts: []Option{WithAPIURL(api.URL + "?token=" + key)},
			setting: "WithAPIURL", rule: validate.URLQuery,
		},
		{
			name: "an empty query", opts: []Option{WithAPIURL(api.URL + "?")},
			setting: "WithAPIURL", rule: validate.URLQuery,
		},
		{
			name: "a fragment", opts: []Option{WithPDPURL(pdp.URL + "#" + key)},
			setting: "WithPDPURL", rule: validate.URLFragment,
		},
		{
			name: "an empty fragment", opts: []Option{WithPDPURL(pdp.URL + "#")},
			setting: "WithPDPURL", rule: validate.URLFragment,
		},
		{
			name:    "a bad escape",
			opts:    []Option{WithAPIURL("http://" + key + "@" + hostPort + "/%zz")},
			setting: "WithAPIURL", rule: validate.URLParse,
		},
		{
			name: "a space", opts: []Option{WithAPIURL(" " + api.URL)},
			setting: "WithAPIURL", rule: validate.URLParse,
		},
		{
			name: "an unclosed IPv6 host", opts: []Option{WithAPIURL("http://[::1")},
			setting: "WithAPIURL", rule: validate.URLParse,
		},
		{
			name: "an API URL variable", env: map[string]string{envAPIURL: "api.permit.io"},
			setting: envAPIURL, rule: validate.URLScheme,
		},
		{
			name: "a PDP URL variable", env: map[string]string{envPDPURL: pdp.URL + "?x=" + key},
			setting: envPDPURL, rule: validate.URLQuery,
		},
		{
			name:    "a PDP URL variable with the key",
			env:     map[string]string{envPDPURL: "http://" + key + "@" + hostPort},
			setting: envPDPURL, rule: validate.URLUserinfo,
		},
		{
			name: "a negative timeout", opts: []Option{WithTimeout(-time.Second)},
			setting: "WithTimeout", rule: validate.TimeoutNotPositive,
		},
		{
			name: "a zero timeout", opts: []Option{WithTimeout(0)},
			setting: "WithTimeout", rule: validate.TimeoutNotPositive,
		},
		{
			name:    "a negative MaxRetries",
			opts:    []Option{WithRetry(RetryPolicy{MaxRetries: -1, MaxBackoff: time.Second})},
			setting: "WithRetry", rule: validate.RetryMaxRetriesNegative,
		},
		{
			name:    "a negative MaxBackoff",
			opts:    []Option{WithRetry(RetryPolicy{MaxRetries: 1, MaxBackoff: -1})},
			setting: "WithRetry", rule: validate.RetryMaxBackoffNegative,
		},
		{
			name:    "a negative MaxRetries and MaxBackoff",
			opts:    []Option{WithRetry(RetryPolicy{MaxRetries: -3, MaxBackoff: -time.Second})},
			setting: "WithRetry", rule: validate.RetryMaxRetriesNegative,
		},
		{
			name: "a nil http.Client", opts: []Option{WithHTTPClient(nil)},
			setting: "WithHTTPClient", rule: validate.HTTPClientNil,
		},
		{
			name: "an environment without a project", opts: []Option{WithScope("", "production")},
			setting: "WithScope", rule: validate.ScopeEnvironmentWithoutProject,
		},
		{
			name: "an empty scope", opts: []Option{WithScope("", "")},
			setting: "WithScope", rule: validate.ScopeProjectEmpty,
		},
		{
			name: "a project that is .", opts: []Option{WithScope(".", "production")},
			setting: "WithScope: project", rule: validate.PathSegmentDot,
		},
		{
			name: "a project that is ..", opts: []Option{WithScope("..", "")},
			setting: "WithScope: project", rule: validate.PathSegmentDot,
		},
		{
			name: "an environment that is ..", opts: []Option{WithScope("my-project", "..")},
			setting: "WithScope: environment", rule: validate.PathSegmentDot,
		},
		{
			name: "an environment that is .", opts: []Option{WithScope("my-project", ".")},
			setting: "WithScope: environment", rule: validate.PathSegmentDot,
		},
		// The three valid options come first, so the zero Option is the fifth.
		{
			name: "a zero Option", opts: []Option{WithTimeout(time.Second), {}},
			setting: "option 5 of New", rule: validate.OptionZero,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			for name, value := range tc.env {
				t.Setenv(name, value)
			}
			// The valid settings come first, so that each case's own option, which validates
			// what it is given, is what New rejects. Without an option, the key and URLs come
			// from tc.env.
			var opts []Option
			if _, ok := tc.env[envAPIKey]; !ok && tc.rule != validate.APIKeyMissing {
				opts = append(opts, WithAPIKey(key))
			}
			if _, ok := tc.env[envAPIURL]; !ok {
				opts = append(opts, WithAPIURL(api.URL))
			}
			if _, ok := tc.env[envPDPURL]; !ok {
				opts = append(opts, WithPDPURL(pdp.URL))
			}
			c, err := New(append(opts, tc.opts...)...)
			checkConfigError(t, c, err, tc.setting, tc.rule)
			_, _ = fmt.Fprintf(scanner, "%v %+v %#v", err, err, err)
		})
	}
	scanner.Check(t)
	api.AssertUntouched()
	pdp.AssertUntouched()
}

// printVerbs are the fmt verbs, with flags and widths, that the printing tests use.
var printVerbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%30v", "%-30s"}

// keyScanner returns a scanner that finds key, also in hexadecimal as %x and %X write it.
func keyScanner(key string) *canary.Scanner {
	hexKey := hex.EncodeToString([]byte(key))
	return canary.NewScanner(key, hexKey, strings.ToUpper(hexKey))
}

func TestOptionsPrintNoKey(t *testing.T) {
	key := canary.Key("printoption")
	mine := &http.Client{}
	logger := slog.New(slog.DiscardHandler)
	for _, tc := range []struct {
		option Option
		want   string
	}{
		{WithAPIKey(key), "permit.WithAPIKey([REDACTED])"},
		{WithAPIKey(""), `permit.WithAPIKey("")`},
		{WithAPIURL("http://127.0.0.1:1/x/"), `permit.WithAPIURL("http://127.0.0.1:1/x")`},
		{WithAPIURL("http://" + key + "@127.0.0.1:1"), "permit.WithAPIURL([REDACTED])"},
		{WithAPIURL("http://127.0.0.1:1?token=" + key), "permit.WithAPIURL([REDACTED])"},
		{WithAPIURL("api.permit.io"), "permit.WithAPIURL([REDACTED])"},
		{WithAPIURL(""), `permit.WithAPIURL("")`},
		{WithPDPURL("http://127.0.0.1:1/" + key), `permit.WithPDPURL("http://127.0.0.1:1/` +
			redact.Marker + `")`},
		{WithPDPURL("http://127.0.0.1:1#" + key), "permit.WithPDPURL([REDACTED])"},
		{WithHTTPClient(nil), "permit.WithHTTPClient(nil)"},
		{WithHTTPClient(mine), fmt.Sprintf("permit.WithHTTPClient(%p)", mine)},
		{WithLogger(nil), "permit.WithLogger(nil)"},
		{WithLogger(logger), fmt.Sprintf("permit.WithLogger(%p)", logger)},
		{WithTimeout(1500 * time.Millisecond), "permit.WithTimeout(1.5s)"},
		{WithTimeout(-time.Second), "permit.WithTimeout(-1s)"},
		{
			WithRetry(RetryPolicy{MaxRetries: 5, MaxBackoff: 1500 * time.Millisecond}),
			"permit.WithRetry(permit.RetryPolicy{MaxRetries: 5, MaxBackoff: 1.5s})",
		},
		{
			WithRetry(RetryPolicy{}),
			"permit.WithRetry(permit.RetryPolicy{MaxRetries: 0, MaxBackoff: 0s})",
		},
		{
			WithRetry(RetryPolicy{MaxRetries: -1, MaxBackoff: -time.Second}),
			"permit.WithRetry(permit.RetryPolicy{MaxRetries: -1, MaxBackoff: -1s})",
		},
		{WithScope("my-project", "production"), `permit.WithScope("my-project", "production")`},
		{WithScope("", "a\"b"), `permit.WithScope("", "a\"b")`},
		{WithScope(key, "env-"+key), `permit.WithScope("[REDACTED]", "env-[REDACTED]")`},
		{Option{}, "permit.Option{}"},
	} {
		if got := tc.option.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
		if got := tc.option.GoString(); got != tc.want {
			t.Errorf("GoString() = %q, want %q", got, tc.want)
		}
		if got := tc.option.LogValue().String(); got != tc.want {
			t.Errorf("LogValue() = %q, want %q", got, tc.want)
		}
		checkFormat(t, tc.option, tc.want)
	}
}

// checkFormat checks that fmt writes value, a Client or an Option, as text, what its String
// method returns, with each verb as fmt writes a string, and in compound values.
func checkFormat(t *testing.T, value any, text string) {
	t.Helper()
	for verb, want := range map[string]string{
		"%v":    text,
		"%+v":   text,
		"%#v":   text,
		"%s":    text,
		"%q":    fmt.Sprintf("%q", text),
		"%x":    fmt.Sprintf("%x", text),
		"%30v":  fmt.Sprintf("%30v", text),
		"%-30s": fmt.Sprintf("%-30s", text),
	} {
		if got := fmt.Sprintf(verb, value); got != want {
			t.Errorf("fmt.Sprintf(%q) = %q, want %q", verb, got, want)
		}
	}
	type holder struct {
		Exported any
		hidden   any
	}
	compound := fmt.Sprintf("%+v %v %#v", []any{value}, map[string]any{"v": value},
		holder{Exported: value, hidden: value})
	if strings.Count(compound, text) < 3 {
		t.Errorf("compound values print as %q, want %q in each", compound, text)
	}
}

func TestPrintingNeverShowsTheKey(t *testing.T) {
	clearEnv(t)
	key := canary.Key("printing")
	options := []Option{
		WithAPIKey(key),
		WithAPIURL("http://127.0.0.1:1/" + key + "/"),
		WithPDPURL(codeURL),
		WithHTTPClient(&http.Client{}),
		WithTimeout(time.Second),
	}
	c := mustNew(t, options...)
	options = append(options, WithPDPURL("http://"+key+"@127.0.0.1:1"),
		WithAPIURL("http://127.0.0.1:1?token="+key))
	scanner := keyScanner(key)
	values := []any{c, options}
	for _, o := range options {
		values = append(values, o)
	}
	for _, value := range values {
		for _, verb := range printVerbs {
			_, _ = fmt.Fprintf(scanner, verb+"\n", value)
		}
		_, _ = fmt.Fprintln(scanner, value)
	}
	scanner.Check(t)
}
