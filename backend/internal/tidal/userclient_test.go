package tidal

import (
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
