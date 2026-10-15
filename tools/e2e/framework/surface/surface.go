// Package surface holds the completeness gate of the end-to-end scenarios: every public method
// of the SDK that sends a request, each method of testdata/operation-map.json, is in exactly one
// end-to-end state, and the methods add up to the states.
//
//   - cloud: a cloud scenario covers it, as the scenario declares, and the scenario's dry run
//     sent each of the method's operations;
//   - local-only: tools/e2e/local-only.txt lists it, with the reason that the cloud lane cannot
//     run it and the phase of the local harness that proves it instead;
//   - advisory: tools/e2e/advisory.txt lists it, with the ticket that allows its scenario to
//     skip, which .github/allowed-skips.txt names for a test of tools/e2e;
//   - pending: tools/e2e/pending.txt lists it, with the area of the scenario that will call it.
//
// A record holds operations, not methods, and two methods may send the same operation, so only a
// scenario's declaration makes a method cloud. A line of a list that names no such method, or a
// method that another state holds, fails the gate, so a local-only or pending line goes stale the
// moment a cloud scenario covers its method. So does a cloud scenario that sends an operation
// that needs an organization-level API key while tools/e2e/keys.yaml gives the cloud lane none.
package surface

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// The states of a method.
const (
	Cloud     = "cloud"
	LocalOnly = "local-only"
	Advisory  = "advisory"
	Pending   = "pending"
)

// States are the states, in the order of the count sentinel.
var States = []string{Cloud, LocalOnly, Advisory, Pending}

// Reasons are the reasons a method may be local-only: it needs an organization's API key, which
// the cloud lane does not have; it writes to the account, outside the scratch environment; it
// needs the ID of a PDP of the account; or the cloud lane cannot check its reply.
var Reasons = []string{"org-key", "account-level-write", "needs-pdp-id", "unverified-response"}

var (
	// methodName matches a method, such as Client.Check or UsersService.Get.
	methodName = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*\.[A-Z][A-Za-z0-9]*$`)

	// phase matches a phase of the local harness, such as api.projects, pdp.user_tenants or
	// sdk.config.
	phase = regexp.MustCompile(`^(offline|api|pdp|ab|sdk|probe)(\.[a-z0-9_]+)+$`)

	// ticket matches a ticket, such as TICKET-123.
	ticket = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[1-9][0-9]*$`)

	// area matches the area of a pending scenario, such as pdp-decisions.
	area = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
)

// pendingMarker is the second field of each line of pending.txt.
const pendingMarker = "e2e-pending"

// e2ePackage is the import path of the end-to-end scenarios, whose skips an entry of
// .github/allowed-skips.txt allows.
const e2ePackage = "github.com/permitio/permit-golang/v2/tools/e2e"

// Files are the inputs of the gate.
type Files struct {
	// OperationMap is testdata/operation-map.json: each public method that sends a request,
	// with the operations that it sends.
	OperationMap string

	// LocalOnly, Advisory and Pending are the lists of tools/e2e.
	LocalOnly, Advisory, Pending string

	// Keys is tools/e2e/keys.yaml, which says whether the cloud lane has an organization-level
	// API key.
	Keys string

	// AllowedSkips is .github/allowed-skips.txt, which must let a test of tools/e2e skip with
	// the ticket of each line of the advisory list.
	AllowedSkips string
}

// Scenario is a cloud scenario, as its dry run ran.
type Scenario struct {
	// Name is its name, such as scope_single_flight.
	Name string

	// Methods are the public methods that it covers, such as Client.Scope.
	Methods []string

	// Sent are the operations that its dry run sent.
	Sent []Op
}

// Op is an operation that a scenario sent.
type Op struct {
	// ID is its operationId, such as get_api_key_scope.
	ID string

	// OrganizationLevel says that it needs an organization-level API key.
	OrganizationLevel bool
}

// Result is what the gate found.
type Result struct {
	// States holds the state of each method that is in exactly one.
	States map[string]string

	// Counts holds the number of methods in each state, the number of methods under "methods",
	// and the number of methods in no state or in more than one under "unplaced".
	Counts map[string]int

	// Problems are the rules that do not hold: each fails the gate.
	Problems []string

	// Broken are the reasons that the gate did not run: an input that cannot be read, and a
	// count sentinel that does not hold.
	Broken []string
}

// Exit returns the exit code of r: 2 when the gate did not run, 1 when a rule does not hold,
// and 0 otherwise.
func (r Result) Exit() int {
	switch {
	case len(r.Broken) > 0:
		return 2
	case len(r.Problems) > 0:
		return 1
	}
	return 0
}

// Verdict returns the verdict of r: PASS, FAIL or DID NOT RUN.
func (r Result) Verdict() string {
	return [...]string{"PASS", "FAIL", "DID NOT RUN"}[r.Exit()]
}

// Summary returns the count sentinel of r, such as "12 methods = 2 cloud + 1 local-only + 0
// advisory + 9 pending + 0 unplaced".
func (r Result) Summary() string {
	var parts []string
	for _, s := range append(slices.Clip(States), "unplaced") {
		parts = append(parts, strconv.Itoa(r.Counts[s])+" "+s)
	}
	return fmt.Sprintf("%d methods = %s", r.Counts["methods"], strings.Join(parts, " + "))
}

// listed is a method that a line of a list names.
type listed struct {
	state, where string
}

// Check reads the files and checks the cloud scenarios of a dry run, scenarios, and that each
// method of the operation map is in exactly one state: cloud when a scenario covers it and sent
// each of its operations. It places no method when an input cannot be read.
func Check(files Files, scenarios []Scenario) Result {
	r := Result{States: map[string]string{}, Counts: map[string]int{}}
	methods, err := readOperationMap(files.OperationMap)
	if err != nil {
		r.Broken = append(r.Broken, err.Error())
		return r
	}
	keys, err := framework.ReadKeys(files.Keys)
	if err != nil {
		r.Broken = append(r.Broken, err.Error())
		return r
	}
	tickets, err := readSkipTickets(files.AllowedSkips)
	if err != nil {
		r.Broken = append(r.Broken, err.Error())
		return r
	}
	lists := map[string][]listed{}
	for _, list := range []struct {
		state, file string
		parse       func(fields []string) string
	}{
		{LocalOnly, files.LocalOnly, parseLocalOnly},
		{Advisory, files.Advisory, parseAdvisory(tickets)},
		{Pending, files.Pending, parsePending},
	} {
		problems, err := readList(list.file, list.state, list.parse, methods, lists)
		if err != nil {
			r.Broken = append(r.Broken, err.Error())
		}
		r.Problems = append(r.Problems, problems...)
	}
	if len(r.Broken) > 0 {
		return r
	}
	r.Problems = append(r.Problems, keyProblems(scenarios, keys)...)
	cover := covered(scenarios, methods)
	r.Problems = append(r.Problems, cover.problems...)
	for _, m := range slices.Sorted(maps.Keys(methods)) {
		place(&r, m, cover, lists[m])
	}
	r.Counts["methods"] = len(methods)
	checkSentinel(&r)
	return r
}

// keyProblems returns a problem for each operation that needs an organization-level API key and
// that a scenario of scenarios sent, unless keys gives the cloud lane such a key.
func keyProblems(scenarios []Scenario, keys framework.Keys) []string {
	if keys.OrganizationKey {
		return nil
	}
	var problems []string
	for _, s := range scenarios {
		seen := map[string]bool{}
		for _, op := range s.Sent {
			if !op.OrganizationLevel || seen[op.ID] {
				continue
			}
			seen[op.ID] = true
			problems = append(problems, fmt.Sprintf("the cloud scenario %s sends %s, which needs "+
				"an organization-level API key, and keys.yaml gives the cloud lane none: remove "+
				"the call from the cloud scenarios and list each method that sends %s in "+
				"local-only.txt with the reason org-key, or set organization_key in keys.yaml "+
				"once the lane has the key", s.Name, op.ID, op.ID))
		}
	}
	return problems
}

// coverage is what the cloud scenarios cover.
type coverage struct {
	// by holds each method that a scenario covers and sent each operation of, with the first
	// such scenario.
	by map[string]string

	// unsent holds each other method that a scenario covers, with why it is not cloud.
	unsent map[string]string

	// problems are the methods that a scenario covers and that the operation map does not hold.
	problems []string
}

// covered returns what scenarios cover of methods, the methods of the operation map.
func covered(scenarios []Scenario, methods map[string][]string) coverage {
	c := coverage{by: map[string]string{}, unsent: map[string]string{}}
	for _, s := range scenarios {
		sent := map[string]bool{}
		for _, op := range s.Sent {
			sent[op.ID] = true
		}
		for _, m := range s.Methods {
			ops, ok := methods[m]
			missing := slices.DeleteFunc(slices.Clone(ops), func(op string) bool {
				return sent[op]
			})
			switch {
			case !ok:
				c.problems = append(c.problems, fmt.Sprintf("the cloud scenario %s covers %s, "+
					"which is not a method of the operation map", s.Name, m))
			case len(ops) == 0:
				c.unsent[m] = "the cloud scenario " + s.Name + " covers it, and the operation " +
					"map gives it no operation to send"
			case len(missing) > 0:
				c.unsent[m] = "the cloud scenario " + s.Name + " covers it, and its dry run did " +
					"not send " + strings.Join(missing, ", ")
			case c.by[m] == "":
				c.by[m] = s.Name
			}
		}
	}
	return c
}

// checkSentinel adds to r.Broken when the methods in each state and those in none or in more
// than one do not add up to the methods of the operation map, which means that the gate placed a
// method twice or not at all.
func checkSentinel(r *Result) {
	sum := r.Counts["unplaced"]
	for _, s := range States {
		sum += r.Counts[s]
	}
	if sum != r.Counts["methods"] {
		r.Broken = append(r.Broken, "count sentinel: "+r.Summary()+" does not add up")
	}
}

// place gives the method m its state in r, or adds a problem to r when it is in none or in more
// than one: cloud when a scenario of cover covers it, and the states of lines, the lines that
// name m.
func place(r *Result, m string, cover coverage, lines []listed) {
	by := cover.by[m]
	cloud := by != ""
	if cloud {
		for _, l := range lines {
			r.Problems = append(r.Problems, fmt.Sprintf("%s: %s is stale: the cloud scenario %s "+
				"covers %s now, so remove the line", l.where, l.state, by, m))
		}
		lines = append([]listed{{state: Cloud}}, lines...)
	}
	if len(lines) != 1 {
		r.Counts["unplaced"]++
	}
	switch len(lines) {
	case 0:
		why, ok := cover.unsent[m]
		if !ok {
			why = "no cloud scenario covers it"
		}
		r.Problems = append(r.Problems, fmt.Sprintf("%s is in no end-to-end state: %s; cover it "+
			"in a cloud scenario that calls it, or list it in local-only.txt, advisory.txt or "+
			"pending.txt", m, why))
	case 1:
		r.States[m] = lines[0].state
		r.Counts[lines[0].state]++
	default:
		if !cloud {
			where := make([]string, 0, len(lines))
			for _, l := range lines {
				where = append(where, l.where)
			}
			r.Problems = append(r.Problems, fmt.Sprintf("%s is in more than one end-to-end "+
				"state: %s", m, strings.Join(where, ", ")))
		}
	}
}

// readOperationMap returns the methods of the operation map in file, each with its operations.
func readOperationMap(file string) (map[string][]string, error) {
	data, err := os.ReadFile(file) //nolint:gosec // G304: the gate reads the file it is given.
	if err != nil {
		return nil, fmt.Errorf("reading the operation map: %w", err)
	}
	var m struct {
		Methods map[string][]string `json:"methods"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parsing the operation map %s: %w", file, err)
	}
	if len(m.Methods) == 0 {
		return nil, fmt.Errorf("the operation map %s has no method", file)
	}
	return m.Methods, nil
}

// readSkipTickets returns the tickets of the entries of file, .github/allowed-skips.txt, that
// let a test of tools/e2e skip: <import path> <test name> <ticket> <expiry> <un-skip
// condition>. checkgotest checks the rest of each entry.
func readSkipTickets(file string) (map[string]bool, error) {
	data, err := os.ReadFile(file) //nolint:gosec // G304: the gate reads the file it is given.
	if err != nil {
		return nil, fmt.Errorf("reading the allowed skips: %w", err)
	}
	tickets := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == e2ePackage {
			tickets[fields[2]] = true
		}
	}
	return tickets, nil
}

// readList reads the list file of the state state, adds each method that a line names to lists,
// and returns a problem for each line that parse rejects, that names no method of methods or a
// method that an earlier line of the list names. Its error says that the file cannot be read.
func readList(file, state string, parse func(fields []string) string,
	methods map[string][]string, lists map[string][]listed,
) ([]string, error) {
	data, err := os.ReadFile(file) //nolint:gosec // G304: the gate reads the file it is given.
	if err != nil {
		return nil, fmt.Errorf("reading the %s list: %w", state, err)
	}
	var problems []string
	seen := map[string]bool{}
	for i, line := range strings.Split(string(data), "\n") {
		where := file + ":" + strconv.Itoa(i+1)
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		m := fields[0]
		problem := parse(fields)
		switch {
		case !methodName.MatchString(m):
			problem = "the method " + m + " is not <Type>.<Method>, such as Client.Check"
		case problem != "":
		case methods[m] == nil:
			problem = m + " is not a method of the operation map; remove the line"
		case seen[m]:
			problem = m + " has a line above already"
		}
		if problem != "" {
			problems = append(problems, where+": "+problem)
			continue
		}
		seen[m] = true
		lists[m] = append(lists[m], listed{state: state, where: where})
	}
	return problems, nil
}

// parseLocalOnly returns what is wrong with the fields of a line of local-only.txt, <Method>
// <reason> <phase>, or the empty string.
func parseLocalOnly(fields []string) string {
	switch {
	case len(fields) != 3:
		return "a line is <Method> <reason> <phase>, such as ProjectsService.List org-key " +
			"api.projects"
	case !slices.Contains(Reasons, fields[1]):
		return "the reason " + fields[1] + " is not one of " + strings.Join(Reasons, ", ")
	case !phase.MatchString(fields[2]):
		return "the phase " + fields[2] + " is not a phase of the local harness, such as " +
			"api.projects"
	}
	return ""
}

// parseAdvisory returns the parse of the fields of a line of advisory.txt, <Method> <ticket>
// <reason>, which returns what is wrong with them, or the empty string: the ticket must be one
// of tickets, those with which .github/allowed-skips.txt lets a test of tools/e2e skip.
func parseAdvisory(tickets map[string]bool) func(fields []string) string {
	return func(fields []string) string {
		switch {
		case len(fields) < 3:
			return "a line is <Method> <ticket> <reason>, such as Client.Check TICKET-123 the " +
				"PDP image is not published yet"
		case !ticket.MatchString(fields[1]):
			return "the ticket " + fields[1] + " is not a ticket, such as TICKET-123"
		case !tickets[fields[1]]:
			return "no entry of allowed-skips.txt lets a test of tools/e2e skip with the ticket " +
				fields[1] + ", and the scenario of an advisory method skips in the dry run too"
		}
		return ""
	}
}

// parsePending returns what is wrong with the fields of a line of pending.txt, <Method>
// e2e-pending <area>, or the empty string.
func parsePending(fields []string) string {
	switch {
	case len(fields) != 3 || fields[1] != pendingMarker:
		return "a line is <Method> " + pendingMarker + " <area>, such as Client.Check " +
			pendingMarker + " pdp-decisions"
	case !area.MatchString(fields[2]):
		return "the area " + fields[2] + " is not lower-case words joined by hyphens, such as " +
			"pdp-decisions"
	}
	return ""
}
