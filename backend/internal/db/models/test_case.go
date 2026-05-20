package models

import (
	"time"
)

const (
	TestCaseStatusDraft     = "draft"
	TestCaseStatusSubmitted = "submitted"
	TestCaseStatusApproved  = "approved"
)

// TestCase persists one section worth of generated test cases.
//
// Cases is stored as a true JSONB array (each element a case object: title,
// priority_id, custom_preconds, custom_steps_separated, affected_products,
// affected_modules, section). SourceContext records the retrieval context
// used to generate this section so each case can be traced back to its
// requirement / knowledge sources.
type TestCase struct {
	ID            int              `bun:"id,pk,autoincrement" json:"id"`
	TenantID      int              `bun:"tenant_id,notnull" json:"tenant_id"`
	TaskID        int              `bun:"task_id,notnull" json:"task_id"`
	Section       string           `bun:"section,notnull" json:"section"`
	Cases         []map[string]any `bun:"cases,notnull,type:jsonb" json:"cases"`
	SourceContext map[string]any   `bun:"source_context,type:jsonb,nullzero" json:"source_context,omitempty"`
	Status        string           `bun:"status,nullzero,default:'draft'" json:"status"`
	CreatedAt     time.Time        `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time        `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
