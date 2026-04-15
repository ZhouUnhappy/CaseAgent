package models

import "time"

type Project struct {
	ID          int       `bun:"id,pk,autoincrement" json:"id"`
	Name        string    `bun:"name,notnull" json:"name"`
	Description string    `bun:"description" json:"description"`
	CreatedAt   time.Time `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
