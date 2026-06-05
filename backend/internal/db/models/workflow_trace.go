package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	WorkflowStatusPending   = "pending"
	WorkflowStatusRunning   = "running"
	WorkflowStatusSucceeded = "succeeded"
	WorkflowStatusFailed    = "failed"
	WorkflowStatusCanceled  = "canceled"

	ArtifactTypeInput          = "input"
	ArtifactTypeOutput         = "output"
	ArtifactTypeGeneratedCases = "generated_cases"
	ArtifactTypeRetrievalTrace = "retrieval_trace"
)

type WorkflowRun struct {
	bun.BaseModel `bun:"table:workflow_runs"`

	ID           int            `bun:"id,pk,autoincrement" json:"id"`
	TenantID     int            `bun:"tenant_id,notnull" json:"tenant_id"`
	WorkflowType string         `bun:"workflow_type,notnull" json:"workflow_type"`
	ResourceType string         `bun:"resource_type,notnull" json:"resource_type"`
	ResourceID   int            `bun:"resource_id,notnull" json:"resource_id"`
	JobID        *int           `bun:"job_id" json:"job_id,omitempty"`
	Status       string         `bun:"status,nullzero,default:'pending'" json:"status"`
	LastError    string         `bun:"last_error" json:"last_error,omitempty"`
	Metadata     map[string]any `bun:"metadata,type:jsonb,nullzero,default:'{}'" json:"metadata,omitempty"`
	StartedAt    *time.Time     `bun:"started_at" json:"started_at,omitempty"`
	FinishedAt   *time.Time     `bun:"finished_at" json:"finished_at,omitempty"`
	CreatedAt    time.Time      `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt    time.Time      `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}

type WorkflowStep struct {
	bun.BaseModel `bun:"table:workflow_steps"`

	ID            int            `bun:"id,pk,autoincrement" json:"id"`
	TenantID      int            `bun:"tenant_id,notnull" json:"tenant_id"`
	WorkflowRunID int            `bun:"workflow_run_id,notnull" json:"workflow_run_id"`
	StepType      string         `bun:"step_type,notnull" json:"step_type"`
	Status        string         `bun:"status,nullzero,default:'pending'" json:"status"`
	LastError     string         `bun:"last_error" json:"last_error,omitempty"`
	Metadata      map[string]any `bun:"metadata,type:jsonb,nullzero,default:'{}'" json:"metadata,omitempty"`
	StartedAt     *time.Time     `bun:"started_at" json:"started_at,omitempty"`
	FinishedAt    *time.Time     `bun:"finished_at" json:"finished_at,omitempty"`
	CreatedAt     time.Time      `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time      `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}

type AgentRun struct {
	bun.BaseModel `bun:"table:agent_runs"`

	ID            int            `bun:"id,pk,autoincrement" json:"id"`
	TenantID      int            `bun:"tenant_id,notnull" json:"tenant_id"`
	WorkflowRunID *int           `bun:"workflow_run_id" json:"workflow_run_id,omitempty"`
	TaskID        *int           `bun:"task_id" json:"task_id,omitempty"`
	AgentName     string         `bun:"agent_name,notnull" json:"agent_name"`
	Stage         string         `bun:"stage,notnull" json:"stage"`
	Status        string         `bun:"status,nullzero,default:'running'" json:"status"`
	InputSummary  string         `bun:"input_summary" json:"input_summary,omitempty"`
	OutputSummary string         `bun:"output_summary" json:"output_summary,omitempty"`
	LastError     string         `bun:"last_error" json:"last_error,omitempty"`
	Metadata      map[string]any `bun:"metadata,type:jsonb,nullzero,default:'{}'" json:"metadata,omitempty"`
	StartedAt     *time.Time     `bun:"started_at" json:"started_at,omitempty"`
	FinishedAt    *time.Time     `bun:"finished_at" json:"finished_at,omitempty"`
	CreatedAt     time.Time      `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time      `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}

type ModelCall struct {
	bun.BaseModel `bun:"table:model_calls"`

	ID            int            `bun:"id,pk,autoincrement" json:"id"`
	TenantID      int            `bun:"tenant_id,notnull" json:"tenant_id"`
	WorkflowRunID *int           `bun:"workflow_run_id" json:"workflow_run_id,omitempty"`
	AgentRunID    *int           `bun:"agent_run_id" json:"agent_run_id,omitempty"`
	Provider      string         `bun:"provider" json:"provider,omitempty"`
	Model         string         `bun:"model" json:"model,omitempty"`
	Status        string         `bun:"status,nullzero,default:'running'" json:"status"`
	PromptChars   int            `bun:"prompt_chars,notnull" json:"prompt_chars"`
	ResponseChars int            `bun:"response_chars,notnull" json:"response_chars"`
	LastError     string         `bun:"last_error" json:"last_error,omitempty"`
	Metadata      map[string]any `bun:"metadata,type:jsonb,nullzero,default:'{}'" json:"metadata,omitempty"`
	StartedAt     *time.Time     `bun:"started_at" json:"started_at,omitempty"`
	FinishedAt    *time.Time     `bun:"finished_at" json:"finished_at,omitempty"`
	CreatedAt     time.Time      `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time      `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}

type RetrievalRun struct {
	bun.BaseModel `bun:"table:retrieval_runs"`

	ID            int            `bun:"id,pk,autoincrement" json:"id"`
	TenantID      int            `bun:"tenant_id,notnull" json:"tenant_id"`
	WorkflowRunID *int           `bun:"workflow_run_id" json:"workflow_run_id,omitempty"`
	TaskID        *int           `bun:"task_id" json:"task_id,omitempty"`
	RetrieverType string         `bun:"retriever_type,notnull" json:"retriever_type"`
	Status        string         `bun:"status,nullzero,default:'succeeded'" json:"status"`
	QueryCount    int            `bun:"query_count,notnull" json:"query_count"`
	HitCount      int            `bun:"hit_count,notnull" json:"hit_count"`
	LastError     string         `bun:"last_error" json:"last_error,omitempty"`
	Metadata      map[string]any `bun:"metadata,type:jsonb,nullzero,default:'{}'" json:"metadata,omitempty"`
	StartedAt     *time.Time     `bun:"started_at" json:"started_at,omitempty"`
	FinishedAt    *time.Time     `bun:"finished_at" json:"finished_at,omitempty"`
	CreatedAt     time.Time      `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time      `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}

type Artifact struct {
	bun.BaseModel `bun:"table:artifacts"`

	ID             int            `bun:"id,pk,autoincrement" json:"id"`
	TenantID       int            `bun:"tenant_id,notnull" json:"tenant_id"`
	WorkflowRunID  *int           `bun:"workflow_run_id" json:"workflow_run_id,omitempty"`
	WorkflowStepID *int           `bun:"workflow_step_id" json:"workflow_step_id,omitempty"`
	ArtifactType   string         `bun:"artifact_type,notnull" json:"artifact_type"`
	ResourceType   string         `bun:"resource_type" json:"resource_type,omitempty"`
	ResourceID     *int           `bun:"resource_id" json:"resource_id,omitempty"`
	Name           string         `bun:"name" json:"name,omitempty"`
	Content        string         `bun:"content" json:"content,omitempty"`
	Payload        map[string]any `bun:"payload,type:jsonb,nullzero,default:'{}'" json:"payload,omitempty"`
	CreatedAt      time.Time      `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
}
