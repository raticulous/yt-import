package spotify

import (
	"context"
	"testing"
)

func TestPublicClient(t *testing.T) {
	client := NewPublicClient()

	// Test with Today's Top Hits public playlist
	playlistID := "37i9dQZF1DXcBWIGoYBM5M"
	ctx := context.Background()

	p, err := client.GetPlaylist(ctx, playlistID)
	if err != nil {
		t.Fatalf("Failed to get public playlist: %v", err)
	}

	if p.Title == "" {
		t.Fatalf("Expected non-empty playlist title")
	}

	if p.TrackCount == 0 {
		t.Fatalf("Expected track count > 0, got %d", p.TrackCount)
	}

	// Test fetching first 3 tracks with offset/limit
	tracks, total, err := client.GetPlaylistTracks(ctx, playlistID, 0, 3)
	if err != nil {
		t.Fatalf("Failed to fetch public tracks: %v", err)
	}

	if len(tracks) != 3 {
		t.Fatalf("Expected 3 tracks, got %d", len(tracks))
	}

	if tracks[0].Title == "" || len(tracks[0].Artists) == 0 {
		t.Fatalf("Track metadata missing: %+v", tracks[0])
	}

	t.Logf("Successfully fetched %d tracks (total: %d). First track: '%s' by %v (%ds)", len(tracks), total, tracks[0].Title, tracks[0].Artists, tracks[0].DurationMs/1000)
}
