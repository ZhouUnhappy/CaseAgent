package suggestion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestManualSuggestionIntegration(t *testing.T) {
	bunDB := openIntegrationDB(t)
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantA := insertTenant(t, ctx, bunDB, "suggestion-a-"+suffix)
	tenantB := insertTenant(t, ctx, bunDB, "suggestion-b-"+suffix)
	t.Cleanup(func() { cleanupTenants(ctx, bunDB, tenantA, tenantB) })

	var taskID, testCaseID, knowledgeID int
	mustTenantTx(t, ctx, bunDB, tenantA, func(ctx context.Context, tx bun.Tx) error {
		project := &models.Project{TenantID: tenantA, Name: "Suggestion Project"}
		if _, err := tx.NewInsert().Model(project).Returning("id").Exec(ctx); err != nil {
			return err
		}
		task := &models.CaseGenerationTask{
			TenantID:    tenantA,
			ProjectID:   project.ID,
			DocumentIDs: []int{},
			Status:      models.TaskStatusCompleted,
		}
		if _, err := tx.NewInsert().Model(task).Returning("id").Exec(ctx); err != nil {
			return err
		}
		tc := &models.TestCase{
			TenantID: tenantA,
			TaskID:   task.ID,
			Section:  "发票核对模块",
			Cases: []map[string]any{{
				"title": "核对发票差异明细",
			}},
			Status: models.TestCaseStatusDraft,
		}
		if _, err := tx.NewInsert().Model(tc).Returning("id").Exec(ctx); err != nil {
			return err
		}
		kb := &models.KnowledgeBase{
			TenantID: tenantA,
			Type:     models.SuggestionCandidateModule,
			Name:     "发票核对模块",
			Content:  "发票核对模块知识",
			Status:   models.KnowledgeStatusCompleted,
		}
		if _, err := tx.NewInsert().Model(kb).Returning("id").Exec(ctx); err != nil {
			return err
		}

		taskID = task.ID
		testCaseID = tc.ID
		knowledgeID = kb.ID
		return nil
	})

	var foreignKnowledgeID int
	mustTenantTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		kb := &models.KnowledgeBase{
			TenantID: tenantB,
			Type:     models.SuggestionCandidateModule,
			Name:     "Other Tenant Module",
			Content:  "foreign",
			Status:   models.KnowledgeStatusCompleted,
		}
		if _, err := tx.NewInsert().Model(kb).Returning("id").Exec(ctx); err != nil {
			return err
		}
		foreignKnowledgeID = kb.ID
		return nil
	})

	var suggestionID int
	mustTenantTx(t, ctx, bunDB, tenantA, func(ctx context.Context, tx bun.Tx) error {
		created, err := New(tx).CreateManual(ctx, ManualSuggestionInput{
			CandidateType: models.SuggestionCandidateModule,
			CandidateName: "发票核对模块",
			SourceTaskID:  taskID,
			SourceCaseID:  testCaseID,
			Note:          "这个模块缺少知识说明",
		})
		if err != nil {
			return err
		}
		suggestionID = created.ID

		if created.TenantID != tenantA {
			t.Fatalf("tenant_id: got %d want %d", created.TenantID, tenantA)
		}
		if created.SourceCaseID == nil || *created.SourceCaseID != testCaseID {
			t.Fatalf("source_case_id: got %#v want %d", created.SourceCaseID, testCaseID)
		}
		if created.Status != models.SuggestionStatusPending {
			t.Fatalf("status: got %q", created.Status)
		}
		if len(created.SourceSnippets) != 2 {
			t.Fatalf("source snippets: got %+v", created.SourceSnippets)
		}
		if created.SourceSnippets[0]["type"] != "case" || created.SourceSnippets[0]["title"] != "核对发票差异明细" {
			t.Fatalf("case snippet not populated from test case: %+v", created.SourceSnippets)
		}
		return nil
	})

	mustTenantTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		rows, err := New(tx).List(ctx, models.SuggestionStatusPending)
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatalf("tenant B saw tenant A suggestions: %+v", rows)
		}
		return nil
	})

	mustTenantTx(t, ctx, bunDB, tenantA, func(ctx context.Context, tx bun.Tx) error {
		_, _, err := New(tx).SetStatus(ctx, suggestionID, models.SuggestionStatusAdopted, &foreignKnowledgeID)
		if !errors.Is(err, ErrInvalidManualSuggestion) || !strings.Contains(err.Error(), "resolved_knowledge_id not found") {
			t.Fatalf("expected cross-tenant resolved knowledge to be rejected, got %v", err)
		}
		return nil
	})

	mustTenantTx(t, ctx, bunDB, tenantA, func(ctx context.Context, tx bun.Tx) error {
		updated, ok, err := New(tx).SetStatus(ctx, suggestionID, models.SuggestionStatusAdopted, &knowledgeID)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("expected pending suggestion to transition to adopted")
		}
		if updated.ResolvedKnowledgeID == nil || *updated.ResolvedKnowledgeID != knowledgeID {
			t.Fatalf("resolved_knowledge_id: got %#v want %d", updated.ResolvedKnowledgeID, knowledgeID)
		}

		stored := &models.KnowledgeUpdateSuggestionGroup{}
		if err := tx.NewSelect().Model(stored).Where("id = ?", suggestionID).Scan(ctx); err != nil {
			return err
		}
		if stored.Status != models.SuggestionStatusAdopted {
			t.Fatalf("stored status: got %q", stored.Status)
		}
		if stored.ResolvedKnowledgeID == nil || *stored.ResolvedKnowledgeID != knowledgeID {
			t.Fatalf("stored resolved_knowledge_id: got %#v want %d", stored.ResolvedKnowledgeID, knowledgeID)
		}
		return nil
	})
}

func TestExpiredPendingCleanupIntegration(t *testing.T) {
	bunDB := openIntegrationDB(t)
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantA := insertTenant(t, ctx, bunDB, "expiry-a-"+suffix)
	tenantB := insertTenant(t, ctx, bunDB, "expiry-b-"+suffix)
	t.Cleanup(func() { cleanupTenants(ctx, bunDB, tenantA, tenantB) })

	var oldPendingA, freshPendingA, oldAdoptedA, oldPendingB int
	mustTenantTx(t, ctx, bunDB, tenantA, func(ctx context.Context, tx bun.Tx) error {
		taskID, err := insertSuggestionFixtureTask(ctx, tx, tenantA)
		if err != nil {
			return err
		}
		oldPendingA, err = insertSuggestionGroup(ctx, tx, tenantA, taskID, models.SuggestionStatusPending, time.Now().Add(-31*24*time.Hour))
		if err != nil {
			return err
		}
		freshPendingA, err = insertSuggestionGroup(ctx, tx, tenantA, taskID, models.SuggestionStatusPending, time.Now().Add(-29*24*time.Hour))
		if err != nil {
			return err
		}
		oldAdoptedA, err = insertSuggestionGroup(ctx, tx, tenantA, taskID, models.SuggestionStatusAdopted, time.Now().Add(-60*24*time.Hour))
		return err
	})
	mustTenantTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		taskID, err := insertSuggestionFixtureTask(ctx, tx, tenantB)
		if err != nil {
			return err
		}
		oldPendingB, err = insertSuggestionGroup(ctx, tx, tenantB, taskID, models.SuggestionStatusPending, time.Now().Add(-45*24*time.Hour))
		return err
	})

	if err := cleanupExpiredPendingForAllTenants(ctx, bunDB, 30*24*time.Hour); err != nil {
		t.Fatalf("cleanup expired pending suggestions: %v", err)
	}

	mustTenantTx(t, ctx, bunDB, tenantA, func(ctx context.Context, tx bun.Tx) error {
		assertSuggestionState(t, ctx, tx, oldPendingA, models.SuggestionStatusDismissed, AutoExpiredDismissedReason)
		assertSuggestionState(t, ctx, tx, freshPendingA, models.SuggestionStatusPending, "")
		assertSuggestionState(t, ctx, tx, oldAdoptedA, models.SuggestionStatusAdopted, "")
		return nil
	})
	mustTenantTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		assertSuggestionState(t, ctx, tx, oldPendingB, models.SuggestionStatusDismissed, AutoExpiredDismissedReason)
		return nil
	})
}

func TestSuggestionGroupAggregationIntegration(t *testing.T) {
	bunDB := openIntegrationDB(t)
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantA := insertTenant(t, ctx, bunDB, "aggregation-a-"+suffix)
	tenantB := insertTenant(t, ctx, bunDB, "aggregation-b-"+suffix)
	t.Cleanup(func() { cleanupTenants(ctx, bunDB, tenantA, tenantB) })

	mustTenantTx(t, ctx, bunDB, tenantA, func(ctx context.Context, tx bun.Tx) error {
		task1, err := insertSuggestionFixtureTask(ctx, tx, tenantA)
		if err != nil {
			return err
		}
		task2, err := insertSuggestionFixtureTask(ctx, tx, tenantA)
		if err != nil {
			return err
		}

		service := New(tx)
		first, created, err := service.recordOccurrence(ctx, occurrenceInput{
			CandidateType:  models.SuggestionCandidateModule,
			CandidateName:  "Billing-Core",
			SourceTaskID:   task1,
			Frequency:      2,
			SourceSnippets: []map[string]any{{"text": "first task"}},
		})
		if err != nil {
			return err
		}
		if !created || first.TotalFrequency != 2 || first.TaskCount != 1 {
			t.Fatalf("first occurrence: created=%v view=%+v", created, first)
		}

		duplicate, created, err := service.recordOccurrence(ctx, occurrenceInput{
			CandidateType:  models.SuggestionCandidateModule,
			CandidateName:  "Billing-Core",
			SourceTaskID:   task1,
			Frequency:      7,
			SourceSnippets: []map[string]any{{"text": "duplicate"}},
		})
		if err != nil {
			return err
		}
		if created || duplicate.TotalFrequency != 2 || duplicate.TaskCount != 1 {
			t.Fatalf("duplicate occurrence should not change counters: created=%v view=%+v", created, duplicate)
		}

		second, created, err := service.recordOccurrence(ctx, occurrenceInput{
			CandidateType:  models.SuggestionCandidateModule,
			CandidateName:  "Billing-Core",
			SourceTaskID:   task2,
			Frequency:      3,
			SourceSnippets: []map[string]any{{"text": "second task"}},
		})
		if err != nil {
			return err
		}
		if !created || second.ID != first.ID || second.TotalFrequency != 5 || second.TaskCount != 2 {
			t.Fatalf("second occurrence should aggregate into same group: created=%v first=%+v second=%+v", created, first, second)
		}
		if len(second.Occurrences) != 2 {
			t.Fatalf("occurrences: got %d want 2", len(second.Occurrences))
		}

		_, created, err = service.recordOccurrence(ctx, occurrenceInput{
			CandidateType:  models.SuggestionCandidateModule,
			CandidateName:  "One-Off",
			SourceTaskID:   task1,
			Frequency:      9,
			SourceSnippets: []map[string]any{{"text": "high frequency single task"}},
		})
		if err != nil {
			return err
		}
		if !created {
			t.Fatal("one-off occurrence should be created")
		}

		rows, err := service.List(ctx, models.SuggestionStatusPending)
		if err != nil {
			return err
		}
		if len(rows) < 2 {
			t.Fatalf("suggestion rows: got %d want at least 2", len(rows))
		}
		if rows[0].CandidateName != "Billing-Core" {
			t.Fatalf("task_count should outrank total_frequency: first row=%+v", rows[0])
		}
		if rows[0].SourceTaskID != task2 {
			t.Fatalf("latest occurrence should populate source_task_id: got %d want %d", rows[0].SourceTaskID, task2)
		}
		return nil
	})

	mustTenantTx(t, ctx, bunDB, tenantB, func(ctx context.Context, tx bun.Tx) error {
		rows, err := New(tx).List(ctx, models.SuggestionStatusPending)
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatalf("tenant B saw tenant A aggregation rows: %+v", rows)
		}
		return nil
	})
}

func openIntegrationDB(t *testing.T) *bun.DB {
	t.Helper()
	dsn := os.Getenv("CASEAGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("set CASEAGENT_TEST_DSN to run suggestion integration tests")
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	t.Cleanup(func() { _ = sqldb.Close() })
	bunDB := bun.NewDB(sqldb, pgdialect.New())

	if bypass, err := roleBypassesRLS(context.Background(), bunDB); err != nil {
		t.Fatalf("inspect role attributes: %v", err)
	} else if bypass {
		t.Skip("CASEAGENT_TEST_DSN role bypasses RLS (superuser or BYPASSRLS); use a NOBYPASSRLS role")
	}

	return bunDB
}

func roleBypassesRLS(ctx context.Context, db *bun.DB) (bool, error) {
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

func insertTenant(t *testing.T, ctx context.Context, db *bun.DB, slug string) int {
	t.Helper()
	tenant := &models.Tenant{Slug: slug, Name: slug}
	if _, err := db.NewInsert().Model(tenant).Returning("id").Exec(ctx); err != nil {
		t.Fatalf("create tenant %s: %v", slug, err)
	}
	return tenant.ID
}

func cleanupTenants(ctx context.Context, db *bun.DB, ids ...int) {
	for _, id := range ids {
		_, _ = db.NewDelete().Model((*models.Tenant)(nil)).Where("id = ?", id).Exec(ctx)
	}
}

func mustTenantTx(t *testing.T, ctx context.Context, bunDB *bun.DB, tenantID int, fn func(context.Context, bun.Tx) error) {
	t.Helper()
	if err := db.RunInTenantTx(db.WithTenant(ctx, tenantID), bunDB, fn); err != nil {
		t.Fatalf("RunInTenantTx for tenant %d: %v", tenantID, err)
	}
}

func insertSuggestionFixtureTask(ctx context.Context, tx bun.Tx, tenantID int) (int, error) {
	project := &models.Project{TenantID: tenantID, Name: "Expiry Project"}
	if _, err := tx.NewInsert().Model(project).Returning("id").Exec(ctx); err != nil {
		return 0, err
	}
	task := &models.CaseGenerationTask{
		TenantID:    tenantID,
		ProjectID:   project.ID,
		DocumentIDs: []int{},
		Status:      models.TaskStatusCompleted,
	}
	if _, err := tx.NewInsert().Model(task).Returning("id").Exec(ctx); err != nil {
		return 0, err
	}
	return task.ID, nil
}

func insertSuggestionGroup(ctx context.Context, tx bun.Tx, tenantID int, taskID int, status string, createdAt time.Time) (int, error) {
	row := &models.KnowledgeUpdateSuggestionGroup{
		TenantID:       tenantID,
		CandidateType:  models.SuggestionCandidateModule,
		CandidateName:  fmt.Sprintf("expiry-candidate-%d", createdAt.UnixNano()),
		TotalFrequency: 1,
		TaskCount:      1,
		Status:         status,
		FirstSeenAt:    createdAt,
		LastSeenAt:     createdAt,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	if _, err := tx.NewInsert().Model(row).Returning("id").Exec(ctx); err != nil {
		return 0, err
	}
	occurrence := &models.KnowledgeUpdateSuggestionOccurrence{
		TenantID:     tenantID,
		GroupID:      row.ID,
		SourceTaskID: taskID,
		Frequency:    1,
		CreatedAt:    createdAt,
	}
	if _, err := tx.NewInsert().Model(occurrence).Exec(ctx); err != nil {
		return 0, err
	}
	return row.ID, nil
}

func assertSuggestionState(t *testing.T, ctx context.Context, tx bun.Tx, id int, status string, dismissedReason string) {
	t.Helper()
	row := &models.KnowledgeUpdateSuggestionGroup{}
	if err := tx.NewSelect().Model(row).Where("id = ?", id).Scan(ctx); err != nil {
		t.Fatalf("load suggestion %d: %v", id, err)
	}
	if row.Status != status {
		t.Fatalf("suggestion %d status: got %q want %q", id, row.Status, status)
	}
	if dismissedReason == "" {
		if row.DismissedReason != nil {
			t.Fatalf("suggestion %d dismissed_reason: got %q want nil", id, *row.DismissedReason)
		}
		return
	}
	if row.DismissedReason == nil || *row.DismissedReason != dismissedReason {
		t.Fatalf("suggestion %d dismissed_reason: got %#v want %q", id, row.DismissedReason, dismissedReason)
	}
}
