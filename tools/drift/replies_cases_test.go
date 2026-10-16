package drift

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixtureReplies is the directory of the reply fixtures of the tests, each against the fixture
// spec: get_env.200.json, list_envs.200.json, list_envs.200.page.json and create_env.422.json fit
// their schemas, and each other one is wrong as the test expects.
const fixtureReplies = "testdata/replies"

func TestReplyFindings(t *testing.T) {
	specs := map[string]*spec{defaultSpec: fixtureSpec(t)}
	found, errs, n, err := replyFindings(fixtureReplies, specs)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"reply delete_env.204.json: the spec's 204 reply of delete_env has no JSON body",
		"reply get_env.200.extra-field.json#/region: is a value where the schema at " +
			"#/components/schemas/EnvReadWithEmailConfig/additionalProperties allows none",
		"reply get_env.404.json: the spec lists no 404 reply of get_env",
		"reply get_node.200.json#/children/0/label: is a property that the schema at " +
			"#/components/schemas/Node does not list",
		"get_user.200.json: no spec has the operation get_user",
		filepath.Join(fixtureReplies, "get_env.200.Upper.json") + " is not a reply fixture: " +
			"name each <operationId>.<status>[.<variant>].json, such as get_user.200.json, " +
			"with a variant of lower-case letters, digits and hyphens",
		filepath.Join(fixtureReplies, "nested") + " is not a reply fixture: name each " +
			"<operationId>.<status>[.<variant>].json, such as get_user.200.json, with a " +
			"variant of lower-case letters, digits and hyphens",
	}
	got := findingTexts(found)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("replyFindings =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if n != 6 {
		t.Errorf("replyFindings validated %d fixtures, want 6", n)
	}
	if len(errs) != 1 || !isUnsupported(errs[0]) || errs[0].Error() !=
		`get_odd.200.json: #/components/schemas/Odd: the keyword "not" is not one the walker `+
			"implements; add it to keywords in tools/drift/schema.go" {
		t.Errorf("replyFindings did not run on %v, want get_odd.200.json", errs)
	}
}

// TestReplyFindingsOfASharedRoute checks that a reply fixture is validated against the reply of
// each other spec that lists the method and path of its operation too, under keys that name that
// spec, and not when it does not validate against its own.
func TestReplyFindingsOfASharedRoute(t *testing.T) {
	operation := func(id, schema string) string {
		return `{"operationId": "` + id + `", "responses": {"200": {"content":
			{"application/json": {"schema": ` + schema + `}}}, "204": {}}}`
	}
	strict := `{"type": "object", "required": ["allow"], "properties": {"allow":
		{"type": "boolean"}}}`
	container := &spec{name: "pdp-container", doc: jsonObject(t, `{"paths": {
		"/allowed": {"post": `+operation("check", `{"type": "object"}`)+`}}}`)}
	cloud := &spec{name: "pdp-cloud", doc: jsonObject(t, `{"paths": {
		"/allowed": {"post": `+operation("allowed_handler", strict)+`},
		"/allowed/bulk": {"post": `+operation("other", strict)+`}}}`)}
	specs := map[string]*spec{"pdp-container": container, "pdp-cloud": cloud}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "check.200.json"), `{"allow": "yes"}`)
	writeFile(t, filepath.Join(dir, "check.200.empty.json"), `{}`)
	writeFile(t, filepath.Join(dir, "check.204.json"), `{}`)
	writeFile(t, filepath.Join(dir, "check.404.json"), `{}`)
	found, errs, n, err := replyFindings(dir, specs)
	want := []string{
		"reply check.200.empty.json@pdp-cloud#: has no property allow, which the schema at " +
			"#/paths/~1allowed/post/responses/200/content/application~1json/schema requires",
		"reply check.200.json@pdp-cloud#/allow: is string, and the schema at " +
			"#/paths/~1allowed/post/responses/200/content/application~1json/schema/properties/" +
			"allow allows boolean",
		"reply check.204.json: the spec's 204 reply of check has no JSON body",
		"reply check.404.json: the spec lists no 404 reply of check",
	}
	if got := findingTexts(found); err != nil || len(errs) != 0 || n != 2 ||
		!slices.Equal(got, want) {
		t.Errorf("replyFindings of a shared route =\n%s, %v, %v, %d\nwant\n%s",
			strings.Join(got, "\n"), errs, err, n, strings.Join(want, "\n"))
	}
	cloud.doc["paths"] = jsonObject(t, `{"/allowed": {"post": {"operationId": "allowed_handler",
		"responses": {"200": {"content": {"application/json": {"schema": {"not": {}}}}}}}}}`)
	if _, errs, _, err = replyFindings(dir, specs); err != nil || len(errs) != 2 ||
		!isUnsupported(errs[0]) {
		t.Errorf("replyFindings of a shared route whose schema the walker does not implement = "+
			"%v, %v; want an *unsupportedError for each fixture of 200", errs, err)
	}
}

func TestReplyFindingsOfOtherDirectories(t *testing.T) {
	specs := map[string]*spec{defaultSpec: fixtureSpec(t)}
	dir := t.TempDir()
	found, errs, n, err := replyFindings(filepath.Join(dir, "replies"), specs)
	if found != nil || errs != nil || n != 0 || err != nil {
		t.Errorf("replyFindings of a directory that does not exist = %v, %v, %d, %v; want "+
			"nothing", found, errs, n, err)
	}
	writeFile(t, filepath.Join(dir, "get_env.200.json"), `{"id": `)
	writeFile(t, filepath.Join(dir, "list_envs.200.json"), `[] []`)
	found, _, n, err = replyFindings(dir, specs)
	if n != 0 {
		t.Errorf("replyFindings validated %d replies that are not JSON", n)
	}
	want := []string{
		"get_env.200.json is not one JSON value: unexpected EOF",
		"list_envs.200.json is not one JSON value: the body holds more than one JSON value",
	}
	if got := findingTexts(found); err != nil || !slices.Equal(got, want) {
		t.Errorf("replyFindings of replies that are not JSON =\n%s, %v\nwant\n%s",
			strings.Join(got, "\n"), err, strings.Join(want, "\n"))
	}
	named := t.TempDir()
	if err = os.Mkdir(filepath.Join(named, "get_env.200.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	found, _, _, err = replyFindings(named, specs)
	want = []string{filepath.Join(named, "get_env.200.json") + " is not a reply fixture: name " +
		"each <operationId>.<status>[.<variant>].json, such as get_user.200.json, with a variant " +
		"of lower-case letters, digits and hyphens"}
	if got := findingTexts(found); err != nil || !slices.Equal(got, want) {
		t.Errorf("replyFindings of a directory named as a fixture = %q, %v; want %q", got, err,
			want)
	}
	file := filepath.Join(dir, "file")
	writeFile(t, file, "")
	if _, _, _, err := replyFindings(file, specs); err == nil {
		t.Error("replyFindings of a file does not fail")
	}
	broken := t.TempDir()
	if err := os.Symlink(filepath.Join(broken, "missing"),
		filepath.Join(broken, "get_env.200.json")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := replyFindings(broken, specs); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("replyFindings of a fixture it cannot read fails with %v, want "+
			"os.ErrNotExist", err)
	}
}

func TestReplyName(t *testing.T) {
	for name, want := range map[string][]string{
		"get_user.200.json":              {"get_user", "200", ""},
		"list_users.200.page.json":       {"list_users", "200", "page"},
		"_x.599.a-1.json":                {"_x", "599", "a-1"},
		"get_user.200":                   nil,
		"get_user.20.json":               nil,
		"get_user.600.json":              nil,
		"get_user.200.-a.json":           nil,
		"get_user.200.a.b.json":          nil,
		"1get.200.json":                  nil,
		"get-user.200.json":              nil,
		"get_user.200.json.txt":          nil,
		"get_user.200.Page.json":         nil,
		"get_user.200.page.json.orig":    nil,
		"prefix/get_user.200.page.json":  nil,
		"get_user.200.page_two.json":     nil,
		"get_user.200.page.two.json.bak": nil,
	} {
		m := replyName.FindStringSubmatch(name)
		if m != nil {
			m = m[1:]
		}
		if !slices.Equal(m, want) {
			t.Errorf("replyName matches %s as %q, want %q", name, m, want)
		}
	}
}
