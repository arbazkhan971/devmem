package search

import (
	"math"
	"time"
)

// typeWeights maps note/commit types to relevance multipliers.
var typeWeights = map[string]float64{
	"decision":  2.0,
	"blocker":   1.5,
	"progress":  1.0,
	"feature":   1.2,
	"note":      0.5,
	"next_step": 1.0,
}

// Score computes a composite relevance score for a search result.
//
// Formula: score = bm25Score * temporalDecay * typeWeight * linkBoost
//
// bm25Score should be the absolute (positive) BM25 value.
// createdAt is an RFC3339/datetime string.
// noteType is the type column value (e.g. "decision", "blocker", "progress", "note").
// linkCount is the number of memory_links referencing this item.
func Score(bm25Score float64, createdAt string, noteType string, linkCount int) float64 {
	decay := temporalDecay(createdAt)
	weight := typeWeight(noteType)
	boost := linkBoost(linkCount)
	return bm25Score * decay * weight * boost
}

// temporalDecay returns an exponential decay factor with a 14-day half-life.
// Items created now return ~1.0; items 14 days old return ~0.5.
func temporalDecay(createdAt string) float64 {
	t, err := time.Parse("2006-01-02 15:04:05", createdAt)
	if err != nil {
		// Try RFC3339 as fallback
		t, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return 1.0
		}
	}
	days := time.Since(t).Hours() / 24.0
	if days < 0 {
		days = 0
	}
	return math.Exp(-0.693 * days / 14.0)
}

// typeWeight returns a relevance multiplier based on the memory type.
func typeWeight(noteType string) float64 {
	if w, ok := typeWeights[noteType]; ok {
		return w
	}
	return 1.0
}

// linkBoost returns a boost factor based on how connected an item is.
func linkBoost(linkCount int) float64 {
	return 1.0 + float64(linkCount)*0.1
}
