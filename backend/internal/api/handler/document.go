package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"caseagent/internal/db/models"
	documentservice "caseagent/internal/service/document"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
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
		if _, err := io.ReadFull(fileContent, bytes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		content = string(bytes)
	}

	tenantID, _ := TenantIDFromContext(c)
	document := &models.Document{
		TenantID:  tenantID,
		ProjectID: pid,
		Name:      req.Name,
		Type:      req.Type,
		Source:    req.Source,
		Content:   content,
		FileID:    req.FileID,
		Status:    "processing",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if _, err := DBFromContext(c).NewInsert().Model(document).Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("document upload accepted",
		"project_id", pid,
		"document_id", document.ID,
		"name", document.Name,
		"source", document.Source,
	)

	docID := document.ID
	docContent := content
	gwsFileID := req.FileID
	RunAsyncAfterCommit(c, h.DB, tenantID, func(ctx context.Context, tx bun.Tx) error {
		docService, err := documentservice.New(ctx, tx)
		if err != nil {
			slog.Error("document service init failed", "document_id", docID, "error", err)
			markDocumentFailed(ctx, tx, docID)
			return err
		}
		if err := docService.ProcessDocument(ctx, docID, docContent, gwsFileID); err != nil {
			slog.Error("document process failed", "document_id", docID, "error", err)
			return err
		}
		return nil
	})

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
	if err := DBFromContext(c).NewSelect().Model(&documents).Where("project_id = ?", pid).Order("created_at DESC").Scan(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, documents)
}

func (h *Handler) GetDocument(c *gin.Context) {
	id := c.Param("id")
	document := &models.Document{ID: 0}

	if err := DBFromContext(c).NewSelect().Model(document).Where("id = ?", id).Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	c.JSON(http.StatusOK, document)
}

func (h *Handler) DeleteDocument(c *gin.Context) {
	id := c.Param("id")

	if _, err := DBFromContext(c).NewDelete().Model(&models.Document{}).Where("id = ?", id).Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) ReprocessDocument(c *gin.Context) {
	id := c.Param("id")
	document := &models.Document{}

	if err := DBFromContext(c).NewSelect().Model(document).Where("id = ?", id).Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	document.Status = "processing"
	document.UpdatedAt = time.Now()

	if _, err := DBFromContext(c).NewUpdate().Model(document).Where("id = ?", id).Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("document reprocess accepted", "document_id", document.ID, "name", document.Name)

	tenantID, _ := TenantIDFromContext(c)
	docID := document.ID
	RunAsyncAfterCommit(c, h.DB, tenantID, func(ctx context.Context, tx bun.Tx) error {
		docService, err := documentservice.New(ctx, tx)
		if err != nil {
			slog.Error("document service init failed", "document_id", docID, "error", err)
			markDocumentFailed(ctx, tx, docID)
			return err
		}
		if err := docService.ReprocessDocument(ctx, docID); err != nil {
			slog.Error("document reprocess failed", "document_id", docID, "error", err)
			return err
		}
		return nil
	})

	c.JSON(http.StatusAccepted, document)
}

func markDocumentFailed(ctx context.Context, db bun.IDB, docID int) {
	_, _ = db.NewUpdate().Model(&models.Document{}).
		Set("status = ?", "failed").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", docID).
		Exec(ctx)
}
