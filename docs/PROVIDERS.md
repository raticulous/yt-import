# Music Providers & Authentication Guide

`yt-import` interacts with **Spotify** as the Source Provider and **YouTube Music** as the Target Provider.

---

## 1. Spotify Provider Setup

### Option A: Interactive OAuth (Recommended)
1. Visit the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard).
2. Create an App (e.g. `yt-import`).
3. Set the Redirect URI to `http://localhost:8080/callback`.
4. Copy your **Client ID** and **Client Secret**.
5. When running `yt-import`, enter these credentials (or set `SPOTIFY_CLIENT_ID` and `SPOTIFY_CLIENT_SECRET`).
6. `yt-import` will launch a local server to capture the authorization code and save the token to `~/.config/yt-import/spotify_token.json`.

### Option B: Direct User Access Token
If you already have a temporary OAuth token with `playlist-read-private` scope, you can pass it directly:
```bash
yt-import --spotify-token "BQ..."
```

---

## 2. YouTube Music Target Setup

### Option A: InnerTube Browser Cookie / Headers (Fastest Setup)
No Google Cloud project is needed.
1. Open [music.youtube.com](https://music.youtube.com) in your browser and log in.
2. Open DevTools (F12) $\to$ **Network** tab.
3. Filter by `/browse` or `/search`, click any request.
4. Copy the request **Cookie** header (or export headers via `ytmusicapi` format).
5. Paste it in the interactive setup wizard or save to `~/.config/yt-import/ytm_cookie.txt`.

### Option B: Google Cloud OAuth 2.0 (YouTube Data API v3)
1. Create a project in Google Cloud Console.
2. Enable **YouTube Data API v3**.
3. Create OAuth 2.0 Desktop Application credentials.
4. Set `YOUTUBE_CLIENT_ID` and `YOUTUBE_CLIENT_SECRET`.
5. Authenticate via Google OAuth consent screen.
