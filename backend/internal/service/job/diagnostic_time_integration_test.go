package job

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"caseagent/internal/clock"
	tenantdb "caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestTaskJobDiagnosticTimestampsRemainOrdered(t *testing.T) {
	dsn := os.Getenv("CASEAGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("set CASEAGENT_TEST_DSN to run diagnostic timestamp integration test")
	}

	ctx := context.Background()
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	t.Cleanup(func() { _ = sqldb.Close() })
	bunDB := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = bunDB.Close() })

	tenant := &models.Tenant{
		Slug: fmt.Sprintf("diagnostic-time-%d", time.Now().UnixNano()),
		Name: "diagnostic time integration",
	}
	if _, err := bunDB.NewInsert().Model(tenant).Returning("id").Exec(ctx); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.Tenant)(nil)).Where("id = ?", tenant.ID).Exec(ctx)
	})

	tenantCtx := tenantdb.WithTenant(ctx, tenant.ID)
	var taskID int
	var jobID int
	if err := tenantdb.RunInTenantTx(tenantCtx, bunDB, func(ctx context.Context, tx bun.Tx) error {
		project := &models.Project{TenantID: tenant.ID, Name: "diagnostic time project"}
		if _, err := tx.NewInsert().Model(project).Returning("id").Exec(ctx); err != nil {
			return err
		}
		now := clock.Now()
		task := &models.CaseGenerationTask{
			TenantID:    tenant.ID,
			ProjectID:   project.ID,
			DocumentIDs: []int{},
			Status:      models.TaskStatusAnalyzing,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if _, err := tx.NewInsert().Model(task).Returning("id").Exec(ctx); err != nil {
			return err
		}
		queued, err := New(tx).Enqueue(ctx, EnqueueInput{TaskID: task.ID, JobType: models.JobTypeAnalyze})
		if err != nil {
			return err
		}
		taskID = task.ID
		jobID = queued.ID
		return nil
	}); err != nil {
		t.Fatalf("create diagnostic fixture: %v", err)
	}

	store := NewBunStore(bunDB)
	claimed, err := store.ClaimNext(ctx, tenant.ID, models.JobTypeAnalyze)
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	if claimed == nil || claimed.ID != jobID {
		t.Fatalf("claimed job = %#v, want id %d", claimed, jobID)
	}
	if err := store.MarkSucceeded(ctx, tenant.ID, jobID); err != nil {
		t.Fatalf("mark job succeeded: %v", err)
	}

	var task models.CaseGenerationTask
	var completed models.BackgroundJob
	if err := tenantdb.RunInTenantTx(tenantCtx, bunDB, func(ctx context.Context, tx bun.Tx) error {
		if err := tx.NewSelect().Model(&task).Where("id = ?", taskID).Scan(ctx); err != nil {
			return err
		}
		return tx.NewSelect().Model(&completed).Where("id = ?", jobID).Scan(ctx)
	}); err != nil {
		t.Fatalf("load completed fixture: %v", err)
	}
	if completed.StartedAt == nil || completed.FinishedAt == nil {
		t.Fatalf("completed job timestamps missing: started=%v finished=%v", completed.StartedAt, completed.FinishedAt)
	}

	timeline := []struct {
		name string
		at   time.Time
	}{
		{name: "task.created_at", at: task.CreatedAt},
		{name: "job.created_at", at: completed.CreatedAt},
		{name: "job.started_at", at: *completed.StartedAt},
		{name: "job.finished_at", at: *completed.FinishedAt},
		{name: "task.updated_at", at: task.UpdatedAt},
	}
	for index := 1; index < len(timeline); index++ {
		if timeline[index].at.Before(timeline[index-1].at) {
			t.Fatalf("diagnostic timeline out of order: %s=%s is before %s=%s", timeline[index].name, timeline[index].at, timeline[index-1].name, timeline[index-1].at)
		}
	}
}
