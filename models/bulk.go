package models

// UserCreateBulkOperation is the body of a bulk create of the users of an environment, which
// UsersService.BulkCreate of the root package sends.
type UserCreateBulkOperation struct {
	// Operations are the users to create, in order, each as UsersService.Create sends one. The
	// request always holds them: UsersService.BulkCreate sends [] for a nil slice.
	Operations []UserCreate `json:"operations"`
}

// UserReplaceBulkOperation is the body of a bulk replace of the users of an environment, which
// UsersService.BulkReplace of the root package sends.
type UserReplaceBulkOperation struct {
	// Operations are the whole users to replace, in order, each as UsersService.Sync sends one.
	// The request always holds them: UsersService.BulkReplace sends [] for a nil slice.
	Operations []UserCreate `json:"operations"`
}

// UserDeleteBulkOperation is the body of a bulk delete of the users of an environment, which
// UsersService.BulkDelete of the root package sends.
type UserDeleteBulkOperation struct {
	// Idents are the keys or the IDs of the users to delete, in order. The request always holds
	// them: UsersService.BulkDelete sends [] for a nil slice.
	Idents []string `json:"idents"`
}

// TenantCreateBulkOperation is the body of a bulk create of the tenants of an environment, which
// TenantsService.BulkCreate of the root package sends.
type TenantCreateBulkOperation struct {
	// Operations are the tenants to create, in order, each as TenantsService.Create sends one.
	// The request always holds them: TenantsService.BulkCreate sends [] for a nil slice.
	Operations []TenantCreate `json:"operations"`
}

// TenantDeleteBulkOperation is the body of a bulk delete of the tenants of an environment, which
// TenantsService.BulkDelete of the root package sends.
type TenantDeleteBulkOperation struct {
	// Idents are the keys or the IDs of the tenants to delete, in order. The request always
	// holds them: TenantsService.BulkDelete sends [] for a nil slice.
	Idents []string `json:"idents"`
}

// ResourceInstanceCreateBulkOperation is the body of a bulk replace of the resource instances of
// an environment, which ResourceInstancesService.BulkReplace of the root package sends.
type ResourceInstanceCreateBulkOperation struct {
	// Operations are the instances to create or replace, in order, each as
	// ResourceInstancesService.Create sends one. The request always holds them:
	// ResourceInstancesService.BulkReplace sends [] for a nil slice.
	Operations []ResourceInstanceCreate `json:"operations"`
}

// ResourceInstanceDeleteBulkOperation is the body of a bulk delete of the resource instances of
// an environment, which ResourceInstancesService.BulkDelete of the root package sends.
type ResourceInstanceDeleteBulkOperation struct {
	// Idents are the instances to delete, in order, each as <resource>:<key>, such as
	// document:doc-1, or as the instance's ID. The request always holds them:
	// ResourceInstancesService.BulkDelete sends [] for a nil slice.
	Idents []string `json:"idents"`
}
