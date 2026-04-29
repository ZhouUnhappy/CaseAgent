package agent

import "testing"

func TestParseGeneratedSections(t *testing.T) {
	raw := "```json\n[\n  {\n    \"section\": \"功能测试\",\n    \"cases\": [\n      {\n        \"title\": \"验证创建成功\",\n        \"custom_preconds\": \"服务已启动\",\n        \"custom_steps_separated\": [\n          {\"content\": \"提交请求\", \"expected\": \"成功\"}\n        ]\n      }\n    ]\n  }\n]\n```"

	sections, err := parseGeneratedSections(raw)
	if err != nil {
		t.Fatalf("parseGeneratedSections returned error: %v", err)
	}
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if sections[0].Section != "功能测试" {
		t.Fatalf("unexpected section: %q", sections[0].Section)
	}
}

func TestDedupeGeneratedSections(t *testing.T) {
	input := []generatedSection{
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

	got := dedupeGeneratedSections(input)
	if len(got) != 1 {
		t.Fatalf("expected one section after dedupe, got %d", len(got))
	}
	if len(got[0].Cases) != 1 {
		t.Fatalf("expected one case after dedupe, got %d", len(got[0].Cases))
	}
}
