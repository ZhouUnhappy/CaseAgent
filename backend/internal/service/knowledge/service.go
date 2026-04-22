package knowledge

import (
	"context"
	"fmt"
	"time"

	"caseagent/internal/config"
	"caseagent/internal/db/models"

	arkembedding "github.com/cloudwego/eino-ext/components/embedding/ark"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/uptrace/bun"
)

type Service struct {
	db        *bun.DB
	embedding embedding.Embedder
}

func New(ctx context.Context, db *bun.DB) (*Service, error) {
	cfg := config.Get()
	if cfg.Model.Embedding.Provider != "ark" {
		return nil, fmt.Errorf("only ark embedding provider is supported, got: %s", cfg.Model.Embedding.Provider)
	}

	embedder, err := arkembedding.NewEmbedder(ctx, &arkembedding.EmbeddingConfig{
		APIKey:    cfg.Model.Embedding.APIKey,
		AccessKey: cfg.Model.Embedding.AccessKey,
		SecretKey: cfg.Model.Embedding.SecretKey,
		BaseURL:   cfg.Model.Embedding.BaseURL,
		Region:    cfg.Model.Embedding.Region,
		Model:     cfg.Model.Embedding.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedding model: %w", err)
	}

	return &Service{
		db:        db,
		embedding: embedder,
	}, nil
}

// ProcessKnowledge generates a single embedding for the current knowledge record.
func (s *Service) ProcessKnowledge(ctx context.Context, kbID int, content string) error {
	embResult, err := s.embedding.EmbedStrings(ctx, []string{content})
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	if len(embResult) == 0 || len(embResult[0]) == 0 {
		return fmt.Errorf("empty embedding result")
	}

	embedding32 := make([]float32, len(embResult[0]))
	for i, v := range embResult[0] {
		embedding32[i] = float32(v)
	}

	_, err = s.db.NewUpdate().Model(&models.KnowledgeBase{}).
		Set("content = ?", content).
		Set("embedding = ?", embedding32).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", kbID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update knowledge base: %w", err)
	}

	return nil
}
