package feedback

import (
	"testing"

	"caseagent/internal/db/models"
)

func TestSummarizeFeedbackRows(t *testing.T) {
	rows := []models.TestCaseFeedback{
		{FeedbackType: models.CaseFeedbackDuplicate, PromptID: "functional", PromptVersion: "v1"},
		{FeedbackType: models.CaseFeedbackUseful, PromptID: "functional", PromptVersion: "v1"},
		{FeedbackType: models.CaseFeedbackDuplicate, PromptID: "functional", PromptVersion: "v1"},
		{FeedbackType: models.CaseFeedbackKnowledgeMissing, PromptID: "ops", PromptVersion: "v2"},
	}

	summary := SummarizeFeedbackRows(rows)

	if summary.Total != 4 {
		t.Fatalf("Total = %d, want 4", summary.Total)
	}
	if len(summary.ByType) != 3 || summary.ByType[0].FeedbackType != models.CaseFeedbackDuplicate || summary.ByType[0].Count != 2 {
		t.Fatalf("ByType = %#v, want duplicate first with count 2", summary.ByType)
	}
	if len(summary.ByPrompt) != 2 {
		t.Fatalf("ByPrompt len = %d, want 2", len(summary.ByPrompt))
	}
	if got := summary.ByPrompt[0]; got.PromptID != "functional" || got.Total != 3 || got.Useful != 1 || got.Negative != 2 {
		t.Fatalf("first prompt summary = %#v, want functional totals", got)
	}
}
