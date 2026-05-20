package models

import "time"

type Tenant struct {
	ID        int       `bun:"id,pk,autoincrement" json:"id"`
	Slug      string    `bun:"slug,notnull,unique" json:"slug"`
	Name      string    `bun:"name,notnull" json:"name"`
	CreatedAt time.Time `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}
