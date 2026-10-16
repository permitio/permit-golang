package main

import (
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The kinds of a use of v1.
const (
	// kindImport is an import of a package of v1.
	kindImport = "import"
	// kindIdentifier is an identifier that names a declaration of v1.
	kindIdentifier = "identifier"
)

// use is a use of v1 that scan found.
type use struct {
	// file is the path of the Go file, with slashes, relative to the directory scanned.
	file string
	// line and column are the position of the import path or of the identifier; the column
	// counts bytes from 1.
	line, column int
	// kind is kindImport or kindIdentifier.
	kind string
	// v1 is the import path of an import, and for an identifier the v1 identifier it names:
	// <package>.<name> for a declaration of a package, <package>.<type>.<method> for a method,
	// and models.<type>.<name> for a method or a field of a value of a models type.
	v1 string
	// typ is the models type that the identifier v1 resolves through when no entry names it:
	// the type itself, a constant's type, the type that a constructor returns, or the type of
	// the value whose method or field v1 is; or "".
	typ string
}

// majorVersion is the path element that ends the module path of a major version from v2 on.
var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

// isV1Path reports whether path is the import path of a package of v1: v1's module path, or a
// path under it that does not start with a major version, such as /v2.
func isV1Path(path string) bool {
	rest, ok := strings.CutPrefix(path, v1Module)
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	rest, ok = strings.CutPrefix(rest, "/")
	first, _, _ := strings.Cut(rest, "/")
	return ok && !majorVersion.MatchString(first)
}

// errNotModelled is the error of an import of a package that the scanner does not type-check.
var errNotModelled = errors.New("the scanner loads only the packages of v1 that it models")

// v1Importer imports the packages that newV1Packages built, and fails to import any other
// package: go/types then gives the code an empty package with the same path, and still records
// what each name of the package refers to.
type v1Importer map[string]*types.Package

// Import returns the package of v1 with the path, or an error.
func (imp v1Importer) Import(path string) (*types.Package, error) {
	if pkg, ok := imp[path]; ok {
		return pkg, nil
	}
	return nil, errNotModelled
}

// scan returns the uses of v1 in the Go files of fsys, sorted by file and position. It reads
// each directory but those that the go command leaves out of ./...: vendor, testdata, and
// directories whose names start with . or _; and each file whose name ends in .go, but those
// whose names start with . or _. It type-checks the files of each package of a directory, test
// files with a package of their own apart, against v1, the packages that newV1Packages built, so
// that it follows a call through a value of a v1 type to the v1 method it reaches, and finds the
// methods and fields of a value of a v1 models type. It imports no other package, so the value
// of an expression that another package declares has no type, and a call on it is not found. A
// file that cannot be read or parsed stops it.
func scan(fsys fs.FS, v1 map[string]*types.Package) ([]use, error) {
	files := map[string][]string{}
	var dirs []string
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("read the directory %s: %w", name, err)
		}
		base := d.Name()
		skipped := strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_")
		switch {
		case name == ".":
		case d.IsDir() && (skipped || base == "vendor" || base == "testdata"):
			return fs.SkipDir
		case !d.IsDir() && !skipped && strings.HasSuffix(base, ".go"):
			dir := path.Dir(name)
			if files[dir] == nil {
				dirs = append(dirs, dir)
			}
			files[dir] = append(files[dir], name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var uses []use
	for _, dir := range dirs {
		found, err := scanDir(fsys, files[dir], v1)
		if err != nil {
			return nil, err
		}
		uses = append(uses, found...)
	}
	slices.SortFunc(uses, func(a, b use) int {
		return cmp.Or(strings.Compare(a.file, b.file), cmp.Compare(a.line, b.line),
			cmp.Compare(a.column, b.column), strings.Compare(a.v1, b.v1))
	})
	return uses, nil
}

// scanDir returns the uses of v1 in names, the Go files of one directory.
func scanDir(fsys fs.FS, names []string, v1 map[string]*types.Package) ([]use, error) {
	fset := token.NewFileSet()
	packages := map[string][]*ast.File{}
	var order []string
	for _, name := range names {
		src, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		pkg := file.Name.Name
		if packages[pkg] == nil {
			order = append(order, pkg)
		}
		packages[pkg] = append(packages[pkg], file)
	}
	var uses []use
	for _, pkg := range order {
		info := &types.Info{
			Types:      map[ast.Expr]types.TypeAndValue{},
			Uses:       map[*ast.Ident]types.Object{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
		}
		conf := types.Config{Importer: v1Importer(v1), Error: func(error) {}}
		// The code imports packages that the scanner does not load, so type errors are
		// expected: Error collects them, and the check goes on and records what it can.
		_, _ = conf.Check(pkg, fset, packages[pkg], info)
		for _, file := range packages[pkg] {
			uses = append(uses, fileUses(fset, file, info)...)
		}
	}
	return uses, nil
}

// fileUses returns the uses of v1 in file, whose package info describes.
func fileUses(fset *token.FileSet, file *ast.File, info *types.Info) []use {
	var uses []use
	add := func(pos token.Pos, kind, v1, typ string) {
		p := fset.Position(pos)
		uses = append(uses, use{
			file: p.Filename, line: p.Line, column: p.Column, kind: kind,
			v1: v1, typ: typ,
		})
	}
	// selectors are the identifiers after the dot of a selector, which the selector names.
	selectors := map[*ast.Ident]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.ImportSpec:
			if path, err := strconv.Unquote(node.Path.Value); err == nil && isV1Path(path) {
				add(node.Path.Pos(), kindImport, path, "")
			}
		case *ast.SelectorExpr:
			selectors[node.Sel] = true
			if v1, typ := selected(node, info); v1 != "" {
				add(node.Sel.Pos(), kindIdentifier, v1, typ)
			}
		case *ast.Ident:
			if v1, typ := dotImported(node, info); v1 != "" && !selectors[node] {
				add(node.Pos(), kindIdentifier, v1, typ)
			}
		}
		return true
	})
	return uses
}

// selected returns the v1 identifier that sel names, and the models type it resolves through
// (use.typ), or "" for each: a name of a package of v1, qualified with the name its file imports
// the package under; a method of a v1 type; or a method or a field of a value of a models type.
// A models type has neither in the model, so go/types records no selection for them.
func selected(sel *ast.SelectorExpr, info *types.Info) (v1, typ string) {
	if x, ok := sel.X.(*ast.Ident); ok {
		if pkgName, ok := info.Uses[x].(*types.PkgName); ok {
			if imported := pkgName.Imported(); isV1Path(imported.Path()) {
				return imported.Name() + "." + sel.Sel.Name, modelsTypeOf(info.Uses[sel.Sel])
			}
			return "", ""
		}
	}
	selection, ok := info.Selections[sel]
	if !ok {
		if name := modelsType(info.TypeOf(sel.X)); name != "" {
			return name + "." + sel.Sel.Name, name
		}
		return "", ""
	}
	// The methods of the universe, such as Error of error, have no package.
	method, ok := selection.Obj().(*types.Func)
	if !ok || method.Pkg() == nil || !isV1Path(method.Pkg().Path()) {
		return "", ""
	}
	recv, ok := method.Signature().Recv().Type().(*types.Named)
	if !ok {
		return "", ""
	}
	return method.Pkg().Name() + "." + recv.Obj().Name() + "." + method.Name(), ""
}

// dotImported returns the v1 identifier that id names when it refers to a declaration of a
// package of v1 without a qualifier, as in a file that imports the package with a dot, and the
// models type it resolves through (use.typ); or "" for each.
func dotImported(id *ast.Ident, info *types.Info) (v1, typ string) {
	obj := info.Uses[id]
	if obj == nil || obj.Pkg() == nil || !isV1Path(obj.Pkg().Path()) ||
		obj.Parent() != obj.Pkg().Scope() {
		return "", ""
	}
	return obj.Pkg().Name() + "." + obj.Name(), modelsTypeOf(obj)
}

// modelsTypeOf returns the models type that obj, a declaration of v1, resolves through: a type
// of models itself, the type of a constant of models, or the type that a constructor returns;
// or "".
func modelsTypeOf(obj types.Object) string {
	if obj == nil {
		return ""
	}
	typ := obj.Type()
	if sig, ok := typ.(*types.Signature); ok && sig.Results().Len() == 1 {
		typ = sig.Results().At(0).Type()
	}
	return modelsType(typ)
}

// modelsType returns models.<name> when typ is a type of v1's models, or a pointer to one, and
// "" otherwise.
func modelsType(typ types.Type) string {
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	named, ok := typ.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != modelsPath {
		return ""
	}
	return "models." + named.Obj().Name()
}
