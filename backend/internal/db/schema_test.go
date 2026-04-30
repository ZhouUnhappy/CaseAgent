package db

import (
	"strings"
	"testing"
)

func TestLoadSchemaSQL(t *testing.T) {
	schema, err := loadSchemaSQL()
	if err != nil {
		t.Fatalf("loadSchemaSQL() returned error: %v", err)
	}

	if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS documents") {
		t.Fatalf("expected documents table definition in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "ALTER TABLE documents ADD COLUMN IF NOT EXISTS content") {
		t.Fatalf("expected documents content backfill in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS knowledge_base") {
		t.Fatalf("expected knowledge_base table definition in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "ALTER TABLE knowledge_base ADD COLUMN IF NOT EXISTS status") {
		t.Fatalf("expected knowledge status backfill in schema, got: %s", schema)
	}
}
