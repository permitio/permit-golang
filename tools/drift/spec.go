package drift

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// specNames are the snapshots of tools/specs that models and operations map to, each in
// <name>.json.
var specNames = []string{"control-plane", "pdp-container", "pdp-cloud"}

// defaultSpec is the spec of a row of model-map.yaml that names none.
const defaultSpec = "control-plane"

// schemaPrefix starts a $ref to a schema of components.schemas.
const schemaPrefix = "#/components/schemas/"

// spec is a snapshot of an API spec, parsed with each number as a json.Number.
type spec struct {
	// name is the snapshot's name, such as control-plane.
	name string

	// doc is the parsed snapshot.
	doc map[string]any
}

// readSpec reads the snapshot in file as the spec name.
func readSpec(name, file string) (*spec, error) {
	data, err := os.ReadFile(file) //nolint:gosec // G304: a snapshot of the spec directory.
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := wire.Decode(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", file, err)
	}
	if doc == nil {
		return nil, fmt.Errorf("parsing %s: it is not a JSON object", file)
	}
	return &spec{name: name, doc: doc}, nil
}

// readSpecs reads the snapshot of each of specNames from dir, or from fallback when dir does not
// hold it, and returns them by name, with a line for each that names the file it read. It fails
// when dir is not a directory, or when a snapshot cannot be read.
func readSpecs(dir, fallback string) (map[string]*spec, []string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("the spec directory: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("the spec directory %s is not a directory", dir)
	}
	specs := map[string]*spec{}
	var lines []string
	for _, name := range specNames {
		file := filepath.Join(dir, name+".json")
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			file = filepath.Join(fallback, name+".json")
		}
		s, err := readSpec(name, file)
		if err != nil {
			return nil, nil, err
		}
		specs[name] = s
		lines = append(lines, name+": "+file)
	}
	return specs, lines, nil
}

// schema returns the schema name of components.schemas, and false when there is none.
func (s *spec) schema(name string) (any, bool) {
	components, _ := s.doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	schema, ok := schemas[name]
	return schema, ok
}

// specOperation is an operation of a spec.
type specOperation struct {
	// spec is the spec that lists it.
	spec *spec

	// method is its HTTP method, upper case, such as GET.
	method string

	// path is its path template, such as /v2/facts/{proj_id}/{env_id}/users/{user_id}.
	path string

	// node is its object in the spec.
	node map[string]any

	// at is where node is in the spec, as a $ref to it.
	at string
}

// findOperation returns the operation whose operationId is id in one of specs, and false when
// none lists it. The specs give each operation an ID of its own, which no other spec gives.
func findOperation(specs map[string]*spec, id string) (specOperation, bool) {
	for _, name := range slices.Sorted(maps.Keys(specs)) {
		s := specs[name]
		paths, _ := s.doc["paths"].(map[string]any)
		for _, path := range slices.Sorted(maps.Keys(paths)) {
			methods, _ := paths[path].(map[string]any)
			for _, method := range slices.Sorted(maps.Keys(methods)) {
				node, _ := methods[method].(map[string]any)
				if node != nil && node["operationId"] == id {
					return specOperation{
						spec: s, method: strings.ToUpper(method), path: path, node: node,
						at: "#/paths/" + escape(path) + "/" + escape(method),
					}, true
				}
			}
		}
	}
	return specOperation{}, false
}

// sameRoute returns the operations of the specs other than that of o whose method and path are
// o's, as the cloud PDP lists the routes of the PDP container that it serves, in the order of the
// names of their specs.
func sameRoute(specs map[string]*spec, o specOperation) []specOperation {
	var same []specOperation
	for _, name := range slices.Sorted(maps.Keys(specs)) {
		s := specs[name]
		paths, _ := s.doc["paths"].(map[string]any)
		methods, _ := paths[o.path].(map[string]any)
		node, _ := methods[strings.ToLower(o.method)].(map[string]any)
		if s != o.spec && node != nil {
			same = append(same, specOperation{
				spec: s, method: o.method, path: o.path, node: node,
				at: "#/paths/" + escape(o.path) + "/" + strings.ToLower(o.method),
			})
		}
	}
	return same
}

// jsonBody returns the schema of the application/json content of node, a request body or a
// response of an operation, and where it is; false when node has none.
func jsonBody(node map[string]any, at string) (any, string, bool) {
	content, _ := node["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	schema, ok := media["schema"]
	return schema, at + "/content/application~1json/schema", ok
}

// resolve returns the value that the $ref ref points at in s, and its name when ref points at a
// schema of components.schemas. A ref that does not start with #/, or that points at nothing,
// is an *unsupportedError: the walker reads only the spec it is in.
func (s *spec) resolve(ref string) (any, string, error) {
	pointer, ok := strings.CutPrefix(ref, "#/")
	if !ok {
		return nil, "", unsupported(ref, "the $ref points outside the spec, and the walker reads "+
			"only the spec it is in")
	}
	tokens := strings.Split(pointer, "/")
	var v any = s.doc
	for i, token := range tokens {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		tokens[i] = token
		var found bool
		switch node := v.(type) {
		case map[string]any:
			v, found = node[token]
		case []any:
			i, err := strconv.Atoi(token)
			found = err == nil && i >= 0 && i < len(node)
			if found {
				v = node[i]
			}
		}
		if !found {
			return nil, "", unsupported(ref, "the $ref points at nothing in the spec")
		}
	}
	if len(tokens) == 3 && tokens[0] == "components" && tokens[1] == "schemas" {
		return v, tokens[2], nil
	}
	return v, "", nil
}

// escape escapes a token of a JSON pointer: ~ as ~0 and / as ~1.
func escape(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

// unsupportedError says that the walker met a schema it does not implement, such as one with a
// keyword it does not know. A check that meets one did not run, rather than skip what it cannot
// read.
type unsupportedError struct {
	// at is where the schema is, as a $ref to it, such as #/components/schemas/User.
	at string

	// what says what the walker does not implement.
	what string
}

func (e *unsupportedError) Error() string {
	return e.at + ": " + e.what
}

// isUnsupported reports whether err is an *unsupportedError, or wraps one.
func isUnsupported(err error) bool {
	_, ok := errors.AsType[*unsupportedError](err)
	return ok
}

// unsupported returns an *unsupportedError for the schema that at locates, which says what.
func unsupported(at, what string) error {
	return &unsupportedError{at: at, what: what}
}
