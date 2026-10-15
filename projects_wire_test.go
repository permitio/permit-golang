package permit_test

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the projects: a key that may not act
// on the organization's projects, a project that does not exist, a key that another project has
// already, and a field and a per_page that the API rejects.
const (
	projectForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the projects of the organization","id":"req-3"}`
	projectNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the project does not exist","id":"req-4"}`
	projectConflictBody = `{"error_code":"CONFLICT","title":"Conflict",` +
		`"message":"a project with the key new-project exists already","id":"req-5"}`
	projectInvalidBody = `{"detail":[{"loc":["body","active_policy_repo_id"],` +
		`"msg":"value is not a valid uuid","type":"type_error.uuid"}]}`
	perPageBody = `{"detail":[{"loc":["query","per_page"],` +
		`"msg":"ensure this value is less than or equal to 100",` +
		`"type":"value_error.number.not_le","ctx":{"limit_value":100}}]}`
)

// The IDs of the organization of the projects of the reply fixtures, and of a project that only
// the list holds.
const (
	exampleOrganizationID = "00000000-0000-4000-8000-000000000001"
	stagingProjectID      = "00000000-0000-4000-8000-000000000004"
)

// at returns the timestamp s, in RFC 3339 with or without a zone, as a models.Time, the way the
// SDK reads it from a reply. It panics on a timestamp that is not one, a mistake in a test.
func at(s string) models.Time {
	var t models.Time
	if err := t.UnmarshalText([]byte(s)); err != nil {
		panic(err)
	}
	return t
}

// fixtureProject is the project of the reply fixture get_project.200.json, with every field.
func fixtureProject() *models.ProjectRead {
	return &models.ProjectRead{
		Key: "my-project", URNNamespace: "my-namespace", ID: exampleProjectID,
		OrganizationID: exampleOrganizationID, CreatedAt: at("2026-10-14T09:30:00Z"),
		UpdatedAt: at("2026-10-15T10:00:00.123456"), Name: "My project",
		Description:        "The main project",
		Settings:           models.Attributes{"limit": json.Number("9007199254740993")},
		ActivePolicyRepoID: "00000000-0000-4000-8000-000000000009",
	}
}

// listedProjects are the projects of the reply fixture list_projects.200.json.
func listedProjects() []models.ProjectRead {
	return []models.ProjectRead{
		{
			Key: "my-project", ID: exampleProjectID, OrganizationID: exampleOrganizationID,
			CreatedAt: at("2026-10-14T09:30:00Z"), UpdatedAt: at("2026-10-14T09:30:00Z"),
			Name: "My project",
		},
		{
			Key: "staging", ID: stagingProjectID, OrganizationID: exampleOrganizationID,
			CreatedAt: at("2026-10-15T08:00:00Z"), UpdatedAt: at("2026-10-15T08:00:00Z"),
			Name: "Staging", Description: "Tests before a release",
		},
	}
}

// firstProjects returns the first n projects that seq yields, and ends the loop there, so that
// it asks for no page past them; or the first error that seq yields.
func firstProjects(seq iter.Seq2[models.ProjectRead, error], n int,
) ([]models.ProjectRead, error) {
	var got []models.ProjectRead
	for project, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, project)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// projectError returns the error that a call of the operation op, along the route method and
// route, returns for an error reply of the API with status and body, without the fields that
// the body fills in, which the caller sets.
func projectError(op, method, route string, status int, body string) *permit.Error {
	return &permit.Error{
		Op: op, Method: method, Route: route, StatusCode: status, Body: []byte(body),
	}
}

// projectCases returns the wire cases of the projects of the organization. Each sends one
// request to the API, and none needs the scope of the client: the runner gives the client a
// scope for the routes with {proj_id}, which the calls do not read, as they take the project.
// The keys of the projects hold characters that the path escapes, a slash, a percent sign and a
// letter outside ASCII, as a/b and 50%é do, and one key has a leading dot, .hidden, which the
// path sends as it is.
func projectCases() []wireCase {
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, projectForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-3"
		e.Detail = "the API key may not act on the projects of the organization"
		return e
	}
	listErr := projectError("list_projects", http.MethodGet, "/v2/projects",
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_project", http.MethodGet, "/v2/projects/{proj_id}",
		http.StatusNotFound, projectNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-4"
	getErr.Detail = "the project does not exist"
	createErr := projectError("create_project", http.MethodPost, "/v2/projects",
		http.StatusConflict, projectConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "CONFLICT", "Conflict", "req-5"
	createErr.Detail = "a project with the key new-project exists already"
	updateErr := projectError("update_project", http.MethodPatch, "/v2/projects/{proj_id}",
		http.StatusUnprocessableEntity, projectInvalidBody)
	updateErr.Detail = "body.active_policy_repo_id: value is not a valid uuid"
	return []wireCase{
		{
			Method: "ProjectsService.List",
			Op:     "list_projects",
			Route:  "GET /v2/projects",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Projects.List(ctx, &permit.ListOptions{Page: 2, PerPage: 10})
			},
			Request: wireRequest{RawPath: "/v2/projects", RawQuery: "page=2&per_page=10"},
			Reply:   mockpermit.Reply{Body: replyFixture("list_projects.200.json")},
			WantResult: &permit.Page[models.ProjectRead]{
				Data: listedProjects(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "ProjectsService.All",
			Op:     "list_projects",
			Route:  "GET /v2/projects",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstProjects(c.Projects.All(ctx, nil), 2)
			},
			Request:    wireRequest{RawPath: "/v2/projects", RawQuery: "page=1&per_page=100"},
			Reply:      mockpermit.Reply{Body: replyFixture("list_projects.200.json")},
			WantResult: listedProjects(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: projectForbiddenBody},
			WantErr:    forbidden("list_projects", http.MethodGet, "/v2/projects"),
		},
		{
			Method: "ProjectsService.Get",
			Op:     "get_project",
			Route:  "GET /v2/projects/{proj_id}",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Projects.Get(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: "/v2/projects/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_project.200.json")},
			WantResult: fixtureProject(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: projectNotFoundBody},
			WantErr:    getErr,
		},
		{
			Method: "ProjectsService.Create",
			Op:     "create_project",
			Route:  "POST /v2/projects",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Projects.Create(ctx, newProject())
			},
			Request:    wireRequest{RawPath: "/v2/projects", Body: newProjectBody},
			Reply:      mockpermit.Reply{Body: replyFixture("create_project.200.json")},
			WantResult: createdProject(),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: projectConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "ProjectsService.Update",
			Op:     "update_project",
			Route:  "PATCH /v2/projects/{proj_id}",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Projects.Update(ctx, "50%é", models.ProjectUpdate{
					Name:        "Renamed project",
					Description: models.Null[string](),
					Settings:    models.Attributes{"limit": 10},
				})
			},
			Request: wireRequest{
				RawPath: "/v2/projects/50%25%C3%A9",
				Body:    `{"name":"Renamed project","description":null,"settings":{"limit":10}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_project.200.json")},
			WantResult: updatedProject(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: projectInvalidBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "ProjectsService.Delete",
			Op:     "delete_project",
			Route:  "DELETE /v2/projects/{proj_id}",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Projects.Delete(ctx, ".hidden")
			},
			Request:    wireRequest{RawPath: "/v2/projects/.hidden"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: projectForbiddenBody},
			WantErr:    forbidden("delete_project", http.MethodDelete, "/v2/projects/{proj_id}"),
		},
	}
}

// newProject is the project that the wire case of ProjectsService.Create creates, with every
// field of the request, and an initial environment with every field of its own.
func newProject() models.ProjectCreate {
	return models.ProjectCreate{
		Key: "new-project", URNNamespace: "new-namespace", Name: "New project",
		Description: "A project of its own", Settings: models.Attributes{},
		ActivePolicyRepoID: "00000000-0000-4000-8000-000000000009",
		InitialEnvironments: []models.EnvironmentCreate{{
			Key: "qa", Name: "QA", Description: "Checks by hand", CustomBranchName: "qa-policy",
			JWKS: &models.JwksConfig{
				Audience: []string{"app", "admin"}, Issuer: "https://issuer.example.com",
				JWKS: &models.JwksObj{Keys: []models.Attributes{{"kty": "RSA", "kid": "k1"}}},
				TTL:  60, URL: "https://issuer.example.com/jwks.json",
			},
			Settings: models.Attributes{"debug": true},
		}},
	}
}

// newProjectBody is the body that the call of ProjectsService.Create of newProject sends.
const newProjectBody = `{"key":"new-project","urn_namespace":"new-namespace",` +
	`"name":"New project","description":"A project of its own","settings":{},` +
	`"active_policy_repo_id":"00000000-0000-4000-8000-000000000009",` +
	`"initial_environments":[{"key":"qa","name":"QA","description":"Checks by hand",` +
	`"custom_branch_name":"qa-policy","jwks":{"audience":["app","admin"],` +
	`"issuer":"https://issuer.example.com","jwks":{"keys":[{"kid":"k1","kty":"RSA"}]},` +
	`"ttl":60,"url":"https://issuer.example.com/jwks.json"},"settings":{"debug":true}}]}`

// createdProject is the project of the reply fixture create_project.200.json.
func createdProject() *models.ProjectRead {
	return &models.ProjectRead{
		Key: "new-project", ID: "00000000-0000-4000-8000-000000000005",
		OrganizationID: exampleOrganizationID, CreatedAt: at("2026-10-15T11:00:00Z"),
		UpdatedAt: at("2026-10-15T11:00:00Z"), Name: "New project",
		Description: "A project of its own", Settings: models.Attributes{},
	}
}

// updatedProject is the project of the reply fixture update_project.200.json.
func updatedProject() *models.ProjectRead {
	return &models.ProjectRead{
		Key: "my-project", ID: exampleProjectID, OrganizationID: exampleOrganizationID,
		CreatedAt: at("2026-10-14T09:30:00Z"), UpdatedAt: at("2026-10-15T12:00:00Z"),
		Name: "Renamed project", Settings: models.Attributes{"limit": json.Number("10")},
	}
}
