package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"caseagent/internal/ai"
	"caseagent/internal/config"
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
		out, err := runSubAgentWithRetry(ctx, "ok", time.Second, func(_ context.Context) (string, error) {
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
		out, err := runSubAgentWithRetry(ctx, "transient", time.Second, func(_ context.Context) (string, error) {
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
		_, err := runSubAgentWithRetry(ctx, "persistent", time.Second, func(_ context.Context) (string, error) {
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

	t.Run("deadline exceeded is not retried", func(t *testing.T) {
		calls := 0
		_, err := runSubAgentWithRetry(ctx, "timeout", time.Nanosecond, func(ctx context.Context) (string, error) {
			calls++
			<-ctx.Done()
			return "", ctx.Err()
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
		if calls != 1 {
			t.Fatalf("expected no retry for deadline exceeded, got %d calls", calls)
		}
	})
}

func TestConfiguredChatCallTimeout(t *testing.T) {
	if got := configuredChatCallTimeout(nil); got != defaultChatCallTimeout {
		t.Fatalf("nil config timeout = %s, want %s", got, defaultChatCallTimeout)
	}
	cfg := &config.Config{}
	if got := configuredChatCallTimeout(cfg); got != defaultChatCallTimeout {
		t.Fatalf("zero config timeout = %s, want %s", got, defaultChatCallTimeout)
	}
	cfg.Model.Chat.RequestTimeoutSeconds = 7
	if got := configuredChatCallTimeout(cfg); got != 7*time.Second {
		t.Fatalf("configured timeout = %s, want 7s", got)
	}
}

func TestGenerationErrorCarriesFailureStage(t *testing.T) {
	cause := errors.New("provider rejected request")
	err := generationError(GenerationStageDeepAgentFallback, cause)
	if err == nil {
		t.Fatal("expected generation error")
	}
	if FailureStage(err) != GenerationStageDeepAgentFallback {
		t.Fatalf("FailureStage() = %q, want %q", FailureStage(err), GenerationStageDeepAgentFallback)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("generation error should unwrap original cause")
	}
}

func TestGenerateCasesWithFakeProvider(t *testing.T) {
	chatModel, err := ai.NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "valid_json",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}
	service, err := New(context.Background(), &Config{ChatModel: chatModel, ChatCallTimeout: time.Second})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	output, err := service.GenerateCases(context.Background(), "fake requirements", "fake knowledge")
	if err != nil {
		t.Fatalf("GenerateCases() returned error: %v", err)
	}
	if !strings.Contains(output, `"section": "功能测试"`) {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestGenerateCasesWithFakePartialFailureProvider(t *testing.T) {
	chatModel, err := ai.NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "partial_failure",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}
	service, err := New(context.Background(), &Config{ChatModel: chatModel, ChatCallTimeout: time.Second})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	output, err := service.GenerateCases(context.Background(), "fake requirements", "fake knowledge")
	if err != nil {
		t.Fatalf("GenerateCases() returned error despite partial failure: %v", err)
	}
	if !strings.Contains(output, "功能测试") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestGenerateCasesWithFakeTimeoutProvider(t *testing.T) {
	chatModel, err := ai.NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "timeout",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}
	service, err := New(context.Background(), &Config{ChatModel: chatModel, ChatCallTimeout: time.Millisecond})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	_, err = service.GenerateCases(context.Background(), "fake requirements", "fake knowledge")
	if err == nil {
		t.Fatal("GenerateCases() expected timeout error")
	}
	if FailureStage(err) != GenerationStageDeepAgentFallback {
		t.Fatalf("FailureStage() = %q, want %q", FailureStage(err), GenerationStageDeepAgentFallback)
	}
}
