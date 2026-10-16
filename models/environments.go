package models

// EnvironmentRead is an environment of a project, as the API returns it after a create or an
// update. A list and a get return an [EnvironmentReadWithEmailConfig], which adds the email
// configuration of the environment.
type EnvironmentRead struct {
	// Key is the environment's key, a URL-friendly name such as production, which a call may give
	// in place of its ID.
	Key string `json:"key"`

	// ID is the environment's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the environment's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the environment's project.
	ProjectID string `json:"project_id"`

	// CreatedAt is when the environment was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the environment was last changed.
	UpdatedAt Time `json:"updated_at"`

	// AVPPolicyStoreID is the avp_policy_store_id of the reply, which the API's documentation
	// does not describe, or empty when the reply holds none.
	AVPPolicyStoreID string `json:"avp_policy_store_id,omitzero"`

	// Name is the environment's name.
	Name string `json:"name"`

	// Description is the environment's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// CustomBranchName is the name of the branch of the environment in the policy Git
	// repository of its project, or empty when it has none.
	CustomBranchName string `json:"custom_branch_name,omitzero"`

	// JWKS configures the JSON Web Key Sets with which Permit Elements check the tokens of a
	// login from a front end, or is nil when the reply holds none.
	JWKS *JwksConfig `json:"jwks,omitzero"`

	// JWKSUnpinned is the jwks_unpinned list of the reply, which the API's documentation does
	// not describe, or nil when the reply holds none.
	JWKSUnpinned []string `json:"jwks_unpinned,omitzero"`

	// Settings are the environment's settings, or nil when the reply holds none.
	Settings Attributes `json:"settings,omitzero"`
}

// EnvironmentReadWithEmailConfig is an environment of a project, as a list and a get return it:
// an [EnvironmentRead], whose fields it promotes, with the ID of its email configuration.
type EnvironmentReadWithEmailConfig struct {
	// EnvironmentRead holds the fields of the environment that every reply holds.
	EnvironmentRead

	// EmailConfiguration is the ID of the environment's email configuration.
	EmailConfiguration string `json:"email_configuration"`
}

// EnvironmentCreate is an environment to create, with EnvironmentsService.Create of the root
// package or as one of the [ProjectCreate.InitialEnvironments] of a new project. Key and Name
// are required; each other field that is its zero value (nil for a pointer or a map, empty for a
// string) is left out of the request.
type EnvironmentCreate struct {
	// Key is the environment's key, a URL-friendly name of letters, digits, - and _, such as
	// production. It cannot be changed later.
	Key string `json:"key"`

	// Name is the environment's name.
	Name string `json:"name"`

	// Description is the environment's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// CustomBranchName is the name of the branch of the environment in the policy Git
	// repository of its project. Empty leaves it out.
	CustomBranchName string `json:"custom_branch_name,omitzero"`

	// JWKS configures the JSON Web Key Sets with which Permit Elements check the tokens of a
	// login from a front end. Nil leaves it out.
	JWKS *JwksConfig `json:"jwks,omitzero"`

	// Settings are the environment's settings. Nil leaves them out; an empty map that is not nil
	// sends {}.
	Settings Attributes `json:"settings,omitzero"`
}

// EnvironmentUpdate is a change to an environment: the request holds only the fields that it
// sets, and the environment keeps the others, so the zero EnvironmentUpdate changes nothing.
type EnvironmentUpdate struct {
	// Name is the environment's new name. Empty leaves it out, and the environment keeps its
	// name.
	Name string `json:"name,omitzero"`

	// Description is the environment's new description: unset, the zero value, leaves it out,
	// and the environment keeps its description; [Null] sends null, and [Some] sends the value.
	Description Optional[string] `json:"description,omitzero"`

	// CustomBranchName is the name of the environment's new branch in the policy Git repository
	// of its project: unset leaves it out, [Null] sends null, and [Some] sends the value.
	CustomBranchName Optional[string] `json:"custom_branch_name,omitzero"`

	// JWKS is the environment's new configuration of its JSON Web Key Sets: unset leaves it out,
	// and the environment keeps its configuration; [Null] sends null, and [Some] sends the
	// configuration as a whole.
	JWKS Optional[JwksConfig] `json:"jwks,omitzero"`

	// Settings are the environment's new settings: unset leaves them out, and the environment
	// keeps its settings; [Null] sends null, and [Some] sends the settings as a whole, an empty
	// map as {}.
	Settings Optional[Attributes] `json:"settings,omitzero"`
}

// JwksConfig says where the JSON Web Key Sets of an environment come from. Each field that is
// its zero value (unset for Audience and Issuer, nil for JWKS, 0 for TTL, empty for URL) is left
// out of the request.
type JwksConfig struct {
	// Audience is the audience that the aud claim of a token is expected to name: one string,
	// from [OneString], or a list, from [StringList]. Unset leaves it out. The API's schema says
	// that it is stored, but that the login of Permit Elements does not check it yet.
	Audience StringOrList `json:"audience,omitzero"`

	// Issuer is the issuer that the iss claim of a token is expected to name: one string, from
	// [OneString], or a list, from [StringList]. Unset leaves it out. The API's schema says that
	// it is stored, but that the login of Permit Elements does not check it yet.
	Issuer StringOrList `json:"issuer,omitzero"`

	// JWKS holds the key sets themselves, in place of a URL. Nil leaves it out.
	JWKS *JwksObj `json:"jwks,omitzero"`

	// TTL is how long, in seconds, the key sets of URL are kept before they are read again; 0
	// leaves it out, and the API keeps them for 600 seconds.
	TTL int `json:"ttl,omitzero"`

	// URL is the https URL that the key sets are read from. Empty leaves it out.
	URL string `json:"url,omitzero"`
}

// JwksObj is a JSON Web Key Set.
type JwksObj struct {
	// Keys are the keys of the set, each a JSON Web Key.
	Keys []Attributes `json:"keys"`
}

// EnvironmentStats is an environment of a project with its statistics and the configurations of
// its PDPs, as EnvironmentsService.Stats of the root package returns it: an [EnvironmentRead],
// whose fields it promotes, with PDPConfigs and Stats. Printed with fmt or slog, or encoded as
// JSON, it shows a marker in place of each credential of its PDPs.
type EnvironmentStats struct {
	// EnvironmentRead holds the fields of the environment that every reply holds.
	EnvironmentRead

	// PDPConfigs are the configurations of the PDPs of the environment.
	PDPConfigs []PDPConfigRead `json:"pdp_configs"`

	// Stats are the statistics of the environment.
	Stats EnvironmentStatistics `json:"stats"`
}

// PDPConfigRead is the configuration of a PDP of an environment, as [EnvironmentStats] holds it.
// ClientSecret and OPALServerAccessToken are credentials, each a [Secret], which prints a marker
// in place of the credential: read it with [Secret.Value].
type PDPConfigRead struct {
	// ID is the configuration's ID, a UUID.
	ID string `json:"id"`

	// Name is the configuration's name, or empty when it has none.
	Name string `json:"name,omitzero"`

	// OrganizationID is the ID of the configuration's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the configuration's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the configuration's environment.
	EnvironmentID string `json:"environment_id"`

	// ClientSecret is the client secret of the PDP, a credential.
	ClientSecret Secret `json:"client_secret"`

	// DebugAuditLogs is the debug_audit_logs flag of the configuration, or nil when the reply
	// holds none; the API's schema gives it the default true.
	DebugAuditLogs *bool `json:"debug_audit_logs,omitzero"`

	// MinPDPVersion is the min_pdp_version of the configuration, a semantic version such as
	// 0.5.0, or empty when the reply holds none.
	MinPDPVersion string `json:"min_pdp_version,omitzero"`

	// NumShards is the num_shards of the configuration, or 0 when the reply holds none.
	NumShards int `json:"num_shards,omitzero"`

	// OPALServerAccessToken is the opal_server_access_token of the reply, a credential, which
	// the API's documentation does not describe, or the empty Secret when the reply holds none.
	OPALServerAccessToken Secret `json:"opal_server_access_token,omitzero"`
}

// EnvironmentStatistics are the statistics of an environment, as [EnvironmentStats] holds them.
// The reply also lists the members of the organization, which EnvironmentStatistics leaves out,
// as the SDK covers no account administration of the members of the organization.
type EnvironmentStatistics struct {
	// Roles is the number of roles of the environment.
	Roles int `json:"roles"`

	// Users is the number of users of the environment.
	Users int `json:"users"`

	// Policies is the number of policies of the environment.
	Policies int `json:"policies"`

	// Resources is the number of resources of the environment.
	Resources int `json:"resources"`

	// Tenants is the number of tenants of the environment.
	Tenants int `json:"tenants"`

	// HasDecisionLogs reports whether the environment has decision logs.
	HasDecisionLogs bool `json:"has_decision_logs"`

	// MAU is the number of monthly active users of the environment.
	MAU int `json:"mau"`
}

// EnvironmentCopy is a copy of an environment into another, which EnvironmentsService.Copy and
// EnvironmentsService.CopyAsync of the root package send. TargetEnv is required; ConflictStrategy
// and Scope are left out of the request when they are their zero values, and the API's schema
// gives them its defaults: fail, and a scope without filters.
type EnvironmentCopy struct {
	// TargetEnv is the environment that the copy goes into: an existing one, or a new one that
	// the copy creates.
	TargetEnv EnvironmentCopyTarget `json:"target_env"`

	// ConflictStrategy is what a copy into an existing environment does with an object that the
	// environment has already: [EnvironmentCopyConflictStrategyFail] or
	// [EnvironmentCopyConflictStrategyOverwrite]. Empty leaves it out. A value that this version
	// of the SDK does not know is sent as it is.
	ConflictStrategy EnvironmentCopyConflictStrategy `json:"conflict_strategy,omitzero"`

	// Scope picks the objects that the copy copies. The zero EnvironmentCopyScope leaves it out.
	Scope EnvironmentCopyScope `json:"scope,omitzero"`
}

// EnvironmentCopyTarget is the environment that a copy goes into: set Existing to copy into an
// environment of the project, or New to create one and copy into it. Each field that is its zero
// value (empty for Existing, nil for New) is left out of the request.
type EnvironmentCopyTarget struct {
	// Existing names an existing environment to copy into, by its key or its ID. Empty leaves it
	// out.
	Existing string `json:"existing,omitzero"`

	// New is the environment to create and copy into, with a key that no environment of the
	// project has. Nil leaves it out.
	New *EnvironmentCreate `json:"new,omitzero"`
}

// EnvironmentCopyScope picks the objects of each kind that a copy copies. Each field that is the
// zero EnvironmentCopyScopeFilters is left out of the request.
type EnvironmentCopyScope struct {
	// Resources filters the resources.
	Resources EnvironmentCopyScopeFilters `json:"resources,omitzero"`

	// Roles filters the roles.
	Roles EnvironmentCopyScopeFilters `json:"roles,omitzero"`

	// UserSets filters the user sets.
	UserSets EnvironmentCopyScopeFilters `json:"user_sets,omitzero"`

	// ResourceSets filters the resource sets.
	ResourceSets EnvironmentCopyScopeFilters `json:"resource_sets,omitzero"`

	// CustomPolicies filters the custom policies.
	CustomPolicies EnvironmentCopyScopeFilters `json:"custom_policies,omitzero"`
}

// EnvironmentCopyScopeFilters filter the objects of one kind that a copy copies, by their keys,
// in which * is a wildcard. A nil list is left out of the request, and an empty list that is not
// nil sends [].
type EnvironmentCopyScopeFilters struct {
	// Include are the objects to copy, such as admin or doc-*.
	Include []string `json:"include,omitzero"`

	// Exclude are the objects not to copy.
	Exclude []string `json:"exclude,omitzero"`
}

// EnvironmentCopyConflictStrategy is what a copy into an existing environment does with an
// object that the environment has already. It is an open enum, as [AttributeType] describes: a
// request sends a value that this version of the SDK does not know as it is.
type EnvironmentCopyConflictStrategy string

// The values of EnvironmentCopyConflictStrategy that this version of the SDK knows.
const (
	// EnvironmentCopyConflictStrategyFail is fail, the API's default.
	EnvironmentCopyConflictStrategyFail EnvironmentCopyConflictStrategy = "fail"

	// EnvironmentCopyConflictStrategyOverwrite is overwrite.
	EnvironmentCopyConflictStrategyOverwrite EnvironmentCopyConflictStrategy = "overwrite"
)

// Known reports whether s is one of the constants of EnvironmentCopyConflictStrategy, the values
// that this version of the SDK knows.
func (s EnvironmentCopyConflictStrategy) Known() bool {
	switch s {
	case EnvironmentCopyConflictStrategyFail, EnvironmentCopyConflictStrategyOverwrite:
		return true
	default:
		return false
	}
}

// TaskResultEnvironmentRead is the task of a copy of an environment that runs in the background,
// as EnvironmentsService.CopyAsync and EnvironmentsService.CopyResult of the root package return
// it: its ID and its status, with the environment that it copied into, or the error that ended
// it, when the reply holds them.
type TaskResultEnvironmentRead struct {
	// TaskID is the task's ID, which EnvironmentsService.CopyResult takes.
	TaskID string `json:"task_id"`

	// Status is the task's status, such as [TaskStatusProcessing] or [TaskStatusSuccess].
	Status TaskStatus `json:"status"`

	// Result is the environment that the copy went into, or nil when the reply holds none.
	Result *EnvironmentRead `json:"result,omitzero"`

	// Error is the error that ended the task, or nil when the reply holds none.
	Error *ErrorDetails `json:"error,omitzero"`
}
