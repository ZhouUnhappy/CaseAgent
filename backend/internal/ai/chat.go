package ai

import (
	"context"
	"fmt"
	"strings"

	"caseagent/internal/config"

	arkmodel "github.com/cloudwego/eino-ext/components/model/ark"
	openaiacl "github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/components/model"
)

const (
	deepSeekDefaultBaseURL = "https://api.deepseek.com"
)

func NewChatModel(ctx context.Context, cfg config.ChatModelConfig) (model.BaseChatModel, error) {
	switch normalizeProvider(cfg.Provider) {
	case "ark":
		return arkmodel.NewChatModel(ctx, &arkmodel.ChatModelConfig{
			APIKey:    cfg.APIKey,
			AccessKey: cfg.AccessKey,
			SecretKey: cfg.SecretKey,
			BaseURL:   cfg.BaseURL,
			Region:    cfg.Region,
			Model:     cfg.Model,
		})
	case "deepseek":
		return openaiacl.NewClient(ctx, openAICompatibleChatConfig(cfg, deepSeekDefaultBaseURL))
	case "openai":
		return openaiacl.NewClient(ctx, openAICompatibleChatConfig(cfg, ""))
	default:
		return nil, fmt.Errorf("unsupported chat provider: %s", cfg.Provider)
	}
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func openAICompatibleChatConfig(cfg config.ChatModelConfig, defaultBaseURL string) *openaiacl.Config {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return &openaiacl.Config{
		APIKey:  cfg.APIKey,
		BaseURL: baseURL,
		Model:   cfg.Model,
	}
}
