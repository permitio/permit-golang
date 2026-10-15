//go:build e2e

package e2e

import (
	"context"
	"errors"
	"maps"
	"net/url"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
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
