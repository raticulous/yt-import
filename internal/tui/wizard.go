package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"yt-import/internal/config"
	"yt-import/internal/domain"
	"yt-import/internal/provider/file"
	"yt-import/internal/provider/spotify"
	"yt-import/internal/provider/ytmusic"
	"yt-import/internal/referee"
)


// RunWizard launches an interactive huh form to gather import parameters.
func RunWizard(cfg *config.Config) (*domain.SyncOptions, error) {
	var (
		sourcePlaylist string
		targetPlaylist string = cfg.LastTargetPlaylist
		offsetStr      string = "0"
		limitStr       string = "0"
		thresholdStr   string = fmt.Sprintf("%.0f", cfg.StrictThreshold*100)
		enableAI       bool   = true
		aiProvider     string = cfg.AIProvider
		apiKey         string
		dryRun         bool   = cfg.LastDryRun
	)

	if cfg.LastOffset > 0 {
		offsetStr = strconv.Itoa(cfg.LastOffset)
	}
	if cfg.LastLimit > 0 {
		limitStr = strconv.Itoa(cfg.LastLimit)
	}
	if cfg.LastStrictThreshold > 0 {
		thresholdStr = fmt.Sprintf("%.0f", cfg.LastStrictThreshold*100)
	}
	if cfg.LastEnableAI != nil {
		enableAI = *cfg.LastEnableAI
	}
	if cfg.LastAIProvider != "" {
		aiProvider = cfg.LastAIProvider
	}
	if aiProvider == "" {
		aiProvider = "antigravity"
	}

	var sourceMethod string = "exportify"
	if cfg.LastSourceMethod != "" && cfg.LastSourceMethod != "retry" {
		sourceMethod = cfg.LastSourceMethod
	}

	var methodOptions []huh.Option[string]

	if cfg.HasLastSession() {
		displaySrc := cfg.LastSourcePlaylist
		if displaySrc == "" {
			if cfg.LastFilePath != "" {
				displaySrc = cfg.LastFilePath
			} else {
				displaySrc = cfg.LastSpotifyURL
			}
		}
		if len(displaySrc) > 40 {
			displaySrc = displaySrc[:18] + "..." + displaySrc[len(displaySrc)-18:]
		}
		targetSummary := "auto-create"
		if cfg.LastTargetPlaylist != "" {
			targetSummary = cfg.LastTargetPlaylist
			if len(targetSummary) > 25 {
				targetSummary = targetSummary[:22] + "..."
			}
		}
		retryLabel := fmt.Sprintf("⚡ Retry Last Import (%s -> %s)", displaySrc, targetSummary)
		methodOptions = append(methodOptions, huh.NewOption(retryLabel, "retry"))
		sourceMethod = "retry"
	}

	methodOptions = append(methodOptions,
		huh.NewOption("🚀 Auto-Export via Exportify (Opens browser & auto-detects CSV download - 100% Free)", "exportify"),
		huh.NewOption("🔗 Spotify Playlist URL (Free embed - 100 songs max)", "spotify_url"),
		huh.NewOption("📁 Local CSV / TXT File path", "file_path"),
	)

	methodForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Choose Playlist Source Method").
				Description("Select how to load your Spotify playlist:").
				Options(methodOptions...).
				Value(&sourceMethod),
		),
	)

	if err := methodForm.Run(); err != nil {
		return nil, err
	}

	var skipForm1 bool

	if sourceMethod == "retry" {
		sourcePlaylist = cfg.LastSourcePlaylist
		if sourcePlaylist == "" {
			if cfg.LastFilePath != "" {
				sourcePlaylist = cfg.LastFilePath
			} else {
				sourcePlaylist = cfg.LastSpotifyURL
			}
		}

		threshVal, _ := strconv.ParseFloat(thresholdStr, 64)
		var retryAction string = "run"
		retryForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("⚡ Quick Retry Last Session").
					Description(fmt.Sprintf("Source: %s\nTarget: %s\nOffset: %s | Limit: %s | Confidence: %.0f%% | AI: %v (%s) | DryRun: %v",
						sourcePlaylist, targetPlaylist, offsetStr, limitStr, threshVal, enableAI, aiProvider, dryRun)).
					Options(
						huh.NewOption("🚀 Run immediately with these settings", "run"),
						huh.NewOption("✏️ Review & edit settings before running", "edit"),
					).
					Value(&retryAction),
			),
		)
		if err := retryForm.Run(); err != nil {
			return nil, err
		}
		if retryAction == "run" {
			skipForm1 = true
		}
	} else if sourceMethod == "exportify" {
		cfg.LastSourceMethod = "exportify"
		_ = cfg.Save()

		fmt.Println()
		fmt.Println(StyleHeader.Render("  🚀 Opening Exportify in your default browser..."))
		fmt.Println(StyleSubtitle.Render("  1. Click 'Get Started' to connect your Spotify account."))
		fmt.Println(StyleSubtitle.Render("  2. Click 'Export' next to your desired playlist."))
		fmt.Println(StyleSuccessText.Render("  ⏳ Watching your Downloads folder for the exported CSV file..."))
		fmt.Println()

		_ = file.OpenExportify()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		detectedPath, err := file.WatchDownloadsForExportify(ctx, func(status string) {
			// Watching status
		})
		if err != nil {
			fmt.Println(BadgeDisqualified.Render("TIMEOUT") + " No CSV detected. Please enter file path manually:")
			if cfg.LastFilePath != "" {
				sourcePlaylist = cfg.LastFilePath
			} else {
				sourcePlaylist = findRecentDownloadCSV()
			}
			fallbackForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Playlist File Path").
						Description("Enter path to your downloaded CSV / TXT file:").
						Value(&sourcePlaylist).
						Validate(func(s string) error {
							if strings.TrimSpace(s) == "" {
								return fmt.Errorf("file path is required")
							}
							return nil
						}),
				),
			)
			if err := fallbackForm.Run(); err != nil {
				return nil, err
			}
			sourcePlaylist = strings.TrimSpace(sourcePlaylist)
			cfg.LastFilePath = sourcePlaylist
			cfg.LastSourcePlaylist = sourcePlaylist
			_ = cfg.Save()
		} else {
			sourcePlaylist = detectedPath
			cfg.LastFilePath = detectedPath
			cfg.LastSourcePlaylist = detectedPath
			_ = cfg.Save()
			fmt.Printf("  %s %s\n\n", BadgeAutoMatch.Render("AUTO-DETECTED"), detectedPath)
		}
	} else {
		promptTitle := "Spotify Playlist URL"
		promptDesc := "Enter public Spotify playlist URL or ID"
		placeholder := "https://open.spotify.com/playlist/..."
		if sourceMethod == "file_path" {
			promptTitle = "Local File Path"
			promptDesc = "Enter path to .csv or .txt file"
			placeholder = "path/to/playlist.csv"
			if cfg.LastFilePath != "" {
				sourcePlaylist = cfg.LastFilePath
			} else if fileExists(cfg.LastSourcePlaylist) {
				sourcePlaylist = cfg.LastSourcePlaylist
			} else {
				sourcePlaylist = findRecentDownloadCSV()
			}
		} else {
			if cfg.LastSpotifyURL != "" {
				sourcePlaylist = cfg.LastSpotifyURL
			} else if strings.Contains(cfg.LastSourcePlaylist, "spotify.com") {
				sourcePlaylist = cfg.LastSourcePlaylist
			}
		}

		sourceForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title(promptTitle).
					Description(promptDesc).
					Placeholder(placeholder).
					Value(&sourcePlaylist).
					Validate(func(s string) error {
						if strings.TrimSpace(s) == "" {
							return fmt.Errorf("source is required")
						}
						return nil
					}),
			),
		)
		if err := sourceForm.Run(); err != nil {
			return nil, err
		}

		sourcePlaylist = strings.TrimSpace(sourcePlaylist)
		cfg.LastSourcePlaylist = sourcePlaylist
		cfg.LastSourceMethod = sourceMethod
		if sourceMethod == "file_path" {
			cfg.LastFilePath = sourcePlaylist
		} else if sourceMethod == "spotify_url" {
			cfg.LastSpotifyURL = sourcePlaylist
		}
		_ = cfg.Save()
	}

	if !skipForm1 {
		// Form 1: Destination Playlist & Slicing
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Destination Playlist (YouTube Music)").
					Description("Enter YouTube Music Playlist URL or ID (leave blank to auto-create new playlist)").
					Placeholder("PL... or https://music.youtube.com/playlist?list=...").
					Value(&targetPlaylist).
					Validate(func(s string) error {
						s = strings.TrimSpace(s)
						if s == "" {
							return nil
						}
						if strings.Contains(s, "watch?") && !strings.Contains(s, "list=") {
							return fmt.Errorf("this is a video link, not a playlist link (URL must contain 'list=' or be a playlist ID starting with 'PL')")
						}
						return nil
					}),
			),

			huh.NewGroup(
				huh.NewInput().
					Title("Offset (Song Index)").
					Description("Start importing from this song index (0 = from the beginning)").
					Value(&offsetStr).
					Validate(func(s string) error {
						if _, err := strconv.Atoi(s); err != nil {
							return fmt.Errorf("must be a valid integer")
						}
						return nil
					}),

				huh.NewInput().
					Title("Limit (Number of Songs)").
					Description("How many songs to import (0 = all remaining songs)").
					Value(&limitStr).
					Validate(func(s string) error {
						if _, err := strconv.Atoi(s); err != nil {
							return fmt.Errorf("must be a valid integer")
						}
						return nil
					}),

				huh.NewInput().
					Title("Strict Confidence Threshold (%)").
					Description("Minimum confidence required to import a song (default: 95%)").
					Value(&thresholdStr).
					Validate(func(s string) error {
						v, err := strconv.ParseFloat(s, 64)
						if err != nil || v < 50 || v > 100 {
							return fmt.Errorf("threshold must be between 50 and 100")
						}
						return nil
					}),
			),

			huh.NewGroup(
				huh.NewConfirm().
					Title("Enable AI LLM Referee?").
					Description("Use an AI provider to arbitrate ambiguous songs and near-threshold matches").
					Value(&enableAI),

				huh.NewSelect[string]().
					Title("AI Referee Provider").
					Options(
						huh.NewOption("Antigravity (IDE / CLI Login - No API Key Needed)", "antigravity"),
						huh.NewOption("Google Gemini (gemini-2.5-flash)", "gemini"),
						huh.NewOption("OpenAI (gpt-4o-mini)", "openai"),
						huh.NewOption("Anthropic Claude (claude-3-5-haiku)", "claude"),
						huh.NewOption("Local Ollama (llama3 - Free / Local)", "ollama"),
						huh.NewOption("Mock Referee (offline / testing)", "mock"),
					).
					Value(&aiProvider),

				huh.NewConfirm().
					Title("Dry Run Mode?").
					Description("If yes, songs will be matched and reported, but NOT inserted to YouTube Music").
					Value(&dryRun),
			),
		)

		if err := form.Run(); err != nil {
			return nil, err
		}
	}

	offset, _ := strconv.Atoi(offsetStr)
	limit, _ := strconv.Atoi(limitStr)
	threshVal, _ := strconv.ParseFloat(thresholdStr, 64)
	threshold := threshVal / 100.0

	// Persist choices immediately
	cfg.LastTargetPlaylist = targetPlaylist
	cfg.LastOffset = offset
	cfg.LastLimit = limit
	cfg.LastStrictThreshold = threshold
	cfg.LastEnableAI = &enableAI
	cfg.LastAIProvider = aiProvider
	cfg.LastDryRun = dryRun
	_ = cfg.Save()

	cleanSource := strings.Trim(strings.TrimSpace(sourcePlaylist), `"'`)
	isLocalFile := strings.HasSuffix(strings.ToLower(cleanSource), ".csv") ||
		strings.HasSuffix(strings.ToLower(cleanSource), ".txt") ||
		fileExists(cleanSource)

	// If source is a Spotify URL:
	if !isLocalFile {
		if HasSpotifyCredentials(cfg) {
			// Check if stored credentials actually work
			fmt.Print(StyleLogReason.Render("  Checking stored Spotify credentials... "))
			if err := spotify.ValidateCredentials(context.Background(), cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.SpotifyAccessToken); err != nil {
				fmt.Println()
				fmt.Println(BadgeDisqualified.Render("EXPIRED / INVALID") + " Stored Spotify credentials/token failed verification: " + err.Error())
				var mode string = "creds"
				modeForm := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title("Spotify Credentials Expired or Invalid").
							Description("Your stored Spotify credentials failed. What would you like to do?").
							Options(
								huh.NewOption("Re-enter Spotify Credentials / Token", "creds"),
								huh.NewOption("Switch to Quick Public Mode (Import first 100 songs - zero setup)", "quick"),
							).
							Value(&mode),
					),
				)
				if err := modeForm.Run(); err != nil {
					return nil, err
				}
				if mode == "creds" {
					if err := PromptSpotifyCredentials(cfg); err != nil {
						return nil, err
					}
				}
			} else {
				fmt.Println(StyleSuccessText.Render("✓ Verified"))
			}
		} else {
			var mode string = "quick"
			modeForm := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Spotify Import Mode").
						Description("Spotify's public web preview returns up to 100 songs without login.\nTo import 100+ songs, you can use a free CSV export (Exportify/Spotlistr) or Spotify Developer credentials.").
						Options(
							huh.NewOption("Quick Public Mode (Import first 100 songs - zero setup)", "quick"),
							huh.NewOption("Spotify Developer App (Client ID & Secret - requires Spotify Premium)", "creds"),
						).
						Value(&mode),
				),
			)

			if err := modeForm.Run(); err != nil {
				return nil, err
			}

			if mode == "creds" {
				if err := PromptSpotifyCredentials(cfg); err != nil {
					return nil, err
				}
			}
		}
	}

	// If AI Referee is enabled:
	if enableAI {
		if aiProvider == "gemini" || aiProvider == "openai" || aiProvider == "claude" {
			currentKey := ""
			switch aiProvider {
			case "gemini":
				currentKey = cfg.GeminiAPIKey
			case "openai":
				currentKey = cfg.OpenAIAPIKey
			case "claude":
				currentKey = cfg.AnthropicAPIKey
			}

			if currentKey == "" {
				for {
					keyForm := huh.NewForm(
						huh.NewGroup(
							huh.NewInput().
								Title(fmt.Sprintf("%s API Key", aiProvider)).
								Description(fmt.Sprintf("Enter your %s API key to enable the AI Referee", aiProvider)).
								EchoMode(huh.EchoModePassword).
								Value(&apiKey).
								Validate(func(s string) error {
									if strings.TrimSpace(s) == "" {
										return fmt.Errorf("API key is required")
									}
									return nil
								}),
						),
					)
					if err := keyForm.Run(); err != nil {
						return nil, err
					}
					apiKey = strings.TrimSpace(apiKey)
					tempCfg := *cfg
					switch aiProvider {
					case "gemini":
						tempCfg.GeminiAPIKey = apiKey
					case "openai":
						tempCfg.OpenAIAPIKey = apiKey
					case "claude":
						tempCfg.AnthropicAPIKey = apiKey
					}
					fmt.Print(StyleLogReason.Render(fmt.Sprintf("  Checking %s API key... ", aiProvider)))
					if err := referee.ValidateProvider(context.Background(), aiProvider, &tempCfg); err != nil {
						fmt.Println()
						fmt.Println(BadgeDisqualified.Render("INVALID KEY") + " " + err.Error())
						continue
					}
					fmt.Println(StyleSuccessText.Render("✓ Verified"))
					switch aiProvider {
					case "gemini":
						cfg.GeminiAPIKey = apiKey
					case "openai":
						cfg.OpenAIAPIKey = apiKey
					case "claude":
						cfg.AnthropicAPIKey = apiKey
					}
					_ = cfg.Save()
					break
				}
			} else {
				// Validate existing cached API key
				fmt.Print(StyleLogReason.Render(fmt.Sprintf("  Checking stored %s API key... ", aiProvider)))
				if err := referee.ValidateProvider(context.Background(), aiProvider, cfg); err != nil {
					fmt.Println()
					fmt.Println(BadgeDisqualified.Render("INVALID KEY") + fmt.Sprintf(" Stored %s API key failed verification: %s", aiProvider, err.Error()))
					for {
						keyForm := huh.NewForm(
							huh.NewGroup(
								huh.NewInput().
									Title(fmt.Sprintf("Re-enter %s API Key", aiProvider)).
									Description("Your stored API key is invalid. Please enter a working key:").
									EchoMode(huh.EchoModePassword).
									Value(&apiKey).
									Validate(func(s string) error {
										if strings.TrimSpace(s) == "" {
											return fmt.Errorf("API key is required")
										}
										return nil
									}),
							),
						)
						if err := keyForm.Run(); err != nil {
							return nil, err
						}
						apiKey = strings.TrimSpace(apiKey)
						tempCfg := *cfg
						switch aiProvider {
						case "gemini":
							tempCfg.GeminiAPIKey = apiKey
						case "openai":
							tempCfg.OpenAIAPIKey = apiKey
						case "claude":
							tempCfg.AnthropicAPIKey = apiKey
						}
						fmt.Print(StyleLogReason.Render(fmt.Sprintf("  Checking %s API key... ", aiProvider)))
						if err := referee.ValidateProvider(context.Background(), aiProvider, &tempCfg); err != nil {
							fmt.Println()
							fmt.Println(BadgeDisqualified.Render("INVALID KEY") + " " + err.Error())
							continue
						}
						fmt.Println(StyleSuccessText.Render("✓ Verified"))
						switch aiProvider {
						case "gemini":
							cfg.GeminiAPIKey = apiKey
						case "openai":
							cfg.OpenAIAPIKey = apiKey
						case "claude":
							cfg.AnthropicAPIKey = apiKey
						}
						_ = cfg.Save()
						break
					}
				} else {
					fmt.Println(StyleSuccessText.Render("✓ Verified"))
				}
			}
		} else if aiProvider == "antigravity" || aiProvider == "ollama" {
			fmt.Print(StyleLogReason.Render(fmt.Sprintf("  Checking %s referee setup... ", aiProvider)))
			if err := referee.ValidateProvider(context.Background(), aiProvider, cfg); err != nil {
				fmt.Println()
				fmt.Println(BadgeDisqualified.Render("SETUP ERROR") + fmt.Sprintf(" %s referee setup failed: %s", aiProvider, err.Error()))
			} else {
				fmt.Println(StyleSuccessText.Render("✓ Verified"))
			}
		}
	}

	// If writing to YouTube Music (not dry run), check and verify YouTube Music cookie
	if !dryRun {
		if cfg.YTMCookie == "" {
			if err := PromptYTMCookie(cfg, &dryRun); err != nil {
				return nil, err
			}
		} else {
			// Check if stored YouTube Music cookie works
			fmt.Print(StyleLogReason.Render("  Checking stored YouTube Music session... "))
			ytmClient := ytmusic.NewClient(cfg.YTMCookie)
			accountName, err := ytmClient.ValidateAuth(context.Background())
			if err != nil {
				fmt.Println()
				fmt.Println(BadgeDisqualified.Render("EXPIRED COOKIE") + " Stored YouTube Music session cookie is invalid or has expired.")
				fmt.Println(StyleLogReason.Render("  Reason: " + err.Error()))
				fmt.Println()
				// Prompt user to enter new cookie or switch to dry run
				if err := PromptYTMCookie(cfg, &dryRun); err != nil {
					return nil, err
				}
			} else {
				if accountName != "" && accountName != "Active Session" {
					fmt.Println(StyleSuccessText.Render(fmt.Sprintf("✓ Verified (Logged in as: %s)", accountName)))
				} else {
					fmt.Println(StyleSuccessText.Render("✓ Verified"))
				}
			}
		}
	}

	return &domain.SyncOptions{
		SourcePlaylistID:  sourcePlaylist,
		TargetPlaylistID:  targetPlaylist,
		Offset:            offset,
		Limit:             limit,
		Threshold:         threshold,
		AIRefereeEnabled:  enableAI,
		AIRefereeProvider: aiProvider,
		DryRun:            dryRun,
		Concurrency:       cfg.Concurrency,
	}, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// findRecentDownloadCSV scans the user's Downloads folder for the most recently updated CSV or TXT file.
func findRecentDownloadCSV() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	downloads := filepath.Join(home, "Downloads")
	entries, err := os.ReadDir(downloads)
	if err != nil {
		return ""
	}
	var mostRecent string
	var mostRecentTime time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".csv") || strings.HasSuffix(name, ".txt") {
			info, err := entry.Info()
			if err == nil {
				if mostRecent == "" || info.ModTime().After(mostRecentTime) {
					mostRecent = filepath.Join(downloads, entry.Name())
					mostRecentTime = info.ModTime()
				}
			}
		}
	}
	return mostRecent
}

