package matcher

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"yt-import/internal/domain"
	"yt-import/internal/referee"
)

// Searcher represents a search service capable of finding candidates on the target platform.
type Searcher interface {
	Search(ctx context.Context, query string) ([]domain.Candidate, error)
}

// Engine coordinates query formulation, search execution, scoring, and referee arbitration.
type Engine struct {
	searcher  Searcher
	referee   referee.Referee
	threshold float64
}

// NewEngine constructs a new matching engine.
func NewEngine(searcher Searcher, ref referee.Referee, threshold float64) *Engine {
	if threshold <= 0 {
		threshold = 0.95
	}
	return &Engine{
		searcher:  searcher,
		referee:   ref,
		threshold: threshold,
	}
}

// CandidateEvaluation contains candidates and preliminary heuristic decision before referee review.
type CandidateEvaluation struct {
	Track            domain.Track
	ScoredCandidates []domain.Candidate
	BestCandidate    *domain.Candidate
	NeedsReferee     bool
	Result           *domain.MatchResult
}

// Match performs the full matching pipeline for a source track (evaluates heuristic + resolves referee).
func (e *Engine) Match(ctx context.Context, track domain.Track) (*domain.MatchResult, error) {
	eval, err := e.EvaluateTrackHeuristic(ctx, track)
	if err != nil {
		return nil, err
	}
	if eval.Result != nil {
		return eval.Result, nil
	}
	if err := e.ResolveBatch(ctx, []*CandidateEvaluation{eval}); err != nil {
		return nil, err
	}
	return eval.Result, nil
}

// EvaluateTrackHeuristic executes queries and heuristic scoring for a track.
// If the match is unambiguous (>= threshold) or disqualified (< 0.70 / no candidates),
// Result is populated immediately. Otherwise NeedsReferee is set to true.
func (e *Engine) EvaluateTrackHeuristic(ctx context.Context, track domain.Track) (*CandidateEvaluation, error) {
	// 1. Multi-tier query generation
	queries := e.generateQueries(track)
	candidatesMap := make(map[string]domain.Candidate)

	for _, q := range queries {
		results, err := e.searcher.Search(ctx, q)
		if err != nil {
			continue
		}
		for _, c := range results {
			if _, exists := candidatesMap[c.VideoID]; !exists {
				candidatesMap[c.VideoID] = c
			}
		}
		if len(candidatesMap) >= 5 {
			hasStrongCandidate := false
			for _, c := range candidatesMap {
				candCopy := c
				score, disq, _ := ScoreCandidate(track, &candCopy)
				if !disq && score >= 0.80 {
					hasStrongCandidate = true
					break
				}
			}
			if hasStrongCandidate {
				break
			}
		}
	}

	if len(candidatesMap) == 0 {
		return &CandidateEvaluation{
			Track: track,
			Result: &domain.MatchResult{
				SourceTrack: track,
				Decision:    domain.DecisionNoCandidates,
				Confidence:  0.0,
				Reason:      "No search candidates returned from YouTube Music",
			},
		}, nil
	}

	// 2. Score all candidates
	var scoredCandidates []domain.Candidate
	for _, c := range candidatesMap {
		candCopy := c
		score, disq, disqReason := ScoreCandidate(track, &candCopy)
		if disq {
			candCopy.Score = 0.0
			candCopy.ChannelTitle += " [DISQUALIFIED: " + disqReason + "]"
		} else {
			candCopy.Score = score
		}
		scoredCandidates = append(scoredCandidates, candCopy)
	}

	sort.Slice(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].Score > scoredCandidates[j].Score
	})

	best := scoredCandidates[0]

	// 3. Check for ambiguity
	isUnambiguous := true
	if len(scoredCandidates) > 1 {
		secondBest := scoredCandidates[1]
		if best.Score-secondBest.Score < 0.04 && best.Score >= 0.85 {
			isUnambiguous = false
		}
	}

	// Case A: High-Confidence Auto-Match
	if best.Score >= e.threshold && isUnambiguous {
		return &CandidateEvaluation{
			Track:            track,
			ScoredCandidates: scoredCandidates,
			BestCandidate:    &best,
			Result: &domain.MatchResult{
				SourceTrack:   track,
				Candidate:     &best,
				AllCandidates: scoredCandidates,
				Confidence:    best.Score,
				Decision:      domain.DecisionAutoMatch,
				Reason:        fmt.Sprintf("Heuristic match (%.1f%% >= %.1f%% threshold)", best.Score*100, e.threshold*100),
			},
		}, nil
	}

	// Case B: Needs referee review (whenever referee is enabled and candidates exist)
	if e.referee != nil {
		return &CandidateEvaluation{
			Track:            track,
			ScoredCandidates: scoredCandidates,
			BestCandidate:    &best,
			NeedsReferee:     true,
		}, nil
	}

	// Case C: Below threshold without referee (referee disabled)
	return &CandidateEvaluation{
		Track:            track,
		ScoredCandidates: scoredCandidates,
		BestCandidate:    &best,
		Result: &domain.MatchResult{
			SourceTrack:   track,
			Candidate:     &best,
			AllCandidates: scoredCandidates,
			Confidence:    best.Score,
			Decision:      domain.DecisionSkippedThreshold,
			Reason:        fmt.Sprintf("Top candidate below %.1f%% threshold (%.1f%%, no AI referee enabled)", e.threshold*100, best.Score*100),
		},
	}, nil
}

// ResolveBatch batches all evaluations requiring referee arbitration into consolidated AI calls.
func (e *Engine) ResolveBatch(ctx context.Context, evals []*CandidateEvaluation) error {
	var batchItems []referee.BatchItem
	itemToEvalIdx := make(map[int]int)

	for evalIdx, ev := range evals {
		if ev == nil || !ev.NeedsReferee {
			continue
		}
		limit := min(len(ev.ScoredCandidates), 5)
		topCandidates := ev.ScoredCandidates[:limit]

		itemID := len(batchItems)
		batchItems = append(batchItems, referee.BatchItem{
			ItemID:     itemID,
			Source:     ev.Track,
			Candidates: topCandidates,
		})
		itemToEvalIdx[itemID] = evalIdx
	}

	if len(batchItems) == 0 {
		return nil
	}

	// Max 20 items per batch prompt to avoid context overflow; chunk if larger
	const chunkSize = 20
	for chunkStart := 0; chunkStart < len(batchItems); chunkStart += chunkSize {
		chunkEnd := chunkStart + chunkSize
		if chunkEnd > len(batchItems) {
			chunkEnd = len(batchItems)
		}
		chunk := batchItems[chunkStart:chunkEnd]

		verdicts, err := e.referee.DecideBatch(ctx, chunk)
		if err != nil {
			// Mark chunk items as referee error
			for _, item := range chunk {
				evalIdx := itemToEvalIdx[item.ItemID]
				ev := evals[evalIdx]
				ev.Result = &domain.MatchResult{
					SourceTrack:   ev.Track,
					Candidate:     ev.BestCandidate,
					AllCandidates: ev.ScoredCandidates,
					Confidence:    0.0,
					Decision:      domain.DecisionSkippedThreshold,
					Reason:        fmt.Sprintf("AI Referee error: %v", err),
				}
			}
			continue
		}

		verdictMap := make(map[int]domain.RefereeVerdict)
		for _, v := range verdicts {
			verdictMap[v.ItemID] = v
		}

		for _, item := range chunk {
			evalIdx := itemToEvalIdx[item.ItemID]
			ev := evals[evalIdx]
			limit := min(len(ev.ScoredCandidates), 5)
			topCandidates := ev.ScoredCandidates[:limit]

			verdict, found := verdictMap[item.ItemID]
			if !found {
				verdict = domain.RefereeVerdict{
					Verdict:      "NO_MATCH",
					MatchedIndex: -1,
					Confidence:   0.0,
					Reasoning:    "Item omitted by referee",
				}
			}

			if verdict.Verdict == "MATCH" && verdict.Confidence >= e.threshold &&
				verdict.MatchedIndex >= 0 && verdict.MatchedIndex < len(topCandidates) {
				matchedCand := topCandidates[verdict.MatchedIndex]
				ev.Result = &domain.MatchResult{
					SourceTrack:   ev.Track,
					Candidate:     &matchedCand,
					AllCandidates: ev.ScoredCandidates,
					Confidence:    verdict.Confidence,
					Decision:      domain.DecisionAIRefereeMatch,
					Reason:        fmt.Sprintf("AI Referee approved (%.1f%% >= %.1f%%): %s", verdict.Confidence*100, e.threshold*100, verdict.Reasoning),
				}
			} else {
				ev.Result = &domain.MatchResult{
					SourceTrack:   ev.Track,
					Candidate:     ev.BestCandidate,
					AllCandidates: ev.ScoredCandidates,
					Confidence:    verdict.Confidence,
					Decision:      domain.DecisionSkippedThreshold,
					Reason:        fmt.Sprintf("AI Referee rejected (%.1f%%): %s", verdict.Confidence*100, verdict.Reasoning),
				}
			}
		}
	}

	return nil
}

// generateQueries creates search queries ordered by specificity.
func (e *Engine) generateQueries(track domain.Track) []string {
	var queries []string

	cleanTitle := CleanTitle(track.Title)
	primaryArtist := CleanArtist(track.PrimaryArtist())

	// Tier 1: ISRC code if available
	if track.ISRC != "" {
		queries = append(queries, track.ISRC)
	}

	// Tier 2: Artist - Title
	if primaryArtist != "" && cleanTitle != "" {
		queries = append(queries, fmt.Sprintf("%s %s", primaryArtist, cleanTitle))
	} else if cleanTitle != "" {
		queries = append(queries, cleanTitle)
	}

	// Tier 3: Title - Artist (Reversed: crucial when artist name is ambiguous or common in YouTube)
	if primaryArtist != "" && cleanTitle != "" {
		queries = append(queries, fmt.Sprintf("%s %s", cleanTitle, primaryArtist))
	}

	// Tier 4: All artists + Title (if multiple artists)
	if len(track.Artists) > 1 && cleanTitle != "" {
		var cleanArtists []string
		for _, a := range track.Artists {
			cleanArtists = append(cleanArtists, CleanArtist(a))
		}
		queries = append(queries, fmt.Sprintf("%s %s", strings.Join(cleanArtists, " "), cleanTitle))
	}

	// Tier 5: Title only (fallback for distinct song titles or cross-script transliteration differences)
	if len(cleanTitle) >= 3 && cleanTitle != primaryArtist {
		queries = append(queries, cleanTitle)
	}

	// Tier 6: Artist - Title + Album (if album is distinct)
	if track.Album != "" && !strings.EqualFold(track.Album, cleanTitle) {
		cleanAlbum := CleanTitle(track.Album)
		queries = append(queries, fmt.Sprintf("%s %s %s", primaryArtist, cleanTitle, cleanAlbum))
	}

	// Deduplicate queries while preserving order
	var uniqueQueries []string
	seen := make(map[string]bool)
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q != "" && !seen[q] {
			seen[q] = true
			uniqueQueries = append(uniqueQueries, q)
		}
	}

	return uniqueQueries
}
