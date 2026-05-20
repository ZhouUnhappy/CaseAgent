package db

import (
	"context"
	"fmt"
	"strconv"

	"github.com/uptrace/bun"
)

// RunInTenantTx runs fn inside a transaction whose session-local
// app.tenant_id is set to the tenant attached to ctx. Phase 3 RLS policies
// rely on this setting; using set_config(...,true) ties the value to the
// transaction so it is cleared on commit/rollback.
//
// The tenant must be present in ctx (set via WithTenant). Otherwise this
// function refuses to open a transaction.
func RunInTenantTx(ctx context.Context, db *bun.DB, fn func(ctx context.Context, tx bun.Tx) error) error {
	tenantID, ok := TenantFromContext(ctx)
	if !ok {
		return fmt.Errorf("RunInTenantTx: no tenant in context")
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx, "SELECT set_config('app.tenant_id', ?, true)", strconv.Itoa(tenantID)); err != nil {
			return fmt.Errorf("set local app.tenant_id: %w", err)
		}
		return fn(ctx, tx)
	})
}
