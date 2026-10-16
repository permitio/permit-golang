package resources

import (
	"context"
	"time"

	"github.com/permitio/permit-golang/pkg/api"    // v1: github.com/permitio/permit-golang/pkg/api
	"github.com/permitio/permit-golang/pkg/config" // v1: github.com/permitio/permit-golang/pkg/config
	"github.com/permitio/permit-golang/pkg/models" // v1: github.com/permitio/permit-golang/pkg/models
	"github.com/permitio/permit-golang/pkg/permit" // v1: github.com/permitio/permit-golang/pkg/permit
)

// Tenant manages tenants, and waits for each change to reach the PDP.
type Tenant struct {
	client *permit.Client // v1: permit.Client
	wait   time.Duration
}

// Create creates the tenant.
func (t *Tenant) Create(ctx context.Context, key, name string) error {
	options := api.WaitForSyncOptions{Policy: config.FactsSyncTimeoutPolicyFail} // v1: api.WaitForSyncOptions config.FactsSyncTimeoutPolicyFail
	tenants := t.client.Api.Tenants.WaitForSync(&t.wait, options)                // v1: api.Tenants.WaitForSync
	_, err := tenants.Create(ctx, *models.NewTenantCreate(key, name))            // v1: api.Tenants.Create models.NewTenantCreate
	return err
}

// Delete deletes the tenant.
func (t *Tenant) Delete(ctx context.Context, key string) error {
	return t.client.Api.Tenants.
		WaitForSync(&t.wait, api.WaitForSyncOptions{}). // v1: api.Tenants.WaitForSync api.WaitForSyncOptions
		Delete(ctx, key)                                // v1: api.Tenants.Delete
}
