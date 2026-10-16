// Package sdk is a fixture of the documentation checks in doc_test.go whose Client is not a
// struct type, so the service-type rule cannot read its fields.
package sdk

// Client calls the API.
type Client = client

// client holds the services.
type client struct {
	// Users reads and writes users.
	Users *UsersAPI
}

// UsersAPI reads and writes users.
type UsersAPI struct{}

// AuditService reads the audit log.
type AuditService struct{}
