package db_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// TestRLSIsolation verifies Phase 3 RLS policies actually scope queries to
// the tenant set via SET LOCAL app.tenant_id (Phase 2's RunInTenantTx).
// Requires a Postgres with the multi-tenancy schema applied. DSN comes from
// CASEAGENT_TEST_DSN; skipped if unset. The DSN role must NOT be superuser
// or have BYPASSRLS — otherwise policies silently no-op.
func TestRLSIsolation(t *testing.T) {
	dsn := os.Getenv("CASEAGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("set CASEAGENT_TEST_DSN to run RLS integration test")
	}

	ctx := context.Background()
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	t.Cleanup(func() { _ = sqldb.Close() })
	bunDB := bun.NewDB(sqldb, pgdialect.New())

	if bypass, err := userBypassesRLS(ctx, bunDB); err != nil {
		t.Fatalf("inspect role attributes: %v", err)
	} else if bypass {
		t.Skip("CASEAGENT_TEST_DSN role bypasses RLS (superuser or BYPASSRLS); use a NOBYPASSRLS role")
	}

	tenantA := insertTestTenant(t, ctx, bunDB, "rls-a")
	tenantB := insertTestTenant(t, ctx, bunDB, "rls-b")
	t.Cleanup(func() { cleanupTestTenants(ctx, bunDB, tenantA, tenantB) })

	var taskID int
	mustTx(t, ctx, bunDB, tenantA, func(ctx context.Context, tx bun.Tx) error {
		project := &models.Project{TenantID: tenantA, Name: "rls-test-a"}
		if _, err := tx.NewInsert().Model(project).Returning("id").Exec(ctx); err != nil {
			return err
		}

		task := &models.CaseGenerationTask{
			TenantID:    tenantA,
			ProjectID:   project.ID,
			DocumentIDs: []int{1},
			Status:      models.TaskStatusGenerating,
		}
		if _, err := tx.NewInsert().Model(task).Returning("id").Exec(ctx); err != nil {
			return err
		}
		taskID = task.ID

		_, err := tx.NewInsert().Model(&models.CaseGenerationJob{
			TenantID:   tenantA,
			TaskID:     intPointer(taskID),
			JobType:    models.JobTypeGenerate,
			Status:     models.JobStatusPending,
			MaxRetries: 1,
		}).Exec(ctx)
		return err
	})

	var count int
	mustTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		count, err = tx.NewSelect().Model((*models.Project)(nil)).Count(ctx)
		return err
	})
	if count != 0 {
		t.Fatalf("tenant B saw %d projects under RLS; expected 0", count)
	}

	mustTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		count, err = tx.NewSelect().Model((*models.CaseGenerationJob)(nil)).Count(ctx)
		return err
	})
	if count != 0 {
		t.Fatalf("tenant B saw %d case generation jobs under RLS; expected 0", count)
	}

	crossErr := db.RunInTenantTx(db.WithTenant(ctx, tenantB), bunDB, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(&models.Project{TenantID: tenantA, Name: "should-fail"}).Exec(ctx)
		return err
	})
	if crossErr == nil {
		t.Fatal("cross-tenant INSERT succeeded; WITH CHECK should have blocked it")
	}

	crossJobErr := db.RunInTenantTx(db.WithTenant(ctx, tenantB), bunDB, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(&models.CaseGenerationJob{
			TenantID:   tenantA,
			TaskID:     intPointer(taskID),
			JobType:    models.JobTypeGenerate,
			Status:     models.JobStatusPending,
			MaxRetries: 1,
		}).Exec(ctx)
		return err
	})
	if crossJobErr == nil {
		t.Fatal("cross-tenant job INSERT succeeded; WITH CHECK should have blocked it")
	}
}

func userBypassesRLS(ctx context.Context, db *bun.DB) (bool, error) {
	var row struct {
		IsSuperuser bool `bun:"is_superuser"`
		BypassRLS   bool `bun:"bypassrls"`
	}
	err := db.NewRaw(`SELECT rolsuper AS is_superuser, rolbypassrls AS bypassrls FROM pg_roles WHERE rolname = current_user`).Scan(ctx, &row)
	if err != nil {
		return false, err
	}
	return row.IsSuperuser || row.BypassRLS, nil
}

func intPointer(value int) *int {
	return &value
}

func insertTestTenant(t *testing.T, ctx context.Context, bunDB *bun.DB, slug string) int {
	t.Helper()
	tenant := &models.Tenant{Slug: slug, Name: slug}
	if _, err := bunDB.NewInsert().Model(tenant).Returning("id").Exec(ctx); err != nil {
		t.Fatalf("create test tenant %s: %v", slug, err)
	}
	return tenant.ID
}

func cleanupTestTenants(ctx context.Context, bunDB *bun.DB, ids ...int) {
	for _, id := range ids {
		_, _ = bunDB.NewDelete().Model((*models.Tenant)(nil)).Where("id = ?", id).Exec(ctx)
	}
}

func mustTx(t *testing.T, ctx context.Context, bunDB *bun.DB, tenantID int, fn func(context.Context, bun.Tx) error) {
	t.Helper()
	if err := db.RunInTenantTx(db.WithTenant(ctx, tenantID), bunDB, fn); err != nil {
		t.Fatalf("RunInTenantTx for tenant %d: %v", tenantID, err)
	}
}
