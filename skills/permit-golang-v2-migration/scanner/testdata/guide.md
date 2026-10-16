# Migrating from v1 to v2

A guide in the format of MIGRATION.md, for the tests. Prose that names `api.Users.Create` maps
nothing, and neither does a field outside an entry:

- v1: `api.Users.Delete`

## Compatibility

<a id="M-module-1"></a>
### M-module-1: The module path ends in /v2

- v1: the module `github.com/permitio/permit-golang`
- v2: the module `github.com/permitio/permit-golang/v2`

<a id="M-module-2"></a>
### M-module-2: v2 needs Go 1.26 or later

- v1: the go directive `go 1.19` of v1's go.mod
- v2: the go directive `go 1.26.0` of v2's go.mod

## API

<a id="M-client-1"></a>
### M-client-1: New takes options

- v1: `permit.New`
- v1: `permit.NewPermit`
- v1: `config.NewConfigBuilder`
- v1: `config.PermitConfig.*`
- v2: `permit.New`

<a id="M-users-1"></a>
### M-users-1: SyncUser becomes Sync

- v1: `permit.Client.SyncUser`
- v1: `api.Users.SyncUser`
- v2: `permit.UsersService.Sync`

<a id="M-users-2"></a>
### M-users-2: Sync sends one request

- v1: `api.Users.SyncUser`
- v1: `api.Users.Create`
- v2: `permit.UsersService.Create`

Text after the fields, such as `- v1: `api.Users.Get``, is not a field.

<a id="M-errors-1"></a>
### M-errors-1: The error codes become sentinels

- v1: `errors.Not*`
- v1: `errors.PermitError`
- v2: `permit.ErrNotFound`

<a id="M-models-1"></a>
### M-models-1: Pointer fields become values

- v1: `models.ResourceCreate`
- v2: `models.ResourceCreate`

The parent, a `models.ResourceId` in v1, which only this text names, is a string.

### A heading that is not an entry

- v1: `api.Tenants.Create`

## Wire behaviour

<a id="M-wire-1"></a>
### M-wire-1: Requests are retried

- v1: each request sent once
- v2: a request sent again after a 429

## Kept on purpose

- v1: `api.Roles.Create`

## Appendix: v1 models types

<!-- apisurface:models:begin -->
<!-- apisurface -mode migration -write writes this table; do not edit it. -->

| v1 | v2 |
| --- | --- |
| `models.AttributeType` | `models.AttributeType` |
| `models.ResourceCreate` | `models.ResourceCreate` ([M-models-1](#M-models-1)) |
| `models.ResourceId` | `models.ResourceCreate.ParentID` ([M-models-1](#M-models-1)) |
| `models.TenantCreate` | `models.TenantCreate` |
| `models.UserCreate` | `models.UserCreate` |

Not mapped yet: 2 of the 7 types of v1's pkg/models.

<!-- apisurface:models:end -->

| `models.RoleCreate` | `models.RoleCreate` |
