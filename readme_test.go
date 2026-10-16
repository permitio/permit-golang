package permit_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"go/version"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// compileFiles are the Markdown files whose marked go blocks TestReadmeBlocksCompile compiles.
var compileFiles = []string{"README.md"}

// compileMarker is the line that marks the fenced go block on the next line as a complete
// program, which TestReadmeBlocksCompile compiles.
const compileMarker = "<!-- compile -->"

// goBlock is a fenced go block of a Markdown file.
type goBlock struct {
	// file is the Markdown file.
	file string

	// line is the line of the block's opening fence.
	line int

	// code is the text between the fences, each line with its newline.
	code string
}

// markedBlocks returns the go blocks of text, the content of the Markdown file name, that a
// compile marker marks, the number of markers, and a problem for each marker that is not
// followed by a go block. A marker is a line that holds only compileMarker, outside a fenced
// block, and the opening fence of its block, three or more backticks or tildes at the start of a
// line followed by go, is the next line.
func markedBlocks(name, text string) (blocks []goBlock, markers int, problems []string) {
	lines := strings.Split(text, "\n")
	marker := 0 // the line of the marker that the next line must follow, or 0
	for i := 0; i < len(lines); i++ {
		n := i + 1
		fence, info, isFence := openingFence(lines[i])
		if !isFence {
			if marker != 0 {
				problems = append(problems, fmt.Sprintf("%s:%d: %s is not followed by a fenced "+
					"go block on the next line", name, marker, compileMarker))
				marker = 0
			}
			if strings.TrimSpace(lines[i]) == compileMarker {
				markers++
				marker = n
			}
			continue
		}
		end := closingFence(lines, i+1, fence)
		if end < 0 {
			problems = append(problems, fmt.Sprintf("%s:%d: the block that %s opens is not "+
				"closed", name, n, fence))
			return blocks, markers, problems
		}
		if marker != 0 {
			if language, _, _ := strings.Cut(info, " "); language == "go" {
				blocks = append(blocks, goBlock{
					file: name, line: n,
					code: strings.Join(lines[i+1:end], "\n") + "\n",
				})
			} else {
				problems = append(problems, fmt.Sprintf("%s:%d: %s is followed by a block of %q, "+
					"not a go block", name, marker, compileMarker, info))
			}
			marker = 0
		}
		i = end
	}
	if marker != 0 {
		problems = append(problems, fmt.Sprintf("%s:%d: %s is not followed by a fenced go block "+
			"on the next line", name, marker, compileMarker))
	}
	return blocks, markers, problems
}

// openingFence returns the fence that line opens and the fence's info string, such as ``` and
// go, when line starts with three or more backticks or tildes. The info string of a backtick
// fence has no backtick.
func openingFence(line string) (fence, info string, ok bool) {
	for _, char := range []string{"`", "~"} {
		rest := strings.TrimLeft(line, char)
		if len(line)-len(rest) < 3 || char == "`" && strings.Contains(rest, "`") {
			continue
		}
		return line[:len(line)-len(rest)], strings.TrimSpace(rest), true
	}
	return "", "", false
}

// closingFence returns the index of the first line of lines from start on that closes the block
// that fence opened: the fence's character at least as many times, and nothing else but spaces.
// It returns -1 when no line does.
func closingFence(lines []string, start int, fence string) int {
	for i := start; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		if len(line) >= len(fence) && strings.Trim(line, fence[:1]) == "" {
			return i
		}
	}
	return -1
}

// readmeModule returns the go.mod and go.sum of the module that compileBlocks builds each block
// in: it requires the SDK and replaces it with the repository root, has the go line of the SDK's
// go.mod, and has the SDK's go.sum, so that it builds offline.
func readmeModule(t *testing.T) (goMod, goSum []byte) {
	t.Helper()
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	sdkMod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	goLine := ""
	for line := range strings.Lines(string(sdkMod)) {
		if strings.HasPrefix(line, "go ") {
			goLine = line
		}
	}
	if goLine == "" {
		t.Fatal("go.mod has no go line")
	}
	goMod = fmt.Appendf(nil, "module example.com/readme\n\n%s\nrequire %s v2.0.0\n\n"+
		"replace %s => %s\n", goLine, sdkModule.path, sdkModule.path, strconv.Quote(root))
	goSum, err = os.ReadFile("go.sum")
	if err != nil {
		t.Fatal(err)
	}
	return goMod, goSum
}

// compileBlocks compiles each of blocks as the main package of a module that requires the SDK
// and replaces it with the repository root, offline, and returns a problem for each block that
// is not a program or does not compile, a problem for each error that a block that compiles
// drops, as droppedErrors finds them, and the number of blocks it built: those it ran go build
// on, whether they compile or not. A //line directive makes the compiler name the lines of the
// Markdown file.
func compileBlocks(t *testing.T, blocks []goBlock) (problems []string, built int) {
	t.Helper()
	goMod, goSum := readmeModule(t)
	env := slices.Concat(os.Environ(), []string{"GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off"})
	for _, b := range blocks {
		// A block without a package clause goes to the compiler, which names what is missing.
		clause, parseErr := parser.ParseFile(token.NewFileSet(), "", b.code,
			parser.PackageClauseOnly)
		if parseErr == nil && clause.Name.Name != "main" {
			problems = append(problems, fmt.Sprintf("%s:%d: the go block is package %s; a marked "+
				"block is a complete program of package main, with its imports", b.file, b.line,
				clause.Name.Name))
			continue
		}
		dir := t.TempDir()
		mainGo := fmt.Appendf(nil, "//line %s:%d:1\n%s", b.file, b.line+1, b.code)
		for name, data := range map[string][]byte{
			"go.mod":  goMod,
			"go.sum":  goSum,
			"main.go": mainGo,
		} {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.CommandContext(t.Context(), "go", "build", "-o", os.DevNull, ".")
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		built++
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s:%d: the go block does not compile: %v\n%s",
				b.file, b.line, err, out))
			continue
		}
		problems = append(problems, droppedErrors(t, dir, mainGo, env)...)
	}
	return problems, built
}

// errorType is the type error, the type of each value that droppedErrors reports.
var errorType = types.Universe.Lookup("error").Type()

// printFuncs are the functions of fmt that write to standard output, whose call statements
// droppedErrors allows, as errcheck does: a go block prints its results with them.
var printFuncs = []string{"fmt.Print", "fmt.Printf", "fmt.Println"}

// droppedErrors returns a problem for each value of type error that the program src drops, the
// main.go that compileBlocks compiled in dir with env: a blank identifier that an assignment or a
// var declaration gives it, such as user, _ := ...; a call statement whose call returns it, alone
// or with other results, such as client.Users.Delete(ctx, "bob") on a line of its own, but for a
// call of one of printFuncs; and a range over a function iterator that gives it to a blank
// identifier or leaves out the iteration variable that would hold it, such as
// for user, _ := range seq or for user := range seq, where seq yields a user and an error. A defer
// or go statement is not a call statement, so defer client.Close() passes. A go block that drops
// an error teaches the reader to drop it. droppedErrors type-checks src with the export data that
// go list -export gives for the packages that it imports, and names each place by the Markdown
// file and line that src's //line directive gives.
func droppedErrors(t *testing.T, dir string, src []byte, env []string) []string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-export", "-deps",
		"-f", "{{.ImportPath}} {{.Export}}", ".")
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = exitErr.Stderr
		}
		t.Fatalf("go list -export in %s: %v\n%s", dir, err, stderr)
	}
	exports := map[string]string{}
	for line := range strings.Lines(string(out)) {
		path, export, _ := strings.Cut(strings.TrimSpace(line), " ")
		exports[path] = export
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, 0)
	if err != nil {
		t.Fatalf("parsing a go block that compiles: %v", err)
	}
	lookup := func(path string) (io.ReadCloser, error) {
		if exports[path] == "" {
			return nil, fmt.Errorf("go list -export gave no export data for %s", path)
		}
		return os.Open(exports[path])
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "gc", lookup)}
	if _, err := conf.Check("main", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("type-checking a go block that compiles: %v", err)
	}
	var problems []string
	report := func(pos token.Pos, typ types.Type, how string) {
		if typ != nil && types.Identical(typ, errorType) {
			problems = append(problems, fmt.Sprintf("%s: %s a value of type %s; a go block "+
				"handles every error", fset.Position(pos), how, typ))
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ExprStmt:
			call, ok := ast.Unparen(n.X).(*ast.CallExpr)
			if !ok || slices.Contains(printFuncs, calledFunc(info, call)) {
				break
			}
			for _, typ := range resultTypes(info.TypeOf(call)) {
				report(call.Pos(), typ, "the call statement drops")
			}
		case *ast.AssignStmt:
			for i, typ := range valueTypes(info, len(n.Lhs), n.Rhs) {
				if isBlank(n.Lhs[i]) {
					report(n.Lhs[i].Pos(), typ, "the blank identifier drops")
				}
			}
		case *ast.ValueSpec:
			for i, typ := range valueTypes(info, len(n.Names), n.Values) {
				if isBlank(n.Names[i]) {
					report(n.Names[i].Pos(), typ, "the blank identifier drops")
				}
			}
		case *ast.RangeStmt:
			key, value := yieldTypes(info.TypeOf(n.X))
			for _, v := range []struct {
				expr ast.Expr
				typ  types.Type
			}{{n.Key, key}, {n.Value, value}} {
				switch {
				case v.expr == nil:
					report(n.For, v.typ, "the range clause leaves out")
				case isBlank(v.expr):
					report(v.expr.Pos(), v.typ, "the blank identifier drops")
				}
			}
		}
		return true
	})
	return problems
}

// valueTypes returns the types of the n values that values give to n names or operands, in
// order: one value each, or the results of one call. It returns nil when it cannot tell them,
// such as for a declaration without values.
func valueTypes(info *types.Info, n int, values []ast.Expr) []types.Type {
	result := make([]types.Type, n)
	if len(values) == n {
		for i, v := range values {
			result[i] = info.TypeOf(v)
		}
		return result
	}
	if len(values) != 1 {
		return nil
	}
	tuple, ok := info.TypeOf(values[0]).(*types.Tuple)
	if !ok || tuple.Len() != n {
		return nil
	}
	for i := range n {
		result[i] = tuple.At(i).Type()
	}
	return result
}

// resultTypes returns the types of the results of a call whose type is typ, as types.Info.TypeOf
// gives it: those of a tuple, the type of a call of no result or of more than one, or else typ.
func resultTypes(typ types.Type) []types.Type {
	tuple, ok := typ.(*types.Tuple)
	if !ok {
		return []types.Type{typ}
	}
	result := make([]types.Type, tuple.Len())
	for i := range tuple.Len() {
		result[i] = tuple.At(i).Type()
	}
	return result
}

// calledFunc returns the full name of the function or method that call names, such as
// fmt.Println, as info.Uses gives it, and "" for a call of anything else, such as a function
// value that another call returns.
func calledFunc(info *types.Info, call *ast.CallExpr) string {
	var name *ast.Ident
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		name = fun
	case *ast.SelectorExpr:
		name = fun.Sel
	}
	if fn, ok := info.Uses[name].(*types.Func); ok {
		return fn.FullName()
	}
	return ""
}

// yieldTypes returns the types of the values that a function iterator of type typ yields, such as
// an iter.Seq2[models.UserRead, error], to a range clause: the key and the value of a function
// whose yield takes two, the key alone of one whose yield takes one, and none for any other type
// or a nil one.
func yieldTypes(typ types.Type) (key, value types.Type) {
	if typ == nil {
		return nil, nil
	}
	seq, ok := typ.Underlying().(*types.Signature)
	if !ok || seq.Params().Len() != 1 {
		return nil, nil
	}
	yield, ok := seq.Params().At(0).Type().Underlying().(*types.Signature)
	if !ok {
		return nil, nil
	}
	switch params := yield.Params(); params.Len() {
	case 1:
		return params.At(0).Type(), nil
	case 2:
		return params.At(0).Type(), params.At(1).Type()
	default:
		return nil, nil
	}
}

// isBlank reports whether e is the blank identifier.
func isBlank(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "_"
}

// compileSentinel returns a problem when the number of blocks built is not the number of compile
// markers, and "" otherwise.
func compileSentinel(markers, built int) string {
	if markers == built {
		return ""
	}
	return fmt.Sprintf("count sentinel: %d %s markers, but %d go blocks were built", markers,
		compileMarker, built)
}

func TestReadmeBlocksCompile(t *testing.T) {
	var blocks []goBlock
	markers := 0
	for _, name := range compileFiles {
		text, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		b, m, problems := markedBlocks(name, string(text))
		for _, problem := range problems {
			t.Error(problem)
		}
		blocks = append(blocks, b...)
		markers += m
	}
	problems, built := compileBlocks(t, blocks)
	for _, problem := range problems {
		t.Error(problem)
	}
	if problem := compileSentinel(markers, built); problem != "" {
		t.Error(problem)
	}
	t.Logf("built %d go blocks for %d %s markers in %s", built, markers, compileMarker,
		strings.Join(compileFiles, ", "))
}

// readReadmeFixture returns the marked go blocks of testdata/readme/<name> and the number of
// markers, and fails the test when the file has a problem.
func readReadmeFixture(t *testing.T, name string) ([]goBlock, int) {
	t.Helper()
	file := filepath.Join("testdata", "readme", name)
	text, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	blocks, markers, problems := markedBlocks(filepath.ToSlash(file), string(text))
	if problems != nil {
		t.Fatalf("%s has problems:\n%s", file, strings.Join(problems, "\n"))
	}
	return blocks, markers
}

func TestReadmeBlocksCompileAFixture(t *testing.T) {
	blocks, markers := readReadmeFixture(t, "compiles.md")
	problems, built := compileBlocks(t, blocks)
	if problems != nil {
		t.Errorf("compiling the blocks of compiles.md found problems:\n%s",
			strings.Join(problems, "\n"))
	}
	if markers != 2 || built != 2 {
		t.Errorf("compiles.md has %d markers and %d built blocks, want 2 and 2", markers, built)
	}
}

func TestReadmeBlocksThatDoNotCompile(t *testing.T) {
	blocks, markers := readReadmeFixture(t, "broken.md")
	problems, built := compileBlocks(t, blocks)
	if markers != 2 || built != 1 || len(problems) != 2 {
		t.Fatalf("broken.md has %d markers, %d built blocks and the problems\n%s\nwant 2, 1 "+
			"and 2 problems", markers, built, strings.Join(problems, "\n"))
	}
	const file = "testdata/readme/broken.md"
	for _, want := range []string{
		file + ":5: the go block does not compile",
		// The //line directive makes the compiler name the line and column in the fixture.
		file + ":15:21: undefined: permit.Nothing",
	} {
		if !strings.Contains(problems[0], want) {
			t.Errorf("the first problem is\n%s\nwant it to hold %q", problems[0], want)
		}
	}
	want := file + ":20: the go block is package readme; a marked block is a complete program " +
		"of package main, with its imports"
	if problems[1] != want {
		t.Errorf("the second problem is\n%s\nwant\n%s", problems[1], want)
	}
	if problem := compileSentinel(markers, built); problem == "" {
		t.Error("compileSentinel passes 2 markers with 1 built block")
	}
}

func TestReadmeMarkedBlocks(t *testing.T) {
	const (
		name = "README.md"

		// noBlock is the problem of a marker on line 1 or 2 that no go block follows.
		noBlock = " <!-- compile --> is not followed by a fenced go block on the next line"
	)
	for _, tc := range []struct {
		name, text   string
		wantBlocks   []goBlock
		wantMarkers  int
		wantProblems []string
	}{
		{name: "no markers", text: "# SDK\n\n```go\nnot compiled\n```\n"},
		{
			name: "two marked blocks",
			text: "<!-- compile -->\n```go\npackage main\n```\n\n<!-- compile -->\n" +
				"~~~~ go title\nfunc main() {}\n\n~~~~~\n",
			wantBlocks: []goBlock{
				{file: name, line: 2, code: "package main\n"},
				{file: name, line: 7, code: "func main() {}\n\n"},
			},
			wantMarkers: 2,
		},
		{
			name: "a marker in a fence",
			text: "````markdown\n<!-- compile -->\n```go\nx\n```\n````\n",
		},
		{
			name:         "a marker before a blank line",
			text:         "<!-- compile -->\n\n```go\npackage main\n```\n",
			wantMarkers:  1,
			wantProblems: []string{"README.md:1:" + noBlock},
		},
		{
			name:         "a marker before another marker",
			text:         "  <!-- compile -->  \n<!-- compile -->\n```go\npackage main\n```\n",
			wantBlocks:   []goBlock{{file: name, line: 3, code: "package main\n"}},
			wantMarkers:  2,
			wantProblems: []string{"README.md:1:" + noBlock},
		},
		{
			name:        "a marker before a shell block",
			text:        "<!-- compile -->\n```sh\ngo test\n```\n",
			wantMarkers: 1,
			wantProblems: []string{
				`README.md:1: <!-- compile --> is followed by a block of "sh", not a go block`,
			},
		},
		{
			name:        "a marker before a block of golang",
			text:        "<!-- compile -->\n```golang\npackage main\n```\n",
			wantMarkers: 1,
			wantProblems: []string{
				`README.md:1: <!-- compile --> is followed by a block of "golang", not a go block`,
			},
		},
		{
			name:         "a marker on the last line",
			text:         "text\n<!-- compile -->",
			wantMarkers:  1,
			wantProblems: []string{"README.md:2:" + noBlock},
		},
		{
			name:         "a block that is not closed",
			text:         "<!-- compile -->\n```go\npackage main\n``\n~~~\n",
			wantMarkers:  1,
			wantProblems: []string{"README.md:2: the block that ``` opens is not closed"},
		},
		{
			name:         "an indented fence",
			text:         "<!-- compile -->\n ```go\npackage main\n ```\n",
			wantMarkers:  1,
			wantProblems: []string{"README.md:1:" + noBlock},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks, markers, problems := markedBlocks(name, tc.text)
			if diff := cmp.Diff(tc.wantBlocks, blocks, cmp.AllowUnexported(goBlock{})); diff != "" {
				t.Errorf("blocks differ (-want +got):\n%s", diff)
			}
			if markers != tc.wantMarkers {
				t.Errorf("markers = %d, want %d", markers, tc.wantMarkers)
			}
			if diff := cmp.Diff(tc.wantProblems, problems); diff != "" {
				t.Errorf("problems differ (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReadmeOpeningFence(t *testing.T) {
	for _, tc := range []struct {
		line, fence, info string
		ok                bool
	}{
		{line: "```go", fence: "```", info: "go", ok: true},
		{line: "````  go  ", fence: "````", info: "go", ok: true},
		{line: "~~~", fence: "~~~", ok: true},
		{line: "~~~ a`b", fence: "~~~", info: "a`b", ok: true},
		{line: "```a`b"},
		{line: "``go"},
		{line: " ```go"},
		{line: "text ```"},
	} {
		fence, info, ok := openingFence(tc.line)
		if fence != tc.fence || info != tc.info || ok != tc.ok {
			t.Errorf("openingFence(%q) = %q, %q, %v, want %q, %q, %v", tc.line, fence, info, ok,
				tc.fence, tc.info, tc.ok)
		}
	}
}

func TestReadmeClosingFence(t *testing.T) {
	lines := []string{"```", "````", "~~~", "``` go", "```` ", "x"}
	for _, tc := range []struct {
		start int
		fence string
		want  int
	}{
		{start: 0, fence: "```", want: 0},
		{start: 1, fence: "```", want: 1},
		{start: 0, fence: "````", want: 1},
		{start: 2, fence: "````", want: 4},
		{start: 0, fence: "~~~", want: 2},
		{start: 3, fence: "~~~", want: -1},
		{start: 5, fence: "```", want: -1},
		{start: 6, fence: "```", want: -1},
	} {
		if got := closingFence(lines, tc.start, tc.fence); got != tc.want {
			t.Errorf("closingFence(lines, %d, %q) = %d, want %d", tc.start, tc.fence, got, tc.want)
		}
	}
}

func TestReadmeCompileSentinel(t *testing.T) {
	if problem := compileSentinel(0, 0); problem != "" {
		t.Errorf("compileSentinel(0, 0) = %q, want none", problem)
	}
	if problem := compileSentinel(3, 3); problem != "" {
		t.Errorf("compileSentinel(3, 3) = %q, want none", problem)
	}
	want := "count sentinel: 2 <!-- compile --> markers, but 1 go blocks were built"
	if problem := compileSentinel(2, 1); problem != want {
		t.Errorf("compileSentinel(2, 1) = %q, want %q", problem, want)
	}
	if problem := compileSentinel(1, 2); problem == "" {
		t.Error("compileSentinel(1, 2) passes")
	}
}

// The tests below check the tables of README.md that the wire cases and the operation registry
// give: which URL each call uses, the calls that only the PDP container serves, and the calls
// that can wait for sync. Each table is the lines between a line <!-- table <name> --> and the
// next line <!-- end table -->, and go test -run TestReadmeTables -update . rewrites them.

// readmeFile is the Markdown file that holds the tables.
const readmeFile = "README.md"

// The lines that start and end a table: tableStart, the table's name and tableStartEnd, and
// tableEnd.
const (
	tableStart    = "<!-- table "
	tableStartEnd = " -->"
	tableEnd      = "<!-- end table -->"
)

// The names of the tables.
const (
	// serversTable names the URL that each call uses.
	serversTable = "servers"

	// containerOnlyTable names the calls that only the PDP container serves, with their routes.
	containerOnlyTable = "container-only"

	// waitsTable names the calls that can wait for sync, with their routes on the PDP container.
	waitsTable = "waits-for-sync"
)

// readmeTable is a table as README.md holds it: the two lines of its header and its rows.
type readmeTable struct {
	header []string
	rows   []tableRow
}

// tableRow is a row of a table: its key, the text of its first cell, which no other row of the
// table has, and its line.
type tableRow struct {
	key, line string
}

// lines returns the lines of t as the file holds them between its start line and its end line:
// a blank line, its header, its rows and a blank line, so that a Markdown renderer reads the
// table apart from the lines around it.
func (t readmeTable) lines() []string {
	lines := append([]string{""}, t.header...)
	for _, row := range t.rows {
		lines = append(lines, row.line)
	}
	return append(lines, "")
}

// newRow returns the row whose cells are cells, the first of which is its key.
func newRow(cells ...string) tableRow {
	return tableRow{key: cells[0], line: "| " + strings.Join(cells, " | ") + " |"}
}

// serverURLs name the URL that a method uses, as the servers table writes it, by the modes of
// its wire cases, in the order of wireModes and joined by spaces. serverOrder orders the rows of
// the table by them.
var (
	serverURLs = map[string]string{
		"pdp":               "PDP URL",
		"api":               "API URL",
		"api facts-via-pdp": "API URL, or PDP URL with `WithFactsViaPDP`",
	}
	serverOrder = []string{"pdp", "api", "api facts-via-pdp"}
)

// clientServices returns the name of the field of Client that holds each service type, such as
// Users for UsersService: the exported fields whose type is a pointer to a named type.
func clientServices() map[string]string {
	services := map[string]string{}
	for field := range reflect.TypeFor[permit.Client]().Fields() {
		if field.IsExported() && field.Type.Kind() == reflect.Pointer &&
			field.Type.Elem().Name() != "" {
			services[field.Type.Elem().Name()] = field.Name
		}
	}
	return services
}

// callName returns method, such as UsersService.Create or Client.Check, as an application calls
// it on a client named client, such as client.Users.Create or client.Check, with services, the
// fields of Client by the service types they hold, and false when no field holds the type.
func callName(method string, services map[string]string) (string, bool) {
	typ, name, _ := strings.Cut(method, ".")
	if typ == "Client" {
		return "client." + name, true
	}
	field, ok := services[typ]
	return "client." + field + "." + name, ok
}

// methodModes returns the modes of the wire cases of each method, as a key of serverURLs.
func methodModes(cases []wireCase) map[string]string {
	modes := map[string][]wireMode{}
	for _, c := range cases {
		for _, mode := range c.Modes {
			if !slices.Contains(modes[c.Method], mode) {
				modes[c.Method] = append(modes[c.Method], mode)
			}
		}
	}
	keys := map[string]string{}
	for method, list := range modes {
		var names []string
		for _, mode := range wireModes {
			if slices.Contains(list, mode) {
				names = append(names, string(mode))
			}
		}
		keys[method] = strings.Join(names, " ")
	}
	return keys
}

// readmeTables returns the tables by name, as cases, the wire cases, and the operations of
// registry give them, and a problem for each method that a table cannot name: one of a type that
// no field of Client holds, by services, the fields of Client by the service types they hold;
// one whose cases list modes that serverURLs does not name; and one that calls an operation that
// registry does not hold.
//
// The servers table has a row for each method of Client, and one for each service whose methods
// all use one URL, such as client.Users.*, or else one for each of its methods; the rows are in
// the order of serverOrder, then of their calls. The container-only table has a row for each
// method that calls an operation that is ContainerOnly, and the waits-for-sync table one for each
// method that calls one that is WaitsForSync, with its route as the PDP container serves it; their
// rows are in the order of their calls.
func readmeTables(cases []wireCase, registry *rest.Registry, services map[string]string,
) (map[string]readmeTable, []string) {
	var problems []string
	methods := buildOperationMap(cases).Methods
	modes := methodModes(cases)
	calls := map[string]string{}
	byType := map[string][]string{}
	for _, method := range slices.Sorted(maps.Keys(methods)) {
		call, ok := callName(method, services)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: no field of Client holds its type, so "+
				"the tables cannot name the call", method))
			continue
		}
		if _, ok := serverURLs[modes[method]]; !ok {
			problems = append(problems, fmt.Sprintf("%s: its wire cases list the modes %q, which "+
				"the servers table does not name; add them to serverURLs", method, modes[method]))
			continue
		}
		calls[method] = call
		typ, _, _ := strings.Cut(method, ".")
		byType[typ] = append(byType[typ], method)
	}

	type serverRow struct {
		rank int
		call string
		row  tableRow
	}
	var servers []serverRow
	addServer := func(call, mode string) {
		servers = append(servers, serverRow{
			rank: slices.Index(serverOrder, mode),
			call: call,
			row:  newRow("`"+call+"`", serverURLs[mode]),
		})
	}
	for typ, list := range byType {
		shared := modes[list[0]]
		uniform := !slices.ContainsFunc(list, func(m string) bool { return modes[m] != shared })
		if typ != "Client" && uniform {
			addServer("client."+services[typ]+".*", shared)
			continue
		}
		for _, method := range list {
			addServer(calls[method], modes[method])
		}
	}
	slices.SortFunc(servers, func(a, b serverRow) int {
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		return strings.Compare(a.call, b.call)
	})

	containerOnly := readmeTable{header: []string{"| Call | Route |", "| --- | --- |"}}
	waits := readmeTable{header: []string{"| Call | Route |", "| --- | --- |"}}
	for _, method := range slices.SortedFunc(maps.Keys(calls), func(a, b string) int {
		return strings.Compare(calls[a], calls[b])
	}) {
		var containerRoutes, waitRoutes []string
		for _, id := range methods[method] {
			op, ok := registry.Op(id)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s calls the operation %s, which the "+
					"operation registry does not hold", method, id))
				continue
			}
			if op.ContainerOnly {
				containerRoutes = append(containerRoutes, "`"+op.Method+" "+op.Route+"`")
			}
			if op.WaitsForSync {
				waitRoutes = append(waitRoutes, "`"+op.Method+" "+op.ViaPDP().Route+"`")
			}
		}
		call := "`" + calls[method] + "`"
		if containerRoutes != nil {
			containerOnly.rows = append(containerOnly.rows,
				newRow(call, strings.Join(containerRoutes, ", ")))
		}
		if waitRoutes != nil {
			waits.rows = append(waits.rows, newRow(call, strings.Join(waitRoutes, ", ")))
		}
	}

	byServer := readmeTable{header: []string{"| Call | URL |", "| --- | --- |"}}
	for _, s := range servers {
		byServer.rows = append(byServer.rows, s.row)
	}
	return map[string]readmeTable{
		serversTable:       byServer,
		containerOnlyTable: containerOnly,
		waitsTable:         waits,
	}, problems
}

// tableRegion is where a table is in the lines of a file: the indexes of its start line and of
// its end line.
type tableRegion struct {
	start, end int
}

// tableRegions returns where each table of lines, the lines of file, is, by name, and a problem
// for each start line that no end line follows before the next start line, each end line without
// a start line, and each name that two tables have.
func tableRegions(file string, lines []string) (map[string]tableRegion, []string) {
	regions := map[string]tableRegion{}
	var problems []string
	name, start := "", -1
	for i, line := range lines {
		if line == tableEnd {
			if start < 0 {
				problems = append(problems, fmt.Sprintf("%s:%d: %s ends no table", file, i+1,
					tableEnd))
				continue
			}
			if _, ok := regions[name]; ok {
				problems = append(problems, fmt.Sprintf("%s:%d: the table %s is in the file "+
					"twice", file, start+1, name))
			} else {
				regions[name] = tableRegion{start: start, end: i}
			}
			start = -1
			continue
		}
		rest, isStart := strings.CutPrefix(line, tableStart)
		inner, hasEnd := strings.CutSuffix(rest, tableStartEnd)
		if isStart && hasEnd {
			if start >= 0 {
				problems = append(problems, fmt.Sprintf("%s:%d: the table %s has no %s before "+
					"the next table", file, start+1, name, tableEnd))
			}
			name, start = inner, i
		}
	}
	if start >= 0 {
		problems = append(problems, fmt.Sprintf("%s:%d: the table %s has no %s", file, start+1,
			name, tableEnd))
	}
	return regions, problems
}

// tableProblems returns a problem for each way in which the table name of lines, the lines of
// file, in region, differs from want: a row that it lacks, a row of a key that want has no row
// for, and a row whose line is not want's; and, when they have the same rows, any other
// difference, such as in the header, the order of the rows or the lines around them. A row is a
// line that starts with | after as many such lines as want's header has, and its key is the text
// of its first cell.
func tableProblems(file string, lines []string, region tableRegion, name string,
	want readmeTable,
) []string {
	if slices.Equal(lines[region.start+1:region.end], want.lines()) {
		return nil
	}
	var problems []string
	wantLines := map[string]string{}
	for _, row := range want.rows {
		wantLines[row.key] = row.line
	}
	haveKeys := map[string]bool{}
	header := 0
	for i := region.start + 1; i < region.end; i++ {
		line := lines[i]
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if header < len(want.header) {
			header++
			continue
		}
		cell, _, _ := strings.Cut(strings.TrimPrefix(line, "|"), "|")
		key := strings.TrimSpace(cell)
		haveKeys[key] = true
		switch wantLine, ok := wantLines[key]; {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s:%d: the table %s has the row of %s, "+
				"which the wire cases and the operation registry do not give", file, i+1, name,
				key))
		case line != wantLine:
			problems = append(problems, fmt.Sprintf("%s:%d: the row of %s in the table %s is "+
				"stale:\nhave %s\nwant %s", file, i+1, key, name, line, wantLine))
		}
	}
	for _, row := range want.rows {
		if !haveKeys[row.key] {
			problems = append(problems, fmt.Sprintf("%s:%d: the table %s lacks the row of %s: %s",
				file, region.end+1, name, row.key, row.line))
		}
	}
	if problems == nil {
		problems = append(problems, fmt.Sprintf("%s:%d: the table %s has the rows that the "+
			"wire cases and the operation registry give, with another header, in another order "+
			"or with other lines around them", file, region.start+1, name))
	}
	return problems
}

// checkTables returns a problem for each table of tables that lines, the lines of file, does not
// hold as tableProblems describes, or holds no region for, and for each problem of the regions of
// lines, as tableRegions finds them, such as a table that tables does not have.
func checkTables(file string, lines []string, tables map[string]readmeTable) []string {
	regions, problems := tableRegions(file, lines)
	for _, name := range slices.Sorted(maps.Keys(regions)) {
		if _, ok := tables[name]; !ok {
			problems = append(problems, fmt.Sprintf("%s:%d: no test writes a table %s; name one "+
				"of %s", file, regions[name].start+1, name,
				strings.Join(slices.Sorted(maps.Keys(tables)), ", ")))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(tables)) {
		region, ok := regions[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s has no table %s; add the lines %s%s%s "+
				"and %s where it belongs", file, name, tableStart, name, tableStartEnd, tableEnd))
			continue
		}
		problems = append(problems, tableProblems(file, lines, region, name, tables[name])...)
	}
	return problems
}

// rewriteTables returns lines, the lines of file, with each table of tables that they hold
// replaced by its lines, or an error when tableRegions finds a problem.
func rewriteTables(file string, lines []string, tables map[string]readmeTable,
) ([]string, error) {
	regions, problems := tableRegions(file, lines)
	if problems != nil {
		return nil, fmt.Errorf("the tables cannot be rewritten:\n%s",
			strings.Join(problems, "\n"))
	}
	byStart := slices.SortedFunc(maps.Keys(regions), func(a, b string) int {
		return regions[b].start - regions[a].start
	})
	for _, name := range byStart {
		table, ok := tables[name]
		if !ok {
			continue
		}
		r := regions[name]
		lines = slices.Concat(lines[:r.start+1], table.lines(), lines[r.end:])
	}
	return lines, nil
}

// TestReadmeTables checks the tables of README.md against the wire cases and the operation
// registry, as checkTables describes, and rewrites them with -update.
func TestReadmeTables(t *testing.T) {
	tables, problems := readmeTables(wireCases(t), rest.Default(), clientServices())
	for _, problem := range problems {
		t.Error(problem)
	}
	data, err := os.ReadFile(readmeFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	if *update {
		if lines, err = rewriteTables(readmeFile, lines, tables); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(readmeFile, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote the tables of %s", readmeFile)
	}
	if problems := checkTables(readmeFile, lines, tables); problems != nil {
		for _, problem := range problems {
			t.Error(problem)
		}
		t.Errorf("When the change is intended, rewrite the tables with go test -run "+
			"TestReadmeTables -update . and review the diff of %s.", readmeFile)
	}
	rows := 0
	for _, name := range slices.Sorted(maps.Keys(tables)) {
		if len(tables[name].rows) == 0 {
			t.Errorf("count sentinel: the wire cases and the operation registry give the table "+
				"%s no rows", name)
		}
		rows += len(tables[name].rows)
	}
	t.Logf("checked %d tables of %s with %d rows", len(tables), readmeFile, rows)
}

func TestReadmeBlocksThatDropErrors(t *testing.T) {
	blocks, markers := readReadmeFixture(t, "drops-errors.md")
	problems, built := compileBlocks(t, blocks)
	if markers != 2 || built != 2 {
		t.Errorf("drops-errors.md has %d markers and %d built blocks, want 2 and 2", markers,
			built)
	}
	const file = "testdata/readme/drops-errors.md"
	want := []string{
		file + ":17:10: the blank identifier drops a value of type error; a go block handles " +
			"every error",
		file + ":18:5: the blank identifier drops a value of type error; a go block handles " +
			"every error",
		file + ":19:2: the blank identifier drops a value of type error; a go block handles " +
			"every error",
		file + ":20:6: the blank identifier drops a value of type error; a go block handles " +
			"every error",
		file + ":22:14: the blank identifier drops a value of type error; a go block handles " +
			"every error",
		file + ":25:2: the range clause leaves out a value of type error; a go block handles " +
			"every error",
		file + ":28:2: the call statement drops a value of type error; a go block handles " +
			"every error",
		file + ":29:2: the call statement drops a value of type error; a go block handles " +
			"every error",
	}
	if diff := cmp.Diff(want, problems); diff != "" {
		t.Errorf("the problems of drops-errors.md differ (-want +got):\n%s", diff)
	}
}

// TestReadmeQuotesTheScopeError checks that README.md quotes, on a line of its own, the error of
// a call whose scope lacks the environment that it acts on as the SDK returns it: that of
// client.Users.List with a project-level key, whose scope the client looks up from a local API.
func TestReadmeQuotesTheScopeError(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle("GET /v2/api-key/scope",
		mockpermit.Reply{Body: replyFixture("get_api_key_scope.200.project.json")})
	client := wireTestClient(t, api, mockpermit.NewPDP(t))
	_, callErr := client.Users.List(t.Context(), nil)
	if !errors.Is(callErr, permit.ErrInvalidConfig) {
		t.Fatalf("client.Users.List with a project-level key returned %v, want an error that "+
			"matches ErrInvalidConfig", callErr)
	}
	data, err := os.ReadFile(readmeFile)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(strings.Split(string(data), "\n"), callErr.Error()) {
		t.Errorf("%s does not quote the error of client.Users.List with a project-level key on "+
			"a line of its own:\n%s", readmeFile, callErr)
	}
}

// ciFile is the workflow whose env pins the Go versions that CI tests.
const ciFile = ".github/workflows/ci.yml"

// ciGoPin matches a Go version that ciFile pins in its env, and captures its name and the
// version: GO_FLOOR, the go line of go.mod, or GO_1NN, such as GO_127, the newest patch of a Go
// release that the test legs test.
var ciGoPin = regexp.MustCompile(`(?m)^ +(GO_FLOOR|GO_1[0-9]+): "(1\.[0-9]+\.[0-9]+)"$`)

// readmeGoVersion matches a Go version that README.md names, such as Go 1.26, go 1.26.0 or
// go1.27.2, and captures the version.
var readmeGoVersion = regexp.MustCompile(`\b[Gg]o ?(1\.[0-9]+(?:\.[0-9]+)?)\b`)

// goRelease returns the Go release of v, a Go version such as 1.26.0: 1.26.
func goRelease(v string) string {
	return strings.TrimPrefix(version.Lang("go"+v), "go")
}

// goVersionProblems returns a problem for each Go version that readme, the text of README.md,
// gets wrong by goMod, the text of go.mod, and ci, the text of ciFile, and for each that it
// lacks: the quick start says the release of the go line of go.mod, and the support policy says
// the go line, GO_FLOOR, and the two Go releases whose newest patches CI tests, by their GO_1NN.
// README.md names no other Go version. The problems are of goMod or ci when it lacks a version.
func goVersionProblems(readme, goMod, ci string) []string {
	goLine := ""
	for line := range strings.Lines(goMod) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "go "); ok {
			goLine = v
		}
	}
	if goLine == "" {
		return []string{"go.mod has no go line"}
	}
	floor := ""
	var releases []string
	for _, pin := range ciGoPin.FindAllStringSubmatch(ci, -1) {
		if pin[1] == "GO_FLOOR" {
			floor = pin[2]
		} else {
			releases = append(releases, goRelease(pin[2]))
		}
	}
	if floor == "" || len(releases) != 2 {
		return []string{fmt.Sprintf("%s pins GO_FLOOR %q and, by GO_1NN, the Go releases %q; "+
			"want a GO_FLOOR and the two newest Go releases, which %s names", ciFile, floor,
			releases, readmeFile)}
	}
	slices.SortFunc(releases, func(a, b string) int { return version.Compare("go"+a, "go"+b) })
	newest := "Go " + releases[0] + " and Go " + releases[1]
	text := strings.Join(strings.Fields(readme), " ")
	var problems []string
	for _, phrase := range []string{
		"You need Go " + goRelease(goLine) + " or later",
		"the two newest major releases of Go, now " + newest + ".",
		"`go.mod`, `go " + goLine + "`, is the oldest Go",
		"CI tests Go " + floor + " and the newest patches of " + newest + ".",
	} {
		if !strings.Contains(text, phrase) {
			problems = append(problems, fmt.Sprintf("%s does not say %q, as go.mod and %s give "+
				"it", readmeFile, phrase, ciFile))
		}
	}
	known := []string{goLine, goRelease(goLine), floor, releases[0], releases[1]}
	for _, named := range readmeGoVersion.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(known, named[1]) {
			problems = append(problems, fmt.Sprintf("%s names Go %s, which is neither the go "+
				"line of go.mod or its release, nor GO_FLOOR or a Go release that %s tests",
				readmeFile, named[1], ciFile))
		}
	}
	return problems
}

// TestReadmeGoVersions checks the Go versions that README.md names against go.mod and ciFile, as
// goVersionProblems describes.
func TestReadmeGoVersions(t *testing.T) {
	var texts []string
	for _, name := range []string{readmeFile, "go.mod", ciFile} {
		data, err := os.ReadFile(filepath.FromSlash(name))
		if err != nil {
			t.Fatal(err)
		}
		texts = append(texts, string(data))
	}
	for _, problem := range goVersionProblems(texts[0], texts[1], texts[2]) {
		t.Error(problem)
	}
}

func TestReadmeGoVersionProblems(t *testing.T) {
	const (
		readme = "You need Go 1.26 or later.\n\n" +
			"- v2 supports the two newest major releases of Go, now Go 1.26 and Go 1.27. The\n" +
			"  `go` line of `go.mod`, `go 1.26.0`, is the oldest Go that it builds with, and CI\n" +
			"  tests Go 1.26.0 and the newest patches of Go 1.26 and Go 1.27.\n"
		goMod = "module example.com/m\n\ngo 1.26.0\n\ntoolchain go1.27.2\n"
		ci    = "env:\n  GO_FLOOR: \"1.26.0\"\n  GO_127: \"1.27.2\"\n  GO_126: \"1.26.9\"\n"
	)
	if problems := goVersionProblems(readme, goMod, ci); problems != nil {
		t.Errorf("goVersionProblems of fresh files = %q, want none", problems)
	}
	// missing is the problem of a phrase that README.md lacks.
	missing := func(phrase string) string {
		return fmt.Sprintf("README.md does not say %q, as go.mod and %s give it", phrase, ciFile)
	}
	for _, tc := range []struct {
		name, readme, goMod, ci string
		want                    []string
	}{
		{
			name:   "a release that README.md gets wrong",
			readme: strings.Replace(readme, "Go 1.26 or", "Go 1.25 or", 1),
			goMod:  goMod, ci: ci,
			want: []string{
				missing("You need Go 1.26 or later"),
				"README.md names Go 1.25, which is neither the go line of go.mod or its release, " +
					"nor GO_FLOOR or a Go release that " + ciFile + " tests",
			},
		},
		{
			name:   "a newer go line",
			readme: readme, ci: ci,
			goMod: strings.Replace(goMod, "go 1.26.0", "go 1.27.0", 1),
			want: []string{
				missing("You need Go 1.27 or later"),
				missing("`go.mod`, `go 1.27.0`, is the oldest Go"),
			},
		},
		{
			name:   "newer Go releases in CI",
			readme: readme, goMod: goMod,
			ci: "env:\n  GO_FLOOR: \"1.26.0\"\n  GO_127: \"1.27.2\"\n  GO_128: \"1.28.0\"\n",
			want: []string{
				missing("the two newest major releases of Go, now Go 1.27 and Go 1.28."),
				missing("CI tests Go 1.26.0 and the newest patches of Go 1.27 and Go 1.28."),
			},
		},
		{
			name:   "one Go release in CI",
			readme: readme, goMod: goMod,
			ci: "env:\n  GO_FLOOR: \"1.26.0\"\n  GO_127: \"1.27.2\"\n",
			want: []string{ciFile + ` pins GO_FLOOR "1.26.0" and, by GO_1NN, the Go releases ` +
				`["1.27"]; want a GO_FLOOR and the two newest Go releases, which README.md names`},
		},
		{
			name:   "no go line",
			readme: readme, ci: ci,
			goMod: "module example.com/m\n\ntoolchain go1.27.2\n",
			want:  []string{"go.mod has no go line"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problems := goVersionProblems(tc.readme, tc.goMod, tc.ci)
			if diff := cmp.Diff(tc.want, problems); diff != "" {
				t.Errorf("goVersionProblems differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReadmeYieldTypes(t *testing.T) {
	stringType := types.Typ[types.String]
	seq := func(params ...types.Type) types.Type {
		vars := make([]*types.Var, len(params))
		for i, p := range params {
			vars[i] = types.NewParam(token.NoPos, nil, "", p)
		}
		yield := types.NewSignatureType(nil, nil, nil, types.NewTuple(vars...),
			types.NewTuple(types.NewParam(token.NoPos, nil, "", types.Typ[types.Bool])), false)
		return types.NewSignatureType(nil, nil, nil,
			types.NewTuple(types.NewParam(token.NoPos, nil, "yield", yield)), nil, false)
	}
	for _, tc := range []struct {
		name       string
		typ        types.Type
		key, value types.Type
	}{
		{name: "a Seq2", typ: seq(stringType, errorType), key: stringType, value: errorType},
		{name: "a Seq", typ: seq(errorType), key: errorType},
		{name: "a yield of three", typ: seq(stringType, stringType, errorType)},
		{name: "a slice", typ: types.NewSlice(errorType)},
		{name: "no type"},
		{
			name: "a function that takes no yield",
			typ: types.NewSignatureType(nil, nil, nil,
				types.NewTuple(types.NewParam(token.NoPos, nil, "", stringType)), nil, false),
		},
	} {
		key, value := yieldTypes(tc.typ)
		if key != tc.key || value != tc.value {
			t.Errorf("%s: yieldTypes = %v, %v; want %v, %v", tc.name, key, value, tc.key,
				tc.value)
		}
	}
}

// tablesCases are the wire cases of TestReadmeTablesFromCases: the methods of Client, a service
// whose methods use one URL and one whose methods use two. The second case of Things.Create adds
// no mode to the first.
var tablesCases = []wireCase{
	{Method: "Client.Check", Op: "t_check", Modes: []wireMode{pdpMode}},
	{Method: "Client.Local", Op: "t_local", Modes: []wireMode{pdpMode}},
	{Method: "Client.Scope", Op: "t_scope", Modes: []wireMode{apiMode}},
	{Method: "ThingsService.Get", Op: "t_get_thing", Modes: []wireMode{apiMode, factsViaPDPMode}},
	{
		Method: "ThingsService.Create", Op: "t_create_thing",
		Modes: []wireMode{factsViaPDPMode, apiMode},
	},
	{Method: "ThingsService.Create", Op: "t_create_thing", Modes: []wireMode{apiMode}},
	{Method: "MixedService.Read", Op: "t_read", Modes: []wireMode{apiMode}},
	{Method: "MixedService.Decide", Op: "t_check", Modes: []wireMode{pdpMode}},
}

// tablesServices are the fields of Client by the service types of tablesCases.
var tablesServices = map[string]string{"ThingsService": "Things", "MixedService": "Mixed"}

// tablesRegistry returns a registry with the operations of tablesCases.
func tablesRegistry(t *testing.T) *rest.Registry {
	t.Helper()
	const facts = rest.TestPrefix + "v2/facts/{proj_id}/{env_id}/things"
	registry, err := rest.Default().WithTestOps(
		rest.Op{
			ID: "t_check", Method: "POST", Route: rest.TestPrefix + "check", Idempotent: true,
			KeyLevel: rest.EnvironmentLevel,
		},
		rest.Op{
			ID: "t_local", Method: "GET", Route: rest.TestPrefix + "local", Idempotent: true,
			ContainerOnly: true, KeyLevel: rest.EnvironmentLevel,
		},
		rest.Op{
			ID: "t_scope", Method: "GET", Route: rest.TestPrefix + "scope", Idempotent: true,
			KeyLevel: rest.EnvironmentLevel,
		},
		rest.Op{
			ID: "t_get_thing", Method: "GET", Route: facts + "/{thing_id}", Idempotent: true,
			Facts: true, KeyLevel: rest.EnvironmentLevel,
		},
		rest.Op{
			ID: "t_create_thing", Method: "POST", Route: facts, Facts: true, WaitsForSync: true,
			KeyLevel: rest.EnvironmentLevel,
		},
		rest.Op{
			ID: "t_read", Method: "GET", Route: rest.TestPrefix + "read", Idempotent: true,
			KeyLevel: rest.ProjectLevel,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestReadmeTablesFromCases(t *testing.T) {
	tables, problems := readmeTables(tablesCases, tablesRegistry(t), tablesServices)
	if problems != nil {
		t.Errorf("readmeTables found the problems %q, want none", problems)
	}
	want := map[string][]string{
		serversTable: {
			"",
			"| Call | URL |",
			"| --- | --- |",
			"| `client.Check` | PDP URL |",
			"| `client.Local` | PDP URL |",
			"| `client.Mixed.Decide` | PDP URL |",
			"| `client.Mixed.Read` | API URL |",
			"| `client.Scope` | API URL |",
			"| `client.Things.*` | API URL, or PDP URL with `WithFactsViaPDP` |",
			"",
		},
		containerOnlyTable: {
			"",
			"| Call | Route |",
			"| --- | --- |",
			"| `client.Local` | `GET /__test/local` |",
			"",
		},
		waitsTable: {
			"",
			"| Call | Route |",
			"| --- | --- |",
			"| `client.Things.Create` | `POST /__test/facts/things` |",
			"",
		},
	}
	got := map[string][]string{}
	for name, table := range tables {
		got[name] = table.lines()
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("readmeTables differs (-want +got):\n%s", diff)
	}
}

func TestReadmeTablesThatCannotNameACall(t *testing.T) {
	services := map[string]string{"ThingsService": "Things", "OddService": "Odd"}
	cases := []wireCase{
		{Method: "ThingsService.Get", Op: "t_get_thing", Modes: []wireMode{apiMode}},
		{Method: "OtherService.Get", Op: "t_get_thing", Modes: []wireMode{apiMode}},
		{Method: "OddService.Do", Op: "t_check", Modes: []wireMode{pdpMode, factsViaPDPMode}},
		{Method: "Client.Missing", Op: "t_missing", Modes: []wireMode{apiMode}},
	}
	tables, problems := readmeTables(cases, tablesRegistry(t), services)
	want := []string{
		"OddService.Do: its wire cases list the modes \"pdp facts-via-pdp\", which the servers " +
			"table does not name; add them to serverURLs",
		"OtherService.Get: no field of Client holds its type, so the tables cannot name the call",
		"Client.Missing calls the operation t_missing, which the operation registry does not hold",
	}
	if diff := cmp.Diff(want, problems); diff != "" {
		t.Errorf("readmeTables found other problems (-want +got):\n%s", diff)
	}
	var keys []string
	for _, row := range tables[serversTable].rows {
		keys = append(keys, row.key)
	}
	if want := []string{"`client.Missing`", "`client.Things.*`"}; !slices.Equal(keys, want) {
		t.Errorf("the servers table has the rows %q, want %q", keys, want)
	}
}

func TestReadmeCallNames(t *testing.T) {
	services := clientServices()
	if services["UsersService"] != "Users" || services["APIKeysService"] != "APIKeys" {
		t.Errorf("clientServices maps UsersService to %q and APIKeysService to %q, want Users "+
			"and APIKeys", services["UsersService"], services["APIKeysService"])
	}
	for method, want := range map[string]string{
		"Client.Check":        "client.Check",
		"UsersService.Create": "client.Users.Create",
	} {
		if got, ok := callName(method, services); got != want || !ok {
			t.Errorf("callName(%q) = %q, %v; want %q, true", method, got, ok, want)
		}
	}
	if _, ok := callName("OtherService.Get", services); ok {
		t.Error("callName names a method of a type that no field of Client holds")
	}
}

// tablesFile returns the lines of a file that holds the table servers with rows, and the table
// waits-for-sync as want gives it.
func tablesFile(want map[string]readmeTable, rows ...string) []string {
	servers := readmeTable{header: want[serversTable].header}
	for _, row := range rows {
		servers.rows = append(servers.rows, tableRow{line: row})
	}
	return slices.Concat(
		[]string{"# SDK", "", tableStart + serversTable + tableStartEnd},
		servers.lines(),
		[]string{tableEnd, "", "Text.", tableStart + waitsTable + tableStartEnd},
		want[waitsTable].lines(),
		[]string{tableEnd, ""},
	)
}

// tablesWant returns the tables of the file of TestReadmeTableProblems: servers with two rows and
// waits-for-sync with one.
func tablesWant() map[string]readmeTable {
	header := []string{"| Call | URL |", "| --- | --- |"}
	return map[string]readmeTable{
		serversTable: {header: header, rows: []tableRow{
			newRow("`client.Check`", "PDP URL"),
			newRow("`client.Users.*`", "API URL"),
		}},
		waitsTable: {header: header, rows: []tableRow{newRow("`client.Users.Create`", "x")}},
	}
}

func TestReadmeTableProblems(t *testing.T) {
	want := tablesWant()
	const check, users = "| `client.Check` | PDP URL |", "| `client.Users.*` | API URL |"
	if problems := checkTables("README.md", tablesFile(want, check, users), want); problems != nil {
		t.Errorf("checkTables of fresh tables = %q, want none", problems)
	}
	for _, tc := range []struct {
		name  string
		lines []string
		want  []string
	}{
		{
			name:  "a missing row",
			lines: tablesFile(want, check),
			want: []string{"README.md:9: the table servers lacks the row of `client.Users.*`: " +
				"| `client.Users.*` | API URL |"},
		},
		{
			name:  "an extra row",
			lines: tablesFile(want, check, users, "| `client.Old.*` | API URL |"),
			want: []string{"README.md:9: the table servers has the row of `client.Old.*`, which " +
				"the wire cases and the operation registry do not give"},
		},
		{
			name:  "a stale row",
			lines: tablesFile(want, check, "| `client.Users.*` | PDP URL |"),
			want: []string{"README.md:8: the row of `client.Users.*` in the table servers is " +
				"stale:\nhave | `client.Users.*` | PDP URL |\nwant | `client.Users.*` | API URL |"},
		},
		{
			name:  "rows in another order",
			lines: tablesFile(want, users, check),
			want: []string{"README.md:3: the table servers has the rows that the wire cases and " +
				"the operation registry give, with another header, in another order or with " +
				"other lines around them"},
		},
		{
			name:  "a table that no test writes",
			lines: append(tablesFile(want, check, users), "<!-- table old -->", tableEnd),
			want: []string{"README.md:21: no test writes a table old; name one of servers, " +
				"waits-for-sync"},
		},
		{
			name:  "a missing table",
			lines: tablesFile(want, check, users)[:10],
			want: []string{"README.md has no table waits-for-sync; add the lines <!-- table " +
				"waits-for-sync --> and <!-- end table --> where it belongs"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, checkTables("README.md", tc.lines, want)); diff != "" {
				t.Errorf("checkTables differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReadmeTableRegions(t *testing.T) {
	start := func(name string) string { return tableStart + name + tableStartEnd }
	lines := []string{
		tableEnd, start("a"), "x", tableEnd, start("b"), start("c"), tableEnd, start("a"), "y",
		tableEnd, start("d"), "<!-- table e", "<!--table f -->",
	}
	regions, problems := tableRegions("README.md", lines)
	wantRegions := map[string]tableRegion{"a": {start: 1, end: 3}, "c": {start: 5, end: 6}}
	if diff := cmp.Diff(wantRegions, regions, cmp.AllowUnexported(tableRegion{})); diff != "" {
		t.Errorf("tableRegions differs (-want +got):\n%s", diff)
	}
	wantProblems := []string{
		"README.md:1: <!-- end table --> ends no table",
		"README.md:5: the table b has no <!-- end table --> before the next table",
		"README.md:8: the table a is in the file twice",
		"README.md:11: the table d has no <!-- end table -->",
	}
	if diff := cmp.Diff(wantProblems, problems); diff != "" {
		t.Errorf("tableRegions found other problems (-want +got):\n%s", diff)
	}
}

func TestReadmeRewriteTables(t *testing.T) {
	want := tablesWant()
	const check, users = "| `client.Check` | PDP URL |", "| `client.Users.*` | API URL |"
	fresh := tablesFile(want, check, users)
	stale := slices.Concat(tablesFile(want, "| `client.Old.*` | API URL |"),
		[]string{"<!-- table old -->", "kept", tableEnd})
	got, err := rewriteTables("README.md", stale, want)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(slices.Concat(fresh, []string{"<!-- table old -->", "kept", tableEnd}),
		got); diff != "" {
		t.Errorf("rewriteTables differs (-want +got):\n%s", diff)
	}
	if _, err := rewriteTables("README.md", fresh[:15], want); err == nil ||
		!strings.Contains(err.Error(), "the table waits-for-sync has no <!-- end table -->") {
		t.Errorf("rewriteTables of a table without its end line returned %v, want an error "+
			"that names it", err)
	}
}
