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
	var (
		embedder embedding.Embedder
		err      error
	)

	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "ark":
		embedder, err = arkembedding.NewEmbedder(ctx, &arkembedding.EmbeddingConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Region:  cfg.Region,
			Model:   cfg.Model,
		})
	case "openai":
		openAIConfig := &openaiembedding.EmbeddingConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		}
		if cfg.Dimensions > 0 {
			dimensions := cfg.Dimensions
			openAIConfig.Dimensions = &dimensions
		}
		embedder, err = openaiembedding.NewEmbedder(ctx, openAIConfig)
	case "fake":
		embedder = newFakeEmbedder(cfg.Dimensions)
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.Provider)
	}

	if err != nil {
		return nil, err
	}

	if cfg.Dimensions > 0 {
		return &dimensionCheckedEmbedder{
			inner:    embedder,
			expected: cfg.Dimensions,
		}, nil
	}

	return embedder, nil
}

type dimensionCheckedEmbedder struct {
	inner    embedding.Embedder
	expected int
}

func (e *dimensionCheckedEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	embeddings, err := e.inner.EmbedStrings(ctx, texts, opts...)
	if err != nil {
		return nil, err
	}

	for idx, item := range embeddings {
		if len(item) != e.expected {
			return nil, fmt.Errorf("embedding dimension mismatch at item %d: expected %d, got %d", idx, e.expected, len(item))
		}
	}

	return embeddings, nil
}
