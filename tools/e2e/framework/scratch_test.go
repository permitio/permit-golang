package framework

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/tools/e2e/fake"
)

// fakeKey is the project's API key of the fakes of these tests.
const fakeKey = "permit_key_FAKEframework"

// startFake starts a fake of the routes of environments, whose check accepts every body.
func startFake(t *testing.T) *fake.Fake {
	t.Helper()
	return fake.New(t, fake.Config{
		Routes: []fake.Route{
			{Method: "POST", Template: "/v2/projects/{proj_id}/envs", Op: "create_environment"},
			{
				Method:   "GET",
				Template: "/v2/projects/{proj_id}/envs/{env_id}",
				Op:       "get_environment",
			},
			{
				Method: "DELETE", Template: "/v2/projects/{proj_id}/envs/{env_id}",
				Op: "delete_environment",
			},
			{
				Method: "GET", Template: "/v2/api-key/{proj_id}/{env_id}",
				Op: "get_environment_api_key",
			},
		},
		Check:         func(string, []byte) ([]string, error) { return nil, nil },
		Project:       "project",
		ProjectAPIKey: fakeKey,
	})
}

func TestCreateEnvironment(t *testing.T) {
	f := startFake(t)
	api, apiErr := NewAPI(f.URL(), fakeKey, &http.Client{})
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	var l Ledger
	env, createErr := CreateEnvironment(t.Context(), api, &l, "project", "gosdk-ci-1-1-cloud")
	if createErr != nil {
		t.Fatal(createErr)
	}
	if env.ProjectID != f.ProjectID() || env.ID == "" || env.Key != "gosdk-ci-1-1-cloud" ||
		!strings.HasPrefix(env.APIKey(), "permit_key_FAKE") {
		t.Errorf("CreateEnvironment = %s, API key %q", env, env.APIKey())
	}
	for _, printed := range []string{fmt.Sprint(env), fmt.Sprintf("%+v %#v", env, env)} {
		if strings.Contains(printed, env.APIKey()) || !strings.Contains(printed, env.ID) {
			t.Errorf("the environment prints as %q, with its API key or without its ID", printed)
		}
	}
	if got := l.Counts(); got != (Counts{Registered: 1}) {
		t.Errorf("the ledger has %v after the create", got)
	}
	if _, err := CreateEnvironment(t.Context(), api, &l, "project", "gosdk-ci-1-1-cloud"); err ==
		nil || !strings.Contains(err.Error(), "answered 409") {
		t.Errorf("a second create of the key returned %v, want the conflict", err)
	}
	if err := l.Teardown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := l.Reconcile(); err != nil {
		t.Error(err)
	}
	reply, getErr := api.Do(t.Context(), Request{
		Method: "GET",
		Path:   "/v2/projects/project/envs/gosdk-ci-1-1-cloud",
	})
	if getErr != nil || reply.Status != http.StatusNotFound {
		t.Errorf("the environment after the teardown: %+v, %v; want 404", reply, getErr)
	}
}

func TestCreateEnvironmentRegistersFirst(t *testing.T) {
	f := startFake(t)
	api, apiErr := NewAPI(f.URL(), fakeKey, &http.Client{})
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	var l Ledger
	if _, err := CreateEnvironment(t.Context(), api, &l, "other", "gosdk-ci-1-1-cloud"); err ==
		nil || !strings.Contains(err.Error(), "answered 404") {
		t.Errorf("a create in a project that does not exist returned %v", err)
	}
	if got := l.Counts(); got.Registered != 1 {
		t.Errorf("the ledger has %v after a failed create, want it registered", got)
	}
}

func TestCreateEnvironmentChecksReplies(t *testing.T) {
	for _, tc := range []struct {
		name    string
		replies []func(http.ResponseWriter)
		want    string
	}{
		{"no id", []func(http.ResponseWriter){
			answer(200, `{"key": "k", "project_id": "p"}`),
		}, `was created as key "k", ID "" in the project "p"`},
		{"another key", []func(http.ResponseWriter){
			answer(200, `{"id": "e", "key": "x", "project_id": "p"}`),
		}, `was created as key "x"`},
		{"no project", []func(http.ResponseWriter){
			answer(200, `{"id": "e", "key": "k"}`),
		}, `in the project ""`},
		{"no secret", []func(http.ResponseWriter){
			answer(200, `{"id": "e", "key": "k", "project_id": "p"}`), answer(200, `{}`),
		}, "the API key of the environment k has no secret"},
		{"no API key", []func(http.ResponseWriter){
			answer(200, `{"id": "e", "key": "k", "project_id": "p"}`), answer(403, `no`),
		}, "GET /v2/api-key/p/e answered 403"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, s, _ := newScripted(t, tc.replies...)
			var l Ledger
			_, err := CreateEnvironment(t.Context(), api, &l, "p", "k")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("CreateEnvironment returned %v, want an error with %q", err, tc.want)
			}
			want := exchange{
				"POST", "/v2/projects/p/envs", "Bearer permit_key_FAKEapi",
				"application/json", "application/json", `{"description":"A scratch ` +
					`environment of the Go SDK's tests","key":"k","name":"k"}`,
			}
			if s.received[0] != want {
				t.Errorf("the create sent %+v, want %+v", s.received[0], want)
			}
		})
	}
}

func TestExistingEnvironment(t *testing.T) {
	env := ExistingEnvironment("p", "e", "gosdk-ci-1-1-pinned", "permit_key_FAKEexisting")
	want := Environment{
		ProjectID: "p", ID: "e", Key: "gosdk-ci-1-1-pinned", apiKey: "permit_key_FAKEexisting",
	}
	if env != want || env.APIKey() != "permit_key_FAKEexisting" {
		t.Errorf("ExistingEnvironment = %s, API key %q; want %s and its API key", env,
			env.APIKey(), want)
	}
	for _, printed := range []string{fmt.Sprint(env), fmt.Sprintf("%+v %#v", env, env)} {
		if strings.Contains(printed, env.APIKey()) {
			t.Errorf("the environment prints as %q, with its API key", printed)
		}
	}
}
