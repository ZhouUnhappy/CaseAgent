package pgvector

import (
	"context"
	"fmt"
	"strings"

	"caseagent/internal/ai"
	"caseagent/internal/config"
	"caseagent/internal/db/models"
	dbvector "caseagent/internal/db/vector"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/uptrace/bun"
)

type Retriever struct {
	db         *bun.DB
	embedding  embedding.Embedder
	dimensions int
}

type RetrieverConfig struct {
	Provider   string
	DB         *bun.DB
	Dimensions int
	APIKey     string
	AccessKey  string
	SecretKey  string
	BaseURL    string
	Region     string
	Model      string
}

func NewRetriever(ctx context.Context, cfg *RetrieverConfig) (*Retriever, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	embedder, err := ai.NewEmbedder(ctx, config.EmbeddingModelConfig{
		Provider:   cfg.Provider,
		Model:      cfg.Model,
		Dimensions: cfg.Dimensions,
		APIKey:     cfg.APIKey,
		AccessKey:  cfg.AccessKey,
		SecretKey:  cfg.SecretKey,
		BaseURL:    cfg.BaseURL,
		Region:     cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedding model: %w", err)
	}

	return &Retriever{
		db:         cfg.DB,
		embedding:  embedder,
		dimensions: cfg.Dimensions,
	}, nil
}

// Retrieve retrieves similar document chunks based on embedding similarity
func (r *Retriever) Retrieve(ctx context.Context, queryEmbedding []float32, topK int) ([]*models.DocumentChunk, error) {
	var chunks []*models.DocumentChunk

	args := []any{models.DocumentStatusCompleted}
	query := strings.TrimSpace(`
		SELECT dc.*
		FROM document_chunks AS dc
		JOIN documents AS d ON d.id = dc.document_id
		WHERE dc.embedding IS NOT NULL
		  AND d.status = ?
	`)
	if r.dimensions > 0 {
		query += "\n  AND vector_dims(dc.embedding) = ?"
		args = append(args, r.dimensions)
	}
	query += "\nORDER BY dc.embedding <=> ?\nLIMIT ?"
	args = append(args, dbvector.New(queryEmbedding), topK)

	err := r.db.NewRaw(query, args...).Scan(ctx, &chunks)

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

	args := []any{models.KnowledgeStatusCompleted}
	query := strings.TrimSpace(`
		SELECT kb.*
		FROM knowledge_base AS kb
		WHERE kb.embedding IS NOT NULL
		  AND kb.status = ?
	`)
	if r.dimensions > 0 {
		query += "\n  AND vector_dims(kb.embedding) = ?"
		args = append(args, r.dimensions)
	}
	query += "\nORDER BY kb.embedding <=> ?\nLIMIT ?"
	args = append(args, dbvector.New(queryEmbedding), topK)

	err := r.db.NewRaw(query, args...).Scan(ctx, &knowledge)

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
