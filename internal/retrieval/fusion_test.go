package retrieval

import "testing"

func TestFuseRRFCombinesRanksDeduplicatesAndLimits(t *testing.T) {
	fused, err := FuseRRF([][]RankedItem{
		{{ID: "vector-a", Payload: "from-vector"}, {ID: "b"}, {ID: "vector-a"}},
		{{ID: "b", Payload: "from-keyword"}, {ID: "c"}, {ID: "vector-a"}},
	}, RRFOptions{K: 10, Weights: []float64{1, 2}, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(fused) != 2 || fused[0].ID != "b" || fused[1].ID != "vector-a" {
		t.Fatalf("unexpected fused results: %#v", fused)
	}
	if fused[0].Ranks[0] != 2 || fused[0].Ranks[1] != 1 {
		t.Fatalf("unexpected b ranks: %#v", fused[0].Ranks)
	}
	if fused[1].Payload != "from-vector" || fused[1].Ranks[0] != 1 || fused[1].Ranks[1] != 3 {
		t.Fatalf("unexpected vector-a: %#v", fused[1])
	}
}

func TestFuseRRFValidatesInputAndTieBreaksStably(t *testing.T) {
	_, err := FuseRRF([][]RankedItem{{{ID: ""}}}, RRFOptions{})
	if err == nil {
		t.Fatal("expected empty ID error")
	}
	_, err = FuseRRF([][]RankedItem{{{ID: "a"}}}, RRFOptions{Weights: []float64{-1}})
	if err == nil {
		t.Fatal("expected negative weight error")
	}
	fused, err := FuseRRF([][]RankedItem{{{ID: "b"}, {ID: "a"}}}, RRFOptions{K: 0})
	if err != nil {
		t.Fatal(err)
	}
	if fused[0].ID != "b" || fused[1].ID != "a" {
		t.Fatalf("order = %#v", fused)
	}
}
