package permit_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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
// is not a program or does not compile, and the number of blocks it built: those it ran go build
// on, whether they compile or not. A //line directive makes the compiler name the lines of the
// Markdown file.
func compileBlocks(t *testing.T, blocks []goBlock) (problems []string, built int) {
	t.Helper()
	goMod, goSum := readmeModule(t)
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
		for name, data := range map[string][]byte{
			"go.mod":  goMod,
			"go.sum":  goSum,
			"main.go": fmt.Appendf(nil, "//line %s:%d:1\n%s", b.file, b.line+1, b.code),
		} {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.CommandContext(t.Context(), "go", "build", "-o", os.DevNull, ".")
		cmd.Dir = dir
		cmd.Env = slices.Concat(os.Environ(),
			[]string{"GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off"})
		out, err := cmd.CombinedOutput()
		built++
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s:%d: the go block does not compile: %v\n%s",
				b.file, b.line, err, out))
		}
	}
	return problems, built
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
