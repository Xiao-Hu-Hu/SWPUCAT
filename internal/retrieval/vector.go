package retrieval

import (
	"fmt"
	"math"
)

// ValidateVector ensures a vector can safely be stored and compared.
func ValidateVector(vector []float32, dimensions int) error {
	if dimensions <= 0 {
		return ErrEmbeddingDimensions
	}
	if len(vector) != dimensions {
		return fmt.Errorf("vector dimensions %d do not match expected %d", len(vector), dimensions)
	}
	for index, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("vector has a non-finite value at dimension %d", index)
		}
	}
	return nil
}

// CosineSimilarity calculates cosine similarity after validating dimensions.
// A zero vector has no defined cosine similarity and returns an error.
func CosineSimilarity(left, right []float32) (float64, error) {
	if len(left) == 0 || len(left) != len(right) {
		return 0, fmt.Errorf("vectors must have the same non-zero dimensions")
	}
	if err := ValidateVector(left, len(left)); err != nil {
		return 0, err
	}
	if err := ValidateVector(right, len(right)); err != nil {
		return 0, err
	}
	var dot, leftNorm, rightNorm float64
	for index := range left {
		l, r := float64(left[index]), float64(right[index])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, fmt.Errorf("cosine similarity is undefined for a zero vector")
	}
	return dot / math.Sqrt(leftNorm*rightNorm), nil
}
