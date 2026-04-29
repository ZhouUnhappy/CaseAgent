package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"caseagent/internal/db/models"
	taskservice "caseagent/internal/service/task"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type CreateTaskRequest struct {
	DocumentIDs []int `json:"document_ids" binding:"required"`
}

type ReviewAffectedRequest struct {
	AffectedProducts []string `json:"affected_products"`
	AffectedModules  []string `json:"affected_modules"`
}

func (h *Handler) CreateGenerationTask(c *gin.Context) {
	projectID := c.Param("id")
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pid, err := strconv.Atoi(projectID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	documentIDs := dedupeInts(req.DocumentIDs)
	if len(documentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one document is required"})
		return
	}

	if err := validateTaskDocuments(c, h.DB, pid, documentIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := &models.CaseGenerationTask{
		ProjectID:   pid,
		DocumentIDs: documentIDs,
		Status:      models.TaskStatusAnalyzing,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err = h.DB.NewInsert().Model(task).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	go func(taskID int) {
		ctx := context.Background()
		svc := taskservice.New(h.DB)
		if err := svc.AnalyzeTask(ctx, taskID); err != nil {
			fmt.Printf("Failed to analyze task %d: %v\n", taskID, err)
		}
	}(task.ID)

	c.JSON(http.StatusCreated, task)
}

func (h *Handler) ListTasks(c *gin.Context) {
	projectID := c.Param("id")
	pid, err := strconv.Atoi(projectID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	var tasks []models.CaseGenerationTask
	err = h.DB.NewSelect().Model(&tasks).Where("project_id = ?", pid).Order("created_at DESC").Scan(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tasks)
}

func (h *Handler) GetTask(c *gin.Context) {
	id := c.Param("id")
	task := &models.CaseGenerationTask{ID: 0}

	err := h.DB.NewSelect().Model(task).Where("id = ?", id).Scan(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	c.JSON(http.StatusOK, task)
}

func (h *Handler) ReviewAffected(c *gin.Context) {
	id := c.Param("id")
	var req ReviewAffectedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := &models.CaseGenerationTask{ID: 0}
	err := h.DB.NewSelect().Model(task).Where("id = ?", id).Scan(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if !canReviewAffected(task.Status) {
		c.JSON(http.StatusConflict, gin.H{
			"error": fmt.Sprintf("task status %q does not allow affected-scope review", task.Status),
		})
		return
	}

	task.AffectedProducts = req.AffectedProducts
	task.AffectedModules = req.AffectedModules
	task.Status = models.TaskStatusReadyToGenerate
	task.UpdatedAt = time.Now()

	_, err = h.DB.NewUpdate().Model(task).Where("id = ?", id).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

func (h *Handler) GenerateCases(c *gin.Context) {
	id := c.Param("id")
	task := &models.CaseGenerationTask{ID: 0}

	err := h.DB.NewSelect().Model(task).Where("id = ?", id).Scan(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if !canStartGeneration(task.Status) {
		c.JSON(http.StatusConflict, gin.H{
			"error": fmt.Sprintf("task status %q does not allow generation", task.Status),
		})
		return
	}

	task.Status = models.TaskStatusGenerating
	task.UpdatedAt = time.Now()

	updateResult, err := h.DB.NewUpdate().
		Model(task).
		Where("id = ?", id).
		Where("status = ?", models.TaskStatusReadyToGenerate).
		Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if affected, _ := updateResult.RowsAffected(); affected == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "task status has changed, please retry"})
		return
	}

	go func(taskID int) {
		ctx := context.Background()
		svc := taskservice.New(h.DB)
		if err := svc.GenerateCases(ctx, taskID); err != nil {
			fmt.Printf("Failed to generate cases for task %d: %v\n", taskID, err)
		}
	}(task.ID)

	c.JSON(http.StatusOK, task)
}

func validateTaskDocuments(ctx context.Context, db *bun.DB, projectID int, documentIDs []int) error {
	projectCount, err := db.NewSelect().Model((*models.Project)(nil)).Where("id = ?", projectID).Count(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify project: %w", err)
	}
	if projectCount == 0 {
		return fmt.Errorf("project not found")
	}

	var documents []models.Document
	if err := db.NewSelect().
		Model(&documents).
		Where("project_id = ?", projectID).
		Where("id IN (?)", bun.In(documentIDs)).
		Scan(ctx); err != nil {
		return fmt.Errorf("failed to verify documents: %w", err)
	}

	if len(documents) != len(documentIDs) {
		return fmt.Errorf("some documents do not belong to the project")
	}

	for _, document := range documents {
		if document.Status != models.DocumentStatusCompleted {
			return fmt.Errorf("document %d is not ready, current status: %s", document.ID, document.Status)
		}
	}

	return nil
}

func dedupeInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func canReviewAffected(status string) bool {
	switch status {
	case models.TaskStatusAwaitingReview, models.TaskStatusReadyToGenerate:
		return true
	default:
		return false
	}
}

func canStartGeneration(status string) bool {
	return status == models.TaskStatusReadyToGenerate
}
