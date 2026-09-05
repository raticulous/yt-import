package file

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsValidSpotifyCSV(t *testing.T) {
	tempDir := t.TempDir()
	validCSV := filepath.Join(tempDir, "valid.csv")
	invalidCSV := filepath.Join(tempDir, "invalid.csv")

	_ = os.WriteFile(validCSV, []byte("Spotify ID,Track Name,Artist Name(s),Album Name\n123,Song A,Artist B,Album C\n"), 0644)
	_ = os.WriteFile(invalidCSV, []byte("ID,RandomData,Price\n1,2,3\n"), 0644)

	if !isValidSpotifyCSV(validCSV) {
		t.Errorf("expected validCSV to be valid")
	}
	if isValidSpotifyCSV(invalidCSV) {
		t.Errorf("expected invalidCSV to be invalid")
	}
}
