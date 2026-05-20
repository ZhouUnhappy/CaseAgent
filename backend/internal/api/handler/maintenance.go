package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"caseagent/internal/db/models"
	documentservice "caseagent/internal/service/document"
	knowledgeservice "caseagent/internal/service/knowledge"
	maintenanceservice "caseagent/internal/service/maintenance"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

func (h *Handler) GetVectorHealth(c *gin.Context) {
	report, err := maintenanceservice.New(DBFromContext(c)).VectorHealth(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, report)
}

// ReindexVectors is tenant-scoped: it only repairs vectors for the calling
// tenant's documents/knowledge. Cross-tenant batch repair would need an admin
// endpoint and a superuser DSN to bypass RLS; not implemented yet.
func (h *Handler) ReindexVectors(c *gin.Context) {
	plan, err := maintenanceservice.New(DBFromContext(c)).RepairPlan(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	if err := markDocumentsProcessing(c, DBFromContext(c), plan.DocumentIDs, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := markKnowledgeProcessing(c, DBFromContext(c), plan.KnowledgeIDs, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tenantID, _ := TenantIDFromContext(c)
	h.runRepairPlan(tenantID, plan)

	c.JSON(http.StatusAccepted, gin.H{
		"queued_documents":  len(plan.DocumentIDs),
		"queued_knowledge":  len(plan.KnowledgeIDs),
		"blocked_documents": plan.BlockedDocumentIDs,
		"blocked_knowledge": plan.BlockedKnowledgeIDs,
	})
}

func (h *Handler) runRepairPlan(tenantID int, plan *maintenanceservice.RepairPlan) {
	documentIDs := append([]int(nil), plan.DocumentIDs...)
	knowledgeIDs := append([]int(nil), plan.KnowledgeIDs...)

	RunAsync(h.DB, tenantID, func(ctx context.Context, tx bun.Tx) error {
		if len(documentIDs) > 0 {
			docService, err := documentservice.New(ctx, tx)
			if err != nil {
				slog.Error("document service init failed for batch reindex", "error", err)
				_ = markDocumentsFailed(ctx, tx, documentIDs, time.Now())
			} else {
				for _, id := range documentIDs {
					if err := docService.ReprocessDocument(ctx, id); err != nil {
						slog.Error("batch reindex document failed", "document_id", id, "error", err)
					}
				}
			}
		}

		if len(knowledgeIDs) > 0 {
			knowledgeService, err := knowledgeservice.New(ctx, tx)
			if err != nil {
				slog.Error("knowledge service init failed for batch reindex", "error", err)
				_ = markKnowledgeFailed(ctx, tx, knowledgeIDs, time.Now())
				return nil
			}
			for _, id := range knowledgeIDs {
				if err := knowledgeService.ReprocessKnowledge(ctx, id); err != nil {
					slog.Error("batch reindex knowledge failed", "knowledge_id", id, "error", err)
				}
			}
		}
		return nil
	})
}

func markDocumentsProcessing(ctx context.Context, db bun.IDB, ids []int, updatedAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := db.NewUpdate().Model(&models.Document{}).
		Set("status = ?", "processing").
		Set("updated_at = ?", updatedAt).
		Where("id IN (?)", bun.In(ids)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mark documents processing: %w", err)
	}
	return nil
}

func markKnowledgeProcessing(ctx context.Context, db bun.IDB, ids []int, updatedAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := db.NewUpdate().Model(&models.KnowledgeBase{}).
		Set("embedding = ?", nil).
		Set("status = ?", models.KnowledgeStatusProcessing).
		Set("updated_at = ?", updatedAt).
		Where("id IN (?)", bun.In(ids)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mark knowledge processing: %w", err)
	}
	return nil
}

func markDocumentsFailed(ctx context.Context, db bun.IDB, ids []int, updatedAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := db.NewUpdate().Model(&models.Document{}).
		Set("status = ?", "failed").
		Set("updated_at = ?", updatedAt).
		Where("id IN (?)", bun.In(ids)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mark documents failed: %w", err)
	}
	return nil
}

func markKnowledgeFailed(ctx context.Context, db bun.IDB, ids []int, updatedAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := db.NewUpdate().Model(&models.KnowledgeBase{}).
		Set("status = ?", models.KnowledgeStatusFailed).
		Set("updated_at = ?", updatedAt).
		Where("id IN (?)", bun.In(ids)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mark knowledge failed: %w", err)
	}
	return nil
}
