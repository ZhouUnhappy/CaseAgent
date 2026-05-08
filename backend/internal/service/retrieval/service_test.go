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

func TestMergeDocumentResultSets(t *testing.T) {
	set1 := []DocumentResult{
		{DocumentID: 1, Name: "Doc-1", MatchedChunks: []MatchedChunk{
			{Text: "step A", Score: 0.9, Query: "q1", Rank: 1},
			{Text: "step B", Score: 0.8, Query: "q1", Rank: 2},
		}, HitQueries: []string{"q1"}, BestScore: 0.9},
		{DocumentID: 2, Name: "Doc-2", MatchedChunks: []MatchedChunk{
			{Text: "step C", Score: 0.7, Query: "q1", Rank: 3},
		}, HitQueries: []string{"q1"}, BestScore: 0.7},
	}
	set2 := []DocumentResult{
		{DocumentID: 2, Name: "Doc-2", MatchedChunks: []MatchedChunk{
			{Text: "step C", Score: 0.85, Query: "q2", Rank: 1},
			{Text: "step D", Score: 0.75, Query: "q2", Rank: 2},
		}, HitQueries: []string{"q2"}, BestScore: 0.85},
		{DocumentID: 3, Name: "Doc-3", MatchedChunks: []MatchedChunk{
			{Text: "step E", Score: 0.6, Query: "q2", Rank: 3},
		}, HitQueries: []string{"q2"}, BestScore: 0.6},
	}

	merged := mergeDocumentResultSets([][]DocumentResult{set1, set2}, 3)
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged results, got %d", len(merged))
	}
	if merged[0].DocumentID != 2 {
		t.Fatalf("expected first document to be ID=2, got %d", merged[0].DocumentID)
	}
	if merged[0].Rank != 1 {
		t.Fatalf("expected merged top result to have Rank=1, got %d", merged[0].Rank)
	}
	if len(merged[0].MatchedChunks) != 2 {
		t.Fatalf("expected deduped matched chunks, got %#v", merged[0].MatchedChunks)
	}
	// "step C" appears in both sets — dedupe should keep the higher-score copy (0.85 from q2).
	if merged[0].MatchedChunks[0].Text != "step C" || merged[0].MatchedChunks[0].Score != 0.85 {
		t.Fatalf("expected first chunk to be step C@0.85, got %#v", merged[0].MatchedChunks[0])
	}
	if len(merged[0].HitQueries) != 2 {
		t.Fatalf("expected hit_queries to merge to 2 entries, got %#v", merged[0].HitQueries)
	}
}

func TestDedupeMatchedChunks(t *testing.T) {
	chunks := []MatchedChunk{
		{Text: "  first chunk ", Score: 0.5, Query: "q1", Rank: 1},
		{Text: "first   chunk", Score: 0.9, Query: "q2", Rank: 1},
		{Text: "", Score: 0.0},
		{Text: "second chunk", Score: 0.7, Query: "q1", Rank: 2},
	}
	got := dedupeMatchedChunks(chunks)
	if len(got) != 2 {
		t.Fatalf("expected 2 deduped chunks, got %#v", got)
	}
	// dedupe should keep the higher-score copy and sort by score desc.
	if got[0].Score != 0.9 || got[0].Query != "q2" {
		t.Fatalf("expected first chunk to win on score (0.9 from q2), got %#v", got[0])
	}
	if got[1].Text != "second chunk" {
		t.Fatalf("expected second chunk text to be 'second chunk', got %q", got[1].Text)
	}
}
