// Package sdk is a fixture of the documentation checks in doc_test.go that passes them all. It
// links the [Client], its field [Client.Users], the method [UsersService.Get], the pointer
// [*Client], the qualified [sdk.Client], the package [models], the type [models.User] and its
// field [models.User.Key], and the standard library's [context.Context] and [net/http.Client].
// Brackets that are not doc links stay text: a[i], []string, [a b] and [1].
package sdk

import (
	"context"
	"net/http"

	"example.com/sdk/models"
)

// Version is the version of the fixture.
const Version = "1.0.0"

// Client calls the API with its [http.Client].
type Client struct {
	// Users reads and writes users.
	Users *UsersService

	// HTTP sends the requests.
	HTTP *http.Client

	apiKey string
}

// New returns a [Client].
func New() *Client {
	return &Client{Users: &UsersService{}}
}

// Check reports whether the user may act. The container PDP only answers it; see
// [Client.Users]. It needs an environment-level API key.
func (c *Client) Check(ctx context.Context, user string) (bool, error) {
	return user != "", ctx.Err()
}

// UsersService reads and writes users.
type UsersService struct{}

// Get returns the user with the key, and waits for
// sync. It needs a project-level API key.
func (s *UsersService) Get(ctx context.Context, key string) (models.User, error) {
	return models.User{Key: key}, ctx.Err()
}

// Create creates the user with the key. It needs an environment-level API key, and is sent
// again only after a 429.
func (s *UsersService) Create(ctx context.Context, key string) (models.User, error) {
	return models.User{Key: key}, ctx.Err()
}

// Rotate gives the user with the key a new token. It needs an environment-level API key, and is
// sent once.
func (s *UsersService) Rotate(ctx context.Context, key string) (models.User, error) {
	return models.User{Key: key}, ctx.Err()
}
