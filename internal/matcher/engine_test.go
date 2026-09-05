package matcher

import (
	"context"
	"testing"

	"yt-import/internal/domain"
	"yt-import/internal/referee"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Bohemian Rhapsody - Remastered 2011", "Bohemian Rhapsody"},
		{"Hotel California (2013 Remaster)", "Hotel California"},
		{"Starboy (feat. Daft Punk)", "Starboy"},
		{"Lose Yourself - From \"8 Mile\" Soundtrack", "Lose Yourself"},
		{"In The End [Official Music Video]", "In The End"},
		{"bad guy - With Justin Bieber", "bad guy"},
		{"Beyoncé - Halo", "Beyonce - Halo"},
		{"Mötley Crüe - Kickstart My Heart", "Motley Crue - Kickstart My Heart"},
		{"One More Time (Radio Edit)", "One More Time (Radio Edit)"}, // modifier kept for modifier detector
	}

	for _, tt := range tests {
		got := CleanTitle(tt.input)
		if got != tt.expected {
			t.Errorf("CleanTitle(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestModifierDisqualification(t *testing.T) {
	// Source track is studio
	source := domain.Track{
		Title:      "Numb",
		Artists:    []string{"Linkin Park"},
		Album:      "Meteora",
		DurationMs: 187000,
	}

	// Candidate 1: Live in Texas (Must be disqualified)
	liveCand := &domain.Candidate{
		Title:      "Linkin Park - Numb [Live in Texas]",
		Artists:    []string{"Linkin Park"},
		DurationMs: 190000,
		VideoType:  domain.TypeOfficialMusicVideo,
	}
	score, disq, reason := ScoreCandidate(source, liveCand)
	if !disq || score != 0.0 {
		t.Fatalf("Expected live candidate to be disqualified, got score: %f, disq: %v, reason: %s", score, disq, reason)
	}

	// Candidate 2: Acoustic Cover (Must be disqualified)
	acousticCand := &domain.Candidate{
		Title:      "Numb (Acoustic Cover)",
		Artists:    []string{"Random Artist"},
		DurationMs: 187000,
		VideoType:  domain.TypeUserGenerated,
	}
	score, disq, _ = ScoreCandidate(source, acousticCand)
	if !disq || score != 0.0 {
		t.Fatalf("Expected acoustic cover candidate to be disqualified, got score: %f", score)
	}

	// Candidate 3: Official studio ATV (Must score high >= 0.95)
	studioCand := &domain.Candidate{
		Title:        "Numb",
		Artists:      []string{"Linkin Park"},
		Album:        "Meteora",
		DurationMs:   187500, // 500ms delta
		VideoType:    domain.TypeAudioTrackVideo,
		ChannelTitle: "Linkin Park - Topic",
	}
	score, disq, _ = ScoreCandidate(source, studioCand)
	if disq {
		t.Fatalf("Studio candidate should not be disqualified")
	}
	if score < 0.95 {
		t.Fatalf("Expected score >= 0.95 for exact official studio track, got: %f", score)
	}
}

func TestDurationPenalty(t *testing.T) {
	source := domain.Track{
		Title:      "Thriller",
		Artists:    []string{"Michael Jackson"},
		DurationMs: 357000, // 5:57
	}

	// Music video candidate with long 14-minute zombie dialogue
	videoCand := &domain.Candidate{
		Title:        "Michael Jackson - Thriller (Official Video)",
		Artists:      []string{"Michael Jackson"},
		DurationMs:   822000, // 13:42
		VideoType:    domain.TypeOfficialMusicVideo,
		ChannelTitle: "Michael Jackson",
	}

	score, _, _ := ScoreCandidate(source, videoCand)
	if videoCand.DurationScore != 0.0 {
		t.Fatalf("Expected duration score 0.0 for massive time delta, got: %f", videoCand.DurationScore)
	}
	if score >= 0.95 {
		t.Fatalf("Candidate with 8 minute duration delta should NEVER reach 95%%! got: %f", score)
	}
}

// MockSearcher for testing matching engine
type MockSearcher struct {
	candidates []domain.Candidate
}

func (m *MockSearcher) Search(ctx context.Context, query string) ([]domain.Candidate, error) {
	return m.candidates, nil
}

func TestEngineAutoMatch(t *testing.T) {
	source := domain.Track{
		Title:      "Get Lucky",
		Artists:    []string{"Daft Punk", "Pharrell Williams"},
		Album:      "Random Access Memories",
		DurationMs: 369000,
	}

	searcher := &MockSearcher{
		candidates: []domain.Candidate{
			{
				VideoID:      "5NV6Rdv1a3I",
				Title:        "Get Lucky",
				Artists:      []string{"Daft Punk", "Pharrell Williams"},
				Album:        "Random Access Memories",
				DurationMs:   369500,
				VideoType:    domain.TypeAudioTrackVideo,
				ChannelTitle: "Daft Punk - Topic",
			},
			{
				VideoID:      "other123",
				Title:        "Get Lucky (Live)",
				Artists:      []string{"Daft Punk"},
				DurationMs:   369000,
				VideoType:    domain.TypeOfficialMusicVideo,
				ChannelTitle: "Live Fest",
			},
		},
	}

	ref := referee.NewMockReferee()
	engine := NewEngine(searcher, ref, 0.95)

	res, err := engine.Match(context.Background(), source)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if res.Decision != domain.DecisionAutoMatch {
		t.Fatalf("Expected DecisionAutoMatch, got: %s (reason: %s)", res.Decision, res.Reason)
	}
	if res.Candidate.VideoID != "5NV6Rdv1a3I" {
		t.Fatalf("Expected candidate 5NV6Rdv1a3I, got: %s", res.Candidate.VideoID)
	}
}

func TestEngineRefereeDisambiguation(t *testing.T) {
	source := domain.Track{
		Title:      "Clint Eastwood",
		Artists:    []string{"Gorillaz"},
		DurationMs: 340000,
	}

	// Candidates with close scores near 85-90%
	searcher := &MockSearcher{
		candidates: []domain.Candidate{
			{
				VideoID:      "cand1",
				Title:        "Gorillaz - Clint Eastwood",
				Artists:      []string{"Gorillaz"},
				DurationMs:   344000, // 4s delta
				VideoType:    domain.TypeOfficialMusicVideo,
				ChannelTitle: "Gorillaz",
			},
			{
				VideoID:      "cand2",
				Title:        "Gorillaz - Clint Eastwood (Audio)",
				Artists:      []string{"Gorillaz"},
				DurationMs:   343000, // 3s delta
				VideoType:    domain.TypeAudioTrackVideo,
				ChannelTitle: "Gorillaz - Topic",
			},
		},
	}

	ref := referee.NewMockReferee()
	ref.ForceMatch = true
	ref.Confidence = 0.97
	engine := NewEngine(searcher, ref, 0.95)

	res, err := engine.Match(context.Background(), source)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if res.Decision != domain.DecisionAIRefereeMatch && res.Decision != domain.DecisionAutoMatch {
		t.Fatalf("Expected AI referee match or auto match, got: %s", res.Decision)
	}
	if res.Confidence < 0.95 {
		t.Fatalf("Confidence should be >= 0.95, got: %f", res.Confidence)
	}
}

func TestEngineResolveBatch(t *testing.T) {
	searcher := &MockSearcher{
		candidates: []domain.Candidate{
			{
				VideoID:      "cand1",
				Title:        "Gorillaz - Clint Eastwood",
				Artists:      []string{"Gorillaz"},
				DurationMs:   344000,
				VideoType:    domain.TypeOfficialMusicVideo,
				ChannelTitle: "Gorillaz",
			},
			{
				VideoID:      "cand2",
				Title:        "Gorillaz - Clint Eastwood (Audio)",
				Artists:      []string{"Gorillaz"},
				DurationMs:   343000,
				VideoType:    domain.TypeAudioTrackVideo,
				ChannelTitle: "Gorillaz - Topic",
			},
		},
	}

	ref := referee.NewMockReferee()
	ref.ForceMatch = true
	ref.Confidence = 0.98
	engine := NewEngine(searcher, ref, 0.95)

	track1 := domain.Track{Title: "Clint Eastwood", Artists: []string{"Gorillaz"}, DurationMs: 340000}
	track2 := domain.Track{Title: "Clint Eastwood (Remaster)", Artists: []string{"Gorillaz"}, DurationMs: 340000}

	eval1, err := engine.EvaluateTrackHeuristic(context.Background(), track1)
	if err != nil {
		t.Fatalf("eval1 error: %v", err)
	}
	eval2, err := engine.EvaluateTrackHeuristic(context.Background(), track2)
	if err != nil {
		t.Fatalf("eval2 error: %v", err)
	}

	evals := []*CandidateEvaluation{eval1, eval2}
	err = engine.ResolveBatch(context.Background(), evals)
	if err != nil {
		t.Fatalf("ResolveBatch error: %v", err)
	}

	for i, ev := range evals {
		if ev.Result == nil {
			t.Fatalf("eval %d result is nil", i)
		}
		if ev.Result.Decision != domain.DecisionAIRefereeMatch && ev.Result.Decision != domain.DecisionAutoMatch {
			t.Fatalf("expected match decision for eval %d, got: %s", i, ev.Result.Decision)
		}
	}
}
