package ai

import (
	"context"
	"strings"
	"testing"
	"time"

	"caseagent/internal/config"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestNewChatModelSupportsDeepSeek(t *testing.T) {
	chatModel, err := NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: " deepseek ",
		Model:    "deepseek-v4-pro",
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}
	if chatModel == nil {
		t.Fatal("NewChatModel() returned nil model")
	}

	var _ model.BaseChatModel = chatModel
}

func TestNewChatModelRejectsUnsupportedProvider(t *testing.T) {
	_, err := NewChatModel(context.Background(), config.ChatModelConfig{Provider: "unknown"})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if !strings.Contains(err.Error(), "unsupported chat provider: unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewChatModelFakeValidJSON(t *testing.T) {
	chatModel, err := NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "valid_json",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}

	result, err := chatModel.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("你是一个功能测试专家。"),
	})
	if err != nil {
		t.Fatalf("Generate() returned error: %v", err)
	}
	if !strings.Contains(result.Content, `"section": "功能测试"`) {
		t.Fatalf("unexpected fake content: %s", result.Content)
	}
}

func TestNewChatModelFakeInvalidJSON(t *testing.T) {
	chatModel, err := NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "invalid_json",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}

	result, err := chatModel.Generate(context.Background(), []*schema.Message{schema.UserMessage("prompt")})
	if err != nil {
		t.Fatalf("Generate() returned error: %v", err)
	}
	if result.Content != "{not-json" {
		t.Fatalf("unexpected fake invalid JSON: %q", result.Content)
	}
}

func TestNewChatModelFakeRateLimit(t *testing.T) {
	chatModel, err := NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "rate_limit",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}

	_, err = chatModel.Generate(context.Background(), []*schema.Message{schema.UserMessage("prompt")})
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("Generate() error = %v, want rate limited", err)
	}
}

func TestNewChatModelFakeTimeout(t *testing.T) {
	chatModel, err := NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "timeout",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err = chatModel.Generate(ctx, []*schema.Message{schema.UserMessage("prompt")})
	if err == nil || err != context.DeadlineExceeded {
		t.Fatalf("Generate() error = %v, want deadline exceeded", err)
	}
}

func TestNewChatModelFakePartialFailure(t *testing.T) {
	chatModel, err := NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "partial_failure",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}

	_, err = chatModel.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("你是一个运维测试专家。"),
	})
	if err == nil || !strings.Contains(err.Error(), "partial failure") {
		t.Fatalf("Generate() error = %v, want partial failure", err)
	}

	result, err := chatModel.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("你是一个功能测试专家。"),
	})
	if err != nil {
		t.Fatalf("Generate() functional error = %v", err)
	}
	if !strings.Contains(result.Content, "功能测试") {
		t.Fatalf("unexpected fake content: %s", result.Content)
	}
}

func TestOpenAICompatibleChatConfigDefaultsBaseURL(t *testing.T) {
	cfg := openAICompatibleChatConfig(config.ChatModelConfig{
		APIKey: "test-key",
		Model:  "deepseek-v4-pro",
	}, deepSeekDefaultBaseURL)

	if cfg.BaseURL != deepSeekDefaultBaseURL {
		t.Fatalf("unexpected BaseURL: got %q want %q", cfg.BaseURL, deepSeekDefaultBaseURL)
	}
	if cfg.APIKey != "test-key" {
		t.Fatalf("unexpected APIKey: %q", cfg.APIKey)
	}
	if cfg.Model != "deepseek-v4-pro" {
		t.Fatalf("unexpected model: %q", cfg.Model)
	}
}

func TestOpenAICompatibleChatConfigKeepsCustomBaseURL(t *testing.T) {
	cfg := openAICompatibleChatConfig(config.ChatModelConfig{
		BaseURL: " https://example.test/v1/ ",
	}, deepSeekDefaultBaseURL)

	if cfg.BaseURL != "https://example.test/v1/" {
		t.Fatalf("unexpected BaseURL: %q", cfg.BaseURL)
	}
}
