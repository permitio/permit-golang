// Package framework holds what the end-to-end scenarios of tools/e2e share: the run prefix that
// names what a run creates, a client of the Permit API in plain net/http that retries a reply
// with status 429, the ledger of what a run creates and must delete, polling bounded by elapsed
// time, and the scratch environment that each run creates and deletes. It does not use the SDK,
// so what it creates and checks does not depend on the code under test.
package framework

import (
	"fmt"
	"regexp"
)

// Prefix starts the key of everything that one run of the scenarios creates:
// gosdk-ci-<run>-<attempt>-<leg>, such as gosdk-ci-1234-1-cloud, so that a run's objects never
// collide with another run's and a sweep can find them.
type Prefix string

var (
	// number matches a run or an attempt: a decimal number of up to 20 digits.
	number = regexp.MustCompile(`^[0-9]{1,20}$`)

	// leg matches a leg: a lower-case letter, then lower-case letters and digits.
	leg = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)

	// name matches the name that Key adds to the prefix.
	name = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

// NewPrefix returns the prefix of a run: run and attempt are decimal numbers, such as the
// workflow's run ID and attempt, and legName names the job that runs the scenarios, such as
// cloud.
func NewPrefix(run, attempt, legName string) (Prefix, error) {
	switch {
	case !number.MatchString(run):
		return "", fmt.Errorf("the run %q is not a decimal number, such as the workflow's run ID",
			run)
	case !number.MatchString(attempt):
		return "", fmt.Errorf("the attempt %q is not a decimal number, such as the workflow's "+
			"run attempt", attempt)
	case !leg.MatchString(legName):
		return "", fmt.Errorf("the leg %q is not 1 to 16 lower-case letters and digits that "+
			"start with a letter, such as cloud", legName)
	}
	return Prefix("gosdk-ci-" + run + "-" + attempt + "-" + legName), nil
}

// Key returns the key of the object called objectName in the run: the prefix, a hyphen and the
// name, which is 1 to 64 lower-case letters, digits and hyphens that start with a letter or a
// digit. It panics on another name, as regexp.MustCompile does on a bad pattern, because the name
// is a literal of a scenario.
func (p Prefix) Key(objectName string) string {
	if !name.MatchString(objectName) {
		panic(fmt.Sprintf("framework: the name %q of an object is not 1 to 64 lower-case "+
			"letters, digits and hyphens", objectName))
	}
	return string(p) + "-" + objectName
}
