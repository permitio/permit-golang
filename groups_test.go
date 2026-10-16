package permit

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the groups of an environment and of their users, as mockpermit.Server.Handle
// takes them, and the paths of the groups and of the users of the group engineering with the
// scope p and e.
const (
	groupsPath           = "/v2/schema/{proj_id}/{env_id}/groups"
	groupPath            = groupsPath + "/{group_instance_key}"
	groupUsersPath       = groupPath + "/users"
	groupUserPath        = groupUsersPath + "/{user_id}"
	listGroupsRoute      = "GET " + groupsPath + "/direct"
	createGroupRoute     = "POST " + groupsPath
	getGroupRoute        = "GET " + groupsPath + "/direct/{group_instance_key}"
	deleteGroupRoute     = "DELETE " + groupPath
	listGroupUsersRoute  = "GET " + groupUsersPath
	assignGroupUserRoute = "PUT " + groupUserPath
	removeGroupUserRoute = "DELETE " + groupUserPath
	groupsRaw            = "/v2/schema/p/e/groups"
	engineeringUsersRaw  = groupsRaw + "/engineering/users"
)

// The routes of the roles of a group and of the groups assigned to a group, as
// mockpermit.Server.Handle takes them.
const (
	groupRolesPath         = groupPath + "/roles"
	groupAssignPath        = groupPath + "/assign_group"
	assignGroupRoleRoute   = "POST " + groupRolesPath
	removeGroupRoleRoute   = "DELETE " + groupRolesPath
	listGroupRolesRoute    = "GET " + groupRolesPath
	assignGroupGroupRoute  = "PUT " + groupAssignPath
	removeGroupGroupRoute  = "DELETE " + groupAssignPath
	listGroupChildrenRoute = "GET " + groupPath + "/children"
	listGroupParentsRoute  = "GET " + groupPath + "/parents"
	groupEmptyPageBody     = `{"data":[],"total_count":0,"page_count":0}`
)

// The bodies of replies of the API: a group as a get and a list return it, and as a create and an
// assignment of a user return it.
const (
	groupSchemaBody = `{"group_instance_key":"engineering","group_tenant":"tenant-a",` +
		`"id":"00000000-0000-4000-8000-000000000081"}`
	groupReadBody = `{"group_instance_key":"engineering","group_tenant":"tenant-a",` +
		`"users":["00000000-0000-4000-8000-000000000031"]}`
)

// groupMembership is the membership of the tests, in the tenant tenant-a; groupRole is the role
// editor on the instance doc-1 of the resource document in that tenant, and groupChild names the
// group backend, as the tests assign them to a group.
var (
	groupMembership = models.GroupAssignUser{Tenant: "tenant-a"}
	groupRole       = models.GroupAddRole{
		Role: "editor", Resource: "document", ResourceInstance: "doc-1", Tenant: "tenant-a",
	}
	groupChild = models.GroupAssignment{GroupInstanceKey: "group:backend"}
)

// groupRolesOf returns the roles r<from> to r<to-1> of a group, with only a key: an empty list
// when to is from.
func groupRolesOf(from, to int) []models.GroupRoleReadSchema {
	items := make([]models.GroupRoleReadSchema, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.GroupRoleReadSchema{Key: "r" + strconv.Itoa(i)})
	}
	return items
}

// groupsOf returns the groups g<from> to g<to-1>, with only an instance key: an empty list when
// to is from.
func groupsOf(from, to int) []models.GroupReadSchema {
	items := make([]models.GroupReadSchema, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.GroupReadSchema{GroupInstanceKey: "g" + strconv.Itoa(i)})
	}
	return items
}

// groupUsersOf returns the users u<from> to u<to-1> of a group, with only a key: an empty list
// when to is from.
func groupUsersOf(from, to int) []models.GroupUserReadSchema {
	items := make([]models.GroupUserReadSchema, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.GroupUserReadSchema{Key: "u" + strconv.Itoa(i)})
	}
	return items
}

// groupCall is a call of a method of the groups, with the ID of its operation, its route and a
// reply of the API to it. Each call takes the instance key of a group and the key of a user;
// a method that names no group or no user leaves them out of its request.
type groupCall struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, groupKey, userKey string) error
}

// The calls of the methods of the groups, by what their paths name: nothing but the scope, one
// group, and one user of a group.
var (
	groupListCalls = []groupCall{
		{
			name: "List", op: opListGroups, route: listGroupsRoute,
			reply: mockpermit.Reply{Body: `{"data":[],"total_count":0,"page_count":0}`},
			call: func(ctx context.Context, c *Client, _, _ string) error {
				_, err := c.Groups.List(ctx, nil)
				return err
			},
		},
		{
			name: "Create", op: opCreateGroup, route: createGroupRoute,
			reply: mockpermit.Reply{Body: groupReadBody},
			call: func(ctx context.Context, c *Client, _, _ string) error {
				_, err := c.Groups.Create(ctx, models.GroupCreate{
					GroupInstanceKey: "engineering", GroupTenant: "tenant-a",
				})
				return err
			},
		},
	}
	groupKeyCalls = []groupCall{
		{
			name: "Get", op: opGetGroup, route: getGroupRoute,
			reply: mockpermit.Reply{Body: groupSchemaBody},
			call: func(ctx context.Context, c *Client, groupKey, _ string) error {
				_, err := c.Groups.Get(ctx, groupKey)
				return err
			},
		},
		{
			name: "Delete", op: opDeleteGroup, route: deleteGroupRoute,
			reply: mockpermit.Reply{Status: http.StatusNoContent},
			call: func(ctx context.Context, c *Client, groupKey, _ string) error {
				return c.Groups.Delete(ctx, groupKey)
			},
		},
		{
			name: "ListUsers", op: opListGroupUsers, route: listGroupUsersRoute,
			reply: mockpermit.Reply{Body: `{"data":[],"total_count":0,"page_count":0}`},
			call: func(ctx context.Context, c *Client, groupKey, _ string) error {
				_, err := c.Groups.ListUsers(ctx, groupKey, nil)
				return err
			},
		},
		{
			name: "AssignRole", op: opAssignRoleToGroup, route: assignGroupRoleRoute,
			reply: mockpermit.Reply{Body: groupReadBody},
			call: func(ctx context.Context, c *Client, groupKey, _ string) error {
				_, err := c.Groups.AssignRole(ctx, groupKey, groupRole)
				return err
			},
		},
		{
			name: "RemoveRole", op: opRemoveRoleFromGroup, route: removeGroupRoleRoute,
			reply: mockpermit.Reply{Status: http.StatusNoContent},
			call: func(ctx context.Context, c *Client, groupKey, _ string) error {
				return c.Groups.RemoveRole(ctx, groupKey, groupRole)
			},
		},
		{
			name: "ListRoles", op: opListGroupRoles, route: listGroupRolesRoute,
			reply: mockpermit.Reply{Body: groupEmptyPageBody},
			call: func(ctx context.Context, c *Client, groupKey, _ string) error {
				_, err := c.Groups.ListRoles(ctx, groupKey, nil)
				return err
			},
		},
		{
			name: "AssignGroup", op: opAssignGroupToGroup, route: assignGroupGroupRoute,
			reply: mockpermit.Reply{Body: groupReadBody},
			call: func(ctx context.Context, c *Client, groupKey, _ string) error {
				_, err := c.Groups.AssignGroup(ctx, groupKey, groupChild)
				return err
			},
		},
		{
			name: "RemoveGroup", op: opRemoveGroupFromGroup, route: removeGroupGroupRoute,
			reply: mockpermit.Reply{Status: http.StatusNoContent},
			call: func(ctx context.Context, c *Client, groupKey, _ string) error {
				return c.Groups.RemoveGroup(ctx, groupKey, groupChild)
			},
		},
		{
			name: "ListChildren", op: opListGroupChildren, route: listGroupChildrenRoute,
			reply: mockpermit.Reply{Body: groupEmptyPageBody},
			call: func(ctx context.Context, c *Client, groupKey, _ string) error {
				_, err := c.Groups.ListChildren(ctx, groupKey, nil)
				return err
			},
		},
		{
			name: "ListParents", op: opListGroupParents, route: listGroupParentsRoute,
			reply: mockpermit.Reply{Body: groupEmptyPageBody},
			call: func(ctx context.Context, c *Client, groupKey, _ string) error {
				_, err := c.Groups.ListParents(ctx, groupKey, nil)
				return err
			},
		},
	}
	groupUserCalls = []groupCall{
		{
			name: "AssignUser", op: opAssignUserToGroup, route: assignGroupUserRoute,
			reply: mockpermit.Reply{Body: groupReadBody},
			call: func(ctx context.Context, c *Client, groupKey, userKey string) error {
				_, err := c.Groups.AssignUser(ctx, groupKey, userKey, groupMembership)
				return err
			},
		},
		{
			name: "RemoveUser", op: opRemoveUserFromGroup, route: removeGroupUserRoute,
			reply: mockpermit.Reply{Status: http.StatusNoContent},
			call: func(ctx context.Context, c *Client, groupKey, userKey string) error {
				return c.Groups.RemoveUser(ctx, groupKey, userKey, groupMembership)
			},
		},
	}
)

// groupCalls returns every call of groupListCalls, groupKeyCalls and groupUserCalls.
func groupCalls() []groupCall {
	var calls []groupCall
	for _, list := range [][]groupCall{groupListCalls, groupKeyCalls, groupUserCalls} {
		calls = append(calls, list...)
	}
	return calls
}

// groupPages is a list of the groups with pages: the route of its list, the path that each of its
// requests goes to, its items i<from> to i<to-1>, and its iterator with ctx, the client c and
// opts.
type groupPages[T any] struct {
	route   string
	rawPath string
	items   func(from, to int) []T
	all     func(ctx context.Context, c *Client, opts *ListOptions) iter.Seq2[T, error]
}

// groupsPages and groupUsersPages are the lists of the groups and of the users of the group
// engineering.
var (
	groupsPages = groupPages[models.GroupReadSchema]{
		route: listGroupsRoute, rawPath: groupsRaw + "/direct", items: groupsOf,
		all: func(ctx context.Context, c *Client, opts *ListOptions,
		) iter.Seq2[models.GroupReadSchema, error] {
			var o *GroupsListOptions
			if opts != nil {
				o = &GroupsListOptions{ListOptions: *opts}
			}
			return c.Groups.All(ctx, o)
		},
	}
	groupUsersPages = groupPages[models.GroupUserReadSchema]{
		route: listGroupUsersRoute, rawPath: engineeringUsersRaw, items: groupUsersOf,
		all: func(ctx context.Context, c *Client, opts *ListOptions,
		) iter.Seq2[models.GroupUserReadSchema, error] {
			return c.Groups.AllUsers(ctx, "engineering", opts)
		},
	}
)

// groupRolesPages, groupChildrenPages and groupParentsPages are the lists of the roles, of the
// children and of the parents of the group engineering.
var (
	groupRolesPages = groupPages[models.GroupRoleReadSchema]{
		route: listGroupRolesRoute, rawPath: groupsRaw + "/engineering/roles",
		items: groupRolesOf,
		all: func(ctx context.Context, c *Client, opts *ListOptions,
		) iter.Seq2[models.GroupRoleReadSchema, error] {
			return c.Groups.AllRoles(ctx, "engineering", opts)
		},
	}
	groupChildrenPages = groupPages[models.GroupReadSchema]{
		route: listGroupChildrenRoute, rawPath: groupsRaw + "/engineering/children",
		items: groupsOf,
		all: func(ctx context.Context, c *Client, opts *ListOptions,
		) iter.Seq2[models.GroupReadSchema, error] {
			return c.Groups.AllChildren(ctx, "engineering", opts)
		},
	}
	groupParentsPages = groupPages[models.GroupReadSchema]{
		route: listGroupParentsRoute, rawPath: groupsRaw + "/engineering/parents",
		items: groupsOf,
		all: func(ctx context.Context, c *Client, opts *ListOptions,
		) iter.Seq2[models.GroupReadSchema, error] {
			return c.Groups.AllParents(ctx, "engineering", opts)
		},
	}
)

// groupPagesErrors returns the errors that the iterator of p yields with the client c and opts.
func groupPagesErrors[T any](t *testing.T, p groupPages[T], c *Client, opts *ListOptions,
) []error {
	t.Helper()
	_, errs := collect(t, p.all(t.Context(), c, opts))
	return errs
}

// groupIteratorErrors returns the errors that each iterator of the groups yields with the client
// c and opts, by the name of its method: All, and AllUsers, AllRoles, AllChildren and AllParents
// of the group engineering.
func groupIteratorErrors(t *testing.T, c *Client, opts *ListOptions) map[string][]error {
	t.Helper()
	return map[string][]error{
		"All":         groupPagesErrors(t, groupsPages, c, opts),
		"AllUsers":    groupPagesErrors(t, groupUsersPages, c, opts),
		"AllRoles":    groupPagesErrors(t, groupRolesPages, c, opts),
		"AllChildren": groupPagesErrors(t, groupChildrenPages, c, opts),
		"AllParents":  groupPagesErrors(t, groupParentsPages, c, opts),
	}
}

// checkGroupPagesReadToTheEnd checks that the iterator of p reads 31 items, in order, asking for
// each page in turn: up to the page whose items reach the total_count of its reply, at a
// per_page of 30 and at the default per_page of 100, the most that the API gives on a page; up to
// the empty page when the replies do not report total_count; and from page 2 on, when the options
// start there, up to the empty page after it, as a loop that starts after the first page does not
// stop at the total_count.
func checkGroupPagesReadToTheEnd[T any](t *testing.T, p groupPages[T]) {
	t.Helper()
	for _, tc := range []struct {
		name    string
		opts    *ListOptions
		replies []mockpermit.Reply
		want    []T
		queries []string
	}{
		{
			name: "per_page 30", opts: &ListOptions{PerPage: 30},
			replies: []mockpermit.Reply{
				pageOf(t, p.items(0, 30), `,"total_count":31,"page_count":30`),
				pageOf(t, p.items(30, 31), `,"total_count":31,"page_count":1`),
			},
			want:    p.items(0, 31),
			queries: []string{"page=1&per_page=30", "page=2&per_page=30"},
		},
		{
			name: "the default per_page",
			replies: []mockpermit.Reply{
				pageOf(t, p.items(0, 31), `,"total_count":31,"page_count":31`),
			},
			want:    p.items(0, 31),
			queries: []string{"page=1&per_page=100"},
		},
		{
			name: "pages without total_count", opts: &ListOptions{PerPage: 30},
			replies: []mockpermit.Reply{
				pageOf(t, p.items(0, 30), `,"page_count":30`),
				pageOf(t, p.items(30, 31), `,"page_count":1`),
				pageOf(t, p.items(0, 0), `,"page_count":0`),
			},
			want: p.items(0, 31),
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name: "from page 2", opts: &ListOptions{Page: 2, PerPage: 30},
			replies: []mockpermit.Reply{
				pageOf(t, p.items(30, 31), `,"total_count":31,"page_count":1`),
				pageOf(t, p.items(0, 0), `,"total_count":31,"page_count":0`),
			},
			want:    p.items(30, 31),
			queries: []string{"page=2&per_page=30", "page=3&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(p.route, tc.replies...)
			items, errs := collect(t, p.all(t.Context(), resourceClient(t, api), tc.opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("the iterator yielded the errors %v and items that differ "+
					"(-want +got):\n%s", errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
			for _, r := range api.Requests() {
				if r.RawPath != p.rawPath {
					t.Errorf("the iterator sent a request to %s, want %s", r.RawPath, p.rawPath)
				}
			}
		})
	}
}

// TestGroupsAllReadsEveryPage checks that All reads every page of the groups, as
// checkGroupPagesReadToTheEnd says.
func TestGroupsAllReadsEveryPage(t *testing.T) {
	checkGroupPagesReadToTheEnd(t, groupsPages)
}

// TestGroupUsersAllReadsEveryPage checks that AllUsers reads every page of the users of a group,
// as checkGroupPagesReadToTheEnd says.
func TestGroupUsersAllReadsEveryPage(t *testing.T) {
	checkGroupPagesReadToTheEnd(t, groupUsersPages)
}

// TestGroupRelativesAllReadEveryPage checks that AllRoles, AllChildren and AllParents read every
// page of the roles, of the children and of the parents of a group, as
// checkGroupPagesReadToTheEnd says.
func TestGroupRelativesAllReadEveryPage(t *testing.T) {
	t.Run("AllRoles", func(t *testing.T) { checkGroupPagesReadToTheEnd(t, groupRolesPages) })
	t.Run("AllChildren", func(t *testing.T) { checkGroupPagesReadToTheEnd(t, groupChildrenPages) })
	t.Run("AllParents", func(t *testing.T) { checkGroupPagesReadToTheEnd(t, groupParentsPages) })
}

// TestGroupUsersListReadsEachPage checks that ListUsers reads the page that it asks for, page 1
// and then page 2 of a group of 31 users at a per_page of 30, each with the users of the reply in
// its order and the counts of the reply: the total_count of the group, and the page_count as the
// reply sends it, the number of users on the page.
func TestGroupUsersListReadsEachPage(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(listGroupUsersRoute,
		pageOf(t, groupUsersOf(0, 30), `,"total_count":31,"page_count":30`),
		pageOf(t, groupUsersOf(30, 31), `,"total_count":31,"page_count":1`))
	c := resourceClient(t, api)
	var got []models.GroupUserReadSchema
	for i, pageCount := range []int{30, 1} {
		page := i + 1
		p, err := c.Groups.ListUsers(t.Context(), "engineering",
			&ListOptions{Page: page, PerPage: 30})
		if err != nil {
			t.Fatalf("ListUsers of page %d returned %v", page, err)
		}
		if p.Page != page || p.PerPage != 30 || p.TotalCount == nil || *p.TotalCount != 31 ||
			p.PageCount == nil || *p.PageCount != pageCount {
			t.Errorf("ListUsers of page %d returned the page %d of %d with the counts %v and "+
				"%v, want 31 users and a page_count of %d", page, p.Page, p.PerPage,
				p.TotalCount, p.PageCount, pageCount)
		}
		got = append(got, p.Data...)
	}
	if diff := cmp.Diff(groupUsersOf(0, 31), got); diff != "" {
		t.Errorf("the two pages hold users that differ (-want +got):\n%s", diff)
	}
	want := []string{"page=1&per_page=30", "page=2&per_page=30"}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// checkGroupPagesReadTheOptionsOnce checks that the iterator of p keeps the options of its call,
// so that a later change to them changes no request, that a loop that breaks asks for no other
// page, and that each loop over the iterator reads the items again from the first page.
func checkGroupPagesReadTheOptionsOnce[T any](t *testing.T, p groupPages[T]) {
	t.Helper()
	api := mockpermit.NewAPI(t)
	two := pageOf(t, p.items(0, 2), `,"total_count":4,"page_count":2`)
	api.Handle(p.route, two, two)
	opts := &ListOptions{PerPage: 2}
	seq := p.all(t.Context(), resourceClient(t, api), opts)
	opts.Page, opts.PerPage = 5, 50
	for loop := 1; loop <= 2; loop++ {
		for item, err := range seq {
			if err != nil || !cmp.Equal(item, p.items(0, 1)[0]) {
				t.Errorf("loop %d yielded %+v and %v first, want the first item", loop, item, err)
			}
			break
		}
	}
	first := "page=1&per_page=2"
	if diff := cmp.Diff([]string{first, first}, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestGroupsAllReadsTheOptionsOnce checks All, AllUsers, AllRoles, AllChildren and AllParents as
// checkGroupPagesReadTheOptionsOnce says, and that All also keeps the filters of its call.
func TestGroupsAllReadsTheOptionsOnce(t *testing.T) {
	t.Run("All", func(t *testing.T) { checkGroupPagesReadTheOptionsOnce(t, groupsPages) })
	t.Run("AllUsers", func(t *testing.T) { checkGroupPagesReadTheOptionsOnce(t, groupUsersPages) })
	t.Run("AllRoles", func(t *testing.T) { checkGroupPagesReadTheOptionsOnce(t, groupRolesPages) })
	t.Run("AllChildren", func(t *testing.T) {
		checkGroupPagesReadTheOptionsOnce(t, groupChildrenPages)
	})
	t.Run("AllParents", func(t *testing.T) {
		checkGroupPagesReadTheOptionsOnce(t, groupParentsPages)
	})
	t.Run("the filters of All", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(listGroupsRoute, pageOf(t, groupsOf(0, 1), `,"total_count":1,"page_count":1`))
		opts := &GroupsListOptions{Tenant: "tenant-a"}
		seq := resourceClient(t, api).Groups.All(t.Context(), opts)
		opts.Tenant, opts.Search = "tenant-b", "x"
		if _, errs := collect(t, seq); errs != nil {
			t.Errorf("All yielded the errors %v", errs)
		}
		want := []string{"page=1&per_page=100&tenant=tenant-a"}
		if diff := cmp.Diff(want, queries(api)); diff != "" {
			t.Errorf("the queries differ (-want +got):\n%s", diff)
		}
	})
}

// checkGroupPagesYieldAnErrorOnce checks that a page that fails ends the iterator of p with its
// error, once, after the items of the pages before it.
func checkGroupPagesYieldAnErrorOnce[T any](t *testing.T, p groupPages[T]) {
	t.Helper()
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(p.route, pageOf(t, p.items(0, 30), `,"total_count":60`),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	seq := p.all(t.Context(), resourceClient(t, api), &ListOptions{PerPage: 30})
	items, errs := collect(t, seq)
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("the iterator yielded %d items and the errors %v, want 30 and one error that "+
			"matches ErrForbidden", len(items), errs)
	}
	if n := len(api.Requests()); n != 2 {
		t.Errorf("the iterator sent %d requests, want 2", n)
	}
}

// TestGroupsAllYieldsAnErrorOnce checks All, AllUsers, AllRoles, AllChildren and AllParents as
// checkGroupPagesYieldAnErrorOnce says.
func TestGroupsAllYieldsAnErrorOnce(t *testing.T) {
	t.Run("All", func(t *testing.T) { checkGroupPagesYieldAnErrorOnce(t, groupsPages) })
	t.Run("AllUsers", func(t *testing.T) { checkGroupPagesYieldAnErrorOnce(t, groupUsersPages) })
	t.Run("AllRoles", func(t *testing.T) { checkGroupPagesYieldAnErrorOnce(t, groupRolesPages) })
	t.Run("AllChildren", func(t *testing.T) {
		checkGroupPagesYieldAnErrorOnce(t, groupChildrenPages)
	})
	t.Run("AllParents", func(t *testing.T) {
		checkGroupPagesYieldAnErrorOnce(t, groupParentsPages)
	})
}

// checkGroupPagesGetTheScopeInEachLoop checks that the iterator of p, without WithScope, looks up
// the scope in each loop over it, before its first page, and not when it is made: a lookup that
// fails ends that loop with its error, and the next loop looks the scope up again, and then sends
// its page to the path of p under the project and the environment of the scope.
func checkGroupPagesGetTheScopeInEachLoop[T any](t *testing.T, p groupPages[T]) {
	t.Helper()
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
		scopeReply(t, ""))
	api.Handle(p.route, pageOf(t, p.items(0, 1), `,"total_count":1,"page_count":1`))
	seq := p.all(t.Context(), projectClient(t, api), nil)
	if items, errs := collect(t, seq); len(items) != 0 || len(errs) != 1 ||
		!errors.Is(errs[0], ErrForbidden) {
		t.Errorf("the first loop yielded %d items and the errors %v, want one error that "+
			"matches ErrForbidden", len(items), errs)
	}
	if items, errs := collect(t, seq); len(items) != 1 || errs != nil {
		t.Errorf("the second loop yielded %d items and the errors %v, want 1", len(items), errs)
	}
	var paths []string
	for _, r := range api.Requests() {
		paths = append(paths, r.RawPath)
	}
	prefix := "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID + "/groups"
	want := []string{
		"/v2/api-key/scope", "/v2/api-key/scope", strings.Replace(p.rawPath, groupsRaw, prefix, 1),
	}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("the requests differ (-want +got):\n%s", diff)
	}
}

// TestGroupsAllGetsTheScopeInEachLoop checks All, AllUsers, AllRoles, AllChildren and AllParents
// as checkGroupPagesGetTheScopeInEachLoop says.
func TestGroupsAllGetsTheScopeInEachLoop(t *testing.T) {
	t.Run("All", func(t *testing.T) { checkGroupPagesGetTheScopeInEachLoop(t, groupsPages) })
	t.Run("AllUsers", func(t *testing.T) {
		checkGroupPagesGetTheScopeInEachLoop(t, groupUsersPages)
	})
	t.Run("AllRoles", func(t *testing.T) {
		checkGroupPagesGetTheScopeInEachLoop(t, groupRolesPages)
	})
	t.Run("AllChildren", func(t *testing.T) {
		checkGroupPagesGetTheScopeInEachLoop(t, groupChildrenPages)
	})
	t.Run("AllParents", func(t *testing.T) {
		checkGroupPagesGetTheScopeInEachLoop(t, groupParentsPages)
	})
}

// TestGroupsListReturnsAPage checks the page that List returns: the groups of the reply in its
// order, the page and the per_page that it asked for, page 1 and no query for nil options, and
// the counts that the reply reports, nil for a count that it leaves out. A page with groups and a
// page without them both have a list that is not nil, empty for the second. Each filter that is
// set is sent in its query parameter, as it is given, and one that is empty is left out.
func TestGroupsListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *GroupsListOptions
		reply mockpermit.Reply
		query string
		want  *Page[models.GroupReadSchema]
	}{
		{
			name:  "nil options",
			reply: pageOf(t, groupsOf(0, 2), `,"total_count":2,"page_count":2`),
			want: &Page[models.GroupReadSchema]{
				Data: groupsOf(0, 2), Page: 1, TotalCount: new(2), PageCount: new(2),
			},
		},
		{
			name:  "a page without page_count",
			opts:  &GroupsListOptions{ListOptions: ListOptions{PerPage: 1}},
			reply: pageOf(t, groupsOf(0, 1), `,"total_count":3`),
			query: "per_page=1",
			want: &Page[models.GroupReadSchema]{
				Data: groupsOf(0, 1), Page: 1, PerPage: 1, TotalCount: new(3),
			},
		},
		{
			name:  "a page after the last",
			opts:  &GroupsListOptions{ListOptions: ListOptions{Page: 3, PerPage: 30}},
			reply: pageOf(t, []models.GroupReadSchema{}, `,"total_count":31,"page_count":0`),
			query: "page=3&per_page=30",
			want: &Page[models.GroupReadSchema]{
				Data: []models.GroupReadSchema{}, Page: 3, PerPage: 30, TotalCount: new(31),
				PageCount: new(0),
			},
		},
		{
			name:  "no groups",
			opts:  &GroupsListOptions{},
			reply: mockpermit.Reply{Body: `{"data":[],"total_count":0,"page_count":0}`},
			want: &Page[models.GroupReadSchema]{
				Data: []models.GroupReadSchema{}, Page: 1, TotalCount: new(0), PageCount: new(0),
			},
		},
		{
			name:  "the tenant",
			opts:  &GroupsListOptions{Tenant: "a b"},
			reply: pageOf(t, groupsOf(0, 0), `,"total_count":0`),
			query: "tenant=a+b",
			want: &Page[models.GroupReadSchema]{
				Data: []models.GroupReadSchema{}, Page: 1, TotalCount: new(0),
			},
		},
		{
			name:  "the resource",
			opts:  &GroupsListOptions{Resource: "team"},
			reply: pageOf(t, groupsOf(0, 0), `,"total_count":0`),
			query: "resource=team",
			want: &Page[models.GroupReadSchema]{
				Data: []models.GroupReadSchema{}, Page: 1, TotalCount: new(0),
			},
		},
		{
			name:  "the search",
			opts:  &GroupsListOptions{Search: "eng&x"},
			reply: pageOf(t, groupsOf(0, 0), `,"total_count":0`),
			query: "search=eng%26x",
			want: &Page[models.GroupReadSchema]{
				Data: []models.GroupReadSchema{}, Page: 1, TotalCount: new(0),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listGroupsRoute, tc.reply)
			got, err := resourceClient(t, api).Groups.List(t.Context(), tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			want := groupsRaw + "/direct"
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != want || requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to %s?%s", requests, want, tc.query)
			}
		})
	}
}

// TestInvalidGroupListOptions checks that a Page or a PerPage less than zero is an error that
// matches ErrInvalidConfig from each list of the groups and from each of their iterators, once,
// and that none sends a request.
func TestInvalidGroupListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		_, listErr := c.Groups.List(t.Context(), &GroupsListOptions{ListOptions: opts})
		_, usersErr := c.Groups.ListUsers(t.Context(), "engineering", &opts)
		_, rolesErr := c.Groups.ListRoles(t.Context(), "engineering", &opts)
		_, childrenErr := c.Groups.ListChildren(t.Context(), "engineering", &opts)
		_, parentsErr := c.Groups.ListParents(t.Context(), "engineering", &opts)
		for _, err := range []error{listErr, usersErr, rolesErr, childrenErr, parentsErr} {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("%+v: the call returned %v, want an error that matches ErrInvalidConfig",
					opts, err)
			}
		}
		for name, errs := range groupIteratorErrors(t, c, &opts) {
			if len(errs) != 1 || !errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("%+v: %s yielded the errors %v, want one that matches "+
					"ErrInvalidConfig", opts, name, errs)
			}
		}
		api.AssertUntouched()
	}
}

// TestGroupsUseTheScope checks that the calls of the groups act on the environment of the
// client's scope. Without WithScope, the client looks up the scope of its API key once, and each
// call fills the route with the project and the environment of an environment-level key: Get
// runs against a server of its own, as a path of its route, such as /groups/direct/users, fits
// the route of ListUsers too, or of another list of a group, which mockpermit refuses to serve
// beside it. A scope without an environment, that of a project-level or an organization-level key,
// is an error that matches ErrInvalidConfig and names the operation, and no call sends a request
// but the lookup, which the API fails for any route that the test did not register.
func TestGroupsUseTheScope(t *testing.T) {
	const prefix = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID + "/groups"
	var get, others []groupCall
	for _, m := range groupCalls() {
		if m.op == opGetGroup {
			get = append(get, m)
		} else {
			others = append(others, m)
		}
	}
	for _, calls := range [][]groupCall{others, get} {
		t.Run("an environment-level key with "+calls[0].name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, ""))
			for _, m := range calls {
				api.Handle(m.route, m.reply)
			}
			c := projectClient(t, api)
			for _, m := range calls {
				if err := m.call(t.Context(), c, "engineering", "alice"); err != nil {
					t.Errorf("the call of %s returned %v", m.op, err)
				}
			}
			requests := api.Requests()
			if len(requests) != len(calls)+1 || requests[0].RawPath != "/v2/api-key/scope" {
				t.Fatalf("the calls sent %+v, want the lookup of the scope and one request for "+
					"each of the %d calls", requests, len(calls))
			}
			for _, r := range requests[1:] {
				if r.RawPath != prefix && !strings.HasPrefix(r.RawPath, prefix+"/") {
					t.Errorf("a call sent its request to %s, want a path under %s", r.RawPath,
						prefix)
				}
			}
		})
	}
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for _, m := range groupCalls() {
				err := m.call(t.Context(), c, "engineering", "alice")
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), m.op) {
					t.Errorf("the call of %s returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", m.op, err, m.op)
				}
			}
			for name, errs := range groupIteratorErrors(t, c, nil) {
				if len(errs) != 1 || !errors.Is(errs[0], ErrInvalidConfig) {
					t.Errorf("%s yielded %v, want one error that matches ErrInvalidConfig",
						name, errs)
				}
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestGroupKeysInThePath checks that each method sends the instance key of the group, and the key
// of the user when it names one, each as one segment of the path, escaped, after the project and
// the environment of the scope, whatever the key holds: escaped where the key holds a slash, a
// percent sign, a space, a question mark and a hash, or a letter outside ASCII, and as it is for
// the shape of a UUID, which the SDK does not tell from a key, a leading dot and dots inside a key.
// A key that is empty, . or .. would change the route, so it is an error that matches
// ErrInvalidConfig and names its placeholder, and no request.
func TestGroupKeysInThePath(t *testing.T) {
	paths := map[string]string{
		"Get": groupsRaw + "/direct/%s", "Delete": groupsRaw + "/%s",
		"ListUsers": groupsRaw + "/%s/users", "AssignUser": groupsRaw + "/%s/users/%s",
		"RemoveUser": groupsRaw + "/%s/users/%s", "AssignRole": groupsRaw + "/%s/roles",
		"RemoveRole": groupsRaw + "/%s/roles", "ListRoles": groupsRaw + "/%s/roles",
		"AssignGroup": groupsRaw + "/%s/assign_group", "ListChildren": groupsRaw + "/%s/children",
		"RemoveGroup": groupsRaw + "/%s/assign_group", "ListParents": groupsRaw + "/%s/parents",
	}
	for key, segment := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"a b?c#d":        "a%20b%3Fc%23d",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
		"eng.v2..x":      "eng.v2..x",
	} {
		for _, m := range append(append([]groupCall{}, groupKeyCalls...), groupUserCalls...) {
			for _, keys := range [][2]string{{key, "alice"}, {"engineering", key}} {
				if m.route != assignGroupUserRoute && m.route != removeGroupUserRoute &&
					keys[0] != key {
					continue
				}
				api := mockpermit.NewAPI(t)
				api.Handle(m.route, m.reply)
				c := resourceClient(t, api)
				if err := m.call(t.Context(), c, keys[0], keys[1]); err != nil {
					t.Errorf("%s(%q, %q) returned %v", m.name, keys[0], keys[1], err)
				}
				group, user := segment, "alice"
				if keys[0] != key {
					group, user = "engineering", segment
				}
				want := strings.Replace(paths[m.name], "%s", group, 1)
				want = strings.Replace(want, "%s", user, 1)
				if requests := api.Requests(); len(requests) != 1 ||
					requests[0].RawPath != want {
					t.Errorf("%s(%q, %q) sent %+v, want one request to %s", m.name, keys[0],
						keys[1], requests, want)
				}
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range groupKeyCalls {
			api := mockpermit.NewAPI(t)
			checkGroupKeyError(t, m.name, key, "{group_instance_key}",
				m.call(t.Context(), resourceClient(t, api), key, ""))
			api.AssertUntouched()
		}
		for _, m := range groupUserCalls {
			api := mockpermit.NewAPI(t)
			c := resourceClient(t, api)
			checkGroupKeyError(t, m.name, key, "{group_instance_key}",
				m.call(t.Context(), c, key, "alice"))
			checkGroupKeyError(t, m.name, key, "{user_id}",
				m.call(t.Context(), c, "engineering", key))
			api.AssertUntouched()
		}
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		for name, seq := range map[string]func() []error{
			"AllUsers": func() []error {
				_, errs := collect(t, c.Groups.AllUsers(t.Context(), key, nil))
				return errs
			},
			"AllRoles": func() []error {
				_, errs := collect(t, c.Groups.AllRoles(t.Context(), key, nil))
				return errs
			},
			"AllChildren": func() []error {
				_, errs := collect(t, c.Groups.AllChildren(t.Context(), key, nil))
				return errs
			},
			"AllParents": func() []error {
				_, errs := collect(t, c.Groups.AllParents(t.Context(), key, nil))
				return errs
			},
		} {
			errs := seq()
			if len(errs) != 1 {
				t.Fatalf("%s with %q yielded %v, want one error", name, key, errs)
			}
			checkGroupKeyError(t, name, key, "{group_instance_key}", errs[0])
		}
		api.AssertUntouched()
	}
}

// checkGroupKeyError checks that err, the error of the call of method with key in the
// placeholder, matches ErrInvalidConfig and names the placeholder.
func checkGroupKeyError(t *testing.T, method, key, placeholder string, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), placeholder) {
		t.Errorf("%s with %q in %s returned %v, want an error that matches ErrInvalidConfig "+
			"and names %s", method, key, placeholder, err, placeholder)
	}
}

// TestDeleteGroupThatIsGone checks that Delete returns nil for a 404, with or without a body, as
// the group that the path names is gone already, after one request to the route of the group;
// and an *Error with the status for any other error status, such as 403, 409 or 422.
func TestDeleteGroupThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the group does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteGroupRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := resourceClient(t, api).Groups.Delete(t.Context(), "gone")
		if err != nil {
			t.Errorf("Delete of a group that the API answers with 404 and the body %q returned "+
				"%v, want nil", body, err)
		}
		requests := api.Requests()
		if want := groupsRaw + "/gone"; len(requests) != 1 || requests[0].RawPath != want {
			t.Errorf("Delete sent %+v, want one request to %s", requests, want)
		}
	}
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteGroupRoute, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).Groups.Delete(t.Context(), "engineering")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteGroup ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteGroup)
		}
	}
}

// TestNotFoundOfOtherGroupCalls checks that a 404 to every call but Delete is an *Error that
// matches ErrNotFound: the lists, Create, Get and the assignments delete nothing, and RemoveUser,
// RemoveRole and RemoveGroup, whose bodies name what they remove, the tenant of the membership,
// the role or the group, may have been sent with a mistyped key, so their 404 is an error too,
// with or without a body.
func TestNotFoundOfOtherGroupCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no group"}`
	removals := []string{opRemoveUserFromGroup, opRemoveRoleFromGroup, opRemoveGroupFromGroup}
	for _, m := range groupCalls() {
		if m.op == opDeleteGroup {
			continue
		}
		bodies := []string{body}
		if slices.Contains(removals, m.op) {
			bodies = append(bodies, "")
		}
		for _, b := range bodies {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, mockpermit.Reply{Status: http.StatusNotFound, Body: b})
			err := m.call(t.Context(), resourceClient(t, api), "gone", "alice")
			if e := mustError(t, err); !errors.Is(err, ErrNotFound) ||
				e.StatusCode != http.StatusNotFound || e.Op != m.op {
				t.Errorf("%s that the API answers with 404 and the body %q returned %v, want an "+
					"*Error of %s that matches ErrNotFound", m.name, b, err, m.op)
			}
		}
	}
}

// TestGroupBodies checks the bodies of Create, AssignUser, RemoveUser, AssignRole, RemoveRole,
// AssignGroup and RemoveGroup byte for byte against the golden files in testdata/golden: a create
// with the required fields only, the zero create, which still sends the instance key and the
// tenant, empty, and a create with every field, whose instance key keeps <, > and & as they are;
// the membership of an assignment and of a removal of a user, with a tenant and the zero
// membership, which still sends the tenant, empty; a role of an assignment and of a removal with
// every field, whose role keeps <, > and & as they are, and the zero role, which still sends each
// field, empty; and the group of an assignment and of a removal of a group, and the zero group,
// which still sends its key, empty.
func TestGroupBodies(t *testing.T) {
	assignRole := func(role models.GroupAddRole) func(ctx context.Context, c *Client) error {
		return func(ctx context.Context, c *Client) error {
			_, err := c.Groups.AssignRole(ctx, "engineering", role)
			return err
		}
	}
	assignGroup := func(a models.GroupAssignment) func(ctx context.Context, c *Client) error {
		return func(ctx context.Context, c *Client) error {
			_, err := c.Groups.AssignGroup(ctx, "engineering", a)
			return err
		}
	}
	everyField := models.GroupAddRole{
		Role: "a" + fixtures.LineSeparator + "b <c> & d", Resource: "document",
		ResourceInstance: "doc-1", Tenant: fixtures.UUIDKey,
	}
	create := func(group models.GroupCreate) func(ctx context.Context, c *Client) error {
		return func(ctx context.Context, c *Client) error {
			_, err := c.Groups.Create(ctx, group)
			return err
		}
	}
	assign := func(m models.GroupAssignUser) func(ctx context.Context, c *Client) error {
		return func(ctx context.Context, c *Client) error {
			_, err := c.Groups.AssignUser(ctx, "engineering", "alice", m)
			return err
		}
	}
	remove := func(m models.GroupAssignUser) func(ctx context.Context, c *Client) error {
		return func(ctx context.Context, c *Client) error {
			return c.Groups.RemoveUser(ctx, "engineering", "alice", m)
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		reply  mockpermit.Reply
		call   func(ctx context.Context, c *Client) error
	}{
		{
			"create_group.required.golden", createGroupRoute,
			mockpermit.Reply{Body: groupReadBody},
			create(models.GroupCreate{GroupInstanceKey: "engineering", GroupTenant: "tenant-a"}),
		},
		{
			"create_group.zero.golden", createGroupRoute,
			mockpermit.Reply{Body: groupReadBody},
			create(models.GroupCreate{}),
		},
		{
			"create_group.every-field.golden", createGroupRoute,
			mockpermit.Reply{Body: groupReadBody},
			create(models.GroupCreate{
				GroupInstanceKey:     "a" + fixtures.LineSeparator + "b <c> & d",
				GroupResourceTypeKey: "team", GroupTenant: fixtures.UUIDKey,
			}),
		},
		{
			"assign_user_to_group.tenant.golden", assignGroupUserRoute,
			mockpermit.Reply{Body: groupReadBody},
			assign(groupMembership),
		},
		{
			"assign_user_to_group.zero.golden", assignGroupUserRoute,
			mockpermit.Reply{Body: groupReadBody},
			assign(models.GroupAssignUser{}),
		},
		{
			"remove_user_from_group.tenant.golden", removeGroupUserRoute,
			mockpermit.Reply{Status: http.StatusNoContent},
			remove(groupMembership),
		},
		{
			"remove_user_from_group.zero.golden", removeGroupUserRoute,
			mockpermit.Reply{Status: http.StatusNoContent},
			remove(models.GroupAssignUser{}),
		},
		{
			"assign_role_to_group.every-field.golden", assignGroupRoleRoute,
			mockpermit.Reply{Body: groupReadBody},
			assignRole(everyField),
		},
		{
			"assign_role_to_group.zero.golden", assignGroupRoleRoute,
			mockpermit.Reply{Body: groupReadBody},
			assignRole(models.GroupAddRole{}),
		},
		{
			"remove_role_from_group.every-field.golden", removeGroupRoleRoute,
			mockpermit.Reply{Status: http.StatusNoContent},
			func(ctx context.Context, c *Client) error {
				return c.Groups.RemoveRole(ctx, "engineering", everyField)
			},
		},
		{
			"assign_group_to_group.key.golden", assignGroupGroupRoute,
			mockpermit.Reply{Body: groupReadBody},
			assignGroup(models.GroupAssignment{
				GroupInstanceKey: "group:a" + fixtures.LineSeparator + "b <c> & d",
			}),
		},
		{
			"assign_group_to_group.zero.golden", assignGroupGroupRoute,
			mockpermit.Reply{Body: groupReadBody},
			assignGroup(models.GroupAssignment{}),
		},
		{
			"remove_group_from_group.key.golden", removeGroupGroupRoute,
			mockpermit.Reply{Status: http.StatusNoContent},
			func(ctx context.Context, c *Client) error {
				return c.Groups.RemoveGroup(ctx, "engineering", groupChild)
			},
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.reply)
			if err := tc.call(t.Context(), resourceClient(t, api)); err != nil {
				t.Fatalf("the call returned %v", err)
			}
			requests := api.Requests()
			if len(requests) != 1 {
				t.Fatalf("the call sent %d requests, want 1", len(requests))
			}
			requests[0].ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
		})
	}
}

// TestGroupRetries checks when a call of the groups sends its request again: Create, AssignUser,
// AssignRole and AssignGroup, which the API may have acted on, after a 429 only, so a 503 ends
// each after one request; the other calls, which do the same when sent twice, after a 503 too.
func TestGroupRetries(t *testing.T) {
	onlyAfter429 := []string{
		opCreateGroup, opAssignUserToGroup, opAssignRoleToGroup, opAssignGroupToGroup,
	}
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	type retryCase struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(c *Client) error
		status  int
	}
	var tests []retryCase
	for _, m := range groupCalls() {
		call := func(c *Client) error { return m.call(t.Context(), c, "engineering", "alice") }
		if slices.Contains(onlyAfter429, m.op) {
			tests = append(tests,
				retryCase{
					name: m.name + " after a 429", route: m.route,
					replies: []mockpermit.Reply{limited, m.reply}, call: call,
				},
				retryCase{
					name: m.name + " after a 503", route: m.route,
					replies: []mockpermit.Reply{unavailable}, call: call,
					status: http.StatusServiceUnavailable,
				})
			continue
		}
		tests = append(tests, retryCase{
			name: m.name + " after a 503", route: m.route,
			replies: []mockpermit.Reply{unavailable, m.reply}, call: call,
		})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.replies...)
			err := tc.call(resourceClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1})))
			if tc.status == 0 && err != nil {
				t.Errorf("the call returned %v, want the result of the second request", err)
			}
			if tc.status != 0 && mustError(t, err).StatusCode != tc.status {
				t.Errorf("the call returned %v, want the error of status %d", err, tc.status)
			}
			if n := len(api.Requests()); n != len(tc.replies) {
				t.Errorf("the call sent %d requests, want %d", n, len(tc.replies))
			}
		})
	}
}

// TestMalformedGroupReplies checks that a reply in 200-299 that is not a group, or for the lists
// not the paginated object of groups or of users, such as a JSON array of them or an object
// without data, is an *Error that names the model, and nothing else.
func TestMalformedGroupReplies(t *testing.T) {
	const prefix = "permit: "
	objects := []struct {
		route, model string
		call         func(c *Client) (any, error)
	}{
		{
			getGroupRoute, "get_direct_group: GET " + groupsPath + "/direct/{group_instance_key}" +
				": status 200: the reply is not a valid models.GroupReadSchema",
			func(c *Client) (any, error) { return c.Groups.Get(t.Context(), "engineering") },
		},
		{
			createGroupRoute, "create_group: POST " + groupsPath +
				": status 200: the reply is not a valid models.GroupRead",
			func(c *Client) (any, error) {
				return c.Groups.Create(t.Context(), models.GroupCreate{})
			},
		},
		{
			assignGroupUserRoute, "assign_user_to_group: PUT " + groupUserPath +
				": status 200: the reply is not a valid models.GroupRead",
			func(c *Client) (any, error) {
				return c.Groups.AssignUser(t.Context(), "engineering", "alice", groupMembership)
			},
		},
		{
			assignGroupRoleRoute, "assign_role_to_group: POST " + groupRolesPath +
				": status 200: the reply is not a valid models.GroupRead",
			func(c *Client) (any, error) {
				return c.Groups.AssignRole(t.Context(), "engineering", groupRole)
			},
		},
		{
			assignGroupGroupRoute, "assign_group_to_group: PUT " + groupAssignPath +
				": status 200: the reply is not a valid models.GroupRead",
			func(c *Client) (any, error) {
				return c.Groups.AssignGroup(t.Context(), "engineering", groupChild)
			},
		},
	}
	for _, o := range objects {
		for _, body := range []string{``, `null`, `[]`, `"g"`, `{"group_tenant":1}`, `{} {}`} {
			api := mockpermit.NewAPI(t)
			api.Handle(o.route, mockpermit.Reply{Body: body})
			got, err := o.call(resourceClient(t, api))
			if e := mustError(t, err); !isNilGroupResult(got) || e.Error() != prefix+o.model {
				t.Errorf("%s of the reply %q returned %+v and\n%v\nwant nothing and\n%s",
					o.route, body, got, err, prefix+o.model)
			}
		}
	}
	lists := []struct {
		route, model, item string
		call               func(c *Client) (any, error)
		all                func(c *Client) (int, []error)
	}{
		{
			listGroupsRoute, "list_direct_group: GET " + groupsPath + "/direct" +
				": status 200: the reply is not a valid list of models.GroupReadSchema",
			`{"group_instance_key":1}`,
			func(c *Client) (any, error) { return c.Groups.List(t.Context(), nil) },
			func(c *Client) (int, []error) {
				items, errs := collect(t, c.Groups.All(t.Context(), nil))
				return len(items), errs
			},
		},
		{
			listGroupUsersRoute, "list_group_users: GET " + groupUsersPath +
				": status 200: the reply is not a valid list of models.GroupUserReadSchema",
			`{"key":1}`,
			func(c *Client) (any, error) {
				return c.Groups.ListUsers(t.Context(), "engineering", nil)
			},
			func(c *Client) (int, []error) {
				items, errs := collect(t, c.Groups.AllUsers(t.Context(), "engineering", nil))
				return len(items), errs
			},
		},
		{
			listGroupRolesRoute, "list_group_roles: GET " + groupRolesPath +
				": status 200: the reply is not a valid list of models.GroupRoleReadSchema",
			`{"key":"editor","resource":[]}`,
			func(c *Client) (any, error) {
				return c.Groups.ListRoles(t.Context(), "engineering", nil)
			},
			func(c *Client) (int, []error) {
				items, errs := collect(t, c.Groups.AllRoles(t.Context(), "engineering", nil))
				return len(items), errs
			},
		},
		{
			listGroupChildrenRoute, "list_group_children: GET " + groupPath + "/children" +
				": status 200: the reply is not a valid list of models.GroupReadSchema",
			`{"group_instance_key":1}`,
			func(c *Client) (any, error) {
				return c.Groups.ListChildren(t.Context(), "engineering", nil)
			},
			func(c *Client) (int, []error) {
				items, errs := collect(t, c.Groups.AllChildren(t.Context(), "engineering", nil))
				return len(items), errs
			},
		},
		{
			listGroupParentsRoute, "list_group_parents: GET " + groupPath + "/parents" +
				": status 200: the reply is not a valid list of models.GroupReadSchema",
			`{"id":1}`,
			func(c *Client) (any, error) {
				return c.Groups.ListParents(t.Context(), "engineering", nil)
			},
			func(c *Client) (int, []error) {
				items, errs := collect(t, c.Groups.AllParents(t.Context(), "engineering", nil))
				return len(items), errs
			},
		},
	}
	for _, l := range lists {
		for _, body := range []string{
			``, `null`, `"g"`, `[]`, `[{"key":"a"}]`, `{"total_count":0}`,
			`{"data":null,"total_count":0}`, `{"data":[` + l.item + `],"total_count":1}`,
			`{"data":[],"total_count":"0"}`,
		} {
			api := mockpermit.NewAPI(t)
			api.Handle(l.route, mockpermit.Reply{Body: body})
			got, err := l.call(resourceClient(t, api))
			if e := mustError(t, err); !isNilGroupResult(got) || e.Error() != prefix+l.model {
				t.Errorf("%s of the reply %q returned %+v and\n%v\nwant no page and\n%s",
					l.route, body, got, err, prefix+l.model)
			}
		}
		for _, body := range []string{`[]`, `[{"key":"a"}]`} {
			api := mockpermit.NewAPI(t)
			api.Handle(l.route, mockpermit.Reply{Body: body})
			n, errs := l.all(resourceClient(t, api))
			if n != 0 || len(errs) != 1 || mustError(t, errs[0]).Error() != prefix+l.model {
				t.Errorf("the iterator of %s of the reply %q yielded %d items and the errors "+
					"%v\nwant none and one error:\n%s", l.route, body, n, errs, prefix+l.model)
			}
		}
	}
}

// isNilGroupResult reports whether v, the result of a call of the groups, is a nil pointer, as
// a call that fails returns.
func isNilGroupResult(v any) bool {
	p := reflect.ValueOf(v)
	return p.Kind() == reflect.Pointer && p.IsNil()
}

// TestGroupsAfterClose checks that each method returns ErrClosed after Close, and sends nothing,
// also without WithScope, whose lookup it does not send either.
func TestGroupsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	for _, m := range groupCalls() {
		if err := m.call(t.Context(), c, "engineering", "alice"); !errors.Is(err, ErrClosed) {
			t.Errorf("%s after Close returned %v, want ErrClosed", m.name, err)
		}
	}
	for name, errs := range groupIteratorErrors(t, c, nil) {
		if len(errs) != 1 || !errors.Is(errs[0], ErrClosed) {
			t.Errorf("%s after Close yielded %v, want ErrClosed once", name, errs)
		}
	}
	api.AssertUntouched()
}
