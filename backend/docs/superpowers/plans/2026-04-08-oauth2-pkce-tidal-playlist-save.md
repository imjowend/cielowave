# OAuth2 PKCE + Save Playlist to Tidal — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a CieloWave user save a generated playlist directly to their Tidal account via OAuth2 Authorization Code + PKCE flow.

**Architecture:** Two new files in `internal/tidal/` (`store.go` for in-memory TTL stores, `userclient.go` for PKCE + Tidal user API calls), and three new HTTP handlers in `main.go`. `UserClient` owns all domain logic; `main.go` only routes and delegates.

**Tech Stack:** Go 1.26, `crypto/rand`, `crypto/sha256`, `encoding/base64`, `net/http/httptest` for tests, `log/slog` for logging.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/tidal/store.go` | Create | `SavedPlaylist`, `OAuthState` types; `PlaylistStore`, `OAuthStateStore` with get/set/delete/cleanup |
| `internal/tidal/store_test.go` | Create | Tests for both stores: hit, miss, expired, cleanup |
| `internal/tidal/userclient.go` | Create | `UserClient`, PKCE helpers, `BuildLoginURL`, `ExchangeCode`, `CreatePlaylist`, `AddTracks`, `SavePlaylist`, `GetPlaylist`, `GetState`, `DeleteState` |
| `internal/tidal/userclient_test.go` | Create | Tests for PKCE, `BuildLoginURL`, `ExchangeCode`, `CreatePlaylist` (incl. 403 fallback), `AddTracks` |
| `main.go` | Modify | Read `TIDAL_REDIRECT_URI`, instantiate `UserClient`, register 3 new routes, 3 new handler funcs |
| `main_test.go` | Create | Handler tests for save, login, callback |
| `.env.example` | Modify | Add `TIDAL_REDIRECT_URI` |

---

## Task 1: store.go — Types and TTL stores

**Files:**
- Create: `internal/tidal/store.go`
- Create: `internal/tidal/store_test.go`

- [ ] **Step 1: Write failing tests for PlaylistStore**

Create `internal/tidal/store_test.go`:

```go
package tidal

import (
	"testing"
	"time"
)

func TestPlaylistStore_SetGet_Hit(t *testing.T) {
	s := newPlaylistStore()
	p := SavedPlaylist{ID: "1", ArtistA: "Duki", ArtistB: "Nicki", CreatedAt: time.Now()}
	s.set("1", p)
	got, ok := s.get("1")
	if !ok {
		t.Fatal("expected hit, got miss")
	}
	if got.ArtistA != "Duki" {
		t.Fatalf("expected ArtistA=Duki, got %q", got.ArtistA)
	}
}

func TestPlaylistStore_GetExpired(t *testing.T) {
	s := newPlaylistStore()
	p := SavedPlaylist{ID: "1", CreatedAt: time.Now().Add(-31 * time.Minute)}
	s.set("1", p)
	_, ok := s.get("1")
	if ok {
		t.Fatal("expected miss for expired entry, got hit")
	}
}

func TestPlaylistStore_Delete(t *testing.T) {
	s := newPlaylistStore()
	p := SavedPlaylist{ID: "1", CreatedAt: time.Now()}
	s.set("1", p)
	s.delete("1")
	_, ok := s.get("1")
	if ok {
		t.Fatal("expected miss after delete, got hit")
	}
}

func TestPlaylistStore_Cleanup(t *testing.T) {
	s := newPlaylistStore()
	s.set("expired", SavedPlaylist{ID: "expired", CreatedAt: time.Now().Add(-31 * time.Minute)})
	s.set("live", SavedPlaylist{ID: "live", CreatedAt: time.Now()})
	s.cleanup()
	if _, ok := s.get("expired"); ok {
		t.Fatal("cleanup should have removed expired entry")
	}
	if _, ok := s.get("live"); !ok {
		t.Fatal("cleanup should not remove live entry")
	}
}

func TestOAuthStateStore_SetGet_Hit(t *testing.T) {
	s := newOAuthStateStore()
	o := OAuthState{CodeVerifier: "abc", PlaylistID: "p1", CreatedAt: time.Now()}
	s.set("state1", o)
	got, ok := s.get("state1")
	if !ok {
		t.Fatal("expected hit, got miss")
	}
	if got.CodeVerifier != "abc" {
		t.Fatalf("expected CodeVerifier=abc, got %q", got.CodeVerifier)
	}
}

func TestOAuthStateStore_GetExpired(t *testing.T) {
	s := newOAuthStateStore()
	o := OAuthState{CreatedAt: time.Now().Add(-11 * time.Minute)}
	s.set("state1", o)
	_, ok := s.get("state1")
	if ok {
		t.Fatal("expected miss for expired state, got hit")
	}
}

func TestOAuthStateStore_Delete(t *testing.T) {
	s := newOAuthStateStore()
	s.set("state1", OAuthState{CreatedAt: time.Now()})
	s.delete("state1")
	_, ok := s.get("state1")
	if ok {
		t.Fatal("expected miss after delete, got hit")
	}
}

func TestOAuthStateStore_Cleanup(t *testing.T) {
	s := newOAuthStateStore()
	s.set("old", OAuthState{CreatedAt: time.Now().Add(-11 * time.Minute)})
	s.set("new", OAuthState{CreatedAt: time.Now()})
	s.cleanup()
	if _, ok := s.get("old"); ok {
		t.Fatal("cleanup should have removed expired state")
	}
	if _, ok := s.get("new"); !ok {
		t.Fatal("cleanup should not remove live state")
	}
}
```

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd internal/tidal && go test -run "TestPlaylistStore|TestOAuthStateStore" -v
```

Expected: `FAIL — undefined: SavedPlaylist, newPlaylistStore`, etc.

- [ ] **Step 3: Create store.go**

Create `internal/tidal/store.go`:

```go
package tidal

import (
	"sync"
	"time"
)

const (
	playlistTTL = 30 * time.Minute
	stateTTL    = 10 * time.Minute
)

// SavedPlaylist holds a generated playlist pending OAuth save.
type SavedPlaylist struct {
	ID        string
	ArtistA   string
	ArtistB   string
	Tracks    []Track
	CreatedAt time.Time
}

// OAuthState holds PKCE state for an in-flight auth flow.
type OAuthState struct {
	CodeVerifier string
	PlaylistID   string
	CreatedAt    time.Time
}

// PlaylistStore is a thread-safe in-memory store for SavedPlaylist with TTL.
type PlaylistStore struct {
	mu      sync.Mutex
	entries map[string]SavedPlaylist
}

func newPlaylistStore() *PlaylistStore {
	return &PlaylistStore{entries: make(map[string]SavedPlaylist)}
}

func (s *PlaylistStore) set(id string, p SavedPlaylist) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[id] = p
}

func (s *PlaylistStore) get(id string) (SavedPlaylist, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.entries[id]
	if !ok || time.Now().After(p.CreatedAt.Add(playlistTTL)) {
		return SavedPlaylist{}, false
	}
	return p, true
}

func (s *PlaylistStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
}

func (s *PlaylistStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, p := range s.entries {
		if now.After(p.CreatedAt.Add(playlistTTL)) {
			delete(s.entries, id)
		}
	}
}

// OAuthStateStore is a thread-safe in-memory store for OAuthState with TTL.
type OAuthStateStore struct {
	mu      sync.Mutex
	entries map[string]OAuthState
}

func newOAuthStateStore() *OAuthStateStore {
	return &OAuthStateStore{entries: make(map[string]OAuthState)}
}

func (s *OAuthStateStore) set(state string, o OAuthState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[state] = o
}

func (s *OAuthStateStore) get(state string) (OAuthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.entries[state]
	if !ok || time.Now().After(o.CreatedAt.Add(stateTTL)) {
		return OAuthState{}, false
	}
	return o, true
}

func (s *OAuthStateStore) delete(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, state)
}

func (s *OAuthStateStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, o := range s.entries {
		if now.After(o.CreatedAt.Add(stateTTL)) {
			delete(s.entries, k)
		}
	}
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd internal/tidal && go test -run "TestPlaylistStore|TestOAuthStateStore" -v
```

Expected: all 8 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tidal/store.go internal/tidal/store_test.go
git commit -m "feat: add in-memory TTL stores for playlists and OAuth states"
```

---

## Task 2: PKCE helpers in userclient.go

**Files:**
- Create: `internal/tidal/userclient.go` (PKCE functions only)
- Create: `internal/tidal/userclient_test.go` (PKCE tests only)

- [ ] **Step 1: Write failing PKCE tests**

Create `internal/tidal/userclient_test.go`:

```go
package tidal

import (
	"strings"
	"testing"
)

// RFC 7636 Appendix B test vector.
const (
	knownVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	knownChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
)

func TestGenerateCodeVerifier_Format(t *testing.T) {
	v, err := generateCodeVerifier()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v) == 0 {
		t.Fatal("verifier is empty")
	}
	// base64url has no padding or '+' or '/'
	if strings.ContainsAny(v, "+/=") {
		t.Fatalf("verifier contains non-base64url chars: %q", v)
	}
}

func TestComputeCodeChallenge_KnownVector(t *testing.T) {
	got := computeCodeChallenge(knownVerifier)
	if got != knownChallenge {
		t.Fatalf("expected %q, got %q", knownChallenge, got)
	}
}

func TestGenerateState_Format(t *testing.T) {
	s, err := generateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) == 0 {
		t.Fatal("state is empty")
	}
	if strings.ContainsAny(s, "+/=") {
		t.Fatalf("state contains non-base64url chars: %q", s)
	}
}
```

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd internal/tidal && go test -run "TestGenerate|TestCompute" -v
```

Expected: FAIL — `undefined: generateCodeVerifier`, etc.

- [ ] **Step 3: Create userclient.go with PKCE helpers**

Create `internal/tidal/userclient.go`:

```go
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
	tidalLoginURL   = "https://login.tidal.com/authorize"
	tidalUserAuthURL = "https://auth.tidal.com/v1/oauth2/token"
	tidalAPIBase    = "https://openapi.tidal.com"
	tidalLegacyBase = "https://listen.tidal.com"
	tidalScopes     = "playlists.read playlists.write collection.read collection.write"
)

// UserClient handles the OAuth2 PKCE user flow and Tidal playlist operations.
type UserClient struct {
	clientID    string
	redirectURI string
	httpClient  *http.Client
	playlists   *PlaylistStore
	states      *OAuthStateStore
}

// NewUserClient creates a UserClient and starts a single background cleanup goroutine.
func NewUserClient(clientID, redirectURI string) *UserClient {
	uc := &UserClient{
		clientID:    clientID,
		redirectURI: redirectURI,
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

// Placeholder to satisfy compiler — full methods added in later tasks.
var _ = bytes.NewReader
var _ = json.Marshal
var _ = io.ReadAll
var _ = slog.Info
var _ = strings.NewReader
var _ = url.Values{}
```

- [ ] **Step 4: Run PKCE tests — expect PASS**

```bash
cd internal/tidal && go test -run "TestGenerate|TestCompute" -v
```

Expected: 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tidal/userclient.go internal/tidal/userclient_test.go
git commit -m "feat: add PKCE helpers and UserClient scaffold"
```

---

## Task 3: SavePlaylist, GetPlaylist, BuildLoginURL, GetState, DeleteState

**Files:**
- Modify: `internal/tidal/userclient.go`
- Modify: `internal/tidal/userclient_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/tidal/userclient_test.go`:

```go
func TestSavePlaylist_ReturnsID(t *testing.T) {
	uc := NewUserClient("client_id", "http://localhost/callback")
	id := uc.SavePlaylist("Duki", "Nicki", []Track{{ID: "1"}})
	if id == "" {
		t.Fatal("expected non-empty playlist ID")
	}
	p, ok := uc.GetPlaylist(id)
	if !ok {
		t.Fatal("expected playlist to be retrievable")
	}
	if p.ArtistA != "Duki" || p.ArtistB != "Nicki" {
		t.Fatalf("unexpected artists: %q %q", p.ArtistA, p.ArtistB)
	}
}

func TestBuildLoginURL_ContainsRequiredParams(t *testing.T) {
	uc := NewUserClient("my_client", "https://example.com/callback")
	loginURL, err := uc.BuildLoginURL("playlist-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, param := range []string{
		"response_type=code",
		"client_id=my_client",
		"code_challenge_method=S256",
		"code_challenge=",
		"state=",
	} {
		if !strings.Contains(loginURL, param) {
			t.Errorf("login URL missing %q: %s", param, loginURL)
		}
	}
}

func TestBuildLoginURL_StoresState(t *testing.T) {
	uc := NewUserClient("client_id", "https://example.com/callback")
	loginURL, err := uc.BuildLoginURL("playlist-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u, _ := url.Parse(loginURL)
	state := u.Query().Get("state")
	oauthState, ok := uc.GetState(state)
	if !ok {
		t.Fatal("expected state to be stored")
	}
	if oauthState.PlaylistID != "playlist-abc" {
		t.Fatalf("expected PlaylistID=playlist-abc, got %q", oauthState.PlaylistID)
	}
	if oauthState.CodeVerifier == "" {
		t.Fatal("expected non-empty CodeVerifier")
	}
}
```

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd internal/tidal && go test -run "TestSavePlaylist|TestBuildLoginURL" -v
```

Expected: FAIL — `undefined: SavePlaylist`, etc.

- [ ] **Step 3: Remove placeholder lines and add real methods to userclient.go**

Replace the placeholder block at the end of `userclient.go` (the `var _ = ...` lines) with:

```go
// SavePlaylist stores a playlist and returns its UUID.
func (uc *UserClient) SavePlaylist(artistA, artistB string, tracks []Track) string {
	id := newUUID()
	uc.playlists.set(id, SavedPlaylist{
		ID:        id,
		ArtistA:   artistA,
		ArtistB:   artistB,
		Tracks:    tracks,
		CreatedAt: time.Now(),
	})
	return id
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
```

Also remove these now-unused blank identifier lines from the top of the file (they were placeholders):
```go
var _ = bytes.NewReader
var _ = json.Marshal
var _ = io.ReadAll
var _ = slog.Info
var _ = strings.NewReader
var _ = url.Values{}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd internal/tidal && go test -run "TestSavePlaylist|TestBuildLoginURL" -v
```

Expected: 3 tests PASS.

- [ ] **Step 5: Verify full suite still passes**

```bash
cd internal/tidal && go test ./... -v
```

Expected: all 11 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tidal/userclient.go internal/tidal/userclient_test.go
git commit -m "feat: add SavePlaylist, GetPlaylist, BuildLoginURL, state management"
```

---

## Task 4: ExchangeCode

**Files:**
- Modify: `internal/tidal/userclient.go`
- Modify: `internal/tidal/userclient_test.go`

- [ ] **Step 1: Write failing test**

Append to `internal/tidal/userclient_test.go`:

```go
func TestExchangeCode_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("unexpected grant_type: %q", r.FormValue("grant_type"))
		}
		if r.FormValue("client_secret") != "" {
			t.Error("client_secret must not be sent in PKCE flow")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":"user-token-xyz","token_type":"Bearer","expires_in":3600}`)
	}))
	defer srv.Close()

	uc := NewUserClient("cid", "https://example.com/cb")
	uc.overrideAuthURL(srv.URL) // test hook — added in implementation step
	token, err := uc.ExchangeCode("auth-code-123", "my-verifier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "user-token-xyz" {
		t.Fatalf("expected user-token-xyz, got %q", token)
	}
}

func TestExchangeCode_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"error":"invalid_grant"}`)
	}))
	defer srv.Close()

	uc := NewUserClient("cid", "https://example.com/cb")
	uc.overrideAuthURL(srv.URL)
	_, err := uc.ExchangeCode("bad-code", "verifier")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}
```

Add this import to the test file's import block:
```go
import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd internal/tidal && go test -run "TestExchangeCode" -v
```

Expected: FAIL — `undefined: ExchangeCode`, `undefined: overrideAuthURL`.

- [ ] **Step 3: Add authURL field + overrideAuthURL + ExchangeCode to userclient.go**

Add `authURL string` field to the `UserClient` struct, initialized in `NewUserClient`:

```go
type UserClient struct {
	clientID    string
	redirectURI string
	authURL     string // overridable for tests
	httpClient  *http.Client
	playlists   *PlaylistStore
	states      *OAuthStateStore
}

func NewUserClient(clientID, redirectURI string) *UserClient {
	uc := &UserClient{
		clientID:    clientID,
		redirectURI: redirectURI,
		authURL:     tidalUserAuthURL,
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
```

Add the test hook and method:

```go
// overrideAuthURL replaces the token endpoint URL; used in tests only.
func (uc *UserClient) overrideAuthURL(u string) { uc.authURL = u }

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
```

- [ ] **Step 4: Run ExchangeCode tests — expect PASS**

```bash
cd internal/tidal && go test -run "TestExchangeCode" -v
```

Expected: 2 tests PASS.

- [ ] **Step 5: Run full suite**

```bash
cd internal/tidal && go test ./... -v
```

Expected: all 13 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tidal/userclient.go internal/tidal/userclient_test.go
git commit -m "feat: add ExchangeCode for PKCE token exchange"
```

---

## Task 5: CreatePlaylist (+ 403 fallback) and AddTracks

**Files:**
- Modify: `internal/tidal/userclient.go`
- Modify: `internal/tidal/userclient_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/tidal/userclient_test.go`:

```go
func TestCreatePlaylist_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer user-token" {
			t.Errorf("unexpected Authorization: %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"data":{"id":"playlist-999","type":"playlists"}}`)
	}))
	defer srv.Close()

	uc := NewUserClient("cid", "https://example.com/cb")
	uc.overrideAPIBase(srv.URL)
	id, err := uc.CreatePlaylist("user-token", "Duki × Nicki — CieloWave")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "playlist-999" {
		t.Fatalf("expected playlist-999, got %q", id)
	}
}

func TestCreatePlaylist_403FallbackToLegacy(t *testing.T) {
	legacyCalled := false
	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		legacyCalled = true
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"data":{"id":"legacy-playlist","type":"playlists"}}`)
	}))
	defer legacy.Close()

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()

	uc := NewUserClient("cid", "https://example.com/cb")
	uc.overrideAPIBase(primary.URL)
	uc.overrideLegacyBase(legacy.URL)
	id, err := uc.CreatePlaylist("user-token", "title")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !legacyCalled {
		t.Fatal("expected legacy endpoint to be called on 403")
	}
	if id != "legacy-playlist" {
		t.Fatalf("expected legacy-playlist, got %q", id)
	}
}

func TestAddTracks_Success(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	uc := NewUserClient("cid", "https://example.com/cb")
	uc.overrideAPIBase(srv.URL)
	err := uc.AddTracks("user-token", "playlist-1", []string{"track-a", "track-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var body struct {
		Data []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("invalid body: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(body.Data))
	}
	if body.Data[0].Type != "tracks" || body.Data[0].ID != "track-a" {
		t.Fatalf("unexpected first item: %+v", body.Data[0])
	}
}
```

Add `io` to the test file imports.

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd internal/tidal && go test -run "TestCreatePlaylist|TestAddTracks" -v
```

Expected: FAIL — `undefined: CreatePlaylist`, `overrideAPIBase`, `AddTracks`.

- [ ] **Step 3: Add apiBase, legacyBase fields + overrides + CreatePlaylist + AddTracks**

Add `apiBase` and `legacyBase` fields to `UserClient` struct and `NewUserClient`:

```go
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
```

Add test hooks and the two methods:

```go
func (uc *UserClient) overrideAPIBase(u string)    { uc.apiBase = u }
func (uc *UserClient) overrideLegacyBase(u string) { uc.legacyBase = u }

type createPlaylistResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

// CreatePlaylist creates a new playlist in the user's Tidal collection.
// Falls back to the legacy endpoint on 403.
func (uc *UserClient) CreatePlaylist(userToken, title string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"type": "playlists",
			"attributes": map[string]string{
				"name":        title,
				"description": "Playlist generada con CieloWave",
			},
		},
	})
	req, err := http.NewRequest(http.MethodPost, uc.apiBase+"/v2/my-collection/playlists", bytes.NewReader(body))
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

	if resp.StatusCode == http.StatusForbidden {
		slog.Warn("create playlist returned 403, trying legacy endpoint")
		return uc.createPlaylistLegacy(userToken, title)
	}
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

func (uc *UserClient) createPlaylistLegacy(userToken, title string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"type": "playlists",
			"attributes": map[string]string{
				"name":        title,
				"description": "Playlist generada con CieloWave",
			},
		},
	})
	req, err := http.NewRequest(http.MethodPost, uc.legacyBase+"/v2/my-collection/playlists/folders/create-playlist", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := uc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create playlist legacy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create playlist legacy failed (%d): %s", resp.StatusCode, b)
	}
	var result createPlaylistResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode legacy response: %w", err)
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

	endpoint := uc.apiBase + "/v2/my-collection/playlists/" + url.PathEscape(playlistID) + "/relationships/items"
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
```

- [ ] **Step 4: Run new tests — expect PASS**

```bash
cd internal/tidal && go test -run "TestCreatePlaylist|TestAddTracks" -v
```

Expected: 3 tests PASS.

- [ ] **Step 5: Run full suite**

```bash
cd internal/tidal && go test ./... -v
```

Expected: all 16 tests PASS.

- [ ] **Step 6: Build check**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 7: Commit**

```bash
git add internal/tidal/userclient.go internal/tidal/userclient_test.go
git commit -m "feat: add CreatePlaylist with 403 fallback and AddTracks"
```

---

## Task 6: Handlers in main.go and main_test.go

**Files:**
- Modify: `main.go`
- Create: `main_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `main_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cielowave/backend/internal/tidal"
)

func newTestUserClient() *tidal.UserClient {
	return tidal.NewUserClient("test_client_id", "https://example.com/callback")
}

func TestHandleSavePlaylist_MissingFields(t *testing.T) {
	uc := newTestUserClient()
	body := `{"artistA":"","artistB":"","tracks":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/playlist/save", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleSavePlaylist(uc)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSavePlaylist_Success(t *testing.T) {
	uc := newTestUserClient()
	tracks := []tidal.Track{{ID: "t1", Title: "Song"}}
	payload, _ := json.Marshal(map[string]any{
		"artistA": "Duki",
		"artistB": "Nicki",
		"tracks":  tracks,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/playlist/save", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleSavePlaylist(uc)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["playlist_id"] == "" {
		t.Fatal("expected non-empty playlist_id")
	}
}

func TestHandleTidalLogin_MissingPlaylistID(t *testing.T) {
	uc := newTestUserClient()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/tidal/login", nil)
	w := httptest.NewRecorder()
	handleTidalLogin(uc)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleTidalLogin_RedirectsToTidal(t *testing.T) {
	uc := newTestUserClient()
	// First save a playlist so the ID is valid
	id := uc.SavePlaylist("A", "B", []tidal.Track{{ID: "1"}})
	req := httptest.NewRequest(http.MethodGet, "/api/auth/tidal/login?playlist_id="+id, nil)
	w := httptest.NewRecorder()
	handleTidalLogin(uc)(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "login.tidal.com") {
		t.Fatalf("expected redirect to login.tidal.com, got %q", loc)
	}
}

func TestHandleTidalCallback_InvalidState(t *testing.T) {
	uc := newTestUserClient()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/tidal/callback?code=x&state=invalid", nil)
	w := httptest.NewRecorder()
	handleTidalCallback(uc)(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error=auth_failed") {
		t.Fatalf("expected error=auth_failed in redirect, got %q", w.Header().Get("Location"))
	}
}
```

- [ ] **Step 2: Run to confirm FAIL**

```bash
go test -run "TestHandleSavePlaylist|TestHandleTidalLogin|TestHandleTidalCallback" -v
```

Expected: FAIL — `undefined: handleSavePlaylist`, etc.

- [ ] **Step 3: Add handlers to main.go**

Add `"fmt"` to the import block in `main.go` (after `"encoding/json"`).

Add these three handler functions at the bottom of `main.go`:

```go
const frontendBase = "https://cielowave.vercel.app"

func handleSavePlaylist(uc *tidal.UserClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ArtistA string        `json:"artistA"`
			ArtistB string        `json:"artistB"`
			Tracks  []tidal.Track `json:"tracks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.ArtistA == "" || req.ArtistB == "" || len(req.Tracks) == 0 {
			writeError(w, http.StatusBadRequest, "artistA, artistB, and tracks are required")
			return
		}
		id := uc.SavePlaylist(req.ArtistA, req.ArtistB, req.Tracks)
		writeJSON(w, http.StatusOK, map[string]string{"playlist_id": id})
	}
}

func handleTidalLogin(uc *tidal.UserClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		playlistID := r.URL.Query().Get("playlist_id")
		if playlistID == "" {
			writeError(w, http.StatusBadRequest, "missing playlist_id")
			return
		}
		loginURL, err := uc.BuildLoginURL(playlistID)
		if err != nil {
			slog.Error("build login URL failed", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to initiate auth")
			return
		}
		http.Redirect(w, r, loginURL, http.StatusFound)
	}
}

func handleTidalCallback(uc *tidal.UserClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		oauthState, ok := uc.GetState(state)
		if !ok {
			slog.Warn("invalid or expired OAuth state", "state", state)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		userToken, err := uc.ExchangeCode(code, oauthState.CodeVerifier)
		if err != nil {
			slog.Error("code exchange failed", "err", err)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		playlist, ok := uc.GetPlaylist(oauthState.PlaylistID)
		if !ok {
			slog.Warn("playlist not found or expired", "playlist_id", oauthState.PlaylistID)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		title := fmt.Sprintf("%s × %s — CieloWave", playlist.ArtistA, playlist.ArtistB)
		playlistID, err := uc.CreatePlaylist(userToken, title)
		if err != nil {
			slog.Error("create playlist failed", "err", err)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		trackIDs := make([]string, len(playlist.Tracks))
		for i, t := range playlist.Tracks {
			trackIDs[i] = t.ID
		}
		if err := uc.AddTracks(userToken, playlistID, trackIDs); err != nil {
			slog.Error("add tracks failed", "err", err)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		uc.DeleteState(state)
		http.Redirect(w, r, frontendBase+"?saved=true", http.StatusFound)
	}
}
```

- [ ] **Step 4: Run handler tests — expect PASS**

```bash
go test -run "TestHandleSavePlaylist|TestHandleTidalLogin|TestHandleTidalCallback" -v
```

Expected: 5 tests PASS.

- [ ] **Step 5: Build check**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 6: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: add save, login, and callback handlers for Tidal OAuth"
```

---

## Task 7: Wire routes + env var + .env.example

**Files:**
- Modify: `main.go` (routing + UserClient instantiation)
- Modify: `.env.example`
- Modify: `.env` (local only — not committed)

- [ ] **Step 1: Add UserClient instantiation and routes to main()**

In `main.go`, after the line `mbClient := musicbrainz.NewMusicBrainzClient(mbUserAgent)`, add:

```go
redirectURI := os.Getenv("TIDAL_REDIRECT_URI")
userClient := tidal.NewUserClient(clientID, redirectURI)
```

In the mux registration block, add after the existing routes:

```go
mux.HandleFunc("POST /api/playlist/save", handleSavePlaylist(userClient))
mux.HandleFunc("GET /api/auth/tidal/login", handleTidalLogin(userClient))
mux.HandleFunc("GET /api/auth/tidal/callback", handleTidalCallback(userClient))
```

- [ ] **Step 2: Update .env.example**

Add at the end of `.env.example`:

```
TIDAL_REDIRECT_URI=https://cielowave.vercel.app/callback
```

- [ ] **Step 3: Add to local .env**

Edit `.env` (not committed) and add:

```
TIDAL_REDIRECT_URI=https://cielowave.vercel.app/callback
```

- [ ] **Step 4: Build and run full test suite**

```bash
go build ./... && go test ./... -v
```

Expected: build success, all tests PASS.

- [ ] **Step 5: Smoke test locally**

```bash
go run main.go
```

In another terminal:
```bash
# Save a playlist
curl -s -X POST http://localhost:8080/api/playlist/save \
  -H "Content-Type: application/json" \
  -d '{"artistA":"Duki","artistB":"Nicki Nicole","tracks":[{"id":"123","title":"Song"}]}' | jq .
# Expected: {"playlist_id": "<uuid>"}

# Initiate login (use the UUID from above)
curl -v "http://localhost:8080/api/auth/tidal/login?playlist_id=<uuid>" 2>&1 | grep "Location:"
# Expected: Location: https://login.tidal.com/authorize?...
```

- [ ] **Step 6: Final commit**

```bash
git add main.go .env.example
git commit -m "feat: wire OAuth2 PKCE routes and UserClient into server"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** POST /api/playlist/save ✓, GET /api/auth/tidal/login ✓, GET /api/auth/tidal/callback ✓, PKCE generation ✓, code exchange ✓, CreatePlaylist two-step ✓, 403 fallback ✓, AddTracks ✓, TTL cleanup single goroutine ✓, scopes ✓, TIDAL_REDIRECT_URI ✓, no client_secret ✓
- [x] **Placeholder scan:** No TBDs, all code blocks complete
- [x] **Type consistency:** `SavedPlaylist`, `OAuthState`, `PlaylistStore`, `OAuthStateStore` defined in Task 1 and used consistently. `UserClient` struct expanded across Tasks 3-5 with matching field names (`authURL`, `apiBase`, `legacyBase`). Handler signatures match across Task 6 and Task 7 wiring.
