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

// The error bodies of the Permit API in the wire cases of the audit logs: a page past the first
// 10000 audit logs of a list, a key that may not read the audit logs, and an audit log that does
// not exist.
const (
	auditLogOverreachBody = `{"error_code":"PAGINATION_SIZE_OVERREACHED",` +
		`"title":"The request was invalid","message":"Max number of results for this API is ` +
		`10000","id":"req-50"}`
	auditLogForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not read the audit logs of the environment","id":"req-51"}`
	auditLogNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the audit log does not exist","id":"req-52"}`
)

// auditLogQuery is the query of the wire case of AuditLogsService.List, which sets every filter:
// page 2 of 10 audit logs, the users, the resources, a decision of false and a timestamp_from of
// 0, which are sent, and the users repeated in their order, the empty one too.
const auditLogQuery = "action=read&decision=false&page=2" +
	"&pdp_id=00000000-0000-4000-8000-000000000601&per_page=10&query=none" +
	"&resources=document&resources=folder&sort_by=None&tenant=default&timestamp_from=0" +
	"&timestamp_to=1767225600&users=idp%7Calice&users=bob%40example.com&users="

// auditLogFilters are the options of the wire case of AuditLogsService.List, which auditLogQuery
// sends.
func auditLogFilters() *permit.AuditLogsListOptions {
	return &permit.AuditLogsListOptions{
		ListOptions: permit.ListOptions{Page: 2, PerPage: 10},
		PDPID:       pdpProductionID,
		Users:       []string{"idp|alice", "bob@example.com", ""},
		Decision:    new(false),
		Resources:   []string{"document", "folder"},
		Tenant:      "default", Action: "read",
		TimestampFrom: new(int64(0)), TimestampTo: new(int64(1767225600)),
		SortBy: models.AuditLogSortKeyNone, Query: models.AuditLogQueryTypeNone,
	}
}

// auditLogOf returns an audit log of the reply fixtures, of the environment of the scope
// fixture, with the fields that every audit log has.
func auditLogOf(id int, timestamp string) models.AuditLogModel {
	return models.AuditLogModel{
		ID: resourceID(id), Timestamp: at(timestamp), OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID,
	}
}

// auditLogsListed are the audit logs of the reply fixture list_audit_logs.200.json: a check that
// the PDP allowed, with every field, and one that it denied, whose decision is false.
func auditLogsListed() []models.AuditLogModel {
	allowed := auditLogOf(501, "2026-10-15T09:30:00Z")
	allowed.CreatedAt, allowed.Query = at("2026-10-15T09:30:02.123456"), "permit/root"
	allowed.UserKey, allowed.UserEmail, allowed.UserName = "idp|alice", "alice@example.com",
		"Alice"
	allowed.ResourceType, allowed.Tenant, allowed.Action = "document", "default", "read"
	allowed.Decision, allowed.Reason = new(true), "the user has the role viewer"
	allowed.PDPConfigID = pdpProductionID
	allowed.Input = json.RawMessage(`{"user":{"key":"idp|alice"},"action":"read",` +
		`"resource":{"type":"document","tenant":"default"}}`)
	allowed.Result = json.RawMessage(`{"allow":true}`)
	allowed.Context = json.RawMessage(`{"ip":"192.0.2.1"}`)
	denied := auditLogOf(502, "2026-10-15T09:31:00Z")
	denied.UserKey, denied.ResourceType, denied.Action = "idp|bob", "document", "delete"
	denied.Decision, denied.Result = new(false), json.RawMessage(`false`)
	return []models.AuditLogModel{allowed, denied}
}

// auditLogFixtureTime is the time of the objects of the reply fixture
// get_detailed_audit_log.200.json but the user, which was changed a day later.
const auditLogFixtureTime = "2026-10-01T08:00:00Z"

// auditLogDetailed is the audit log of the reply fixture get_detailed_audit_log.200.json, with
// the decision log of the engine OPA and an object of each kind.
func auditLogDetailed() *models.DetailedAuditLogModel {
	return &models.DetailedAuditLogModel{
		ID: resourceID(501), Timestamp: at("2026-10-15T09:30:00Z"), Query: "permit/root",
		UserKey: "idp|alice", ResourceType: "document", Tenant: "default", Action: "read",
		Decision: new(true), OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, PDPConfigID: pdpProductionID,
		Input:  json.RawMessage(`{"user":{"key":"idp|alice"},"action":"read"}`),
		Result: json.RawMessage(`{"allow":true}`),
		RawData: json.RawMessage(`{"engine":"OPA",` +
			`"decision_id":"00000000-0000-4000-8000-000000000801",` +
			`"labels":{"id":"00000000-0000-4000-8000-000000000601","version":"0.9.16"},` +
			`"timestamp":"2026-10-15T09:30:00Z","path":"permit/root",` +
			`"metrics":{"timer_server_handler_ns":1200},"result":{"allow":true}}`),
		Objects: models.AuditLogObjectsModel{
			OrganizationObject: json.RawMessage(`{"id":"00000000-0000-4000-8000-000000000001",` +
				`"key":"my-org","created_at":"2026-01-01T00:00:00Z",` +
				`"updated_at":"2026-01-01T00:00:00Z"}`),
			PDPConfigObject: json.RawMessage(`{"id":"00000000-0000-4000-8000-000000000601",` +
				`"name":"Production PDP"}`),
			UserObject: &models.UserObj{
				ID: resourceID(901), Key: "idp|alice", Email: "alice@example.com",
				FirstName: "Alice", LastName: "Smith",
				Attributes:    models.Attributes{"level": json.Number("3")},
				AssignedRoles: []string{"viewer"}, CreatedAt: at(auditLogFixtureTime),
				UpdatedAt: at("2026-10-02T08:00:00Z"),
			},
			ActionObject: &models.ActionObj{
				ID: resourceID(902), Key: "read", Name: "Read", CreatedAt: at(auditLogFixtureTime),
				UpdatedAt: at(auditLogFixtureTime),
			},
			ResourceTypeObject: &models.ResourceTypeObj{
				ID: resourceID(903), Key: "document", Name: "Document",
				Attributes: []models.ResourceAttributes{
					{Key: "private", Type: models.AttributeTypeBool},
				},
				CreatedAt: at(auditLogFixtureTime), UpdatedAt: at(auditLogFixtureTime),
			},
			TenantObject: &models.TenantObj{
				ID: resourceID(904), Key: "default", Name: "Default",
				Attributes: models.Attributes{"tier": "gold"}, CreatedAt: at(auditLogFixtureTime),
				UpdatedAt: at(auditLogFixtureTime),
			},
		},
	}
}

// auditLogFirst returns the first n audit logs of seq, stopping the loop there, or the first
// error it yields.
func auditLogFirst(seq iter.Seq2[models.AuditLogModel, error], n int,
) ([]models.AuditLogModel, error) {
	var got []models.AuditLogModel
	for log, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, log)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// auditLogCases returns the wire cases of the audit logs of an environment. Each sends one
// request to the API, along a route of /v2/pdps/ that the runner's scope fills, p and e, as
// WithScope gives it. List sends every filter, and Get an ID that holds a slash, which the path
// escapes.
func auditLogCases() []wireCase {
	const (
		logsRoute = "/v2/pdps/{proj_id}/{env_id}/audit_logs"
		logRoute  = logsRoute + "/{log_id}"
		logs      = "/v2/pdps/p/e/audit_logs"
	)
	listErr := projectError("list_audit_logs", http.MethodGet, logsRoute,
		http.StatusBadRequest, auditLogOverreachBody)
	listErr.Code, listErr.Title, listErr.RequestID = "PAGINATION_SIZE_OVERREACHED",
		"The request was invalid", "req-50"
	listErr.Detail = "Max number of results for this API is 10000"
	allErr := projectError("list_audit_logs", http.MethodGet, logsRoute,
		http.StatusForbidden, auditLogForbiddenBody)
	allErr.Code, allErr.Title, allErr.RequestID = "FORBIDDEN", "Forbidden", "req-51"
	allErr.Detail = "the API key may not read the audit logs of the environment"
	getErr := projectError("get_detailed_audit_log", http.MethodGet, logRoute,
		http.StatusNotFound, auditLogNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-52"
	getErr.Detail = "the audit log does not exist"
	return []wireCase{
		{
			Method: "AuditLogsService.List",
			Op:     "list_audit_logs",
			Route:  "GET " + logsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.AuditLogs.List(ctx, auditLogFilters())
			},
			Request: wireRequest{RawPath: logs, RawQuery: auditLogQuery},
			Reply:   mockpermit.Reply{Body: replyFixture("list_audit_logs.200.json")},
			WantResult: &permit.Page[models.AuditLogModel]{
				Data: auditLogsListed(), Page: 2, PerPage: 10, TotalCount: new(2),
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusBadRequest, Body: auditLogOverreachBody,
			},
			WantErr: listErr,
		},
		{
			Method: "AuditLogsService.All",
			Op:     "list_audit_logs",
			Route:  "GET " + logsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return auditLogFirst(c.AuditLogs.All(ctx, nil), 2)
			},
			Request:    wireRequest{RawPath: logs, RawQuery: "page=1&per_page=100"},
			Reply:      mockpermit.Reply{Body: replyFixture("list_audit_logs.200.json")},
			WantResult: auditLogsListed(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: auditLogForbiddenBody},
			WantErr:    allErr,
		},
		{
			Method: "AuditLogsService.Get",
			Op:     "get_detailed_audit_log",
			Route:  "GET " + logRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.AuditLogs.Get(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: logs + "/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_detailed_audit_log.200.json")},
			WantResult: auditLogDetailed(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: auditLogNotFoundBody},
			WantErr:    getErr,
		},
	}
}
