package config

import (
	"encoding/json"
	"testing"
)

func TestLastSession(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.HasLastSession() {
		t.Errorf("expected HasLastSession() to be false for default config")
	}

	cfg.LastSourcePlaylist = "https://open.spotify.com/playlist/test"
	cfg.LastTargetPlaylist = "PLtest123"
	cfg.LastOffset = 10
	cfg.LastLimit = 50
	cfg.LastStrictThreshold = 0.90
	enableAI := true
	cfg.LastEnableAI = &enableAI
	cfg.LastAIProvider = "openai"
	cfg.LastDryRun = true

	if !cfg.HasLastSession() {
		t.Errorf("expected HasLastSession() to be true after setting playlist")
	}

	// Verify JSON marshaling & unmarshaling roundtrip
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	loaded := &Config{}
	if err := json.Unmarshal(data, loaded); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	if loaded.LastSourcePlaylist != cfg.LastSourcePlaylist {
		t.Errorf("expected LastSourcePlaylist %s, got %s", cfg.LastSourcePlaylist, loaded.LastSourcePlaylist)
	}
	if loaded.LastTargetPlaylist != cfg.LastTargetPlaylist {
		t.Errorf("expected LastTargetPlaylist %s, got %s", cfg.LastTargetPlaylist, loaded.LastTargetPlaylist)
	}
	if loaded.LastOffset != 10 || loaded.LastLimit != 50 {
		t.Errorf("expected offset 10 and limit 50, got %d and %d", loaded.LastOffset, loaded.LastLimit)
	}
	if loaded.LastStrictThreshold != 0.90 {
		t.Errorf("expected threshold 0.90, got %f", loaded.LastStrictThreshold)
	}
	if loaded.LastEnableAI == nil || !*loaded.LastEnableAI {
		t.Errorf("expected LastEnableAI true")
	}
	if loaded.LastAIProvider != "openai" {
		t.Errorf("expected LastAIProvider openai, got %s", loaded.LastAIProvider)
	}
	if !loaded.LastDryRun {
		t.Errorf("expected LastDryRun true")
	}

	// Test ClearLastSession
	cfg.ClearLastSession()
	if cfg.HasLastSession() {
		t.Errorf("expected HasLastSession() to be false after ClearLastSession")
	}
	if cfg.LastTargetPlaylist != "" || cfg.LastOffset != 0 || cfg.LastLimit != 0 {
		t.Errorf("expected last session fields to be zeroed")
	}
}
