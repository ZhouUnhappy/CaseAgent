package handler

import (
	"net/http"
	"strconv"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

func (h *Handler) ListWorkflows(c *gin.Context) {
	query := DBFromContext(c).NewSelect().
		Model((*models.WorkflowRun)(nil)).
		Order("created_at DESC", "id DESC")

	if value := c.Query("workflow_type"); value != "" {
		query.Where("workflow_type = ?", value)
	}
	if value := c.Query("resource_type"); value != "" {
		query.Where("resource_type = ?", value)
	}
	if ok := applyWorkflowIntFilter(c, query, "resource_id"); !ok {
		return
	}
	if ok := applyWorkflowIntFilter(c, query, "job_id"); !ok {
		return
	}
	if status := c.Query("status"); status != "" {
		if ok := applyWorkflowStatusFilter(c, query, status); !ok {
			return
		}
	}

	var runs []models.WorkflowRun
	if err := query.Limit(200).Scan(c.Request.Context(), &runs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, runs)
}

func applyWorkflowIntFilter(c *gin.Context, query *bun.SelectQuery, field string) bool {
	raw := c.Query(field)
	if raw == "" {
		return true
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": field + " must be a positive integer"})
		return false
	}
	query.Where(field+" = ?", id)
	return true
}

func applyWorkflowStatusFilter(c *gin.Context, query *bun.SelectQuery, status string) bool {
	switch status {
	case models.WorkflowStatusPending,
		models.WorkflowStatusRunning,
		models.WorkflowStatusSucceeded,
		models.WorkflowStatusFailed,
		models.WorkflowStatusCanceled:
		query.Where("status = ?", status)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported workflow status"})
		return false
	}
	return true
}
