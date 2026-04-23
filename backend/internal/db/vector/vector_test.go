package vector

import "testing"

func TestVectorValue(t *testing.T) {
	value, err := New([]float32{1.25, 2, 3.5}).Value()
	if err != nil {
		t.Fatalf("Value() returned error: %v", err)
	}

	if value != "[1.25,2,3.5]" {
		t.Fatalf("unexpected encoded vector: %v", value)
	}
}

func TestVectorScan(t *testing.T) {
	var vector Vector
	if err := vector.Scan("[1.25, 2, 3.5]"); err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	expected := []float32{1.25, 2, 3.5}
	if len(vector) != len(expected) {
		t.Fatalf("unexpected vector length: got %d want %d", len(vector), len(expected))
	}
	for i := range expected {
		if vector[i] != expected[i] {
			t.Fatalf("unexpected vector value at %d: got %v want %v", i, vector[i], expected[i])
		}
	}
}

func TestVectorScanRejectsInvalidFormat(t *testing.T) {
	var vector Vector
	if err := vector.Scan("{1,2,3}"); err == nil {
		t.Fatal("expected Scan() to reject invalid pgvector format")
	}
}
