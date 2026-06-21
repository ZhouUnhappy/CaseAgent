package feedback

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"caseagent/internal/clock"
	tenantdb "caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

var (
	ErrInvalidInput     = errors.New("case feedback: invalid input")
	ErrTestCaseNotFound = errors.New("case feedback: test case not found")
)

type Service struct {
	db bun.IDB
}

type CreateInput struct {
	TaskID       int
	TestCaseID   int
	CaseIndex    int
	FeedbackType string
	Note         string
	Metadata     map[string]any
}

type SummaryInput struct {
	TaskID        int
	FeedbackType  string
	PromptID      string
	PromptVersion string
	From          *time.Time
	To            *time.Time
}

type Summary struct {
	Total         int                       `json:"total"`
	ReviewedCases int                       `json:"reviewed_cases"`
	UsefulCases   int                       `json:"useful_cases"`
	IssueCases    int                       `json:"issue_cases"`
	ByType        []TypeSummary             `json:"by_type"`
	ByPrompt      []PromptSummary           `json:"by_prompt"`
	Recent        []models.TestCaseFeedback `json:"recent"`
}

type TypeSummary struct {
	FeedbackType string `json:"feedback_type"`
	Count        int    `json:"count"`
}

type PromptSummary struct {
	PromptID      string `json:"prompt_id"`
	PromptVersion string `json:"prompt_version"`
	Total         int    `json:"total"`
	Useful        int    `json:"useful"`
	Negative      int    `json:"negative"`
}

type QualityOverview struct {
	TotalFeedback     int                    `json:"total_feedback"`
	PromptComparison  []PromptSummary        `json:"prompt_comparison"`
	ProfileComparison []ProfileSummary       `json:"profile_comparison"`
	FeedbackTrend     []TrendSummary         `json:"feedback_trend"`
	ReportHistory     []QualityReportSummary `json:"report_history"`
}

type ProfileSummary struct {
	ProfileID      string `json:"profile_id"`
	ProfileVersion string `json:"profile_version"`
	Total          int    `json:"total"`
	Useful         int    `json:"useful"`
	Negative       int    `json:"negative"`
}

type TrendSummary struct {
	Date         string `json:"date"`
	FeedbackType string `json:"feedback_type"`
	Count        int    `json:"count"`
}

type QualityReportSummary struct {
	ArtifactID     int       `json:"artifact_id"`
	TaskID         int       `json:"task_id,omitempty"`
	Name           string    `json:"name,omitempty"`
	ArtifactType   string    `json:"artifact_type"`
	SectionCount   int       `json:"section_count"`
	CaseCount      int       `json:"case_count"`
	ProfileID      string    `json:"profile_id,omitempty"`
	ProfileVersion string    `json:"profile_version,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func New(db bun.IDB) *Service {
	return &Service{db: db}
}

func (s *Service) CreateCaseFeedback(ctx context.Context, input CreateInput) (*models.TestCaseFeedback, error) {
	rows, err := s.CreateCaseFeedbackBatch(ctx, []CreateInput{input})
	if err != nil {
		return nil, err
	}
	return &rows[0], nil
}

func (s *Service) CreateCaseFeedbackBatch(ctx context.Context, inputs []CreateInput) ([]models.TestCaseFeedback, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("%w: cases is required", ErrInvalidInput)
	}
	if len(inputs) > 200 {
		return nil, fmt.Errorf("%w: at most 200 cases are allowed", ErrInvalidInput)
	}
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("%w: no tenant in context", ErrInvalidInput)
	}

	caseIDs := make([]int, 0, len(inputs))
	seenIDs := make(map[int]struct{}, len(inputs))
	seenRefs := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if err := validateCreateInput(input); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%d:%d", input.TestCaseID, input.CaseIndex)
		if _, exists := seenRefs[key]; exists {
			return nil, fmt.Errorf("%w: duplicate case reference %s", ErrInvalidInput, key)
		}
		seenRefs[key] = struct{}{}
		if _, exists := seenIDs[input.TestCaseID]; !exists {
			seenIDs[input.TestCaseID] = struct{}{}
			caseIDs = append(caseIDs, input.TestCaseID)
		}
	}

	var testCases []models.TestCase
	if err := s.db.NewSelect().
		Model(&testCases).
		Where("id IN (?)", bun.In(caseIDs)).
		Scan(ctx); err != nil {
		return nil, err
	}
	testCasesByID := make(map[int]models.TestCase, len(testCases))
	for _, testCase := range testCases {
		testCasesByID[testCase.ID] = testCase
	}

	now := clock.Now()
	rows := make([]models.TestCaseFeedback, 0, len(inputs))
	for _, input := range inputs {
		testCase, exists := testCasesByID[input.TestCaseID]
		if !exists || testCase.TaskID != input.TaskID {
			return nil, fmt.Errorf("%w: test_case_id %d", ErrTestCaseNotFound, input.TestCaseID)
		}
		if input.CaseIndex >= len(testCase.Cases) {
			return nil, fmt.Errorf("%w: case_index out of range", ErrInvalidInput)
		}

		caseItem := testCase.Cases[input.CaseIndex]
		modelCallID, promptID, promptVersion := SelectTraceModelCall(testCase.SourceContext)
		rows = append(rows, models.TestCaseFeedback{
			TenantID:             tenantID,
			TaskID:               input.TaskID,
			TestCaseID:           input.TestCaseID,
			CaseIndex:            input.CaseIndex,
			CaseTitle:            stringFromAny(caseItem["title"]),
			FeedbackType:         strings.TrimSpace(input.FeedbackType),
			Note:                 strings.TrimSpace(input.Note),
			SourceContextSummary: SummarizeSourceContext(testCase.SourceContext),
			PromptID:             promptID,
			PromptVersion:        promptVersion,
			ModelCallID:          modelCallID,
			Metadata:             defaultMap(input.Metadata),
			CreatedAt:            now,
			UpdatedAt:            now,
		})
	}

	if _, err := s.db.NewInsert().Model(&rows).Exec(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) ListTaskFeedback(ctx context.Context, taskID int) ([]models.TestCaseFeedback, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("%w: task_id is required", ErrInvalidInput)
	}
	rows := []models.TestCaseFeedback{}
	if err := s.db.NewSelect().
		Model(&rows).
		Where("task_id = ?", taskID).
		Order("created_at DESC", "id DESC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) FeedbackSummary(ctx context.Context, input SummaryInput) (*Summary, error) {
	rows, err := s.loadFeedbackRows(ctx, input, 500)
	if err != nil {
		return nil, err
	}
	summary := SummarizeFeedbackRows(rows)
	summary.Recent = rows
	return summary, nil
}

func (s *Service) QualityOverview(ctx context.Context, input SummaryInput) (*QualityOverview, error) {
	rows, err := s.loadFeedbackRows(ctx, input, 1000)
	if err != nil {
		return nil, err
	}
	testCases, err := s.loadFeedbackTestCases(ctx, rows)
	if err != nil {
		return nil, err
	}
	artifacts, err := s.loadQualityReportArtifacts(ctx, input)
	if err != nil {
		return nil, err
	}
	return BuildQualityOverview(rows, testCases, artifacts), nil
}

func (s *Service) loadFeedbackRows(ctx context.Context, input SummaryInput, limit int) ([]models.TestCaseFeedback, error) {
	if input.TaskID < 0 {
		return nil, fmt.Errorf("%w: task_id must be >= 0", ErrInvalidInput)
	}
	if input.FeedbackType != "" && !IsFeedbackType(input.FeedbackType) {
		return nil, fmt.Errorf("%w: unsupported feedback_type %q", ErrInvalidInput, input.FeedbackType)
	}

	rows := []models.TestCaseFeedback{}
	query := s.db.NewSelect().
		Model(&rows).
		Order("created_at DESC", "id DESC").
		Limit(limit)
	if input.TaskID > 0 {
		query.Where("task_id = ?", input.TaskID)
	}
	if input.FeedbackType != "" {
		query.Where("feedback_type = ?", strings.TrimSpace(input.FeedbackType))
	}
	if strings.TrimSpace(input.PromptID) != "" {
		query.Where("prompt_id = ?", strings.TrimSpace(input.PromptID))
	}
	if strings.TrimSpace(input.PromptVersion) != "" {
		query.Where("prompt_version = ?", strings.TrimSpace(input.PromptVersion))
	}
	if input.From != nil {
		query.Where("created_at >= ?", *input.From)
	}
	if input.To != nil {
		query.Where("created_at <= ?", *input.To)
	}
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) loadFeedbackTestCases(ctx context.Context, rows []models.TestCaseFeedback) ([]models.TestCase, error) {
	ids := make([]int, 0, len(rows))
	seen := map[int]struct{}{}
	for _, row := range rows {
		if row.TestCaseID <= 0 {
			continue
		}
		if _, ok := seen[row.TestCaseID]; ok {
			continue
		}
		seen[row.TestCaseID] = struct{}{}
		ids = append(ids, row.TestCaseID)
	}
	if len(ids) == 0 {
		return []models.TestCase{}, nil
	}
	testCases := []models.TestCase{}
	if err := s.db.NewSelect().
		Model(&testCases).
		Where("id IN (?)", bun.In(ids)).
		Scan(ctx); err != nil {
		return nil, err
	}
	return testCases, nil
}

func (s *Service) loadQualityReportArtifacts(ctx context.Context, input SummaryInput) ([]models.Artifact, error) {
	rows := []models.Artifact{}
	query := s.db.NewSelect().
		Model(&rows).
		Where("artifact_type IN (?)", bun.In([]string{models.ArtifactTypeGeneratedCases, models.ArtifactTypeOutput})).
		Where("resource_type = ?", "task").
		Order("created_at DESC", "id DESC").
		Limit(50)
	if input.TaskID > 0 {
		query.Where("resource_id = ?", input.TaskID)
	}
	if input.From != nil {
		query.Where("created_at >= ?", *input.From)
	}
	if input.To != nil {
		query.Where("created_at <= ?", *input.To)
	}
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

func SummarizeFeedbackRows(rows []models.TestCaseFeedback) *Summary {
	byType := map[string]int{}
	byPrompt := map[string]*PromptSummary{}
	latestByCase := map[string]models.TestCaseFeedback{}
	for _, row := range rows {
		byType[row.FeedbackType]++
		caseKey := fmt.Sprintf("%d:%d", row.TestCaseID, row.CaseIndex)
		current, exists := latestByCase[caseKey]
		if !exists || row.CreatedAt.After(current.CreatedAt) || (row.CreatedAt.Equal(current.CreatedAt) && row.ID > current.ID) {
			latestByCase[caseKey] = row
		}
		key := row.PromptID + "\x00" + row.PromptVersion
		prompt := byPrompt[key]
		if prompt == nil {
			prompt = &PromptSummary{
				PromptID:      row.PromptID,
				PromptVersion: row.PromptVersion,
			}
			byPrompt[key] = prompt
		}
		prompt.Total++
		if row.FeedbackType == models.CaseFeedbackUseful {
			prompt.Useful++
		} else {
			prompt.Negative++
		}
	}

	summary := &Summary{
		Total:         len(rows),
		ReviewedCases: len(latestByCase),
		ByType:        make([]TypeSummary, 0, len(byType)),
		ByPrompt:      make([]PromptSummary, 0, len(byPrompt)),
		Recent:        []models.TestCaseFeedback{},
	}
	for _, row := range latestByCase {
		if row.FeedbackType == models.CaseFeedbackUseful {
			summary.UsefulCases++
		} else {
			summary.IssueCases++
		}
	}
	for feedbackType, count := range byType {
		summary.ByType = append(summary.ByType, TypeSummary{FeedbackType: feedbackType, Count: count})
	}
	for _, prompt := range byPrompt {
		summary.ByPrompt = append(summary.ByPrompt, *prompt)
	}
	sortTypeSummary(summary.ByType)
	sortPromptSummary(summary.ByPrompt)
	return summary
}

func BuildQualityOverview(feedbackRows []models.TestCaseFeedback, testCases []models.TestCase, artifacts []models.Artifact) *QualityOverview {
	summary := SummarizeFeedbackRows(feedbackRows)
	return &QualityOverview{
		TotalFeedback:     len(feedbackRows),
		PromptComparison:  summary.ByPrompt,
		ProfileComparison: profileComparison(feedbackRows, testCases),
		FeedbackTrend:     feedbackTrend(feedbackRows),
		ReportHistory:     qualityReportHistory(artifacts),
	}
}

func profileComparison(feedbackRows []models.TestCaseFeedback, testCases []models.TestCase) []ProfileSummary {
	casesByID := make(map[int]models.TestCase, len(testCases))
	for _, testCase := range testCases {
		casesByID[testCase.ID] = testCase
	}
	byProfile := map[string]*ProfileSummary{}
	for _, row := range feedbackRows {
		profileID, profileVersion := feedbackProfile(row, casesByID[row.TestCaseID])
		key := profileID + "\x00" + profileVersion
		summary := byProfile[key]
		if summary == nil {
			summary = &ProfileSummary{ProfileID: profileID, ProfileVersion: profileVersion}
			byProfile[key] = summary
		}
		summary.Total++
		if row.FeedbackType == models.CaseFeedbackUseful {
			summary.Useful++
		} else {
			summary.Negative++
		}
	}
	rows := make([]ProfileSummary, 0, len(byProfile))
	for _, summary := range byProfile {
		rows = append(rows, *summary)
	}
	sortProfileSummary(rows)
	return rows
}

func feedbackProfile(row models.TestCaseFeedback, testCase models.TestCase) (string, string) {
	for _, source := range []map[string]any{testCase.SourceContext, row.SourceContextSummary, row.Metadata} {
		id := stringFromAny(sourceValue(source, "generation_profile_id"))
		version := stringFromAny(sourceValue(source, "generation_profile_version"))
		if id != "" || version != "" {
			return defaultLabel(id), defaultLabel(version)
		}
	}
	return "unknown", "unknown"
}

func feedbackTrend(rows []models.TestCaseFeedback) []TrendSummary {
	counts := map[string]int{}
	for _, row := range rows {
		date := row.CreatedAt.UTC().Format("2006-01-02")
		key := date + "\x00" + row.FeedbackType
		counts[key]++
	}
	trend := make([]TrendSummary, 0, len(counts))
	for key, count := range counts {
		parts := strings.SplitN(key, "\x00", 2)
		trend = append(trend, TrendSummary{Date: parts[0], FeedbackType: parts[1], Count: count})
	}
	sort.Slice(trend, func(i, j int) bool {
		if trend[i].Date != trend[j].Date {
			return trend[i].Date < trend[j].Date
		}
		return trend[i].FeedbackType < trend[j].FeedbackType
	})
	return trend
}

func qualityReportHistory(artifacts []models.Artifact) []QualityReportSummary {
	rows := make([]QualityReportSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		taskID := 0
		if artifact.ResourceID != nil {
			taskID = *artifact.ResourceID
		}
		source := mapFromAny(artifact.Payload["source_context"])
		profileID := stringFromAny(sourceValue(source, "generation_profile_id"))
		profileVersion := stringFromAny(sourceValue(source, "generation_profile_version"))
		rows = append(rows, QualityReportSummary{
			ArtifactID:     artifact.ID,
			TaskID:         taskID,
			Name:           artifact.Name,
			ArtifactType:   artifact.ArtifactType,
			SectionCount:   intFromAny(artifact.Payload["section_count"]),
			CaseCount:      intFromAny(artifact.Payload["case_count"]),
			ProfileID:      profileID,
			ProfileVersion: profileVersion,
			CreatedAt:      artifact.CreatedAt,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	return rows
}

func sortProfileSummary(rows []ProfileSummary) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Total != rows[j].Total {
			return rows[i].Total > rows[j].Total
		}
		if rows[i].Negative != rows[j].Negative {
			return rows[i].Negative > rows[j].Negative
		}
		if rows[i].ProfileID != rows[j].ProfileID {
			return rows[i].ProfileID < rows[j].ProfileID
		}
		return rows[i].ProfileVersion < rows[j].ProfileVersion
	})
}

func sortTypeSummary(rows []TypeSummary) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].FeedbackType < rows[j].FeedbackType
	})
}

func sortPromptSummary(rows []PromptSummary) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Total != rows[j].Total {
			return rows[i].Total > rows[j].Total
		}
		if rows[i].Negative != rows[j].Negative {
			return rows[i].Negative > rows[j].Negative
		}
		if rows[i].PromptID != rows[j].PromptID {
			return rows[i].PromptID < rows[j].PromptID
		}
		return rows[i].PromptVersion < rows[j].PromptVersion
	})
}

func validateCreateInput(input CreateInput) error {
	if input.TaskID <= 0 {
		return fmt.Errorf("%w: task_id is required", ErrInvalidInput)
	}
	if input.TestCaseID <= 0 {
		return fmt.Errorf("%w: test_case_id is required", ErrInvalidInput)
	}
	if input.CaseIndex < 0 {
		return fmt.Errorf("%w: case_index must be >= 0", ErrInvalidInput)
	}
	if !IsFeedbackType(input.FeedbackType) {
		return fmt.Errorf("%w: unsupported feedback_type %q", ErrInvalidInput, input.FeedbackType)
	}
	return nil
}

func IsFeedbackType(value string) bool {
	switch strings.TrimSpace(value) {
	case models.CaseFeedbackUseful,
		models.CaseFeedbackDuplicate,
		models.CaseFeedbackMissingSteps,
		models.CaseFeedbackRequirementMismatch,
		models.CaseFeedbackKnowledgeMissing:
		return true
	default:
		return false
	}
}

func SummarizeSourceContext(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	documentHits := mapsFromAny(source["document_hits"])
	knowledgeHits := mapsFromAny(source["knowledge_hits"])
	modelCalls := mapsFromAny(source["model_calls"])
	return map[string]any{
		"document_queries":           source["document_queries"],
		"knowledge_queries":          source["knowledge_queries"],
		"document_hit_count":         len(documentHits),
		"knowledge_hit_count":        len(knowledgeHits),
		"model_call_count":           len(modelCalls),
		"document_hits":              summarizeMaps(documentHits, "document_id", "name", "rank", "best_score"),
		"knowledge_hits":             summarizeMaps(knowledgeHits, "id", "name", "type", "rank", "score"),
		"knowledge_shipped_ids":      source["knowledge_shipped_ids"],
		"knowledge_shipped_names":    source["knowledge_shipped_names"],
		"generation_profile_id":      source["generation_profile_id"],
		"generation_profile_version": source["generation_profile_version"],
	}
}

func SelectTraceModelCall(source map[string]any) (*int, string, string) {
	calls := mapsFromAny(sourceValue(source, "model_calls"))
	if len(calls) == 0 {
		return nil, "", ""
	}
	selected := calls[0]
	for _, call := range calls {
		if stringFromAny(call["status"]) == models.WorkflowStatusSucceeded {
			selected = call
			break
		}
	}
	id := intFromAny(selected["id"])
	var idPtr *int
	if id > 0 {
		idPtr = &id
	}
	return idPtr, stringFromAny(selected["prompt_id"]), stringFromAny(selected["prompt_version"])
}

func summarizeMaps(rows []map[string]any, keys ...string) []map[string]any {
	maxRows := 5
	if len(rows) < maxRows {
		maxRows = len(rows)
	}
	summary := make([]map[string]any, 0, maxRows)
	for idx := 0; idx < maxRows; idx++ {
		row := make(map[string]any, len(keys))
		for _, key := range keys {
			if value, ok := rows[idx][key]; ok {
				row[key] = value
			}
		}
		summary = append(summary, row)
	}
	return summary
}

func mapsFromAny(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
		return rows
	default:
		return nil
	}
}

func mapFromAny(value any) map[string]any {
	row, _ := value.(map[string]any)
	return row
}

func sourceValue(source map[string]any, key string) any {
	if source == nil {
		return nil
	}
	return source[key]
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func defaultLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return strings.TrimSpace(value)
}

func defaultMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}
