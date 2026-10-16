// Command scanner lists the uses of v1 of the Go SDK for Permit.io, the module
// github.com/permitio/permit-golang, in Go code, each with the entries of v2's migration guide,
// MIGRATION.md, that say what replaces it. The skill in the directory above runs it before and
// after a migration to v2.
//
// Usage:
//
//	scanner -guide MIGRATION.md [dir]
//
// -guide names MIGRATION.md of v2, such as the one in the directory that
// go list -m -f '{{.Dir}}' github.com/permitio/permit-golang/v2 prints. dir is the directory of
// the code, the current directory by default. scanner reads every Go file under dir, test files
// and the files of nested modules included, except in vendor and testdata directories and in
// directories whose names start with . or _, and except files whose names start with . or _.
//
// A use of v1 is one of these:
//
//   - import: an import of a package of v1, such as github.com/permitio/permit-golang/pkg/permit,
//     but not of v2 or later, whose paths end in /v2 and so on.
//   - identifier: a name that a package of v1 declares, qualified with its package, such as
//     config.NewConfigBuilder or a dot import's unqualified name, reported as
//     <package>.<name> with the package's own name, whatever name the file imports it under;
//     a method of a type of v1 that a call or a selector reaches, such as
//     client.Api.Users.Create, reported as <package>.<type>.<method>, here api.Users.Create; or
//     a method or a field of a value of a type of pkg/models, such as user.SetEmail on a
//     *models.UserCreate, reported as models.<type>.<name>, here models.UserCreate.SetEmail.
//
// scanner type-checks the code of each package with a model of v1's API, from v1-api.txt and
// v1-models-consts.txt: the types of v1's packages but pkg/openapi, the fields that lead from the
// client to its services, the methods of each type outside pkg/models, the constructors
// models.New<type> and models.New<type>WithDefaults, and each constant of pkg/models with its
// type. So it follows a value of a v1 type through variables, parameters, fields, composite
// literals and chained calls in a package. It does not follow:
//
//   - a value that a method of v1's API returns, unless the model has the result's type, as it
//     has for the builders and for the client: the user that client.Api.Users.Get returns has no
//     type, and user.GetEmail() on it is not found;
//   - a value across packages: a v1 value that a function of another package returns, or a
//     field that a struct of another package declares, such as a client that another package
//     holds, has no type, and a method called on it is not found;
//   - a call through one of v1's interfaces, such as permit.PermitInterface, which the model
//     declares with no method: only the name of the interface is found;
//   - a name of pkg/models that the model does not declare, such as PtrString, used through a
//     dot import of pkg/models;
//   - a call through pkg/openapi, v1's generated client, such as client.UsersApi.ListUsers: only
//     the import and the names that a file qualifies with the package, such as
//     openapi.NewConfiguration, are found.
//
// go build ./... finds each of these once the imports name v2, and the guide's entries say what
// replaces them; those whose v1: lines name openapi.<service>.<operation> cover pkg/openapi.
//
// The guide maps a use to entries by their IDs, such as M-users-5, each the anchor of an entry,
// which never changes: an import to the entries whose v1: line names v1's module path, and an
// identifier to the entries whose v1: line names it, as an id or a pattern such as
// api.Users.*. An identifier of pkg/models that no entry names, such as a type, a constructor,
// a constant, or a method or a field of a value, has the entries of its type: those whose v1:
// lines name the type, or else those that the type's row of the guide's models appendix links;
// the appendix lists every type of v1's models. A row that links no entry and names the same
// type in v2 gives the hit that v2 type.
//
// scanner writes a JSON object to standard output: hits, the uses in the order of their files
// and positions, and unmapped, the number of uses without an entry. Each hit has file, the
// path relative to dir with slashes; line and column, from 1, the column in bytes; kind, import
// or identifier; v1, the import path or the identifier; mapped, whether the guide maps it; ids,
// the IDs of its entries, in the guide's order, possibly none; and v2, the v2 models type that
// keeps the name of the hit's models type, when the appendix gives it and no entry does. The
// last line it writes to standard error is its verdict: PASS, FAIL or DID NOT RUN, then what it
// found.
//
// Exit codes:
//
//	0  no use of v1 was found, or the guide maps each one
//	1  a use of v1 has no entry in the guide; it is listed with mapped false
//	2  scanner did not run: the command line is wrong, the guide cannot be read or has no
//	   entry, dir is not a directory, or a directory or a Go file under it cannot be read or
//	   parsed; it writes nothing to standard output
//
// Build it before use, because go run reports every non-zero exit as 1. It needs only the
// standard library, so it builds offline; GOWORK=off builds it inside a directory tree that has
// a go.work file too:
//
//	GOWORK=off go -C <skill directory> build -o <bin> ./scanner
//	<bin> -guide <MIGRATION.md> .
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/types"
	"io"
	"os"
	"slices"
	"strings"
)

// The exit codes.
const (
	exitPassed    = 0
	exitUnmapped  = 1
	exitDidNotRun = 2
)

// usage starts the text of -h.
const usage = `Usage: scanner -guide MIGRATION.md [dir]

scanner lists the uses of v1 of the Go SDK for Permit.io, github.com/permitio/permit-golang, in
the Go files under dir, the current directory by default, as JSON, each with the entries of the
v2 migration guide that say what replaces it. It exits 0 when the guide maps each use or there
is none, 1 when a use has no entry, and 2 when it did not run.

Flags:
`

// hit is a use of v1 in the report, with what the guide maps it to.
type hit struct {
	File   string   `json:"file"`
	Line   int      `json:"line"`
	Column int      `json:"column"`
	Kind   string   `json:"kind"`
	V1     string   `json:"v1"`
	Mapped bool     `json:"mapped"`
	IDs    []string `json:"ids"`
	V2     string   `json:"v2,omitempty"`
}

// report is what scanner writes to standard output.
type report struct {
	Hits     []hit `json:"hits"`
	Unmapped int   `json:"unmapped"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run runs scanner with the arguments args and returns its exit code.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("scanner", flag.ContinueOnError)
	flags.SetOutput(stderr)
	guidePath := flags.String("guide", "", "the `path` of MIGRATION.md of v2")
	flags.Usage = func() {
		_, _ = io.WriteString(flags.Output(), usage)
		flags.PrintDefaults()
	}
	didNotRun := func(format string, values ...any) int {
		_, _ = fmt.Fprintf(stderr, "DID NOT RUN: "+format+"\n", values...)
		return exitDidNotRun
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitPassed
		}
		return didNotRun("%v", err)
	}
	switch {
	case *guidePath == "":
		return didNotRun("-guide is missing; pass the path of MIGRATION.md of v2, such as the " +
			"one in the directory that go list -m -f '{{.Dir}}' " +
			"github.com/permitio/permit-golang/v2 prints")
	case flags.NArg() > 1:
		return didNotRun("scanner takes one directory, and was given %d", flags.NArg())
	}
	dir := "."
	if flags.NArg() == 1 {
		dir = flags.Arg(0)
	}
	text, err := os.ReadFile(*guidePath)
	if err != nil {
		return didNotRun("read the guide: %v", err)
	}
	g, err := parseGuide(string(text))
	if err != nil {
		return didNotRun("read the guide %s: %v", *guidePath, err)
	}
	pkgs, err := v1Model()
	if err != nil {
		return didNotRun("build the model of v1: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return didNotRun("%v", err)
	}
	if !info.IsDir() {
		return didNotRun("%s is not a directory; pass the directory of the code to scan", dir)
	}
	uses, err := scan(os.DirFS(dir), pkgs)
	if err != nil {
		return didNotRun("scan %s: %v", dir, err)
	}
	out := newReport(uses, g)
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(out); err != nil {
		return didNotRun("write the report: %v", err)
	}
	_, _ = fmt.Fprintln(stderr, verdict(out))
	if out.Unmapped > 0 {
		return exitUnmapped
	}
	return exitPassed
}

// v1Model builds the packages of v1 from v1-api.txt and v1-models-consts.txt.
func v1Model() (map[string]*types.Package, error) {
	items, err := parseV1API(v1API)
	if err != nil {
		return nil, fmt.Errorf("v1-api.txt: %w", err)
	}
	consts, err := parseV1Consts(v1Consts)
	if err != nil {
		return nil, fmt.Errorf("v1-models-consts.txt: %w", err)
	}
	return newV1Packages(slices.Concat(items, consts))
}

// newReport maps each use to its entries in g.
func newReport(uses []use, g guide) report {
	out := report{Hits: make([]hit, 0, len(uses))}
	for _, u := range uses {
		h := hit{File: u.file, Line: u.line, Column: u.column, Kind: u.kind, V1: u.v1}
		ids := g.module
		if u.kind == kindIdentifier {
			ids, h.V2 = g.resolve(u.v1, u.typ)
		}
		h.IDs = append([]string{}, ids...)
		h.Mapped = len(h.IDs) > 0 || h.V2 != ""
		if !h.Mapped {
			out.Unmapped++
		}
		out.Hits = append(out.Hits, h)
	}
	return out
}

// verdict returns the last line that scanner writes to standard error.
func verdict(out report) string {
	if len(out.Hits) == 0 {
		return "PASS: no use of v1"
	}
	if out.Unmapped == 0 {
		return fmt.Sprintf("PASS: %s of v1, each with an entry of the guide or a v2 type",
			count(len(out.Hits)))
	}
	var unmapped []string
	for _, h := range out.Hits {
		if !h.Mapped && !slices.Contains(unmapped, h.V1) {
			unmapped = append(unmapped, h.V1)
		}
	}
	slices.Sort(unmapped)
	return fmt.Sprintf("FAIL: %s of v1 without an entry in the guide, of %d: %s",
		count(out.Unmapped), len(out.Hits), strings.Join(unmapped, ", "))
}

// count returns "1 use" or "<n> uses".
func count(n int) string {
	if n == 1 {
		return "1 use"
	}
	return fmt.Sprintf("%d uses", n)
}
