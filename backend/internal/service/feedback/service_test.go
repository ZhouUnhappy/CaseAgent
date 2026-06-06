package feedback

import (
	"errors"
	"testing"

	"caseagent/internal/db/models"
)

func TestValidateCreateInput(t *testing.T) {
	valid := CreateInput{
		TaskID:       1,
		TestCaseID:   2,
		CaseIndex:    0,
		FeedbackType: models.CaseFeedbackUseful,
	}
	if err := validateCreateInput(valid); err != nil {
		t.Fatalf("validateCreateInput(valid) = %v", err)
	}

	invalid := valid
	invalid.FeedbackType = "semantic_score"
	if err := validateCreateInput(invalid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("validateCreateInput(invalid type) = %v, want ErrInvalidInput", err)
	}

	invalid = valid
	invalid.CaseIndex = -1
	if err := validateCreateInput(invalid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("validateCreateInput(negative index) = %v, want ErrInvalidInput", err)
	}
}

func TestSummarizeSourceContext(t *testing.T) {
	source := map[string]any{
		"document_queries":        []any{"登录流程"},
		"knowledge_queries":       []any{"Product-A"},
		"knowledge_shipped_ids":   []any{11, 22},
		"knowledge_shipped_names": []any{"Product-A", "Module-B"},
		"document_hits": []any{
			map[string]any{"document_id": 1, "name": "doc-1", "rank": 1, "best_score": 0.9, "ignored": "x"},
			map[string]any{"document_id": 2, "name": "doc-2", "rank": 2, "best_score": 0.8},
			map[string]any{"document_id": 3, "name": "doc-3", "rank": 3, "best_score": 0.7},
			map[string]any{"document_id": 4, "name": "doc-4", "rank": 4, "best_score": 0.6},
			map[string]any{"document_id": 5, "name": "doc-5", "rank": 5, "best_score": 0.5},
			map[string]any{"document_id": 6, "name": "doc-6", "rank": 6, "best_score": 0.4},
		},
		"knowledge_hits": []any{
			map[string]any{"id": 11, "name": "Product-A", "type": "product", "rank": 1, "score": 0.88},
		},
		"model_calls": []any{
			map[string]any{"id": 501},
			map[string]any{"id": 502},
		},
	}

	summary := SummarizeSourceContext(source)
	if summary["document_hit_count"] != 6 || summary["knowledge_hit_count"] != 1 || summary["model_call_count"] != 2 {
		t.Fatalf("unexpected summary counts: %#v", summary)
	}
	docs, ok := summary["document_hits"].([]map[string]any)
	if !ok {
		t.Fatalf("document_hits = %#v, want []map[string]any", summary["document_hits"])
	}
	if len(docs) != 5 {
		t.Fatalf("document summary length = %d, want max 5", len(docs))
	}
	if _, ok := docs[0]["ignored"]; ok {
		t.Fatalf("document summary leaked non-summary key: %#v", docs[0])
	}
	if summary["knowledge_shipped_names"] == nil {
		t.Fatalf("expected shipped knowledge names in summary: %#v", summary)
	}
}

func TestSelectTraceModelCall(t *testing.T) {
	source := map[string]any{
		"model_calls": []any{
			map[string]any{
				"id":             501,
				"status":         models.WorkflowStatusFailed,
				"prompt_id":      "functional_cases",
				"prompt_version": "v1",
			},
			map[string]any{
				"id":             502,
				"status":         models.WorkflowStatusSucceeded,
				"prompt_id":      "deep_fallback_cases",
				"prompt_version": "v2",
			},
		},
	}

	id, promptID, promptVersion := SelectTraceModelCall(source)
	if id == nil || *id != 502 || promptID != "deep_fallback_cases" || promptVersion != "v2" {
		t.Fatalf("SelectTraceModelCall() = id=%v prompt=%q version=%q", id, promptID, promptVersion)
	}
}
