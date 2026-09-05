package spotify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// ExtractPlaylistID parses Spotify URLs or raw IDs.
func ExtractPlaylistID(input string) string {
	input = strings.TrimSpace(input)
	if strings.Contains(input, "spotify.com/playlist/") {
		parts := strings.Split(input, "spotify.com/playlist/")
		if len(parts) > 1 {
			id := parts[1]
			if idx := strings.IndexAny(id, "?/&"); idx != -1 {
				id = id[:idx]
			}
			return id
		}
	}
	// Also handle spotify:playlist:URI
	if strings.HasPrefix(input, "spotify:playlist:") {
		return strings.TrimPrefix(input, "spotify:playlist:")
	}
	return input
}

// AuthenticateClient creates an HTTP client for Spotify API access.
func AuthenticateClient(ctx context.Context, clientID, clientSecret, token string) (*http.Client, error) {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.Trim(token, `"'`)

	if token != "" {
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: token,
			TokenType:   "Bearer",
		})
		return oauth2.NewClient(ctx, tokenSource), nil
	}


	if clientID != "" && clientSecret != "" {
		// Client Credentials flow (sufficient for reading any public playlist)
		config := &clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TokenURL:     spotifyauth.TokenURL,
		}
		return config.Client(ctx), nil
	}

	return nil, fmt.Errorf("no Spotify credentials provided. Please supply an Access Token or Client ID & Client Secret")
}

// ValidateCredentials checks if the provided Spotify Bearer token or Client ID & Secret are active and valid.
func ValidateCredentials(ctx context.Context, clientID, clientSecret, token string) error {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.Trim(token, `"'`)

	if token != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.spotify.com/v1/me", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to contact Spotify API: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("spotify access token is invalid or has expired (status 401)")
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusForbidden {
			return fmt.Errorf("spotify authentication error (status %d)", resp.StatusCode)
		}
		return nil
	}

	if clientID != "" && clientSecret != "" {
		config := &clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TokenURL:     spotifyauth.TokenURL,
		}
		_, err := config.Token(ctx)
		if err != nil {
			return fmt.Errorf("spotify client credentials authentication failed: %w", err)
		}
		return nil
	}

	return fmt.Errorf("no Spotify credentials or token provided")
}


// InteractiveOAuthLogin launches a temporary local server to perform OAuth 2.0 authorization code grant.
func InteractiveOAuthLogin(ctx context.Context, clientID, clientSecret, redirectURL string) (*oauth2.Token, error) {
	if redirectURL == "" {
		redirectURL = "http://localhost:8080/callback"
	}

	auth := spotifyauth.New(
		spotifyauth.WithClientID(clientID),
		spotifyauth.WithClientSecret(clientSecret),
		spotifyauth.WithRedirectURL(redirectURL),
		spotifyauth.WithScopes(spotifyauth.ScopePlaylistReadPrivate, spotifyauth.ScopePlaylistReadCollaborative),
	)

	state := fmt.Sprintf("ytimport_%d", time.Now().UnixNano())
	authURL := auth.AuthURL(state)

	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "State mismatch error", http.StatusBadRequest)
			errChan <- fmt.Errorf("oauth state mismatch")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			errChan <- fmt.Errorf("missing code in callback")
			return
		}

		fmt.Fprintln(w, "<h1>Authorization Successful!</h1><p>You can close this window and return to your terminal.</p>")
		codeChan <- code
	})

	u, err := url.Parse(redirectURL)
	if err != nil {
		return nil, err
	}
	server := &http.Server{
		Addr:    ":" + u.Port(),
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	fmt.Printf("\nPlease open the following URL in your browser to authorize yt-import:\n\n%s\n\n", authURL)

	select {
	case <-ctx.Done():
		_ = server.Shutdown(context.Background())
		return nil, ctx.Err()
	case err := <-errChan:
		_ = server.Shutdown(context.Background())
		return nil, err
	case code := <-codeChan:
		_ = server.Shutdown(context.Background())
		tok, err := auth.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange token: %w", err)
		}
		return tok, nil
	}
}
