// Package surface lists the exported API of a Go package from its source: its constants,
// variables, types with their exported fields and interface methods, functions and methods, each
// as one normalized line. The SDK's tests use it to keep the public surface in
// testdata/api-surface.txt and to check every public method against the wire cases; nothing else
// imports it.
//
// Read parses the Go files of the package but the tests and the files that //go:build ignore
// leaves out. A file that another build constraint applies to, such as p_linux.go or a file with
// //go:build cgo, is an error, so that the list is the same on every machine. Read associates
// methods with their types through go/doc, and a struct also lists the methods it promotes from
// the types of its own package that it embeds, structs and interfaces, exported or not. It cannot
// list the methods that a struct promotes from a type of another package or from error;
// Item.Unlisted names those types.
//
// An exported struct that embeds an unexported type with exported fields is an error, since the
// list would show neither the type nor the fields: embed an exported type, or declare the fields
// on the struct. So is a declaration that names an unexported alias of a type that mentions
// another package, which would hide that package from Item.Uses: use the type itself.
//
// A line keeps the parameter names and writes each type as the source does, after the package
// name of its import, such as *models.User. A parameter list that shares a type, such as
// (a, b string), becomes one parameter per name. A constant shows its value, with iota replaced by
// its value; a variable shows its type, or its value when it has no declared type. A type or value
// that does not fit on one line, such as an inline struct type with two fields, is an error:
// declare a named type for it.
package surface

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Kind says what an exported declaration is.
type Kind int

const (
	// The zero Kind is no kind, so that an Item that was never filled in has none.
	_ Kind = iota

	// Const is a constant.
	Const

	// Var is a variable.
	Var

	// Type is a defined type or an alias.
	Type

	// Member is an exported field or embedded type of a struct, or an exported method or embedded
	// element of an interface.
	Member

	// Func is a function.
	Func

	// Method is a method, also one that a struct promotes from a type it embeds.
	Method
)

// Param is a parameter or result of a function or method.
type Param struct {
	// Name is the parameter's name, or empty when the signature does not name it.
	Name string

	// Type is the parameter's type as the source writes it, such as *models.User or ...Option.
	Type string
}

// Item is one exported declaration.
type Item struct {
	// Kind says what the declaration is.
	Kind Kind

	// Recv is the name of the type that a method or member belongs to, without a * or type
	// parameters, such as Client. It is empty for the other kinds.
	Recv string

	// Name is the declared name. For an embedded type it is the type's name, and for an
	// interface's type-set element, such as ~int | ~string, it is empty.
	Name string

	// Params holds the parameters of a function or method in order.
	Params []Param

	// Uses holds the import paths of the packages that Line mentions, sorted, or nil.
	Uses []string

	// Unlisted holds, for a struct type, the types whose methods the struct promotes but Read
	// cannot list, sorted, or nil: the types of other packages, such as models.Base, and error,
	// which the struct embeds itself or through the types of its own package that it embeds.
	Unlisted []string

	// Line is the declaration on one line. Lines start with const, var, type, func or method;
	// a member's line starts with its type's line and a comma, such as
	// "type Client struct, Users *UsersService".
	Line string
}

// Package is the exported API of one package.
type Package struct {
	// Name is the package name, such as permit.
	Name string

	// Items holds the exported declarations, sorted by Line.
	Items []Item
}

// Read lists the exported API of the Go package in dir. It returns an error when dir has no Go
// file to build outside the tests, when a file has a build constraint other than //go:build
// ignore, when its files do not parse or hold more than one package, when a declaration cannot be
// written on one line, and in the cases that the package documentation names.
func Read(dir string) (*Package, error) {
	fset := token.NewFileSet()
	files, err := parseFiles(fset, dir)
	if err != nil {
		return nil, err
	}
	l := &lister{
		fset:    fset,
		imports: make(map[*token.File]map[string]string),
		types:   make(map[string]*ast.TypeSpec),
		uses:    make(map[string]bool),
	}
	name := files[0].Name.Name
	for _, f := range files {
		if f.Name.Name != name {
			return nil, fmt.Errorf(
				"surface: %s holds package %s and package %s; give the directory of one package",
				dir, name, f.Name.Name)
		}
		l.readImports(f)
	}
	if l.err != nil {
		return nil, l.err
	}
	p, err := doc.NewFromFiles(fset, files, name, doc.AllDecls|doc.AllMethods)
	if err != nil {
		return nil, fmt.Errorf("surface: reading the declarations of %s: %w", dir, err)
	}
	l.list(p)
	if l.err != nil {
		return nil, l.err
	}
	slices.SortFunc(l.items, func(a, b Item) int { return strings.Compare(a.Line, b.Line) })
	return &Package{Name: name, Items: l.items}, nil
}

// parseFiles parses the Go files in dir, without the tests and the files that //go:build ignore
// leaves out, in the order of their names.
func parseFiles(fset *token.FileSet, dir string) ([]*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("surface: %w", err)
	}
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		// The go command leaves out the files whose names start with _ or a dot.
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		ignored, err := buildIgnored(dir, name)
		if err != nil {
			return nil, err
		}
		if ignored {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("surface: %w", err)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("surface: %s has no Go file to build outside the tests; give "+
			"the directory of a package", dir)
	}
	return files, nil
}

// anyMachine is a build context of no operating system and no architecture that reads every
// file as a bare package clause: it builds a Go file unless the file's name, such as p_linux.go
// or p_arm64.go, names an operating system or an architecture.
var anyMachine = build.Context{
	OpenFile: func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("package p\n")), nil
	},
}

// buildIgnored reports whether //go:build ignore leaves out the file name in dir, and returns an
// error when the file's name or another build constraint makes whether a build includes the file
// depend on the machine or the toolchain.
func buildIgnored(dir, name string) (bool, error) {
	path := filepath.Join(dir, name)
	plain, err := anyMachine.MatchFile(dir, name)
	if err != nil {
		return false, fmt.Errorf("surface: %w", err)
	}
	if !plain {
		return false, fmt.Errorf("surface: %s: the file name names an operating system or an "+
			"architecture, so whether a build includes the file depends on the machine; move its "+
			"declarations to a file that every machine builds", path)
	}
	fset := token.NewFileSet()
	header, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return false, fmt.Errorf("surface: %w", err)
	}
	ignored := false
	for _, group := range header.Comments {
		for _, c := range group.List {
			if !constraint.IsGoBuild(c.Text) && !constraint.IsPlusBuild(c.Text) {
				continue
			}
			expr, err := constraint.Parse(c.Text)
			if err != nil {
				return false, fmt.Errorf("surface: %s: %w", fset.Position(c.Pos()), err)
			}
			if expr.String() != "ignore" {
				return false, fmt.Errorf("surface: %s: %s makes whether a build includes the "+
					"file depend on the machine or the toolchain; move its declarations to a file "+
					"without a build constraint, or leave the file out with //go:build ignore",
					fset.Position(c.Pos()), c.Text)
			}
			ignored = true
		}
	}
	return ignored, nil
}

// lister collects the items of a package. Its methods record the first error in err and go on,
// so that the caller checks once.
type lister struct {
	fset *token.FileSet

	// imports maps each file to its import names and the paths they stand for.
	imports map[*token.File]map[string]string

	// types maps the name of each type that the package declares, exported or not, to its
	// declaration.
	types map[string]*ast.TypeSpec

	// uses collects the import paths of the item being built; add moves them into the item.
	uses map[string]bool

	items []Item
	err   error
}

// fail records an error at pos unless one is recorded already.
func (l *lister) fail(pos token.Pos, format string, args ...any) {
	if l.err == nil {
		l.err = fmt.Errorf("surface: %s: %s", l.fset.Position(pos), fmt.Sprintf(format, args...))
	}
}

// readImports records the import names of f. An import without a name is known by the last
// element of its path.
func (l *lister) readImports(f *ast.File) {
	names := make(map[string]string)
	for _, spec := range f.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			l.fail(spec.Pos(), "import path %s: %v", spec.Path.Value, err)
			return
		}
		name := path.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "." {
			l.fail(spec.Pos(), "%s is a dot import, which hides the package of the names it "+
				"brings in; import it with a name", spec.Path.Value)
		}
		names[name] = importPath
	}
	l.imports[l.fset.File(f.Pos())] = names
}

// list collects the exported declarations of p.
func (l *lister) list(p *doc.Package) {
	for _, t := range p.Types {
		if spec, ok := t.Decl.Specs[0].(*ast.TypeSpec); ok {
			l.types[t.Name] = spec
		}
	}
	l.values(p.Consts)
	l.values(p.Vars)
	l.funcs(p.Funcs)
	for _, t := range p.Types {
		l.values(t.Consts)
		l.values(t.Vars)
		l.funcs(t.Funcs)
		if token.IsExported(t.Name) {
			l.typ(t)
			l.methods(t)
		}
	}
}

// add appends item with the import paths collected since the last add.
func (l *lister) add(item Item) {
	item.Uses = slices.Sorted(maps.Keys(l.uses))
	clear(l.uses)
	l.items = append(l.items, item)
}

// valueSpec is one spec of a constant or variable declaration, with the type and values that
// apply to it: a constant spec without a type or values repeats those of the spec before it.
type valueSpec struct {
	tok    token.Token
	names  []*ast.Ident
	typ    ast.Expr
	values []ast.Expr

	// index is the spec's index in its declaration, the value of iota in a constant spec.
	index int
}

// values collects the exported constants or variables of each declaration in vs.
func (l *lister) values(vs []*doc.Value) {
	for _, v := range vs {
		s := valueSpec{tok: v.Decl.Tok}
		for i, spec := range v.Decl.Specs {
			vspec, ok := spec.(*ast.ValueSpec)
			if !ok {
				l.fail(spec.Pos(), "a %s declaration holds a %T", v.Decl.Tok, spec)
				return
			}
			s.names, s.index = vspec.Names, i
			// Only a constant spec can lack both; it repeats those of the spec before it.
			if vspec.Type != nil || vspec.Values != nil {
				s.typ, s.values = vspec.Type, vspec.Values
			}
			for j, name := range s.names {
				if name.IsExported() {
					l.value(s, j)
				}
			}
		}
	}
}

// value collects the index-th name of s.
func (l *lister) value(s valueSpec, index int) {
	name := s.names[index]
	kind := Const
	if s.tok == token.VAR {
		kind = Var
	}
	line := s.tok.String() + " " + name.Name
	if s.typ != nil {
		line += " " + l.text(s.typ)
	}
	// A variable with a declared type shows only the type: its value is set when the program runs.
	if kind == Const || s.typ == nil {
		if len(s.values) != len(s.names) {
			l.fail(name.Pos(), "%s %s has no value of its own; declare its type", s.tok,
				name.Name)
			return
		}
		line += " = " + l.valueText(s.values[index], s.index)
	}
	l.add(Item{Kind: kind, Name: name.Name, Line: line})
}

// typ collects the exported type t and its exported members.
func (l *lister) typ(t *doc.Type) {
	spec, ok := t.Decl.Specs[0].(*ast.TypeSpec)
	if !ok {
		l.fail(t.Decl.Pos(), "type %s is declared by a %T", t.Name, t.Decl.Specs[0])
		return
	}
	line := "type " + t.Name + l.typeParams(spec.TypeParams)
	if spec.Assign.IsValid() {
		l.add(Item{Kind: Type, Name: t.Name, Line: line + " = " + l.text(spec.Type)})
		return
	}
	switch typ := spec.Type.(type) {
	case *ast.StructType:
		line += " struct"
		e := l.embeds(typ)
		l.add(Item{Kind: Type, Name: t.Name, Unlisted: e.unlisted, Line: line})
		l.fields(t.Name, line+", ", typ.Fields)
		l.promoted(t, e.methods)
	case *ast.InterfaceType:
		line += " interface"
		l.add(Item{Kind: Type, Name: t.Name, Line: line})
		l.interfaceMembers(t.Name, line+", ", typ.Methods)
	default:
		l.add(Item{Kind: Type, Name: t.Name, Line: line + " " + l.text(spec.Type)})
	}
}

// fields collects the exported fields and embedded types of the struct named recv, whose lines
// start with prefix. An embedded unexported type that gives the struct exported fields is an
// error.
func (l *lister) fields(recv, prefix string, fields *ast.FieldList) {
	for _, field := range fields.List {
		if field.Names == nil {
			name := baseName(field.Type)
			if token.IsExported(name) {
				l.add(Item{
					Kind: Member, Recv: recv, Name: name,
					Line: prefix + "embedded " + l.text(field.Type),
				})
				continue
			}
			l.checkAliases(field.Type)
			if l.promotesFields(field.Type, make(map[string]bool)) {
				l.fail(field.Pos(), "%s embeds the unexported type %s, which has exported "+
					"fields; embed an exported type or declare the fields on %s", recv, name, recv)
			}
			continue
		}
		for _, name := range field.Names {
			if name.IsExported() {
				l.add(Item{
					Kind: Member, Recv: recv, Name: name.Name,
					Line: prefix + name.Name + " " + l.text(field.Type),
				})
			}
		}
	}
}

// promotesFields reports whether embedding the type that expr names gives a struct exported
// fields: a struct type of the package with an exported field or embedded type, or one that
// embeds such a type in turn. A type of another package counts as one, since Read cannot see its
// fields.
func (l *lister) promotesFields(expr ast.Expr, seen map[string]bool) bool {
	pkg, name := typeName(expr)
	if pkg != "" {
		return true
	}
	spec := l.types[name]
	if spec == nil || seen[name] {
		return false
	}
	seen[name] = true
	st, ok := spec.Type.(*ast.StructType)
	if !ok {
		// An alias, or a type defined as another type, has the fields of that type.
		return l.promotesFields(spec.Type, seen)
	}
	for _, field := range st.Fields.List {
		if slices.ContainsFunc(field.Names, (*ast.Ident).IsExported) {
			return true
		}
		if field.Names == nil &&
			(token.IsExported(baseName(field.Type)) || l.promotesFields(field.Type, seen)) {
			return true
		}
	}
	return false
}

// embedding is what a struct gets from the types it embeds that go/doc does not list.
type embedding struct {
	// seen holds the types of the package that the walk has reached.
	seen map[string]bool

	// methods holds the exported methods of the interface types of the package that the struct
	// embeds, itself or through the types it embeds.
	methods []*ast.Field

	// unlisted holds the types whose methods the struct promotes but Read cannot list.
	unlisted []string
}

// embeds walks the types that the struct st embeds, and the types that those embed in turn,
// through the types of the package.
func (l *lister) embeds(st *ast.StructType) embedding {
	e := embedding{seen: make(map[string]bool)}
	for _, field := range st.Fields.List {
		if field.Names == nil {
			l.embed(&e, field.Type)
		}
	}
	slices.Sort(e.unlisted)
	e.unlisted = slices.Compact(e.unlisted)
	return e
}

// embed adds to e what a struct gets from embedding the type that expr names.
func (l *lister) embed(e *embedding, expr ast.Expr) {
	pkg, name := typeName(expr)
	if pkg != "" {
		e.unlisted = append(e.unlisted, pkg+"."+name)
		return
	}
	spec := l.types[name]
	if spec == nil {
		// Of the predeclared types, only error has a method.
		if name == "error" {
			e.unlisted = append(e.unlisted, name)
		}
		return
	}
	if e.seen[name] {
		return
	}
	e.seen[name] = true
	switch typ := spec.Type.(type) {
	case *ast.StructType:
		for _, field := range typ.Fields.List {
			if field.Names == nil {
				l.embed(e, field.Type)
			}
		}
	case *ast.InterfaceType:
		for _, field := range typ.Methods.List {
			if field.Names == nil {
				l.embed(e, field.Type)
			} else if field.Names[0].IsExported() {
				e.methods = append(e.methods, field)
			}
		}
	default:
		// An alias stands for the type it names, and a type defined as another type has the
		// fields and the interface methods of that type.
		l.embed(e, spec.Type)
	}
}

// promoted collects the methods in methods, of the interfaces that the struct t embeds, that t
// neither declares nor promotes from a struct; a method that two interfaces declare is listed
// once.
func (l *lister) promoted(t *doc.Type, methods []*ast.Field) {
	listed := make(map[string]bool)
	for _, m := range t.Methods {
		listed[m.Name] = true
	}
	for _, field := range methods {
		name := field.Names[0].Name
		ft, ok := field.Type.(*ast.FuncType)
		if !ok || listed[name] {
			continue
		}
		listed[name] = true
		params, sig := l.signature(ft)
		l.add(Item{
			Kind: Method, Recv: t.Name, Name: name, Params: params,
			Line: "method (" + t.Name + ") " + name + sig,
		})
	}
}

// interfaceMembers collects the exported methods and the embedded elements of the interface named
// recv, whose lines start with prefix.
func (l *lister) interfaceMembers(recv, prefix string, methods *ast.FieldList) {
	for _, field := range methods.List {
		if field.Names == nil {
			l.add(Item{
				Kind: Member, Recv: recv, Name: baseName(field.Type),
				Line: prefix + l.text(field.Type),
			})
			continue
		}
		name := field.Names[0]
		ft, ok := field.Type.(*ast.FuncType)
		if !ok || !name.IsExported() {
			continue
		}
		_, sig := l.signature(ft)
		l.add(Item{Kind: Member, Recv: recv, Name: name.Name, Line: prefix + name.Name + sig})
	}
}

// funcs collects the exported functions in fs.
func (l *lister) funcs(fs []*doc.Func) {
	for _, f := range fs {
		if !token.IsExported(f.Name) {
			continue
		}
		if f.Decl.Recv != nil {
			l.fail(f.Decl.Pos(), "method %s has a receiver type that the package does not declare",
				f.Name)
			continue
		}
		params, sig := l.signature(f.Decl.Type)
		l.add(Item{
			Kind: Func, Name: f.Name, Params: params,
			Line: "func " + f.Name + l.typeParams(f.Decl.Type.TypeParams) + sig,
		})
	}
}

// methods collects the exported methods of the type t.
func (l *lister) methods(t *doc.Type) {
	for _, m := range t.Methods {
		if !token.IsExported(m.Name) {
			continue
		}
		recv := l.text(m.Decl.Recv.List[0].Type)
		params, sig := l.signature(m.Decl.Type)
		l.add(Item{
			Kind: Method, Recv: t.Name, Name: m.Name, Params: params,
			Line: "method (" + recv + ") " + m.Name + sig,
		})
	}
}

// signature returns the parameters of ft and its parameter and result lists as a line writes
// them, such as (ctx context.Context, key string) (*User, error).
func (l *lister) signature(ft *ast.FuncType) ([]Param, string) {
	params := l.params(ft.Params)
	results := l.params(ft.Results)
	sig := "(" + joinParams(params) + ")"
	if len(results) == 1 && results[0].Name == "" {
		sig += " " + results[0].Type
	} else if results != nil {
		sig += " (" + joinParams(results) + ")"
	}
	return params, sig
}

// typeParams returns the type parameter list fl as a line writes it, such as [K comparable, V any],
// or "" when fl is nil.
func (l *lister) typeParams(fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}
	return "[" + joinParams(l.params(fl)) + "]"
}

// params returns the parameters of fl, one for each name, or nil when fl is empty.
func (l *lister) params(fl *ast.FieldList) []Param {
	if fl == nil {
		return nil
	}
	var params []Param
	for _, field := range fl.List {
		typ := l.text(field.Type)
		if field.Names == nil {
			params = append(params, Param{Type: typ})
		}
		for _, name := range field.Names {
			params = append(params, Param{Name: name.Name, Type: typ})
		}
	}
	return params
}

// joinParams writes params as a parameter list writes them, without the parentheses.
func joinParams(params []Param) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = strings.TrimSpace(p.Name + " " + p.Type)
	}
	return strings.Join(parts, ", ")
}

// valueText writes the value expr of a constant or variable as text does, with each iota written
// as iotaValue. iota appears only in constants, where it is the index of the spec.
func (l *lister) valueText(expr ast.Expr, iotaValue int) string {
	var renamed []*ast.Ident
	ast.Inspect(expr, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "iota" {
			renamed = append(renamed, id)
		}
		return true
	})
	for _, id := range renamed {
		id.Name = strconv.Itoa(iotaValue)
	}
	text := l.text(expr)
	for _, id := range renamed {
		id.Name = "iota"
	}
	return text
}

// text writes expr on one line and records the imports it names.
func (l *lister) text(expr ast.Expr) string {
	ast.Inspect(expr, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			l.use(sel)
		}
		return true
	})
	l.checkAliases(expr)
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), expr); err != nil {
		l.fail(expr.Pos(), "writing the expression: %v", err)
		return ""
	}
	first, _, multiline := strings.Cut(b.String(), "\n")
	if multiline {
		l.fail(expr.Pos(), "%s ... does not fit on one line; declare a named type for an "+
			"inline struct or interface type, and write a value on one line", first)
		return ""
	}
	return first
}

// use records the import that the qualified name sel names, if any.
func (l *lister) use(sel *ast.SelectorExpr) {
	if importPath := l.importPath(sel); importPath != "" {
		l.uses[importPath] = true
	}
}

// importPath returns the import path of the package that the qualified name sel names, or ""
// when sel names no import of its file, such as a field of a variable.
func (l *lister) importPath(sel *ast.SelectorExpr) string {
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return l.imports[l.fset.File(sel.Pos())][x.Name]
}

// checkAliases fails when expr names an unexported alias of a type that mentions another
// package: a line that names the alias would hide that package from Uses.
func (l *lister) checkAliases(expr ast.Expr) {
	ast.Inspect(expr, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if ok && !token.IsExported(id.Name) && l.aliasesImport(id.Name, make(map[string]bool)) {
			l.fail(id.Pos(), "%s is an unexported alias of a type that mentions another package, "+
				"which the list would not show; use the type itself", id.Name)
		}
		return true
	})
}

// aliasesImport reports whether name is an alias, declared in the package, of a type that
// mentions another package directly or through another alias, such as pkg in
// type pkg = surface.Package.
func (l *lister) aliasesImport(name string, seen map[string]bool) bool {
	spec := l.types[name]
	if spec == nil || !spec.Assign.IsValid() || seen[name] {
		return false
	}
	seen[name] = true
	found := false
	ast.Inspect(spec.Type, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && l.importPath(sel) != "" {
			found = true
		}
		if id, ok := n.(*ast.Ident); ok && l.aliasesImport(id.Name, seen) {
			found = true
		}
		return !found
	})
	return found
}

// typeName returns the package and the name of the type that expr names, after a * and type
// arguments, such as models and User for *models.User. The package is empty for a type of the
// package itself or a predeclared type, and both are empty when expr does not name one type.
func typeName(expr ast.Expr) (pkg, name string) {
	switch e := expr.(type) {
	case *ast.Ident:
		return "", e.Name
	case *ast.StarExpr:
		return typeName(e.X)
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok {
			return x.Name, e.Sel.Name
		}
	case *ast.IndexExpr:
		return typeName(e.X)
	case *ast.IndexListExpr:
		return typeName(e.X)
	}
	return "", ""
}

// baseName returns the name of the type that expr names, without a package, * or type
// arguments, or "" when expr does not name one type.
func baseName(expr ast.Expr) string {
	_, name := typeName(expr)
	return name
}
