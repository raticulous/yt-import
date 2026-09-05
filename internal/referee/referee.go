package referee

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"yt-import/internal/config"
	"yt-import/internal/domain"
)

// Referee defines the contract for an AI LLM referee that arbitrates match decisions.
type Referee interface {
	Decide(ctx context.Context, source domain.Track, candidates []domain.Candidate) (*domain.RefereeVerdict, error)
	DecideBatch(ctx context.Context, items []BatchItem) ([]domain.RefereeVerdict, error)
}

// NewReferee creates an appropriate referee instance based on configuration.
func NewReferee(provider string, cfg *config.Config) (Referee, error) {
	switch provider {
	case "antigravity":
		cliPath := ""
		if cfg != nil {
			cliPath = cfg.AntigravityPath
		}
		return NewAntigravityReferee(cliPath)
	case "gemini":
		if cfg.GeminiAPIKey == "" {
			return nil, fmt.Errorf("gemini_api_key is required for Gemini referee")
		}
		return NewGeminiReferee(cfg.GeminiAPIKey), nil
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("openai_api_key is required for OpenAI referee")
		}
		return NewOpenAIReferee(cfg.OpenAIAPIKey), nil
	case "claude":
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("anthropic_api_key is required for Claude referee")
		}
		return NewClaudeReferee(cfg.AnthropicAPIKey), nil
	case "ollama":
		endpoint := cfg.OllamaEndpoint
		if endpoint == "" {
			endpoint = "http://localhost:11434"
		}
		model := cfg.OllamaModel
		if model == "" {
			model = "llama3"
		}
		return NewOllamaReferee(endpoint, model), nil
	case "mock", "":
		return NewMockReferee(), nil
	default:
		return nil, fmt.Errorf("unsupported AI referee provider: %s", provider)
	}
}

// ValidateProvider tests if the configured AI provider and API key/session are functional.
func ValidateProvider(ctx context.Context, provider string, cfg *config.Config) error {
	client := &http.Client{Timeout: 10 * time.Second}

	switch provider {
	case "gemini":
		if cfg == nil || cfg.GeminiAPIKey == "" {
			return fmt.Errorf("gemini_api_key is required")
		}
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", cfg.GeminiAPIKey)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to contact Gemini API: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("gemini API key validation failed (status %d)", resp.StatusCode)
		}
		return nil

	case "openai":
		if cfg == nil || cfg.OpenAIAPIKey == "" {
			return fmt.Errorf("openai_api_key is required")
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.openai.com/v1/models", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+cfg.OpenAIAPIKey)
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to contact OpenAI API: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("openAI API key validation failed (status %d)", resp.StatusCode)
		}
		return nil

	case "claude":
		if cfg == nil || cfg.AnthropicAPIKey == "" {
			return fmt.Errorf("anthropic_api_key is required")
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/v1/models", nil)
		if err != nil {
			return err
		}
		req.Header.Set("x-api-key", cfg.AnthropicAPIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to contact Anthropic API: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("anthropic API key validation failed (status %d)", resp.StatusCode)
		}
		return nil

	case "antigravity":
		cliPath := "agy"
		if cfg != nil && cfg.AntigravityPath != "" {
			cliPath = cfg.AntigravityPath
		}
		_, err := exec.LookPath(cliPath)
		if err != nil {
			return fmt.Errorf("antigravity CLI ('%s') not found in PATH", cliPath)
		}
		return nil

	case "ollama":
		endpoint := "http://localhost:11434"
		if cfg != nil && cfg.OllamaEndpoint != "" {
			endpoint = cfg.OllamaEndpoint
		}
		endpoint = strings.TrimSuffix(endpoint, "/")
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"/api/tags", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to reach Ollama at %s: %w", endpoint, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("ollama returned status %d", resp.StatusCode)
		}
		return nil

	case "mock", "":
		return nil

	default:
		return fmt.Errorf("unsupported AI referee provider: %s", provider)
	}
}

