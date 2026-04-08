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
