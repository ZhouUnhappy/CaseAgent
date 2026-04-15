package pgvector

import (
	"context"
	"fmt"

	"caseagent/internal/db/models"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/uptrace/bun"
)

type Retriever struct {
	db        *bun.DB
	embedding embedding.Embedder
}

type RetrieverConfig struct {
	DB      *bun.DB
	APIKey  string
	BaseURL string
	Model   string
}

func NewRetriever(ctx context.Context, cfg *RetrieverConfig) (*Retriever, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	// Initialize embedding model
	embedder, err := openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedding model: %w", err)
	}

	return &Retriever{
		db:        cfg.DB,
		embedding: embedder,
	}, nil
}

// Retrieve retrieves similar document chunks based on embedding similarity
func (r *Retriever) Retrieve(ctx context.Context, queryEmbedding []float32, topK int) ([]*models.DocumentChunk, error) {
	var chunks []*models.DocumentChunk

	// Use pgvector cosine similarity search
	err := r.db.NewSelect().
		Model(&chunks).
		Where("embedding IS NOT NULL").
		OrderExpr("embedding <=> ?", queryEmbedding).
		Limit(topK).
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve chunks: %w", err)
	}

	return chunks, nil
}

// RetrieveWithQuery generates embedding for query and retrieves similar chunks
func (r *Retriever) RetrieveWithQuery(ctx context.Context, query string, topK int) ([]*models.DocumentChunk, error) {
	// Generate embedding for query
	embResult, err := r.embedding.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	if len(embResult) == 0 || len(embResult[0]) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}

	// Convert []float64 to []float32
	queryEmbedding := make([]float32, len(embResult[0]))
	for i, v := range embResult[0] {
		queryEmbedding[i] = float32(v)
	}

	return r.Retrieve(ctx, queryEmbedding, topK)
}

// RetrieveFromKnowledge retrieves similar knowledge base entries
func (r *Retriever) RetrieveFromKnowledge(ctx context.Context, queryEmbedding []float32, topK int) ([]*models.KnowledgeBase, error) {
	var knowledge []*models.KnowledgeBase

	err := r.db.NewSelect().
		Model(&knowledge).
		Where("embeddings IS NOT NULL").
		OrderExpr("embeddings <=> ?", queryEmbedding).
		Limit(topK).
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve knowledge: %w", err)
	}

	return knowledge, nil
}

// RetrieveKnowledgeWithQuery generates embedding for query and retrieves similar knowledge
func (r *Retriever) RetrieveKnowledgeWithQuery(ctx context.Context, query string, topK int) ([]*models.KnowledgeBase, error) {
	// Generate embedding for query
	embResult, err := r.embedding.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	if len(embResult) == 0 || len(embResult[0]) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}

	// Convert []float64 to []float32
	queryEmbedding := make([]float32, len(embResult[0]))
	for i, v := range embResult[0] {
		queryEmbedding[i] = float32(v)
	}

	return r.RetrieveFromKnowledge(ctx, queryEmbedding, topK)
}

func (r *Retriever) GetType() string {
	return "PGVectorRetriever"
}
