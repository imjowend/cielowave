package tidal

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tidalAuthURL     = "https://auth.tidal.com/v1/oauth2/token"
	tidalOpenAPIBase = "https://openapi.tidal.com"
)

// TidalClient is an authenticated client for the Tidal Open API v2.
type TidalClient struct {
	clientID     string
	clientSecret string
	apiBase      string
	httpClient   *http.Client
	mu           sync.Mutex
	accessToken  string
	tokenExpiry  time.Time
}

// tokenResponse models the OAuth 2.1 token endpoint response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// jsonAPIResource is a single JSON:API resource object.
type jsonAPIResource struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Attributes json.RawMessage `json:"attributes"`
}

// searchResponse models GET /v2/searchresults/{query}?include=artists.
type searchResponse struct {
	Data struct {
		Relationships struct {
			Artists struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"artists"`
		} `json:"relationships"`
	} `json:"data"`
	Included []jsonAPIResource `json:"included"`
}

type artistAttributes struct {
	Name       string `json:"name"`
	ImageLinks []struct {
		Href string `json:"href"`
	} `json:"imageLinks"`
}

// tracksRelationshipResponse models GET /v2/artists/{id}/relationships/tracks?include=tracks.
type tracksRelationshipResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	Included []jsonAPIResource `json:"included"`
	Links    struct {
		Next string `json:"next"`
	} `json:"links"`
}

// isrcTracksResponse models GET /v2/tracks?filter[isrc]={isrc}&countryCode=US&include=artists.
type isrcTracksResponse struct {
	Data []struct {
		Relationships struct {
			Artists struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"artists"`
		} `json:"relationships"`
	} `json:"data"`
	Included []jsonAPIResource `json:"included"`
}

// profileArtResponse models GET /v2/artists/{id}/relationships/profileArt.
type profileArtResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// artworkResponse models GET /v2/artworks/{id}.
type artworkResponse struct {
	Data struct {
		Attributes struct {
			Files []struct {
				Href string `json:"href"`
				Meta struct {
					Width int `json:"width"`
				} `json:"meta"`
			} `json:"files"`
		} `json:"attributes"`
	} `json:"data"`
}

// flexInt unmarshals a JSON number or quoted string into an int.
// Tidal's API sometimes returns duration as a string (e.g. "209").
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	var i int
	if err := json.Unmarshal(b, &i); err == nil {
		*f = flexInt(i)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	// Plain integer string (e.g. "209")
	if i, err := strconv.Atoi(s); err == nil {
		*f = flexInt(i)
		return nil
	}
	// ISO 8601 duration (e.g. "PT3M13S")
	secs, err := parseISO8601Seconds(s)
	if err != nil {
		return err
	}
	*f = flexInt(secs)
	return nil
}

// parseISO8601Seconds converts a PT duration string to total seconds.
// Handles PTxHxMxS, PTxMxS, PTxS forms.
func parseISO8601Seconds(s string) (int, error) {
	s = strings.TrimPrefix(s, "PT")
	var total int
	if h, rest, ok := strings.Cut(s, "H"); ok {
		n, err := strconv.Atoi(h)
		if err != nil {
			return 0, fmt.Errorf("invalid hours in duration %q: %w", s, err)
		}
		total += n * 3600
		s = rest
	}
	if m, rest, ok := strings.Cut(s, "M"); ok {
		n, err := strconv.Atoi(m)
		if err != nil {
			return 0, fmt.Errorf("invalid minutes in duration %q: %w", s, err)
		}
		total += n * 60
		s = rest
	}
	if sec, _, ok := strings.Cut(s, "S"); ok {
		n, err := strconv.Atoi(sec)
		if err != nil {
			return 0, fmt.Errorf("invalid seconds in duration %q: %w", s, err)
		}
		total += n
	}
	return total, nil
}

type trackAttributes struct {
	Title    string  `json:"title"`
	Duration flexInt `json:"duration"`
	ISRC     string `json:"isrc"`
	Album    struct {
		Title string `json:"title"`
	} `json:"album"`
	Artists []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Main bool   `json:"main"`
	} `json:"artists"`
}

// NewTidalClient creates a TidalClient and fetches an initial access token.
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

func (c *TidalClient) refreshToken() error {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", tidalAuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.clientID, c.clientSecret) // <-- esto genera el header Basic automáticamente
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth failed (%d): %s", resp.StatusCode, body)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return err
	}

	// Subtract 30s buffer so we refresh before the token actually expires.
	expiry := time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - 30*time.Second)

	c.mu.Lock()
	c.accessToken = tr.AccessToken
	c.tokenExpiry = expiry
	c.mu.Unlock()

	return nil
}

func (c *TidalClient) getToken() (string, error) {
	c.mu.Lock()
	expired := time.Now().After(c.tokenExpiry)
	token := c.accessToken
	c.mu.Unlock()

	if expired {
		if err := c.refreshToken(); err != nil {
			return "", err
		}
		c.mu.Lock()
		token = c.accessToken
		c.mu.Unlock()
	}
	return token, nil
}

func (c *TidalClient) doRequest(method, path string) (*http.Response, error) {
	const maxRetries = 3
	retryDelays := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		token, err := c.getToken()
		if err != nil {
			return nil, err
		}

		fullURL := c.apiBase + path
		slog.Debug("doRequest", "method", method, "url", fullURL, "attempt", attempt+1)

		req, err := http.NewRequest(method, fullURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.api+json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if attempt == maxRetries {
			return nil, fmt.Errorf("tidal rate limited after %d retries: %s", maxRetries, body)
		}

		wait := retryDelays[attempt]
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				wait = time.Duration(secs) * time.Second
			}
		}
		slog.Warn("rate limited, retrying", "wait", wait, "attempt", attempt+1)
		time.Sleep(wait)
	}

	return nil, fmt.Errorf("doRequest: unexpected exit from retry loop")
}

// GetArtistImage returns the highest-resolution profile image URL for the given artist.
// Returns ("", nil) if the artist has no profile art.
func (c *TidalClient) GetArtistImage(artistID string) (string, error) {
	// Step 1: resolve the artwork ID from the profileArt relationship.
	resp, err := c.doRequest("GET", "/v2/artists/"+url.PathEscape(artistID)+"/relationships/profileArt?countryCode=US")
	if err != nil {
		return "", err
	}
	var par profileArtResponse
	err = json.NewDecoder(resp.Body).Decode(&par)
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	if len(par.Data) == 0 {
		return "", nil
	}
	artworkID := par.Data[0].ID

	// Step 2: fetch the artwork and pick the file with the greatest width.
	resp, err = c.doRequest("GET", "/v2/artworks/"+url.PathEscape(artworkID)+"?countryCode=US")
	if err != nil {
		return "", err
	}
	var ar artworkResponse
	err = json.NewDecoder(resp.Body).Decode(&ar)
	resp.Body.Close()
	if err != nil {
		return "", err
	}

	var best string
	var bestWidth int
	for _, f := range ar.Data.Attributes.Files {
		if f.Meta.Width > bestWidth {
			bestWidth = f.Meta.Width
			best = f.Href
		}
	}
	return best, nil
}

// SearchArtists searches for artists matching query.
// Calls GET /v2/searchresults/{query}?countryCode=US&include=artists
func (c *TidalClient) SearchArtists(query string) ([]Artist, error) {
	path := "/v2/searchresults/" + url.PathEscape(query) + "?countryCode=US&include=artists"
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

	// Index included resources by ID for O(1) lookup.
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
		artists = append(artists, Artist{ID: res.ID, Name: attr.Name})
	}

	// Fetch profile images in parallel.
	var wg sync.WaitGroup
	for i := range artists {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			imgURL, err := c.GetArtistImage(artists[i].ID)
			if err == nil {
				artists[i].ImageURL = imgURL
			}
		}(i)
	}
	wg.Wait()

	return artists, nil
}

// ResolveArtistByISRC looks up the Tidal artist that owns the given ISRC.
// Returns (nil, nil) if the ISRC is not found in Tidal.
// The filter[isrc] query param must be sent with literal brackets (not percent-encoded).
func (c *TidalClient) ResolveArtistByISRC(isrc string) (*Artist, error) {
	// Build the path with literal brackets so they reach Tidal unencoded.
	path := "/v2/tracks?filter[isrc]=" + url.QueryEscape(isrc) + "&countryCode=US&include=artists"
	resp, err := c.doRequest("GET", path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tidal isrc lookup failed (%d): %s", resp.StatusCode, body)
	}

	var result isrcTracksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, nil
	}
	artistRefs := result.Data[0].Relationships.Artists.Data
	if len(artistRefs) == 0 {
		return nil, nil
	}
	artistID := artistRefs[0].ID

	for _, inc := range result.Included {
		if inc.Type == "artists" && inc.ID == artistID {
			var attr artistAttributes
			if err := json.Unmarshal(inc.Attributes, &attr); err != nil {
				return nil, err
			}
			return &Artist{ID: inc.ID, Name: attr.Name}, nil
		}
	}
	return nil, nil
}

// SearchArtistByName tries to find a Tidal artist by searching tracks filtered by artist name.
// This is a best-effort fallback; returns (nil, nil) if the endpoint doesn't exist or yields no results.
func (c *TidalClient) SearchArtistByName(name string) (*Artist, error) {
	path := "/v2/tracks?filter[artistName]=" + url.QueryEscape(name) + "&countryCode=US&include=artists"
	resp, err := c.doRequest("GET", path)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var result isrcTracksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}

	if len(result.Data) == 0 {
		return nil, nil
	}
	artistRefs := result.Data[0].Relationships.Artists.Data
	if len(artistRefs) == 0 {
		return nil, nil
	}
	artistID := artistRefs[0].ID

	for _, inc := range result.Included {
		if inc.Type == "artists" && inc.ID == artistID {
			var attr artistAttributes
			if err := json.Unmarshal(inc.Attributes, &attr); err != nil {
				slog.Warn("unmarshal artist attributes", "artist_id", inc.ID, "err", err)
				return nil, nil
			}
			return &Artist{ID: inc.ID, Name: attr.Name}, nil
		}
	}
	return nil, nil
}

// GetArtistTracks returns tracks for the given artist, paginating as needed.
// If maxTracks > 0, pagination stops once that many tracks have been collected.
// Calls GET /v2/artists/{artistID}/relationships/tracks?countryCode=US&include=tracks
func (c *TidalClient) GetArtistTracks(artistID string, maxTracks int) ([]Track, error) {
	var all []Track
	path := "/v2/artists/" + url.PathEscape(artistID) + "/relationships/tracks?countryCode=US&include=tracks&collapseBy=FINGERPRINT"
	slog.Debug("GetArtistTracks", "path", path)
	firstPage := true
	for path != "" {
		if maxTracks > 0 && len(all) >= maxTracks {
			break
		}
		if !firstPage {
			time.Sleep(200 * time.Millisecond)
		}
		firstPage = false
		resp, err := c.doRequest("GET", path)
		if err != nil {
			return nil, err
		}

		/*if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("get tracks failed (%d): %s", resp.StatusCode, body)
		}*/
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			slog.Error("get tracks failed", "status", resp.StatusCode, "body", string(body))
			return nil, fmt.Errorf("get tracks failed (%d): %s", resp.StatusCode, body)
		}

		var tr tracksRelationshipResponse
		err = json.NewDecoder(resp.Body).Decode(&tr)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		// Index included track resources by ID.
		includedByID := make(map[string]jsonAPIResource, len(tr.Included))
		slog.Debug("page fetched", "data_count", len(tr.Data), "included_count", len(tr.Included))
		for _, res := range tr.Included {
			slog.Debug("included resource", "type", res.Type, "id", res.ID)

			if res.Type == "tracks" {
				includedByID[res.ID] = res
			}
		}

		for _, ref := range tr.Data {
			slog.Debug("data ref", "id", ref.ID)

			res, ok := includedByID[ref.ID]
			if !ok {
				continue
			}
			var attr trackAttributes
			if err := json.Unmarshal(res.Attributes, &attr); err != nil {
				slog.Error("unmarshal track attributes", "track_id", res.ID, "err", err)
				continue
			}
			slog.Debug("parsed track", "id", res.ID, "title", attr.Title)
			t := Track{
				ID:              res.ID,
				Title:           attr.Title,
				DurationSeconds: int(attr.Duration),
				AlbumName:       attr.Album.Title,
				ISRC:            attr.ISRC,
			}
			for _, a := range attr.Artists {
				if t.ArtistID == "" {
					t.ArtistID = a.ID
					t.ArtistName = a.Name
				}
				if a.Main {
					t.ArtistID = a.ID
					t.ArtistName = a.Name
					break
				}
			}
			all = append(all, t)
		}

		// Follow pagination cursor.
		next := tr.Links.Next
		if next == "" {
			break
		}
		// next may be a full URL or a relative path; extract path+query either way.
		var nextPath string
		if strings.HasPrefix(next, "http") {
			u, err := url.Parse(next)
			if err != nil {
				break
			}
			nextPath = u.RequestURI()
		} else {
			nextPath = next
		}
		// Ensure the path includes /v2 regardless of which branch was taken.
		if !strings.HasPrefix(nextPath, "/v2") {
			nextPath = "/v2" + nextPath
		}
		path = nextPath
	}

	return all, nil
}
