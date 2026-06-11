package handler

import (
	"testing"

	"caseagent/internal/db/models"
)

func TestBuildTaskDiagnosticsSummary(t *testing.T) {
	trace := newTaskTraceView([]models.WorkflowRun{
		{ID: 1, Status: models.WorkflowStatusSucceeded},
		{ID: 2, Status: models.WorkflowStatusFailed, LastError: "generation failed"},
	})
	trace.AgentRuns = []models.AgentRun{{ID: 11}}
	trace.ModelCalls = []models.ModelCall{{ID: 21}}
	trace.RetrievalRuns = []models.RetrievalRun{{ID: 31}}
	trace.Artifacts = []models.Artifact{{ID: 41}}
	trace.FeedbackSummary = map[string]int{models.CaseFeedbackDuplicate: 2}

	summary := buildTaskDiagnosticsSummary([]models.TestCase{
		{ID: 101, Cases: []map[string]any{{"title": "a"}, {"title": "b"}}},
		{ID: 102, Cases: []map[string]any{{"title": "c"}}},
	}, trace)

	if summary.SectionCount != 2 || summary.CaseCount != 3 {
		t.Fatalf("section/case counts = %d/%d, want 2/3", summary.SectionCount, summary.CaseCount)
	}
	if summary.WorkflowRuns != 2 || summary.AgentRuns != 1 || summary.ModelCalls != 1 || summary.RetrievalRuns != 1 || summary.Artifacts != 1 {
		t.Fatalf("unexpected trace counts: %#v", summary)
	}
	if summary.FeedbackCount != 2 {
		t.Fatalf("FeedbackCount = %d, want 2", summary.FeedbackCount)
	}
	if summary.LastError != "generation failed" {
		t.Fatalf("LastError = %q, want generation failed", summary.LastError)
	}
}
