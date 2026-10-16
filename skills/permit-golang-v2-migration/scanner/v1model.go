package main

import (
	_ "embed" // v1-api.txt and v1-models-consts.txt
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"
)

// v1Module is the module path of v1.
const v1Module = "github.com/permitio/permit-golang"

// modelsPath is the import path of v1's package models.
const modelsPath = v1Module + "/pkg/models"

// v1API is v1-api.txt: the exported declarations of v1's packages api, config, enforcement,
// errors and permit, one per line as "a <id> <kind>", and the types of its package models, as
// "c models.<name> type".
//
//go:embed v1-api.txt
var v1API string

// v1Consts is v1-models-consts.txt: the constants of v1's package models, one per line as
// "<constant> <type>".
//
//go:embed v1-models-consts.txt
var v1Consts string

// v1Item is a declaration of v1: its id, <package>.<name> or <package>.<type>.<method>; its
// kind, one of type, func, const, var and method; and for a constant of models, its type.
type v1Item struct {
	id, kind, typ string
}

// parseV1API parses the lines of text in the format of v1-api.txt.
func parseV1API(text string) ([]v1Item, error) {
	var items []v1Item
	n := 0
	for line := range strings.Lines(text) {
		n++
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		block := ""
		if len(fields) == 3 {
			block = fields[0]
		}
		if block != "a" && (block != "c" || fields[2] != "type" || !isModelsName(fields[1])) {
			return nil, fmt.Errorf("line %d: %q is not \"a <id> <kind>\" or "+
				"\"c models.<name> type\"", n, line)
		}
		parts := strings.Split(fields[1], ".")
		wantParts := 2
		switch fields[2] {
		case "method":
			wantParts = 3
		case "type", "func", "const", "var":
		default:
			return nil, fmt.Errorf("line %d: the kind %q is not type, func, const, var or "+
				"method", n, fields[2])
		}
		if len(parts) != wantParts || slices.Contains(parts, "") {
			return nil, fmt.Errorf("line %d: a %s is <package>.<name>, or "+
				"<package>.<type>.<name> for a method, not %q", n, fields[2], fields[1])
		}
		items = append(items, v1Item{id: fields[1], kind: fields[2]})
	}
	return items, nil
}

// parseV1Consts parses the lines of text in the format of v1-models-consts.txt into constants of
// models, each with its type.
func parseV1Consts(text string) ([]v1Item, error) {
	var items []v1Item
	n := 0
	for line := range strings.Lines(text) {
		n++
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || !isModelsName(fields[0]) || !isModelsName(fields[1]) {
			return nil, fmt.Errorf("line %d: %q is not \"models.<constant> models.<type>\"", n,
				line)
		}
		items = append(items, v1Item{id: fields[0], kind: "const", typ: fields[1]})
	}
	return items, nil
}

// isModelsName reports whether s is models.<name>, with a name that has no dot.
func isModelsName(s string) bool {
	name, ok := strings.CutPrefix(s, "models.")
	return ok && name != "" && !strings.Contains(name, ".")
}

// v1Field is an exported field of a v1 type: its name, or "" for an embedded field, and its
// type, as <package>.<type> with a * in front for a pointer.
type v1Field struct {
	name, typ string
}

// v1Fields are the fields of the v1 types that lead from the client to the methods of its
// services, which v1-api.txt does not list: the client's Api and Elements, the services of
// api.PermitApiClient, and the PermitConfig that config.PermitBuilder embeds, whose methods
// the builder has.
var v1Fields = map[string][]v1Field{
	"permit.Client": {{"Api", "*api.PermitApiClient"}, {"Elements", "*api.Elements"}},
	"api.PermitApiClient": {
		{"ConditionSets", "*api.ConditionSets"},
		{"Elements", "*api.Elements"},
		{"Environments", "*api.Environments"},
		{"ImplicitGrants", "*api.ImplicitGrants"},
		{"Projects", "*api.Projects"},
		{"ProxyConfigs", "*api.ProxyConfigs"},
		{"RelationshipTuples", "*api.RelationshipTuples"},
		{"ResourceActionGroups", "*api.ResourceActionGroups"},
		{"ResourceActions", "*api.ResourceActions"},
		{"ResourceAttributes", "*api.ResourceAttributes"},
		{"ResourceInstances", "*api.ResourceInstances"},
		{"ResourceRelations", "*api.ResourceRelations"},
		{"ResourceRoles", "*api.ResourceRoles"},
		{"Resources", "*api.Resources"},
		{"RoleAssignments", "*api.RoleAssignments"},
		{"Roles", "*api.Roles"},
		{"Tenants", "*api.Tenants"},
		{"Users", "*api.Users"},
	},
	"config.PermitBuilder": {{"", "config.PermitConfig"}},
}

// v1Results are the results of the v1 functions and methods that return a v1 value whose methods
// a call can be chained to, in the format of v1Field.typ. resultOf adds the rules that cover the
// rest.
var v1Results = map[string]string{
	"permit.New":                          "*permit.Client",
	"permit.NewPermit":                    "*permit.Client",
	"config.NewConfigBuilder":             "*config.PermitBuilder",
	"config.NewPermitConfig":              "*config.PermitConfig",
	"config.NewPermitContext":             "*config.PermitContext",
	"config.PermitConfig.Build":           "config.PermitConfig",
	"config.PermitConfig.GetContext":      "*config.PermitContext",
	"config.PermitContext.GetContext":     "*config.PermitContext",
	"enforcement.UserBuilder":             "*enforcement.User",
	"enforcement.User.Build":              "enforcement.User",
	"enforcement.ResourceBuilder":         "*enforcement.Resource",
	"enforcement.Resource.Build":          "enforcement.Resource",
	"enforcement.NewPermitEnforcerClient": "*enforcement.PermitEnforcer",
	"api.NewPermitApiClient":              "*api.PermitApiClient",
	"api.NewPermitBaseFactsApi":           "*api.PermitBaseFactsApi",
}

// resultOf returns the result of the function or method id of the given kind, or "" when it
// returns no v1 value that a call can be chained to. Besides v1Results: a method named With...
// returns its receiver, as each builder method of v1 does, and so does WaitForSync;
// api.New<service>Api returns the service; and a constructor of models, models.New<type> or
// models.New<type>WithDefaults, returns a pointer to its type.
func resultOf(id, kind string) string {
	if typ, ok := v1Results[id]; ok {
		return typ
	}
	parts := strings.Split(id, ".")
	name := parts[len(parts)-1]
	switch {
	case kind == "method" && (strings.HasPrefix(name, "With") || name == "WaitForSync"):
		return "*" + parts[0] + "." + parts[1]
	case kind == "func" && parts[0] == "api" && strings.HasPrefix(name, "New") &&
		strings.HasSuffix(name, "Api"):
		return "*api." + strings.TrimSuffix(strings.TrimPrefix(name, "New"), "Api")
	case kind == "func" && parts[0] == "models" && strings.HasPrefix(name, "New"):
		return "*models." + strings.TrimSuffix(strings.TrimPrefix(name, "New"), "WithDefaults")
	}
	return ""
}

// newV1Packages builds a package of go/types for each package of v1 that items name, with an
// object for each item, keyed by the import path. The scanner type-checks the code it scans
// against them, so it can tell which v1 method a call reaches, and which v1 models type a value
// has. A type is a struct with the fields of v1Fields, or no field, so a type of models has
// neither field nor method; a function or a method takes any arguments, and returns what
// resultOf says, or nothing; a constant has no value, and a constant of models is a variable of
// its type. Each type of models gets its constructors too, models.New<type> and
// models.New<type>WithDefaults. A method has a value receiver, so that a call of it is found
// whatever its receiver: v1 code that compiles calls each method on a value that has it.
func newV1Packages(items []v1Item) (map[string]*types.Package, error) {
	pkgs := map[string]*types.Package{}
	pkgOf := func(name string) *types.Package {
		path := v1Module + "/pkg/" + name
		if pkgs[path] == nil {
			pkgs[path] = types.NewPackage(path, name)
		}
		return pkgs[path]
	}
	declare := func(id string, obj types.Object) error {
		if obj.Pkg().Scope().Insert(obj) != nil {
			return fmt.Errorf("%s is declared twice", id)
		}
		return nil
	}
	named := map[string]*types.Named{}
	var constructors []v1Item
	for _, item := range items {
		if item.kind != "type" {
			continue
		}
		pkg, name, _ := strings.Cut(item.id, ".")
		obj := types.NewTypeName(token.NoPos, pkgOf(pkg), name, nil)
		named[item.id] = types.NewNamed(obj, types.NewStruct(nil, nil), nil)
		if err := declare(item.id, obj); err != nil {
			return nil, err
		}
		if pkg == "models" {
			constructors = append(constructors, v1Item{id: "models.New" + name, kind: "func"},
				v1Item{id: "models.New" + name + "WithDefaults", kind: "func"})
		}
	}
	items = slices.Concat(items, constructors)
	typeOf := func(expr string) (types.Type, bool) {
		name, pointer := strings.CutPrefix(expr, "*")
		typ, ok := named[name]
		if pointer {
			return types.NewPointer(typ), ok
		}
		return typ, ok
	}
	for _, item := range items {
		fields := v1Fields[item.id]
		if item.kind != "type" || fields == nil {
			continue
		}
		var vars []*types.Var
		for _, field := range fields {
			typ, ok := typeOf(field.typ)
			if !ok {
				return nil, fmt.Errorf("the field %s of %s has the type %s, which is not a type "+
					"of v1-api.txt", field.name, item.id, strings.TrimPrefix(field.typ, "*"))
			}
			pkg := named[item.id].Obj().Pkg()
			vars = append(vars, types.NewField(token.NoPos, pkg, field.name, typ, field.name == ""))
		}
		named[item.id].SetUnderlying(types.NewStruct(vars, nil))
	}
	anyType := types.Universe.Lookup("any").Type()
	for _, item := range items {
		parts := strings.Split(item.id, ".")
		pkg := pkgOf(parts[0])
		name := parts[len(parts)-1]
		var results *types.Tuple
		if result := resultOf(item.id, item.kind); result != "" {
			typ, ok := typeOf(result)
			if !ok {
				return nil, fmt.Errorf("the result of %s is %s, which is not a type of "+
					"v1-api.txt", item.id, strings.TrimPrefix(result, "*"))
			}
			results = types.NewTuple(types.NewParam(token.NoPos, pkg, "", typ))
		}
		params := types.NewTuple(types.NewParam(token.NoPos, pkg, "args",
			types.NewSlice(anyType)))
		var obj types.Object
		switch item.kind {
		case "method":
			recvID := parts[0] + "." + parts[1]
			recv, ok := named[recvID]
			if !ok {
				return nil, fmt.Errorf("%s is a method of %s, which is not a type of "+
					"v1-api.txt", item.id, recvID)
			}
			sig := types.NewSignatureType(types.NewParam(token.NoPos, pkg, "", recv), nil, nil,
				params, results, true)
			recv.AddMethod(types.NewFunc(token.NoPos, pkg, name, sig))
		case "func":
			sig := types.NewSignatureType(nil, nil, nil, params, results, true)
			obj = types.NewFunc(token.NoPos, pkg, name, sig)
		case "var":
			var typ types.Type = types.Typ[types.Invalid]
			if results != nil {
				typ = types.NewSignatureType(nil, nil, nil, params, results, true)
			}
			obj = types.NewVar(token.NoPos, pkg, name, typ)
		case "const":
			if item.typ == "" {
				obj = types.NewConst(token.NoPos, pkg, name, types.Typ[types.Invalid],
					constant.MakeUnknown())
				break
			}
			typ, ok := typeOf(item.typ)
			if !ok {
				return nil, fmt.Errorf("the constant %s has the type %s, which is not a type of "+
					"v1-api.txt", item.id, item.typ)
			}
			obj = types.NewVar(token.NoPos, pkg, name, typ)
		}
		if obj != nil {
			if err := declare(item.id, obj); err != nil {
				return nil, err
			}
		}
	}
	for _, pkg := range pkgs {
		pkg.MarkComplete()
	}
	return pkgs, nil
}
