package file

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"yt-import/internal/domain"
)

// Client implements provider.SourceProvider for local CSV or text track files.
type Client struct {
	filePath string
	tracks   []domain.Track
	loaded   bool
}

// NewClient creates a new file-based SourceProvider.
func NewClient(filePath string) *Client {
	return &Client{
		filePath: filePath,
	}
}

func (c *Client) Name() string {
	return fmt.Sprintf("File (%s)", filepath.Base(c.filePath))
}

func (c *Client) loadTracks() error {
	if c.loaded {
		return nil
	}

	file, err := os.Open(c.filePath)
	if err != nil {
		return fmt.Errorf("failed to open track file: %w", err)
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(c.filePath))
	if ext == ".csv" {
		tracks, err := parseCSV(file)
		if err != nil {
			return err
		}
		c.tracks = tracks
	} else {
		tracks, err := parseTXT(file)
		if err != nil {
			return err
		}
		c.tracks = tracks
	}

	c.loaded = true
	return nil
}

// GetPlaylist retrieves the file playlist summary.
func (c *Client) GetPlaylist(ctx context.Context, playlistID string) (*domain.Playlist, error) {
	if err := c.loadTracks(); err != nil {
		return nil, err
	}

	name := strings.TrimSuffix(filepath.Base(c.filePath), filepath.Ext(c.filePath))
	return &domain.Playlist{
		ID:          c.filePath,
		Title:       name,
		Description: fmt.Sprintf("Imported from %s", filepath.Base(c.filePath)),
		TrackCount:  len(c.tracks),
	}, nil
}

// GetPlaylistTracks retrieves a slice of tracks adhering to offset and limit.
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistID string, offset int, limit int) ([]domain.Track, int, error) {
	if err := c.loadTracks(); err != nil {
		return nil, 0, err
	}

	total := len(c.tracks)
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

	return c.tracks[offset:end], total, nil
}

func parseCSV(r io.Reader) ([]domain.Track, error) {
	reader := csv.NewReader(r)
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("CSV file is empty")
	}

	header := records[0]
	titleIdx := -1
	artistIdx := -1
	albumIdx := -1
	durationIdx := -1
	isrcIdx := -1

	for i, col := range header {
		clean := strings.ToLower(strings.TrimSpace(col))
		clean = strings.ReplaceAll(clean, "_", " ")
		switch {
		case clean == "track name" || clean == "title" || clean == "track" || clean == "song":
			titleIdx = i
		case clean == "artist name(s)" || clean == "artist name" || clean == "artists" || clean == "artist":
			artistIdx = i
		case clean == "album name" || clean == "album":
			albumIdx = i
		case strings.Contains(clean, "duration"):
			durationIdx = i
		case clean == "isrc":
			isrcIdx = i
		}
	}

	if titleIdx == -1 || artistIdx == -1 {
		// Fallback: try assuming column 0 is Title and column 1 is Artist, or vice versa
		if len(header) >= 2 {
			titleIdx = 0
			artistIdx = 1
		} else {
			return nil, fmt.Errorf("could not identify Title and Artist columns in CSV header: %v", header)
		}
	}

	var tracks []domain.Track
	for rowIdx, row := range records[1:] {
		if titleIdx >= len(row) || artistIdx >= len(row) {
			continue
		}

		title := strings.TrimSpace(row[titleIdx])
		artistStr := strings.TrimSpace(row[artistIdx])
		if title == "" && artistStr == "" {
			continue
		}

		var artists []string
		for _, a := range strings.Split(artistStr, ",") {
			trimmed := strings.TrimSpace(a)
			if trimmed != "" {
				artists = append(artists, trimmed)
			}
		}
		if len(artists) == 0 {
			artists = []string{artistStr}
		}

		album := ""
		if albumIdx != -1 && albumIdx < len(row) {
			album = strings.TrimSpace(row[albumIdx])
		}

		durationMs := 0
		if durationIdx != -1 && durationIdx < len(row) {
			dStr := strings.TrimSpace(row[durationIdx])
			if d, err := strconv.Atoi(dStr); err == nil {
				durationMs = d
			}
		}

		isrc := ""
		if isrcIdx != -1 && isrcIdx < len(row) {
			isrc = strings.TrimSpace(row[isrcIdx])
		}

		tracks = append(tracks, domain.Track{
			ID:         fmt.Sprintf("file_%d", rowIdx+1),
			Title:      title,
			Artists:    artists,
			Album:      album,
			DurationMs: durationMs,
			ISRC:       isrc,
		})
	}

	return tracks, nil
}

func parseTXT(r io.Reader) ([]domain.Track, error) {
	scanner := bufio.NewScanner(r)
	var tracks []domain.Track
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Look for common separators: " - " or " by "
		var artist, title string
		if strings.Contains(line, " - ") {
			parts := strings.SplitN(line, " - ", 2)
			artist = strings.TrimSpace(parts[0])
			title = strings.TrimSpace(parts[1])
		} else if strings.Contains(line, " by ") {
			parts := strings.SplitN(line, " by ", 2)
			title = strings.TrimSpace(parts[0])
			artist = strings.TrimSpace(parts[1])
		} else {
			// Just title
			title = line
			artist = "Unknown"
		}

		tracks = append(tracks, domain.Track{
			ID:      fmt.Sprintf("txt_%d", lineNum),
			Title:   title,
			Artists: []string{artist},
		})
	}

	return tracks, scanner.Err()
}
