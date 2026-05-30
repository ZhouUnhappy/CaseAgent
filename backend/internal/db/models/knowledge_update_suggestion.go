package models

import "time"

const (
	SuggestionStatusPending   = "pending"
	SuggestionStatusAdopted   = "adopted"
	SuggestionStatusDismissed = "dismissed"

	SuggestionCandidateProduct = "product"
	SuggestionCandidateModule  = "module"
)

// KnowledgeUpdateSuggestion records a "knowledge gap" detected during task
// analyze: a name-like token that appeared frequently in the requirements
// but is not covered by any existing knowledge_base entry (retrieval top-1
// score below threshold). Operators can adopt (create a knowledge entry
// from it) or dismiss the suggestion via the API.
type KnowledgeUpdateSuggestion struct {
	ID                  int              `bun:"id,pk,autoincrement" json:"id"`
	TenantID            int              `bun:"tenant_id,notnull" json:"tenant_id"`
	SourceTaskID        int              `bun:"source_task_id,notnull" json:"source_task_id"`
	SourceCaseID        *int             `bun:"source_case_id" json:"source_case_id,omitempty"`
	ResolvedKnowledgeID *int             `bun:"resolved_knowledge_id" json:"resolved_knowledge_id,omitempty"`
	CandidateType       string           `bun:"candidate_type,notnull" json:"candidate_type"`
	CandidateName       string           `bun:"candidate_name,notnull" json:"candidate_name"`
	Frequency           int              `bun:"frequency,notnull,default:0" json:"frequency"`
	SourceSnippets      []map[string]any `bun:"source_snippets,type:jsonb" json:"source_snippets"`
	Status              string           `bun:"status,nullzero,default:'pending'" json:"status"`
	DismissedReason     *string          `bun:"dismissed_reason" json:"dismissed_reason,omitempty"`
	CreatedAt           time.Time        `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt           time.Time        `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
