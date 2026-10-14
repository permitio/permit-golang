// Package logging is where the SDK logs, and the one package of the SDK that writes log lines.
//
// Each client has a [Logger], which writes to the *slog.Logger that permit.WithLogger gives
// through a handler that replaces the client's credentials with [redact.Marker] before a line
// reaches that logger's handler: the members of the client's [redact.Set], and what the
// stateless patterns of [redact.Scrub] find. Two clients that share one *slog.Logger each
// replace their own credentials, and the pattern of Permit API keys finds the key of the other.
//
// A line has a fixed message. Debug lines say what a request did: its route template, status,
// attempt, duration and error, never its headers or its body. Warn lines are warnings about the
// configuration, which permit.New writes once.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/permitio/permit-golang/v2/internal/redact"
)

// The messages of the SDK's log lines. Each is fixed: what changes goes in an attribute.
const (
	msgRequest   = "permit: request"
	msgPlainHTTP = "permit: the API key is sent over http:// to a host that is not loopback; " +
		"use https://"
)

// The keys of the attributes of the SDK's log lines.
const (
	keyRoute    = "route"
	keyStatus   = "status"
	keyAttempt  = "attempt"
	keyDuration = "duration"
	keyError    = "error"
	keyURL      = "url"
)

// Logger writes the log lines of one client. Build one with [New].
type Logger struct {
	// logger writes through the handler that replaces credentials.
	logger *slog.Logger

	// output is the handler that the lines reach once their credentials are replaced.
	output slog.Handler
}

// New returns the Logger of a client whose credentials secrets holds. It writes to the handler of
// logger through a handler that replaces those credentials. A nil logger, and one whose handler
// is nil, such as the zero slog.Logger, on which a call panics, give a Logger that discards every
// line.
func New(logger *slog.Logger, secrets *redact.Set) *Logger {
	output := slog.DiscardHandler
	if logger != nil && logger.Handler() != nil {
		output = logger.Handler()
	}
	return &Logger{logger: slog.New(&handler{next: output, secrets: secrets}), output: output}
}

// Output returns the handler that l writes its lines to, once their credentials are replaced:
// the handler of the *slog.Logger given to New, or [slog.DiscardHandler].
func (l *Logger) Output() slog.Handler { return l.output }

// Request is what the Debug line of one attempt of a request says.
type Request struct {
	// Route is the route template of the request, such as /v2/facts/{proj_id}/{env_id}/users.
	Route string

	// Status is the status of the reply, or 0 when no reply came.
	Status int

	// Attempt is the number of the attempt, from 1.
	Attempt int

	// Duration is how long the attempt took.
	Duration time.Duration

	// Err is why the attempt failed, or nil.
	Err error
}

// Request writes the Debug line of one attempt of a request, with the attributes route, status,
// attempt, duration and, when r.Err is not nil, error. It never writes a header or a body.
func (l *Logger) Request(ctx context.Context, r Request) {
	attrs := []slog.Attr{
		slog.String(keyRoute, r.Route),
		slog.Int(keyStatus, r.Status),
		slog.Int(keyAttempt, r.Attempt),
		slog.Duration(keyDuration, r.Duration),
	}
	if r.Err != nil {
		attrs = append(attrs, slog.Any(keyError, r.Err))
	}
	l.logger.LogAttrs(ctx, slog.LevelDebug, msgRequest, attrs...)
}

// PlainHTTP writes the Warn line that says that apiURL, the API URL of the client, uses http://
// to a host that is not loopback, with the attribute url.
func (l *Logger) PlainHTTP(apiURL string) {
	l.logger.LogAttrs(context.Background(), slog.LevelWarn, msgPlainHTTP,
		slog.String(keyURL, apiURL))
}

// handler is the slog.Handler that replaces the credentials of a line and passes it to next.
type handler struct {
	next    slog.Handler
	secrets *redact.Set
}

// Enabled reports whether next handles records at level.
func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle passes a copy of record to next, with the credentials of its message and attributes
// replaced.
func (h *handler) Handle(ctx context.Context, record slog.Record) error {
	scrubbed := slog.NewRecord(record.Time, record.Level, h.text(record.Message), record.PC)
	record.Attrs(func(a slog.Attr) bool {
		scrubbed.AddAttrs(h.attr(a))
		return true
	})
	return h.next.Handle(ctx, scrubbed)
}

// WithAttrs returns a handler that passes its lines to next with attrs, their credentials
// replaced.
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &handler{next: h.next.WithAttrs(h.attrs(attrs)), secrets: h.secrets}
}

// WithGroup returns a handler that passes its lines to next in the group name, its credentials
// replaced, or h when name is empty.
func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &handler{next: h.next.WithGroup(h.text(name)), secrets: h.secrets}
}

// text returns s with its credentials replaced.
func (h *handler) text(s string) string { return redact.ScrubString(h.secrets, s) }

// attrs returns a copy of attrs with the credentials of each replaced.
func (h *handler) attrs(attrs []slog.Attr) []slog.Attr {
	scrubbed := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		scrubbed[i] = h.attr(a)
	}
	return scrubbed
}

// attr returns a with the credentials of its key and value replaced. The empty Attr, which
// handlers leave out, stays empty.
func (h *handler) attr(a slog.Attr) slog.Attr {
	if a.Equal(slog.Attr{}) {
		return a
	}
	return slog.Attr{Key: h.text(a.Key), Value: h.value(a.Value)}
}

// value returns v, resolved when it is a slog.LogValuer, with its credentials replaced: in a
// string, in each attribute of a group, at any depth, and in any other value, which becomes its
// text, an error as [scrubbedError]. A value without text, such as a number, is kept.
func (h *handler) value(v slog.Value) slog.Value {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.StringValue(h.text(v.String()))
	case slog.KindGroup:
		return slog.GroupValue(h.attrs(v.Group())...)
	case slog.KindAny:
		switch value := v.Any().(type) {
		case nil:
			return v
		case error:
			return slog.AnyValue(scrubbedError{text: h.text(fmt.Sprint(value))})
		case []byte:
			return slog.StringValue(h.text(string(value)))
		default:
			return slog.StringValue(h.text(fmt.Sprintf("%+v", value)))
		}
	default:
		return v
	}
}

// scrubbedError is an error of a log line as the handler of the line gets it: the text of the
// error with its credentials replaced. It keeps nothing else of the error, no cause and no other
// text, so a handler that unwraps an error, or prints it with %+v, finds that text only.
type scrubbedError struct {
	text string
}

// Error returns the text of the error, with its credentials replaced.
func (e scrubbedError) Error() string { return e.text }
