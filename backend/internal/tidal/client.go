package tidal

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

func (c *TidalClient) refreshToken() error {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", authURL, strings.NewReader(data.Encode()))
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
	token, err := c.getToken()
	if err != nil {
		return nil, err
	}

	fullURL := apiBase + path
	log.Printf("doRequest: %s %s", method, fullURL)

	req, err := http.NewRequest(method, apiBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.api+json")

	return c.httpClient.Do(req)
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

// GetArtistTracks returns tracks for the given artist, paginating as needed.
// If maxTracks > 0, pagination stops once that many tracks have been collected.
// Calls GET /v2/artists/{artistID}/relationships/tracks?countryCode=US&include=tracks
func (c *TidalClient) GetArtistTracks(artistID string, maxTracks int) ([]Track, error) {
	var all []Track
	path := "/v2/artists/" + url.PathEscape(artistID) + "/relationships/tracks?countryCode=US&include=tracks&collapseBy=FINGERPRINT"
	log.Printf("GetArtistTracks path: %s", path)
	for path != "" {
		if maxTracks > 0 && len(all) >= maxTracks {
			break
		}
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
			log.Printf("get tracks failed: status=%d body=%s", resp.StatusCode, body)
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
