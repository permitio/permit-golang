package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

// recordFormat is the version of the record format that apicoverage reads, which
// internal/testhook writes.
const recordFormat = 1

// The modes of a request in a record.
const (
	modeWire   = "wire"
	modeDryRun = "dryrun"
	modeLive   = "live"
	modeLocal  = "local"
)

// The targets of a request.
const (
	targetAPI = "api"
	targetPDP = "pdp"
)

// testPrefix starts the path of a test-only route, which a record must not hold.
const testPrefix = "/__test/"

// requestMethod matches a request method.
var requestMethod = regexp.MustCompile(`^[A-Z]+$`)

// recordLine is a line of a record, as internal/testhook writes it.
type recordLine struct {
	Type       string `json:"type"`
	Format     *int   `json:"format"`
	Mode       string `json:"mode"`
	Target     string `json:"target"`
	Method     string `json:"method"`
	Template   string `json:"template"`
	Path       string `json:"path"`
	Status     *int   `json:"status"`
	Test       string `json:"test"`
	ExitStatus *int   `json:"exit_status"`
	Tests      *int   `json:"tests"`
}

// request is a request of a record.
type request struct {
	file     string
	line     int
	mode     string
	target   string
	method   string
	template string
	path     string
	status   int
	test     string
}

// where returns the file and line of the request, for messages.
func (r request) where() string {
	return fmt.Sprintf("%s:%d", r.file, r.line)
}

// record is a request record that apicoverage read.
type record struct {
	file     string
	requests []request
	tests    int
}

// readRecord reads a record, and checks that it is complete: a header, request lines whose mode
// is one of modes, and the session trailer last, from a run whose tests passed and of which at
// least one took part.
func readRecord(file string, modes []string) (*record, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	rec := &record{file: file}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(nil, 1<<20)
	n := 0
	ended := false
	for scanner.Scan() {
		n++
		where := fmt.Sprintf("%s:%d", file, n)
		if ended {
			return nil, fmt.Errorf("%s: a line after the session trailer", where)
		}
		var l recordLine
		dec := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&l); err != nil {
			return nil, fmt.Errorf("%s: not a line of a request record: %w", where, err)
		}
		if dec.More() {
			return nil, fmt.Errorf("%s: more than one JSON value", where)
		}
		if n == 1 {
			if err := checkHeader(l); err != nil {
				return nil, fmt.Errorf("%s: %w", where, err)
			}
			continue
		}
		switch l.Type {
		case "request":
			r, err := checkRequest(l, modes)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", where, err)
			}
			r.file, r.line = file, n
			rec.requests = append(rec.requests, r)
		case "session":
			if err := checkSession(l, rec); err != nil {
				return nil, fmt.Errorf("%s: %w", where, err)
			}
			ended = true
		default:
			return nil, fmt.Errorf("%s: the type %q is not request or session", where, l.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	switch {
	case n == 0:
		return nil, fmt.Errorf("%s is empty: the run that writes it did not start", file)
	case !ended:
		return nil, fmt.Errorf("%s has no session trailer: the run that wrote it did not "+
			"finish", file)
	}
	return rec, nil
}

// checkHeader checks the first line of a record.
func checkHeader(l recordLine) error {
	if l.Type != "header" || l.Format == nil || *l.Format != recordFormat ||
		l != (recordLine{Type: l.Type, Format: l.Format}) {
		return fmt.Errorf("the first line is not the header of a record of format %d, "+
			`{"type":"header","format":%d}`, recordFormat, recordFormat)
	}
	return nil
}

// checkRequest checks a request line and returns the request.
func checkRequest(l recordLine, modes []string) (request, error) {
	r := request{
		mode: l.Mode, target: l.Target, method: l.Method, template: l.Template,
		path: l.Path, test: l.Test,
	}
	if l.Status != nil {
		r.status = *l.Status
	}
	_, pathErr := splitPath(l.Path)
	switch {
	case l.Format != nil || l.ExitStatus != nil || l.Tests != nil:
		return request{}, errors.New("a request line has a field of another line type")
	case !slices.Contains(modes, l.Mode):
		return request{}, fmt.Errorf("the mode %q is not one of %v here", l.Mode, modes)
	case l.Target != targetAPI && l.Target != targetPDP:
		return request{}, fmt.Errorf("the target %q is not api or pdp", l.Target)
	case !requestMethod.MatchString(l.Method):
		return request{}, fmt.Errorf("the method %q is not a request method", l.Method)
	case strings.ContainsAny(l.Path, "?#"):
		return request{}, fmt.Errorf("the path %q holds a query or a fragment, which a record "+
			"leaves out", l.Path)
	case pathErr != nil:
		return request{}, pathErr
	case l.Status == nil || r.status != 0 && (r.status < 100 || r.status > 599):
		return request{}, errors.New("the status must be 0, for no response, or 100 to 599")
	case l.Test == "":
		return request{}, errors.New("the request names no test")
	}
	if l.Template != "" {
		if _, err := parseTemplate(l.Template); err != nil {
			return request{}, fmt.Errorf("the template: %w", err)
		}
	}
	return r, nil
}

// checkSession checks the session trailer.
func checkSession(l recordLine, rec *record) error {
	switch {
	case l.ExitStatus == nil || l.Tests == nil ||
		l != (recordLine{Type: l.Type, ExitStatus: l.ExitStatus, Tests: l.Tests}):
		return errors.New(`the session trailer is {"type":"session","exit_status":<code>,` +
			`"tests":<count>}`)
	case *l.ExitStatus != 0:
		return fmt.Errorf("the tests that wrote the record failed (exit status %d), so it may "+
			"lack requests; fix the tests first", *l.ExitStatus)
	case *l.Tests <= 0:
		return fmt.Errorf("no test took part in the run that wrote the record (tests: %d); "+
			"check what it ran", *l.Tests)
	}
	rec.tests = *l.Tests
	return nil
}
