package retrieval

import (
	"math"
	"testing"
)

func TestValidateVectorAndCosineSimilarity(t *testing.T) {
	if err := ValidateVector([]float32{1}, 2); err == nil {
		t.Fatal("expected dimensions error")
	}
	if err := ValidateVector([]float32{float32(math.NaN())}, 1); err == nil {
		t.Fatal("expected non-finite error")
	}
	score, err := CosineSimilarity([]float32{1, 0}, []float32{0, 1})
	if err != nil || score != 0 {
		t.Fatalf("score=%v, err=%v", score, err)
	}
	if _, err := CosineSimilarity([]float32{0, 0}, []float32{1, 0}); err == nil {
		t.Fatal("expected zero-vector error")
	}
}
