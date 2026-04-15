package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"caseagent/internal/db/models"
	documentservice "caseagent/internal/service/document"

	"github.com/gin-gonic/gin"
)

type UploadDocumentRequest struct {
	Name   string `form:"name" binding:"required"`
	Type   string `form:"type" binding:"required"`   // 'markdown', 'gdrive'
	Source string `form:"source" binding:"required"` // 'upload', 'gdrive'
	FileID string `form:"file_id"`                   // Google Drive file ID
}

func (h *Handler) UploadDocument(c *gin.Context) {
	projectID := c.Param("id")
	var req UploadDocumentRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pid, err := strconv.Atoi(projectID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	// Get file content if uploaded
	var content string
	if req.Source == "upload" {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
			return
		}

		fileContent, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer fileContent.Close()

		bytes := make([]byte, file.Size)
		_, err = io.ReadFull(fileContent, bytes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		content = string(bytes)
	}

	document := &models.Document{
		ProjectID: pid,
		Name:      req.Name,
		Type:      req.Type,
		Source:    req.Source,
		FileID:    req.FileID,
		Status:    "processing",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = h.DB.NewInsert().Model(document).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Process document asynchronously
	go func(docID int, content string, gwsFileID string) {
		ctx := context.Background()
		docService, err := documentservice.New(ctx, h.DB)
		if err != nil {
			fmt.Printf("Failed to initialize document service: %v\n", err)
			return
		}
		err = docService.ProcessDocument(ctx, docID, content, gwsFileID)
		if err != nil {
			fmt.Printf("Failed to process document: %v\n", err)
		}
	}(document.ID, content, req.FileID)

	c.JSON(http.StatusCreated, document)
}

func (h *Handler) ListDocuments(c *gin.Context) {
	projectID := c.Param("id")
	pid, err := strconv.Atoi(projectID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	var documents []models.Document
	err = h.DB.NewSelect().Model(&documents).Where("project_id = ?", pid).Order("created_at DESC").Scan(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, documents)
}

func (h *Handler) GetDocument(c *gin.Context) {
	id := c.Param("id")
	document := &models.Document{ID: 0}

	err := h.DB.NewSelect().Model(document).Where("id = ?", id).Scan(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	c.JSON(http.StatusOK, document)
}

func (h *Handler) DeleteDocument(c *gin.Context) {
	id := c.Param("id")

	_, err := h.DB.NewDelete().Model(&models.Document{}).Where("id = ?", id).Exec(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}
