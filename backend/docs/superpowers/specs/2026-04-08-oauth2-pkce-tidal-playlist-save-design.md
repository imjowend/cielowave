# OAuth2 PKCE + Save Playlist to Tidal — Design Spec

**Date:** 2026-04-08
**Status:** Approved

---

## Overview

Add a flow that lets a CieloWave user save a generated playlist directly to their Tidal account. The backend handles the full OAuth2 Authorization Code + PKCE flow, in-memory state, and Tidal playlist creation via the user's access token.

---

## Files

| File | Action | Purpose |
|---|---|---|
| `internal/tidal/store.go` | New | `SavedPlaylist`, `OAuthState`, `PlaylistStore`, `OAuthStateStore` structs with get/set/delete |
| `internal/tidal/userclient.go` | New | `UserClient`: PKCE generation, login URL, code exchange, playlist creation |
| `main.go` | Modified | Instantiate `UserClient`, register 3 new handlers, read `TIDAL_REDIRECT_URI` |

---

## Data Structures

```go
type SavedPlaylist struct {
    ID        string
    ArtistA   string
    ArtistB   string
    Tracks    []Track
    CreatedAt time.Time
}

type OAuthState struct {
    CodeVerifier string
    PlaylistID   string
    CreatedAt    time.Time
}
```

---

## New Endpoints

### POST /api/playlist/save

Stores a generated playlist in memory and returns a short-lived UUID to use in the auth flow.

**Request body:**
```json
{ "artistA": "Duki", "artistB": "NICKI NICOLE", "tracks": [...] }
```

**Response:**
```json
{ "playlist_id": "uuid" }
```

TTL: 30 minutes.

---

### GET /api/auth/tidal/login?playlist_id=

1. Validates `playlist_id` exists in store.
2. Generates `code_verifier`: 64 bytes random, base64url (no padding, `+`→`-`, `/`→`_`).
3. Generates `code_challenge`: `BASE64URL(SHA256(code_verifier))`.
4. Generates `state`: 32 bytes random, base64url.
5. Stores `state → { code_verifier, playlist_id }` with TTL 10 min.
6. HTTP 302 redirect to:

```
https://login.tidal.com/authorize
  ?response_type=code
  &client_id={TIDAL_CLIENT_ID}
  &redirect_uri={TIDAL_REDIRECT_URI}
  &scope=playlists.read+playlists.write+collection.read+collection.write
  &code_challenge_method=S256
  &code_challenge={code_challenge}
  &state={state}
```

---

### GET /api/auth/tidal/callback?code=&state=

1. Validates `state` exists in store and is not expired → on fail: redirect to `https://cielowave.vercel.app?error=auth_failed`.
2. Retrieves `code_verifier` and `playlist_id` from state store.
3. Exchanges code for token: `POST https://auth.tidal.com/v1/oauth2/token` with `grant_type=authorization_code`, `client_id`, `code`, `redirect_uri`, `code_verifier`. **No `client_secret`** (public PKCE client).
4. On exchange failure → redirect to `https://cielowave.vercel.app?error=auth_failed`.
5. Retrieves `SavedPlaylist` from store.
6. Creates playlist on Tidal (two-step, see below).
7. Deletes state from memory.
8. Redirect 302 to `https://cielowave.vercel.app?saved=true`.

---

## Playlist Creation (Two-Step + Fallback)

**Step 1 — Create playlist:**
```
POST https://openapi.tidal.com/v2/my-collection/playlists
Authorization: Bearer {user_access_token}

{
  "data": {
    "type": "playlists",
    "attributes": {
      "name": "ArtistA × ArtistB — CieloWave",
      "description": "Playlist generada con CieloWave"
    }
  }
}
```

If this returns 403 → fallback to:
```
POST https://listen.tidal.com/v2/my-collection/playlists/folders/create-playlist
Authorization: Bearer {user_access_token}
```

**Step 2 — Add tracks:**
```
POST https://openapi.tidal.com/v2/my-collection/playlists/{playlistId}/relationships/items
Authorization: Bearer {user_access_token}

{
  "data": [
    { "type": "tracks", "id": "12345" },
    { "type": "tracks", "id": "67890" }
  ]
}
```

---

## UserClient

```go
type UserClient struct {
    clientID    string
    redirectURI string
    httpClient  *http.Client
    playlists   *PlaylistStore
    states      *OAuthStateStore
}
```

**Methods:**
- `BuildLoginURL(playlistID string) (loginURL string, err error)`
- `ExchangeCode(code string) (accessToken string, err error)`
- `CreatePlaylist(userToken, title string) (playlistID string, err error)`
- `AddTracks(userToken, playlistID string, trackIDs []string) error`

---

## TTL Cleanup

Single goroutine launched in `NewUserClient()`:

```go
go func() {
    for range time.Tick(5 * time.Minute) {
        uc.playlists.cleanup()
        uc.states.cleanup()
    }
}()
```

Playlist TTL: 30 min. State TTL: 10 min.

---

## Environment Variables

| Variable | Description |
|---|---|
| `TIDAL_REDIRECT_URI` | e.g. `https://cielowave.vercel.app/callback` |

Existing vars unchanged: `TIDAL_CLIENT_ID`, `TIDAL_CLIENT_SECRET`.

---

## Go Version & Patterns

- Go 1.26 — use `new(val)` for pointer literals, `errors.AsType[T]`, `wg.Go()`, `slog` for logging.
- No client_secret in PKCE callback.
- New endpoints have no auth middleware (public).
- Existing client credentials flow untouched.
- CORS already configured globally — covers new paths.
