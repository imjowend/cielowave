package tidal

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	tidalLoginURL    = "https://login.tidal.com/authorize"
	tidalUserAuthURL = "https://auth.tidal.com/v1/oauth2/token"
	tidalAPIBase     = "https://openapi.tidal.com"
	tidalLegacyBase  = "https://listen.tidal.com"
	tidalScopes      = "playlists.read playlists.write collection.read collection.write"
)

// UserClient handles the OAuth2 PKCE user flow and Tidal playlist operations.
type UserClient struct {
	clientID    string
	redirectURI string
	authURL     string
	apiBase     string
	legacyBase  string
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
		legacyBase:  tidalLegacyBase,
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
func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Blank identifiers to keep imports used until methods are added in later tasks.
var (
	_ = bytes.NewReader
	_ = json.Marshal
	_ = io.ReadAll
	_ = slog.Info
	_ = strings.NewReader
	_ = url.Values{}
)
