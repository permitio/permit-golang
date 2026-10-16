package main

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// routeMethods are the methods a route of a spec or of coverage.yaml can have. anyMethod stands
// for each method of a route of pdp-container.extra.json that takes several.
var routeMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

const anyMethod = "ANY"

// restParam is the name of the parameter that takes the rest of a path, one or more segments,
// as the facts catch-all of the container PDP does: pdp-container.extra.json writes it
// {full_path}.
const restParam = "{full_path}"

// segment is a segment of a path template: a literal, or a parameter such as {user_id}.
type segment struct {
	literal string
	param   bool
}

// template is the path of a route, such as /v2/facts/{proj_id}/{env_id}/users/{user_id}.
type template struct {
	segments []segment
	// rest says that the last segment is {full_path}, which takes one or more segments.
	rest bool
}

// parseRoute splits a route, "METHOD /path", into its method and path.
func parseRoute(route string) (string, string, error) {
	method, path, ok := strings.Cut(route, " ")
	if !ok || (!slices.Contains(routeMethods, method) && method != anyMethod) {
		return "", "", fmt.Errorf("%q is not a method (%s or %s), a space and a path", route,
			strings.Join(routeMethods, ", "), anyMethod)
	}
	if _, err := parseTemplate(path); err != nil {
		return "", "", fmt.Errorf("%q: %w", route, err)
	}
	return method, path, nil
}

// parseTemplate parses a path template. Each segment is a literal, or a parameter that is the
// whole segment, a name in braces. Only the last segment may be empty, for a path that ends in
// a /, as one of the control plane's does.
func parseTemplate(path string) (template, error) {
	rest, ok := strings.CutPrefix(path, "/")
	if !ok || rest == "" {
		return template{}, fmt.Errorf("the path %q does not start with / and a segment", path)
	}
	var t template
	parts := strings.Split(rest, "/")
	for i, part := range parts {
		name, opened := strings.CutPrefix(part, "{")
		name, closed := strings.CutSuffix(name, "}")
		switch {
		case part == "" && i < len(parts)-1:
			return template{}, fmt.Errorf("the path %q has an empty segment", path)
		case opened && closed && name != "" && !strings.ContainsAny(name, "{}"):
			t.segments = append(t.segments, segment{param: true})
			t.rest = part == restParam && i == len(parts)-1
		case strings.ContainsAny(part, "{}"):
			return template{}, fmt.Errorf("the path %q has the segment %q; a parameter is a "+
				"whole segment, such as {user_id}", path, part)
		default:
			t.segments = append(t.segments, segment{literal: part})
		}
	}
	return t, nil
}

// paramSegment matches a parameter of a path template, such as {user_id}.
var paramSegment = regexp.MustCompile(`\{[^{}/]+\}`)

// shape returns a route, or a path template, with each parameter's name left out, such as
// GET /v2/users/{} for GET /v2/users/{user_id}: two routes of one shape are the same route,
// whatever names a test or a file gives their parameters.
func shape(route string) string {
	return paramSegment.ReplaceAllString(route, "{}")
}

// splitPath returns the segments of a path as a client sent it, each unescaped. A segment may be
// empty, which no template matches.
func splitPath(path string) ([]string, error) {
	rest, ok := strings.CutPrefix(path, "/")
	if !ok {
		return nil, fmt.Errorf("the path %q does not start with /", path)
	}
	var segments []string
	for part := range strings.SplitSeq(rest, "/") {
		unescaped, err := url.PathUnescape(part)
		if err != nil {
			return nil, fmt.Errorf("the path %q has a bad escape in %q", path, part)
		}
		segments = append(segments, unescaped)
	}
	return segments, nil
}

// matches reports whether t matches the segments of a path: a literal matches the same segment,
// a parameter one segment that is not empty, and {full_path} at the end one or more such
// segments.
func (t template) matches(segments []string) bool {
	if len(segments) < len(t.segments) || len(segments) > len(t.segments) && !t.rest {
		return false
	}
	for i, s := range t.segments {
		if s.param && segments[i] == "" || !s.param && s.literal != segments[i] {
			return false
		}
	}
	return !slices.Contains(segments[len(t.segments):], "")
}

// literals returns the number of literal segments of t.
func (t template) literals() int {
	n := 0
	for _, s := range t.segments {
		if !s.param {
			n++
		}
	}
	return n
}

// moreLiteral reports whether a path that both a and b match belongs to a rather than b: the
// template with more literal segments wins, then the one without {full_path}, then the one
// whose first literal segment comes first where the other has a parameter.
func moreLiteral(a, b template) bool {
	if a.literals() > b.literals() {
		return true
	}
	if a.literals() < b.literals() {
		return false
	}
	if a.rest != b.rest {
		return b.rest
	}
	for i := range min(len(a.segments), len(b.segments)) {
		if a.segments[i].param != b.segments[i].param {
			return b.segments[i].param
		}
	}
	return false
}

// mount is a path prefix under which a server serves the routes of another prefix, such as the
// container PDP's /v2/facts/{proj_id}/{env_id}, which serves the routes of /facts.
type mount struct {
	prefix template
	sameAs []string
}

// errNoMount says that a path is not under a mount.
var errNoMount = errors.New("the path is under no mount")

// resolve returns the segments of a path under the mount as the path of the routes it serves.
func (m mount) resolve(segments []string) ([]string, error) {
	n := len(m.prefix.segments)
	if len(segments) <= n || !(template{segments: m.prefix.segments}).matches(segments[:n]) {
		return nil, errNoMount
	}
	return append(slices.Clone(m.sameAs), segments[n:]...), nil
}
