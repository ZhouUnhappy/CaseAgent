package task

import (
	"errors"
	"testing"

	"caseagent/internal/db/models"
	agentservice "caseagent/internal/service/agent"
	retrievalservice "caseagent/internal/service/retrieval"
)

func TestParseGeneratedSectionsSectionedJSON(t *testing.T) {
	raw := "```json\n[\n  {\n    \"section\": \"功能测试\",\n    \"cases\": [\n      {\n        \"title\": \"验证创建成功\",\n        \"priority_id\": 4,\n        \"custom_preconds\": \"服务已启动\",\n        \"custom_steps_separated\": [\n          {\"content\": \"提交创建请求\", \"expected\": \"请求成功\"}\n        ]\n      }\n    ]\n  }\n]\n```"

	sections, err := parseGeneratedSections(raw)
	if err != nil {
		t.Fatalf("parseGeneratedSections returned error: %v", err)
	}

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}

	if sections[0].Section != "功能测试" {
		t.Fatalf("expected section 功能测试, got %s", sections[0].Section)
	}

	if len(sections[0].Cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(sections[0].Cases))
	}

	if sections[0].Cases[0]["title"] != "验证创建成功" {
		t.Fatalf("unexpected case title: %#v", sections[0].Cases[0]["title"])
	}
}

func TestParseGeneratedSectionsFlatJSON(t *testing.T) {
	raw := `[
	  {
	    "type": "故障测试",
	    "title": "节点重启后恢复",
	    "description": "验证节点重启场景",
	    "steps": ["重启节点", "检查服务状态"],
	    "expected_result": "业务恢复正常"
	  }
	]`

	sections, err := parseGeneratedSections(raw)
	if err != nil {
		t.Fatalf("parseGeneratedSections returned error: %v", err)
	}

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}

	if sections[0].Section != "故障测试" {
		t.Fatalf("expected section 故障测试, got %s", sections[0].Section)
	}

	caseItem := sections[0].Cases[0]
	if caseItem["priority_id"] != 3 {
		t.Fatalf("expected default priority 3, got %#v", caseItem["priority_id"])
	}

	steps, ok := caseItem["custom_steps_separated"].([]map[string]any)
	if !ok {
		t.Fatalf("expected normalized steps slice, got %#v", caseItem["custom_steps_separated"])
	}

	if len(steps) != 2 {
		t.Fatalf("expected 2 normalized steps, got %d", len(steps))
	}

	if steps[1]["expected"] != "业务恢复正常" {
		t.Fatalf("expected final step to carry expected result, got %#v", steps[1]["expected"])
	}
}

func TestInferAffectedKnowledge(t *testing.T) {
	requirements := "本次需求涉及 Product-A 的升级流程，同时需要校验模块 Module-B 的异常处理。"
	knowledge := []models.KnowledgeBase{
		{Type: "product", Name: "Product-A"},
		{Type: "module", Name: "Module-B"},
		{Type: "module", Name: "Module-C", Metadata: map[string]any{"aliases": []any{"模块C"}}},
	}

	products, modules := inferAffectedKnowledge(requirements, knowledge)

	if len(products) != 1 || products[0] != "Product-A" {
		t.Fatalf("unexpected products: %#v", products)
	}

	if len(modules) != 1 || modules[0] != "Module-B" {
		t.Fatalf("unexpected modules: %#v", modules)
	}
}

func TestBuildKnowledgeQueries(t *testing.T) {
	requirements := "升级 Product-A。需要覆盖 Module-B 的故障恢复；并验证回滚流程。"
	queries := buildKnowledgeQueries(requirements, []string{"Product-A"}, []string{"Module-B"})

	if len(queries) < 3 {
		t.Fatalf("expected multiple knowledge queries, got %#v", queries)
	}

	if queries[0] != requirements {
		t.Fatalf("expected full requirements as first query, got %q", queries[0])
	}
}

func TestDedupeGeneratedSections(t *testing.T) {
	sections := []generatedSection{
		{
			Section: "功能测试",
			Cases: []map[string]any{
				{
					"title":           "验证创建成功",
					"custom_preconds": "服务已启动",
					"custom_steps_separated": []map[string]any{
						{"content": "提交请求", "expected": "成功"},
					},
				},
			},
		},
		{
			Section: "边界测试",
			Cases: []map[string]any{
				{
					"title":           "验证创建成功",
					"custom_preconds": "服务已启动",
					"custom_steps_separated": []map[string]any{
						{"content": "提交请求", "expected": "成功"},
					},
				},
			},
		},
	}

	deduped := dedupeGeneratedSections(sections)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 section after dedupe, got %d", len(deduped))
	}
	if len(deduped[0].Cases) != 1 {
		t.Fatalf("expected 1 case after dedupe, got %d", len(deduped[0].Cases))
	}
}

func TestBuildDocumentQueries(t *testing.T) {
	requirements := "升级 Product-A。需要覆盖 Module-B 的故障恢复；并验证回滚流程。"
	queries := buildDocumentQueries(requirements, []string{"Product-A"}, []string{"Module-B"})

	if len(queries) < 3 {
		t.Fatalf("expected multiple document queries, got %#v", queries)
	}
	if queries[0] != requirements {
		t.Fatalf("expected full requirements as first query, got %q", queries[0])
	}
}

func TestAttachCaseContext(t *testing.T) {
	sections := []generatedSection{
		{
			Section: "功能测试",
			Cases: []map[string]any{
				{
					"title": "验证创建成功",
				},
			},
		},
	}

	enriched := attachCaseContext(sections, []string{"Product-A"}, []string{"Module-B"})
	if len(enriched) != 1 || len(enriched[0].Cases) != 1 {
		t.Fatalf("unexpected enriched result: %#v", enriched)
	}

	caseItem := enriched[0].Cases[0]
	products, ok := caseItem["affected_products"].([]string)
	if !ok || len(products) != 1 || products[0] != "Product-A" {
		t.Fatalf("unexpected affected_products: %#v", caseItem["affected_products"])
	}
	modules, ok := caseItem["affected_modules"].([]string)
	if !ok || len(modules) != 1 || modules[0] != "Module-B" {
		t.Fatalf("unexpected affected_modules: %#v", caseItem["affected_modules"])
	}
	if caseItem["section"] != "功能测试" {
		t.Fatalf("unexpected section field: %#v", caseItem["section"])
	}
}

func TestBuildSourceContext(t *testing.T) {
	docHits := []retrievalservice.DocumentResult{
		{
			DocumentID:  101,
			ParentDocID: 200,
			Name:        "需求文档 A",
			Rank:        1,
			BestScore:   0.91,
			HitQueries:  []string{"升级流程", "回滚流程"},
			MatchedChunks: []retrievalservice.MatchedChunk{
				{Text: "升级步骤片段", Score: 0.92, Query: "升级流程", Rank: 1},
				{Text: "回滚步骤片段", Score: 0.88, Query: "回滚流程", Rank: 2},
				{Text: "异常处理片段", Score: 0.81, Query: "升级流程", Rank: 3},
				{Text: "应当被截断的第 4 个片段", Score: 0.70, Query: "升级流程", Rank: 4},
			},
		},
	}
	kbHits := []retrievalservice.KnowledgeResult{
		{ID: 11, Type: "product", Name: "Product-A", Rank: 1, Score: 0.83, HitQueries: []string{"Product-A"}},
		{ID: 22, Type: "module", Name: "Module-B", Rank: 2, Score: 0.79, HitQueries: []string{"Module-B"}},
	}
	shipped := []models.KnowledgeBase{
		{ID: 11, Name: "Product-A"},
		{ID: 22, Name: "Module-B"},
		{ID: 33, Name: "Module-C"},
		{ID: 0, Name: "should be skipped"},
	}

	ctx := buildSourceContext(
		[]string{"升级 Product-A", "回滚流程"},
		[]string{"Product-A", "Module-B"},
		docHits,
		kbHits,
		shipped,
		[]agentservice.ModelCallTrace{
			{
				ID:                  501,
				AgentRunID:          301,
				Agent:               "functional",
				Attempt:             "initial",
				Provider:            "fake",
				Model:               "valid_json",
				ProviderRole:        "primary",
				Status:              "succeeded",
				PromptID:            "functional_cases",
				PromptVersion:       "v1",
				EstimatedTotalToken: 42,
				TokenSource:         "estimated_chars",
			},
		},
	)

	docQueries, ok := ctx["document_queries"].([]string)
	if !ok || len(docQueries) != 2 || docQueries[0] != "升级 Product-A" {
		t.Fatalf("unexpected document_queries: %#v", ctx["document_queries"])
	}

	kbQueries, ok := ctx["knowledge_queries"].([]string)
	if !ok || len(kbQueries) != 2 {
		t.Fatalf("unexpected knowledge_queries: %#v", ctx["knowledge_queries"])
	}

	docs, ok := ctx["document_hits"].([]map[string]any)
	if !ok || len(docs) != 1 {
		t.Fatalf("unexpected document_hits: %#v", ctx["document_hits"])
	}
	doc := docs[0]
	if doc["document_id"] != 101 || doc["parent_doc_id"] != 200 {
		t.Fatalf("unexpected doc ids: %#v", doc)
	}
	if doc["name"] != "需求文档 A" || doc["rank"] != 1 || doc["best_score"] != 0.91 {
		t.Fatalf("unexpected doc summary: %#v", doc)
	}
	chunks, ok := doc["top_chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("expected top_chunks slice, got %#v", doc["top_chunks"])
	}
	if len(chunks) != 3 {
		t.Fatalf("expected top 3 chunks (cap of 3), got %d", len(chunks))
	}
	if chunks[0]["text"] != "升级步骤片段" || chunks[0]["query"] != "升级流程" || chunks[0]["rank"] != 1 {
		t.Fatalf("unexpected first chunk: %#v", chunks[0])
	}

	kbs, ok := ctx["knowledge_hits"].([]map[string]any)
	if !ok || len(kbs) != 2 || kbs[0]["id"] != 11 || kbs[1]["id"] != 22 {
		t.Fatalf("unexpected knowledge_hits: %#v", ctx["knowledge_hits"])
	}

	shippedIDs, ok := ctx["knowledge_shipped_ids"].([]int)
	if !ok || len(shippedIDs) != 3 || shippedIDs[0] != 11 || shippedIDs[2] != 33 {
		t.Fatalf("unexpected knowledge_shipped_ids (should skip id=0): %#v", ctx["knowledge_shipped_ids"])
	}
	shippedNames, ok := ctx["knowledge_shipped_names"].([]string)
	if !ok || len(shippedNames) != 3 || shippedNames[0] != "Product-A" {
		t.Fatalf("unexpected knowledge_shipped_names: %#v", ctx["knowledge_shipped_names"])
	}

	modelCalls, ok := ctx["model_calls"].([]map[string]any)
	if !ok || len(modelCalls) != 1 || modelCalls[0]["id"] != 501 || modelCalls[0]["prompt_version"] != "v1" {
		t.Fatalf("unexpected model_calls provenance: %#v", ctx["model_calls"])
	}
	agentRuns, ok := ctx["agent_runs"].([]map[string]any)
	if !ok || len(agentRuns) != 1 || agentRuns[0]["id"] != 301 {
		t.Fatalf("unexpected agent_runs provenance: %#v", ctx["agent_runs"])
	}
}

func TestGenerationFailureStageAndContextGapEligibility(t *testing.T) {
	parseErr := generationFailure(GenerationStageParseCases, "bad response: %w", errors.New("not json"))
	if GenerationFailureStage(parseErr) != GenerationStageParseCases {
		t.Fatalf("GenerationFailureStage() = %q", GenerationFailureStage(parseErr))
	}
	if !ShouldRecordContextGap(parseErr) {
		t.Fatal("parse failures should record context_gap")
	}

	initErr := generationFailure(GenerationStageInitializeAgent, "bad config")
	if ShouldRecordContextGap(initErr) {
		t.Fatal("initialize failures should not record context_gap")
	}

	agentErr := generationFailure(
		agentservice.GenerationStageDeepAgentFallback,
		"failed to generate cases: %w",
		errors.New("provider rejected request"),
	)
	if GenerationFailureStage(agentErr) != agentservice.GenerationStageDeepAgentFallback {
		t.Fatalf("agent stage not preserved: %q", GenerationFailureStage(agentErr))
	}
	if !ShouldRecordContextGap(agentErr) {
		t.Fatal("deep agent fallback failures should record context_gap")
	}
}

func TestGenerationFailureNonRetryableForGuardrailErrors(t *testing.T) {
	err := generationFailure(GenerationStageAgentGenerate, "failed to generate cases: %w", agentservice.ErrModelBudgetExceeded)
	var nonRetryable interface {
		NonRetryable() bool
	}
	if !errors.As(err, &nonRetryable) || !nonRetryable.NonRetryable() {
		t.Fatalf("guardrail generation failure should be non-retryable: %v", err)
	}
}
