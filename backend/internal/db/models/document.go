package models

import "time"

const (
	DocumentStatusPending    = "pending"
	DocumentStatusProcessing = "processing"
	DocumentStatusCompleted  = "completed"
	DocumentStatusFailed     = "failed"
)

type Document struct {
	ID        int       `bun:"id,pk,autoincrement" json:"id"`
	TenantID  int       `bun:"tenant_id,notnull" json:"tenant_id"`
	ProjectID int       `bun:"project_id,notnull" json:"project_id"`
	Name      string    `bun:"name,notnull" json:"name"`
	Type      string    `bun:"type,notnull" json:"type"`     // 'markdown', 'gdrive'
	Source    string    `bun:"source,notnull" json:"source"` // 'upload', 'gdrive'
	Content   string    `bun:"content,notnull,default:''" json:"content"`
	FileID    string    `bun:"file_id" json:"file_id"`                          // Google Drive file ID
	Status    string    `bun:"status,nullzero,default:'pending'" json:"status"` // 'pending', 'processing', 'completed', 'failed'
	CreatedAt time.Time `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
