package task

import (
	"testing"

	"caseagent/internal/db/models"
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
