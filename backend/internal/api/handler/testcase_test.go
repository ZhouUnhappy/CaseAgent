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
	"strings"
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
	taskSingle, sectionSingle := insertCaseTestData(t, ctx, bunDB, tenantA.ID, "Profile", []map[string]any{
		{
			"title":                  "update profile",
			"priority_id":            2,
			"affected_products":      []string{"Shop"},
			"affected_modules":       []string{"Profile"},
			"custom_steps_separated": []map[string]string{{"content": "save", "expected": "updated"}},
		},
	})
	taskFeedback, sectionFeedback := insertCaseTestData(t, ctx, bunDB, tenantA.ID, "Batch Feedback", []map[string]any{
		{
			"title":                  "first selected case",
			"priority_id":            2,
			"affected_products":      []string{"Shop"},
			"affected_modules":       []string{"Review"},
			"custom_steps_separated": []map[string]string{{"content": "run", "expected": "pass"}},
		},
		{
			"title":                  "second selected case",
			"priority_id":            2,
			"affected_products":      []string{"Shop"},
			"affected_modules":       []string{"Review"},
			"custom_steps_separated": []map[string]string{{"content": "run", "expected": "pass"}},
		},
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

	rejectedBatch := caseRequest[map[string]any](t, router, tenantA.Slug, http.MethodPut,
		fmt.Sprintf("/api/v1/tasks/%d/cases/batch/submit", taskA.ID),
		map[string]any{"test_case_ids": []int{sectionA.ID}},
		http.StatusConflict,
	)
	if caseNumber(rejectedBatch["unreviewed_count"]) != len(sectionA.Cases) {
		t.Fatalf("batch submit rejection = %#v, want %d unreviewed cases", rejectedBatch, len(sectionA.Cases))
	}
	insertCaseTestFeedbacks(t, ctx, bunDB, tenantA.ID, taskA.ID, sectionA.ID, len(sectionA.Cases))
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

	rejectedSingle := caseRequest[map[string]any](t, router, tenantA.Slug, http.MethodPut,
		fmt.Sprintf("/api/v1/tasks/%d/cases/%d/submit", taskSingle.ID, sectionSingle.ID),
		nil,
		http.StatusConflict,
	)
	if caseNumber(rejectedSingle["unreviewed_count"]) != len(sectionSingle.Cases) {
		t.Fatalf("single submit rejection = %#v, want %d unreviewed cases", rejectedSingle, len(sectionSingle.Cases))
	}
	insertCaseTestFeedback(t, ctx, bunDB, tenantA.ID, taskSingle.ID, sectionSingle.ID, 0, models.CaseFeedbackMissingSteps)
	unresolvedSingle := caseRequest[map[string]any](t, router, tenantA.Slug, http.MethodPut,
		fmt.Sprintf("/api/v1/tasks/%d/cases/%d/submit", taskSingle.ID, sectionSingle.ID),
		nil,
		http.StatusConflict,
	)
	if caseNumber(unresolvedSingle["pending_count"]) != 0 || caseNumber(unresolvedSingle["unresolved_count"]) != 1 {
		t.Fatalf("single unresolved rejection = %#v, want unresolved_count=1", unresolvedSingle)
	}
	insertCaseTestFeedback(t, ctx, bunDB, tenantA.ID, taskSingle.ID, sectionSingle.ID, 0, models.CaseFeedbackUseful)
	singleSubmitted := caseRequest[models.TestCase](t, router, tenantA.Slug, http.MethodPut,
		fmt.Sprintf("/api/v1/tasks/%d/cases/%d/submit", taskSingle.ID, sectionSingle.ID),
		nil,
		http.StatusOK,
	)
	if singleSubmitted.Status != models.TestCaseStatusSubmitted {
		t.Fatalf("single submit response = %#v, want submitted section", singleSubmitted)
	}

	batchPayload := map[string]any{
		"cases": []map[string]any{
			{"test_case_id": sectionFeedback.ID, "case_index": 0},
			{"test_case_id": sectionFeedback.ID, "case_index": 1},
		},
		"feedback_type": models.CaseFeedbackUseful,
		"note":          "batch pass",
	}
	createdFeedback := caseRequest[[]models.TestCaseFeedback](t, router, tenantA.Slug, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/cases/batch/feedback", taskFeedback.ID),
		batchPayload,
		http.StatusCreated,
	)
	if len(createdFeedback) != len(sectionFeedback.Cases) {
		t.Fatalf("batch feedback response = %#v, want %d rows", createdFeedback, len(sectionFeedback.Cases))
	}
	caseRequest[[]models.TestCaseFeedback](t, router, tenantA.Slug, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/cases/batch/feedback", taskFeedback.ID),
		batchPayload,
		http.StatusCreated,
	)
	beforeInvalid := countCaseTestFeedback(t, ctx, bunDB, tenantA.ID, taskFeedback.ID)
	caseRequest[map[string]any](t, router, tenantA.Slug, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/cases/batch/feedback", taskFeedback.ID),
		map[string]any{
			"cases": []map[string]any{
				{"test_case_id": sectionFeedback.ID, "case_index": 0},
				{"test_case_id": sectionFeedback.ID, "case_index": 99},
			},
			"feedback_type": models.CaseFeedbackUseful,
		},
		http.StatusBadRequest,
	)
	if afterInvalid := countCaseTestFeedback(t, ctx, bunDB, tenantA.ID, taskFeedback.ID); afterInvalid != beforeInvalid {
		t.Fatalf("partial invalid batch inserted feedback: before=%d after=%d", beforeInvalid, afterInvalid)
	}
	caseRequest[map[string]any](t, router, tenantA.Slug, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/cases/batch/feedback", taskB.ID),
		map[string]any{
			"cases":         []map[string]any{{"test_case_id": sectionB.ID, "case_index": 0}},
			"feedback_type": models.CaseFeedbackUseful,
		},
		http.StatusNotFound,
	)
	feedbackSubmitted := caseRequest[[]models.TestCase](t, router, tenantA.Slug, http.MethodPut,
		fmt.Sprintf("/api/v1/tasks/%d/cases/batch/submit", taskFeedback.ID),
		map[string]any{"test_case_ids": []int{sectionFeedback.ID}},
		http.StatusOK,
	)
	if len(feedbackSubmitted) != 1 || feedbackSubmitted[0].Status != models.TestCaseStatusSubmitted {
		t.Fatalf("batch feedback section submit = %#v", feedbackSubmitted)
	}
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
		cases.POST("/batch/feedback", h.CreateBatchCaseFeedback)
		cases.PUT("/:case_id/submit", h.SubmitTestCase)
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

func insertCaseTestFeedbacks(t *testing.T, ctx context.Context, db *bun.DB, tenantID, taskID, testCaseID, caseCount int) {
	t.Helper()
	if err := tenantdb.RunInTenantTx(tenantdb.WithTenant(ctx, tenantID), db, func(ctx context.Context, tx bun.Tx) error {
		rows := make([]models.TestCaseFeedback, 0, caseCount)
		for caseIndex := 0; caseIndex < caseCount; caseIndex++ {
			rows = append(rows, models.TestCaseFeedback{
				TenantID:     tenantID,
				TaskID:       taskID,
				TestCaseID:   testCaseID,
				CaseIndex:    caseIndex,
				FeedbackType: models.CaseFeedbackUseful,
			})
		}
		_, err := tx.NewInsert().Model(&rows).Exec(ctx)
		return err
	}); err != nil {
		t.Fatalf("insert feedback for test case %d: %v", testCaseID, err)
	}
}

func insertCaseTestFeedback(t *testing.T, ctx context.Context, db *bun.DB, tenantID, taskID, testCaseID, caseIndex int, feedbackType string) {
	t.Helper()
	if err := tenantdb.RunInTenantTx(tenantdb.WithTenant(ctx, tenantID), db, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(&models.TestCaseFeedback{
			TenantID:     tenantID,
			TaskID:       taskID,
			TestCaseID:   testCaseID,
			CaseIndex:    caseIndex,
			FeedbackType: feedbackType,
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		}).Exec(ctx)
		return err
	}); err != nil {
		t.Fatalf("insert %s feedback for test case %d[%d]: %v", feedbackType, testCaseID, caseIndex, err)
	}
}

func countCaseTestFeedback(t *testing.T, ctx context.Context, db *bun.DB, tenantID, taskID int) int {
	t.Helper()
	count := 0
	if err := tenantdb.RunInTenantTx(tenantdb.WithTenant(ctx, tenantID), db, func(ctx context.Context, tx bun.Tx) error {
		var err error
		count, err = tx.NewSelect().Model((*models.TestCaseFeedback)(nil)).Where("task_id = ?", taskID).Count(ctx)
		return err
	}); err != nil {
		t.Fatalf("count feedback for task %d: %v", taskID, err)
	}
	return count
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

func TestValidateCaseRows(t *testing.T) {
	valid := []map[string]any{{
		"title":             "checkout succeeds",
		"priority_id":       3,
		"affected_products": []string{"Shop"},
		"affected_modules":  []string{"Checkout"},
		"custom_steps_separated": []map[string]any{
			{"content": "pay", "expected": "success"},
		},
	}}
	if err := validateCaseRows(valid); err != nil {
		t.Fatalf("validateCaseRows(valid) = %v", err)
	}

	invalid := []map[string]any{{
		"title":             " ",
		"priority_id":       9,
		"affected_products": []string{},
		"affected_modules":  []any{""},
		"custom_steps_separated": []any{
			map[string]any{"content": "click", "expected": ""},
		},
	}}
	err := validateCaseRows(invalid)
	if err == nil {
		t.Fatal("validateCaseRows(invalid) returned nil")
	}
	for _, want := range []string{"title is required", "priority_id", "custom_steps_separated", "affected_products", "affected_modules"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validateCaseRows(invalid) error = %v, want contains %q", err, want)
		}
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
