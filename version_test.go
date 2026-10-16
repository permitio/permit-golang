package permit

import (
	"regexp"
	"runtime/debug"
	"strings"
	"testing"
)

// The parts of a Semantic Versioning 2.0.0 version (https://semver.org): numbers have no leading
// zero, and a prerelease identifier is a number or contains a letter or hyphen.
const (
	semverNumber     = `(0|[1-9][0-9]*)`
	semverPrerelease = `(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)`
	semverBuild      = `[0-9a-zA-Z-]+`
)

// semverPattern matches a whole semantic version, which has no "v" prefix.
var semverPattern = regexp.MustCompile(`^` +
	semverNumber + `\.` + semverNumber + `\.` + semverNumber +
	`(-` + semverPrerelease + `(\.` + semverPrerelease + `)*)?` +
	`(\+` + semverBuild + `(\.` + semverBuild + `)*)?$`)

func TestVersion(t *testing.T) {
	if !semverPattern.MatchString(Version) {
		t.Fatalf("Version is %q, want a semantic version without a v prefix, such as 2.0.0",
			Version)
	}
}

func TestVersionMatchesModulePath(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("the test binary has no build information; run it with go test in module mode")
	}
	major, _, _ := strings.Cut(Version, ".")
	want := "/v" + major
	if !strings.HasSuffix(info.Main.Path, want) {
		t.Fatalf("module path is %q and Version is %q, want a module path that ends in %q",
			info.Main.Path, Version, want)
	}
}
