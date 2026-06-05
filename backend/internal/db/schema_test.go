package db

import (
	"strings"
	"testing"
)

func TestLoadSchemaSQL(t *testing.T) {
	files, err := schemaFilePaths()
	if err != nil {
		t.Fatalf("schemaFilePaths() returned error: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("expected at least 2 schema files, got %d", len(files))
	}

	var schemas []string
	for _, file := range files {
		schema, err := loadSchemaSQL(file)
		if err != nil {
			t.Fatalf("loadSchemaSQL(%q) returned error: %v", file, err)
		}
		schemas = append(schemas, schema)
	}
	schema := strings.Join(schemas, "\n")

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
	if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS knowledge_update_suggestion_groups") {
		t.Fatalf("expected suggestion group table definition in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS knowledge_update_suggestion_occurrences") {
		t.Fatalf("expected suggestion occurrence table definition in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS case_generation_jobs") {
		t.Fatalf("expected case generation jobs table definition in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "case_generation_jobs_tenant_isolation") {
		t.Fatalf("expected case generation jobs RLS policy in schema, got: %s", schema)
	}
}
