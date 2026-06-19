package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"caseagent/internal/config"
	"caseagent/internal/db/models"
	jobservice "caseagent/internal/service/job"
	taskservice "caseagent/internal/service/task"

	"github.com/gin-gonic/gin"
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

	if _, err := jobservice.New(DBFromContext(c)).Enqueue(c.Request.Context(), jobservice.EnqueueInput{
		TaskID:     task.ID,
		JobType:    models.JobTypeAnalyze,
		MaxRetries: configuredJobMaxRetriesFor(models.JobTypeAnalyze),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

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

	if _, err := jobservice.New(DBFromContext(c)).Enqueue(c.Request.Context(), jobservice.EnqueueInput{
		TaskID:     taskID,
		JobType:    models.JobTypeGenerate,
		MaxRetries: configuredJobMaxRetriesFor(models.JobTypeGenerate),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

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
		if _, err := jobservice.New(DBFromContext(c)).Enqueue(c.Request.Context(), jobservice.EnqueueInput{
			TaskID:     taskID,
			JobType:    models.JobTypeAnalyze,
			MaxRetries: configuredJobMaxRetriesFor(models.JobTypeAnalyze),
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	slog.Info("task retry accepted",
		"task_id", decision.Task.ID,
		"phase", map[bool]string{true: "analyze", false: "ready_to_generate"}[decision.RerunAnalyze],
	)

	c.JSON(http.StatusAccepted, decision.Task)
}

func configuredJobMaxRetriesFor(jobType string) int {
	cfg := config.Get()
	if cfg == nil {
		return 2
	}
	if typeOptions, ok := cfg.JobRunner.Types[jobType]; ok && typeOptions.MaxRetries >= 0 {
		return typeOptions.MaxRetries
	}
	if cfg.JobRunner.MaxRetries < 0 {
		return 2
	}
	return cfg.JobRunner.MaxRetries
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
