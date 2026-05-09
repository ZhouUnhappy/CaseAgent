package agent

import (
	"context"
	"errors"
	"testing"
)

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

func TestRunSubAgentWithRetry(t *testing.T) {
	ctx := context.Background()

	t.Run("success on first try makes only one call", func(t *testing.T) {
		calls := 0
		out, err := runSubAgentWithRetry(ctx, "ok", func(_ context.Context) (string, error) {
			calls++
			return "payload", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "payload" {
			t.Fatalf("unexpected output %q", out)
		}
		if calls != 1 {
			t.Fatalf("expected 1 call when first try succeeds, got %d", calls)
		}
	})

	t.Run("transient failure recovered on retry", func(t *testing.T) {
		calls := 0
		out, err := runSubAgentWithRetry(ctx, "transient", func(_ context.Context) (string, error) {
			calls++
			if calls == 1 {
				return "", errors.New("rate limited")
			}
			return "payload-2", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "payload-2" {
			t.Fatalf("unexpected output %q", out)
		}
		if calls != 2 {
			t.Fatalf("expected exactly 2 calls (try + 1 retry), got %d", calls)
		}
	})

	t.Run("persistent failure surfaces after exactly one retry", func(t *testing.T) {
		calls := 0
		_, err := runSubAgentWithRetry(ctx, "persistent", func(_ context.Context) (string, error) {
			calls++
			return "", errors.New("hard fail")
		})
		if err == nil {
			t.Fatal("expected error after exhausting retry")
		}
		if calls != 2 {
			t.Fatalf("expected exactly 2 calls (try + 1 retry) before giving up, got %d", calls)
		}
	})
}
