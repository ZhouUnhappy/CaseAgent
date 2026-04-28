package knowledge

import (
	"context"
	"fmt"
	"time"

	"caseagent/internal/ai"
	"caseagent/internal/config"
	"caseagent/internal/db/models"
	dbvector "caseagent/internal/db/vector"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/uptrace/bun"
)

type Service struct {
	db        *bun.DB
	embedding embedding.Embedder
}

func New(ctx context.Context, db *bun.DB) (*Service, error) {
	cfg := config.Get()
	embedder, err := ai.NewEmbedder(ctx, cfg.Model.Embedding)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedding model: %w", err)
	}

	return &Service{
		db:        db,
		embedding: embedder,
	}, nil
}

// ProcessKnowledge generates a single embedding for the current knowledge record.
func (s *Service) ProcessKnowledge(ctx context.Context, kbID int, content string) (err error) {
	defer func() {
		if err != nil {
			if updateErr := s.updateKnowledgeStatus(ctx, kbID, models.KnowledgeStatusFailed); updateErr != nil {
				err = fmt.Errorf("%v; failed to update knowledge status: %w", err, updateErr)
			}
		}
	}()

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
		Set("embedding = ?", dbvector.New(embedding32)).
		Set("status = ?", models.KnowledgeStatusCompleted).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", kbID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update knowledge base: %w", err)
	}

	return nil
}

func (s *Service) ReprocessKnowledge(ctx context.Context, kbID int) error {
	kb := &models.KnowledgeBase{}
	if err := s.db.NewSelect().Model(kb).Where("id = ?", kbID).Scan(ctx); err != nil {
		return fmt.Errorf("failed to load knowledge base: %w", err)
	}
	if kb.Content == "" {
		if err := s.updateKnowledgeStatus(ctx, kbID, models.KnowledgeStatusFailed); err != nil {
			return fmt.Errorf("knowledge base %d has empty content; additionally failed to update status: %w", kbID, err)
		}
		return fmt.Errorf("knowledge base %d has empty content", kbID)
	}
	return s.ProcessKnowledge(ctx, kbID, kb.Content)
}

func (s *Service) updateKnowledgeStatus(ctx context.Context, kbID int, status string) error {
	_, err := s.db.NewUpdate().Model(&models.KnowledgeBase{}).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", kbID).
		Exec(ctx)
	return err
}
