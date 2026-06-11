package feedback

import (
	"testing"
	"time"

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

func TestBuildQualityOverview(t *testing.T) {
	day1 := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 6, 11, 8, 0, 0, 0, time.UTC)
	taskID := 77
	overview := BuildQualityOverview(
		[]models.TestCaseFeedback{
			{
				TestCaseID:    101,
				FeedbackType:  models.CaseFeedbackUseful,
				PromptID:      "functional",
				PromptVersion: "v1",
				CreatedAt:     day1,
			},
			{
				TestCaseID:    101,
				FeedbackType:  models.CaseFeedbackDuplicate,
				PromptID:      "functional",
				PromptVersion: "v1",
				CreatedAt:     day1,
			},
			{
				TestCaseID:    102,
				FeedbackType:  models.CaseFeedbackKnowledgeMissing,
				PromptID:      "ops",
				PromptVersion: "v2",
				CreatedAt:     day2,
				SourceContextSummary: map[string]any{
					"generation_profile_id":      "summary-profile",
					"generation_profile_version": "s1",
				},
			},
		},
		[]models.TestCase{
			{
				ID: 101,
				SourceContext: map[string]any{
					"generation_profile_id":      "caseagent-generation-default",
					"generation_profile_version": "p1",
				},
			},
		},
		[]models.Artifact{
			{
				ID:           501,
				ArtifactType: models.ArtifactTypeGeneratedCases,
				ResourceType: "task",
				ResourceID:   &taskID,
				Name:         "deduped generated cases",
				Payload: map[string]any{
					"section_count": 2,
					"case_count":    8,
					"source_context": map[string]any{
						"generation_profile_id":      "caseagent-generation-default",
						"generation_profile_version": "p1",
					},
				},
				CreatedAt: day2,
			},
		},
	)

	if overview.TotalFeedback != 3 {
		t.Fatalf("TotalFeedback = %d, want 3", overview.TotalFeedback)
	}
	if len(overview.PromptComparison) != 2 || overview.PromptComparison[0].PromptID != "functional" || overview.PromptComparison[0].Negative != 1 {
		t.Fatalf("unexpected prompt comparison: %#v", overview.PromptComparison)
	}
	if len(overview.ProfileComparison) != 2 || overview.ProfileComparison[0].ProfileID != "caseagent-generation-default" || overview.ProfileComparison[0].Total != 2 {
		t.Fatalf("unexpected profile comparison: %#v", overview.ProfileComparison)
	}
	if len(overview.FeedbackTrend) != 3 || overview.FeedbackTrend[0].Date != "2026-06-10" {
		t.Fatalf("unexpected feedback trend: %#v", overview.FeedbackTrend)
	}
	if len(overview.ReportHistory) != 1 || overview.ReportHistory[0].TaskID != taskID || overview.ReportHistory[0].CaseCount != 8 || overview.ReportHistory[0].ProfileVersion != "p1" {
		t.Fatalf("unexpected report history: %#v", overview.ReportHistory)
	}
}
