package models

import "time"

type CaseGenerationTask struct {
	ID               int           `bun:"id,pk,autoincrement" json:"id"`
	ProjectID        int           `bun:"project_id,notnull" json:"project_id"`
	DocumentIDs      []int         `bun:"document_ids,array,notnull" json:"document_ids"`
	AffectedProducts []string      `bun:"affected_products,type:jsonb" json:"affected_products"`
	AffectedModules  []string      `bun:"affected_modules,type:jsonb" json:"affected_modules"`
	Status           string        `bun:"status,nullzero,default:'analyzing'" json:"status"` // 'analyzing', 'awaiting_review', 'generating', 'completed', 'failed'
	CreatedAt        time.Time     `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt        time.Time     `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
