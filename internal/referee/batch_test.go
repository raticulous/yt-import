package referee

import (
	"testing"
	"yt-import/internal/domain"
)

func TestBatchPromptAndParsing(t *testing.T) {
	items := []BatchItem{
		{
			ItemID: 0,
			Source: domain.Track{
				Title:      "Karma Police",
				Artists:    []string{"Radiohead"},
				DurationMs: 261000,
			},
			Candidates: []domain.Candidate{
				{
					VideoID:      "IBH97ma9EiI",
					Title:        "Karma Police",
					Artists:      []string{"Radiohead"},
					ChannelTitle: "Radiohead - Topic",
					DurationMs:   264000,
					VideoType:    "ATV",
					Score:        0.92,
				},
			},
		},
		{
			ItemID: 1,
			Source: domain.Track{
				Title:      "Wonderwall",
				Artists:    []string{"Oasis"},
				DurationMs: 258000,
			},
			Candidates: []domain.Candidate{
				{
					VideoID:      "6hzrDeceEKc",
					Title:        "Wonderwall",
					Artists:      []string{"Oasis"},
					ChannelTitle: "Oasis - Topic",
					DurationMs:   258000,
					VideoType:    "ATV",
					Score:        0.93,
				},
			},
		},
	}

	prompt := BuildBatchPrompt(items)
	if len(prompt) == 0 {
		t.Fatalf("expected non-empty batch prompt")
	}

	// Test array JSON parsing
	rawJSON := `[
		{"item_id": 0, "verdict": "MATCH", "matched_index": 0, "confidence": 0.98, "reasoning": "Official ATV"},
		{"item_id": 1, "verdict": "MATCH", "matched_index": 0, "confidence": 0.99, "reasoning": "Exact studio master"}
	]`

	verdicts, err := ParseBatchVerdictJSON(rawJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verdicts) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(verdicts))
	}
	if verdicts[0].ItemID != 0 || verdicts[1].ItemID != 1 {
		t.Errorf("item IDs not preserved: %+v", verdicts)
	}

	// Test markdown fenced JSON
	fencedJSON := "```json\n" + rawJSON + "\n```"
	verdictsFenced, err := ParseBatchVerdictJSON(fencedJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing fenced JSON: %v", err)
	}
	if len(verdictsFenced) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(verdictsFenced))
	}
}
