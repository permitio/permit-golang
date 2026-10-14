package surface_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/surface"
)

// writePackage writes files, a map from file name to source, into a new directory and returns
// the directory.
func writePackage(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// read lists the package of the one file src.
func read(t *testing.T, src string) *surface.Package {
	t.Helper()
	p, err := surface.Read(writePackage(t, map[string]string{"p.go": src}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return p
}

// lines returns the lines of the items of p, in order.
func lines(p *surface.Package) []string {
	out := make([]string, len(p.Items))
	for i, item := range p.Items {
		out[i] = item.Line
	}
	return out
}

// item returns the item of p whose line is line.
func item(t *testing.T, p *surface.Package, line string) surface.Item {
	t.Helper()
	for _, it := range p.Items {
		if it.Line == line {
			return it
		}
	}
	t.Fatalf("no item has the line %q; the lines are %q", line, lines(p))
	return surface.Item{}
}

func TestReadValues(t *testing.T) {
	p := read(t, `package p

import "errors"

const Version = "1.0.0"

const (
	hidden Kind = iota
	First
	Second
	_
	Fourth
)

const (
	KB = 1 << (10 * (iota + 1))
	MB
)

const Typed, Other Kind = 7, 8

type Kind int

var ErrMissing = errors.New("missing")

var Default *Kind

var A, b, C = 1, "b", 2.5

var hiddenVar = 1
`)
	want := []string{
		"const First Kind = 1",
		"const Fourth Kind = 4",
		"const KB = 1 << (10 * (0 + 1))",
		"const MB = 1 << (10 * (1 + 1))",
		"const Other Kind = 8",
		"const Second Kind = 2",
		"const Typed Kind = 7",
		`const Version = "1.0.0"`,
		"type Kind int",
		"var A = 1",
		"var C = 2.5",
		"var Default *Kind",
		`var ErrMissing = errors.New("missing")`,
	}
	if p.Name != "p" {
		t.Errorf("Name is %q, want p", p.Name)
	}
	if diff := cmp.Diff(want, lines(p)); diff != "" {
		t.Errorf("lines differ (-want +got):\n%s", diff)
	}
	for line, want := range map[string]surface.Item{
		`const Version = "1.0.0"`: {Kind: surface.Const, Name: "Version"},
		"var Default *Kind":       {Kind: surface.Var, Name: "Default"},
		`var ErrMissing = errors.New("missing")`: {
			Kind: surface.Var, Name: "ErrMissing", Uses: []string{"errors"},
		},
		"type Kind int": {Kind: surface.Type, Name: "Kind"},
	} {
		want.Line = line
		if diff := cmp.Diff(want, item(t, p, line)); diff != "" {
			t.Errorf("item %q differs (-want +got):\n%s", line, diff)
		}
	}
}

func TestReadTypes(t *testing.T) {
	p := read(t, `package p

import (
	"context"
	"io"
)

type Client struct {
	Users     *UsersService
	Name, Tag string
	secret    string
	*Base
	base
	io.Reader
	Opt[int]
	Pair[string, int]
}

type Base struct{}

type base struct{}

type UsersService struct{ c *Client }

type Pager interface {
	io.Closer
	Next(ctx context.Context) bool
	Page() (int, error)
	reset()
}

type Number interface {
	~int | ~float64
}

type Opt[T any] struct{ Value T }

type Pair[K comparable, V any] struct{}

type ID = string

type Handler func(context.Context) error

type hidden struct{ Exported int }
`)
	want := []string{
		"type Base struct",
		"type Client struct",
		"type Client struct, Name string",
		"type Client struct, Tag string",
		"type Client struct, Users *UsersService",
		"type Client struct, embedded *Base",
		"type Client struct, embedded Opt[int]",
		"type Client struct, embedded Pair[string, int]",
		"type Client struct, embedded io.Reader",
		"type Handler func(context.Context) error",
		"type ID = string",
		"type Number interface",
		"type Number interface, ~int | ~float64",
		"type Opt[T any] struct",
		"type Opt[T any] struct, Value T",
		"type Pager interface",
		"type Pager interface, Next(ctx context.Context) bool",
		"type Pager interface, Page() (int, error)",
		"type Pager interface, io.Closer",
		"type Pair[K comparable, V any] struct",
		"type UsersService struct",
	}
	if diff := cmp.Diff(want, lines(p)); diff != "" {
		t.Errorf("lines differ (-want +got):\n%s", diff)
	}
	member := func(recv, name string) surface.Item {
		return surface.Item{Kind: surface.Member, Recv: recv, Name: name}
	}
	for line, want := range map[string]surface.Item{
		"type Client struct": {
			Kind: surface.Type, Name: "Client", Unlisted: []string{"io.Reader"},
		},
		"type Client struct, Tag string":                 member("Client", "Tag"),
		"type Client struct, embedded *Base":             member("Client", "Base"),
		"type Client struct, embedded Opt[int]":          member("Client", "Opt"),
		"type Client struct, embedded Pair[string, int]": member("Client", "Pair"),
		"type Client struct, embedded io.Reader": {
			Kind: surface.Member, Recv: "Client", Name: "Reader", Uses: []string{"io"},
		},
		"type Number interface, ~int | ~float64": member("Number", ""),
		"type Pager interface, Next(ctx context.Context) bool": {
			Kind: surface.Member, Recv: "Pager", Name: "Next", Uses: []string{"context"},
		},
		"type Handler func(context.Context) error": {
			Kind: surface.Type, Name: "Handler", Uses: []string{"context"},
		},
		"type ID = string": {Kind: surface.Type, Name: "ID"},
	} {
		want.Line = line
		if diff := cmp.Diff(want, item(t, p, line)); diff != "" {
			t.Errorf("item %q differs (-want +got):\n%s", line, diff)
		}
	}
}

func TestReadFuncsAndMethods(t *testing.T) {
	p := read(t, `package p

import (
	"context"

	rest "example.com/sdk/internal/wire"
)

type Client struct {
	*Base
	base
}

type Base struct{}

func (*Base) Close() error { return nil }

type base struct{}

func (base) Promoted(ctx context.Context) {}

func (base) hidden() {}

type UsersService struct{ c *Client }

func (s *UsersService) Get(ctx context.Context, userKey string) (*User, error) {
	return nil, nil
}

func (s UsersService) List(ctx context.Context, page, perPage int) (users []User, err error) {
	return nil, nil
}

func (s *UsersService) Delete(context.Context, string) error { return nil }

func (s *UsersService) hidden() {}

type User struct{}

type Opt[T any] struct{}

func (o *Opt[T]) Get() (T, bool) {
	var zero T
	return zero, false
}

type Option func(*Client)

func New(apiKey string, opts ...Option) (*Client, error) { return nil, nil }

func WithName(name string) Option { return nil }

func Map[T, U any](in []T, f func(T) U) []U { return nil }

func Count() (n int) { return 0 }

func Wire() *rest.Op { return nil }

func Run() {}

func helper() {}

type hidden struct{}

func (hidden) Exported() {}
`)
	want := []string{
		"func Count() (n int)",
		"func Map[T any, U any](in []T, f func(T) U) []U",
		"func New(apiKey string, opts ...Option) (*Client, error)",
		"func Run()",
		"func Wire() *rest.Op",
		"func WithName(name string) Option",
		"method (*Base) Close() error",
		"method (*Opt[T]) Get() (T, bool)",
		"method (*UsersService) Delete(context.Context, string) error",
		"method (*UsersService) Get(ctx context.Context, userKey string) (*User, error)",
		"method (Client) Close() error",
		"method (Client) Promoted(ctx context.Context)",
		"method (UsersService) List(ctx context.Context, page int, perPage int) " +
			"(users []User, err error)",
		"type Base struct",
		"type Client struct",
		"type Client struct, embedded *Base",
		"type Opt[T any] struct",
		"type Option func(*Client)",
		"type User struct",
		"type UsersService struct",
	}
	if diff := cmp.Diff(want, lines(p)); diff != "" {
		t.Errorf("lines differ (-want +got):\n%s", diff)
	}
	ctx := surface.Param{Name: "ctx", Type: "context.Context"}
	for line, want := range map[string]surface.Item{
		"func New(apiKey string, opts ...Option) (*Client, error)": {
			Kind: surface.Func, Name: "New",
			Params: []surface.Param{
				{Name: "apiKey", Type: "string"}, {Name: "opts", Type: "...Option"},
			},
		},
		"func Wire() *rest.Op": {
			Kind: surface.Func, Name: "Wire", Uses: []string{"example.com/sdk/internal/wire"},
		},
		"method (*UsersService) Get(ctx context.Context, userKey string) (*User, error)": {
			Kind: surface.Method, Recv: "UsersService", Name: "Get",
			Params: []surface.Param{ctx, {Name: "userKey", Type: "string"}},
			Uses:   []string{"context"},
		},
		"method (UsersService) List(ctx context.Context, page int, perPage int) " +
			"(users []User, err error)": {
			Kind: surface.Method, Recv: "UsersService", Name: "List",
			Params: []surface.Param{
				ctx, {Name: "page", Type: "int"}, {Name: "perPage", Type: "int"},
			},
			Uses: []string{"context"},
		},
		"method (*UsersService) Delete(context.Context, string) error": {
			Kind: surface.Method, Recv: "UsersService", Name: "Delete",
			Params: []surface.Param{{Type: "context.Context"}, {Type: "string"}},
			Uses:   []string{"context"},
		},
		"method (Client) Promoted(ctx context.Context)": {
			Kind: surface.Method, Recv: "Client", Name: "Promoted",
			Params: []surface.Param{ctx}, Uses: []string{"context"},
		},
		"method (*Opt[T]) Get() (T, bool)": {Kind: surface.Method, Recv: "Opt", Name: "Get"},
	} {
		want.Line = line
		if diff := cmp.Diff(want, item(t, p, line)); diff != "" {
			t.Errorf("item %q differs (-want +got):\n%s", line, diff)
		}
	}
}

// TestReadEmbedded checks what a struct lists from the types it embeds: the methods of the
// interfaces of its own package, exported or not and at any depth, and the types whose methods
// Read cannot list.
func TestReadEmbedded(t *testing.T) {
	p := read(t, `package p

import (
	"context"
	"io"

	"example.com/sdk/models"
)

type UsersService struct {
	getter
	*core
	Lister
	finder
}

func (s *UsersService) Get(ctx context.Context, userKey string) error { return nil }

type getter interface {
	Get(ctx context.Context, userKey string) error
	Delete(ctx context.Context, userKey string) error
	reset()
	io.Closer
}

type core struct {
	pager
	error
}

type pager interface {
	Next(ctx context.Context) bool
}

type Lister interface {
	List(ctx context.Context) ([]models.User, error)
	Next(ctx context.Context) bool
	pager
}

type finder = lookup

type lookup interface {
	Find(key string) error
}

type Node struct {
	*Node
	*models.Base
}

type Page struct{ cursor }

type cursor struct{ next int }

func (cursor) More() bool { return false }
`)
	want := []string{
		"method (*UsersService) Get(ctx context.Context, userKey string) error",
		"method (Page) More() bool",
		"method (UsersService) Delete(ctx context.Context, userKey string) error",
		"method (UsersService) Find(key string) error",
		"method (UsersService) List(ctx context.Context) ([]models.User, error)",
		"method (UsersService) Next(ctx context.Context) bool",
		"type Lister interface",
		"type Lister interface, List(ctx context.Context) ([]models.User, error)",
		"type Lister interface, Next(ctx context.Context) bool",
		"type Lister interface, pager",
		"type Node struct",
		"type Node struct, embedded *Node",
		"type Node struct, embedded *models.Base",
		"type Page struct",
		"type UsersService struct",
		"type UsersService struct, embedded Lister",
	}
	if diff := cmp.Diff(want, lines(p)); diff != "" {
		t.Errorf("lines differ (-want +got):\n%s", diff)
	}
	ctx := surface.Param{Name: "ctx", Type: "context.Context"}
	for line, want := range map[string]surface.Item{
		"type UsersService struct": {
			Kind: surface.Type, Name: "UsersService", Unlisted: []string{"error", "io.Closer"},
		},
		"type Node struct": {Kind: surface.Type, Name: "Node", Unlisted: []string{"models.Base"}},
		"type Page struct": {Kind: surface.Type, Name: "Page"},
		"method (UsersService) Next(ctx context.Context) bool": {
			Kind: surface.Method, Recv: "UsersService", Name: "Next",
			Params: []surface.Param{ctx}, Uses: []string{"context"},
		},
		"method (UsersService) List(ctx context.Context) ([]models.User, error)": {
			Kind: surface.Method, Recv: "UsersService", Name: "List",
			Params: []surface.Param{ctx}, Uses: []string{"context", "example.com/sdk/models"},
		},
		"method (UsersService) Find(key string) error": {
			Kind: surface.Method, Recv: "UsersService", Name: "Find",
			Params: []surface.Param{{Name: "key", Type: "string"}},
		},
	} {
		want.Line = line
		if diff := cmp.Diff(want, item(t, p, line)); diff != "" {
			t.Errorf("item %q differs (-want +got):\n%s", line, diff)
		}
	}
}

// TestReadAccepts checks declarations that come close to the errors of Read but are fine: an
// exported alias of a type of another package, which its own line shows, unexported aliases of
// types that mention no other package, an embedded unexported type without exported fields that
// embeds itself, and aliases of each other, which Go rejects but Read must still get through.
func TestReadAccepts(t *testing.T) {
	p := read(t, `package p

import "context"

type Ctx = context.Context

type ids = []string

type set = ids

func Do(c Ctx, keys set) ids { return nil }

type a = b

type b = a

func Loop() a { return nil }

type Client struct {
	*loop
	quiet
}

type loop struct {
	*loop
	count int
}

type quiet interface{ hush() }
`)
	want := []string{
		"func Do(c Ctx, keys set) ids",
		"func Loop() a",
		"type Client struct",
		"type Ctx = context.Context",
	}
	if diff := cmp.Diff(want, lines(p)); diff != "" {
		t.Errorf("lines differ (-want +got):\n%s", diff)
	}
}

// TestReadUses checks that an item names the import paths of the packages its line mentions,
// looked up in the imports of its own file.
func TestReadUses(t *testing.T) {
	dir := writePackage(t, map[string]string{
		"a.go": `package p

import (
	"context"
	_ "embed"
	"net/http"
	"time"

	cfg "example.com/x/config"
)

func Do(ctx context.Context, c *http.Client) cfg.Value { return cfg.Value{} }

var Default = cfg.Default

var Started = time.Now().Unix

var Timeout = settings.timeout

var settings = struct{ timeout int }{}
`,
		"b.go": `package p

import "example.com/fake/http"

func Fake() http.Client { return http.Client{} }
`,
	})
	p, err := surface.Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for line, want := range map[string][]string{
		"func Do(ctx context.Context, c *http.Client) cfg.Value": {
			"context", "example.com/x/config", "net/http",
		},
		"var Default = cfg.Default":      {"example.com/x/config"},
		"var Started = time.Now().Unix":  {"time"},
		"var Timeout = settings.timeout": nil,
		"func Fake() http.Client":        {"example.com/fake/http"},
	} {
		if diff := cmp.Diff(want, item(t, p, line).Uses); diff != "" {
			t.Errorf("Uses of %q differ (-want +got):\n%s", line, diff)
		}
	}
}

// TestReadFiles checks that Read lists the Go files of the package, without the tests and the
// files that the go command leaves out on every machine.
func TestReadFiles(t *testing.T) {
	dir := writePackage(t, map[string]string{
		"p.go":              "package p\n\nfunc Listed() {}\n",
		"p_test.go":         "package p\n\nfunc FromATest() {}\n",
		"p_windows_test.go": "//go:build cgo\n\npackage p\n\nfunc FromAWindowsTest() {}\n",
		"ignored.go":        "//go:build ignore\n\npackage main\n\nfunc Ignored() {}\n",
		"generator.go":      "// +build ignore\n\npackage main\n\nfunc {\n",
		"_hidden.go":        "package q\n\nfunc Hidden() {}\n",
		".hidden.go":        "package q\n\nfunc DotHidden() {}\n",
		"notes.txt":         "package p\n\nfunc NotGo() {}\n",
		"external.go.x":     "package p\n\nfunc NotGoEither() {}\n",
	})
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "sub.go"),
		[]byte("package sub\n\nfunc InSub() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := surface.Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if diff := cmp.Diff([]string{"func Listed()"}, lines(p)); diff != "" {
		t.Errorf("lines differ (-want +got):\n%s", diff)
	}
}

// TestReadEmptyPackage checks that a package without exported declarations lists nothing.
func TestReadEmptyPackage(t *testing.T) {
	p := read(t, "// Package p has no API.\npackage p\n\nfunc hidden() {}\n")
	if p.Name != "p" || p.Items != nil {
		t.Errorf("Read returned %+v, want package p with no items", p)
	}
}

func TestReadErrors(t *testing.T) {
	const pairFunc = "func pair() (int, int) { return 1, 2 }\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "no Go files",
			files: map[string]string{"README.md": "# p\n"},
			want:  "has no Go file to build outside the tests; give the directory of a package",
		},
		{
			name:  "only tests",
			files: map[string]string{"p_test.go": "package p\n"},
			want:  "has no Go file to build outside the tests",
		},
		{
			name:  "syntax error",
			files: map[string]string{"p.go": "package p\n\nfunc {\n"},
			want:  "p.go:3:6: expected 'IDENT', found '{'",
		},
		{
			name: "two packages",
			files: map[string]string{
				"a.go": "package a\n",
				"b.go": "package b\n",
			},
			want: "holds package a and package b; give the directory of one package",
		},
		{
			name:  "dot import",
			files: map[string]string{"p.go": "package p\n\nimport . \"strings\"\n"},
			want:  "p.go:3:8: \"strings\" is a dot import",
		},
		{
			name: "inline struct type",
			files: map[string]string{"p.go": `package p

type T struct {
	Field struct {
		A int
		B int
	}
}
`},
			want: "p.go:4:8: struct { ... does not fit on one line; declare a named type",
		},
		{
			name:  "value on several lines",
			files: map[string]string{"p.go": "package p\n\nconst Text = `a\nb`\n"},
			want:  "p.go:3:14: `a ... does not fit on one line",
		},
		{
			name: "variables of one call",
			files: map[string]string{
				"p.go": "package p\n\nvar first, Second = pair()\n\n" + pairFunc,
			},
			want: "p.go:3:12: var Second has no value of its own; declare its type",
		},
		{
			name: "method of an undeclared type",
			files: map[string]string{
				"p.go": "package p\n\ntype A struct{ B }\n\nfunc (B) Method() {}\n",
			},
			want: "p.go:5:1: method Method has a receiver type that the package does not declare",
		},
		{
			name: "first error",
			files: map[string]string{
				"p.go": "package p\n\nvar A, B = pair()\n\n" + pairFunc,
			},
			want: "p.go:3:5: var A has no value",
		},
		{
			name: "an embedded type with an exported field",
			files: map[string]string{
				"p.go": "package p\n\ntype Client struct{ core }\n\ntype core struct{ Name int }\n",
			},
			want: "p.go:3:21: Client embeds the unexported type core, which has exported fields; " +
				"embed an exported type or declare the fields on Client",
		},
		{
			name: "an embedded type that embeds an exported type",
			files: map[string]string{"p.go": `package p

type Client struct {
	*core
}

type core struct{ *Base }

type Base struct{}
`},
			want: "p.go:4:2: Client embeds the unexported type core, which has exported fields",
		},
		{
			name: "an exported field two embeddings down",
			files: map[string]string{"p.go": `package p

type Client struct{ core }

type core struct {
	inner
	hidden int
}

type inner struct{ Name string }
`},
			want: "p.go:3:21: Client embeds the unexported type core, which has exported fields",
		},
		{
			name: "an embedded alias of a struct with an exported field",
			files: map[string]string{"p.go": `package p

type Client struct{ core }

type core = inner

type inner struct{ Name string }
`},
			want: "p.go:3:21: Client embeds the unexported type core, which has exported fields",
		},
		{
			name: "an embedded type defined as a type of another package",
			files: map[string]string{"p.go": `package p

import "example.com/sdk/models"

type Client struct{ base }

type base models.Base
`},
			want: "p.go:5:21: Client embeds the unexported type base, which has exported fields",
		},
		{
			name: "an unexported alias of a type of another package",
			files: map[string]string{"p.go": `package p

import "example.com/sdk/internal/surface"

type pkg = surface.Package

func Planted() *pkg { return nil }
`},
			want: "p.go:7:17: pkg is an unexported alias of a type that mentions another " +
				"package, which the list would not show; use the type itself",
		},
		{
			name: "an alias of an alias of a type of another package",
			files: map[string]string{"p.go": `package p

import "context"

type ctx = Ctx

type Ctx = []context.Context

func Do(c ctx) {}
`},
			want: "p.go:9:11: ctx is an unexported alias of a type that mentions another package",
		},
		{
			name: "an embedded alias of a type of another package",
			files: map[string]string{"p.go": `package p

import "io"

type Client struct{ closer }

type closer = io.Closer
`},
			want: "p.go:5:21: closer is an unexported alias of a type that mentions another",
		},
		{
			name: "a file for one operating system",
			files: map[string]string{
				"p.go":         "package p\n",
				"p_windows.go": "package p\n\nfunc OnWindows() {}\n",
			},
			want: "p_windows.go: the file name names an operating system or an architecture, so " +
				"whether a build includes the file depends on the machine; move its declarations " +
				"to a file that every machine builds",
		},
		{
			name:  "a file for one architecture",
			files: map[string]string{"p_arm64.go": "package p\n"},
			want:  "p_arm64.go: the file name names an operating system or an architecture",
		},
		{
			name:  "a build constraint",
			files: map[string]string{"p.go": "//go:build linux\n\npackage p\n"},
			want: "p.go:1:1: //go:build linux makes whether a build includes the file depend on " +
				"the machine or the toolchain; move its declarations to a file without a build " +
				"constraint, or leave the file out with //go:build ignore",
		},
		{
			name:  "cgo",
			files: map[string]string{"p.go": "// Package p.\n\n//go:build cgo\n\npackage p\n"},
			want:  "p.go:3:1: //go:build cgo makes whether a build includes the file depend",
		},
		{
			name:  "a constraint that most machines meet",
			files: map[string]string{"p.go": "//go:build !windows\n\npackage p\n"},
			want:  "p.go:1:1: //go:build !windows makes whether",
		},
		{
			name:  "a +build line",
			files: map[string]string{"p.go": "// +build linux\n\npackage p\n"},
			want:  "p.go:1:1: // +build linux makes whether",
		},
		{
			name:  "ignore with another tag",
			files: map[string]string{"p.go": "//go:build ignore && linux\n\npackage p\n"},
			want:  "p.go:1:1: //go:build ignore && linux makes whether",
		},
		{
			name:  "a malformed constraint",
			files: map[string]string{"p.go": "//go:build linux &&\n\npackage p\n"},
			want:  "p.go:1:1: unexpected end of expression",
		},
		{
			name:  "no package clause",
			files: map[string]string{"p.go": "func F() {}\n"},
			want:  "p.go:1:1: expected 'package', found 'func'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := surface.Read(writePackage(t, tc.files))
			if err == nil || !strings.HasPrefix(err.Error(), "surface: ") ||
				!strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Read returned %+v, %v; want an error that starts with surface: and "+
					"contains %q", p, err, tc.want)
			}
		})
	}
}

func TestReadMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	p, err := surface.Read(dir)
	if err == nil || !strings.Contains(err.Error(), "surface: open "+dir) {
		t.Fatalf("Read returned %+v, %v; want an error that names the directory", p, err)
	}
}

// TestKindsAreNotZero checks that the zero Kind is none of the kinds, so an Item that was never
// filled in has no kind.
func TestKindsAreNotZero(t *testing.T) {
	var zero surface.Kind
	for _, k := range []surface.Kind{
		surface.Const, surface.Var, surface.Type, surface.Member, surface.Func, surface.Method,
	} {
		if k == zero {
			t.Errorf("a kind is the zero Kind")
		}
	}
}
