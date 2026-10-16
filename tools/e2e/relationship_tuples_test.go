//go:build e2e

package e2e

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// relatedTuple is a relationship tuple as a scenario compares it: its subject, its relation and
// its object.
type relatedTuple struct {
	subject, relation, object string
}

// sortedTuples returns the relatedTuple of each of list, which key gives, sorted by object.
func sortedTuples[T any](list []T, key func(T) relatedTuple) []relatedTuple {
	got := make([]relatedTuple, len(list))
	for i, tuple := range list {
		got[i] = key(tuple)
	}
	slices.SortFunc(got, func(a, b relatedTuple) int { return cmp.Compare(a.object, b.object) })
	return got
}

// plainTuple returns t, a relationship tuple of List, as a scenario compares it.
func plainTuple(t models.RelationshipTupleRead) relatedTuple {
	return relatedTuple{t.Subject, t.Relation, t.Object}
}

// detailedTuple returns t, a relationship tuple of ListDetailed, as a scenario compares it.
func detailedTuple(t models.RelationshipTupleDetailedRead) relatedTuple {
	return relatedTuple{t.Subject, t.Relation, t.Object}
}

// relationshipTupleLifecycle checks the calls of the relationship tuples with the
// environment-level API key of the scratch environment, and no WithScope, on resource instances
// of its own seed, whose resources' keys start with the run's prefix: a folder and two documents
// in a tenant, and the relation parent of the documents, whose subjects are folders. A list of
// the relationship tuples of the folder is an empty page before any. It creates the relationship
// tuple of the folder as the parent of the first document with Create, registered with the
// ledger before, and finds it in the lists of the folder's relationship tuples of List, All,
// ListDetailed and AllDetailed. It deletes it with Delete, after which a second Delete of it is
// an error that matches ErrNotFound and List of the folder is an empty page again. Then it
// creates the relationship tuples of the folder as the parent of each document in one request,
// and deletes them in one request, as bulkTuples says.
func relationshipTupleLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		folder, document := s.cfg.prefix.Key("sdk-tup-folder"), s.cfg.prefix.Key("sdk-tup-doc")
		tenant := s.cfg.prefix.Key("sdk-tup-tenant")
		s.seed(t, world{
			resources: []seed.Resource{
				{Key: folder, Actions: []string{"read"}},
				{Key: document, Actions: []string{"read"}},
			},
			tenants: []string{tenant},
			instances: []seed.Instance{
				{Resource: folder, Key: "f1", Tenant: tenant},
				{Resource: document, Key: "d1", Tenant: tenant},
				{Resource: document, Key: "d2", Tenant: tenant},
			},
		})
		b := seed.New(s.api, s.ledger, s.env)
		parent := seed.Relation{Resource: document, Key: "parent", SubjectResource: folder}
		if err := b.Relation(t.Context(), parent); err != nil {
			t.Fatalf("setup: %v", err)
		}
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		tuple := models.RelationshipTupleCreate{
			Subject: folder + ":f1", Relation: "parent", Object: document + ":d1", Tenant: tenant,
		}
		key := models.RelationshipTupleDelete{
			Subject: tuple.Subject, Relation: tuple.Relation, Object: tuple.Object,
		}
		want := relatedTuple{tuple.Subject, tuple.Relation, tuple.Object}
		mine := &permit.RelationshipTuplesListOptions{Subject: tuple.Subject}
		expectNoTuples(ctx, t, c, mine)
		b.RegisterTuple(seed.Tuple{
			Subject: tuple.Subject, Relation: tuple.Relation, Object: tuple.Object,
			Tenant: tuple.Tenant,
		})
		got, err := c.RelationshipTuples.Create(ctx, tuple)
		if err != nil || got.ID == "" || plainTuple(*got) != want {
			t.Fatalf("Create of %+v returned %+v, %v; want the relationship tuple", tuple, got, err)
		}
		listTuples(ctx, t, c, mine, []relatedTuple{want})
		if err := c.RelationshipTuples.Delete(ctx, key); err != nil {
			t.Fatalf("Delete of %+v: %v", key, err)
		}
		if err := c.RelationshipTuples.Delete(ctx, key); !errors.Is(err, permit.ErrNotFound) {
			t.Errorf("a second Delete of %+v returned %v, want an error that matches "+
				"ErrNotFound", key, err)
		}
		expectNoTuples(ctx, t, c, mine)
		second := tuple
		second.Object = document + ":d2"
		bulkTuples(ctx, t, c, b, []models.RelationshipTupleCreate{tuple, second})
	}
}

// bulkTuples creates tuples, at least one and all of the subject of the first, each registered
// with the ledger of b before, in one request with BulkCreate, finds them, and no other, in the
// lists of the relationship tuples of that subject, as listTuples says, deletes them in one
// request with BulkDelete, and checks that List of the subject is an empty page after it.
func bulkTuples(ctx context.Context, t *testing.T, c *permit.Client, b *seed.Builder,
	tuples []models.RelationshipTupleCreate,
) {
	t.Helper()
	opts := &permit.RelationshipTuplesListOptions{Subject: tuples[0].Subject}
	keys := make([]models.RelationshipTupleDelete, 0, len(tuples))
	want := make([]relatedTuple, 0, len(tuples))
	for _, tuple := range tuples {
		b.RegisterTuple(seed.Tuple{
			Subject: tuple.Subject, Relation: tuple.Relation, Object: tuple.Object,
			Tenant: tuple.Tenant,
		})
		keys = append(keys, models.RelationshipTupleDelete{
			Subject: tuple.Subject, Relation: tuple.Relation, Object: tuple.Object,
		})
		want = append(want, relatedTuple{tuple.Subject, tuple.Relation, tuple.Object})
	}
	if err := c.RelationshipTuples.BulkCreate(ctx, tuples); err != nil {
		t.Fatalf("BulkCreate of %+v: %v", tuples, err)
	}
	listTuples(ctx, t, c, opts, want)
	if err := c.RelationshipTuples.BulkDelete(ctx, keys); err != nil {
		t.Fatalf("BulkDelete of %+v: %v", keys, err)
	}
	expectNoTuples(ctx, t, c, opts)
}

// expectNoTuples checks that List with opts returns a page without relationship tuples, whose
// Data is empty and not nil.
func expectNoTuples(ctx context.Context, t *testing.T, c *permit.Client,
	opts *permit.RelationshipTuplesListOptions,
) {
	t.Helper()
	page, err := c.RelationshipTuples.List(ctx, opts)
	if err != nil || page.Data == nil || len(page.Data) != 0 {
		t.Fatalf("List of the subject %s returned %#v, %v; want a page whose Data is empty and "+
			"not nil", opts.Subject, page, err)
	}
}

// listTuples checks that List and ListDetailed with opts, and All and AllDetailed, each return
// the relationship tuples of want, and no other.
func listTuples(ctx context.Context, t *testing.T, c *permit.Client,
	opts *permit.RelationshipTuplesListOptions, want []relatedTuple,
) {
	t.Helper()
	page, err := c.RelationshipTuples.List(ctx, opts)
	if err != nil {
		t.Fatalf("List of the subject %s: %v", opts.Subject, err)
	}
	detailed, err := c.RelationshipTuples.ListDetailed(ctx, opts)
	if err != nil {
		t.Fatalf("ListDetailed of the subject %s: %v", opts.Subject, err)
	}
	var all []models.RelationshipTupleRead
	for tuple, err := range c.RelationshipTuples.All(ctx, opts) {
		if err != nil {
			t.Fatalf("All of the subject %s: %v", opts.Subject, err)
		}
		all = append(all, tuple)
	}
	var allDetailed []models.RelationshipTupleDetailedRead
	for tuple, err := range c.RelationshipTuples.AllDetailed(ctx, opts) {
		if err != nil {
			t.Fatalf("AllDetailed of the subject %s: %v", opts.Subject, err)
		}
		allDetailed = append(allDetailed, tuple)
	}
	for name, got := range map[string][]relatedTuple{
		"List":         sortedTuples(page.Data, plainTuple),
		"All":          sortedTuples(all, plainTuple),
		"ListDetailed": sortedTuples(detailed.Data, detailedTuple),
		"AllDetailed":  sortedTuples(allDetailed, detailedTuple),
	} {
		if !slices.Equal(got, want) {
			t.Errorf("%s of the subject %s returned %+v, want %+v", name, opts.Subject, got, want)
		}
	}
}
