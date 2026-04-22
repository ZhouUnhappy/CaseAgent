package models

import "time"

const (
	TestCaseStatusDraft     = "draft"
	TestCaseStatusSubmitted = "submitted"
	TestCaseStatusApproved  = "approved"
)

type TestCase struct {
	ID        int       `bun:"id,pk,autoincrement" json:"id"`
	TaskID    int       `bun:"task_id,notnull" json:"task_id"`
	Section   string    `bun:"section,notnull" json:"section"`
	Cases     string    `bun:"cases,notnull,type:jsonb" json:"cases"`         // JSON string
	Status    string    `bun:"status,nullzero,default:'draft'" json:"status"` // draft, submitted, approved
	CreatedAt time.Time `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
