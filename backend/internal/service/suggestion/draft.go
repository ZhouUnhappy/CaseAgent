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
	row := &models.KnowledgeUpdateSuggestion{}
	if err := s.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}

	draft, err := DraftFromSuggestion(ctx, row, s.draftGenerator)
	if err != nil {
		return nil, true, err
	}
	return &DraftResult{DraftContent: draft}, true, nil
}

func DraftFromSuggestion(ctx context.Context, row *models.KnowledgeUpdateSuggestion, generator DraftGenerator) (string, error) {
	if row == nil {
		return "", fmt.Errorf("suggestion is required")
	}
	if len(row.SourceSnippets) == 0 {
		return "", nil
	}
	if generator == nil {
		return "", fmt.Errorf("draft generator is required")
	}

	return generator.GenerateDraft(ctx, agentknowledge.DraftInput{
		CandidateType:  row.CandidateType,
		CandidateName:  row.CandidateName,
		SourceSnippets: row.SourceSnippets,
	})
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
