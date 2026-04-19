package tidal

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	tidalLoginURL    = "https://login.tidal.com/authorize"
	tidalUserAuthURL = "https://auth.tidal.com/v1/oauth2/token"
	tidalAPIBase     = "https://openapi.tidal.com"
	tidalScopes      = "playlists.read playlists.write collection.read collection.write"
)

// UserClient handles the OAuth2 PKCE user flow and Tidal playlist operations.
type UserClient struct {
	clientID    string
	redirectURI string
	authURL     string
	apiBase     string
	httpClient  *http.Client
	playlists   *PlaylistStore
	states      *OAuthStateStore
}

// NewUserClient creates a UserClient and starts a single background cleanup goroutine.
func NewUserClient(clientID, redirectURI string) *UserClient {
	uc := &UserClient{
		clientID:    clientID,
		redirectURI: redirectURI,
		authURL:     tidalUserAuthURL,
		apiBase:     tidalAPIBase,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		playlists:   newPlaylistStore(),
		states:      newOAuthStateStore(),
	}
	go func() {
		for range time.Tick(5 * time.Minute) {
			uc.playlists.cleanup()
			uc.states.cleanup()
		}
	}()
	return uc
}

// generateCodeVerifier returns a cryptographically random base64url string (64 bytes).
func generateCodeVerifier() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// computeCodeChallenge returns BASE64URL(SHA256(verifier)) per RFC 7636.
func computeCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// generateState returns a cryptographically random base64url string (32 bytes).
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// newUUID returns a random UUID v4 string.
func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// SavePlaylist stores a generated playlist and returns its UUID.
func (uc *UserClient) SavePlaylist(artistA, artistB string, tracks []Track) (string, error) {
	id, err := newUUID()
	if err != nil {
		return "", fmt.Errorf("generate playlist id: %w", err)
	}
	uc.playlists.set(id, SavedPlaylist{
		ID:        id,
		ArtistA:   artistA,
		ArtistB:   artistB,
		Tracks:    tracks,
		CreatedAt: time.Now(),
	})
	return id, nil
}

// GetPlaylist retrieves a stored playlist by ID.
func (uc *UserClient) GetPlaylist(id string) (SavedPlaylist, bool) {
	return uc.playlists.get(id)
}

// GetState retrieves a stored OAuth state.
func (uc *UserClient) GetState(state string) (OAuthState, bool) {
	return uc.states.get(state)
}

// DeleteState removes an OAuth state after use.
func (uc *UserClient) DeleteState(state string) {
	uc.states.delete(state)
}

// BuildLoginURL generates PKCE params, stores the state, and returns the Tidal login URL.
func (uc *UserClient) BuildLoginURL(playlistID string) (string, error) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		return "", fmt.Errorf("generate verifier: %w", err)
	}
	challenge := computeCodeChallenge(verifier)
	state, err := generateState()
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	uc.states.set(state, OAuthState{
		CodeVerifier: verifier,
		PlaylistID:   playlistID,
		CreatedAt:    time.Now(),
	})
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {uc.clientID},
		"redirect_uri":          {uc.redirectURI},
		"scope":                 {tidalScopes},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
		"state":                 {state},
	}
	return tidalLoginURL + "?" + params.Encode(), nil
}

// OverrideAuthURL replaces the token endpoint URL; used in tests only.
func (uc *UserClient) OverrideAuthURL(u string) { uc.authURL = u }

// OverrideAPIBase replaces the API base URL; used in tests only.
func (uc *UserClient) OverrideAPIBase(u string) { uc.apiBase = u }

type userTokenResponse struct {
	AccessToken string `json:"access_token"`
}

// ExchangeCode exchanges an authorization code for a user access token (PKCE — no client_secret).
func (uc *UserClient) ExchangeCode(code, codeVerifier string) (string, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {uc.clientID},
		"code":          {code},
		"redirect_uri":  {uc.redirectURI},
		"code_verifier": {codeVerifier},
	}
	resp, err := uc.httpClient.Post(uc.authURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("token exchange request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, body)
	}
	var tr userTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	return tr.AccessToken, nil
}

type createPlaylistResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

// CreatePlaylist creates a new public playlist in the user's Tidal account.
func (uc *UserClient) CreatePlaylist(userToken, title string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"type": "playlists",
			"attributes": map[string]string{
				"name":        title,
				"description": "Playlist generada con CieloWave",
				"accessType":  "PUBLIC",
			},
		},
	})
	req, err := http.NewRequest(http.MethodPost, uc.apiBase+"/v2/playlists?countryCode=US", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := uc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create playlist failed (%d): %s", resp.StatusCode, b)
	}
	var result createPlaylistResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode create playlist response: %w", err)
	}
	return result.Data.ID, nil
}

// AddTracks adds track IDs to an existing playlist via JSON:API relationships.
func (uc *UserClient) AddTracks(userToken, playlistID string, trackIDs []string) error {
	items := make([]map[string]string, len(trackIDs))
	for i, id := range trackIDs {
		items[i] = map[string]string{"type": "tracks", "id": id}
	}
	body, _ := json.Marshal(map[string]any{"data": items})

	endpoint := uc.apiBase + "/v2/playlists/" + url.PathEscape(playlistID) + "/relationships/items"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := uc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("add tracks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add tracks failed (%d): %s", resp.StatusCode, b)
	}
	return nil
}
