// Package suggestion produces "knowledge gap" candidates that the operator
// can review in the workbench. Detection is invoked from taskservice after
// AnalyzeTask completes: requirement-level identifier candidates that are
// NOT already covered by an existing knowledge_base entry land in the
// knowledge_update_suggestions table with status='pending'.
//
// The extraction (extractor.go) is intentionally a pure function with no DB
// dependency so it can be unit-tested. The DB-bound classification logic
// (score threshold, persistence) lives here.
package suggestion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"caseagent/internal/db"
	"caseagent/internal/db/models"
	retrievalservice "caseagent/internal/service/retrieval"

	"github.com/uptrace/bun"
)

// MissingScoreThreshold: 候选用 retrieval top-1 与已知 knowledge 比对，
// score（cosine 相似度）严格小于该阈值视为「未覆盖」，写成 suggestion。
// 0.5 是一个折中：低于此值通常意味着不是同义表达。
const MissingScoreThreshold = 0.5

// MaxCandidatesPerTask: 单个 task 最多落多少条 suggestion，避免一篇噪声
// 大的需求文档把表淹没。
const MaxCandidatesPerTask = 20

const AutoExpiredDismissedReason = "auto_expired"

type Service struct {
	db             bun.IDB
	retrieval      *retrievalservice.Service
	threshold      float64
	draftGenerator DraftGenerator
}

type ManualSuggestionInput struct {
	CandidateType   string
	CandidateName   string
	SourceTaskID    int
	SourceCaseID    int
	SourceCaseTitle string
	Note            string
}

var ErrInvalidManualSuggestion = errors.New("invalid manual suggestion")

func New(db bun.IDB) *Service {
	return &Service{
		db:             db,
		retrieval:      retrievalservice.New(db),
		threshold:      MissingScoreThreshold,
		draftGenerator: draftGeneratorFunc(newConfiguredDraftGenerator),
	}
}

// RecordCandidates 把当前任务的 requirements 中未被现有 knowledge 覆盖的
// 候选写成 pending suggestion。已存在（同 task + 同 name + 同 type）的不重复
// 插入。安全可重入。
//
// inferredProducts / inferredModules 用于排除已识别的影响范围；它们不会
// 被记为「缺失」。
func (s *Service) RecordCandidates(
	ctx context.Context,
	taskID int,
	requirements string,
	inferredProducts, inferredModules []string,
) error {
	exclude := append([]string{}, inferredProducts...)
	exclude = append(exclude, inferredModules...)
	candidates := ExtractCandidates(requirements, exclude)
	if len(candidates) == 0 {
		return nil
	}

	recorded := 0
	for _, c := range candidates {
		if recorded >= MaxCandidatesPerTask {
			break
		}
		ok, candidateType, err := s.classify(ctx, c.Name)
		if err != nil {
			slog.Warn("knowledge suggestion classify failed; skipping candidate",
				"task_id", taskID, "candidate", c.Name, "error", err)
			continue
		}
		if !ok {
			continue
		}
		if err := s.upsert(ctx, taskID, candidateType, c); err != nil {
			slog.Warn("knowledge suggestion upsert failed",
				"task_id", taskID, "candidate", c.Name, "error", err)
			continue
		}
		recorded++
	}

	if recorded > 0 {
		slog.Info("knowledge suggestions recorded", "task_id", taskID, "count", recorded)
	}
	return nil
}

// classify 用 retrieval 在 knowledge_base 上查 top-1，若 score < threshold
// 则视为「未覆盖」。返回 (是否需要记录, 推断的 candidate_type, error)。
//
// candidate_type 取 top-1 命中条目的 type；若完全无命中则默认 "module"
// （多数测试场景中模块覆盖面广于产品，落到模块更易被采纳；操作人可在
// 采纳时手动改正）。
func (s *Service) classify(ctx context.Context, name string) (bool, string, error) {
	results, err := s.retrieval.SearchKnowledgeMultiQuery(ctx, []string{name}, 1, "")
	if err != nil {
		return false, "", err
	}
	if len(results) == 0 {
		return true, models.SuggestionCandidateModule, nil
	}
	top := results[0]
	if top.Score >= s.threshold {
		return false, "", nil
	}
	candidateType := top.Type
	if candidateType != models.SuggestionCandidateProduct && candidateType != models.SuggestionCandidateModule {
		candidateType = models.SuggestionCandidateModule
	}
	return true, candidateType, nil
}

func (s *Service) upsert(ctx context.Context, taskID int, candidateType string, c CandidateMatch) error {
	// 同 task + 同 type + 同 name 已存在则跳过（在 task 重跑 analyze 时尤其重要）
	count, err := s.db.NewSelect().Model((*models.KnowledgeUpdateSuggestion)(nil)).
		Where("source_task_id = ?", taskID).
		Where("candidate_type = ?", candidateType).
		Where("candidate_name = ?", c.Name).
		Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	snippets := make([]map[string]any, 0, len(c.Snippets))
	for _, s := range c.Snippets {
		snippets = append(snippets, map[string]any{"text": s})
	}

	tenantID, _ := db.TenantFromContext(ctx)
	now := time.Now()
	row := &models.KnowledgeUpdateSuggestion{
		TenantID:       tenantID,
		SourceTaskID:   taskID,
		CandidateType:  candidateType,
		CandidateName:  strings.TrimSpace(c.Name),
		Frequency:      c.Frequency,
		SourceSnippets: snippets,
		Status:         models.SuggestionStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = s.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func ValidateManualSuggestionInput(input ManualSuggestionInput) error {
	if input.SourceTaskID <= 0 {
		return fmt.Errorf("%w: source_task_id is required", ErrInvalidManualSuggestion)
	}
	if input.SourceCaseID <= 0 {
		return fmt.Errorf("%w: source_case_id is required", ErrInvalidManualSuggestion)
	}
	if input.CandidateType != models.SuggestionCandidateProduct && input.CandidateType != models.SuggestionCandidateModule {
		return fmt.Errorf("%w: candidate_type must be product or module", ErrInvalidManualSuggestion)
	}
	if strings.TrimSpace(input.CandidateName) == "" {
		return fmt.Errorf("%w: candidate_name is required", ErrInvalidManualSuggestion)
	}
	return nil
}

func (s *Service) CreateManual(ctx context.Context, input ManualSuggestionInput) (*models.KnowledgeUpdateSuggestion, error) {
	if err := ValidateManualSuggestionInput(input); err != nil {
		return nil, err
	}

	tc := &models.TestCase{}
	if err := s.db.NewSelect().Model(tc).
		Where("id = ?", input.SourceCaseID).
		Where("task_id = ?", input.SourceTaskID).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: source_case_id does not belong to source_task_id", ErrInvalidManualSuggestion)
		}
		return nil, err
	}

	title := strings.TrimSpace(input.SourceCaseTitle)
	if title == "" {
		title = firstCaseTitle(tc)
	}
	if title == "" {
		title = tc.Section
	}

	snippets := []map[string]any{{
		"type":    "case",
		"case_id": input.SourceCaseID,
		"title":   title,
	}}
	if note := strings.TrimSpace(input.Note); note != "" {
		snippets = append(snippets, map[string]any{
			"type": "note",
			"text": note,
		})
	}

	tenantID, _ := db.TenantFromContext(ctx)
	now := time.Now()
	sourceCaseID := input.SourceCaseID
	row := &models.KnowledgeUpdateSuggestion{
		TenantID:       tenantID,
		SourceTaskID:   input.SourceTaskID,
		SourceCaseID:   &sourceCaseID,
		CandidateType:  input.CandidateType,
		CandidateName:  strings.TrimSpace(input.CandidateName),
		Frequency:      1,
		SourceSnippets: snippets,
		Status:         models.SuggestionStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, err
	}
	return row, nil
}

func firstCaseTitle(tc *models.TestCase) string {
	for _, item := range tc.Cases {
		if title, ok := item["title"].(string); ok {
			if title = strings.TrimSpace(title); title != "" {
				return title
			}
		}
	}
	return ""
}

// List 列出 suggestion；status 为空时返回全部。按 created_at 倒序。
func (s *Service) List(ctx context.Context, status string) ([]models.KnowledgeUpdateSuggestion, error) {
	rows := []models.KnowledgeUpdateSuggestion{}
	q := s.db.NewSelect().Model(&rows).OrderExpr("created_at DESC")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

// SetStatus 把 suggestion 从 pending 推进到 adopted/dismissed。其它转换返回
// false（调用方决定 409 还是 silent）。
func (s *Service) SetStatus(ctx context.Context, id int, target string, resolvedKnowledgeID *int) (*models.KnowledgeUpdateSuggestion, bool, error) {
	if target != models.SuggestionStatusAdopted && target != models.SuggestionStatusDismissed {
		return nil, false, nil
	}
	if target == models.SuggestionStatusDismissed && resolvedKnowledgeID != nil {
		return nil, false, fmt.Errorf("%w: resolved_knowledge_id is only valid for adopted suggestions", ErrInvalidManualSuggestion)
	}
	if target == models.SuggestionStatusAdopted && resolvedKnowledgeID != nil {
		if *resolvedKnowledgeID <= 0 {
			return nil, false, fmt.Errorf("%w: resolved_knowledge_id must be positive", ErrInvalidManualSuggestion)
		}
		count, err := s.db.NewSelect().Model((*models.KnowledgeBase)(nil)).
			Where("id = ?", *resolvedKnowledgeID).
			Count(ctx)
		if err != nil {
			return nil, false, err
		}
		if count == 0 {
			return nil, false, fmt.Errorf("%w: resolved_knowledge_id not found", ErrInvalidManualSuggestion)
		}
	}

	row := &models.KnowledgeUpdateSuggestion{}
	if err := s.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, false, err
	}
	if row.Status != models.SuggestionStatusPending {
		return row, false, nil
	}

	row.Status = target
	row.ResolvedKnowledgeID = resolvedKnowledgeID
	row.DismissedReason = nil
	row.UpdatedAt = time.Now()
	q := s.db.NewUpdate().Model(row).
		Set("status = ?", row.Status).
		Set("dismissed_reason = ?", nil).
		Set("updated_at = ?", row.UpdatedAt)
	if target == models.SuggestionStatusAdopted {
		q = q.Set("resolved_knowledge_id = ?", row.ResolvedKnowledgeID)
	}
	if _, err := q.Where("id = ?", id).Exec(ctx); err != nil {
		return nil, false, err
	}
	return row, true, nil
}

func (s *Service) DismissExpiredPending(ctx context.Context, maxAge time.Duration) (int64, error) {
	if maxAge <= 0 {
		return 0, nil
	}

	now := time.Now()
	result, err := s.db.NewUpdate().
		Model((*models.KnowledgeUpdateSuggestion)(nil)).
		Set("status = ?", models.SuggestionStatusDismissed).
		Set("dismissed_reason = ?", AutoExpiredDismissedReason).
		Set("updated_at = ?", now).
		Where("status = ?", models.SuggestionStatusPending).
		Where("created_at < ?", now.Add(-maxAge)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}
