package handler

import (
	"context"
	"log/slog"

	"caseagent/internal/db"

	"github.com/uptrace/bun"
)

// RunAsync starts a goroutine that opens its own RLS-scoped tx for tenantID
// and runs fn inside it. Used for handlers that need background work after
// returning a response — the request's own tx cannot be reused because it
// commits/rolls back when the request ends.
func RunAsync(bunDB *bun.DB, tenantID int, fn func(ctx context.Context, tx bun.Tx) error) {
	go func() {
		ctx := db.WithTenant(context.Background(), tenantID)
		if err := db.RunInTenantTx(ctx, bunDB, fn); err != nil {
			slog.Error("async tenant tx failed", "tenant_id", tenantID, "error", err)
		}
	}()
}
