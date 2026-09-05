package referee

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"yt-import/internal/domain"
)

type OpenAIReferee struct {
	apiKey string
	client *http.Client
}

func NewOpenAIReferee(apiKey string) *OpenAIReferee {
	return &OpenAIReferee{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OpenAIReferee) Decide(ctx context.Context, source domain.Track, candidates []domain.Candidate) (*domain.RefereeVerdict, error) {
	url := "https://api.openai.com/v1/chat/completions"

	userPrompt := BuildUserPrompt(source, candidates)

	payload := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "system", "content": SystemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.1,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var openaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBytes, &openaiResp); err != nil {
		return nil, err
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices from openai")
	}

	rawJSON := openaiResp.Choices[0].Message.Content
	rawJSON = strings.TrimSpace(rawJSON)

	var verdict domain.RefereeVerdict
	if err := json.Unmarshal([]byte(rawJSON), &verdict); err != nil {
		return nil, fmt.Errorf("failed to parse referee verdict JSON: %w (raw: %s)", err, rawJSON)
	}

	return &verdict, nil
}

func (o *OpenAIReferee) DecideBatch(ctx context.Context, items []BatchItem) ([]domain.RefereeVerdict, error) {
	if len(items) == 0 {
		return nil, nil
	}

	url := "https://api.openai.com/v1/chat/completions"
	batchPrompt := BuildBatchPrompt(items)

	payload := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "system", "content": BatchSystemPrompt},
			{"role": "user", "content": batchPrompt},
		},
		"temperature": 0.1,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var openaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBytes, &openaiResp); err != nil {
		return nil, err
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices from openai")
	}

	return ParseBatchVerdictJSON(openaiResp.Choices[0].Message.Content)
}
