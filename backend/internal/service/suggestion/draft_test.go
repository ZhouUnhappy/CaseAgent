package suggestion

import (
	"context"
	"testing"

	agentknowledge "caseagent/internal/agent/knowledge"
	"caseagent/internal/db/models"
)

type failingDraftGenerator struct{}

func (f failingDraftGenerator) GenerateDraft(ctx context.Context, input agentknowledge.DraftInput) (string, error) {
	panic("draft generator should not be called")
}

type stubDraftGenerator struct {
	got agentknowledge.DraftInput
}

func (s *stubDraftGenerator) GenerateDraft(ctx context.Context, input agentknowledge.DraftInput) (string, error) {
	s.got = input
	return "# Draft", nil
}

func TestDraftFromSuggestionReturnsEmptyWithoutSourceSnippets(t *testing.T) {
	draft, err := DraftFromSuggestion(context.Background(), &models.KnowledgeUpdateSuggestion{
		CandidateType: models.SuggestionCandidateModule,
		CandidateName: "Billing-Core",
	}, failingDraftGenerator{})
	if err != nil {
		t.Fatalf("DraftFromSuggestion() returned error: %v", err)
	}
	if draft != "" {
		t.Fatalf("expected empty draft, got %q", draft)
	}
}

func TestDraftFromSuggestionCallsGeneratorWithContext(t *testing.T) {
	generator := &stubDraftGenerator{}
	draft, err := DraftFromSuggestion(context.Background(), &models.KnowledgeUpdateSuggestion{
		CandidateType: models.SuggestionCandidateModule,
		CandidateName: "Billing-Core",
		SourceSnippets: []map[string]any{
			{"text": "Billing-Core 需要校验账单明细"},
		},
	}, generator)
	if err != nil {
		t.Fatalf("DraftFromSuggestion() returned error: %v", err)
	}
	if draft != "# Draft" {
		t.Fatalf("unexpected draft: %q", draft)
	}
	if generator.got.CandidateName != "Billing-Core" || len(generator.got.SourceSnippets) != 1 {
		t.Fatalf("unexpected generator input: %+v", generator.got)
	}
}
