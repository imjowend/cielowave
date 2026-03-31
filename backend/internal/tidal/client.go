package tidal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	authURL = "https://auth.tidal.com/v1/oauth2/token"
	apiBase = "https://openapi.tidal.com"
)

// TidalClient is an authenticated client for the Tidal Open API v2.
type TidalClient struct {
	clientID     string
	clientSecret string
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

type trackAttributes struct {
	Title    string `json:"title"`
	Duration int    `json:"duration"`
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
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
	if err := c.refreshToken(); err != nil {
		return nil, fmt.Errorf("initial auth failed: %w", err)
	}
	return c, nil
}

func (c *TidalClient) refreshToken() error {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.clientID)
	data.Set("client_secret", c.clientSecret)

	resp, err := c.httpClient.Post(authURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
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
	token, err := c.getToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, apiBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.api+json")

	return c.httpClient.Do(req)
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
			continue
		}
		a := Artist{ID: res.ID, Name: attr.Name}
		if len(attr.ImageLinks) > 0 {
			a.ImageURL = attr.ImageLinks[0].Href
		}
		artists = append(artists, a)
	}
	return artists, nil
}

// GetArtistTracks returns all tracks for the given artist, paginating as needed.
// Calls GET /v2/artists/{artistID}/relationships/tracks?countryCode=US&include=tracks
func (c *TidalClient) GetArtistTracks(artistID string) ([]Track, error) {
	var all []Track
	path := "/v2/artists/" + url.PathEscape(artistID) + "/relationships/tracks?countryCode=US&include=tracks"

	for path != "" {
		resp, err := c.doRequest("GET", path)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
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
		for _, res := range tr.Included {
			if res.Type == "tracks" {
				includedByID[res.ID] = res
			}
		}

		for _, ref := range tr.Data {
			res, ok := includedByID[ref.ID]
			if !ok {
				continue
			}
			var attr trackAttributes
			if err := json.Unmarshal(res.Attributes, &attr); err != nil {
				continue
			}
			t := Track{
				ID:              res.ID,
				Title:           attr.Title,
				DurationSeconds: attr.Duration,
				AlbumName:       attr.Album.Title,
				ISRC:            attr.ISRC,
			}
			// Use main artist; fall back to first listed.
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
		// next may be a full URL or a relative path.
		if strings.HasPrefix(next, "http") {
			u, err := url.Parse(next)
			if err != nil {
				break
			}
			path = u.RequestURI()
		} else {
			path = next
		}
	}

	return all, nil
}
