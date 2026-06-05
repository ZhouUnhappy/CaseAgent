package handler

import (
	"log/slog"
	"net/http"
	"time"

	"caseagent/internal/db/models"
	jobservice "caseagent/internal/service/job"

	"github.com/gin-gonic/gin"
)

type UploadKnowledgeRequest struct {
	Type     string         `json:"type" binding:"required"` // 'product', 'module'
	Name     string         `json:"name" binding:"required"`
	Content  string         `json:"content" binding:"required"`
	Metadata map[string]any `json:"metadata"`
}

type UpdateKnowledgeRequest struct {
	Name     string         `json:"name"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

func (h *Handler) UploadKnowledge(c *gin.Context) {
	var req UploadKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID, _ := TenantIDFromContext(c)
	kb := &models.KnowledgeBase{
		TenantID:  tenantID,
		Type:      req.Type,
		Name:      req.Name,
		Content:   req.Content,
		Metadata:  req.Metadata,
		Status:    models.KnowledgeStatusProcessing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if _, err := DBFromContext(c).NewInsert().Model(kb).Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("knowledge create accepted", "knowledge_id", kb.ID, "type", kb.Type, "name", kb.Name)

	if _, err := jobservice.New(DBFromContext(c)).Enqueue(c, jobservice.EnqueueInput{
		KnowledgeID: kb.ID,
		JobType:     models.JobTypeKnowledgeProcess,
		MaxRetries:  configuredJobMaxRetriesFor(models.JobTypeKnowledgeProcess),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, kb)
}

func (h *Handler) ListKnowledge(c *gin.Context) {
	kbType := c.Query("type")
	var knowledge []models.KnowledgeBase

	query := DBFromContext(c).NewSelect().Model(&knowledge)
	if kbType != "" {
		query = query.Where("type = ?", kbType)
	}

	if err := query.Order("created_at DESC").Scan(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, knowledge)
}

func (h *Handler) GetKnowledge(c *gin.Context) {
	id := c.Param("id")
	kb := &models.KnowledgeBase{ID: 0}

	if err := DBFromContext(c).NewSelect().Model(kb).Where("id = ?", id).Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Knowledge not found"})
		return
	}

	c.JSON(http.StatusOK, kb)
}

func (h *Handler) UpdateKnowledge(c *gin.Context) {
	id := c.Param("id")
	var req UpdateKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	kb := &models.KnowledgeBase{ID: 0}
	if err := DBFromContext(c).NewSelect().Model(kb).Where("id = ?", id).Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Knowledge not found"})
		return
	}

	if req.Name != "" {
		kb.Name = req.Name
	}
	if req.Metadata != nil {
		kb.Metadata = req.Metadata
	}
	needsReprocess := applyKnowledgeUpdate(kb, req)
	kb.UpdatedAt = time.Now()

	update := DBFromContext(c).NewUpdate().Model(&models.KnowledgeBase{}).
		Set("name = ?", kb.Name).
		Set("content = ?", kb.Content).
		Set("metadata = ?", kb.Metadata).
		Set("status = ?", kb.Status).
		Set("updated_at = ?", kb.UpdatedAt).
		Where("id = ?", id)
	if needsReprocess {
		update = update.Set("embedding = ?", nil)
	}

	if _, err := update.Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if needsReprocess {
		if _, err := jobservice.New(DBFromContext(c)).Enqueue(c, jobservice.EnqueueInput{
			KnowledgeID: kb.ID,
			JobType:     models.JobTypeKnowledgeReprocess,
			MaxRetries:  configuredJobMaxRetriesFor(models.JobTypeKnowledgeReprocess),
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	slog.Info("knowledge update", "knowledge_id", kb.ID, "name", kb.Name, "reprocess", needsReprocess)

	c.JSON(http.StatusOK, kb)
}

func (h *Handler) ReprocessKnowledge(c *gin.Context) {
	id := c.Param("id")
	kb := &models.KnowledgeBase{}

	if err := DBFromContext(c).NewSelect().Model(kb).Where("id = ?", id).Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Knowledge not found"})
		return
	}

	kb.Embedding = nil
	kb.Status = models.KnowledgeStatusProcessing
	kb.UpdatedAt = time.Now()

	if _, err := DBFromContext(c).NewUpdate().Model(&models.KnowledgeBase{}).
		Set("embedding = ?", nil).
		Set("status = ?", kb.Status).
		Set("updated_at = ?", kb.UpdatedAt).
		Where("id = ?", id).
		Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("knowledge reprocess accepted", "knowledge_id", kb.ID, "name", kb.Name)

	if _, err := jobservice.New(DBFromContext(c)).Enqueue(c, jobservice.EnqueueInput{
		KnowledgeID: kb.ID,
		JobType:     models.JobTypeKnowledgeReprocess,
		MaxRetries:  configuredJobMaxRetriesFor(models.JobTypeKnowledgeReprocess),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, kb)
}

func (h *Handler) DeleteKnowledge(c *gin.Context) {
	id := c.Param("id")

	if _, err := DBFromContext(c).NewDelete().Model(&models.KnowledgeBase{}).Where("id = ?", id).Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func applyKnowledgeUpdate(kb *models.KnowledgeBase, req UpdateKnowledgeRequest) bool {
	if req.Content == "" {
		return false
	}

	kb.Content = req.Content
	kb.Embedding = nil
	kb.Status = models.KnowledgeStatusProcessing
	return true
}
