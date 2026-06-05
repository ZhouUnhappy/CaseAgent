package task

import (
	"context"

	agentservice "caseagent/internal/service/agent"
	retrievalservice "caseagent/internal/service/retrieval"
	suggestionservice "caseagent/internal/service/suggestion"
)

type CaseGenerator interface {
	GenerateCases(ctx context.Context, requirements string, knowledge string) (string, error)
}

type Retriever interface {
	SearchDocumentsMultiQuery(ctx context.Context, queries []string, topK int, documentIDs []int) ([]retrievalservice.DocumentResult, error)
	SearchKnowledgeMultiQuery(ctx context.Context, queries []string, topK int, kbType string) ([]retrievalservice.KnowledgeResult, error)
}

type SuggestionRecorder interface {
	RecordCandidates(ctx context.Context, taskID int, requirements string, inferredProducts []string, inferredModules []string) error
	RecordContextGap(ctx context.Context, input suggestionservice.ContextGapInput) (*suggestionservice.SuggestionGroupView, error)
}

func defaultCaseGeneratorFactory(ctx context.Context) (CaseGenerator, error) {
	return agentservice.New(ctx, &agentservice.Config{})
}

func (s *Service) caseGenerator(ctx context.Context) (CaseGenerator, error) {
	if s.newCaseGenerator == nil {
		return defaultCaseGeneratorFactory(ctx)
	}
	return s.newCaseGenerator(ctx)
}

func (s *Service) retriever() Retriever {
	if s.newRetriever == nil {
		return retrievalservice.New(s.db)
	}
	return s.newRetriever()
}

func (s *Service) suggestionRecorder() SuggestionRecorder {
	if s.newSuggestionRecorder == nil {
		return suggestionservice.New(s.db)
	}
	return s.newSuggestionRecorder()
}
