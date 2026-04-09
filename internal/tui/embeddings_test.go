package tui

import (
	"math"
	"testing"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float64{1, 2, 3}
	sim := embeddingCosineSimilarity(a, a)
	if math.Abs(sim-1.0) > 0.001 {
		t.Errorf("identical vectors should have similarity 1.0, got %f", sim)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{0, 1, 0}
	sim := embeddingCosineSimilarity(a, b)
	if math.Abs(sim) > 0.001 {
		t.Errorf("orthogonal vectors should have similarity 0, got %f", sim)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{-1, -2, -3}
	sim := embeddingCosineSimilarity(a, b)
	if math.Abs(sim-(-1.0)) > 0.001 {
		t.Errorf("opposite vectors should have similarity -1, got %f", sim)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	a := []float64{1, 2}
	b := []float64{1, 2, 3}
	sim := embeddingCosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("different lengths should return 0, got %f", sim)
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	sim := embeddingCosineSimilarity(nil, nil)
	if sim != 0 {
		t.Errorf("empty vectors should return 0, got %f", sim)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float64{0, 0, 0}
	b := []float64{1, 2, 3}
	sim := embeddingCosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("zero vector should return 0, got %f", sim)
	}
}

func TestCosineSimilarity_Similar(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{1, 2, 4} // slightly different
	sim := embeddingCosineSimilarity(a, b)
	if sim < 0.98 || sim > 1.0 {
		t.Errorf("similar vectors should have high similarity, got %f", sim)
	}
}
