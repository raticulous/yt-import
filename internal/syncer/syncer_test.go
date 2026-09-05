package syncer

import (
	"context"
	"fmt"
	"testing"

	"yt-import/internal/domain"
	"yt-import/internal/matcher"
	"yt-import/internal/referee"
)

type mockSourceProvider struct {
	tracks []domain.Track
}

func (m *mockSourceProvider) Name() string { return "MockSource" }
func (m *mockSourceProvider) GetPlaylist(ctx context.Context, playlistID string) (*domain.Playlist, error) {
	return &domain.Playlist{ID: playlistID, Title: "Mock Playlist", TrackCount: len(m.tracks)}, nil
}
func (m *mockSourceProvider) GetPlaylistTracks(ctx context.Context, playlistID string, offset, limit int) ([]domain.Track, int, error) {
	total := len(m.tracks)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return nil, total, nil
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return m.tracks[offset:end], total, nil
}


type mockTargetProvider struct {
	inserted []string
}

func (m *mockTargetProvider) Name() string { return "MockTarget" }
func (m *mockTargetProvider) Search(ctx context.Context, query string) ([]domain.Candidate, error) {
	return []domain.Candidate{
		{
			VideoID:    "vid_" + query,
			Title:      query,
			Artists:    []string{"Artist"},
			DurationMs: 180000,
			VideoType:  domain.TypeAudioTrackVideo,
		},
	}, nil
}
func (m *mockTargetProvider) GetPlaylist(ctx context.Context, playlistID string) (*domain.Playlist, error) {
	return &domain.Playlist{ID: playlistID, Title: "Target"}, nil
}
func (m *mockTargetProvider) GetPlaylistTracks(ctx context.Context, playlistID string) ([]domain.Candidate, error) {
	return nil, nil
}
func (m *mockTargetProvider) CreatePlaylist(ctx context.Context, title, desc string) (string, error) {
	return "created_pl", nil
}
func (m *mockTargetProvider) AddTracksToPlaylist(ctx context.Context, playlistID string, videoIDs []string) error {
	m.inserted = append(m.inserted, videoIDs...)
	return nil
}

func TestConcurrentSyncerOrderAndCompletion(t *testing.T) {
	trackCount := 20
	var tracks []domain.Track
	for i := 0; i < trackCount; i++ {
		tracks = append(tracks, domain.Track{
			ID:         fmt.Sprintf("t_%d", i),
			Title:      fmt.Sprintf("Song_%d", i),
			Artists:    []string{"Artist"},
			DurationMs: 180000,
		})
	}

	src := &mockSourceProvider{tracks: tracks}
	target := &mockTargetProvider{}
	engine := matcher.NewEngine(target, referee.NewMockReferee(), 0.95)

	opts := domain.SyncOptions{
		SourcePlaylistID: "src_pl",
		TargetPlaylistID: "target_pl",
		Concurrency:      8, // 8 concurrent workers
		Threshold:        0.95,
		DryRun:           false,
	}

	s := NewSyncer(src, target, engine, opts)
	eventChan := make(chan SyncEvent, 100)
	go func() {
		for range eventChan {
		}
	}()

	report, err := s.Run(context.Background(), eventChan)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if report.ProcessedTracks != trackCount {
		t.Errorf("Expected %d processed tracks, got %d", trackCount, report.ProcessedTracks)
	}

	if len(report.Results) != trackCount {
		t.Fatalf("Expected %d results, got %d", trackCount, len(report.Results))
	}

	// Verify exact sequence preservation
	for i := 0; i < trackCount; i++ {
		expectedTitle := fmt.Sprintf("Song_%d", i)
		if report.Results[i].SourceTrack.Title != expectedTitle {
			t.Errorf("Result at index %d has title %s, want %s", i, report.Results[i].SourceTrack.Title, expectedTitle)
		}
	}

	if len(target.inserted) != trackCount {
		t.Errorf("Expected %d inserted tracks, got %d", trackCount, len(target.inserted))
	}
}

type mockTargetWithExisting struct {
	mockTargetProvider
	existing []domain.Candidate
}

func (m *mockTargetWithExisting) GetPlaylistTracks(ctx context.Context, playlistID string) ([]domain.Candidate, error) {
	return m.existing, nil
}

func TestFastSkipExistingTracks(t *testing.T) {
	source := &mockSourceProvider{
		tracks: []domain.Track{
			{ID: "1", Title: "Already Here", Artists: []string{"Artist A"}, DurationMs: 180000},
			{ID: "2", Title: "Brand New Song", Artists: []string{"Artist B"}, DurationMs: 180000},
		},
	}
	target := &mockTargetWithExisting{
		existing: []domain.Candidate{
			{Title: "Already Here", Artists: []string{"Artist A"}, VideoID: "existing_vid_1"},
		},
	}

	engine := matcher.NewEngine(target, referee.NewMockReferee(), 0.95)
	opts := domain.SyncOptions{
		SourcePlaylistID: "src_pl",
		TargetPlaylistID: "tgt_pl",
		Threshold:        0.95,
		Concurrency:      2,
	}

	s := NewSyncer(source, target, engine, opts)
	eventChan := make(chan SyncEvent, 10)
	go func() {
		for range eventChan {
		}
	}()

	report, err := s.Run(context.Background(), eventChan)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if report.AlreadyPresentTracks != 1 {
		t.Errorf("Expected 1 already present track, got %d", report.AlreadyPresentTracks)
	}
	if report.Results[0].Decision != domain.DecisionAlreadyExists {
		t.Errorf("Expected track 0 to have DecisionAlreadyExists, got %s", report.Results[0].Decision)
	}
	if len(target.inserted) != 1 {
		t.Errorf("Expected only 1 new song inserted, got %d", len(target.inserted))
	}
}

func TestMultiBatchSyncing250Tracks(t *testing.T) {
	trackCount := 250
	var tracks []domain.Track
	for i := 0; i < trackCount; i++ {
		tracks = append(tracks, domain.Track{
			ID:         fmt.Sprintf("track_%03d", i),
			Title:      fmt.Sprintf("Song %03d", i),
			Artists:    []string{"Artist"},
			DurationMs: 200000,
		})
	}

	src := &mockSourceProvider{tracks: tracks}
	target := &mockTargetProvider{}
	engine := matcher.NewEngine(target, referee.NewMockReferee(), 0.95)

	opts := domain.SyncOptions{
		SourcePlaylistID: "large_playlist",
		TargetPlaylistID: "ytm_target",
		Concurrency:      10,
		Threshold:        0.95,
		DryRun:           false,
	}

	s := NewSyncer(src, target, engine, opts)
	eventChan := make(chan SyncEvent, 100)

	var batchSavedEvents []SyncEvent
	done := make(chan struct{})
	go func() {
		for ev := range eventChan {
			if ev.Type == EventBatchInserted {
				batchSavedEvents = append(batchSavedEvents, ev)
			}
		}
		close(done)
	}()

	report, err := s.Run(context.Background(), eventChan)
	close(eventChan)
	<-done

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if report.ProcessedTracks != 250 {
		t.Errorf("Expected 250 processed tracks, got %d", report.ProcessedTracks)
	}

	if len(target.inserted) != 250 {
		t.Errorf("Expected 250 tracks inserted into target, got %d", len(target.inserted))
	}

	// 250 tracks with batchSize=100 should yield 3 batch insertion events (100, 100, 50)
	if len(batchSavedEvents) != 3 {
		t.Errorf("Expected 3 EventBatchInserted events, got %d", len(batchSavedEvents))
	}

	// Check final batch sequence
	for i := 0; i < 250; i++ {
		expectedTitle := fmt.Sprintf("Song %03d", i)
		if report.Results[i].SourceTrack.Title != expectedTitle {
			t.Fatalf("Track %d out of sequence: got %s, expected %s", i, report.Results[i].SourceTrack.Title, expectedTitle)
		}
	}
}

func TestBatchRefereeDisambiguation(t *testing.T) {
	trackCount := 15
	var tracks []domain.Track
	for i := 0; i < trackCount; i++ {
		tracks = append(tracks, domain.Track{
			ID:         fmt.Sprintf("ambig_%02d", i),
			Title:      fmt.Sprintf("Song %02d", i),
			Artists:    []string{"Artist"},
			DurationMs: 200000,
		})
	}

	src := &mockSourceProvider{tracks: tracks}
	target := &mockTargetProvider{}
	ref := referee.NewMockReferee()
	ref.ForceMatch = true
	ref.Confidence = 0.98
	engine := matcher.NewEngine(target, ref, 0.95)

	opts := domain.SyncOptions{
		SourcePlaylistID: "ambig_playlist",
		TargetPlaylistID: "ytm_target",
		Concurrency:      5,
		Threshold:        0.95,
		DryRun:           false,
	}

	s := NewSyncer(src, target, engine, opts)
	eventChan := make(chan SyncEvent, 50)
	go func() {
		for range eventChan {
		}
	}()

	report, err := s.Run(context.Background(), eventChan)
	close(eventChan)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if report.ProcessedTracks != 15 {
		t.Errorf("Expected 15 processed tracks, got %d", report.ProcessedTracks)
	}

	// Crucial check: All 15 ambiguous tracks in this 15-track batch should trigger exactly 1 batch call!
	if ref.BatchCallCount != 1 {
		t.Errorf("Expected exactly 1 batched referee call for the entire batch, got %d", ref.BatchCallCount)
	}

	if report.AIRefereeMatches != 15 {
		t.Errorf("Expected 15 referee matches, got %d", report.AIRefereeMatches)
	}
}


