package referee

import (
	"encoding/json"
	"fmt"
	"strings"

	"yt-import/internal/domain"
)

// BatchItem wraps a track and its candidates for batch AI referee evaluation.
type BatchItem struct {
	ItemID     int
	Source     domain.Track
	Candidates []domain.Candidate
}

const SystemPrompt = `You are an expert music metadata referee operating under strict 95%+ precision rules.
Your task is to determine whether ANY of the provided candidate tracks is an EXACT match for the Source Track from Spotify.

CRITICAL OPERATIONAL CONSTRAINTS:
1. Do NOT invoke any tools, subagents, commands, or browse any URLs.
2. Judge solely on the text metadata provided in this prompt.
3. You MUST respond ONLY with valid JSON. No explanatory preamble, no markdown formatting other than optional code fence.

CRITICAL DISQUALIFICATION RULES:
1. NEVER match a song that is a different version, cover, live version (unless source is live), acoustic version (unless source is acoustic), remix (unless source is remix), instrumental, karaoke, or parody.
2. If candidate is by a tribute artist or different band, return verdict: "NO_MATCH".
3. Pay close attention to track duration! Official digital masters match within 1-3 seconds. Music videos with long intros/skits or dialogue should NOT be accepted if an official audio track is available.
   NOTE: If an official audio track (type: ATV / Topic channel) shows duration 0:00 (0 ms), this is an omission in YouTube Music search metadata. If the track title and artist match the source, accept it as the official studio master.
4. If multiple candidates represent the same recording, ALWAYS select the official studio audio track (type: ATV / Topic channel) over music videos.
5. If NO candidate meets >= 0.95 confidence, return verdict: "NO_MATCH" with matched_index: -1.

You MUST respond ONLY with valid JSON in this exact structure:
{
  "verdict": "MATCH" or "NO_MATCH",
  "matched_index": 0,
  "confidence": 0.98,
  "reasoning": "Candidate 0 is the official label ATV release matching artist, title, album, and duration within 1s."
}`

const BatchSystemPrompt = `You are an expert music metadata referee operating under strict 95%+ precision rules.
Your task is to evaluate multiple Source Tracks against their YouTube Music candidates and decide whether an EXACT match exists for each item.

CRITICAL OPERATIONAL CONSTRAINTS:
1. Do NOT invoke any tools, subagents, commands, or browse any URLs.
2. Judge solely on the text metadata provided in this prompt.
3. You MUST respond ONLY with a valid JSON array of verdicts matching all item_ids. No preamble, no markdown formatting other than optional code fence.

CRITICAL DISQUALIFICATION RULES:
1. NEVER match a song that is a different version, cover, live version (unless source is live), acoustic version (unless source is acoustic), remix (unless source is remix), instrumental, karaoke, or parody.
2. If candidate is by a tribute artist or different band, return verdict: "NO_MATCH" with matched_index: -1.
3. Pay close attention to track duration! Official digital masters match within 1-3 seconds. Music videos with long intros/skits or dialogue should NOT be accepted if an official audio track is available.
   NOTE: If an official audio track (type: ATV / Topic channel) shows duration 0:00 (0 ms), this is an omission in YouTube Music search metadata. If the track title and artist match the source, accept it as the official studio master.
4. If multiple candidates represent the same recording, ALWAYS select the official studio audio track (type: ATV / Topic channel) over music videos.
5. If NO candidate meets >= 0.95 confidence, return verdict: "NO_MATCH" with matched_index: -1.

You MUST respond ONLY with a valid JSON array of verdict objects matching this exact structure:
[
  {
    "item_id": 0,
    "verdict": "MATCH",
    "matched_index": 0,
    "confidence": 0.98,
    "reasoning": "Candidate 0 is the official label release matching artist, title, album, and duration within 1s."
  },
  {
    "item_id": 1,
    "verdict": "NO_MATCH",
    "matched_index": -1,
    "confidence": 0.40,
    "reasoning": "No official studio match found; only covers or remixes available."
  }
]`

// BuildUserPrompt generates the user query comparing source track against top candidates.
func BuildUserPrompt(source domain.Track, candidates []domain.Candidate) string {
	var sb strings.Builder

	sb.WriteString("### SOURCE TRACK (SPOTIFY)\n")
	sb.WriteString(fmt.Sprintf("- Title: %s\n", source.Title))
	sb.WriteString(fmt.Sprintf("- Artist(s): %s\n", strings.Join(source.Artists, ", ")))
	if source.Album != "" {
		sb.WriteString(fmt.Sprintf("- Album: %s\n", source.Album))
	}
	sb.WriteString(fmt.Sprintf("- Duration: %s (%d ms)\n", source.FormattedDuration(), source.DurationMs))
	if source.ISRC != "" {
		sb.WriteString(fmt.Sprintf("- ISRC: %s\n", source.ISRC))
	}
	sb.WriteString(fmt.Sprintf("- Explicit: %v\n", source.Explicit))
	sb.WriteString("\n### YOUTUBE MUSIC CANDIDATES\n")

	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("[%d] VideoID: %s\n", i, c.VideoID))
		sb.WriteString(fmt.Sprintf("    Title: %s\n", c.Title))
		sb.WriteString(fmt.Sprintf("    Artist(s) / Channel: %s / %s\n", strings.Join(c.Artists, ", "), c.ChannelTitle))
		if c.Album != "" {
			sb.WriteString(fmt.Sprintf("    Album: %s\n", c.Album))
		}
		sb.WriteString(fmt.Sprintf("    Duration: %s (%d ms)\n", c.FormattedDuration(), c.DurationMs))
		sb.WriteString(fmt.Sprintf("    Type: %s\n", c.VideoType))
		sb.WriteString(fmt.Sprintf("    Heuristic Score: %.2f%%\n\n", c.Score*100))
	}

	sb.WriteString("Judge each candidate carefully and return your JSON verdict.")
	return sb.String()
}

// BuildBatchPrompt constructs a consolidated prompt evaluating multiple ambiguous tracks simultaneously.
func BuildBatchPrompt(items []BatchItem) string {
	var sb strings.Builder
	sb.WriteString("Evaluate the following tracks and return a JSON array containing one verdict object per item_id:\n\n")

	for _, item := range items {
		sb.WriteString(fmt.Sprintf("=== ITEM %d ===\n", item.ItemID))
		sb.WriteString(fmt.Sprintf("SOURCE TRACK: %s - %s", item.Source.PrimaryArtist(), item.Source.Title))
		if item.Source.Album != "" {
			sb.WriteString(fmt.Sprintf(" | Album: %s", item.Source.Album))
		}
		sb.WriteString(fmt.Sprintf(" | Duration: %s (%d ms)", item.Source.FormattedDuration(), item.Source.DurationMs))
		if item.Source.ISRC != "" {
			sb.WriteString(fmt.Sprintf(" | ISRC: %s", item.Source.ISRC))
		}
		sb.WriteString("\nCANDIDATES:\n")

		for ci, c := range item.Candidates {
			sb.WriteString(fmt.Sprintf("  [%d] VideoID: %s | Title: %s | Channel: %s | Album: %s | Duration: %s | Type: %s | Heuristic: %.1f%%\n",
				ci, c.VideoID, c.Title, c.ChannelTitle, c.Album, c.FormattedDuration(), c.VideoType, c.Score*100))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Return ONLY the JSON array of verdicts matching all item_ids.")
	return sb.String()
}

// ParseVerdictJSON extracts and unmarshals a single RefereeVerdict from LLM response text.
func ParseVerdictJSON(raw string) (*domain.RefereeVerdict, error) {
	cleaned := strings.TrimSpace(raw)

	// Remove markdown code fences if present
	if idx := strings.Index(cleaned, "```json"); idx != -1 {
		cleaned = cleaned[idx+7:]
	} else if idx := strings.Index(cleaned, "```"); idx != -1 {
		cleaned = cleaned[idx+3:]
	}
	if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
		cleaned = cleaned[:idx]
	}
	cleaned = strings.TrimSpace(cleaned)

	// Extract outermost JSON object
	firstBrace := strings.Index(cleaned, "{")
	lastBrace := strings.LastIndex(cleaned, "}")
	if firstBrace != -1 && lastBrace != -1 && lastBrace > firstBrace {
		cleaned = cleaned[firstBrace : lastBrace+1]
	}

	var verdict domain.RefereeVerdict
	if err := json.Unmarshal([]byte(cleaned), &verdict); err != nil {
		return nil, fmt.Errorf("failed to parse referee verdict JSON: %w (raw output: %s)", err, raw)
	}

	return &verdict, nil
}

// ParseBatchVerdictJSON extracts and unmarshals a list of RefereeVerdicts from batch LLM response text.
func ParseBatchVerdictJSON(raw string) ([]domain.RefereeVerdict, error) {
	cleaned := strings.TrimSpace(raw)

	// Remove markdown code fences if present
	if idx := strings.Index(cleaned, "```json"); idx != -1 {
		cleaned = cleaned[idx+7:]
	} else if idx := strings.Index(cleaned, "```"); idx != -1 {
		cleaned = cleaned[idx+3:]
	}
	if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
		cleaned = cleaned[:idx]
	}
	cleaned = strings.TrimSpace(cleaned)

	// Case 1: Outermost JSON array [...]
	firstBracket := strings.Index(cleaned, "[")
	lastBracket := strings.LastIndex(cleaned, "]")
	if firstBracket != -1 && lastBracket != -1 && lastBracket > firstBracket {
		arrayStr := cleaned[firstBracket : lastBracket+1]
		var verdicts []domain.RefereeVerdict
		if err := json.Unmarshal([]byte(arrayStr), &verdicts); err == nil && len(verdicts) > 0 {
			return verdicts, nil
		}
	}

	// Case 2: Wrapped in an object e.g. {"verdicts": [...]} or {"results": [...]}
	var wrapper map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &wrapper); err == nil {
		for _, key := range []string{"verdicts", "results", "items", "data"} {
			if val, ok := wrapper[key]; ok {
				subBytes, _ := json.Marshal(val)
				var verdicts []domain.RefereeVerdict
				if err := json.Unmarshal(subBytes, &verdicts); err == nil && len(verdicts) > 0 {
					return verdicts, nil
				}
			}
		}
	}

	// Case 3: Single object returned instead of array (e.g. if only 1 item was in batch)
	firstBrace := strings.Index(cleaned, "{")
	lastBrace := strings.LastIndex(cleaned, "}")
	if firstBrace != -1 && lastBrace != -1 && lastBrace > firstBrace {
		objStr := cleaned[firstBrace : lastBrace+1]
		var single domain.RefereeVerdict
		if err := json.Unmarshal([]byte(objStr), &single); err == nil && single.Verdict != "" {
			return []domain.RefereeVerdict{single}, nil
		}
	}

	return nil, fmt.Errorf("failed to parse batch referee verdicts JSON (raw: %s)", raw)
}
