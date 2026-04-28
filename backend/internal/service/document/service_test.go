package document

import "testing"

func TestRemoveBase64Images(t *testing.T) {
	input := "before\n![diagram](data:image/png;base64,AAAA)\nafter"
	got := removeBase64Images(input)

	if got != "before\n\nafter" {
		t.Fatalf("unexpected cleaned content: %q", got)
	}
}

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
