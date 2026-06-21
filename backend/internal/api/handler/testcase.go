package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type UpdateTestCaseRequest struct {
	Section string           `json:"section"`
	Cases   []map[string]any `json:"cases"`
}

type CaseRefRequest struct {
	TestCaseID int `json:"test_case_id"`
	CaseIndex  int `json:"case_index"`
}

type BatchCasePatchRequest struct {
	PriorityID       *int      `json:"priority_id"`
	AffectedProducts *[]string `json:"affected_products"`
	AffectedModules  *[]string `json:"affected_modules"`
	DuplicateHidden  *bool     `json:"duplicate_hidden"`
}

type BatchUpdateTestCasesRequest struct {
	Cases []CaseRefRequest      `json:"cases"`
	Patch BatchCasePatchRequest `json:"patch"`
}

type BatchSubmitTestCasesRequest struct {
	TestCaseIDs []int `json:"test_case_ids"`
}

func (h *Handler) ListTestCases(c *gin.Context) {
	taskID := c.Param("id")
	tid, err := strconv.Atoi(taskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var testCases []models.TestCase
	if err := DBFromContext(c).NewSelect().Model(&testCases).Where("task_id = ?", tid).Order("created_at DESC").Scan(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, testCases)
}

func (h *Handler) UpdateTestCase(c *gin.Context) {
	id := c.Param("case_id")
	var req UpdateTestCaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tc := &models.TestCase{ID: 0}
	if err := DBFromContext(c).NewSelect().Model(tc).Where("id = ?", id).Scan(c.Request.Context()); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test case not found"})
		return
	}

	if req.Section != "" {
		tc.Section = req.Section
	}
	if len(req.Cases) > 0 {
		if err := validateCaseRows(req.Cases); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		tc.Cases = req.Cases
	}
	tc.UpdatedAt = time.Now()

	if _, err := DBFromContext(c).NewUpdate().Model(tc).Where("id = ?", id).Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("testcase update",
		"task_id", tc.TaskID,
		"case_id", tc.ID,
		"section", tc.Section,
		"cases", len(tc.Cases),
	)

	c.JSON(http.StatusOK, tc)
}

func (h *Handler) BatchUpdateTestCases(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}
	var req BatchUpdateTestCasesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Cases) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cases is required"})
		return
	}
	if !hasBatchPatch(req.Patch) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "patch is required"})
		return
	}

	ids := uniqueTestCaseIDs(req.Cases)
	var sections []models.TestCase
	if err := DBFromContext(c).NewSelect().
		Model(&sections).
		Where("task_id = ?", taskID).
		Where("id IN (?)", bun.In(ids)).
		Order("id ASC").
		Scan(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	sectionsByID := make(map[int]*models.TestCase, len(sections))
	for idx := range sections {
		sectionsByID[sections[idx].ID] = &sections[idx]
	}

	changed := make(map[int]struct{})
	for _, item := range req.Cases {
		section := sectionsByID[item.TestCaseID]
		if section == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Test case not found"})
			return
		}
		if item.CaseIndex < 0 || item.CaseIndex >= len(section.Cases) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "case_index out of range"})
			return
		}
		applyBatchCasePatch(section.Cases[item.CaseIndex], req.Patch)
		if err := validateCaseRows(section.Cases); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		changed[section.ID] = struct{}{}
	}

	now := time.Now()
	updated := make([]models.TestCase, 0, len(changed))
	for idx := range sections {
		if _, ok := changed[sections[idx].ID]; !ok {
			continue
		}
		sections[idx].UpdatedAt = now
		if _, err := DBFromContext(c).NewUpdate().
			Model(&sections[idx]).
			Set("cases = ?", sections[idx].Cases).
			Set("updated_at = ?", now).
			Where("id = ?", sections[idx].ID).
			Exec(c.Request.Context()); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updated = append(updated, sections[idx])
	}
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) SubmitTestCase(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}
	id := c.Param("case_id")
	tc := &models.TestCase{ID: 0}

	if err := DBFromContext(c).NewSelect().Model(tc).
		Where("id = ?", id).
		Where("task_id = ?", taskID).
		Scan(c.Request.Context()); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test case not found"})
		return
	}
	if err := validateCaseRows(tc.Cases); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !requireReviewedCases(c, []models.TestCase{*tc}) {
		return
	}

	tc.Status = models.TestCaseStatusSubmitted
	tc.UpdatedAt = time.Now()

	if _, err := DBFromContext(c).NewUpdate().Model(tc).Where("id = ?", id).Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("testcase submit",
		"task_id", tc.TaskID,
		"case_id", tc.ID,
		"section", tc.Section,
	)

	c.JSON(http.StatusOK, tc)
}

func (h *Handler) BatchSubmitTestCases(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}
	var req BatchSubmitTestCasesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ids := uniqueInts(req.TestCaseIDs)
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "test_case_ids is required"})
		return
	}

	var updated []models.TestCase
	if err := DBFromContext(c).NewSelect().
		Model(&updated).
		Where("task_id = ?", taskID).
		Where("id IN (?)", bun.In(ids)).
		Order("id ASC").
		Scan(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(updated) != len(ids) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test case not found"})
		return
	}
	for _, section := range updated {
		if err := validateCaseRows(section.Cases); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if !requireReviewedCases(c, updated) {
		return
	}

	now := time.Now()
	if _, err := DBFromContext(c).NewUpdate().
		Model((*models.TestCase)(nil)).
		Set("status = ?", models.TestCaseStatusSubmitted).
		Set("updated_at = ?", now).
		Where("task_id = ?", taskID).
		Where("id IN (?)", bun.In(ids)).
		Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for idx := range updated {
		updated[idx].Status = models.TestCaseStatusSubmitted
		updated[idx].UpdatedAt = now
	}

	c.JSON(http.StatusOK, updated)
}

func requireReviewedCases(c *gin.Context, sections []models.TestCase) bool {
	pending, err := pendingCaseReviewCount(c.Request.Context(), DBFromContext(c), sections)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if pending > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":            fmt.Sprintf("%d test cases are pending review", pending),
			"unreviewed_count": pending,
		})
		return false
	}
	return true
}

func pendingCaseReviewCount(ctx context.Context, db bun.IDB, sections []models.TestCase) (int, error) {
	if len(sections) == 0 {
		return 0, nil
	}

	sectionIDs := make([]int, 0, len(sections))
	expected := make(map[int]int, len(sections))
	for _, section := range sections {
		sectionIDs = append(sectionIDs, section.ID)
		expected[section.ID] = len(section.Cases)
	}

	feedbackRows := []models.TestCaseFeedback{}
	if err := db.NewSelect().
		Model(&feedbackRows).
		Column("test_case_id", "case_index").
		Where("test_case_id IN (?)", bun.In(sectionIDs)).
		Scan(ctx); err != nil {
		return 0, err
	}

	reviewed := make(map[CaseRefRequest]struct{}, len(feedbackRows))
	for _, row := range feedbackRows {
		caseCount, ok := expected[row.TestCaseID]
		if !ok || row.CaseIndex < 0 || row.CaseIndex >= caseCount {
			continue
		}
		reviewed[CaseRefRequest{TestCaseID: row.TestCaseID, CaseIndex: row.CaseIndex}] = struct{}{}
	}

	pending := 0
	for sectionID, caseCount := range expected {
		for caseIndex := 0; caseIndex < caseCount; caseIndex++ {
			if _, ok := reviewed[CaseRefRequest{TestCaseID: sectionID, CaseIndex: caseIndex}]; !ok {
				pending++
			}
		}
	}
	return pending, nil
}

func hasBatchPatch(patch BatchCasePatchRequest) bool {
	return patch.PriorityID != nil ||
		patch.AffectedProducts != nil ||
		patch.AffectedModules != nil ||
		patch.DuplicateHidden != nil
}

func applyBatchCasePatch(row map[string]any, patch BatchCasePatchRequest) {
	if patch.PriorityID != nil {
		row["priority_id"] = *patch.PriorityID
	}
	if patch.AffectedProducts != nil {
		row["affected_products"] = append([]string{}, (*patch.AffectedProducts)...)
	}
	if patch.AffectedModules != nil {
		row["affected_modules"] = append([]string{}, (*patch.AffectedModules)...)
	}
	if patch.DuplicateHidden != nil {
		row["duplicate_hidden"] = *patch.DuplicateHidden
	}
}

func validateCaseRows(rows []map[string]any) error {
	if len(rows) == 0 {
		return fmt.Errorf("cases must contain at least one case")
	}
	failures := []string{}
	for idx, row := range rows {
		prefix := fmt.Sprintf("case[%d]", idx)
		if strings.TrimSpace(stringFromAny(row["title"])) == "" {
			failures = append(failures, prefix+".title is required")
		}
		priority := intFromAny(row["priority_id"])
		if priority < 1 || priority > 4 {
			failures = append(failures, prefix+".priority_id must be 1..4")
		}
		steps := mapsFromAny(row["custom_steps_separated"])
		validSteps := 0
		for stepIdx, step := range steps {
			content := strings.TrimSpace(stringFromAny(step["content"]))
			expected := strings.TrimSpace(stringFromAny(step["expected"]))
			if content == "" || expected == "" {
				failures = append(failures, fmt.Sprintf("%s.custom_steps_separated[%d] content and expected are required", prefix, stepIdx))
				continue
			}
			validSteps++
		}
		if validSteps == 0 {
			failures = append(failures, prefix+".custom_steps_separated must contain at least one complete step")
		}
		if len(stringsFromAny(row["affected_products"])) == 0 {
			failures = append(failures, prefix+".affected_products is required")
		}
		if len(stringsFromAny(row["affected_modules"])) == 0 {
			failures = append(failures, prefix+".affected_modules is required")
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("test case validation failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

func stringsFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return nonEmptyStrings(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, stringFromAny(item))
		}
		return nonEmptyStrings(values)
	default:
		return nil
	}
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func uniqueTestCaseIDs(items []CaseRefRequest) []int {
	ids := make([]int, 0, len(items))
	seen := map[int]struct{}{}
	for _, item := range items {
		if item.TestCaseID <= 0 {
			continue
		}
		if _, ok := seen[item.TestCaseID]; ok {
			continue
		}
		seen[item.TestCaseID] = struct{}{}
		ids = append(ids, item.TestCaseID)
	}
	return ids
}

func uniqueInts(values []int) []int {
	ids := make([]int, 0, len(values))
	seen := map[int]struct{}{}
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ids = append(ids, value)
	}
	return ids
}
