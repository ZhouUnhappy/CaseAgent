package ai

import (
	"context"
	"fmt"
	"strings"

	"caseagent/internal/config"

	arkembedding "github.com/cloudwego/eino-ext/components/embedding/ark"
	openaiembedding "github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
)

func NewEmbedder(ctx context.Context, cfg config.EmbeddingModelConfig) (embedding.Embedder, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "ark":
		return arkembedding.NewEmbedder(ctx, &arkembedding.EmbeddingConfig{
			APIKey:    cfg.APIKey,
			AccessKey: cfg.AccessKey,
			SecretKey: cfg.SecretKey,
			BaseURL:   cfg.BaseURL,
			Region:    cfg.Region,
			Model:     cfg.Model,
		})
	case "openai":
		return openaiembedding.NewEmbedder(ctx, &openaiembedding.EmbeddingConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		})
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.Provider)
	}
}
