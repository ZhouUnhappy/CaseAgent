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
//
// Race caveat: the goroutine launches before middleware.Tx commits the
// request tx. If fn's first query reads a row inserted by the same handler,
// the read may not see the row until the request commit lands. In practice
// the main path commits in ~1ms (PG roundtrip) while the goroutine needs
// BEGIN + SET app.tenant_id + first SELECT (~3 roundtrips) before reading,
// so on a local PG the main commit always wins. Higher-latency PG or
// connection-pool contention can shift this — if AnalyzeTask/etc. start
// reporting "row not found" on freshly created tasks, this is the cause and
// the fix is to defer goroutine launch until after middleware commit.
func RunAsync(bunDB *bun.DB, tenantID int, fn func(ctx context.Context, tx bun.Tx) error) {
	go func() {
		ctx := db.WithTenant(context.Background(), tenantID)
		if err := db.RunInTenantTx(ctx, bunDB, fn); err != nil {
			slog.Error("async tenant tx failed", "tenant_id", tenantID, "error", err)
		}
	}()
}
