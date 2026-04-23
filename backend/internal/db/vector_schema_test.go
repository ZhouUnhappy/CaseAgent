package db

import "testing"

func TestParseVectorDimensions(t *testing.T) {
	dimensions, err := parseVectorDimensions("vector(4096)")
	if err != nil {
		t.Fatalf("parseVectorDimensions() returned error: %v", err)
	}
	if dimensions != 4096 {
		t.Fatalf("unexpected vector dimensions: got %d want 4096", dimensions)
	}
}

func TestParseVectorDimensionsRejectsUnexpectedType(t *testing.T) {
	if _, err := parseVectorDimensions("text"); err == nil {
		t.Fatal("expected parseVectorDimensions() to reject non-vector types")
	}
}
