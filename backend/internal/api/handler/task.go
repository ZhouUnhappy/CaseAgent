package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

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

	task, err := taskservice.New(DBFromContext(c)).CreateTask(c, pid, req.DocumentIDs)
	if err != nil {
		writeTaskServiceError(c, err)
		return
	}

	slog.Info("task created",
		"task_id", task.ID,
		"project_id", task.ProjectID,
		"document_ids", task.DocumentIDs,
	)

	tenantID, _ := TenantIDFromContext(c)
	taskID := task.ID
	RunAsyncAfterCommitWithFailure(c, h.DB, tenantID, func(ctx context.Context, tx bun.Tx) error {
		if err := taskservice.New(tx).AnalyzeTask(ctx, taskID); err != nil {
			slog.Error("task analyze failed", "task_id", taskID, "error", err)
			return err
		}
		return nil
	}, func(ctx context.Context, tx bun.Tx, cause error) error {
		return taskservice.New(tx).MarkTaskFailed(ctx, taskID)
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
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	var req ReviewAffectedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := taskservice.New(DBFromContext(c)).ReviewAffected(c, taskID, req.AffectedProducts, req.AffectedModules)
	if err != nil {
		writeTaskServiceError(c, err)
		return
	}

	slog.Info("task review",
		"task_id", taskID,
		"products", task.AffectedProducts,
		"modules", task.AffectedModules,
	)

	c.JSON(http.StatusOK, task)
}

func (h *Handler) GenerateCases(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	task, err := taskservice.New(DBFromContext(c)).StartGeneration(c, taskID)
	if err != nil {
		writeTaskServiceError(c, err)
		return
	}

	tenantID, _ := TenantIDFromContext(c)
	RunAsyncAfterCommitWithFailure(c, h.DB, tenantID, func(ctx context.Context, tx bun.Tx) error {
		if err := taskservice.New(tx).GenerateCases(ctx, taskID); err != nil {
			slog.Error("task generate failed", "task_id", taskID, "error", err)
			return err
		}
		return nil
	}, func(ctx context.Context, tx bun.Tx, cause error) error {
		if err := taskservice.New(tx).MarkGenerationFailed(ctx, taskID, cause); err != nil {
			slog.Error("task generate failure handling failed", "task_id", taskID, "error", err)
			return err
		}
		return nil
	})

	slog.Info("task generate accepted", "task_id", task.ID)

	c.JSON(http.StatusOK, task)
}

func (h *Handler) RetryTask(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	decision, err := taskservice.New(DBFromContext(c)).RetryTask(c, taskID)
	if err != nil {
		writeTaskServiceError(c, err)
		return
	}

	if decision.RerunAnalyze {
		tenantID, _ := TenantIDFromContext(c)
		RunAsyncAfterCommitWithFailure(c, h.DB, tenantID, func(ctx context.Context, tx bun.Tx) error {
			if err := taskservice.New(tx).AnalyzeTask(ctx, taskID); err != nil {
				slog.Error("task retry analyze failed", "task_id", taskID, "error", err)
				return err
			}
			return nil
		}, func(ctx context.Context, tx bun.Tx, cause error) error {
			return taskservice.New(tx).MarkTaskFailed(ctx, taskID)
		})
	}

	slog.Info("task retry accepted",
		"task_id", decision.Task.ID,
		"phase", map[bool]string{true: "analyze", false: "ready_to_generate"}[decision.RerunAnalyze],
	)

	c.JSON(http.StatusAccepted, decision.Task)
}

func parseTaskID(c *gin.Context) (int, bool) {
	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return 0, false
	}
	return taskID, true
}

func writeTaskServiceError(c *gin.Context, err error) {
	var badRequest *taskservice.BadRequestError
	if errors.As(err, &badRequest) {
		c.JSON(http.StatusBadRequest, gin.H{"error": badRequest.Error()})
		return
	}

	var notFound *taskservice.NotFoundError
	if errors.As(err, &notFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": notFound.Error()})
		return
	}

	var conflict *taskservice.ConflictError
	if errors.As(err, &conflict) {
		c.JSON(http.StatusConflict, gin.H{"error": conflict.Error()})
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
