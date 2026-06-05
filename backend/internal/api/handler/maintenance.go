package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"caseagent/internal/db/models"
	jobservice "caseagent/internal/service/job"
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

func (h *Handler) ListStaleIndex(c *gin.Context) {
	plan, err := maintenanceservice.New(DBFromContext(c)).RepairPlan(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plan)
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

	if err := enqueueRepairPlan(c, plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"queued_documents":  len(plan.DocumentIDs),
		"queued_knowledge":  len(plan.KnowledgeIDs),
		"blocked_documents": plan.BlockedDocumentIDs,
		"blocked_knowledge": plan.BlockedKnowledgeIDs,
	})
}

func enqueueRepairPlan(c *gin.Context, plan *maintenanceservice.RepairPlan) error {
	jobs := jobservice.New(DBFromContext(c))
	payload := map[string]any{
		"reason":        "stale_index",
		"index_profile": plan.Profile.Name,
		"index_version": plan.Profile.Version,
	}
	for _, id := range plan.DocumentIDs {
		if _, err := jobs.Enqueue(c, jobservice.EnqueueInput{
			DocumentID: id,
			JobType:    models.JobTypeDocumentReprocess,
			MaxRetries: configuredJobMaxRetriesFor(models.JobTypeDocumentReprocess),
			Payload:    payload,
		}); err != nil {
			return fmt.Errorf("enqueue document reindex job %d: %w", id, err)
		}
	}
	for _, id := range plan.KnowledgeIDs {
		if _, err := jobs.Enqueue(c, jobservice.EnqueueInput{
			KnowledgeID: id,
			JobType:     models.JobTypeKnowledgeReprocess,
			MaxRetries:  configuredJobMaxRetriesFor(models.JobTypeKnowledgeReprocess),
			Payload:     payload,
		}); err != nil {
			return fmt.Errorf("enqueue knowledge reindex job %d: %w", id, err)
		}
	}
	return nil
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
