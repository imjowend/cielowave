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

func TestSortArtistsByQuery_StartsWithFirst(t *testing.T) {
	artists := []tidal.Artist{
		{ID: "1", Name: "Stardust"},
		{ID: "2", Name: "Daft Punk"},
		{ID: "3", Name: "Daftside"},
	}
	sortArtistsByQuery(artists, "daft")
	if artists[0].Name != "Daft Punk" && artists[0].Name != "Daftside" {
		t.Errorf("expected starts-with match first, got %q", artists[0].Name)
	}
	if artists[2].Name != "Stardust" {
		t.Errorf("expected non-match last, got %q", artists[2].Name)
	}
}

func TestSortArtistsByQuery_CaseInsensitive(t *testing.T) {
	artists := []tidal.Artist{
		{ID: "1", Name: "miles davis"},
		{ID: "2", Name: "Miles Davis"},
	}
	sortArtistsByQuery(artists, "MILES")
	// Both match; original order preserved by SliceStable.
	if artists[0].Name != "miles davis" {
		t.Errorf("expected stable sort to preserve order of equal elements, got %q", artists[0].Name)
	}
}

func TestHandleSearchArtists_ReturnsSortedTop5(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := []string{"1", "2", "3", "4", "5", "6"}
		data := make([]map[string]string, len(ids))
		included := make([]map[string]any, len(ids))
		names := []string{"Something Else", "Bad Bunny Fan", "Bad Bunny", "Bad Bunny Club", "Bad Bunnies", "Bad Bunny Remix"}
		for i, id := range ids {
			data[i] = map[string]string{"id": id, "type": "artists"}
			included[i] = map[string]any{
				"id":   id,
				"type": "artists",
				"attributes": map[string]any{
					"name":       names[i],
					"imageLinks": []any{map[string]string{"href": "https://img/" + id}},
				},
			}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"relationships": map[string]any{
					"artists": map[string]any{"data": data},
				},
			},
			"included": included,
		})
	}))
	defer srv.Close()

	c := tidal.NewTidalClientForTest(srv.URL, "test-token")
	req := httptest.NewRequest(http.MethodGet, "/api/artists?q=bad+bunny", nil)
	w := httptest.NewRecorder()
	handleSearchArtists(c)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var artists []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&artists); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(artists) != 5 {
		t.Fatalf("expected 5 artists, got %d", len(artists))
	}
	// First result must start with "bad bunny" (case-insensitive).
	first := artists[0]["name"].(string)
	if !strings.HasPrefix(strings.ToLower(first), "bad bunny") {
		t.Errorf("expected first result to start with 'bad bunny', got %q", first)
	}
	// "Something Else" must not appear (it's 6th and doesn't match prefix).
	for _, a := range artists {
		if a["name"] == "Something Else" {
			t.Error("'Something Else' should not appear in top 5")
		}
	}
}

func TestHandleSearchArtists_MissingQuery(t *testing.T) {
	c := tidal.NewTidalClientForTest("http://unused", "tok")
	req := httptest.NewRequest(http.MethodGet, "/api/artists", nil)
	w := httptest.NewRecorder()
	handleSearchArtists(c)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
