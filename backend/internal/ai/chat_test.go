package ai

import (
	"context"
	"strings"
	"testing"

	"caseagent/internal/config"

	"github.com/cloudwego/eino/components/model"
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
