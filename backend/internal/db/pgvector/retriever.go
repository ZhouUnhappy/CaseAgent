package pgvector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"caseagent/internal/ai"
	"caseagent/internal/config"
	"caseagent/internal/db/models"
	dbvector "caseagent/internal/db/vector"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/uptrace/bun"
)

type Retriever struct {
	db         bun.IDB
	embedding  embedding.Embedder
	dimensions int
}

type RetrieverConfig struct {
	Provider   string
	DB         bun.IDB
	Dimensions int
	APIKey     string
	AccessKey  string
	SecretKey  string
	BaseURL    string
	Region     string
	Model      string
}

// ChunkHit pairs a retrieved document chunk with its similarity score.
type ChunkHit struct {
	Chunk *models.DocumentChunk
	Score float64
}

// KnowledgeHit pairs a retrieved knowledge entry with its similarity score.
type KnowledgeHit struct {
	Knowledge *models.KnowledgeBase
	Score     float64
}

type chunkScoreRow struct {
	bun.BaseModel `bun:"table:document_chunks,alias:dc"`

	ID          int             `bun:"id"`
	DocumentID  int             `bun:"document_id"`
	Content     string          `bun:"content"`
	Embedding   dbvector.Vector `bun:"embedding"`
	ParentDocID int             `bun:"parent_doc_id"`
	Metadata    map[string]any  `bun:"metadata,type:jsonb"`
	CreatedAt   time.Time       `bun:"created_at"`
	Score       float64         `bun:"score"`
}

type knowledgeScoreRow struct {
	bun.BaseModel `bun:"table:knowledge_base,alias:kb"`

	ID        int             `bun:"id"`
	Type      string          `bun:"type"`
	Name      string          `bun:"name"`
	Content   string          `bun:"content"`
	Embedding dbvector.Vector `bun:"embedding"`
	Metadata  map[string]any  `bun:"metadata,type:jsonb"`
	Status    string          `bun:"status"`
	CreatedAt time.Time       `bun:"created_at"`
	UpdatedAt time.Time       `bun:"updated_at"`
	Score     float64         `bun:"score"`
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

// Retrieve returns scored document chunks ranked by cosine similarity to the query embedding.
func (r *Retriever) Retrieve(ctx context.Context, queryEmbedding []float32, topK int) ([]ChunkHit, error) {
	args := []any{models.DocumentStatusCompleted}
	query := strings.TrimSpace(`
		SELECT dc.id, dc.document_id, dc.content, dc.embedding, dc.parent_doc_id, dc.metadata, dc.created_at,
		       (1 - (dc.embedding <=> ?)) AS score
		FROM document_chunks AS dc
		JOIN documents AS d ON d.id = dc.document_id
		WHERE dc.embedding IS NOT NULL
		  AND d.status = ?
	`)
	queryVector := dbvector.New(queryEmbedding)
	args = append([]any{queryVector}, args...)

	if r.dimensions > 0 {
		query += "\n  AND vector_dims(dc.embedding) = ?"
		args = append(args, r.dimensions)
	}
	query += "\nORDER BY dc.embedding <=> ?\nLIMIT ?"
	args = append(args, queryVector, topK)

	var rows []chunkScoreRow
	if err := r.db.NewRaw(query, args...).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("failed to retrieve chunks: %w", err)
	}

	hits := make([]ChunkHit, 0, len(rows))
	for i := range rows {
		row := rows[i]
		hits = append(hits, ChunkHit{
			Chunk: &models.DocumentChunk{
				ID:          row.ID,
				DocumentID:  row.DocumentID,
				Content:     row.Content,
				Embedding:   row.Embedding,
				ParentDocID: row.ParentDocID,
				Metadata:    row.Metadata,
				CreatedAt:   row.CreatedAt,
			},
			Score: row.Score,
		})
	}
	return hits, nil
}

// RetrieveWithQuery generates an embedding for the query and returns scored chunks.
func (r *Retriever) RetrieveWithQuery(ctx context.Context, query string, topK int) ([]ChunkHit, error) {
	embResult, err := r.embedding.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}
	if len(embResult) == 0 || len(embResult[0]) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}

	queryEmbedding := make([]float32, len(embResult[0]))
	for i, v := range embResult[0] {
		queryEmbedding[i] = float32(v)
	}

	return r.Retrieve(ctx, queryEmbedding, topK)
}

// RetrieveFromKnowledge returns scored knowledge entries ranked by cosine similarity.
func (r *Retriever) RetrieveFromKnowledge(ctx context.Context, queryEmbedding []float32, topK int) ([]KnowledgeHit, error) {
	queryVector := dbvector.New(queryEmbedding)
	args := []any{queryVector, models.KnowledgeStatusCompleted}
	query := strings.TrimSpace(`
		SELECT kb.id, kb.type, kb.name, kb.content, kb.embedding, kb.metadata, kb.status, kb.created_at, kb.updated_at,
		       (1 - (kb.embedding <=> ?)) AS score
		FROM knowledge_base AS kb
		WHERE kb.embedding IS NOT NULL
		  AND kb.status = ?
	`)
	if r.dimensions > 0 {
		query += "\n  AND vector_dims(kb.embedding) = ?"
		args = append(args, r.dimensions)
	}
	query += "\nORDER BY kb.embedding <=> ?\nLIMIT ?"
	args = append(args, queryVector, topK)

	var rows []knowledgeScoreRow
	if err := r.db.NewRaw(query, args...).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("failed to retrieve knowledge: %w", err)
	}

	hits := make([]KnowledgeHit, 0, len(rows))
	for i := range rows {
		row := rows[i]
		hits = append(hits, KnowledgeHit{
			Knowledge: &models.KnowledgeBase{
				ID:        row.ID,
				Type:      row.Type,
				Name:      row.Name,
				Content:   row.Content,
				Embedding: row.Embedding,
				Metadata:  row.Metadata,
				Status:    row.Status,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
			},
			Score: row.Score,
		})
	}
	return hits, nil
}

// RetrieveKnowledgeWithQuery generates an embedding and returns scored knowledge hits.
func (r *Retriever) RetrieveKnowledgeWithQuery(ctx context.Context, query string, topK int) ([]KnowledgeHit, error) {
	embResult, err := r.embedding.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}
	if len(embResult) == 0 || len(embResult[0]) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}

	queryEmbedding := make([]float32, len(embResult[0]))
	for i, v := range embResult[0] {
		queryEmbedding[i] = float32(v)
	}

	return r.RetrieveFromKnowledge(ctx, queryEmbedding, topK)
}

func (r *Retriever) GetType() string {
	return "PGVectorRetriever"
}
