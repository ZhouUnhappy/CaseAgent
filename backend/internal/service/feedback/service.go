package feedback

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

func New(db bun.IDB) *Service {
	return &Service{db: db}
}

func (s *Service) CreateCaseFeedback(ctx context.Context, input CreateInput) (*models.TestCaseFeedback, error) {
	if err := validateCreateInput(input); err != nil {
		return nil, err
	}
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("%w: no tenant in context", ErrInvalidInput)
	}

	testCase := new(models.TestCase)
	if err := s.db.NewSelect().
		Model(testCase).
		Where("id = ?", input.TestCaseID).
		Where("task_id = ?", input.TaskID).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTestCaseNotFound, err)
	}
	if input.CaseIndex >= len(testCase.Cases) {
		return nil, fmt.Errorf("%w: case_index out of range", ErrInvalidInput)
	}

	caseItem := testCase.Cases[input.CaseIndex]
	modelCallID, promptID, promptVersion := SelectTraceModelCall(testCase.SourceContext)
	now := time.Now()
	row := &models.TestCaseFeedback{
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
	}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, err
	}
	return row, nil
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
		"document_queries":        source["document_queries"],
		"knowledge_queries":       source["knowledge_queries"],
		"document_hit_count":      len(documentHits),
		"knowledge_hit_count":     len(knowledgeHits),
		"model_call_count":        len(modelCalls),
		"document_hits":           summarizeMaps(documentHits, "document_id", "name", "rank", "best_score"),
		"knowledge_hits":          summarizeMaps(knowledgeHits, "id", "name", "type", "rank", "score"),
		"knowledge_shipped_ids":   source["knowledge_shipped_ids"],
		"knowledge_shipped_names": source["knowledge_shipped_names"],
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

func defaultMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}
