package drift

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// replyName matches the name of a reply fixture, <operationId>.<status>[.<variant>].json, such
// as get_user.200.json or list_users.200.page.json.
var replyName = regexp.MustCompile(
	`^([A-Za-z_][A-Za-z0-9_]*)\.([1-5][0-9][0-9])(?:\.([a-z0-9][a-z0-9-]*))?\.json$`)

// replyFindings returns the findings of each reply fixture in dir against the schema of its
// reply in specs, the errors of the fixtures whose schemas the walker does not implement, and
// the number of fixtures that it validated. A dir that does not exist holds no fixture.
func replyFindings(dir string, specs map[string]*spec) ([]finding, []error, int, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, 0, nil
	}
	if err != nil {
		return nil, nil, 0, err
	}
	var found []finding
	var errs []error
	validated := 0
	for _, entry := range entries {
		more, ok, err := checkReply(dir, entry, specs)
		switch {
		case isUnsupported(err):
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
		case err != nil:
			return nil, nil, 0, err
		case ok:
			validated++
		}
		found = append(found, more...)
	}
	return found, errs, validated, nil
}

// checkReply returns the findings of the reply fixture entry of dir against the schema of its
// reply in specs: in the spec that lists its operation, and in each other spec that lists the
// operation's method and path, as the cloud PDP lists the routes of the PDP container that it
// serves. It returns whether it validated the fixture against the schema of the first.
func checkReply(dir string, entry os.DirEntry, specs map[string]*spec) ([]finding, bool, error) {
	name := entry.Name()
	m := replyName.FindStringSubmatch(name)
	if entry.IsDir() || m == nil {
		return []finding{{text: filepath.Join(dir, name) + " is not a reply fixture: name each " +
			"<operationId>.<status>[.<variant>].json, such as get_user.200.json, with a variant " +
			"of lower-case letters, digits and hyphens"}}, false, nil
	}
	o, ok := findOperation(specs, m[1])
	if !ok {
		return []finding{{text: name + ": no spec has the operation " + m[1]}}, false, nil
	}
	found, validated, err := validateReply(dir, name, o, "reply "+name)
	if err != nil || !validated {
		return found, false, err
	}
	for _, same := range sameRoute(specs, o) {
		more, _, err := validateReply(dir, name, same, "reply "+name+"@"+same.spec.name)
		if err != nil {
			return nil, false, err
		}
		found = append(found, more...)
	}
	return found, true, nil
}

// validateReply returns the findings of the reply fixture name of dir against the schema of its
// reply of o, keyed by key, and whether it validated the fixture against the schema.
func validateReply(dir, name string, o specOperation, key string) ([]finding, bool, error) {
	status := replyName.FindStringSubmatch(name)[2]
	id, _ := o.node["operationId"].(string)
	responses, _ := o.node["responses"].(map[string]any)
	reply, ok := responses[status].(map[string]any)
	if !ok {
		return []finding{{key, "the spec lists no " + status + " reply of " + id}}, false, nil
	}
	schema, at, ok := jsonBody(reply, o.at+"/responses/"+status)
	if !ok {
		return []finding{{key, "the spec's " + status + " reply of " + id + " has no JSON body"}},
			false, nil
	}
	if err := o.spec.audit(schema, at); err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, false, err
	}
	value, problem := decodeReply(data)
	if problem != "" {
		return []finding{{text: name + " is not one JSON value: " + problem}}, false, nil
	}
	var found []finding
	for _, v := range o.spec.validate(value, schema, at) {
		found = append(found, finding{key + pointer(v.at), v.text})
	}
	return found, true, nil
}

// decodeReply returns the JSON value that data, a reply fixture, holds, with each number as a
// json.Number, or what is wrong with it.
func decodeReply(data []byte) (any, string) {
	var value any
	if err := wire.Decode(data, &value); err != nil {
		return nil, err.Error()
	}
	return value, ""
}
