package suggestion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	agentknowledge "caseagent/internal/agent/knowledge"
	"caseagent/internal/ai"
	"caseagent/internal/config"
	"caseagent/internal/db/models"
)

type DraftResult struct {
	DraftContent string `json:"draft_content"`
}

type DraftGenerator interface {
	GenerateDraft(ctx context.Context, input agentknowledge.DraftInput) (string, error)
}

type draftGeneratorFunc func(context.Context) (DraftGenerator, error)

func (f draftGeneratorFunc) GenerateDraft(ctx context.Context, input agentknowledge.DraftInput) (string, error) {
	generator, err := f(ctx)
	if err != nil {
		return "", err
	}
	return generator.GenerateDraft(ctx, input)
}

func (s *Service) Draft(ctx context.Context, id int) (*DraftResult, bool, error) {
	group := &models.KnowledgeUpdateSuggestionGroup{}
	if err := s.db.NewSelect().Model(group).Where("id = ?", id).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}

	occurrences := []models.KnowledgeUpdateSuggestionOccurrence{}
	if err := s.db.NewSelect().
		Model(&occurrences).
		Where("group_id = ?", group.ID).
		OrderExpr("created_at DESC").
		OrderExpr("id DESC").
		Scan(ctx); err != nil {
		return nil, true, err
	}

	draft, err := DraftFromSuggestion(ctx, group, occurrences, s.draftGenerator)
	if err != nil {
		return nil, true, err
	}
	return &DraftResult{DraftContent: draft}, true, nil
}

func DraftFromSuggestion(
	ctx context.Context,
	group *models.KnowledgeUpdateSuggestionGroup,
	occurrences []models.KnowledgeUpdateSuggestionOccurrence,
	generator DraftGenerator,
) (string, error) {
	if group == nil {
		return "", fmt.Errorf("suggestion group is required")
	}

	snippets := flattenOccurrenceSnippets(occurrences)
	if len(snippets) == 0 {
		return "", nil
	}
	if generator == nil {
		return "", fmt.Errorf("draft generator is required")
	}

	return generator.GenerateDraft(ctx, agentknowledge.DraftInput{
		CandidateType:  group.CandidateType,
		CandidateName:  group.CandidateName,
		SourceSnippets: snippets,
	})
}

func flattenOccurrenceSnippets(occurrences []models.KnowledgeUpdateSuggestionOccurrence) []map[string]any {
	total := 0
	for _, occurrence := range occurrences {
		total += len(occurrence.SourceSnippets)
	}
	snippets := make([]map[string]any, 0, total)
	for _, occurrence := range occurrences {
		for _, snippet := range occurrence.SourceSnippets {
			item := make(map[string]any, len(snippet)+3)
			for key, value := range snippet {
				item[key] = value
			}
			item["source_task_id"] = occurrence.SourceTaskID
			if occurrence.SourceCaseID != nil {
				item["source_case_id"] = *occurrence.SourceCaseID
			}
			item["frequency"] = occurrence.Frequency
			snippets = append(snippets, item)
		}
	}
	return snippets
}

func newConfiguredDraftGenerator(ctx context.Context) (DraftGenerator, error) {
	appCfg := config.Get()
	if appCfg == nil {
		return nil, fmt.Errorf("config is not loaded")
	}

	chatModel, err := ai.NewChatModel(ctx, appCfg.Model.Chat)
	if err != nil {
		return nil, fmt.Errorf("initialize chat model: %w", err)
	}
	return agentknowledge.New(ctx, &agentknowledge.Config{ChatModel: chatModel})
}
