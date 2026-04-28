package handler

import (
	"testing"

	"caseagent/internal/db/models"
	dbvector "caseagent/internal/db/vector"
)

func TestApplyKnowledgeUpdate(t *testing.T) {
	t.Run("content change clears embedding and requires reprocess", func(t *testing.T) {
		kb := &models.KnowledgeBase{
			Content:   "old content",
			Embedding: dbvector.New([]float32{1, 2, 3}),
			Status:    models.KnowledgeStatusCompleted,
		}

		needsReprocess := applyKnowledgeUpdate(kb, UpdateKnowledgeRequest{
			Content: "new content",
		})

		if !needsReprocess {
			t.Fatal("expected content change to require reprocess")
		}
		if kb.Content != "new content" {
			t.Fatalf("unexpected content: %q", kb.Content)
		}
		if kb.Embedding != nil {
			t.Fatalf("expected embedding to be cleared, got %#v", kb.Embedding)
		}
		if kb.Status != models.KnowledgeStatusProcessing {
			t.Fatalf("expected status %q, got %q", models.KnowledgeStatusProcessing, kb.Status)
		}
	})

	t.Run("metadata only change keeps current embedding", func(t *testing.T) {
		embedding := dbvector.New([]float32{4, 5, 6})
		kb := &models.KnowledgeBase{
			Content:   "stable content",
			Embedding: embedding,
			Status:    models.KnowledgeStatusCompleted,
		}

		needsReprocess := applyKnowledgeUpdate(kb, UpdateKnowledgeRequest{
			Metadata: map[string]any{"owner": "qa"},
		})

		if needsReprocess {
			t.Fatal("did not expect metadata-only change to require reprocess")
		}
		if kb.Embedding == nil {
			t.Fatal("expected embedding to remain available")
		}
		if kb.Status != models.KnowledgeStatusCompleted {
			t.Fatalf("expected status to remain %q, got %q", models.KnowledgeStatusCompleted, kb.Status)
		}
	})
}
