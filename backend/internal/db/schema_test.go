package db

import (
	"regexp"
	"strings"
	"testing"
)

func TestLoadSchemaSQL(t *testing.T) {
	schema, err := loadSchemaSQL(schemaFilePath(), 1536)
	if err != nil {
		t.Fatalf("loadSchemaSQL() returned error: %v", err)
	}

	for _, table := range []string{
		"caseagent_schema",
		"tenants",
		"projects",
		"documents",
		"document_chunks",
		"knowledge_base",
		"case_generation_tasks",
		"test_cases",
		"knowledge_update_suggestion_groups",
		"knowledge_update_suggestion_occurrences",
		"background_jobs",
		"workflow_runs",
		"workflow_steps",
		"agent_runs",
		"model_calls",
		"retrieval_runs",
		"artifacts",
		"test_case_feedback",
	} {
		if !strings.Contains(schema, "CREATE TABLE "+table) {
			t.Errorf("schema does not define table %s", table)
		}
	}

	if strings.Count(schema, "vector(1536)") != 2 {
		t.Fatalf("schema must render both vector columns with the configured dimensions")
	}
	for _, fragment := range []string{
		"archived_at TIMESTAMP",
		"source_context JSONB",
		"document_id INTEGER REFERENCES documents(id)",
		"knowledge_id INTEGER REFERENCES knowledge_base(id)",
		"workflow_run_id INTEGER",
		"status IN ('pending', 'running', 'succeeded', 'failed', 'canceled')",
		"index_profile VARCHAR(96) NOT NULL",
		"index_version VARCHAR(96) NOT NULL",
		"source VARCHAR(64) NOT NULL DEFAULT 'manual'",
		"duplicate_of_id INTEGER REFERENCES knowledge_base(id) ON DELETE SET NULL",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("schema does not contain final definition %q", fragment)
		}
	}

	for _, table := range []string{
		"projects",
		"documents",
		"document_chunks",
		"knowledge_base",
		"case_generation_tasks",
		"test_cases",
		"knowledge_update_suggestion_groups",
		"knowledge_update_suggestion_occurrences",
		"background_jobs",
		"workflow_runs",
		"workflow_steps",
		"agent_runs",
		"model_calls",
		"retrieval_runs",
		"artifacts",
		"test_case_feedback",
	} {
		if !strings.Contains(schema, "CREATE POLICY "+table+"_tenant_isolation") {
			t.Errorf("schema does not define RLS policy for %s", table)
		}
	}

	for table, columns := range map[string][]string{
		"case_generation_tasks": {"created_at", "updated_at"},
		"background_jobs":       {"run_after", "locked_at", "started_at", "finished_at", "created_at", "updated_at"},
		"workflow_runs":         {"started_at", "finished_at", "created_at", "updated_at"},
		"workflow_steps":        {"started_at", "finished_at", "created_at", "updated_at"},
		"agent_runs":            {"started_at", "finished_at", "created_at", "updated_at"},
		"model_calls":           {"started_at", "finished_at", "created_at", "updated_at"},
		"retrieval_runs":        {"started_at", "finished_at", "created_at", "updated_at"},
		"test_case_feedback":    {"created_at", "updated_at"},
	} {
		requireTimestampWithTimeZoneColumns(t, schema, table, columns)
	}
}

func TestSchemaContainsNoHistoricalMigrationLogic(t *testing.T) {
	schema, err := loadSchemaSQL(schemaFilePath(), 2000)
	if err != nil {
		t.Fatalf("loadSchemaSQL() returned error: %v", err)
	}

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS",
		"ADD COLUMN IF NOT EXISTS",
		"DROP CONSTRAINT IF EXISTS",
		"DROP POLICY IF EXISTS",
		"ALTER COLUMN",
		"'legacy'",
		"timestamp without time zone",
	} {
		if strings.Contains(schema, fragment) {
			t.Errorf("schema baseline contains historical migration fragment %q", fragment)
		}
	}
}

func TestLoadSchemaSQLRejectsInvalidDimensions(t *testing.T) {
	if _, err := loadSchemaSQL(schemaFilePath(), 0); err == nil {
		t.Fatal("expected invalid embedding dimensions to fail")
	}
}

func requireTimestampWithTimeZoneColumns(t *testing.T, schema, table string, columns []string) {
	t.Helper()
	tablePattern := regexp.MustCompile(`(?s)CREATE TABLE ` + regexp.QuoteMeta(table) + ` \((.*?)\n\);`)
	match := tablePattern.FindStringSubmatch(schema)
	if len(match) != 2 {
		t.Fatalf("expected CREATE TABLE block for %s", table)
	}
	for _, column := range columns {
		columnPattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(column) + `\s+TIMESTAMPTZ(?:\s|,)`)
		if !columnPattern.MatchString(match[1]) {
			t.Errorf("expected %s.%s to be declared as TIMESTAMPTZ", table, column)
		}
	}
}
