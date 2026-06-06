package handler

import (
	"testing"

	"caseagent/internal/db/models"
)

func TestBuildCaseProvenanceViewsLinksCasesToTraceRows(t *testing.T) {
	agentRunID := 301
	testCases := []models.TestCase{
		{
			ID:      10,
			Section: "功能测试",
			Cases: []map[string]any{
				{"title": "验证创建成功"},
			},
			SourceContext: map[string]any{
				"document_queries":  []any{"创建流程"},
				"knowledge_queries": []any{"Product-A"},
				"document_hits": []any{
					map[string]any{"document_id": 101, "name": "需求文档", "rank": 1, "best_score": 0.91},
				},
				"knowledge_hits": []any{
					map[string]any{"id": 11, "name": "Product-A", "rank": 1, "score": 0.88},
				},
				"agent_runs": []any{
					map[string]any{"id": agentRunID, "agent": "functional"},
				},
				"model_calls": []any{
					map[string]any{"id": 501, "agent_run_id": agentRunID, "prompt_id": "functional_cases"},
				},
			},
		},
	}
	agentRuns := []models.AgentRun{
		{ID: agentRunID, AgentName: "functional", Stage: "initial", Status: models.WorkflowStatusSucceeded},
	}
	modelCalls := []models.ModelCall{
		{
			ID:          501,
			AgentRunID:  &agentRunID,
			Provider:    "fake",
			Model:       "valid_json",
			Status:      models.WorkflowStatusSucceeded,
			PromptChars: 120,
			Metadata: map[string]any{
				"agent":          "functional",
				"attempt":        "initial",
				"prompt_id":      "functional_cases",
				"prompt_version": "v1",
				"provider_role":  "primary",
			},
		},
	}

	views := buildCaseProvenanceViews(testCases, agentRuns, modelCalls)
	if len(views) != 1 {
		t.Fatalf("views = %#v, want one row", views)
	}
	view := views[0]
	if view.CaseTitle != "验证创建成功" || view.TestCaseID != 10 || view.CaseIndex != 0 {
		t.Fatalf("unexpected case view: %#v", view)
	}
	if len(view.AgentRuns) != 1 || view.AgentRuns[0].ID != agentRunID || view.AgentRuns[0].Agent != "functional" {
		t.Fatalf("unexpected agent provenance: %#v", view.AgentRuns)
	}
	if len(view.ModelCalls) != 1 || view.ModelCalls[0].ID != 501 || view.ModelCalls[0].PromptVersion != "v1" {
		t.Fatalf("unexpected model provenance: %#v", view.ModelCalls)
	}
	if view.DocumentQueries == nil || view.KnowledgeHits == nil {
		t.Fatalf("expected retrieval provenance in view: %#v", view)
	}
}
