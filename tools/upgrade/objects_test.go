package upgrade_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"testing"

	v1models "github.com/permitio/permit-golang/pkg/models"
	v1 "github.com/permitio/permit-golang/pkg/permit"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
	"github.com/permitio/permit-golang/v2/tools/upgrade"
)

// The keys of the objects that TestUpgrade writes. Each holds upgrade, or names an object of a
// resource, such as an action, so that none starts with gosdk-ci-, as the keys of the objects of
// the end-to-end scenarios in the same scratch environment do. The key of a user may also hold
// |, @, + and ., as the API allows; another key may hold letters, digits, - and _ only.
const (
	docResource      = "upgrade-doc"
	folderResource   = "upgrade-folder"
	archiveAction    = "archive"
	editingGroup     = "editing"
	regionAttr       = "region"
	parentRelation   = "parent"
	editorRole       = "upgrade-editor"
	ownerRole        = "upgrade-owner"
	folderEditorRole = "upgrade-folder-editor"
	viewerRole       = "upgrade-viewer"
	adminRole        = "upgrade-admin"
	firstTenant      = "upgrade-tenant-1"
	secondTenant     = "upgrade-tenant-2"
	firstUser        = "idp|upgrade.user+1@example.com"
	secondUser       = "upgrade_user-2"
	thirdUser        = "upgrade.user.3"
	userSet          = "upgrade-example-users"
	resourceSet      = "upgrade-public-docs"
	bearerProxy      = "upgrade-proxy-bearer"
	basicProxy       = "upgrade-proxy-basic"
)

// The resource instances that TestUpgrade writes, each as <resource>:<key>.
var (
	firstDoc     = instance{docResource, "doc-1"}
	secondDoc    = instance{docResource, "doc-2"}
	firstFolder  = instance{folderResource, "folder-1"}
	secondFolder = instance{folderResource, "folder-2"}
)

// instance is a resource instance: the key of its resource and its own key.
type instance struct {
	resource, key string
}

// String returns the instance as the API names it, <resource>:<key>.
func (i instance) String() string {
	return i.resource + ":" + i.key
}

// The credentials of the proxy configs: plain words, not secrets.
const (
	bearerSecret = "not-a-real-token"
	basicSecret  = "upgrade-user:not-a-real-password"
)

// steps returns the steps of TestUpgrade in r, in the order in which they run: each step writes
// what a later one names first, and the run's ledger tears the objects down in the reverse
// order. They write 31 objects, of every kind that v1 writes in an environment, with hostile
// values that the API accepts: unicode, quotes, a backslash, control characters and an emoji in
// names and descriptions, : and / in a URN, a URL and attribute names, empty and nested
// attributes, an integer beyond 2^53, which a float64 cannot hold, and the values of the open
// enums that v1 can send: each type of an attribute and of a condition set, each HTTP method of
// a mapping rule, the URL type regex, and the auth mechanisms Bearer and Basic; v1 sends a proxy
// config's secret as a string, which the mechanism Headers does not take. v1 also writes
// projects and environments, which hold the other kinds, with an organization's API key; the
// test runs in an environment, with the environment's key.
func steps(r *run) []step {
	b := seed.New(r.api, r.ledger, r.env)
	return []step{
		resourceStep(r),
		{
			name: "resource_with_little",
			register: func(*testing.T) {
				r.register("resource "+folderResource, r.schema("/resources/"+folderResource))
			},
			write: func(ctx context.Context, c *v1.Client) error {
				_, err := c.Api.Resources.Create(ctx, v1models.ResourceCreate{
					Key: folderResource, Name: "Folder",
					Actions: map[string]v1models.ActionBlockEditable{"list": {}},
				})
				return err
			},
			reads: []read{{
				name: "the resource " + folderResource,
				get: func(ctx context.Context, c *permit.Client) (any, error) {
					return view(c.Resources.Get(ctx, folderResource))
				},
			}},
		},
		{
			name: "resource_action",
			register: func(*testing.T) {
				r.register("action "+archiveAction, docPath(r, "/actions/"+archiveAction))
			},
			write: func(ctx context.Context, c *v1.Client) error {
				_, err := c.Api.ResourceActions.Create(ctx, docResource,
					v1models.ResourceActionCreate{
						Key: archiveAction, Name: "Archive 🗄", Description: new("Moves it away"),
						Attributes: map[string]any{"risk": 2.5, "audited": true, "steps": []any{}},
					})
				return err
			},
			reads: []read{{
				name: "the action " + archiveAction,
				get: func(ctx context.Context, c *permit.Client) (any, error) {
					return view(c.ResourceActions.Get(ctx, docResource, archiveAction))
				},
			}},
		},
		{
			name: "resource_action_group",
			register: func(*testing.T) {
				r.register("action group "+editingGroup,
					docPath(r, "/action_groups/"+editingGroup))
			},
			write: func(ctx context.Context, c *v1.Client) error {
				_, err := c.Api.ResourceActionGroups.Create(ctx, docResource,
					v1models.ResourceActionGroupCreate{
						Key: editingGroup, Name: "Editing ✎ – 編集",
						Description: new("Writes \"and\" shares"),
						Attributes: map[string]any{
							"weight": 0.5, "labels": []any{"a", "b"},
							"nested": map[string]any{"empty": map[string]any{}},
						},
						Actions: []string{"write", "share-link_v2"},
					})
				return err
			},
			reads: []read{{
				name: "the action group " + editingGroup,
				get: func(ctx context.Context, c *permit.Client) (any, error) {
					return view(c.ResourceActionGroups.Get(ctx, docResource, editingGroup))
				},
				unordered: []string{"#/actions"},
			}},
		},
		{
			name: "resource_attribute",
			register: func(*testing.T) {
				r.register("attribute "+regionAttr, docPath(r, "/attributes/"+regionAttr))
			},
			write: func(ctx context.Context, c *v1.Client) error {
				_, err := c.Api.ResourceAttributes.Create(ctx, docResource,
					v1models.ResourceAttributeCreate{
						Key: regionAttr, Type: v1models.STRING, Description: new("Où – wo – 何処"),
					})
				return err
			},
			reads: []read{{
				name: "the attribute " + regionAttr,
				get: func(ctx context.Context, c *permit.Client) (any, error) {
					return view(c.ResourceAttributes.Get(ctx, docResource, regionAttr))
				},
			}},
		},
		{
			name: "resource_relation",
			register: func(*testing.T) {
				r.register("relation "+parentRelation,
					docPath(r, "/relations/"+parentRelation))
			},
			write: func(ctx context.Context, c *v1.Client) error {
				_, err := c.Api.ResourceRelations.Create(ctx, docResource, v1models.RelationCreate{
					Key: parentRelation, Name: "Parent folder ↑",
					Description: new("The folder that holds it"), SubjectResource: folderResource,
				})
				return err
			},
			reads: []read{{
				name: "the relation " + parentRelation,
				get: func(ctx context.Context, c *permit.Client) (any, error) {
					return view(c.ResourceRelations.Get(ctx, docResource, parentRelation))
				},
			}},
		},
		resourceRoleStep(r, "resource_role", docResource, v1models.ResourceRoleCreate{
			Key: editorRole, Name: "Éditeur", Description: new("Reads and shares it"),
			Permissions: []string{"read", "share-link_v2"},
		}),
		resourceRoleStep(r, "resource_role_that_extends_another", docResource,
			v1models.ResourceRoleCreate{
				Key: ownerRole, Name: "Owner", Permissions: []string{"write"},
				Extends: []string{editorRole},
			}),
		resourceRoleStep(r, "resource_role_of_another_resource", folderResource,
			v1models.ResourceRoleCreate{
				Key: folderEditorRole, Name: "Folder editor", Permissions: []string{"list"},
			}),
		implicitGrantStep(r),
		roleStep(r, "role", v1models.RoleCreate{
			Key: viewerRole, Name: "Viewer / Leser 👀", Description: new("Reads \"everything\""),
			Permissions: []string{docResource + ":read", folderResource + ":list"},
			Attributes:  map[string]string{"level": "1", "emoji": "👀"},
		}),
		roleStep(r, "role_that_extends_another", v1models.RoleCreate{
			Key: adminRole, Name: "Admin", Permissions: []string{docResource + ":write"},
			Extends: []string{viewerRole},
		}),
		tenantStep(r, "tenant", v1models.TenantCreate{
			Key: firstTenant, Name: "Tenant – 租户", Description: new("Line one\nline two\ttab"),
			Attributes: map[string]any{
				"plan": "gold", "seats": json.Number("9007199254740993"), "ratio": 0.1,
				"flags": []any{true, false, nil}, "unicode": "Zoë ✓ \u2028 😀", "a/b~c": "/~",
				"nested": map[string]any{
					"empty": map[string]any{}, "list": []any{},
					"deep": map[string]any{"a": map[string]any{"b": []any{map[string]any{}}}},
				},
			},
		}),
		tenantStep(r, "tenant_with_little", v1models.TenantCreate{
			Key: secondTenant, Name: "Second",
		}),
		userStep(r, "user", func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.Users.Create(ctx, v1models.UserCreate{
				Key: firstUser, Email: new("upgrade.user+1@example.com"), FirstName: new("Zoë"),
				LastName: new("O'Brien-Ünal 李"),
				Attributes: map[string]any{
					"age": 42, "score": 99.5, "teams": []any{"a", "b"}, "verified": true,
					"address": map[string]any{"city": "Zürich", "zip": "8000"}, "nothing": nil,
				},
			})
			return err
		}, firstUser),
		userStep(r, "user_with_a_key_alone", func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.Users.Create(ctx, v1models.UserCreate{Key: secondUser})
			return err
		}, secondUser),
		userStep(r, "user_that_sync_creates", func(ctx context.Context, c *v1.Client) error {
			_, err := c.SyncUser(ctx, v1models.UserCreate{
				Key: thirdUser, Email: new("upgrade.user.3@example.com"), FirstName: new("Ada"),
			})
			return err
		}, thirdUser),
		instanceStep(r, "resource_instance", v1models.ResourceInstanceCreate{
			Key: firstDoc.key, Resource: firstDoc.resource, Tenant: new(firstTenant),
			Attributes: map[string]any{
				"owner": firstUser, "size": 1024, "public": true, "labels": []any{"x", "y"},
				"created": "2026-10-16T10:00:00Z",
				"meta":    map[string]any{"k": map[string]any{"v": []any{1, 2.5, "3"}}},
			},
		}),
		instanceStep(r, "resource_instance_of_another_resource", v1models.ResourceInstanceCreate{
			Key: firstFolder.key, Resource: firstFolder.resource, Tenant: new(firstTenant),
		}),
		instanceStep(r, "resource_instance_in_another_tenant", v1models.ResourceInstanceCreate{
			Key: secondDoc.key, Resource: secondDoc.resource, Tenant: new(secondTenant),
			Attributes: map[string]any{"public": false},
		}),
		instanceStep(r, "resource_instance_in_another_tenant_of_another_resource",
			v1models.ResourceInstanceCreate{
				Key: secondFolder.key, Resource: secondFolder.resource, Tenant: new(secondTenant),
			}),
		tupleStep(b, "relationship_tuple", v1models.RelationshipTupleCreate{
			Subject: firstFolder.String(), Relation: parentRelation, Object: firstDoc.String(),
			Tenant: new(firstTenant),
		}),
		tupleStep(b, "relationship_tuple_in_the_tenant_of_its_object",
			v1models.RelationshipTupleCreate{
				Subject: secondFolder.String(), Relation: parentRelation,
				Object: secondDoc.String(),
			}),
		roleAssignmentsStep(b, []v1models.RoleAssignmentCreate{
			{User: firstUser, Role: viewerRole, Tenant: firstTenant},
			{User: secondUser, Role: adminRole, Tenant: secondTenant},
			{
				User: firstUser, Role: editorRole, Tenant: firstTenant,
				ResourceInstance: new(firstDoc.String()),
			},
		}),
		conditionSetStep(r, "user_set", func() v1models.ConditionSetCreate {
			return v1models.ConditionSetCreate{
				Key: userSet, Type: new(v1models.USERSET), Name: "Example users 📧",
				Description: new("Users whose email is at example.com"),
				Conditions: map[string]any{"allOf": []any{map[string]any{"allOf": []any{
					map[string]any{"user.email": map[string]any{"contains": "@example.com"}},
				}}}},
			}
		}),
		conditionSetStep(r, "resource_set", func() v1models.ConditionSetCreate {
			return v1models.ConditionSetCreate{
				Key: resourceSet, Type: new(v1models.RESOURCESET), Name: "Public documents",
				ResourceId: &v1models.ResourceId{String: new(r.ids[docResource])},
				Conditions: map[string]any{"allOf": []any{map[string]any{"allOf": []any{
					map[string]any{"resource.public": map[string]any{"equals": true}},
					map[string]any{"resource.size": map[string]any{
						"less-than": json.Number("9007199254740993"),
					}},
				}}}},
			}
		}),
		conditionSetRuleStep(r),
		proxyConfigStep(r, "proxy_config_with_bearer_auth", v1models.ProxyConfigCreate{
			Key: bearerProxy, Name: "Proxy ✓ – Bearer", Secret: bearerSecret,
			AuthMechanism: new(v1models.BEARER),
			MappingRules: []v1models.MappingRule{
				{
					Url: "https://api.example.com/v1/docs/[^/]+?q=a:b", HttpMethod: v1models.GET,
					Resource: docResource, Action: new("read"), Priority: new(int32(7)),
					UrlType: new(v1models.URLMatchTypeRegex),
					Headers: &map[string]string{"X-Trace-Id": "upgrade ✓", "Accept": "*/*"},
				},
				{
					Url: "https://api.example.com/v1/docs", HttpMethod: v1models.OPTIONS,
					Resource: docResource, Priority: new(int32(0)),
				},
				{
					Url: "https://api.example.com/v1/docs", HttpMethod: v1models.HEAD,
					Resource: docResource, Action: new("read"),
				},
				{
					Url: "https://api.example.com/v1/docs/1", HttpMethod: v1models.PUT,
					Resource: docResource, Action: new("write"),
				},
				{
					Url: "https://api.example.com/v1/docs/1", HttpMethod: v1models.PATCH,
					Resource: docResource, Action: new("write"),
				},
				{
					Url: "https://api.example.com/v1/docs/1", HttpMethod: v1models.DELETE,
					Resource: docResource, Action: new(archiveAction),
				},
			},
		}),
		proxyConfigStep(r, "proxy_config_with_basic_auth", v1models.ProxyConfigCreate{
			Key: basicProxy, Name: "Proxy – Basic", Secret: basicSecret,
			AuthMechanism: new(v1models.BASIC),
			MappingRules: []v1models.MappingRule{{
				Url: "https://api.example.com/v1/folders", HttpMethod: v1models.POST,
				Resource: folderResource, Action: new("list"),
			}},
		}),
	}
}

// docPath returns the path of rest under the resource upgrade-doc, such as that of one of its
// actions for /actions/<key>.
func docPath(r *run, rest string) string {
	return r.schema("/resources/" + docResource + rest)
}

// resourceStep returns the step that writes the resource upgrade-doc, with an action of each
// shape, an attribute of each type that v1 knows, and its ID in r.ids, which the resource set
// names it by.
func resourceStep(r *run) step {
	return step{
		name: "resource",
		register: func(*testing.T) {
			r.register("resource "+docResource, docPath(r, ""))
		},
		write: func(ctx context.Context, c *v1.Client) error {
			attributes := map[string]v1models.AttributeBlockEditable{
				"owner":   {Type: v1models.STRING, Description: new("Who owns it – 所有者")},
				"size":    {Type: v1models.NUMBER},
				"public":  {Type: v1models.BOOL},
				"created": {Type: v1models.TIME},
				"labels":  {Type: v1models.ARRAY},
				"meta":    {Type: v1models.JSON},
			}
			created, err := c.Api.Resources.Create(ctx, v1models.ResourceCreate{
				Key: docResource, Name: "Document – 文書 📄", Urn: new("prn:upgrade:doc/v1"),
				Description: new("A quote \", a backslash \\, a tab\tand a newline\n"),
				Actions: map[string]v1models.ActionBlockEditable{
					"read": {
						Name: new("Read ✓"), Description: new("Reads <it> & more"),
						Attributes: map[string]any{
							"risk": 1, "tags": []any{"a", "b"},
							"nested": map[string]any{"empty": map[string]any{}, "list": []any{}},
						},
					},
					"write":         {},
					"share-link_v2": {Name: new("Share / link")},
				},
				Attributes: &attributes,
			})
			if err == nil {
				r.ids[docResource] = created.Id
			}
			return err
		},
		reads: []read{{
			name: "the resource " + docResource,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				return view(c.Resources.Get(ctx, docResource))
			},
		}},
	}
}

// roleStep returns the step name, which writes the role of create.
func roleStep(r *run, name string, create v1models.RoleCreate) step {
	return step{
		name: name,
		register: func(*testing.T) {
			r.register("role "+create.Key, r.schema("/roles/"+create.Key))
		},
		write: func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.Roles.Create(ctx, create)
			return err
		},
		reads: []read{{
			name: "the role " + create.Key,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				return view(c.Roles.Get(ctx, create.Key))
			},
			unordered: []string{"#/permissions", "#/extends"},
		}},
	}
}

// resourceRoleStep returns the step name, which writes the role of create of the resource
// resource. The API's documentation does not say in which order it returns the permissions of a
// role and the roles that it extends.
func resourceRoleStep(r *run, name, resource string, create v1models.ResourceRoleCreate) step {
	return step{
		name: name,
		register: func(*testing.T) {
			r.register("resource role "+create.Key+" of "+resource,
				r.schema("/resources/"+resource+"/roles/"+create.Key))
		},
		write: func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.ResourceRoles.Create(ctx, resource, create)
			return err
		},
		reads: []read{{
			name: "the resource role " + create.Key + " of " + resource,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				return view(c.ResourceRoles.Get(ctx, resource, create.Key))
			},
			unordered: []string{"#/permissions", "#/extends"},
		}},
	}
}

// grantRule is the rule of an implicit grant, as the body of its removal names it.
type grantRule struct {
	Role             string `json:"role"`
	OnResource       string `json:"on_resource"`
	LinkedByRelation string `json:"linked_by_relation"`
}

// implicitGrantStep returns the step that writes an implicit grant of the role upgrade-editor of
// upgrade-doc: a user with the role upgrade-folder-editor on the folder that is the parent of a
// document gets upgrade-editor on the document, unless the document has a direct role, as
// no_direct_roles_on_object asks. The API keeps the rules of a role's implicit grants in the
// role's granted_to, where v2 reads it. The ledger's check that it is gone reads the role.
func implicitGrantStep(r *run) step {
	role := docPath(r, "/roles/"+editorRole)
	rule := grantRule{
		Role: folderEditorRole, OnResource: folderResource, LinkedByRelation: parentRelation,
	}
	what := "the implicit grant of " + editorRole + " to the " + folderEditorRole + " of the " +
		parentRelation + " " + folderResource
	return step{
		name: "implicit_grant",
		register: func(*testing.T) {
			r.ledger.Register(what, r.api.RemoverWithBody(role+"/implicit_grants", rule),
				r.api.Unlisted(role, framework.AbsenceTimeout, func(body []byte) (bool, error) {
					var stored struct {
						GrantedTo struct {
							UsersWithRole []grantRule `json:"users_with_role"`
						} `json:"granted_to"`
					}
					if err := json.Unmarshal(body, &stored); err != nil {
						return false, fmt.Errorf("decoding the role: %w", err)
					}
					return slices.Contains(stored.GrantedTo.UsersWithRole, rule), nil
				}))
		},
		write: func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.ImplicitGrants.Create(ctx, docResource, editorRole,
				v1models.DerivedRoleRuleCreate{
					Role: rule.Role, OnResource: rule.OnResource,
					LinkedByRelation: rule.LinkedByRelation,
					When: &v1models.PermitBackendSchemasSchemaDerivedRoleRuleDerivationSettings{
						NoDirectRolesOnObject: new(true),
					},
				})
			return err
		},
		reads: []read{{
			name: what,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				got, err := c.ResourceRoles.Get(ctx, docResource, editorRole)
				switch {
				case err != nil:
					return nil, err
				case got.GrantedTo == nil || len(got.GrantedTo.UsersWithRole) != 1:
					return nil, fmt.Errorf("the role %s of %s has the implicit grants %+v, "+
						"want 1", editorRole, docResource, got.GrantedTo)
				}
				return upgrade.View(got.GrantedTo.UsersWithRole[0])
			},
		}},
	}
}

// tenantStep returns the step name, which writes the tenant of create.
func tenantStep(r *run, name string, create v1models.TenantCreate) step {
	return step{
		name: name,
		register: func(*testing.T) {
			r.register("tenant "+create.Key, r.facts("/tenants/"+create.Key))
		},
		write: func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.Tenants.Create(ctx, create)
			return err
		},
		reads: []read{{
			name: "the tenant " + create.Key,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				return view(c.Tenants.Get(ctx, create.Key))
			},
		}},
	}
}

// userStep returns the step name, whose write writes the user key.
func userStep(r *run, name string, write func(context.Context, *v1.Client) error,
	key string,
) step {
	return step{
		name: name,
		register: func(*testing.T) {
			r.register("user "+key, r.facts("/users/"+url.PathEscape(key)))
		},
		write: write,
		reads: []read{{
			name: "the user " + key,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				return view(c.Users.Get(ctx, key))
			},
		}},
	}
}

// instanceStep returns the step name, which writes the resource instance of create.
func instanceStep(r *run, name string, create v1models.ResourceInstanceCreate) step {
	ident := instance{create.Resource, create.Key}.String()
	return step{
		name: name,
		register: func(*testing.T) {
			r.register("resource instance "+ident,
				r.facts("/resource_instances/"+url.PathEscape(ident)))
		},
		write: func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.ResourceInstances.Create(ctx, create)
			return err
		},
		reads: []read{{
			name: "the resource instance " + ident,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				return view(c.ResourceInstances.Get(ctx, ident))
			},
		}},
	}
}

// tupleStep returns the step name, which writes the relationship tuple of create and registers
// it with b.
func tupleStep(b *seed.Builder, name string, create v1models.RelationshipTupleCreate) step {
	tenant := ""
	if create.Tenant != nil {
		tenant = *create.Tenant
	}
	what := "the relationship tuple " + create.Subject + " " + create.Relation + " " +
		create.Object
	return step{
		name: name,
		register: func(*testing.T) {
			b.RegisterTuple(seed.Tuple{
				Subject: create.Subject, Relation: create.Relation, Object: create.Object,
				Tenant: tenant,
			})
		},
		write: func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.RelationshipTuples.Create(ctx, create)
			return err
		},
		reads: []read{{
			name: what,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				page, err := c.RelationshipTuples.List(ctx, &permit.RelationshipTuplesListOptions{
					Subject: create.Subject, Relation: create.Relation, Object: create.Object,
					Tenant: tenant,
				})
				return only(what, page, err)
			},
		}},
	}
}

// roleAssignmentsStep returns the step that writes the role assignments of creates in one bulk
// request, the one route of v1 whose role assignments the fake of a dry run lists, and registers
// each with b.
func roleAssignmentsStep(b *seed.Builder, creates []v1models.RoleAssignmentCreate) step {
	s := step{
		name: "role_assignments",
		register: func(t *testing.T) {
			t.Helper()
			for _, a := range creates {
				err := b.RegisterRoleAssignment(seed.RoleAssignment{
					User: a.User, Role: a.Role, Tenant: a.Tenant,
					ResourceInstance: a.GetResourceInstance(),
				})
				if err != nil {
					t.Fatal(err)
				}
			}
		},
		write: func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.Roles.BulkAssignRole(ctx, creates)
			return err
		},
	}
	for _, a := range creates {
		what := "the role assignment of " + a.Role + " to " + a.User + " in " + a.Tenant
		if a.ResourceInstance != nil {
			what += " on " + *a.ResourceInstance
		}
		s.reads = append(s.reads, read{
			name: what,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				page, err := c.RoleAssignments.List(ctx, &permit.RoleAssignmentsListOptions{
					Users: []string{a.User}, Roles: []string{a.Role}, Tenants: []string{a.Tenant},
					ResourceInstance: a.GetResourceInstance(),
				})
				return only(what, page, err)
			},
		})
	}
	return s
}

// conditionSetStep returns the step name, which writes the condition set that create returns
// when the step runs, after the steps that write what it names.
func conditionSetStep(r *run, name string, create func() v1models.ConditionSetCreate) step {
	key := create().Key
	return step{
		name: name,
		register: func(*testing.T) {
			r.register("condition set "+key, r.schema("/condition_sets/"+key))
		},
		write: func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.ConditionSets.Create(ctx, create())
			return err
		},
		reads: []read{{
			name: "the condition set " + key,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				return view(c.ConditionSets.Get(ctx, key))
			},
		}},
	}
}

// setRule is a condition set rule, as the body of its removal names it.
type setRule struct {
	UserSet     string `json:"user_set"`
	Permission  string `json:"permission"`
	ResourceSet string `json:"resource_set"`
}

// conditionSetRuleStep returns the step that writes the condition set rule that grants the users
// of the user set the action read of upgrade-doc on the resource set. The ledger's check that it
// is gone lists the rules of both sets, and lists the rule when it lists any: what form of a
// permission the API lists, and a filter of it matches, is not measured yet.
func conditionSetRuleStep(r *run) step {
	rule := setRule{UserSet: userSet, Permission: docResource + ":read", ResourceSet: resourceSet}
	what := "the condition set rule " + rule.Permission + " of " + userSet + " on " + resourceSet
	return step{
		name: "condition_set_rule",
		register: func(*testing.T) {
			path := r.facts("/set_rules")
			query := url.Values{"user_set": {rule.UserSet}, "resource_set": {rule.ResourceSet}}
			r.ledger.Register(what, r.api.RemoverWithBody(path, rule),
				r.api.Unlisted(path+"?"+query.Encode(), framework.AbsenceTimeout,
					func(body []byte) (bool, error) {
						var listed []json.RawMessage
						if err := json.Unmarshal(body, &listed); err != nil {
							return false, fmt.Errorf("decoding the list: %w", err)
						}
						return len(listed) > 0, nil
					}))
		},
		write: func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.ConditionSets.AssignSetPermissions(ctx, rule.UserSet, rule.Permission,
				rule.ResourceSet)
			return err
		},
		reads: []read{{
			name: what,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				page, err := c.ConditionSetRules.List(ctx, &permit.ConditionSetRulesListOptions{
					UserSet: rule.UserSet, ResourceSet: rule.ResourceSet,
				})
				return only(what, page, err)
			},
			writeOnly: map[string]string{
				"is_role": "v1 always sends it, false by default; true would ask the API to use " +
					"the user set of a role, and the API stores the rule without it, as its " +
					"read in the spec has no such field",
				"is_resource": "v1 always sends it, false by default; true would ask the API to " +
					"use the resource set of a resource, and the API stores the rule without " +
					"it, as its read in the spec has no such field",
			},
		}},
	}
}

// proxyConfigStep returns the step name, which writes the proxy config of create. Its read puts
// the credential that v2 read, with Secret.Value, in place of the marker that a Secret of v2
// encodes as, so that the comparison checks the credential too. The API's documentation does not
// say in which order it returns the mapping rules.
func proxyConfigStep(r *run, name string, create v1models.ProxyConfigCreate) step {
	return step{
		name: name,
		register: func(*testing.T) {
			r.register("proxy config "+create.Key, r.facts("/proxy_configs/"+create.Key))
		},
		write: func(ctx context.Context, c *v1.Client) error {
			_, err := c.Api.ProxyConfigs.Create(ctx, create)
			return err
		},
		reads: []read{{
			name: "the proxy config " + create.Key,
			get: func(ctx context.Context, c *permit.Client) (any, error) {
				got, err := c.ProxyConfigs.Get(ctx, create.Key)
				v, err := view(got, err)
				if err != nil {
					return nil, err
				}
				fields, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("the proxy config that v2 read is %v", v)
				}
				switch secret := got.Secret; {
				case secret.Bearer != nil:
					fields["secret"] = secret.Bearer.Value()
				case secret.Basic != nil:
					fields["secret"] = secret.Basic.Value()
				}
				return fields, nil
			},
			unordered: []string{"#/mapping_rules"},
		}},
	}
}
