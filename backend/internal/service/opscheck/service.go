package opscheck

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"caseagent/internal/config"
	tenantdb "caseagent/internal/db"
	"caseagent/internal/indexing"
	maintenanceservice "caseagent/internal/service/maintenance"

	"github.com/uptrace/bun"
)

const (
	StatusPass = "pass"
	StatusWarn = "warn"
	StatusFail = "fail"
)

type Service struct {
	db  bun.IDB
	cfg *config.Config
}

type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Overall     string    `json:"overall"`
	TenantID    int       `json:"tenant_id,omitempty"`
	Checks      []Check   `json:"checks"`
}

type Check struct {
	ID       string         `json:"id"`
	Label    string         `json:"label"`
	Status   string         `json:"status"`
	Message  string         `json:"message"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type roleRow struct {
	User      string `bun:"user"`
	Superuser bool   `bun:"superuser"`
	BypassRLS bool   `bun:"bypass_rls"`
}

type rlsRow struct {
	TableName string `bun:"table_name"`
	Exists    bool   `bun:"exists"`
	Enabled   bool   `bun:"enabled"`
	Forced    bool   `bun:"forced"`
}

func New(db bun.IDB, cfg *config.Config) *Service {
	return &Service{db: db, cfg: cfg}
}

func (s *Service) Get(ctx context.Context) (*Report, error) {
	tenantID, _ := tenantdb.TenantFromContext(ctx)
	checks := []Check{
		s.checkTenantContext(ctx, tenantID),
		s.checkRole(ctx),
		s.checkPgvector(ctx),
		s.checkBusinessTableRLS(ctx),
		s.checkConfig(),
		s.checkVectorHealth(ctx),
	}

	return &Report{
		GeneratedAt: time.Now(),
		Overall:     OverallStatus(checks),
		TenantID:    tenantID,
		Checks:      checks,
	}, nil
}

func OverallStatus(checks []Check) string {
	overall := StatusPass
	for _, check := range checks {
		switch check.Status {
		case StatusFail:
			return StatusFail
		case StatusWarn:
			overall = StatusWarn
		}
	}
	return overall
}

func (s *Service) checkTenantContext(ctx context.Context, tenantID int) Check {
	if tenantID <= 0 {
		return fail("tenant_context", "Tenant context", "request has no tenant context", nil)
	}

	var row struct {
		TenantSetting string `bun:"tenant_setting"`
	}
	err := s.db.NewRaw(`SELECT current_setting('app.tenant_id', true) AS tenant_setting`).Scan(ctx, &row)
	if err != nil {
		return fail("tenant_context", "Tenant context", fmt.Sprintf("failed to read app.tenant_id: %v", err), nil)
	}
	if row.TenantSetting != strconv.Itoa(tenantID) {
		return fail("tenant_context", "Tenant context", "database app.tenant_id does not match request tenant", map[string]any{
			"request_tenant_id": tenantID,
			"app_tenant_id":     row.TenantSetting,
		})
	}
	return pass("tenant_context", "Tenant context", "request tenant and database tenant setting match", map[string]any{
		"tenant_id": tenantID,
	})
}

func (s *Service) checkRole(ctx context.Context) Check {
	var row roleRow
	err := s.db.NewRaw(`
		SELECT current_user AS "user", rolsuper AS superuser, rolbypassrls AS bypass_rls
		FROM pg_roles
		WHERE rolname = current_user
	`).Scan(ctx, &row)
	if err != nil {
		return fail("database_role", "Database role", fmt.Sprintf("failed to inspect current role: %v", err), nil)
	}
	meta := map[string]any{"user": row.User, "superuser": row.Superuser, "bypass_rls": row.BypassRLS}
	if row.Superuser || row.BypassRLS {
		return fail("database_role", "Database role", "current database role can bypass RLS", meta)
	}
	return pass("database_role", "Database role", "current database role is subject to RLS", meta)
}

func (s *Service) checkPgvector(ctx context.Context) Check {
	var installed bool
	err := s.db.NewRaw(`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')`).Scan(ctx, &installed)
	if err != nil {
		return fail("pgvector", "pgvector", fmt.Sprintf("failed to inspect pgvector extension: %v", err), nil)
	}
	if !installed {
		return fail("pgvector", "pgvector", "pgvector extension is not installed", nil)
	}
	return pass("pgvector", "pgvector", "pgvector extension is installed", nil)
}

func (s *Service) checkBusinessTableRLS(ctx context.Context) Check {
	names := businessRLSTables()
	rows := make([]rlsRow, 0, len(names))
	err := s.db.NewRaw(`
		WITH expected(table_name) AS (
			VALUES `+tableNamesValuesSQL(names)+`
		)
		SELECT
			expected.table_name,
			cls.oid IS NOT NULL AS "exists",
			COALESCE(cls.relrowsecurity, false) AS enabled,
			COALESCE(cls.relforcerowsecurity, false) AS forced
		FROM expected
		LEFT JOIN pg_class AS cls
			ON cls.relname = expected.table_name
			AND cls.relnamespace = 'public'::regnamespace
		ORDER BY expected.table_name
	`).Scan(ctx, &rows)
	if err != nil {
		return fail("business_table_rls", "Business table RLS", fmt.Sprintf("failed to inspect RLS tables: %v", err), nil)
	}

	missing := []string{}
	weak := []string{}
	for _, row := range rows {
		switch {
		case !row.Exists:
			missing = append(missing, row.TableName)
		case !row.Enabled || !row.Forced:
			weak = append(weak, row.TableName)
		}
	}
	meta := map[string]any{"table_count": len(names), "missing_tables": missing, "rls_not_forced": weak}
	if len(missing) > 0 || len(weak) > 0 {
		return fail("business_table_rls", "Business table RLS", "one or more business tables are missing RLS or FORCE RLS", meta)
	}
	return pass("business_table_rls", "Business table RLS", "business tables have RLS and FORCE RLS enabled", meta)
}

func tableNamesValuesSQL(names []string) string {
	values := make([]string, 0, len(names))
	for _, name := range names {
		values = append(values, "('"+strings.ReplaceAll(name, "'", "''")+"')")
	}
	return strings.Join(values, ",")
}

func (s *Service) checkConfig() Check {
	if s.cfg == nil {
		return warn("config_loaded", "Runtime config", "runtime config is not loaded in process", nil)
	}
	meta := map[string]any{
		"chat_provider":              s.cfg.Model.Chat.Provider,
		"chat_model":                 s.cfg.Model.Chat.Model,
		"chat_timeout_seconds":       s.cfg.Model.Chat.RequestTimeoutSeconds,
		"chat_has_fallback":          strings.TrimSpace(s.cfg.Model.Chat.Fallback.Provider) != "",
		"embedding_provider":         s.cfg.Model.Embedding.Provider,
		"embedding_model":            s.cfg.Model.Embedding.Model,
		"embedding_dimensions":       s.cfg.Model.Embedding.Dimensions,
		"job_runner_max_concurrency": s.cfg.JobRunner.MaxConcurrency,
		"tenant_max_concurrency":     s.cfg.JobRunner.TenantMaxConcurrency,
	}
	if err := config.Validate(s.cfg); err != nil {
		return fail("config_loaded", "Runtime config", err.Error(), meta)
	}
	return pass("config_loaded", "Runtime config", "runtime config is loaded and validates", meta)
}

func (s *Service) checkVectorHealth(ctx context.Context) Check {
	report, err := maintenanceservice.New(s.db).VectorHealth(ctx)
	if err != nil {
		return fail("vector_health", "Vector health", fmt.Sprintf("failed to inspect vector health: %v", err), nil)
	}
	docIssues := len(report.Documents.ReprocessableIDs) + len(report.Documents.BlockedIDs) + len(report.Documents.ProcessingIDs)
	knowledgeIssues := len(report.Knowledge.ReprocessableIDs) + len(report.Knowledge.BlockedIDs) + len(report.Knowledge.ProcessingIDs)
	meta := map[string]any{
		"profile":                         report.Profile,
		"dimensions":                      report.Dimensions,
		"documents_total":                 report.Documents.Total,
		"documents_reprocessable_count":   len(report.Documents.ReprocessableIDs),
		"documents_blocked_count":         len(report.Documents.BlockedIDs),
		"documents_processing_count":      len(report.Documents.ProcessingIDs),
		"knowledge_total":                 report.Knowledge.Total,
		"knowledge_reprocessable_count":   len(report.Knowledge.ReprocessableIDs),
		"knowledge_blocked_count":         len(report.Knowledge.BlockedIDs),
		"knowledge_processing_count":      len(report.Knowledge.ProcessingIDs),
		"current_index_profile_name":      indexing.CurrentProfile().Name,
		"current_index_profile_version":   indexing.CurrentProfile().Version,
		"current_index_profile_dimension": indexing.CurrentProfile().Dimensions,
	}
	if docIssues > 0 || knowledgeIssues > 0 {
		return warn("vector_health", "Vector health", "some documents or knowledge rows need processing/reindex", meta)
	}
	return pass("vector_health", "Vector health", "vectors are healthy for current tenant", meta)
}

func businessRLSTables() []string {
	return []string{
		"projects",
		"documents",
		"document_chunks",
		"knowledge_base",
		"case_generation_tasks",
		"background_jobs",
		"workflow_runs",
		"workflow_steps",
		"agent_runs",
		"model_calls",
		"retrieval_runs",
		"artifacts",
		"test_cases",
		"knowledge_update_suggestion_groups",
		"knowledge_update_suggestion_occurrences",
		"test_case_feedback",
	}
}

func pass(id string, label string, message string, metadata map[string]any) Check {
	return Check{ID: id, Label: label, Status: StatusPass, Message: message, Metadata: metadata}
}

func warn(id string, label string, message string, metadata map[string]any) Check {
	return Check{ID: id, Label: label, Status: StatusWarn, Message: message, Metadata: metadata}
}

func fail(id string, label string, message string, metadata map[string]any) Check {
	return Check{ID: id, Label: label, Status: StatusFail, Message: message, Metadata: metadata}
}
