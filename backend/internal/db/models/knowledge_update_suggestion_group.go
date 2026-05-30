package models

import "time"

// KnowledgeUpdateSuggestionGroup is the review-level aggregate for a repeated
// knowledge gap candidate. Individual sightings are kept in
// KnowledgeUpdateSuggestionOccurrence.
type KnowledgeUpdateSuggestionGroup struct {
	ID                  int       `bun:"id,pk,autoincrement" json:"id"`
	TenantID            int       `bun:"tenant_id,notnull" json:"tenant_id"`
	CandidateType       string    `bun:"candidate_type,notnull" json:"candidate_type"`
	CandidateName       string    `bun:"candidate_name,notnull" json:"candidate_name"`
	TotalFrequency      int       `bun:"total_frequency,notnull,default:0" json:"total_frequency"`
	TaskCount           int       `bun:"task_count,notnull,default:0" json:"task_count"`
	Status              string    `bun:"status,nullzero,default:'pending'" json:"status"`
	DismissedReason     *string   `bun:"dismissed_reason" json:"dismissed_reason,omitempty"`
	ResolvedKnowledgeID *int      `bun:"resolved_knowledge_id" json:"resolved_knowledge_id,omitempty"`
	FirstSeenAt         time.Time `bun:"first_seen_at,nullzero,default:current_timestamp" json:"first_seen_at"`
	LastSeenAt          time.Time `bun:"last_seen_at,nullzero,default:current_timestamp" json:"last_seen_at"`
	CreatedAt           time.Time `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt           time.Time `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}

type KnowledgeUpdateSuggestionOccurrence struct {
	ID             int              `bun:"id,pk,autoincrement" json:"id"`
	TenantID       int              `bun:"tenant_id,notnull" json:"tenant_id"`
	GroupID        int              `bun:"group_id,notnull" json:"group_id"`
	SourceTaskID   int              `bun:"source_task_id,notnull" json:"source_task_id"`
	SourceCaseID   *int             `bun:"source_case_id" json:"source_case_id,omitempty"`
	Frequency      int              `bun:"frequency,notnull,default:0" json:"frequency"`
	SourceSnippets []map[string]any `bun:"source_snippets,type:jsonb" json:"source_snippets"`
	CreatedAt      time.Time        `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
}
