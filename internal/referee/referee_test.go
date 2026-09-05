package referee

import (
	"context"
	"strings"
	"testing"

	"yt-import/internal/config"
	"yt-import/internal/domain"
)

func TestBuildUserPrompt(t *testing.T) {
	source := domain.Track{
		Title:      "One More Time",
		Artists:    []string{"Daft Punk"},
		Album:      "Discovery",
		DurationMs: 320000,
		ISRC:       "GBDUW0000053",
		Explicit:   false,
	}

	candidates := []domain.Candidate{
		{
			VideoID:      "fa5IWHDbftI",
			Title:        "One More Time",
			Artists:      []string{"Daft Punk"},
			Album:        "Discovery",
			DurationMs:   320500,
			VideoType:    domain.TypeAudioTrackVideo,
			ChannelTitle: "Daft Punk - Topic",
			Score:        0.98,
		},
	}

	prompt := BuildUserPrompt(source, candidates)
	if !strings.Contains(prompt, "One More Time") {
		t.Fatalf("Prompt missing title")
	}
	if !strings.Contains(prompt, "fa5IWHDbftI") {
		t.Fatalf("Prompt missing video id")
	}
	if !strings.Contains(prompt, "GBDUW0000053") {
		t.Fatalf("Prompt missing ISRC")
	}
}

func TestMockReferee(t *testing.T) {
	ref := NewMockReferee()

	source := domain.Track{
		Title:      "One More Time",
		Artists:    []string{"Daft Punk"},
		DurationMs: 320000,
	}

	candidates := []domain.Candidate{
		{
			VideoID: "fa5IWHDbftI",
			Title:   "One More Time",
			Score:   0.88,
		},
	}

	verdict, err := ref.Decide(context.Background(), source, candidates)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if verdict.Verdict != "MATCH" || verdict.MatchedIndex != 0 {
		t.Fatalf("Expected match at index 0, got %s at %d", verdict.Verdict, verdict.MatchedIndex)
	}
}

func TestRefereeFactory(t *testing.T) {
	cfg := config.DefaultConfig()

	// Mock provider should always succeed without credentials
	ref, err := NewReferee("mock", cfg)
	if err != nil {
		t.Fatalf("Failed to create mock referee: %v", err)
	}
	if ref == nil {
		t.Fatalf("Mock referee is nil")
	}

	// Antigravity referee should succeed since agy is in PATH
	antigravityRef, err := NewReferee("antigravity", cfg)
	if err != nil {
		t.Logf("Antigravity referee creation skipped/noted: %v", err)
	} else if antigravityRef == nil {
		t.Fatalf("Antigravity referee is nil")
	}

	// Gemini without key should error
	cfg.GeminiAPIKey = ""
	_, err = NewReferee("gemini", cfg)
	if err == nil {
		t.Fatalf("Expected error when Gemini API key is missing")
	}
}

func TestParseVerdictJSON(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantMatch  string
		wantIndex  int
		wantReason string
	}{
		{
			name:       "Plain JSON",
			raw:        `{"verdict": "MATCH", "matched_index": 0, "confidence": 0.98, "reasoning": "exact match"}`,
			wantMatch:  "MATCH",
			wantIndex:  0,
			wantReason: "exact match",
		},
		{
			name:       "Markdown fenced JSON",
			raw:        "Here is my verdict:\n```json\n{\"verdict\": \"NO_MATCH\", \"matched_index\": -1, \"confidence\": 0.45, \"reasoning\": \"different track\"}\n```\nHope that helps!",
			wantMatch:  "NO_MATCH",
			wantIndex:  -1,
			wantReason: "different track",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := ParseVerdictJSON(tt.raw)
			if err != nil {
				t.Fatalf("ParseVerdictJSON failed: %v", err)
			}
			if v.Verdict != tt.wantMatch {
				t.Errorf("Verdict = %q, want %q", v.Verdict, tt.wantMatch)
			}
			if v.MatchedIndex != tt.wantIndex {
				t.Errorf("MatchedIndex = %d, want %d", v.MatchedIndex, tt.wantIndex)
			}
			if v.Reasoning != tt.wantReason {
				t.Errorf("Reasoning = %q, want %q", v.Reasoning, tt.wantReason)
			}
		})
	}
}

func TestValidateProvider(t *testing.T) {
	ctx := context.Background()

	// Mock provider should always be valid
	if err := ValidateProvider(ctx, "mock", nil); err != nil {
		t.Errorf("expected mock provider to be valid, got: %v", err)
	}

	// Unknown provider should error
	if err := ValidateProvider(ctx, "nonexistent", nil); err == nil {
		t.Errorf("expected error for nonexistent provider, got nil")
	}

	// Gemini without key should error
	cfg := config.DefaultConfig()
	cfg.GeminiAPIKey = ""
	if err := ValidateProvider(ctx, "gemini", cfg); err == nil {
		t.Errorf("expected error for missing gemini key, got nil")
	}

	// OpenAI without key should error
	cfg.OpenAIAPIKey = ""
	if err := ValidateProvider(ctx, "openai", cfg); err == nil {
		t.Errorf("expected error for missing openai key, got nil")
	}

	// Claude without key should error
	cfg.AnthropicAPIKey = ""
	if err := ValidateProvider(ctx, "claude", cfg); err == nil {
		t.Errorf("expected error for missing claude key, got nil")
	}
}

