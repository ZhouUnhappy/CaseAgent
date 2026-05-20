package pgvector

import (
	"context"
	"fmt"
	"time"

	"caseagent/internal/db/models"
	dbvector "caseagent/internal/db/vector"

	"github.com/uptrace/bun"
)

type Indexer struct {
	db bun.IDB
}

type Config struct {
	DB bun.IDB
}

func New(ctx context.Context, cfg *Config) (*Indexer, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	return &Indexer{db: cfg.DB}, nil
}

// Store stores document chunks with embeddings in the database
func (i *Indexer) Store(ctx context.Context, documentID int, chunks []string, embeddings [][]float32) error {
	for idx, chunk := range chunks {
		var embedding []float32
		if idx < len(embeddings) {
			embedding = embeddings[idx]
		}

		docChunk := &models.DocumentChunk{
			DocumentID: documentID,
			Content:    chunk,
			Embedding:  dbvector.New(embedding),
			CreatedAt:  time.Now(),
		}

		_, err := i.db.NewInsert().Model(docChunk).Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to store document chunk: %w", err)
		}
	}
	return nil
}

func (i *Indexer) GetType() string {
	return "PGVectorIndexer"
}
