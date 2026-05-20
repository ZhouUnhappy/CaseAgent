package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
)

type UpdateTestCaseRequest struct {
	Section string           `json:"section"`
	Cases   []map[string]any `json:"cases"`
}

func (h *Handler) ListTestCases(c *gin.Context) {
	taskID := c.Param("id")
	tid, err := strconv.Atoi(taskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var testCases []models.TestCase
	if err := DBFromContext(c).NewSelect().Model(&testCases).Where("task_id = ?", tid).Order("created_at DESC").Scan(c); err != nil {
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
	if err := DBFromContext(c).NewSelect().Model(tc).Where("id = ?", id).Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test case not found"})
		return
	}

	if req.Section != "" {
		tc.Section = req.Section
	}
	if len(req.Cases) > 0 {
		tc.Cases = req.Cases
	}
	tc.UpdatedAt = time.Now()

	if _, err := DBFromContext(c).NewUpdate().Model(tc).Where("id = ?", id).Exec(c); err != nil {
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

func (h *Handler) SubmitTestCase(c *gin.Context) {
	id := c.Param("case_id")
	tc := &models.TestCase{ID: 0}

	if err := DBFromContext(c).NewSelect().Model(tc).Where("id = ?", id).Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test case not found"})
		return
	}

	tc.Status = models.TestCaseStatusSubmitted
	tc.UpdatedAt = time.Now()

	if _, err := DBFromContext(c).NewUpdate().Model(tc).Where("id = ?", id).Exec(c); err != nil {
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
