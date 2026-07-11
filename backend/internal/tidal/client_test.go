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

func TestGetArtistTracks_ResolvesArtistAlbumFromRelationships(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !containsParam(r.URL.RawQuery, "include=tracks,tracks.artists,tracks.albums") {
			t.Errorf("expected nested include param, got %q", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []any{
				map[string]string{"id": "108093771", "type": "tracks"},
				map[string]string{"id": "108409581", "type": "tracks"},
			},
			"included": []any{
				map[string]any{
					"id":   "108093771",
					"type": "tracks",
					"attributes": map[string]any{
						"title":    "Rockstar",
						"duration": "PT1M52S",
						"isrc":     "CA5KR1702134",
					},
					"relationships": map[string]any{
						"artists": map[string]any{"data": []any{
							map[string]string{"id": "6772771", "type": "artists"},
						}},
						"albums": map[string]any{"data": []any{
							map[string]string{"id": "108093770", "type": "albums"},
						}},
					},
				},
				map[string]any{
					"id":   "108409581",
					"type": "tracks",
					"attributes": map[string]any{
						"title":    "She Don't Give a Fo",
						"duration": "PT3M5S",
						"isrc":     "QZW9L2249675",
					},
					"relationships": map[string]any{
						"artists": map[string]any{"data": []any{
							map[string]string{"id": "6772771", "type": "artists"},
							map[string]string{"id": "9031404", "type": "artists"},
						}},
						"albums": map[string]any{"data": []any{
							map[string]string{"id": "108409580", "type": "albums"},
						}},
					},
				},
				map[string]any{
					"id":         "6772771",
					"type":       "artists",
					"attributes": map[string]any{"name": "Duki"},
				},
				map[string]any{
					"id":         "9031404",
					"type":       "artists",
					"attributes": map[string]any{"name": "Maria Becerra"},
				},
				map[string]any{
					"id":   "108093770",
					"type": "albums",
					"attributes": map[string]any{
						"title":       "Rockstar",
						"releaseDate": "2018-03-09",
					},
				},
				map[string]any{
					"id":   "108409580",
					"type": "albums",
					"attributes": map[string]any{
						"title":       "She Don't Give a Fo (Single)",
						"releaseDate": "2019-11-21",
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestTidalClient(srv.URL)
	tracks, err := c.GetArtistTracks("6772771", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}

	solo := tracks[0]
	if solo.ArtistName != "Duki" || solo.ArtistID != "6772771" {
		t.Errorf("track solista: esperaba artista Duki/6772771, got %q/%q", solo.ArtistName, solo.ArtistID)
	}
	if solo.AlbumName != "Rockstar" {
		t.Errorf("esperaba albumName=Rockstar, got %q", solo.AlbumName)
	}
	if solo.ReleaseDate != "2018-03-09" {
		t.Errorf("esperaba releaseDate=2018-03-09, got %q", solo.ReleaseDate)
	}

	collab := tracks[1]
	if collab.ArtistName != "Duki" || collab.ArtistID != "6772771" {
		t.Errorf("track colab: esperaba primer artista Duki/6772771, got %q/%q", collab.ArtistName, collab.ArtistID)
	}
	if collab.AlbumName != "She Don't Give a Fo (Single)" {
		t.Errorf("esperaba albumName de colab, got %q", collab.AlbumName)
	}
	if collab.ReleaseDate != "2019-11-21" {
		t.Errorf("esperaba releaseDate=2019-11-21, got %q", collab.ReleaseDate)
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
