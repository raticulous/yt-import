package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"yt-import/internal/domain"
	"yt-import/internal/syncer"
)

// RenderSummary displays a formatted summary in the terminal.
func RenderSummary(report *syncer.SyncReport) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(StyleBannerBox.Render(
		fmt.Sprintf("%s\n%s",
			StyleHeader.Render("IMPORT REPORT SUMMARY"),
			StyleSubtitle.Render(fmt.Sprintf("Completed in %s | Started at %s", report.Duration.Round(time.Second), report.StartTime.Format(time.Kitchen))),
		),
	))
	sb.WriteString("\n")

	matchedTotal := report.DirectMatches + report.AIRefereeMatches

	summaryContent := fmt.Sprintf(
		"Total Tracks in Slice: %d (Offset: %d, Limit: %d)\n"+
			"Successfully Matched:   %d (Direct: %d, AI Referee: %d)\n"+
			"Already in Target YT:   %d (Fast Skipped)\n"+
			"Skipped (< 95%% conf):  %d\n",
		report.ProcessedTracks, report.Offset, report.Limit,
		matchedTotal, report.DirectMatches, report.AIRefereeMatches,
		report.AlreadyPresentTracks,
		report.SkippedTracks,
	)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSuccess).
		Padding(1, 2)

	sb.WriteString(boxStyle.Render(summaryContent))
	sb.WriteString("\n\n")

	if report.SkippedTracks > 0 {
		sb.WriteString(StyleHeader.Render(fmt.Sprintf("Skipped Tracks (%d):", report.SkippedTracks)))
		sb.WriteString("\n")
		count := 0
		for _, r := range report.Results {
			if r.Decision == domain.DecisionSkippedThreshold || r.Decision == domain.DecisionDisqualified || r.Decision == domain.DecisionNoCandidates {
				sb.WriteString(fmt.Sprintf("  %s %s - %s: %s\n",
					BadgeSkipped.Render("SKIPPED"),
					r.SourceTrack.PrimaryArtist(),
					r.SourceTrack.Title,
					StyleLogReason.Render(r.Reason),
				))
				count++
				if count >= 10 {
					sb.WriteString(fmt.Sprintf("  ... and %d more skipped tracks (see import_report.md)\n", report.SkippedTracks-count))
					break
				}
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ExportMarkdownReport generates a markdown file summarizing matched and skipped songs.
func ExportMarkdownReport(report *syncer.SyncReport, filename string) error {
	if filename == "" {
		filename = "import_report.md"
	}

	var sb strings.Builder
	sb.WriteString("# yt-import Migration Report\n\n")
	sb.WriteString(fmt.Sprintf("- **Date**: %s\n", report.StartTime.Format(time.RFC1123)))
	sb.WriteString(fmt.Sprintf("- **Source Playlist**: `%s`\n", report.SourcePlaylistID))
	sb.WriteString(fmt.Sprintf("- **Target Playlist**: `%s`\n", report.TargetPlaylistID))
	sb.WriteString(fmt.Sprintf("- **Offset / Limit**: `%d / %d`\n", report.Offset, report.Limit))
	sb.WriteString(fmt.Sprintf("- **Total Processed**: `%d`\n", report.ProcessedTracks))
	sb.WriteString(fmt.Sprintf("- **Matched**: `%d` (Direct: `%d`, AI Referee: `%d`)\n",
		report.DirectMatches+report.AIRefereeMatches, report.DirectMatches, report.AIRefereeMatches))
	sb.WriteString(fmt.Sprintf("- **Already in Target YT**: `%d` (Fast Skipped)\n", report.AlreadyPresentTracks))
	sb.WriteString(fmt.Sprintf("- **Skipped**: `%d`\n\n", report.SkippedTracks))

	sb.WriteString("## Matched Tracks (>=95% Precision)\n\n")
	sb.WriteString("| Source Title | Source Artist | YouTube Match | Duration | Video ID | Conf | Decision |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")

	var unmatchedList []domain.Track

	for _, r := range report.Results {
		if r.Decision == domain.DecisionAutoMatch || r.Decision == domain.DecisionAIRefereeMatch {
			candTitle := ""
			candDur := ""
			candID := ""
			if r.Candidate != nil {
				candTitle = r.Candidate.Title
				candDur = r.Candidate.FormattedDuration()
				candID = r.Candidate.VideoID
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | [`%s`](https://music.youtube.com/watch?v=%s) | %.1f%% | `%s` |\n",
				escapePipes(r.SourceTrack.Title),
				escapePipes(r.SourceTrack.PrimaryArtist()),
				escapePipes(candTitle),
				candDur,
				candID,
				candID,
				r.Confidence*100,
				r.Decision,
			))
		} else {
			unmatchedList = append(unmatchedList, r.SourceTrack)
		}
	}

	if len(unmatchedList) > 0 {
		sb.WriteString("\n## Skipped Tracks (<95% Precision Standard)\n\n")
		sb.WriteString("| Source Title | Source Artist | Reason |\n")
		sb.WriteString("| :--- | :--- | :--- |\n")
		for _, r := range report.Results {
			if r.Decision != domain.DecisionAutoMatch && r.Decision != domain.DecisionAIRefereeMatch {
				sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
					escapePipes(r.SourceTrack.Title),
					escapePipes(r.SourceTrack.PrimaryArtist()),
					escapePipes(r.Reason),
				))
			}
		}

		// Also export unmatched.json
		unmatchedData, _ := json.MarshalIndent(unmatchedList, "", "  ")
		_ = os.WriteFile(filepath.Join(filepath.Dir(filename), "unmatched.json"), unmatchedData, 0644)
	}

	return os.WriteFile(filename, []byte(sb.String()), 0644)
}

func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
