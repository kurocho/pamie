// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"math"
	"time"
)

type Depth string

const (
	DepthShallow  Depth = "shallow"
	DepthStandard Depth = "standard"
	DepthDeep     Depth = "deep"
)

func (d Depth) Valid() bool {
	switch d {
	case "", DepthShallow, DepthStandard, DepthDeep:
		return true
	default:
		return false
	}
}

func (d Depth) CandidateLimit(resultLimit int) int {
	if resultLimit <= 0 {
		resultLimit = 10
	}
	multiplier := 3
	switch d {
	case DepthShallow:
		multiplier = 1
	case DepthDeep:
		multiplier = 8
	}
	limit := resultLimit * multiplier
	if limit < resultLimit {
		return resultLimit
	}
	if limit > 500 {
		return 500
	}
	return limit
}

type Score struct {
	Total      float64 `json:"total"`
	Keyword    float64 `json:"keyword"`
	Vector     float64 `json:"vector"`
	Recency    float64 `json:"recency"`
	Tier       float64 `json:"tier"`
	Pinned     float64 `json:"pinned"`
	Importance float64 `json:"importance"`
	Access     float64 `json:"access"`
}

type Candidate struct {
	FTSRank          float64
	KeywordMatched   bool
	VectorSimilarity *float64
	Tier             string
	Pinned           bool
	Importance       int
	UpdatedAt        time.Time
	LastAccessedAt   *time.Time
	AccessCount      int
	Now              time.Time
}

func ScoreCandidate(candidate Candidate) Score {
	now := candidate.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lastActivity := candidate.UpdatedAt
	if candidate.LastAccessedAt != nil && candidate.LastAccessedAt.After(lastActivity) {
		lastActivity = *candidate.LastAccessedAt
	}

	score := Score{
		Recency:    recencyScore(now.Sub(lastActivity)),
		Tier:       tierScore(candidate.Tier),
		Importance: importanceScore(candidate.Importance),
		Access:     accessScore(candidate.AccessCount),
	}
	if candidate.KeywordMatched || candidate.FTSRank != 0 {
		score.Keyword = keywordScore(candidate.FTSRank)
	}
	if candidate.VectorSimilarity != nil {
		score.Vector = vectorScore(*candidate.VectorSimilarity)
	}
	if candidate.Pinned {
		score.Pinned = 3
	}
	score.Total = score.Keyword + score.Vector + score.Recency + score.Tier + score.Pinned + score.Importance + score.Access
	return score
}

func keywordScore(rank float64) float64 {
	return 10 / (1 + math.Abs(rank))
}

func vectorScore(similarity float64) float64 {
	if similarity <= 0 || math.IsNaN(similarity) {
		return 0
	}
	if similarity > 1 {
		similarity = 1
	}
	return similarity * 6
}

func recencyScore(age time.Duration) float64 {
	switch {
	case age < 0:
		return 4
	case age <= 24*time.Hour:
		return 4
	case age <= 7*24*time.Hour:
		return 3
	case age <= 30*24*time.Hour:
		return 2
	case age <= 90*24*time.Hour:
		return 1
	default:
		return 0
	}
}

func tierScore(tier string) float64 {
	switch tier {
	case "working":
		return 4
	case "hot":
		return 3
	case "warm":
		return 2
	case "cold":
		return 1
	default:
		return 0
	}
}

func importanceScore(importance int) float64 {
	if importance < 0 {
		return 0
	}
	if importance > 100 {
		importance = 100
	}
	return float64(importance) / 25
}

func accessScore(accessCount int) float64 {
	if accessCount < 0 {
		return 0
	}
	if accessCount > 10 {
		accessCount = 10
	}
	return float64(accessCount) * 0.5
}

type VectorResult struct {
	MemoryID string
	Score    float64
}

// VectorSearcher is an extension point for alternate local vector backends.
type VectorSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]VectorResult, error)
}
