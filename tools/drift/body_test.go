package drift

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixtureSpecs returns the Specs of a directory whose control-plane.json is the fixture spec,
// testdata/spec.json, and whose specs of the PDP list no operation.
func fixtureSpecs(t *testing.T) *Specs {
	t.Helper()
	data, err := os.ReadFile("testdata/spec.json")
	if err != nil {
		t.Fatal(err)
	}
	return specsOf(t, string(data))
}

// specsOf returns the Specs of a directory whose control-plane.json is controlPlane, and whose
// specs of the PDP list no operation.
func specsOf(t *testing.T, controlPlane string) *Specs {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"control-plane.json": controlPlane,
		"pdp-container.json": `{"paths": {}}`,
		"pdp-cloud.json":     `{"paths": {}}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := ReadSpecs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSpecsOperations(t *testing.T) {
	s := fixtureSpecs(t)
	got := s.Operations("control-plane")
	want := []Operation{
		{"GET", "/envs", "list_envs"},
		{"POST", "/envs", "create_env"},
		{"DELETE", "/envs/{env_id}", "delete_env"},
		{"GET", "/envs/{env_id}", "get_env"},
		{"PATCH", "/envs/{env_id}", "update_env"},
		{"GET", "/nodes/{key}", "get_node"},
		{"GET", "/odd", "get_odd"},
		{"POST", "/things", "create_thing"},
		{"GET", "/things/{key}", "get_thing"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Operations = %v, want %v", got, want)
	}
	if got := s.Operations("pdp-cloud"); got != nil {
		t.Errorf("Operations of a spec without paths = %v, want none", got)
	}
	if got := s.Operations("staging"); got != nil {
		t.Errorf("Operations of a spec that s does not hold = %v, want none", got)
	}
}

func TestSpecsCheckRequestBody(t *testing.T) {
	s := fixtureSpecs(t)
	problems, err := s.CheckRequestBody("create_env", []byte(`{"organization_id": "o"}`))
	if err != nil || problems != nil {
		t.Errorf("a body that fits = %q, %v; want no problem", problems, err)
	}
	problems, err = s.CheckRequestBody("create_env", []byte(`{"project_id": 1, "x": true}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"# has no property organization_id, which the schema at #/components/schemas/Scope " +
			"requires",
		"#/project_id is number, and the schema at " +
			"#/components/schemas/Scope/properties/project_id allows string",
		"#/x is a value where the schema at " +
			"#/components/schemas/Scope/additionalProperties allows none",
	}
	if !slices.Equal(problems, want) {
		t.Errorf("CheckRequestBody = %q, want %q", problems, want)
	}
	for _, tc := range []struct{ id, body, want string }{
		{"create_user", `{}`, "no spec has the operation create_user"},
		{"get_env", `{}`, "the operation get_env has no JSON request body"},
		{"create_env", `{"organization_id": "o"} {}`, "is not one JSON value"},
		{"create_env", ``, "is not one JSON value"},
	} {
		if _, err := s.CheckRequestBody(tc.id, []byte(tc.body)); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Errorf("CheckRequestBody(%s, %q) returned %v, want an error with %q", tc.id,
				tc.body, err, tc.want)
		}
	}
	odd := specsOf(t, `{"paths": {"/odd": {"post": {"operationId": "create_odd", `+
		`"requestBody": {"content": {"application/json": {"schema": `+
		`{"not": {"type": "string"}, "type": "object"}}}}}}}}`)
	oddProblems, oddErr := odd.CheckRequestBody("create_odd", []byte(`{}`))
	wantErr := `#/paths/~1odd/post/requestBody/content/application~1json/schema: the keyword ` +
		`"not" is not one the walker implements; add it to keywords in tools/drift/schema.go`
	if oddProblems != nil || !isUnsupported(oddErr) || oddErr.Error() != wantErr {
		t.Errorf("CheckRequestBody of a schema with a keyword that the walker lacks = %q, %v; "+
			"want the error %q", oddProblems, oddErr, wantErr)
	}
}

func TestReadSpecsErrors(t *testing.T) {
	if _, err := ReadSpecs(filepath.Join(t.TempDir(), "none")); err == nil {
		t.Error("ReadSpecs of a directory that does not exist returned no error")
	}
	if _, err := ReadSpecs(t.TempDir()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ReadSpecs of an empty directory returned %v, want a missing file", err)
	}
}

// TestRepositoryReadSpecs checks a request body against the committed snapshots, which the
// end-to-end dry run reads.
func TestRepositoryReadSpecs(t *testing.T) {
	s, err := ReadSpecs(committedSpecs)
	if err != nil {
		t.Fatal(err)
	}
	problems, err := s.CheckRequestBody("create_environment", []byte(`{"key": "k", "name": "n"}`))
	if err != nil || problems != nil {
		t.Errorf("an environment's create body = %q, %v; want no problem", problems, err)
	}
	if len(s.Operations("control-plane")) == 0 {
		t.Error("the committed control plane's spec lists no operation")
	}
}
