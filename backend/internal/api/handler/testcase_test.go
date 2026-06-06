package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"caseagent/internal/api/middleware"
	tenantdb "caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestBatchTestCaseHandlers(t *testing.T) {
	dsn := os.Getenv("CASEAGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("set CASEAGENT_TEST_DSN to run test case handler integration test")
	}

	ctx := context.Background()
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	t.Cleanup(func() { _ = sqldb.Close() })
	bunDB := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = bunDB.Close() })

	suffix := time.Now().UnixNano()
	tenantA := insertCaseTestTenant(t, ctx, bunDB, fmt.Sprintf("case-batch-a-%d", suffix))
	tenantB := insertCaseTestTenant(t, ctx, bunDB, fmt.Sprintf("case-batch-b-%d", suffix))
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.Tenant)(nil)).
			Where("id IN (?)", bun.In([]int{tenantA.ID, tenantB.ID})).
			Exec(ctx)
	})

	taskA, sectionA := insertCaseTestData(t, ctx, bunDB, tenantA.ID, "Checkout", []map[string]any{
		{
			"title":                  "open cart",
			"priority_id":            2,
			"affected_products":      []string{"Shop"},
			"affected_modules":       []string{"Cart"},
			"custom_steps_separated": []map[string]string{{"content": "open", "expected": "cart"}},
		},
		{
			"title":                  "pay order",
			"priority_id":            2,
			"affected_products":      []string{"Shop"},
			"affected_modules":       []string{"Order"},
			"custom_steps_separated": []map[string]string{{"content": "pay", "expected": "success"}},
		},
	})
	taskB, sectionB := insertCaseTestData(t, ctx, bunDB, tenantB.ID, "External", []map[string]any{
		{"title": "other tenant", "priority_id": 1},
	})

	router := caseTestRouter(bunDB)
	updated := caseRequest[[]models.TestCase](t, router, tenantA.Slug, http.MethodPut,
		fmt.Sprintf("/api/v1/tasks/%d/cases/batch", taskA.ID),
		map[string]any{
			"cases": []map[string]any{
				{"test_case_id": sectionA.ID, "case_index": 1},
			},
			"patch": map[string]any{
				"priority_id":       4,
				"affected_products": []string{"Payments"},
				"affected_modules":  []string{"Checkout"},
				"duplicate_hidden":  true,
			},
		},
		http.StatusOK,
	)
	if len(updated) != 1 || updated[0].ID != sectionA.ID {
		t.Fatalf("batch update response = %#v, want one updated section", updated)
	}
	changed := updated[0].Cases[1]
	if caseNumber(changed["priority_id"]) != 4 || changed["duplicate_hidden"] != true {
		t.Fatalf("patched row = %#v, want priority 4 and duplicate hidden", changed)
	}
	if got := caseStringSlice(changed["affected_products"]); len(got) != 1 || got[0] != "Payments" {
		t.Fatalf("affected_products = %#v", changed["affected_products"])
	}

	submitted := caseRequest[[]models.TestCase](t, router, tenantA.Slug, http.MethodPut,
		fmt.Sprintf("/api/v1/tasks/%d/cases/batch/submit", taskA.ID),
		map[string]any{"test_case_ids": []int{sectionA.ID}},
		http.StatusOK,
	)
	if len(submitted) != 1 || submitted[0].Status != models.TestCaseStatusSubmitted {
		t.Fatalf("batch submit response = %#v, want submitted section", submitted)
	}

	caseRequest[map[string]any](t, router, tenantA.Slug, http.MethodPut,
		fmt.Sprintf("/api/v1/tasks/%d/cases/batch/submit", taskB.ID),
		map[string]any{"test_case_ids": []int{sectionB.ID}},
		http.StatusNotFound,
	)
}

func caseTestRouter(db *bun.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := New(db)
	router := gin.New()
	biz := router.Group("/api/v1")
	biz.Use(middleware.Tenant(db), middleware.Tx(db))
	cases := biz.Group("/tasks/:id/cases")
	{
		cases.PUT("/batch", h.BatchUpdateTestCases)
		cases.PUT("/batch/submit", h.BatchSubmitTestCases)
	}
	return router
}

func insertCaseTestTenant(t *testing.T, ctx context.Context, db *bun.DB, slug string) *models.Tenant {
	t.Helper()
	tenant := &models.Tenant{Slug: slug, Name: slug}
	if _, err := db.NewInsert().Model(tenant).Returning("id").Exec(ctx); err != nil {
		t.Fatalf("insert tenant %q: %v", slug, err)
	}
	return tenant
}

func insertCaseTestData(t *testing.T, ctx context.Context, db *bun.DB, tenantID int, section string, cases []map[string]any) (*models.CaseGenerationTask, *models.TestCase) {
	t.Helper()
	var task *models.CaseGenerationTask
	var testcase *models.TestCase
	if err := tenantdb.RunInTenantTx(tenantdb.WithTenant(ctx, tenantID), db, func(ctx context.Context, tx bun.Tx) error {
		project := &models.Project{TenantID: tenantID, Name: section + " Project"}
		if _, err := tx.NewInsert().Model(project).Returning("id").Exec(ctx); err != nil {
			return err
		}
		task = &models.CaseGenerationTask{
			TenantID:    tenantID,
			ProjectID:   project.ID,
			DocumentIDs: []int{},
			Status:      models.TaskStatusCompleted,
		}
		if _, err := tx.NewInsert().Model(task).Returning("id").Exec(ctx); err != nil {
			return err
		}
		testcase = &models.TestCase{
			TenantID: tenantID,
			TaskID:   task.ID,
			Section:  section,
			Cases:    cases,
			Status:   models.TestCaseStatusDraft,
		}
		if _, err := tx.NewInsert().Model(testcase).Returning("id").Exec(ctx); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("insert case test data for tenant %d: %v", tenantID, err)
	}
	return task, testcase
}

func caseRequest[T any](t *testing.T, router *gin.Engine, tenantSlug string, method string, path string, payload any, wantStatus int) T {
	t.Helper()
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", tenantSlug)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	var out T
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s %s response: %v; body=%s", method, path, err, rec.Body.String())
	}
	return out
}

func caseNumber(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func caseStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func TestApplyBatchCasePatch(t *testing.T) {
	priority := 4
	duplicate := true
	products := []string{"Product-A"}
	modules := []string{"Module-B"}
	row := map[string]any{
		"title":             "old",
		"priority_id":       2,
		"affected_products": []string{"OldProduct"},
		"affected_modules":  []string{"OldModule"},
		"duplicate_hidden":  false,
		"custom_preconds":   "keep",
	}

	applyBatchCasePatch(row, BatchCasePatchRequest{
		PriorityID:       &priority,
		AffectedProducts: &products,
		AffectedModules:  &modules,
		DuplicateHidden:  &duplicate,
	})

	if row["priority_id"] != 4 || row["duplicate_hidden"] != true {
		t.Fatalf("scalar patch not applied: %#v", row)
	}
	gotProducts, ok := row["affected_products"].([]string)
	if !ok || len(gotProducts) != 1 || gotProducts[0] != "Product-A" {
		t.Fatalf("affected_products patch = %#v", row["affected_products"])
	}
	gotModules, ok := row["affected_modules"].([]string)
	if !ok || len(gotModules) != 1 || gotModules[0] != "Module-B" {
		t.Fatalf("affected_modules patch = %#v", row["affected_modules"])
	}
	if row["custom_preconds"] != "keep" {
		t.Fatalf("unexpected unrelated field change: %#v", row)
	}
	products[0] = "mutated"
	if row["affected_products"].([]string)[0] != "Product-A" {
		t.Fatal("patch should copy input slices")
	}
}

func TestBatchHelpersDedupeIDs(t *testing.T) {
	ids := uniqueTestCaseIDs([]CaseRefRequest{
		{TestCaseID: 2, CaseIndex: 0},
		{TestCaseID: 0, CaseIndex: 1},
		{TestCaseID: 2, CaseIndex: 2},
		{TestCaseID: 3, CaseIndex: 0},
	})
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 3 {
		t.Fatalf("uniqueTestCaseIDs() = %#v", ids)
	}
	ints := uniqueInts([]int{0, 5, 5, 6})
	if len(ints) != 2 || ints[0] != 5 || ints[1] != 6 {
		t.Fatalf("uniqueInts() = %#v", ints)
	}
}
