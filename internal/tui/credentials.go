package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"yt-import/internal/config"
	"yt-import/internal/provider/spotify"
	"yt-import/internal/provider/ytmusic"
)

// HasSpotifyCredentials checks if Spotify credentials or token are configured.
func HasSpotifyCredentials(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.SpotifyAccessToken != "" || (cfg.SpotifyClientID != "" && cfg.SpotifyClientSecret != "")
}

// PromptSpotifyCredentials asks the user for Spotify credentials interactively and saves them.
func PromptSpotifyCredentials(cfg *config.Config) error {
	for {
		var authMethod string = "client_credentials"

		methodForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Spotify Authentication Required").
					Description("yt-import needs Spotify access to fetch playlist tracks.\nHow would you like to authenticate?").
					Options(
						huh.NewOption("Spotify Developer App (Client ID & Secret) - Recommended", "client_credentials"),
						huh.NewOption("Direct Access Token (Temporary token from Spotify Console)", "token"),
					).
					Value(&authMethod),
			),
		)

		if err := methodForm.Run(); err != nil {
			return err
		}

		if authMethod == "client_credentials" {
			var clientID, clientSecret string
			clientID = cfg.SpotifyClientID
			clientSecret = cfg.SpotifyClientSecret

			credForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Spotify Client ID").
						Description("Create a free app at https://developer.spotify.com/dashboard and copy Client ID").
						Placeholder("e.g. 4a2b8...").
						Value(&clientID).
						Validate(func(s string) error {
							if strings.TrimSpace(s) == "" {
								return fmt.Errorf("client ID is required")
							}
							return nil
						}),

					huh.NewInput().
						Title("Spotify Client Secret").
						Description("From your Spotify app settings").
						Placeholder("e.g. 9f1e0...").
						EchoMode(huh.EchoModePassword).
						Value(&clientSecret).
						Validate(func(s string) error {
							if strings.TrimSpace(s) == "" {
								return fmt.Errorf("client secret is required")
							}
							return nil
						}),
				),
			)

			if err := credForm.Run(); err != nil {
				return err
			}

			clientID = strings.TrimSpace(clientID)
			clientSecret = strings.TrimSpace(clientSecret)

			fmt.Print(StyleLogReason.Render("  Verifying Spotify credentials... "))
			if err := spotify.ValidateCredentials(context.Background(), clientID, clientSecret, ""); err != nil {
				fmt.Println()
				fmt.Println(BadgeDisqualified.Render("AUTH FAILED") + " " + err.Error())
				fmt.Println()
				continue
			}

			fmt.Println()
			fmt.Println(StyleSuccessText.Render("✓ Spotify client credentials verified successfully"))

			cfg.SpotifyClientID = clientID
			cfg.SpotifyClientSecret = clientSecret
			if err := cfg.Save(); err != nil {
				fmt.Printf("Warning: Failed to persist config to disk: %v\n", err)
			} else {
				fmt.Println(StyleSuccessText.Render("✓ Spotify credentials saved to config"))
			}
			return nil
		} else {
			var token string
			token = cfg.SpotifyAccessToken

			tokenForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Spotify Access Token").
						Description("Paste a Bearer access token (from developer.spotify.com or web player)").
						Placeholder("BQC...").
						EchoMode(huh.EchoModePassword).
						Value(&token).
						Validate(func(s string) error {
							if strings.TrimSpace(s) == "" {
								return fmt.Errorf("access token is required")
							}
							return nil
						}),
				),
			)

			if err := tokenForm.Run(); err != nil {
				return err
			}

			cleanedToken := strings.TrimSpace(token)
			cleanedToken = strings.TrimPrefix(cleanedToken, "Bearer ")
			cleanedToken = strings.TrimPrefix(cleanedToken, "bearer ")
			cleanedToken = strings.Trim(cleanedToken, `"'`)

			fmt.Print(StyleLogReason.Render("  Verifying Spotify token... "))
			if err := spotify.ValidateCredentials(context.Background(), "", "", cleanedToken); err != nil {
				fmt.Println()
				fmt.Println(BadgeDisqualified.Render("AUTH FAILED") + " " + err.Error())
				fmt.Println()
				continue
			}

			fmt.Println()
			fmt.Println(StyleSuccessText.Render("✓ Spotify access token verified successfully"))

			cfg.SpotifyAccessToken = cleanedToken
			if err := cfg.Save(); err != nil {
				fmt.Printf("Warning: Failed to persist config to disk: %v\n", err)
			} else {
				fmt.Println(StyleSuccessText.Render("✓ Spotify access token saved to config"))
			}
			return nil
		}
	}
}

// PromptYTMCookie asks for YouTube Music session cookie if needed for writing to a playlist.
func PromptYTMCookie(cfg *config.Config, dryRun *bool) error {
	for {
		var action string = "cookie"

		actionForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("YouTube Music Authentication Required").
					Description("Inserting tracks into a YouTube Music playlist requires an active session cookie.\nWhat would you like to do?").
					Options(
						huh.NewOption("Enter YouTube Music Cookie (from browser F12 Network tab)", "cookie"),
						huh.NewOption("Switch to Dry Run (simulate and test matches without inserting)", "dry_run"),
					).
					Value(&action),
			),
		)

		if err := actionForm.Run(); err != nil {
			return err
		}

		if action == "dry_run" {
			*dryRun = true
			return nil
		}

		var cookie string
		cookieForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("YouTube Music Cookie Header").
					Description("Open music.youtube.com (logged in) -> F12 -> Network -> click any request -> copy 'cookie:' header").
					EchoMode(huh.EchoModePassword).
					Value(&cookie).
					Validate(func(s string) error {
						s = strings.TrimSpace(s)
						if s == "" {
							return fmt.Errorf("cookie is required")
						}
						if !strings.Contains(s, "SAPISID") && !strings.Contains(s, "__Secure-3PAPISID") {
							return fmt.Errorf("cookie is missing SAPISID (ensure you are logged into your Google account at music.youtube.com before copying)")
						}
						return nil
					}),
			),
		)

		if err := cookieForm.Run(); err != nil {
			return err
		}

		cookie = strings.TrimSpace(cookie)

		// Real-time verification of entered cookie
		fmt.Print(StyleLogReason.Render("  Verifying YouTube Music session... "))
		client := ytmusic.NewClient(cookie)
		accountName, err := client.ValidateAuth(context.Background())
		if err != nil {
			fmt.Println()
			fmt.Println(BadgeDisqualified.Render("AUTH FAILED") + " " + err.Error())
			fmt.Println(StyleSubtitle.Render("  Please ensure you are logged in to music.youtube.com in your browser before copying the cookie."))
			fmt.Println()
			continue
		}

		fmt.Println()
		if accountName != "" && accountName != "Active Session" {
			fmt.Println(StyleSuccessText.Render(fmt.Sprintf("✓ YouTube Music session verified (Logged in as: %s)", accountName)))
		} else {
			fmt.Println(StyleSuccessText.Render("✓ YouTube Music session verified successfully"))
		}

		cfg.YTMCookie = cookie
		if err := cfg.Save(); err != nil {
			fmt.Printf("Warning: Failed to persist config to disk: %v\n", err)
		} else {
			fmt.Println(StyleSuccessText.Render("✓ YouTube Music cookie saved to config"))
		}

		return nil
	}
}

