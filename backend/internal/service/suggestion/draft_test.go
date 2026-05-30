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
	draft, err := DraftFromSuggestion(context.Background(), &models.KnowledgeUpdateSuggestionGroup{
		CandidateType: models.SuggestionCandidateModule,
		CandidateName: "Billing-Core",
	}, nil, failingDraftGenerator{})
	if err != nil {
		t.Fatalf("DraftFromSuggestion() returned error: %v", err)
	}
	if draft != "" {
		t.Fatalf("expected empty draft, got %q", draft)
	}
}

func TestDraftFromSuggestionCallsGeneratorWithContext(t *testing.T) {
	generator := &stubDraftGenerator{}
	sourceCaseID := 9
	draft, err := DraftFromSuggestion(context.Background(), &models.KnowledgeUpdateSuggestionGroup{
		CandidateType: models.SuggestionCandidateModule,
		CandidateName: "Billing-Core",
	}, []models.KnowledgeUpdateSuggestionOccurrence{
		{
			SourceTaskID: 42,
			SourceCaseID: &sourceCaseID,
			Frequency:    3,
			SourceSnippets: []map[string]any{
				{"text": "Billing-Core 需要校验账单明细"},
			},
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
	if generator.got.SourceSnippets[0]["source_task_id"] != 42 {
		t.Fatalf("source task not carried into snippet: %+v", generator.got.SourceSnippets[0])
	}
}

func TestFlattenOccurrenceSnippetsKeepsOccurrenceContext(t *testing.T) {
	sourceCaseID := 7
	snippets := flattenOccurrenceSnippets([]models.KnowledgeUpdateSuggestionOccurrence{
		{
			SourceTaskID: 5,
			SourceCaseID: &sourceCaseID,
			Frequency:    2,
			SourceSnippets: []map[string]any{
				{"text": "原始片段"},
			},
		},
	})

	if len(snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(snippets))
	}
	want := map[string]any{
		"text":           "原始片段",
		"source_task_id": 5,
		"source_case_id": 7,
		"frequency":      2,
	}
	for key, value := range want {
		if snippets[0][key] != value {
			t.Fatalf("snippet[%s] = %#v, want %#v in %+v", key, snippets[0][key], value, snippets[0])
		}
	}
}

func TestFlattenOccurrenceSnippetsDoesNotMutateOriginal(t *testing.T) {
	original := map[string]any{"text": "原始片段"}
	snippets := flattenOccurrenceSnippets([]models.KnowledgeUpdateSuggestionOccurrence{
		{
			SourceTaskID: 5,
			Frequency:    2,
			SourceSnippets: []map[string]any{
				original,
			},
		},
	})
	snippets[0]["text"] = "changed"
	if original["text"] != "原始片段" {
		t.Fatalf("original snippet mutated: %+v", original)
	}
}
