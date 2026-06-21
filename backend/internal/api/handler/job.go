package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"caseagent/internal/clock"
	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type jobView struct {
	ID            int            `json:"id"`
	TaskID        *int           `json:"task_id,omitempty"`
	DocumentID    *int           `json:"document_id,omitempty"`
	KnowledgeID   *int           `json:"knowledge_id,omitempty"`
	WorkflowRunID *int           `json:"workflow_run_id,omitempty"`
	JobType       string         `json:"job_type"`
	Payload       map[string]any `json:"payload,omitempty"`
	Status        string         `json:"status"`
	RetryCount    int            `json:"retry_count"`
	MaxRetries    int            `json:"max_retries"`
	LastError     string         `json:"last_error,omitempty"`
	RunAfter      time.Time      `json:"run_after"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func (h *Handler) ListJobs(c *gin.Context) {
	query := DBFromContext(c).NewSelect().
		Model((*models.BackgroundJob)(nil)).
		Order("created_at ASC", "id ASC")

	if ok := applyIntJobFilter(c, query, "task_id"); !ok {
		return
	}
	if ok := applyIntJobFilter(c, query, "document_id"); !ok {
		return
	}
	if ok := applyIntJobFilter(c, query, "knowledge_id"); !ok {
		return
	}
	if status := c.Query("status"); status != "" {
		if ok := applyJobStatusFilter(c, query, status); !ok {
			return
		}
	}
	if jobType := c.Query("job_type"); jobType != "" {
		if ok := applyJobTypeFilter(c, query, jobType); !ok {
			return
		}
	}

	var jobs []models.BackgroundJob
	if err := query.Scan(c.Request.Context(), &jobs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	views := make([]jobView, 0, len(jobs))
	for _, job := range jobs {
		views = append(views, toJobView(job))
	}
	c.JSON(http.StatusOK, views)
}

func applyIntJobFilter(c *gin.Context, query *bun.SelectQuery, field string) bool {
	raw := c.Query(field)
	if raw == "" {
		return true
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": field + " must be a positive integer"})
		return false
	}
	query.Where(field+" = ?", id)
	return true
}

func applyJobTypeFilter(c *gin.Context, query *bun.SelectQuery, jobType string) bool {
	for _, known := range models.AllJobTypes {
		if jobType == known {
			query.Where("job_type = ?", jobType)
			return true
		}
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported job_type"})
	return false
}

func applyJobStatusFilter(c *gin.Context, query *bun.SelectQuery, status string) bool {
	switch status {
	case models.JobStatusPending:
		query.Where("status = ?", models.JobStatusPending).Where("retry_count = 0")
	case "retrying":
		query.Where("status = ?", models.JobStatusPending).Where("retry_count > 0")
	case models.JobStatusRunning, models.JobStatusSucceeded, models.JobStatusFailed, models.JobStatusCanceled:
		query.Where("status = ?", status)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported job status"})
		return false
	}
	return true
}

func toJobView(job models.BackgroundJob) jobView {
	status := job.Status
	if status == models.JobStatusPending && job.RetryCount > 0 {
		status = "retrying"
	}
	return jobView{
		ID:            job.ID,
		TaskID:        job.TaskID,
		DocumentID:    job.DocumentID,
		KnowledgeID:   job.KnowledgeID,
		WorkflowRunID: job.WorkflowRunID,
		JobType:       job.JobType,
		Payload:       job.Payload,
		Status:        status,
		RetryCount:    job.RetryCount,
		MaxRetries:    job.MaxRetries,
		LastError:     job.LastError,
		RunAfter:      job.RunAfter,
		StartedAt:     job.StartedAt,
		FinishedAt:    job.FinishedAt,
		CreatedAt:     job.CreatedAt,
		UpdatedAt:     job.UpdatedAt,
	}
}

func (h *Handler) RetryJob(c *gin.Context) {
	operator, ok := parseInterventionRequest(c)
	if !ok {
		return
	}
	job, ok := h.loadJobForAction(c)
	if !ok {
		return
	}
	if !canRetryJob(job.Status) {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("job status %q is not retryable", job.Status)})
		return
	}

	now := clock.Now()
	if _, err := DBFromContext(c).NewUpdate().
		Model((*models.BackgroundJob)(nil)).
		Set("status = ?", models.JobStatusPending).
		Set("retry_count = 0").
		Set("last_error = ''").
		Set("run_after = ?", now).
		Set("locked_at = NULL").
		Set("finished_at = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", job.ID).
		Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := markJobResourceQueued(c, *job, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := recordJobIntervention(c, *job, "retry", operator, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.getJobByID(c, job.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, toJobView(*updated))
}

func (h *Handler) CancelJob(c *gin.Context) {
	operator, ok := parseInterventionRequest(c)
	if !ok {
		return
	}
	job, ok := h.loadJobForAction(c)
	if !ok {
		return
	}
	if !canCancelJob(job.Status) {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("job status %q is not cancelable", job.Status)})
		return
	}

	now := clock.Now()
	if _, err := DBFromContext(c).NewUpdate().
		Model((*models.BackgroundJob)(nil)).
		Set("status = ?", models.JobStatusCanceled).
		Set("last_error = ?", operator.CancelMessage()).
		Set("locked_at = NULL").
		Set("finished_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", job.ID).
		Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := markJobResourceCanceled(c, *job, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := cancelJobWorkflow(c, *job, now, operator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := recordJobIntervention(c, *job, "cancel", operator, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.getJobByID(c, job.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, toJobView(*updated))
}

func (h *Handler) ReplayJob(c *gin.Context) {
	operator, ok := parseInterventionRequest(c)
	if !ok {
		return
	}
	job, ok := h.loadJobForAction(c)
	if !ok {
		return
	}
	if !canReplayJob(job.Status) {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("job status %q is not replayable", job.Status)})
		return
	}

	now := clock.Now()
	payload := clonePayload(job.Payload)
	payload["replayed_from_job_id"] = job.ID
	payload["intervention"] = "replay"
	payload["operator"] = operator.Metadata()
	replay := &models.BackgroundJob{
		TenantID:    job.TenantID,
		TaskID:      job.TaskID,
		DocumentID:  job.DocumentID,
		KnowledgeID: job.KnowledgeID,
		JobType:     job.JobType,
		Payload:     payload,
		Status:      models.JobStatusPending,
		MaxRetries:  job.MaxRetries,
		RunAfter:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := DBFromContext(c).NewInsert().Model(replay).Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("replay job: %v", err)})
		return
	}
	if err := markJobResourceQueued(c, *replay, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := recordJobIntervention(c, *job, "replay", operator, map[string]any{"new_job_id": replay.ID}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, toJobView(*replay))
}

func (h *Handler) loadJobForAction(c *gin.Context) (*models.BackgroundJob, bool) {
	jobID, ok := parsePositiveIDParam(c, "id", "job ID")
	if !ok {
		return nil, false
	}
	job, err := h.getJobByID(c, jobID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return nil, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	return job, true
}

func (h *Handler) getJobByID(c *gin.Context, jobID int) (*models.BackgroundJob, error) {
	job := new(models.BackgroundJob)
	if err := DBFromContext(c).NewSelect().Model(job).Where("id = ?", jobID).Scan(c.Request.Context()); err != nil {
		return nil, err
	}
	return job, nil
}

func parsePositiveIDParam(c *gin.Context, name string, label string) (int, bool) {
	value, err := strconv.Atoi(c.Param(name))
	if err != nil || value <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": label + " must be a positive integer"})
		return 0, false
	}
	return value, true
}

func canRetryJob(status string) bool {
	return status == models.JobStatusFailed || status == models.JobStatusCanceled
}

func canCancelJob(status string) bool {
	return status == models.JobStatusPending || status == models.JobStatusRunning
}

func canReplayJob(status string) bool {
	return status == models.JobStatusSucceeded || status == models.JobStatusFailed || status == models.JobStatusCanceled
}

func markJobResourceQueued(c *gin.Context, job models.BackgroundJob, updatedAt time.Time) error {
	switch job.JobType {
	case models.JobTypeAnalyze:
		if job.TaskID == nil {
			return nil
		}
		_, err := DBFromContext(c).NewUpdate().Model((*models.CaseGenerationTask)(nil)).
			Set("status = ?", models.TaskStatusAnalyzing).
			Set("updated_at = ?", updatedAt).
			Where("id = ?", *job.TaskID).
			Exec(c.Request.Context())
		return err
	case models.JobTypeGenerate:
		if job.TaskID == nil {
			return nil
		}
		_, err := DBFromContext(c).NewUpdate().Model((*models.CaseGenerationTask)(nil)).
			Set("status = ?", models.TaskStatusGenerating).
			Set("updated_at = ?", updatedAt).
			Where("id = ?", *job.TaskID).
			Exec(c.Request.Context())
		return err
	case models.JobTypeDocumentProcess, models.JobTypeDocumentReprocess:
		if job.DocumentID == nil {
			return nil
		}
		_, err := DBFromContext(c).NewUpdate().Model((*models.Document)(nil)).
			Set("status = ?", models.DocumentStatusProcessing).
			Set("updated_at = ?", updatedAt).
			Where("id = ?", *job.DocumentID).
			Exec(c.Request.Context())
		return err
	case models.JobTypeKnowledgeProcess, models.JobTypeKnowledgeReprocess:
		if job.KnowledgeID == nil {
			return nil
		}
		_, err := DBFromContext(c).NewUpdate().Model((*models.KnowledgeBase)(nil)).
			Set("status = ?", models.KnowledgeStatusProcessing).
			Set("updated_at = ?", updatedAt).
			Where("id = ?", *job.KnowledgeID).
			Exec(c.Request.Context())
		return err
	default:
		return nil
	}
}

func markJobResourceCanceled(c *gin.Context, job models.BackgroundJob, updatedAt time.Time) error {
	switch job.JobType {
	case models.JobTypeAnalyze, models.JobTypeGenerate:
		if job.TaskID == nil {
			return nil
		}
		_, err := DBFromContext(c).NewUpdate().Model((*models.CaseGenerationTask)(nil)).
			Set("status = ?", models.TaskStatusFailed).
			Set("updated_at = ?", updatedAt).
			Where("id = ?", *job.TaskID).
			Exec(c.Request.Context())
		return err
	case models.JobTypeDocumentProcess, models.JobTypeDocumentReprocess:
		if job.DocumentID == nil {
			return nil
		}
		_, err := DBFromContext(c).NewUpdate().Model((*models.Document)(nil)).
			Set("status = ?", models.DocumentStatusFailed).
			Set("updated_at = ?", updatedAt).
			Where("id = ?", *job.DocumentID).
			Exec(c.Request.Context())
		return err
	case models.JobTypeKnowledgeProcess, models.JobTypeKnowledgeReprocess:
		if job.KnowledgeID == nil {
			return nil
		}
		_, err := DBFromContext(c).NewUpdate().Model((*models.KnowledgeBase)(nil)).
			Set("status = ?", models.KnowledgeStatusFailed).
			Set("updated_at = ?", updatedAt).
			Where("id = ?", *job.KnowledgeID).
			Exec(c.Request.Context())
		return err
	default:
		return nil
	}
}

func cancelJobWorkflow(c *gin.Context, job models.BackgroundJob, updatedAt time.Time, operator trustedOperator) error {
	if job.WorkflowRunID == nil {
		return nil
	}
	if _, err := DBFromContext(c).NewUpdate().Model((*models.WorkflowRun)(nil)).
		Set("status = ?", models.WorkflowStatusCanceled).
		Set("last_error = ?", operator.CancelMessage()).
		Set("finished_at = ?", updatedAt).
		Set("updated_at = ?", updatedAt).
		Where("id = ?", *job.WorkflowRunID).
		Where("status IN (?)", bun.In([]string{models.WorkflowStatusPending, models.WorkflowStatusRunning})).
		Exec(c.Request.Context()); err != nil {
		return err
	}
	_, err := DBFromContext(c).NewUpdate().Model((*models.WorkflowStep)(nil)).
		Set("status = ?", models.WorkflowStatusCanceled).
		Set("last_error = ?", operator.CancelMessage()).
		Set("finished_at = ?", updatedAt).
		Set("updated_at = ?", updatedAt).
		Where("workflow_run_id = ?", *job.WorkflowRunID).
		Where("status IN (?)", bun.In([]string{models.WorkflowStatusPending, models.WorkflowStatusRunning})).
		Exec(c.Request.Context())
	return err
}

func recordJobIntervention(c *gin.Context, job models.BackgroundJob, action string, operator trustedOperator, extra map[string]any) error {
	tenantID, ok := TenantIDFromContext(c)
	if !ok {
		return fmt.Errorf("record job intervention: missing tenant")
	}
	resourceID := job.ID
	payload := map[string]any{
		"action":        action,
		"job_id":        job.ID,
		"job_type":      job.JobType,
		"status_before": job.Status,
		"operator_id":   operator.ID,
		"operator_name": operator.Name,
		"reason":        operator.Reason,
	}
	for key, value := range extra {
		payload[key] = value
	}
	artifact := &models.Artifact{
		TenantID:      tenantID,
		WorkflowRunID: job.WorkflowRunID,
		ArtifactType:  models.ArtifactTypeIntervention,
		ResourceType:  "job",
		ResourceID:    &resourceID,
		Name:          "job " + action,
		Payload:       payload,
		CreatedAt:     clock.Now(),
	}
	_, err := DBFromContext(c).NewInsert().Model(artifact).Exec(c.Request.Context())
	return err
}

func clonePayload(payload map[string]any) map[string]any {
	next := make(map[string]any, len(payload)+2)
	for key, value := range payload {
		next[key] = value
	}
	return next
}
