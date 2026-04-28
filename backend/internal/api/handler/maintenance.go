package handler

import (
	"context"
	"fmt"
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
	report, err := maintenanceservice.New(h.DB).VectorHealth(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, report)
}

func (h *Handler) ReindexVectors(c *gin.Context) {
	plan, err := maintenanceservice.New(h.DB).RepairPlan(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	if err := markDocumentsProcessing(c, h.DB, plan.DocumentIDs, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := markKnowledgeProcessing(c, h.DB, plan.KnowledgeIDs, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.runRepairPlan(plan)

	c.JSON(http.StatusAccepted, gin.H{
		"queued_documents":  len(plan.DocumentIDs),
		"queued_knowledge":  len(plan.KnowledgeIDs),
		"blocked_documents": plan.BlockedDocumentIDs,
		"blocked_knowledge": plan.BlockedKnowledgeIDs,
	})
}

func (h *Handler) runRepairPlan(plan *maintenanceservice.RepairPlan) {
	go func(documentIDs []int, knowledgeIDs []int) {
		ctx := context.Background()

		if len(documentIDs) > 0 {
			docService, err := documentservice.New(ctx, h.DB)
			if err != nil {
				fmt.Printf("Failed to initialize document service for batch reindex: %v\n", err)
				_ = markDocumentsFailed(ctx, h.DB, documentIDs, time.Now())
			} else {
				for _, id := range documentIDs {
					if err := docService.ReprocessDocument(ctx, id); err != nil {
						fmt.Printf("Failed to reprocess document %d in batch reindex: %v\n", id, err)
					}
				}
			}
		}

		if len(knowledgeIDs) > 0 {
			knowledgeService, err := knowledgeservice.New(ctx, h.DB)
			if err != nil {
				fmt.Printf("Failed to initialize knowledge service for batch reindex: %v\n", err)
				_ = markKnowledgeFailed(ctx, h.DB, knowledgeIDs, time.Now())
				return
			}
			for _, id := range knowledgeIDs {
				if err := knowledgeService.ReprocessKnowledge(ctx, id); err != nil {
					fmt.Printf("Failed to reprocess knowledge %d in batch reindex: %v\n", id, err)
				}
			}
		}
	}(append([]int(nil), plan.DocumentIDs...), append([]int(nil), plan.KnowledgeIDs...))
}

func markDocumentsProcessing(ctx context.Context, db *bun.DB, ids []int, updatedAt time.Time) error {
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

func markKnowledgeProcessing(ctx context.Context, db *bun.DB, ids []int, updatedAt time.Time) error {
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

func markDocumentsFailed(ctx context.Context, db *bun.DB, ids []int, updatedAt time.Time) error {
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

func markKnowledgeFailed(ctx context.Context, db *bun.DB, ids []int, updatedAt time.Time) error {
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
