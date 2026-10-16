package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the bulk operations: a user that the
// environment has already, a role of an inline role assignment that it does not have, a request
// above the API's bulk limit, an empty list of keys to delete, keys that name no object, and a
// field that the API rejects.
const (
	bulkConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"the user a/b exists already","id":"req-21"}`
	bulkRoleMissingBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the environment has no role editor","id":"req-22"}`
	bulkLimitBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the request exceeds the bulk limit of the API","id":"req-23"}`
	bulkEmptyBody    = `{"detail":"the idents list is empty"}`
	bulkNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"an ident names no object","id":"req-24"}`
	bulkInvalidBody = `{"detail":[{"loc":["body","operations",0,"key"],` +
		`"msg":"string does not match regex","type":"value_error.str.regex"}]}`
)

// bulkInstanceID is an ID of a resource instance in the shape of a UUID, which a bulk delete
// sends as it is, among the keys of the other instances.
const bulkInstanceID = "00000000-0000-4000-8000-000000000041"

// bulkCases returns the wire cases of the bulk operations of the users, the tenants and the
// resource instances, under /v2/facts/{proj_id}/{env_id}/bulk, and, with WithFactsViaPDP, under
// /facts/bulk. The routes take no key in their path: the keys go in the body, a JSON object that
// holds them in operations or idents, in order, as they are, a/b, 50%é, idp|dave@example.com,
// .hidden and an ID in the shape of a UUID too. Nil sends an empty array, never null. Each call
// reads no body from a reply in 200-299 and returns only an error, that of the reply for an
// error status, such as one that matches ErrNotFound for a 404 to a delete, whose body names the
// objects.
func bulkCases() []wireCase {
	const (
		users     = "/v2/facts/{proj_id}/{env_id}/bulk/users"
		tenants   = "/v2/facts/{proj_id}/{env_id}/bulk/tenants"
		instances = "/v2/facts/{proj_id}/{env_id}/bulk/resource_instances"
	)
	conflict := projectError("bulk_create_users", http.MethodPost, users, http.StatusConflict,
		bulkConflictBody)
	conflict.Code, conflict.Title, conflict.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-21"
	conflict.Detail = "the user a/b exists already"
	roleMissing := projectError("bulk_replace_users", http.MethodPut, users,
		http.StatusNotFound, bulkRoleMissingBody)
	roleMissing.Code, roleMissing.Title, roleMissing.RequestID = "NOT_FOUND", "Not Found", "req-22"
	roleMissing.Detail = "the environment has no role editor"
	overLimit := projectError("bulk_replace_users", http.MethodPut, users, http.StatusForbidden,
		bulkLimitBody)
	overLimit.Code, overLimit.Title, overLimit.RequestID = "FORBIDDEN", "Forbidden", "req-23"
	overLimit.Detail = "the request exceeds the bulk limit of the API"
	empty := projectError("bulk_delete_users", http.MethodDelete, users, http.StatusBadRequest,
		bulkEmptyBody)
	empty.Detail = "the idents list is empty"
	notFound := func(op, route string) *permit.Error {
		e := projectError(op, http.MethodDelete, route, http.StatusNotFound, bulkNotFoundBody)
		e.Code, e.Title, e.RequestID = "NOT_FOUND", "Not Found", "req-24"
		e.Detail = "an ident names no object"
		return e
	}
	invalid := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusUnprocessableEntity, bulkInvalidBody)
		e.Detail = "body.operations.0.key: string does not match regex"
		return e
	}
	both := []wireMode{apiMode, factsViaPDPMode}
	return []wireCase{
		{
			Method: "UsersService.BulkCreate",
			Op:     "bulk_create_users",
			Route:  "POST " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Users.BulkCreate(ctx, []models.UserCreate{
					{Key: "a/b"},
					{
						Key: syncedKey, Email: "dave@example.com", FirstName: "50%é",
						RoleAssignments: []models.UserRoleCreate{
							{Role: "editor", Tenant: "50%é"},
							{Role: "owner", ResourceInstance: "document:.hidden"},
						},
					},
					{Key: ".hidden", Attributes: models.Attributes{"a/b": "50%é"}},
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/bulk/users",
				Body: `{"operations":[{"key":"a/b"},` +
					`{"key":"idp|dave@example.com","email":"dave@example.com",` +
					`"first_name":"50%é","role_assignments":[` +
					`{"role":"editor","tenant":"50%é"},` +
					`{"role":"owner","resource_instance":"document:.hidden"}]},` +
					`{"key":".hidden","attributes":{"a/b":"50%é"}}]}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("bulk_create_users.200.json")},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: bulkConflictBody},
			WantErr:    conflict,
		},
		{
			Method: "UsersService.BulkCreate",
			Op:     "bulk_create_users",
			Route:  "POST " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Users.BulkCreate(ctx, nil)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/bulk/users",
				Body:    `{"operations":[]}`,
			},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: bulkInvalidBody,
			},
			WantErr: invalid("bulk_create_users", http.MethodPost, users),
		},
		{
			Method: "UsersService.BulkReplace",
			Op:     "bulk_replace_users",
			Route:  "PUT " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Users.BulkReplace(ctx, []models.UserCreate{
					{Key: "50%é", LastName: "a/b"},
					{
						Key:             bulkInstanceID,
						RoleAssignments: []models.UserRoleCreate{{Role: "editor", Tenant: "a/b"}},
					},
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/bulk/users",
				Body: `{"operations":[{"key":"50%é","last_name":"a/b"},` +
					`{"key":"00000000-0000-4000-8000-000000000041",` +
					`"role_assignments":[{"role":"editor","tenant":"a/b"}]}]}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("bulk_replace_users.200.json")},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: bulkRoleMissingBody},
			WantErr:    roleMissing,
		},
		{
			Method: "UsersService.BulkReplace",
			Op:     "bulk_replace_users",
			Route:  "PUT " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Users.BulkReplace(ctx, []models.UserCreate{})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/bulk/users",
				Body:    `{"operations":[]}`,
			},
			Reply:      mockpermit.Reply{Body: `{}`},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: bulkLimitBody},
			WantErr:    overLimit,
		},
		{
			Method: "UsersService.BulkDelete",
			Op:     "bulk_delete_users",
			Route:  "DELETE " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Users.BulkDelete(ctx, []string{
					"a/b", "50%é", syncedKey, ".hidden", bulkInstanceID,
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/bulk/users",
				Body: `{"idents":["a/b","50%é","idp|dave@example.com",".hidden",` +
					`"00000000-0000-4000-8000-000000000041"]}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("bulk_delete_users.200.json")},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: bulkNotFoundBody},
			WantErr:    notFound("bulk_delete_users", users),
		},
		{
			Method: "UsersService.BulkDelete",
			Op:     "bulk_delete_users",
			Route:  "DELETE " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Users.BulkDelete(ctx, nil)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/bulk/users",
				Body:    `{"idents":[]}`,
			},
			Reply:      mockpermit.Reply{Body: `{}`},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusBadRequest, Body: bulkEmptyBody},
			WantErr:    empty,
		},
		{
			Method: "TenantsService.BulkCreate",
			Op:     "bulk_create_tenants",
			Route:  "POST " + tenants,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Tenants.BulkCreate(ctx, []models.TenantCreate{
					{Key: "a/b", Name: "50%é"},
					{
						Key: ".hidden", Name: syncedKey, Description: "a/b",
						Attributes: models.Attributes{"tier": "gold"},
					},
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/bulk/tenants",
				Body: `{"operations":[{"key":"a/b","name":"50%é"},` +
					`{"key":".hidden","name":"idp|dave@example.com","description":"a/b",` +
					`"attributes":{"tier":"gold"}}]}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("bulk_create_tenants.200.json")},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: bulkInvalidBody,
			},
			WantErr: invalid("bulk_create_tenants", http.MethodPost, tenants),
		},
		{
			Method: "TenantsService.BulkDelete",
			Op:     "bulk_delete_tenants",
			Route:  "DELETE " + tenants,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Tenants.BulkDelete(ctx, []string{
					"a/b", "50%é", ".hidden", bulkInstanceID,
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/bulk/tenants",
				Body: `{"idents":["a/b","50%é",".hidden",` +
					`"00000000-0000-4000-8000-000000000041"]}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("bulk_delete_tenants.200.json")},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: bulkNotFoundBody},
			WantErr:    notFound("bulk_delete_tenants", tenants),
		},
		{
			Method: "ResourceInstancesService.BulkReplace",
			Op:     "bulk_replace_resource_instances",
			Route:  "PUT " + instances,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ResourceInstances.BulkReplace(ctx,
					[]models.ResourceInstanceCreate{
						{
							Key: "doc-1", Tenant: "50%é", Resource: "a/b",
							Attributes: models.Attributes{"owner": syncedKey},
						},
						{Key: ".hidden", Tenant: "tenant-a", Resource: "document"},
					})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/bulk/resource_instances",
				Body: `{"operations":[{"key":"doc-1","tenant":"50%é","resource":"a/b",` +
					`"attributes":{"owner":"idp|dave@example.com"}},` +
					`{"key":".hidden","tenant":"tenant-a","resource":"document"}]}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("bulk_replace_resource_instances.200.json"),
			},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: bulkInvalidBody,
			},
			WantErr: invalid("bulk_replace_resource_instances", http.MethodPut, instances),
		},
		{
			Method: "ResourceInstancesService.BulkDelete",
			Op:     "bulk_delete_resource_instances",
			Route:  "DELETE " + instances,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ResourceInstances.BulkDelete(ctx, []string{
					"a/b:50%é", "document:.hidden", bulkInstanceID,
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/bulk/resource_instances",
				Body: `{"idents":["a/b:50%é","document:.hidden",` +
					`"00000000-0000-4000-8000-000000000041"]}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("bulk_delete_resource_instances.200.json"),
			},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: bulkNotFoundBody},
			WantErr:    notFound("bulk_delete_resource_instances", instances),
		},
	}
}
