package matcher

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	// Remaster and release edition patterns
	reRemasterHyphen = regexp.MustCompile(`(?i)\s*-\s*(\d{4}\s*)?remaster(ed)?(\s*\d{4})?.*$`)
	reRemasterParen  = regexp.MustCompile(`(?i)\s*[\(\[]\s*(\d{4}\s*)?remaster(ed)?(\s*\d{4})?\s*[\)\]]`)
	reAnniversary    = regexp.MustCompile(`(?i)\s*[\(\[]\s*(\d{4}\s*)?anniversary(\s*(edition|mix|remaster))?\s*[\)\]]`)
	reDeluxeEdition  = regexp.MustCompile(`(?i)\s*[\(\[]\s*(deluxe|expanded|bonus\s*track|special)\s*(edition|version)?\s*[\)\]]`)

	// YouTube video / audio noise patterns
	reVideoNoiseParen  = regexp.MustCompile(`(?i)\s*[\(\[]\s*(official\s*(music\s*)?(audio|video|lyric\s*video)|audio|official|hq|hd|4k|lyrics|visualizer|clip\s*officiel)\s*[\)\]]`)
	reVideoNoiseHyphen = regexp.MustCompile(`(?i)\s*-\s*(official\s*(music\s*)?(audio|video|lyric\s*video)|audio|lyrics).*$`)

	// Soundtrack tags
	reOSTParen  = regexp.MustCompile(`(?i)\s*[\(\[]\s*(from|music\s*from)\s+.*?["']?.*?(soundtrack|ost|motion\s*picture)?\s*[\)\]]`)
	reOSTHyphen = regexp.MustCompile(`(?i)\s*-\s*(from|music\s*from)\s+.*$`)

	// Featuring extraction
	reFeatParen  = regexp.MustCompile(`(?i)[\(\[]\s*(?:feat\.?|featuring|with|ft\.?)\s+([^()\[\]]+)[\)\]]`)
	reFeatInline = regexp.MustCompile(`(?i)\s+(?:feat\.?|featuring|ft\.?)\s+([^-()\[\]]+)`)
	reWithHyphen = regexp.MustCompile(`(?i)\s*-\s*with\s+.*$`)

	// Version modifiers
	reLive         = regexp.MustCompile(`(?i)\b(live(\s+(at|in|from|version))?)\b`)
	reAcoustic     = regexp.MustCompile(`(?i)\b(acoustic|unplugged|stripped)\b`)
	reRemix        = regexp.MustCompile(`(?i)\b(remix|club\s*mix|extended\s*(mix|version)|dub\s*mix|vip\s*mix)\b`)
	reInstrumental = regexp.MustCompile(`(?i)\b(instrumental|karaoke|backing\s*track)\b`)
	reCover        = regexp.MustCompile(`(?i)\b(cover|tribute|style\s*of)\b`)
	reRadioEdit    = regexp.MustCompile(`(?i)\b(radio\s*edit|single\s*version)\b`)
	reFanEdits     = regexp.MustCompile(`(?i)\b(slowed(\s*\+\s*reverb)?|nightcore|8d\s*audio|speed\s*up)\b`)
)

// StripAccents removes diacritics and converts non-ASCII latin to standard ASCII.
func StripAccents(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, err := transform.String(t, s)
	if err != nil {
		return s
	}
	return result
}

// CleanTitle removes noise (remaster tags, video noise, soundtrack info) from song titles.
func CleanTitle(title string) string {
	s := title
	s = reRemasterHyphen.ReplaceAllString(s, "")
	s = reRemasterParen.ReplaceAllString(s, "")
	s = reAnniversary.ReplaceAllString(s, "")
	s = reDeluxeEdition.ReplaceAllString(s, "")
	s = reVideoNoiseParen.ReplaceAllString(s, "")
	s = reVideoNoiseHyphen.ReplaceAllString(s, "")
	s = reOSTParen.ReplaceAllString(s, "")
	s = reOSTHyphen.ReplaceAllString(s, "")
	s = reFeatParen.ReplaceAllString(s, "")
	s = reFeatInline.ReplaceAllString(s, "")
	s = reWithHyphen.ReplaceAllString(s, "")

	s = StripAccents(s)
	s = strings.TrimSpace(s)
	return s
}

// ExtractFeaturedArtists extracts artist names mentioned in title features.
func ExtractFeaturedArtists(title string) []string {
	var artists []string
	if matches := reFeatParen.FindStringSubmatch(title); len(matches) > 1 {
		parts := strings.Split(matches[1], ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				artists = append(artists, trimmed)
			}
		}
	}
	if matches := reFeatInline.FindStringSubmatch(title); len(matches) > 1 {
		parts := strings.Split(matches[1], ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				artists = append(artists, trimmed)
			}
		}
	}
	return artists
}

// CleanArtist normalizes artist names, removing " - Topic" and extra whitespace.
func CleanArtist(artist string) string {
	s := strings.TrimSuffix(artist, " - Topic")
	s = strings.TrimSuffix(s, "VEVO")
	s = StripAccents(s)
	return strings.TrimSpace(s)
}

// Modifiers holds flags for version variations.
type Modifiers struct {
	IsLive         bool
	IsAcoustic     bool
	IsRemix        bool
	IsInstrumental bool
	IsCover        bool
	IsRadioEdit    bool
	IsFanEdit      bool
}

// ExtractModifiers detects version modifiers present in text.
func ExtractModifiers(text string) Modifiers {
	return Modifiers{
		IsLive:         reLive.MatchString(text),
		IsAcoustic:     reAcoustic.MatchString(text),
		IsRemix:        reRemix.MatchString(text),
		IsInstrumental: reInstrumental.MatchString(text),
		IsCover:        reCover.MatchString(text),
		IsRadioEdit:    reRadioEdit.MatchString(text),
		IsFanEdit:      reFanEdits.MatchString(text),
	}
}
