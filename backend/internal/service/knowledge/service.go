package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"caseagent/internal/config"
	"caseagent/internal/db/models"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/uptrace/bun"
)

type Service struct {
	db        *bun.DB
	embedding embedding.Embedder
}

func New(ctx context.Context, db *bun.DB) (*Service, error) {
	cfg := config.Get()

	// Initialize embedding model based on provider
	var embedder embedding.Embedder
	var err error

	switch cfg.Model.Embedding.Provider {
	case "openai":
		embedder, err = openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
			APIKey:  cfg.Model.Embedding.APIKey,
			BaseURL: cfg.Model.Embedding.BaseURL,
			Model:   cfg.Model.Embedding.Model,
		})
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.Model.Embedding.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedding model: %w", err)
	}

	return &Service{
		db:        db,
		embedding: embedder,
	}, nil
}

// ProcessKnowledge processes a knowledge base document: splits and stores with embedding
func (s *Service) ProcessKnowledge(ctx context.Context, kbID int, content string) error {
	// Step 1: Split content by headers
	chunks := s.splitByHeaders(content)

	// Step 2: Generate embeddings for each chunk and store
	embeddings := make([][]float32, 0, len(chunks))
	for _, chunk := range chunks {
		// Generate embedding
		embResult, err := s.embedding.EmbedStrings(ctx, []string{chunk})
		if err != nil {
			return fmt.Errorf("failed to generate embedding: %w", err)
		}

		if len(embResult) == 0 || len(embResult[0]) == 0 {
			return fmt.Errorf("empty embedding result")
		}

		// Convert []float64 to []float32
		embedding32 := make([]float32, len(embResult[0]))
		for i, v := range embResult[0] {
			embedding32[i] = float32(v)
		}

		embeddings = append(embeddings, embedding32)
	}

	// Update knowledge base with content and embeddings
	_, err := s.db.NewUpdate().Model(&models.KnowledgeBase{}).
		Set("content = ?", content).
		Set("embeddings = ?", embeddings).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", kbID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update knowledge base: %w", err)
	}

	return nil
}

// splitByHeaders splits markdown content by ## and ### headers
func (s *Service) splitByHeaders(content string) []string {
	lines := strings.Split(content, "\n")
	var chunks []string
	var currentChunk []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if line is a header (## or ###)
		if strings.HasPrefix(trimmed, "##") && !strings.HasPrefix(trimmed, "####") {
			// Save previous chunk if not empty
			if len(currentChunk) > 0 {
				chunks = append(chunks, strings.Join(currentChunk, "\n"))
			}
			// Start new chunk with the header
			currentChunk = []string{line}
		} else {
			currentChunk = append(currentChunk, line)
		}
	}

	// Add the last chunk
	if len(currentChunk) > 0 {
		chunks = append(chunks, strings.Join(currentChunk, "\n"))
	}

	return chunks
}
