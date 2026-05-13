package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"caseagent/internal/db/models"
	"caseagent/internal/service/knowledge"

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

	kb := &models.KnowledgeBase{
		Type:      req.Type,
		Name:      req.Name,
		Content:   req.Content,
		Metadata:  req.Metadata,
		Status:    models.KnowledgeStatusProcessing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := h.DB.NewInsert().Model(kb).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	fmt.Printf("[handler] knowledge create accepted: knowledge_id=%d type=%s name=%q\n", kb.ID, kb.Type, kb.Name)

	h.processKnowledgeAsync(kb.ID, req.Content)

	c.JSON(http.StatusCreated, kb)
}

func (h *Handler) ListKnowledge(c *gin.Context) {
	kbType := c.Query("type")
	var knowledge []models.KnowledgeBase

	query := h.DB.NewSelect().Model(&knowledge)
	if kbType != "" {
		query = query.Where("type = ?", kbType)
	}

	err := query.Order("created_at DESC").Scan(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, knowledge)
}

func (h *Handler) GetKnowledge(c *gin.Context) {
	id := c.Param("id")
	kb := &models.KnowledgeBase{ID: 0}

	err := h.DB.NewSelect().Model(kb).Where("id = ?", id).Scan(c)
	if err != nil {
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
	err := h.DB.NewSelect().Model(kb).Where("id = ?", id).Scan(c)
	if err != nil {
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

	update := h.DB.NewUpdate().Model(&models.KnowledgeBase{}).
		Set("name = ?", kb.Name).
		Set("content = ?", kb.Content).
		Set("metadata = ?", kb.Metadata).
		Set("status = ?", kb.Status).
		Set("updated_at = ?", kb.UpdatedAt).
		Where("id = ?", id)
	if needsReprocess {
		update = update.Set("embedding = ?", nil)
	}

	_, err = update.Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if needsReprocess {
		h.processKnowledgeAsync(kb.ID, kb.Content)
	}

	fmt.Printf("[handler] knowledge update: knowledge_id=%d name=%q reprocess=%t\n", kb.ID, kb.Name, needsReprocess)

	c.JSON(http.StatusOK, kb)
}

func (h *Handler) ReprocessKnowledge(c *gin.Context) {
	id := c.Param("id")
	kb := &models.KnowledgeBase{}

	if err := h.DB.NewSelect().Model(kb).Where("id = ?", id).Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Knowledge not found"})
		return
	}

	kb.Embedding = nil
	kb.Status = models.KnowledgeStatusProcessing
	kb.UpdatedAt = time.Now()

	if _, err := h.DB.NewUpdate().Model(&models.KnowledgeBase{}).
		Set("embedding = ?", nil).
		Set("status = ?", kb.Status).
		Set("updated_at = ?", kb.UpdatedAt).
		Where("id = ?", id).
		Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	fmt.Printf("[handler] knowledge reprocess accepted: knowledge_id=%d name=%q\n", kb.ID, kb.Name)

	go func(kbID int) {
		ctx := context.Background()
		kbService, err := knowledge.New(ctx, h.DB)
		if err != nil {
			fmt.Printf("[handler] knowledge_id=%d service init failed: %v\n", kbID, err)
			_, _ = h.DB.NewUpdate().Model(&models.KnowledgeBase{}).
				Set("status = ?", models.KnowledgeStatusFailed).
				Set("updated_at = ?", time.Now()).
				Where("id = ?", kbID).
				Exec(ctx)
			return
		}
		if err := kbService.ReprocessKnowledge(ctx, kbID); err != nil {
			fmt.Printf("[handler] knowledge_id=%d reprocess failed: %v\n", kbID, err)
		}
	}(kb.ID)

	c.JSON(http.StatusAccepted, kb)
}

func (h *Handler) DeleteKnowledge(c *gin.Context) {
	id := c.Param("id")

	_, err := h.DB.NewDelete().Model(&models.KnowledgeBase{}).Where("id = ?", id).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) processKnowledgeAsync(kbID int, content string) {
	go func() {
		ctx := context.Background()
		kbService, err := knowledge.New(ctx, h.DB)
		if err != nil {
			fmt.Printf("[handler] knowledge_id=%d service init failed: %v\n", kbID, err)
			_, _ = h.DB.NewUpdate().Model(&models.KnowledgeBase{}).
				Set("status = ?", models.KnowledgeStatusFailed).
				Set("updated_at = ?", time.Now()).
				Where("id = ?", kbID).
				Exec(ctx)
			return
		}
		if err := kbService.ProcessKnowledge(ctx, kbID, content); err != nil {
			fmt.Printf("[handler] knowledge_id=%d process failed: %v\n", kbID, err)
		}
	}()
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
