package main

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

func main() {
	// Configura slog por defecto en formato JSON para salida estándar de errores (logs estructurados).
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	// Carga variables de entorno desde el archivo .env si existe.
	if err := godotenv.Load(); err != nil {
		slog.Info("No se encontró el archivo .env, usando variables de entorno del sistema")
	}

	// Recupera las variables de configuración requeridas.
	clientID := os.Getenv("TIDAL_CLIENT_ID")
	clientSecret := os.Getenv("TIDAL_CLIENT_SECRET")
	redirectURI := os.Getenv("TIDAL_REDIRECT_URI")

	if redirectURI == "" {
		slog.Error("TIDAL_REDIRECT_URI es obligatorio para el inicio de sesión del usuario")
		os.Exit(1)
	}

	// Determina el puerto del servidor HTTP (por defecto 8080).
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Inicializa el cliente base de Tidal (para búsqueda y catálogo con Client Credentials).
	client, err := tidal.NewTidalClient(clientID, clientSecret)
	if err != nil {
		slog.Error("error al inicializar el cliente Tidal", "err", err)
		os.Exit(1)
	}

	// Inicializa el cliente de usuario de Tidal (para autorización PKCE y guardado de playlists).
	userClient := tidal.NewUserClient(clientID, redirectURI)

	// Registra y configura las rutas HTTP.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /api/artists", handleSearchArtists(client))
	mux.HandleFunc("GET /api/artists/{id}/tracks", handleGetArtistTracks(client))
	mux.HandleFunc("POST /api/playlist", handleCreatePlaylist(client))
	mux.HandleFunc("POST /api/playlist/save", handleSavePlaylist(userClient))
	mux.HandleFunc("GET /api/auth/tidal/login", handleTidalLogin(userClient))
	mux.HandleFunc("GET /api/auth/tidal/callback", handleTidalCallback(userClient))

	// Inicia el servidor HTTP. CORS y preflight OPTIONS los maneja Traefik (middleware headers).
	slog.Info("Servidor de CieloWave backend escuchando", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("error crítico del servidor", "err", err)
		os.Exit(1)
	}
}

// writeJSON envía una respuesta formateada en JSON con su correspondiente código HTTP.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError envía una respuesta de error estructurada en JSON.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleHealth retorna el estado de salud del backend (OK).
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// queryMatchScore calcula cuántos caracteres iniciales de 'q' coinciden como prefijo del 'name'.
// Permite ordenar artistas para destacar aquellos cuyo nombre empiece directamente con la consulta.
func queryMatchScore(name, q string) int {
	nl := strings.ToLower(name)
	ql := strings.ToLower(q)
	for i := len(ql); i > 0; i-- {
		if strings.HasPrefix(nl, ql[:i]) {
			return i
		}
	}
	return 0
}

// sortArtistsByQuery ordena los artistas de forma descendente basándose en su puntuación de coincidencia de prefijo.
func sortArtistsByQuery(artists []tidal.Artist, q string) {
	sort.SliceStable(artists, func(i, j int) bool {
		si := queryMatchScore(artists[i].Name, q)
		sj := queryMatchScore(artists[j].Name, q)
		return si > sj
	})
}

// handleSearchArtists busca artistas directamente en la API de Tidal v1 usando el App Token.
func handleSearchArtists(c *tidal.TidalClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			writeError(w, http.StatusBadRequest, "falta el parámetro de búsqueda: q")
			return
		}

		// Llama a la búsqueda directa en Tidal v1.
		artists, err := c.SearchArtists(q)
		if err != nil {
			writeError(w, http.StatusBadGateway, "la búsqueda en tidal falló: "+err.Error())
			return
		}

		// Ordena por coincidencia de prefijo y limita a los mejores 5 resultados.
		sortArtistsByQuery(artists, q)
		if len(artists) > 5 {
			artists = artists[:5]
		}

		writeJSON(w, http.StatusOK, artists)
	}
}

// handleGetArtistTracks obtiene las canciones de catálogo de un artista en Tidal.
func handleGetArtistTracks(c *tidal.TidalClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		tracks, err := c.GetArtistTracks(id, 0)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, tracks)
	}
}

// handleCreatePlaylist mezcla las canciones obtenidas de los dos artistas.
func handleCreatePlaylist(c *tidal.TidalClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req tidal.PlaylistRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "cuerpo de solicitud inválido")
			return
		}
		if req.ArtistAID == "" || req.ArtistBID == "" {
			writeError(w, http.StatusBadRequest, "artistAId y artistBId son obligatorios")
			return
		}
		if req.Count <= 0 {
			req.Count = 20
		}

		var (
			tracksA, tracksB []tidal.Track
			errA, errB       error
		)
		tracksA, errA = c.GetArtistTracks(req.ArtistAID, req.Count*2)
		tracksB, errB = c.GetArtistTracks(req.ArtistBID, req.Count*2)
		if errA != nil {
			writeError(w, http.StatusBadGateway, "fallo al obtener canciones del artista A: "+errA.Error())
			return
		}
		if errB != nil {
			writeError(w, http.StatusBadGateway, "fallo al obtener canciones del artista B: "+errB.Error())
			return
		}

		mixed := playlist.MixPlaylist(tracksA, tracksB, req.Count)
		writeJSON(w, http.StatusOK, tidal.PlaylistResponse{
			Tracks:     mixed,
			TotalCount: len(mixed),
		})
	}
}

const frontendBase = "https://cielowave.vercel.app"

// handleSavePlaylist guarda una playlist generada de forma temporal en memoria y devuelve un identificador UUID.
func handleSavePlaylist(uc *tidal.UserClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ArtistA string        `json:"artistA"`
			ArtistB string        `json:"artistB"`
			Tracks  []tidal.Track `json:"tracks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "cuerpo de solicitud inválido")
			return
		}
		if req.ArtistA == "" || req.ArtistB == "" || len(req.Tracks) == 0 {
			writeError(w, http.StatusBadRequest, "artistA, artistB y tracks son requeridos")
			return
		}
		id, err := uc.SavePlaylist(req.ArtistA, req.ArtistB, req.Tracks)
		if err != nil {
			slog.Error("falló al guardar la playlist", "err", err)
			writeError(w, http.StatusInternalServerError, "error al guardar la playlist temporal")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"playlist_id": id})
	}
}

// handleTidalLogin redirige al usuario a la página de login y autorización de Tidal con PKCE.
func handleTidalLogin(uc *tidal.UserClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		playlistID := r.URL.Query().Get("playlist_id")
		if playlistID == "" {
			writeError(w, http.StatusBadRequest, "falta el parámetro playlist_id")
			return
		}
		if _, ok := uc.GetPlaylist(playlistID); !ok {
			writeError(w, http.StatusNotFound, "playlist no encontrada o expirada")
			return
		}
		loginURL, err := uc.BuildLoginURL(playlistID)
		if err != nil {
			slog.Error("falló al construir URL de login", "err", err)
			writeError(w, http.StatusInternalServerError, "error al iniciar autenticación con Tidal")
			return
		}
		http.Redirect(w, r, loginURL, http.StatusFound)
	}
}

// handleTidalCallback recibe el código de retorno de Tidal, intercambia el código por el token de usuario y crea la playlist.
func handleTidalCallback(uc *tidal.UserClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		oauthState, ok := uc.GetState(state)
		if !ok {
			slog.Warn("estado OAuth inválido o expirado", "state", state)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}
		uc.DeleteState(state)

		userToken, err := uc.ExchangeCode(code, oauthState.CodeVerifier)
		if err != nil {
			slog.Error("intercambio de código fallido", "err", err)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		playlist, ok := uc.GetPlaylist(oauthState.PlaylistID)
		if !ok {
			slog.Warn("playlist no encontrada o expirada", "playlist_id", oauthState.PlaylistID)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		title := fmt.Sprintf("%s × %s — CieloWave", playlist.ArtistA, playlist.ArtistB)
		playlistID, err := uc.CreatePlaylist(userToken, title)
		if err != nil {
			slog.Error("creación de playlist fallida", "err", err)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		trackIDs := make([]string, len(playlist.Tracks))
		for i, t := range playlist.Tracks {
			trackIDs[i] = t.ID
		}
		if err := uc.AddTracks(userToken, playlistID, trackIDs); err != nil {
			slog.Error("falló al agregar pistas a la playlist", "err", err)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		http.Redirect(w, r, frontendBase+"?saved=true", http.StatusFound)
	}
}
