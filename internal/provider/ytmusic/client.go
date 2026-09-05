package ytmusic

import (
	"context"
	"strings"

	"yt-import/internal/domain"
)

// Client implements provider.TargetProvider and matcher.Searcher for YouTube Music.
type Client struct {
	innerTube *InnerTubeClient
}

// NewClient creates a new YouTube Music provider client.
func NewClient(cookie string) *Client {
	return &Client{
		innerTube: NewInnerTubeClient(cookie),
	}
}

func (c *Client) Name() string {
	return "YouTube Music"
}

// ExtractPlaylistID extracts clean playlist ID from YouTube Music URLs or raw IDs.
func ExtractPlaylistID(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	// If it contains "watch?v=" without "list=", it's a single video link, not a playlist!
	if strings.Contains(input, "watch?") && !strings.Contains(input, "list=") {
		return ""
	}
	if strings.Contains(input, "list=") {
		parts := strings.Split(input, "list=")
		if len(parts) > 1 {
			id := parts[1]
			if idx := strings.IndexAny(id, "&?/#"); idx != -1 {
				id = id[:idx]
			}
			return strings.TrimPrefix(id, "VL")
		}
	}
	return strings.TrimPrefix(input, "VL")
}

// Search searches for tracks on YouTube Music.
func (c *Client) Search(ctx context.Context, query string) ([]domain.Candidate, error) {
	return c.innerTube.Search(ctx, query)
}

// GetPlaylist retrieves playlist summary.
func (c *Client) GetPlaylist(ctx context.Context, playlistID string) (*domain.Playlist, error) {
	cleanID := ExtractPlaylistID(playlistID)
	// Return a stub playlist container for targeting
	return &domain.Playlist{
		ID:    cleanID,
		Title: "YouTube Music Playlist",
	}, nil
}

// GetPlaylistTracks retrieves existing tracks from a YouTube Music playlist.
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistID string) ([]domain.Candidate, error) {
	cleanID := ExtractPlaylistID(playlistID)
	return c.innerTube.GetPlaylistTracks(ctx, cleanID)
}

// CreatePlaylist creates a new playlist on YouTube Music.
func (c *Client) CreatePlaylist(ctx context.Context, title string, description string) (string, error) {
	return c.innerTube.CreatePlaylist(ctx, title, description)
}

// AddTracksToPlaylist appends track video IDs to the target playlist.
func (c *Client) AddTracksToPlaylist(ctx context.Context, playlistID string, videoIDs []string) error {
	cleanID := ExtractPlaylistID(playlistID)
	return c.innerTube.AddTracksToPlaylist(ctx, cleanID, videoIDs)
}

// ValidateAuth checks if the configured YouTube Music session cookie is active and valid.
func (c *Client) ValidateAuth(ctx context.Context) (string, error) {
	return c.innerTube.ValidateAuth(ctx)
}

