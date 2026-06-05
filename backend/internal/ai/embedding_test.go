package ai

import (
	"context"
	"strings"
	"testing"

	"caseagent/internal/config"

	"github.com/cloudwego/eino/components/embedding"
)

type stubEmbedder struct {
	result [][]float64
}

func (s stubEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	return s.result, nil
}

func TestDimensionCheckedEmbedderAcceptsExpectedDimensions(t *testing.T) {
	embedder := &dimensionCheckedEmbedder{
		inner:    stubEmbedder{result: [][]float64{{1, 2, 3}, {4, 5, 6}}},
		expected: 3,
	}

	result, err := embedder.EmbedStrings(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedStrings() returned error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("unexpected embedding count: got %d want 2", len(result))
	}
}

func TestDimensionCheckedEmbedderRejectsUnexpectedDimensions(t *testing.T) {
	embedder := &dimensionCheckedEmbedder{
		inner:    stubEmbedder{result: [][]float64{{1, 2, 3}}},
		expected: 4,
	}

	_, err := embedder.EmbedStrings(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("expected EmbedStrings() to reject mismatched dimensions")
	}
	if !strings.Contains(err.Error(), "expected 4, got 3") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewEmbedderFakeReturnsConfiguredDimensions(t *testing.T) {
	embedder, err := NewEmbedder(context.Background(), config.EmbeddingModelConfig{
		Provider:   "fake",
		Dimensions: 6,
	})
	if err != nil {
		t.Fatalf("NewEmbedder() returned error: %v", err)
	}

	result, err := embedder.EmbedStrings(context.Background(), []string{"alpha", "alpha"})
	if err != nil {
		t.Fatalf("EmbedStrings() returned error: %v", err)
	}
	if len(result) != 2 || len(result[0]) != 6 {
		t.Fatalf("unexpected fake embedding shape: %#v", result)
	}
	for i := range result[0] {
		if result[0][i] != result[1][i] {
			t.Fatalf("fake embedding not deterministic: %#v", result)
		}
	}
}
