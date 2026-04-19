package tidal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestTidalClient creates a TidalClient with a pre-set token for testing.
// It does not call the real Tidal auth endpoint.
func newTestTidalClient(apiBase string) *TidalClient {
	return &TidalClient{
		clientID:    "test-client",
		apiBase:     apiBase,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		accessToken: "test-token",
		tokenExpiry: time.Now().Add(time.Hour),
	}
}

func TestSearchArtists_SendsLimit10(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"relationships": map[string]any{
					"artists": map[string]any{"data": []any{}},
				},
			},
			"included": []any{},
		})
	}))
	defer srv.Close()

	c := newTestTidalClient(srv.URL)
	_, err := c.SearchArtists("daft punk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsParam(gotPath, "limit=10") {
		t.Errorf("expected limit=10 in query, got %q", gotPath)
	}
}

func TestSearchArtists_ParsesImageLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"relationships": map[string]any{
					"artists": map[string]any{
						"data": []any{
							map[string]string{"id": "1", "type": "artists"},
						},
					},
				},
			},
			"included": []any{
				map[string]any{
					"id":   "1",
					"type": "artists",
					"attributes": map[string]any{
						"name":       "Daft Punk",
						"imageLinks": []any{map[string]string{"href": "https://img.example.com/daft.jpg"}},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestTidalClient(srv.URL)
	artists, err := c.SearchArtists("daft")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artists) != 1 {
		t.Fatalf("expected 1 artist, got %d", len(artists))
	}
	if artists[0].Name != "Daft Punk" {
		t.Errorf("expected name=Daft Punk, got %q", artists[0].Name)
	}
	if artists[0].ImageURL != "https://img.example.com/daft.jpg" {
		t.Errorf("expected imageUrl from imageLinks, got %q", artists[0].ImageURL)
	}
}

func containsParam(query, param string) bool {
	for _, p := range splitParams(query) {
		if p == param {
			return true
		}
	}
	return false
}

func splitParams(query string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(query); i++ {
		if i == len(query) || query[i] == '&' {
			out = append(out, query[start:i])
			start = i + 1
		}
	}
	return out
}
