package drift

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// Specs holds the spec snapshots, for the checks that code outside the drift tests runs on them.
type Specs struct {
	specs map[string]*spec
}

// ReadSpecs reads the snapshots of the control plane's, the container PDP's and the cloud PDP's
// specs from dir, such as tools/specs.
func ReadSpecs(dir string) (*Specs, error) {
	specs, _, err := readSpecs(dir, dir)
	if err != nil {
		return nil, err
	}
	return &Specs{specs: specs}, nil
}

// Operation is an operation of a spec.
type Operation struct {
	// Method is its HTTP method, upper case, such as POST.
	Method string

	// Route is its path template, such as /v2/facts/{proj_id}/{env_id}/users.
	Route string

	// ID is its operationId, such as create_user.
	ID string
}

// Operations returns the operations of the spec name, such as control-plane, sorted by route and
// then by method, or nil when s holds no spec of that name.
func (s *Specs) Operations(name string) []Operation {
	sp, ok := s.specs[name]
	if !ok {
		return nil
	}
	var ops []Operation
	paths, _ := sp.doc["paths"].(map[string]any)
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		methods, _ := paths[path].(map[string]any)
		for _, method := range slices.Sorted(maps.Keys(methods)) {
			node, _ := methods[method].(map[string]any)
			id, _ := node["operationId"].(string)
			ops = append(ops, Operation{Method: strings.ToUpper(method), Route: path, ID: id})
		}
	}
	return ops
}

// CheckRequestBody returns the places where body, the body of a request of the operation id,
// does not fit the schema of the operation's JSON request body, each as its JSON pointer, such as
// #/key, a space and what does not fit. It returns an error when no spec has the operation, the
// operation has no JSON request body, the walker does not implement its schema, or body is not
// one JSON value.
func (s *Specs) CheckRequestBody(id string, body []byte) ([]string, error) {
	o, ok := findOperation(s.specs, id)
	if !ok {
		return nil, fmt.Errorf("no spec has the operation %s", id)
	}
	request, _ := o.node["requestBody"].(map[string]any)
	schema, at, ok := jsonBody(request, o.at+"/requestBody")
	if !ok {
		return nil, fmt.Errorf("the operation %s has no JSON request body", id)
	}
	if err := o.spec.audit(schema, at); err != nil {
		return nil, err
	}
	var value any
	if err := wire.Decode(body, &value); err != nil {
		return nil, fmt.Errorf("the body of a request of %s is not one JSON value: %w", id, err)
	}
	var problems []string
	for _, v := range o.spec.validate(value, schema, at) {
		problems = append(problems, pointer(v.at)+" "+v.text)
	}
	return problems, nil
}
