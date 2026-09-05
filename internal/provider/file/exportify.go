package file

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// OpenExportify opens https://exportify.net in the user's default web browser.
func OpenExportify() error {
	url := "https://exportify.net"
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// WatchDownloadsForExportify watches the user's Downloads directory for a newly created or modified Spotify CSV.
func WatchDownloadsForExportify(ctx context.Context, onStatus func(string)) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to locate user home directory: %w", err)
	}

	downloadsDir := filepath.Join(home, "Downloads")
	if _, err := os.Stat(downloadsDir); os.IsNotExist(err) {
		return "", fmt.Errorf("downloads directory not found at: %s", downloadsDir)
	}

	// Record existing files snapshot
	startTime := time.Now().Add(-2 * time.Minute) // Also consider files downloaded within last 2 minutes
	existingFiles := make(map[string]time.Time)
	entries, _ := os.ReadDir(downloadsDir)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".csv") {
			info, err := entry.Info()
			if err == nil {
				existingFiles[entry.Name()] = info.ModTime()
			}
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	if onStatus != nil {
		onStatus(fmt.Sprintf("Watching %s for newly exported playlist CSV...", downloadsDir))
	}

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			currentEntries, err := os.ReadDir(downloadsDir)
			if err != nil {
				continue
			}

			var newestFile string
			var newestTime time.Time

			for _, entry := range currentEntries {
				if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".csv") {
					continue
				}
				info, err := entry.Info()
				if err != nil {
					continue
				}

				// Check if file is new or modified after start time
				modTime := info.ModTime()
				prevTime, existed := existingFiles[entry.Name()]
				isNewOrUpdated := (!existed && modTime.After(startTime)) || (existed && modTime.After(prevTime))

				if isNewOrUpdated {
					fullPath := filepath.Join(downloadsDir, entry.Name())
					// Verify file is a valid Spotify export CSV
					if isValidSpotifyCSV(fullPath) {
						if modTime.After(newestTime) {
							newestTime = modTime
							newestFile = fullPath
						}
					}
				}
			}

			if newestFile != "" {
				return newestFile, nil
			}
		}
	}
}

// isValidSpotifyCSV checks if the CSV file contains standard Spotify / playlist columns
func isValidSpotifyCSV(filePath string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	reader := csv.NewReader(f)
	header, err := reader.Read()
	if err != nil {
		return false
	}

	hasTrack := false
	hasArtist := false
	for _, col := range header {
		clean := strings.ToLower(strings.TrimSpace(col))
		if strings.Contains(clean, "track") || strings.Contains(clean, "song") || strings.Contains(clean, "title") {
			hasTrack = true
		}
		if strings.Contains(clean, "artist") {
			hasArtist = true
		}
	}

	if !hasTrack && !hasArtist {
		return false
	}

	// Make sure it has at least one row
	_, err = reader.Read()
	return err == nil || err != io.EOF
}
