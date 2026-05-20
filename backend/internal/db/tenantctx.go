package db

import "context"

type tenantCtxKey struct{}

// WithTenant attaches a tenant ID to ctx so downstream RunInTenantTx can
// read it back when scoping a transaction.
func WithTenant(ctx context.Context, tenantID int) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenantID)
}

// TenantFromContext returns the tenant ID attached via WithTenant.
// The bool is false if no tenant was set.
func TenantFromContext(ctx context.Context) (int, bool) {
	v, ok := ctx.Value(tenantCtxKey{}).(int)
	return v, ok
}
