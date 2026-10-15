package drift

import (
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/rest"
)

// opsComparer returns the comparer of the operation tests: fixtureComparer with rows for the
// schemas of the fixture spec's operations, and a spec, pdp-cloud, whose operations the tests
// break, with a bulk route whose request is an array.
func opsComparer(t *testing.T) *comparer {
	t.Helper()
	c := fixtureComparer(t)
	for _, name := range []string{"EnvRead", "EnvReadWithEmailConfig", "ThingUpdate"} {
		c.rows[name] = modelRow{Schema: name}
	}
	c.rows["RemoteScope"] = modelRow{Spec: "pdp-cloud", Schema: "Scope"}
	cloud := jsonObject(t, `{
		"components": {
			"requestBodies": {"Query": {"content": {"application/json": {"schema":
				{"$ref": "#/components/schemas/Scope"}}}, "required": true}},
			"schemas": {"Scope": {"type": "object"}}
		},
		"paths": {
			"/allowed": {"post": {"operationId": "is_allowed",
				"requestBody": {"$ref": "#/components/requestBodies/Query"},
				"responses": {
					"200": {"content": {"application/json": {"schema": {"type": "object"}}}},
					"default": {"content": {"application/json": {"schema": {"type": "string"}}}}}}},
			"/allowed/bulk": {"post": {"operationId": "is_allowed_bulk",
				"requestBody": {"content": {"application/json": {"schema": {"type": "array",
					"items": {"$ref": "#/components/schemas/Scope"}}}}, "required": true},
				"responses": {"200": {"content": {"application/json": {"schema": {"type": "array",
					"items": {"$ref": "#/components/schemas/Scope"}}}}}}}},
			"/lost": {"post": {"operationId": "lost_body",
				"requestBody": {"$ref": "#/components/requestBodies/Gone"}}}
		}
	}`)
	c.specs["pdp-cloud"] = &spec{name: "pdp-cloud", doc: cloud}
	return c
}

func TestOperation(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   rest.Op
		want []string
	}{
		{"a Get of its reply's model", rest.Op{
			ID:            "get_env",
			ResponseModel: "EnvReadWithEmailConfig",
		}, nil},
		{"a Get of a plain Read model, where the spec returns a richer one", rest.Op{
			ID: "get_env", ResponseModel: "EnvRead",
		}, []string{
			"operation get_env response: its 200 reply: the model EnvRead maps to the schema " +
				"EnvRead of control-plane, and the body holds EnvReadWithEmailConfig of " +
				"control-plane",
		}},
		{"a list of either shape", rest.Op{
			ID: "list_envs", ResponseModel: "EnvRead", MaxPerPage: 100,
		}, nil},
		{"a list of another model", rest.Op{
			ID: "list_envs", ResponseModel: "Scope", MaxPerPage: 100,
		}, []string{
			"operation list_envs response: its 200 reply: the model Scope maps to the schema " +
				"Scope of control-plane, and the body holds an array of EnvRead, or a paginated " +
				"envelope PaginatedResult_EnvRead_ of EnvRead of control-plane; the reply of a " +
				"list is an array or a paginated envelope of its model",
		}},
		{"a Get whose reply is a list of its model", rest.Op{
			ID: "list_envs", ResponseModel: "EnvRead",
		}, []string{
			"operation list_envs response: its 200 reply: the model EnvRead maps to the schema " +
				"EnvRead of control-plane, and the body holds an array of EnvRead, or a " +
				"paginated envelope PaginatedResult_EnvRead_ of EnvRead of control-plane",
		}},
		{"a list whose reply is its model itself", rest.Op{
			ID: "get_env", ResponseModel: "EnvReadWithEmailConfig", MaxPerPage: 100,
		}, []string{
			"operation get_env response: its 200 reply: the model EnvReadWithEmailConfig maps " +
				"to the schema EnvReadWithEmailConfig of control-plane, and the body holds " +
				"EnvReadWithEmailConfig of control-plane; the reply of a list is an array or a " +
				"paginated envelope of its model",
		}},
		{
			"a create",
			rest.Op{ID: "create_env", RequestModel: "Scope", ResponseModel: "EnvRead"},
			nil,
		},
		{"a body that the spec does not require, left out", rest.Op{ID: "create_env"}, nil},
		{"a required body left out", rest.Op{ID: "update_env"}, []string{
			"operation update_env request: the spec requires a request body, and the operation " +
				"has no RequestModel",
		}},
		{
			"a request of another model",
			rest.Op{ID: "update_env", RequestModel: "EnvRead"},
			[]string{
				"operation update_env request: the model EnvRead maps to the schema EnvRead of " +
					"control-plane, and the body holds ThingUpdate of control-plane",
			},
		},
		{"models of an operation without bodies", rest.Op{
			ID: "delete_env", RequestModel: "Scope", ResponseModel: "Scope",
		}, []string{
			"operation delete_env request: its RequestModel is Scope, and the spec gives the " +
				"operation no JSON request body",
			"operation delete_env response: its ResponseModel is Scope, and no reply of the " +
				"operation in 200-299 has a JSON body in the spec",
		}},
		{
			"a request of one alternative",
			rest.Op{ID: "create_thing", RequestModel: "Scope"},
			nil,
		},
		{
			"a reply of one alternative",
			rest.Op{ID: "create_thing", ResponseModel: "Thing"},
			[]string{
				"operation create_thing response: its 200 reply: the model Thing maps to the " +
					"schema Thing of control-plane, and the body holds Thing, or Scope of " +
					"control-plane",
			},
		},
		{"every reply in 200-299", rest.Op{ID: "get_thing", ResponseModel: "Thing"}, nil},
		{"replies of another model", rest.Op{ID: "get_thing", ResponseModel: "Scope"}, []string{
			"operation get_thing response: its 200 reply: the model Scope maps to the schema " +
				"Scope of control-plane, and the body holds Thing of control-plane",
			"operation get_thing response: its 202 reply: the model Scope maps to the schema " +
				"Scope of control-plane, and the body holds Thing of control-plane",
		}},
		{
			"an operation that no spec lists",
			rest.Op{ID: "get_nothing", ResponseModel: "Scope"},
			[]string{
				"operation get_nothing: no spec lists the operation, so its models cannot be " +
					"checked",
			},
		},
		{"an operation that no spec lists, without models", rest.Op{ID: "get_nothing"}, nil},
		{"models without rows", rest.Op{
			ID: "get_nothing", RequestModel: "Lost", ResponseModel: "Unknown",
		}, []string{
			"the operation get_nothing: its model Lost has no row under models in model-map.yaml",
			"the operation get_nothing: its model Unknown has no row under models in " +
				"model-map.yaml",
		}},
		{
			"a model of another spec",
			rest.Op{ID: "create_env", RequestModel: "RemoteScope"},
			[]string{
				"operation create_env request: the model RemoteScope maps to the schema Scope " +
					"of pdp-cloud, and the body holds Scope of control-plane",
			},
		},
		{
			"a request body behind a $ref",
			rest.Op{ID: "is_allowed", RequestModel: "RemoteScope"},
			nil,
		},
		{"a request body behind a $ref, left out", rest.Op{ID: "is_allowed"}, []string{
			"operation is_allowed request: the spec requires a request body, and the operation " +
				"has no RequestModel",
		}},
		{
			"a request of an array of its model, as a bulk route takes",
			rest.Op{ID: "is_allowed_bulk", RequestModel: "[]RemoteScope"},
			nil,
		},
		{
			"a request of an array of another model",
			rest.Op{ID: "is_allowed_bulk", RequestModel: "[]Scope"},
			[]string{
				"operation is_allowed_bulk request: the model Scope maps to the schema Scope of " +
					"control-plane, and the body holds an array of Scope of pdp-cloud; the " +
					"RequestModel []Scope is a JSON array of the model",
			},
		},
		{
			"a request of one model, where the spec takes an array",
			rest.Op{ID: "is_allowed_bulk", RequestModel: "RemoteScope"},
			[]string{
				"operation is_allowed_bulk request: the model RemoteScope maps to the schema " +
					"Scope of pdp-cloud, and the body holds an array of Scope of pdp-cloud",
			},
		},
		{
			"a request of an array, where the spec takes one model",
			rest.Op{ID: "is_allowed", RequestModel: "[]RemoteScope"},
			[]string{
				"operation is_allowed request: the model RemoteScope maps to the schema Scope of " +
					"pdp-cloud, and the body holds Scope of pdp-cloud; the RequestModel " +
					"[]RemoteScope is a JSON array of the model",
			},
		},
		{
			"a request of an array of a model without a row",
			rest.Op{ID: "is_allowed_bulk", RequestModel: "[]Lost"},
			[]string{
				"the operation is_allowed_bulk: its model Lost has no row under models in " +
					"model-map.yaml",
			},
		},
		{
			"a reply of an array of its model, which only a list is",
			rest.Op{
				ID: "is_allowed_bulk", RequestModel: "[]RemoteScope", ResponseModel: "RemoteScope",
			},
			[]string{
				"operation is_allowed_bulk response: its 200 reply: the model RemoteScope maps " +
					"to the schema Scope of pdp-cloud, and the body holds an array of Scope of " +
					"pdp-cloud",
			},
		},
		{
			"a reply of an array of its model, as a create of several items answers",
			rest.Op{
				ID: "is_allowed_bulk", RequestModel: "[]RemoteScope",
				ResponseModel: "[]RemoteScope",
			},
			nil,
		},
		{
			"a reply of an array of another model",
			rest.Op{ID: "is_allowed_bulk", RequestModel: "[]RemoteScope", ResponseModel: "[]Scope"},
			[]string{
				"operation is_allowed_bulk response: its 200 reply: the model Scope maps to the " +
					"schema Scope of control-plane, and the body holds an array of Scope of " +
					"pdp-cloud; the ResponseModel []Scope is a JSON array of the model",
			},
		},
		{
			"a reply of an array, where the spec returns one model",
			rest.Op{ID: "get_env", ResponseModel: "[]EnvReadWithEmailConfig"},
			[]string{
				"operation get_env response: its 200 reply: the model EnvReadWithEmailConfig " +
					"maps to the schema EnvReadWithEmailConfig of control-plane, and the body " +
					"holds EnvReadWithEmailConfig of control-plane; the ResponseModel " +
					"[]EnvReadWithEmailConfig is a JSON array of the model",
			},
		},
		{
			"a reply of an array of a model without a row",
			rest.Op{ID: "is_allowed_bulk", RequestModel: "[]RemoteScope", ResponseModel: "[]Lost"},
			[]string{
				"the operation is_allowed_bulk: its model Lost has no row under models in " +
					"model-map.yaml",
			},
		},
		{"an inline reply", rest.Op{
			ID: "is_allowed", RequestModel: "RemoteScope",
			ResponseModel: "RemoteScope",
		}, []string{
			"operation is_allowed response: its 200 reply: the model RemoteScope maps to the " +
				"schema Scope of pdp-cloud, and the body holds an inline schema of pdp-cloud",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			found, err := opsComparer(t).operation(tc.op)
			if err != nil {
				t.Fatal(err)
			}
			if got := findingTexts(found); !slices.Equal(got, tc.want) {
				t.Errorf("operation(%+v) =\n%s\nwant\n%s", tc.op, strings.Join(got, "\n"),
					strings.Join(tc.want, "\n"))
			}
		})
	}
}

// TestOperationOfASharedRoute checks that an operation is compared with the operation of each
// other spec that lists its method and path, as the cloud PDP lists the routes of the PDP
// container, under keys that name that spec, and that a model maps to a schema of that spec
// through the also of its row.
func TestOperationOfASharedRoute(t *testing.T) {
	c := fixtureComparer(t)
	operation := func(id, query string) string {
		return `{"operationId": "` + id + `",
			"requestBody": {"content": {"application/json": {"schema":
				{"$ref": "#/components/schemas/` + query + `"}}}, "required": true},
			"responses": {"200": {"content": {"application/json": {"schema":
				{"$ref": "#/components/schemas/Result"}}}}}}`
	}
	schemas := `"schemas": {"Query": {"type": "object"}, "AllowedQuery": {"type": "object"},
		"Result": {"type": "object"}}`
	c.specs["pdp-container"] = &spec{name: "pdp-container", doc: jsonObject(t, `{
		"components": {`+schemas+`},
		"paths": {"/allowed": {"post": `+operation("check", "Query")+`}}}`)}
	c.specs["pdp-cloud"] = &spec{name: "pdp-cloud", doc: jsonObject(t, `{
		"components": {`+schemas+`},
		"paths": {"/allowed": {"post": `+operation("allowed_handler", "AllowedQuery")+`},
			"/other": {"post": `+operation("other", "AllowedQuery")+`}}}`)}
	both := modelRow{
		Spec: "pdp-container", Schema: "Query",
		Also: []schemaRef{{Spec: "pdp-cloud", Schema: "AllowedQuery"}},
	}
	c.rows["Check"] = both
	c.rows["Result"] = modelRow{
		Spec: "pdp-container", Schema: "Result",
		Also: []schemaRef{{Spec: "pdp-cloud", Schema: "Result"}},
	}
	c.rows["ContainerCheck"] = modelRow{Spec: "pdp-container", Schema: "Query"}
	c.rows["ContainerResult"] = modelRow{Spec: "pdp-container", Schema: "Result"}
	for _, tc := range []struct {
		name string
		op   rest.Op
		want []string
	}{
		{"a model of both specs", rest.Op{ID: "check", RequestModel: "Check"}, nil},
		{
			"a reply of both specs",
			rest.Op{ID: "check", RequestModel: "Check", ResponseModel: "Result"},
			nil,
		},
		{"a model of one spec", rest.Op{ID: "check", RequestModel: "ContainerCheck"}, []string{
			"operation check@pdp-cloud request: the model ContainerCheck maps to the schema " +
				"Query of pdp-container, and the body holds AllowedQuery of pdp-cloud",
		}},
		{
			"a reply of one spec",
			rest.Op{ID: "check", RequestModel: "Check", ResponseModel: "ContainerResult"},
			[]string{
				"operation check@pdp-cloud response: its 200 reply: the model ContainerResult " +
					"maps to the schema Result of pdp-container, and the body holds Result of " +
					"pdp-cloud",
			},
		},
	} {
		found, err := c.operation(tc.op)
		if got := findingTexts(found); err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%s: operation =\n%s, %v\nwant\n%s", tc.name, strings.Join(got, "\n"), err,
				strings.Join(tc.want, "\n"))
		}
	}
	o, _ := findOperation(c.specs, "check")
	same := sameRoute(c.specs, o)
	if len(same) != 1 || same[0].spec.name != "pdp-cloud" || same[0].node["operationId"] !=
		"allowed_handler" || same[0].at != "#/paths/~1allowed/post" || same[0].method != "POST" ||
		same[0].path != "/allowed" {
		t.Errorf("sameRoute(check) = %+v, want the operation allowed_handler of pdp-cloud", same)
	}
	if label(rest.Op{ID: "check"}, o) != "check" || label(rest.Op{ID: "check"}, same[0]) !=
		"check@pdp-cloud" {
		t.Error("label does not name the spec of the operation of another spec, and only it")
	}
}

func TestOperationDidNotRun(t *testing.T) {
	c := opsComparer(t)
	c.rows["Odd"] = modelRow{Schema: "Odd"}
	for _, tc := range []struct {
		op   rest.Op
		want string
	}{
		{rest.Op{ID: "get_odd", ResponseModel: "Odd"}, `#/components/schemas/Odd: the keyword ` +
			`"not" is not one the walker implements`},
		{rest.Op{ID: "lost_body", RequestModel: "Scope"}, "#/components/requestBodies/Gone: " +
			"the $ref points at nothing in the spec"},
	} {
		_, err := c.operation(tc.op)
		if !isUnsupported(err) || !strings.HasPrefix(err.Error(), tc.want) {
			t.Errorf("operation(%s) fails with %v, want %q", tc.op.ID, err, tc.want)
		}
	}
}

func TestHeldModels(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{
		"Item": {"type": "object"},
		"Page": {"type": "object", "properties": {"data": {"type": "array",
			"items": {"$ref": "#/components/schemas/Item"}}, "total_count": {"type": "integer"}}},
		"NotAPage": {"type": "object", "properties": {"data": {"type": "array",
			"items": {"$ref": "#/components/schemas/Item"}}, "next": {"type": "string"}}},
		"DataObject": {"type": "object",
			"properties": {"data": {"$ref": "#/components/schemas/Item"}}},
		"List": {"type": "array", "items": {"$ref": "#/components/schemas/Item"}},
		"Loop": {"anyOf": [{"$ref": "#/components/schemas/Loop"}]}
	}`)
	for _, tc := range []struct {
		schema string
		want   []string
	}{
		{`{"$ref": "#/components/schemas/Page"}`, []string{"a paginated envelope Page of Item"}},
		{`{"$ref": "#/components/schemas/NotAPage"}`, []string{"NotAPage"}},
		{`{"$ref": "#/components/schemas/DataObject"}`, []string{"DataObject"}},
		{`{"$ref": "#/components/schemas/List"}`, []string{"an array List of Item"}},
		{
			`{"type": "object", "properties": {"data": {"type": "array",
				"items": {"$ref": "#/components/schemas/Item"}},
				"total_count": {"type": "integer"}}}`,
			[]string{"an inline schema"},
		},
		{
			`{"type": "array", "items": {"type": "array",
				"items": {"$ref": "#/components/schemas/Item"}}}`,
			[]string{"an array of an inline schema"},
		},
		{`{"type": "array", "items": {"anyOf": [{"$ref": "#/components/schemas/Item"},
			{"$ref": "#/components/schemas/Page"}]}}`, []string{"an array of Item or Page"}},
		{`{"type": "array"}`, []string{"an array of an inline schema"}},
		{
			`{"anyOf": [{"type": "string"}, {"$ref": "#/components/schemas/Item"}]}`,
			[]string{"an inline schema", "Item"},
		},
	} {
		held, err := s.heldModels(jsonValue(t, tc.schema), "#/b")
		got := make([]string, 0, len(held))
		for _, h := range held {
			got = append(got, h.String())
		}
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("heldModels(%s) = %q, %v; want %q", tc.schema, got, err, tc.want)
		}
	}
	for _, schema := range []string{
		`{"$ref": "#/components/schemas/Loop"}`,
		`{"type": "array", "items": {"$ref": "#/components/schemas/Loop"}}`,
	} {
		if _, err := s.heldModels(jsonValue(t, schema), "#/b"); err == nil {
			t.Errorf("heldModels(%s) does not fail", schema)
		}
	}
}
