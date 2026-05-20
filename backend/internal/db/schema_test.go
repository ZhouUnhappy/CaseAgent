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

	if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS tenants") {
		t.Fatalf("expected tenants table definition in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS documents") {
		t.Fatalf("expected documents table definition in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS knowledge_base") {
		t.Fatalf("expected knowledge_base table definition in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "tenant_id INTEGER NOT NULL REFERENCES tenants(id)") {
		t.Fatalf("expected tenant_id columns referencing tenants(id) in schema, got: %s", schema)
	}
}
