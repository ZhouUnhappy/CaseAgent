package handler

import (
	"context"
	"fmt"
	"log/slog"
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

	if err := validateTaskDocuments(c, DBFromContext(c), pid, documentIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID, _ := TenantIDFromContext(c)
	task := &models.CaseGenerationTask{
		TenantID:    tenantID,
		ProjectID:   pid,
		DocumentIDs: documentIDs,
		Status:      models.TaskStatusAnalyzing,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if _, err := DBFromContext(c).NewInsert().Model(task).Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("task created",
		"task_id", task.ID,
		"project_id", task.ProjectID,
		"document_ids", task.DocumentIDs,
	)

	taskID := task.ID
	RunAsync(h.DB, tenantID, func(ctx context.Context, tx bun.Tx) error {
		if err := taskservice.New(tx).AnalyzeTask(ctx, taskID); err != nil {
			slog.Error("task analyze failed", "task_id", taskID, "error", err)
			return err
		}
		return nil
	})

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
	if err := DBFromContext(c).NewSelect().Model(&tasks).Where("project_id = ?", pid).Order("created_at DESC").Scan(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tasks)
}

func (h *Handler) GetTask(c *gin.Context) {
	id := c.Param("id")
	task := &models.CaseGenerationTask{ID: 0}

	if err := DBFromContext(c).NewSelect().Model(task).Where("id = ?", id).Scan(c); err != nil {
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
	if err := DBFromContext(c).NewSelect().Model(task).Where("id = ?", id).Scan(c); err != nil {
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

	if _, err := DBFromContext(c).NewUpdate().Model(task).Where("id = ?", id).Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("task review",
		"task_id", id,
		"products", task.AffectedProducts,
		"modules", task.AffectedModules,
	)

	c.JSON(http.StatusOK, task)
}

func (h *Handler) GenerateCases(c *gin.Context) {
	id := c.Param("id")
	task := &models.CaseGenerationTask{ID: 0}

	if err := DBFromContext(c).NewSelect().Model(task).Where("id = ?", id).Scan(c); err != nil {
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

	updateResult, err := DBFromContext(c).NewUpdate().
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

	tenantID, _ := TenantIDFromContext(c)
	taskID := task.ID
	RunAsync(h.DB, tenantID, func(ctx context.Context, tx bun.Tx) error {
		if err := taskservice.New(tx).GenerateCases(ctx, taskID); err != nil {
			slog.Error("task generate failed", "task_id", taskID, "error", err)
			return err
		}
		return nil
	})

	slog.Info("task generate accepted", "task_id", task.ID)

	c.JSON(http.StatusOK, task)
}

func (h *Handler) RetryTask(c *gin.Context) {
	id := c.Param("id")
	task := &models.CaseGenerationTask{ID: 0}

	if err := DBFromContext(c).NewSelect().Model(task).Where("id = ?", id).Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if task.Status != models.TaskStatusFailed {
		c.JSON(http.StatusConflict, gin.H{
			"error": fmt.Sprintf("task status %q is not retryable; only failed tasks can be retried", task.Status),
		})
		return
	}

	rerunAnalyze := len(task.AffectedProducts) == 0 && len(task.AffectedModules) == 0
	if rerunAnalyze {
		task.Status = models.TaskStatusAnalyzing
	} else {
		task.Status = models.TaskStatusReadyToGenerate
	}
	task.UpdatedAt = time.Now()

	if _, err := DBFromContext(c).NewUpdate().Model(task).
		Set("status = ?", task.Status).
		Set("updated_at = ?", task.UpdatedAt).
		Where("id = ?", id).
		Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if rerunAnalyze {
		tenantID, _ := TenantIDFromContext(c)
		taskID := task.ID
		RunAsync(h.DB, tenantID, func(ctx context.Context, tx bun.Tx) error {
			if err := taskservice.New(tx).AnalyzeTask(ctx, taskID); err != nil {
				slog.Error("task retry analyze failed", "task_id", taskID, "error", err)
				return err
			}
			return nil
		})
	}

	slog.Info("task retry accepted",
		"task_id", task.ID,
		"phase", map[bool]string{true: "analyze", false: "ready_to_generate"}[rerunAnalyze],
	)

	c.JSON(http.StatusAccepted, task)
}

func validateTaskDocuments(ctx context.Context, db bun.IDB, projectID int, documentIDs []int) error {
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
