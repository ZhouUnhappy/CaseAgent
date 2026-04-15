package handler

import (
	"net/http"
	"strconv"
	"time"

	"caseagent/internal/db/models"

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

	task := &models.CaseGenerationTask{
		ProjectID:   pid,
		DocumentIDs: req.DocumentIDs,
		Status:      "analyzing",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err = h.DB.NewInsert().Model(task).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// TODO: Start async analysis to determine affected products/modules

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

	task.AffectedProducts = req.AffectedProducts
	task.AffectedModules = req.AffectedModules
	task.Status = "awaiting_review"
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

	task.Status = "generating"
	task.UpdatedAt = time.Now()

	_, err = h.DB.NewUpdate().Model(task).Where("id = ?", id).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// TODO: Start async case generation using AI agents

	c.JSON(http.StatusOK, task)
}
