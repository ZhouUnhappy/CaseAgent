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
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := h.DB.NewInsert().Model(kb).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Process knowledge base asynchronously
	go func(kbID int, content string) {
		ctx := context.Background()
		kbService, err := knowledge.New(ctx, h.DB)
		if err != nil {
			fmt.Printf("Failed to initialize knowledge service: %v\n", err)
			return
		}
		err = kbService.ProcessKnowledge(ctx, kbID, content)
		if err != nil {
			fmt.Printf("Failed to process knowledge: %v\n", err)
		}
	}(kb.ID, req.Content)

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
	if req.Content != "" {
		kb.Content = req.Content
	}
	if req.Metadata != nil {
		kb.Metadata = req.Metadata
	}
	kb.UpdatedAt = time.Now()

	_, err = h.DB.NewUpdate().Model(kb).Where("id = ?", id).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.Content != "" {
		go func(kbID int, content string) {
			ctx := context.Background()
			kbService, err := knowledge.New(ctx, h.DB)
			if err != nil {
				fmt.Printf("Failed to initialize knowledge service: %v\n", err)
				return
			}
			err = kbService.ProcessKnowledge(ctx, kbID, content)
			if err != nil {
				fmt.Printf("Failed to re-process knowledge: %v\n", err)
			}
		}(kb.ID, kb.Content)
	}

	c.JSON(http.StatusOK, kb)
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
