// Package models holds the types of the fixture in testdata/doccheck/links; [Usr] does not
// resolve, and [User] does.
package models

// User is a user.
type User struct {
	// Key identifies the user.
	Key string
}
