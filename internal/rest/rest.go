// Package rest holds the registry of the operations that the SDK sends to the Permit API and the
// PDP. An [Op] holds what the SDK knows about one operation: the ID that the API's spec gives it,
// its method and route, and the facts that the transport and the checks before a request read,
// such as whether a retry may send it again. One table holds them all, so that no fact is kept
// in a list of its own that could fall out of step with the others.
//
// A permit.Client holds a [*Registry], [Default] in production. The registry of a test client
// also holds test-only operations, whose routes start with [TestPrefix]; Default never holds
// one, so none reaches an application.
package rest

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/permitio/permit-golang/v2/internal/testhook"
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// TestPrefix starts the route of every test-only operation, and of no operation of the SDK.
const TestPrefix = testhook.TestPrefix

// KeyLevel is the level of the API key that an operation needs: the narrowest key that may call
// it. A key of a wider level may call it too, so an organization key may call every operation.
type KeyLevel int

const (
	// The zero KeyLevel is no level, so that an Op that does not set one is not valid.
	_ KeyLevel = iota

	// EnvironmentLevel is a key of the operation's environment, or a wider one.
	EnvironmentLevel

	// ProjectLevel is a key of the operation's project, or of its organization.
	ProjectLevel

	// OrganizationLevel is a key of the organization.
	OrganizationLevel
)

// Op is one operation of the Permit API or the PDP that the SDK sends.
type Op struct {
	// ID is the operationId of the operation in the API's spec, such as get_user, and its op in
	// tools/specs/coverage.yaml.
	ID string

	// Method is the HTTP method, such as GET.
	Method string

	// Route is the path of the operation in the API's spec, a route template such as
	// /v2/facts/{proj_id}/{env_id}/users/{user_id}, which wire.Path fills in.
	Route string

	// Idempotent says that sending the request twice does what sending it once does, so that a
	// retry may send it again after a 5xx reply or a request without a reply: the GET, PUT and
	// DELETE operations, and the PDP's decisions, which change nothing.
	Idempotent bool

	// Facts says that the operation is one of the facts API, /v2/facts/{proj_id}/{env_id}/...,
	// that the PDP container also serves under /facts, so that facts can go through it.
	Facts bool

	// WaitsForSync says that the PDP container can hold its answer until the change reaches its
	// policy, when the request asks it to wait. Only a Facts operation can wait.
	WaitsForSync bool

	// ContainerOnly says that only the PDP container serves the operation, and the cloud PDP
	// answers 404 with an empty body.
	ContainerOnly bool

	// KeyLevel is the level of the API key that the operation needs.
	KeyLevel KeyLevel

	// DeleteSelf says that the operation is a DELETE whose path names the object it deletes, so
	// that a 404 means that the object is gone already. A removal whose body names what it
	// removes, such as a role assignment, is not one: there, a 404 can mean a mistyped key.
	DeleteSelf bool

	// NoRetry says that the request is never sent twice, not even after a 429, as a second
	// request would undo or repeat what the first did, such as rotating an API key.
	NoRetry bool

	// SensitiveBodyFields are the fields of the operation's bodies that hold a credential under a
	// name that redact.Scrub does not take for one; their values are replaced in the bodies of
	// its error replies. The registry keeps its own copy; callers must not change it.
	SensitiveBodyFields []string

	// MaxPerPage is the largest per_page that a list operation accepts, or 0 for an operation
	// that is not a list.
	MaxPerPage int

	// RequestModel is the name of the type of package models that the request body holds, such
	// as UserCreate, or empty when the request has no body.
	RequestModel string

	// ResponseModel is the name of the type of package models that the reply body holds, such
	// as UserRead, or empty when the SDK reads no body from the reply.
	ResponseModel string
}

// methods are the HTTP methods that an operation can have.
var methods = []string{
	http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
}

// opID matches an operationId: a letter or an underscore, then letters, digits and underscores.
var opID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// check returns an error that says what is wrong with op, or nil when op is valid.
func (op Op) check() error {
	if !opID.MatchString(op.ID) {
		return fmt.Errorf("the ID %q is not an operationId, such as get_user", op.ID)
	}
	if !slices.Contains(methods, op.Method) {
		return fmt.Errorf("the method %q is not one of %s", op.Method, strings.Join(methods, ", "))
	}
	if _, err := wire.Placeholders(op.Route); err != nil {
		return err
	}
	if op.KeyLevel < EnvironmentLevel || op.KeyLevel > OrganizationLevel {
		return fmt.Errorf("the key level %d is not EnvironmentLevel, ProjectLevel or "+
			"OrganizationLevel", op.KeyLevel)
	}
	if op.MaxPerPage < 0 {
		return fmt.Errorf("MaxPerPage is %d; give the largest per_page of a list, or 0",
			op.MaxPerPage)
	}
	if op.DeleteSelf && op.Method != http.MethodDelete {
		return fmt.Errorf("DeleteSelf is set on a %s; only a DELETE deletes its object", op.Method)
	}
	if op.WaitsForSync && !op.Facts {
		return errors.New("WaitsForSync is set without Facts; only a facts operation waits")
	}
	if op.NoRetry && op.Idempotent {
		return errors.New("NoRetry and Idempotent are both set; an operation that is never sent " +
			"twice is not retried")
	}
	return nil
}

// Registry holds operations by their IDs. It does not change once built, so it is safe for
// concurrent use.
type Registry struct {
	ops map[string]Op
}

// defaultRegistry is the registry that Default returns. Building it panics at package init when
// defaultOps holds an operation that is not valid, naming the operation and why, so that the
// tests of every package that imports this one fail at once.
var defaultRegistry = mustRegistry(defaultOps)

// Default returns the registry of the SDK's operations, which every Client that permit.New builds
// holds. It holds no test-only operation.
func Default() *Registry {
	return defaultRegistry
}

// mustRegistry returns the registry of ops, and panics when newRegistry rejects them.
func mustRegistry(ops []Op) *Registry {
	r, err := newRegistry(ops)
	if err != nil {
		panic(err)
	}
	return r
}

// newRegistry returns a registry of ops, or an error that names the first op that is not valid,
// whose ID another op has too, or whose route starts with TestPrefix.
func newRegistry(ops []Op) (*Registry, error) {
	r := &Registry{ops: make(map[string]Op, len(ops))}
	for _, op := range ops {
		if strings.HasPrefix(op.Route, TestPrefix) {
			return nil, fmt.Errorf("rest: the operation %s: the route %s starts with %s, which "+
				"only a test-only operation may use", op.ID, op.Route, TestPrefix)
		}
		if err := r.add(op); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// WithTestOps returns a new registry that holds the operations of r and ops, the test-only
// operations of a test, and leaves r as it is. It returns an error that names the first op that
// is not valid, whose route does not start with TestPrefix, or whose ID is in r or another op
// has too.
func (r *Registry) WithTestOps(ops ...Op) (*Registry, error) {
	t := &Registry{ops: map[string]Op{}}
	maps.Copy(t.ops, r.ops)
	for _, op := range ops {
		if !strings.HasPrefix(op.Route, TestPrefix) {
			return nil, fmt.Errorf("rest: the test-only operation %s: the route %s does not "+
				"start with %s", op.ID, op.Route, TestPrefix)
		}
		if err := t.add(op); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// add adds op to r, with a copy of its SensitiveBodyFields, or returns an error when op is not
// valid or r holds its ID already.
func (r *Registry) add(op Op) error {
	if err := op.check(); err != nil {
		return fmt.Errorf("rest: the operation %q: %w", op.ID, err)
	}
	if _, ok := r.ops[op.ID]; ok {
		return fmt.Errorf("rest: the operation %s is registered already; give each operation "+
			"its own ID", op.ID)
	}
	op.SensitiveBodyFields = slices.Clone(op.SensitiveBodyFields)
	r.ops[op.ID] = op
	return nil
}

// Op returns the operation with the ID id, and false when r holds none.
func (r *Registry) Op(id string) (Op, bool) {
	op, ok := r.ops[id]
	return op, ok
}

// Ops returns the operations of r, sorted by ID.
func (r *Registry) Ops() []Op {
	ops := slices.Collect(maps.Values(r.ops))
	slices.SortFunc(ops, func(a, b Op) int { return strings.Compare(a.ID, b.ID) })
	return ops
}
