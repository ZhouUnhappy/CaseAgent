// Package suggestion produces "knowledge gap" candidates that the operator
// can review in the workbench. Detection is invoked from taskservice after
// AnalyzeTask completes: requirement-level identifier candidates that are
// NOT already covered by an existing knowledge_base entry land in the
// knowledge_update_suggestion_groups / occurrences tables with status='pending'.
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

type SuggestionOccurrenceView struct {
	ID             int              `json:"id"`
	SourceTaskID   int              `json:"source_task_id"`
	SourceCaseID   *int             `json:"source_case_id,omitempty"`
	Frequency      int              `json:"frequency"`
	SourceSnippets []map[string]any `json:"source_snippets"`
	CreatedAt      time.Time        `json:"created_at"`
}

type SuggestionGroupView struct {
	ID                  int                        `json:"id"`
	TenantID            int                        `json:"tenant_id"`
	CandidateType       string                     `json:"candidate_type"`
	CandidateName       string                     `json:"candidate_name"`
	Frequency           int                        `json:"frequency"`
	TotalFrequency      int                        `json:"total_frequency"`
	TaskCount           int                        `json:"task_count"`
	SourceTaskID        int                        `json:"source_task_id,omitempty"`
	SourceCaseID        *int                       `json:"source_case_id,omitempty"`
	ResolvedKnowledgeID *int                       `json:"resolved_knowledge_id,omitempty"`
	SourceSnippets      []map[string]any           `json:"source_snippets"`
	Status              string                     `json:"status"`
	DismissedReason     *string                    `json:"dismissed_reason,omitempty"`
	FirstSeenAt         time.Time                  `json:"first_seen_at"`
	LastSeenAt          time.Time                  `json:"last_seen_at"`
	CreatedAt           time.Time                  `json:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at"`
	Occurrences         []SuggestionOccurrenceView `json:"occurrences,omitempty"`
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
		created, err := s.upsert(ctx, taskID, candidateType, c)
		if err != nil {
			slog.Warn("knowledge suggestion upsert failed",
				"task_id", taskID, "candidate", c.Name, "error", err)
			continue
		}
		if created {
			recorded++
		}
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

func (s *Service) upsert(ctx context.Context, taskID int, candidateType string, c CandidateMatch) (bool, error) {
	snippets := make([]map[string]any, 0, len(c.Snippets))
	for _, snippet := range c.Snippets {
		snippets = append(snippets, map[string]any{"text": snippet})
	}

	_, created, err := s.recordOccurrence(ctx, occurrenceInput{
		CandidateType:  candidateType,
		CandidateName:  c.Name,
		SourceTaskID:   taskID,
		Frequency:      c.Frequency,
		SourceSnippets: snippets,
	})
	return created, err
}

type occurrenceInput struct {
	CandidateType  string
	CandidateName  string
	SourceTaskID   int
	SourceCaseID   *int
	Frequency      int
	SourceSnippets []map[string]any
}

func (s *Service) recordOccurrence(ctx context.Context, input occurrenceInput) (*SuggestionGroupView, bool, error) {
	tenantID, _ := db.TenantFromContext(ctx)
	now := time.Now()

	frequency := input.Frequency
	if frequency <= 0 {
		frequency = 1
	}

	group := &models.KnowledgeUpdateSuggestionGroup{
		TenantID:       tenantID,
		CandidateType:  input.CandidateType,
		CandidateName:  strings.TrimSpace(input.CandidateName),
		TotalFrequency: 0,
		TaskCount:      0,
		Status:         models.SuggestionStatusPending,
		FirstSeenAt:    now,
		LastSeenAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := s.db.NewInsert().Model(group).
		On("CONFLICT (tenant_id, candidate_type, candidate_name) DO UPDATE").
		Set("updated_at = EXCLUDED.updated_at").
		Returning("*").
		Exec(ctx); err != nil {
		return nil, false, err
	}

	existing := s.db.NewSelect().
		Model((*models.KnowledgeUpdateSuggestionOccurrence)(nil)).
		Where("group_id = ?", group.ID).
		Where("source_task_id = ?", input.SourceTaskID)
	if input.SourceCaseID == nil {
		existing = existing.Where("source_case_id IS NULL")
	} else {
		existing = existing.Where("source_case_id = ?", *input.SourceCaseID)
	}
	count, err := existing.Count(ctx)
	if err != nil {
		return nil, false, err
	}
	if count > 0 {
		view, err := s.getGroupView(ctx, group.ID)
		return view, false, err
	}

	taskSeen, err := s.db.NewSelect().
		Model((*models.KnowledgeUpdateSuggestionOccurrence)(nil)).
		Where("group_id = ?", group.ID).
		Where("source_task_id = ?", input.SourceTaskID).
		Count(ctx)
	if err != nil {
		return nil, false, err
	}

	occurrence := &models.KnowledgeUpdateSuggestionOccurrence{
		TenantID:       tenantID,
		GroupID:        group.ID,
		SourceTaskID:   input.SourceTaskID,
		SourceCaseID:   input.SourceCaseID,
		Frequency:      frequency,
		SourceSnippets: input.SourceSnippets,
		CreatedAt:      now,
	}
	if _, err := s.db.NewInsert().Model(occurrence).Exec(ctx); err != nil {
		return nil, false, err
	}

	taskIncrement := 0
	if taskSeen == 0 {
		taskIncrement = 1
	}
	if _, err := s.db.NewUpdate().
		Model((*models.KnowledgeUpdateSuggestionGroup)(nil)).
		Set("total_frequency = total_frequency + ?", frequency).
		Set("task_count = task_count + ?", taskIncrement).
		Set("last_seen_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", group.ID).
		Exec(ctx); err != nil {
		return nil, false, err
	}

	view, err := s.getGroupView(ctx, group.ID)
	return view, true, err
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

func (s *Service) CreateManual(ctx context.Context, input ManualSuggestionInput) (*SuggestionGroupView, error) {
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

	sourceCaseID := input.SourceCaseID
	row, _, err := s.recordOccurrence(ctx, occurrenceInput{
		CandidateType:  input.CandidateType,
		CandidateName:  input.CandidateName,
		SourceTaskID:   input.SourceTaskID,
		SourceCaseID:   &sourceCaseID,
		Frequency:      1,
		SourceSnippets: snippets,
	})
	if err != nil {
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

// List 列出聚合后的 suggestions；status 为空时返回全部。按任务覆盖和频次优先级排序。
func (s *Service) List(ctx context.Context, status string) ([]SuggestionGroupView, error) {
	rows := []models.KnowledgeUpdateSuggestionGroup{}
	q := s.db.NewSelect().Model(&rows).
		OrderExpr("task_count DESC").
		OrderExpr("total_frequency DESC").
		OrderExpr("last_seen_at DESC")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return s.groupViews(ctx, rows)
}

// SetStatus 把 suggestion group 从 pending 推进到 adopted/dismissed。其它转换返回
// false（调用方决定 409 还是 silent）。
func (s *Service) SetStatus(ctx context.Context, id int, target string, resolvedKnowledgeID *int) (*SuggestionGroupView, bool, error) {
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

	row := &models.KnowledgeUpdateSuggestionGroup{}
	if err := s.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, false, err
	}
	if row.Status != models.SuggestionStatusPending {
		view, err := s.getGroupView(ctx, row.ID)
		return view, false, err
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
	view, err := s.getGroupView(ctx, row.ID)
	return view, true, err
}

func (s *Service) DismissExpiredPending(ctx context.Context, maxAge time.Duration) (int64, error) {
	if maxAge <= 0 {
		return 0, nil
	}

	now := time.Now()
	result, err := s.db.NewUpdate().
		Model((*models.KnowledgeUpdateSuggestionGroup)(nil)).
		Set("status = ?", models.SuggestionStatusDismissed).
		Set("dismissed_reason = ?", AutoExpiredDismissedReason).
		Set("updated_at = ?", now).
		Where("status = ?", models.SuggestionStatusPending).
		Where("first_seen_at < ?", now.Add(-maxAge)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (s *Service) getGroupView(ctx context.Context, id int) (*SuggestionGroupView, error) {
	group := &models.KnowledgeUpdateSuggestionGroup{}
	if err := s.db.NewSelect().Model(group).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	views, err := s.groupViews(ctx, []models.KnowledgeUpdateSuggestionGroup{*group})
	if err != nil {
		return nil, err
	}
	if len(views) == 0 {
		return nil, sql.ErrNoRows
	}
	return &views[0], nil
}

func (s *Service) groupViews(ctx context.Context, groups []models.KnowledgeUpdateSuggestionGroup) ([]SuggestionGroupView, error) {
	views := make([]SuggestionGroupView, 0, len(groups))
	if len(groups) == 0 {
		return views, nil
	}

	groupIDs := make([]int, 0, len(groups))
	for _, group := range groups {
		groupIDs = append(groupIDs, group.ID)
	}

	occurrences := []models.KnowledgeUpdateSuggestionOccurrence{}
	if err := s.db.NewSelect().
		Model(&occurrences).
		Where("group_id IN (?)", bun.In(groupIDs)).
		OrderExpr("created_at DESC").
		OrderExpr("id DESC").
		Scan(ctx); err != nil {
		return nil, err
	}

	occByGroup := make(map[int][]SuggestionOccurrenceView, len(groups))
	for _, occurrence := range occurrences {
		occByGroup[occurrence.GroupID] = append(occByGroup[occurrence.GroupID], SuggestionOccurrenceView{
			ID:             occurrence.ID,
			SourceTaskID:   occurrence.SourceTaskID,
			SourceCaseID:   occurrence.SourceCaseID,
			Frequency:      occurrence.Frequency,
			SourceSnippets: occurrence.SourceSnippets,
			CreatedAt:      occurrence.CreatedAt,
		})
	}

	for _, group := range groups {
		view := SuggestionGroupView{
			ID:                  group.ID,
			TenantID:            group.TenantID,
			CandidateType:       group.CandidateType,
			CandidateName:       group.CandidateName,
			Frequency:           group.TotalFrequency,
			TotalFrequency:      group.TotalFrequency,
			TaskCount:           group.TaskCount,
			ResolvedKnowledgeID: group.ResolvedKnowledgeID,
			Status:              group.Status,
			DismissedReason:     group.DismissedReason,
			FirstSeenAt:         group.FirstSeenAt,
			LastSeenAt:          group.LastSeenAt,
			CreatedAt:           group.CreatedAt,
			UpdatedAt:           group.UpdatedAt,
			Occurrences:         occByGroup[group.ID],
		}
		if len(view.Occurrences) > 0 {
			latest := view.Occurrences[0]
			view.SourceTaskID = latest.SourceTaskID
			view.SourceCaseID = latest.SourceCaseID
			view.SourceSnippets = latest.SourceSnippets
		}
		views = append(views, view)
	}
	return views, nil
}
