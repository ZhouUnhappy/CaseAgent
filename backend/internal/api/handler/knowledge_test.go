package handler

import (
	"strings"
	"testing"
	"time"

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

func TestKnowledgeSourceForWrite(t *testing.T) {
	source, err := knowledgeSourceForWrite(" Public Fixture ")
	if err != nil {
		t.Fatalf("knowledgeSourceForWrite returned error: %v", err)
	}
	if source != "public-fixture" {
		t.Fatalf("unexpected source: %q", source)
	}

	source, err = knowledgeSourceForWrite("")
	if err != nil {
		t.Fatalf("empty source should default, got error: %v", err)
	}
	if source != defaultKnowledgeSource {
		t.Fatalf("expected default source %q, got %q", defaultKnowledgeSource, source)
	}

	if _, err := knowledgeSourceForWrite(strings.Repeat("a", 65)); err == nil {
		t.Fatal("expected overlong source to fail")
	}
}

func TestParseTriStateBoolQuery(t *testing.T) {
	if got, err := parseTriStateBoolQuery("expired", ""); err != nil || got != nil {
		t.Fatalf("empty filter should be nil without error, got value=%#v err=%v", got, err)
	}

	got, err := parseTriStateBoolQuery("expired", "true")
	if err != nil {
		t.Fatalf("true filter returned error: %v", err)
	}
	if got == nil || !*got {
		t.Fatalf("expected true pointer, got %#v", got)
	}

	got, err = parseTriStateBoolQuery("duplicate", "false")
	if err != nil {
		t.Fatalf("false filter returned error: %v", err)
	}
	if got == nil || *got {
		t.Fatalf("expected false pointer, got %#v", got)
	}

	if _, err := parseTriStateBoolQuery("expired", "sometimes"); err == nil {
		t.Fatal("expected invalid bool filter to fail")
	}
}

func TestApplyKnowledgeGovernanceUpdate(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	expiresAt := now.Add(24 * time.Hour)
	source := "Imported Fixture"
	duplicateID := 4
	markedAt := now.Add(-time.Hour)

	kb := &models.KnowledgeBase{
		Source:            "manual",
		ExpiresAt:         nil,
		DuplicateOfID:     &duplicateID,
		DuplicateMarkedAt: &markedAt,
	}

	if err := applyKnowledgeGovernanceUpdate(nil, kb, UpdateKnowledgeRequest{
		Source:         &source,
		ExpiresAt:      &expiresAt,
		ClearDuplicate: true,
	}, now); err != nil {
		t.Fatalf("applyKnowledgeGovernanceUpdate returned error: %v", err)
	}

	if kb.Source != "imported-fixture" {
		t.Fatalf("unexpected source: %q", kb.Source)
	}
	if kb.ExpiresAt == nil || !kb.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected expires_at: %#v", kb.ExpiresAt)
	}
	if kb.DuplicateOfID != nil || kb.DuplicateMarkedAt != nil {
		t.Fatalf("expected duplicate mark to be cleared, got id=%#v marked=%#v", kb.DuplicateOfID, kb.DuplicateMarkedAt)
	}
}

func TestDuplicateMarkedAt(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	previous := now.Add(-time.Hour)
	currentID := 12
	nextID := 12

	if got := duplicateMarkedAt(&nextID, &currentID, &previous, now); got == nil || !got.Equal(previous) {
		t.Fatalf("expected existing marked_at to be preserved, got %#v", got)
	}

	otherID := 13
	if got := duplicateMarkedAt(&otherID, &currentID, &previous, now); got == nil || !got.Equal(now) {
		t.Fatalf("expected new marked_at, got %#v", got)
	}

	if got := duplicateMarkedAt(nil, &currentID, &previous, now); got != nil {
		t.Fatalf("expected nil marked_at when duplicate is cleared, got %#v", got)
	}
}
