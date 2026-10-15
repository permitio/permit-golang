package models

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// AuthMechanism is how the Permit proxy authenticates the requests that it sends on for a proxy
// config, with the config's [ProxySecret]: Bearer, Basic or Headers.
type AuthMechanism string

// The values of AuthMechanism that this version of the SDK knows.
const (
	// AuthMechanismBearer sends the Bearer token of the secret in the Authorization header: the
	// mechanism of a proxy config that leaves it out.
	AuthMechanismBearer AuthMechanism = "Bearer"

	// AuthMechanismBasic sends the Basic credential of the secret, user:password, in the
	// Authorization header.
	AuthMechanismBasic AuthMechanism = "Basic"

	// AuthMechanismHeaders adds the Headers of the secret to the request.
	AuthMechanismHeaders AuthMechanism = "Headers"
)

// Known reports whether m is one of the constants of AuthMechanism, the values that this version
// of the SDK knows.
func (m AuthMechanism) Known() bool {
	switch m {
	case AuthMechanismBearer, AuthMechanismBasic, AuthMechanismHeaders:
		return true
	default:
		return false
	}
}

// Methods is the HTTP method of the requests that a mapping rule matches, in lower case, such as
// get; the API names the type Methods.
type Methods string

// The values of Methods that this version of the SDK knows.
const (
	// MethodsGet is GET.
	MethodsGet Methods = "get"

	// MethodsPost is POST.
	MethodsPost Methods = "post"

	// MethodsPut is PUT.
	MethodsPut Methods = "put"

	// MethodsPatch is PATCH.
	MethodsPatch Methods = "patch"

	// MethodsDelete is DELETE.
	MethodsDelete Methods = "delete"

	// MethodsHead is HEAD.
	MethodsHead Methods = "head"

	// MethodsOptions is OPTIONS.
	MethodsOptions Methods = "options"
)

// Known reports whether m is one of the constants of Methods, the values that this version of
// the SDK knows.
func (m Methods) Known() bool {
	switch m {
	case MethodsGet, MethodsPost, MethodsPut, MethodsPatch, MethodsDelete, MethodsHead,
		MethodsOptions:
		return true
	default:
		return false
	}
}

// URLType is how a mapping rule matches the URL of a request with its own URL: as a regular
// expression with URLTypeRegex, or, empty, as the API matches a URL that has no type.
type URLType string

// The values of URLType that this version of the SDK knows.
const (
	// URLTypeRegex reads the URL of the rule as a regular expression.
	URLTypeRegex URLType = "regex"
)

// Known reports whether t is one of the constants of URLType, the values that this version of
// the SDK knows.
func (t URLType) Known() bool {
	return t == URLTypeRegex
}

// MappingRule maps the requests to a URL with an HTTP method to a resource and an action, which
// the PDP container's check of a URL decides on, as a create of a proxy config sends it and as
// the API returns it.
type MappingRule struct {
	// URL is the URL that the rule matches the URL of a request with, such as
	// https://api.example.com/documents/{id}.
	URL string `json:"url"`

	// URLType is how the rule matches URL, such as a regular expression, or empty when the reply
	// holds none. Empty leaves it out of a request.
	URLType URLType `json:"url_type,omitzero"`

	// HTTPMethod is the HTTP method that the rule matches.
	HTTPMethod Methods `json:"http_method"`

	// Resource is the key of the resource that a matched request acts on.
	Resource string `json:"resource"`

	// Headers are the headers that the rule matches the headers of a request with, by name. Nil
	// leaves them out of a request, and an empty map that is not nil sends {}.
	Headers map[string]string `json:"headers,omitzero"`

	// Action is the action that a matched request does on the resource, or empty when the reply
	// holds none. Empty leaves it out of a request.
	Action string `json:"action,omitzero"`

	// Priority is the priority of the rule: the higher it is, the more the rule takes
	// precedence. Nil leaves it out of a request, or says that the reply holds none.
	Priority *int `json:"priority,omitzero"`
}

// MappingRuleUpdate is a mapping rule of an update of a proxy config: a rule to add or to change,
// or with ShouldDelete, the rule to remove.
type MappingRuleUpdate struct {
	// URL is the URL that the rule matches the URL of a request with.
	URL string `json:"url"`

	// URLType is how the rule matches URL, such as a regular expression. Empty leaves it out.
	URLType URLType `json:"url_type,omitzero"`

	// HTTPMethod is the HTTP method that the rule matches.
	HTTPMethod Methods `json:"http_method"`

	// Resource is the key of the resource that a matched request acts on.
	Resource string `json:"resource"`

	// Headers are the headers that the rule matches the headers of a request with, by name. Nil
	// leaves them out, and an empty map that is not nil sends {}.
	Headers map[string]string `json:"headers,omitzero"`

	// Action is the action that a matched request does on the resource. Empty leaves it out.
	Action string `json:"action,omitzero"`

	// Priority is the priority of the rule: the higher it is, the more the rule takes
	// precedence. Nil leaves it out.
	Priority *int `json:"priority,omitzero"`

	// ShouldDelete removes the rule from the proxy config, when it is true: the only way that an
	// update removes a rule. False leaves it out, as the API's default is false.
	ShouldDelete bool `json:"should_delete,omitzero"`
}

// The errors of the JSON of a ProxySecret. None holds a credential.
var (
	// errProxySecretMany is the error of encoding a ProxySecret that sets two or three branches.
	errProxySecretMany = errors.New("models: the ProxySecret sets more than one of Basic, " +
		"Bearer and Headers; set only the one of the proxy config's auth mechanism")

	// errNotAProxySecret is the error of decoding a JSON value that is not a string, an object
	// of strings or null into a ProxySecret. It does not hold the value, which may be a
	// credential.
	errNotAProxySecret = errors.New("models: a ProxySecret is read from a JSON string, a JSON " +
		"object of strings or null")
)

// ProxySecret is the credential of a proxy config, which the Permit proxy adds to the requests
// that it sends on: one of three branches, as the config's [AuthMechanism] names it.
//
//   - Basic is a credential of the form user:password, for [AuthMechanismBasic];
//   - Bearer is a token, for [AuthMechanismBearer];
//   - Headers are headers to add to the request, each value by the header's name, for
//     [AuthMechanismHeaders].
//
// A branch is set when it is not nil: Basic or Bearer, and Headers, also when it is empty. The
// SDK sends the credentials themselves, which [Secret.Value] returns, in the body of a request
// that creates or changes a proxy config, only for a ProxySecret that sets exactly one branch,
// with a credential that is not empty: a Basic or a Bearer that is not the empty string, or
// Headers with at least one header, each with a value that is not empty. The request sends the
// auth mechanism of that branch when the config's AuthMechanism is empty.
//
// Each credential is a [Secret], which does not print, so neither does a ProxySecret: String,
// GoString, Format with every verb and MarshalJSON show the branch that is set, the names of the
// headers, and the marker, the word REDACTED in square brackets, in place of each credential.
//
// A client that sends a ProxySecret, or reads one from a reply, adds each of its credentials, the
// value of each header too, to the credentials that it replaces with the marker wherever they
// appear in its later errors and log lines, for as long as the client lives. So a secret of
// headers should hold only credentials: the value of a header that is not one, such as a region
// or a version, is replaced in each of those too, also inside a request ID, a path or a status
// code.
//
// The API sends the secret back as it stores it: a JSON object for Headers, and a JSON string
// for Basic and Bearer, which the auth mechanism of the config tells apart.
// [ProxySecret.UnmarshalJSON] reads a JSON string as Bearer, and [ProxyConfigRead] reads it as
// Basic when its AuthMechanism is Basic.
type ProxySecret struct {
	// Basic is the credential of Basic auth, of the form user:password, or nil.
	Basic *Secret

	// Bearer is the token of Bearer auth, or nil.
	Bearer *Secret

	// Headers are the headers to add, each value by the header's name, or nil.
	Headers map[string]Secret
}

// branches returns the number of the branches of s that are set.
func (s ProxySecret) branches() int {
	n := 0
	if s.Basic != nil {
		n++
	}
	if s.Bearer != nil {
		n++
	}
	if s.Headers != nil {
		n++
	}
	return n
}

// String returns the branches of s that are set, in braces, each as its name, a colon and its
// value: the marker, the word REDACTED in square brackets, for Basic and Bearer, and for Headers
// the map of the names of the headers to the marker, as fmt prints a map; {} when s sets none.
// The Example of ProxyConfigsService in the root package shows it.
func (s ProxySecret) String() string {
	var parts []string
	if s.Basic != nil {
		parts = append(parts, "Basic:"+s.Basic.String())
	}
	if s.Bearer != nil {
		parts = append(parts, "Bearer:"+s.Bearer.String())
	}
	if s.Headers != nil {
		parts = append(parts, fmt.Sprintf("Headers:%v", s.Headers))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// GoString returns what [ProxySecret.String] returns, for the %#v verb.
func (s ProxySecret) GoString() string { return s.String() }

// Format writes what [ProxySecret.String] returns, for every verb, flag and width, so fmt, and
// slog's text handler with a model that holds s, never write a credential.
func (s ProxySecret) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, s.String()) }

// MarshalJSON returns the branch that s sets with the marker in place of each credential: a JSON
// string for Basic and Bearer, and a JSON object of the headers for Headers, so encoding/json, and
// slog's JSON handler with a model that holds s, never write a credential. It returns null when s
// sets no branch, such as the zero ProxySecret, as [ProxySecret.UnmarshalJSON] reads null. It
// fails when s sets more than one branch; the error holds no credential. A request of the SDK
// never holds what MarshalJSON returns: it sends the credentials themselves, and a secret that
// sets no branch, or more than one, is not sent.
func (s ProxySecret) MarshalJSON() ([]byte, error) {
	switch {
	case s.branches() == 0:
		return []byte(jsonNull), nil
	case s.branches() > 1:
		return nil, errProxySecretMany
	case s.Basic != nil:
		return s.Basic.MarshalJSON()
	case s.Bearer != nil:
		return s.Bearer.MarshalJSON()
	default:
		return wire.Encode(s.Headers)
	}
}

// UnmarshalJSON sets s to the branch that data holds: a JSON object of strings as Headers, and a
// JSON string as Bearer, as the API's default mechanism is Bearer; null sets s to no branch. Any
// other JSON value, such as a number, or an object with a value that is not a string, is an error
// that holds no part of data, and leaves s as it is.
func (s *ProxySecret) UnmarshalJSON(data []byte) error {
	switch {
	case string(data) == jsonNull:
		*s = ProxySecret{}
		return nil
	case strings.HasPrefix(string(data), "{"):
		var headers map[string]Secret
		if wire.Decode(data, &headers) != nil {
			return errNotAProxySecret
		}
		*s = ProxySecret{Headers: headers}
		return nil
	case strings.HasPrefix(string(data), `"`):
		var token Secret
		if wire.Decode(data, &token) != nil {
			return errNotAProxySecret
		}
		*s = ProxySecret{Bearer: &token}
		return nil
	default:
		return errNotAProxySecret
	}
}

// ProxyConfigCreate is a proxy config to create in an environment: the credential that the Permit
// proxy adds to the requests that it sends on, and the mapping rules that map a URL and an HTTP
// method to a resource and an action. Key, Name, Secret and AuthMechanism are always sent, the
// last as its field says; MappingRules is left out when it is nil.
type ProxyConfigCreate struct {
	// Key is the config's key, a URL-friendly name such as payments, which a call may give in place
	// of its ID.
	Key string `json:"key"`

	// Name is the config's name, such as Payments API.
	Name string `json:"name"`

	// AuthMechanism is how the proxy sends Secret. A create always sends it: empty sends the
	// mechanism of the branch that Secret sets, Basic, Bearer or Headers. A mechanism of this
	// version of the SDK that names another branch is an error, and the request is not sent; one
	// that the SDK does not know is sent as it is.
	AuthMechanism AuthMechanism `json:"auth_mechanism,omitzero"`

	// Secret is the credential, which sets exactly one branch, with a credential that is not
	// empty, as [ProxySecret] says: a request whose secret does not is not sent.
	Secret ProxySecret `json:"secret"`

	// MappingRules are the config's mapping rules. Nil leaves them out, and an empty slice that
	// is not nil sends [].
	MappingRules []MappingRule `json:"mapping_rules,omitzero"`
}

// ProxyConfigUpdate is a change to a proxy config. The API answers 422 to an update without the
// secret, so Secret is always sent, with the auth mechanism of its branch, and must set exactly
// one branch; Name and MappingRules are left out when they are their zero values.
type ProxyConfigUpdate struct {
	// Name is the config's new name. Empty leaves it out.
	Name string `json:"name,omitzero"`

	// AuthMechanism is how the proxy sends Secret. An update always sends it: empty sends the
	// mechanism of the branch that Secret sets, Basic, Bearer or Headers. A mechanism of this
	// version of the SDK that names another branch is an error, and the request is not sent; one
	// that the SDK does not know is sent as it is.
	AuthMechanism AuthMechanism `json:"auth_mechanism,omitzero"`

	// Secret is the credential, which sets exactly one branch, with a credential that is not
	// empty, as [ProxySecret] says: a request whose secret does not is not sent.
	Secret ProxySecret `json:"secret"`

	// MappingRules are the rules to add, to change or, with ShouldDelete, to remove. The API
	// keeps each rule that it stores and that no rule here removes. Nil leaves them out, and an
	// empty slice that is not nil sends [].
	MappingRules []MappingRuleUpdate `json:"mapping_rules,omitzero"`
}

// ProxyConfigRead is a proxy config of an environment, as the API returns it, with its secret:
// each reply holds the credentials of the secret. Its Secret prints the marker in place of each
// credential; read a credential with [Secret.Value].
type ProxyConfigRead struct {
	// Key is the config's key, a URL-friendly name such as payments, which a call may give in place
	// of its ID.
	Key string `json:"key"`

	// ID is the config's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the config's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the config's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the config's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the config was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the config was last changed.
	UpdatedAt Time `json:"updated_at"`

	// Name is the config's name.
	Name string `json:"name"`

	// AuthMechanism is how the proxy sends Secret, or empty when the reply holds none, which the
	// API's documentation gives as Bearer.
	AuthMechanism AuthMechanism `json:"auth_mechanism,omitzero"`

	// Secret is the config's credential, as [ProxyConfigRead.UnmarshalJSON] reads it.
	Secret ProxySecret `json:"secret"`

	// MappingRules are the config's mapping rules, or nil when the reply holds none.
	MappingRules []MappingRule `json:"mapping_rules,omitzero"`
}

// UnmarshalJSON reads data, a JSON object, into c as encoding/json does, with the secret as
// [ProxySecret.UnmarshalJSON] reads it, except that a JSON string is Basic when AuthMechanism is
// Basic. null leaves c as it is, as encoding/json does; a value that does not decode is an error,
// and leaves c as it is.
func (c *ProxyConfigRead) UnmarshalJSON(data []byte) error {
	if string(data) == jsonNull {
		return nil
	}
	type plain ProxyConfigRead
	var read plain
	if err := wire.Decode(data, &read); err != nil {
		return err
	}
	if read.AuthMechanism == AuthMechanismBasic && read.Secret.Bearer != nil {
		read.Secret = ProxySecret{Basic: read.Secret.Bearer}
	}
	*c = ProxyConfigRead(read)
	return nil
}
