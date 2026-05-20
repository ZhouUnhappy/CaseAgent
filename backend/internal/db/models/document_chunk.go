package models

import (
	"time"

	dbvector "caseagent/internal/db/vector"
)

type DocumentChunk struct {
	ID          int             `bun:"id,pk,autoincrement" json:"id"`
	TenantID    int             `bun:"tenant_id,notnull" json:"tenant_id"`
	DocumentID  int             `bun:"document_id,notnull" json:"document_id"`
	Content     string          `bun:"content,notnull" json:"content"`
	Embedding   dbvector.Vector `bun:"embedding" json:"embedding"`
	ParentDocID int             `bun:"parent_doc_id" json:"parent_doc_id"` // 用于 parent retriever
	Metadata    map[string]any  `bun:"metadata,type:jsonb" json:"metadata"`
	CreatedAt   time.Time       `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
}
