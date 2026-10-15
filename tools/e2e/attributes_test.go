//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"slices"
	"strconv"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testhook"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// attributeTypes are the attributes that resourceAttributeLifecycle creates, by their keys: one
// of each type that the SDK knows.
var attributeTypes = map[string]models.AttributeType{
	"archived": models.AttributeTypeBool,
	"pages":    models.AttributeTypeNumber,
	"owner":    models.AttributeTypeString,
	"due":      models.AttributeTypeTime,
	"tags":     models.AttributeTypeArray,
	"settings": models.AttributeTypeJSON,
	"address":  models.AttributeTypeObject,
	"contacts": models.AttributeTypeObjectArray,
}

// resourceAttributeLifecycle checks the calls of the attributes of a resource with the
// environment-level API key of the scratch environment, whose scope the client looks up, on a
// resource of its own, whose key starts with the run's prefix, which it creates with the action
// read and registers with the ledger before it creates it, as it registers each attribute. It
// creates one attribute of each type that the SDK knows, lists a first page of 3 of them with
// List, and every page of 5 with All, to the end, which holds each attribute once with its type,
// gets owner by its key and by its ID, and changes the description of owner, which keeps its
// type. It deletes each attribute, waits until a get of each matches ErrNotFound, and deletes
// owner again, which returns nil.
func resourceAttributeLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		resourceKey := s.cfg.prefix.Key("sdk-attributes")
		self := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
			"/resources/" + url.PathEscape(resourceKey)
		s.ledger.Register("resource "+resourceKey, s.api.Remover(self),
			s.api.Absence(self, framework.AbsenceTimeout))
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		if _, err := c.Resources.Create(ctx, models.ResourceCreate{
			Key: resourceKey, Name: resourceKey,
			Actions: map[string]models.ActionBlockEditable{"read": {}},
		}); err != nil {
			t.Fatalf("setup: Create of the resource %s: %v", resourceKey, err)
		}
		keys := slices.Sorted(maps.Keys(attributeTypes))
		ids := map[string]string{}
		for _, key := range keys {
			path := self + "/attributes/" + url.PathEscape(key)
			s.ledger.Register("attribute "+key+" of "+resourceKey, s.api.Remover(path),
				s.api.Absence(path, framework.AbsenceTimeout))
			created, err := c.ResourceAttributes.Create(ctx, resourceKey,
				models.ResourceAttributeCreate{
					Key: key, Type: attributeTypes[key],
					Description: "An attribute of the Go SDK's tests",
				})
			if err != nil {
				t.Fatalf("Create of the attribute %s of %s: %v", key, resourceKey, err)
			}
			if created.Key != key || created.Type != attributeTypes[key] || created.ID == "" ||
				created.EnvironmentID != s.env.ID {
				t.Fatalf("Create returned the attribute %q (ID %q) of the type %q in the "+
					"environment %q, want %s of the type %s in %s", created.Key, created.ID,
					created.Type, created.EnvironmentID, key, attributeTypes[key], s.env)
			}
			ids[key] = created.ID
		}
		first, err := c.ResourceAttributes.List(ctx, resourceKey, &permit.ListOptions{PerPage: 3})
		if err != nil {
			t.Fatalf("List of the attributes of %s: %v", resourceKey, err)
		}
		if len(first.Data) != 3 {
			t.Errorf("List of a page of 3 attributes returned %d", len(first.Data))
		}
		checkAttributeTypes(t, "List", first.Data)
		var listed []models.ResourceAttributeRead
		for attribute, err := range c.ResourceAttributes.All(ctx, resourceKey,
			&permit.ListOptions{PerPage: 5}) {
			if err != nil {
				t.Fatalf("All of the attributes of %s: %v", resourceKey, err)
			}
			listed = append(listed, attribute)
		}
		checkAttributeTypes(t, "All", listed)
		if got := attributeKeys(listed); !slices.Equal(got, keys) {
			t.Errorf("All listed the attributes %q of the test, want each of %q once", got, keys)
		}
		for _, ref := range []string{"owner", ids["owner"]} {
			got, getErr := c.ResourceAttributes.Get(ctx, resourceKey, ref)
			if getErr != nil {
				t.Fatalf("Get of the attribute %s of %s: %v", ref, resourceKey, getErr)
			}
			if got.ID != ids["owner"] || got.Key != "owner" ||
				got.Type != models.AttributeTypeString {
				t.Errorf("Get of %s returned the attribute %s (ID %s) of the type %s, want owner "+
					"(ID %s) of the type string", ref, got.Key, got.ID, got.Type, ids["owner"])
			}
		}
		updated, err := c.ResourceAttributes.Update(ctx, resourceKey, "owner",
			models.ResourceAttributeUpdate{Description: models.Some("Updated")})
		if err != nil {
			t.Fatalf("Update of the attribute owner of %s: %v", resourceKey, err)
		}
		if updated.Description != "Updated" || updated.Type != models.AttributeTypeString {
			t.Errorf("Update returned the description %q and the type %q, want Updated and "+
				"string", updated.Description, updated.Type)
		}
		for _, key := range keys {
			if err := c.ResourceAttributes.Delete(ctx, resourceKey, key); err != nil {
				t.Fatalf("Delete of the attribute %s of %s: %v", key, resourceKey, err)
			}
		}
		for _, key := range keys {
			gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
				func(ctx context.Context) (bool, error) {
					_, err := c.ResourceAttributes.Get(ctx, resourceKey, ids[key])
					if errors.Is(err, permit.ErrNotFound) {
						return true, nil
					}
					return false, err
				})
			if gone != nil {
				t.Fatalf("Get of the deleted attribute %s (ID %s): %v", key, ids[key], gone)
			}
		}
		if err := c.ResourceAttributes.Delete(ctx, resourceKey, "owner"); err != nil {
			t.Errorf("Delete of the attribute owner that is gone returned %v, want nil", err)
		}
	}
}

// checkAttributeTypes checks that each attribute of attributes that the scenario created has the
// type that it was created with, a type that the SDK knows; call names the call that listed them.
func checkAttributeTypes(t *testing.T, call string, attributes []models.ResourceAttributeRead) {
	t.Helper()
	for _, a := range attributes {
		want, ours := attributeTypes[a.Key]
		if ours && (a.Type != want || !a.Type.Known()) {
			t.Errorf("%s listed the attribute %s of the type %q (known: %t), want %s", call,
				a.Key, a.Type, a.Type.Known(), want)
		}
	}
}

// attributeKeys returns the keys of the attributes of attributes that resourceAttributeLifecycle
// created, sorted, with a key that attributes holds twice appearing twice, and without any other
// attribute.
func attributeKeys(attributes []models.ResourceAttributeRead) []string {
	keys := []string{}
	for _, a := range attributes {
		if _, ours := attributeTypes[a.Key]; ours {
			keys = append(keys, a.Key)
		}
	}
	slices.Sort(keys)
	return keys
}

// userAttributeTypes are the attributes of the users that userAttributeLifecycle creates, by the
// names that their keys end with after the run's prefix: one of each type that v1 could not read.
var userAttributeTypes = map[string]models.AttributeType{
	"sdk-user-address":  models.AttributeTypeObject,
	"sdk-user-contacts": models.AttributeTypeObjectArray,
}

// userAttributeLifecycle checks the calls of the attributes of the users with the
// environment-level API key of the scratch environment, whose scope the client looks up. Its
// attributes have keys that start with the run's prefix, as the users of the environment may have
// other attributes, such as those that are built in, and it registers each with the ledger before
// it creates it: one of the type object and one of the type object_array. It lists a first page
// of one attribute with List, and every page of one with All, to the end, which holds each of its
// attributes once with its type, and gets the first by its key and by its ID. Its subtest update
// changes the description of the first, which keeps its type. While the API answers 500 to an
// update of a user attribute, a live run skips the subtest when Update returns a *permit.Error of
// the status 500, as skipsUserAttributeUpdate says, and tools/e2e/allowed-skips.txt names the
// ticket; any other error fails it. Once the API answers the update with 200, the subtest asserts
// the update and passes, and checkgotest reports that entry as stale, which prompts its removal.
// A dry run runs it against the fake, where any error fails it. It deletes each attribute, waits
// until a get of each matches ErrNotFound, and deletes the first again, which returns nil.
func userAttributeLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		attributes := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" +
			url.PathEscape(s.env.ID) + "/users/attributes/"
		types := map[string]models.AttributeType{}
		ids := map[string]string{}
		for _, name := range slices.Sorted(maps.Keys(userAttributeTypes)) {
			key, want := s.cfg.prefix.Key(name), userAttributeTypes[name]
			self := attributes + url.PathEscape(key)
			s.ledger.Register("user attribute "+key, s.api.Remover(self),
				s.api.Absence(self, framework.AbsenceTimeout))
			created, err := c.UserAttributes.Create(ctx, models.ResourceAttributeCreate{
				Key: key, Type: want, Description: "An attribute of the Go SDK's tests",
			})
			if err != nil {
				t.Fatalf("Create of the user attribute %s: %v", key, err)
			}
			if created.Key != key || created.Type != want || created.ID == "" ||
				created.EnvironmentID != s.env.ID {
				t.Fatalf("Create returned the user attribute %q (ID %q) of the type %q in the "+
					"environment %q, want %s of the type %s in %s", created.Key, created.ID,
					created.Type, created.EnvironmentID, key, want, s.env)
			}
			types[key], ids[key] = want, created.ID
		}
		keys := slices.Sorted(maps.Keys(types))
		first, err := c.UserAttributes.List(ctx, &permit.ListOptions{PerPage: 1})
		if err != nil {
			t.Fatalf("List of the user attributes: %v", err)
		}
		if len(first.Data) != 1 {
			t.Errorf("List of a page of one user attribute returned %d", len(first.Data))
		}
		checkListedUserAttributes(ctx, t, c, types)
		key := keys[0]
		for _, ref := range []string{key, ids[key]} {
			got, getErr := c.UserAttributes.Get(ctx, ref)
			if getErr != nil {
				t.Fatalf("Get of the user attribute %s: %v", ref, getErr)
			}
			if got.ID != ids[key] || got.Key != key || got.Type != types[key] {
				t.Errorf("Get of %s returned the user attribute %s (ID %s) of the type %s, want "+
					"%s (ID %s) of the type %s", ref, got.Key, got.ID, got.Type, key, ids[key],
					types[key])
			}
		}
		t.Run("update", func(t *testing.T) {
			updated, err := c.UserAttributes.Update(ctx, key,
				models.ResourceAttributeUpdate{Description: models.Some("Updated")})
			if skipsUserAttributeUpdate(s.cfg.mode, err) {
				t.Skipf("Update of the user attribute %s: %v; the API answers 500 to an update "+
					"of a user attribute, and tools/e2e/allowed-skips.txt names the ticket", key,
					err)
			}
			if err != nil {
				t.Fatalf("Update of the user attribute %s: %v", key, err)
			}
			if updated.Description != "Updated" || updated.Type != types[key] {
				t.Errorf("Update returned the description %q and the type %q, want Updated and %s",
					updated.Description, updated.Type, types[key])
			}
		})
		for _, k := range keys {
			if err := c.UserAttributes.Delete(ctx, k); err != nil {
				t.Fatalf("Delete of the user attribute %s: %v", k, err)
			}
		}
		for _, k := range keys {
			gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
				func(ctx context.Context) (bool, error) {
					_, err := c.UserAttributes.Get(ctx, ids[k])
					if errors.Is(err, permit.ErrNotFound) {
						return true, nil
					}
					return false, err
				})
			if gone != nil {
				t.Fatalf("Get of the deleted user attribute %s (ID %s): %v", k, ids[k], gone)
			}
		}
		if err := c.UserAttributes.Delete(ctx, key); err != nil {
			t.Errorf("Delete of the user attribute %s that is gone returned %v, want nil", key,
				err)
		}
	}
}

// skipsUserAttributeUpdate reports whether the subtest update of userAttributeLifecycle skips
// after Update returned err: in a live run, when err is a *permit.Error of the status 500, as the
// API answers an update of a user attribute today. Any other error fails the subtest, and so does
// any error in a dry run.
func skipsUserAttributeUpdate(mode testhook.Mode, err error) bool {
	perr, ok := errors.AsType[*permit.Error](err)
	return mode == testhook.Live && ok && perr.StatusCode == http.StatusInternalServerError
}

// TestSkipsUserAttributeUpdate checks the guard of the subtest update of userAttributeLifecycle
// on the errors of Update against a local server that answers 500 or 422: a live run skips after
// the 500 alone, also when an error wraps it, and not after the 422, another error or none; a dry
// run skips after none of them.
func TestSkipsUserAttributeUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, err := strconv.Atoi(path.Base(r.URL.Path))
		if err != nil || r.Method != http.MethodPatch {
			status = http.StatusBadRequest
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"detail":"a reply of the test"}`)
	}))
	t.Cleanup(server.Close)
	c, err := permit.New(permit.WithAPIKey(canary.Key("e2eupdateguard")),
		permit.WithAPIURL(server.URL), permit.WithPDPURL(server.URL), permit.WithScope("p", "e"),
		permit.WithRetry(permit.RetryPolicy{}))
	if err != nil {
		t.Fatalf("permit.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	update := func(status int) error {
		_, err := c.UserAttributes.Update(t.Context(), strconv.Itoa(status),
			models.ResourceAttributeUpdate{Description: models.Some("Updated")})
		if perr, ok := errors.AsType[*permit.Error](err); !ok || perr.StatusCode != status {
			t.Fatalf("Update against a server that answers %d returned %v, want a "+
				"*permit.Error of that status", status, err)
		}
		return err
	}
	internal, invalid := update(http.StatusInternalServerError),
		update(http.StatusUnprocessableEntity)
	for _, tc := range []struct {
		name string
		mode testhook.Mode
		err  error
		want bool
	}{
		{"live 500", testhook.Live, internal, true},
		{"live wrapped 500", testhook.Live, fmt.Errorf("the update: %w", internal), true},
		{"live 422", testhook.Live, invalid, false},
		{"live other error", testhook.Live, context.DeadlineExceeded, false},
		{"live no error", testhook.Live, nil, false},
		{"dry run 500", testhook.DryRun, internal, false},
		{"dry run 422", testhook.DryRun, invalid, false},
		{"dry run no error", testhook.DryRun, nil, false},
	} {
		if got := skipsUserAttributeUpdate(tc.mode, tc.err); got != tc.want {
			t.Errorf("%s: skipsUserAttributeUpdate(%q, %v) = %t, want %t", tc.name, tc.mode,
				tc.err, got, tc.want)
		}
	}
}

// checkListedUserAttributes checks that All of the user attributes, a page of one at a time up to
// the end of its iterator, holds each attribute of types, those that userAttributeLifecycle
// created, once and with its type, a type that the SDK knows.
func checkListedUserAttributes(ctx context.Context, t *testing.T, c *permit.Client,
	types map[string]models.AttributeType,
) {
	t.Helper()
	var got []string
	for a, err := range c.UserAttributes.All(ctx, &permit.ListOptions{PerPage: 1}) {
		if err != nil {
			t.Fatalf("All of the user attributes: %v", err)
		}
		want, ours := types[a.Key]
		if !ours {
			continue
		}
		got = append(got, a.Key)
		if a.Type != want || !a.Type.Known() {
			t.Errorf("All listed the user attribute %s of the type %q (known: %t), want %s",
				a.Key, a.Type, a.Type.Known(), want)
		}
	}
	slices.Sort(got)
	if want := slices.Sorted(maps.Keys(types)); !slices.Equal(got, want) {
		t.Errorf("All listed the user attributes %q of the test, want each of %q once", got, want)
	}
}
