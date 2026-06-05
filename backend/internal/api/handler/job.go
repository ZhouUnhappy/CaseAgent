package handler

import (
	"net/http"
	"strconv"
	"time"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type jobView struct {
	ID          int        `json:"id"`
	TaskID      *int       `json:"task_id,omitempty"`
	DocumentID  *int       `json:"document_id,omitempty"`
	KnowledgeID *int       `json:"knowledge_id,omitempty"`
	JobType     string     `json:"job_type"`
	Status      string     `json:"status"`
	RetryCount  int        `json:"retry_count"`
	MaxRetries  int        `json:"max_retries"`
	LastError   string     `json:"last_error,omitempty"`
	RunAfter    time.Time  `json:"run_after"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (h *Handler) ListJobs(c *gin.Context) {
	query := DBFromContext(c).NewSelect().
		Model((*models.CaseGenerationJob)(nil)).
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

	var jobs []models.CaseGenerationJob
	if err := query.Scan(c, &jobs); err != nil {
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

func applyJobStatusFilter(c *gin.Context, query *bun.SelectQuery, status string) bool {
	switch status {
	case models.JobStatusPending:
		query.Where("status = ?", models.JobStatusPending).Where("retry_count = 0")
	case "retrying":
		query.Where("status = ?", models.JobStatusPending).Where("retry_count > 0")
	case models.JobStatusRunning, models.JobStatusSucceeded, models.JobStatusFailed:
		query.Where("status = ?", status)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported job status"})
		return false
	}
	return true
}

func toJobView(job models.CaseGenerationJob) jobView {
	status := job.Status
	if status == models.JobStatusPending && job.RetryCount > 0 {
		status = "retrying"
	}
	return jobView{
		ID:          job.ID,
		TaskID:      job.TaskID,
		DocumentID:  job.DocumentID,
		KnowledgeID: job.KnowledgeID,
		JobType:     job.JobType,
		Status:      status,
		RetryCount:  job.RetryCount,
		MaxRetries:  job.MaxRetries,
		LastError:   job.LastError,
		RunAfter:    job.RunAfter,
		StartedAt:   job.StartedAt,
		FinishedAt:  job.FinishedAt,
		CreatedAt:   job.CreatedAt,
		UpdatedAt:   job.UpdatedAt,
	}
}
