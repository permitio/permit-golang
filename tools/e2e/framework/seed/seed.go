// Package seed creates the test data of the end-to-end scenarios of tools/e2e in an environment
// of the Permit API: resources, roles, resource roles, relations, tenants, users, resource
// instances, role assignments and relationship tuples. Like the rest of tools/e2e/framework, it
// sends plain net/http requests through framework.API and not through the SDK, so the data that
// a scenario checks the SDK against does not depend on the code under test.
//
// Each call registers its object in the builder's ledger before it sends the request that
// creates the object, so that an object whose create reply is lost is still torn down, with a
// removal that succeeds on 404 and a check that the object is gone. The routes and the bodies
// are those of the control plane's spec snapshot, tools/specs/control-plane.json, which the fake
// of the dry run checks each body against.
package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// Builder creates objects in one environment. It is safe for concurrent use.
type Builder struct {
	api    *framework.API
	ledger *framework.Ledger

	// schema and facts are the paths of the environment's schema and facts, such as
	// /v2/schema/<project ID>/<environment ID>.
	schema, facts string

	// timeout bounds each check that an object is gone.
	timeout time.Duration
}

// New returns a Builder that creates objects in env with api and registers each in ledger: the
// ledger that holds env, which checks that each object is gone before it deletes env, or one of
// the scenario's own that is torn down before env is deleted.
func New(api *framework.API, ledger *framework.Ledger, env framework.Environment) *Builder {
	scope := "/" + url.PathEscape(env.ProjectID) + "/" + url.PathEscape(env.ID)
	return &Builder{
		api: api, ledger: ledger, schema: "/v2/schema" + scope, facts: "/v2/facts" + scope,
		timeout: framework.AbsenceTimeout,
	}
}

// post sends POST to path with body, and returns an error unless the reply's status is in
// 200-299.
func (b *Builder) post(ctx context.Context, path string, body any) error {
	return b.api.Call(ctx, framework.Request{Method: http.MethodPost, Path: path, Body: body}, nil)
}

// item registers the object called name in the ledger, with the removal and the check that it is
// gone of the object key of the collection at path, then creates it there with body.
func (b *Builder) item(ctx context.Context, name, path, key string, body any) error {
	self := path + "/" + url.PathEscape(key)
	b.ledger.Register(name, b.api.Remover(self), b.api.Absence(self, b.timeout))
	return b.post(ctx, path, body)
}

// lists returns a check of the body of a list, a JSON array of objects, that reports whether it
// holds an object whose fields that T decodes are those of want.
func lists[T comparable](want T) func(body []byte) (bool, error) {
	return func(body []byte) (bool, error) {
		var listed []T
		if err := json.Unmarshal(body, &listed); err != nil {
			return false, fmt.Errorf("decoding the list: %w", err)
		}
		return slices.Contains(listed, want), nil
	}
}

// Resource is a resource: a type of object, such as document, with the actions that a role may
// grant on it.
type Resource struct {
	// Key is its key.
	Key string

	// Actions are the keys of its actions, such as read.
	Actions []string
}

// resourceBody is the body of create_resource.
type resourceBody struct {
	Key     string              `json:"key"`
	Name    string              `json:"name"`
	Actions map[string]struct{} `json:"actions"`
}

// Resource creates r, with its key as its name.
func (b *Builder) Resource(ctx context.Context, r Resource) error {
	body := resourceBody{Key: r.Key, Name: r.Key, Actions: map[string]struct{}{}}
	for _, action := range r.Actions {
		body.Actions[action] = struct{}{}
	}
	return b.item(ctx, "resource "+r.Key, b.schema+"/resources", r.Key, body)
}

// roleBody is the body of create_role and create_resource_role.
type roleBody struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions,omitempty"`
}

// Role is a role of the environment, which grants its permissions to the users that have it in a
// tenant.
type Role struct {
	// Key is its key.
	Key string

	// Permissions are the actions that it grants, each as <resource>:<action>, such as
	// document:read.
	Permissions []string
}

// Role creates r, with its key as its name.
func (b *Builder) Role(ctx context.Context, r Role) error {
	body := roleBody{Key: r.Key, Name: r.Key, Permissions: r.Permissions}
	return b.item(ctx, "role "+r.Key, b.schema+"/roles", r.Key, body)
}

// ResourceRole is a role of a resource, which grants its permissions to the users that have it
// on an instance of the resource.
type ResourceRole struct {
	// Resource is the key of its resource.
	Resource string

	// Key is its key.
	Key string

	// Permissions are the actions of the resource that it grants, such as read.
	Permissions []string
}

// ResourceRole creates r, with its key as its name.
func (b *Builder) ResourceRole(ctx context.Context, r ResourceRole) error {
	path := b.schema + "/resources/" + url.PathEscape(r.Resource) + "/roles"
	body := roleBody{Key: r.Key, Name: r.Key, Permissions: r.Permissions}
	return b.item(ctx, "resource role "+r.Key+" of "+r.Resource, path, r.Key, body)
}

// Relation is a relation of a resource, such as parent, whose relationship tuples relate an
// instance of another resource, the subject, to an instance of the resource, the object.
type Relation struct {
	// Resource is the key of the resource of its objects, which has it.
	Resource string

	// Key is its key.
	Key string

	// SubjectResource is the key of the resource of its subjects.
	SubjectResource string
}

// relationBody is the body of create_resource_relation.
type relationBody struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	SubjectResource string `json:"subject_resource"`
}

// Relation creates r, with its key as its name.
func (b *Builder) Relation(ctx context.Context, r Relation) error {
	path := b.schema + "/resources/" + url.PathEscape(r.Resource) + "/relations"
	body := relationBody{Key: r.Key, Name: r.Key, SubjectResource: r.SubjectResource}
	return b.item(ctx, "relation "+r.Key+" of "+r.Resource, path, r.Key, body)
}

// Tenant creates the tenant key, with its key as its name.
func (b *Builder) Tenant(ctx context.Context, key string) error {
	body := map[string]string{"key": key, "name": key}
	return b.item(ctx, "tenant "+key, b.facts+"/tenants", key, body)
}

// User creates the user key.
func (b *Builder) User(ctx context.Context, key string) error {
	return b.item(ctx, "user "+key, b.facts+"/users", key, map[string]string{"key": key})
}

// Instance is a resource instance: an object of a resource in a tenant, which the Permit API
// names <resource>:<key>, such as document:doc-1.
type Instance struct {
	// Resource is the key of its resource.
	Resource string `json:"resource"`

	// Key is its key.
	Key string `json:"key"`

	// Tenant is the key of its tenant.
	Tenant string `json:"tenant"`
}

// Instance creates i, which needs a tenant.
func (b *Builder) Instance(ctx context.Context, i Instance) error {
	name := i.Resource + ":" + i.Key
	if i.Tenant == "" {
		return errors.New("seed: the resource instance " + name + " has no tenant, which the " +
			"API requires")
	}
	return b.item(ctx, "resource instance "+name, b.facts+"/resource_instances", name, i)
}

// RoleAssignment is a role of a user in a tenant, or, with a resource instance, a role of the
// instance's resource on the instance.
type RoleAssignment struct {
	// User is the key of the user.
	User string `json:"user"`

	// Role is the key of the role.
	Role string `json:"role"`

	// Tenant is the key of the tenant, the instance's tenant for a role on an instance.
	Tenant string `json:"tenant"`

	// ResourceInstance is the instance as <resource>:<key>, such as document:doc-1, or empty for
	// a role in the tenant.
	ResourceInstance string `json:"resource_instance,omitempty"`
}

// RoleAssignment registers a in the ledger, as [Builder.RegisterRoleAssignment] does, and then
// assigns it. A role assignment without a tenant is an error, and no request is sent.
func (b *Builder) RoleAssignment(ctx context.Context, a RoleAssignment) error {
	if err := b.RegisterRoleAssignment(a); err != nil {
		return err
	}
	return b.post(ctx, b.facts+"/role_assignments", a)
}

// RegisterRoleAssignment registers a in the builder's ledger and sends no request, for a scenario
// that assigns a itself, such as through the code under test: call it before the request that
// assigns a, so that a is torn down also when the reply to that request is lost. The teardown
// removes a, which succeeds when a is gone already, and checks that the list of the role
// assignments of its user, role, tenant and instance no longer holds it. The removal names the
// tenant, so a needs one, also on an instance: without one, RegisterRoleAssignment returns an
// error and registers nothing.
func (b *Builder) RegisterRoleAssignment(a RoleAssignment) error {
	name := "role assignment of " + a.Role + " to " + a.User
	if a.Tenant == "" {
		return errors.New("seed: the " + name + " has no tenant, which its removal needs")
	}
	name += " in " + a.Tenant
	if a.ResourceInstance != "" {
		name += " on " + a.ResourceInstance
	}
	path := b.facts + "/role_assignments"
	query := url.Values{"user": {a.User}, "role": {a.Role}, "tenant": {a.Tenant}}
	if a.ResourceInstance != "" {
		query.Set("resource_instance", a.ResourceInstance)
	}
	b.ledger.Register(name, b.api.RemoverWithBody(path, a),
		b.api.Unlisted(path+"?"+query.Encode(), b.timeout, lists(a)))
	return nil
}

// Tuple is a relationship tuple: the subject, an instance, has the relation to the object,
// another instance, such as folder:f1 parent document:doc-1.
type Tuple struct {
	// Subject is the subject as <resource>:<key>.
	Subject string `json:"subject"`

	// Relation is the key of the relation, which the object's resource has.
	Relation string `json:"relation"`

	// Object is the object as <resource>:<key>.
	Object string `json:"object"`

	// Tenant is the key of its tenant, or empty for the object's.
	Tenant string `json:"tenant,omitempty"`
}

// tupleKey names a relationship tuple: it is the body of delete_relationship_tuple.
type tupleKey struct {
	Subject  string `json:"subject"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
}

// Tuple registers t in the ledger, as [Builder.RegisterTuple] does, and then creates it.
func (b *Builder) Tuple(ctx context.Context, t Tuple) error {
	b.RegisterTuple(t)
	return b.post(ctx, b.facts+"/relationship_tuples", t)
}

// RegisterTuple registers t in the builder's ledger and sends no request, for a scenario that
// creates t itself, such as through the code under test: call it before the request that creates
// t, so that t is torn down also when the reply to that request is lost. The teardown deletes t,
// which succeeds when t is gone already, and checks that the list of the tuples of its subject,
// relation and object no longer holds it.
func (b *Builder) RegisterTuple(t Tuple) {
	path := b.facts + "/relationship_tuples"
	key := tupleKey{Subject: t.Subject, Relation: t.Relation, Object: t.Object}
	query := url.Values{"subject": {t.Subject}, "relation": {t.Relation}, "object": {t.Object}}
	b.ledger.Register("relationship tuple "+t.Subject+" "+t.Relation+" "+t.Object,
		b.api.RemoverWithBody(path, key),
		b.api.Unlisted(path+"?"+query.Encode(), b.timeout, lists(key)))
}
