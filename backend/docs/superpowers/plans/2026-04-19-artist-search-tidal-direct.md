# Artist Search — Tidal Direct Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the MusicBrainz → ISRC → Tidal search pipeline with a direct Tidal search call, returning the top 5 results sorted so names starting with the query come first.

**Architecture:** `SearchArtists` in `client.go` is updated to send `limit=10` and parse `imageLinks` from the search response (no extra `GetArtistImage` calls). `handleSearchArtists` in `main.go` is simplified to: call → sort → slice to 5. All MusicBrainz code and the `artistCache` are deleted.

**Tech Stack:** Go 1.22+, `net/http`, `sort`, `strings`, `httptest` (tests)

---

## File Map

| File | Action | Change |
|------|--------|--------|
| `internal/tidal/client.go` | Modify | Convert `apiBase` const to struct field; add `OverrideAPIBase`; add `NewTidalClientForTest`; update `SearchArtists` |
| `internal/tidal/client_test.go` | Create | Tests for updated `SearchArtists` |
| `main.go` | Modify | Delete `artistCache`; simplify `handleSearchArtists`; remove MB imports |
| `main_test.go` | Modify | Add `sortArtistsByQuery` test and `handleSearchArtists` handler test |

---

### Task 1: Make `TidalClient.apiBase` a struct field and add test helpers

**Files:**
- Modify: `internal/tidal/client.go`

- [ ] **Step 1: Write a failing test that calls `OverrideAPIBase`**

Create `internal/tidal/client_test.go` with this content:

```go
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
```

- [ ] **Step 2: Run test to confirm it fails to compile**

```bash
go test ./internal/tidal/ -run TestSearchArtists -v
```

Expected: compile error — `apiBase` is a package-level const, not a field.

- [ ] **Step 3: Update `client.go` — convert `apiBase` const to struct field**

In `client.go`, make these changes:

**3a.** Rename the `apiBase` package-level const. Replace:
```go
const (
	authURL = "https://auth.tidal.com/v1/oauth2/token"
	apiBase = "https://openapi.tidal.com"
)
```
With:
```go
const (
	tidalAuthURL    = "https://auth.tidal.com/v1/oauth2/token"
	tidalOpenAPIBase = "https://openapi.tidal.com"
)
```

**3b.** Add `apiBase string` field to `TidalClient` struct. Replace:
```go
type TidalClient struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
	mu           sync.Mutex
	accessToken  string
	tokenExpiry  time.Time
}
```
With:
```go
type TidalClient struct {
	clientID     string
	clientSecret string
	apiBase      string
	httpClient   *http.Client
	mu           sync.Mutex
	accessToken  string
	tokenExpiry  time.Time
}
```

**3c.** Initialize `apiBase` in `NewTidalClient`. Replace:
```go
func NewTidalClient(clientID, clientSecret string) (*TidalClient, error) {
	c := &TidalClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
			},
		},
	}
	if err := c.refreshToken(); err != nil {
		return nil, fmt.Errorf("initial auth failed: %w", err)
	}
	return c, nil
}
```
With:
```go
func NewTidalClient(clientID, clientSecret string) (*TidalClient, error) {
	c := &TidalClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		apiBase:      tidalOpenAPIBase,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
			},
		},
	}
	if err := c.refreshToken(); err != nil {
		return nil, fmt.Errorf("initial auth failed: %w", err)
	}
	return c, nil
}
```

**3d.** Update `refreshToken` to use the renamed const. Replace:
```go
req, err := http.NewRequest("POST", authURL, strings.NewReader(data.Encode()))
```
With:
```go
req, err := http.NewRequest("POST", tidalAuthURL, strings.NewReader(data.Encode()))
```

**3e.** Update `doRequest` to use `c.apiBase`. Replace:
```go
fullURL := apiBase + path
```
With:
```go
fullURL := c.apiBase + path
```

**3f.** Add `NewTidalClientForTest` and `OverrideAPIBase` after `NewTidalClient`:
```go
// NewTidalClientForTest creates a TidalClient with a pre-set token for testing.
// It does not call the real Tidal auth endpoint.
func NewTidalClientForTest(apiBase, token string) *TidalClient {
	return &TidalClient{
		apiBase:     apiBase,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		accessToken: token,
		tokenExpiry: time.Now().Add(time.Hour),
	}
}

// OverrideAPIBase replaces the API base URL; used in tests only.
func (c *TidalClient) OverrideAPIBase(u string) { c.apiBase = u }
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tidal/ -run TestSearchArtists -v
```

Expected: both `TestSearchArtists_SendsLimit10` and `TestSearchArtists_ParsesImageLinks` pass.

- [ ] **Step 5: Run the full test suite to check nothing broke**

```bash
go test ./...
```

Expected: all existing tests still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/tidal/client.go internal/tidal/client_test.go
git commit -m "refactor(tidal): make apiBase a struct field, add NewTidalClientForTest"
```

---

### Task 2: Update `SearchArtists` to use `limit=10` and parse `imageLinks` directly

**Files:**
- Modify: `internal/tidal/client.go`

The `artistAttributes` struct already has `ImageLinks` parsed. This task removes the separate `GetArtistImage` goroutines and uses the embedded URLs instead.

- [ ] **Step 1: Verify the failing test from Task 1 now runs (but `limit=10` test still fails)**

```bash
go test ./internal/tidal/ -run TestSearchArtists_SendsLimit10 -v
```

Expected: FAIL — `expected limit=10 in query` (because the URL doesn't have it yet).

- [ ] **Step 2: Update `SearchArtists` in `client.go`**

Replace the entire `SearchArtists` function body with:

```go
func (c *TidalClient) SearchArtists(query string) ([]Artist, error) {
	path := "/v2/searchresults/" + url.PathEscape(query) + "?countryCode=US&include=artists&limit=10"
	resp, err := c.doRequest("GET", path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed (%d): %s", resp.StatusCode, body)
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}

	includedByID := make(map[string]jsonAPIResource, len(sr.Included))
	for _, res := range sr.Included {
		if res.Type == "artists" {
			includedByID[res.ID] = res
		}
	}

	artists := make([]Artist, 0, len(sr.Data.Relationships.Artists.Data))
	for _, ref := range sr.Data.Relationships.Artists.Data {
		res, ok := includedByID[ref.ID]
		if !ok {
			continue
		}
		var attr artistAttributes
		if err := json.Unmarshal(res.Attributes, &attr); err != nil {
			slog.Warn("unmarshal artist attributes", "artist_id", res.ID, "err", err)
			continue
		}
		var imgURL string
		if len(attr.ImageLinks) > 0 {
			imgURL = attr.ImageLinks[0].Href
		}
		artists = append(artists, Artist{ID: res.ID, Name: attr.Name, ImageURL: imgURL})
	}

	return artists, nil
}
```

- [ ] **Step 3: Run the `SearchArtists` tests**

```bash
go test ./internal/tidal/ -run TestSearchArtists -v
```

Expected: PASS for both tests.

- [ ] **Step 4: Run the full test suite**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/tidal/client.go
git commit -m "feat(tidal): add limit=10 to SearchArtists, use imageLinks directly"
```

---

### Task 3: Simplify `handleSearchArtists` and remove MusicBrainz

**Files:**
- Modify: `main.go`
- Modify: `main_test.go`

- [ ] **Step 1: Write tests for `sortArtistsByQuery` in `main_test.go`**

Add this test at the end of `main_test.go`:

```go
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
```

- [ ] **Step 2: Run the new tests to confirm they fail to compile**

```bash
go test ./... -run "TestSortArtistsByQuery|TestHandleSearchArtists" -v
```

Expected: compile error — `sortArtistsByQuery` and updated `handleSearchArtists` don't exist yet.

- [ ] **Step 3: Rewrite `main.go`**

**3a.** Update imports — replace the current import block with:

```go
import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"

	"cielowave/backend/internal/tidal"

	"github.com/joho/godotenv"
)
```

(Removed: `context`, `sync`, `time`, `cielowave/backend/internal/musicbrainz`, `cielowave/backend/internal/playlist` — wait, `playlist` is still used by `handleCreatePlaylist`. Keep it.)

The final import block:

```go
import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"

	"cielowave/backend/internal/playlist"
	"cielowave/backend/internal/tidal"

	"github.com/joho/godotenv"
)
```

**3b.** Delete the `artistCache` struct and all its methods. Remove these lines entirely from `main.go`:

```go
// artistCache is an in-memory TTL cache for resolved Tidal artists.
type artistCache struct {
	mu      sync.Mutex
	entries map[string]artistCacheEntry
}

type artistCacheEntry struct {
	artist    tidal.Artist
	expiresAt time.Time
}

func newArtistCache() *artistCache {
	return &artistCache{entries: make(map[string]artistCacheEntry)}
}

func (ac *artistCache) get(key string) (tidal.Artist, bool) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	e, ok := ac.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return tidal.Artist{}, false
	}
	return e.artist, true
}

func (ac *artistCache) set(key string, artist tidal.Artist) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.entries[key] = artistCacheEntry{artist: artist, expiresAt: time.Now().Add(time.Hour)}
}
```

**3c.** In `main()`, remove MB client setup and cache. Replace:

```go
	// Inicializa el cliente de MusicBrainz
	mbUserAgent := os.Getenv("MUSICBRAINZ_USER_AGENT")
	if mbUserAgent == "" {
		mbUserAgent = "CieloWave/0.1.0 (noreply@example.com)"
	}
	mbClient := musicbrainz.NewMusicBrainzClient(mbUserAgent)

	// Caché de artistas resueltos (1 hora TTL)
	cache := newArtistCache()
```

With nothing (delete those lines entirely).

**3d.** In `main()`, update the handler registration. Replace:

```go
	mux.HandleFunc("GET /api/artists", handleSearchArtists(client, mbClient, cache))
```

With:

```go
	mux.HandleFunc("GET /api/artists", handleSearchArtists(client))
```

**3e.** Replace the entire `handleSearchArtists` function with:

```go
func sortArtistsByQuery(artists []tidal.Artist, q string) {
	ql := strings.ToLower(q)
	sort.SliceStable(artists, func(i, j int) bool {
		iStarts := strings.HasPrefix(strings.ToLower(artists[i].Name), ql)
		jStarts := strings.HasPrefix(strings.ToLower(artists[j].Name), ql)
		return iStarts && !jStarts
	})
}

func handleSearchArtists(c *tidal.TidalClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			writeError(w, http.StatusBadRequest, "missing query parameter: q")
			return
		}

		artists, err := c.SearchArtists(q)
		if err != nil {
			writeError(w, http.StatusBadGateway, "tidal search failed: "+err.Error())
			return
		}

		sortArtistsByQuery(artists, q)

		if len(artists) > 5 {
			artists = artists[:5]
		}

		writeJSON(w, http.StatusOK, artists)
	}
}
```

- [ ] **Step 4: Run the full test suite**

```bash
go test ./...
```

Expected: all tests pass, including the new ones.

- [ ] **Step 5: Verify the binary compiles cleanly**

```bash
go build ./...
```

Expected: no output (success).

- [ ] **Step 6: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: replace MusicBrainz search with direct Tidal search, sort by query match"
```

---

## Self-Review

**Spec coverage:**
- ✅ `limit=10` added to Tidal search URL (Task 2)
- ✅ `imageLinks[0].href` used directly; no extra `GetArtistImage` calls (Task 2)
- ✅ Sort: starts-with-query first, rest in Tidal order (Task 3 — `sortArtistsByQuery`)
- ✅ Slice to 5 (Task 3 — `handleSearchArtists`)
- ✅ MusicBrainz import removed (Task 3)
- ✅ `artistCache` and all related code removed (Task 3)
- ✅ Response shape `{ id, name, imageUrl }` — unchanged, `tidal.Artist` already has these fields

**Placeholder scan:** None found. All code blocks are complete.

**Type consistency:**
- `sortArtistsByQuery(artists []tidal.Artist, q string)` defined in Task 3 step 3e, called in step 3e and tested in step 1 — consistent.
- `tidal.NewTidalClientForTest(apiBase, token string)` defined in Task 1 step 3f, used in Task 3 step 1 — consistent.
- `handleSearchArtists(c *tidal.TidalClient)` signature defined in Task 3 step 3e, registered in step 3d — consistent.
