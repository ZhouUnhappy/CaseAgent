package models

import (
	"time"

	dbvector "caseagent/internal/db/vector"

	"github.com/uptrace/bun"
)

const (
	KnowledgeStatusPending    = "pending"
	KnowledgeStatusProcessing = "processing"
	KnowledgeStatusCompleted  = "completed"
	KnowledgeStatusFailed     = "failed"
)

type KnowledgeBase struct {
	bun.BaseModel `bun:"table:knowledge_base"`

	ID                int             `bun:"id,pk,autoincrement" json:"id"`
	TenantID          int             `bun:"tenant_id,notnull" json:"tenant_id"`
	Type              string          `bun:"type,notnull" json:"type"` // 'product', 'module'
	Name              string          `bun:"name,notnull" json:"name"`
	Content           string          `bun:"content,notnull" json:"content"`
	Embedding         dbvector.Vector `bun:"embedding" json:"embedding"`
	Metadata          map[string]any  `bun:"metadata,type:jsonb" json:"metadata"`
	Source            string          `bun:"source,notnull,default:'manual'" json:"source"`
	ExpiresAt         *time.Time      `bun:"expires_at,nullzero" json:"expires_at,omitempty"`
	DuplicateOfID     *int            `bun:"duplicate_of_id,nullzero" json:"duplicate_of_id,omitempty"`
	DuplicateMarkedAt *time.Time      `bun:"duplicate_marked_at,nullzero" json:"duplicate_marked_at,omitempty"`
	IndexProfile      string          `bun:"index_profile,notnull,default:'legacy'" json:"index_profile"`
	IndexVersion      string          `bun:"index_version,notnull,default:'legacy'" json:"index_version"`
	Status            string          `bun:"status,nullzero,default:'pending'" json:"status"` // pending, processing, completed, failed
	CreatedAt         time.Time       `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt         time.Time       `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
