package tidal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestSavePlaylist_ReturnsID(t *testing.T) {
	uc := NewUserClient("client_id", "http://localhost/callback")
	id, err := uc.SavePlaylist("Duki", "Nicki", []Track{{ID: "1"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	uc.overrideAuthURL(srv.URL)
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
