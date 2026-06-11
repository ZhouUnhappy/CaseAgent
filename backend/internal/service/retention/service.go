package retention

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tenantdb "caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

const (
	ModeDryRun  = "dry_run"
	ModeExecute = "execute"
)

var (
	ErrInvalidInput  = errors.New("invalid retention cleanup input")
	ErrMissingTenant = errors.New("retention cleanup requires tenant context")
)

type Service struct {
	db bun.IDB
}

type Input struct {
	RetentionDays int
	Now           time.Time
	Execute       bool
	OperatorID    string
	OperatorName  string
	Reason        string
}

type Report struct {
	GeneratedAt      time.Time    `json:"generated_at"`
	TenantID         int          `json:"tenant_id"`
	Mode             string       `json:"mode"`
	RetentionDays    int          `json:"retention_days"`
	Cutoff           time.Time    `json:"cutoff"`
	Before           []TargetStat `json:"before"`
	Candidates       []TargetStat `json:"candidates"`
	Deleted          []TargetStat `json:"deleted,omitempty"`
	After            []TargetStat `json:"after,omitempty"`
	Preserved        Preservation `json:"preserved"`
	AuditArtifactID  *int         `json:"audit_artifact_id,omitempty"`
	AuditArtifactErr string       `json:"audit_artifact_error,omitempty"`
}

type TargetStat struct {
	Target        string `json:"target"`
	Operation     string `json:"operation"`
	Rows          int64  `json:"rows"`
	BytesEstimate int64  `json:"bytes_estimate"`
}

type Preservation struct {
	TaskFinalStatus       bool     `json:"task_final_status"`
	InterventionArtifacts bool     `json:"intervention_artifacts"`
	CleanupAuditArtifact  bool     `json:"cleanup_audit_artifact"`
	SourceContextRedacted bool     `json:"source_context_redacted"`
	Notes                 []string `json:"notes"`
}

type statRow struct {
	Rows          int64 `bun:"rows"`
	BytesEstimate int64 `bun:"bytes_estimate"`
}

type targetDefinition struct {
	Name      string
	Operation string
}

func New(db bun.IDB) *Service {
	return &Service{db: db}
}

func (s *Service) Cleanup(ctx context.Context, input Input) (*Report, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok || tenantID <= 0 {
		return nil, ErrMissingTenant
	}
	input, err := normalizeInput(input)
	if err != nil {
		return nil, err
	}

	cutoff := input.Now.AddDate(0, 0, -input.RetentionDays)
	mode := ModeDryRun
	if input.Execute {
		mode = ModeExecute
	}
	report := &Report{
		GeneratedAt:   input.Now,
		TenantID:      tenantID,
		Mode:          mode,
		RetentionDays: input.RetentionDays,
		Cutoff:        cutoff,
		Preserved:     defaultPreservation(input.Execute),
	}

	before, err := s.collectStats(ctx, tenantID, cutoff, statScopeBefore)
	if err != nil {
		return nil, err
	}
	candidates, err := s.collectStats(ctx, tenantID, cutoff, statScopeCandidate)
	if err != nil {
		return nil, err
	}
	report.Before = before
	report.Candidates = candidates

	if !input.Execute {
		return report, nil
	}

	deleted, err := s.executeCleanup(ctx, tenantID, cutoff, candidates, input.Now)
	if err != nil {
		return nil, err
	}
	report.Deleted = deleted
	if id, err := s.recordAuditArtifact(ctx, tenantID, input, report); err != nil {
		report.AuditArtifactErr = err.Error()
		return nil, err
	} else {
		report.AuditArtifactID = &id
	}
	after, err := s.collectStats(ctx, tenantID, cutoff, statScopeBefore)
	if err != nil {
		return nil, err
	}
	report.After = after
	return report, nil
}

func normalizeInput(input Input) (Input, error) {
	if input.RetentionDays <= 0 {
		return input, fmt.Errorf("%w: retention_days must be > 0", ErrInvalidInput)
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}
	if input.Execute && strings.TrimSpace(input.Reason) == "" {
		return input, fmt.Errorf("%w: reason is required for execute", ErrInvalidInput)
	}
	input.OperatorID = strings.TrimSpace(input.OperatorID)
	input.OperatorName = strings.TrimSpace(input.OperatorName)
	input.Reason = strings.TrimSpace(input.Reason)
	return input, nil
}

func defaultPreservation(executed bool) Preservation {
	return Preservation{
		TaskFinalStatus:       true,
		InterventionArtifacts: true,
		CleanupAuditArtifact:  executed,
		SourceContextRedacted: true,
		Notes: []string{
			"case_generation_tasks and test_cases rows are retained",
			"artifacts with artifact_type=intervention are retained",
			"test_cases.source_context is redacted instead of deleting test_cases",
		},
	}
}

type statScope string

const (
	statScopeBefore    statScope = "before"
	statScopeCandidate statScope = "candidate"
)

func cleanupTargets() []targetDefinition {
	return []targetDefinition{
		{Name: "test_case_feedback", Operation: "delete"},
		{Name: "artifacts", Operation: "delete"},
		{Name: "retrieval_runs", Operation: "delete"},
		{Name: "model_calls", Operation: "delete"},
		{Name: "agent_runs", Operation: "delete"},
		{Name: "workflow_steps", Operation: "delete"},
		{Name: "workflow_runs", Operation: "delete"},
		{Name: "test_cases.source_context", Operation: "redact"},
	}
}

func (s *Service) collectStats(ctx context.Context, tenantID int, cutoff time.Time, scope statScope) ([]TargetStat, error) {
	stats := make([]TargetStat, 0, len(cleanupTargets()))
	for _, target := range cleanupTargets() {
		sqlText, args, err := statSQL(target.Name, scope, tenantID, cutoff)
		if err != nil {
			return nil, err
		}
		var row statRow
		if err := s.db.NewRaw(sqlText, args...).Scan(ctx, &row); err != nil {
			return nil, fmt.Errorf("retention stat %s/%s: %w", scope, target.Name, err)
		}
		stats = append(stats, TargetStat{
			Target:        target.Name,
			Operation:     target.Operation,
			Rows:          row.Rows,
			BytesEstimate: row.BytesEstimate,
		})
	}
	return stats, nil
}

func (s *Service) executeCleanup(ctx context.Context, tenantID int, cutoff time.Time, candidates []TargetStat, now time.Time) ([]TargetStat, error) {
	candidateBytes := bytesByTarget(candidates)
	deleted := make([]TargetStat, 0, len(cleanupTargets()))
	for _, target := range cleanupTargets() {
		sqlText, args, err := cleanupSQL(target.Name, tenantID, cutoff, now)
		if err != nil {
			return nil, err
		}
		result, err := s.db.NewRaw(sqlText, args...).Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("retention cleanup %s: %w", target.Name, err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("retention cleanup %s rows affected: %w", target.Name, err)
		}
		deleted = append(deleted, TargetStat{
			Target:        target.Name,
			Operation:     target.Operation,
			Rows:          rows,
			BytesEstimate: candidateBytes[target.Name],
		})
	}
	return deleted, nil
}

func bytesByTarget(stats []TargetStat) map[string]int64 {
	out := make(map[string]int64, len(stats))
	for _, stat := range stats {
		out[stat.Target] = stat.BytesEstimate
	}
	return out
}

func (s *Service) recordAuditArtifact(ctx context.Context, tenantID int, input Input, report *Report) (int, error) {
	artifact := &models.Artifact{
		TenantID:     tenantID,
		ArtifactType: models.ArtifactTypeIntervention,
		ResourceType: "retention",
		Name:         "diagnostic retention cleanup",
		Payload:      cleanupArtifactPayload(input, report),
		CreatedAt:    report.GeneratedAt,
	}
	_, err := s.db.NewInsert().Model(artifact).Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("record retention cleanup audit artifact: %w", err)
	}
	return artifact.ID, nil
}

func cleanupArtifactPayload(input Input, report *Report) map[string]any {
	return map[string]any{
		"action":         "diagnostic_retention_cleanup",
		"mode":           report.Mode,
		"retention_days": report.RetentionDays,
		"cutoff":         report.Cutoff,
		"operator_id":    input.OperatorID,
		"operator_name":  input.OperatorName,
		"reason":         input.Reason,
		"before":         statsPayload(report.Before),
		"candidates":     statsPayload(report.Candidates),
		"deleted":        statsPayload(report.Deleted),
		"preserved":      report.Preserved,
		"generated_at":   report.GeneratedAt,
		"tenant_id":      report.TenantID,
	}
}

func statsPayload(stats []TargetStat) []map[string]any {
	out := make([]map[string]any, 0, len(stats))
	for _, stat := range stats {
		out = append(out, map[string]any{
			"target":         stat.Target,
			"operation":      stat.Operation,
			"rows":           stat.Rows,
			"bytes_estimate": stat.BytesEstimate,
		})
	}
	return out
}

func statSQL(target string, scope statScope, tenantID int, cutoff time.Time) (string, []any, error) {
	if scope == statScopeBefore {
		return beforeStatSQL(target, tenantID)
	}
	if scope == statScopeCandidate {
		return candidateStatSQL(target, tenantID, cutoff)
	}
	return "", nil, fmt.Errorf("unknown retention stat scope %q", scope)
}

func beforeStatSQL(target string, tenantID int) (string, []any, error) {
	switch target {
	case "workflow_runs":
		return tableStatSQL("workflow_runs", "wr", "wr.tenant_id = ?"), []any{tenantID}, nil
	case "workflow_steps":
		return tableStatSQL("workflow_steps", "ws", "ws.tenant_id = ?"), []any{tenantID}, nil
	case "agent_runs":
		return tableStatSQL("agent_runs", "ar", "ar.tenant_id = ?"), []any{tenantID}, nil
	case "model_calls":
		return tableStatSQL("model_calls", "mc", "mc.tenant_id = ?"), []any{tenantID}, nil
	case "retrieval_runs":
		return tableStatSQL("retrieval_runs", "rr", "rr.tenant_id = ?"), []any{tenantID}, nil
	case "artifacts":
		return tableStatSQL("artifacts", "a", "a.tenant_id = ?"), []any{tenantID}, nil
	case "test_case_feedback":
		return tableStatSQL("test_case_feedback", "tcf", "tcf.tenant_id = ?"), []any{tenantID}, nil
	case "test_cases.source_context":
		return `
			SELECT
				COUNT(*) AS rows,
				COALESCE(SUM(pg_column_size(tc.source_context)), 0)::bigint AS bytes_estimate
			FROM test_cases AS tc
			WHERE tc.tenant_id = ?
				AND tc.source_context IS NOT NULL
				AND tc.source_context <> '{}'::jsonb
		`, []any{tenantID}, nil
	default:
		return "", nil, fmt.Errorf("unknown retention target %q", target)
	}
}

func candidateStatSQL(target string, tenantID int, cutoff time.Time) (string, []any, error) {
	sqlText, args, err := candidateWhereSQL(target, tenantID, cutoff)
	if err != nil {
		return "", nil, err
	}
	switch target {
	case "workflow_runs":
		return tableStatSQL("workflow_runs", "wr", sqlText), args, nil
	case "workflow_steps":
		return tableStatSQL("workflow_steps", "ws", sqlText), args, nil
	case "agent_runs":
		return tableStatSQL("agent_runs", "ar", sqlText), args, nil
	case "model_calls":
		return tableStatSQL("model_calls", "mc", sqlText), args, nil
	case "retrieval_runs":
		return tableStatSQL("retrieval_runs", "rr", sqlText), args, nil
	case "artifacts":
		return tableStatSQL("artifacts", "a", sqlText), args, nil
	case "test_case_feedback":
		return tableStatSQL("test_case_feedback", "tcf", sqlText), args, nil
	case "test_cases.source_context":
		return `
			SELECT
				COUNT(*) AS rows,
				COALESCE(SUM(pg_column_size(tc.source_context)), 0)::bigint AS bytes_estimate
			FROM test_cases AS tc
			WHERE ` + sqlText, args, nil
	default:
		return "", nil, fmt.Errorf("unknown retention target %q", target)
	}
}

func tableStatSQL(table string, alias string, where string) string {
	return fmt.Sprintf(`
		SELECT
			COUNT(*) AS rows,
			COALESCE(SUM(pg_column_size(%s)), 0)::bigint AS bytes_estimate
		FROM %s AS %s
		WHERE %s
	`, alias, table, alias, where)
}

func cleanupSQL(target string, tenantID int, cutoff time.Time, now time.Time) (string, []any, error) {
	where, args, err := candidateWhereSQL(target, tenantID, cutoff)
	if err != nil {
		return "", nil, err
	}
	switch target {
	case "workflow_runs":
		return "DELETE FROM workflow_runs AS wr WHERE " + where, args, nil
	case "workflow_steps":
		return "DELETE FROM workflow_steps AS ws WHERE " + where, args, nil
	case "agent_runs":
		return "DELETE FROM agent_runs AS ar WHERE " + where, args, nil
	case "model_calls":
		return "DELETE FROM model_calls AS mc WHERE " + where, args, nil
	case "retrieval_runs":
		return "DELETE FROM retrieval_runs AS rr WHERE " + where, args, nil
	case "artifacts":
		return "DELETE FROM artifacts AS a WHERE " + where, args, nil
	case "test_case_feedback":
		return "DELETE FROM test_case_feedback AS tcf WHERE " + where, args, nil
	case "test_cases.source_context":
		return "UPDATE test_cases AS tc SET source_context = NULL, updated_at = ? WHERE " + where, append([]any{now}, args...), nil
	default:
		return "", nil, fmt.Errorf("unknown retention target %q", target)
	}
}

func candidateWhereSQL(target string, tenantID int, cutoff time.Time) (string, []any, error) {
	switch target {
	case "workflow_runs":
		return terminalWorkflowRunWhere("wr"), []any{tenantID, cutoff}, nil
	case "workflow_steps":
		return fmt.Sprintf(`ws.tenant_id = ? AND ws.workflow_run_id IN (%s)`, terminalWorkflowRunSubquery()), []any{tenantID, tenantID, cutoff}, nil
	case "agent_runs":
		return fmt.Sprintf(`ar.tenant_id = ? AND (
			ar.workflow_run_id IN (%s)
			OR (ar.workflow_run_id IS NULL AND %s)
		)`, terminalWorkflowRunSubquery(), terminalStatusAgeWhere("ar")), []any{tenantID, tenantID, cutoff, cutoff}, nil
	case "model_calls":
		return fmt.Sprintf(`mc.tenant_id = ? AND (
			mc.workflow_run_id IN (%s)
			OR mc.agent_run_id IN (%s)
			OR (mc.workflow_run_id IS NULL AND mc.agent_run_id IS NULL AND %s)
		)`, terminalWorkflowRunSubquery(), terminalAgentRunSubquery(), terminalStatusAgeWhere("mc")), []any{tenantID, tenantID, cutoff, tenantID, tenantID, cutoff, cutoff, cutoff}, nil
	case "retrieval_runs":
		return fmt.Sprintf(`rr.tenant_id = ? AND (
			rr.workflow_run_id IN (%s)
			OR (rr.workflow_run_id IS NULL AND %s)
		)`, terminalWorkflowRunSubquery(), terminalStatusAgeWhere("rr")), []any{tenantID, tenantID, cutoff, cutoff}, nil
	case "artifacts":
		return fmt.Sprintf(`a.tenant_id = ?
			AND a.artifact_type <> ?
			AND (
				a.workflow_run_id IN (%s)
				OR a.workflow_step_id IN (%s)
				OR (a.workflow_run_id IS NULL AND a.workflow_step_id IS NULL AND a.created_at < ?)
			)`, terminalWorkflowRunSubquery(), terminalWorkflowStepSubquery()), []any{tenantID, models.ArtifactTypeIntervention, tenantID, cutoff, tenantID, tenantID, cutoff, cutoff}, nil
	case "test_case_feedback":
		return `tcf.tenant_id = ? AND tcf.created_at < ?`, []any{tenantID, cutoff}, nil
	case "test_cases.source_context":
		return `tc.tenant_id = ?
			AND tc.source_context IS NOT NULL
			AND tc.source_context <> '{}'::jsonb
			AND tc.updated_at < ?
			AND EXISTS (
				SELECT 1
				FROM case_generation_tasks AS task
				WHERE task.tenant_id = ?
					AND task.id = tc.task_id
					AND task.status IN ('completed', 'failed')
					AND task.updated_at < ?
			)`, []any{tenantID, cutoff, tenantID, cutoff}, nil
	default:
		return "", nil, fmt.Errorf("unknown retention target %q", target)
	}
}

func terminalWorkflowRunSubquery() string {
	return "SELECT wr.id FROM workflow_runs AS wr WHERE " + terminalWorkflowRunWhere("wr")
}

func terminalWorkflowStepSubquery() string {
	return fmt.Sprintf(`SELECT ws.id
		FROM workflow_steps AS ws
		WHERE ws.tenant_id = ?
			AND ws.workflow_run_id IN (%s)`, terminalWorkflowRunSubquery())
}

func terminalAgentRunSubquery() string {
	return fmt.Sprintf(`SELECT ar.id
		FROM agent_runs AS ar
		WHERE ar.tenant_id = ?
			AND (
				ar.workflow_run_id IN (%s)
				OR (ar.workflow_run_id IS NULL AND %s)
			)`, terminalWorkflowRunSubquery(), terminalStatusAgeWhere("ar"))
}

func terminalWorkflowRunWhere(alias string) string {
	return fmt.Sprintf(`%s.tenant_id = ?
		AND %s.status IN ('succeeded', 'failed', 'canceled')
		AND COALESCE(%s.finished_at, %s.updated_at, %s.created_at) < ?`, alias, alias, alias, alias, alias)
}

func terminalStatusAgeWhere(alias string) string {
	return fmt.Sprintf(`%s.status IN ('succeeded', 'failed', 'canceled')
		AND COALESCE(%s.finished_at, %s.updated_at, %s.created_at) < ?`, alias, alias, alias, alias)
}
