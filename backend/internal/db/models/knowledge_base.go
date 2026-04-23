package models

import (
	"time"

	dbvector "caseagent/internal/db/vector"

	"github.com/uptrace/bun"
)

type KnowledgeBase struct {
	bun.BaseModel `bun:"table:knowledge_base"`

	ID        int             `bun:"id,pk,autoincrement" json:"id"`
	Type      string          `bun:"type,notnull" json:"type"` // 'product', 'module'
	Name      string          `bun:"name,notnull" json:"name"`
	Content   string          `bun:"content,notnull" json:"content"`
	Embedding dbvector.Vector `bun:"embedding" json:"embedding"`
	Metadata  map[string]any  `bun:"metadata,type:jsonb" json:"metadata"`
	CreatedAt time.Time       `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time       `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
