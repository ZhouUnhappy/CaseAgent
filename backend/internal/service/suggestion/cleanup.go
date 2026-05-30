package suggestion

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

func StartExpiredPendingCleanup(ctx context.Context, bunDB *bun.DB, maxAge time.Duration, interval time.Duration) {
	if maxAge <= 0 || interval <= 0 {
		return
	}

	go func() {
		run := func() {
			if err := cleanupExpiredPendingForAllTenants(ctx, bunDB, maxAge); err != nil {
				slog.Warn("knowledge suggestion expiry cleanup failed", "error", err)
			}
		}

		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func cleanupExpiredPendingForAllTenants(ctx context.Context, bunDB *bun.DB, maxAge time.Duration) error {
	var tenants []models.Tenant
	if err := bunDB.NewSelect().Model(&tenants).Scan(ctx); err != nil {
		return fmt.Errorf("list tenants for suggestion cleanup: %w", err)
	}

	for _, tenant := range tenants {
		if err := db.RunInTenantTx(db.WithTenant(ctx, tenant.ID), bunDB, func(ctx context.Context, tx bun.Tx) error {
			count, err := New(tx).DismissExpiredPending(ctx, maxAge)
			if err != nil {
				return err
			}
			if count > 0 {
				slog.Info("expired pending knowledge suggestions dismissed",
					"tenant_id", tenant.ID,
					"count", count,
				)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("cleanup tenant %d suggestions: %w", tenant.ID, err)
		}
	}
	return nil
}
