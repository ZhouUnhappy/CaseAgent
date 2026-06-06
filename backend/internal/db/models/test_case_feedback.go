package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	CaseFeedbackUseful              = "useful"
	CaseFeedbackDuplicate           = "duplicate"
	CaseFeedbackMissingSteps        = "missing_steps"
	CaseFeedbackRequirementMismatch = "requirement_mismatch"
	CaseFeedbackKnowledgeMissing    = "knowledge_missing"
)

type TestCaseFeedback struct {
	bun.BaseModel `bun:"table:test_case_feedback"`

	ID                   int            `bun:"id,pk,autoincrement" json:"id"`
	TenantID             int            `bun:"tenant_id,notnull" json:"tenant_id"`
	TaskID               int            `bun:"task_id,notnull" json:"task_id"`
	TestCaseID           int            `bun:"test_case_id,notnull" json:"test_case_id"`
	CaseIndex            int            `bun:"case_index,notnull" json:"case_index"`
	CaseTitle            string         `bun:"case_title" json:"case_title,omitempty"`
	FeedbackType         string         `bun:"feedback_type,notnull" json:"feedback_type"`
	Note                 string         `bun:"note" json:"note,omitempty"`
	SourceContextSummary map[string]any `bun:"source_context_summary,type:jsonb,nullzero,default:'{}'" json:"source_context_summary,omitempty"`
	PromptID             string         `bun:"prompt_id" json:"prompt_id,omitempty"`
	PromptVersion        string         `bun:"prompt_version" json:"prompt_version,omitempty"`
	ModelCallID          *int           `bun:"model_call_id" json:"model_call_id,omitempty"`
	Metadata             map[string]any `bun:"metadata,type:jsonb,nullzero,default:'{}'" json:"metadata,omitempty"`
	CreatedAt            time.Time      `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt            time.Time      `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
