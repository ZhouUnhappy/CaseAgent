package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"caseagent/internal/db/models"
	jobservice "caseagent/internal/service/job"

	"github.com/gin-gonic/gin"
)

type UploadKnowledgeRequest struct {
	Type          string         `json:"type" binding:"required"` // 'product', 'module'
	Name          string         `json:"name" binding:"required"`
	Content       string         `json:"content" binding:"required"`
	Metadata      map[string]any `json:"metadata"`
	Source        string         `json:"source"`
	ExpiresAt     *time.Time     `json:"expires_at"`
	DuplicateOfID *int           `json:"duplicate_of_id"`
}

type UpdateKnowledgeRequest struct {
	Name           string         `json:"name"`
	Content        string         `json:"content"`
	Metadata       map[string]any `json:"metadata"`
	Source         *string        `json:"source"`
	ExpiresAt      *time.Time     `json:"expires_at"`
	ClearExpiresAt bool           `json:"clear_expires_at"`
	DuplicateOfID  *int           `json:"duplicate_of_id"`
	ClearDuplicate bool           `json:"clear_duplicate"`
}

type KnowledgeImpactedTask struct {
	TaskID              int       `bun:"task_id" json:"task_id"`
	Status              string    `bun:"status" json:"status"`
	SectionCount        int       `bun:"section_count" json:"section_count"`
	CaseCount           int       `bun:"case_count" json:"case_count"`
	LastSourceContextAt time.Time `bun:"last_source_context_at" json:"last_source_context_at"`
	UpdatedAt           time.Time `bun:"updated_at" json:"updated_at"`
}

func (h *Handler) UploadKnowledge(c *gin.Context) {
	var req UploadKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	source, err := knowledgeSourceForWrite(req.Source)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateKnowledgeDuplicateTarget(c, 0, req.DuplicateOfID); err != nil {
		writeKnowledgeValidationError(c, err)
		return
	}

	tenantID, _ := TenantIDFromContext(c)
	now := time.Now()
	kb := &models.KnowledgeBase{
		TenantID:          tenantID,
		Type:              req.Type,
		Name:              req.Name,
		Content:           req.Content,
		Metadata:          req.Metadata,
		Source:            source,
		ExpiresAt:         req.ExpiresAt,
		DuplicateOfID:     req.DuplicateOfID,
		DuplicateMarkedAt: duplicateMarkedAt(req.DuplicateOfID, nil, nil, now),
		Status:            models.KnowledgeStatusProcessing,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if _, err := DBFromContext(c).NewInsert().Model(kb).Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("knowledge create accepted", "knowledge_id", kb.ID, "type", kb.Type, "name", kb.Name)

	if _, err := jobservice.New(DBFromContext(c)).Enqueue(c.Request.Context(), jobservice.EnqueueInput{
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
	source := strings.TrimSpace(c.Query("source"))
	expired, err := parseTriStateBoolQuery("expired", c.Query("expired"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	duplicate, err := parseTriStateBoolQuery("duplicate", c.Query("duplicate"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	knowledge := []models.KnowledgeBase{}

	query := DBFromContext(c).NewSelect().Model(&knowledge)
	if kbType != "" {
		query = query.Where("type = ?", kbType)
	}
	if source != "" {
		normalizedSource, err := knowledgeSourceForWrite(source)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		query = query.Where("source = ?", normalizedSource)
	}
	if expired != nil {
		if *expired {
			query = query.Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now())
		} else {
			query = query.Where("(expires_at IS NULL OR expires_at > ?)", time.Now())
		}
	}
	if duplicate != nil {
		if *duplicate {
			query = query.Where("duplicate_of_id IS NOT NULL")
		} else {
			query = query.Where("duplicate_of_id IS NULL")
		}
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
	if err := applyKnowledgeGovernanceUpdate(c, kb, req, time.Now()); err != nil {
		writeKnowledgeValidationError(c, err)
		return
	}
	needsReprocess := applyKnowledgeUpdate(kb, req)
	kb.UpdatedAt = time.Now()

	update := DBFromContext(c).NewUpdate().Model(&models.KnowledgeBase{}).
		Set("name = ?", kb.Name).
		Set("content = ?", kb.Content).
		Set("metadata = ?", kb.Metadata).
		Set("source = ?", kb.Source).
		Set("expires_at = ?", kb.ExpiresAt).
		Set("duplicate_of_id = ?", kb.DuplicateOfID).
		Set("duplicate_marked_at = ?", kb.DuplicateMarkedAt).
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
		if _, err := jobservice.New(DBFromContext(c)).Enqueue(c.Request.Context(), jobservice.EnqueueInput{
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

func (h *Handler) ListKnowledgeImpactedTasks(c *gin.Context) {
	knowledgeID, ok := parseKnowledgeID(c)
	if !ok {
		return
	}

	kb := &models.KnowledgeBase{ID: knowledgeID}
	if err := DBFromContext(c).NewSelect().Model(kb).Where("id = ?", knowledgeID).Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Knowledge not found"})
		return
	}

	var items []KnowledgeImpactedTask
	if err := DBFromContext(c).NewRaw(knowledgeImpactedTasksSQL, knowledgeID, knowledgeID).Scan(c, &items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
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

	if _, err := jobservice.New(DBFromContext(c)).Enqueue(c.Request.Context(), jobservice.EnqueueInput{
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

const defaultKnowledgeSource = "manual"

const knowledgeImpactedTasksSQL = `
SELECT
    cgt.id AS task_id,
    cgt.status AS status,
    COUNT(DISTINCT tc.id)::int AS section_count,
    COALESCE(SUM(
        CASE
            WHEN jsonb_typeof(tc.cases) = 'array' THEN jsonb_array_length(tc.cases)
            ELSE 0
        END
    ), 0)::int AS case_count,
    MAX(tc.updated_at) AS last_source_context_at,
    cgt.updated_at AS updated_at
FROM case_generation_tasks AS cgt
JOIN test_cases AS tc ON tc.task_id = cgt.id
WHERE (
    EXISTS (
        SELECT 1
        FROM jsonb_array_elements(
            CASE
                WHEN jsonb_typeof(tc.source_context->'knowledge_hits') = 'array'
                    THEN tc.source_context->'knowledge_hits'
                ELSE '[]'::jsonb
            END
        ) AS hit(value)
        WHERE hit.value ? 'id'
            AND (hit.value->>'id') ~ '^[0-9]+$'
            AND (hit.value->>'id')::int = ?
    )
    OR EXISTS (
        SELECT 1
        FROM jsonb_array_elements_text(
            CASE
                WHEN jsonb_typeof(tc.source_context->'knowledge_shipped_ids') = 'array'
                    THEN tc.source_context->'knowledge_shipped_ids'
                ELSE '[]'::jsonb
            END
        ) AS kid(value)
        WHERE kid.value ~ '^[0-9]+$'
            AND kid.value::int = ?
    )
)
GROUP BY cgt.id, cgt.status, cgt.updated_at
ORDER BY last_source_context_at DESC, cgt.id DESC
LIMIT 100
`

type knowledgeValidationError struct {
	status int
	err    error
}

func (e knowledgeValidationError) Error() string {
	return e.err.Error()
}

func newKnowledgeValidationError(status int, format string, args ...any) error {
	return knowledgeValidationError{status: status, err: fmt.Errorf(format, args...)}
}

func writeKnowledgeValidationError(c *gin.Context, err error) {
	if typed, ok := err.(knowledgeValidationError); ok {
		c.JSON(typed.status, gin.H{"error": typed.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func knowledgeSourceForWrite(value string) (string, error) {
	source := normalizeKnowledgeSource(value)
	if len([]rune(source)) > 64 {
		return "", fmt.Errorf("source must be 64 characters or fewer")
	}
	return source, nil
}

func normalizeKnowledgeSource(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return defaultKnowledgeSource
	}
	return strings.ToLower(strings.Join(fields, "-"))
}

func parseTriStateBoolQuery(name string, value string) (*bool, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be true or false", name)
	}
	return &parsed, nil
}

func applyKnowledgeGovernanceUpdate(c *gin.Context, kb *models.KnowledgeBase, req UpdateKnowledgeRequest, now time.Time) error {
	if req.Source != nil {
		source, err := knowledgeSourceForWrite(*req.Source)
		if err != nil {
			return newKnowledgeValidationError(http.StatusBadRequest, "%s", err.Error())
		}
		kb.Source = source
	}
	if req.ClearExpiresAt {
		kb.ExpiresAt = nil
	} else if req.ExpiresAt != nil {
		kb.ExpiresAt = req.ExpiresAt
	}

	if req.ClearDuplicate {
		kb.DuplicateOfID = nil
		kb.DuplicateMarkedAt = nil
	} else if req.DuplicateOfID != nil {
		if err := validateKnowledgeDuplicateTarget(c, kb.ID, req.DuplicateOfID); err != nil {
			return err
		}
		kb.DuplicateMarkedAt = duplicateMarkedAt(req.DuplicateOfID, kb.DuplicateOfID, kb.DuplicateMarkedAt, now)
		kb.DuplicateOfID = req.DuplicateOfID
	}
	return nil
}

func duplicateMarkedAt(next *int, current *int, currentMarkedAt *time.Time, now time.Time) *time.Time {
	if next == nil {
		return nil
	}
	if current != nil && *current == *next {
		return currentMarkedAt
	}
	return &now
}

func validateKnowledgeDuplicateTarget(c *gin.Context, selfID int, targetID *int) error {
	if targetID == nil {
		return nil
	}
	if *targetID <= 0 {
		return newKnowledgeValidationError(http.StatusBadRequest, "duplicate_of_id must be a positive integer")
	}
	if selfID > 0 && *targetID == selfID {
		return newKnowledgeValidationError(http.StatusBadRequest, "knowledge cannot be marked duplicate of itself")
	}

	count, err := DBFromContext(c).NewSelect().
		Model((*models.KnowledgeBase)(nil)).
		Where("id = ?", *targetID).
		Count(c)
	if err != nil {
		return fmt.Errorf("validate duplicate target: %w", err)
	}
	if count == 0 {
		return newKnowledgeValidationError(http.StatusBadRequest, "duplicate_of_id %d not found", *targetID)
	}
	return nil
}

func parseKnowledgeID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge id must be a positive integer"})
		return 0, false
	}
	return id, true
}
