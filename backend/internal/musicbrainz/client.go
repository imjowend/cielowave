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
	path := "/artist/?query=" + url.QueryEscape(query) + "&fmt=json&limit=5"
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

// GetArtistISRC returns the first ISRC found for the artist.
// Strategy 1: search recordings by artist MBID (query=arid:{mbid}).
// Strategy 2 (fallback): fetch releases with embedded recordings+isrcs.
// Returns ("", nil) if no ISRC is found via either strategy.
func (c *MusicBrainzClient) GetArtistISRC(mbid string) (string, error) {
	// Strategy 1: recording search by artist MBID
	path1 := "/recording/?query=" + url.QueryEscape("arid:"+mbid) + "&fmt=json&limit=10"
	resp1, err := c.doRequest(path1)
	if err != nil {
		return "", err
	}

	if resp1.StatusCode == http.StatusOK {
		var rr recordingsResponse
		if err := json.NewDecoder(resp1.Body).Decode(&rr); err == nil {
			for _, rec := range rr.Recordings {
				if len(rec.ISRCs) > 0 {
					resp1.Body.Close()
					return rec.ISRCs[0], nil
				}
			}
		}
	}
	resp1.Body.Close()

	// Strategy 2: browse releases with recordings+isrcs included
	path2 := "/release/?artist=" + url.QueryEscape(mbid) + "&fmt=json&limit=1&inc=recordings+isrcs"
	resp2, err := c.doRequest(path2)
	if err != nil {
		return "", nil
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return "", nil
	}

	var lr releasesResponse
	if err := json.NewDecoder(resp2.Body).Decode(&lr); err != nil {
		return "", nil
	}

	for _, release := range lr.Releases {
		for _, media := range release.Media {
			for _, track := range media.Tracks {
				if len(track.Recording.ISRCs) > 0 {
					return track.Recording.ISRCs[0], nil
				}
			}
		}
	}

	return "", nil
}
