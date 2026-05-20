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

	ID        int             `bun:"id,pk,autoincrement" json:"id"`
	TenantID  int             `bun:"tenant_id,notnull" json:"tenant_id"`
	Type      string          `bun:"type,notnull" json:"type"` // 'product', 'module'
	Name      string          `bun:"name,notnull" json:"name"`
	Content   string          `bun:"content,notnull" json:"content"`
	Embedding dbvector.Vector `bun:"embedding" json:"embedding"`
	Metadata  map[string]any  `bun:"metadata,type:jsonb" json:"metadata"`
	Status    string          `bun:"status,nullzero,default:'pending'" json:"status"` // pending, processing, completed, failed
	CreatedAt time.Time       `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time       `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
