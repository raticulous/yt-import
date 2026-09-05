package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// Config holds all credentials and options for yt-import.
type Config struct {
	// Spotify Credentials
	SpotifyClientID     string `json:"spotify_client_id,omitempty"`
	SpotifyClientSecret string `json:"spotify_client_secret,omitempty"`
	SpotifyAccessToken  string `json:"spotify_access_token,omitempty"`

	// YouTube Music Credentials
	YTMCookie           string `json:"ytm_cookie,omitempty"`
	YouTubeClientID     string `json:"youtube_client_id,omitempty"`
	YouTubeClientSecret string `json:"youtube_client_secret,omitempty"`

	// AI Referee Options
	AIProvider      string `json:"ai_provider,omitempty"` // "antigravity", "gemini", "openai", "claude", "ollama", "mock"
	AntigravityPath string `json:"antigravity_path,omitempty"` // Default: "agy" (auto-detected in PATH)
	GeminiAPIKey    string `json:"gemini_api_key,omitempty"`
	OpenAIAPIKey    string `json:"openai_api_key,omitempty"`
	AnthropicAPIKey string `json:"anthropic_api_key,omitempty"`
	OllamaEndpoint  string `json:"ollama_endpoint,omitempty"` // Default: http://localhost:11434
	OllamaModel     string `json:"ollama_model,omitempty"`    // Default: llama3

	// Matching Options
	StrictThreshold float64 `json:"strict_threshold"` // Default: 0.95
	Concurrency     int     `json:"concurrency"`      // Default: 5

	// Last Session Choices & Inputs (for instant retry and smart defaults)
	LastSourceMethod    string  `json:"last_source_method,omitempty"`    // "exportify", "spotify_url", "file_path"
	LastSourcePlaylist  string  `json:"last_source_playlist,omitempty"`  // Raw input or detected path
	LastSpotifyURL      string  `json:"last_spotify_url,omitempty"`      // Last entered Spotify URL/ID
	LastFilePath        string  `json:"last_file_path,omitempty"`        // Last entered local CSV/TXT path
	LastTargetPlaylist  string  `json:"last_target_playlist,omitempty"`  // Last entered target YouTube Music playlist URL/ID
	LastOffset          int     `json:"last_offset,omitempty"`           // Last offset (e.g. 0)
	LastLimit           int     `json:"last_limit,omitempty"`            // Last limit (e.g. 0)
	LastStrictThreshold float64 `json:"last_strict_threshold,omitempty"` // Last strict threshold (e.g. 0.95)
	LastEnableAI        *bool   `json:"last_enable_ai,omitempty"`        // Last AI enable choice
	LastAIProvider      string  `json:"last_ai_provider,omitempty"`      // Last AI provider
	LastDryRun          bool    `json:"last_dry_run,omitempty"`          // Last dry-run choice
}

// HasLastSession reports whether previous session parameters exist for retry.
func (c *Config) HasLastSession() bool {
	return c.LastSourcePlaylist != "" || c.LastFilePath != "" || c.LastSpotifyURL != ""
}

// ClearLastSession resets all stored last session choices and inputs.
func (c *Config) ClearLastSession() {
	c.LastSourceMethod = ""
	c.LastSourcePlaylist = ""
	c.LastSpotifyURL = ""
	c.LastFilePath = ""
	c.LastTargetPlaylist = ""
	c.LastOffset = 0
	c.LastLimit = 0
	c.LastStrictThreshold = 0
	c.LastEnableAI = nil
	c.LastAIProvider = ""
	c.LastDryRun = false
}

// DefaultConfig returns reasonable defaults.
func DefaultConfig() *Config {
	return &Config{
		AIProvider:      "antigravity",
		AntigravityPath: "agy",
		OllamaEndpoint:  "http://localhost:11434",
		OllamaModel:     "llama3",
		StrictThreshold: 0.95,
		Concurrency:     5,
	}
}

// ConfigFilePath returns the standard location for yt-import config.
func ConfigFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "yt-import.json", nil
	}
	dir := filepath.Join(home, ".config", "yt-import")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "yt-import.json", nil
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads config from standard file and env variables.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	path, err := ConfigFilePath()
	if err == nil {
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, cfg)
		}
	}

	// Environment variable overrides
	if val := os.Getenv("SPOTIFY_CLIENT_ID"); val != "" {
		cfg.SpotifyClientID = val
	}
	if val := os.Getenv("SPOTIFY_CLIENT_SECRET"); val != "" {
		cfg.SpotifyClientSecret = val
	}
	if val := os.Getenv("SPOTIFY_ACCESS_TOKEN"); val != "" {
		cfg.SpotifyAccessToken = val
	}
	if val := os.Getenv("YTM_COOKIE"); val != "" {
		cfg.YTMCookie = val
	}
	if val := os.Getenv("ANTIGRAVITY_PATH"); val != "" {
		cfg.AntigravityPath = val
	} else if val := os.Getenv("AGY_PATH"); val != "" {
		cfg.AntigravityPath = val
	}
	if val := os.Getenv("GEMINI_API_KEY"); val != "" {
		cfg.GeminiAPIKey = val
	}
	if val := os.Getenv("OPENAI_API_KEY"); val != "" {
		cfg.OpenAIAPIKey = val
	}
	if val := os.Getenv("ANTHROPIC_API_KEY"); val != "" {
		cfg.AnthropicAPIKey = val
	}
	if val := os.Getenv("OLLAMA_ENDPOINT"); val != "" {
		cfg.OllamaEndpoint = val
	}

	if val := os.Getenv("CONCURRENCY"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.Concurrency = n
		}
	}

	if cfg.StrictThreshold <= 0 {
		cfg.StrictThreshold = 0.95
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 5
	}

	return cfg, nil
}

// Save persists the configuration to the standard path.
func (c *Config) Save() error {
	path, err := ConfigFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
