package main

import (
	"bytes"
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// goModule is a module the gate mutates packages of, as go list reports it.
type goModule struct {
	path string
	// dir is the module's root directory.
	dir  string
	pkgs []goPackage
}

// goPackage is one package of a module.
type goPackage struct {
	dir        string
	importPath string
	// files are the names of the non-test Go files go builds.
	files []string
	// rel is dir relative to the repository root, which the gate sets.
	rel string
}

// moduleScope is a module, the directories in it the gate was given, and what
// changed in it.
type moduleScope struct {
	module goModule
	// rel is the module's root, and dirs the directories given, relative to the
	// repository root.
	rel  string
	dirs []string
	// changedFiles counts the changed Go files of the module, or with -all the Go
	// files of its packages in the directories, mutated or not.
	changedFiles int
	notMutated   []notMutated
}

// notMutated is a changed Go file the gate does not mutate, and why.
type notMutated struct{ file, reason string }

// unit is one package to mutate. gremlins runs once per unit, in the package's
// directory with its subdirectories left out, and the go shim tests the package,
// so a mutant is always tested with its own package's tests.
type unit struct {
	module int
	pkg    goPackage
	// lines are the lines to mutate, by file name in the package's directory.
	lines map[string][]lineRange
	// exclude are gremlins --exclude-files patterns: every file in a subdirectory,
	// and the Go files in the directory that go does not build here.
	exclude []string
}

// plan is what a run mutates.
type plan struct {
	root string
	// realRoot is root with symbolic links resolved.
	realRoot string
	modules  []moduleScope
	units    []unit
}

// planRun loads the modules of the directories given and works out the units: the
// packages with changed lines, or with -all every package in the directories.
func planRun(t tools, opts gateOptions, root string) (plan, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return plan{}, err
	}
	p := plan{root: root, realRoot: realRoot}
	if err = p.loadModules(t, opts); err != nil {
		return p, err
	}
	scoped := map[string][]lineRange{}
	if opts.all {
		for i := range p.modules {
			if p.modules[i].changedFiles, err = p.allLines(i, scoped); err != nil {
				return p, err
			}
		}
	} else {
		diff, err := t.diff(root, opts.base)
		if err != nil {
			return p, err
		}
		changed, err := parseAddedLines(diff)
		if err != nil {
			return p, err
		}
		if err := p.classify(changed, scoped); err != nil {
			return p, err
		}
	}
	return p, p.makeUnits(scoped)
}

// loadModules finds the module of each directory given and lists its packages.
func (p *plan) loadModules(t tools, opts gateOptions) error {
	for _, given := range opts.dirs {
		abs, err := filepath.Abs(given)
		if err != nil {
			return err
		}
		if info, statErr := os.Stat(abs); statErr != nil || !info.IsDir() {
			return fmt.Errorf("%s is not a directory", given)
		}
		rel, err := p.relative(abs)
		if err != nil {
			return fmt.Errorf("directory %s: %w", given, err)
		}
		modRel, ok := p.moduleOfDir(rel)
		if !ok {
			return fmt.Errorf("directory %s is in no Go module of the repository", given)
		}
		i := slices.IndexFunc(p.modules, func(m moduleScope) bool { return m.rel == modRel })
		if i < 0 {
			m, err := p.loadModule(t, modRel, opts.tags)
			if err != nil {
				return err
			}
			p.modules = append(p.modules, moduleScope{module: m, rel: modRel})
			i = len(p.modules) - 1
		}
		if !slices.Contains(p.modules[i].dirs, rel) {
			p.modules[i].dirs = append(p.modules[i].dirs, rel)
		}
	}
	return nil
}

// loadModule lists the module whose root is modRel, relative to the repository
// root, and its packages.
func (p *plan) loadModule(t tools, modRel, tags string) (goModule, error) {
	modDir := filepath.Join(p.root, filepath.FromSlash(modRel))
	m, err := t.module(modDir, tags)
	if err != nil {
		return m, err
	}
	if !sameDir(m.dir, modDir) {
		return m, fmt.Errorf("go list in %s reports the module root %s", modDir, m.dir)
	}
	for i := range m.pkgs {
		if m.pkgs[i].rel, err = p.relative(m.pkgs[i].dir); err != nil {
			return m, fmt.Errorf("package %s: %w", m.pkgs[i].importPath, err)
		}
	}
	return m, nil
}

// relative returns abs relative to the repository root, with slashes.
func (p *plan) relative(abs string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	rel, err := filepath.Rel(p.realRoot, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%s is outside the repository %s", abs, p.root)
	}
	return filepath.ToSlash(rel), nil
}

// moduleOfDir returns the root, relative to the repository root, of the module a
// directory is in: the nearest directory at or above it with a go.mod, leaving out
// test data.
func (p *plan) moduleOfDir(dir string) (string, bool) {
	for ; ; dir = path.Dir(dir) {
		if !inTestdata(dir) {
			goMod := filepath.Join(p.root, filepath.FromSlash(dir), "go.mod")
			if info, err := os.Stat(goMod); err == nil && info.Mode().IsRegular() {
				return dir, true
			}
		}
		if dir == "." {
			return "", false
		}
	}
}

func inTestdata(file string) bool {
	return slices.Contains(strings.Split(file, "/"), "testdata")
}

// under reports whether rel, relative to the repository root, is dir or below it.
func under(rel, dir string) bool {
	return dir == "." || rel == dir || strings.HasPrefix(rel, dir+"/")
}

// inDirs reports whether rel, relative to the repository root, is in one of the
// module's directories.
func (m moduleScope) inDirs(rel string) bool {
	return slices.ContainsFunc(m.dirs, func(dir string) bool { return under(rel, dir) })
}

// isBuilt reports whether go builds a file, relative to the repository root, in
// one of the module's packages.
func (m moduleScope) isBuilt(file string) bool {
	dir, name := path.Dir(file), path.Base(file)
	return slices.ContainsFunc(m.module.pkgs, func(pkg goPackage) bool {
		return pkg.rel == dir && slices.Contains(pkg.files, name)
	})
}

// classify sorts the changed Go files: the changed lines that hold code that Go's
// coverage counts go in scoped, the files not mutated in their module's notMutated,
// and a file in a module that no directory given is in, or a file to mutate that
// does not parse, is an error.
func (p *plan) classify(changed, scoped map[string][]lineRange) error {
	var outside []string
	for _, file := range slices.Sorted(maps.Keys(changed)) {
		modRel, ok := p.moduleOfDir(path.Dir(file))
		i := slices.IndexFunc(p.modules, func(m moduleScope) bool { return m.rel == modRel })
		if !ok || i < 0 {
			outside = append(outside, file)
			continue
		}
		m := &p.modules[i]
		m.changedFiles++
		switch {
		case inTestdata(file):
			m.notMutated = append(m.notMutated, notMutated{file, "test data"})
		case !m.inDirs(path.Dir(file)):
			m.notMutated = append(m.notMutated, notMutated{file, "outside the directories given"})
		case !m.isBuilt(file):
			m.notMutated = append(m.notMutated, notMutated{file, "go does not build it here " +
				"(another platform, a build tag or an ignored name)"})
		default:
			data, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(file)))
			if err != nil {
				return err
			}
			inBody, uncounted, outsideBodies, err := codeLines(file, data)
			if err != nil {
				return err
			}
			if lines := keepLines(changed[file], inBody); len(lines) > 0 {
				scoped[file] = lines
			} else if len(keepLines(changed[file], uncounted)) > 0 {
				m.notMutated = append(m.notMutated, notMutated{file, "no code that Go's coverage " +
					"counts on the changed lines (a case clause up to its colon, or the rest of " +
					"a statement after the body of a function literal in it)"})
			} else if len(keepLines(changed[file], outsideBodies)) > 0 {
				m.notMutated = append(m.notMutated, notMutated{file, "no code in a function body " +
					"on the changed lines (declarations outside function bodies, which Go's " +
					"coverage does not count)"})
			} else {
				m.notMutated = append(m.notMutated, notMutated{file, "no code on the changed " +
					"lines (comments, blank lines, the package clause or imports)"})
			}
		}
	}
	if len(outside) > 0 {
		return fmt.Errorf("go lines changed in %s, in no module of the directories given; give "+
			"the directory of its module too", strings.Join(outside, ", "))
	}
	return nil
}

// allLines puts in scoped every line, of every Go file of the packages in module i's
// directories, that holds code that Go's coverage counts, and returns how many files
// there are.
func (p *plan) allLines(i int, scoped map[string][]lineRange) (int, error) {
	m := p.modules[i]
	files := 0
	for _, pkg := range m.module.pkgs {
		if !m.inDirs(pkg.rel) {
			continue
		}
		for _, name := range pkg.files {
			file := path.Join(pkg.rel, name)
			data, err := os.ReadFile(filepath.Join(pkg.dir, name))
			if err != nil {
				return 0, err
			}
			files++
			inBody, _, _, err := codeLines(file, data)
			if err != nil {
				return 0, err
			}
			if lines := keepLines([]lineRange{{1, lineCount(data)}}, inBody); len(lines) > 0 {
				scoped[file] = lines
			}
		}
	}
	return files, nil
}

// codeLines returns the lines of the Go source src that hold code: a token of a
// declaration other than an import, or the inside of a raw string that starts in
// one. inBody are those with such a token that Go's coverage counts: inside a
// function body that coverage counts, after its { and before its }, and in none of
// the spans of it that uncountedSpans returns. Coverage counts nothing else, so
// gremlins reports every other mutant as on a line no test covers, and the gate
// mutates only the lines in inBody. uncounted are the lines with a token in those
// spans, such as the expressions of a case clause, and outside the lines with a
// token outside those bodies, such as a package-level variable's value, a
// function's signature or a body's closing }; a line can be in more than one.
// Comments, blank lines, automatic semicolons, the package clause and imports hold
// no mutant and are in none. name names the file in the error when src does not
// parse.
func codeLines(name string, src []byte) (inBody, uncounted, outside map[int]bool, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, nil, fmt.Errorf(
			"cannot tell the code lines of a Go file that does not parse: %w", err)
	}
	// Imports come before the other declarations, and between two declarations
	// there is only the semicolon that ends the first, on its last line: the lines
	// with code are those with a token from the first other declaration on. Lines
	// are those in src, not the ones a //line directive names.
	i := slices.IndexFunc(f.Decls, func(d ast.Decl) bool {
		g, ok := d.(*ast.GenDecl)
		return !ok || g.Tok != token.IMPORT
	})
	inBody, uncounted, outside = map[int]bool{}, map[int]bool{}, map[int]bool{}
	if i < 0 {
		return inBody, uncounted, outside, nil
	}
	start := fset.PositionFor(f.Decls[i].Pos(), false).Line
	bodies := functionBodies(fset.File(f.Pos()), f)
	spans := uncountedSpans(fset.File(f.Pos()), f)
	file := fset.AddFile(name, -1, len(src))
	var s scanner.Scanner
	s.Init(file, src, nil, 0)
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return inBody, uncounted, outside, nil
		}
		first := file.PositionFor(pos, false).Line
		if first < start {
			continue
		}
		// The scanner puts an automatic semicolon on the line of the token before it,
		// and coverage does not count it as code.
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}
		offset := file.Offset(pos)
		for len(bodies) > 0 && offset >= bodies[0].rbrace {
			bodies = bodies[1:]
		}
		lines := outside
		if len(bodies) > 0 && offset > bodies[0].lbrace {
			lines = inBody
			for len(spans) > 0 && offset >= spans[0].to {
				spans = spans[1:]
			}
			if len(spans) > 0 && offset >= spans[0].from {
				lines = uncounted
			}
		}
		last := first
		if tok == token.STRING {
			last += strings.Count(lit, "\n")
		}
		for line := first; line <= last; line++ {
			lines[line] = true
		}
	}
}

// braces are the offsets in its file of the { and the } of a function body.
type braces struct{ lbrace, rbrace int }

// functionBodies returns the braces of the function bodies of f that Go's coverage
// counts, in the order of the source: those of its functions, and of its function
// literals outside them, such as one in a package-level variable's value; a
// function literal in a function is inside that function's body. Coverage leaves
// out a function named _, which cannot be called, with the function literals in
// it. file is f's file.
func functionBodies(file *token.File, f *ast.File) []braces {
	var bodies []braces
	ast.Inspect(f, func(n ast.Node) bool {
		var body *ast.BlockStmt
		switch n := n.(type) {
		case *ast.FuncDecl:
			if n.Name.Name != "_" {
				body = n.Body
			}
		case *ast.FuncLit:
			body = n.Body
		default:
			return true
		}
		if body != nil {
			bodies = append(bodies, braces{file.Offset(body.Lbrace), file.Offset(body.Rbrace)})
		}
		return false
	})
	return bodies
}

// span is the offsets in its file of the first byte of some code and of the byte
// after its last.
type span struct{ from, to int }

// uncountedSpans returns the spans of code in the function bodies of f that Go's
// coverage counts in no block, in the order of the source:
//   - a switch, type switch or select clause from its case or default to its colon,
//     as coverage starts the clause's block after the colon;
//   - a statement from the { of the first function literal in the parts of it that
//     statementHead returns to the end of the statement, or to the { of its body
//     when it has one, as coverage ends the block that holds the statement at the
//     function literal's {.
//
// The insides of the bodies of the function literals in such a span, between their
// braces, have blocks of their own and are left out of it. file is f's file.
func uncountedSpans(file *token.File, f *ast.File) []span {
	var spans []span
	ast.Inspect(f, func(n ast.Node) bool {
		var stmts []ast.Stmt
		switch n := n.(type) {
		case *ast.BlockStmt:
			stmts = n.List
		case *ast.CaseClause:
			exprs := make([]ast.Node, len(n.List))
			for i, x := range n.List {
				exprs[i] = x
			}
			spans = appendSpans(spans, file, n.Case, n.Colon+1, exprs...)
			stmts = n.Body
		case *ast.CommClause:
			spans = appendSpans(spans, file, n.Case, n.Colon+1, n.Comm)
			stmts = n.Body
		case *ast.IfStmt:
			// Coverage puts an else if in a block of its own.
			if elseIf, ok := n.Else.(*ast.IfStmt); ok {
				stmts = []ast.Stmt{elseIf}
			}
		}
		for _, s := range stmts {
			parts, body := statementHead(s)
			lit := firstFuncLit(parts...)
			if lit == nil {
				continue
			}
			end := s.End()
			if body != nil {
				end = body.Lbrace + 1
			}
			spans = appendSpans(spans, file, lit.Body.Lbrace, end, parts...)
		}
		return true
	})
	slices.SortFunc(spans, func(a, b span) int { return cmp.Compare(a.from, b.from) })
	return spans
}

// statementHead returns the parts of statement s, in the order of the source, in
// which Go's coverage looks for a function literal at whose { to end the block that
// holds s, and the body of s after them, if s has one: all of a simple statement,
// or the init, condition and post statement of a for statement, for example. Of a
// type switch, coverage looks only at the init statement: its parts are nil when
// that holds no function literal, and else the init statement and the x.(type)
// after it. parts is nil for a block or select statement, which coverage does not
// look into, and for a switch or select clause, whose spans uncountedSpans finds
// itself.
func statementHead(s ast.Stmt) (parts []ast.Node, body *ast.BlockStmt) {
	switch s := s.(type) {
	case *ast.LabeledStmt:
		return statementHead(s.Stmt)
	case *ast.BlockStmt, *ast.SelectStmt, *ast.CaseClause, *ast.CommClause:
		return nil, nil
	case *ast.IfStmt:
		return []ast.Node{s.Init, s.Cond}, s.Body
	case *ast.ForStmt:
		return []ast.Node{s.Init, s.Cond, s.Post}, s.Body
	case *ast.RangeStmt:
		return []ast.Node{s.X}, s.Body
	case *ast.SwitchStmt:
		return []ast.Node{s.Init, s.Tag}, s.Body
	case *ast.TypeSwitchStmt:
		if firstFuncLit(s.Init) == nil {
			return nil, nil
		}
		return []ast.Node{s.Init, s.Assign}, s.Body
	}
	return []ast.Node{s}, nil
}

// firstFuncLit returns the first function literal in nodes, in the order of the
// source, or nil when there is none. nodes can hold nil.
func firstFuncLit(nodes ...ast.Node) *ast.FuncLit {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		for m := range ast.Preorder(n) {
			if lit, ok := m.(*ast.FuncLit); ok {
				return lit
			}
		}
	}
	return nil
}

// appendSpans appends to spans the span of the code from start to before end, less
// the insides of the bodies of the function literals in nodes, which coverage
// counts in blocks of their own. nodes are the parts of the span, in the order of
// the source, and can hold nil.
func appendSpans(spans []span, file *token.File, start, end token.Pos, nodes ...ast.Node) []span {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		ast.Inspect(n, func(m ast.Node) bool {
			lit, ok := m.(*ast.FuncLit)
			if ok {
				spans = append(spans, span{file.Offset(start), file.Offset(lit.Body.Lbrace) + 1})
				start = lit.Body.Rbrace
			}
			return !ok
		})
	}
	return append(spans, span{file.Offset(start), file.Offset(end)})
}

// keepLines returns the lines of ranges that are in keep, as ranges.
func keepLines(ranges []lineRange, keep map[int]bool) []lineRange {
	var kept []lineRange
	for _, r := range ranges {
		for line := r.first; line <= r.last; line++ {
			if !keep[line] {
				continue
			}
			if n := len(kept); n > 0 && kept[n-1].last == line-1 {
				kept[n-1].last = line
			} else {
				kept = append(kept, lineRange{line, line})
			}
		}
	}
	return kept
}

func lineCount(data []byte) int {
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// makeUnits makes a unit of each package in the directories given that has lines
// in scoped, in the order of the modules and then of the import paths.
func (p *plan) makeUnits(scoped map[string][]lineRange) error {
	for i, m := range p.modules {
		pkgs := slices.SortedFunc(slices.Values(m.module.pkgs), func(a, b goPackage) int {
			return strings.Compare(a.importPath, b.importPath)
		})
		for _, pkg := range pkgs {
			if !m.inDirs(pkg.rel) {
				continue
			}
			u := unit{module: i, pkg: pkg, lines: map[string][]lineRange{}}
			for _, name := range pkg.files {
				if ranges, ok := scoped[path.Join(pkg.rel, name)]; ok {
					u.lines[name] = ranges
				}
			}
			if len(u.lines) == 0 {
				continue
			}
			var err error
			if u.exclude, err = excludedFiles(pkg); err != nil {
				return err
			}
			p.units = append(p.units, u)
		}
	}
	return nil
}

// excludedFiles returns the gremlins --exclude-files patterns of a package's
// directory: every file in a subdirectory, and each Go file go does not build here,
// whose mutants would otherwise live.
func excludedFiles(pkg goPackage) ([]string, error) {
	entries, err := os.ReadDir(pkg.dir)
	if err != nil {
		return nil, err
	}
	patterns := []string{"/"}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			slices.Contains(pkg.files, name) {
			continue
		}
		patterns = append(patterns, "^"+regexp.QuoteMeta(name)+"$")
	}
	return patterns, nil
}

// scopeRef is the ref the gate passes to gremlins' --diff. gremlins runs
// git diff --merge-base scopeRef, which the git shim answers with scopeDiff.
const scopeRef = "mutationgate-scope"

// scopeDiff returns the git diff that the git shim gives gremlins: one that adds
// exactly lines, by file name, with no context, so gremlins marks those lines as
// changed and no others.
func scopeDiff(lines map[string][]lineRange) []byte {
	var b bytes.Buffer
	for _, name := range slices.Sorted(maps.Keys(lines)) {
		ranges := slices.SortedFunc(slices.Values(lines[name]), func(x, y lineRange) int {
			return x.first - y.first
		})
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n", name, name, name, name)
		added := 0
		for _, r := range ranges {
			n := r.last - r.first + 1
			fmt.Fprintf(&b, "@@ -%d,0 +%d,%d @@\n%s", r.first-1-added, r.first, n,
				strings.Repeat("+\n", n))
			added += n
		}
	}
	return b.Bytes()
}

// isChanged reports whether line of file is in lines.
func isChanged(lines map[string][]lineRange, file string, line int) bool {
	for _, r := range lines[file] {
		if line >= r.first && line <= r.last {
			return true
		}
	}
	return false
}

// withoutLines returns lines less the lines in drop, both by file name.
func withoutLines(lines map[string][]lineRange, drop map[string][]int) map[string][]lineRange {
	kept := map[string][]lineRange{}
	for name, ranges := range lines {
		for _, r := range ranges {
			first := r.first
			for line := r.first; line <= r.last; line++ {
				if !slices.Contains(drop[name], line) {
					continue
				}
				if line > first {
					kept[name] = append(kept[name], lineRange{first, line - 1})
				}
				first = line + 1
			}
			if first <= r.last {
				kept[name] = append(kept[name], lineRange{first, r.last})
			}
		}
	}
	return kept
}
