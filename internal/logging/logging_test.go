package logging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/slogtest"
	"time"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
)

// lines is an io.Writer that keeps each write as one line: slog's handlers, and recorder, write
// each record with one Write. It is safe for concurrent use.
type lines struct {
	mu   sync.Mutex
	list []string
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = append(l.list, string(p))
	return len(p), nil
}

// count returns the number of lines written so far.
func (l *lines) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.list)
}

// since returns the lines written after the first n.
func (l *lines) since(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.list[n:])
}

// recorder is a slog.Handler that keeps each record at level or above, with the attributes that
// WithAttrs gave it and the groups that WithGroup opened, and writes it to out as one line of
// text, its values resolved as a handler resolves them.
type recorder struct {
	out   io.Writer
	level slog.Level

	mu      *sync.Mutex
	records *[]slog.Record

	// attrs are the attributes that WithAttrs gave, each under the groups open at the time.
	attrs []slog.Attr

	// groups are the groups that WithGroup opened, the outermost first.
	groups []string
}

func newRecorder(out io.Writer, level slog.Level) *recorder {
	return &recorder{out: out, level: level, mu: &sync.Mutex{}, records: &[]slog.Record{}}
}

func (r *recorder) Enabled(_ context.Context, level slog.Level) bool { return level >= r.level }

func (r *recorder) Handle(_ context.Context, record slog.Record) error {
	var own []slog.Attr
	record.Attrs(func(a slog.Attr) bool {
		own = append(own, a)
		return true
	})
	kept := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	kept.AddAttrs(r.attrs...)
	kept.AddAttrs(nest(r.groups, own)...)
	r.mu.Lock()
	defer r.mu.Unlock()
	*r.records = append(*r.records, kept)
	_, err := io.WriteString(r.out, render(kept)+"\n")
	return err
}

func (r *recorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	with := *r
	with.attrs = append(slices.Clone(r.attrs), nest(r.groups, attrs)...)
	return &with
}

func (r *recorder) WithGroup(name string) slog.Handler {
	with := *r
	with.groups = append(slices.Clone(r.groups), name)
	return &with
}

// list returns the records that r and the handlers derived from it kept.
func (r *recorder) list() []slog.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(*r.records)
}

// nest returns attrs inside groups, the outermost first.
func nest(groups []string, attrs []slog.Attr) []slog.Attr {
	for _, group := range slices.Backward(groups) {
		attrs = []slog.Attr{{Key: group, Value: slog.GroupValue(attrs...)}}
	}
	return attrs
}

// render returns record as one line: its level, its message, and each attribute as key=value,
// with groups in braces and each value resolved and printed with %+v.
func render(record slog.Record) string {
	var b strings.Builder
	b.WriteString(record.Level.String() + " " + record.Message)
	record.Attrs(func(a slog.Attr) bool {
		renderAttr(&b, a)
		return true
	})
	return b.String()
}

func renderAttr(b *strings.Builder, a slog.Attr) {
	value := a.Value.Resolve()
	b.WriteString(" " + a.Key + "=")
	if value.Kind() != slog.KindGroup {
		_, _ = fmt.Fprintf(b, "%+v", value.Any())
		return
	}
	b.WriteString("{")
	for _, member := range value.Group() {
		renderAttr(b, member)
	}
	b.WriteString("}")
}

// handlerOf returns the handler that l writes through: the redacting handler.
func handlerOf(l *Logger) slog.Handler { return l.logger.Handler() }

// newRecord returns a record at level with the message msg and attrs.
func newRecord(level slog.Level, msg string, attrs ...slog.Attr) slog.Record {
	record := slog.NewRecord(time.Now(), level, msg, 0)
	record.AddAttrs(attrs...)
	return record
}

// valuer is a slog.LogValuer whose value holds a secret.
type valuer struct{ secret string }

func (v valuer) LogValue() slog.Value { return slog.StringValue("user " + v.secret) }

// groupValuer is a slog.LogValuer whose value is a group that holds a secret.
type groupValuer struct{ secret string }

func (v groupValuer) LogValue() slog.Value {
	return slog.GroupValue(slog.String("name", "user"), slog.String("value", v.secret))
}

// leakyError is an error that holds a secret in its text and in its cause, and prints more with
// %+v than its text says, as errors with stack traces do.
type leakyError struct {
	secret string
	cause  error
}

func (e leakyError) Error() string { return "refused for " + e.secret }

func (e leakyError) Unwrap() error { return e.cause }

func (e leakyError) Format(s fmt.State, verb rune) {
	if verb == 'v' && s.Flag('+') {
		_, _ = io.WriteString(s, "refused for "+e.secret+"\n\tat stack frame of "+e.secret)
		return
	}
	_, _ = io.WriteString(s, e.Error())
}

// outputs are the handlers that the redaction tests write through, each at Debug so that it
// keeps every line: recorder, and slog's JSON and text handlers.
var outputs = map[string]func(w io.Writer) slog.Handler{
	"recorder": func(w io.Writer) slog.Handler { return newRecorder(w, slog.LevelDebug) },
	"JSON": func(w io.Writer) slog.Handler {
		return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	},
	"text": func(w io.Writer) slog.Handler {
		return slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	},
}

// levels are the levels that the redaction tests log at.
var levels = []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}

// planting puts a secret in one part of a line: with returns the handler to log through, h or h
// with attributes or a group, and record returns the line to log at level.
type planting struct {
	name   string
	with   func(h slog.Handler, secret string) slog.Handler
	record func(level slog.Level, secret string) slog.Record
}

// as returns h as it is, for a planting that puts the secret in the record.
func as(h slog.Handler, _ string) slog.Handler { return h }

// plantings put a secret in each part of a line that a handler writes.
var plantings = []planting{
	{"the message", as, func(l slog.Level, s string) slog.Record {
		return newRecord(l, "connect with "+s)
	}},
	{"a string", as, func(l slog.Level, s string) slog.Record {
		return newRecord(l, "a string", slog.String("detail", "key "+s))
	}},
	{"a URL", as, func(l slog.Level, s string) slog.Record {
		return newRecord(l, "a URL", slog.String("url", "http://api.example.com/v2/"+s+"/users"))
	}},
	{"a key", as, func(l slog.Level, s string) slog.Record {
		return newRecord(l, "a key", slog.Attr{Key: "field_" + s, Value: slog.IntValue(1)})
	}},
	{"an error", as, func(l slog.Level, s string) slog.Record {
		err := fmt.Errorf("send: %w", errors.New("refused for "+s))
		return newRecord(l, "an error", slog.Any("error", err))
	}},
	{"an error that formats", as, func(l slog.Level, s string) slog.Record {
		return newRecord(l, "an error that formats", slog.Any("error", leakyError{secret: s}))
	}},
	{"a LogValuer", as, func(l slog.Level, s string) slog.Record {
		return newRecord(l, "a LogValuer", slog.Any("user", valuer{s}))
	}},
	{"a LogValuer group", as, func(l slog.Level, s string) slog.Record {
		return newRecord(l, "a LogValuer group", slog.Any("user", groupValuer{s}))
	}},
	{"a nested group", as, func(l slog.Level, s string) slog.Record {
		return newRecord(l, "a nested group", slog.Group("request",
			slog.Int("attempt", 1), slog.Group("header", slog.String("value", s))))
	}},
	{"a struct", as, func(l slog.Level, s string) slog.Record {
		return newRecord(l, "a struct", slog.Any("body", struct{ Text string }{"key " + s}))
	}},
	{
		"WithAttrs",
		func(h slog.Handler, s string) slog.Handler {
			return h.WithAttrs([]slog.Attr{slog.String("scope", s)})
		},
		func(l slog.Level, _ string) slog.Record { return newRecord(l, "WithAttrs") },
	},
	{
		"a group in WithAttrs",
		func(h slog.Handler, s string) slog.Handler {
			return h.WithAttrs([]slog.Attr{slog.Group("scope", slog.Any("user", valuer{s}))})
		},
		func(l slog.Level, _ string) slog.Record { return newRecord(l, "a group in WithAttrs") },
	},
	{
		"WithGroup",
		func(h slog.Handler, s string) slog.Handler { return h.WithGroup("group_" + s) },
		func(l slog.Level, _ string) slog.Record {
			return newRecord(l, "WithGroup", slog.Int("attempt", 1))
		},
	},
	{
		"a record after WithAttrs",
		func(h slog.Handler, _ string) slog.Handler {
			return h.WithAttrs([]slog.Attr{slog.Int("n", 1)})
		},
		func(l slog.Level, s string) slog.Record {
			return newRecord(l, "after WithAttrs", slog.String("detail", s))
		},
	},
	{"bytes", as, func(l slog.Level, s string) slog.Record {
		return newRecord(l, "bytes", slog.Any("body", []byte("key "+s)))
	}},
	{
		"WithAttrs in WithGroup",
		func(h slog.Handler, s string) slog.Handler {
			return h.WithGroup("outer").WithAttrs([]slog.Attr{slog.String("value", s)})
		},
		func(l slog.Level, _ string) slog.Record { return newRecord(l, "WithAttrs in WithGroup") },
	},
}

// TestNoSecretInAnyLine logs, through two Loggers that share one *slog.Logger as two clients do,
// lines with a secret in each part of a line, at each level, into each kind of handler. No line
// may hold a secret: each Logger replaces the members of its own Set, and the pattern of Permit
// API keys replaces the key of the other client. The sentinel fails when a line has no marker,
// so that a planting that puts no secret in the line cannot pass.
func TestNoSecretInAnyLine(t *testing.T) {
	keyA, keyB := canary.Key("loggera"), canary.Key("loggerb")
	// The opaque credentials match no pattern, so only the Set of their client replaces them.
	opaqueA, opaqueB := "opaqueaaaaaaaa", "opaquebbbbbbbb"
	scanner := canary.NewScanner(keyA, keyB, opaqueA, opaqueB)
	for name, output := range outputs {
		sink := &lines{}
		shared := slog.New(output(sink))
		setA, setB := redact.NewSet(keyA), redact.NewSet(keyB)
		setA.Add(opaqueA)
		setB.Add(opaqueB)
		a, b := New(shared, setA), New(shared, setB)
		for _, plant := range []struct {
			via    string
			logger *Logger
			secret string
		}{
			{"client A, its key", a, keyA},
			{"client B, its key", b, keyB},
			{"client A, the key of B", a, keyB},
			{"client B, the key of A", b, keyA},
			{"client A, its opaque credential", a, opaqueA},
			{"client B, its opaque credential", b, opaqueB},
		} {
			for _, level := range levels {
				for _, p := range plantings {
					where := fmt.Sprintf("%s handler, %s, %v, %s", name, plant.via, level, p.name)
					before := sink.count()
					h := p.with(handlerOf(plant.logger), plant.secret)
					if err := h.Handle(t.Context(), p.record(level, plant.secret)); err != nil {
						t.Errorf("%s: Handle: %v", where, err)
					}
					written := sink.since(before)
					if len(written) != 1 {
						t.Errorf("%s: the handler wrote %d lines, want 1: %q", where,
							len(written), written)
						continue
					}
					_, _ = io.WriteString(scanner, written[0])
					if strings.Contains(written[0], plant.secret) {
						t.Errorf("%s: the line holds the secret: %s", where, written[0])
					}
					if !strings.Contains(written[0], redact.Marker) {
						t.Errorf("%s: sentinel: the line has no %s, so nothing was replaced: %s",
							where, redact.Marker, written[0])
					}
				}
			}
		}
	}
	scanner.Check(t)
}

// TestErrorsReachTheHandlerAsText checks that an error reaches the handler as its text with the
// credentials replaced, and nothing else of it: no cause to unwrap, no other type and no other
// text for %+v, which zap writes as errorVerbose.
func TestErrorsReachTheHandlerAsText(t *testing.T) {
	key := canary.Key("errortext")
	rec := newRecorder(io.Discard, slog.LevelDebug)
	cause := errors.New("dial " + key)
	logged := fmt.Errorf("send: %w", leakyError{secret: key, cause: cause})
	if err := handlerOf(New(slog.New(rec), redact.NewSet(key))).Handle(t.Context(),
		newRecord(slog.LevelError, "an error", slog.Any("error", logged))); err != nil {
		t.Fatal(err)
	}
	records := rec.list()
	if len(records) != 1 {
		t.Fatalf("the handler got %d records, want 1", len(records))
	}
	attrs := attrsOf(records[0])
	if len(attrs) != 1 || attrs[0].Key != "error" {
		t.Fatalf("the record has the attributes %v, want one named error", attrs)
	}
	got, ok := attrs[0].Value.Any().(error)
	if !ok {
		t.Fatalf("the error reached the handler as a %T, want an error", attrs[0].Value.Any())
	}
	want := "send: refused for " + redact.Marker
	if got.Error() != want {
		t.Errorf("the error's text is %q, want %q", got.Error(), want)
	}
	if verbose := fmt.Sprintf("%+v", got); verbose != want {
		t.Errorf("%%+v of the error is %q, want its text %q", verbose, want)
	}
	if errors.Unwrap(got) != nil || errors.Is(got, cause) {
		t.Error("the handler can unwrap the error to its cause")
	}
	if _, ok := errors.AsType[leakyError](got); ok {
		t.Error("the handler can reach the original error with errors.As")
	}
	if _, ok := got.(fmt.Formatter); ok {
		t.Error("the error that reaches the handler formats itself, so it can print more")
	}

	// An error that formats itself, logged as it is, reaches the handler as its plain text, not
	// as what it prints for %+v.
	unwrapped := newRecord(slog.LevelError, "an error", slog.Any("error", leakyError{secret: key}))
	if err := handlerOf(New(slog.New(rec), redact.NewSet(key))).Handle(t.Context(),
		unwrapped); err != nil {
		t.Fatal(err)
	}
	records = rec.list()
	if len(records) != 2 {
		t.Fatalf("the handler got %d records, want 2", len(records))
	}
	plain, isError := attrsOf(records[1])[0].Value.Any().(error)
	if !isError {
		t.Fatal("the second error did not reach the handler as an error")
	}
	if want := "refused for " + redact.Marker; plain.Error() != want {
		t.Errorf("the text of an error that formats itself is %q, want %q", plain.Error(), want)
	}
}

// ctxKey is the type of the key of the value that TestTheContextReachesTheHandler puts in its
// context.
type ctxKey struct{}

// contextRecorder records the value under ctxKey of the context of each line it handles.
type contextRecorder struct {
	slog.Handler
	seen *[]any
}

func (c contextRecorder) Handle(ctx context.Context, record slog.Record) error {
	*c.seen = append(*c.seen, ctx.Value(ctxKey{}))
	return c.Handler.Handle(ctx, record)
}

// TestTheContextReachesTheHandler checks that a line reaches the handler with the context of the
// call that logged it, which handlers read trace IDs from.
func TestTheContextReachesTheHandler(t *testing.T) {
	var seen []any
	next := newRecorder(io.Discard, slog.LevelDebug)
	l := New(slog.New(contextRecorder{Handler: next, seen: &seen}), new(redact.Set))
	ctx := context.WithValue(t.Context(), ctxKey{}, "trace")
	l.Request(ctx, Request{Route: "/v2/x", Status: 200, Attempt: 1})
	if err := handlerOf(l).Handle(ctx, newRecord(slog.LevelInfo, "direct")); err != nil {
		t.Fatal(err)
	}
	if want := []any{"trace", "trace"}; !slices.Equal(seen, want) {
		t.Errorf("the handler saw the context values %v, want %v", seen, want)
	}
}

// attrsOf returns the attributes of record.
func attrsOf(record slog.Record) []slog.Attr {
	var attrs []slog.Attr
	record.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	return attrs
}

// TestValuesKeepTheirKind checks that the handler leaves values without text as they are, and
// gives a value of another type, such as a struct, to the handler as its text.
func TestValuesKeepTheirKind(t *testing.T) {
	key := canary.Key("kinds")
	rec := newRecorder(io.Discard, slog.LevelDebug)
	at := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	if err := handlerOf(New(slog.New(rec), redact.NewSet(key))).Handle(t.Context(),
		newRecord(slog.LevelInfo, "kinds",
			slog.Int("int", -3), slog.Uint64("uint", 4), slog.Float64("float", 1.5),
			slog.Bool("bool", true), slog.Duration("duration", time.Second), slog.Time("time", at),
			slog.Any("nil", nil), slog.Any("struct", struct{ N int }{7}),
			slog.String("text", "plain"))); err != nil {
		t.Fatal(err)
	}
	records := rec.list()
	if len(records) != 1 {
		t.Fatalf("the handler got %d records, want 1", len(records))
	}
	want := []slog.Attr{
		slog.Int("int", -3), slog.Uint64("uint", 4), slog.Float64("float", 1.5),
		slog.Bool("bool", true), slog.Duration("duration", time.Second), slog.Time("time", at),
		slog.Any("nil", nil), slog.String("struct", "{N:7}"), slog.String("text", "plain"),
	}
	got := attrsOf(records[0])
	if !slices.EqualFunc(got, want, slog.Attr.Equal) {
		t.Errorf("the handler got the attributes\n%v\nwant\n%v", got, want)
	}
}

// TestHandlerFollowsTheSlogContract runs slog's own conformance tests on the handler, which
// writes to a JSON handler.
func TestHandlerFollowsTheSlogContract(t *testing.T) {
	var out strings.Builder
	slogtest.Run(t, func(*testing.T) slog.Handler {
		out.Reset()
		return handlerOf(New(slog.New(slog.NewJSONHandler(&out, nil)), new(redact.Set)))
	}, func(t *testing.T) map[string]any {
		t.Helper()
		var line map[string]any
		if err := json.Unmarshal([]byte(out.String()), &line); err != nil {
			t.Fatalf("the JSON handler wrote %q: %v", out.String(), err)
		}
		return line
	})
}

func TestHandleKeepsTheRecord(t *testing.T) {
	rec := newRecorder(io.Discard, slog.LevelDebug)
	at := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	record := slog.NewRecord(at, slog.LevelWarn+1, "kept", 42)
	if err := handlerOf(New(slog.New(rec), new(redact.Set))).Handle(t.Context(),
		record); err != nil {
		t.Fatal(err)
	}
	records := rec.list()
	if len(records) != 1 {
		t.Fatalf("the handler got %d records, want 1", len(records))
	}
	got := records[0]
	if !got.Time.Equal(at) || got.Level != slog.LevelWarn+1 || got.Message != "kept" ||
		got.PC != 42 {
		t.Errorf("the handler got the record %v %v %q %d, want %v %v %q %d", got.Time,
			got.Level, got.Message, got.PC, at, slog.LevelWarn+1, "kept", 42)
	}
}

func TestEnabledFollowsTheHandler(t *testing.T) {
	h := handlerOf(New(slog.New(newRecorder(io.Discard, slog.LevelWarn)), new(redact.Set)))
	for level, want := range map[slog.Level]bool{
		slog.LevelDebug: false, slog.LevelInfo: false, slog.LevelWarn: true, slog.LevelError: true,
	} {
		if got := h.Enabled(t.Context(), level); got != want {
			t.Errorf("Enabled(%v) = %t, want %t as the handler given", level, got, want)
		}
	}
}

func TestWithGroupOfNoName(t *testing.T) {
	h := handlerOf(New(slog.New(newRecorder(io.Discard, slog.LevelDebug)), new(redact.Set)))
	if got := h.WithGroup(""); got != h {
		t.Errorf("WithGroup(\"\") = %v, want the handler itself, as slog.Handler asks", got)
	}
}

func TestOutput(t *testing.T) {
	rec := newRecorder(io.Discard, slog.LevelDebug)
	if got := New(slog.New(rec), new(redact.Set)).Output(); got != rec {
		t.Errorf("Output() = %v, want the handler of the logger given", got)
	}
}

// TestNilAndZeroLoggerDiscard checks that a nil *slog.Logger, and one whose handler is nil such
// as the zero slog.Logger, on which a call panics, log nothing, without a panic.
func TestNilAndZeroLoggerDiscard(t *testing.T) {
	key := canary.Key("discard")
	for name, logger := range map[string]*slog.Logger{"nil": nil, "zero": {}} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("logging through a %s *slog.Logger panicked: %v", name, r)
				}
			}()
			l := New(logger, redact.NewSet(key))
			if got := l.Output(); got != slog.DiscardHandler {
				t.Errorf("Output() = %v, want slog.DiscardHandler", got)
			}
			l.PlainHTTP("http://api.example.com/" + key)
			l.Request(t.Context(), Request{Route: "/v2/" + key, Err: errors.New(key)})
			for _, level := range levels {
				l.logger.Log(t.Context(), level, "discarded")
			}
		})
	}
}

func TestRequest(t *testing.T) {
	key := canary.Key("request")
	rec := newRecorder(io.Discard, slog.LevelDebug)
	l := New(slog.New(rec), redact.NewSet(key))
	l.Request(t.Context(), Request{
		Route:    "/v2/facts/{proj_id}/{env_id}/users/" + key,
		Status:   503,
		Attempt:  2,
		Duration: 1500 * time.Millisecond,
		Err:      errors.New("refused for " + key),
	})
	l.Request(t.Context(), Request{
		Route: "/v2/api-key/scope", Status: 200, Attempt: 1,
		Duration: time.Millisecond,
	})
	records := rec.list()
	if len(records) != 2 {
		t.Fatalf("Request wrote %d records, want 2", len(records))
	}
	for _, record := range records {
		if record.Level != slog.LevelDebug || record.Message != msgRequest {
			t.Errorf("Request wrote %v %q, want %v %q", record.Level, record.Message,
				slog.LevelDebug, msgRequest)
		}
	}
	for i, want := range [][]slog.Attr{
		{
			slog.String("route", "/v2/facts/{proj_id}/{env_id}/users/"+redact.Marker),
			slog.Int("status", 503),
			slog.Int("attempt", 2),
			slog.Duration("duration", 1500*time.Millisecond),
			slog.String("error", "refused for "+redact.Marker),
		},
		{
			slog.String("route", "/v2/api-key/scope"),
			slog.Int("status", 200),
			slog.Int("attempt", 1),
			slog.Duration("duration", time.Millisecond),
		},
	} {
		got := attrsOf(records[i])
		// The error reaches the handler as an error; compare its text.
		for j, a := range got {
			if err, ok := a.Value.Any().(error); ok {
				got[j] = slog.String(a.Key, err.Error())
			}
		}
		if !slices.EqualFunc(got, want, slog.Attr.Equal) {
			t.Errorf("request %d has the attributes\n%v\nwant\n%v", i+1, got, want)
		}
	}
}

// TestRequestAtInfo checks that a handler at Info gets no request line.
func TestRequestAtInfo(t *testing.T) {
	rec := newRecorder(io.Discard, slog.LevelInfo)
	New(slog.New(rec), new(redact.Set)).Request(t.Context(), Request{
		Route: "/v2/api-key/scope", Status: 500, Attempt: 1, Err: errors.New("refused"),
	})
	if records := rec.list(); len(records) != 0 {
		t.Errorf("a handler at Info got %d request lines, want none: %v", len(records), records)
	}
}

func TestPlainHTTP(t *testing.T) {
	key := canary.Key("plainhttp")
	rec := newRecorder(io.Discard, slog.LevelDebug)
	New(slog.New(rec), redact.NewSet(key)).PlainHTTP("http://api.example.com/" + key)
	records := rec.list()
	if len(records) != 1 {
		t.Fatalf("PlainHTTP wrote %d records, want 1", len(records))
	}
	record := records[0]
	if record.Level != slog.LevelWarn || record.Message != msgPlainHTTP {
		t.Errorf("PlainHTTP wrote %v %q, want %v %q", record.Level, record.Message,
			slog.LevelWarn, msgPlainHTTP)
	}
	want := []slog.Attr{slog.String("url", "http://api.example.com/"+redact.Marker)}
	if got := attrsOf(record); !slices.EqualFunc(got, want, slog.Attr.Equal) {
		t.Errorf("PlainHTTP wrote the attributes %v, want %v", got, want)
	}
}

func TestFactsViaCloudPDP(t *testing.T) {
	key := canary.Key("cloudpdp")
	rec := newRecorder(io.Discard, slog.LevelDebug)
	New(slog.New(rec), redact.NewSet(key)).FactsViaCloudPDP("https://cloudpdp.api.permit.io/" +
		key)
	records := rec.list()
	if len(records) != 1 {
		t.Fatalf("FactsViaCloudPDP wrote %d records, want 1", len(records))
	}
	record := records[0]
	if record.Level != slog.LevelWarn || record.Message != msgFactsViaCloudPDP {
		t.Errorf("FactsViaCloudPDP wrote %v %q, want %v %q", record.Level, record.Message,
			slog.LevelWarn, msgFactsViaCloudPDP)
	}
	want := []slog.Attr{slog.String("url", "https://cloudpdp.api.permit.io/"+redact.Marker)}
	if got := attrsOf(record); !slices.EqualFunc(got, want, slog.Attr.Equal) {
		t.Errorf("FactsViaCloudPDP wrote the attributes %v, want %v", got, want)
	}
}
