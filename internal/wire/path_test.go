package wire

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/validate"
)

// userRoute is the route of a user, with three placeholders.
const userRoute = "/v2/facts/{proj_id}/{env_id}/users/{user_id}"

func TestPlaceholders(t *testing.T) {
	for route, want := range map[string][]string{
		"/v2/api-key/scope":                 nil,
		"/allowed":                          nil,
		userRoute:                           {"proj_id", "env_id", "user_id"},
		"/v2/x/{a}/{b_2}":                   {"a", "b_2"},
		"/v2/schema/{proj_id}/{env_id}/x.y": {"proj_id", "env_id"},
	} {
		got, err := Placeholders(route)
		if err != nil {
			t.Errorf("Placeholders(%q) returned %v", route, err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("Placeholders(%q) differs (-want +got):\n%s", route, diff)
		}
	}
	for route, want := range map[string]string{
		"":                   `the route "" does not start with a slash`,
		"v2/users":           `the route "v2/users" does not start with a slash`,
		"GET /v2/users":      "does not start with a slash",
		"/":                  `the route "/" has an empty segment; write it without // or a slash`,
		"/v2//users":         `the route "/v2//users" has an empty segment`,
		"/v2/users/":         `the route "/v2/users/" has an empty segment`,
		"/v2/users/{}":       `the route "/v2/users/{}" has the segment "{}"; a placeholder is a`,
		"/v2/users/{UserID}": `has the segment "{UserID}"`,
		"/v2/{user-id}":      `has the segment "{user-id}"`,
		"/v2/x{id}":          `has the segment "x{id}"`,
		"/v2/{id}x":          `has the segment "{id}x"`,
		"/v2/{id":            `has the segment "{id"`,
		"/v2/id}":            `has the segment "id}"`,
	} {
		got, err := Placeholders(route)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Placeholders(%q) = %q, %v; want an error that contains %q", route, got, err,
				want)
		}
	}
}

func TestPath(t *testing.T) {
	for _, tc := range []struct {
		route  string
		values []string
		want   string
	}{
		{userRoute, []string{"p", "e", "u"}, "/v2/facts/p/e/users/u"},
		{userRoute, []string{"p", "e", "a/b"}, "/v2/facts/p/e/users/a%2Fb"},
		{userRoute, []string{"p", "e", "100%"}, "/v2/facts/p/e/users/100%25"},
		{userRoute, []string{"p", "e", "a b"}, "/v2/facts/p/e/users/a%20b"},
		{userRoute, []string{"p", "e", "a?b#c;d,e"}, "/v2/facts/p/e/users/a%3Fb%23c%3Bd%2Ce"},
		{userRoute, []string{"p", "e", "é" + fixtures.LineSeparator}, "/v2/facts/p/e/users/" +
			"%C3%A9%E2%80%A8"},
		{userRoute, []string{"p", "e", fixtures.UUIDKey}, "/v2/facts/p/e/users/" +
			fixtures.UUIDKey},
		{userRoute, []string{"p", "e", fixtures.TypedKey}, "/v2/facts/p/e/users/" +
			fixtures.TypedKey},
		{userRoute, []string{"p", "e", fixtures.HTMLSpecial}, "/v2/facts/p/e/users/%3C%3E&"},
		{userRoute, []string{"p", "e", ".x"}, "/v2/facts/p/e/users/.x"},
		{userRoute, []string{"p", "e", "..."}, "/v2/facts/p/e/users/..."},
		{userRoute, []string{"p", "e", "../x"}, "/v2/facts/p/e/users/..%2Fx"},
		{userRoute, []string{"p", "e", "a+b@c=d"}, "/v2/facts/p/e/users/a+b@c=d"},
		{userRoute, []string{"p", "e", "\xff"}, "/v2/facts/p/e/users/%FF"},
		{"/v2/api-key/scope", nil, "/v2/api-key/scope"},
		{"/v2/x/{a}/y/{b}", []string{"1", "2"}, "/v2/x/1/y/2"},
	} {
		got, err := Path(tc.route, tc.values...)
		if err != nil {
			t.Errorf("Path(%q, %q) returned %v", tc.route, tc.values, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Path(%q, %q) = %q, want %q", tc.route, tc.values, got, tc.want)
		}
	}
}

func TestPathRejectsEmptyAndDotSegments(t *testing.T) {
	for _, tc := range []struct {
		values      []string
		placeholder string
		rule        validate.Rule
	}{
		{[]string{"", "e", "u"}, "proj_id", validate.PathSegmentEmpty},
		{[]string{"p", "", "u"}, "env_id", validate.PathSegmentEmpty},
		{[]string{"p", "e", ""}, "user_id", validate.PathSegmentEmpty},
		{[]string{"p", "e", "."}, "user_id", validate.PathSegmentDot},
		{[]string{"p", "..", "u"}, "env_id", validate.PathSegmentDot},
		{[]string{"..", "", "."}, "proj_id", validate.PathSegmentDot},
		{[]string{"p", "", ".."}, "env_id", validate.PathSegmentEmpty},
	} {
		got, err := Path(userRoute, tc.values...)
		pathErr, ok := errors.AsType[*PathError](err)
		if !ok {
			t.Errorf("Path(%q) = %q, %v; want a *PathError", tc.values, got, err)
			continue
		}
		want := &PathError{Route: userRoute, Placeholder: tc.placeholder, Rule: tc.rule}
		if diff := cmp.Diff(want, pathErr); diff != "" {
			t.Errorf("Path(%q) returned another *PathError (-want +got):\n%s", tc.values, diff)
		}
		if got != "" {
			t.Errorf("Path(%q) returned the path %q with its error, want none", tc.values, got)
		}
	}
}

func TestPathErrorText(t *testing.T) {
	err := &PathError{Route: userRoute, Placeholder: "user_id", Rule: validate.PathSegmentEmpty}
	want := "{user_id} of /v2/facts/{proj_id}/{env_id}/users/{user_id}: " +
		validate.PathSegmentEmpty.Message
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestPathRejectsBadArguments(t *testing.T) {
	for _, tc := range []struct {
		route  string
		values []string
		want   string
	}{
		{userRoute, []string{"p", "e"}, `the route "` + userRoute + `" has 3 placeholders, and ` +
			"2 values fill them"},
		{userRoute, []string{"p", "e", "u", "x"}, "has 3 placeholders, and 4 values fill them"},
		{"/v2/api-key/scope", []string{"x"}, "has 0 placeholders, and 1 values fill them"},
		{"/v2/users/{id", []string{"x"}, `has the segment "{id"`},
		{"/v2/x{id}y", []string{"x"}, `has the segment "x{id}y"`},
		{"/v2/a{b", []string{"x"}, `has the segment "a{b"`},
		{"/v2/a}b", []string{"x"}, `has the segment "a}b"`},
		{"v2/users/{id}", []string{"x"}, "does not start with a slash"},
	} {
		got, err := Path(tc.route, tc.values...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Path(%q, %q) = %q, %v; want an error that contains %q", tc.route, tc.values,
				got, err, tc.want)
		}
		if _, ok := errors.AsType[*PathError](err); ok {
			t.Errorf("Path(%q, %q) returned a *PathError, which is for a value", tc.route,
				tc.values)
		}
	}
}

// fuzzRoute is the route of FuzzPath, with literal segments around its two placeholders.
const fuzzRoute = "/v2/x/{a_id}/y/{b_id}"

func FuzzPath(f *testing.F) {
	f.Add("a/b", "100%")
	f.Add("..", "x")
	f.Add("", "x")
	f.Add(fixtures.UUIDKey, fixtures.TypedKey)
	f.Fuzz(func(t *testing.T, a, b string) {
		got, err := Path(fuzzRoute, a, b)
		if bad, placeholder := firstRejected(a, b); bad {
			pathErr, ok := errors.AsType[*PathError](err)
			if !ok || pathErr.Placeholder != placeholder || got != "" {
				t.Fatalf("Path(%q, %q) = %q, %v; want a *PathError for {%s}", a, b, got, err,
					placeholder)
			}
			return
		}
		if err != nil {
			t.Fatalf("Path(%q, %q) returned %v", a, b, err)
		}
		segments := strings.Split(got, "/")
		if len(segments) != 6 || segments[0] != "" || segments[1] != "v2" ||
			segments[2] != "x" || segments[4] != "y" {
			t.Fatalf("Path(%q, %q) = %q, which is not the route's 5 segments", a, b, got)
		}
		for i, want := range map[int]string{3: a, 5: b} {
			value, unescapeErr := url.PathUnescape(segments[i])
			if unescapeErr != nil || value != want {
				t.Fatalf("Path(%q, %q) = %q, whose segment %q unescapes to %q, %v", a, b, got,
					segments[i], value, unescapeErr)
			}
		}
		u, err := url.Parse("http://127.0.0.1" + got)
		if err != nil {
			t.Fatalf("Path(%q, %q) = %q, which does not parse: %v", a, b, got, err)
		}
		if u.EscapedPath() != got || u.RawQuery != "" || u.Fragment != "" {
			t.Fatalf("Path(%q, %q) = %q, which a request sends as the path %q, the query %q "+
				"and the fragment %q", a, b, got, u.EscapedPath(), u.RawQuery, u.Fragment)
		}
	})
}

// firstRejected reports whether Path rejects one of the values a and b, which fill {a_id} and
// {b_id}, and names the placeholder of the first such value.
func firstRejected(a, b string) (bool, string) {
	for _, v := range []struct{ value, placeholder string }{{a, "a_id"}, {b, "b_id"}} {
		if v.value == "" || v.value == "." || v.value == ".." {
			return true, v.placeholder
		}
	}
	return false, ""
}
