package referee

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"yt-import/internal/domain"
)

// AntigravityReferee uses the user's local Antigravity CLI session (`agy`),
// which utilizes their logged-in Antigravity account without requiring any external API key.
type AntigravityReferee struct {
	cliPath string
	mu      sync.Mutex
}

// NewAntigravityReferee creates an Antigravity referee using the local `agy` binary.
func NewAntigravityReferee(cliPath string) (*AntigravityReferee, error) {
	if cliPath == "" {
		cliPath = "agy"
	}
	path, err := exec.LookPath(cliPath)
	if err != nil {
		return nil, fmt.Errorf("antigravity CLI ('%s') not found in PATH: please ensure Antigravity CLI is installed", cliPath)
	}
	return &AntigravityReferee{cliPath: path}, nil
}

// runCLI runs agy with the given prompt and returns the output text.
func (a *AntigravityReferee) runCLI(ctx context.Context, prompt string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Sanitize environment: strip nested agent markers so agy runs as a clean standalone invocation
	var cleanEnv []string
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(envVar, "ANTIGRAVITY_AGENT=") ||
			strings.HasPrefix(envVar, "ANTIGRAVITY_CONVERSATION_ID=") ||
			strings.HasPrefix(envVar, "ANTIGRAVITY_TRAJECTORY_ID=") {
			continue
		}
		cleanEnv = append(cleanEnv, envVar)
	}

	var rawOut string
	var lastErr error

	for attempt := 1; attempt <= 2; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		ctxTimeout, cancel := context.WithTimeout(ctx, 120*time.Second)

		// Flags MUST precede --print for Go standard flag parser in agy
		cmd := exec.CommandContext(ctxTimeout, a.cliPath,
			"--output-format", "text",
			"--disable-slash-commands",
			"--dangerously-skip-permissions",
			"--effort", "low",
			"--print", prompt,
		)

		// Disconnect stdin so it does not conflict with terminal / TUI event loops
		cmd.Stdin = bytes.NewReader(nil)
		cmd.Env = cleanEnv

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		timedOut := ctxTimeout.Err() != nil
		cancel()

		if err == nil && strings.TrimSpace(stdout.String()) != "" {
			rawOut = strings.TrimSpace(stdout.String())
			lastErr = nil
			break
		}

		outStr := strings.TrimSpace(stdout.String())
		errStr := strings.TrimSpace(stderr.String())
		if timedOut {
			lastErr = fmt.Errorf("antigravity CLI timed out after 120s (attempt %d/2)", attempt)
		} else if err != nil {
			lastErr = fmt.Errorf("antigravity CLI failed (attempt %d/2): %w (stderr: %s, stdout: %s)", attempt, err, errStr, outStr)
		} else {
			lastErr = fmt.Errorf("antigravity CLI returned empty output (attempt %d/2)", attempt)
		}

		if attempt < 2 {
			time.Sleep(1 * time.Second)
		}
	}

	if lastErr != nil {
		return "", lastErr
	}
	return rawOut, nil
}

// Decide arbitrates candidates for a single track.
func (a *AntigravityReferee) Decide(ctx context.Context, source domain.Track, candidates []domain.Candidate) (*domain.RefereeVerdict, error) {
	userPrompt := BuildUserPrompt(source, candidates)
	combinedPrompt := fmt.Sprintf("%s\n\n%s", SystemPrompt, userPrompt)

	rawOut, err := a.runCLI(ctx, combinedPrompt)
	if err != nil {
		return nil, err
	}

	return ParseVerdictJSON(rawOut)
}

// DecideBatch arbitrates multiple ambiguous tracks in a single consolidated prompt.
func (a *AntigravityReferee) DecideBatch(ctx context.Context, items []BatchItem) ([]domain.RefereeVerdict, error) {
	if len(items) == 0 {
		return nil, nil
	}

	batchPrompt := BuildBatchPrompt(items)
	combinedPrompt := fmt.Sprintf("%s\n\n%s", BatchSystemPrompt, batchPrompt)

	rawOut, err := a.runCLI(ctx, combinedPrompt)
	if err != nil {
		return nil, err
	}

	return ParseBatchVerdictJSON(rawOut)
}
