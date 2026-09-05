package matcher

import (
	"math"
	"strings"

	"yt-import/internal/domain"
)

const (
	WeightTitle    = 0.35
	WeightArtist   = 0.30
	WeightDuration = 0.25
	WeightType     = 0.10
)

// ScoreCandidate evaluates a single candidate against the source track.
// Returns the composite score and detailed sub-scores.
func ScoreCandidate(source domain.Track, cand *domain.Candidate) (score float64, disqualified bool, reason string) {
	// 1. Disqualification checks (strict precision guardrails)
	srcMods := ExtractModifiers(source.Title + " " + source.Album)
	candMods := ExtractModifiers(cand.Title + " " + cand.ChannelTitle + " " + cand.Album)

	if !srcMods.IsLive && candMods.IsLive {
		return 0.0, true, "Candidate is a live recording while source is studio"
	}
	if !srcMods.IsAcoustic && candMods.IsAcoustic {
		return 0.0, true, "Candidate is acoustic version while source is original"
	}
	if !srcMods.IsRemix && candMods.IsRemix {
		return 0.0, true, "Candidate is a remix while source is original"
	}
	if !srcMods.IsInstrumental && candMods.IsInstrumental {
		return 0.0, true, "Candidate is instrumental/karaoke"
	}
	if !srcMods.IsCover && candMods.IsCover {
		return 0.0, true, "Candidate is a cover or tribute recording"
	}
	if candMods.IsFanEdit {
		return 0.0, true, "Candidate is a fan edit (slowed/reverb/nightcore/8d audio)"
	}

	// 2. Title Scoring
	cleanSrcTitle := CleanTitle(source.Title)
	cleanCandTitle := CleanTitle(cand.Title)

	// If candidate title contains artist name prefix (e.g. "Artist - Song"), strip artist
	for _, art := range source.Artists {
		cleanArt := CleanArtist(art)
		cleanCandTitle = strings.TrimPrefix(cleanCandTitle, cleanArt+" - ")
		cleanCandTitle = strings.TrimPrefix(cleanCandTitle, cleanArt+": ")
	}
	cleanCandTitle = strings.TrimSpace(cleanCandTitle)

	tokenSim := TokenSetRatio(cleanSrcTitle, cleanCandTitle)
	levSim := LevenshteinSimilarity(strings.ToLower(cleanSrcTitle), strings.ToLower(cleanCandTitle))
	cand.TitleScore = 0.6*tokenSim + 0.4*levSim
	if strings.EqualFold(cleanSrcTitle, cleanCandTitle) {
		cand.TitleScore = 1.0
	}

	// 3. Artist Scoring
	cand.ArtistScore = computeArtistScore(source, cand)

	// 4. Duration Scoring
	cand.DurationScore = computeDurationScore(source.DurationMs, cand.DurationMs)
	// If candidate duration was omitted by YouTube Music search snippet, but candidate is an official
	// studio release (ATV) with near-perfect title and artist match, elevate neutral duration score
	if cand.DurationMs <= 0 && cand.VideoType == domain.TypeAudioTrackVideo && cand.TitleScore >= 0.98 && cand.ArtistScore >= 0.98 {
		cand.DurationScore = 0.85
	}

	// 5. Track Type Scoring
	switch cand.VideoType {
	case domain.TypeAudioTrackVideo:
		cand.TypeScore = 1.0
	case domain.TypeOfficialMusicVideo:
		cand.TypeScore = 0.75
	case domain.TypeUserGenerated:
		cand.TypeScore = 0.20
	default:
		cand.TypeScore = 0.50
	}

	// Composite Weighted Score
	total := (WeightTitle * cand.TitleScore) +
		(WeightArtist * cand.ArtistScore) +
		(WeightDuration * cand.DurationScore) +
		(WeightType * cand.TypeScore)

	cand.Score = math.Max(0.0, math.Min(1.0, total))
	return cand.Score, false, ""
}

func computeArtistScore(source domain.Track, cand *domain.Candidate) float64 {
	primary := source.PrimaryArtist()
	cleanPrimary := strings.ToLower(CleanArtist(primary))

	// Collect candidate artist tokens
	candArtistsJoined := strings.ToLower(strings.Join(cand.Artists, " ") + " " + CleanArtist(cand.ChannelTitle))

	// If primary artist isn't mentioned anywhere in candidate artists, channel, or raw title:
	rawCandText := strings.ToLower(cand.Title + " " + cand.ChannelTitle + " " + strings.Join(cand.Artists, " "))
	if cleanPrimary != "" && !strings.Contains(rawCandText, cleanPrimary) {
		// Test fuzzy match on primary artist
		maxSim := 0.0
		for _, ca := range cand.Artists {
			sim := JaroWinkler(cleanPrimary, strings.ToLower(CleanArtist(ca)))
			if sim > maxSim {
				maxSim = sim
			}
		}
		if maxSim < 0.75 {
			// Severe penalty if primary artist is completely wrong
			return 0.10
		}
	}

	// Check token set overlap across all source artists and candidate artists
	srcArtistStr := strings.Join(source.Artists, " ")
	for _, f := range ExtractFeaturedArtists(source.Title) {
		srcArtistStr += " " + f
	}

	return TokenSetRatio(srcArtistStr, candArtistsJoined)
}

func computeDurationScore(srcMs, candMs int) float64 {
	if srcMs <= 0 || candMs <= 0 {
		return 0.70 // Neutral score if duration is unknown
	}

	deltaSec := math.Abs(float64(srcMs-candMs)) / 1000.0

	switch {
	case deltaSec <= 2.0:
		return 1.0
	case deltaSec <= 4.0:
		return 0.85
	case deltaSec <= 8.0:
		return 0.50
	case deltaSec <= 12.0:
		return 0.20
	default:
		return 0.0
	}
}
