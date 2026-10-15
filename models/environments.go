package models

// EnvironmentCreate is an environment to create, such as one of the
// [ProjectCreate.InitialEnvironments] of a new project. Key and Name are required; each other
// field that is its zero value (nil for a pointer or a map, empty for a string) is left out of
// the request.
type EnvironmentCreate struct {
	// Key is the environment's key, a URL-friendly name of letters, digits, - and _, such as
	// production.
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

	// Settings are the environment's settings. Nil leaves them out.
	Settings Attributes `json:"settings,omitzero"`
}

// JwksConfig says where the JSON Web Key Sets of an environment come from. Each field that is
// its zero value (nil for Audience, Issuer and JWKS, 0 for TTL, empty for URL) is left out of the
// request.
type JwksConfig struct {
	// Audience is the audience, or the audiences, that a token is expected to name: a string, or
	// a list of strings as a []string. Nil leaves it out.
	Audience any `json:"audience,omitzero"`

	// Issuer is the issuer, or the issuers, that a token is expected to name: a string, or a list
	// of strings as a []string. Nil leaves it out.
	Issuer any `json:"issuer,omitzero"`

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
