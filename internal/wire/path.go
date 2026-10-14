package wire

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/permitio/permit-golang/v2/internal/validate"
)

// Placeholders returns the names of the placeholders of route in order: route is a route
// template, the path of an operation in the API's spec, such as
// /v2/facts/{proj_id}/{env_id}/users/{user_id}. It returns an error when route does not start
// with a slash, has an empty segment, as // or a slash at the end make, or has a placeholder
// that is not a whole segment named with lower-case letters, digits and underscores.
func Placeholders(route string) ([]string, error) {
	rest, ok := strings.CutPrefix(route, "/")
	if !ok {
		return nil, fmt.Errorf("the route %q does not start with a slash", route)
	}
	var names []string
	for segment := range strings.SplitSeq(rest, "/") {
		if segment == "" {
			return nil, fmt.Errorf("the route %q has an empty segment; write it without // or "+
				"a slash at the end", route)
		}
		if !strings.ContainsAny(segment, "{}") {
			continue
		}
		name, opened := strings.CutPrefix(segment, "{")
		name, closed := strings.CutSuffix(name, "}")
		if !opened || !closed || name == "" ||
			strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
			return nil, fmt.Errorf("the route %q has the segment %q; a placeholder is a whole "+
				"segment, a lower-case name in braces, such as {user_id}", route, segment)
		}
		names = append(names, name)
	}
	return names, nil
}

// PathError is the error of a path parameter that the SDK does not send, as it would change the
// request's route: an empty one, which would leave an empty segment, or . or .., which a server
// or a proxy on the way may resolve as a step up or across the path. [Path] returns it before
// anything is sent.
type PathError struct {
	// Route is the route template, such as /v2/facts/{proj_id}/{env_id}/users/{user_id}.
	Route string

	// Placeholder is the name of the placeholder that the parameter fills, such as user_id.
	Placeholder string

	// Rule is the validation rule that the parameter breaks.
	Rule validate.Rule
}

// Error returns the placeholder, the route and the rule's message.
func (e *PathError) Error() string {
	return "{" + e.Placeholder + "} of " + e.Route + ": " + e.Rule.Message
}

// Path returns route, a route template that [Placeholders] accepts, with its placeholders
// replaced by values, in order: the path as the request sends it. It escapes each value on its
// own with url.PathEscape, so that a value stays one segment whatever it holds: a / becomes %2F,
// a % becomes %25 and a space %20, such as /v2/facts/p/e/users/a%2Fb for the user key a/b. It
// returns a *PathError when a value is empty, or is . or .., and an error when route is not a
// route template or values has another number of values than route has placeholders.
func Path(route string, values ...string) (string, error) {
	names, err := Placeholders(route)
	if err != nil {
		return "", err
	}
	if len(values) != len(names) {
		return "", fmt.Errorf("the route %q has %d placeholders, and %d values fill them", route,
			len(names), len(values))
	}
	segments := strings.Split(route, "/")
	next := 0
	for i, segment := range segments {
		if !strings.HasPrefix(segment, "{") {
			continue
		}
		value := values[next]
		switch value {
		case "":
			return "", &PathError{
				Route: route, Placeholder: names[next],
				Rule: validate.PathSegmentEmpty,
			}
		case ".", "..":
			return "", &PathError{
				Route: route, Placeholder: names[next],
				Rule: validate.PathSegmentDot,
			}
		}
		segments[i] = url.PathEscape(value)
		next++
	}
	return strings.Join(segments, "/"), nil
}
