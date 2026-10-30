package retrieval

import (
	"fmt"
	"sort"
	"strings"
)

// RankedItem is one result from a retrieval channel, already ordered by that
// channel's relevance score. ID must be stable across all channels.
type RankedItem struct {
	ID      string
	Payload any
}

// RRFOptions controls reciprocal rank fusion. Missing weights default to 1.
type RRFOptions struct {
	K       int
	Weights []float64
	Limit   int
}

// FusedItem carries the combined score and the best source item. Ranks uses
// the zero-based retrieval channel index and one-based rank within that list.
type FusedItem struct {
	ID      string
	Score   float64
	Payload any
	Ranks   map[int]int
}

// FuseRRF combines independently ranked lists with reciprocal rank fusion.
// Repeated IDs within one list contribute only their first (best) rank.
func FuseRRF(rankings [][]RankedItem, options RRFOptions) ([]FusedItem, error) {
	k := options.K
	if k == 0 {
		k = 60
	}
	if k < 0 {
		return nil, fmt.Errorf("RRF k must not be negative")
	}
	if len(options.Weights) > len(rankings) {
		return nil, fmt.Errorf("RRF weights exceed the number of rankings")
	}
	type accumulator struct {
		item      FusedItem
		firstSeen int
	}
	combined := make(map[string]*accumulator)
	sequence := 0
	for sourceIndex, ranking := range rankings {
		weight := 1.0
		if sourceIndex < len(options.Weights) {
			weight = options.Weights[sourceIndex]
		}
		if weight < 0 {
			return nil, fmt.Errorf("RRF weight at index %d must not be negative", sourceIndex)
		}
		seen := make(map[string]struct{}, len(ranking))
		for position, candidate := range ranking {
			id := strings.TrimSpace(candidate.ID)
			if id == "" {
				return nil, fmt.Errorf("RRF result at channel %d rank %d has an empty ID", sourceIndex, position+1)
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			entry := combined[id]
			if entry == nil {
				entry = &accumulator{item: FusedItem{ID: id, Payload: candidate.Payload, Ranks: make(map[int]int)}, firstSeen: sequence}
				combined[id] = entry
				sequence++
			}
			entry.item.Score += weight / float64(k+position+1)
			entry.item.Ranks[sourceIndex] = position + 1
		}
	}
	result := make([]accumulator, 0, len(combined))
	for _, entry := range combined {
		result = append(result, *entry)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].item.Score == result[j].item.Score {
			if result[i].firstSeen == result[j].firstSeen {
				return result[i].item.ID < result[j].item.ID
			}
			return result[i].firstSeen < result[j].firstSeen
		}
		return result[i].item.Score > result[j].item.Score
	})
	limit := options.Limit
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	fused := make([]FusedItem, len(result))
	for index := range result {
		fused[index] = result[index].item
	}
	return fused, nil
}
