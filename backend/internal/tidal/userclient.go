package tidal

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Constantes de las URLs de Tidal utilizadas para el inicio de sesión y guardado de listas de reproducción
const (
	// tidalLoginURL es el endpoint oficial de inicio de sesión e interfaz de consentimiento de Tidal.
	tidalLoginURL    = "https://login.tidal.com/authorize"
	// tidalUserAuthURL es el endpoint para intercambiar códigos de autorización por tokens de acceso de usuario.
	tidalUserAuthURL = "https://auth.tidal.com/v1/oauth2/token"
	// tidalAPIBase es la base de la OpenAPI de Tidal.
	tidalAPIBase     = "https://openapi.tidal.com"
	// Scopes (alcances de permisos) solicitados para leer y escribir las playlists del usuario.
	tidalScopes      = "playlists.read playlists.write collection.read collection.write"
)

// UserClient gestiona el flujo de autenticación OAuth 2.1 PKCE y las operaciones de listas de reproducción de usuario.
type UserClient struct {
	clientID    string
	redirectURI string
	authURL     string
	apiBase     string
	httpClient  *http.Client
	playlists   *PlaylistStore
	states      *OAuthStateStore
}

// NewUserClient inicializa el cliente UserClient e inicia una tarea en segundo plano para limpiar las playlists y estados OAuth expirados.
func NewUserClient(clientID, redirectURI string) *UserClient {
	uc := &UserClient{
		clientID:    clientID,
		redirectURI: redirectURI,
		authURL:     tidalUserAuthURL,
		apiBase:     tidalAPIBase,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		playlists:   newPlaylistStore(),
		states:      newOAuthStateStore(),
	}
	// Tarea recurrente cada 5 minutos para eliminar sesiones y playlists temporales expiradas de la memoria.
	go func() {
		for range time.Tick(5 * time.Minute) {
			uc.playlists.cleanup()
			uc.states.cleanup()
		}
	}()
	return uc
}

// generateCodeVerifier genera un verificador de código (Code Verifier) aleatorio criptográficamente seguro (64 bytes) codificado en base64url.
func generateCodeVerifier() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// computeCodeChallenge calcula el desafío de código (Code Challenge) usando SHA256 sobre el verifier, cumpliendo con RFC 7636.
func computeCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// generateState genera un parámetro 'state' aleatorio para proteger el flujo contra ataques de falsificación de solicitudes (CSRF).
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// newUUID genera un identificador UUID versión 4 estándar.
func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// SavePlaylist almacena de forma temporal una playlist generada en la tienda interna y devuelve su ID único UUID.
func (uc *UserClient) SavePlaylist(artistA, artistB string, tracks []Track) (string, error) {
	id, err := newUUID()
	if err != nil {
		return "", fmt.Errorf("generate playlist id: %w", err)
	}
	uc.playlists.set(id, SavedPlaylist{
		ID:        id,
		ArtistA:   artistA,
		ArtistB:   artistB,
		Tracks:    tracks,
		CreatedAt: time.Now(),
	})
	return id, nil
}

// GetPlaylist recupera una playlist temporal guardada por su ID.
func (uc *UserClient) GetPlaylist(id string) (SavedPlaylist, bool) {
	return uc.playlists.get(id)
}

// GetState recupera un estado OAuth almacenado temporalmente.
func (uc *UserClient) GetState(state string) (OAuthState, bool) {
	return uc.states.get(state)
}

// DeleteState elimina un estado OAuth después de haber completado su uso.
func (uc *UserClient) DeleteState(state string) {
	uc.states.delete(state)
}

// BuildLoginURL genera los parámetros PKCE, los almacena temporalmente y retorna el enlace de inicio de sesión de Tidal.
func (uc *UserClient) BuildLoginURL(playlistID string) (string, error) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		return "", fmt.Errorf("generate verifier: %w", err)
	}
	challenge := computeCodeChallenge(verifier)
	state, err := generateState()
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	// Guarda la relación de estado y verifier para validarla al retornar de la redirección.
	uc.states.set(state, OAuthState{
		CodeVerifier: verifier,
		PlaylistID:   playlistID,
		CreatedAt:    time.Now(),
	})
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {uc.clientID},
		"redirect_uri":          {uc.redirectURI},
		"scope":                 {tidalScopes},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
		"state":                 {state},
	}
	return tidalLoginURL + "?" + params.Encode(), nil
}

// OverrideAuthURL reemplaza la URL del endpoint del token (usado en tests).
func (uc *UserClient) OverrideAuthURL(u string) { uc.authURL = u }

// OverrideAPIBase reemplaza la URL base de la OpenAPI de Tidal (usado en tests).
func (uc *UserClient) OverrideAPIBase(u string) { uc.apiBase = u }

type userTokenResponse struct {
	AccessToken string `json:"access_token"`
}

// ExchangeCode intercambia el código de autorización obtenido del callback por un token de acceso del usuario (PKCE sin client_secret).
func (uc *UserClient) ExchangeCode(code, codeVerifier string) (string, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {uc.clientID},
		"code":          {code},
		"redirect_uri":  {uc.redirectURI},
		"code_verifier": {codeVerifier},
	}
	resp, err := uc.httpClient.Post(uc.authURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("token exchange request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, body)
	}
	var tr userTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	return tr.AccessToken, nil
}

type createPlaylistResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

// CreatePlaylist crea una nueva playlist pública en la cuenta personal de Tidal del usuario.
func (uc *UserClient) CreatePlaylist(userToken, title string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"type": "playlists",
			"attributes": map[string]string{
				"name":        title,
				"description": "Playlist generada con CieloWave",
				"accessType":  "PUBLIC",
			},
		},
	})
	req, err := http.NewRequest(http.MethodPost, uc.apiBase+"/v2/playlists?countryCode=US", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := uc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create playlist failed (%d): %s", resp.StatusCode, b)
	}
	var result createPlaylistResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode create playlist response: %w", err)
	}
	return result.Data.ID, nil
}

// AddTracks añade canciones de forma masiva a una playlist del usuario en formato estándar de relaciones de JSON:API.
func (uc *UserClient) AddTracks(userToken, playlistID string, trackIDs []string) error {
	items := make([]map[string]string, len(trackIDs))
	for i, id := range trackIDs {
		items[i] = map[string]string{"type": "tracks", "id": id}
	}
	body, _ := json.Marshal(map[string]any{"data": items})

	endpoint := uc.apiBase + "/v2/playlists/" + url.PathEscape(playlistID) + "/relationships/items"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := uc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("add tracks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add tracks failed (%d): %s", resp.StatusCode, b)
	}
	return nil
}
