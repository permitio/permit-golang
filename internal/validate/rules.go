// Package validate holds the rules that the SDK checks a configuration or an argument against
// before it sends a request. Each rule has an ID that never changes and the message of the error
// it returns. testdata/validation-rules.txt lists the rules; TestValidationRules
// (validation_rules_test.go) keeps it in step with this package, and apisurface's diff mode
// (tools/cmd/apisurface) counts an ID that is new there as a breaking change, because a new rule
// rejects input that the SDK accepted before.
package validate

import "slices"

// Rule is one check of a configuration or an argument.
type Rule struct {
	// ID names the rule: lower-case words, each joined to the next by a dot, such as
	// config.api-key.missing. It never changes, so a new ID is a new rule.
	ID string

	// Message is the message of the error the rule returns, on one line.
	Message string
}

// rules lists every rule, sorted by ID.
var rules = []Rule{}

// Rules returns every rule, sorted by ID.
func Rules() []Rule {
	return slices.Clone(rules)
}
