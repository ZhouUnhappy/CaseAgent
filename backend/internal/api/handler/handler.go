package handler

import (
	"github.com/uptrace/bun"
)

type Handler struct {
	DB *bun.DB
}

func New(db *bun.DB) *Handler {
	return &Handler{DB: db}
}
