package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// inputs is what apicoverage checks.
type inputs struct {
	inv  *inventory
	cov  *coverage
	seed v1Seed
	ops  opMap
	// advisory holds the methods whose end-to-end scenario is advisory, or is nil.
	advisory map[string]bool
	records  []*record
	// local is the record of a run against a full local stack, or nil.
	local *record
}

// rowState is what the records and the operation map say about a row.
type rowState struct {
	// requests counts the row's requests in the records, by mode.
	requests map[string]int
	// methods are the public methods whose wire cases send the row's operation.
	methods []string
}

// result is what apicoverage found.
type result struct {
	// incomplete holds why the records cannot show the coverage, which makes the run one that
	// did not run, and problems what fails the check.
	incomplete []string
	problems   []string
	sections   []sectionResult
	// covered are the covered rows, in the order of coverage.yaml.
	covered []coveredRow
	// requests counts the requests of the records, by mode, and tests the tests that took part.
	requests map[string]int
	tests    int
	files    int
}

// sectionResult counts the rows of a section of coverage.yaml.
type sectionResult struct {
	name string
	rows int
	// byStatus counts the rows by status; byStage the rows of a spec's operations by their
	// stage and status, sdkOnly those of the routes that the SDK sends and no spec lists, and
	// undecided the operations of the specs without a row, by stage.
	byStatus  map[string]int
	byStage   map[string]map[string]int
	sdkOnly   map[string]int
	undecided map[string]int
}

// coveredRow is a covered row with what covers it.
type coveredRow struct {
	row      *row
	methods  []string
	requests int
	e2e      string
}

// The end-to-end states of a covered row.
const (
	e2eCloud    = "cloud"
	e2eLocal    = "local"
	e2eAdvisory = "advisory"
	e2eDryRun   = "dry run"
	e2eNone     = "none"
)

// checker runs the checks.
type checker struct {
	in    inputs
	res   *result
	state map[*row]*rowState
	byOp  map[string]*row
	// seen holds the problems found, and the keys of the problems of requests, so that a request
	// that tests send many times is reported once, with the first line that holds it.
	seen map[string]bool
}

// check checks coverage.yaml against the specs, the operation map and the records.
func check(in inputs) *result {
	c := &checker{
		in:    in,
		res:   &result{requests: map[string]int{}},
		state: map[*row]*rowState{},
		byOp:  map[string]*row{},
		seen:  map[string]bool{},
	}
	for _, r := range c.rows() {
		c.state[r] = &rowState{requests: map[string]int{}}
		if r.Op != "" {
			c.byOp[r.Op] = r
		}
	}
	for _, r := range in.cov.PDP {
		if k := in.inv.pdp[r.Route]; k != nil {
			for _, alias := range k.aliases {
				c.byOp[alias] = r
			}
		}
	}
	c.checkRows(sectionControlPlane, in.cov.ControlPlane, in.inv.controlPlane)
	c.checkRows(sectionPDP, in.cov.PDP, in.inv.pdp)
	c.checkReasons()
	c.checkParams()
	c.checkOperationMap()
	for _, rec := range in.records {
		c.res.files++
		c.res.tests += rec.tests
		for _, r := range rec.requests {
			c.res.requests[r.mode]++
			c.checkRequest(r)
		}
	}
	if in.local != nil {
		for _, r := range in.local.requests {
			c.checkRequest(r)
		}
	}
	c.checkDecisions()
	c.checkFloor()
	return c.res
}

// rows returns every row of coverage.yaml, the control plane's first.
func (c *checker) rows() []*row {
	return append(slices.Clone(c.in.cov.ControlPlane), c.in.cov.PDP...)
}

// problem adds a problem, once.
func (c *checker) problem(format string, args ...any) {
	c.once(fmt.Sprintf(format, args...), format, args...)
}

// once adds a problem unless one with the key was added.
func (c *checker) once(key, format string, args ...any) {
	if !c.seen[key] {
		c.seen[key] = true
		c.res.problems = append(c.res.problems, fmt.Sprintf(format, args...))
	}
}

// checkRows checks the rows of a section against what the specs list, and counts them.
func (c *checker) checkRows(name string, rows []*row, listed map[string]*known) {
	s := sectionResult{
		name: name, rows: len(rows), byStatus: map[string]int{},
		byStage: map[string]map[string]int{}, sdkOnly: map[string]int{},
		undecided: map[string]int{},
	}
	ids := map[string]bool{}
	for _, k := range listed {
		ids[k.id] = true
		for _, alias := range k.aliases {
			ids[alias] = true
		}
	}
	decided := map[string]bool{}
	for _, r := range rows {
		decided[r.Route] = true
		s.byStatus[r.Status]++
		if r.SDKOnly != "" {
			s.sdkOnly[r.Status]++
		} else {
			stage := r.effectiveStage()
			if s.byStage[stage] == nil {
				s.byStage[stage] = map[string]int{}
			}
			s.byStage[stage][r.Status]++
		}
		c.checkRow(r, listed[r.Route], ids)
	}
	for _, key := range slices.Sorted(maps.Keys(listed)) {
		k := listed[key]
		if decided[key] {
			continue
		}
		s.undecided[k.stage]++
		if k.stage == stageGA {
			c.problem("%s %s (%s): a GA operation without a row in coverage.yaml; add a row "+
				"that decides it: planned, excluded with a reason, or deferred", name, key,
				orNone(k.id))
		}
	}
	c.res.sections = append(c.res.sections, s)
}

// checkRow checks a row against the route the specs list for it, or nil when none does.
func (c *checker) checkRow(r *row, k *known, ids map[string]bool) {
	sends := r.Status == statusCovered || r.Status == statusPlanned
	stage := stageGA
	switch {
	case k == nil && r.SDKOnly == "":
		c.problem("%s: the specs no longer list the route; remove the row, and decide the "+
			"operation that replaces it", r.name())
		return
	case k == nil && r.Op != "" && ids[r.Op]:
		c.problem("%s: its op %s is the operationId of another route of the spec; give the "+
			"route that no spec lists an op of its own", r.name(), r.Op)
	case k != nil && k.listed && r.SDKOnly != "":
		c.problem("%s: the spec lists the route now; remove sdk_only", r.name())
	case k != nil && !k.listed && sends && r.SDKOnly == "":
		c.problem("%s: no spec lists the route, so a %s row needs sdk_only with the reason the "+
			"SDK sends it", r.name(), r.Status)
	}
	if k != nil {
		stage = k.stage
		if k.id != "" && r.Op != k.id {
			c.problem("%s: the spec's operationId for the route is %s, not %q; fix op", r.name(),
				k.id, r.Op)
		}
	}
	if sends && r.Op == "" {
		c.problem("%s: a %s row needs an op; give a route that no spec lists an op of its own",
			r.name(), r.Status)
	}
	if r.Stage != stage {
		c.problem("%s: stage change: the spec marks it %s, and coverage.yaml says %s; decide "+
			"it again, then set its stage", r.name(), stage, r.Stage)
	}
}

// orNone returns id, or "no operationId" when it is empty.
func orNone(id string) string {
	if id == "" {
		return "no operationId"
	}
	return id
}

// checkReasons checks the reasons of the excluded rows against the vocabulary.
func (c *checker) checkReasons() {
	used := map[string]bool{}
	for _, r := range c.rows() {
		if r.Status != statusExcluded {
			continue
		}
		if _, ok := c.in.cov.Reasons[r.Reason]; !ok {
			c.problem("%s: the reason %q is not a key of reasons; use one of %s, or add a key "+
				"with a public sentence", r.name(), r.Reason,
				strings.Join(slices.Sorted(maps.Keys(c.in.cov.Reasons)), ", "))
			continue
		}
		used[r.Reason] = true
	}
	for _, key := range slices.Sorted(maps.Keys(c.in.cov.Reasons)) {
		if !used[key] {
			c.problem("reasons: no excluded row gives %s; remove it", key)
		}
	}
}

// checkParams checks sdk_only_params against the rows and the specs.
func (c *checker) checkParams() {
	for _, p := range c.in.cov.SDKOnlyParams {
		r := c.byOp[p.Op]
		if r == nil {
			c.problem("sdk_only_params: %s names no row of coverage.yaml", p.Op)
			continue
		}
		if r.Status != statusCovered && r.Status != statusPlanned {
			c.problem("sdk_only_params: %s is %s, so the SDK sends it no parameter; remove %s",
				p.Op, r.Status, p.Param)
		}
		k := c.inventory(r)[r.Route]
		if k == nil {
			continue
		}
		prefix, isPrefix := strings.CutSuffix(p.Param, "*")
		for _, name := range k.params {
			if name == p.Param || isPrefix && strings.HasPrefix(name, prefix) {
				c.problem("sdk_only_params: the spec lists %s for %s now; remove the entry",
					name, p.Op)
			}
		}
	}
}

// inventory returns the routes of the row's section.
func (c *checker) inventory(r *row) map[string]*known {
	if r.section == sectionPDP {
		return c.in.inv.pdp
	}
	return c.in.inv.controlPlane
}

// checkOperationMap checks that each operation of the map has a row, and notes the methods of
// each row.
func (c *checker) checkOperationMap() {
	for _, method := range slices.Sorted(maps.Keys(c.in.ops.Methods)) {
		for _, op := range c.in.ops.Methods[method] {
			r := c.byOp[op]
			if r == nil {
				c.problem("%s: the wire cases of %s send the op %s, which no row of "+
					"coverage.yaml has; fix the case's Op, or add the row", operationMapFile,
					method, op)
				continue
			}
			c.state[r].methods = append(c.state[r].methods, method)
		}
	}
}

// checkRequest finds the row of a request and checks it.
func (c *checker) checkRequest(r request) {
	key := r.target + " " + r.method + " " + r.path
	if strings.HasPrefix(r.path, testPrefix) {
		c.once("test route "+key, "%s: a request to the test-only route %s %s; the record "+
			"must leave out test routes", r.where(), r.method, r.path)
		return
	}
	matched := c.match(r)
	if matched == nil {
		c.once("unexplained "+key, "%s: an SDK-only request that no row explains: %s %s to "+
			"the %s (%s); add a row with sdk_only and the reason, or fix the request",
			r.where(), r.method, r.path, r.target, r.test)
		return
	}
	c.state[matched].requests[r.mode]++
	if r.template != "" {
		c.checkTemplate(r, matched)
	}
	route := r.method + " " + matched.path
	if v1, ok := c.in.seed.allows(matched.Op, route); !ok {
		c.once("v1 "+matched.Op+" "+route, "%s: the request for %s went to the route %s, but "+
			"v1 sent %s (%s), and %s lists no such change; use v1's route, or add the change "+
			"with its reason", r.where(), matched.Op, route, v1, c.in.seed.routesFile,
			c.in.seed.fixesFile)
	}
}

// checkTemplate checks that the template of a request fits its path and is the route of the
// row that the path reaches, matched.
func (c *checker) checkTemplate(r request, matched *row) {
	key := "template " + r.target + " " + r.method + " " + r.template + " " + r.path
	tmpl, _ := parseTemplate(r.template) // readRecord checked it.
	segments, _ := splitPath(r.path)     // match split it.
	if !tmpl.matches(segments) {
		c.once(key, "%s: the template %s does not fit the path %s %s; record the route that the "+
			"request was made for", r.where(), r.template, r.method, r.path)
		return
	}
	named := c.match(request{target: r.target, method: r.method, path: r.template})
	if named != matched {
		name := "no row"
		if named != nil {
			name = named.name()
		}
		c.once(key, "%s: the template %s of %s %s is the route of %s, but the path reaches %s; "+
			"record the route that the request was made for", r.where(), r.template, r.method,
			r.path, name, matched.name())
	}
}

// match returns the row of the request: the row of its target whose route takes its method
// and whose template matches its path with the most literal segments, or nil. A path under a
// mount of the container PDP matches as the path it stands for.
func (c *checker) match(r request) *row {
	segments, err := splitPath(r.path)
	if err != nil {
		return nil
	}
	rows, listed := c.in.cov.ControlPlane, c.in.inv.controlPlane
	if r.target == targetPDP {
		rows, listed = c.in.cov.PDP, c.in.inv.pdp
		for _, m := range c.in.inv.mounts {
			if resolved, err := m.resolve(segments); err == nil {
				segments = resolved
				break
			}
		}
	}
	var best *row
	for _, row := range rows {
		if !row.matchesMethod(r.method, listed[row.Route]) || !row.tmpl.matches(segments) {
			continue
		}
		if best == nil || moreLiteral(row.tmpl, best.tmpl) {
			best = row
		}
	}
	return best
}

// checkDecisions checks each row's status against its requests and the operation map, and
// lists the covered rows.
func (c *checker) checkDecisions() {
	for _, r := range c.rows() {
		st := c.state[r]
		recorded := st.requests[modeWire] + st.requests[modeDryRun] + st.requests[modeLive]
		sent := recorded + st.requests[modeLocal]
		switch {
		case r.Status == statusCovered && recorded == 0 && r.SDKOnly != "":
			c.problem("%s: an unused sdk_only entry: it is covered, but the records hold no "+
				"request to it", r.name())
		case r.Status == statusCovered && recorded == 0:
			c.problem("%s: untested: it is covered, but the records hold no request to it; add "+
				"a wire case, or mark it planned", r.name())
		case r.Status == statusPlanned && (sent > 0 || len(st.methods) > 0):
			c.problem("%s: stale: it is planned, but %s; mark it covered", r.name(),
				evidence(sent, st.methods))
		case (r.Status == statusExcluded || r.Status == statusDeferred) &&
			(sent > 0 || len(st.methods) > 0):
			c.problem("%s: it is %s, but %s; decide it again", r.name(), r.Status,
				evidence(sent, st.methods))
		}
		if r.Status == statusCovered {
			c.res.covered = append(c.res.covered, coveredRow{
				row: r, methods: st.methods, requests: recorded, e2e: c.e2e(st),
			})
		}
	}
}

// evidence says what shows that the SDK sends a row's route.
func evidence(requests int, methods []string) string {
	if len(methods) > 0 {
		return fmt.Sprintf("the wire cases of %s send it", strings.Join(methods, ", "))
	}
	return fmt.Sprintf("the records hold %s to it", count(requests, "request"))
}

// e2e returns the end-to-end state of a covered row: cloud when a live request reached it,
// else local when the run against a full local stack did, else advisory when one of its
// methods is advisory, else dry run when only an end-to-end scenario run offline reached it.
// A dry-run request is no end-to-end evidence.
func (c *checker) e2e(st *rowState) string {
	switch {
	case st.requests[modeLive] > 0:
		return e2eCloud
	case st.requests[modeLocal] > 0:
		return e2eLocal
	case slices.ContainsFunc(st.methods, func(m string) bool { return c.in.advisory[m] }):
		return e2eAdvisory
	case st.requests[modeDryRun] > 0:
		return e2eDryRun
	}
	return e2eNone
}

// checkFloor checks that the records hold at least one request for each covered row: fewer
// means that tests did not run.
func (c *checker) checkFloor() {
	total := 0
	for _, n := range c.res.requests {
		total += n
	}
	if total < len(c.res.covered) {
		c.res.incomplete = append(c.res.incomplete, fmt.Sprintf("the records hold %s, fewer "+
			"than the %s of coverage.yaml: tests did not run", count(total, "request"),
			count(len(c.res.covered), "covered row")))
	}
}
