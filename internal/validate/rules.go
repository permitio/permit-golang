// Package validate holds the rules that the SDK checks a configuration or an argument against
// before it sends a request. Each rule has an ID that never changes and the message of the error
// it returns. testdata/validation-rules.txt lists the rules; TestValidationRules
// (validation_rules_test.go) keeps it in step with this package, and apisurface's diff mode
// (tools/cmd/apisurface) counts an ID that is new there as a breaking change, because a new rule
// rejects input that the SDK accepted before.
package validate

import "slices"

// Rule is one check of a configuration or an argument.
type Rule struct {
	// ID names the rule: lower-case words, each joined to the next by a dot, such as
	// config.api-key.missing. It never changes, so a new ID is a new rule.
	ID string

	// Message is the message of the error the rule returns, on one line.
	Message string
}

// The rules of a client's configuration, which permit.New checks. The error of a rule that an
// option or an environment variable breaks names the option or the variable before the message.
var (
	// APIKeyEmpty rejects an API key that an option gives as the empty string.
	APIKeyEmpty = Rule{
		ID: "config.api-key.empty",
		Message: "the API key is empty; pass a Permit API key, or leave the option out to " +
			"read PERMIT_API_KEY",
	}

	// APIKeyMissing rejects a configuration with no API key, from an option or from the
	// environment.
	APIKeyMissing = Rule{
		ID:      "config.api-key.missing",
		Message: "no API key; pass WithAPIKey, or set PERMIT_API_KEY",
	}

	// HTTPClientNil rejects a nil *http.Client.
	HTTPClientNil = Rule{
		ID: "config.http-client.nil",
		Message: "the *http.Client is nil; pass a client, or leave the option out to use one " +
			"of the SDK's own",
	}

	// OptionZero rejects an Option that no With function built, such as the zero Option.
	OptionZero = Rule{
		ID:      "config.option.zero",
		Message: "the Option is the zero value; build options with the With functions",
	}

	// RetryMaxBackoffNegative rejects a RetryPolicy whose MaxBackoff is less than zero.
	RetryMaxBackoffNegative = Rule{
		ID: "config.retry.max-backoff.negative",
		Message: "MaxBackoff is less than zero; give 0 or more, where 0 is the default, " +
			"5 seconds",
	}

	// RetryMaxRetriesNegative rejects a RetryPolicy whose MaxRetries is less than zero.
	RetryMaxRetriesNegative = Rule{
		ID:      "config.retry.max-retries.negative",
		Message: "MaxRetries is less than zero; give 0 or more, where 0 sends each request once",
	}

	// ScopeEnvironmentWithoutProject rejects a scope that names an environment and no project.
	ScopeEnvironmentWithoutProject = Rule{
		ID: "config.scope.environment-without-project",
		Message: "the environment is set and the project is empty; give the project that the " +
			"environment belongs to",
	}

	// ScopeProjectEmpty rejects a scope whose project is empty.
	ScopeProjectEmpty = Rule{
		ID: "config.scope.project-empty",
		Message: "the project is empty; give the key or ID of the project, or leave WithScope " +
			"out to use the API key's scope",
	}

	// TimeoutNotPositive rejects a timeout of zero or less.
	TimeoutNotPositive = Rule{
		ID:      "config.timeout.not-positive",
		Message: "the timeout must be more than zero, such as 30 * time.Second",
	}

	// URLEmpty rejects a URL that an option gives as the empty string.
	URLEmpty = Rule{
		ID: "config.url.empty",
		Message: "the URL is empty; leave the option out to read the environment variable or " +
			"to use the default",
	}

	// URLFragment rejects a URL with a fragment, the part from a #.
	URLFragment = Rule{
		ID:      "config.url.fragment",
		Message: "the URL has a fragment (#...); give the URL without it",
	}

	// URLHost rejects a URL without a host, such as http:///v2 or http:localhost.
	URLHost = Rule{
		ID:      "config.url.host",
		Message: "the URL has no host; give one, such as api.permit.io in https://api.permit.io",
	}

	// URLParse rejects a URL that net/url cannot parse.
	URLParse = Rule{
		ID: "config.url.parse",
		Message: "the URL does not parse; check that it starts with http:// or https://, that " +
			"its port is a number, and that it has no spaces and no % without two hex digits " +
			"after it",
	}

	// URLQuery rejects a URL with a query, the part from a ?.
	URLQuery = Rule{
		ID:      "config.url.query",
		Message: "the URL has a query (?...); give the URL without it",
	}

	// URLScheme rejects a URL whose scheme is not http or https, or that has none.
	URLScheme = Rule{
		ID:      "config.url.scheme",
		Message: "the URL must start with http:// or https://",
	}

	// URLUserinfo rejects a URL with a user name or a password before its host.
	URLUserinfo = Rule{
		ID: "config.url.userinfo",
		Message: "the URL has a user name or a password; give the API key with WithAPIKey or " +
			"PERMIT_API_KEY instead",
	}
)

// The rules of the arguments of a decision, which the SDK checks before it sends the request to
// the PDP. The error of such a rule names the method and the argument before the message.
var (
	// DecisionFilterEmpty rejects a filter of the permissions of a user without keys, such as
	// WithTenants(), which would send no filter, and so ask for the permissions in every tenant.
	DecisionFilterEmpty = Rule{
		ID: "decision.filter.empty",
		Message: "the filter has no keys; give at least one, or leave the option out to get the " +
			"permissions of every tenant, resource or resource type",
	}

	// DecisionOptionNil rejects a nil PermissionsOption.
	DecisionOptionNil = Rule{
		ID: "decision.option.nil",
		Message: "the PermissionsOption is nil; build options with the With functions, such as " +
			"WithTenants",
	}

	// DecisionOptionZero rejects a CheckOption that no With function built, such as the zero
	// CheckOption.
	DecisionOptionZero = Rule{
		ID: "decision.option.zero",
		Message: "the CheckOption is the zero value; build options with the With functions, such " +
			"as WithDecisionContext",
	}

	// DecisionResourceTypeEmpty rejects a resource without a type, which no PDP decides on.
	DecisionResourceTypeEmpty = Rule{
		ID: "decision.resource-type.empty",
		Message: "the resource type is empty; give the key of the resource type, such as " +
			"document, as the PDP rejects a request without one",
	}

	// DecisionURLEmpty rejects a check of an empty URL, which the PDP rejects.
	DecisionURLEmpty = Rule{
		ID: "decision.url.empty",
		Message: "the URL is empty; give the URL of the request to check, as the PDP rejects a " +
			"request without one",
	}

	// DecisionUserKeyEmpty rejects a user without a key, which no PDP decides on.
	DecisionUserKeyEmpty = Rule{
		ID: "decision.user-key.empty",
		Message: "the user key is empty; give the key of the user, as the PDP rejects a request " +
			"without one",
	}
)

// The rules of the arguments of a call, which the SDK checks before it sends the request. The
// error of such a rule names the argument before the message.
var (
	// ConditionSetUserSetResource rejects a create of a user set with a resource: a user set
	// takes no resource, only a resource set has one.
	ConditionSetUserSetResource = Rule{
		ID: "condition-set.user-set.resource",
		Message: "the user set has a ResourceID, and a user set has no resource; leave " +
			"ResourceID empty, or set Type to ConditionSetTypeResourceSet for a resource set",
	}

	// CopyResultWaitNegative rejects a CopyResultOptions whose Wait is less than zero.
	CopyResultWaitNegative = Rule{
		ID: "copy-result.wait.negative",
		Message: "Wait is less than zero; give 0 to read the task without a wait, or a wait of " +
			"up to 60 seconds",
	}

	// ListAttributeFilterNameEmpty rejects an attribute filter without the name of an attribute,
	// such as the zero AttributeFilter, which would send the parameter attr_ alone.
	ListAttributeFilterNameEmpty = Rule{
		ID: "list.attribute-filter.name-empty",
		Message: "the attribute filter names no attribute; build it with AttributeEquals and the " +
			"name of the attribute",
	}

	// ListAttributeFilterNotFinite rejects an attribute filter whose value is NaN or an
	// infinity, which a query cannot send as a number.
	ListAttributeFilterNotFinite = Rule{
		ID: "list.attribute-filter.not-finite",
		Message: "the value of the attribute filter is NaN or an infinity, which the API does " +
			"not read as a number; give a finite number",
	}

	// ListAttributeFilterRepeated rejects an attribute filter that names the attribute of an
	// earlier filter of the list, which would send the parameter attr_<name> twice.
	ListAttributeFilterRepeated = Rule{
		ID: "list.attribute-filter.repeated",
		Message: "the attribute filter names the attribute of an earlier filter; give one filter " +
			"for each attribute",
	}

	// ListPageNegative rejects a ListOptions whose Page is less than zero.
	ListPageNegative = Rule{
		ID: "list.page.negative",
		Message: "Page is less than zero; give 0 or 1 for the first page, or the number of a " +
			"later page",
	}

	// ListPerPageNegative rejects a ListOptions whose PerPage is less than zero.
	ListPerPageNegative = Rule{
		ID: "list.per-page.negative",
		Message: "PerPage is less than zero; give 0 to leave the number of items on a page to " +
			"the method, or a number from 1",
	}

	// PathAPIKeyIDScope rejects the API key ID scope in a call that acts on one API key: GET
	// /v2/api-key/scope is the route of the scope of the API key, so the request would reach
	// that route, or no route, in place of the key's.
	PathAPIKeyIDScope = Rule{
		ID: "path.api-key-id.scope",
		Message: "the API key ID is scope, which names the route of the scope of the API key, " +
			"not a key; pass the ID of an API key",
	}

	// PathSegmentDot rejects a path parameter that is . or .., which a server or a proxy on the
	// way may resolve as a step in the path, so that the request reaches another route.
	PathSegmentDot = Rule{
		ID: "path.segment.dot",
		Message: "the path parameter is . or .., which would change the route; pass the key or " +
			"ID of the object",
	}

	// PathSegmentEmpty rejects an empty path parameter, which would leave an empty segment in
	// the path, so that the request reaches another route or none.
	PathSegmentEmpty = Rule{
		ID:      "path.segment.empty",
		Message: "the path parameter is empty; pass the key or ID of the object",
	}
)

// The rules of the secret of a proxy config and of its auth mechanism, which a create and an
// update send. The error of such a rule names the operation and the secret, or the auth
// mechanism, before the message.
var (
	// ProxyConfigAuthMechanismMismatch rejects an auth mechanism of a proxy config that this
	// version of the SDK knows and that names another branch of the secret than the one that the
	// secret sets, such as Basic for a secret whose Bearer is set.
	ProxyConfigAuthMechanismMismatch = Rule{
		ID: "proxy-config.auth-mechanism.mismatch",
		Message: "the auth mechanism names another branch of the secret than the one that it " +
			"sets; leave AuthMechanism empty to send the mechanism of that branch, or set the " +
			"branch that it names",
	}

	// ProxyConfigSecretMany rejects a secret of a proxy config that sets more than one of its
	// branches, Basic, Bearer and Headers, which the API would read as one of them.
	ProxyConfigSecretMany = Rule{
		ID: "proxy-config.secret.many",
		Message: "the secret sets more than one of Basic, Bearer and Headers; set only the one " +
			"of the proxy config's auth mechanism",
	}

	// ProxyConfigSecretNone rejects a secret of a proxy config that holds no credential: it sets
	// none of its branches, such as the zero ProxySecret, or the branch that it sets is empty,
	// such as a Bearer token that is the empty string, Headers without a header, or a header whose
	// value is empty. The API requires the secret in a create and in an update.
	ProxyConfigSecretNone = Rule{
		ID: "proxy-config.secret.none",
		Message: "the secret holds no credential: it sets none of Basic, Bearer and Headers, or " +
			"the one that it sets is empty or has an empty value; set the one of the proxy " +
			"config's auth mechanism, with values that are not empty",
	}
)

// The rules of the scope of a call: the project and the environment that its operation acts on,
// which the SDK checks before it sends the request. The error of such a rule names the
// operation before the message.
var (
	// ScopeEnvironmentMissing rejects a call of an operation that acts on an environment when the
	// client's scope has no environment.
	ScopeEnvironmentMissing = Rule{
		ID: "scope.environment.missing",
		Message: "the operation acts on an environment, and the client's scope has none; pass " +
			"WithScope(project, environment) to New",
	}

	// ScopeProjectMissing rejects a call of an operation that acts on a project when the client's
	// scope has no project.
	ScopeProjectMissing = Rule{
		ID: "scope.project.missing",
		Message: "the operation acts on a project, and the client's scope has none; pass " +
			"WithScope(project, environment) to New",
	}
)

// The rules of a facts call that waits for sync, which the SDK checks before it sends the
// request. The error of such a rule names the operation before the message.
var (
	// WaitForSyncFactsViaAPI rejects a call that waits for sync from a client that sends facts
	// to the Permit API, which does not wait: only the PDP container does.
	WaitForSyncFactsViaAPI = Rule{
		ID: "wait-for-sync.facts-via-api",
		Message: "the call waits for sync, which only the PDP container does, and the client " +
			"sends facts to the API; pass WithFactsViaPDP to New, or make the call without the " +
			"wait",
	}

	// WaitForSyncNotWaiting rejects a call that waits for sync of an operation that the PDP
	// container answers without waiting.
	WaitForSyncNotWaiting = Rule{
		ID: "wait-for-sync.not-waiting",
		Message: "the PDP container does not wait for sync on this route; make the call without " +
			"the wait",
	}

	// WaitForSyncPolicyUnknown rejects a wait for sync whose SyncOption gives a policy other
	// than fail and ignore, such as the zero SyncOption.
	WaitForSyncPolicyUnknown = Rule{
		ID: "wait-for-sync.policy.unknown",
		Message: "the policy of the wait for sync is neither SyncPolicyFail nor " +
			"SyncPolicyIgnore; pass one of them to WithSyncPolicy, or leave the option out to " +
			"leave the policy to the PDP container",
	}

	// WaitForSyncTimeoutNegative rejects a wait for sync whose timeout is less than zero.
	WaitForSyncTimeoutNegative = Rule{
		ID:      "wait-for-sync.timeout-negative",
		Message: "the timeout of the wait for sync is less than zero; give 0 or more",
	}
)

// rules lists every rule, sorted by ID.
var rules = []Rule{
	ConditionSetUserSetResource,
	APIKeyEmpty,
	APIKeyMissing,
	HTTPClientNil,
	OptionZero,
	RetryMaxBackoffNegative,
	RetryMaxRetriesNegative,
	ScopeEnvironmentWithoutProject,
	ScopeProjectEmpty,
	TimeoutNotPositive,
	URLEmpty,
	URLFragment,
	URLHost,
	URLParse,
	URLQuery,
	URLScheme,
	URLUserinfo,
	CopyResultWaitNegative,
	DecisionFilterEmpty,
	DecisionOptionNil,
	DecisionOptionZero,
	DecisionResourceTypeEmpty,
	DecisionURLEmpty,
	DecisionUserKeyEmpty,
	ListAttributeFilterNameEmpty,
	ListAttributeFilterNotFinite,
	ListAttributeFilterRepeated,
	ListPageNegative,
	ListPerPageNegative,
	PathAPIKeyIDScope,
	PathSegmentDot,
	PathSegmentEmpty,
	ProxyConfigAuthMechanismMismatch,
	ProxyConfigSecretMany,
	ProxyConfigSecretNone,
	ScopeEnvironmentMissing,
	ScopeProjectMissing,
	WaitForSyncFactsViaAPI,
	WaitForSyncNotWaiting,
	WaitForSyncPolicyUnknown,
	WaitForSyncTimeoutNegative,
}

// Rules returns every rule, sorted by ID.
func Rules() []Rule {
	return slices.Clone(rules)
}
