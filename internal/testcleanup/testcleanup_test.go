package testcleanup_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/permitio/permit-golang/internal/testcleanup"
	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	prefix  = "gosdk-ci-7-1"
	apiKey  = "permit_key_FAKEcleanup0123456789"
	project = "proj"
	env     = "env"
)

// collections are the list routes of the object types Run deletes, with the route group they
// are under.
var collections = map[string]string{
	"proxy_configs":      "facts",
	"condition_sets":     "schema",
	"resource_instances": "facts",
	"users":              "facts",
	"tenants":            "facts",
	"roles":              "schema",
	"resources":          "schema",
}

// referredBy lists, for each collection, the collections whose objects can refer to its objects:
// role assignments (deleted with their user) refer to roles, tenants and resource instances, and
// proxy configs, resource sets, resource instances and roles refer to resources.
var referredBy = map[string][]string{
	"resources":          {"proxy_configs", "condition_sets", "resource_instances", "roles"},
	"roles":              {"users"},
	"tenants":            {"users", "resource_instances"},
	"resource_instances": {"users"},
}

// notOwned are keys that the run with prefix must never delete.
var notOwned = []string{
	prefix,                  // the prefix alone
	prefix + "-",            // the prefix and the hyphen alone
	prefix + "0-user",       // attempt 10 of the same workflow run
	"gosdk-ci-7-10-user",    // likewise, written out
	"gosdk-ci-7-2-user",     // another attempt
	"gosdk-ci-70-1-user",    // another workflow run
	"other-" + prefix + "-", // contains the prefix
	"default",
	"GOSDK-CI-7-1-USER",
}

// TestMain removes the wait before rounds 2 and 3, which is for the API's rate limit, from the
// tests that do not set it themselves.
func TestMain(m *testing.M) {
	testcleanup.SetRoundDelay(0)
	os.Exit(m.Run())
}

type fakeObject struct {
	key string
	ref string
}

// fakeAPI serves the list and delete routes of the Permit API that Run calls, for the objects
// added to it.
type fakeAPI struct {
	t       *testing.T
	mu      sync.Mutex
	objects map[string][]fakeObject
	// deleteStatuses holds, by reference, the statuses to answer the next deletes with instead
	// of deleting. A 404 also removes the object, as when someone else deleted it after Run
	// listed it.
	deleteStatuses map[string][]int
	// notFoundButKept holds the references whose deletes are answered with 404 although the
	// object stays.
	notFoundButKept map[string]bool
	listStatus      map[string]int
	// endless holds the collections whose list answers every page with the first page, like an
	// API that ignores the page number.
	endless map[string]bool
	// afterPage, when set, runs after each list page is served, with mu held.
	afterPage func(collection string, page int)
	// strict refuses to delete an object while a collection in referredBy of its collection
	// still has objects, like an API that refuses to delete an object another one refers to.
	strict      bool
	deletes     []string
	listedPages map[string][]int
}

func newFakeAPI(t *testing.T) (*fakeAPI, *api.PermitApiClient) {
	f := &fakeAPI{
		t:               t,
		objects:         map[string][]fakeObject{},
		deleteStatuses:  map[string][]int{},
		notFoundButKept: map[string]bool{},
		listStatus:      map[string]int{},
		endless:         map[string]bool{},
		listedPages:     map[string][]int{},
	}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return f, newClient(server.URL)
}

// newClient returns a client that sends both its API and its PDP requests to url.
func newClient(url string) *api.PermitApiClient {
	return permit.New(config.NewConfigBuilder(apiKey).
		WithApiUrl(url).
		WithPdpUrl(url).
		WithContext(config.NewPermitContext(config.EnvironmentAPIKeyLevel, project, env)).
		WithLogger(zap.NewNop()).
		Build()).Api
}

func (f *fakeAPI) add(collection string, keys ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, key := range keys {
		ref := key
		if collection == "resource_instances" {
			ref = "id-" + key
		}
		f.objects[collection] = append(f.objects[collection], fakeObject{key: key, ref: ref})
	}
}

func (f *fakeAPI) keys(collection string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keys []string
	for _, obj := range f.objects[collection] {
		keys = append(keys, obj.key)
	}
	return keys
}

func (f *fakeAPI) failDeletes(ref string, statuses ...int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteStatuses[ref] = statuses
}

func (f *fakeAPI) failList(collection string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listStatus[collection] = status
}

func (f *fakeAPI) pages(collection string) []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.listedPages[collection]...)
}

func (f *fakeAPI) listed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.listedPages)
}

func (f *fakeAPI) deleted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deletes...)
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+apiKey {
		http.Error(w, `{"detail":"bad key"}`, http.StatusUnauthorized)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 5 || parts[0] != "v2" || parts[2] != project || parts[3] != env ||
		collections[parts[4]] != parts[1] {
		http.Error(w, `{"detail":"no such route"}`, http.StatusNotFound)
		return
	}
	collection := parts[4]
	switch {
	case len(parts) == 5 && r.Method == http.MethodGet:
		f.list(w, r, collection)
	case len(parts) == 6 && r.Method == http.MethodDelete:
		f.delete(w, collection, parts[5])
	default:
		http.Error(w, `{"detail":"no such route"}`, http.StatusNotFound)
	}
}

func (f *fakeAPI) list(w http.ResponseWriter, r *http.Request, collection string) {
	page, pageErr := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, perPageErr := strconv.Atoi(r.URL.Query().Get("per_page"))
	if pageErr != nil || perPageErr != nil || page < 1 || perPage < 1 {
		f.t.Errorf("list request without a valid page and per_page: %s", r.URL)
		http.Error(w, `{"detail":"bad paging"}`, http.StatusBadRequest)
		return
	}
	f.listedPages[collection] = append(f.listedPages[collection], page)
	if status := f.listStatus[collection]; status != 0 {
		http.Error(w, `{"detail":"list failed"}`, status)
		return
	}
	served := page
	if f.endless[collection] {
		served = 1
	}
	all := f.objects[collection]
	start := (served - 1) * perPage
	end := start + perPage
	if start > len(all) {
		start = len(all)
	}
	if end > len(all) {
		end = len(all)
	}
	items := make([]interface{}, 0, end-start)
	for _, obj := range all[start:end] {
		items = append(items, readModel(collection, obj))
	}
	var body interface{} = items
	if collection == "users" {
		body = map[string]interface{}{"data": items, "total_count": len(all), "page_count": 1}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		f.t.Errorf("encoding the %s list: %v", collection, err)
	}
	if f.afterPage != nil {
		f.afterPage(collection, page)
	}
}

func (f *fakeAPI) delete(w http.ResponseWriter, collection, ref string) {
	f.deletes = append(f.deletes, collection+"/"+ref)
	index := -1
	for i, obj := range f.objects[collection] {
		if obj.ref == ref {
			index = i
		}
	}
	if statuses := f.deleteStatuses[ref]; len(statuses) > 0 {
		f.deleteStatuses[ref] = statuses[1:]
		if statuses[0] != http.StatusNotFound {
			http.Error(w, `{"detail":"delete failed"}`, statuses[0])
			return
		}
		index = -1
		f.remove(collection, ref)
	}
	if index < 0 || f.notFoundButKept[ref] {
		http.Error(w, `{"detail":"not found"}`, http.StatusNotFound)
		return
	}
	if f.strict {
		for _, referrer := range referredBy[collection] {
			if len(f.objects[referrer]) > 0 {
				http.Error(w, `{"detail":"referred to by `+referrer+`"}`, http.StatusConflict)
				return
			}
		}
	}
	f.remove(collection, ref)
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeAPI) remove(collection, ref string) {
	kept := f.objects[collection][:0]
	for _, obj := range f.objects[collection] {
		if obj.ref != ref {
			kept = append(kept, obj)
		}
	}
	f.objects[collection] = kept
}

// readModel returns the object as the API sends it, built with the SDK's model constructors so
// that it has every required field.
func readModel(collection string, obj fakeObject) interface{} {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	id := "id-" + obj.key
	switch collection {
	case "proxy_configs":
		return models.NewProxyConfigRead(obj.key, id, "org", project, env, now, now, "", obj.key)
	case "condition_sets":
		return models.NewConditionSetRead(obj.key, id, "org", project, env, now, now, obj.key)
	case "resource_instances":
		return models.NewResourceInstanceRead(obj.key, "document", obj.ref, "org", project, env,
			now, now, "resource-id")
	case "users":
		return models.NewUserRead(obj.key, id, "org", project, env)
	case "tenants":
		return models.NewTenantRead(obj.key, id, "org", project, env, now, now, now, obj.key)
	case "roles":
		return models.NewRoleRead(obj.key, obj.key, id, "org", project, env, now, now)
	case "resources":
		return models.NewResourceRead(obj.key, id, "org", project, env, now, now, obj.key)
	}
	panic("unknown collection " + collection)
}

func sortedCollections() []string {
	var names []string
	for name := range collections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestRunDeletesOnlyTheRunsObjects(t *testing.T) {
	fake, client := newFakeAPI(t)
	for _, collection := range sortedCollections() {
		fake.add(collection, notOwned...)
		fake.add(collection, prefix+"-a", prefix+"-b-"+collection)
	}

	var out bytes.Buffer
	require.NoError(t, testcleanup.Run(context.Background(), client, prefix, &out))

	var wantDeletes []string
	for _, collection := range sortedCollections() {
		assert.ElementsMatch(t, notOwned, fake.keys(collection), "%s left", collection)
		ref := func(key string) string {
			if collection == "resource_instances" {
				return collection + "/id-" + key
			}
			return collection + "/" + key
		}
		wantDeletes = append(wantDeletes, ref(prefix+"-a"), ref(prefix+"-b-"+collection))
	}
	assert.ElementsMatch(t, wantDeletes, fake.deleted())
	assert.Contains(t, out.String(), "deleted "+prefix+"-b-users\n")
	assert.Contains(t, out.String(), "users: 2 found, 2 deleted, 0 already deleted, 0 failed")
	assert.Equal(t, 1, strings.Count(out.String(), "Round "), "rounds in:\n%s", out.String())
	assert.Contains(t, out.String(), "none left")
}

func TestRunDeletesObjectsBeforeTheObjectsTheyReferTo(t *testing.T) {
	fake, client := newFakeAPI(t)
	fake.strict = true
	for _, collection := range sortedCollections() {
		fake.add(collection, prefix+"-"+collection)
	}

	var out bytes.Buffer
	require.NoError(t, testcleanup.Run(context.Background(), client, prefix, &out))

	for _, collection := range sortedCollections() {
		assert.Empty(t, fake.keys(collection), collection)
	}
	assert.Equal(t, 1, strings.Count(out.String(), "Round "),
		"every delete succeeds in the first round:\n%s", out.String())
}

func TestRunReadsEveryPageBeforeDeleting(t *testing.T) {
	fake, client := newFakeAPI(t)
	// Three full pages of 100. An owned tenant opens the first and the second page: deleting
	// the first before reading page 2 would move the second onto page 1, and it would be missed.
	var keys []string
	for i := 0; i < 300; i++ {
		keys = append(keys, fmt.Sprintf("other-tenant-%03d", i))
	}
	keys[0] = prefix + "-tenant-first"
	keys[100] = prefix + "-tenant-second"
	keys[299] = prefix + "-tenant-last"
	fake.add("tenants", keys...)
	var users []string
	for i := 0; i < 150; i++ {
		users = append(users, fmt.Sprintf("other-user-%03d", i))
	}
	users[120] = prefix + "-user-on-page-2"
	fake.add("users", users...)

	require.NoError(t, testcleanup.Run(context.Background(), client, prefix, &bytes.Buffer{}))

	assert.Len(t, fake.keys("tenants"), 297)
	assert.NotContains(t, fake.keys("tenants"), prefix+"-tenant-second")
	assert.NotContains(t, fake.keys("tenants"), prefix+"-tenant-last")
	assert.Equal(t, []int{1, 2, 3, 4, 1, 2, 3}, fake.pages("tenants"),
		"the pages of the round, then of the check")
	assert.Len(t, fake.keys("users"), 149)
	assert.Equal(t, []int{1, 2, 1, 2}, fake.pages("users"))
}

func TestRunStopsAtAListThatNeverEnds(t *testing.T) {
	defer testcleanup.SetMaxPages(3)()
	fake, client := newFakeAPI(t)
	for i := 0; i < 100; i++ {
		fake.add("tenants", fmt.Sprintf("other-tenant-%03d", i))
	}
	fake.endless["tenants"] = true
	fake.add("users", prefix+"-user")

	err := testcleanup.Run(context.Background(), client, prefix, &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "listing tenants: stopped after 3 pages of 100")
	assert.Equal(t, []int{1, 2, 3, 1, 2, 3, 1, 2, 3}, fake.pages("tenants"),
		"3 pages in each of the 3 rounds")
	assert.Empty(t, fake.keys("users"), "the other types are still cleaned")
}

func TestRunTreatsAnAlreadyDeletedObjectAsDeleted(t *testing.T) {
	fake, client := newFakeAPI(t)
	fake.add("roles", prefix+"-role", "other-role")
	fake.add("resource_instances", prefix+"-doc")
	fake.failDeletes(prefix+"-role", http.StatusNotFound)
	fake.failDeletes("id-"+prefix+"-doc", http.StatusNotFound)

	var out bytes.Buffer
	require.NoError(t, testcleanup.Run(context.Background(), client, prefix, &out))

	assert.Equal(t, []string{"other-role"}, fake.keys("roles"))
	assert.Empty(t, fake.keys("resource_instances"))
	assert.Contains(t, out.String(), "already deleted: "+prefix+"-role\n")
	assert.Contains(t, out.String(), "roles: 1 found, 0 deleted, 1 already deleted, 0 failed")

	out.Reset()
	require.NoError(t, testcleanup.Run(context.Background(), client, prefix, &out),
		"a second run finds nothing to delete")
	assert.Len(t, fake.deleted(), 2)
	assert.Contains(t, out.String(), "roles: 0 found, 0 deleted, 0 already deleted, 0 failed")
}

func TestRunChecksThatNothingIsLeft(t *testing.T) {
	t.Run("a page shrinks between reads", func(t *testing.T) {
		fake, client := newFakeAPI(t)
		var users []string
		for i := 0; i < 150; i++ {
			users = append(users, fmt.Sprintf("other-user-%03d", i))
		}
		users[100] = prefix + "-user-first-on-page-2"
		fake.add("users", users...)
		shrunk := false
		// Someone deletes 5 users after Run reads page 1, so the owned user moves onto page 1,
		// which Run has already read.
		fake.afterPage = func(collection string, page int) {
			if collection == "users" && page == 1 && !shrunk {
				fake.objects["users"] = fake.objects["users"][5:]
				shrunk = true
			}
		}

		var out bytes.Buffer
		err := testcleanup.Run(context.Background(), client, prefix, &out)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "users "+prefix+"-user-first-on-page-2 is still listed")
		assert.Contains(t, out.String(), "users: 0 found")
	})
	t.Run("a delete answers 404 for an object that stays", func(t *testing.T) {
		fake, client := newFakeAPI(t)
		fake.add("users", prefix+"-user")
		fake.notFoundButKept[prefix+"-user"] = true

		var out bytes.Buffer
		err := testcleanup.Run(context.Background(), client, prefix, &out)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "users "+prefix+"-user is still listed")
		assert.Contains(t, out.String(), "already deleted: "+prefix+"-user\n")
	})
	t.Run("the list fails in the check", func(t *testing.T) {
		fake, client := newFakeAPI(t)
		fake.add("users", prefix+"-user")
		fake.afterPage = func(collection string, page int) {
			if collection == "users" {
				fake.listStatus["users"] = http.StatusInternalServerError
			}
		}

		err := testcleanup.Run(context.Background(), client, prefix, &bytes.Buffer{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "listing users: page 1: ")
		assert.Empty(t, fake.keys("users"), "the round deleted the user")
	})
}

func TestRunRetriesAFailedDeleteAndReportsWhatItCouldNotDelete(t *testing.T) {
	fake, client := newFakeAPI(t)
	fake.add("roles", prefix+"-base", prefix+"-stuck", prefix+"-extends")
	fake.add("users", prefix+"-user")
	// The base role can be deleted once the role that extends it is gone: a later round.
	fake.failDeletes(prefix+"-base", http.StatusBadRequest)
	fake.failDeletes(prefix+"-stuck", 500, 500, 500)

	var out bytes.Buffer
	err := testcleanup.Run(context.Background(), client, prefix, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deleting roles "+prefix+"-stuck: ")
	assert.Contains(t, err.Error(),
		`the API answered 500 Internal Server Error: {"detail":"delete failed"}`)
	assert.NotContains(t, err.Error(), prefix+"-base")
	assert.Equal(t, []string{prefix + "-stuck"}, fake.keys("roles"))
	assert.Empty(t, fake.keys("users"))
	var stuckDeletes int
	for _, deleted := range fake.deleted() {
		if deleted == "roles/"+prefix+"-stuck" {
			stuckDeletes++
		}
	}
	assert.Equal(t, 3, stuckDeletes, "one delete in each round")
	assert.Contains(t, out.String(), "Round 3")
	assert.Contains(t, out.String(), "could not delete "+prefix+`-base: `)
	assert.Contains(t, out.String(), `the API answered 400 Bad Request: {"detail":"delete failed"}`)
}

func TestRunWaitsBetweenRounds(t *testing.T) {
	defer testcleanup.SetRoundDelay(200 * time.Millisecond)()
	fake, client := newFakeAPI(t)
	fake.add("roles", prefix+"-stuck")
	fake.failDeletes(prefix+"-stuck", 500, 500, 500)

	start := time.Now()
	require.Error(t, testcleanup.Run(context.Background(), client, prefix, &bytes.Buffer{}))

	assert.GreaterOrEqual(t, time.Since(start), 400*time.Millisecond,
		"a wait before rounds 2 and 3")
}

func TestRunStopsWaitingWhenTheContextEnds(t *testing.T) {
	defer testcleanup.SetRoundDelay(10 * time.Second)()
	fake, client := newFakeAPI(t)
	fake.add("roles", prefix+"-stuck")
	fake.failDeletes(prefix+"-stuck", 500, 500, 500)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := testcleanup.Run(ctx, client, prefix, &bytes.Buffer{})

	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Contains(t, err.Error(), "stopped before round 2: context deadline exceeded")
	assert.Contains(t, err.Error(), "deleting roles "+prefix+"-stuck: ")
}

func TestRunReportsAListThatFailsAndCleansTheOtherTypes(t *testing.T) {
	fake, client := newFakeAPI(t)
	fake.add("condition_sets", prefix+"-set")
	fake.add("tenants", prefix+"-tenant")
	fake.failList("condition_sets", http.StatusInternalServerError)

	err := testcleanup.Run(context.Background(), client, prefix, &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "listing condition sets: page 1: ")
	assert.NotContains(t, err.Error(), apiKey)
	assert.Equal(t, []string{prefix + "-set"}, fake.keys("condition_sets"))
	assert.Empty(t, fake.keys("tenants"))
}

func TestRunRefusesAPrefixThatCouldMatchOtherObjects(t *testing.T) {
	bad := []string{"", "gosdk", "gosdk-ci", "gosdk-ci-", "gosdk-CI-7-1", "gosdk-ci-7-1-",
		"gosdk--7-1", "other-ci-7-1", " gosdk-ci-7-1", "gosdk-ci-7-1*",
		// Valid run prefixes start with these, so they would own the objects of other runs.
		"gosdk-ci-7", "gosdk-ci-7-", "gosdk-ci-7-1-rbac", "gosdk-local", "gosdk-local-0a1b2c",
		// Not the form CI or the tests create.
		"gosdk-ci-7-1-2", "gosdk-ci-x-1", "gosdk-other-7-1", "gosdk-local-0a1b2c3d4e5",
		"gosdk-local-0a1b2c3d4e5f0", "gosdk-local-0A1B2C3D4E5F", "gosdk-local-0a1b2c3d4e5g"}
	for _, bad := range bad {
		t.Run(strconv.Quote(bad), func(t *testing.T) {
			fake, client := newFakeAPI(t)
			fake.add("tenants", "gosdk-ci-7-1-tenant", "other")

			err := testcleanup.Run(context.Background(), client, bad, &bytes.Buffer{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "run prefix")
			assert.Zero(t, fake.listed(), "no request is sent")
			assert.Len(t, fake.keys("tenants"), 2)
		})
	}
}

func TestCheckPrefixAcceptsTheRunPrefixes(t *testing.T) {
	prefixes := []string{"gosdk-ci-7-1", "gosdk-ci-18234567890-12", "gosdk-local-0a1b2c3d4e5f"}
	for _, good := range prefixes {
		assert.NoError(t, testcleanup.CheckPrefix(good), good)
	}
}

func TestIsNotFound(t *testing.T) {
	fake, client := newFakeAPI(t)
	fake.add("users", "present")
	ctx := context.Background()

	_, missingUser := client.Users.Get(ctx, "missing")
	require.Error(t, missingUser)
	assert.True(t, testcleanup.IsNotFound(missingUser), "PermitError 404: %v", missingUser)

	// The generated client's error, which ResourceActionGroups returns as it is.
	_, missingGroup := client.ResourceActionGroups.Get(ctx, "document", "group")
	require.Error(t, missingGroup)
	assert.True(t, testcleanup.IsNotFound(missingGroup), "generated 404 error: %v", missingGroup)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"detail":"failed"}`, http.StatusInternalServerError)
	}))
	defer failing.Close()
	_, failedGroup := newClient(failing.URL).ResourceActionGroups.Get(ctx, "document", "group")
	require.Error(t, failedGroup)
	assert.False(t, testcleanup.IsNotFound(failedGroup), "generated 500 error: %v", failedGroup)

	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusConflict} {
		fake.failDeletes("present", status)
		refused := client.Users.Delete(ctx, "present")
		require.Error(t, refused)
		assert.False(t, testcleanup.IsNotFound(refused), "status %d: %v", status, refused)
	}

	fake.failList("users", http.StatusInternalServerError)
	_, serverError := client.Users.List(ctx, 1, 10)
	require.Error(t, serverError)
	assert.False(t, testcleanup.IsNotFound(serverError), "status 500: %v", serverError)

	_, unreachable := newClient("http://127.0.0.1:1").Users.Get(ctx, "present")
	require.Error(t, unreachable)
	assert.False(t, testcleanup.IsNotFound(unreachable), "connection error: %v", unreachable)

	assert.False(t, testcleanup.IsNotFound(nil))
	assert.False(t, testcleanup.IsNotFound(errors.New("404 Not Found")))
}
