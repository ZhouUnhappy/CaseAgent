package retrieval

import (
	"testing"

	"caseagent/internal/db/models"
)

func TestPreferredDocumentContent(t *testing.T) {
	t.Run("prefer stored content when present", func(t *testing.T) {
		got := preferredDocumentContent("  original markdown  ", "chunk fallback")
		if got != "original markdown" {
			t.Fatalf("unexpected preferred content: %q", got)
		}
	})

	t.Run("fallback to chunk content for legacy documents", func(t *testing.T) {
		got := preferredDocumentContent("   ", "chunk fallback")
		if got != "chunk fallback" {
			t.Fatalf("unexpected fallback content: %q", got)
		}
	})
}

func TestFilterSearchableDocumentIDs(t *testing.T) {
	documentIDs := []int{3, 1, 2, 4}
	documents := map[int]models.Document{
		1: {ID: 1, Status: "completed"},
		2: {ID: 2, Status: "processing"},
		3: {ID: 3, Status: "failed"},
	}

	filtered := filterSearchableDocumentIDs(documentIDs, documents)
	if len(filtered) != 1 || filtered[0] != 1 {
		t.Fatalf("unexpected filtered ids: %#v", filtered)
	}
}

func TestIsKnowledgeSearchable(t *testing.T) {
	if isKnowledgeSearchable(&models.KnowledgeBase{Status: models.KnowledgeStatusProcessing}) {
		t.Fatal("processing knowledge should not be searchable")
	}
	if !isKnowledgeSearchable(&models.KnowledgeBase{Status: models.KnowledgeStatusCompleted}) {
		t.Fatal("completed knowledge should be searchable")
	}
	if isKnowledgeSearchable(nil) {
		t.Fatal("nil knowledge entry should not be searchable")
	}
}
