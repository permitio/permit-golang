package main

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDeclName(t *testing.T) {
	for decl, want := range map[string]struct {
		name   string
		isType bool
	}{
		`const Version = "2.0.0"`:                                  {"Version", false},
		"const Max int = 3":                                        {"Max", false},
		"var ErrNotFound = errors.New(\"not found\")":              {"ErrNotFound", false},
		"var DefaultClient *Client":                                {"DefaultClient", false},
		"func New(apiKey string, opts ...Option) (*Client, error)": {"New", false},
		"func Map[T any, U any](in []T, f func(T) U) []U":          {"Map", false},
		"func Run()": {"Run", false},
		"method (*UsersService) Get(ctx context.Context) (*User, error)": {
			"UsersService.Get", false,
		},
		"method (Client) Close() error":                      {"Client.Close", false},
		"method (*Opt[T]) Get() (T, bool)":                   {"Opt.Get", false},
		"method (Pair[K, V]) Keys() []K":                     {"Pair.Keys", false},
		"type Client struct":                                 {"Client", true},
		"type Opt[T any] struct":                             {"Opt", true},
		"type Pair[K comparable, V any] struct":              {"Pair", true},
		"type Bytes[T ~[]byte] struct":                       {"Bytes", true},
		"type Option func(*Client)":                          {"Option", true},
		"type Alias = models.User":                           {"Alias", true},
		"type Reader interface":                              {"Reader", true},
		"type Client struct, Users *UsersService":            {"Client.Users", false},
		"type Client struct, embedded *Base":                 {"Client.Base", false},
		"type Client struct, embedded models.Base":           {"Client.Base", false},
		"type Opt[T any] struct, Value T":                    {"Opt.Value", false},
		"type Reader interface, Read(p []byte) (int, error)": {"Reader.Read", false},
		"type Reader interface, io.Closer":                   {"Reader.Closer", false},
		"type Number interface, ~int | ~float64":             {"", false},
	} {
		name, isType, ok := declName(decl)
		if !ok || name != want.name || isType != want.isType {
			t.Errorf("declName(%q) = %q, %v, %v, want %q, %v, true", decl, name, isType, ok,
				want.name, want.isType)
		}
	}
	for _, decl := range []string{
		"", "interface Client", "type", "type 1Client struct", "func (s *S) M()", "method M()",
		"method (*S) (x)", "const", "var = 1", "types Client struct", "func New",
	} {
		if name, _, ok := declName(decl); ok {
			t.Errorf("declName(%q) = %q, true, want false", decl, name)
		}
	}
}

func TestReadV2Surface(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{v2SurfaceFile: "# The exported API.\n\n" +
		"models: type User struct\nmodels: type User struct, Key string\n" +
		"models: func NewUser() *User\nmodels: type Kind string\n" +
		"permit: const Version = \"2.0.0\"\npermit: method (*UsersService) Get()\n" +
		"permit: type Number interface, ~int | ~float64\n"})
	s, err := readV2Surface(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{
		"models.Kind", "models.NewUser", "models.User", "models.User.Key",
		"permit.UsersService.Get", "permit.Version",
	}
	if got := slices.Sorted(maps.Keys(s.names)); !slices.Equal(got, wantNames) {
		t.Errorf("names = %q, want %q", got, wantNames)
	}
	wantModels := []string{"models.Kind", "models.User"}
	if got := slices.Sorted(maps.Keys(s.models)); !slices.Equal(got, wantModels) {
		t.Errorf("models = %q, want %q", got, wantModels)
	}
	for name, content := range map[string]string{
		"a line without a package":  "type Client struct\n",
		"a package that is not one": "per mit: type Client struct\n",
		"an unknown declaration":    "permit: interface Client\n",
	} {
		writeFiles(t, dir, map[string]string{v2SurfaceFile: content})
		_, readErr := readV2Surface(dir)
		if readErr == nil || !strings.Contains(readErr.Error(), "is not <package>: <declaration>") {
			t.Errorf("%s: readV2Surface error = %v, want an unknown line", name, readErr)
		}
	}
	_, err = readV2Surface(filepath.Join(dir, "none"))
	if err == nil || !strings.Contains(err.Error(), "reading v2's surface: open ") {
		t.Errorf("readV2Surface of a missing file: error = %v", err)
	}
}
