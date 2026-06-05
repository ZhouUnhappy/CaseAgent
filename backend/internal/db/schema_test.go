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
	if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS background_jobs") {
		t.Fatalf("expected background jobs table definition in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "background_jobs_tenant_isolation") {
		t.Fatalf("expected background jobs RLS policy in schema, got: %s", schema)
	}
	for _, table := range []string{
		"workflow_runs",
		"workflow_steps",
		"agent_runs",
		"model_calls",
		"retrieval_runs",
		"artifacts",
	} {
		if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("expected %s table definition in schema, got: %s", table, schema)
		}
		if !strings.Contains(schema, table+"_tenant_isolation") {
			t.Fatalf("expected %s RLS policy in schema, got: %s", table, schema)
		}
	}
	if !strings.Contains(schema, "ADD COLUMN IF NOT EXISTS document_id") ||
		!strings.Contains(schema, "ADD COLUMN IF NOT EXISTS knowledge_id") ||
		!strings.Contains(schema, "ADD COLUMN IF NOT EXISTS payload JSONB") ||
		!strings.Contains(schema, "ADD COLUMN IF NOT EXISTS workflow_run_id") {
		t.Fatalf("expected generic background job columns in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "ADD CONSTRAINT background_jobs_status_check\n        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled'))") {
		t.Fatalf("expected canceled background job status in schema, got: %s", schema)
	}
	if !strings.Contains(schema, "ADD COLUMN IF NOT EXISTS index_profile") ||
		!strings.Contains(schema, "ADD COLUMN IF NOT EXISTS index_version") ||
		!strings.Contains(schema, "document_chunks_index_profile_idx") ||
		!strings.Contains(schema, "knowledge_base_index_profile_idx") {
		t.Fatalf("expected index profile columns and indexes in schema, got: %s", schema)
	}
}
