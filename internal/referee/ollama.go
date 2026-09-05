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

type OllamaReferee struct {
	endpoint string
	model    string
	client   *http.Client
}

func NewOllamaReferee(endpoint, model string) *OllamaReferee {
	return &OllamaReferee{
		endpoint: strings.TrimSuffix(endpoint, "/"),
		model:    model,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (o *OllamaReferee) Decide(ctx context.Context, source domain.Track, candidates []domain.Candidate) (*domain.RefereeVerdict, error) {
	url := fmt.Sprintf("%s/api/chat", o.endpoint)

	userPrompt := BuildUserPrompt(source, candidates)

	payload := map[string]interface{}{
		"model":  o.model,
		"format": "json",
		"messages": []map[string]string{
			{"role": "system", "content": SystemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.1,
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
		return nil, fmt.Errorf("ollama api error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var ollamaResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := json.Unmarshal(respBytes, &ollamaResp); err != nil {
		return nil, err
	}

	rawJSON := strings.TrimSpace(ollamaResp.Message.Content)
	var verdict domain.RefereeVerdict
	if err := json.Unmarshal([]byte(rawJSON), &verdict); err != nil {
		return nil, fmt.Errorf("failed to parse referee verdict JSON: %w (raw: %s)", err, rawJSON)
	}

	return &verdict, nil
}

func (o *OllamaReferee) DecideBatch(ctx context.Context, items []BatchItem) ([]domain.RefereeVerdict, error) {
	if len(items) == 0 {
		return nil, nil
	}

	url := fmt.Sprintf("%s/api/chat", o.endpoint)
	batchPrompt := BuildBatchPrompt(items)

	payload := map[string]interface{}{
		"model":  o.model,
		"format": "json",
		"messages": []map[string]string{
			{"role": "system", "content": BatchSystemPrompt},
			{"role": "user", "content": batchPrompt},
		},
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.1,
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
		return nil, fmt.Errorf("ollama api error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var ollamaResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := json.Unmarshal(respBytes, &ollamaResp); err != nil {
		return nil, err
	}

	return ParseBatchVerdictJSON(ollamaResp.Message.Content)
}
