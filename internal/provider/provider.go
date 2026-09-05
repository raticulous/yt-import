package provider

import (
	"context"

	"yt-import/internal/domain"
)

// SourceProvider defines the interface for music platforms serving as track sources (e.g. Spotify).
type SourceProvider interface {
	Name() string
	GetPlaylist(ctx context.Context, playlistID string) (*domain.Playlist, error)
	GetPlaylistTracks(ctx context.Context, playlistID string, offset int, limit int) (tracks []domain.Track, total int, err error)
}

// TargetProvider defines the interface for music platforms receiving imported tracks (e.g. YouTube Music).
type TargetProvider interface {
	Name() string
	Search(ctx context.Context, query string) ([]domain.Candidate, error)
	GetPlaylist(ctx context.Context, playlistID string) (*domain.Playlist, error)
	GetPlaylistTracks(ctx context.Context, playlistID string) ([]domain.Candidate, error)
	CreatePlaylist(ctx context.Context, title string, description string) (playlistID string, err error)
	AddTracksToPlaylist(ctx context.Context, playlistID string, videoIDs []string) error
}
