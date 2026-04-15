package models

import "time"

type DocumentChunk struct {
	ID          int       `bun:"id,pk,autoincrement" json:"id"`
	DocumentID  int       `bun:"document_id,notnull" json:"document_id"`
	Content     string    `bun:"content,notnull" json:"content"`
	Embedding   []float32 `bun:"embedding,array" json:"embedding"`
	ParentDocID int       `bun:"parent_doc_id" json:"parent_doc_id"` // 用于 parent retriever
	Metadata    map[string]any `bun:"metadata,type:jsonb" json:"metadata"`
	CreatedAt   time.Time `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
}
