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
	var testCaseID int
	var modelCallID int
	mustTx(t, ctx, bunDB, tenantA, func(ctx context.Context, tx bun.Tx) error {
		project := &models.Project{TenantID: tenantA, Name: "rls-test-a"}
		if _, err := tx.NewInsert().Model(project).Returning("id").Exec(ctx); err != nil {
			return err
		}

		document := &models.Document{
			TenantID:  tenantA,
			ProjectID: project.ID,
			Name:      "rls-doc",
			Type:      "markdown",
			Source:    "upload",
			Content:   "# doc",
			Status:    models.DocumentStatusCompleted,
		}
		if _, err := tx.NewInsert().Model(document).Returning("id").Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(&models.DocumentChunk{
			TenantID:     tenantA,
			DocumentID:   document.ID,
			Content:      "chunk",
			ParentDocID:  document.ID,
			IndexProfile: "rls-profile",
			IndexVersion: "rls-version",
		}).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(&models.KnowledgeBase{
			TenantID:     tenantA,
			Type:         "module",
			Name:         "rls-knowledge",
			Content:      "knowledge",
			Status:       models.KnowledgeStatusCompleted,
			IndexProfile: "rls-profile",
			IndexVersion: "rls-version",
		}).Exec(ctx); err != nil {
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

		testCase := &models.TestCase{
			TenantID: tenantA,
			TaskID:   taskID,
			Section:  "rls-section",
			Cases: []map[string]any{
				{"title": "rls generated case"},
			},
			Status: models.TestCaseStatusDraft,
			SourceContext: map[string]any{
				"model_calls": []map[string]any{
					{"id": 0, "status": models.WorkflowStatusSucceeded, "prompt_id": "rls_prompt", "prompt_version": "v1"},
				},
			},
		}
		if _, err := tx.NewInsert().Model(testCase).Returning("id").Exec(ctx); err != nil {
			return err
		}
		testCaseID = testCase.ID

		if _, err := tx.NewInsert().Model(&models.BackgroundJob{
			TenantID:   tenantA,
			TaskID:     intPointer(taskID),
			JobType:    models.JobTypeGenerate,
			Status:     models.JobStatusPending,
			MaxRetries: 1,
		}).Exec(ctx); err != nil {
			return err
		}

		workflowRun := &models.WorkflowRun{
			TenantID:     tenantA,
			WorkflowType: models.JobTypeGenerate,
			ResourceType: "task",
			ResourceID:   taskID,
			Status:       models.WorkflowStatusRunning,
		}
		if _, err := tx.NewInsert().Model(workflowRun).Returning("id").Exec(ctx); err != nil {
			return err
		}
		modelCall := &models.ModelCall{
			TenantID:      tenantA,
			WorkflowRunID: intPointer(workflowRun.ID),
			Provider:      "fake",
			Model:         "valid_json",
			Status:        models.WorkflowStatusSucceeded,
			PromptChars:   120,
			ResponseChars: 240,
			Metadata: map[string]any{
				"prompt_id":      "rls_prompt",
				"prompt_version": "v1",
			},
		}
		if _, err := tx.NewInsert().Model(modelCall).Returning("id").Exec(ctx); err != nil {
			return err
		}
		modelCallID = modelCall.ID

		_, err := tx.NewInsert().Model(&models.TestCaseFeedback{
			TenantID:             tenantA,
			TaskID:               taskID,
			TestCaseID:           testCaseID,
			CaseIndex:            0,
			CaseTitle:            "rls generated case",
			FeedbackType:         models.CaseFeedbackUseful,
			Note:                 "visible to tenant A only",
			SourceContextSummary: map[string]any{"model_call_count": 1},
			PromptID:             "rls_prompt",
			PromptVersion:        "v1",
			ModelCallID:          intPointer(modelCallID),
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
		count, err = tx.NewSelect().Model((*models.BackgroundJob)(nil)).Count(ctx)
		return err
	})
	if count != 0 {
		t.Fatalf("tenant B saw %d background jobs under RLS; expected 0", count)
	}

	mustTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		count, err = tx.NewSelect().Model((*models.DocumentChunk)(nil)).Count(ctx)
		return err
	})
	if count != 0 {
		t.Fatalf("tenant B saw %d document chunks under RLS; expected 0", count)
	}

	mustTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		count, err = tx.NewSelect().Model((*models.KnowledgeBase)(nil)).Count(ctx)
		return err
	})
	if count != 0 {
		t.Fatalf("tenant B saw %d knowledge rows under RLS; expected 0", count)
	}

	mustTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		count, err = tx.NewSelect().Model((*models.WorkflowRun)(nil)).Count(ctx)
		return err
	})
	if count != 0 {
		t.Fatalf("tenant B saw %d workflow runs under RLS; expected 0", count)
	}

	mustTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		count, err = tx.NewSelect().Model((*models.TestCaseFeedback)(nil)).Count(ctx)
		return err
	})
	if count != 0 {
		t.Fatalf("tenant B saw %d case feedback rows under RLS; expected 0", count)
	}

	crossErr := db.RunInTenantTx(db.WithTenant(ctx, tenantB), bunDB, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(&models.Project{TenantID: tenantA, Name: "should-fail"}).Exec(ctx)
		return err
	})
	if crossErr == nil {
		t.Fatal("cross-tenant INSERT succeeded; WITH CHECK should have blocked it")
	}

	crossJobErr := db.RunInTenantTx(db.WithTenant(ctx, tenantB), bunDB, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(&models.BackgroundJob{
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

	crossWorkflowErr := db.RunInTenantTx(db.WithTenant(ctx, tenantB), bunDB, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(&models.WorkflowRun{
			TenantID:     tenantA,
			WorkflowType: models.JobTypeGenerate,
			ResourceType: "task",
			ResourceID:   taskID,
			Status:       models.WorkflowStatusRunning,
		}).Exec(ctx)
		return err
	})
	if crossWorkflowErr == nil {
		t.Fatal("cross-tenant workflow INSERT succeeded; WITH CHECK should have blocked it")
	}

	crossFeedbackErr := db.RunInTenantTx(db.WithTenant(ctx, tenantB), bunDB, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(&models.TestCaseFeedback{
			TenantID:             tenantA,
			TaskID:               taskID,
			TestCaseID:           testCaseID,
			CaseIndex:            0,
			FeedbackType:         models.CaseFeedbackDuplicate,
			SourceContextSummary: map[string]any{},
			ModelCallID:          intPointer(modelCallID),
		}).Exec(ctx)
		return err
	})
	if crossFeedbackErr == nil {
		t.Fatal("cross-tenant feedback INSERT succeeded; WITH CHECK should have blocked it")
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
