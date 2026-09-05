package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"yt-import/internal/domain"
)

var nextDataRegex = regexp.MustCompile(`<script id="__NEXT_DATA__" type="application/json">([^<]+)</script>`)

// PublicClient fetches tracks from public Spotify playlists without any API credentials or login.
type PublicClient struct {
	httpClient *http.Client
}

// NewPublicClient creates a zero-credential public Spotify playlist client.
func NewPublicClient() *PublicClient {
	return &PublicClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *PublicClient) Name() string {
	return "Spotify (Public / Zero-Creds)"
}

type embedData struct {
	Props struct {
		PageProps struct {
			State struct {
				Data struct {
					Entity struct {
						Name      string `json:"name"`
						Title     string `json:"title"`
						TrackList []struct {
							URI        string `json:"uri"`
							Title      string `json:"title"`
							Subtitle   string `json:"subtitle"`
							Duration   int    `json:"duration"`
							IsExplicit bool   `json:"isExplicit"`
						} `json:"trackList"`
					} `json:"entity"`
				} `json:"data"`
			} `json:"state"`
		} `json:"pageProps"`
	} `json:"props"`
}

func (p *PublicClient) fetchEmbedData(ctx context.Context, playlistID string) (*embedData, error) {
	cleanID := ExtractPlaylistID(playlistID)
	url := fmt.Sprintf("https://open.spotify.com/embed/playlist/%s", cleanID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach spotify embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify embed returned status %d (check if playlist is public)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	matches := nextDataRegex.FindSubmatch(body)
	if len(matches) < 2 {
		return nil, fmt.Errorf("could not parse public spotify playlist data (ensure playlist is public)")
	}

	var data embedData
	if err := json.Unmarshal(matches[1], &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal public playlist JSON: %w", err)
	}

	return &data, nil
}

// GetPlaylist retrieves playlist title and track count without credentials.
func (p *PublicClient) GetPlaylist(ctx context.Context, playlistID string) (*domain.Playlist, error) {
	data, err := p.fetchEmbedData(ctx, playlistID)
	if err != nil {
		return nil, err
	}

	entity := data.Props.PageProps.State.Data.Entity
	name := entity.Name
	if name == "" {
		name = entity.Title
	}
	if name == "" {
		name = "Public Spotify Playlist"
	}

	return &domain.Playlist{
		ID:          ExtractPlaylistID(playlistID),
		Title:       name,
		Description: "Imported via public web parser",
		TrackCount:  len(entity.TrackList),
	}, nil
}

// GetPlaylistTracks parses tracks from the public playlist and applies offset/limit slicing.
func (p *PublicClient) GetPlaylistTracks(ctx context.Context, playlistID string, offset int, limit int) ([]domain.Track, int, error) {
	data, err := p.fetchEmbedData(ctx, playlistID)
	if err != nil {
		return nil, 0, err
	}

	entity := data.Props.PageProps.State.Data.Entity
	rawTracks := entity.TrackList
	total := len(rawTracks)

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

	sliced := rawTracks[offset:end]
	var tracks []domain.Track

	for _, item := range sliced {
		trackID := strings.TrimPrefix(item.URI, "spotify:track:")

		// Parse artist names (split comma separated if multiple)
		var artists []string
		for _, a := range strings.Split(item.Subtitle, ",") {
			trimmed := strings.TrimSpace(a)
			if trimmed != "" {
				artists = append(artists, trimmed)
			}
		}
		if len(artists) == 0 {
			artists = []string{item.Subtitle}
		}

		tracks = append(tracks, domain.Track{
			ID:         trackID,
			Title:      item.Title,
			Artists:    artists,
			DurationMs: item.Duration,
			Explicit:   item.IsExplicit,
		})
	}

	return tracks, total, nil
}
