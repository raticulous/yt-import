package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"yt-import/internal/config"
	"yt-import/internal/domain"
	"yt-import/internal/matcher"
	"yt-import/internal/provider"
	"yt-import/internal/provider/file"
	"yt-import/internal/provider/spotify"
	"yt-import/internal/provider/ytmusic"
	"yt-import/internal/referee"
	"yt-import/internal/syncer"
	"yt-import/internal/tui"
)

var (
	version = "1.0.0"

	// Flags for sync command
	flagSource      string
	flagTarget      string
	flagOffset      int
	flagLimit       int
	flagThreshold   float64
	flagAIProvider  string
	flagDryRun      bool
	flagHeadless    bool
	flagReportFile  string
	flagConcurrency int
	flagGeminiKey   string
	flagOpenAIKey   string
	flagRetry       bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "yt-import",
		Short: "High-precision music importer from Spotify to YouTube Music",
		Long:  tui.AppBanner + "\nyt-import is a high-precision CLI tool to import playlists from Spotify to YouTube Music with a 95%+ precision guarantee and AI referee disambiguation.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// When run without arguments, launch the interactive Charm TUI wizard
			return runInteractive()
		},
	}

	// Sync command for scripted / CLI flag usage
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize tracks with command-line flags",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			if flagRetry {
				if flagSource == "" && cfg.LastSourcePlaylist != "" {
					flagSource = cfg.LastSourcePlaylist
				}
				if flagTarget == "" && cfg.LastTargetPlaylist != "" {
					flagTarget = cfg.LastTargetPlaylist
				}
				if !cmd.Flags().Changed("offset") && cfg.LastOffset > 0 {
					flagOffset = cfg.LastOffset
				}
				if !cmd.Flags().Changed("limit") && cfg.LastLimit > 0 {
					flagLimit = cfg.LastLimit
				}
				if !cmd.Flags().Changed("threshold") && cfg.LastStrictThreshold > 0 {
					flagThreshold = cfg.LastStrictThreshold
				}
				if !cmd.Flags().Changed("dry-run") {
					flagDryRun = cfg.LastDryRun
				}
				if !cmd.Flags().Changed("ai-provider") && cfg.LastAIProvider != "" {
					flagAIProvider = cfg.LastAIProvider
				}
			}

			if flagSource == "" {
				return fmt.Errorf("source playlist is required (specify -s/--source or use --retry to reuse previous session)")
			}

			opts := domain.SyncOptions{
				SourcePlaylistID:  flagSource,
				TargetPlaylistID:  flagTarget,
				Offset:            flagOffset,
				Limit:             flagLimit,
				Threshold:         flagThreshold,
				AIRefereeEnabled:  flagAIProvider != "",
				AIRefereeProvider: flagAIProvider,
				DryRun:            flagDryRun,
				Concurrency:       flagConcurrency,
			}
			return executeSync(opts, flagHeadless)
		},
	}

	syncCmd.Flags().StringVarP(&flagSource, "source", "s", "", "Spotify playlist URL, CSV path, or playlist ID")
	syncCmd.Flags().StringVarP(&flagTarget, "target", "t", "", "YouTube Music playlist URL or ID")
	syncCmd.Flags().IntVar(&flagOffset, "offset", 0, "Starting track index offset (default: 0)")
	syncCmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum number of tracks to import (default: 0 for all)")
	syncCmd.Flags().Float64Var(&flagThreshold, "threshold", 0.95, "Confidence threshold (0.50 to 1.0, default: 0.95)")
	syncCmd.Flags().StringVar(&flagAIProvider, "ai-provider", "antigravity", "AI Referee provider: antigravity, gemini, openai, claude, ollama, mock")
	syncCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Simulate matching without inserting tracks into YouTube Music")
	syncCmd.Flags().BoolVar(&flagHeadless, "headless", false, "Disable interactive TUI and output plain text logs")
	syncCmd.Flags().StringVar(&flagReportFile, "report", "import_report.md", "Path to save markdown summary report")
	syncCmd.Flags().IntVarP(&flagConcurrency, "concurrency", "c", 5, "Number of concurrent tracks to process simultaneously (default: 5)")
	syncCmd.Flags().StringVar(&flagGeminiKey, "gemini-api-key", "", "Google Gemini API Key")
	syncCmd.Flags().StringVar(&flagOpenAIKey, "openai-api-key", "", "OpenAI API Key")
	syncCmd.Flags().BoolVar(&flagRetry, "retry", false, "Reuse all parameters and inputs from the previous session")

	// Test-Match command to inspect candidate scoring for any track
	testMatchCmd := &cobra.Command{
		Use:   "test-match <title> <artist>",
		Short: "Inspect candidate search and 95% scoring for a single track",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]
			artist := args[1]
			return executeTestMatch(title, artist)
		},
	}

	var (
		flagResetSpotify bool
		flagResetToken   bool
		flagResetYTM     bool
		flagResetAll     bool
		flagResetHistory bool
	)

	// Config command
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Show or modify configuration and paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			p, _ := config.ConfigFilePath()

			if flagResetAll {
				_ = os.Remove(p)
				fmt.Println(tui.StyleSuccessText.Render("✓ Configuration file deleted: " + p))
				return nil
			}

			if flagResetSpotify {
				cfg.SpotifyClientID = ""
				cfg.SpotifyClientSecret = ""
				cfg.SpotifyAccessToken = ""
				_ = cfg.Save()
				fmt.Println(tui.StyleSuccessText.Render("✓ Reset all Spotify credentials (client ID, secret, and access token)."))
				return nil
			}

			if flagResetToken {
				cfg.SpotifyAccessToken = ""
				_ = cfg.Save()
				fmt.Println(tui.StyleSuccessText.Render("✓ Cleared Spotify bearer access token from config."))
				return nil
			}

			if flagResetYTM {
				cfg.YTMCookie = ""
				_ = cfg.Save()
				fmt.Println(tui.StyleSuccessText.Render("✓ Cleared YouTube Music session cookie from config."))
				return nil
			}

			if flagResetHistory {
				cfg.ClearLastSession()
				_ = cfg.Save()
				fmt.Println(tui.StyleSuccessText.Render("✓ Cleared all saved session inputs and retry history."))
				return nil
			}

			fmt.Printf("Config File: %s\n", p)
			fmt.Printf("Strict Threshold: %.0f%%\n", cfg.StrictThreshold*100)
			fmt.Printf("AI Provider: %s\n", cfg.AIProvider)
			fmt.Printf("Antigravity CLI: %s\n", cfg.AntigravityPath)
			fmt.Printf("Spotify Client ID set: %v\n", cfg.SpotifyClientID != "")
			fmt.Printf("Spotify Token set: %v\n", cfg.SpotifyAccessToken != "")
			fmt.Printf("YTM Cookie set: %v\n", cfg.YTMCookie != "")
			fmt.Printf("Gemini API Key set: %v\n", cfg.GeminiAPIKey != "")
			fmt.Printf("OpenAI API Key set: %v\n", cfg.OpenAIAPIKey != "")

			if cfg.HasLastSession() {
				fmt.Println()
				fmt.Println(tui.StyleHeader.Render("Last Session Inputs (for Retry):"))
				if cfg.LastSourceMethod != "" {
					fmt.Printf("  Method:          %s\n", cfg.LastSourceMethod)
				}
				if cfg.LastSourcePlaylist != "" {
					fmt.Printf("  Source Playlist: %s\n", cfg.LastSourcePlaylist)
				}
				if cfg.LastFilePath != "" {
					fmt.Printf("  File Path:       %s\n", cfg.LastFilePath)
				}
				if cfg.LastSpotifyURL != "" {
					fmt.Printf("  Spotify URL:     %s\n", cfg.LastSpotifyURL)
				}
				if cfg.LastTargetPlaylist != "" {
					fmt.Printf("  Target Playlist: %s\n", cfg.LastTargetPlaylist)
				}
				fmt.Printf("  Offset / Limit:  %d / %d\n", cfg.LastOffset, cfg.LastLimit)
				if cfg.LastStrictThreshold > 0 {
					fmt.Printf("  Threshold:       %.0f%%\n", cfg.LastStrictThreshold*100)
				}
				if cfg.LastEnableAI != nil {
					fmt.Printf("  AI Referee:      %v (%s)\n", *cfg.LastEnableAI, cfg.LastAIProvider)
				}
				fmt.Printf("  Dry Run:         %v\n", cfg.LastDryRun)
			}
			return nil
		},
	}
	configCmd.Flags().BoolVar(&flagResetSpotify, "reset-spotify", false, "Reset all Spotify credentials (client ID, secret, and bearer token)")
	configCmd.Flags().BoolVar(&flagResetToken, "reset-token", false, "Clear only the Spotify bearer access token")
	configCmd.Flags().BoolVar(&flagResetYTM, "reset-ytm", false, "Clear YouTube Music session cookie")
	configCmd.Flags().BoolVar(&flagResetHistory, "reset-history", false, "Clear all saved session inputs, playlists, and retry history")
	configCmd.Flags().BoolVar(&flagResetAll, "reset-all", false, "Delete entire config file and reset all settings")


	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number of yt-import",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("yt-import v%s (Go 1.27)\n", version)
		},
	}

	rootCmd.AddCommand(syncCmd, testMatchCmd, configCmd, versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runInteractive() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	opts, err := tui.RunWizard(cfg)
	if err != nil {
		return err
	}

	return executeSync(*opts, false)
}

func executeSync(opts domain.SyncOptions, headless bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if flagGeminiKey != "" {
		cfg.GeminiAPIKey = flagGeminiKey
	}
	if flagOpenAIKey != "" {
		cfg.OpenAIAPIKey = flagOpenAIKey
	}
	if opts.Concurrency <= 0 {
		if cfg.Concurrency > 0 {
			opts.Concurrency = cfg.Concurrency
		} else {
			opts.Concurrency = 5
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT/SIGTERM gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// 1. Initialize Source Client (check if source is local CSV/TXT file, otherwise Spotify)
	var spotifySource provider.SourceProvider
	cleanSource := strings.Trim(strings.TrimSpace(opts.SourcePlaylistID), `"'`)

	// Persist session parameters to config so future retries have the latest values
	cfg.LastSourcePlaylist = cleanSource
	cfg.LastTargetPlaylist = opts.TargetPlaylistID
	cfg.LastOffset = opts.Offset
	cfg.LastLimit = opts.Limit
	cfg.LastStrictThreshold = opts.Threshold
	cfg.LastEnableAI = &opts.AIRefereeEnabled
	cfg.LastAIProvider = opts.AIRefereeProvider
	cfg.LastDryRun = opts.DryRun
	if strings.HasSuffix(strings.ToLower(cleanSource), ".csv") || strings.HasSuffix(strings.ToLower(cleanSource), ".txt") || isExistingFile(cleanSource) {
		cfg.LastFilePath = cleanSource
		cfg.LastSourceMethod = "file_path"
	} else if strings.Contains(cleanSource, "spotify.com") {
		cfg.LastSpotifyURL = cleanSource
		cfg.LastSourceMethod = "spotify_url"
	}
	_ = cfg.Save()

	if strings.HasSuffix(strings.ToLower(cleanSource), ".csv") || strings.HasSuffix(strings.ToLower(cleanSource), ".txt") || isExistingFile(cleanSource) {
		spotifySource = file.NewClient(cleanSource)
	} else if tui.HasSpotifyCredentials(cfg) {
		if err := spotify.ValidateCredentials(ctx, cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.SpotifyAccessToken); err != nil {
			fmt.Println(tui.BadgeDisqualified.Render("EXPIRED / INVALID") + " Stored Spotify credentials failed: " + err.Error())
			if !headless {
				_ = tui.PromptSpotifyCredentials(cfg)
			}
		}
		spotifyHTTP, err := spotify.AuthenticateClient(ctx, cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.SpotifyAccessToken)
		if err == nil {
			client := spotify.NewClient(spotifyHTTP)
			// Validate that token actually works before proceeding
			if _, testErr := client.GetPlaylist(ctx, opts.SourcePlaylistID); testErr != nil && strings.Contains(strings.ToLower(testErr.Error()), "token") {
				fmt.Println(tui.BadgeDisqualified.Render("EXPIRED TOKEN") + " Stored Spotify access token is invalid or expired.")
				if cfg.SpotifyAccessToken != "" {
					cfg.SpotifyAccessToken = ""
					_ = cfg.Save()
					fmt.Println(tui.StyleLogReason.Render("  Cleared expired bearer token from config."))
				}
				// If Client ID & Secret exist, retry with client credentials flow
				if cfg.SpotifyClientID != "" && cfg.SpotifyClientSecret != "" {
					if spotifyHTTP2, err2 := spotify.AuthenticateClient(ctx, cfg.SpotifyClientID, cfg.SpotifyClientSecret, ""); err2 == nil {
						spotifySource = spotify.NewClient(spotifyHTTP2)
					}
				}
				if spotifySource == nil {
					fmt.Println(tui.StyleLogReason.Render("  Falling back to Public Spotify Mode (100-track preview)."))
					spotifySource = spotify.NewPublicClient()
				}
			} else {
				spotifySource = client
			}
		}
	}
	if spotifySource == nil {
		// Zero-Creds Public Mode: directly extracts tracks from public Spotify playlist embed
		spotifySource = spotify.NewPublicClient()
	}


	// Validate destination playlist format
	if opts.TargetPlaylistID != "" {
		if strings.Contains(opts.TargetPlaylistID, "watch?") && !strings.Contains(opts.TargetPlaylistID, "list=") {
			return fmt.Errorf("invalid destination playlist: '%s' is a single song URL, not a playlist. Please provide a playlist link containing 'list=PL...'", opts.TargetPlaylistID)
		}
	}

	// Pre-flight validation: if writing to YouTube Music, verify cookie actually works
	if !opts.DryRun {
		var valid bool
		var accountName string
		if cfg.YTMCookie != "" {
			testYTM := ytmusic.NewClient(cfg.YTMCookie)
			name, err := testYTM.ValidateAuth(ctx)
			if err == nil {
				valid = true
				accountName = name
			}
		}

		if !valid {
			if !headless {
				if cfg.YTMCookie != "" {
					fmt.Println(tui.BadgeDisqualified.Render("EXPIRED COOKIE") + " Stored YouTube Music session cookie is invalid or has expired.")
				}
				if err := tui.PromptYTMCookie(cfg, &opts.DryRun); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("youtube music authentication error: no valid YTM_COOKIE session provided. Run './yt-import' to re-authenticate or set YTM_COOKIE")
			}
		} else if accountName != "" && accountName != "Active Session" && !headless {
			fmt.Println(tui.StyleSuccessText.Render(fmt.Sprintf("✓ YouTube Music session verified (Logged in as: %s)", accountName)))
		}
	}

	// 2. Initialize YouTube Music Target Client
	ytmTarget := ytmusic.NewClient(cfg.YTMCookie)

	// 3. Initialize AI Referee
	var aiRef referee.Referee
	if opts.AIRefereeEnabled {
		refProvider := opts.AIRefereeProvider
		if refProvider == "" {
			refProvider = cfg.AIProvider
		}
		aiRef, err = referee.NewReferee(refProvider, cfg)
		if err != nil {
			fmt.Printf("Warning: Failed to initialize AI referee (%v). Running heuristic-only.\n", err)
		}
	}

	// 4. Initialize Matching Engine
	engine := matcher.NewEngine(ytmTarget, aiRef, opts.Threshold)

	// 5. Initialize Syncer
	syncCoordinator := syncer.NewSyncer(spotifySource, ytmTarget, engine, opts)

	eventChan := make(chan syncer.SyncEvent, 100)

	if headless {
		// Run headless console logging
		go func() {
			for ev := range eventChan {
				switch ev.Type {
				case syncer.EventTrackMatched:
					fmt.Printf("[MATCH] %s\n", ev.Message)
				case syncer.EventTrackSkipped:
					fmt.Printf("[SKIPPED] %s\n", ev.Message)
				case syncer.EventError:
					fmt.Fprintf(os.Stderr, "[ERROR] %s\n", ev.Message)
				case syncer.EventComplete:
					fmt.Printf("[COMPLETE] %s\n", ev.Message)
				}
			}
		}()

		report, err := syncCoordinator.Run(ctx, eventChan)
		close(eventChan)
		if err != nil {
			return err
		}
		fmt.Print(tui.RenderSummary(report))
		_ = tui.ExportMarkdownReport(report, flagReportFile)
		if spotifySource.Name() == "Spotify (Public / Zero-Creds)" && report.ProcessedTracks >= 100 {
			fmt.Println()
			fmt.Println(tui.StyleBannerBox.Render(
				"💡 Public Mode Notice: Spotify's public embed page only provides the first 100 tracks.\n" +
					"To import your ENTIRE playlist in 100-track batches (no 100-song ceiling):\n" +
					"1. Create a free app at https://developer.spotify.com/dashboard\n" +
					"2. Run 'yt-import' and choose 'Full Playlist', or set SPOTIFY_CLIENT_ID / SPOTIFY_CLIENT_SECRET."))
		}
		return nil
	}

	// Interactive Bubble Tea Dashboard
	progressModel := tui.NewProgressModel(eventChan)
	p := tea.NewProgram(progressModel)

	var report *syncer.SyncReport
	var runErr error

	go func() {
		report, runErr = syncCoordinator.Run(ctx, eventChan)
		close(eventChan)
	}()

	if _, err := p.Run(); err != nil {
		return err
	}

	if runErr != nil {
		return runErr
	}

	if report != nil {
		fmt.Print(tui.RenderSummary(report))
		_ = tui.ExportMarkdownReport(report, flagReportFile)
		fmt.Printf("Exported migration report to %s\n", flagReportFile)

		if spotifySource.Name() == "Spotify (Public / Zero-Creds)" && report.ProcessedTracks >= 100 {
			fmt.Println()
			fmt.Println(tui.StyleBannerBox.Render(
				"💡 Public Mode Notice: Spotify's public embed page only provides the first 100 tracks.\n" +
					"To import your ENTIRE playlist in 100-track batches (no 100-song ceiling):\n" +
					"1. Create a free app at https://developer.spotify.com/dashboard\n" +
					"2. Run 'yt-import' and choose 'Full Playlist', or set SPOTIFY_CLIENT_ID / SPOTIFY_CLIENT_SECRET."))
		}
	}

	return nil
}


func executeTestMatch(title, artist string) error {
	ctx := context.Background()
	ytm := ytmusic.NewClient("")

	track := domain.Track{
		Title:   title,
		Artists: []string{artist},
	}

	fmt.Println(tui.StyleHeader.Render(fmt.Sprintf("Querying YouTube Music for: %s - %s", artist, title)))

	candidates, err := ytm.Search(ctx, fmt.Sprintf("%s %s", artist, title))
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(candidates) == 0 {
		fmt.Println(tui.BadgeSkipped.Render("NO CANDIDATES RETURNED"))
		return nil
	}

	fmt.Printf("\nFound %d candidates:\n\n", len(candidates))

	for i, c := range candidates {
		candCopy := c
		score, disq, reason := matcher.ScoreCandidate(track, &candCopy)

		status := fmt.Sprintf("%.1f%%", score*100)
		if disq {
			status = fmt.Sprintf("DISQUALIFIED (%s)", reason)
		}

		typeBadge := string(c.VideoType)
		if c.VideoType == domain.TypeAudioTrackVideo {
			typeBadge = "OFFICIAL AUDIO (ATV)"
		}

		fmt.Printf("[%d] %s\n", i, tui.StyleLogSong.Render(c.Title))
		fmt.Printf("    VideoID:   https://music.youtube.com/watch?v=%s\n", c.VideoID)
		fmt.Printf("    Channel:   %s\n", c.ChannelTitle)
		fmt.Printf("    Duration:  %s\n", c.FormattedDuration())
		fmt.Printf("    Type:      %s\n", typeBadge)
		fmt.Printf("    Scores:    Title: %.1f%% | Artist: %.1f%% | Duration: %.1f%% | Composite: %s\n\n",
			candCopy.TitleScore*100, candCopy.ArtistScore*100, candCopy.DurationScore*100, status)
	}

	// Evaluate with Engine
	engine := matcher.NewEngine(ytm, referee.NewMockReferee(), 0.95)
	res, _ := engine.Match(ctx, track)

	fmt.Println(tui.StyleBannerBox.Render(
		fmt.Sprintf("Engine Verdict: %s\nConfidence: %.1f%%\nReason: %s",
			res.Decision, res.Confidence*100, res.Reason),
	))

	return nil
}

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

