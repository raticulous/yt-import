package referee

import (
	"testing"

	"yt-import/internal/domain"
)

func TestParseVerdictJSONFormats(t *testing.T) {
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
			raw:        "```json\n{\"verdict\": \"NO_MATCH\", \"matched_index\": -1, \"confidence\": 0.45, \"reasoning\": \"different track\"}\n```",
			wantMatch:  "NO_MATCH",
			wantIndex:  -1,
			wantReason: "different track",
		},
		{
			name:       "JSON with surrounding text",
			raw:        "Here is the evaluation:\n{\"verdict\": \"MATCH\", \"matched_index\": 1, \"confidence\": 0.97, \"reasoning\": \"official ATV track\"}\nDone.",
			wantMatch:  "MATCH",
			wantIndex:  1,
			wantReason: "official ATV track",
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

func TestAntigravityRefereeInstantiation(t *testing.T) {
	ref, err := NewAntigravityReferee("")
	if err != nil {
		t.Logf("Antigravity CLI not present in test environment: %v", err)
		return
	}
	if ref == nil {
		t.Fatalf("Expected non-nil referee")
	}
}

func TestBuildUserPromptFormatting(t *testing.T) {
	source := domain.Track{
		Title:      "Вертайсь росою",
		Artists:    []string{"Нельсон"},
		DurationMs: 180000,
	}
	candidates := []domain.Candidate{
		{
			VideoID:   "v1",
			Title:     "Вертайсь росою",
			Artists:   []string{"Нельсон"},
			VideoType: domain.TypeAudioTrackVideo,
			Score:     0.625,
		},
	}

	prompt := BuildUserPrompt(source, candidates)
	if prompt == "" {
		t.Fatalf("Prompt should not be empty")
	}
}

func TestAntigravityRefereeLiveDecide(t *testing.T) {
	ref, err := NewAntigravityReferee("")
	if err != nil {
		t.Skip("Antigravity CLI not found in PATH")
	}

	source := domain.Track{
		Title:      "Disassociate",
		Artists:    []string{"Jutes"},
		DurationMs: 184000,
	}
	candidates := []domain.Candidate{
		{
			VideoID:    "fKJFFYtxc_M",
			Title:      "Disassociate",
			Artists:    []string{"Jutes"},
			DurationMs: 283000,
			VideoType:  domain.TypeOfficialMusicVideo,
			Score:      0.90,
		},
		{
			VideoID:    "SPJRHWagfDY",
			Title:      "Disassociate",
			Artists:    []string{"Jutes"},
			DurationMs: 184000,
			VideoType:  domain.TypeAudioTrackVideo,
			Score:      0.925,
		},
	}

	verdict, err := ref.Decide(t.Context(), source, candidates)
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	t.Logf("Decide verdict: %+v", verdict)
}
