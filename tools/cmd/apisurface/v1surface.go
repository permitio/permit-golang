package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Blocks of the v1 surface. Every item belongs to one.
const (
	// blockIdentifiers holds the exported identifiers of the hand-written packages.
	blockIdentifiers = "a"
	// blockOperations holds the operations of the generated client that only it sends.
	blockOperations = "b"
	// blockModels holds the types of pkg/models.
	blockModels = "c"
)

// identifierDirs are the directories of the v1 packages whose exported identifiers make block a.
var identifierDirs = []string{
	"pkg/api",
	"pkg/config",
	"pkg/enforcement",
	"pkg/errors",
	"pkg/permit",
}

const (
	// openapiDir holds v1's generated client, whose operations make block b.
	openapiDir = "pkg/openapi"
	// modelsDir holds v1's models, whose types make block c.
	modelsDir = "pkg/models"
	// clientType is the type of the generated client, whose fields are its services.
	clientType = "APIClient"
)

// Details of a block b item: whether the API spec has the operation's route.
const (
	inSpec    = "in-spec"
	notInSpec = "not-in-spec"
)

// item is one line of the v1 surface: an identifier, an operation or a models type.
type item struct {
	// block is blockIdentifiers, blockOperations or blockModels.
	block string
	// id names the item, such as api.Users.Create, and is what MIGRATION.md and pending.txt
	// refer to.
	id string
	// details is the rest of the line: the kind of an identifier or type, or the route of an
	// operation and whether the spec has it.
	details string
}

// line returns the item as a line of the v1 surface, without the newline.
func (it item) line() string {
	return it.block + " " + it.id + " " + it.details
}

// sortItems sorts items by block, then id.
func sortItems(items []item) {
	slices.SortFunc(items, func(x, y item) int {
		return cmp.Or(strings.Compare(x.block, y.block), strings.Compare(x.id, y.id))
	})
}

// sourceFile is a Go file of the v1 module: its slash path in the module and its content.
type sourceFile struct {
	name    string
	content []byte
}

// parsedPackage is a v1 package: its name and its parsed files, which exclude the tests.
type parsedPackage struct {
	dir   string
	name  string
	files []*ast.File
}

// parsePackage parses the Go files of the module that are directly in dir, but its tests. It
// fails when dir has none, when a file does not parse or has a build constraint, or when the
// files declare more than one package: the surface must not depend on the machine.
func parsePackage(fset *token.FileSet, files []sourceFile, dir string) (parsedPackage, error) {
	p := parsedPackage{dir: dir}
	for _, f := range files {
		if path.Dir(f.name) != dir || path.Ext(f.name) != ".go" ||
			strings.HasSuffix(f.name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f.name, f.content,
			parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return parsedPackage{}, fmt.Errorf("parsing %s: %w", f.name, err)
		}
		if constraint := buildConstraint(file); constraint != "" {
			return parsedPackage{}, fmt.Errorf("%s has the build constraint %q, so its "+
				"identifiers would depend on the machine", f.name, constraint)
		}
		if p.name == "" {
			p.name = file.Name.Name
		} else if file.Name.Name != p.name {
			return parsedPackage{}, fmt.Errorf("%s declares package %s and package %s", dir,
				p.name, file.Name.Name)
		}
		p.files = append(p.files, file)
	}
	if len(p.files) == 0 {
		return parsedPackage{}, fmt.Errorf("the module has no Go file in %s", dir)
	}
	return p, nil
}

// buildConstraint returns the text of the file's //go:build or // +build line before its package
// clause, or "" when it has none.
func buildConstraint(file *ast.File) string {
	for _, group := range file.Comments {
		if group.Pos() >= file.Package {
			break
		}
		for _, c := range group.List {
			if strings.HasPrefix(c.Text, "//go:build") || strings.HasPrefix(c.Text, "// +build") {
				return c.Text
			}
		}
	}
	return ""
}

// identifiers returns block a's items of a package: each exported type, function, constant and
// variable as <package>.<name>, and each exported method as <package>.<type>.<name>, with its
// kind as the details.
func identifiers(p parsedPackage) []item {
	var items []item
	add := func(id, kind string) {
		items = append(items, item{block: blockIdentifiers, id: p.name + "." + id, details: kind})
	}
	for _, file := range p.files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if !decl.Name.IsExported() {
					continue
				}
				if decl.Recv == nil {
					add(decl.Name.Name, "func")
					continue
				}
				add(receiverName(decl.Recv.List[0].Type)+"."+decl.Name.Name, "method")
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						if spec.Name.IsExported() {
							add(spec.Name.Name, "type")
						}
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							if name.IsExported() {
								add(name.Name, decl.Tok.String())
							}
						}
					}
				}
			}
		}
	}
	return items
}

// receiverName returns the name of a method's receiver type, without a * or type parameters.
func receiverName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return fmt.Sprintf("%T", expr)
		}
	}
}

// modelTypes returns block c's items: each exported type of the models package, as
// <package>.<name>.
func modelTypes(p parsedPackage) []item {
	var items []item
	for _, file := range p.files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
					items = append(items, item{
						block: blockModels, id: p.name + "." + ts.Name.Name, details: "type",
					})
				}
			}
		}
	}
	return items
}

// operation is an operation of the generated client: a method of one of its services that
// builds a request, with the Execute method that sends it.
type operation struct {
	// service is the type of the service, such as UsersApiService.
	service string
	// name is the name of the method that builds the request, such as CreateUser.
	name string
	// method and route are the HTTP method and the path template that Execute sends.
	method, route string
}

// operations returns the operations of the generated client: one for each <name>Execute method
// of a service, with the HTTP method and the route its body sets. It fails when an Execute
// method sets no method or route, or when its service has no <name> method to build the request.
func operations(p parsedPackage) ([]operation, error) {
	builders := map[string]bool{}
	var ops []operation
	var problems []string
	for _, file := range p.files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			service := receiverName(fn.Recv.List[0].Type)
			if !strings.HasSuffix(service, "Service") {
				continue
			}
			name, isExecute := strings.CutSuffix(fn.Name.Name, "Execute")
			if !isExecute || name == "" {
				builders[service+"."+fn.Name.Name] = true
				continue
			}
			method, route := requestRoute(fn.Body)
			if method == "" || route == "" {
				problems = append(problems, fmt.Sprintf("%s.%s sets no HTTP method or no "+
					"route", service, fn.Name.Name))
				continue
			}
			ops = append(ops, operation{service: service, name: name, method: method, route: route})
		}
	}
	for _, op := range ops {
		if !builders[op.service+"."+op.name] {
			problems = append(problems, fmt.Sprintf("%s.%sExecute has no %s method that builds "+
				"its request", op.service, op.name, op.name))
		}
	}
	if len(problems) > 0 {
		slices.Sort(problems)
		return nil, fmt.Errorf("reading the operations of %s: %s", p.dir,
			strings.Join(problems, "; "))
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("%s has no operation: no service has an Execute method", p.dir)
	}
	return ops, nil
}

// requestRoute returns the HTTP method and the route that an Execute method of the generated
// client sends: the value of localVarHTTPMethod, such as http.MethodPost, and the string that
// localVarPath appends to localBasePath. It returns "" for what the body does not set.
func requestRoute(body *ast.BlockStmt) (method, route string) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ValueSpec:
			for i, name := range n.Names {
				if name.Name != "localVarHTTPMethod" || i >= len(n.Values) {
					continue
				}
				if sel, ok := n.Values[i].(*ast.SelectorExpr); ok {
					if m, ok := strings.CutPrefix(sel.Sel.Name, "Method"); ok {
						method = strings.ToUpper(m)
					}
				}
			}
		case *ast.AssignStmt:
			if len(n.Lhs) != 1 || len(n.Rhs) != 1 || n.Tok != token.DEFINE {
				return true
			}
			if id, ok := n.Lhs[0].(*ast.Ident); !ok || id.Name != "localVarPath" {
				return true
			}
			if bin, ok := n.Rhs[0].(*ast.BinaryExpr); ok && bin.Op == token.ADD {
				if lit, ok := bin.Y.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil {
						route = s
					}
				}
			}
		}
		return true
	})
	return method, route
}

// serviceFields returns the services of the generated client: a map from the name of each field
// of APIClient whose type is a pointer to a service to that service's type.
func serviceFields(p parsedPackage) (map[string]string, error) {
	fields := map[string]string{}
	for _, file := range p.files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name.Name != clientType {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					return nil, fmt.Errorf("%s.%s is not a struct", p.name, clientType)
				}
				for _, field := range st.Fields.List {
					star, ok := field.Type.(*ast.StarExpr)
					if !ok {
						continue
					}
					typ, ok := star.X.(*ast.Ident)
					if !ok || !strings.HasSuffix(typ.Name, "Service") {
						continue
					}
					for _, name := range field.Names {
						fields[name.Name] = typ.Name
					}
				}
			}
		}
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("%s declares no %s with services", p.dir, clientType)
	}
	return fields, nil
}

// calledOperations returns the operations that the packages call through a field of the
// generated client, such as client.UsersApi.CreateUser or client.UsersApi.CreateUserExecute, as
// <service type>.<operation>.
func calledOperations(pkgs []parsedPackage, fields map[string]string) map[string]bool {
	called := map[string]bool{}
	for _, p := range pkgs {
		for _, file := range p.files {
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				inner, ok := sel.X.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if service, ok := fields[inner.Sel.Name]; ok {
					name := strings.TrimSuffix(sel.Sel.Name, "Execute")
					called[service+"."+name] = true
				}
				return true
			})
		}
	}
	return called
}

// specRoutes is the set of the API spec's operations, each as "<METHOD> <route>" with every
// path parameter written as {}.
type specRoutes map[string]bool

// specMethods are the keys of a path item that are operations.
var specMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// parseSpec reads the operations of an OpenAPI document. It fails when the document has none.
func parseSpec(content []byte) (specRoutes, error) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parsing the API spec: %w", err)
	}
	routes := specRoutes{}
	for route, item := range doc.Paths {
		for _, method := range specMethods {
			if _, ok := item[method]; ok {
				routes[strings.ToUpper(method)+" "+normalizeRoute(route)] = true
			}
		}
	}
	if len(routes) == 0 {
		return nil, errors.New("the API spec has no operation")
	}
	return routes, nil
}

// pathParam matches a path parameter of a route, such as {user_id}.
var pathParam = regexp.MustCompile(`\{[^/{}]*\}`)

// normalizeRoute writes each path parameter of a route as {}, so that routes compare by their
// shape and not by the names of their parameters.
func normalizeRoute(route string) string {
	return pathParam.ReplaceAllString(route, "{}")
}

// generatedOnly returns block b's items: the operations of the generated client that the
// hand-written packages do not call, and every operation whose route the spec does not have,
// each as <package>.<service>.<operation> with its route and inSpec or notInSpec.
func generatedOnly(pkgName string, ops []operation, called map[string]bool,
	spec specRoutes,
) []item {
	var items []item
	for _, op := range ops {
		known := spec[op.method+" "+normalizeRoute(op.route)]
		if known && called[op.service+"."+op.name] {
			continue
		}
		where := notInSpec
		if known {
			where = inSpec
		}
		items = append(items, item{
			block:   blockOperations,
			id:      pkgName + "." + op.service + "." + op.name,
			details: op.method + " " + op.route + " " + where,
		})
	}
	return items
}

// extractSurface returns the v1 surface of the module's files: blocks a, b and c, sorted. spec
// is the API spec's operations. It fails when a package is missing or cannot be read, when an
// id is listed twice, or when a block is empty.
func extractSurface(files []sourceFile, spec specRoutes) ([]item, error) {
	fset := token.NewFileSet()
	var items []item
	var handWritten []parsedPackage
	for _, dir := range identifierDirs {
		p, err := parsePackage(fset, files, dir)
		if err != nil {
			return nil, err
		}
		handWritten = append(handWritten, p)
		items = append(items, identifiers(p)...)
	}
	openapi, err := parsePackage(fset, files, openapiDir)
	if err != nil {
		return nil, err
	}
	ops, err := operations(openapi)
	if err != nil {
		return nil, err
	}
	fields, err := serviceFields(openapi)
	if err != nil {
		return nil, err
	}
	items = append(items, generatedOnly(openapi.name, ops, calledOperations(handWritten, fields),
		spec)...)
	models, err := parsePackage(fset, files, modelsDir)
	if err != nil {
		return nil, err
	}
	items = append(items, modelTypes(models)...)
	sortItems(items)
	if err := checkItems(items); err != nil {
		return nil, err
	}
	return items, nil
}

// checkItems fails when two items have the same id, or when a block has no item: a surface
// without one of its blocks was not read completely.
func checkItems(items []item) error {
	blocks := map[string]bool{}
	seen := map[string]bool{}
	for _, it := range items {
		if seen[it.id] {
			return fmt.Errorf("the v1 surface lists %s twice", it.id)
		}
		seen[it.id] = true
		blocks[it.block] = true
	}
	for _, block := range []string{blockIdentifiers, blockOperations, blockModels} {
		if !blocks[block] {
			return fmt.Errorf("block %s of the v1 surface is empty", block)
		}
	}
	return nil
}

// surfaceHeader starts the v1 surface file, after its first newline. The line that names the
// module version and its hash follows it.
const surfaceHeader = `
# The v1 API that v2 replaces or drops. apisurface's migration mode checks that MIGRATION.md
# or tools/migration/pending.txt covers every line. apisurface wrote this file from the module
# zip that the Go module proxy served for the version below, verified by the checksum database:
#
#   go -C tools build -o ../bin/apisurface ./cmd/apisurface
#   bin/apisurface -module <module path>@<version>
#
# Do not edit it by hand. Each line is <block> <id> <details>, sorted by block, then id:
#
#   a  an exported identifier of pkg/api, pkg/config, pkg/enforcement, pkg/errors or pkg/permit:
#      <package>.<name> for a type, function, constant or variable, <package>.<type>.<name> for
#      a method. The details are its kind.
#   b  an operation of the generated client in pkg/openapi, <package>.<service>.<operation>,
#      that the packages of block a never call, or whose route the API spec does not have. The
#      details are its route and in-spec or not-in-spec: whether the spec
#      (tools/specs/control-plane.json) has the route.
#   c  a type of pkg/models, models.<name>.
#
`

// writeSurface returns the v1 surface file of the items, read from the module version with its
// hash, such as h1:....
func writeSurface(modulePath, version, sum string, items []item) []byte {
	var b bytes.Buffer
	b.WriteString(strings.TrimPrefix(surfaceHeader, "\n"))
	b.WriteString("# module " + modulePath + " " + version + " " + sum + "\n")
	for _, it := range items {
		b.WriteString(it.line() + "\n")
	}
	return b.Bytes()
}

// itemLine matches a line of the v1 surface file: a block, an id of two or more dotted Go
// identifiers, and the details.
var itemLine = regexp.MustCompile(`^([abc]) (` + goIdent + `(?:\.` + goIdent + `)+) (\S.*)$`)

// goIdent matches a Go identifier of ASCII letters, digits and underscores.
const goIdent = `[A-Za-z_][A-Za-z0-9_]*`

// readSurface parses a v1 surface file. It fails on a line that is not a comment, blank or an
// item, on items out of order or listed twice, and when a block is empty.
func readSurface(r io.Reader) ([]item, error) {
	var items []item
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := itemLine.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("line %d is not <block> <id> <details>: %q", n, line)
		}
		it := item{block: m[1], id: m[2], details: m[3]}
		if len(items) > 0 {
			last := items[len(items)-1]
			if cmp.Or(strings.Compare(last.block, it.block), strings.Compare(last.id, it.id)) >= 0 {
				return nil, fmt.Errorf("line %d, %s, is not after %s: the lines must be sorted "+
					"by block, then id, each once", n, it.id, last.id)
			}
		}
		items = append(items, it)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if err := checkItems(items); err != nil {
		return nil, err
	}
	return items, nil
}
