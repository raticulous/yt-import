package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Color Palette (Tokyo Night / Catppuccin hybrid)
	ColorPrimary   = lipgloss.Color("#A78BFA") // Lavender / Purple
	ColorSecondary = lipgloss.Color("#38BDF8") // Electric Cyan
	ColorSuccess   = lipgloss.Color("#34D399") // Emerald Green
	ColorWarning   = lipgloss.Color("#FBBF24") // Amber
	ColorDanger    = lipgloss.Color("#F87171") // Coral Red
	ColorMuted     = lipgloss.Color("#94A3B8") // Slate Gray
	ColorDarkBg    = lipgloss.Color("#1E1E2E") // Charcoal

	// Typography & Layout
	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Padding(0, 1)

	StyleSubtitle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	StyleBannerBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2).
			MarginBottom(1)

	StyleStatBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSecondary).
			Padding(0, 1).
			MarginRight(1)

	StyleStatLabel = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Bold(true)

	StyleStatValue = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF"))

	// Badges
	BadgeAutoMatch = lipgloss.NewStyle().
			Bold(true).
			Background(ColorSuccess).
			Foreground(ColorDarkBg).
			Padding(0, 1)

	BadgeAIReferee = lipgloss.NewStyle().
			Bold(true).
			Background(ColorSecondary).
			Foreground(ColorDarkBg).
			Padding(0, 1)

	BadgeSkipped = lipgloss.NewStyle().
			Bold(true).
			Background(ColorWarning).
			Foreground(ColorDarkBg).
			Padding(0, 1)

	BadgeDisqualified = lipgloss.NewStyle().
				Bold(true).
				Background(ColorDanger).
				Foreground(ColorDarkBg).
				Padding(0, 1)

	BadgeOffsetLimit = lipgloss.NewStyle().
				Bold(true).
				Background(ColorPrimary).
				Foreground(ColorDarkBg).
				Padding(0, 1)

	BadgeBatch = lipgloss.NewStyle().
			Bold(true).
			Background(ColorPrimary).
			Foreground(ColorDarkBg).
			Padding(0, 1)


	StyleLogSong = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF"))

	StyleLogReason = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	StyleSuccessText = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorSuccess)
)

const AppBanner = `
  __   ___    _                            _   
  \ \ / / |_ (_)_ __ ___  _ __   ___  _ __| |_ 
   \ V /| __|| | '_ ' _ \| '_ \ / _ \| '__| __|
    | | | |_ | | | | | | | |_) | (_) | |  | |_ 
    |_|  \__||_|_| |_| |_| .__/ \___/|_|   \__|
                         |_|                   
`
