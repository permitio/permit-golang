package framework

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

// Keys is what tools/e2e/keys.yaml says of the API keys of the cloud lane, beyond the
// project-level key of its project.
type Keys struct {
	// OrganizationKey says that the cloud lane has an organization-level key too, so that a
	// cloud scenario may call an operation that needs one. Without it, such an operation's
	// methods are local-only, with the reason org-key.
	OrganizationKey bool
}

// ReadKeys reads file, a keys.yaml, which sets organization_key to true or false and nothing
// else.
func ReadKeys(file string) (Keys, error) {
	data, err := os.ReadFile(file) //nolint:gosec // G304: the scenarios read their own keys.yaml.
	if err != nil {
		return Keys{}, fmt.Errorf("reading the keys of the cloud lane: %w", err)
	}
	var doc struct {
		OrganizationKey *bool `yaml:"organization_key"` //nolint:tagliatelle // The file's key.
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return Keys{}, fmt.Errorf("parsing %s: %w", file, err)
	}
	if doc.OrganizationKey == nil {
		return Keys{}, errors.New(file + " does not set organization_key to true or false")
	}
	return Keys{OrganizationKey: *doc.OrganizationKey}, nil
}
