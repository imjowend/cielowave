package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	id, err := uc.SavePlaylist("A", "B", []tidal.Track{{ID: "1"}})
	if err != nil {
		t.Fatalf("save playlist: %v", err)
	}
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

func TestHandleTidalCallback_ExchangeFails(t *testing.T) {
	// Mock token endpoint that returns 400
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"error":"invalid_grant"}`)
	}))
	defer tokenSrv.Close()

	uc := newTestUserClient()
	uc.OverrideAuthURL(tokenSrv.URL)

	playlistID, err := uc.SavePlaylist("A", "B", []tidal.Track{{ID: "1"}})
	if err != nil {
		t.Fatalf("save playlist: %v", err)
	}
	loginURL, err := uc.BuildLoginURL(playlistID)
	if err != nil {
		t.Fatalf("build login URL: %v", err)
	}
	u, _ := url.Parse(loginURL)
	state := u.Query().Get("state")

	req := httptest.NewRequest(http.MethodGet, "/api/auth/tidal/callback?code=badcode&state="+state, nil)
	w := httptest.NewRecorder()
	handleTidalCallback(uc)(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error=auth_failed") {
		t.Fatalf("expected error=auth_failed, got %q", w.Header().Get("Location"))
	}
}

func TestHandleTidalCallback_Success(t *testing.T) {
	// Mock token endpoint — returns access token
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":"user-tok","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	// Mock Tidal API — handles POST /v2/my-collection/playlists and relationships/items
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "relationships/items") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"data":{"id":"new-pl-id","type":"playlists"}}`)
	}))
	defer apiSrv.Close()

	uc := newTestUserClient()
	uc.OverrideAuthURL(tokenSrv.URL)
	uc.OverrideAPIBase(apiSrv.URL)

	playlistID, err := uc.SavePlaylist("Duki", "Nicki", []tidal.Track{{ID: "track-1"}, {ID: "track-2"}})
	if err != nil {
		t.Fatalf("save playlist: %v", err)
	}
	loginURL, err := uc.BuildLoginURL(playlistID)
	if err != nil {
		t.Fatalf("build login URL: %v", err)
	}
	u, _ := url.Parse(loginURL)
	state := u.Query().Get("state")

	req := httptest.NewRequest(http.MethodGet, "/api/auth/tidal/callback?code=goodcode&state="+state, nil)
	w := httptest.NewRecorder()
	handleTidalCallback(uc)(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "saved=true") {
		t.Fatalf("expected saved=true, got %q", w.Header().Get("Location"))
	}
}
