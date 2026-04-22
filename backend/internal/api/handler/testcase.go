package handler

import (
	"net/http"
	"strconv"
	"time"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
)

type UpdateTestCaseRequest struct {
	Section string `json:"section"`
	Cases   string `json:"cases"` // JSON string
}

func (h *Handler) ListTestCases(c *gin.Context) {
	taskID := c.Param("id")
	tid, err := strconv.Atoi(taskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var testCases []models.TestCase
	err = h.DB.NewSelect().Model(&testCases).Where("task_id = ?", tid).Order("created_at DESC").Scan(c)
	if err != nil {
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
	err := h.DB.NewSelect().Model(tc).Where("id = ?", id).Scan(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test case not found"})
		return
	}

	if req.Section != "" {
		tc.Section = req.Section
	}
	if req.Cases != "" {
		tc.Cases = req.Cases
	}
	tc.UpdatedAt = time.Now()

	_, err = h.DB.NewUpdate().Model(tc).Where("id = ?", id).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tc)
}

func (h *Handler) SubmitTestCase(c *gin.Context) {
	id := c.Param("case_id")
	tc := &models.TestCase{ID: 0}

	err := h.DB.NewSelect().Model(tc).Where("id = ?", id).Scan(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test case not found"})
		return
	}

	tc.Status = models.TestCaseStatusSubmitted
	tc.UpdatedAt = time.Now()

	_, err = h.DB.NewUpdate().Model(tc).Where("id = ?", id).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tc)
}
