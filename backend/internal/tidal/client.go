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
	tidalV1Base      = "https://api.tidal.com"
	tidalV1Token     = "CzET4vdadNUFQ5JU"
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

// v1SearchResponse models GET /v1/search?types=ARTISTS.
type v1SearchResponse struct {
	Artists struct {
		Items []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Picture string `json:"picture"`
		} `json:"items"`
	} `json:"artists"`
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

// SearchArtists searches for artists matching query using the Tidal v1 API.
// Calls GET /v1/search?query={query}&types=ARTISTS&limit=10&countryCode=US
func (c *TidalClient) SearchArtists(query string) ([]Artist, error) {
	reqURL := tidalV1Base + "/v1/search?query=" + url.QueryEscape(query) + "&types=ARTISTS&limit=10&countryCode=US"

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Tidal-Token", tidalV1Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed (%d): %s", resp.StatusCode, body)
	}

	var sr v1SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}

	artists := make([]Artist, 0, len(sr.Artists.Items))
	for _, item := range sr.Artists.Items {
		var imgURL string
		if item.Picture != "" {
			imgURL = "https://resources.tidal.com/images/" + strings.ReplaceAll(item.Picture, "-", "/") + "/320x320.jpg"
		}
		artists = append(artists, Artist{ID: strconv.Itoa(item.ID), Name: item.Name, ImageURL: imgURL})
	}

	return artists, nil
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
