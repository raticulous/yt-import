package referee

import (
	"context"

	"yt-import/internal/domain"
)

// MockReferee implements Referee for unit tests and offline dry runs.
type MockReferee struct {
	ForceMatch     bool
	Confidence     float64
	BatchCallCount int
}

func NewMockReferee() *MockReferee {
	return &MockReferee{
		ForceMatch: true,
		Confidence: 0.98,
	}
}

func (m *MockReferee) Decide(ctx context.Context, source domain.Track, candidates []domain.Candidate) (*domain.RefereeVerdict, error) {
	if len(candidates) == 0 {
		return &domain.RefereeVerdict{
			Verdict:      "NO_MATCH",
			MatchedIndex: -1,
			Confidence:   0.0,
			Reasoning:    "No candidates available",
		}, nil
	}

	// Pick highest scoring candidate if above 0.70
	bestIdx := 0
	bestScore := candidates[0].Score
	for i, c := range candidates {
		if c.Score > bestScore {
			bestScore = c.Score
			bestIdx = i
		}
	}

	if m.ForceMatch {
		return &domain.RefereeVerdict{
			Verdict:      "MATCH",
			MatchedIndex: bestIdx,
			Confidence:   m.Confidence,
			Reasoning:    "Mock referee approved highest scoring candidate",
		}, nil
	}

	return &domain.RefereeVerdict{
		Verdict:      "NO_MATCH",
		MatchedIndex: -1,
		Confidence:   bestScore,
		Reasoning:    "Mock referee rejected candidate below threshold",
	}, nil
}

func (m *MockReferee) DecideBatch(ctx context.Context, items []BatchItem) ([]domain.RefereeVerdict, error) {
	m.BatchCallCount++
	verdicts := make([]domain.RefereeVerdict, len(items))
	for i, item := range items {
		v, err := m.Decide(ctx, item.Source, item.Candidates)
		if err != nil {
			return nil, err
		}
		v.ItemID = item.ItemID
		verdicts[i] = *v
	}
	return verdicts, nil
}
