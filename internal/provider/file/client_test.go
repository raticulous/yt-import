package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseExportifyCSV(t *testing.T) {
	csvContent := `Spotify ID,Track URI,Track Name,Artist Name(s),Album Image URL,Album Name,Album Artist Name(s),Album Release Date,Disc Number,Track Number,Track Duration (ms),Explicit,Popularity,Added By,Added At
123,spotify:track:123,Вертайсь росою,Нельсон,http://img,Вертайсь росою,Нельсон,2023,1,1,208000,false,50,user,2024
456,spotify:track:456,Little Talks,Of Monsters and Men,http://img,My Head Is An Animal,Of Monsters and Men,2011,1,1,266000,false,80,user,2024
`
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test_exportify.csv")
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatal(err)
	}

	client := NewClient(csvPath)
	ctx := context.Background()

	pl, err := client.GetPlaylist(ctx, "")
	if err != nil {
		t.Fatalf("GetPlaylist failed: %v", err)
	}
	if pl.TrackCount != 2 {
		t.Errorf("Expected 2 tracks, got %d", pl.TrackCount)
	}

	tracks, total, err := client.GetPlaylistTracks(ctx, "", 0, 100)
	if err != nil {
		t.Fatalf("GetPlaylistTracks failed: %v", err)
	}
	if total != 2 || len(tracks) != 2 {
		t.Fatalf("Expected 2 tracks, got %d (total: %d)", len(tracks), total)
	}

	if tracks[0].Title != "Вертайсь росою" || tracks[0].PrimaryArtist() != "Нельсон" {
		t.Errorf("Unexpected track 0: %+v", tracks[0])
	}
	if tracks[1].Title != "Little Talks" || tracks[1].PrimaryArtist() != "Of Monsters and Men" {
		t.Errorf("Unexpected track 1: %+v", tracks[1])
	}
}

func TestParseTXTFile(t *testing.T) {
	txtContent := `# My favorite songs
Artist 1 - Song 1
Song 2 by Artist 2
`
	tmpDir := t.TempDir()
	txtPath := filepath.Join(tmpDir, "test_songs.txt")
	if err := os.WriteFile(txtPath, []byte(txtContent), 0644); err != nil {
		t.Fatal(err)
	}

	client := NewClient(txtPath)
	ctx := context.Background()

	tracks, total, err := client.GetPlaylistTracks(ctx, "", 0, 10)
	if err != nil {
		t.Fatalf("GetPlaylistTracks failed: %v", err)
	}
	if total != 2 || len(tracks) != 2 {
		t.Fatalf("Expected 2 tracks, got %d", len(tracks))
	}
	if tracks[0].Title != "Song 1" || tracks[0].PrimaryArtist() != "Artist 1" {
		t.Errorf("Track 0 mismatch: %+v", tracks[0])
	}
	if tracks[1].Title != "Song 2" || tracks[1].PrimaryArtist() != "Artist 2" {
		t.Errorf("Track 1 mismatch: %+v", tracks[1])
	}
}
