package spotify

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/zmb3/spotify/v2"
	"yt-import/internal/domain"
)

// Client implements provider.SourceProvider for Spotify.
type Client struct {
	client *spotify.Client
}

// NewClient wraps an authenticated HTTP client in a Spotify client.
func NewClient(httpClient *http.Client) *Client {
	return &Client{
		client: spotify.New(httpClient),
	}
}

func (c *Client) Name() string {
	return "Spotify"
}

// GetPlaylist retrieves playlist details and total track count.
func (c *Client) GetPlaylist(ctx context.Context, playlistID string) (*domain.Playlist, error) {
	cleanID := ExtractPlaylistID(playlistID)
	p, err := c.client.GetPlaylist(ctx, spotify.ID(cleanID))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spotify playlist %s: %w", cleanID, err)
	}

	return &domain.Playlist{
		ID:          string(p.ID),
		Title:       p.Name,
		Description: p.Description,
		TrackCount:  int(p.Tracks.Total),
	}, nil
}

// GetPlaylistTracks retrieves a slice of tracks adhering to offset and limit.
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistID string, offset int, limit int) ([]domain.Track, int, error) {
	cleanID := ExtractPlaylistID(playlistID)

	// First fetch playlist summary for total count
	summary, err := c.GetPlaylist(ctx, cleanID)
	if err != nil {
		return nil, 0, err
	}
	totalTracks := summary.TrackCount

	if offset < 0 {
		offset = 0
	}
	if offset >= totalTracks {
		return nil, totalTracks, nil
	}

	targetCount := totalTracks - offset
	if limit > 0 && limit < targetCount {
		targetCount = limit
	}

	var tracks []domain.Track
	currOffset := offset

	for len(tracks) < targetCount {
		select {
		case <-ctx.Done():
			return nil, totalTracks, ctx.Err()
		default:
		}

		batchLimit := min(targetCount-len(tracks), 100)

		page, err := c.client.GetPlaylistItems(
			ctx,
			spotify.ID(cleanID),
			spotify.Offset(currOffset),
			spotify.Limit(batchLimit),
		)
		if err != nil {
			return nil, totalTracks, fmt.Errorf("failed to fetch playlist items at offset %d: %w", currOffset, err)
		}

		for _, item := range page.Items {
			if item.Track.Track == nil {
				continue // Skip podcast episodes or unavailable tracks
			}

			fullTrack := item.Track.Track
			var artists []string
			for _, a := range fullTrack.Artists {
				artists = append(artists, a.Name)
			}

			year := 0
			if len(fullTrack.Album.ReleaseDate) >= 4 {
				if y, err := strconv.Atoi(fullTrack.Album.ReleaseDate[:4]); err == nil {
					year = y
				}
			}

			isrc := ""
			if fullTrack.ExternalIDs != nil {
				isrc = fullTrack.ExternalIDs["isrc"]
			}

			tracks = append(tracks, domain.Track{
				ID:          string(fullTrack.ID),
				Title:       strings.TrimSpace(fullTrack.Name),
				Artists:     artists,
				Album:       strings.TrimSpace(fullTrack.Album.Name),
				DurationMs:  int(fullTrack.Duration),
				ISRC:        isrc,
				Explicit:    fullTrack.Explicit,
				ReleaseYear: year,
			})
		}

		currOffset += len(page.Items)
		if len(page.Items) == 0 || currOffset >= totalTracks {
			break
		}
	}

	return tracks, totalTracks, nil
}
