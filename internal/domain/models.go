package domain

import "fmt"

// Track represents a music track in the system.
type Track struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Artists     []string `json:"artists"`
	Album       string   `json:"album,omitempty"`
	DurationMs  int      `json:"duration_ms"`
	ISRC        string   `json:"isrc,omitempty"`
	Explicit    bool     `json:"explicit,omitempty"`
	ReleaseYear int      `json:"release_year,omitempty"`
}

// FormattedDuration returns mm:ss representation of the track duration.
func (t Track) FormattedDuration() string {
	seconds := t.DurationMs / 1000
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

// PrimaryArtist returns the first listed artist or empty string.
func (t Track) PrimaryArtist() string {
	if len(t.Artists) > 0 {
		return t.Artists[0]
	}
	return ""
}

// Playlist represents a collection of tracks.
type Playlist struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	TrackCount  int     `json:"track_count"`
	Tracks      []Track `json:"tracks,omitempty"`
}

// CandidateVideoType enumerates YouTube Music video classifications.
type CandidateVideoType string

const (
	TypeAudioTrackVideo CandidateVideoType = "ATV" // Official label audio track
	TypeOfficialMusicVideo CandidateVideoType = "OMV" // Official music video
	TypeUserGenerated   CandidateVideoType = "UGC" // Community/cover upload
	TypeUnknown         CandidateVideoType = "UNKNOWN"
)

// Candidate represents a candidate song found on the target platform.
type Candidate struct {
	VideoID       string             `json:"video_id"`
	Title         string             `json:"title"`
	Artists       []string           `json:"artists"`
	Album         string             `json:"album,omitempty"`
	DurationMs    int                `json:"duration_ms"`
	VideoType     CandidateVideoType `json:"video_type"`
	ChannelTitle  string             `json:"channel_title,omitempty"`
	Score         float64            `json:"score"`
	TitleScore    float64            `json:"title_score"`
	ArtistScore   float64            `json:"artist_score"`
	DurationScore float64            `json:"duration_score"`
	TypeScore     float64            `json:"type_score"`
}

// FormattedDuration returns mm:ss representation of the candidate duration.
func (c Candidate) FormattedDuration() string {
	seconds := c.DurationMs / 1000
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

// DecisionType categorizes how a match decision was made.
type DecisionType string

const (
	DecisionAutoMatch        DecisionType = "AUTO_MATCH"
	DecisionAIRefereeMatch   DecisionType = "AI_REFEREE_MATCH"
	DecisionSkippedThreshold DecisionType = "SKIPPED_BELOW_THRESHOLD"
	DecisionDisqualified     DecisionType = "DISQUALIFIED"
	DecisionNoCandidates     DecisionType = "NO_CANDIDATES_FOUND"
	DecisionAlreadyExists    DecisionType = "ALREADY_EXISTS"
)

// MatchResult stores the final outcome for a source track evaluation.
type MatchResult struct {
	SourceTrack    Track        `json:"source_track"`
	Candidate      *Candidate   `json:"candidate,omitempty"`
	AllCandidates  []Candidate  `json:"all_candidates,omitempty"`
	Confidence     float64      `json:"confidence"`
	Decision       DecisionType `json:"decision"`
	Reason         string       `json:"reason"`
	InsertedToYTM  bool         `json:"inserted_to_ytm"`
}

// RefereeVerdict represents the structured output from an AI Referee.
type RefereeVerdict struct {
	ItemID       int     `json:"item_id,omitempty"`
	Verdict      string  `json:"verdict"` // "MATCH" or "NO_MATCH"
	MatchedIndex int     `json:"matched_index"`
	Confidence   float64 `json:"confidence"`
	Reasoning    string  `json:"reasoning"`
}

// SyncOptions regulates the execution flow of the import.
type SyncOptions struct {
	SourcePlaylistID  string  `json:"source_playlist_id"`
	TargetPlaylistID  string  `json:"target_playlist_id"`
	TargetTitle       string  `json:"target_title,omitempty"`
	Offset            int     `json:"offset"`
	Limit             int     `json:"limit"`
	Threshold         float64 `json:"threshold"` // Default: 0.95
	AIRefereeEnabled  bool    `json:"ai_referee_enabled"`
	AIRefereeProvider string  `json:"ai_referee_provider"`
	DryRun            bool    `json:"dry_run"`
	Concurrency       int     `json:"concurrency"` // Default: 5
}
