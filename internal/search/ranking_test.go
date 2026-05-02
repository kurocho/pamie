// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"testing"
	"time"
)

func TestScoreCandidateBoostsRelevantSignals(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	base := Candidate{
		FTSRank:    -1,
		Tier:       "cold",
		UpdatedAt:  now.Add(-60 * 24 * time.Hour),
		Importance: 10,
		Now:        now,
	}
	boosted := base
	boosted.Tier = "working"
	boosted.Pinned = true
	boosted.Importance = 90
	boosted.AccessCount = 5
	boosted.UpdatedAt = now.Add(-time.Hour)

	if ScoreCandidate(boosted).Total <= ScoreCandidate(base).Total {
		t.Fatalf("boosted score = %+v, base score = %+v", ScoreCandidate(boosted), ScoreCandidate(base))
	}
}

func TestScoreCandidateIncludesVectorAsOneSignal(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	similarity := 0.75
	score := ScoreCandidate(Candidate{
		VectorSimilarity: &similarity,
		Tier:             "working",
		UpdatedAt:        now,
		Now:              now,
	})
	if score.Vector == 0 {
		t.Fatalf("Vector score = 0, want positive component")
	}
	if score.Keyword != 0 {
		t.Fatalf("Keyword score = %v, want no keyword score without a keyword match", score.Keyword)
	}
	if score.Total <= score.Vector {
		t.Fatalf("Total = %v, vector = %v, want vector blended with other signals", score.Total, score.Vector)
	}
}

func TestDepthCandidateLimit(t *testing.T) {
	if DepthShallow.CandidateLimit(10) != 10 {
		t.Fatalf("shallow candidate limit mismatch")
	}
	if DepthStandard.CandidateLimit(10) != 30 {
		t.Fatalf("standard candidate limit mismatch")
	}
	if DepthDeep.CandidateLimit(10) != 80 {
		t.Fatalf("deep candidate limit mismatch")
	}
	if DepthDeep.CandidateLimit(100) != 500 {
		t.Fatalf("deep candidate limit should cap at 500")
	}
}
