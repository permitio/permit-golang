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
