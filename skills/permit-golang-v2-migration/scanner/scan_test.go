package main

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestIsV1Path(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]bool{
		"github.com/permitio/permit-golang":                   true,
		"github.com/permitio/permit-golang/pkg/permit":        true,
		"github.com/permitio/permit-golang/pkg/models":        true,
		"github.com/permitio/permit-golang/pkg/openapi":       true,
		"github.com/permitio/permit-golang/v2":                false,
		"github.com/permitio/permit-golang/v2/models":         false,
		"github.com/permitio/permit-golang/v3/models":         false,
		"github.com/permitio/permit-golang/v10":               false,
		"github.com/permitio/permit-golang/vendor":            true,
		"github.com/permitio/permit-golang-extras/pkg/permit": false,
		"github.com/permitio/permit-golangx":                  false,
		"example.com/github.com/permitio/permit-golang":       false,
		"": false,
	} {
		if got := isV1Path(path); got != want {
			t.Errorf("isV1Path(%q) = %v, want %v", path, got, want)
		}
	}
}

// scanSource scans one file, main.go, with src as its content, and returns each use as
// "<line>:<column> <kind> <v1>".
func scanSource(t *testing.T, src string) []string {
	t.Helper()
	uses, err := scan(fstest.MapFS{"main.go": {Data: []byte(src)}}, v1Packages(t))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var got []string
	for _, u := range uses {
		if u.file != "main.go" {
			t.Errorf("a use in %s, want main.go", u.file)
		}
		got = append(got, fmt.Sprintf("%d:%d %s %s", u.line, u.column, u.kind, u.v1))
	}
	return got
}

func TestScan(t *testing.T) {
	t.Parallel()
	const imports = `package main

import (
	"context"

	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

var _ = context.Background
var _ = config.DefaultApiUrl
var _ = enforcement.AllowKey
var _ models.UserCreate
var _ permit.Client
`
	// The imports and their uses above are on lines 6 to 16.
	head := []string{
		"6:2 import github.com/permitio/permit-golang/pkg/config",
		"7:2 import github.com/permitio/permit-golang/pkg/enforcement",
		"8:2 import github.com/permitio/permit-golang/pkg/models",
		"9:2 import github.com/permitio/permit-golang/pkg/permit",
		"13:16 identifier config.DefaultApiUrl",
		"14:21 identifier enforcement.AllowKey",
		"15:14 identifier models.UserCreate",
		"16:14 identifier permit.Client",
	}
	for name, test := range map[string]struct {
		code string
		want []string
	}{
		"a method of the client": {
			code: "func f(c *permit.Client) { c.SyncUser(context.Background(), " +
				"models.UserCreate{}) }",
			want: []string{"17:18 identifier permit.Client", "17:30 identifier " +
				"permit.Client.SyncUser", "17:68 identifier models.UserCreate"},
		},
		"a method of a service reached through the client's fields": {
			code: "func f(c *permit.Client) { c.Api.Users.Create(nil, nil); " +
				"c.Elements.LoginAs(nil, nil); c.Api.Elements.LoginAs(nil, nil) }",
			want: []string{
				"17:18 identifier permit.Client", "17:40 identifier " +
					"api.Users.Create", "17:69 identifier api.Elements.LoginAs",
				"17:103 identifier api.Elements.LoginAs",
			},
		},
		"a service held in a variable": {
			code: "func f(c *permit.Client) { users := c.Api.Users; users.Delete(nil, \"k\") }",
			want: []string{"17:18 identifier permit.Client", "17:56 identifier " +
				"api.Users.Delete"},
		},
		"a chain of builder methods": {
			code: "var cfg = config.NewConfigBuilder(\"k\").WithPdpUrl(\"u\").Build()",
			want: []string{"17:18 identifier config.NewConfigBuilder", "17:40 identifier " +
				"config.PermitConfig.WithPdpUrl", "17:56 identifier config.PermitConfig.Build"},
		},
		"a builder held in a variable": {
			code: "func f() { b := enforcement.UserBuilder(\"u\"); b.WithEmail(\"e\"); " +
				"_ = b.Build() }",
			want: []string{"17:29 identifier enforcement.UserBuilder", "17:49 identifier " +
				"enforcement.User.WithEmail", "17:71 identifier enforcement.User.Build"},
		},
		"the client that New returns": {
			code: "var c = permit.New(config.PermitConfig{})\nvar _ = c.Check",
			want: []string{"17:16 identifier permit.New", "17:27 identifier " +
				"config.PermitConfig", "18:11 identifier permit.Client.Check"},
		},
		"WaitForSync returns its service": {
			code: "func f(c *permit.Client) { c.Api.Tenants.WaitForSync(nil, nil).Delete(nil, " +
				"\"t\") }",
			want: []string{"17:18 identifier permit.Client", "17:42 identifier " +
				"api.Tenants.WaitForSync", "17:64 identifier api.Tenants.Delete"},
		},
		"a method of a value that is not addressable": {
			code: "var _ = enforcement.ResourceBuilder(\"r\").Build().GetID",
			want: []string{"17:21 identifier enforcement.ResourceBuilder", "17:42 identifier " +
				"enforcement.Resource.Build", "17:50 identifier enforcement.Resource.GetID"},
		},
		"a method promoted from an embedded v1 type": {
			code: "type wrapper struct{ *permit.Client }\nfunc f(w wrapper) { w.Check(nil, nil, " +
				"nil) }",
			want: []string{"17:30 identifier permit.Client", "18:23 identifier " +
				"permit.Client.Check"},
		},
		"a method expression": {
			code: "var _ = (*permit.Client).AllTenantsCheck",
			want: []string{"17:18 identifier permit.Client", "17:26 identifier " +
				"permit.Client.AllTenantsCheck"},
		},
		"a field of a client is not a use": {
			code: "func f(c *permit.Client) { _ = c.Api; _ = c.Api.Users; _ = permit.Client{Api: " +
				"nil} }",
			want: []string{"17:18 identifier permit.Client", "17:67 identifier permit.Client"},
		},
		"an identifier of models that v1-api.txt does not list": {
			code: "var _ = models.PtrString(\"x\")",
			want: []string{"17:16 identifier models.PtrString"},
		},
		"methods of the value that a constructor returns": {
			code: "func f() { u := models.NewUserCreate(\"k\"); u.SetEmail(\"e\"); " +
				"_ = u.GetEmail() }",
			want: []string{
				"17:24 identifier models.NewUserCreate",
				"17:46 identifier models.UserCreate.SetEmail",
				"17:67 identifier models.UserCreate.GetEmail",
			},
		},
		"a method chained to a constructor": {
			code: "var _ = models.NewUserCreateWithDefaults().SetKey",
			want: []string{
				"17:16 identifier models.NewUserCreateWithDefaults",
				"17:44 identifier models.UserCreate.SetKey",
			},
		},
		"fields of a composite literal, a parameter and a field": {
			code: "type state struct{ user *models.UserRead }\n" +
				"func f(s state, r models.UserRead) { c := &models.UserCreate{Key: \"k\"}; " +
				"_, _, _ = c.Key, r.Email, s.user.Key }",
			want: []string{
				"17:33 identifier models.UserRead", "18:26 identifier models.UserRead",
				"18:51 identifier models.UserCreate", "18:85 identifier models.UserCreate.Key",
				"18:92 identifier models.UserRead.Email", "18:106 identifier models.UserRead.Key",
			},
		},
		"a field of a function's result": {
			code: "func g() *models.UserRead { return nil }\nvar _ = g().Email",
			want: []string{
				"17:18 identifier models.UserRead", "18:13 identifier models.UserRead.Email",
			},
		},
		"a method of a constant": {
			code: "var _ = models.STRING.Ptr()",
			want: []string{
				"17:16 identifier models.STRING", "17:23 identifier models.AttributeType.Ptr",
			},
		},
		"what a v1 call or a models method returns is not followed": {
			code: "func f(c *permit.Client, r models.UserRead) { u, _ := c.Api.Users.Get(nil, " +
				"\"k\"); _ = u.Email; _ = r.GetAttributes().Get }",
			want: []string{
				"17:18 identifier permit.Client", "17:35 identifier models.UserRead",
				"17:67 identifier api.Users.Get", "17:101 identifier models.UserRead.GetAttributes",
			},
		},
		"an identifier that v1 does not have is still reported": {
			code: "var _ = permit.Missing",
			want: []string{"17:16 identifier permit.Missing"},
		},
		"a local name that shadows an import": {
			code: "func f() { config := map[string]int{}; _ = config.Build; models := 1; " +
				"_ = models }",
		},
		"a method of error, which has no package": {
			code: "func f(err error) string { return err.Error() }",
		},
		"a method of the user's own type of the same name": {
			code: "type Client struct{}\nfunc (Client) Check() {}\nfunc f(c Client) { c.Check() }",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := scanSource(t, imports+test.code+"\n")
			want := slices.Concat(head, test.want)
			if !slices.Equal(got, want) {
				t.Errorf("scan =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestScanImports(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		src  string
		want []string
	}{
		"a renamed import is reported under its package's name": {
			src: "package p\n\nimport permitErrors " +
				"\"github.com/permitio/permit-golang/pkg/errors\"\n\n" +
				"var _ = permitErrors.NotFound\n",
			want: []string{
				"3:21 import github.com/permitio/permit-golang/pkg/errors",
				"5:22 identifier errors.NotFound",
			},
		},
		"a dot import": {
			src: "package p\n\nimport . \"github.com/permitio/permit-golang/pkg/enforcement\"\n\n" +
				"var _ = UserBuilder(\"u\").Build()\nvar _ = DefaultTenant\n",
			want: []string{
				"3:10 import github.com/permitio/permit-golang/pkg/enforcement",
				"5:9 identifier enforcement.UserBuilder", "5:26 identifier enforcement.User.Build",
				"6:9 identifier enforcement.DefaultTenant",
			},
		},
		"a dot import of models finds only what the model declares": {
			src: "package p\n\nimport . \"github.com/permitio/permit-golang/pkg/models\"\n\n" +
				"var _ = PtrString\nvar _ = NewUserRead().Key\n",
			want: []string{
				"3:10 import github.com/permitio/permit-golang/pkg/models",
				"6:9 identifier models.NewUserRead", "6:23 identifier models.UserRead.Key",
			},
		},
		"a blank import": {
			src:  "package p\n\nimport _ \"github.com/permitio/permit-golang/pkg/openapi\"\n",
			want: []string{"3:10 import github.com/permitio/permit-golang/pkg/openapi"},
		},
		"v2 and other modules": {
			src: "package p\n\nimport (\n\t\"github.com/permitio/permit-golang/v2\"\n\t" +
				"\"github.com/permitio/permit-golang/v2/models\"\n\t\"fmt\"\n)\n\n" +
				"var _ = permit.New\nvar _ models.UserCreate\nvar _ = fmt.Sprint\n",
		},
		"a file with import \"C\"": {
			src: "package p\n\n// #include <stdio.h>\nimport \"C\"\n\n" +
				"import \"github.com/permitio/permit-golang/pkg/permit\"\n\n" +
				"var _ = permit.NewPermit\n",
			want: []string{
				"6:8 import github.com/permitio/permit-golang/pkg/permit",
				"8:16 identifier permit.NewPermit",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := scanSource(t, test.src); !slices.Equal(got, test.want) {
				t.Errorf("scan =\n%s\nwant\n%s", strings.Join(got, "\n"),
					strings.Join(test.want, "\n"))
			}
		})
	}
}

func TestScanFiles(t *testing.T) {
	t.Parallel()
	const v1File = "package p\n\nimport \"github.com/permitio/permit-golang/pkg/permit\"\n\n" +
		"var _ *permit.Client\n"
	fsys := fstest.MapFS{
		"b.go": {Data: []byte(v1File)},
		"a.go": {Data: []byte(v1File)},
		"a_test.go": {
			Data: []byte(strings.Replace(v1File, "package p", "package p_test", 1)),
		},
		"gen/x.go":          {Data: []byte(v1File)},
		"notes.txt":         {Data: []byte(v1File)},
		"vendor/v/x.go":     {Data: []byte(v1File)},
		"gen/vendor/v/x.go": {Data: []byte(v1File)},
		"testdata/x.go":     {Data: []byte(v1File)},
		".hidden/x.go":      {Data: []byte(v1File)},
		"_skipped/x.go":     {Data: []byte(v1File)},
		"gen/.x.go":         {Data: []byte(v1File)},
		"gen/_x.go":         {Data: []byte(v1File)},
		"dir.go/x.go":       {Data: []byte(v1File)},
		"nested/go.mod":     {Data: []byte("module example.com/nested\n")},
		"nested/y.go":       {Data: []byte(v1File)},
		"two/one.go":        {Data: []byte(v1File)},
		"two/other.go": {
			Data: []byte(strings.Replace(v1File, "package p", "package q", 1)),
		},
		"two/zz_generated_mock.go": {Data: []byte(v1File)},
	}
	uses, err := scan(fsys, v1Packages(t))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var got []string
	for _, u := range uses {
		got = append(got, fmt.Sprintf("%s:%d %s", u.file, u.line, u.v1))
	}
	var want []string
	for _, file := range []string{
		"a.go", "a_test.go", "b.go", "dir.go/x.go", "gen/x.go", "nested/y.go", "two/one.go",
		"two/other.go", "two/zz_generated_mock.go",
	} {
		want = append(want, file+":3 github.com/permitio/permit-golang/pkg/permit",
			file+":5 permit.Client")
	}
	if !slices.Equal(got, want) {
		t.Errorf("scan =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// failingFS is a file system whose Open fails for the name bad. It has no other method, so that
// fs.ReadDir and fs.ReadFile open each file through it.
type failingFS struct {
	files fstest.MapFS
	bad   string
}

func (f failingFS) Open(name string) (fs.File, error) {
	if name == f.bad {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return f.files.Open(name)
}

func TestScanRejects(t *testing.T) {
	t.Parallel()
	good := &fstest.MapFile{Data: []byte("package p\n")}
	for name, test := range map[string]struct {
		fsys fs.FS
		want string
	}{
		"a file that does not parse": {
			fsys: fstest.MapFS{"a.go": good, "dir/b.go": {Data: []byte("package p\n\nfunc {\n")}},
			want: "parse dir/b.go: dir/b.go:3:6: expected 'IDENT', found '{'",
		},
		"a file that cannot be read": {
			fsys: failingFS{files: fstest.MapFS{"a.go": good, "dir/b.go": good}, bad: "dir/b.go"},
			want: "read dir/b.go: open dir/b.go: permission denied",
		},
		"a directory that cannot be read": {
			fsys: failingFS{files: fstest.MapFS{"a.go": good, "dir/b.go": good}, bad: "dir"},
			want: "read the directory dir: open dir: permission denied",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := scan(test.fsys, v1Packages(t))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("scan = %v, want an error with %q", err, test.want)
			}
		})
	}
}
