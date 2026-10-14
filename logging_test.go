package permit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/logging"
	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
)

// remoteHTTP is an API URL over plain http to a host that is not loopback. New sends no request,
// so no test reaches it.
const remoteHTTP = "http://api.example.com"

// logHandlers are the handlers of slog that the logging tests write through, at Debug.
var logHandlers = map[string]func(w io.Writer) slog.Handler{
	"JSON": func(w io.Writer) slog.Handler {
		return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	},
	"text": func(w io.Writer) slog.Handler {
		return slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	},
}

// jsonLines decodes what slog's JSON handler wrote to out: one object for each line.
func jsonLines(t *testing.T, out []byte) []map[string]any {
	t.Helper()
	var decoded []map[string]any
	for line := range bytes.Lines(out) {
		var object map[string]any
		if err := json.Unmarshal(line, &object); err != nil {
			t.Fatalf("the JSON handler wrote %q: %v", line, err)
		}
		decoded = append(decoded, object)
	}
	return decoded
}

// TestClientsSharingALoggerKeepKeysOut builds two clients that share one logger, each with its
// key in its API URL, so that New warns with the URL, and logs a request line for each with the
// keys of both clients, and a credential that only the client's own Set holds, in the route and
// the error. No line may hold a key or the credential; the sentinel fails when the output has
// fewer markers than the places the test planted them in.
func TestClientsSharingALoggerKeepKeysOut(t *testing.T) {
	for name, handler := range logHandlers {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			var out bytes.Buffer
			logger := slog.New(handler(&out))
			keys := []string{canary.Key("sharedloggera"), canary.Key("sharedloggerb")}
			opaque := []string{"opaquecccccccc", "opaquedddddddd"}
			for i, key := range keys {
				c := mustNew(t, WithAPIKey(key), WithAPIURL(remoteHTTP+"/"+key),
					WithPDPURL(codeURL), WithLogger(logger))
				// A credential that a call returned joins the client's Set after New.
				c.secrets.Add(opaque[i])
				c.logger.Request(t.Context(), logging.Request{
					Route:    "/v2/facts/" + key + "/" + opaque[i],
					Status:   401,
					Attempt:  1,
					Duration: time.Millisecond,
					Err:      fmt.Errorf("refused %s, %s and %s", key, keys[1-i], opaque[i]),
				})
			}
			scanner := canary.NewScanner(append(keys, opaque...)...)
			_, _ = scanner.Write(out.Bytes())
			scanner.Check(t)
			// Each client: one marker in the warning's URL, two in the route, three in the error.
			if got, want := strings.Count(out.String(), redact.Marker), 2*6; got < want {
				t.Errorf("sentinel: the output has %d markers, want at least %d, so not every "+
					"planted credential was replaced:\n%s", got, want, out.String())
			}
		})
	}
}

// TestNewWarnsAboutPlainHTTP checks that New warns once when the API URL uses http:// to a host
// that is not loopback, whether an option or the environment gives it, and never for a loopback
// host, for https:// or for a PDP URL.
func TestNewWarnsAboutPlainHTTP(t *testing.T) {
	for _, tc := range []struct {
		name string
		// apiURL is the API URL from an option; empty to leave the option out.
		apiURL string
		// envAPIURL is the value of PERMIT_API_URL; empty to leave it unset.
		envAPIURL string
		pdpURL    string
		warn      bool
	}{
		{name: "a remote host", apiURL: remoteHTTP, warn: true},
		{
			name: "a remote host with a port and a path", apiURL: "HTTP://API.example.com:8080/x/",
			warn: true,
		},
		{name: "a private address", apiURL: "http://10.0.0.5:8000", warn: true},
		{name: "an IPv6 address", apiURL: "http://[2001:db8::1]:7000", warn: true},
		{name: "a link-local address with a zone", apiURL: "http://[fe80::1%25en0]", warn: true},
		{
			name: "a name that starts with localhost", apiURL: "http://localhost.example.com",
			warn: true,
		},
		{name: "a remote host from the environment", envAPIURL: remoteHTTP, warn: true},
		{name: "127.0.0.1", apiURL: "http://127.0.0.1"},
		{name: "another address of 127.0.0.0/8", apiURL: "http://127.31.0.9:9000"},
		{name: "localhost", apiURL: "http://localhost"},
		{name: "localhost in upper case with a port", apiURL: "http://LOCALHOST:7000"},
		{name: "::1", apiURL: "http://[::1]"},
		{name: "::1 with a port", apiURL: "http://[::1]:7766/permit"},
		{name: "127.0.0.1 mapped to IPv6", apiURL: "http://[::ffff:127.0.0.1]"},
		{name: "https to a remote host", apiURL: "https://api.example.com"},
		{name: "the default API URL"},
		{
			name: "a remote PDP over http", apiURL: "https://api.example.com",
			pdpURL: "http://pdp.example.com:7766",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			if tc.envAPIURL != "" {
				t.Setenv(envAPIURL, tc.envAPIURL)
			}
			key := canary.Key("plainhttp")
			var out bytes.Buffer
			opts := []Option{
				WithAPIKey(key), WithPDPURL(codeURL),
				WithLogger(slog.New(logHandlers["JSON"](&out))),
			}
			if tc.apiURL != "" {
				opts = append(opts, WithAPIURL(tc.apiURL))
			}
			if tc.pdpURL != "" {
				opts = append(opts, WithPDPURL(tc.pdpURL))
			}
			c := mustNew(t, opts...)
			got := jsonLines(t, out.Bytes())
			if !tc.warn {
				if len(got) != 0 {
					t.Errorf("New logged %d lines, want none: %v", len(got), got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("New logged %d lines, want 1 warning: %v", len(got), got)
			}
			if got[0]["level"] != "WARN" || got[0]["url"] != c.apiURL ||
				!strings.HasPrefix(fmt.Sprint(got[0]["msg"]), "permit: ") {
				t.Errorf("New logged %v, want a warning whose message starts with permit: and "+
					"whose url is %s", got[0], c.apiURL)
			}
		})
	}
}

// TestNewLogsNothingAtInfoOrAbove checks that New logs no line at Info or above for a
// configuration over https, the default API URL.
func TestNewLogsNothingAtInfoOrAbove(t *testing.T) {
	clearEnv(t)
	var out bytes.Buffer
	mustNew(t, WithAPIKey(canary.Key("quiet")), WithPDPURL(codeURL),
		WithLogger(slog.New(logHandlers["JSON"](&out))))
	for _, line := range jsonLines(t, out.Bytes()) {
		if line["level"] != "DEBUG" {
			t.Errorf("New logged %v", line)
		}
	}
}

// TestWithLoggerNilOrZeroDiscards checks that a nil *slog.Logger, and the zero slog.Logger, on
// which a call panics, make a client that logs nothing, without a panic.
func TestWithLoggerNilOrZeroDiscards(t *testing.T) {
	for name, logger := range map[string]*slog.Logger{"nil": nil, "zero": {}} {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("a client with a %s *slog.Logger panicked: %v", name, r)
				}
			}()
			c := mustNew(t, WithAPIKey(canary.Key("discard")), WithAPIURL(remoteHTTP),
				WithPDPURL(codeURL), WithLogger(logger))
			c.logger.Request(t.Context(), logging.Request{Route: "/v2/api-key/scope"})
			if got := c.logger.Output(); got != slog.DiscardHandler {
				t.Errorf("the client logs to %v, want slog.DiscardHandler", got)
			}
		})
	}
}
