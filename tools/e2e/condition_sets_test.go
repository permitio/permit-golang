//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/url"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// conditionSetCount is the number of user sets that conditionSetLifecycle creates: more than one
// page of 30, so that All reads a second page.
const conditionSetCount = 31

// conditionSetLifecycle checks the calls of the condition sets with the environment-level API
// key of the scratch environment, whose scope the client looks up, on a resource and sets of its
// own, whose keys start with the run's prefix, each of which it registers with the ledger before
// it creates it. It creates 31 user sets, and checks that All, at 30 sets on a page, lists each of
// them once, and that List returns a page of sets; the environment can hold other sets, such as
// those that the API creates with a role. It creates a resource and a resource set of it, which
// All of the resource sets lists, and checks a condition set rule that grants the users of the
// first user set the action read of the resource on the resource set, as conditionSetRules says.
// It gets the first user set by its key and by its ID, makes the second its child, and reads the
// ancestors of the child, the first set alone, and the descendants of the first set, the child
// alone, each with the call of a page and with its iterator. It changes the name and the
// description of the child, deletes it, waits until a get matches ErrNotFound, and deletes it
// again, which returns nil.
func conditionSetLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		schema := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID)
		register := func(what, path string) {
			s.ledger.Register(what, s.api.Remover(schema+path),
				s.api.Absence(schema+path, framework.AbsenceTimeout))
		}
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		keys := make([]string, 0, conditionSetCount)
		ids := map[string]string{}
		for i := range conditionSetCount {
			key := s.cfg.prefix.Key(fmt.Sprintf("sdk-user-set-%02d", i))
			register("condition set "+key, "/condition_sets/"+url.PathEscape(key))
			created, err := c.ConditionSets.Create(ctx, models.ConditionSetCreate{
				Key: key, Type: models.ConditionSetTypeUserSet, Name: key,
			})
			if err != nil {
				t.Fatalf("Create of the user set %s: %v", key, err)
			}
			if created.Key != key || created.ID == "" || created.EnvironmentID != s.env.ID ||
				created.Type != models.ConditionSetTypeUserSet {
				t.Fatalf("Create returned the set %q (ID %q, type %q) of the environment %q, "+
					"want the user set %s in %s", created.Key, created.ID, created.Type,
					created.EnvironmentID, key, s.env)
			}
			keys = append(keys, key)
			ids[key] = created.ID
		}
		listConditionSets(ctx, t, c, keys)
		resource, set := resourceSet(ctx, t, s, c, register)
		conditionSetRules(ctx, t, s, c, models.ConditionSetRuleCreate{
			UserSet: keys[0], Permission: resource + ":read", ResourceSet: set,
		})
		parent, child := keys[0], keys[1]
		for _, ref := range []string{parent, ids[parent]} {
			got, err := c.ConditionSets.Get(ctx, ref)
			if err != nil || got.ID != ids[parent] || got.Key != parent {
				t.Errorf("Get of the set %s returned %+v and %v, want %s (ID %s)", ref, got, err,
					parent, ids[parent])
			}
		}
		updated, err := c.ConditionSets.Update(ctx, child, models.ConditionSetUpdate{
			Name: child + "-renamed", Description: "A child of " + parent, ParentID: ids[parent],
		})
		if err != nil {
			t.Fatalf("Update of the set %s: %v", child, err)
		}
		if updated.Name != child+"-renamed" || updated.ParentID != ids[parent] {
			t.Errorf("Update returned the name %q and the parent %q, want %s-renamed and %s",
				updated.Name, updated.ParentID, child, ids[parent])
		}
		conditionSetRelatives(ctx, t, c, parent, child)
		if err := c.ConditionSets.Delete(ctx, child); err != nil {
			t.Fatalf("Delete of the set %s: %v", child, err)
		}
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				_, err := c.ConditionSets.Get(ctx, child)
				if errors.Is(err, permit.ErrNotFound) {
					return true, nil
				}
				return false, err
			})
		if gone != nil {
			t.Fatalf("Get of the deleted set %s: %v", child, gone)
		}
		if err := c.ConditionSets.Delete(ctx, child); err != nil {
			t.Errorf("Delete of the set %s that is gone returned %v, want nil", child, err)
		}
	}
}

// listConditionSets checks that All, at 30 sets on a page, lists each of keys once, and that List
// returns a page of sets, whichever form the API's reply has.
func listConditionSets(ctx context.Context, t *testing.T, c *permit.Client, keys []string) {
	t.Helper()
	seen := map[string]int{}
	for set, err := range c.ConditionSets.All(ctx,
		&permit.ConditionSetsListOptions{ListOptions: permit.ListOptions{PerPage: 30}}) {
		if err != nil {
			t.Fatalf("All of the condition sets: %v", err)
		}
		seen[set.Key]++
	}
	for _, key := range keys {
		if seen[key] != 1 {
			t.Errorf("All, 30 sets on a page, listed the set %s %d times, want once", key,
				seen[key])
		}
	}
	page, err := c.ConditionSets.List(ctx, nil)
	if err != nil || len(page.Data) == 0 {
		t.Errorf("List of the condition sets returned %+v and %v, want a page of sets", page, err)
	}
}

// resourceSet checks that a resource set of a resource of its own, with the action read, which it
// creates, names the resource's ID, and that All of the resource sets lists it. It returns the
// keys of the resource and of the resource set.
func resourceSet(ctx context.Context, t *testing.T, s *suite, c *permit.Client,
	register func(what, path string),
) (string, string) {
	t.Helper()
	resource := s.cfg.prefix.Key("sdk-set-resource")
	register("resource "+resource, "/resources/"+url.PathEscape(resource))
	created, err := c.Resources.Create(ctx, models.ResourceCreate{
		Key: resource, Name: resource,
		Actions: map[string]models.ActionBlockEditable{"read": {}},
	})
	if err != nil {
		t.Fatalf("setup: Create of the resource %s: %v", resource, err)
	}
	key := s.cfg.prefix.Key("sdk-resource-set")
	register("condition set "+key, "/condition_sets/"+url.PathEscape(key))
	set, err := c.ConditionSets.Create(ctx, models.ConditionSetCreate{
		Key: key, Type: models.ConditionSetTypeResourceSet, ResourceID: created.ID, Name: key,
	})
	if err != nil {
		t.Fatalf("Create of the resource set %s: %v", key, err)
	}
	if set.Type != models.ConditionSetTypeResourceSet || set.ResourceID != created.ID {
		t.Errorf("Create returned the type %q and the resource %q, want a resource set of %s",
			set.Type, set.ResourceID, created.ID)
	}
	var listed []string
	for set, err := range c.ConditionSets.All(ctx,
		&permit.ConditionSetsListOptions{Type: models.ConditionSetTypeResourceSet}) {
		if err != nil {
			t.Fatalf("All of the resource sets: %v", err)
		}
		listed = append(listed, set.Key)
	}
	if !slices.Contains(listed, key) {
		t.Errorf("All of the resource sets listed %q, want %s among them", listed, key)
	}
	return resource, key
}

// conditionSetRules checks the calls of the condition set rules on rule, which grants the users
// of a user set of the run a permission on a resource set of the run, and which it registers with
// the ledger, by its fields, before it creates it. It checks that each rule that the create
// returns has an ID and is of the environment, that All and List, each with the filters of the
// user set and the resource set, list each of them once, deletes the rule, and waits until All
// lists none of them. A filter of the permission is left out: what form of a permission it
// matches is not measured yet.
func conditionSetRules(ctx context.Context, t *testing.T, s *suite, c *permit.Client,
	rule models.ConditionSetRuleCreate,
) {
	t.Helper()
	path := "/v2/facts/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
		"/set_rules"
	remove := models.ConditionSetRuleRemove{
		UserSet: rule.UserSet, Permission: rule.Permission, ResourceSet: rule.ResourceSet,
	}
	query := url.Values{"user_set": {rule.UserSet}, "resource_set": {rule.ResourceSet}}
	s.ledger.Register("condition set rule "+rule.Permission+" of "+rule.UserSet+" on "+
		rule.ResourceSet, s.api.RemoverWithBody(path, remove),
		s.api.Unlisted(path+"?"+query.Encode(), framework.AbsenceTimeout, listsAnyRule))
	created, err := c.ConditionSetRules.Create(ctx, rule)
	if err != nil {
		t.Fatalf("Create of the condition set rule %+v: %v", rule, err)
	}
	var ids []string
	for _, r := range created {
		if r.ID == "" || r.EnvironmentID != s.env.ID {
			t.Errorf("Create returned the rule %q of the environment %q, want a rule with an ID "+
				"in %s", r.ID, r.EnvironmentID, s.env)
		}
		ids = append(ids, r.ID)
	}
	if len(ids) == 0 {
		t.Fatalf("Create of the condition set rule %+v returned no rule", rule)
	}
	filter := &permit.ConditionSetRulesListOptions{
		UserSet: rule.UserSet, ResourceSet: rule.ResourceSet,
	}
	all, err := ruleIDs(c.ConditionSetRules.All(ctx, filter))
	if err != nil {
		t.Fatalf("All of the condition set rules of %s on %s: %v", rule.UserSet,
			rule.ResourceSet, err)
	}
	page, err := c.ConditionSetRules.List(ctx, filter)
	if err != nil {
		t.Fatalf("List of the condition set rules of %s on %s: %v", rule.UserSet,
			rule.ResourceSet, err)
	}
	listed := map[string]int{}
	for _, r := range page.Data {
		listed[r.ID]++
	}
	for _, id := range ids {
		if all[id] != 1 || listed[id] != 1 {
			t.Errorf("All and List of the condition set rules of %s on %s listed the rule %s %d "+
				"and %d times, want once", rule.UserSet, rule.ResourceSet, id, all[id], listed[id])
		}
	}
	if err := c.ConditionSetRules.Delete(ctx, remove); err != nil {
		t.Fatalf("Delete of the condition set rule %+v: %v", remove, err)
	}
	gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
		func(ctx context.Context) (bool, error) {
			left, err := ruleIDs(c.ConditionSetRules.All(ctx, filter))
			return !slices.ContainsFunc(ids, func(id string) bool { return left[id] > 0 }), err
		})
	if gone != nil {
		t.Fatalf("All of the condition set rules of %s on %s after Delete: %v", rule.UserSet,
			rule.ResourceSet, gone)
	}
}

// ruleIDs returns how many times rules yields each ID, or the first error that it yields.
func ruleIDs(rules iter.Seq2[models.ConditionSetRuleRead, error]) (map[string]int, error) {
	ids := map[string]int{}
	for r, err := range rules {
		if err != nil {
			return nil, err
		}
		ids[r.ID]++
	}
	return ids, nil
}

// listsAnyRule is the ledger's check of the body of a reply to a list of the condition set rules
// that the query filters by a user set and a resource set of the run: whether it lists any rule.
// It does not compare the user_set and resource_set of a listed rule with the keys that the run
// sent, as whether the reply holds keys or IDs there is not measured yet.
func listsAnyRule(body []byte) (bool, error) {
	var listed []json.RawMessage
	if err := json.Unmarshal(body, &listed); err != nil {
		return false, fmt.Errorf("decoding the list of the condition set rules: %w", err)
	}
	return len(listed) > 0, nil
}

// TestListsAnyRule checks the ledger's check of a list of the condition set rules on bodies of
// replies: a rule is listed whether its user set and resource set are keys or IDs, an empty list
// and null list none, and a body that is not a JSON array is an error.
func TestListsAnyRule(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
		wantErr    bool
	}{
		{
			name: "a rule by keys", want: true,
			body: `[{"id":"r1","user_set":"adults","permission":"document:read",` +
				`"resource_set":"public_docs"}]`,
		},
		{
			name: "a rule by IDs", want: true,
			body: `[{"id":"r1","user_set":"00000000-0000-4000-8000-000000000001",` +
				`"permission":"document:read",` +
				`"resource_set":"00000000-0000-4000-8000-000000000002"}]`,
		},
		{name: "no rule", body: `[]`},
		{name: "null", body: `null`},
		{name: "an object", body: `{"detail":"Not Found"}`, wantErr: true},
		{name: "not JSON", body: `<html></html>`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := listsAnyRule([]byte(tc.body))
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Errorf("listsAnyRule(%s) = %v, %v; want %v, and an error: %v", tc.body, got,
					err, tc.want, tc.wantErr)
			}
		})
	}
}

// conditionSetRelatives checks that the ancestors of child, whose parent is parent, are parent
// alone, and that the descendants of parent are child alone, with the call of a page and with
// its iterator.
func conditionSetRelatives(ctx context.Context, t *testing.T, c *permit.Client, parent,
	child string,
) {
	t.Helper()
	for _, tc := range []struct {
		name, key, want string
		page            func() (*permit.Page[models.ConditionSetRead], error)
		all             iter.Seq2[models.ConditionSetRead, error]
	}{
		{
			name: "the ancestors", key: child, want: parent,
			page: func() (*permit.Page[models.ConditionSetRead], error) {
				return c.ConditionSets.Ancestors(ctx, child, nil)
			},
			all: c.ConditionSets.AncestorsAll(ctx, child, nil),
		},
		{
			name: "the descendants", key: parent, want: child,
			page: func() (*permit.Page[models.ConditionSetRead], error) {
				return c.ConditionSets.Descendants(ctx, parent, nil)
			},
			all: c.ConditionSets.DescendantsAll(ctx, parent, nil),
		},
	} {
		page, err := tc.page()
		if err != nil {
			t.Fatalf("a page of %s of the set %s: %v", tc.name, tc.key, err)
		}
		if got := setKeys(slices.Values(page.Data)); !slices.Equal(got, []string{tc.want}) {
			t.Errorf("a page of %s of the set %s holds %q, want %s alone", tc.name, tc.key, got,
				tc.want)
		}
		var all []models.ConditionSetRead
		for set, err := range tc.all {
			if err != nil {
				t.Fatalf("the iterator of %s of the set %s: %v", tc.name, tc.key, err)
			}
			all = append(all, set)
		}
		if got := setKeys(slices.Values(all)); !slices.Equal(got, []string{tc.want}) {
			t.Errorf("the iterator of %s of the set %s yielded %q, want %s alone", tc.name,
				tc.key, got, tc.want)
		}
	}
}

// setKeys returns the keys of sets, in order.
func setKeys(sets iter.Seq[models.ConditionSetRead]) []string {
	keys := []string{}
	for set := range sets {
		keys = append(keys, set.Key)
	}
	return keys
}
