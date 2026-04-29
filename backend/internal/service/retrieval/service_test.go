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

func TestNormalizeQueries(t *testing.T) {
	queries := []string{
		"  product A upgrade  ",
		"Product  A   Upgrade",
		"",
		"module-b",
		" module-b ",
	}

	normalized := normalizeQueries(queries)
	if len(normalized) != 2 {
		t.Fatalf("expected 2 unique queries, got %#v", normalized)
	}

	if normalized[0] != "product A upgrade" {
		t.Fatalf("unexpected first query: %q", normalized[0])
	}
	if normalized[1] != "module-b" {
		t.Fatalf("unexpected second query: %q", normalized[1])
	}
}

func TestMergeKnowledgeResultSets(t *testing.T) {
	set1 := []KnowledgeResult{
		{ID: 1, Name: "Product-A"},
		{ID: 2, Name: "Module-B"},
	}
	set2 := []KnowledgeResult{
		{ID: 2, Name: "Module-B"},
		{ID: 3, Name: "Module-C"},
	}

	merged := mergeKnowledgeResultSets([][]KnowledgeResult{set1, set2}, 3)
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged results, got %d", len(merged))
	}

	// ID=2 appears in both result sets and should rank first after score aggregation.
	if merged[0].ID != 2 {
		t.Fatalf("expected first result ID=2, got %d", merged[0].ID)
	}
}
