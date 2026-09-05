package tui

import (
	"fmt"
	"strings"

	"yt-import/internal/domain"
	"yt-import/internal/syncer"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type logEntry struct {
	badge   string
	title   string
	artist  string
	details string
}

// ProgressModel represents the live sync dashboard.
type ProgressModel struct {
	spinner   spinner.Model
	progress  progress.Model
	events    <-chan syncer.SyncEvent
	lastState syncer.SyncProgress
	logs      []logEntry
	done      bool
	err       error
	report    *syncer.SyncReport
	width     int
	height    int
}

// NewProgressModel constructs the progress dashboard.
func NewProgressModel(events <-chan syncer.SyncEvent) ProgressModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorSecondary)

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(48),
	)

	return ProgressModel{
		spinner:  s,
		progress: p,
		events:   events,
		width:    80,
		height:   24,
	}
}

func (m ProgressModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		waitForEvent(m.events),
	)
}

func waitForEvent(ch <-chan syncer.SyncEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return nil
		}
		return event
	}
}

func (m ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 20
		if m.progress.Width > 60 {
			m.progress.Width = 60
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case syncer.SyncEvent:
		m.lastState = msg.Progress

		switch msg.Type {
		case syncer.EventTrackMatched:
			res := msg.Progress.LastResult
			badge := BadgeAutoMatch.Render("AUTO 95%+")
			if res.Decision == domain.DecisionAIRefereeMatch {
				badge = BadgeAIReferee.Render("AI REFEREE")
			}
			m.addLog(logEntry{
				badge:   badge,
				title:   res.SourceTrack.Title,
				artist:  res.SourceTrack.PrimaryArtist(),
				details: fmt.Sprintf("-> %s (Conf: %.1f%%)", res.Candidate.Title, res.Confidence*100),
			})

		case syncer.EventTrackSkipped:
			res := msg.Progress.LastResult
			badgeText := "SKIPPED"
			if res != nil && res.Decision == domain.DecisionAlreadyExists {
				badgeText = "EXISTS"
			}
			title := ""
			artist := ""
			details := msg.Message
			if res != nil {
				title = res.SourceTrack.Title
				artist = res.SourceTrack.PrimaryArtist()
				details = res.Reason
			}
			m.addLog(logEntry{
				badge:   BadgeSkipped.Render(badgeText),
				title:   title,
				artist:  artist,
				details: details,
			})

		case syncer.EventBatchInserted:
			m.addLog(logEntry{
				badge:   BadgeBatch.Render("BATCH SAVED"),
				title:   fmt.Sprintf("Batch %d/%d", msg.Progress.CurrentBatch, msg.Progress.TotalBatches),
				artist:  "YouTube Music",
				details: msg.Message,
			})

		case syncer.EventError:
			m.addLog(logEntry{
				badge:   BadgeDisqualified.Render("ERROR"),
				title:   "Sync Error",
				details: msg.Message,
			})

		case syncer.EventComplete:
			m.done = true
			return m, tea.Quit
		}

		return m, waitForEvent(m.events)
	}

	return m, nil
}

func (m *ProgressModel) addLog(entry logEntry) {
	m.logs = append(m.logs, entry)
	if len(m.logs) > 8 {
		m.logs = m.logs[len(m.logs)-8:]
	}
}

func (m ProgressModel) View() string {
	var sb strings.Builder

	// Header banner
	sb.WriteString(StyleHeader.Render("YT-IMPORT: High-Precision Spotify -> YouTube Music"))
	sb.WriteString("\n")
	sb.WriteString(StyleSubtitle.Render("Strict 95%+ Matching Standard with AI LLM Disambiguation Referee"))
	sb.WriteString("\n\n")

	// Stats Row
	total := m.lastState.TotalTracks
	processed := m.lastState.Processed
	pct := 0.0
	if total > 0 {
		pct = float64(processed) / float64(total)
	}

	statTotal := StyleStatBox.Render(fmt.Sprintf("%s\n%s", StyleStatLabel.Render("Processed"), StyleStatValue.Render(fmt.Sprintf("%d / %d", processed, total))))
	statDirect := StyleStatBox.Render(fmt.Sprintf("%s\n%s", StyleStatLabel.Render("Auto Match"), StyleStatValue.Render(fmt.Sprintf("%d", m.lastState.DirectMatches))))
	statAI := StyleStatBox.Render(fmt.Sprintf("%s\n%s", StyleStatLabel.Render("AI Referee"), StyleStatValue.Render(fmt.Sprintf("%d", m.lastState.AIRefereeMatches))))
	statSkipped := StyleStatBox.Render(fmt.Sprintf("%s\n%s", StyleStatLabel.Render("Skipped (<95%)"), StyleStatValue.Render(fmt.Sprintf("%d", m.lastState.Skipped))))

	statBoxes := []string{statTotal}
	if m.lastState.TotalBatches > 1 {
		statBatch := StyleStatBox.Render(fmt.Sprintf("%s\n%s", StyleStatLabel.Render("Batch"), StyleStatValue.Render(fmt.Sprintf("%d / %d", m.lastState.CurrentBatch, m.lastState.TotalBatches))))
		statBoxes = append(statBoxes, statBatch)
	}
	statBoxes = append(statBoxes, statDirect, statAI, statSkipped)
	if m.lastState.AlreadyPresent > 0 {
		statExists := StyleStatBox.Render(fmt.Sprintf("%s\n%s", StyleStatLabel.Render("Already in YT"), StyleStatValue.Render(fmt.Sprintf("%d", m.lastState.AlreadyPresent))))
		statBoxes = append(statBoxes, statExists)
	}

	statsRow := lipgloss.JoinHorizontal(lipgloss.Top, statBoxes...)

	sb.WriteString(statsRow)
	sb.WriteString("\n\n")

	// Progress bar & Spinner
	statusText := "Processing tracks..."
	if m.done {
		statusText = "Synchronization Complete!"
	} else if m.lastState.CurrentTrack != nil {
		statusText = fmt.Sprintf("Evaluating: %s - %s", m.lastState.CurrentTrack.PrimaryArtist(), m.lastState.CurrentTrack.Title)
	}

	spin := m.spinner.View()
	if m.done {
		spin = "✓"
	}

	sb.WriteString(fmt.Sprintf("%s %s\n", spin, statusText))
	sb.WriteString(m.progress.ViewAs(pct))
	sb.WriteString(fmt.Sprintf("  %.1f%%\n\n", pct*100))

	// Live Activity Log
	sb.WriteString(StyleHeader.Render("Live Evaluation Feed:"))
	sb.WriteString("\n")
	if len(m.logs) == 0 {
		sb.WriteString(StyleLogReason.Render("  Waiting for tracks...\n"))
	} else {
		for _, l := range m.logs {
			line := fmt.Sprintf("  %s %s - %s  %s\n",
				l.badge,
				StyleLogSong.Render(l.artist),
				l.title,
				StyleLogReason.Render(l.details),
			)
			sb.WriteString(line)
		}
	}

	sb.WriteString("\n" + StyleSubtitle.Render("Press Ctrl+C to cancel at any time"))
	return sb.String()
}
