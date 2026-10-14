// Package sdk is a fixture of the documentation checks in doc_test.go whose doc links do not
// resolve: [Nothing], [Client.Nothing], [sdk.Nothing] and [*Nothing]. [Client] resolves, and
// brackets that are not doc links stay text: a[i], []string, [a b], [1] and [func].
package sdk

import (
	"context"

	"example.com/sdk/models"
)

// Client links [models.Nothing] and [model.User],
// [context.Nothing] and [strings.Nothing],
// [http.Client] and [example.com/elsewhere.Thing],
// [example.com/sdk/nothere.Thing], a package of the module that does not exist,
// and [example.com/sdk/models.Nothing]; [example.com/sdk/models.User] resolves. A list:
//
//   - [Nothing] in a list item.
type Client struct {
	// Users links [UsersService.Nothing].
	Users *UsersService

	Kind Kind // Kind links [Kind.Nothing] in its line comment.
}

// Kind is a kind of user.
type Kind string

// The kinds of user.
const (
	// KindUser links [Nothing].
	KindUser Kind = "user"

	KindAdmin Kind = "admin" // KindAdmin links [Nothing] in its line comment.
)

// Level is a level.
type Level int // Level links [Nothing] in its line comment.

// Getter gets.
type Getter interface {
	// Get links [Getter.Nothing].
	Get() string
}

// UsersService reads and writes users; [UsersService.Get] resolves.
type UsersService struct{}

// Get links [Nothing]. The comments in its body are not documentation.
func (s *UsersService) Get(ctx context.Context) models.User {
	// [Nothing] is not checked here.

	// local links [Nothing] in the doc comment of a local declaration, which is not checked.
	const local = 0
	return models.User{}
}

// hidden links [hidden], which is not a link to an unexported type.
type hidden struct{}
