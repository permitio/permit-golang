// Package sdk is a fixture of the documentation checks in doc_test.go whose Examples are missing
// or do not run.
package sdk

// Client calls the API.
type Client struct {
	// Users reads and writes users.
	Users *UsersService

	// Roles reads and writes roles.
	Roles RolesAPI

	// Tenants reads and writes tenants.
	Tenants *TenantsService

	// Timeout has a type that the package does not declare.
	Timeout int

	admin *AdminAPI
}

// New returns a Client.
func New() *Client { return &Client{} }

// Check reports whether the request is allowed.
func (c *Client) Check() bool { return true }

// Allowed reports whether the request is allowed too.
func (c *Client) Allowed() bool { return true }

// RolesAPI reads and writes roles.
type RolesAPI struct{}

// List lists the roles.
func (RolesAPI) List() []string { return nil }

// UsersService reads and writes users.
type UsersService struct{}

// TenantsService reads and writes tenants.
type TenantsService struct{}

// AuditService reads the audit log; Client does not hold it.
type AuditService struct{}

// AdminAPI administers the account; only an unexported field of Client holds it.
type AdminAPI struct{}

// Settings configures a Client; the types of its fields are not services.
type Settings struct {
	// Admin administers the account.
	Admin *AdminAPI
}
