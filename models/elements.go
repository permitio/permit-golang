package models

// UserLoginRequestInput names the user to log in to Permit Elements as, and the tenant that the
// login gives access to, as ElementsService.LoginAs of the root package sends it. Both fields are
// required and always sent: an empty one is sent as the empty string.
type UserLoginRequestInput struct {
	// UserID is the key or the ID of the user to log in as.
	UserID string `json:"user_id"`

	// TenantID is the key or the ID of the tenant that the login gives access to.
	TenantID string `json:"tenant_id"`
}

// EmbeddedLoginRequestOutput is the reply to a login to Permit Elements: the URL to send the
// user's browser to, which completes the login, and the tokens of the login. RedirectURL, Token
// and ElementBearerToken are each a [Secret], which prints a marker in place of the credential:
// read it with [Secret.Value]. When the login failed, Error and ErrorCode say why.
type EmbeddedLoginRequestOutput struct {
	// Error is the message of the error when the login failed, or empty when the reply holds
	// none.
	Error string `json:"error,omitzero"`

	// ErrorCode is the code of the error when the login failed, or 0 when the reply holds none.
	ErrorCode int `json:"error_code,omitzero"`

	// Token is the token of the login, or the empty Secret when the reply holds none.
	Token Secret `json:"token,omitzero"`

	// Extra is the extra data of the login, or empty when the reply holds none.
	Extra string `json:"extra,omitzero"`

	// RedirectURL is the URL to send the user's browser to, to complete the login. It is a Secret,
	// as the URL that completes a login can hold its token.
	RedirectURL Secret `json:"redirect_url"`

	// ElementBearerToken is the bearer token of the login, or the empty Secret when the reply
	// holds none.
	ElementBearerToken Secret `json:"element_bearer_token,omitzero"`
}
