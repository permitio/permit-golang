package permit_test

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/surface"
)

// The SDK's API reference is its documentation on pkg.go.dev, which go/doc reads from the source.
// The tests in this file are the fail-closed form of what pkg.go.dev needs. godoclint and
// testableexamples cover part of it: the doc comments of packages and top-level declarations,
// and Examples without an output comment. The tests add doc links that resolve, the doc comments
// of struct fields and of interface and promoted methods, Examples for the service types and the
// decision methods, docRules, and a count sentinel. Of the repository, they read only files of
// the module zip: the sources of the packages in surfaceDirs, their tests, and testdata; a doc
// link to the standard library makes them read its package in GOROOT. CONTRIBUTING.md ("API
// reference") says how to run them.

// sdkModule is the SDK as the documentation checks read it.
var sdkModule = docModule{
	path: "github.com/permitio/permit-golang/v2",
	root: ".",
	dirs: surfaceDirs,
}

// decisionMethods are the methods that make an authorization decision, as <Type>.<Method>, such
// as Client.Check. Each needs an Example with an output comment, like each service type. The
// commits that add the methods add them here.
var decisionMethods = []string{"Client.BulkCheck", "Client.Check", "Client.FilterObjects"}

// docRule requires a phrase in the doc comment of a method, or forbids it.
type docRule struct {
	// Method is the method, as <Type>.<Method>, such as UsersService.Create.
	Method string

	// Phrase is what the doc comment must say, such as "waits for sync". The comparison ignores
	// case, and counts a line break or a run of spaces as one space.
	Phrase string

	// Absent says that the doc comment must not say Phrase, such as "waits for sync" on a method
	// that does not wait.
	Absent bool
}

// The phrases of the doc rules other than the key levels.
const (
	containerOnlyPhrase = "container PDP only"
	waitsForSyncPhrase  = "waits for sync"
)

// keyLevelPhrases name the level of API key that an operation needs, as the doc comments of the
// methods that call it say it.
var keyLevelPhrases = map[rest.KeyLevel]string{
	rest.EnvironmentLevel:  "environment-level API key",
	rest.ProjectLevel:      "project-level API key",
	rest.OrganizationLevel: "organization-level API key",
}

// docRules returns the rules of the doc comments of the methods that methods binds to the IDs of
// the operations they call, such as the Methods of the operation map, with the operations of
// registry, sorted by method, and a problem for each ID that registry does not hold. The doc
// comment of each method names the level of API key that it needs, the widest level of its
// operations; says "container PDP only" when only the container PDP serves one of them; and says
// "waits for sync" when one of them waits for the PDP's policy, and only then.
func docRules(methods map[string][]string, registry *rest.Registry) ([]docRule, []string) {
	var (
		rules    []docRule
		problems []string
	)
	for _, method := range slices.Sorted(maps.Keys(methods)) {
		var level rest.KeyLevel
		var containerOnly, waits, known bool
		for _, id := range methods[method] {
			op, ok := registry.Op(id)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s calls the operation %s, which the "+
					"operation registry does not hold", method, id))
				continue
			}
			known = true
			level = max(level, op.KeyLevel)
			containerOnly = containerOnly || op.ContainerOnly
			waits = waits || op.WaitsForSync
		}
		if !known {
			continue
		}
		rules = append(rules, docRule{Method: method, Phrase: keyLevelPhrases[level]})
		if containerOnly {
			rules = append(rules, docRule{Method: method, Phrase: containerOnlyPhrase})
		}
		rules = append(rules, docRule{Method: method, Phrase: waitsForSyncPhrase, Absent: !waits})
	}
	return rules, problems
}

// docModule is a module whose public packages the documentation checks read.
type docModule struct {
	// path is the module path.
	path string

	// root is the module's directory.
	root string

	// dirs are the directories of the packages to check, relative to root, the root package first.
	dirs []string
}

// importPath returns the import path of the package in dir, a directory relative to the module
// root.
func (m docModule) importPath(dir string) string {
	if dir == "." {
		return m.path
	}
	return m.path + "/" + filepath.ToSlash(dir)
}

// read reads the documentation of the packages in m.dirs, in order.
func (m docModule) read() ([]*docPackage, error) {
	pkgs := make([]*docPackage, 0, len(m.dirs))
	for _, dir := range m.dirs {
		p, err := readDocPackage(filepath.Join(m.root, dir), m.importPath(dir))
		if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// docPackage is the documentation of a package as pkg.go.dev shows it.
type docPackage struct {
	// path is the import path.
	path string

	fset *token.FileSet

	// files are the files that build the package, without its tests, with their comments.
	files []*ast.File

	// typeSpecs maps the name of each type that files declare to its declaration, with every
	// member. go/doc leaves the unexported members out of its copy, such as an unexported
	// interface that an exported interface embeds, which internal/surface lists.
	typeSpecs map[string]*ast.TypeSpec

	// doc is go/doc's reading of a copy of files, with the Examples of the external test package.
	// Its lists hold the exported identifiers and, for each type, the methods it promotes too.
	doc *doc.Package

	// tests are the test files of the package, internal and external, with their comments.
	tests []*ast.File

	// examples are the Examples of tests.
	examples []*doc.Example
}

// readDocPackage reads the documentation of the package in dir, whose import path is path: the
// files that go/build selects to build it on this machine, and its test files for the Examples.
func readDocPackage(dir, path string) (*docPackage, error) {
	bp, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		return nil, fmt.Errorf("reading the package in %s: %w", dir, err)
	}
	fset := token.NewFileSet()
	names := slices.Concat(bp.GoFiles, bp.CgoFiles)
	files, err := parseGoFiles(fset, dir, names)
	if err != nil {
		return nil, err
	}
	// go/doc removes the unexported declarations from the files it reads, so it reads a copy.
	docFiles, err := parseGoFiles(fset, dir, names)
	if err != nil {
		return nil, err
	}
	internal, err := parseGoFiles(fset, dir, bp.TestGoFiles)
	if err != nil {
		return nil, err
	}
	external, err := parseGoFiles(fset, dir, bp.XTestGoFiles)
	if err != nil {
		return nil, err
	}
	// Only an Example of the external test package counts, as pkg.go.dev shows it with code that
	// uses the package the way an application does.
	d, err := doc.NewFromFiles(fset, slices.Concat(docFiles, external), path,
		doc.AllMethods|doc.PreserveAST)
	if err != nil {
		return nil, fmt.Errorf("reading the documentation of %s: %w", path, err)
	}
	typeSpecs := make(map[string]*ast.TypeSpec)
	for _, f := range files {
		for _, decl := range f.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
				for _, spec := range gen.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						typeSpecs[ts.Name.Name] = ts
					}
				}
			}
		}
	}
	tests := slices.Concat(internal, external)
	return &docPackage{
		path:      path,
		fset:      fset,
		files:     files,
		typeSpecs: typeSpecs,
		doc:       d,
		tests:     tests,
		examples:  doc.Examples(tests...),
	}, nil
}

// parseGoFiles parses the files names in dir with their comments.
func parseGoFiles(fset *token.FileSet, dir string, names []string) ([]*ast.File, error) {
	files := make([]*ast.File, 0, len(names))
	for _, name := range names {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// position returns pos as file:line, with the file as the files were parsed, relative to the
// test's directory.
func position(fset *token.FileSet, pos token.Pos) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d", filepath.ToSlash(p.Filename), p.Line)
}

// readSDKDocs reads the documentation of the SDK's public packages, the root package first.
func readSDKDocs(t *testing.T) []*docPackage {
	t.Helper()
	pkgs, err := sdkModule.read()
	if err != nil {
		t.Fatal(err)
	}
	return pkgs
}

// anyLinkParser parses all bracketed text that has the shape of a doc link as a doc link, whether
// or not it names anything, so that the check also sees the links that go/doc leaves as plain
// text. A package name has the shape of an identifier, and the parser keeps it as the link's
// ImportPath.
var anyLinkParser = &comment.Parser{
	LookupPackage: func(name string) (string, bool) { return name, token.IsIdentifier(name) },
	LookupSym:     func(string, string) bool { return true },
}

// linkChecker resolves the doc links of a module's packages, and reads each package that a link
// names once.
type linkChecker struct {
	module docModule

	// read holds go/doc's reading of each package read so far, by import path.
	read map[string]*doc.Package
}

// newLinkChecker returns a linkChecker for the packages pkgs of module.
func newLinkChecker(module docModule, pkgs []*docPackage) *linkChecker {
	c := &linkChecker{module: module, read: make(map[string]*doc.Package)}
	for _, p := range pkgs {
		c.read[p.path] = p.doc
	}
	return c
}

// problems returns a problem for each doc link in the doc comments of p that names nothing, with
// its file and line, and the number of doc links it checked. It reads the doc comments of the
// files of p and those of its Examples, which pkg.go.dev shows above their code; a link in the doc
// comment of an Example names what p declares or imports, also in the external test package.
func (c *linkChecker) problems(p *docPackage) (problems []string, links int) {
	var groups []*ast.CommentGroup
	for _, f := range p.files {
		groups = append(groups, docComments(f)...)
	}
	for _, group := range slices.Concat(groups, exampleDocs(p)) {
		for _, link := range docLinks(anyLinkParser.Parse(group.Text())) {
			links++
			if err := c.resolve(p, link); err != nil {
				text := linkText(link)
				problems = append(problems, fmt.Sprintf("%s: the doc link [%s] does not "+
					"resolve: %v", linkPosition(p.fset, group, text), text, err))
			}
		}
	}
	return problems, links
}

// exampleDocs returns the doc comments of the Examples of p, in the order of its test files.
func exampleDocs(p *docPackage) []*ast.CommentGroup {
	examples := make(map[string]bool, len(p.examples))
	for _, ex := range p.examples {
		examples["Example"+ex.Name] = true
	}
	var groups []*ast.CommentGroup
	for _, f := range p.tests {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Doc != nil && examples[fn.Name.Name] {
				groups = append(groups, fn.Doc)
			}
		}
	}
	return groups
}

// resolve returns nil when link, in the documentation of p, names a package or an exported
// identifier the way go/doc resolves it, and otherwise an error that says why it does not. Unlike
// go/doc, it also checks that the identifier of a qualified link, such as [models.User], exists.
func (c *linkChecker) resolve(p *docPackage, link *comment.DocLink) error {
	path := link.ImportPath
	if path != "" && !strings.Contains(path, "/") {
		name := path
		var ok bool
		if path, ok = p.doc.Parser().LookupPackage(name); !ok {
			if path, ok = comment.DefaultLookupPackage(name); !ok {
				return fmt.Errorf("package %s imports no package named %s, and %s is not a "+
					"standard library package with a one-element path", p.doc.Name, name, name)
			}
		}
	}
	target := p.doc
	if path != "" && path != p.path {
		var err error
		if target, err = c.pkg(path); err != nil {
			return err
		}
	}
	switch {
	case link.Name == "" || target.Parser().LookupSym(link.Recv, link.Name):
		return nil
	case link.Recv == "":
		return fmt.Errorf("package %s has no exported %s", target.Name, link.Name)
	default:
		return fmt.Errorf("package %s has no exported %s.%s", target.Name, link.Recv, link.Name)
	}
}

// pkg returns go/doc's reading of the package with the import path path: a package of the module,
// or of the standard library in GOROOT, which go test sets. The packages that c was made with, the
// module's root package among them, are read already.
func (c *linkChecker) pkg(path string) (*doc.Package, error) {
	if p, ok := c.read[path]; ok {
		return p, nil
	}
	var dir string
	first, _, _ := strings.Cut(path, "/")
	switch {
	case strings.HasPrefix(path, c.module.path+"/"):
		dir = filepath.Join(c.module.root, filepath.FromSlash(path[len(c.module.path)+1:]))
	case !strings.Contains(first, "."):
		dir = filepath.Join(build.Default.GOROOT, "src", filepath.FromSlash(path))
	default:
		return nil, fmt.Errorf("%s is a package outside %s and the standard library, which the "+
			"check cannot read; link its documentation with a URL instead", path, c.module.path)
	}
	bp, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		return nil, fmt.Errorf("reading package %s: %w", path, err)
	}
	fset := token.NewFileSet()
	files, err := parseGoFiles(fset, dir, slices.Concat(bp.GoFiles, bp.CgoFiles))
	if err != nil {
		return nil, fmt.Errorf("reading package %s: %w", path, err)
	}
	p, err := doc.NewFromFiles(fset, files, path, doc.AllMethods)
	if err != nil {
		return nil, fmt.Errorf("reading package %s: %w", path, err)
	}
	c.read[path] = p
	return p, nil
}

// docComments returns the comments of f that are documentation: the package comment, and the doc
// and line comments of declarations, specs, struct fields and interface methods, outside function
// bodies.
func docComments(f *ast.File) []*ast.CommentGroup {
	var groups []*ast.CommentGroup
	add := func(cgs ...*ast.CommentGroup) {
		for _, cg := range cgs {
			if cg != nil {
				groups = append(groups, cg)
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.File:
			add(n.Doc)
		case *ast.GenDecl:
			add(n.Doc)
		case *ast.FuncDecl:
			add(n.Doc)
			return false
		case *ast.TypeSpec:
			add(n.Doc, n.Comment)
		case *ast.ValueSpec:
			add(n.Doc, n.Comment)
			return false
		case *ast.Field:
			add(n.Doc, n.Comment)
		}
		return true
	})
	return groups
}

// docLinks returns the doc links of d, in order: those of its paragraphs and list items. Its
// headings and code blocks hold no doc links.
func docLinks(d *comment.Doc) []*comment.DocLink {
	var links []*comment.DocLink
	var blocks func([]comment.Block)
	blocks = func(bs []comment.Block) {
		for _, b := range bs {
			switch b := b.(type) {
			case *comment.Paragraph:
				for _, t := range b.Text {
					if link, ok := t.(*comment.DocLink); ok {
						links = append(links, link)
					}
				}
			case *comment.List:
				for _, item := range b.Items {
					blocks(item.Content)
				}
			}
		}
	}
	blocks(d.Content)
	return links
}

// linkText returns the text of link as its comment writes it between the brackets, such as
// *Client.
func linkText(link *comment.DocLink) string {
	var b strings.Builder
	for _, t := range link.Text {
		if plain, ok := t.(comment.Plain); ok {
			b.WriteString(string(plain))
		}
	}
	return b.String()
}

// linkPosition returns the file and line of the first comment line of group that holds the doc
// link [text], or of the group when no line does. The comment parser keeps no positions, so when
// the same link text is on more than one line of group, each of its problems names the first of
// them: a known limit.
func linkPosition(fset *token.FileSet, group *ast.CommentGroup, text string) string {
	for _, c := range group.List {
		if strings.Contains(c.Text, "["+text+"]") {
			return position(fset, c.Pos())
		}
	}
	return position(fset, group.Pos())
}

// commentCheck collects what commentProblems finds.
type commentCheck struct {
	pkg      *docPackage
	problems []string

	// checked is the number of identifiers checked.
	checked int
}

// commentProblems returns a problem when p has no package comment and for each exported
// identifier of p without a doc comment, and the number of identifiers it checked: constants,
// variables, types, functions, methods, also those a type promotes, and the exported fields and
// embedded types of structs and the methods and embedded elements of interfaces. A constant or
// variable has a doc comment when its declaration, its spec or its line has one, and so does a
// field or an interface method.
func commentProblems(p *docPackage) (problems []string, checked int) {
	c := &commentCheck{pkg: p}
	if strings.TrimSpace(p.doc.Doc) == "" {
		c.problems = append(c.problems, fmt.Sprintf("package %s (%s) has no package comment; "+
			"pkg.go.dev shows it as the package's overview", p.doc.Name, p.path))
	}
	c.values(p.doc.Consts)
	c.values(p.doc.Vars)
	c.funcs(p.doc.Funcs)
	for _, t := range p.doc.Types {
		c.typ(t)
		c.values(t.Consts)
		c.values(t.Vars)
		c.funcs(t.Funcs)
		c.methods(t)
	}
	return c.problems, c.checked
}

// check counts the identifier what, declared at pos, and records a problem when it is not
// documented.
func (c *commentCheck) check(pos token.Pos, what string, documented bool) {
	c.checked++
	if !documented {
		c.problems = append(c.problems, fmt.Sprintf("%s: %s has no doc comment",
			position(c.pkg.fset, pos), what))
	}
}

// values checks the exported constants or variables of vs.
func (c *commentCheck) values(vs []*doc.Value) {
	for _, v := range vs {
		for _, spec := range v.Decl.Specs {
			vspec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vspec.Names {
				if name.IsExported() {
					c.check(name.Pos(), v.Decl.Tok.String()+" "+name.Name,
						v.Doc != "" || vspec.Doc != nil || vspec.Comment != nil)
				}
			}
		}
	}
}

// funcs checks the functions fs.
func (c *commentCheck) funcs(fs []*doc.Func) {
	for _, f := range fs {
		c.check(f.Decl.Name.Pos(), "func "+f.Name, f.Doc != "")
	}
}

// methods checks the methods of t, which go/doc lists only when they are exported, as it reads
// the package without doc.AllDecls. A method that t promotes from an exported type is checked
// there, so it only counts here; one promoted from an unexported type is checked here, at its
// declaration.
func (c *commentCheck) methods(t *doc.Type) {
	for _, m := range t.Methods {
		what := "method " + t.Name + "." + m.Name
		if m.Level > 0 {
			what += " (promoted from " + m.Orig + ")"
		}
		fromExported := m.Level > 0 && token.IsExported(strings.TrimPrefix(m.Orig, "*"))
		c.check(m.Decl.Name.Pos(), what, m.Doc != "" || fromExported)
	}
}

// typ checks the type t, and the members of a struct or interface type, as the package declares
// them.
func (c *commentCheck) typ(t *doc.Type) {
	spec := c.pkg.typeSpecs[t.Name]
	c.check(spec.Name.Pos(), "type "+t.Name, t.Doc != "")
	switch typ := spec.Type.(type) {
	case *ast.StructType:
		c.members(t.Name, "field", typ.Fields)
		c.interfaceMethods(t, typ)
	case *ast.InterfaceType:
		c.members(t.Name, "method", typ.Methods)
	}
}

// promotedMethod is a method that a struct gets from an interface that it embeds.
type promotedMethod struct {
	// iface is the name of the interface that declares the method.
	iface string

	field *ast.Field
}

// interfaceMethods checks the exported methods that the struct type t gets from the interfaces
// of the package that it embeds, itself or through the types it embeds, which go/doc does not
// list, and internal/surface does: those that t neither declares nor promotes from a struct. A
// method of an exported interface is checked there, so it only counts here.
func (c *commentCheck) interfaceMethods(t *doc.Type, st *ast.StructType) {
	methods := make(map[string]promotedMethod)
	seen := make(map[string]bool)
	for _, field := range st.Fields.List {
		if field.Names == nil {
			c.embed(field.Type, seen, methods)
		}
	}
	listed := make(map[string]bool)
	for _, m := range t.Methods {
		listed[m.Name] = true
	}
	for _, name := range slices.Sorted(maps.Keys(methods)) {
		m := methods[name]
		if !listed[name] {
			c.check(m.field.Pos(), "method "+t.Name+"."+name+" (promoted from "+m.iface+")",
				m.field.Doc != nil || m.field.Comment != nil || token.IsExported(m.iface))
		}
	}
}

// embed adds to methods the exported methods of the interfaces of the package that embedding the
// type that expr names gives a struct, through the structs and interfaces it embeds; seen holds
// the types already walked.
func (c *commentCheck) embed(expr ast.Expr, seen map[string]bool,
	methods map[string]promotedMethod,
) {
	name := localTypeName(expr)
	spec := c.pkg.typeSpecs[name]
	if spec == nil || seen[name] {
		return
	}
	seen[name] = true
	switch typ := spec.Type.(type) {
	case *ast.StructType:
		for _, field := range typ.Fields.List {
			if field.Names == nil {
				c.embed(field.Type, seen, methods)
			}
		}
	case *ast.InterfaceType:
		for _, field := range typ.Methods.List {
			switch {
			case field.Names == nil:
				c.embed(field.Type, seen, methods)
			case field.Names[0].IsExported() && methods[field.Names[0].Name].field == nil:
				methods[field.Names[0].Name] = promotedMethod{iface: name, field: field}
			}
		}
	default:
		// An alias, or a type defined as another type, has the methods of that type.
		c.embed(spec.Type, seen, methods)
	}
}

// members checks the exported fields or methods, as kind names them, and the embedded types of
// the struct or interface type named typeName. The embedded unexported types of a struct are not
// members of their own.
func (c *commentCheck) members(typeName, kind string, list *ast.FieldList) {
	for _, field := range list.List {
		documented := field.Doc != nil || field.Comment != nil
		if field.Names == nil {
			if kind == "field" && !token.IsExported(embeddedName(field.Type)) {
				continue
			}
			c.check(field.Pos(), "embedded "+types.ExprString(field.Type)+" in "+typeName,
				documented)
			continue
		}
		for _, name := range field.Names {
			if name.IsExported() {
				c.check(name.Pos(), kind+" "+typeName+"."+name.Name, documented)
			}
		}
	}
}

// embeddedName returns the name of the type that an embedded field names, without its package,
// a * or type arguments, or "" when expr names no one type.
func embeddedName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.StarExpr:
		return embeddedName(e.X)
	case *ast.IndexExpr:
		return embeddedName(e.X)
	case *ast.IndexListExpr:
		return embeddedName(e.X)
	}
	return ""
}

// surfaceDeclarations returns the number of declarations in text, the content of
// testdata/api-surface.txt: its lines that are neither blank nor comments.
func surfaceDeclarations(text string) int {
	n := 0
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			n++
		}
	}
	return n
}

// identifierSentinel returns a problem when the doc comment check counted fewer identifiers than
// the surface snapshot lists declarations, which means that it skipped some, and "" otherwise.
func identifierSentinel(checked, declarations int) string {
	if checked >= declarations {
		return ""
	}
	return fmt.Sprintf("count sentinel: the doc comment check counted %d exported identifiers, "+
		"but %s lists %d declarations; the check skipped some", checked, surfaceFile, declarations)
}

// hasOutput reports whether ex has an output comment, so that go test runs it and compares what
// it prints.
func hasOutput(ex *doc.Example) bool {
	return ex.Output != "" || ex.EmptyOutput
}

// serviceTypes returns the names of the service types of p, which need an Example: the exported
// types of the package that the exported fields of Client have, such as UsersService for the
// field Users *UsersService, and every exported type whose name ends in Service. When p declares
// Client as another kind of type than a struct type, such as an alias, it returns the types whose
// names end in Service and a problem, as the rule cannot read the fields of Client; otherwise the
// problem is "".
func serviceTypes(p *docPackage) (services map[string]bool, problem string) {
	exportedTypes := make(map[string]bool)
	services = make(map[string]bool)
	for _, t := range p.doc.Types {
		exportedTypes[t.Name] = true
		if strings.HasSuffix(t.Name, "Service") {
			services[t.Name] = true
		}
	}
	client, ok := p.typeSpecs["Client"]
	if !ok {
		return services, ""
	}
	st, ok := client.Type.(*ast.StructType)
	if !ok {
		return services, fmt.Sprintf("%s: Client is not a struct type; the service-type rule "+
			"cannot read its fields", position(p.fset, client.Name.Pos()))
	}
	for _, field := range st.Fields.List {
		exported := slices.ContainsFunc(field.Names, (*ast.Ident).IsExported) ||
			field.Names == nil && token.IsExported(embeddedName(field.Type))
		if name := localTypeName(field.Type); exported && exportedTypes[name] {
			services[name] = true
		}
	}
	return services, ""
}

// localTypeName returns the name of the type of the package that expr names, as T or *T, or ""
// when expr names another kind of type. It does not follow a generic type, such as Service[T] or
// *Service[T]: a known limit, so serviceTypes does not count a field of Client with such a type,
// and embed does not walk such an embedded type.
func localTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// findMethod returns the method of p that name names as <Type>.<Method>, also one the type
// promotes, or nil when p has no such method.
func findMethod(p *doc.Package, name string) *doc.Func {
	typeName, methodName, _ := strings.Cut(name, ".")
	for _, t := range p.Types {
		if t.Name != typeName {
			continue
		}
		for _, m := range t.Methods {
			if m.Name == methodName {
				return m
			}
		}
	}
	return nil
}

// exampleProblems returns a problem for each Example of p without an output comment, which go
// test compiles but does not run, when serviceTypes cannot read the fields of Client, for each
// service type of p without an Example, and for each method in methods, the decision methods,
// that p does not have or that has no Example. An Example counts for a type or method only when
// it is in the external test package and has an output comment. A type's Example is named
// Example<Type>, or Example<Type>_<suffix> with a suffix that starts with a lower-case letter;
// Example<Type>_<Method> is the Example of the method and does not count for the type.
func exampleProblems(p *docPackage, methods []string) []string {
	var problems []string
	for _, ex := range p.examples {
		if !hasOutput(ex) {
			problems = append(problems, fmt.Sprintf("%s: Example%s has no // Output: comment, so "+
				"go test compiles it but does not run it; end it with the output it prints",
				position(p.fset, ex.Code.Pos()), ex.Name))
		}
	}
	services, problem := serviceTypes(p)
	if problem != "" {
		problems = append(problems, problem)
	}
	for _, t := range p.doc.Types {
		if services[t.Name] && !slices.ContainsFunc(t.Examples, hasOutput) {
			problems = append(problems, fmt.Sprintf("%s: service type %s has no Example with an "+
				"output comment in package %s_test; add Example%s", position(p.fset,
				p.typeSpecs[t.Name].Name.Pos()), t.Name, p.doc.Name, t.Name))
		}
	}
	for _, name := range methods {
		m := findMethod(p.doc, name)
		switch {
		case m == nil:
			problems = append(problems, fmt.Sprintf("decisionMethods lists %s, which is not a "+
				"method of package %s", name, p.doc.Name))
		case !slices.ContainsFunc(m.Examples, hasOutput):
			problems = append(problems, fmt.Sprintf("%s: decision method %s has no Example with "+
				"an output comment in package %s_test; add Example%s", position(p.fset,
				m.Decl.Pos()), name, p.doc.Name, strings.Replace(name, ".", "_", 1)))
		}
	}
	return problems
}

// docRuleProblems returns a problem for each rule whose method p does not have, whose phrase is
// empty, or whose phrase the method's doc comment does not hold, or holds when the rule says it
// must be absent.
func docRuleProblems(p *docPackage, rules []docRule) []string {
	var problems []string
	for _, rule := range rules {
		m := findMethod(p.doc, rule.Method)
		if m == nil {
			problems = append(problems, fmt.Sprintf("docRules names %s, which is not a method of "+
				"package %s", rule.Method, p.doc.Name))
			continue
		}
		says := strings.Contains(normalizePhrase(m.Doc), normalizePhrase(rule.Phrase))
		switch {
		case normalizePhrase(rule.Phrase) == "":
			problems = append(problems, "docRules has an empty phrase for "+rule.Method)
		case rule.Absent && says:
			problems = append(problems, fmt.Sprintf("%s: the doc comment of %s says %q, which "+
				"it must not say", position(p.fset, m.Decl.Pos()), rule.Method, rule.Phrase))
		case !rule.Absent && !says:
			problems = append(problems, fmt.Sprintf("%s: the doc comment of %s does not say %q",
				position(p.fset, m.Decl.Pos()), rule.Method, rule.Phrase))
		}
	}
	return problems
}

// normalizePhrase returns s in lower case, with each run of white space, a line break too, as one
// space, and without white space at either end.
func normalizePhrase(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func TestDocLinks(t *testing.T) {
	pkgs := readSDKDocs(t)
	c := newLinkChecker(sdkModule, pkgs)
	links := 0
	for _, p := range pkgs {
		problems, n := c.problems(p)
		links += n
		for _, problem := range problems {
			t.Error(problem)
		}
	}
	t.Logf("checked %d doc links in %d packages", links, len(pkgs))
}

func TestDocComments(t *testing.T) {
	checked := 0
	for _, p := range readSDKDocs(t) {
		problems, n := commentProblems(p)
		checked += n
		for _, problem := range problems {
			t.Error(problem)
		}
	}
	text, err := os.ReadFile(surfaceFile)
	if err != nil {
		t.Fatal(err)
	}
	declarations := surfaceDeclarations(string(text))
	if problem := identifierSentinel(checked, declarations); problem != "" {
		t.Error(problem)
	}
	t.Logf("checked the doc comments of %d exported identifiers; %s lists %d declarations",
		checked, surfaceFile, declarations)
}

func TestDocExamples(t *testing.T) {
	examples := 0
	for i, p := range readSDKDocs(t) {
		var methods []string
		if i == 0 {
			methods = decisionMethods
		}
		for _, problem := range exampleProblems(p, methods) {
			t.Error(problem)
		}
		examples += len(p.examples)
	}
	t.Logf("checked %d Examples and %d decision methods", examples, len(decisionMethods))
}

// TestDocRules checks the doc comment of every public method that a wire case binds to an
// operation against the rules that docRules derives from the operation registry.
func TestDocRules(t *testing.T) {
	rules, problems := docRules(buildOperationMap(wireCases(t)).Methods, rest.Default())
	problems = append(problems, docRuleProblems(readSDKDocs(t)[0], rules)...)
	for _, problem := range problems {
		t.Error(problem)
	}
	t.Logf("checked %d doc rules", len(rules))
}

// readFixture reads the documentation of the packages in dirs of the fixture module in
// testdata/doccheck/<name>, whose module path is example.com/sdk, the root package first.
func readFixture(t *testing.T, name string, dirs ...string) (docModule, []*docPackage) {
	t.Helper()
	m := docModule{
		path: "example.com/sdk", root: filepath.Join("testdata", "doccheck", name),
		dirs: dirs,
	}
	pkgs, err := m.read()
	if err != nil {
		t.Fatal(err)
	}
	return m, pkgs
}

// fixtureChecks runs every documentation check on the fixture module in
// testdata/doccheck/<name>, with methods as the decision methods and rules as the doc rules, and
// returns the problems and the numbers of doc links and identifiers checked.
func fixtureChecks(t *testing.T, name string, methods []string,
	rules []docRule,
) (problems []string, links, identifiers int) {
	t.Helper()
	m, pkgs := readFixture(t, name, ".", "models")
	c := newLinkChecker(m, pkgs)
	for i, p := range pkgs {
		linkProblems, n := c.problems(p)
		docProblems, checked := commentProblems(p)
		problems = slices.Concat(problems, linkProblems, docProblems)
		links += n
		identifiers += checked
		if i == 0 {
			problems = slices.Concat(problems, exampleProblems(p, methods),
				docRuleProblems(p, rules))
		} else {
			problems = append(problems, exampleProblems(p, nil)...)
		}
	}
	return problems, links, identifiers
}

func TestDocChecksPassAFixture(t *testing.T) {
	problems, links, identifiers := fixtureChecks(t, "pass", []string{"Client.Check"}, []docRule{
		{Method: "UsersService.Get", Phrase: "waits for sync"},
		{Method: "Client.Check", Phrase: "Container  PDP\tonly"},
		{Method: "Client.Check", Phrase: "waits for sync", Absent: true},
	})
	if len(problems) != 0 {
		t.Errorf("the checks found problems in the pass fixture:\n%s",
			strings.Join(problems, "\n"))
	}
	if links != 14 || identifiers != 10 {
		t.Errorf("the checks counted %d doc links and %d identifiers, want 14 and 10", links,
			identifiers)
	}
}

func TestDocLinksThatDoNotResolve(t *testing.T) {
	m, pkgs := readFixture(t, "links", ".", "models")
	c := newLinkChecker(m, pkgs)
	var got []string
	links := 0
	for _, p := range pkgs {
		problems, n := c.problems(p)
		got = append(got, problems...)
		links += n
	}
	const dir = "testdata/doccheck/links/"
	const noNothing = " does not resolve: package sdk has no exported "
	want := []string{
		dir + "sdk.go:2: the doc link [Nothing]" + noNothing + "Nothing",
		dir + "sdk.go:2: the doc link [Client.Nothing]" + noNothing + "Client.Nothing",
		dir + "sdk.go:2: the doc link [sdk.Nothing]" + noNothing + "Nothing",
		dir + "sdk.go:2: the doc link [*Nothing]" + noNothing + "Nothing",
		dir + "sdk.go:12: the doc link [models.Nothing] does not resolve: package models has " +
			"no exported Nothing",
		dir + "sdk.go:12: the doc link [model.User] does not resolve: package sdk imports no " +
			"package named model, and model is not a standard library package with a " +
			"one-element path",
		dir + "sdk.go:13: the doc link [context.Nothing] does not resolve: package context " +
			"has no exported Nothing",
		dir + "sdk.go:13: the doc link [strings.Nothing] does not resolve: package strings " +
			"has no exported Nothing",
		dir + "sdk.go:14: the doc link [http.Client] does not resolve: package sdk imports no " +
			"package named http, and http is not a standard library package with a " +
			"one-element path",
		dir + "sdk.go:14: the doc link [example.com/elsewhere.Thing] does not resolve: " +
			"example.com/elsewhere is a package outside example.com/sdk and the standard " +
			"library, which the check cannot read; link its documentation with a URL instead",
		dir + "sdk.go:15: the doc link [example.com/sdk/nothere.Thing] does not resolve: " +
			"reading package example.com/sdk/nothere: cannot find package \".\" in:\n\t" +
			filepath.Join("testdata", "doccheck", "links", "nothere"),
		dir + "sdk.go:16: the doc link [example.com/sdk/models.Nothing] does not resolve: " +
			"package models has no exported Nothing",
		dir + "sdk.go:18: the doc link [Nothing]" + noNothing + "Nothing",
		dir + "sdk.go:20: the doc link [UsersService.Nothing]" + noNothing +
			"UsersService.Nothing",
		dir + "sdk.go:23: the doc link [Kind.Nothing]" + noNothing + "Kind.Nothing",
		dir + "sdk.go:31: the doc link [Nothing]" + noNothing + "Nothing",
		dir + "sdk.go:34: the doc link [Nothing]" + noNothing + "Nothing",
		dir + "sdk.go:38: the doc link [Nothing]" + noNothing + "Nothing",
		dir + "sdk.go:42: the doc link [Getter.Nothing]" + noNothing + "Getter.Nothing",
		dir + "sdk.go:49: the doc link [Nothing]" + noNothing + "Nothing",
		dir + "sdk.go:58: the doc link [hidden] does not resolve: package sdk imports no " +
			"package named hidden, and hidden is not a standard library package with a " +
			"one-element path",
		dir + "internal_test.go:5: the doc link [Kind.Nothing]" + noNothing + "Kind.Nothing",
		dir + "sdk_test.go:5: the doc link [Client.Nothing]" + noNothing + "Client.Nothing",
		dir + "models/models.go:1: the doc link [Usr] does not resolve: package models has no " +
			"exported Usr",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the link problems differ (-want +got):\n%s", diff)
	}
	if links != 29 {
		t.Errorf("checked %d doc links, want 29", links)
	}
}

func TestDocCommentsThatAreMissing(t *testing.T) {
	_, pkgs := readFixture(t, "undocumented", ".", "models")
	var got []string
	checked := 0
	for _, p := range pkgs {
		problems, n := commentProblems(p)
		got = append(got, problems...)
		checked += n
	}
	const dir = "testdata/doccheck/undocumented/"
	want := []string{
		"package sdk (example.com/sdk) has no package comment; pkg.go.dev shows it as the " +
			"package's overview",
		// go/doc lists grouped constants and variables first.
		dir + "sdk.go:17: const E has no doc comment",
		dir + "sdk.go:5: const Version has no doc comment",
		dir + "sdk.go:20: var Debug has no doc comment",
		dir + "sdk.go:22: func Helper has no doc comment",
		dir + "sdk.go:83: method Aliased.Read (promoted from reader) has no doc comment",
		// Client promotes Bye from Base, an exported type, which reports it once.
		dir + "sdk.go:29: method Base.Bye has no doc comment",
		dir + "sdk.go:27: method Base.Hello has no doc comment",
		dir + "sdk.go:31: type Client has no doc comment",
		dir + "sdk.go:32: field Client.Users has no doc comment",
		dir + "sdk.go:36: embedded Base in Client has no doc comment",
		dir + "sdk.go:41: func New has no doc comment",
		dir + "sdk.go:45: method Client.Get (promoted from hidden) has no doc comment",
		dir + "sdk.go:54: method Getter.Get has no doc comment",
		dir + "sdk.go:55: embedded fmt.Stringer in Getter has no doc comment",
		dir + "sdk.go:50: const KindUser has no doc comment",
		dir + "sdk.go:83: method Reader.Read (promoted from reader) has no doc comment",
		dir + "sdk.go:102: method UsersService.List has no doc comment",
		dir + "sdk.go:83: method Wrapper.Read (promoted from reader) has no doc comment",
		"package models (example.com/sdk/models) has no package comment; pkg.go.dev shows " +
			"it as the package's overview",
		dir + "models/models.go:5: field User.Key has no doc comment",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the doc comment problems differ (-want +got):\n%s", diff)
	}
	if checked != 49 {
		t.Errorf("checked %d identifiers, want 49", checked)
	}
}

// TestDocCommentsCountWhatSurfaceLists checks the count sentinel's premise on the fixtures: the
// doc comment check counts each declaration that internal/surface lists, which
// testdata/api-surface.txt holds for the SDK.
func TestDocCommentsCountWhatSurfaceLists(t *testing.T) {
	for _, name := range []string{"pass", "undocumented"} {
		m, pkgs := readFixture(t, name, ".", "models")
		checked, listed := 0, 0
		for i, p := range pkgs {
			_, n := commentProblems(p)
			checked += n
			s, err := surface.Read(filepath.Join(m.root, m.dirs[i]))
			if err != nil {
				t.Fatal(err)
			}
			listed += len(s.Items)
		}
		if checked != listed || checked == 0 {
			t.Errorf("%s: the doc comment check counted %d identifiers, and internal/surface "+
				"lists %d declarations; want the same number, and more than 0", name, checked,
				listed)
		}
	}
}

func TestDocExamplesThatAreMissing(t *testing.T) {
	_, pkgs := readFixture(t, "examples", ".")
	got := exampleProblems(pkgs[0], []string{"Client.Check", "Client.Allowed", "Client.Missing"})
	const dir = "testdata/doccheck/examples/"
	want := []string{
		dir + "internal_test.go:5: ExampleClient has no // Output: comment, so go test compiles " +
			"it but does not run it; end it with the output it prints",
		dir + "sdk_test.go:28: ExampleClient_Allowed has no // Output: comment, so go test " +
			"compiles it but does not run it; end it with the output it prints",
		dir + "sdk_test.go:24: ExampleTenantsService has no // Output: comment, so go test " +
			"compiles it but does not run it; end it with the output it prints",
		dir + "sdk.go:44: service type AuditService has no Example with an output comment in " +
			"package sdk_test; add ExampleAuditService",
		dir + "sdk.go:32: service type RolesAPI has no Example with an output comment in " +
			"package sdk_test; add ExampleRolesAPI",
		dir + "sdk.go:41: service type TenantsService has no Example with an output comment " +
			"in package sdk_test; add ExampleTenantsService",
		dir + "sdk.go:38: service type UsersService has no Example with an output comment in " +
			"package sdk_test; add ExampleUsersService",
		dir + "sdk.go:29: decision method Client.Allowed has no Example with an output " +
			"comment in package sdk_test; add ExampleClient_Allowed",
		"decisionMethods lists Client.Missing, which is not a method of package sdk",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the Example problems differ (-want +got):\n%s", diff)
	}

	_, pkgs = readFixture(t, "notstruct", ".")
	got = exampleProblems(pkgs[0], nil)
	want = []string{
		"testdata/doccheck/notstruct/sdk.go:6: Client is not a struct type; the service-type " +
			"rule cannot read its fields",
		"testdata/doccheck/notstruct/sdk.go:18: service type AuditService has no Example with " +
			"an output comment in package sdk_test; add ExampleAuditService",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the Example problems of a Client that is not a struct differ (-want +got):\n%s",
			diff)
	}
}

func TestDocServiceTypes(t *testing.T) {
	for _, tc := range []struct {
		fixture     string
		want        map[string]bool
		wantProblem string
	}{
		{
			fixture: "examples",
			want: map[string]bool{
				"AuditService":   true,
				"RolesAPI":       true,
				"TenantsService": true,
				"UsersService":   true,
			},
		},
		{
			// The rule cannot read the fields of an alias, and only the names count.
			fixture: "notstruct",
			want:    map[string]bool{"AuditService": true},
			wantProblem: "testdata/doccheck/notstruct/sdk.go:6: Client is not a struct type; " +
				"the service-type rule cannot read its fields",
		},
	} {
		_, pkgs := readFixture(t, tc.fixture, ".")
		got, problem := serviceTypes(pkgs[0])
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("%s: serviceTypes differs (-want +got):\n%s", tc.fixture, diff)
		}
		if problem != tc.wantProblem {
			t.Errorf("%s: serviceTypes returned the problem %q, want %q", tc.fixture, problem,
				tc.wantProblem)
		}
	}
}

func TestDocRulesThatFail(t *testing.T) {
	_, pkgs := readFixture(t, "pass", ".", "models")
	got := docRuleProblems(pkgs[0], []docRule{
		{Method: "Client.Check", Phrase: "waits for sync"},
		{Method: "Client.Nothing", Phrase: "container PDP only"},
		{Method: "UsersService.Get", Phrase: " \n "},
		{Method: "UsersService.Get", Phrase: "", Absent: true},
		{Method: "UsersService.Get", Phrase: "waits for the sync"},
		{Method: "UsersService.Get", Phrase: "Waits  for\nsync", Absent: true},
		{Method: "Client.Nothing", Phrase: "waits for sync", Absent: true},
	})
	const dir = "testdata/doccheck/pass/"
	want := []string{
		dir + `sdk.go:36: the doc comment of Client.Check does not say "waits for sync"`,
		"docRules names Client.Nothing, which is not a method of package sdk",
		"docRules has an empty phrase for UsersService.Get",
		"docRules has an empty phrase for UsersService.Get",
		dir + `sdk.go:45: the doc comment of UsersService.Get does not say "waits for the sync"`,
		dir + `sdk.go:45: the doc comment of UsersService.Get says "Waits  for\nsync", which ` +
			"it must not say",
		"docRules names Client.Nothing, which is not a method of package sdk",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("docRuleProblems differs (-want +got):\n%s", diff)
	}
}

// docTestOps are test-only operations, one for each fact that a doc rule reads.
var docTestOps = []rest.Op{
	{
		ID: "test_doc_check", Method: "POST", Route: rest.TestPrefix + "check",
		ContainerOnly: true, KeyLevel: rest.EnvironmentLevel,
	},
	{
		ID: "test_doc_get", Method: "GET",
		Route: rest.TestPrefix + "v2/facts/{proj_id}/{env_id}/users/{user_id}",
		Facts: true, WaitsForSync: true, KeyLevel: rest.ProjectLevel,
	},
	{
		ID: "test_doc_list", Method: "GET",
		Route: rest.TestPrefix + "v2/facts/{proj_id}/{env_id}/users",
		Facts: true, KeyLevel: rest.EnvironmentLevel,
	},
	{
		ID: "test_doc_org", Method: "GET", Route: rest.TestPrefix + "projects",
		KeyLevel: rest.OrganizationLevel,
	},
}

// docTestRegistry returns rest.Default with docTestOps.
func docTestRegistry(t *testing.T) *rest.Registry {
	t.Helper()
	registry, err := rest.Default().WithTestOps(docTestOps...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

// TestDocRulesFromTheRegistry checks the rules that docRules derives from the facts of the
// operations a method calls, and that the pass fixture, whose methods say what the rules ask,
// passes them: a method that calls several operations needs the widest key level of them, waits
// for sync when one of them waits, and is container PDP only when one of them is, also when an
// operation without the fact comes after it.
func TestDocRulesFromTheRegistry(t *testing.T) {
	rules, problems := docRules(map[string][]string{
		"UsersService.Get": {"test_doc_get", "test_doc_list"},
		"Client.Check":     {"test_doc_check", "test_doc_list"},
	}, docTestRegistry(t))
	want := []docRule{
		{Method: "Client.Check", Phrase: "environment-level API key"},
		{Method: "Client.Check", Phrase: "container PDP only"},
		{Method: "Client.Check", Phrase: "waits for sync", Absent: true},
		{Method: "UsersService.Get", Phrase: "project-level API key"},
		{Method: "UsersService.Get", Phrase: "waits for sync"},
	}
	if diff := cmp.Diff(want, rules); diff != "" || problems != nil {
		t.Errorf("docRules returned the problems %q and rules that differ (-want +got):\n%s",
			problems, diff)
	}
	_, pkgs := readFixture(t, "pass", ".")
	if problems := docRuleProblems(pkgs[0], rules); problems != nil {
		t.Errorf("the pass fixture breaks the rules of the registry:\n%s",
			strings.Join(problems, "\n"))
	}
	if rules, problems := docRules(nil, docTestRegistry(t)); rules != nil || problems != nil {
		t.Errorf("docRules of no methods returned %v and %q, want neither", rules, problems)
	}
}

// TestDocRulesFromTheRegistryThatFail binds the methods of the pass fixture to operations whose
// facts their doc comments do not say: a method that waits for sync without the phrase, one
// with the phrase that does not wait, a key level that the comment does not name, and an
// operation that the registry does not hold.
func TestDocRulesFromTheRegistryThatFail(t *testing.T) {
	rules, problems := docRules(map[string][]string{
		"Client.Check":     {"test_doc_get"},
		"UsersService.Get": {"test_doc_org", "test_doc_list"},
		"UsersService.Put": {"test_doc_nothing"},
	}, docTestRegistry(t))
	_, pkgs := readFixture(t, "pass", ".")
	problems = append(problems, docRuleProblems(pkgs[0], rules)...)
	const dir = "testdata/doccheck/pass/"
	want := []string{
		"UsersService.Put calls the operation test_doc_nothing, which the operation registry " +
			"does not hold",
		dir + `sdk.go:36: the doc comment of Client.Check does not say "project-level API key"`,
		dir + `sdk.go:36: the doc comment of Client.Check does not say "waits for sync"`,
		dir + `sdk.go:45: the doc comment of UsersService.Get does not say ` +
			`"organization-level API key"`,
		dir + `sdk.go:45: the doc comment of UsersService.Get says "waits for sync", which it ` +
			"must not say",
	}
	if diff := cmp.Diff(want, problems); diff != "" {
		t.Errorf("the problems differ (-want +got):\n%s", diff)
	}
}

func TestDocIdentifierSentinel(t *testing.T) {
	for _, tc := range []struct {
		checked, declarations int
		want                  string
	}{
		{checked: 0, declarations: 0},
		{checked: 3, declarations: 3},
		{checked: 4, declarations: 3},
		{
			checked: 2, declarations: 3,
			want: "count sentinel: the doc comment check counted 2 exported identifiers, but " +
				"testdata/api-surface.txt lists 3 declarations; the check skipped some",
		},
	} {
		if got := identifierSentinel(tc.checked, tc.declarations); got != tc.want {
			t.Errorf("identifierSentinel(%d, %d) = %q, want %q", tc.checked, tc.declarations,
				got, tc.want)
		}
	}
}

func TestDocSurfaceDeclarations(t *testing.T) {
	for text, want := range map[string]int{
		"":                       0,
		"# header\n#\n":          0,
		"permit: func New()\n":   1,
		"permit: func New()":     1,
		"# a\n\nA\n  \nB\n# c\n": 2,
	} {
		if got := surfaceDeclarations(text); got != want {
			t.Errorf("surfaceDeclarations(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestDocReadPackageFailsWithoutAPackage(t *testing.T) {
	dir := t.TempDir()
	if _, err := readDocPackage(dir, "example.com/empty"); err == nil ||
		!strings.Contains(err.Error(), "reading the package in "+dir) {
		t.Errorf("readDocPackage of an empty directory returned %v, want an error that names "+
			"the directory", err)
	}
}
