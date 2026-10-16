package main

import (
	"go/types"
	"slices"
	"strings"
	"testing"
)

func TestParseV1API(t *testing.T) {
	t.Parallel()
	items, err := parseV1API("# a comment\n\na permit.Client type\na permit.Client.Check method\n" +
		"a permit.New var\na errors.NotFound const\na config.NewConfigBuilder func\n" +
		"c models.UserRead type\n")
	if err != nil {
		t.Fatalf("parseV1API: %v", err)
	}
	want := []v1Item{
		{id: "permit.Client", kind: "type"},
		{id: "permit.Client.Check", kind: "method"},
		{id: "permit.New", kind: "var"},
		{id: "errors.NotFound", kind: "const"},
		{id: "config.NewConfigBuilder", kind: "func"},
		{id: "models.UserRead", kind: "type"},
	}
	if !slices.Equal(items, want) {
		t.Errorf("parseV1API = %v, want %v", items, want)
	}
}

func TestParseV1APIRejects(t *testing.T) {
	t.Parallel()
	for name, line := range map[string]string{
		"a line of another block":   "b openapi.UsersApiService.ListUsers GET /v2/users in-spec",
		"too few fields":            "a permit.Client",
		"too many fields":           "a permit.Client type extra",
		"an unknown kind":           "a permit.Client struct",
		"a method without its type": "a permit.Check method",
		"a type with a method part": "a permit.Client.Check type",
		"an empty part":             "a permit..Check method",
		"no package":                "a Client type",
		"c: not a type":             "c models.PtrString func",
		"c: not of models":          "c permit.Client type",
		"c: a method part":          "c models.UserRead.GetKey type",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := parseV1API("a permit.Client type\n" + line + "\n")
			if err == nil || !strings.Contains(err.Error(), "line 2") {
				t.Errorf("parseV1API(%q) = %v, want an error that names line 2", line, err)
			}
		})
	}
}

func TestParseV1Consts(t *testing.T) {
	t.Parallel()
	items, err := parseV1Consts("# a comment\n\nmodels.GET models.Methods\n" +
		"  models.STRING   models.AttributeType  \n")
	if err != nil {
		t.Fatalf("parseV1Consts: %v", err)
	}
	want := []v1Item{
		{id: "models.GET", kind: "const", typ: "models.Methods"},
		{id: "models.STRING", kind: "const", typ: "models.AttributeType"},
	}
	if !slices.Equal(items, want) {
		t.Errorf("parseV1Consts = %v, want %v", items, want)
	}
	for name, line := range map[string]string{
		"no type":                       "models.POST",
		"a third field":                 "models.POST models.Methods extra",
		"a constant of another package": "errors.NotFound models.Methods",
		"a type of another package":     "models.POST errors.ErrorCode",
		"a constant with no name":       "models. models.Methods",
		"a type with a dot":             "models.POST models.Methods.Get",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := parseV1Consts("models.GET models.Methods\n" + line + "\n")
			if err == nil || !strings.Contains(err.Error(), "line 2") {
				t.Errorf("parseV1Consts(%q) = %v, want an error that names line 2", line, err)
			}
		})
	}
}

// v1Packages builds the packages of the v1 API that the scanner embeds.
func v1Packages(t *testing.T) map[string]*types.Package {
	t.Helper()
	pkgs, err := v1Model()
	if err != nil {
		t.Fatalf("v1Model: %v", err)
	}
	return pkgs
}

func TestNewV1Packages(t *testing.T) {
	t.Parallel()
	pkgs := v1Packages(t)
	var paths []string
	for path, pkg := range pkgs {
		paths = append(paths, path)
		if !pkg.Complete() {
			t.Errorf("%s is not complete", path)
		}
		if want := v1Module + "/pkg/" + pkg.Name(); path != want || pkg.Path() != want {
			t.Errorf("package %s has the path %s under the key %s, want %s", pkg.Name(),
				pkg.Path(), path, want)
		}
	}
	slices.Sort(paths)
	want := []string{
		v1Module + "/pkg/api", v1Module + "/pkg/config", v1Module + "/pkg/enforcement",
		v1Module + "/pkg/errors", modelsPath, v1Module + "/pkg/permit",
	}
	if !slices.Equal(paths, want) {
		t.Errorf("packages %v, want %v", paths, want)
	}
	lookup := func(pkg, name string) types.Object {
		t.Helper()
		obj := pkgs[v1Module+"/pkg/"+pkg].Scope().Lookup(name)
		if obj == nil {
			t.Fatalf("%s.%s is missing", pkg, name)
		}
		return obj
	}
	for _, check := range []struct {
		pkg, name string
		want      string
	}{
		{"permit", "Client", "struct{Api *api.PermitApiClient; Elements *api.Elements}"},
		{"permit", "NewPermit", "func(args ...any) *permit.Client"},
		{"permit", "New", "func(args ...any) *permit.Client"},
		{"permit", "PermitInterface", "struct{}"},
		{"config", "PermitBuilder", "struct{config.PermitConfig}"},
		{"config", "NewConfigBuilder", "func(args ...any) *config.PermitBuilder"},
		{"enforcement", "UserBuilder", "func(args ...any) *enforcement.User"},
		{"enforcement", "ResourceBuilder", "func(args ...any) *enforcement.Resource"},
		{"enforcement", "WithTenants", "func(args ...any)"},
		{"api", "NewUsersApi", "func(args ...any) *api.Users"},
		{"api", "NewPermitApiClient", "func(args ...any) *api.PermitApiClient"},
		{"errors", "NotFound", "invalid type"},
		{"models", "UserRead", "struct{}"},
		{"models", "NewUserRead", "func(args ...any) *models.UserRead"},
		{"models", "NewUserReadWithDefaults", "func(args ...any) *models.UserRead"},
		{"models", "NewNullableUserRead", "func(args ...any) *models.NullableUserRead"},
		{"models", "STRING", "models.AttributeType"},
		{"models", "URLMatchTypeRegex", "models.UrlMatchType"},
	} {
		obj := lookup(check.pkg, check.name)
		got := obj.Type().String()
		if _, ok := obj.(*types.TypeName); ok {
			got = obj.Type().Underlying().String()
		}
		got = strings.ReplaceAll(got, v1Module+"/pkg/", "")
		if got != check.want {
			t.Errorf("%s.%s is %s, want %s", check.pkg, check.name, got, check.want)
		}
	}
	if _, ok := lookup("models", "STRING").(*types.Var); !ok {
		t.Error("models.STRING is not a variable of its type")
	}
	client, ok := lookup("api", "PermitApiClient").Type().Underlying().(*types.Struct)
	if !ok {
		t.Fatal("api.PermitApiClient is not a struct")
	}
	var services []string
	for field := range client.Fields() {
		if field.Type().String() != "*"+v1Module+"/pkg/api."+field.Name() {
			t.Errorf("api.PermitApiClient.%s is %s", field.Name(), field.Type())
		}
		services = append(services, field.Name())
	}
	if len(services) != 18 {
		t.Errorf("api.PermitApiClient has %d services, want the 18 of v1: %v", len(services),
			services)
	}
	for _, check := range []struct {
		pkg, typ, method string
		want             string
	}{
		{"permit", "Client", "Check", "func(args ...any)"},
		{"config", "PermitConfig", "WithApiUrl", "func(args ...any) *config.PermitConfig"},
		{"config", "PermitConfig", "Build", "func(args ...any) config.PermitConfig"},
		{"config", "PermitConfig", "GetContext", "func(args ...any) *config.PermitContext"},
		{"enforcement", "User", "WithEmail", "func(args ...any) *enforcement.User"},
		{"enforcement", "User", "Build", "func(args ...any) enforcement.User"},
		{"enforcement", "Resource", "WithTenant", "func(args ...any) *enforcement.Resource"},
		{"enforcement", "Resource", "Build", "func(args ...any) enforcement.Resource"},
		{"api", "Tenants", "WaitForSync", "func(args ...any) *api.Tenants"},
		{"api", "Users", "Create", "func(args ...any)"},
	} {
		named, ok := lookup(check.pkg, check.typ).Type().(*types.Named)
		if !ok {
			t.Fatalf("%s.%s is not a named type", check.pkg, check.typ)
		}
		obj, _, _ := types.LookupFieldOrMethod(named, false, named.Obj().Pkg(), check.method)
		fn, ok := obj.(*types.Func)
		if !ok {
			t.Errorf("%s.%s has no method %s", check.pkg, check.typ, check.method)
			continue
		}
		sig := fn.Signature()
		if recv := sig.Recv().Type(); recv != named {
			t.Errorf("%s.%s.%s has the receiver %s, want the value %s", check.pkg, check.typ,
				check.method, recv, named)
		}
		got := strings.ReplaceAll(types.NewSignatureType(nil, nil, nil, sig.Params(),
			sig.Results(), sig.Variadic()).String(), v1Module+"/pkg/", "")
		if got != check.want {
			t.Errorf("%s.%s.%s is %s, want %s", check.pkg, check.typ, check.method, got,
				check.want)
		}
	}
	// Each row of v1Results, written out apart from it, so that a row that is lost or changed
	// fails here; each matches v1.3.0's signature.
	wantResults := map[string]string{
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
	if len(v1Results) != len(wantResults) {
		t.Errorf("v1Results has %d rows, want the %d of wantResults", len(v1Results),
			len(wantResults))
	}
	for id, want := range wantResults {
		parts := strings.Split(id, ".")
		obj := lookup(parts[0], parts[1])
		if len(parts) == 3 {
			obj, _, _ = types.LookupFieldOrMethod(obj.Type(), false, obj.Pkg(), parts[2])
		}
		got := "nothing"
		if obj == nil {
			got = "no such method"
		} else if sig, ok := obj.Type().(*types.Signature); ok && sig.Results().Len() == 1 {
			got = strings.ReplaceAll(sig.Results().At(0).Type().String(), v1Module+"/pkg/", "")
		}
		if got != want {
			t.Errorf("%s returns %s, want %s", id, got, want)
		}
	}
}

func TestNewV1PackagesRejects(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		items []v1Item
		want  string
	}{
		"a method of an unknown type": {
			items: []v1Item{{id: "permit.Client.Check", kind: "method"}},
			want:  "permit.Client.Check is a method of permit.Client, which is not a type",
		},
		"a field of a type that is missing": {
			items: []v1Item{{id: "permit.Client", kind: "type"}},
			want: "the field Api of permit.Client has the type api.PermitApiClient, which is " +
				"not a type",
		},
		"a result that is missing": {
			items: []v1Item{{id: "config.NewConfigBuilder", kind: "func"}},
			want: "the result of config.NewConfigBuilder is config.PermitBuilder, which is " +
				"not a type",
		},
		"a constant of a type that is missing": {
			items: []v1Item{{id: "models.GET", kind: "const", typ: "models.Methods"}},
			want:  "the constant models.GET has the type models.Methods, which is not a type",
		},
		"a constructor that a name has already": {
			items: []v1Item{
				{id: "models.UserRead", kind: "type"},
				{id: "models.NewUserRead", kind: "const", typ: "models.UserRead"},
			},
			want: "models.NewUserRead is declared twice",
		},
		"a type listed twice": {
			items: []v1Item{
				{id: "models.UserRead", kind: "type"}, {id: "models.UserRead", kind: "type"},
			},
			want: "models.UserRead is declared twice",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := newV1Packages(test.items)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("newV1Packages = %v, want an error with %q", err, test.want)
			}
		})
	}
}
