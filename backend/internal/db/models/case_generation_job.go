package models

import "time"

const (
	JobTypeAnalyze            = "analyze"
	JobTypeGenerate           = "generate"
	JobTypeDocumentProcess    = "document_process"
	JobTypeDocumentReprocess  = "document_reprocess"
	JobTypeKnowledgeProcess   = "knowledge_process"
	JobTypeKnowledgeReprocess = "knowledge_reprocess"

	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusSucceeded = "succeeded"
	JobStatusFailed    = "failed"
)

var AllJobTypes = []string{
	JobTypeAnalyze,
	JobTypeGenerate,
	JobTypeDocumentProcess,
	JobTypeDocumentReprocess,
	JobTypeKnowledgeProcess,
	JobTypeKnowledgeReprocess,
}

type CaseGenerationJob struct {
	ID          int            `bun:"id,pk,autoincrement" json:"id"`
	TenantID    int            `bun:"tenant_id,notnull" json:"tenant_id"`
	TaskID      *int           `bun:"task_id" json:"task_id,omitempty"`
	DocumentID  *int           `bun:"document_id" json:"document_id,omitempty"`
	KnowledgeID *int           `bun:"knowledge_id" json:"knowledge_id,omitempty"`
	JobType     string         `bun:"job_type,notnull" json:"job_type"`
	Payload     map[string]any `bun:"payload,type:jsonb,nullzero,default:'{}'" json:"payload,omitempty"`
	Status      string         `bun:"status,nullzero,default:'pending'" json:"status"`
	RetryCount  int            `bun:"retry_count,notnull" json:"retry_count"`
	MaxRetries  int            `bun:"max_retries,notnull" json:"max_retries"`
	LastError   string         `bun:"last_error" json:"last_error,omitempty"`
	RunAfter    time.Time      `bun:"run_after,nullzero,default:current_timestamp" json:"run_after"`
	LockedAt    *time.Time     `bun:"locked_at" json:"locked_at,omitempty"`
	StartedAt   *time.Time     `bun:"started_at" json:"started_at,omitempty"`
	FinishedAt  *time.Time     `bun:"finished_at" json:"finished_at,omitempty"`
	CreatedAt   time.Time      `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time      `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
