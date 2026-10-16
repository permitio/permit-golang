package fake

import (
	"net/http"
	"net/url"
	"slices"
)

// The operations of the user invites of an environment that userInvite answers. The fake
// answers the create, the get, the update and the delete of an invite as it answers those of any
// object of a collection.
const (
	opListUserInvites   = "list_user_invites"
	opApproveUserInvite = "approve_user_invite"
)

// userInviteOps are the operations that userInvite answers.
var userInviteOps = []string{opListUserInvites, opApproveUserInvite}

// The statuses of a user invite that the fake gives one.
const (
	invitePending  = "pending"
	inviteApproved = "approved"
)

// userInvite answers a request of the user invites of an environment to rt, whose path has
// segments, with IDs in place of the keys of the project and the environment, whose body holds
// fields, nil without a body, and whose query is query:
//
//   - the list answers the invites of the environment, in the order in which they were created,
//     those that the filters of the query keep, the filters role and tenant those whose role_id
//     and tenant_id are their values, and the filter search those whose email or key is its
//     value, as listFilters says, in the paginated object of the page of the query, as envelope
//     says;
//   - an approval of a pending invite, by its ID or its key, whose email address fields hold,
//     creates the user of the key, the email address and the attributes of fields, as a POST to
//     the users of the environment creates one, sets the status of the invite to approved, and
//     answers the user; an invite that the environment does not have gets 404, and the approval
//     of an invite that is approved already, or of another email address than the invite's, gets
//     400 with the detail of the refusal, as the API's documentation says.
//
// What an approval does is not confirmed yet: the fake creates the user of the reply, as the
// reply of the API is a user, and does not give the user the role of the invite. Nor is the
// filter search, which the documentation says matches the email, the key or the name of an
// invite: the fake reads it as the email or the key, as an invite has no name. The approval of
// an invite whose key a user of the environment has already is a request that the fake does not
// implement. The caller holds f.mu.
func (f *Fake) userInvite(rt route, segments []string, fields object, query url.Values,
) (int, any) {
	invites := collectionOf(segments[:6])
	switch {
	case rt.Op == opListUserInvites:
		return http.StatusOK, envelope(f.keep(rt.Op, segments, f.store[invites], query),
			query)
	case rt.Op == opApproveUserInvite && fields != nil:
		return f.approveUserInvite(rt, segments, fields)
	}
	return f.unsupported(rt)
}

// approveUserInvite answers an approval of the invite of the path, which has segments, with IDs in
// place of the keys of the project and the environment, whose body holds fields, as userInvite
// says. The caller holds f.mu.
func (f *Fake) approveUserInvite(rt route, segments []string, fields object) (int, any) {
	invite, ok := find(f.store[collectionOf(segments[:6])], segments[6])
	switch {
	case !ok:
		return http.StatusNotFound, errorBody("NOT_FOUND", "no user invite "+segments[6])
	case text(invite, "status") != invitePending:
		return http.StatusBadRequest, object{"detail": "the user is approved already"}
	case text(fields, "email") != text(invite, "email"):
		return http.StatusBadRequest, object{
			"detail": "the email requested does not match the email of the invite",
		}
	}
	create, users := bulkCollection(append(slices.Clone(segments[:5]), "users"))
	if _, taken := find(f.store[collectionOf(users)], text(fields, "key")); taken {
		return f.unsupported(rt)
	}
	status, created := f.create(create, users, fields)
	if status == http.StatusOK {
		invite["status"] = inviteApproved
	}
	return status, created
}
