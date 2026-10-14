package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The files that apicoverage reads from the specs directory.
const (
	controlPlaneFile = "control-plane.json"
	pdpContainerFile = "pdp-container.json"
	pdpCloudFile     = "pdp-cloud.json"
	pdpExtraFile     = "pdp-container.extra.json"
	v1RoutesFile     = "v1-routes.json"
	v1FixesFile      = "v1-route-fixes.yaml"
	coverageFile     = "coverage.yaml"
)

// The stages of an operation.
const (
	stageGA         = "GA"
	stageEAP        = "EAP"
	stageDeprecated = "deprecated"
)

// eapTag matches a tag that marks an operation EAP, such as "Policy Guards (EAP)".
var eapTag = regexp.MustCompile(`\bEAP\b`)

// known is a route that a spec or pdp-container.extra.json lists.
type known struct {
	// method is the route's method, or ANY for a route of pdp-container.extra.json that takes
	// several, which methods lists.
	method  string
	path    string
	methods []string
	// id is the operationId of the route: pdp-container.json's when it lists it, else
	// pdp-cloud.json's; empty for a route that only pdp-container.extra.json lists. aliases are
	// the route's other operationIds.
	id      string
	aliases []string
	// stage is the stage that the spec gives the operation; GA for a route no spec lists.
	stage string
	// listed says that a published spec lists the route, not only pdp-container.extra.json.
	listed bool
	// params are the names of the operation's query parameters.
	params []string
}

// key returns the route as coverage.yaml writes it.
func (k known) key() string {
	return k.method + " " + k.path
}

// inventory is what the specs list: the routes of the control plane and of the PDP, and the
// mounts of the container PDP.
type inventory struct {
	controlPlane map[string]*known
	pdp          map[string]*known
	mounts       []mount
}

// openAPI is the part of an OpenAPI spec that apicoverage reads.
type openAPI struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

// openAPIOperation is the part of an operation of an OpenAPI spec that apicoverage reads.
type openAPIOperation struct {
	OperationID string   `json:"operationId"` //nolint:tagliatelle // OpenAPI's name.
	Tags        []string `json:"tags"`
	Deprecated  bool     `json:"deprecated"`
	Parameters  []struct {
		Name string `json:"name"`
		In   string `json:"in"`
	} `json:"parameters"`
}

// readSpec returns the operations of the OpenAPI spec in the file, by route.
func readSpec(file string) (map[string]*known, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var spec openAPI
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", file, err)
	}
	ops := map[string]*known{}
	for path, item := range spec.Paths {
		if _, err := parseTemplate(path); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		for name, raw := range item {
			method := strings.ToUpper(name)
			if !slices.Contains(routeMethods, method) {
				continue
			}
			var op openAPIOperation
			if err := json.Unmarshal(raw, &op); err != nil {
				return nil, fmt.Errorf("%s: %s %s: %w", file, method, path, err)
			}
			if op.OperationID == "" {
				return nil, fmt.Errorf("%s: %s %s has no operationId", file, method, path)
			}
			k := &known{
				method: method, path: path, id: op.OperationID, stage: stageOf(op),
				listed: true,
			}
			for _, p := range op.Parameters {
				if p.In == "query" {
					k.params = append(k.params, p.Name)
				}
			}
			ops[k.key()] = k
		}
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("%s lists no operation", file)
	}
	return ops, nil
}

// stageOf returns the stage of an operation: deprecated when the spec marks it deprecated, else
// EAP when one of its tags says EAP, else GA.
func stageOf(op openAPIOperation) string {
	if op.Deprecated {
		return stageDeprecated
	}
	if slices.ContainsFunc(op.Tags, eapTag.MatchString) {
		return stageEAP
	}
	return stageGA
}

// extras is the part of pdp-container.extra.json that apicoverage reads.
type extras struct {
	Mounts []struct {
		Prefix string `json:"prefix"`
		SameAs string `json:"same_as"`
	} `json:"mounts"`
	Routes []struct {
		Methods []string `json:"methods"`
		Path    string   `json:"path"`
	} `json:"routes"`
}

// readInventory reads the specs from dir, and the control plane's and the cloud PDP's from live
// instead when it is not empty.
func readInventory(dir, live string) (*inventory, error) {
	liveDir := dir
	if live != "" {
		liveDir = live
	}
	control, err := readSpec(filepath.Join(liveDir, controlPlaneFile))
	if err != nil {
		return nil, err
	}
	container, err := readSpec(filepath.Join(dir, pdpContainerFile))
	if err != nil {
		return nil, err
	}
	cloud, err := readSpec(filepath.Join(liveDir, pdpCloudFile))
	if err != nil {
		return nil, err
	}
	pdp := container
	for key, op := range cloud {
		if k, ok := pdp[key]; ok {
			if op.id != k.id {
				k.aliases = append(k.aliases, op.id)
			}
			continue
		}
		pdp[key] = op
	}
	inv := &inventory{controlPlane: control, pdp: pdp}
	if err := inv.addExtras(filepath.Join(dir, pdpExtraFile)); err != nil {
		return nil, err
	}
	return inv, nil
}

// addExtras adds the routes and mounts of pdp-container.extra.json to the PDP's. The tests of
// specsnap check the rest of the file.
func (inv *inventory) addExtras(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var ex extras
	if err := json.Unmarshal(data, &ex); err != nil {
		return fmt.Errorf("parsing %s: %w", file, err)
	}
	for _, r := range ex.Routes {
		if len(r.Methods) == 0 {
			return fmt.Errorf("%s: %s has no methods", file, r.Path)
		}
		for _, m := range r.Methods {
			if !slices.Contains(routeMethods, m) {
				return fmt.Errorf("%s: %s has the method %q", file, r.Path, m)
			}
		}
		k := &known{method: r.Methods[0], path: r.Path, stage: stageGA}
		if len(r.Methods) > 1 {
			k.method, k.methods = anyMethod, slices.Clone(r.Methods)
		}
		if _, err := parseTemplate(r.Path); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		if _, ok := inv.pdp[k.key()]; !ok {
			inv.pdp[k.key()] = k
		}
	}
	for _, m := range ex.Mounts {
		prefix, err := parseTemplate(m.Prefix)
		if err != nil {
			return fmt.Errorf("%s: the mount %s: %w", file, m.Prefix, err)
		}
		sameAs, err := parseTemplate(m.SameAs)
		if err != nil || sameAs.literals() != len(sameAs.segments) {
			return fmt.Errorf("%s: the mount %s is the same as %q, which is not a path of "+
				"literal segments", file, m.Prefix, m.SameAs)
		}
		mnt := mount{prefix: prefix}
		for _, s := range sameAs.segments {
			mnt.sameAs = append(mnt.sameAs, s.literal)
		}
		inv.mounts = append(inv.mounts, mnt)
	}
	return nil
}

// readJSON decodes the JSON file into v, rejecting unknown fields.
func readJSON(file string, v any) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("parsing %s: %w", file, err)
	}
	return nil
}

// v1Routes is the part of v1-routes.json that apicoverage reads.
type v1Routes struct {
	Routes []struct {
		Method      string `json:"method"`
		OperationID string `json:"operation_id"`
		Path        string `json:"path"`
	} `json:"routes"`
}

// v1Fixes is the part of v1-route-fixes.yaml that apicoverage reads.
type v1Fixes struct {
	Fixes []struct {
		ID      string `yaml:"id"`
		Changes []struct {
			V1 string `yaml:"v1"`
			V2 string `yaml:"v2"`
		} `yaml:"changes"`
	} `yaml:"fixes"`
}

// v1Seed is what v1.2.8 sent: the route of each operation it had, by operationId, and the
// shapes of the routes that v2 sends instead of a route v1 sent, on purpose, by the shape of
// v1's route. routesFile and fixesFile are the files it was read from.
type v1Seed struct {
	routes                map[string]string
	fixes                 map[string][]string
	routesFile, fixesFile string
}

// readV1Seed reads v1-routes.json and v1-route-fixes.yaml from dir. Each operation of
// v1-routes.json must be the op of a control-plane row of cov, as the file lists only the
// control plane's routes.
func readV1Seed(dir string, cov *coverage) (v1Seed, error) {
	seed := v1Seed{
		routes: map[string]string{}, fixes: map[string][]string{},
		routesFile: filepath.Join(dir, v1RoutesFile), fixesFile: filepath.Join(dir, v1FixesFile),
	}
	var routes v1Routes
	data, err := os.ReadFile(seed.routesFile)
	if err != nil {
		return v1Seed{}, err
	}
	if err = json.Unmarshal(data, &routes); err != nil {
		return v1Seed{}, fmt.Errorf("parsing %s: %w", v1RoutesFile, err)
	}
	controlPlane := map[string]bool{}
	for _, r := range cov.ControlPlane {
		controlPlane[r.Op] = true
	}
	for _, r := range routes.Routes {
		switch {
		case r.OperationID == "" || r.Method == "" || r.Path == "":
			return v1Seed{}, fmt.Errorf("%s: a route lacks its operation_id, method or path",
				v1RoutesFile)
		case !controlPlane[r.OperationID]:
			return v1Seed{}, fmt.Errorf("%s: %s is not the op of a control-plane row of "+
				"coverage.yaml; the file lists only operations of the control plane",
				seed.routesFile, r.OperationID)
		}
		seed.routes[r.OperationID] = r.Method + " " + r.Path
	}
	if len(seed.routes) == 0 {
		return v1Seed{}, fmt.Errorf("%s lists no route", v1RoutesFile)
	}
	data, err = os.ReadFile(seed.fixesFile)
	if err != nil {
		return v1Seed{}, err
	}
	var fixes v1Fixes
	if err := yaml.Unmarshal(data, &fixes); err != nil {
		return v1Seed{}, fmt.Errorf("parsing %s: %w", v1FixesFile, err)
	}
	for _, f := range fixes.Fixes {
		for _, c := range f.Changes {
			if c.V1 == "" || c.V2 == "" {
				return v1Seed{}, fmt.Errorf("%s: a change of the fix %s lacks v1 or v2",
					v1FixesFile, f.ID)
			}
			seed.fixes[shape(c.V1)] = append(seed.fixes[shape(c.V1)], shape(c.V2))
		}
	}
	return seed, nil
}

// allows reports whether a request for the operation id may go to the route: v1.2.8 did not
// have the operation, or sent it to a route of the same shape, or v1-route-fixes.yaml lists a
// route of the route's shape as the one v2 sends instead of v1's. It also returns v1's route.
func (s v1Seed) allows(id, route string) (string, bool) {
	v1, ok := s.routes[id]
	if !ok || shape(route) == shape(v1) {
		return v1, true
	}
	return v1, slices.Contains(s.fixes[shape(v1)], shape(route))
}

// operationMapFile is the operation map's default path, relative to the repository root.
const operationMapFile = "testdata/operation-map.json"

// opMap is testdata/operation-map.json: the operationIds of the wire cases of each public
// method.
type opMap struct {
	Methods map[string][]string `json:"methods"`
}

// readOperationMap reads the operation map.
func readOperationMap(file string) (opMap, error) {
	var m opMap
	if err := readJSON(file, &m); err != nil {
		return opMap{}, err
	}
	if m.Methods == nil {
		return opMap{}, fmt.Errorf("%s has no methods object", file)
	}
	return m, nil
}

// methodName matches a public method, <Type>.<Method>.
var methodName = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*\.[A-Z][A-Za-z0-9_]*$`)

// readAdvisory reads the list of methods whose end-to-end scenario is advisory: one method and
// its reason per line; blank lines and lines that start with # are ignored.
func readAdvisory(file string) (map[string]bool, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // Read only; a close error loses nothing.
	methods := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for n := 1; scanner.Scan(); n++ {
		text := scanner.Text()
		if strings.TrimSpace(text) == "" || strings.HasPrefix(text, "#") {
			continue
		}
		method, reason, _ := strings.Cut(text, " ")
		switch {
		case !methodName.MatchString(method):
			return nil, fmt.Errorf("%s:%d: %q does not start with a method, such as "+
				"UsersService.Get", file, n, text)
		case strings.TrimSpace(reason) == "":
			return nil, fmt.Errorf("%s:%d: %s has no reason", file, n, method)
		case methods[method]:
			return nil, fmt.Errorf("%s:%d: %s is listed twice", file, n, method)
		}
		methods[method] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	return methods, nil
}
