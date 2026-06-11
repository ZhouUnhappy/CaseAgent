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
	trace.AgentRuns = []models.AgentRun{{ID: 11, Status: models.WorkflowStatusSucceeded}}
	trace.ModelCalls = []models.ModelCall{{ID: 21, Status: models.WorkflowStatusFailed, LastError: "rate limit"}}
	trace.RetrievalRuns = []models.RetrievalRun{{ID: 31, Status: models.WorkflowStatusSucceeded}}
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
	if summary.AgentRunsByStatus[models.WorkflowStatusSucceeded] != 1 || summary.ModelCallsByStatus[models.WorkflowStatusFailed] != 1 || summary.RetrievalsByStatus[models.WorkflowStatusSucceeded] != 1 {
		t.Fatalf("unexpected status summaries: %#v", summary)
	}
	if len(summary.Errors) != 2 || summary.Errors[0].Source != "workflow_run" || summary.Errors[1].Source != "model_call" {
		t.Fatalf("unexpected error summaries: %#v", summary.Errors)
	}
	if len(summary.SourceContexts) != 2 {
		t.Fatalf("SourceContexts len = %d, want 2", len(summary.SourceContexts))
	}
}

func TestDiagnosticsRedactsSensitiveFields(t *testing.T) {
	rawPrompt := "prompt:" + longString(700)
	source := map[string]any{
		"document_queries":  []any{"raw requirement query"},
		"knowledge_queries": []any{"raw knowledge query"},
		"document_hits": []any{
			map[string]any{"document_id": 7, "name": "Requirement", "rank": 1, "best_score": 0.91, "content": "private requirement body"},
		},
		"knowledge_hits": []any{
			map[string]any{"id": 9, "name": "Module", "type": "module", "rank": 1, "score": 0.82, "content": "private knowledge body"},
		},
		"model_calls": []any{map[string]any{"id": 21}},
	}
	trace := taskTraceView{
		WorkflowRuns: []models.WorkflowRun{{ID: 1, Metadata: map[string]any{"api_key": "secret", "safe": "ok"}}},
		AgentRuns: []models.AgentRun{{
			ID:            11,
			Status:        models.WorkflowStatusSucceeded,
			InputSummary:  longString(700),
			OutputSummary: "short",
			Metadata:      map[string]any{"prompt": rawPrompt, "safe": "ok"},
		}},
		ModelCalls: []models.ModelCall{{
			ID:       21,
			Status:   models.WorkflowStatusSucceeded,
			Metadata: map[string]any{"authorization": "Bearer secret", "latency_ms": 12},
		}},
		Artifacts: []models.Artifact{{
			ID:      31,
			Content: "private artifact content",
			Payload: map[string]any{"body": "private body", "count": 1},
		}},
		CaseProvenance: []caseProvenanceView{{
			TestCaseID:       41,
			SourceContext:    source,
			DocumentQueries:  []any{"raw requirement query"},
			KnowledgeQueries: []any{"raw knowledge query"},
			DocumentHits:     source["document_hits"],
			KnowledgeHits:    source["knowledge_hits"],
			ModelCalls: []modelCallProvenance{{
				ID:       21,
				Metadata: map[string]any{"prompt": rawPrompt},
			}},
		}},
		Feedback: []models.TestCaseFeedback{{
			ID:                   51,
			Note:                 longString(700),
			SourceContextSummary: map[string]any{"raw": "private"},
			Metadata:             map[string]any{"token": "secret"},
		}},
		FeedbackSummary: map[string]int{},
	}

	redacted := redactDiagnosticsTrace(trace)

	if redacted.WorkflowRuns[0].Metadata["api_key"] == "secret" {
		t.Fatal("api_key metadata was not redacted")
	}
	if redacted.AgentRuns[0].Metadata["prompt"] == rawPrompt {
		t.Fatal("prompt metadata was not redacted")
	}
	if redacted.AgentRuns[0].InputSummary == trace.AgentRuns[0].InputSummary {
		t.Fatal("long input summary was not truncated")
	}
	if redacted.ModelCalls[0].Metadata["authorization"] == "Bearer secret" {
		t.Fatal("authorization metadata was not redacted")
	}
	if redacted.Artifacts[0].Content == "private artifact content" {
		t.Fatal("artifact content was not redacted")
	}
	if redacted.Artifacts[0].Payload["body"] == "private body" {
		t.Fatal("artifact payload body was not redacted")
	}
	if redacted.CaseProvenance[0].DocumentQueries != 1 || redacted.CaseProvenance[0].KnowledgeQueries != 1 {
		t.Fatalf("queries were not summarized as counts: %#v", redacted.CaseProvenance[0])
	}
	docHits, ok := redacted.CaseProvenance[0].DocumentHits.([]map[string]any)
	if !ok || len(docHits) != 1 || docHits[0]["content"] != nil || docHits[0]["document_id"] != 7 {
		t.Fatalf("document hits were not safely summarized: %#v", redacted.CaseProvenance[0].DocumentHits)
	}
	if redacted.Feedback[0].Metadata["token"] == "secret" || redacted.Feedback[0].SourceContextSummary["raw"] == "private" {
		t.Fatalf("feedback sensitive fields were not redacted: %#v", redacted.Feedback[0])
	}
}

func TestBuildDiagnosticTestCasesSummarizesSourceContext(t *testing.T) {
	rows := buildDiagnosticTestCases([]models.TestCase{{
		ID:      101,
		Section: "checkout",
		Cases:   []map[string]any{{"title": "case"}},
		SourceContext: map[string]any{
			"document_queries": []any{"raw query"},
			"document_hits": []any{
				map[string]any{"document_id": 7, "name": "Requirement", "content": "private body"},
			},
		},
	}})

	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	summary := rows[0].SourceContextSummary
	if _, ok := summary["document_queries"]; ok {
		t.Fatalf("summary leaked document_queries: %#v", summary)
	}
	if summary["document_query_count"] != 1 {
		t.Fatalf("document_query_count = %#v, want 1", summary["document_query_count"])
	}
	hits, ok := summary["document_hits"].([]map[string]any)
	if !ok || len(hits) != 1 || hits[0]["content"] != nil || hits[0]["document_id"] != 7 {
		t.Fatalf("document_hits summary = %#v", summary["document_hits"])
	}
}

func longString(length int) string {
	out := make([]byte, length)
	for idx := range out {
		out[idx] = 'x'
	}
	return string(out)
}
