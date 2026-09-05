package syncer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"yt-import/internal/domain"
	"yt-import/internal/matcher"
	"yt-import/internal/provider"
)

// EventType defines the kind of progress notification emitted by the syncer.
type EventType string

const (
	EventStart           EventType = "START"
	EventTrackProcessing EventType = "TRACK_PROCESSING"
	EventTrackMatched    EventType = "TRACK_MATCHED"
	EventTrackSkipped    EventType = "TRACK_SKIPPED"
	EventBatchInserted   EventType = "BATCH_INSERTED"
	EventComplete        EventType = "COMPLETE"
	EventError           EventType = "ERROR"
)

// SyncProgress represents the current real-time state of the sync job.
type SyncProgress struct {
	TotalTracks      int
	Processed        int
	DirectMatches    int
	AIRefereeMatches int
	Skipped          int
	AlreadyPresent   int
	Inserted         int
	CurrentBatch     int
	TotalBatches     int
	CurrentTrack     *domain.Track
	LastResult       *domain.MatchResult
	ErrorMessage     string
}

// SyncEvent carries status updates from the syncer to listeners (such as TUI).
type SyncEvent struct {
	Type     EventType
	Progress SyncProgress
	Message  string
}

// Syncer orchestrates the end-to-end migration between source and target.
type Syncer struct {
	source  provider.SourceProvider
	target  provider.TargetProvider
	matcher *matcher.Engine
	options domain.SyncOptions
}

// NewSyncer constructs a new Syncer coordinator.
func NewSyncer(source provider.SourceProvider, target provider.TargetProvider, engine *matcher.Engine, opts domain.SyncOptions) *Syncer {
	return &Syncer{
		source:  source,
		target:  target,
		matcher: engine,
		options: opts,
	}
}

// SyncReport summarizes the outcome of the import run.
type SyncReport struct {
	SourcePlaylistID     string               `json:"source_playlist_id"`
	TargetPlaylistID     string               `json:"target_playlist_id"`
	TotalInPlaylist      int                  `json:"total_in_playlist"`
	Offset               int                  `json:"offset"`
	Limit                int                  `json:"limit"`
	ProcessedTracks      int                  `json:"processed_tracks"`
	DirectMatches        int                  `json:"direct_matches"`
	AIRefereeMatches     int                  `json:"ai_referee_matches"`
	SkippedTracks        int                  `json:"skipped_tracks"`
	AlreadyPresentTracks int                  `json:"already_present_tracks"`
	Results              []domain.MatchResult `json:"results"`
	StartTime            time.Time            `json:"start_time"`
	Duration             time.Duration        `json:"duration"`
}

// Run executes the synchronization process in 100-track batches and streams progress through eventChan.
func (s *Syncer) Run(ctx context.Context, eventChan chan<- SyncEvent) (*SyncReport, error) {
	startTime := time.Now()
	report := &SyncReport{
		SourcePlaylistID: s.options.SourcePlaylistID,
		TargetPlaylistID: s.options.TargetPlaylistID,
		StartTime:        startTime,
	}

	// 1. Discover source playlist metadata and true total track count if available
	sourcePL, _ := s.source.GetPlaylist(ctx, s.options.SourcePlaylistID)
	totalInPlaylist := 0
	playlistTitle := "Imported Spotify Playlist"
	if sourcePL != nil {
		totalInPlaylist = sourcePL.TrackCount
		if sourcePL.Title != "" {
			playlistTitle = sourcePL.Title
		}
	}

	startOffset := s.options.Offset
	if startOffset < 0 {
		startOffset = 0
	}

	report.TotalInPlaylist = totalInPlaylist
	report.Offset = startOffset
	report.Limit = s.options.Limit

	// 2. Target playlist handling: Auto-create target playlist if not specified
	if !s.options.DryRun && s.options.TargetPlaylistID == "" {
		newID, err := s.target.CreatePlaylist(ctx, playlistTitle, "Imported from Spotify via yt-import")
		if err != nil {
			s.emit(eventChan, SyncEvent{
				Type:    EventError,
				Message: fmt.Sprintf("Failed to auto-create target YouTube Music playlist: %v", err),
			})
		} else {
			s.options.TargetPlaylistID = newID
			report.TargetPlaylistID = newID
			s.emit(eventChan, SyncEvent{
				Type:    EventStart,
				Message: fmt.Sprintf("Auto-created target YouTube Music playlist: '%s' (ID: %s)", playlistTitle, newID),
			})
		}
	}

	// Fast pre-check: inspect what target playlist already contains and build in-memory index
	var existingTracks []domain.Candidate
	if s.options.TargetPlaylistID != "" {
		s.emit(eventChan, SyncEvent{
			Type:    EventStart,
			Message: "Scanning target YouTube Music playlist to index all existing tracks...",
		})
		existing, err := s.target.GetPlaylistTracks(ctx, s.options.TargetPlaylistID)
		if err == nil && len(existing) > 0 {
			existingTracks = existing
			s.emit(eventChan, SyncEvent{
				Type:    EventStart,
				Message: fmt.Sprintf("Indexed %d existing tracks in target YouTube Music playlist. Duplicate detection active across all batches.", len(existingTracks)),
			})
		}
	}
	existingIndex := buildExistingIndex(existingTracks)

	// Determine total expected tracks to process
	totalToProcess := 0
	if totalInPlaylist > startOffset {
		totalToProcess = totalInPlaylist - startOffset
	}
	if s.options.Limit > 0 {
		if totalToProcess == 0 || s.options.Limit < totalToProcess {
			totalToProcess = s.options.Limit
		}
	}

	const batchSize = 100
	currOffset := startOffset
	batchNum := 0
	progress := SyncProgress{
		TotalTracks: totalToProcess,
	}

	concurrency := s.options.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}

	var allResults []domain.MatchResult

	// 3. Process playlist in incremental batches of 100 tracks
	for {
		select {
		case <-ctx.Done():
			report.Duration = time.Since(startTime)
			report.Results = allResults
			report.ProcessedTracks = len(allResults)
			return report, ctx.Err()
		default:
		}

		if totalToProcess > 0 && progress.Processed >= totalToProcess {
			break
		}

		curBatchLimit := batchSize
		if s.options.Limit > 0 {
			remainingForLimit := s.options.Limit - progress.Processed
			if remainingForLimit <= 0 {
				break
			}
			if remainingForLimit < curBatchLimit {
				curBatchLimit = remainingForLimit
			}
		}

		// Fetch slice from source
		batchTracks, reportedTotal, err := s.source.GetPlaylistTracks(ctx, s.options.SourcePlaylistID, currOffset, curBatchLimit)
		if err != nil {
			s.emit(eventChan, SyncEvent{
				Type:     EventError,
				Progress: progress,
				Message:  fmt.Sprintf("Failed to fetch tracks from source at offset %d: %v", currOffset, err),
			})
			report.Duration = time.Since(startTime)
			report.Results = allResults
			report.ProcessedTracks = len(allResults)
			return report, err
		}

		if len(batchTracks) == 0 {
			break
		}

		if reportedTotal > totalInPlaylist {
			totalInPlaylist = reportedTotal
			report.TotalInPlaylist = totalInPlaylist
			if totalInPlaylist > startOffset {
				computed := totalInPlaylist - startOffset
				if s.options.Limit > 0 && s.options.Limit < computed {
					computed = s.options.Limit
				}
				totalToProcess = computed
				progress.TotalTracks = totalToProcess
			}
		}

		if progress.TotalTracks == 0 {
			progress.TotalTracks = len(batchTracks)
		}

		batchNum++
		totalBatches := 1
		if progress.TotalTracks > 0 {
			totalBatches = (progress.TotalTracks + batchSize - 1) / batchSize
		}
		if totalBatches < batchNum {
			totalBatches = batchNum
		}
		progress.CurrentBatch = batchNum
		progress.TotalBatches = totalBatches

		s.emit(eventChan, SyncEvent{
			Type:     EventStart,
			Progress: progress,
			Message:  fmt.Sprintf("Batch %d/%d: Processing %d tracks (offset %d)...", batchNum, totalBatches, len(batchTracks), currOffset),
		})

		batchResults := make([]domain.MatchResult, len(batchTracks))
		batchEvals := make([]*matcher.CandidateEvaluation, len(batchTracks))
		var mu sync.Mutex

		workerCount := concurrency
		if workerCount > len(batchTracks) {
			workerCount = len(batchTracks)
		}
		if workerCount <= 0 {
			workerCount = 1
		}

		type trackJob struct {
			index int
			track domain.Track
		}
		jobs := make(chan trackJob, len(batchTracks))
		for i, track := range batchTracks {
			jobs <- trackJob{index: i, track: track}
		}
		close(jobs)

		// Phase 1: Concurrent Heuristic Matching & Candidate Searching
		var wg sync.WaitGroup
		for w := 0; w < workerCount; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					select {
					case <-ctx.Done():
						return
					default:
					}

					// Fast Check: Skip if already present in target playlist
					mu.Lock()
					exists, matchedTitle := checkAlreadyInPlaylist(job.track, existingIndex, existingTracks)
					mu.Unlock()

					if exists {
						mu.Lock()
						res := domain.MatchResult{
							SourceTrack: job.track,
							Decision:    domain.DecisionAlreadyExists,
							Confidence:  1.0,
							Reason:      fmt.Sprintf("Already in target YouTube Music playlist ('%s')", matchedTitle),
						}
						batchResults[job.index] = res
						progress.Processed++
						progress.AlreadyPresent++
						report.AlreadyPresentTracks++
						progress.LastResult = &res

						s.emit(eventChan, SyncEvent{
							Type:     EventTrackSkipped,
							Progress: progress,
							Message:  fmt.Sprintf("EXISTS (SKIPPED): %s - Already in target playlist", job.track.Title),
						})
						mu.Unlock()
						continue
					}

					mu.Lock()
					progress.CurrentTrack = &job.track
					s.emit(eventChan, SyncEvent{
						Type:     EventTrackProcessing,
						Progress: progress,
						Message:  fmt.Sprintf("Searching: %s - %s", job.track.PrimaryArtist(), job.track.Title),
					})
					mu.Unlock()

					// Heuristic candidate evaluation
					eval, err := s.matcher.EvaluateTrackHeuristic(ctx, job.track)
					if err != nil {
						eval = &matcher.CandidateEvaluation{
							Track: job.track,
							Result: &domain.MatchResult{
								SourceTrack: job.track,
								Decision:    domain.DecisionSkippedThreshold,
								Reason:      fmt.Sprintf("Matching error: %v", err),
							},
						}
					}

					mu.Lock()
					batchEvals[job.index] = eval
					if eval.Result != nil {
						// Resolved directly without referee (AutoMatch or Disqualified)
						batchResults[job.index] = *eval.Result
						progress.Processed++
						progress.LastResult = eval.Result

						if eval.Result.Decision == domain.DecisionAutoMatch {
							progress.DirectMatches++
							report.DirectMatches++
							s.emit(eventChan, SyncEvent{
								Type:     EventTrackMatched,
								Progress: progress,
								Message:  fmt.Sprintf("AUTO-MATCH (%.1f%%): %s -> %s", eval.Result.Confidence*100, job.track.Title, eval.Result.Candidate.Title),
							})
						} else {
							progress.Skipped++
							report.SkippedTracks++
							s.emit(eventChan, SyncEvent{
								Type:     EventTrackSkipped,
								Progress: progress,
								Message:  fmt.Sprintf("SKIPPED: %s (%s)", job.track.Title, eval.Result.Reason),
							})
						}
					} else if eval.NeedsReferee {
						s.emit(eventChan, SyncEvent{
							Type:     EventTrackProcessing,
							Progress: progress,
							Message:  fmt.Sprintf("AMBIGUOUS: %s (queued for batch AI referee)", job.track.Title),
						})
					}
					mu.Unlock()
				}
			}()
		}

		wg.Wait()

		if ctx.Err() != nil {
			report.Duration = time.Since(startTime)
			report.Results = allResults
			report.ProcessedTracks = len(allResults)
			return report, ctx.Err()
		}

		// Phase 2: Single Batched AI Referee Call for all ambiguous tracks in this batch
		var ambiguousCount int
		for _, ev := range batchEvals {
			if ev != nil && ev.NeedsReferee {
				ambiguousCount++
			}
		}

		if ambiguousCount > 0 {
			s.emit(eventChan, SyncEvent{
				Type:     EventTrackProcessing,
				Progress: progress,
				Message:  fmt.Sprintf("AI REFEREE BATCH: Evaluating %d ambiguous tracks in single LLM prompt...", ambiguousCount),
			})

			_ = s.matcher.ResolveBatch(ctx, batchEvals)

			for i, ev := range batchEvals {
				if ev != nil && ev.NeedsReferee && ev.Result != nil {
					batchResults[i] = *ev.Result
					progress.Processed++
					progress.LastResult = ev.Result

					if ev.Result.Decision == domain.DecisionAIRefereeMatch {
						progress.AIRefereeMatches++
						report.AIRefereeMatches++
						s.emit(eventChan, SyncEvent{
							Type:     EventTrackMatched,
							Progress: progress,
							Message:  fmt.Sprintf("REFEREE MATCH (%.1f%%): %s -> %s", ev.Result.Confidence*100, ev.Track.Title, ev.Result.Candidate.Title),
						})
					} else {
						progress.Skipped++
						report.SkippedTracks++
						s.emit(eventChan, SyncEvent{
							Type:     EventTrackSkipped,
							Progress: progress,
							Message:  fmt.Sprintf("REFEREE SKIPPED: %s (%s)", ev.Track.Title, ev.Result.Reason),
						})
					}
				}
			}
		}

		allResults = append(allResults, batchResults...)

		// Extract newly matched video IDs for this batch in sequence
		var batchVideoIDs []string
		var batchCandidates []domain.Candidate
		for _, res := range batchResults {
			if (res.Decision == domain.DecisionAutoMatch || res.Decision == domain.DecisionAIRefereeMatch) && res.Candidate != nil {
				batchVideoIDs = append(batchVideoIDs, res.Candidate.VideoID)
				batchCandidates = append(batchCandidates, *res.Candidate)
			}
		}

		// Immediately insert this batch of newly matched songs into target playlist (unless dry run)
		if !s.options.DryRun && len(batchVideoIDs) > 0 && s.options.TargetPlaylistID != "" {
			err := s.target.AddTracksToPlaylist(ctx, s.options.TargetPlaylistID, batchVideoIDs)
			if err != nil {
				s.emit(eventChan, SyncEvent{
					Type:     EventError,
					Progress: progress,
					Message:  fmt.Sprintf("Failed to add Batch %d tracks to target playlist: %v", batchNum, err),
				})
			} else {
				progress.Inserted += len(batchVideoIDs)

				// Update in-memory existing cache so subsequent batches skip duplicates
				mu.Lock()
				for _, cand := range batchCandidates {
					existingTracks = append(existingTracks, cand)
					cleanT := strings.ToLower(matcher.CleanTitle(cand.Title))
					if cleanT != "" {
						existingIndex[cleanT] = append(existingIndex[cleanT], cand)
					}
				}
				mu.Unlock()

				s.emit(eventChan, SyncEvent{
					Type:     EventBatchInserted,
					Progress: progress,
					Message:  fmt.Sprintf("Batch %d inserted: %d new tracks added to %s", batchNum, len(batchVideoIDs), s.options.TargetPlaylistID),
				})
			}
		}

		currOffset += len(batchTracks)

		// If returned tracks were fewer than batchLimit, source has reached the end
		if len(batchTracks) < curBatchLimit {
			break
		}
	}

	report.Results = allResults
	report.ProcessedTracks = len(allResults)
	report.Duration = time.Since(startTime)

	s.emit(eventChan, SyncEvent{
		Type:     EventComplete,
		Progress: progress,
		Message:  fmt.Sprintf("Sync complete! Matched: %d, Already in YT: %d, Skipped: %d, Total: %d", progress.DirectMatches+progress.AIRefereeMatches, progress.AlreadyPresent, progress.Skipped, len(allResults)),
	})

	return report, nil

}

func (s *Syncer) emit(ch chan<- SyncEvent, event SyncEvent) {
	if ch != nil {
		ch <- event
	}
}

func buildExistingIndex(existing []domain.Candidate) map[string][]domain.Candidate {
	idx := make(map[string][]domain.Candidate)
	for _, cand := range existing {
		cleanT := strings.ToLower(matcher.CleanTitle(cand.Title))
		if cleanT != "" {
			idx[cleanT] = append(idx[cleanT], cand)
		}
	}
	return idx
}

func checkAlreadyInPlaylist(track domain.Track, index map[string][]domain.Candidate, existingList []domain.Candidate) (bool, string) {
	if len(index) == 0 && len(existingList) == 0 {
		return false, ""
	}
	cleanSrcTitle := strings.ToLower(matcher.CleanTitle(track.Title))
	cleanSrcArtist := strings.ToLower(matcher.CleanArtist(track.PrimaryArtist()))

	// 1. Direct Title match in index
	if candidates, ok := index[cleanSrcTitle]; ok {
		for _, cand := range candidates {
			candArtistsJoined := strings.ToLower(strings.Join(cand.Artists, " ") + " " + matcher.CleanArtist(cand.ChannelTitle))
			if cleanSrcArtist == "" || strings.Contains(candArtistsJoined, cleanSrcArtist) || strings.Contains(cleanSrcArtist, candArtistsJoined) || matcher.TokenSetRatio(cleanSrcArtist, candArtistsJoined) >= 0.70 {
				return true, cand.Title
			}
		}
	}

	// 2. High-similarity title + artist check across all existing
	for _, cand := range existingList {
		cleanExistTitle := strings.ToLower(matcher.CleanTitle(cand.Title))
		candArtistsJoined := strings.ToLower(strings.Join(cand.Artists, " ") + " " + matcher.CleanArtist(cand.ChannelTitle))
		artistMatch := cleanSrcArtist == "" || strings.Contains(candArtistsJoined, cleanSrcArtist) || strings.Contains(cleanSrcArtist, candArtistsJoined) || matcher.TokenSetRatio(cleanSrcArtist, candArtistsJoined) >= 0.75

		if artistMatch {
			if strings.EqualFold(cleanSrcTitle, cleanExistTitle) || matcher.TokenSetRatio(cleanSrcTitle, cleanExistTitle) >= 0.90 {
				return true, cand.Title
			}
		}
	}

	return false, ""
}
