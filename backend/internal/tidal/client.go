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

// Constantes de las URLs para interactuar con la API de Tidal
const (
	// tidalAuthURL es el endpoint OAuth 2.1 para la autenticación de servidor.
	tidalAuthURL     = "https://auth.tidal.com/v1/oauth2/token"
	// tidalOpenAPIBase es la URL base oficial para la OpenAPI v2 de Tidal.
	tidalOpenAPIBase = "https://openapi.tidal.com"
	// tidalV1Base es la URL base no oficial (legacy) para la API v1.
	tidalV1Base      = "https://api.tidal.com"
	// tidalV1Token es el token no oficial (App Token) extraído que permite realizar búsquedas en la API v1.
	tidalV1Token     = "CzET4vdadNUFQ5JU"
)

// TidalClient gestiona la comunicación autenticada con la API de Tidal.
type TidalClient struct {
	clientID     string
	clientSecret string
	apiBase      string
	httpClient   *http.Client
	mu           sync.Mutex
	accessToken  string
	tokenExpiry  time.Time
}

// tokenResponse modela la respuesta del endpoint de autenticación de Tidal.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// jsonAPIResource representa un único recurso en el formato estándar de JSON:API.
type jsonAPIResource struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Attributes    json.RawMessage `json:"attributes"`
	Relationships json.RawMessage `json:"relationships"`
}

// v1SearchResponse modela la estructura de respuesta JSON de la API legacy v1.
// GET /v1/search?types=ARTISTS
type v1SearchResponse struct {
	Artists struct {
		Items []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Picture string `json:"picture"` // Hash UUID de la imagen de perfil del artista.
		} `json:"items"`
	} `json:"artists"`
}

// tracksRelationshipResponse modela la respuesta del catálogo de canciones de un artista.
// GET /v2/artists/{id}/relationships/tracks?include=tracks
type tracksRelationshipResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	Included []jsonAPIResource `json:"included"`
	Links    struct {
		Next string `json:"next"`
	} `json:"links"`
}

// isrcTracksResponse modela la búsqueda de canciones a través del código ISRC.
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

// profileArtResponse modela la respuesta de relaciones de arte de perfil.
type profileArtResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// artworkResponse modela la respuesta de recursos de imágenes (artworks).
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

// flexInt es un tipo especial para deserializar números o strings con formato numérico de forma flexible en JSON.
// Esto es necesario porque Tidal a veces retorna la duración como entero y otras como string (ej. "209" o formato ISO 8601).
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	var i int
	// Intenta unmarshal como número entero simple.
	if err := json.Unmarshal(b, &i); err == nil {
		*f = flexInt(i)
		return nil
	}
	var s string
	// Si falla, intenta unmarshal como string.
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	// Intenta parsear si es una cadena con un entero (ej. "209").
	if i, err := strconv.Atoi(s); err == nil {
		*f = flexInt(i)
		return nil
	}
	// Intenta parsear si es una cadena de duración ISO 8601 (ej. "PT3M13S").
	secs, err := parseISO8601Seconds(s)
	if err != nil {
		return err
	}
	*f = flexInt(secs)
	return nil
}

// parseISO8601Seconds convierte una cadena de duración ISO 8601 (tipo PT3M13S) a segundos.
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

// trackAttributes modela los atributos de una canción en Tidal v2.
// album y artistas NO vienen acá — Tidal los expone como relationships (ver trackRelationships).
type trackAttributes struct {
	Title    string  `json:"title"`
	Duration flexInt `json:"duration"`
	ISRC     string  `json:"isrc"`
}

// trackRelationships modela las referencias a artistas y álbum de un track,
// que hay que resolver contra el array "included" de la respuesta.
type trackRelationships struct {
	Artists struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	} `json:"artists"`
	Albums struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	} `json:"albums"`
}

// artistAttributes modela los atributos de un recurso "artists" incluido (sideloaded).
type artistAttributes struct {
	Name string `json:"name"`
}

// albumAttributes modela los atributos de un recurso "albums" incluido (sideloaded).
type albumAttributes struct {
	Title       string `json:"title"`
	ReleaseDate string `json:"releaseDate"`
}

// NewTidalClient inicializa un nuevo cliente de Tidal y obtiene el token OAuth 2.1 inicial.
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

// NewTidalClientForTest crea una instancia para entorno de pruebas con un token predefinido.
func NewTidalClientForTest(apiBase, token string) *TidalClient {
	return &TidalClient{
		apiBase:     apiBase,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		accessToken: token,
		tokenExpiry: time.Now().Add(time.Hour),
	}
}

// OverrideAPIBase permite sobrescribir la URL base de la API (solo usado en testing).
func (c *TidalClient) OverrideAPIBase(u string) { c.apiBase = u }

// refreshToken solicita un nuevo Access Token a Tidal (Client Credentials flow).
func (c *TidalClient) refreshToken() error {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", tidalAuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.clientID, c.clientSecret)
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

	expiry := time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - 30*time.Second)

	c.mu.Lock()
	c.accessToken = tr.AccessToken
	c.tokenExpiry = expiry
	c.mu.Unlock()

	return nil
}

// getToken obtiene el token activo, renovándolo de forma transparente si ha expirado.
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

// doRequest realiza solicitudes http.Client manejando el retroceso exponencial automático (HTTP 429).
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

// SearchArtists busca artistas en Tidal usando el endpoint v1 y un App Token de ingeniería inversa.
// Realiza una petición GET a /v1/search?query={query}&types=ARTISTS e interpreta la respuesta.
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
		// Construye la URL de la imagen si se retorna un hash picture válido de Tidal v1.
		if item.Picture != "" {
			imgURL = "https://resources.tidal.com/images/" + strings.ReplaceAll(item.Picture, "-", "/") + "/320x320.jpg"
		}
		artists = append(artists, Artist{ID: strconv.Itoa(item.ID), Name: item.Name, ImageURL: imgURL})
	}

	return artists, nil
}

// GetArtistTracks retorna la lista completa de canciones del artista, manejando la paginación según corresponda.
func (c *TidalClient) GetArtistTracks(artistID string, maxTracks int) ([]Track, error) {
	var all []Track
	path := "/v2/artists/" + url.PathEscape(artistID) + "/relationships/tracks?countryCode=US&include=tracks,tracks.artists,tracks.albums&collapseBy=FINGERPRINT"
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

		includedByKey := make(map[string]jsonAPIResource, len(tr.Included))
		slog.Debug("page fetched", "data_count", len(tr.Data), "included_count", len(tr.Included))
		for _, res := range tr.Included {
			slog.Debug("included resource", "type", res.Type, "id", res.ID)
			includedByKey[res.Type+":"+res.ID] = res
		}

		for _, ref := range tr.Data {
			slog.Debug("data ref", "id", ref.ID)

			res, ok := includedByKey["tracks:"+ref.ID]
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
				ISRC:            attr.ISRC,
			}

			var rel trackRelationships
			if err := json.Unmarshal(res.Relationships, &rel); err != nil {
				slog.Error("unmarshal track relationships", "track_id", res.ID, "err", err)
			} else {
				if len(rel.Artists.Data) > 0 {
					artistRef := rel.Artists.Data[0]
					if artistRes, ok := includedByKey["artists:"+artistRef.ID]; ok {
						var aAttr artistAttributes
						if err := json.Unmarshal(artistRes.Attributes, &aAttr); err == nil {
							t.ArtistID = artistRef.ID
							t.ArtistName = aAttr.Name
						}
					}
				}
				if len(rel.Albums.Data) > 0 {
					albumRef := rel.Albums.Data[0]
					if albumRes, ok := includedByKey["albums:"+albumRef.ID]; ok {
						var alAttr albumAttributes
						if err := json.Unmarshal(albumRes.Attributes, &alAttr); err == nil {
							t.AlbumName = alAttr.Title
							t.ReleaseDate = alAttr.ReleaseDate
						}
					}
				}
			}

			all = append(all, t)
		}

		next := tr.Links.Next
		if next == "" {
			break
		}
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
		if !strings.HasPrefix(nextPath, "/v2") {
			nextPath = "/v2" + nextPath
		}
		path = nextPath
	}

	return all, nil
}
