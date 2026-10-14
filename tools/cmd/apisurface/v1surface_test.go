package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strings"
	"testing"
)

// fixtureModule returns the Go files of a module laid out as v1 is, as a map from slash path to
// content.
func fixtureModule() map[string]string {
	return map[string]string{
		"main.go": "package main\n\nfunc Main() {}\n",
		"pkg/api/api.go": `// Package api is a fixture.
package api

import "example.com/sdk/pkg/openapi"

const DefaultPerPageLimit = 100

type Users struct{ client *openapi.APIClient }

func NewUsersApi() *Users { return nil }

func (u *Users) Create() { u.client.UsersApi.CreateUser(nil).Execute() }

func (u Users) List() { _, _ = u.client.UsersApi.ListUsersExecute(nil) }

func (u *Users) login() { u.client.AuthApi.Login(nil) }

type hiddenType struct{}

func (hiddenType) Exported() {}

func helper() {}
`,
		"pkg/api/api_test.go": "package api\n\nfunc TestOnly() {}\n",
		"pkg/config/config.go": `package config

type PermitConfig struct{ Token string }

func (c *PermitConfig) GetToken() string { return c.Token }

type Box[T any] struct{ v T }

func (b *Box[T]) Get() T { return b.v }

var (
	shared, Exported = 1, 2
)
`,
		"pkg/enforcement/user.go": "package enforcement\n\ntype User struct{}\n\n" +
			"func UserBuilder() *User { return nil }\n",
		"pkg/errors/errors.go":  "package errors\n\nconst (\n\tA = iota\n\tB\n\tc\n)\n",
		"pkg/permit/permit.go":  "package permit\n\nvar New = 1\n",
		"pkg/permit/sub/sub.go": "package sub\n\nfunc Nested() {}\n",
		"pkg/openapi/client.go": `package openapi

type APIClient struct {
	cfg      *int
	UsersApi *UsersApiService
	AuthApi  *AuthApiService
	Count    int
}
`,
		"pkg/openapi/api_users.go": `package openapi

import "net/http"

type UsersApiService struct{}

type ApiCreateUserRequest struct{}

func (r ApiCreateUserRequest) Execute() {}

func (a *UsersApiService) CreateUser(ctx any) ApiCreateUserRequest {
	return ApiCreateUserRequest{}
}

func (a *UsersApiService) CreateUserExecute(r ApiCreateUserRequest) {
	var (
		localVarHTTPMethod = http.MethodPost
	)
	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/users"
	_, _ = localVarHTTPMethod, localVarPath
}

func (a *UsersApiService) ListUsers(ctx any) int { return 0 }

func (a *UsersApiService) ListUsersExecute(r int) (int, error) {
	var localVarHTTPMethod = http.MethodGet
	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/users"
	_, _ = localVarHTTPMethod, localVarPath
	return 0, nil
}

func (a *UsersApiService) ReplaceUser(ctx any) int { return 0 }

func (a *UsersApiService) ReplaceUserExecute(r int) {
	var localVarHTTPMethod = http.MethodPut
	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/users/{user_id}"
	_, _ = localVarHTTPMethod, localVarPath
}

func (a *UsersApiService) DeleteUser(ctx any) int { return 0 }

func (a *UsersApiService) DeleteUserExecute(r int) {
	var localVarHTTPMethod = http.MethodDelete
	localVarPath := localBasePath + "/v2/old/{x}"
	_, _ = localVarHTTPMethod, localVarPath
}
`,
		"pkg/openapi/api_auth.go": `package openapi

import "net/http"

type AuthApiService struct{}

func (a *AuthApiService) Login(ctx any) int { return 0 }

func (a *AuthApiService) LoginExecute(r int) {
	var localVarHTTPMethod = http.MethodPost
	localVarPath := localBasePath + "/v2/auth/login"
	_, _ = localVarHTTPMethod, localVarPath
}
`,
		"pkg/models/user.go": "package models\n\ntype User struct{}\n\n" +
			"type NullableUser struct{}\n\ntype kind string\n\n" +
			"func NewUser() *User { return nil }\n\nconst X = 1\n",
		"pkg/models/README.md": "not Go\n",
	}
}

// fixtureSpec is the API spec of the fixture module: a path item's parameters are not an
// operation, and a route's parameters may be named differently from the generated client's.
const fixtureSpec = `{"paths": {
	"/v2/facts/{proj_id}/{env_id}/users": {"get": {}, "post": {}, "parameters": []},
	"/v2/facts/{proj_id}/{env_id}/users/{id}": {"put": {}}
}}`

// fixtureSurface is the v1 surface of fixtureModule.
var fixtureSurface = []string{
	"a api.DefaultPerPageLimit const",
	"a api.NewUsersApi func",
	"a api.Users type",
	"a api.Users.Create method",
	"a api.Users.List method",
	"a api.hiddenType.Exported method",
	"a config.Box type",
	"a config.Box.Get method",
	"a config.Exported var",
	"a config.PermitConfig type",
	"a config.PermitConfig.GetToken method",
	"a enforcement.User type",
	"a enforcement.UserBuilder func",
	"a errors.A const",
	"a errors.B const",
	"a permit.New var",
	"b openapi.AuthApiService.Login POST /v2/auth/login not-in-spec",
	"b openapi.UsersApiService.DeleteUser DELETE /v2/old/{x} not-in-spec",
	"b openapi.UsersApiService.ReplaceUser PUT /v2/facts/{proj_id}/{env_id}/users/{user_id} " +
		"in-spec",
	"c models.NullableUser type",
	"c models.User type",
}

// sourceFiles returns the files of a fixture as extractSurface takes them.
func sourceFiles(files map[string]string) []sourceFile {
	var list []sourceFile
	for _, name := range slices.Sorted(maps.Keys(files)) {
		list = append(list, sourceFile{name: name, content: []byte(files[name])})
	}
	return list
}

// itemLines returns the lines of items.
func itemLines(items []item) []string {
	lines := make([]string, 0, len(items))
	for _, it := range items {
		lines = append(lines, it.line())
	}
	return lines
}

func TestExtractSurface(t *testing.T) {
	spec, err := parseSpec([]byte(fixtureSpec))
	if err != nil {
		t.Fatal(err)
	}
	items, err := extractSurface(sourceFiles(fixtureModule()), spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := itemLines(items); !slices.Equal(got, fixtureSurface) {
		t.Errorf("extractSurface =\n%s\nwant\n%s", strings.Join(got, "\n"),
			strings.Join(fixtureSurface, "\n"))
	}
}

func TestExtractSurfaceErrors(t *testing.T) {
	spec, err := parseSpec([]byte(fixtureSpec))
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		files map[string]string
		want  string
	}{
		"a missing package": {
			files: map[string]string{"pkg/permit/permit.go": ""},
			want:  "the module has no Go file in pkg/permit",
		},
		"a build constraint": {
			files: map[string]string{
				"pkg/api/linux.go": "//go:build linux\n\npackage api\n\nfunc OnLinux() {}\n",
			},
			want: `pkg/api/linux.go has the build constraint "//go:build linux"`,
		},
		"an old build constraint": {
			files: map[string]string{
				"pkg/errors/old.go": "// +build cgo\n\npackage errors\n",
			},
			want: `has the build constraint "// +build cgo"`,
		},
		"two packages in one directory": {
			files: map[string]string{"pkg/config/other.go": "package other\n"},
			want:  "pkg/config declares package config and package other",
		},
		"a file that does not parse": {
			files: map[string]string{"pkg/enforcement/bad.go": "package enforcement\n\nfunc (\n"},
			want:  "parsing pkg/enforcement/bad.go",
		},
		"an Execute method without a route": {
			files: map[string]string{
				"pkg/openapi/api_bad.go": "package openapi\n\ntype BadApiService struct{}\n\n" +
					"func (a *BadApiService) Get() {}\n\nfunc (a *BadApiService) GetExecute() {\n" +
					"\tvar localVarHTTPMethod = http.MethodGet\n\t_ = localVarHTTPMethod\n}\n",
			},
			want: "BadApiService.GetExecute sets no HTTP method or no route",
		},
		"an Execute method without a method": {
			files: map[string]string{
				"pkg/openapi/api_bad.go": "package openapi\n\ntype BadApiService struct{}\n\n" +
					"func (a *BadApiService) Get() {}\n\nfunc (a *BadApiService) GetExecute() {\n" +
					"\tlocalVarPath := localBasePath + \"/v2/x\"\n\t_ = localVarPath\n}\n",
			},
			want: "BadApiService.GetExecute sets no HTTP method or no route",
		},
		"an Execute method that declares its method without a value": {
			files: map[string]string{
				"pkg/openapi/api_bad.go": "package openapi\n\ntype BadApiService struct{}\n\n" +
					"func (a *BadApiService) Get() {}\n\nfunc (a *BadApiService) GetExecute() {\n" +
					"\tvar localVarHTTPMethod string\n" +
					"\tlocalVarPath := localBasePath + \"/v2/x\"\n" +
					"\t_, _ = localVarHTTPMethod, localVarPath\n}\n",
			},
			want: "BadApiService.GetExecute sets no HTTP method or no route",
		},
		"an Execute method without its builder": {
			files: map[string]string{
				"pkg/openapi/api_bad.go": "package openapi\n\ntype BadApiService struct{}\n\n" +
					"func (a *BadApiService) GetExecute() {\n" +
					"\tvar localVarHTTPMethod = http.MethodGet\n" +
					"\tlocalVarPath := localBasePath + \"/v2/x\"\n" +
					"\t_, _ = localVarHTTPMethod, localVarPath\n}\n",
			},
			want: "BadApiService.GetExecute has no Get method that builds its request",
		},
		"a client without operations": {
			files: map[string]string{
				"pkg/openapi/api_users.go": "", "pkg/openapi/api_auth.go": "",
			},
			want: "pkg/openapi has no operation",
		},
		"a client without services": {
			files: map[string]string{
				"pkg/openapi/client.go": "package openapi\n\ntype APIClient struct{ Count int }\n",
			},
			want: "pkg/openapi declares no APIClient with services",
		},
		"a client that is not a struct": {
			files: map[string]string{
				"pkg/openapi/client.go": "package openapi\n\ntype APIClient int\n",
			},
			want: "openapi.APIClient is not a struct",
		},
		"no models type": {
			files: map[string]string{"pkg/models/user.go": "package models\n\ntype hidden int\n"},
			want:  "block c of the v1 surface is empty",
		},
		"a method declared twice": {
			files: map[string]string{
				"pkg/config/again.go": "package config\n\nfunc (c *PermitConfig) GetToken() {}\n",
			},
			want: "the v1 surface lists config.PermitConfig.GetToken twice",
		},
	} {
		t.Run(name, func(t *testing.T) {
			files := fixtureModule()
			maps.Copy(files, tc.files)
			maps.DeleteFunc(files, func(_, content string) bool { return content == "" })
			_, err := extractSurface(sourceFiles(files), spec)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("extractSurface error = %v, want one that contains %q", err, tc.want)
			}
		})
	}
}

func TestEmptyBlocks(t *testing.T) {
	spec, err := parseSpec([]byte(fixtureSpec))
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		files map[string]string
		want  string
	}{
		"no exported identifier": {
			files: map[string]string{
				"pkg/api/api.go":          "package api\n\nfunc (u *Users) login() {}\n",
				"pkg/config/config.go":    "package config\n",
				"pkg/enforcement/user.go": "package enforcement\n",
				"pkg/errors/errors.go":    "package errors\n",
				"pkg/permit/permit.go":    "package permit\n",
			},
			want: "block a of the v1 surface is empty",
		},
		"every operation is called and in the spec": {
			files: map[string]string{
				"pkg/openapi/api_auth.go": "",
				"pkg/openapi/api_users.go": strings.Join(slices.DeleteFunc(
					strings.SplitAfter(fixtureModule()["pkg/openapi/api_users.go"], "\n\n"),
					func(part string) bool {
						return strings.Contains(part, "ReplaceUser") ||
							strings.Contains(part, "DeleteUser")
					}), ""),
			},
			want: "block b of the v1 surface is empty",
		},
	} {
		t.Run(name, func(t *testing.T) {
			files := fixtureModule()
			maps.Copy(files, tc.files)
			maps.DeleteFunc(files, func(_, content string) bool { return content == "" })
			_, err := extractSurface(sourceFiles(files), spec)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("extractSurface error = %v, want one that contains %q", err, tc.want)
			}
		})
	}
}

func TestParseSpec(t *testing.T) {
	spec, err := parseSpec([]byte(`{"paths": {"/v2/a/{x}": {"get": {}, "head": {},
		"trace": {}, "summary": "s", "servers": []}, "/v2/b": {"patch": {}, "options": {},
		"put": {}, "post": {}, "delete": {}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"DELETE /v2/b", "GET /v2/a/{}", "HEAD /v2/a/{}", "OPTIONS /v2/b",
		"PATCH /v2/b", "POST /v2/b", "PUT /v2/b", "TRACE /v2/a/{}",
	}
	if got := slices.Sorted(maps.Keys(spec)); !slices.Equal(got, want) {
		t.Errorf("parseSpec = %q, want %q", got, want)
	}
	for name, content := range map[string]string{
		"not JSON":      "{",
		"no paths":      `{"openapi": "3.1.0"}`,
		"no operations": `{"paths": {"/v2/a": {"parameters": []}}}`,
	} {
		if _, err := parseSpec([]byte(content)); err == nil {
			t.Errorf("parseSpec of %s succeeded, want an error", name)
		}
	}
}

func TestNormalizeRoute(t *testing.T) {
	for route, want := range map[string]string{
		"/v2/a/{proj_id}/b/{x}": "/v2/a/{}/b/{}",
		"/v2/a":                 "/v2/a",
		"/v2/{}":                "/v2/{}",
		"/v2/a/{b}{c}":          "/v2/a/{}{}",
	} {
		if got := normalizeRoute(route); got != want {
			t.Errorf("normalizeRoute(%q) = %q, want %q", route, got, want)
		}
	}
}

func TestReceiverName(t *testing.T) {
	for src, want := range map[string]string{
		"func (u *Users) F() {}":        "Users",
		"func (u Users) F() {}":         "Users",
		"func (b *Box[T]) F() {}":       "Box",
		"func (b Pair[K, V]) F() {}":    "Pair",
		"func (b *(Paren)) F() {}":      "Paren",
		"func (b *Box[T, U]) F() {}":    "Box",
		"func (b *pkg.Other) F() {}":    "*ast.SelectorExpr",
		"func (b *[]Users) F() {}":      "*ast.ArrayType",
		"func (b *Box[Inner]) F() {}":   "Box",
		"func (b **Users) F() {}":       "Users",
		"func (b *(Box[T])) F() {}":     "Box",
		"func (b (*Users)) F() {}":      "Users",
		"func (b Users) Execute() {}":   "Users",
		"func (b *Users2) F() {}":       "Users2",
		"func (b *users) Exported() {}": "users",
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "", "package p\n"+src, 0)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		fn, ok := file.Decls[0].(*ast.FuncDecl)
		if !ok {
			t.Fatalf("%s declares a %T", src, file.Decls[0])
		}
		if got := receiverName(fn.Recv.List[0].Type); got != want {
			t.Errorf("receiverName of %s = %q, want %q", src, got, want)
		}
	}
}

func TestSurfaceFileRoundTrip(t *testing.T) {
	items := []item{
		{block: "a", id: "api.Users", details: "type"},
		{block: "a", id: "api.Users.Create", details: "method"},
		{block: "b", id: "openapi.S.Op", details: "GET /v2/x in-spec"},
		{block: "c", id: "models.User", details: "type"},
	}
	content := writeSurface("example.com/sdk", "v1.2.3", "h1:abc=", items)
	if !strings.Contains(string(content), "\n# module example.com/sdk v1.2.3 h1:abc=\n") {
		t.Errorf("the header does not name the module version:\n%s", content)
	}
	got, err := readSurface(strings.NewReader(string(content)))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, items) {
		t.Errorf("readSurface(writeSurface(items)) = %v, want %v", got, items)
	}
}

func TestReadSurfaceErrors(t *testing.T) {
	const valid = "a api.Users type\nb openapi.S.Op GET /x in-spec\nc models.User type\n"
	if _, err := readSurface(strings.NewReader("# comment\n\n" + valid)); err != nil {
		t.Fatalf("readSurface of a valid file: %v", err)
	}
	for name, tc := range map[string]struct {
		content, want string
	}{
		"an unknown block": {
			content: valid + "d models.Other type\n",
			want:    `line 4 is not <block> <id> <details>: "d models.Other type"`,
		},
		"no details": {
			content: "a api.Users\n" + valid,
			want:    "line 1 is not <block> <id> <details>",
		},
		"an id without a package": {
			content: "a Users type\n" + valid,
			want:    "line 1 is not <block> <id> <details>",
		},
		"an id with an empty name": {
			content: "a api..Users type\n" + valid,
			want:    "line 1 is not <block> <id> <details>",
		},
		"a leading space": {
			content: " a api.A type\n" + valid,
			want:    "line 1 is not <block> <id> <details>",
		},
		"ids out of order": {
			content: "a api.Z type\n" + valid,
			want:    "line 2, api.Users, is not after api.Z",
		},
		"blocks out of order": {
			content: "a api.Users type\nc models.User type\nb openapi.S.Op GET /x in-spec\n",
			want:    "line 3, openapi.S.Op, is not after models.User",
		},
		"a repeated id": {
			content: "a api.Users type\na api.Users method\nb openapi.S.Op GET /x in-spec\n" +
				"c models.User type\n",
			want: "line 2, api.Users, is not after api.Users",
		},
		"an id in two blocks": {
			content: "a api.Users type\nb api.Users GET /x in-spec\nc models.User type\n",
			want:    "the v1 surface lists api.Users twice",
		},
		"an empty block": {
			content: "a api.Users type\nc models.User type\n",
			want:    "block b of the v1 surface is empty",
		},
		"no item": {
			content: "# only a comment\n",
			want:    "block a of the v1 surface is empty",
		},
	} {
		_, err := readSurface(strings.NewReader(tc.content))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: readSurface error = %v, want one that contains %q", name, err, tc.want)
		}
	}
}
