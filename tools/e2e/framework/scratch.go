package framework

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// AbsenceTimeout bounds the check that a deleted object is gone.
const AbsenceTimeout = time.Minute

// Environment is a scratch environment: one that a run creates for itself in the project of the
// end-to-end tests, and deletes at its end. It prints without its API key.
type Environment struct {
	// ProjectID is the ID of its project.
	ProjectID string

	// ID is its ID.
	ID string

	// Key is its key, the run's prefix.
	Key string

	// apiKey is its API key.
	apiKey string
}

// APIKey returns the environment's API key.
func (e Environment) APIKey() string {
	return e.apiKey
}

// String returns the environment's key and IDs, without its API key.
func (e Environment) String() string {
	return fmt.Sprintf("environment %s (ID %s, project %s)", e.Key, e.ID, e.ProjectID)
}

// GoString returns what String returns, so that %#v leaves the API key out too.
func (e Environment) GoString() string {
	return e.String()
}

// CreateEnvironment creates the environment key in project, the key or the ID of a project, with
// api, whose API key may create environments in it, and returns it with its API key. It registers
// the environment's deletion and the check that it is gone in l before it sends the create.
func CreateEnvironment(ctx context.Context, api *API, l *Ledger, project, key string) (
	Environment, error,
) {
	envs := "/v2/projects/" + url.PathEscape(project) + "/envs"
	self := envs + "/" + url.PathEscape(key)
	l.Register("environment "+key, api.Remover(self), api.Absence(self, AbsenceTimeout))
	var created struct {
		ID        string `json:"id"`
		Key       string `json:"key"`
		ProjectID string `json:"project_id"`
	}
	body := map[string]string{
		"key": key, "name": key, "description": "A scratch environment of the Go SDK's tests",
	}
	err := api.Call(ctx, Request{Method: http.MethodPost, Path: envs, Body: body}, &created)
	if err != nil {
		return Environment{}, err
	}
	if created.ID == "" || created.Key != key || created.ProjectID == "" {
		return Environment{}, fmt.Errorf("framework: the environment %s was created as key %q, "+
			"ID %q in the project %q", key, created.Key, created.ID, created.ProjectID)
	}
	var secret struct {
		Secret string `json:"secret"`
	}
	path := "/v2/api-key/" + url.PathEscape(created.ProjectID) + "/" + url.PathEscape(created.ID)
	if err := api.Call(ctx, Request{Method: http.MethodGet, Path: path}, &secret); err != nil {
		return Environment{}, err
	}
	if secret.Secret == "" {
		return Environment{}, errors.New("framework: the API key of the environment " + key +
			" has no secret")
	}
	return Environment{
		ProjectID: created.ProjectID, ID: created.ID, Key: key, apiKey: secret.Secret,
	}, nil
}
