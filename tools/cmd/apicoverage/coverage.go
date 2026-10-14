package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The statuses of a row of coverage.yaml.
const (
	statusCovered  = "covered"
	statusPlanned  = "planned"
	statusExcluded = "excluded"
	statusDeferred = "deferred"
)

// statuses are the statuses, in the order the report lists them.
var statuses = []string{statusCovered, statusPlanned, statusExcluded, statusDeferred}

// stages are the stages, in the order the report lists them.
var stages = []string{stageGA, stageEAP, stageDeprecated}

// The sections of coverage.yaml that hold rows.
const (
	sectionControlPlane = "control-plane"
	sectionPDP          = "pdp"
)

// coverage is coverage.yaml.
type coverage struct {
	Reasons       map[string]string `yaml:"reasons"`
	ControlPlane  []*row            `yaml:"control-plane"` //nolint:tagliatelle // The file's key.
	PDP           []*row            `yaml:"pdp"`
	SDKOnlyParams []sdkOnlyParam    `yaml:"sdk_only_params"` //nolint:tagliatelle // The file's key.
}

// row is the decision for one route.
type row struct {
	Op            string         `yaml:"op"`
	Route         string         `yaml:"route"`
	Stage         string         `yaml:"stage"`
	Status        string         `yaml:"status"`
	Reason        string         `yaml:"reason"`
	Trigger       string         `yaml:"trigger"`
	SDKOnly       string         `yaml:"sdk_only"`       //nolint:tagliatelle // The file's key.
	StageOverride *stageOverride `yaml:"stage_override"` //nolint:tagliatelle // The file's key.

	// section is the section of coverage.yaml that holds the row, and method, path and tmpl its
	// route.
	section string
	method  string
	path    string
	tmpl    template
}

// stageOverride is the stage the SDK treats an operation as, and why.
type stageOverride struct {
	Stage  string `yaml:"stage"`
	Reason string `yaml:"reason"`
}

// sdkOnlyParam is a query parameter that the SDK sends to an operation whose spec does not list
// it.
type sdkOnlyParam struct {
	Op     string `yaml:"op"`
	Param  string `yaml:"param"`
	Reason string `yaml:"reason"`
}

// effectiveStage returns the stage the SDK treats the row's operation as.
func (r *row) effectiveStage() string {
	if r.StageOverride != nil {
		return r.StageOverride.Stage
	}
	return r.Stage
}

// name returns the row's name for messages: its section and route.
func (r *row) name() string {
	return r.section + " " + r.Route
}

// matchesMethod reports whether the row's route takes the method. A row whose route is ANY
// takes the methods that the inventory lists for it, or any method when it lists none.
func (r *row) matchesMethod(method string, k *known) bool {
	if r.method != anyMethod {
		return r.method == method
	}
	return k == nil || len(k.methods) == 0 || slices.Contains(k.methods, method)
}

// reasonKey matches a key of the reasons vocabulary.
var reasonKey = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// opID matches an operationId, or the id of a row whose route no spec lists.
var opID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// paramName matches a query parameter's name, or a prefix followed by *.
var paramName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\[\]-]*\*?$`)

// readCoverage reads and checks the shape of coverage.yaml. An error means the file is not one
// that apicoverage can check; how its rows fit the specs is checked later.
func readCoverage(file string) (*coverage, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c coverage
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", file, err)
	}
	if err := c.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return &c, nil
}

// check checks the shape of coverage.yaml.
func (c *coverage) check() error {
	if len(c.Reasons) == 0 {
		return errors.New("reasons lists no reason for an excluded row")
	}
	for key, text := range c.Reasons {
		if !reasonKey.MatchString(key) || strings.TrimSpace(text) == "" {
			return fmt.Errorf("reasons: %q needs a key of lower-case letters, digits and "+
				"hyphens, and a sentence", key)
		}
	}
	if len(c.ControlPlane) == 0 || len(c.PDP) == 0 {
		return errors.New("control-plane and pdp must each have rows")
	}
	ops := map[string]string{}
	for _, section := range []struct {
		name string
		rows []*row
	}{{sectionControlPlane, c.ControlPlane}, {sectionPDP, c.PDP}} {
		routes := map[string]bool{}
		for i, r := range section.rows {
			if r == nil {
				return fmt.Errorf("%s: row %d is empty", section.name, i+1)
			}
			r.section = section.name
			if err := r.check(); err != nil {
				return fmt.Errorf("%s: %w", r.name(), err)
			}
			if routes[r.Route] {
				return fmt.Errorf("%s has two rows", r.name())
			}
			routes[r.Route] = true
			if other, ok := ops[r.Op]; ok && r.Op != "" {
				return fmt.Errorf("%s and %s have the same op %s", other, r.name(), r.Op)
			}
			ops[r.Op] = r.name()
		}
	}
	seen := map[string]bool{}
	for i, p := range c.SDKOnlyParams {
		switch {
		case !opID.MatchString(p.Op) || !paramName.MatchString(p.Param) ||
			strings.TrimSpace(p.Reason) == "":
			return fmt.Errorf("sdk_only_params: entry %d needs an op, a param (a name, or a "+
				"prefix and *) and a reason", i+1)
		case seen[p.Op+" "+p.Param]:
			return fmt.Errorf("sdk_only_params: %s %s is listed twice", p.Op, p.Param)
		}
		seen[p.Op+" "+p.Param] = true
	}
	return nil
}

// check checks the shape of a row.
func (r *row) check() error {
	method, path, err := parseRoute(r.Route)
	if err != nil {
		return err
	}
	r.method, r.path = method, path
	r.tmpl, _ = parseTemplate(path) // parseRoute checked it.
	switch {
	case r.Op != "" && !opID.MatchString(r.Op):
		return fmt.Errorf("op %q is not an operationId", r.Op)
	case !slices.Contains(stages, r.Stage):
		return fmt.Errorf("stage %q is not one of %s", r.Stage, strings.Join(stages, ", "))
	case !slices.Contains(statuses, r.Status):
		return fmt.Errorf("status %q is not one of %s", r.Status, strings.Join(statuses, ", "))
	case r.StageOverride != nil && (!slices.Contains(stages, r.StageOverride.Stage) ||
		strings.TrimSpace(r.StageOverride.Reason) == ""):
		return errors.New("stage_override needs a stage and a reason")
	}
	hasReason := strings.TrimSpace(r.Reason) != ""
	hasTrigger := strings.TrimSpace(r.Trigger) != ""
	sends := r.Status == statusCovered || r.Status == statusPlanned
	switch {
	case r.Status == statusExcluded && (!hasReason || r.Trigger != ""):
		return errors.New("an excluded row needs a reason, a key of reasons, and no trigger")
	case r.Status == statusDeferred && (!hasReason || !hasTrigger):
		return errors.New("a deferred row needs a reason and a trigger")
	case sends && (r.Reason != "" || r.Trigger != ""):
		return fmt.Errorf("a %s row has no reason or trigger", r.Status)
	case !sends && r.SDKOnly != "":
		return fmt.Errorf("sdk_only is for a row that the SDK sends, covered or planned, and "+
			"this one is %s", r.Status)
	case r.SDKOnly != "" && strings.TrimSpace(r.SDKOnly) == "":
		return errors.New("sdk_only is empty")
	}
	return nil
}
