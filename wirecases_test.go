package permit_test

import (
	"errors"
	"fmt"
	"go/token"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/surface"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// wireCase is the wire test of a public method: the request the method must send, the reply it
// gets and the result it must return, and an error reply of the API with the error the method
// must return for it. A method can have more than one case, such as one for each of its options.
type wireCase struct {
	// Method is the method, as its type and name, such as UsersService.Get.
	Method string

	// Op is the operationId of the route's operation in the API spec, such as get_user.
	Op string

	// Route is the route that the request goes to, as mockpermit.Server.Handle takes it, such as
	// "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}".
	Route string

	// Modes lists the ways the client sends the request; the case runs once in each.
	Modes []wireMode

	// Request is the request the method must send.
	Request wireRequest

	// Reply is the reply to Request.
	Reply mockpermit.Reply

	// WantResult is the result the method must return for Reply, or wireNoResult{} when the
	// method returns only an error.
	WantResult any

	// ErrorReply is an error reply of the API to Request, with the API's error body.
	ErrorReply mockpermit.Reply

	// WantErr is the error the method must return for ErrorReply.
	WantErr error
}

// wireMode is a way the client sends a case's request.
type wireMode string

const (
	// apiMode sends the request to the Permit API.
	apiMode wireMode = "api"

	// pdpMode sends the request to the PDP.
	pdpMode wireMode = "pdp"

	// factsViaPDPMode sends a facts request to the PDP container instead of the Permit API.
	factsViaPDPMode wireMode = "facts-via-pdp"
)

// wireModes are the modes a case can list.
var wireModes = []wireMode{apiMode, pdpMode, factsViaPDPMode}

// wireRequest is the request a case expects its method to send.
type wireRequest struct {
	// RawPath is the path as the client sends it, with its escapes, such as
	// /v2/facts/p/e/users/a%2Fb.
	RawPath string

	// RawQuery is the query without the "?", or empty when the method sends none.
	RawQuery string

	// Body is the JSON body, which mockpermit.Request.ExpectJSON compares, or empty when the
	// method sends none.
	Body string
}

// wireNoResult is the WantResult of a case whose method returns only an error.
type wireNoResult struct{}

// wireCaseLists holds the <area>Cases function of each *_wire_test.go file, which returns the
// wire cases of the methods of its area. Add the function of a new file here.
var wireCaseLists = []func() []wireCase{}

// routeMethods are the request methods a route can have.
var routeMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// collectWireCases returns the cases of every list in lists, and an error that names each case
// the registry rejects and what is wrong with it.
func collectWireCases(lists []func() []wireCase) ([]wireCase, error) {
	var cases []wireCase
	var errs []error
	for _, list := range lists {
		for _, c := range list() {
			if problems := checkWireCase(c); problems != nil {
				errs = append(errs, fmt.Errorf("the wire case of %s (%s): %s", c.Method, c.Op,
					strings.Join(problems, "; ")))
			}
			cases = append(cases, c)
		}
	}
	return cases, errors.Join(errs...)
}

// wireCases returns the registered wire cases, and fails the test when the registry rejects one.
func wireCases(t *testing.T) []wireCase {
	t.Helper()
	cases, err := collectWireCases(wireCaseLists)
	if err != nil {
		t.Fatal(err)
	}
	return cases
}

// checkWireCase returns what the case lacks or gets wrong, or nil when it is complete.
func checkWireCase(c wireCase) []string {
	var problems []string
	if !isMethodName(c.Method) {
		problems = append(problems,
			"Method must name a method as <Type>.<Method>, such as UsersService.Get")
	}
	if c.Op == "" {
		problems = append(problems,
			"Op must be the operationId of the route's operation in the API spec")
	}
	if _, err := routePlaceholders(c.Route); err != nil {
		problems = append(problems, "Route: "+err.Error())
	}
	if len(c.Modes) == 0 {
		problems = append(problems, "Modes must list at least one mode")
	}
	for i, mode := range c.Modes {
		switch {
		case !slices.Contains(wireModes, mode):
			problems = append(problems, fmt.Sprintf("Modes has the unknown mode %q", mode))
		case slices.Contains(c.Modes[:i], mode):
			problems = append(problems, fmt.Sprintf("Modes lists %q twice", mode))
		}
	}
	if !strings.HasPrefix(c.Request.RawPath, "/") {
		problems = append(problems,
			"Request.RawPath must be the path the method sends, such as /v2/facts/p/e/users/a%2Fb")
	}
	if c.WantResult == nil {
		problems = append(problems, "WantResult must be the result the method returns for "+
			"Reply, or wireNoResult{} when it returns only an error")
	}
	if c.ErrorReply.Status < 400 || c.ErrorReply.Status > 599 || c.ErrorReply.Body == "" {
		problems = append(problems, "ErrorReply must be an error reply of the API: a status "+
			"from 400 to 599 and the API's error body")
	}
	if c.WantErr == nil {
		problems = append(problems, "WantErr must be the error the method returns for ErrorReply")
	}
	return problems
}

// isMethodName reports whether s names an exported method of an exported type, as
// <Type>.<Method>.
func isMethodName(s string) bool {
	typ, name, ok := strings.Cut(s, ".")
	return ok && token.IsIdentifier(typ) && token.IsExported(typ) &&
		token.IsIdentifier(name) && token.IsExported(name)
}

// routePlaceholders returns the names of the placeholders of route in order, or an error when
// route is not a method, a space and a path of segments that are not empty, whose placeholders
// are whole segments.
func routePlaceholders(route string) ([]string, error) {
	method, path, ok := strings.Cut(route, " ")
	if !ok || !slices.Contains(routeMethods, method) || !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("%q is not a method (%s), a space and a path, such as "+
			"\"GET /v2/facts/{proj_id}/{env_id}/users/{user_id}\"", route,
			strings.Join(routeMethods, ", "))
	}
	var names []string
	for segment := range strings.SplitSeq(path[1:], "/") {
		if segment == "" {
			return nil, fmt.Errorf("%q has an empty segment, which mockpermit.Server.Handle "+
				"rejects; write the path without // or a / at the end", route)
		}
		name, opened := strings.CutPrefix(segment, "{")
		name, closed := strings.CutSuffix(name, "}")
		if !opened && !closed {
			continue
		}
		if !opened || !closed || name == "" ||
			strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
			return nil, fmt.Errorf("%q has the segment %q; a placeholder is a whole segment, a "+
				"lower-case name in braces, such as {user_id}", route, segment)
		}
		names = append(names, name)
	}
	return names, nil
}

// publicMethods returns the methods that need a wire case, by their names such as
// UsersService.Get: the methods of Client and of the types whose names end in Service, also those
// they promote from the types of the package that they embed. New and the With options are
// functions, not methods, so they are not among them.
func publicMethods(items []surface.Item) map[string]surface.Item {
	methods := make(map[string]surface.Item)
	for _, item := range items {
		if item.Kind == surface.Method && isServiceType(item.Recv) {
			methods[item.Recv+"."+item.Name] = item
		}
	}
	return methods
}

// isServiceType reports whether the type name is Client or ends in Service, whose methods need
// wire cases.
func isServiceType(name string) bool {
	return name == "Client" || strings.HasSuffix(name, "Service")
}

// noWireFile lists the public methods that need no wire case.
const noWireFile = "testdata/no-wire.txt"

// exemption is a line of testdata/no-wire.txt: a public method that needs no wire case, and why.
type exemption struct {
	method string
	reason string
	line   int
}

// parseExemptions returns the exemptions in text, the content of testdata/no-wire.txt, and an
// error that names each line that is not one.
func parseExemptions(text string) ([]exemption, error) {
	var exemptions []exemption
	var errs []error
	lineOf := make(map[string]int)
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		method, reason, _ := strings.Cut(line, " ")
		reason = strings.TrimSpace(reason)
		switch {
		case !isMethodName(method):
			errs = append(errs, fmt.Errorf("%s:%d: %q does not start with a method as "+
				"<Type>.<Method>, such as UsersService.WaitForSync", noWireFile, n, line))
		case reason == "":
			errs = append(errs, fmt.Errorf("%s:%d: %s has no reason; write after it why it "+
				"needs no wire case", noWireFile, n, method))
		case lineOf[method] != 0:
			errs = append(errs, fmt.Errorf("%s:%d: %s is on line %d already", noWireFile, n,
				method, lineOf[method]))
		default:
			lineOf[method] = n
			exemptions = append(exemptions, exemption{method: method, reason: reason, line: n})
		}
	}
	return exemptions, errors.Join(errs...)
}

// readExemptions returns the exemptions in testdata/no-wire.txt, and fails the test when it
// cannot read them.
func readExemptions(t *testing.T) []exemption {
	t.Helper()
	text, err := os.ReadFile(noWireFile)
	if err != nil {
		t.Fatal(err)
	}
	exemptions, err := parseExemptions(string(text))
	if err != nil {
		t.Fatal(err)
	}
	return exemptions
}

// coverage is what checkCoverage found: the number of public methods, of those with a wire case
// and of those exempt, and the problems.
type coverage struct {
	methods, covered, exempt int
	problems                 []string
}

// checkCoverage checks that each public method among items has a wire case in cases or an
// exemption, that each case and exemption names a public method, and that no method has both.
// Its count sentinel checks that methods = covered + exempt. A service type that promotes
// methods internal/surface cannot list, such as those of a type of another package that it
// embeds, is a problem too: those methods would go without a case unseen.
func checkCoverage(items []surface.Item, cases []wireCase, exemptions []exemption) coverage {
	methods := publicMethods(items)
	cov := coverage{methods: len(methods)}
	for _, item := range items {
		if item.Kind != surface.Type || !isServiceType(item.Name) {
			continue
		}
		for _, unlisted := range item.Unlisted {
			cov.problems = append(cov.problems, fmt.Sprintf("%s embeds %s, whose methods "+
				"internal/surface cannot list; declare the methods on %s", item.Name, unlisted,
				item.Name))
		}
	}
	hasCase := make(map[string]bool)
	for _, c := range cases {
		if _, ok := methods[c.Method]; !ok {
			cov.problems = append(cov.problems, fmt.Sprintf("the wire case of %s (%s) names no "+
				"public method; fix its Method or remove it", c.Method, c.Op))
			continue
		}
		hasCase[c.Method] = true
	}
	cov.covered = len(hasCase)
	exempt := make(map[string]bool)
	for _, e := range exemptions {
		_, isMethod := methods[e.method]
		switch {
		case !isMethod:
			cov.problems = append(cov.problems, fmt.Sprintf("%s:%d: %s is not a public method; "+
				"remove the line", noWireFile, e.line, e.method))
		case hasCase[e.method]:
			cov.problems = append(cov.problems, fmt.Sprintf("%s:%d: %s has a wire case; remove "+
				"the line", noWireFile, e.line, e.method))
		default:
			exempt[e.method] = true
		}
	}
	cov.exempt = len(exempt)
	for _, name := range slices.Sorted(maps.Keys(methods)) {
		if !hasCase[name] && !exempt[name] {
			cov.problems = append(cov.problems, fmt.Sprintf("%s has no wire case; add one to "+
				"the <area>Cases function of its *_wire_test.go file, or list it in %s with the "+
				"reason it needs none", name, noWireFile))
		}
	}
	if cov.methods != cov.covered+cov.exempt {
		cov.problems = append(cov.problems, fmt.Sprintf("count sentinel: %d public methods, "+
			"but %d have a wire case and %d are exempt", cov.methods, cov.covered, cov.exempt))
	}
	return cov
}

func TestEveryPublicMethodHasAWireCase(t *testing.T) {
	root := readSurface(t)[0]
	cov := checkCoverage(root.Items, wireCases(t), readExemptions(t))
	for _, problem := range cov.problems {
		t.Error(problem)
	}
	t.Logf("%d public methods: %d with a wire case, %d exempt in %s", cov.methods, cov.covered,
		cov.exempt, noWireFile)
}

// scopePlaceholders are the placeholders of the project and the environment, which the client
// fills in from its scope when the method takes no parameter for them.
var scopePlaceholders = []string{"proj_id", "env_id"}

// stemAliases maps the short stems of placeholders to the words that parameter names use.
var stemAliases = map[string]string{"proj": "project", "env": "environment"}

// placeholderStem returns the stem of a placeholder: its name without an _id or else a _key
// suffix and without underscores, so user_id has the stem user and api_key_id apikey.
func placeholderStem(name string) string {
	base, ok := strings.CutSuffix(name, "_id")
	if !ok {
		base = strings.TrimSuffix(name, "_key")
	}
	return aliasStem(strings.ReplaceAll(base, "_", ""))
}

// paramStem returns the stem of a parameter name: its lower-case name without an ID or Key
// suffix, so userKey and userID have the stem user.
func paramStem(name string) string {
	for _, suffix := range []string{"ID", "Key"} {
		if base, ok := strings.CutSuffix(name, suffix); ok && base != "" {
			name = base
			break
		}
	}
	return aliasStem(strings.ToLower(name))
}

// aliasStem returns the word for the stem s, such as project for proj.
func aliasStem(s string) string {
	if word, ok := stemAliases[s]; ok {
		return word
	}
	return s
}

// routeParamOrder returns an error unless the placeholders of route name parameters of params in
// the route's order. A placeholder names the parameter with its stem, so {resource_id} names
// resourceKey or resourceID and {proj_id} names projectKey; the placeholders in scopePlaceholders
// need a parameter only when the method takes one.
func routeParamOrder(route string, params []surface.Param) error {
	placeholders, err := routePlaceholders(route)
	if err != nil {
		return err
	}
	taken := make([]bool, len(params))
	last, previous := -1, ""
	for _, placeholder := range placeholders {
		i := freeParam(params, taken, placeholderStem(placeholder))
		switch {
		case i < 0 && slices.Contains(scopePlaceholders, placeholder):
			continue
		case i < 0:
			return fmt.Errorf("no parameter is named after the placeholder {%s} of %q; name it "+
				"after the placeholder, as userKey or userID for {user_id}", placeholder, route)
		case i < last:
			return fmt.Errorf("the parameter %s comes before %s, but %q has {%s} after {%s}; "+
				"take the parameters in the route's order", params[i].Name, params[last].Name,
				route, placeholder, previous)
		}
		taken[i] = true
		last, previous = i, placeholder
	}
	return nil
}

// freeParam returns the index of the first parameter of params with the stem that an earlier
// placeholder has not taken, or -1, so that two placeholders of one stem, such as {user_id} and
// {user_key}, name two parameters.
func freeParam(params []surface.Param, taken []bool, stem string) int {
	for i, p := range params {
		if !taken[i] && paramStem(p.Name) == stem {
			return i
		}
	}
	return -1
}

// routeOrderProblems checks the route of each case against the parameters of its method among
// items.
func routeOrderProblems(items []surface.Item, cases []wireCase) []string {
	methods := publicMethods(items)
	var problems []string
	for _, c := range cases {
		m, ok := methods[c.Method]
		if !ok {
			problems = append(problems, fmt.Sprintf("the wire case of %s (%s) names no public "+
				"method", c.Method, c.Op))
			continue
		}
		if err := routeParamOrder(c.Route, m.Params); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", c.Method, err))
		}
	}
	return problems
}

func TestRouteParamOrder(t *testing.T) {
	root := readSurface(t)[0]
	for _, problem := range routeOrderProblems(root.Items, wireCases(t)) {
		t.Error(problem)
	}
}

// The tests below check the checks on a small SDK written for them.

// syntheticSDK is the source of an SDK with a client, two services, a page type, a constructor
// and an option.
const syntheticSDK = `package permit

import "context"

type Client struct {
	Users          *UsersService
	ImplicitGrants *ImplicitGrantsService
}

func New(apiKey string, opts ...Option) (*Client, error) { return nil, nil }

type Option func(*Client)

func WithScope(projectKey, environmentKey string) Option { return nil }

func (c *Client) Scope(ctx context.Context) (string, error) { return "", nil }

type UsersService struct{}

func (s *UsersService) Get(ctx context.Context, userKey string) (string, error) {
	return "", nil
}

func (s *UsersService) Delete(ctx context.Context, userKey string) error { return nil }

func (s *UsersService) WaitForSync() *UsersService { return s }

type ImplicitGrantsService struct{}

func (s *ImplicitGrantsService) Delete(ctx context.Context, resourceKey, roleKey string) error {
	return nil
}

type Page struct{}

func (p *Page) Next(ctx context.Context) bool { return false }
`

// errSynthetic is the WantErr of the synthetic cases.
var errSynthetic = errors.New("the API answered with an error")

// readSynthetic lists the exported API of the source src, a variant of syntheticSDK.
func readSynthetic(t *testing.T, src string) []surface.Item {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "permit.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := surface.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p.Items
}

// syntheticCase returns a complete wire case of method that sends rawPath to route.
func syntheticCase(method, route, rawPath string) wireCase {
	return wireCase{
		Method:     method,
		Op:         "op_" + strings.ReplaceAll(method, ".", "_"),
		Route:      route,
		Modes:      []wireMode{apiMode},
		Request:    wireRequest{RawPath: rawPath, RawQuery: "", Body: ""},
		Reply:      mockpermit.Reply{Status: http.StatusNoContent},
		WantResult: wireNoResult{},
		ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: `{"detail": "not found"}`},
		WantErr:    errSynthetic,
	}
}

// syntheticCases returns a wire case for each method of syntheticSDK that sends a request.
func syntheticCases() []wireCase {
	return []wireCase{
		syntheticCase("Client.Scope", "GET /v2/api-key/scope", "/v2/api-key/scope"),
		syntheticCase("UsersService.Get", "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
			"/v2/facts/p/e/users/u"),
		syntheticCase("UsersService.Delete",
			"DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}", "/v2/facts/p/e/users/u"),
		syntheticCase("ImplicitGrantsService.Delete", "DELETE /v2/schema/{proj_id}/{env_id}/"+
			"resources/{resource_id}/roles/{role_id}/implicit_grants",
			"/v2/schema/p/e/resources/doc/roles/viewer/implicit_grants"),
	}
}

// syntheticExemptions exempts the one method of syntheticSDK that sends no request.
var syntheticExemptions = []exemption{
	{method: "UsersService.WaitForSync", reason: "returns a copy of the service", line: 5},
}

func TestPublicMethods(t *testing.T) {
	got := slices.Sorted(maps.Keys(publicMethods(readSynthetic(t, syntheticSDK))))
	want := []string{
		"Client.Scope",
		"ImplicitGrantsService.Delete",
		"UsersService.Delete",
		"UsersService.Get",
		"UsersService.WaitForSync",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("publicMethods differs (-want +got):\n%s", diff)
	}
}

func TestCheckCoveragePasses(t *testing.T) {
	cov := checkCoverage(readSynthetic(t, syntheticSDK), syntheticCases(), syntheticExemptions)
	want := coverage{methods: 5, covered: 4, exempt: 1}
	if diff := cmp.Diff(want, cov, cmp.AllowUnexported(coverage{})); diff != "" {
		t.Errorf("checkCoverage differs (-want +got):\n%s", diff)
	}
}

func TestCheckCoverageFails(t *testing.T) {
	items := readSynthetic(t, syntheticSDK)
	cases := syntheticCases()
	for _, tc := range []struct {
		name       string
		cases      []wireCase
		exemptions []exemption
		want       coverage
	}{
		{
			name:       "a method without a case",
			cases:      slices.Delete(slices.Clone(cases), 2, 3),
			exemptions: syntheticExemptions,
			want: coverage{methods: 5, covered: 3, exempt: 1, problems: []string{
				"UsersService.Delete has no wire case; add one to the <area>Cases function of " +
					"its *_wire_test.go file, or list it in testdata/no-wire.txt with the reason " +
					"it needs none",
				"count sentinel: 5 public methods, but 3 have a wire case and 1 are exempt",
			}},
		},
		{
			name: "a case that names no method",
			cases: append(slices.Clone(cases), syntheticCase("UsersService.Remove",
				"DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}", "/v2/facts/p/e/users/u")),
			exemptions: syntheticExemptions,
			want: coverage{methods: 5, covered: 4, exempt: 1, problems: []string{
				"the wire case of UsersService.Remove (op_UsersService_Remove) names no public " +
					"method; fix its Method or remove it",
			}},
		},
		{
			name:  "an exemption of no method",
			cases: cases,
			exemptions: append(slices.Clone(syntheticExemptions),
				exemption{method: "UsersService.List", reason: "gone", line: 7}),
			want: coverage{methods: 5, covered: 4, exempt: 1, problems: []string{
				"testdata/no-wire.txt:7: UsersService.List is not a public method; remove the line",
			}},
		},
		{
			name:  "an exemption of a method with a case",
			cases: cases,
			exemptions: append(slices.Clone(syntheticExemptions),
				exemption{method: "UsersService.Get", reason: "no request", line: 9}),
			want: coverage{methods: 5, covered: 4, exempt: 1, problems: []string{
				"testdata/no-wire.txt:9: UsersService.Get has a wire case; remove the line",
			}},
		},
		{
			name:  "a method that is neither",
			cases: cases,
			want: coverage{methods: 5, covered: 4, exempt: 0, problems: []string{
				"UsersService.WaitForSync has no wire case; add one to the <area>Cases function " +
					"of its *_wire_test.go file, or list it in testdata/no-wire.txt with the " +
					"reason it needs none",
				"count sentinel: 5 public methods, but 4 have a wire case and 0 are exempt",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkCoverage(items, tc.cases, tc.exemptions)
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(coverage{})); diff != "" {
				t.Errorf("checkCoverage differs (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCheckCoverageEmbedded checks what a service type gets from the types it embeds: a method of
// an interface of the package needs a wire case, and a type of another package or error, whose
// methods internal/surface cannot list, is a problem. Other types may embed such types.
func TestCheckCoverageEmbedded(t *testing.T) {
	const imports = "import (\n\t\"context\"\n\n\t\"example.com/sdk/models\"\n)"
	src := strings.NewReplacer(
		`import "context"`, imports,
		"ImplicitGrants *ImplicitGrantsService\n}",
		"ImplicitGrants *ImplicitGrantsService\n\terror\n}",
		"type UsersService struct{}", "type UsersService struct {\n\tlister\n\tmodels.Base\n}\n\n"+
			"type lister interface {\n\tList(ctx context.Context) ([]string, error)\n}",
		"type Page struct{}", "type Page struct{ models.Base }",
	).Replace(syntheticSDK)
	got := checkCoverage(readSynthetic(t, src), syntheticCases(), syntheticExemptions)
	want := coverage{methods: 6, covered: 4, exempt: 1, problems: []string{
		"Client embeds error, whose methods internal/surface cannot list; declare the methods " +
			"on Client",
		"UsersService embeds models.Base, whose methods internal/surface cannot list; declare " +
			"the methods on UsersService",
		"UsersService.List has no wire case; add one to the <area>Cases function of its " +
			"*_wire_test.go file, or list it in testdata/no-wire.txt with the reason it needs none",
		"count sentinel: 6 public methods, but 4 have a wire case and 1 are exempt",
	}}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(coverage{})); diff != "" {
		t.Errorf("checkCoverage differs (-want +got):\n%s", diff)
	}
}

func TestRouteOrderProblemsPasses(t *testing.T) {
	if problems := routeOrderProblems(readSynthetic(t, syntheticSDK),
		syntheticCases()); problems != nil {
		t.Errorf("routeOrderProblems found %q, want none", problems)
	}
}

// TestRouteOrderProblemsFindsSwappedParameters checks a method whose parameters name the route's
// placeholders the wrong way round.
func TestRouteOrderProblemsFindsSwappedParameters(t *testing.T) {
	const inOrder, swappedNames = "resourceKey, roleKey string", "roleKey, resourceKey string"
	swapped := strings.Replace(syntheticSDK, inOrder, swappedNames, 1)
	if swapped == syntheticSDK {
		t.Fatal("the synthetic SDK has no ImplicitGrantsService.Delete to swap")
	}
	got := routeOrderProblems(readSynthetic(t, swapped), syntheticCases())
	want := []string{
		"ImplicitGrantsService.Delete: the parameter roleKey comes before resourceKey, but " +
			`"DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/` +
			`implicit_grants" has {role_id} after {resource_id}; take the parameters in the ` +
			"route's order",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("routeOrderProblems differs (-want +got):\n%s", diff)
	}
}

func TestRouteOrderProblemsFindsACaseWithoutAMethod(t *testing.T) {
	cases := []wireCase{syntheticCase("UsersService.Remove",
		"DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}", "/v2/facts/p/e/users/u")}
	got := routeOrderProblems(readSynthetic(t, syntheticSDK), cases)
	want := []string{
		"the wire case of UsersService.Remove (op_UsersService_Remove) names no public method",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("routeOrderProblems differs (-want +got):\n%s", diff)
	}
}

func TestRouteParamOrderRules(t *testing.T) {
	ctx := surface.Param{Name: "ctx", Type: "context.Context"}
	str := func(name string) surface.Param { return surface.Param{Name: name, Type: "string"} }
	for _, tc := range []struct {
		name   string
		route  string
		params []surface.Param
		want   string
	}{
		{
			name:   "no placeholders",
			route:  "GET /v2/api-key/scope",
			params: []surface.Param{ctx},
		},
		{
			name:   "scope filled by the client",
			route:  "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
			params: []surface.Param{ctx, str("userID")},
		},
		{
			name:   "scope taken as parameters",
			route:  "GET /v2/projects/{proj_id}/envs/{env_id}",
			params: []surface.Param{ctx, str("projectKey"), str("environmentKey")},
		},
		{
			name:   "short scope names",
			route:  "GET /v2/projects/{proj_id}/envs/{env_id}",
			params: []surface.Param{ctx, str("projID"), str("envID")},
		},
		{
			name:   "scope parameters swapped",
			route:  "GET /v2/projects/{proj_id}/envs/{env_id}",
			params: []surface.Param{ctx, str("environmentKey"), str("projectKey")},
			want: `the parameter environmentKey comes before projectKey, but "GET ` +
				`/v2/projects/{proj_id}/envs/{env_id}" has {env_id} after {proj_id}`,
		},
		{
			name:   "names with underscores and a key suffix",
			route:  "GET /v2/facts/{proj_id}/{env_id}/resource_instances/{resource_instance_key}",
			params: []surface.Param{ctx, str("resourceInstanceKey")},
		},
		{
			name:   "a placeholder named key",
			route:  "DELETE /v2/api-key/{api_key_id}",
			params: []surface.Param{ctx, str("apiKeyID")},
		},
		{
			name:   "a parameter without a suffix",
			route:  "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
			params: []surface.Param{ctx, str("user")},
		},
		{
			name:   "parameters between the path parameters",
			route:  "PUT /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users/{user_id}",
			params: []surface.Param{ctx, str("tenantKey"), str("note"), str("userKey")},
		},
		{
			name:   "two placeholders of one stem",
			route:  "PUT /v2/users/{user_id}/delegates/{user_key}",
			params: []surface.Param{ctx, str("userID"), str("userKey")},
		},
		{
			name:   "a second placeholder of one stem without a parameter",
			route:  "PUT /v2/users/{user_id}/delegates/{user_key}",
			params: []surface.Param{ctx, str("userID")},
			want: `no parameter is named after the placeholder {user_key} of "PUT ` +
				`/v2/users/{user_id}/delegates/{user_key}"`,
		},
		{
			name:   "swapped parameters and a later one of the same stem",
			route:  "PUT /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users/{user_id}",
			params: []surface.Param{ctx, str("userKey"), str("tenantKey"), str("user")},
			want:   "the parameter userKey comes before tenantKey",
		},
		{
			name:   "a placeholder without a parameter",
			route:  "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
			params: []surface.Param{ctx, str("key")},
			want: `no parameter is named after the placeholder {user_id} of "GET ` +
				`/v2/facts/{proj_id}/{env_id}/users/{user_id}"; name it after the placeholder`,
		},
		{
			name:   "unnamed parameters",
			route:  "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
			params: []surface.Param{{Type: "context.Context"}, {Type: "string"}},
			want:   "no parameter is named after the placeholder {user_id}",
		},
		{
			name:   "a malformed route",
			route:  "GET v2/users",
			params: []surface.Param{ctx},
			want: `"GET v2/users" is not a method (GET, POST, PUT, PATCH, DELETE), a space and ` +
				"a path",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := routeParamOrder(tc.route, tc.params)
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("routeParamOrder returned %v, want nil", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("routeParamOrder returned %v, want an error that contains %q", err,
					tc.want)
			}
		})
	}
}

func TestStems(t *testing.T) {
	for name, want := range map[string]string{
		"user_id":              "user",
		"group_instance_key":   "groupinstance",
		"resource_instance_id": "resourceinstance",
		"proj_id":              "project",
		"env_id":               "environment",
		"api_key_id":           "apikey",
		"template_type":        "templatetype",
	} {
		if got := placeholderStem(name); got != want {
			t.Errorf("placeholderStem(%q) = %q, want %q", name, got, want)
		}
	}
	for name, want := range map[string]string{
		"userKey":     "user",
		"userID":      "user",
		"user":        "user",
		"ID":          "id",
		"Key":         "key",
		"projectKey":  "project",
		"projID":      "project",
		"envKey":      "environment",
		"keyID":       "key",
		"groupIDKey":  "groupid",
		"apiKeyID":    "apikey",
		"resourceKey": "resource",
	} {
		if got := paramStem(name); got != want {
			t.Errorf("paramStem(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestRoutePlaceholders(t *testing.T) {
	for route, want := range map[string][]string{
		"GET /v2/api-key/scope": nil,
		"POST /allowed":         nil,
		"PUT /v2/x/{a}/{b}":     {"a", "b"},
		"DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}": {
			"proj_id", "env_id", "user_id",
		},
		"PATCH /v2/schema/{proj_id}/{env_id}/resources/{res_9_id}": {
			"proj_id", "env_id", "res_9_id",
		},
	} {
		got, err := routePlaceholders(route)
		if err != nil {
			t.Errorf("routePlaceholders(%q) returned %v", route, err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("routePlaceholders(%q) differs (-want +got):\n%s", route, diff)
		}
	}
	for route, want := range map[string]string{
		"":                        `"" is not a method`,
		"GET":                     `"GET" is not a method`,
		"FETCH /v2/users":         `"FETCH /v2/users" is not a method`,
		"get /v2/users":           `"get /v2/users" is not a method`,
		"GET v2/users":            `"GET v2/users" is not a method`,
		"GET /v2/users/{user_id":  `has the segment "{user_id"; a placeholder is a whole segment`,
		"GET /v2/users/user_id}":  `has the segment "user_id}"`,
		"GET /v2/users/{}":        `has the segment "{}"`,
		"GET /v2/users/{UserID}":  `has the segment "{UserID}"`,
		"GET /v2/users/{user-id}": `has the segment "{user-id}"`,
		"GET /v2/users/x{id}":     `has the segment "x{id}"`,
		"GET /v2//users":          `"GET /v2//users" has an empty segment`,
		"GET /v2/users/":          `"GET /v2/users/" has an empty segment`,
		"GET /":                   `"GET /" has an empty segment`,
	} {
		got, err := routePlaceholders(route)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("routePlaceholders(%q) returned %q, %v; want an error that contains %q",
				route, got, err, want)
		}
	}
}

func TestCheckWireCase(t *testing.T) {
	valid := syntheticCase("UsersService.Get", "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
		"/v2/facts/p/e/users/u")
	if problems := checkWireCase(valid); problems != nil {
		t.Fatalf("checkWireCase rejected a complete case: %q", problems)
	}
	for _, tc := range []struct {
		name   string
		change func(*wireCase)
		want   string
	}{
		{"no method", func(c *wireCase) { c.Method = "" }, "Method must name a method"},
		{"a method without its type", func(c *wireCase) { c.Method = "Get" }, "Method must name"},
		{"an unexported type", func(c *wireCase) { c.Method = "usersService.Get" }, "Method must"},
		{"an unexported method", func(c *wireCase) { c.Method = "Users.get" }, "Method must"},
		{"a method of three parts", func(c *wireCase) { c.Method = "A.B.C" }, "Method must name"},
		{"no operation", func(c *wireCase) { c.Op = "" }, "Op must be the operationId"},
		{"no route", func(c *wireCase) { c.Route = "" }, `Route: "" is not a method`},
		{"a malformed route", func(c *wireCase) { c.Route = "GET /users/{id" }, "Route: "},
		{"no modes", func(c *wireCase) { c.Modes = nil }, "Modes must list at least one mode"},
		{
			"empty modes", func(c *wireCase) { c.Modes = []wireMode{} },
			"Modes must list at least one mode",
		},
		{
			"an unknown mode", func(c *wireCase) { c.Modes = []wireMode{"cloud"} },
			`Modes has the unknown mode "cloud"`,
		},
		{
			"a mode twice", func(c *wireCase) { c.Modes = []wireMode{apiMode, pdpMode, apiMode} },
			`Modes lists "api" twice`,
		},
		{"no request", func(c *wireCase) { c.Request = wireRequest{} }, "Request.RawPath must be"},
		{
			"a relative path", func(c *wireCase) { c.Request.RawPath = "v2/users" },
			"Request.RawPath must be",
		},
		{"no result", func(c *wireCase) { c.WantResult = nil }, "WantResult must be the result"},
		{
			"no error reply", func(c *wireCase) { c.ErrorReply = mockpermit.Reply{} },
			"ErrorReply must be an error reply of the API",
		},
		{
			"a success as the error reply", func(c *wireCase) { c.ErrorReply.Status = 200 },
			"ErrorReply must be",
		},
		{"status 399", func(c *wireCase) { c.ErrorReply.Status = 399 }, "ErrorReply must be"},
		{"status 600", func(c *wireCase) { c.ErrorReply.Status = 600 }, "ErrorReply must be"},
		{
			"an error reply without a body", func(c *wireCase) { c.ErrorReply.Body = "" },
			"ErrorReply must be",
		},
		{"no error", func(c *wireCase) { c.WantErr = nil }, "WantErr must be the error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			c.Modes = slices.Clone(valid.Modes)
			tc.change(&c)
			problems := checkWireCase(c)
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Errorf("checkWireCase returned %q, want one problem that contains %q", problems,
					tc.want)
			}
		})
	}
	for _, status := range []int{400, 599} {
		c := valid
		c.ErrorReply.Status = status
		if problems := checkWireCase(c); problems != nil {
			t.Errorf("checkWireCase rejected an error reply with status %d: %q", status, problems)
		}
	}
	allModes := valid
	allModes.Modes = []wireMode{apiMode, pdpMode, factsViaPDPMode}
	if problems := checkWireCase(allModes); problems != nil {
		t.Errorf("checkWireCase rejected a case in every mode: %q", problems)
	}
}

func TestCollectWireCases(t *testing.T) {
	good := syntheticCases()
	bad := syntheticCase("UsersService.Get", "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
		"/v2/facts/p/e/users/u")
	bad.ErrorReply = mockpermit.Reply{}
	bad.WantErr = nil
	got, err := collectWireCases([]func() []wireCase{
		func() []wireCase { return good[:2] },
		func() []wireCase { return []wireCase{bad} },
		func() []wireCase { return good[2:] },
	})
	want := "the wire case of UsersService.Get (op_UsersService_Get): ErrorReply must be an " +
		"error reply of the API: a status from 400 to 599 and the API's error body; WantErr must " +
		"be the error the method returns for ErrorReply"
	if err == nil || err.Error() != want {
		t.Errorf("collectWireCases returned the error %v, want %q", err, want)
	}
	if len(got) != len(good)+1 || got[2].ErrorReply.Status != 0 {
		t.Errorf("collectWireCases returned %d cases, want all %d in order", len(got), len(good)+1)
	}
	cases, err := collectWireCases([]func() []wireCase{syntheticCases})
	if err != nil || len(cases) != len(good) {
		t.Errorf("collectWireCases of complete cases returned %d cases and %v, want %d and nil",
			len(cases), err, len(good))
	}
	if cases, err := collectWireCases(nil); cases != nil || err != nil {
		t.Errorf("collectWireCases of no lists returned %v, %v; want nil, nil", cases, err)
	}
}

func TestParseExemptions(t *testing.T) {
	got, err := parseExemptions("# header\n\nUsersService.WaitForSync returns a copy of the " +
		"service\nClient.Close  closes idle connections; sends nothing \n")
	want := []exemption{
		{method: "UsersService.WaitForSync", reason: "returns a copy of the service", line: 3},
		{method: "Client.Close", reason: "closes idle connections; sends nothing", line: 4},
	}
	if err != nil {
		t.Errorf("parseExemptions returned %v", err)
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(exemption{})); diff != "" {
		t.Errorf("parseExemptions differs (-want +got):\n%s", diff)
	}
	const noReason = "testdata/no-wire.txt:1: Users.WaitForSync has no reason"
	for text, want := range map[string]string{
		"Users.WaitForSync\n":   noReason,
		"Users.WaitForSync  \n": noReason,
		"WaitForSync returns a copy\n": `testdata/no-wire.txt:1: "WaitForSync returns a copy" ` +
			"does not start with a method",
		" Users.Get indented\n": `testdata/no-wire.txt:1: " Users.Get indented" does not ` +
			"start with a method",
		"A.B one\n\nA.B two\n": "testdata/no-wire.txt:3: A.B is on line 1 already",
	} {
		if _, err := parseExemptions(text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("parseExemptions(%q) returned %v, want an error that contains %q", text, err,
				want)
		}
	}
}
