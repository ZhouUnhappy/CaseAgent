package models

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestKnowledgeBaseUsesSingularTableName(t *testing.T) {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN("postgres://localhost/test?sslmode=disable")))
	defer sqldb.Close()

	db := bun.NewDB(sqldb, pgdialect.New())
	defer db.Close()

	query := db.NewSelect().Model((*KnowledgeBase)(nil)).String()
	if !strings.Contains(query, "\"knowledge_base\"") {
		t.Fatalf("expected query to target knowledge_base table, got %s", query)
	}
}
