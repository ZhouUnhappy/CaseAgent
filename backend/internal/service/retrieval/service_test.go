package retrieval

import "testing"

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
