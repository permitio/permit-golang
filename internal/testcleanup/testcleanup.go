// Package testcleanup deletes what one run of the integration tests in pkg/tests created in a
// Permit environment. A run builds the key of every object it creates from its run prefix, and
// this package deletes only the objects whose key is that prefix, a hyphen and more characters:
// never an object that another run or a person created.
package testcleanup

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
)

// PrefixEnvVar names the environment variable that holds the run prefix.
const PrefixEnvVar = "PERMIT_TEST_RUN_PREFIX"

const (
	// perPage is the largest page the SDK's list methods accept.
	perPage = 100
	// maxRounds is how many times Run lists and deletes. A delete that fails because another
	// object still depends on the object can succeed in a later round.
	maxRounds = 3
)

var (
	// maxPages stops a list that never ends, for example when the API ignores the page number.
	maxPages = 1000
	// roundDelay is how long Run waits before rounds 2 and 3, so that a round that failed on the
	// API's rate limit is not retried at once.
	roundDelay = 5 * time.Second
)

// prefixPattern is the form of a run prefix: gosdk-ci-<workflow run ID>-<attempt>, which CI sets,
// or gosdk-local-<12 hex digits>, which the tests choose when it is not set. No run prefix and a
// hyphen starts another run prefix, so a run never owns the objects of another run. Any other
// prefix, such as gosdk-ci-7, could match the objects of other runs, so it is refused.
var prefixPattern = regexp.MustCompile(`^gosdk-(ci-[0-9]+-[0-9]+|local-[0-9a-f]{12})$`)

// CheckPrefix returns an error when prefix is not a valid run prefix.
func CheckPrefix(prefix string) error {
	if !prefixPattern.MatchString(prefix) {
		return fmt.Errorf("run prefix %q does not match %s; use for example gosdk-ci-1234-1",
			prefix, prefixPattern)
	}
	return nil
}

// isOwned reports whether key belongs to the run with prefix: whether it is prefix, a hyphen and
// at least one more character. The hyphen keeps run gosdk-ci-7-1 from owning gosdk-ci-7-10-user.
func isOwned(prefix, key string) bool {
	return len(key) > len(prefix)+1 && strings.HasPrefix(key, prefix+"-")
}

// IsNotFound reports whether err is the SDK's error for a 404 response: a PermitError with
// status 404, or the generated client's error that the ResourceActions, ResourceActionGroups and
// ResourceAttributes methods return.
func IsNotFound(err error) bool {
	var permitErr errors.PermitError
	if stderrors.As(err, &permitErr) {
		return permitErr.StatusCode == http.StatusNotFound
	}
	var openapiErr *openapi.GenericOpenAPIError
	if stderrors.As(err, &openapiErr) {
		return strings.HasPrefix(openapiErr.Error(), "404 ")
	}
	return false
}

// object is one listed object: the key that decides whether a run owns it, and the reference
// that its delete method takes.
type object struct {
	key string
	ref string
}

// kind is one type of object that the integration tests create.
type kind struct {
	name   string
	list   func(ctx context.Context, page int) ([]object, error)
	delete func(ctx context.Context, ref string) error
}

// kinds returns the types of object the integration tests create, in the order Run deletes them:
// objects that refer to others come before the objects they refer to. A user's role assignments,
// including those on resource instances, are deleted with the user, and a resource's actions,
// action groups, attributes and roles with the resource.
func kinds(client *api.PermitApiClient) []kind {
	return []kind{
		{
			name: "proxy configs",
			list: func(ctx context.Context, page int) ([]object, error) {
				items, err := client.ProxyConfigs.List(ctx, page, perPage)
				return objects(items, err, func(p models.ProxyConfigRead) object {
					return object{key: p.Key, ref: p.Key}
				})
			},
			delete: client.ProxyConfigs.Delete,
		},
		{
			name: "condition sets",
			list: func(ctx context.Context, page int) ([]object, error) {
				items, err := client.ConditionSets.List(ctx, page, perPage)
				return objects(items, err, func(c models.ConditionSetRead) object {
					return object{key: c.Key, ref: c.Key}
				})
			},
			delete: client.ConditionSets.Delete,
		},
		{
			name: "users",
			list: func(ctx context.Context, page int) ([]object, error) {
				items, err := client.Users.List(ctx, page, perPage)
				return objects(items, err, func(u models.UserRead) object {
					return object{key: u.Key, ref: u.Key}
				})
			},
			delete: client.Users.Delete,
		},
		{
			name: "resource instances",
			list: func(ctx context.Context, page int) ([]object, error) {
				items, err := client.ResourceInstances.List(ctx, page, perPage, "", "", "")
				if err != nil {
					return nil, err
				}
				return objects(*items, nil, func(r models.ResourceInstanceRead) object {
					return object{key: r.Key, ref: r.Id}
				})
			},
			delete: client.ResourceInstances.Delete,
		},
		{
			name: "tenants",
			list: func(ctx context.Context, page int) ([]object, error) {
				items, err := client.Tenants.List(ctx, page, perPage)
				return objects(items, err, func(t models.TenantRead) object {
					return object{key: t.Key, ref: t.Key}
				})
			},
			delete: client.Tenants.Delete,
		},
		{
			name: "roles",
			list: func(ctx context.Context, page int) ([]object, error) {
				items, err := client.Roles.List(ctx, page, perPage)
				return objects(items, err, func(r models.RoleRead) object {
					return object{key: r.Key, ref: r.Key}
				})
			},
			delete: client.Roles.Delete,
		},
		{
			name: "resources",
			list: func(ctx context.Context, page int) ([]object, error) {
				items, err := client.Resources.List(ctx, page, perPage)
				return objects(items, err, func(r models.ResourceRead) object {
					return object{key: r.Key, ref: r.Key}
				})
			},
			delete: client.Resources.Delete,
		},
	}
}

func objects[T any](items []T, err error, toObject func(T) object) ([]object, error) {
	if err != nil {
		return nil, err
	}
	result := make([]object, len(items))
	for i, item := range items {
		result[i] = toObject(item)
	}
	return result, nil
}

// Run deletes every object of the types in kinds whose key the run with prefix owns, and writes
// what it deleted to out. It pages through each list before deleting from it, treats an object
// that is already gone (404) as deleted, and lists again after a round with failures. After a
// round without failures it lists every type once more, and fails when an owned object is still
// listed. The error names each object it could not list or delete, or found left.
func Run(ctx context.Context, client *api.PermitApiClient, prefix string, out io.Writer) error {
	if err := CheckPrefix(prefix); err != nil {
		return err
	}
	return run(ctx, kinds(client), prefix, out)
}

func run(ctx context.Context, kinds []kind, prefix string, out io.Writer) error {
	var failures []string
	for round := 1; round <= maxRounds; round++ {
		if round > 1 {
			if err := wait(ctx, roundDelay); err != nil {
				failures = append(failures, fmt.Sprintf("stopped before round %d: %v", round, err))
				break
			}
		}
		fmt.Fprintf(out, "Round %d: deleting the objects whose key starts with %s-\n",
			round, prefix)
		failures = nil
		for _, k := range kinds {
			failures = append(failures, deleteOwned(ctx, k, prefix, out)...)
		}
		if len(failures) == 0 {
			return checkNoneLeft(ctx, kinds, prefix, out)
		}
	}
	return fmt.Errorf("could not delete every object whose key starts with %s-:\n%s",
		prefix, strings.Join(failures, "\n"))
}

// wait returns after d, or with the error of ctx when ctx ends first.
func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// checkNoneLeft lists every type once more, and returns an error naming each owned object that is
// still listed: one that a list missed because objects moved between its pages, or that a delete
// answered as already gone although it was not.
func checkNoneLeft(ctx context.Context, kinds []kind, prefix string, out io.Writer) error {
	fmt.Fprintf(out, "Checking that no object whose key starts with %s- is left\n", prefix)
	var left []string
	for _, k := range kinds {
		owned, err := listOwned(ctx, k, prefix)
		if err != nil {
			left = append(left, fmt.Sprintf("listing %s: %v", k.name, err))
			continue
		}
		for _, obj := range owned {
			left = append(left, fmt.Sprintf("%s %s is still listed", k.name, obj.key))
		}
	}
	if len(left) > 0 {
		return fmt.Errorf("after deleting, objects whose key starts with %s- are left:\n%s",
			prefix, strings.Join(left, "\n"))
	}
	fmt.Fprintln(out, "  none left")
	return nil
}

func deleteOwned(ctx context.Context, k kind, prefix string, out io.Writer) []string {
	owned, err := listOwned(ctx, k, prefix)
	if err != nil {
		fmt.Fprintf(out, "  %s: could not list them: %v\n", k.name, err)
		return []string{fmt.Sprintf("listing %s: %v", k.name, err)}
	}
	var deleted, gone int
	var failures []string
	for _, obj := range owned {
		err := k.delete(ctx, obj.ref)
		switch {
		case err == nil:
			deleted++
			fmt.Fprintf(out, "  deleted %s\n", obj.key)
		case IsNotFound(err):
			gone++
			fmt.Fprintf(out, "  already deleted: %s\n", obj.key)
		default:
			failures = append(failures,
				fmt.Sprintf("deleting %s %s: %s", k.name, obj.key, reason(err)))
			fmt.Fprintf(out, "  could not delete %s: %s\n", obj.key, reason(err))
		}
	}
	fmt.Fprintf(out, "  %s: %d found, %d deleted, %d already deleted, %d failed\n",
		k.name, len(owned), deleted, gone, len(failures))
	return failures
}

// reason returns the text of err and, when the API answered with an error status, that status and
// the body the API sent, which says why it refused, for example that another object still refers
// to the object. The SDK has already replaced the credentials in that body.
func reason(err error) string {
	var permitErr errors.PermitError
	if !stderrors.As(err, &permitErr) || permitErr.StatusCode == 0 {
		return err.Error()
	}
	answer := fmt.Sprintf("%d %s", permitErr.StatusCode, http.StatusText(permitErr.StatusCode))
	if body := strings.TrimSpace(permitErr.ResponseBody); body != "" {
		answer += ": " + body
	}
	return fmt.Sprintf("%v; the API answered %s", err, answer)
}

// listOwned pages through the list of k and returns the objects that the run with prefix owns.
// It reads every page before anything is deleted, so deletes cannot shift objects onto pages it
// has already read.
func listOwned(ctx context.Context, k kind, prefix string) ([]object, error) {
	var owned []object
	for page := 1; page <= maxPages; page++ {
		items, err := k.list(ctx, page)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		for _, item := range items {
			if isOwned(prefix, item.key) {
				owned = append(owned, item)
			}
		}
		if len(items) < perPage {
			return owned, nil
		}
	}
	return nil, fmt.Errorf("stopped after %d pages of %d", maxPages, perPage)
}
