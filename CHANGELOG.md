# Changelog

## v2.0.0 (unreleased)

v2 is the module `github.com/permitio/permit-golang/v2`. [MIGRATION.md](MIGRATION.md) lists each
change from v1 and what to use instead.

### Client

- `permit.New(opts ...permit.Option)` builds a `*permit.Client` from the options `WithAPIKey`,
  `WithAPIURL`, `WithPDPURL`, `WithHTTPClient`, `WithTimeout`, `WithRetry`, `WithLogger`,
  `WithScope` and `WithFactsViaPDP`, and returns an error when the configuration is not one the
  SDK can use.
- A setting that no option gives comes from the environment variables `PERMIT_API_KEY`,
  `PERMIT_API_URL` and `PERMIT_PDP_URL`, and else from its default: `https://api.permit.io` for
  the API, `http://localhost:7766` for the PDP and 30 seconds for the timeout. A variable that is
  set to the empty string counts as unset. There is no default API key.
- `New` sends no request. It returns an error that matches `ErrInvalidConfig`, and names the
  option or the variable, when there is no API key; when an option gives an empty key or URL, a
  nil `*http.Client`, a timeout of zero or less, or a `MaxRetries` or `MaxBackoff` of less than
  zero; when a URL does not use `http` or `https` (in any case), has no host, or has a user name
  or a password, a query or a fragment; and for the zero `Option`. A path after the host of a URL
  is kept, and one slash at its end is removed.
- `WithTimeout` sets the longest that a call may take, across all the requests it sends. When the
  context of a call has a deadline, the deadline bounds the call instead. A call that runs out of
  time returns an error that matches `context.DeadlineExceeded`.
- Each client has an `*http.Client` of its own, whose transport is a copy of
  `http.DefaultTransport`: it uses the proxy that the environment names and the default TLS
  settings, and keeps up to 64 idle connections to each host. `WithHTTPClient` sends the requests
  through the `*http.Client` given, and changes none of its fields.
- `Client.Close` closes the idle connections of the client's own `*http.Client`, and not those of
  one that `WithHTTPClient` gave. It returns nil, also when called again, and a call that starts
  after it fails with an error that matches `ErrClosed` and sends no request. A call in flight
  when `Close` is called runs on and returns its result or its error; it sends no other attempt,
  and a retry it was waiting for returns the error of its last attempt. A client that is garbage
  collected without `Close` has the idle connections of its own `*http.Client` closed too.
- A `Client` and an `Option` print with `[REDACTED]` in place of the API key: `fmt` with every
  verb, `String`, `GoString` and the `LogValue` that `log/slog` calls.
- `WithScope(project, environment)` sets the project and the environment that the client's calls
  act on, by key or by ID, and the client then sends no request for the scope of its API key.
  `New` returns an error that matches `ErrInvalidConfig` for an environment without a project, an
  empty project, and a project or an environment that is `.` or `..`.
- `Client.Scope(ctx)` returns the `permit.Scope`, the project and the environment, that the
  client's calls act on: those that `WithScope` gave, or else those of the API key, which the
  client looks up once with `GET /v2/api-key/scope`. The calls that need the scope while the
  lookup runs wait for it, and the client keeps the scope of the first lookup that succeeds; a
  lookup that fails is not kept, so the next call looks it up again. A failed lookup returns the
  `*permit.Error` of the reply, such as one that matches `ErrUnauthorized`, or after the retries
  `ErrRateLimited`. A call whose context ends while it waits for the lookup returns an error that
  matches the context's error at once, and the lookup goes on for the other calls; `Close` ends
  the lookup.
- A call that acts on an environment needs a project and an environment in the scope, and one
  that acts on a project needs a project. When the scope lacks one, the call returns an error
  that matches `ErrInvalidConfig`, names its operation and `WithScope`, and sends no request for
  the operation; without `WithScope`, the client may first look up the scope of its API key. A
  call that acts on the organization needs no scope, and sends no lookup.

### Facts

- `WithFactsViaPDP()` sends the calls of the facts API, those of users, tenants and the users in
  them, role assignments, resource instances and relationship tuples, one at a time and in bulk,
  to the PDP container at the PDP URL under `/facts/...`, instead of to the Permit API under
  `/v2/facts/{proj_id}/{env_id}/...`. These calls fill in no project and no environment, as the
  container acts on the environment of its own API key, and send no request for the scope of the
  client's API key. The calls of set rules, user invites, proxy configs, the schema and projects
  go to the API with or without the option. v1's `WithProxyFactsViaPDP(true)` did not change
  where facts requests went: v1.2.8 and later sent every one to the API.
- A 404 with an empty body to a facts call through the PDP, which is how the cloud PDP answers
  every route under `/facts/`, is an error that matches `ErrContainerPDPOnly`, not `ErrNotFound`,
  and names the route under `/facts/`, also for a delete, which otherwise returns nil for a 404.
- The package documentation lists the 11 routes on which the PDP container can hold its answer
  until a write reaches its policy, and says that it answers every other facts call without
  waiting. A test checks the list against the container's spec.

### Decisions

- `Client.Check(ctx, user, action, resource, opts...)` asks the PDP for one decision with
  `POST /allowed`. `Client.BulkCheck(ctx, checks)` asks for the decisions of many checks, of any
  users, actions, resources and tenants, with one `POST /allowed/bulk` request, and returns them
  in the order of the checks. `Client.FilterObjects(ctx, user, action, resources, opts...)`
  returns the resources that the PDP allows, each as it was given, with one `POST /allowed/bulk`
  request. The PDP container and the cloud PDP both serve these routes. A call without checks or
  resources sends no request.
- `permit.User`, `permit.Resource` and `permit.CheckRequest` are built as struct literals. A
  resource has a `Key`, which makes the check one of that instance, and no ID, and each check
  sends the key, the tenant, the attributes and the context of its resource as given; a resource
  without a tenant is sent with the tenant `default`. `WithDecisionContext(map[string]any)` gives
  the checks of a call their context, with values of any JSON type. A nil or empty map of
  attributes or of context is left out of the body, never sent as `null`.
- A call returns a decision only when the PDP's reply holds one: a reply in 200-299 that is not a
  JSON object with one key `allow` whose value is a boolean, such as `{}`, `{"allow": null}` or
  `{"Allow": true}`, or that does not hold such a decision for each check, is an error that
  matches `ErrMalformedDecision`. A reply with an error status, such as 400, 421, 422 or 429
  after the retries, is a `*permit.Error` with the status and the body. A user without a key, a
  resource without a type and the zero `CheckOption` are errors that match `ErrInvalidConfig`,
  and send no request.
- A decision changes nothing, so a call sends its request again after a 500, 502, 503 or 504
  reply or a failed connection, as after a 429.
- Decisions go to the PDP only: v1's `WithOpaUrl`, which sent them to OPA, has no counterpart.
- `Client.UserPermissions(ctx, user, opts...)` takes the place of v1's `GetUserPermissions` and
  `GetUserPermissionsWithOptions`, with one `POST /user-permissions` request. It returns a
  `map[string]permit.ObjectPermissions` under the keys that the PDP gives: `__tenant:<tenant>`
  for the roles and the permissions of the user in a tenant, and `<type>:<key>` for those on a
  resource instance. `WithTenants`, `WithResources` and `WithResourceTypes` take their keys as
  arguments and return a `PermissionsOption`, which only `UserPermissions` takes, and
  `WithDecisionContext` gives the call its context: v1's `GetUserPermissions` sent no context,
  resources or resource types, and `GetUserPermissionsWithOptions` sent the same fields as v2. A
  filter without keys is an error that matches `ErrInvalidConfig`, where v1 left an empty list out
  of the body and so asked about every tenant, resource or resource type.
- `Client.AuthorizedUsers(ctx, action, resource, opts...)` is new: it returns the users that the
  PDP allows to do an action on a resource instance or a resource type, each with the role
  assignments that allow it, with one `POST /authorized_users` request, and takes its context
  from `WithDecisionContext`. A resource without a tenant is sent with the tenant `default`, as
  in a check.
- The PDP container and the cloud PDP both serve these two routes. A reply in 200-299 that is not
  a list of the expected shape, such as `null`, the permissions inside another object, or a reply
  of authorized users without `users`, is an error that matches `ErrMalformedDecision`, never an
  empty result; v1 returned a nil map and no error for `null`. Both calls change nothing, so they
  are sent again after a 500, 502, 503 or 504 reply or a failed connection, as a check is.
- `Client.AllTenantsCheck(ctx, user, action, resource, opts...)` takes the place of v1's
  `AllTenantsCheck`, with one `POST /allowed/all-tenants` request: it returns the tenants in which
  the PDP allows the check, as a `[]permit.TenantDetails`, and takes its context from
  `WithDecisionContext`; v1's `permit.Client.AllTenantsCheck` sent no context. It sends the check
  as `Check` does. A later version of the PDP may drop the route: `UserPermissions` returns the
  permissions of a user in each tenant.
- `Client.UserTenants(ctx, user, opts...)` is new: it returns the tenants of a user as a
  `[]permit.TenantDetails`, with one `POST /user-tenants` request, and takes its context from
  `WithDecisionContext`. The PDP lists the tenants in which the user has a role, so a user who is a
  member of a tenant without a role in it is not listed.
- `Client.CheckURL(ctx, user, method, url, tenant, opts...)` is new: it asks the PDP, with one
  `POST /allowed_url` request, whether a user may send a request with an HTTP method to a URL in a
  tenant, by the mapping rules that map URLs to resources and actions, and returns the decision
  as a `bool`. It takes its context from `WithDecisionContext`. An empty tenant is sent as
  `default`, and an empty URL is an error that matches `ErrInvalidConfig`, with no request.
- Only the PDP container serves these three routes. The cloud PDP answers each with 404 and an
  empty body, which the call returns, after that one request, as an error that matches
  `ErrContainerPDPOnly`, not `ErrNotFound`, and names the route; a 404 with a body stays an error
  that matches `ErrNotFound`. A reply in 200-299 that is not of the expected shape, such as a
  decision of a tenant whose `allow` is not a boolean, a list of tenants that is `null`, or a
  decision of a URL without `allow`, is an error that matches `ErrMalformedDecision`. The three
  calls change nothing, so they are sent again after a 500, 502, 503 or 504 reply or a failed
  connection, as a check is.
- `Client.LocalRoleAssignments(ctx, opts)` is new: it returns one page of the role assignments
  that the PDP container holds, as a `*permit.Page[permit.LocalRoleAssignment]`, with one
  `GET /local/role_assignments` request, and `Client.LocalRoleAssignmentsAll(ctx, opts)` reads
  every page. `permit.LocalRoleAssignmentsOptions` embeds `ListOptions` and filters by `User`,
  `Role`, `Tenant`, `Resource` (a resource type) and `ResourceInstance`; an empty filter is not
  sent. The PDP gives 30 role assignments on a page unless `PerPage` is set, and at most 100,
  which `LocalRoleAssignmentsAll` asks for. A role in a tenant has a nil `ResourceInstance`. Only
  the PDP container serves the route: the cloud PDP's 404 with an empty body is an error that
  matches `ErrContainerPDPOnly` and names the route. A reply in 200-299 that is not a JSON array,
  or that holds an item with a field of another type, is an error, never an empty page; an item
  that leaves a field out decodes with it empty. The list changes nothing, so it is sent again
  after a 500, 502, 503 or 504 reply or a failed connection, as a check is.

### Projects

- The `Projects` field of the client, a `*permit.ProjectsService`, lists, reads, creates,
  changes and deletes the projects of the organization of the API key: `List` returns a page
  as a `*permit.Page[models.ProjectRead]` and `All` reads every page, 100 projects at a time;
  `Get` reads one project by its key or its ID, in place of v1's `Get`, `GetById` and `GetByKey`;
  `Create` takes a `models.ProjectCreate`, `Update` a `models.ProjectUpdate`, and both return the
  project as a `*models.ProjectRead`; `Delete` deletes a project with its environments.
- The calls need no scope: they send no request for the scope of the API key, and read none of
  `WithScope`, so they work with an organization-level API key, whose scope has no project and no
  environment. `List`, `All` and `Create` need an organization-level key; `Get`, `Update` and
  `Delete` need a project-level key of the project, or higher.
- `models.ProjectCreate.InitialEnvironments` is new: nil lets the API create its default
  environments, Development and Production, and an empty slice that is not nil sends an empty
  list.
  Each environment is a `models.EnvironmentCreate`, with its settings and the configuration of
  its JSON Web Key Sets, `models.JwksConfig`.
- `models.ProjectUpdate.Description` is a `models.Optional[string]`, so an update can send null
  for the description with `models.Null[string]()`; the other fields of an update are left out
  when they are their zero value, such as an empty name or nil settings, and the project keeps
  them.
- A key of a project is sent as one segment of the path, escaped, so a key that holds a `/` or a
  `%` names that project; an empty key, `.` or `..` is an error that matches `ErrInvalidConfig`,
  and no request is sent. `Delete` returns nil when the API answers 404, as the project is gone
  already. `Create` is sent again after a 429 only; the other calls are sent again after a 500,
  502, 503 or 504 reply or a failed connection too.

### Environments

- The `Environments` field of the client, a `*permit.EnvironmentsService`, lists, reads, creates,
  changes, copies and deletes the environments of a project and reads their statistics; each method
  takes the key or the ID of the project after the context, in place of the project of v1's context:
  `List` returns a page as a `*permit.Page[models.EnvironmentReadWithEmailConfig]` and `All` reads
  every page, 100 environments at a time; `Get` reads one environment by its key or its ID, in place
  of v1's `Get`, `GetById` and `GetByKey`, as a `*models.EnvironmentReadWithEmailConfig`; `Create`
  takes a `models.EnvironmentCreate`, `Update` a `models.EnvironmentUpdate`, and both return the
  environment as a `*models.EnvironmentRead`; `Delete` deletes an environment with its data.
- The calls need no scope: they send no request for the scope of the API key, and read none of
  `WithScope`. Each is sent with a project-level API key of the project, or an
  organization-level key.
- `models.EnvironmentRead` has the fields of the reply that v1 dropped: `Settings`, `JWKS`,
  `JWKSUnpinned` and `AVPPolicyStoreID`; `models.EnvironmentReadWithEmailConfig` embeds it and
  adds `EmailConfiguration`, the ID of the environment's email configuration.
- A create and an update send the settings of an environment and the configuration of its JSON
  Web Key Sets, which v1 could not send. In `models.EnvironmentUpdate`, `Description`,
  `CustomBranchName`, `JWKS` and `Settings` are `models.Optional` fields: unset leaves the field
  out, `models.Null` sends null and `models.Some` sends the value; `Name` is left out when empty.
- `models.JwksConfig.Audience` and `Issuer` are a `models.StringOrList` (see Models). A reply
  whose audience or issuer is neither a string nor a list of strings is an error that names the
  model.
- The keys of a project and of an environment are each sent as one segment of the path, escaped;
  an empty key, `.` or `..` is an error that matches `ErrInvalidConfig`, and no request is sent.
  `Delete` returns nil when the API answers 404, as the environment is gone already. `Create`,
  `Copy` and `CopyAsync` are sent again after a 429 only; the other calls are sent again after a
  500, 502, 503 or 504 reply or a failed connection too.
- `Stats` returns an environment with its statistics and the configurations of its PDPs, as a
  `*models.EnvironmentStats`, which v1 read only through its generated client. The client secret
  of a PDP, `models.PDPConfigRead.ClientSecret`, and its `OPALServerAccessToken` are each a
  `models.Secret` (see API keys), and the client replaces them in its later errors and log lines.
  `models.EnvironmentStatistics` leaves out the members of the organization that the reply lists.
- `Copy` copies an environment into a new or an existing one, as a `models.EnvironmentCopy` says,
  and returns the environment that it copied into; v1 copied only through its generated client.
  `CopyAsync` submits the same copy to run in the background and returns its task, a
  `*models.TaskResultEnvironmentRead`, after one request: it does not poll the task. `CopyResult`
  reads the task by its ID, once, with an optional wait, `CopyResultOptions.Wait`, sent in seconds;
  a `Wait` less than zero is an error that matches `ErrInvalidConfig`, with the rule
  `copy-result.wait.negative`. A copy needs a project-level API key, or an organization-level one.
- `models.EnvironmentCopyConflictStrategy` and `models.TaskStatus` are open enums, and the error
  that ended a task is a `models.ErrorDetails`, whose code is the open enum `models.ErrorCode`.

### API keys

- The `APIKeys` field of the client, a `*permit.APIKeysService`, reads the API key of an
  environment and manages the API keys of the organization: `GetEnvironmentKey` reads the key
  of an environment by the key or the ID of its project and its own; `List` returns a page as a
  `*permit.Page[models.APIKeyRead]`, with the counts of the reply, and `All` reads every page,
  100 keys at a time, both filtered by `permit.APIKeysListOptions`, by the level of the key and
  by its project; `Get` reads one key by its ID; `Create` takes a `models.APIKeyCreate`;
  `Delete` deletes a key; and `RotateSecret` is new: it gives a key a new secret.
- `GetEnvironmentKey` needs a project-level API key; the other calls need an organization-level
  key. None needs a scope: they send no request for the scope of the API key, and read none of
  `WithScope`.
- The secret of a key is a `models.Secret`, which prints `[REDACTED]` in place of the credential
  with `fmt` and every verb, `String`, `GoString`, the `LogValue` that `log/slog` calls and
  `MarshalJSON`, so a printed, logged or encoded `models.APIKeyRead` holds no credential; its
  `Value` method returns the credential, `Equal` compares two credentials in a time that does not
  depend on their content, and `models.NewSecret` makes one. Its `UnmarshalJSON` reads a JSON string, and null as the empty
  `models.Secret`; any other value is an error that does not hold the value. The client adds each
  secret that its calls return to the credentials that it keeps out of its errors and its log
  lines, as it does its own API key.
- `RotateSecret` is sent once: never again. After a 500, 502, 503 or 504 reply or a failed
  connection, the API may have rotated the secret, and a second rotation would replace the secret
  that the first returned; after a 429, which the API did not act on, the call also returns the
  error, and leaves the retry to the caller. `Create` is sent again after a 429 only; the other
  calls are sent again after a 500, 502, 503 or 504 reply or a failed connection too. `Delete`
  returns nil when the API answers 404, as the key is gone already. An ID or a key is sent as one
  segment of the path, escaped; an empty one, `.` or `..` is an error that matches
  `ErrInvalidConfig`, and no request is sent. So is the ID `scope` for `Get`, `Delete` and
  `RotateSecret`, as it names the route of the scope of the API key, not a key.
- `models.APIKeyOwnerType`, `models.MemberAccessObj` and `models.MemberAccessLevel` are open enums;
  `models.APIKeyOwnerType` knows `nats_pdp_config`. `models.APIKeyRead.Env` is the environment of
  the key, a `*models.EnvironmentRead`; `models.APIKeyRead` leaves out the member who created the
  key.

### Resources

- The `Resources` field of the client, a `*permit.ResourcesService`, lists, reads, creates,
  replaces, changes and deletes the resources of the environment of the client's scope: `List`
  returns a page as a `*permit.Page[models.ResourceRead]` and `All` reads every page, 100
  resources at a time; `Get` reads one resource by its key or its ID, in place of v1's `Get`,
  `GetById` and `GetByKey`; `Create` takes a `models.ResourceCreate`, `Replace` a
  `models.ResourceReplace` and `Update` a `models.ResourceUpdate`, and each returns the resource
  as a `*models.ResourceRead`; `Delete` deletes a resource with its data.
- `Replace` is new: it sends the whole definition of a resource with PUT, whose actions are the
  whole set of actions of the resource. The API's documentation says that it creates the
  resource when the environment has none with the key, and that the actions that the definition
  leaves out or renames lose their role and permission assignments.
- v1's `Search` is the `Search` of `permit.ResourcesListOptions`, which `List` and `All` send in
  the query parameter `search`.
- The calls act on the environment of the client's scope, from `WithScope` or from the scope of
  the API key, which the client looks up once; a scope without an environment is an error that
  matches `ErrInvalidConfig`, and no request for the call is sent. Each call is sent with an
  environment-level API key, or with a key of a wider level and `WithScope`.
- `models.ResourceRead` has the fields of the reply that v1 dropped: `TypeAttributes`, `Roles`,
  each a `models.ResourceRoleRead`, `Relations`, each a `models.RelationBlockRead`, and
  `ActionGroups`. A create, a replace and an update can send `TypeAttributes`, `Roles`, each a
  `models.RoleBlockEditable`, and `Relations`, which v1 could not send. The blocks of a resource,
  such as `models.ActionBlockRead` and `models.ActionBlockEditable`, and the derived roles of a
  role, `models.DerivedRoleBlockRead`, `models.DerivedRoleBlockEdit`, their rules and
  `models.DerivationSettings`, are new types.
- In `models.ResourceUpdate`, `Description` is a `models.Optional[string]`, so an update can send
  null for it; each map, such as `Actions`, is sent as it is given, as a whole: nil leaves it
  out, an empty map that is not nil sends `{}`, and the SDK never sends null for a map. A create
  and a replace always send their actions, `{}` for an empty map and null for a nil one.
- A key of a resource is sent as one segment of the path, escaped; an empty key, `.` or `..` is
  an error that matches `ErrInvalidConfig`, and no request for the call is sent. `Delete`
  returns nil when the API answers 404, as the resource is gone already. `Create` is sent again
  after a 429 only; the other calls are sent again after a 500, 502, 503 or 504 reply or a
  failed connection too.

### Tenants

- The `Tenants` field of the client, a `*permit.TenantsService`, lists, reads, creates, changes
  and deletes the tenants of an environment, and lists, adds and removes the users in a tenant:
  `List` returns a page as a `*permit.Page[models.TenantRead]`, with the counts of a paginated
  reply, and `All` reads every page, 100 tenants at a time; `Get` reads one tenant by its key or
  its ID, in place of v1's `Get`, `GetById` and `GetByKey`; `Create` takes a
  `models.TenantCreate`, `Update` a `models.TenantUpdate`, and both return the tenant as a
  `*models.TenantRead`; `Delete` deletes a tenant. A reply with an error status to `List` is an
  error, also a 500; before v1.3.0, v1's `List` and `ListByAttributes` returned no error for a
  failed request, so a 500 looked like an empty list.
- `ListUsers` returns a page of the users in a tenant as a `*permit.Page[models.UserRead]`, with
  the counts of the reply, in place of v1's `ListTenantUsers`, which returned the reply's data
  alone, and `AllUsers` reads every page, 100 users at a time; `permit.TenantUsersListOptions`
  filters the list with `Search` and `Role`. `AddUser` sends a `models.UserCreate` to
  `POST /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users`, which v1 could not send, and
  returns the user as a `*models.UserRead`: the API's documentation describes it as the create of
  a user and its add to the tenant, without a role when the user has no `RoleAssignments`.
  `RemoveUser` removes a user from a tenant, which only v1's generated client could do, with
  `DeleteTenantUser`.
- The calls are calls of the facts API: they act on the environment of the client's scope, or,
  with `WithFactsViaPDP`, go to the PDP container under `/facts/tenants`. Each is sent with an
  environment-level API key, or higher.
- `permit.TenantsListOptions` filters a list by the attributes of the tenants, in place of v1's
  `ListByAttributes`: each `permit.AttributeFilter`, which `permit.AttributeEquals(name, value)`
  builds from a string, a bool, an int, an int64 or a float64, is sent as the query parameter
  `attr_<name>`, which the API's spec does not list. A filter without a name, a float64 that is
  NaN or an infinity, and a filter whose name an earlier filter has are errors that match
  `ErrInvalidConfig`, with the rules `list.attribute-filter.name-empty`,
  `list.attribute-filter.not-finite` and `list.attribute-filter.repeated`, and no request is
  sent.
- `models.TenantUpdate.Description` and `Attributes` are `models.Optional` fields: unset leaves
  the field out, `models.Null` sends null and `models.Some` sends the value; attributes replace the
  tenant's attributes as a whole, and an empty map sends `{}`, which removes them all. The
  `Attributes` of a `models.TenantRead` are nil when the reply holds null, as after an update
  that sent null, and an empty map that is not nil for `{}`.
- `TenantsService.WaitForSync(timeout, opts...)` returns a copy of the service whose creates the
  PDP container holds until the new tenant reaches its policy, for at most `timeout`, sent in
  `X-Wait-Timeout` as decimal seconds, such as `0.5`; `permit.WithSyncPolicy` sends
  `permit.SyncPolicyFail` or `permit.SyncPolicyIgnore` in `X-Timeout-Policy`. The copy shares no
  state with the service it came from. Only `Create` waits: any other call through the copy, a
  call from a client without `WithFactsViaPDP`, a timeout less than zero and a `permit.SyncOption`
  that is not valid, in any place among the options, are errors that match `ErrInvalidConfig`,
  the last with the rule `wait-for-sync.policy.unknown`, and no request is sent.
- A key of a tenant, or of a user in it, is sent as one segment of the path, escaped; an empty
  key, `.` or `..` is an error that matches `ErrInvalidConfig`, and no request for the tenant is
  sent. `Delete` returns nil when the API answers 404, as the tenant is gone already, and
  `RemoveUser` when the user is not in the tenant. `Create` and `AddUser` are sent again after a
  429 only; the other calls are sent again after a 500, 502, 503 or 504 reply or a failed
  connection too.

### Resource actions

- The `ResourceActions` field of the client, a `*permit.ResourceActionsService`, lists, reads,
  creates, changes and deletes the actions of a resource of the environment of the client's
  scope. Each method takes the key or the ID of the resource first: `List` returns a page as a
  `*permit.Page[models.ResourceActionRead]` and `All` reads every page, 100 actions at a time;
  `Get` reads one action by its key or its ID, in place of v1's `Get`, `GetById` and
  `GetByKey`; `Create` takes a `models.ResourceActionCreate` and `Update` a
  `models.ResourceActionUpdate`, and each returns the action as a `*models.ResourceActionRead`;
  `Delete` deletes an action.
- v1's `ListByAttributes` is the `Attributes` of `permit.ResourceActionsListOptions`, which `List`
  and `All` send in the query parameters `attr_<name>`. Each is a `permit.AttributeFilter`, which
  `permit.AttributeEquals(name, value)` builds, as for the tenants. A filter without a name, a
  float64 that is NaN or an infinity, and a filter whose name an earlier filter has are errors
  that match `ErrInvalidConfig`, and no request is sent.
- Every call returns the SDK's errors, a `*permit.Error` for a reply with an error status, where
  v1's `List`, `Get`, `Create`, `Update` and `Delete` returned the error of the generated client.
- In `models.ResourceActionUpdate`, `Description` is a `models.Optional[string]` and `Attributes`
  a `models.Optional[models.Attributes]`, so an update can send null for them, which clears them.
- The keys of the resource and of the action are sent as segments of the path, escaped; an empty
  key, `.` or `..` is an error that matches `ErrInvalidConfig`, and no request for the call is
  sent. `Delete` returns nil when the API answers 404, as the action is gone already. `Create` is
  sent again after a 429 only; the other calls are sent again after a 500, 502, 503 or 504 reply
  or a failed connection too.

### Resource action groups

- The `ResourceActionGroups` field of the client, a `*permit.ResourceActionGroupsService`, lists,
  reads, creates, changes and deletes the action groups of a resource of the environment of the
  client's scope; each method takes the key or the ID of the resource first. `List` returns a
  page as a `*permit.Page[models.ResourceActionGroupRead]` and `All` reads every page, 100 action
  groups at a time; `Get` reads one action group by its key or its ID, in place of v1's `Get`,
  `GetById` and `GetByKey`; `Create` takes a `models.ResourceActionGroupCreate` and `Update` a
  `models.ResourceActionGroupUpdate`, and both return the action group as a
  `*models.ResourceActionGroupRead`; `Delete` deletes an action group.
- `Delete` sends
  `DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups/{action_group_id}`,
  the route of the action group. Up to v1.2.8, v1's `Delete` sent the route of the action of the
  resource whose key was the group's key, so it deleted that action, or failed and left the group
  in place.
- `Update` is new: v1 had no update of an action group. In `models.ResourceActionGroupUpdate`,
  `Description` and `Actions` are `models.Optional` fields: unset leaves the field out,
  `models.Null` sends null and `models.Some` sends the value, `[]` for an empty slice; `Name` is
  left out when empty, and `Attributes` when nil. Set `Actions` in each update to the keys of the
  actions that the group keeps: an update that leaves them out may remove them all.
- The list sends no attribute filter: v1's `ListByAttributes` never sent its filter, and listed
  every action group of the resource. Filter the action groups that `List` or `All` return by
  their `Attributes` instead.
- The calls act on the environment of the client's scope, from `WithScope` or from the scope of
  the API key, which the client looks up once; a scope without an environment is an error that
  matches `ErrInvalidConfig`, and no request for the call is sent. Each call is sent with an
  environment-level API key, or with a key of a wider level and `WithScope`.
- A key of a resource or of an action group is sent as one segment of the path, escaped; an
  empty key, `.` or `..` is an error that matches `ErrInvalidConfig`, and no request for the call
  is sent. `Delete` returns nil when the API answers 404, as the action group is gone already.
  `Create` is sent again after a 429 only; the other calls are sent again after a 500, 502, 503 or
  504 reply or a failed connection too.

### Resource attributes

- The `ResourceAttributes` field of the client, a `*permit.ResourceAttributesService`, lists,
  reads, creates, changes and deletes the attributes of a resource of the environment of the
  client's scope. Each method takes the key or the ID of the resource first: `List` returns a page
  as a `*permit.Page[models.ResourceAttributeRead]` and `All` reads every page, 100 attributes at
  a time; `Get` reads one attribute by its key or its ID, in place of v1's `Get`, `GetById` and
  `GetByKey`; `Create` takes a `models.ResourceAttributeCreate` and `Update` a
  `models.ResourceAttributeUpdate`, and each returns the attribute as a
  `*models.ResourceAttributeRead`; `Delete` deletes an attribute.
- `List`, `All`, `Get`, `Create` and `Update` read an attribute of every type. v1's `List`, `Get`,
  `Create` and `Update` failed for an attribute of the type `object` or `object_array`, `Create`
  and `Update` after the API had created or changed it; the open enum `models.AttributeType` has
  constants for both, and keeps a type that the API adds later as it is, with `Known` reporting
  false for it.
- `models.ResourceAttributeRead` has `BuiltIn`, the `built_in` of the reply, which v1 dropped.
- Every call returns the SDK's errors, a `*permit.Error` for a reply with an error status, where
  v1's `List`, `Get`, `Create`, `Update` and `Delete` returned the error of the generated client.
- In `models.ResourceAttributeUpdate`, `Description` is a `models.Optional[string]`, so an update
  can send null for it, which clears it.
- The keys of the resource and of the attribute are sent as segments of the path, escaped; an
  empty key, `.` or `..` is an error that matches `ErrInvalidConfig`, and no request for the call
  is sent. `Delete` sends no query, and returns nil when the API answers 404, as the attribute is
  gone already. `Create` is sent again after a 429 only; the other calls are sent again after a
  500, 502, 503 or 504 reply or a failed connection too.

### Resource relations

- The `ResourceRelations` field of the client, a `*permit.ResourceRelationsService`, lists,
  reads, creates and deletes the relations of a resource of the environment of the client's
  scope; each method takes the key or the ID of the resource that has the relation first. `List`
  returns a page as a `*permit.Page[models.RelationRead]`, with the `TotalCount` and the
  `PageCount` of the API's reply, which v1's `List` dropped, and `All` reads every page, 100
  relations at a time; `Get` reads one relation by its key or its ID; `Create` takes a
  `models.RelationCreate` and returns the relation as a `*models.RelationRead`; `Delete` deletes a
  relation. The API has no update of a relation.
- `List` takes the resource first and a `*permit.ListOptions` next, where v1's `List` took the
  page and the number of relations before the resource.
- The calls act on the environment of the client's scope, from `WithScope` or from the scope of
  the API key, which the client looks up once; a scope without an environment is an error that
  matches `ErrInvalidConfig`, and no request for the call is sent. Each call is sent with an
  environment-level API key, or with a key of a wider level and `WithScope`.
- A key of a resource or of a relation is sent as one segment of the path, escaped; an empty
  key, `.` or `..` is an error that matches `ErrInvalidConfig`, and no request for the call is
  sent. `Delete` returns nil when the API answers 404, as the relation is gone already. `Create`
  is sent again after a 429 only; the other calls are sent again after a 500, 502, 503 or 504
  reply or a failed connection too.

### Roles

- The `Roles` field of the client, a `*permit.RolesService`, lists, reads, creates, changes and
  deletes the roles of the environment of the client's scope: `List` returns a page as a
  `*permit.Page[models.RoleRead]` and `All` reads every page, 100 roles at a time; `Get` reads one
  role by its key or its ID, in place of v1's `Get`, `GetById` and `GetByKey`; `Create` takes a
  `models.RoleCreate` and `Update` a `models.RoleUpdate`, and each returns the role as a
  `*models.RoleRead`; `Delete` deletes a role.
- The attributes of a role are a `models.Attributes`, whose values may be of any JSON type, in
  place of v1's `map[string]string`: v1 failed on a reply with a role whose attribute was a
  number, a boolean or an object.
- v1's `ListByAttributes` is the `Attributes` of `permit.RolesListOptions`, which `List` and `All`
  send in the query parameters `attr_<name>`, each a `permit.AttributeFilter`, as for the tenants.
- `AssignPermissions` and `RemovePermissions` return the role after the change, where v1 returned
  only an error; a nil slice of permissions sends `[]`.
- In `models.RoleUpdate`, `Permissions` and `Extends` are plain slices, each sent as a whole: nil
  leaves the list out, and the role keeps it; an empty slice sends `[]`, which clears it; and a
  list replaces it. `Description`, `Attributes` and `GrantedTo` are `models.Optional` fields, so
  an update can send null for them, which clears them; v1 could not send null for a description
  or attributes.
- v1's `AddParentRole` and `RemoveParentRole` are removed: they sent a route that the API's
  documentation does not list. A role's parents are the roles that it extends: set them with the
  `Extends` of `Update`.
- `Ancestors` and `Descendants` are new: each returns the roles that the API lists as the
  ancestors or the descendants of a role, as a `[]models.RoleRead`, whether the reply holds them
  as a JSON array or as a paginated object, and an empty slice, not nil, when it holds none.
- `BulkReplace` is new: it creates or replaces many roles with one request,
  `PUT /v2/schema/{proj_id}/{env_id}/bulk/roles`, each a `models.RoleCreateBulk`, and returns the
  reply's lists of the roles that it created and of those that it replaced, as a
  `*models.RoleCreateBulkOperationResult`.
- A key of a role is sent as one segment of the path, escaped; an empty key, `.` or `..` is an
  error that matches `ErrInvalidConfig`, and no request for the call is sent. `Delete` returns
  nil when the API answers 404, as the role is gone already; a 404 to `RemovePermissions` is an
  error that matches `ErrNotFound`. `Create` is sent again after a 429 only; the other calls are
  sent again after a 500, 502, 503 or 504 reply or a failed connection too.

### Resource roles

- The `ResourceRoles` field of the client, a `*permit.ResourceRolesService`, lists, reads,
  creates, changes and deletes the roles of a resource of the environment of the client's scope,
  each method taking the key or the ID of the resource first: `List` returns a page as a
  `*permit.Page[models.ResourceRoleRead]` and `All` reads every page, 100 roles at a time; `Get`
  reads one role by its key or its ID; `Create` takes a `models.ResourceRoleCreate` and `Update` a
  `models.ResourceRoleUpdate`, and each returns the role as a `*models.ResourceRoleRead`; `Delete`
  deletes a role.
- `models.ResourceRoleRead` has the attributes of a role, a `models.Attributes` whose values may
  be of any JSON type, and the key of its resource, which v1's model dropped;
  `models.ResourceRoleCreate` and `models.ResourceRoleUpdate` have the attributes and `GrantedTo`,
  which v1's did not have.
- In `models.ResourceRoleUpdate`, `Permissions` and `Extends` are plain slices, each sent as a
  whole: nil leaves the list out of the request; an empty slice sends `[]`, which clears it; and a
  list replaces it. `Description`, `Attributes` and `GrantedTo` are `models.Optional` fields, so
  an update can send null for them.
- `AssignPermissions` and `RemovePermissions` take the permissions as a `[]string`, in place of
  v1's request models, and return the role after the change; a nil slice sends `[]`.
- `Ancestors` and `Descendants` take the place of v1's `GetAncestors` and `GetDescendants`: each
  returns the roles that the API lists as the ancestors or the descendants of a role of a
  resource, as a `[]models.ResourceRoleRead`, whether the reply holds them as a JSON array or as a
  paginated object, and an empty slice, not nil, when it holds none.
- v1's `AddParent` and `RemoveParent` are removed: they sent a route that the API's documentation
  does not list. A role's parents are the roles of its resource that it extends: set them with
  the `Extends` of `Update`.
- A key of a resource or of a role is sent as one segment of the path, escaped; an empty key, `.`
  or `..` is an error that matches `ErrInvalidConfig`, and no request for the call is sent.
  `Delete` returns nil when the API answers 404, as the role is gone already; a 404 to
  `RemovePermissions` is an error that matches `ErrNotFound`. `Create` is sent again after a 429
  only; the other calls are sent again after a 500, 502, 503 or 504 reply or a failed connection
  too.

### Implicit grants

- The `ImplicitGrants` field of the client, a `*permit.ImplicitGrantsService`, creates and
  deletes the implicit grants of a role of a resource of the environment of the client's scope,
  its role derivations, and sets their conditions: `Create` takes a `models.DerivedRoleRuleCreate`
  and returns the rule as a `*models.DerivedRoleRuleRead`; `Delete` takes a
  `models.DerivedRoleRuleDelete`; `UpdateConditions` takes a `models.DerivationSettings` and
  returns the conditions as the API stores them. A `models.DerivationSettings`, there and in the
  `When` of a rule, always sends `no_direct_roles_on_object`, `false` too, as v1's constructor of
  the settings did: the PUT of the conditions replaces them.
- Each method takes the key or the ID of the resource first and then that of the role, as the
  route names them. v1's `Delete(ctx, roleId, resourceId, ...)` named its parameters the other way
  round and sent its first argument as the resource; `Delete` takes `resourceKey` first, as
  `Create` does: a v1 call that passed the resource first works unchanged, and one that passed the
  role first, as v1's names said, must swap its two arguments.
- A key of a resource or of a role is sent as one segment of the path, escaped; an empty key, `.`
  or `..` is an error that matches `ErrInvalidConfig`, and no request for the call is sent. A 404
  to `Delete` is an error that matches `ErrNotFound`, as its body names the rule to delete.
  `Create` is sent again after a 429 only; `Delete` and `UpdateConditions` are sent again after a
  500, 502, 503 or 504 reply or a failed connection too.

### User attributes

- The `UserAttributes` field of the client, a `*permit.UserAttributesService`, lists, reads,
  creates, changes and deletes the attributes of the users of the environment of the client's
  scope: the schema of what a user may hold, not the values of a user. v1 sent these routes only
  through its generated client. `List` returns a page as a
  `*permit.Page[models.ResourceAttributeRead]` and `All` reads every page, 100 attributes at a
  time; `Get` reads one attribute by its key or its ID; `Create` takes a
  `models.ResourceAttributeCreate` and `Update` a `models.ResourceAttributeUpdate`, and each
  returns the attribute as a `*models.ResourceAttributeRead`, the models of the attributes of a
  resource, as the API takes and returns them; `Delete` deletes an attribute.
- No call sends `resource_id`, which the API's documentation lists on each route, so the calls
  act on the attributes of the users alone; `ResourceAttributes` acts on those of another
  resource. `List`, `All`, `Get`, `Create` and `Update` read an attribute of every type, `object`
  and `object_array` included, and keep a type that the API adds later as it is.
- The calls act on the environment of the client's scope, from `WithScope` or from the scope of
  the API key, which the client looks up once; a scope without an environment is an error that
  matches `ErrInvalidConfig`, and no request for the call is sent. Each call is sent with an
  environment-level API key, or with a key of a wider level and `WithScope`.
- The key of an attribute is sent as one segment of the path, escaped; an empty key, `.` or `..`
  is an error that matches `ErrInvalidConfig`, and no request for the call is sent. `Delete`
  sends no query, and returns nil when the API answers 404, as the attribute is gone already.
  `Create` is sent again after a 429 only; the other calls are sent again after a 500, 502, 503
  or 504 reply or a failed connection too.

### Condition sets

- The `ConditionSets` field of the client, a `*permit.ConditionSetsService`, lists, reads,
  creates, changes and deletes the condition sets of the environment of the client's scope, its
  user sets and resource sets: `List` returns a page as a `*permit.Page[models.ConditionSetRead]`
  and `All` reads every page, 100 sets at a time; `Get` reads one set by its key or its ID, in
  place of v1's `Get`, `GetById` and `GetByKey`; `Create` takes a `models.ConditionSetCreate` and
  `Update` a `models.ConditionSetUpdate`, and each returns the set as a
  `*models.ConditionSetRead`; `Delete` deletes a set.
- `List` and `All` read the list whether the API answers it as a JSON array or as the paginated
  object, as v1.3.0 does. A reply in 200-299 whose body is empty, `null`, or an object without a
  `data` array is an error that names the model, where v1.3.0 returned an empty list. The `Type`
  and the `Search` of `permit.ConditionSetsListOptions`, which v1 did not send, are sent in the
  query only when they are set.
- `Ancestors` and `Descendants`, which v1 sent only through its generated client, are new: each
  returns a page of the sets that the API lists as the ancestors or the descendants of a set, as
  a `*permit.Page[models.ConditionSetRead]`, and `AncestorsAll` and `DescendantsAll` read every
  page.
- `models.ConditionSetType` is an open enum, with `ConditionSetTypeUserSet` and
  `ConditionSetTypeResourceSet`: a set of a type that the API adds later is read, where v1 failed
  on it. The conditions of a set are a `models.Attributes`, whose numbers keep every digit.
- A user set takes no resource: `Create` refuses a user set with a `ResourceID` before any
  request, with an error that matches `ErrInvalidConfig`. Only a resource set has a resource.
- A key of a set is sent as one segment of the path, escaped; an empty key, `.` or `..` is an
  error that matches `ErrInvalidConfig`, and no request for the call is sent. `Delete` returns nil
  when the API answers 404, as the set is gone already. `Create` is sent again after a 429 only;
  the other calls are sent again after a 500, 502, 503 or 504 reply or a failed connection too.

### Condition set rules

- The `ConditionSetRules` field of the client, a `*permit.ConditionSetRulesService`, lists,
  creates and deletes the condition set rules of the environment of the client's scope, each of
  which grants the users of a user set a permission on the resources of a resource set, in place
  of v1's `ConditionSets.ListSetPermissions`, `AssignSetPermissions` and `UnassignSetPermissions`:
  `List` returns a page as a `*permit.Page[models.ConditionSetRuleRead]` and `All` reads every
  page, 100 rules at a time; `Create` takes a `models.ConditionSetRuleCreate` and returns the
  rules of the API's reply as a `[]models.ConditionSetRuleRead`; `Delete` takes a
  `models.ConditionSetRuleRemove`.
- The `UserSet`, `Permission` and `ResourceSet` of `permit.ConditionSetRulesListOptions` are sent
  in the query only when they are set, where v1.3.0 sent `user_set=`, `permission=` and
  `resource_set=` on every call, and `All` reads past the first page, where v1.3.0 read only the
  API's first page of 30 rules.
- The routes of the rules start with `/v2/facts/`, but only the API serves them: a client with
  `WithFactsViaPDP` sends them to the API, and no call waits for sync. A 404 to `Delete`, whose
  body names the rule, is an error that matches `ErrNotFound`, never nil. The API's documentation
  says that a create of a permission that is granted already, and a delete of one that is not,
  are skipped, so each call is sent again after a 500, 502, 503 or 504 reply or a failed
  connection, as after a 429.

### Users

- The `Users` field of the client, a `*permit.UsersService`, lists, reads, creates, syncs,
  changes and deletes the users of an environment, and assigns and unassigns their roles: `List`
  returns a page as a `*permit.Page[models.UserRead]`, with the counts of the reply, and `All`
  reads every page, 100 users at a time; `permit.UsersListOptions` filters the list with `Search`
  and `Role`; `Get` reads one user by its key or its ID, in place of v1's `Get`, `GetById` and
  `GetByKey`; `Create` takes a `models.UserCreate` and `Update` a `models.UserUpdate`; `Delete`
  deletes a user.
- `AssignRole` takes a `models.UserRoleCreate`, a role in a tenant or on a resource instance, in
  place of v1's `AssignRole` and `AssignResourceRole`, and returns a `*models.RoleAssignmentRead`;
  an empty tenant is left out of the request, where v1 sent it empty. `UnassignRole` takes a
  `models.UserRoleRemove`, in place of v1's `UnassignRole` and `UnassignResourceRole`, and returns
  only an error, where v1 returned a `*models.UserRead`: nil for a reply in 200-299, a 204 without
  a body or a 200 with the role assignment, and an error that matches `ErrNotFound` for a 404, as
  its body, not its path, names the role to remove.
- `Sync`, in place of v1's `SyncUser`, sends the whole user in one request,
  `PUT /v2/facts/{proj_id}/{env_id}/users/{user_id}`, which creates the user or replaces it:
  **each of `Email`, `FirstName`, `LastName` and `Attributes` that the `models.UserCreate` leaves
  out is cleared**, where v1's `SyncUser` read the user and then sent a `PATCH` of the fields that
  were set, or a `POST`. The request also sends the `RoleAssignments` of the `models.UserCreate`;
  what a sync with or without them does to the roles that the user has already is not confirmed
  yet. It returns a `*permit.UserSyncResult`, with the user as the API stores it and `Created`,
  true for a reply of 201 and false for a 200. The client's own `SyncUser` is gone: call
  `client.Users.Sync`.
- `models.UserCreate.RoleAssignments` assigns roles with a create or a sync, each a
  `models.UserRoleCreate` in a tenant or on a resource instance. `models.UserRead` adds
  `CreatedAt`, `UpdatedAt` and `AssociatedTenants`, the tenants in which the user has a role, and
  leaves out `Roles`, which the API's spec marks deprecated. Each field of `models.UserUpdate` is
  a `models.Optional`: unset leaves the field out, `models.Null` sends null and `models.Some`
  sends the value; attributes replace the user's attributes as a whole.
- The calls are calls of the facts API: they act on the environment of the client's scope, or,
  with `WithFactsViaPDP`, go to the PDP container under `/facts/users`. Each is sent with an
  environment-level API key, or higher. `UsersService.WaitForSync(timeout, opts...)` returns a
  copy whose `Create`, `Sync`, `Update`, `AssignRole` and `UnassignRole` the PDP container holds
  until the change reaches its policy; any other call through the copy, and a call from a client
  without `WithFactsViaPDP`, is an error that matches `ErrInvalidConfig`, and no request is sent.
- A key of a user is sent as one segment of the path, escaped; an empty key, `.` or `..` is an
  error that matches `ErrInvalidConfig`, and no request for the user is sent. `Delete` returns
  nil when the API answers 404, as the user is gone already. A reply in 200-299 that is not a
  user, or not a role assignment to `AssignRole`, an empty body too, is an error that names the
  model. `Create` and `AssignRole` are sent again after a 429 only; the other calls are sent
  again after a 500, 502, 503 or 504 reply or a failed connection too.

### Role assignments

- The `RoleAssignments` field of the client, a `*permit.RoleAssignmentsService`, lists, assigns
  and unassigns the roles that the users of an environment have in its tenants and on its
  resource instances, in place of v1's `RoleAssignments`: `List` returns a page as a
  `*permit.Page[models.RoleAssignmentRead]`, and `All` reads every page, 1000 role assignments at
  a time, the most that the route gives; `ListDetailed` and `AllDetailed` do the same with
  `models.RoleAssignmentDetailedRead`, whose user, role, tenant and resource instance are objects.
  A page without role assignments has an empty `Data`, not nil.
- `permit.RoleAssignmentsListOptions` filters the lists with `Users`, `Roles` and `Tenants`, each
  a list of keys sent in its query parameter once for each key, and `ResourceInstance`; a filter
  that is not set is not sent. `Users.GetAssignedRoles` of v1 is a `List` with `Users` and
  `Tenants`.
- `ListDetailed` reads `GET /v2/facts/{proj_id}/{env_id}/role_assignments/detailed`, where v1
  sent the deprecated query parameter `detailed=true` to the list; no call sends `detailed`,
  `include_total_count` or `return_deleted`.
- `Assign` takes a `models.RoleAssignmentCreate` and returns the `*models.RoleAssignmentRead`; an
  empty tenant is left out of the request, where v1's model sent it empty. `Unassign` takes a
  `models.RoleAssignmentRemove` and returns only an error: nil for a reply in 200-299, and an
  error that matches `ErrNotFound` for a 404, as its body, not its path, names the role to
  remove. v1 had both only in its generated client.
- The calls are calls of the facts API: they act on the environment of the client's scope, or,
  with `WithFactsViaPDP`, go to the PDP container under `/facts/role_assignments`. Each is sent
  with an environment-level API key, or higher. `RoleAssignmentsService.WaitForSync(timeout,
  opts...)` returns a copy whose `Assign` and `Unassign` the PDP container holds until the change
  reaches its policy; a list through the copy, and a call from a client without
  `WithFactsViaPDP`, is an error that matches `ErrInvalidConfig`, and no request is sent.
- `Assign` is sent again after a 429 only; the other calls are sent again after a 500, 502, 503
  or 504 reply or a failed connection too. A reply in 200-299 to `Assign` that is not a role
  assignment, or to a list that is not a list of role assignments, an empty body too, is an error
  that names the model.

### Proxy configs

- The `ProxyConfigs` field of the client, a `*permit.ProxyConfigsService`, lists, reads,
  creates, changes and deletes the proxy configs of the environment of the client's scope, in
  place of v1's `ProxyConfigs`: `List` returns a page as a `*permit.Page[models.ProxyConfigRead]`
  and `All` reads every page, 100 configs at a time; `Get` takes the key or the ID of a config, in
  place of v1's `Get`, `GetByKey` and `GetById`; `Create` takes a `models.ProxyConfigCreate`, and
  `Update` a `models.ProxyConfigUpdate`, each as a struct literal; `Delete` returns nil when the
  API answers 404, as the config is gone already.
- The secret of a proxy config is a `models.ProxySecret`, a union of three branches, one of which
  is set, as the config's `models.AuthMechanism` names it: `Basic`, a credential of the form
  `user:password`, and `Bearer`, a token, each a `*models.Secret`, and `Headers`, a
  `map[string]models.Secret` of the headers that the Permit proxy adds to a request. Create and
  Update send a secret of headers as a JSON object, which v1 could not send, and the two others
  as a string. A reply's secret is read as its form and the config's auth mechanism say: a JSON
  object as `Headers`, which v1 could not read, and a string as `Basic` when the auth mechanism
  is `Basic`, and as `Bearer` otherwise.
- The branch of the secret decides the auth mechanism: Create and Update always send one, the
  `AuthMechanism` of the call, or, when it is empty, the mechanism of the branch that the secret
  sets. Each of these is an error that matches `ErrInvalidConfig`, and no request is sent: a
  secret that sets no branch, or more than one; a branch whose credential is empty, such as a
  `Bearer` of the empty string, `Headers` without a header, or a header whose value is empty; and
  an `AuthMechanism` of the SDK's constants that names another branch than the one that the
  secret sets. An auth mechanism that the SDK does not know is sent as it is.
- A `models.ProxySecret` prints `[REDACTED]` in place of each credential with `fmt` and every
  verb, `String`, `GoString` and `MarshalJSON`, so a printed, logged or encoded config holds no
  credential, while the request of a create or an update holds the credentials themselves. A
  `models.ProxySecret` that sets no branch encodes as `null`, as it decodes from `null`. The
  client adds each credential that a call sends or returns to the credentials that it keeps out
  of its errors and its log lines, as it does its own API key, the value of each header of a
  secret of headers too, also where the body of an error reply echoes it. It replaces each of
  them wherever it appears in a later error or log line of the client, so a secret of headers
  should hold only credentials: the value of a header that is not one, such as a region or a
  version, is replaced in those too, also inside a request ID, a path or a status code.
- An update always sends the secret, as the API answers an update without it with a 422. Create
  and Update are sent again after a 429 only: the API adds the mapping rules of an update to
  those that it stores, and removes one only when its `ShouldDelete` is set, so a second request
  could add or remove a rule again. The other calls are sent again after a 500, 502, 503 or 504
  reply or a failed connection too.
- The routes of the proxy configs start with `/v2/facts/`, but only the API serves them: a client
  with `WithFactsViaPDP` sends them to the API, and no call waits for sync. A key is sent as one
  segment of the path, escaped; an empty key, `.` or `..` is an error that matches
  `ErrInvalidConfig`, and no request is sent.
- `models.MappingRule` and `models.MappingRuleUpdate`, which adds `ShouldDelete`, map a URL and
  an HTTP method to a resource and an action, which `Client.CheckURL` decides on.
  `models.AuthMechanism`, `models.Methods` and `models.URLType` are open enums.

### Errors

- A call whose request fails returns a `*permit.Error`, or an error that wraps one. It names the
  operation, the HTTP method and the route template of the request, and holds the status, the
  API's error code, title, detail and request ID, the body of a reply whose status is not in
  200-299, cut to 8 KiB, and the cause of a request that got no reply. Its text never includes the
  body.
- Check what failed with `errors.Is` and the sentinels `ErrNotFound` (404), `ErrConflict` (409),
  `ErrUnauthorized` (401), `ErrForbidden` (403) and `ErrRateLimited` (429), and
  `ErrContainerPDPOnly`, `ErrInvalidConfig`, `ErrMalformedDecision` and `ErrClosed`. Read the
  fields with `errors.AsType[*permit.Error]` or `errors.As`.
- The cause of a request that got no reply stays wrapped, so `errors.Is(err,
  context.DeadlineExceeded)` reports a deadline.
- An error has its credentials replaced with a marker when the SDK creates it: the client's API
  key and the credentials its calls return, which each client keeps to itself, Permit API keys,
  Authorization header values, and the values of JSON fields that name a credential, such as
  `token` or `password`. A reply in 200-299 that the SDK cannot decode gives an error that names
  the model and keeps nothing of the body.

### Logging

- `WithLogger(*slog.Logger)` sets the logger that the client writes to. By default the client
  logs nothing, and so it does with a nil `*slog.Logger` or one whose handler is nil, such as the
  zero `slog.Logger`. The module does not require zap: an application that logs with zap gives
  the client a logger whose handler is zapslog's, as the documentation of `WithLogger` shows.
- Every line has a fixed message that starts with `permit: `. At Debug, the client writes one
  line for each attempt of a request, with the attributes `route` (the route template),
  `status`, `attempt`, `duration` and `error`, and never a header or a body. At Warn, `New`
  writes each warning about the configuration once: when the API URL uses `http://` to a host
  that is not loopback (localhost, 127.0.0.0/8 or ::1), and when `WithFactsViaPDP` sends facts to
  the cloud PDP, `cloudpdp.api.permit.io`, which does not serve them. A PDP URL over `http://`
  gets no warning.
- Each line has the client's credentials replaced with a marker before the handler gets it: in
  the message, in the keys and string values of attributes, in groups at any depth, in what a
  `slog.LogValuer` returns, and in errors, which reach the handler as their text only.

### Requests and replies

- Every request, to the API and to the PDP, carries `Authorization: Bearer <API key>`,
  `User-Agent: permit-golang/<version> (<Go version>)`, such as
  `permit-golang/2.0.0 (go1.27.1)`, `X-Permit-SDK-Language: go`,
  `X-Permit-SDK-Version: <version>` and `Accept: application/json`, and a request with a body
  `Content-Type: application/json`.
- The client closes the body of every reply. Before it does, it reads and discards up to 1 MiB of
  what it did not read, so that the next request goes over the same connection instead of a new
  one.
- A reply in 200-299 whose body is empty, or is not what the call expects (a JSON object, or a
  list as a JSON array or as the paginated object), is an error that names the model, or the kind
  of list for a list of the PDP, never an empty result without an error.
- Deleting an object by a path that names it, such as a user by its key, returns nil when the API
  answers 404, as the object is gone already. A removal whose body names what it removes, such as
  a role assignment, returns an error that matches `ErrNotFound` for a 404, which can mean a
  mistyped key.
- A 404 with an empty body from a route that only the PDP container serves, as the cloud PDP
  answers it, is an error that matches `ErrContainerPDPOnly`, not `ErrNotFound`, and names the
  route.
- Request bodies are JSON that `encoding/json` writes, with `<`, `>` and `&` sent as they are
  rather than escaped as `\u003c`, `\u003e` and `\u0026`.
- A number in a query keeps every digit: a `float64` is the shortest text that reads back as the
  same value, as `encoding/json` writes it, such as `0.30000000000000004`, and an integer is its
  decimal digits, such as `9007199254740993`.
- A list in a query repeats its key, such as `user=a&user=b`. A filter that is not set is left
  out, and the query parameters that the API marks deprecated, `detailed`, `include_total_count`
  and `return_deleted`, are never sent.
- Each path parameter is escaped on its own, so a key such as `a/b` stays one segment, `a%2Fb`.
  An empty path parameter, or one that is `.` or `..`, fails with an error that matches
  `ErrInvalidConfig` before any request is sent.
- A list reply is read by its first byte, as a JSON array or as the paginated object that holds
  the array in `data`, so both shapes give the same items, and a reply of another shape is an
  error rather than an empty list.
- A number in a reply that a model holds in an `any`, such as an attribute value, is read as a
  `json.Number` with every digit of its text, such as `9007199254740993`, rather than as a
  `float64`, which rounds integers beyond 2^53. Read it with its `Int64`, `Float64` or `String`
  method.

### Lists

- A `List` method returns one page as a `*permit.Page[T]`: its items in `Data`, the `Page` and
  `PerPage` that the call asked for, and `TotalCount` and `PageCount` when the reply reports
  them, else nil. `permit.ListOptions{Page, PerPage}` picks the page; 0 asks for the first page,
  and for the API's default number of items on it. A `Page` or `PerPage` less than zero is an
  error that matches `ErrInvalidConfig`, and no request is sent.
- An `All` method reads every page and returns an `iter.Seq2[T, error]` of the items, for a
  `range` loop. It asks for the largest page that the route accepts, unless `PerPage` is set,
  and starts at `Page`. It stops after a page without items, or after the last page when the
  reply reports `page_count`, and never after a page with fewer items than it asked for. A
  request that fails ends the loop with its error, yielded once. A loop that stops early sends no
  other request. Each page is a call of its own, with its own timeout and retries.
- The client sends `per_page` as given, with no limit of its own, also above the route's limit,
  which differs from route to route.

### Models

- `models.Optional[T]` is the type of a field of an update model that the API accepts null for.
  Its zero value leaves the field out of the body, `models.Null[T]()` sends null and
  `models.Some(v)` sends `v`, also when `v` is `""`, `0` or `false`. `Get`, `IsNull` and
  `IsZero` tell the three states apart. A field of this type is tagged `omitzero`; encoding an
  unset `Optional` without it fails rather than sending null, and so does `Some` of a value that
  encodes as null, such as a nil slice.
- Enums are open: `models.AttributeType` is a string type with a constant for each value the SDK
  knows, such as `models.AttributeTypeString`, and a `Known` method. A value that the API adds
  later decodes as it is, and `Known` reports false for it.
- `models.Attributes` is a `map[string]any` that keeps each number as a `json.Number`, also in
  nested objects and arrays and under a plain `json.Unmarshal`. A plain `Attributes` field of an
  update model leaves the field out when it is nil, and sends `{}` for an empty map;
  `models.EnvironmentUpdate.Settings` is a `models.Optional[models.Attributes]` instead.
- `models.Time` embeds `time.Time` and reads an RFC 3339 timestamp with or without a zone; one
  without a zone is in UTC.
- `models.StringOrList` is the type of a property that is a string or a list of strings:
  `models.OneString` sends a JSON string and `models.StringList` a JSON array, a value read from
  a reply keeps the form that the reply gave it, and `Values` returns its strings in either form.
- `models.APIKeyScopeRead` is the scope of an API key: the IDs of its organization, project and
  environment.

### Retries

- A call sends its request again after a reply with status 429, to any operation except one whose
  documentation says that it is sent once, after the wait that the reply's `Retry-After` header
  asks for, in seconds or as an HTTP-date.
- A call sends its request again after a reply with status 500, 502, 503 or 504, or a request
  that got no complete reply, only to an operation that does the same when it is sent twice, such
  as a GET, a PUT or a DELETE. A POST that creates an object is not sent again after these
  replies; only a 429 makes the client send it again.
- Without a `Retry-After` header, the wait before a retry is random: up to 200 milliseconds
  before the first, and up to twice as long before each later one, up to 5 seconds.
- `WithRetry(permit.RetryPolicy{MaxRetries: n, MaxBackoff: d})` sets the most retries of a call,
  3 by default, and the longest wait without a `Retry-After` header, 5 seconds by default (and
  for a `MaxBackoff` of 0). A `MaxRetries` of 0 sends each request once.
- The timeout of a call covers every attempt and every wait between them. A call never waits past
  its deadline, nor for a `Retry-After` of more than 60 seconds: it returns the error of its last
  attempt at once, which matches `ErrRateLimited` for a 429. A call whose context ends while it
  waits returns an error that matches the context's error.
- Each attempt has its own Debug line, with its number, from 1, in `attempt`.

## v1.3.0 (2026-10-13)

### API keys and proxy config secrets in logs from v1.2.8 and earlier

If your application printed the SDK configuration or a client, or logged the SDK's errors or its
log output, with v1.2.8 or earlier, those logs may contain your Permit API key. Rotate the API key
and delete those logs. If you read proxy configs whose secret is a set of headers, and logged
`PermitError.ResponseBody` or debug output, those logs may also contain the header values: rotate
those secrets too.

### Behaviour changes

- Checks send a resource key for resources that have an ID and no key, so the PDP checks the
  resource instance instead of only its type:
  - `Check`, `BulkCheck` and `AllTenantsCheck`: a resource built with `WithID` and no `WithKey`
    sends its ID as `key`, the field the PDP reads.
  - `FilterObjects`: an `*enforcement.Resource` built with `WithKey` sends its key, which was
    dropped before, and any other object, including your own `ResourceI` types, sends `GetID()`
    as `key`.

  Before, the PDP received no key for these resources, so it checked the resource type, and every
  object of that type got the same decision. Decisions can now change for resources that have
  instance-level roles or relationships in Permit. Check the calls that pass resources built with
  `WithID`, and the objects you pass to `FilterObjects`: the IDs they send must be the resource
  instance keys you use in Permit. To keep a type-level check, leave both the ID and the key empty.
- `Check`, `BulkCheck`, `FilterObjects`, `AllTenantsCheck`, `GetUserPermissions` and
  `GetUserPermissionsWithOptions` return a `PermitError` with `ErrorCode` `UnexpectedError` and the
  status when the PDP answers with a status the SDK did not handle, such as 400, 421 or 429.
  Before, `Check` returned `false` with a nil error, so a failure looked like a deny, and
  `AllTenantsCheck` returned no tenants. `errors.HttpErrorHandle(nil, response)` now returns this
  error, instead of nil, for a status outside 200-299 that it does not map to another code.
  A rate-limited check (429) now returns an error instead of `false`: handle the error, for
  example by retrying with a backoff, instead of treating it as a deny.
- A JSON document in `PermitError.ResponseBody` that has a key naming a credential (see Fixes), or
  a credential written with JSON escapes, is now stored encoded again: compact, with the keys of
  each object sorted. Other bodies keep their bytes, apart from the values replaced in them.
- `Check`, `BulkCheck`, `FilterObjects`, `AllTenantsCheck`, `GetUserPermissions` and
  `GetUserPermissionsWithOptions` log the PDP response body of a failed request only at Debug
  level, with credentials replaced as in `PermitError.ResponseBody`. Before, they logged the whole
  body at Error level, and all but `Check` used it as the log message. The Error level line now
  has a fixed message, the error and the status code (`status_code`), without the body.

### Dependencies

- The module no longer requires `golang.org/x/net` or `golang.org/x/oauth2`, so its `go.mod` also
  drops `google.golang.org/appengine`, `github.com/golang/protobuf` and
  `google.golang.org/protobuf`. Applications that use the SDK no longer inherit these modules from
  it, including the old minimum versions of x/net and x/oauth2, which have published advisories.

### Releases

- Releases no longer attach prebuilt archives of the example server in `main.go`; a module
  version is published by its git tag. A manual workflow checks that the module proxy.golang.org
  serves for a version matches its tag.

### CI

- Pull requests, and pushes to `master` and release branches, run gofmt, `go vet`, the unit tests
  with the race detector on Go 1.19 and Go 1.27, and govulncheck, without Permit credentials, and
  fail when `go.mod` or `go.sum` is not tidy. The tests that need live Permit credentials now build
  only with the `integration` build tag.
- A separate workflow runs the integration tests against Permit for pushes to `master` and release
  branches and for pull requests from branches of this repository. It runs them with a PDP
  container pinned by digest, and deletes what each run created.
- The GoReleaser and pre-commit workflows and `.pre-commit-config.yaml` are removed.

### Fixes

- `WaitForSync` no longer changes the client it is called on. It wrote `X-Wait-Timeout` and
  `X-Timeout-Policy` into headers shared by every API (`Users`, `Tenants`, ...) of the same
  client, so later requests that were not meant to wait carried them, and calling it while other
  requests were in flight could crash the process with a fatal concurrent map access error. The
  headers now go only on the requests of the client that `WaitForSync` returns, and `WaitForSync`
  no longer shares headers with other requests.
- `WaitForSync` and `NewFactsClientConfig` send the timeout in seconds without truncating it:
  1.5s is sent as `1.5` instead of `1`, and a timeout under one second is no longer sent as `0`.
- `ConditionSets.List` could never work before: the API answers with a JSON array, the SDK read
  only the paginated form, and the result was always empty. It now reads both forms.
- `RoleAssignments.List` returns an empty list instead of a nil pointer when there are no role
  assignments. `ResourceRoles.GetAncestors` and `ResourceRoles.GetDescendants` do the same when
  the response holds no roles.
- `ConditionSets.List`, `RoleAssignments.List`, `RoleAssignments.ListDetailed`,
  `Users.GetAssignedRoles`, `ResourceRoles.GetAncestors` and `ResourceRoles.GetDescendants` return
  an empty list instead of panicking when a successful response has an empty body.
- An API URL too short to hold a host, such as the empty string in `NewPermitConfig("", ...)`, no
  longer makes `NewPermit` panic; requests made with it return an error. `NewFactsClientConfig`
  handles a short PDP URL the same way.
- A nil logger no longer makes the SDK panic the first time it logs an error. `NewPermitConfig`
  with a nil logger, a builder chain used without `Build()`, a `PermitConfig` literal and
  `WithLogger(nil)` now all give a client that uses a no-op logger.
- `ResourceActionGroups.Delete` could never work before: it sent the request to the resource
  action route, so it deleted the action whose key matched the group's key, or failed when there
  was no such action. It now deletes the action group.
- Concurrent first calls on a client whose context is not set no longer each send a
  `GET /v2/api-key/scope` request and write the client's context at the same time. One call sends
  the request; calls that start while it is in flight wait for it, or until their own context
  ends, instead of sending their own, and share its result. The next call after a failed request
  sends a new one.
- When the API key scope request fails, the error is still a `PermitError` with `ErrorCode`
  `ContextError`, and it now keeps the cause: the text includes the status and the underlying
  error, `StatusCode` and `ResponseBody` come from the scope response, and `errors.Is`,
  `errors.As` and `errors.Unwrap` reach the underlying error, such as `context.Canceled` or a
  connection error. Before, the status, body and cause were dropped, and every failure other than
  401 and 403 had the same text. Because the text now includes the cause, match on `ErrorCode`
  (`ContextError`) or on the message constants, such as `errors.UnauthorizedMessage`, not on the
  full text. `PermitError` has a new `Unwrap` method, and `errors.NewPermitContextErrorWithCause`
  is new.
- Errors from `BulkCheck`, `FilterObjects`, `AllTenantsCheck`, `GetUserPermissions` and
  `GetUserPermissionsWithOptions` include the PDP response body in `ResponseBody`; it was empty.
- `AllTenantsCheck` returns only the tenants where the PDP allowed the action. Before, it also
  returned tenants whose decision was `"allow": false`.
- Printing the SDK configuration or a client with `fmt` (the `%v`, `%+v`, `%#v`, `%s`, `%q`,
  `%x`, `%X` and `%d` verbs, with any flags and width) no longer shows the API key:
  - `PermitConfig` holds the API key behind a pointer, so a `PermitConfig`, the config builder, a
    `permit.Client`, its `Api` and the APIs in it such as `Users`, and an enforcer print an
    address in place of the key. Their other fields print as before. Two `PermitConfig` values
    from separate `NewPermitConfig` calls with the same arguments are no longer equal with `==`,
    because the key is compared by its pointer; copies of one config are still equal.
  - `openapi.Configuration`, such as the ones that `api.NewClientConfig` and
    `api.NewFactsClientConfig` return, and `openapi.APIClient` print `[REDACTED]` in place of the
    values of credential headers such as `Authorization`. They have new `Format` methods, and
    `openapi.Configuration` also has new `String`, `GoString` and `MarshalJSON` methods, so
    `encoding/json` and loggers that encode it as JSON show `[REDACTED]` for those headers too. A
    pointer to either type now prints without the leading `&`. A struct that embeds either type
    prints as that value alone, and one that embeds `openapi.Configuration` also encodes to JSON
    as it alone. If you set `Debug` on an `openapi.Configuration`, the generated client still
    logs each request with its headers; the SDK never sets it.
- The text and `ResponseBody` of a `PermitError` made from an API or PDP response have the API
  key the request was sent with, and any value that starts with `permit_key_`, replaced with
  `[REDACTED]`, also where a JSON body writes them with escapes. In a JSON `ResponseBody`, the
  strings and numbers under keys named `secret`, `token`, `api_key`, `apikey`, `password`,
  `passwd`, `pwd`, `authorization`, `secret_key`, `private_key`, `access_key`, `credential`,
  `credentials` or `cookie`, or ending in one of them such as `access_token`, `X-API-Key`,
  `aws_secret_access_key` or `Set-Cookie` (ignoring case, `-` and `_`), are replaced with
  `[REDACTED]` at any depth, and the rest of the body is kept. This includes the body kept when
  `ProxyConfigs` cannot read a proxy config whose secret is a set of headers. An API key shorter
  than 16 characters is not replaced, and a body nested more than 1000 levels deep is replaced
  with `[REDACTED]` as a whole. `ResourceActions`, `ResourceActionGroups` and
  `ResourceAttributes` still return the generated client's error type unchanged, as in v1.2.8;
  the SDK's own log line for those errors has the API key and `permit_key_` values replaced.
- `Tenants.List` and `Tenants.ListByAttributes` return the error when the request fails. Before,
  they logged it and returned a nil error, so a 401 or 500 response or a connection failure looked
  like an empty list, and a 200 response whose body they could not read gave no tenants or a
  partly read list. Now they return a nil list with the error: a `PermitError` with the status
  code when a response arrived, or else the error from sending the request, such as a connection
  failure or a cancelled context.
- The context error that a method returns when the client has no project or environment, such as
  with an organization API key and no `SetContext` call, names the call that sets them,
  `client.Api.SetContext(ctx, project, environment)`, and what to pass to it. Before, it named
  `PermitClient.SetPermitContext()`, which does not exist. The `ErrorCode` is still
  `ContextError`: match on `ErrorCode`, not on the text.
- When `SetContext` fails, its Error log line names the project and environment it could not set;
  the message was empty. With a project API key and no project, the logged error also asks for an
  environment.
- The list methods' log line for a page or page size out of range gives the page size limit as
  `100` and names what the method lists. Before, it gave `d`, and some methods named the wrong
  objects.

### Documentation

- The README examples compile. The `SyncUser` example checks the error before it reads the user,
  and the `Check` example prints the error and passes a resource type to `ResourceBuilder`. New
  README sections say which methods use the API URL and which use the PDP URL, with their
  defaults, and when to call `SetContext` for each API key level.
- The usage examples in the `pkg/api` doc comments call the method they document, with arguments
  of the right types, and the examples this release changes render as code in godoc. godoc now
  shows the `Environments.Update` doc comment. The doc comments of the `ResourceActionGroups`
  methods, of `ListByAttributes` on `Tenants`, `Roles` and `ResourceActions`, and of
  `ProxyConfigs.GetByKey` name their own method and object. `ProxyConfigs.List` says it lists the
  proxy configs of the context's environment, the `Projects` methods give the API key level each
  one needs, and `SetContext` has a doc comment.
