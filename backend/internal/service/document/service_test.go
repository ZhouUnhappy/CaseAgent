package document

import (
	"strings"
	"testing"
)

func TestSplitByHeaders(t *testing.T) {
	service := &Service{}
	content := "# Title\n\nintro\n## First\nline 1\n### Child\nline 2\n#### Not Split\nline 3\n## Second\nline 4"

	chunks := service.splitByHeaders(content)
	if len(chunks) != 4 {
		t.Fatalf("expected 4 chunks, got %d: %#v", len(chunks), chunks)
	}

	if chunks[0] != "# Title\n\nintro" {
		t.Fatalf("unexpected first chunk: %q", chunks[0])
	}
	if chunks[1] != "## First\nline 1" {
		t.Fatalf("unexpected second chunk: %q", chunks[1])
	}
	if chunks[2] != "### Child\nline 2\n#### Not Split\nline 3" {
		t.Fatalf("expected third chunk to keep #### content under ###, got %q", chunks[2])
	}
	if chunks[len(chunks)-1] != "## Second\nline 4" {
		t.Fatalf("unexpected final chunk: %q", chunks[len(chunks)-1])
	}
}

func TestSplitLargeChunk(t *testing.T) {
	longParagraph := strings.Repeat("a", 1400)
	chunk := "## Header\n\n" + longParagraph + "\n\n" + strings.Repeat("b", 500)

	parts := splitLargeChunk(chunk, 600)
	if len(parts) < 3 {
		t.Fatalf("expected long chunk to be split into multiple parts, got %d", len(parts))
	}

	for idx, part := range parts {
		if len([]rune(part)) > 600 {
			t.Fatalf("part %d exceeds limit: %d", idx, len([]rune(part)))
		}
	}
}
