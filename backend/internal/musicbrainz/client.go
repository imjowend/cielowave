package musicbrainz

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const mbAPIBase = "https://musicbrainz.org/ws/2"

// MusicBrainzClient is a rate-limited client for the MusicBrainz API (1 req/sec).
type MusicBrainzClient struct {
	httpClient  *http.Client
	userAgent   string
	mu          sync.Mutex
	lastRequest time.Time
}

func NewMusicBrainzClient(userAgent string) *MusicBrainzClient {
	return &MusicBrainzClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		userAgent:  userAgent,
	}
}

func (c *MusicBrainzClient) doRequest(path string) (*http.Response, error) {
	c.mu.Lock()
	if since := time.Since(c.lastRequest); since < time.Second {
		time.Sleep(time.Second - since)
	}
	c.lastRequest = time.Now()
	c.mu.Unlock()

	req, err := http.NewRequest("GET", mbAPIBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	return c.httpClient.Do(req)
}

// SearchArtists queries MusicBrainz for artists matching the given name.
func (c *MusicBrainzClient) SearchArtists(query string) ([]ArtistResult, error) {
	path := "/artist/?query=" + url.QueryEscape(query) + "&fmt=json&limit=10"
	resp, err := c.doRequest(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("musicbrainz search failed (%d): %s", resp.StatusCode, body)
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}

	results := make([]ArtistResult, 0, len(sr.Artists))
	for _, a := range sr.Artists {
		results = append(results, ArtistResult{MBID: a.ID, Name: a.Name, Score: a.Score})
	}
	return results, nil
}

// GetArtistISRC returns the first ISRC found among the artist's recordings.
// Returns ("", nil) if no ISRC is available.
func (c *MusicBrainzClient) GetArtistISRC(mbid string) (string, error) {
	path := "/recording?artist=" + url.PathEscape(mbid) + "&inc=isrcs&fmt=json&limit=1"
	resp, err := c.doRequest(path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("musicbrainz recordings failed (%d): %s", resp.StatusCode, body)
	}

	var rr recordingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return "", err
	}

	for _, rec := range rr.Recordings {
		if len(rec.ISRCs) > 0 {
			return rec.ISRCs[0], nil
		}
	}
	return "", nil
}
