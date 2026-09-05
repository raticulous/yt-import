package ytmusic

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"yt-import/internal/domain"
)

const (
	InnerTubeBaseURL     = "https://music.youtube.com/youtubei/v1"
	SongsFilterParam     = "EgWKAQIIAWoKEAMQBBAJEAo%3D"
	DefaultClientVersion = "1.20240101.01.00"
)

// InnerTubeClient handles direct HTTP interactions with the YouTube Music InnerTube API.
type InnerTubeClient struct {
	cookie string
	client *http.Client
}

// NewInnerTubeClient initializes an InnerTube client.
func NewInnerTubeClient(cookie string) *InnerTubeClient {
	return &InnerTubeClient{
		cookie: cookie,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Search queries YouTube Music for songs.
func (c *InnerTubeClient) Search(ctx context.Context, query string) ([]domain.Candidate, error) {
	url := fmt.Sprintf("%s/search", InnerTubeBaseURL)

	payload := map[string]interface{}{
		"context": map[string]interface{}{
			"client": map[string]interface{}{
				"clientName":    "WEB_REMIX",
				"clientVersion": DefaultClientVersion,
				"hl":            "en",
				"gl":            "US",
			},
		},
		"query":  query,
		"params": SongsFilterParam, // Focus on official songs
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("innertube search error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var data map[string]interface{}
	if err := json.Unmarshal(respBytes, &data); err != nil {
		return nil, err
	}

	return parseSearchCandidates(data), nil
}

// GetPlaylistTracks fetches all tracks currently inside a YouTube Music playlist.
func (c *InnerTubeClient) GetPlaylistTracks(ctx context.Context, playlistID string) ([]domain.Candidate, error) {
	cleanID := ExtractPlaylistID(playlistID)
	if cleanID == "" {
		return nil, fmt.Errorf("invalid or empty YouTube Music playlist ID: '%s'", playlistID)
	}
	if !strings.HasPrefix(cleanID, "VL") {
		cleanID = "VL" + cleanID
	}

	url := fmt.Sprintf("%s/browse", InnerTubeBaseURL)

	payload := map[string]interface{}{
		"context": map[string]interface{}{
			"client": map[string]interface{}{
				"clientName":    "WEB_REMIX",
				"clientVersion": DefaultClientVersion,
				"hl":            "en",
				"gl":            "US",
			},
		},
		"browseId": cleanID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get playlist tracks (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var data map[string]interface{}
	if err := json.Unmarshal(respBytes, &data); err != nil {
		return nil, err
	}

	allTracks := parseSearchCandidates(data)
	token := extractContinuationToken(data)

	// Follow continuation tokens to fetch all tracks across pages
	seenTokens := make(map[string]bool)
	for token != "" && !seenTokens[token] {
		seenTokens[token] = true

		select {
		case <-ctx.Done():
			return allTracks, ctx.Err()
		default:
		}

		contURL := fmt.Sprintf("%s/browse?continuation=%s", InnerTubeBaseURL, token)
		contPayload := map[string]interface{}{
			"context": map[string]interface{}{
				"client": map[string]interface{}{
					"clientName":    "WEB_REMIX",
					"clientVersion": DefaultClientVersion,
					"hl":            "en",
					"gl":            "US",
				},
			},
		}

		cBody, err := json.Marshal(contPayload)
		if err != nil {
			break
		}

		cReq, err := http.NewRequestWithContext(ctx, "POST", contURL, bytes.NewReader(cBody))
		if err != nil {
			break
		}
		c.setHeaders(cReq)

		cResp, err := c.client.Do(cReq)
		if err != nil {
			break
		}

		cBytes, err := io.ReadAll(cResp.Body)
		cResp.Body.Close()
		if err != nil || cResp.StatusCode != http.StatusOK {
			break
		}

		var contData map[string]interface{}
		if err := json.Unmarshal(cBytes, &contData); err != nil {
			break
		}

		pageTracks := parseSearchCandidates(contData)
		if len(pageTracks) == 0 {
			break
		}
		allTracks = append(allTracks, pageTracks...)

		token = extractContinuationToken(contData)
	}

	return allTracks, nil
}

// extractContinuationToken searches for a pagination token in YouTube Music browse responses.
func extractContinuationToken(data map[string]interface{}) string {
	var token string
	var walk func(v interface{})
	walk = func(v interface{}) {
		if token != "" {
			return
		}
		switch node := v.(type) {
		case map[string]interface{}:
			if cont, ok := node["continuationItemRenderer"].(map[string]interface{}); ok {
				if endp, ok := cont["continuationEndpoint"].(map[string]interface{}); ok {
					if cmd, ok := endp["continuationCommand"].(map[string]interface{}); ok {
						if t, ok := cmd["token"].(string); ok && t != "" {
							token = t
							return
						}
					}
					if exec, ok := endp["commandExecutorCommand"].(map[string]interface{}); ok {
						if cmds, ok := exec["commands"].([]interface{}); ok {
							for _, c := range cmds {
								if cm, ok := c.(map[string]interface{}); ok {
									if cc, ok := cm["continuationCommand"].(map[string]interface{}); ok {
										if t, ok := cc["token"].(string); ok && t != "" {
											token = t
											return
										}
									}
								}
							}
						}
					}
				}
			}
			for _, val := range node {
				walk(val)
			}
		case []interface{}:
			for _, val := range node {
				walk(val)
			}
		}
	}
	walk(data)
	return token
}


// AddTracksToPlaylist adds video IDs to an existing YouTube Music playlist in batches.
func (c *InnerTubeClient) AddTracksToPlaylist(ctx context.Context, playlistID string, videoIDs []string) error {
	if c.cookie == "" {
		return fmt.Errorf("ytm_cookie is required to modify YouTube Music playlists")
	}

	cleanID := ExtractPlaylistID(playlistID)
	if cleanID == "" {
		return fmt.Errorf("invalid playlist link '%s': appears to be a single song URL rather than a playlist (must contain 'list=' or be a playlist ID starting with 'PL')", playlistID)
	}

	url := fmt.Sprintf("%s/browse/edit_playlist", InnerTubeBaseURL)

	// Batch in chunks of 50 to prevent request payload / action count limits
	const batchSize = 50
	for i := 0; i < len(videoIDs); i += batchSize {
		end := i + batchSize
		if end > len(videoIDs) {
			end = len(videoIDs)
		}
		chunk := videoIDs[i:end]

		var actions []map[string]interface{}
		for _, vid := range chunk {
			actions = append(actions, map[string]interface{}{
				"action":       "ACTION_ADD_VIDEO",
				"addedVideoId": vid,
			})
		}

		payload := map[string]interface{}{
			"context": map[string]interface{}{
				"client": map[string]interface{}{
					"clientName":    "WEB_REMIX",
					"clientVersion": DefaultClientVersion,
					"hl":            "en",
					"gl":            "US",
				},
			},
			"playlistId": cleanID,
			"actions":    actions,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		c.setHeaders(req)

		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBytes, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == http.StatusUnauthorized || strings.Contains(string(respBytes), "UNAUTHENTICATED") {
				return fmt.Errorf("youtube music authentication error (status 401 UNAUTHENTICATED): your YouTube Music session cookie has expired or doesn't have edit permission for this playlist. Refresh your cookie from music.youtube.com (or run './yt-import.exe config --reset-ytm')")
			}
			return fmt.Errorf("failed to add tracks to playlist (status %d): %s", resp.StatusCode, string(respBytes))
		}
	}

	return nil
}

// CreatePlaylist creates a new playlist on YouTube Music.
func (c *InnerTubeClient) CreatePlaylist(ctx context.Context, title, description string) (string, error) {
	if c.cookie == "" {
		return "", fmt.Errorf("ytm_cookie is required to create a YouTube Music playlist")
	}

	url := fmt.Sprintf("%s/playlist/create", InnerTubeBaseURL)

	payload := map[string]interface{}{
		"context": map[string]interface{}{
			"client": map[string]interface{}{
				"clientName":    "WEB_REMIX",
				"clientVersion": DefaultClientVersion,
				"hl":            "en",
				"gl":            "US",
			},
		},
		"title":       title,
		"description": description,
		"privacyStatus": "PRIVATE",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || strings.Contains(string(respBytes), "UNAUTHENTICATED") {
			return "", fmt.Errorf("youtube music authentication error (status 401 UNAUTHENTICATED): your YouTube Music session cookie has expired. Refresh your cookie from music.youtube.com (or run './yt-import.exe config --reset-ytm')")
		}
		return "", fmt.Errorf("failed to create playlist (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		PlaylistID string `json:"playlistId"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", err
	}

	return result.PlaylistID, nil
}

// ValidateAuth performs a pre-flight verification to confirm that the session cookie is valid and active.
// Returns the user's account/channel name if available, or an error if unauthenticated or expired.
func (c *InnerTubeClient) ValidateAuth(ctx context.Context) (string, error) {
	if c.cookie == "" {
		return "", fmt.Errorf("no YouTube Music cookie provided")
	}

	cleanCookie := strings.TrimSpace(c.cookie)
	cleanCookie = strings.TrimPrefix(cleanCookie, "cookie: ")
	cleanCookie = strings.TrimPrefix(cleanCookie, "Cookie: ")
	cleanCookie = strings.Trim(cleanCookie, `"'`)

	sapisid := extractCookieValue(cleanCookie, "__Secure-3PAPISID")
	if sapisid == "" {
		sapisid = extractCookieValue(cleanCookie, "SAPISID")
	}
	if sapisid == "" {
		return "", fmt.Errorf("cookie is missing SAPISID or __Secure-3PAPISID (ensure you are logged into your Google account at music.youtube.com)")
	}

	// Step 1: Read-only check to browse liked playlists (requires valid authenticated session)
	url := fmt.Sprintf("%s/browse", InnerTubeBaseURL)
	payload := map[string]interface{}{
		"context": map[string]interface{}{
			"client": map[string]interface{}{
				"clientName":    "WEB_REMIX",
				"clientVersion": DefaultClientVersion,
				"hl":            "en",
				"gl":            "US",
			},
		},
		"browseId": "FEmusic_liked_playlists",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to contact YouTube Music: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	respStr := string(respBytes)

	if resp.StatusCode == http.StatusUnauthorized || strings.Contains(respStr, "UNAUTHENTICATED") {
		return "", fmt.Errorf("youtube music authentication error (status 401 UNAUTHENTICATED): your YouTube Music session cookie has expired")
	}
	if strings.Contains(respStr, "signInEndpoint") {
		return "", fmt.Errorf("youtube music authentication error: session cookie is invalid or has expired (YouTube Music requested sign-in)")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("youtube music authentication error (status %d): %s", resp.StatusCode, respStr)
	}

	// Step 2: Try to retrieve account display name from account_menu
	menuURL := fmt.Sprintf("%s/account/account_menu", InnerTubeBaseURL)
	menuPayload := map[string]interface{}{
		"context": map[string]interface{}{
			"client": map[string]interface{}{
				"clientName":    "WEB_REMIX",
				"clientVersion": DefaultClientVersion,
				"hl":            "en",
				"gl":            "US",
			},
		},
	}
	if menuBody, err := json.Marshal(menuPayload); err == nil {
		if menuReq, err := http.NewRequestWithContext(ctx, "POST", menuURL, bytes.NewReader(menuBody)); err == nil {
			c.setHeaders(menuReq)
			if menuResp, err := c.client.Do(menuReq); err == nil {
				defer menuResp.Body.Close()
				if menuBytes, err := io.ReadAll(menuResp.Body); err == nil {
					var menuData map[string]interface{}
					if err := json.Unmarshal(menuBytes, &menuData); err == nil {
						if name := extractAccountName(menuData); name != "" {
							return name, nil
						}
					}
				}
			}
		}
	}

	return "Active Session", nil
}

// extractAccountName attempts to parse the user's name or channel handle from account_menu response.
func extractAccountName(data map[string]interface{}) string {
	var name string
	var walk func(v interface{})
	walk = func(v interface{}) {
		if name != "" {
			return
		}
		switch node := v.(type) {
		case map[string]interface{}:
			if header, ok := node["activeAccountHeaderRenderer"].(map[string]interface{}); ok {
				if accName, ok := header["accountName"].(map[string]interface{}); ok {
					if runs, ok := accName["runs"].([]interface{}); ok && len(runs) > 0 {
						if r0, ok := runs[0].(map[string]interface{}); ok {
							if txt, ok := r0["text"].(string); ok {
								name = txt
								return
							}
						}
					}
				}
				if handle, ok := header["channelHandle"].(map[string]interface{}); ok {
					if runs, ok := handle["runs"].([]interface{}); ok && len(runs) > 0 {
						if r0, ok := runs[0].(map[string]interface{}); ok {
							if txt, ok := r0["text"].(string); ok {
								name = txt
								return
							}
						}
					}
				}
			}
			for _, val := range node {
				walk(val)
			}
		case []interface{}:
			for _, val := range node {
				walk(val)
			}
		}
	}
	walk(data)
	return name
}

func (c *InnerTubeClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Origin", "https://music.youtube.com")
	req.Header.Set("Referer", "https://music.youtube.com/")
	req.Header.Set("x-origin", "https://music.youtube.com")
	req.Header.Set("X-Goog-AuthUser", "0")
	req.Header.Set("X-YouTube-Client-Name", "67") // WEB_REMIX
	req.Header.Set("X-YouTube-Client-Version", DefaultClientVersion)

	if c.cookie != "" {
		cleanCookie := strings.TrimSpace(c.cookie)
		cleanCookie = strings.TrimPrefix(cleanCookie, "cookie: ")
		cleanCookie = strings.TrimPrefix(cleanCookie, "Cookie: ")
		cleanCookie = strings.Trim(cleanCookie, `"'`)
		req.Header.Set("Cookie", cleanCookie)

		// Prefer __Secure-3PAPISID for modern HTTPS, fallback to SAPISID
		sapisid := extractCookieValue(cleanCookie, "__Secure-3PAPISID")
		if sapisid == "" {
			sapisid = extractCookieValue(cleanCookie, "SAPISID")
		}
		if sapisid != "" {
			ts := strconv.FormatInt(time.Now().Unix(), 10)
			msg := fmt.Sprintf("%s %s https://music.youtube.com", ts, sapisid)
			h := sha1.Sum([]byte(msg))
			req.Header.Set("Authorization", fmt.Sprintf("SAPISIDHASH %s_%x", ts, h))
		}
	}
}

func extractCookieValue(cookie, name string) string {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, name+"=") {
			return strings.TrimPrefix(part, name+"=")
		}
	}
	return ""
}

// parseSearchCandidates extracts structured candidates from the InnerTube search response.
func parseSearchCandidates(data map[string]interface{}) []domain.Candidate {
	var candidates []domain.Candidate
	seen := make(map[string]bool)

	var walk func(v interface{})
	walk = func(v interface{}) {
		switch node := v.(type) {
		case map[string]interface{}:
			// 1. Featured Top Result Card (musicCardShelfRenderer)
			if card, ok := node["musicCardShelfRenderer"].(map[string]interface{}); ok {
				c := parseCardShelf(card)
				if c.VideoID != "" && !seen[c.VideoID] {
					seen[c.VideoID] = true
					// Always place the featured top result card at the very front
					candidates = append([]domain.Candidate{c}, candidates...)
				}
			}

			// 2. Standard Search List Items (musicResponsiveListItemRenderer)
			if item, ok := node["musicResponsiveListItemRenderer"].(map[string]interface{}); ok {
				c := parseItem(item)
				if c.VideoID != "" && !seen[c.VideoID] {
					seen[c.VideoID] = true
					candidates = append(candidates, c)
				}
			}

			for _, val := range node {
				walk(val)
			}
		case []interface{}:
			for _, val := range node {
				walk(val)
			}
		}
	}
	walk(data)
	return candidates
}

func parseCardShelf(card map[string]interface{}) domain.Candidate {
	var c domain.Candidate
	c.VideoType = domain.TypeUnknown

	// 1. Extract Title and VideoID from title runs
	if titleObj, ok := card["title"].(map[string]interface{}); ok {
		if runs, ok := titleObj["runs"].([]interface{}); ok {
			for _, r := range runs {
				if rm, ok := r.(map[string]interface{}); ok {
					if text, ok := rm["text"].(string); ok {
						c.Title += text
					}
					if c.VideoID == "" {
						if nav, ok := rm["navigationEndpoint"].(map[string]interface{}); ok {
							if wep, ok := nav["watchEndpoint"].(map[string]interface{}); ok {
								if vid, ok := wep["videoId"].(string); ok {
									c.VideoID = vid
								}
								extractVideoType(wep, &c)
							}
						}
					}
				}
			}
		}
	}

	// 2. Fallback VideoID and VideoType from onTap
	if c.VideoID == "" {
		if onTap, ok := card["onTap"].(map[string]interface{}); ok {
			if wep, ok := onTap["watchEndpoint"].(map[string]interface{}); ok {
				if vid, ok := wep["videoId"].(string); ok {
					c.VideoID = vid
				}
				extractVideoType(wep, &c)
			}
		}
	}

	// 3. Fallback VideoID and VideoType from thumbnailOverlay
	if c.VideoID == "" || c.VideoType == domain.TypeUnknown {
		if overlay, ok := card["thumbnailOverlay"].(map[string]interface{}); ok {
			if mitr, ok := overlay["musicItemThumbnailOverlayRenderer"].(map[string]interface{}); ok {
				if content, ok := mitr["content"].(map[string]interface{}); ok {
					if mpbr, ok := content["musicPlayButtonRenderer"].(map[string]interface{}); ok {
						if pne, ok := mpbr["playNavigationEndpoint"].(map[string]interface{}); ok {
							if wep, ok := pne["watchEndpoint"].(map[string]interface{}); ok {
								if c.VideoID == "" {
									if vid, ok := wep["videoId"].(string); ok {
										c.VideoID = vid
									}
								}
								extractVideoType(wep, &c)
							}
						}
					}
				}
			}
		}
	}

	// 4. Subtitle runs for Artists, Duration, and Album
	if subObj, ok := card["subtitle"].(map[string]interface{}); ok {
		if runs, ok := subObj["runs"].([]interface{}); ok {
			var textRuns []string
			for _, r := range runs {
				if rm, ok := r.(map[string]interface{}); ok {
					if text, ok := rm["text"].(string); ok {
						textRuns = append(textRuns, text)
					}
				}
			}

			combined := strings.Join(textRuns, "")
			segments := strings.Split(combined, " • ")
			for _, seg := range segments {
				segTrim := strings.TrimSpace(seg)
				if isTypeLabel(segTrim) {
					if isSongLabel(segTrim) && c.VideoType == domain.TypeUnknown {
						c.VideoType = domain.TypeAudioTrackVideo
					}
					continue
				}

				if strings.Contains(segTrim, ":") && isDurationFormat(segTrim) {
					c.DurationMs = parseDurationStringToMs(segTrim)
				} else if len(c.Artists) == 0 {
					artParts := strings.Split(segTrim, ", ")
					for _, ap := range artParts {
						apTrim := strings.TrimSpace(ap)
						if apTrim != "" {
							c.Artists = append(c.Artists, apTrim)
						}
					}
					if len(c.Artists) > 0 {
						c.ChannelTitle = c.Artists[0]
					}
				} else if c.Album == "" {
					c.Album = segTrim
				}
			}
		}
	}

	return c
}

func extractVideoType(wep map[string]interface{}, c *domain.Candidate) {
	if sc, ok := wep["watchEndpointMusicSupportedConfigs"].(map[string]interface{}); ok {
		if mc, ok := sc["watchEndpointMusicConfig"].(map[string]interface{}); ok {
			if mvt, ok := mc["musicVideoType"].(string); ok {
				switch mvt {
				case "MUSIC_VIDEO_TYPE_ATV":
					c.VideoType = domain.TypeAudioTrackVideo
				case "MUSIC_VIDEO_TYPE_OMV":
					c.VideoType = domain.TypeOfficialMusicVideo
				case "MUSIC_VIDEO_TYPE_UGC":
					c.VideoType = domain.TypeUserGenerated
				}
			}
		}
	}
}

func isTypeLabel(s string) bool {
	sLower := strings.ToLower(strings.TrimSpace(s))
	switch sLower {
	case "song", "video", "пісня", "відео", "песня", "видео", "track", "titel", "morceau", "canción", "cancion":
		return true
	default:
		return false
	}
}

func isSongLabel(s string) bool {
	sLower := strings.ToLower(strings.TrimSpace(s))
	switch sLower {
	case "song", "пісня", "песня", "track", "titel", "morceau", "canción", "cancion":
		return true
	default:
		return false
	}
}

func parseItem(item map[string]interface{}) domain.Candidate {
	var c domain.Candidate
	c.VideoType = domain.TypeUnknown

	// 1. Video ID from playlistItemData
	if pid, ok := item["playlistItemData"].(map[string]interface{}); ok {
		if vid, ok := pid["videoId"].(string); ok {
			c.VideoID = vid
		}
	}

	// 2. Video type from overlay play navigation watchEndpoint
	if overlay, ok := item["overlay"].(map[string]interface{}); ok {
		if mitr, ok := overlay["musicItemThumbnailOverlayRenderer"].(map[string]interface{}); ok {
			if content, ok := mitr["content"].(map[string]interface{}); ok {
				if mpbr, ok := content["musicPlayButtonRenderer"].(map[string]interface{}); ok {
					if pne, ok := mpbr["playNavigationEndpoint"].(map[string]interface{}); ok {
						if wep, ok := pne["watchEndpoint"].(map[string]interface{}); ok {
							if c.VideoID == "" {
								if vid, ok := wep["videoId"].(string); ok {
									c.VideoID = vid
								}
							}
							extractVideoType(wep, &c)
						}
					}
				}
			}
		}
	}

	// 3. Title, Artists, Album, Duration from flexColumns
	if cols, ok := item["flexColumns"].([]interface{}); ok {
		for i, col := range cols {
			colMap, ok := col.(map[string]interface{})
			if !ok {
				continue
			}
			rfcr, ok := colMap["musicResponsiveListItemFlexColumnRenderer"].(map[string]interface{})
			if !ok {
				continue
			}
			textObj, ok := rfcr["text"].(map[string]interface{})
			if !ok {
				continue
			}
			runs, ok := textObj["runs"].([]interface{})
			if !ok {
				continue
			}

			if i == 0 {
				// Column 0: Song Title
				for _, r := range runs {
					if rm, ok := r.(map[string]interface{}); ok {
						if text, ok := rm["text"].(string); ok {
							c.Title += text
						}
						if c.VideoID == "" {
							if nav, ok := rm["navigationEndpoint"].(map[string]interface{}); ok {
								if wep, ok := nav["watchEndpoint"].(map[string]interface{}); ok {
									if vid, ok := wep["videoId"].(string); ok {
										c.VideoID = vid
									}
									extractVideoType(wep, &c)
								}
							}
						}
					}
				}
			} else if i == 1 {
				// Column 1: Subtitle runs (Song • Artist • Album • Duration, or Artist • Duration)
				var textRuns []string
				for _, r := range runs {
					if rm, ok := r.(map[string]interface{}); ok {
						if text, ok := rm["text"].(string); ok {
							textRuns = append(textRuns, text)
						}
					}
				}

				combined := strings.Join(textRuns, "")
				segments := strings.Split(combined, " • ")

				for _, seg := range segments {
					segTrim := strings.TrimSpace(seg)
					if isTypeLabel(segTrim) {
						if isSongLabel(segTrim) && c.VideoType == domain.TypeUnknown {
							c.VideoType = domain.TypeAudioTrackVideo
						}
						continue
					}

					// Check if segment is duration (contains digits and colon)
					if strings.Contains(segTrim, ":") && isDurationFormat(segTrim) {
						c.DurationMs = parseDurationStringToMs(segTrim)
					} else if len(c.Artists) == 0 {
						// Artists are comma-separated
						artParts := strings.Split(segTrim, ", ")
						for _, ap := range artParts {
							apTrim := strings.TrimSpace(ap)
							if apTrim != "" {
								c.Artists = append(c.Artists, apTrim)
							}
						}
						if len(c.Artists) > 0 {
							c.ChannelTitle = c.Artists[0]
						}
					} else if c.Album == "" {
						c.Album = segTrim
					}
				}
			}
		}
	}

	return c
}

func isDurationFormat(s string) bool {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(strings.TrimSpace(p)); err != nil {
			return false
		}
	}
	return true
}

func parseDurationStringToMs(s string) int {
	parts := strings.Split(s, ":")
	if len(parts) == 2 {
		min, _ := strconv.Atoi(parts[0])
		sec, _ := strconv.Atoi(parts[1])
		return (min*60 + sec) * 1000
	} else if len(parts) == 3 {
		hr, _ := strconv.Atoi(parts[0])
		min, _ := strconv.Atoi(parts[1])
		sec, _ := strconv.Atoi(parts[2])
		return (hr*3600 + min*60 + sec) * 1000
	}
	return 0
}
