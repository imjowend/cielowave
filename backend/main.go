package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"cielowave/backend/internal/musicbrainz"
	"cielowave/backend/internal/playlist"
	"cielowave/backend/internal/tidal"

	"github.com/joho/godotenv"
)

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

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	// Cargar variables de entorno desde .env (si existe), con fallback a variables del sistema
	if err := godotenv.Load(); err != nil {
		slog.Info("No .env file found, using system environment")
	}

	// Carga las variables de entorno necesarias para el cliente de Tidal
	clientID := os.Getenv("TIDAL_CLIENT_ID")
	clientSecret := os.Getenv("TIDAL_CLIENT_SECRET")
	redirectURI := os.Getenv("TIDAL_REDIRECT_URI")

	if redirectURI == "" {
		slog.Error("TIDAL_REDIRECT_URI is required")
		os.Exit(1)
	}

	// Carga el puerto del servidor, con valor por defecto 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Inicializa el cliente de Tidal
	client, err := tidal.NewTidalClient(clientID, clientSecret)
	if err != nil {
		slog.Error("failed to initialize Tidal client", "err", err)
		os.Exit(1)
	}

	userClient := tidal.NewUserClient(clientID, redirectURI)

	// Inicializa el cliente de MusicBrainz
	mbUserAgent := os.Getenv("MUSICBRAINZ_USER_AGENT")
	if mbUserAgent == "" {
		mbUserAgent = "CieloWave/0.1.0 (noreply@example.com)"
	}
	mbClient := musicbrainz.NewMusicBrainzClient(mbUserAgent)

	// Caché de artistas resueltos (1 hora TTL)
	cache := newArtistCache()

	// Configura las rutas del servidor HTTP
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /api/artists", handleSearchArtists(client, mbClient, cache))
	mux.HandleFunc("GET /api/artists/{id}/tracks", handleGetArtistTracks(client))
	mux.HandleFunc("POST /api/playlist", handleCreatePlaylist(client))
	mux.HandleFunc("POST /api/playlist/save", handleSavePlaylist(userClient))
	mux.HandleFunc("GET /api/auth/tidal/login", handleTidalLogin(userClient))
	mux.HandleFunc("GET /api/auth/tidal/callback", handleTidalCallback(userClient))

	// Inicia el servidor HTTP con middleware CORS
	slog.Info("CieloWave backend listening", "port", port)
	if err := http.ListenAndServe(":"+port, corsMiddleware(mux)); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleSearchArtists(c *tidal.TidalClient, mb *musicbrainz.MusicBrainzClient, cache *artistCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			writeError(w, http.StatusBadRequest, "missing query parameter: q")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		mbResults, err := mb.SearchArtists(q)
		if err != nil {
			writeError(w, http.StatusBadGateway, "musicbrainz search failed: "+err.Error())
			return
		}

		// Resolve Tidal IDs in parallel, max 2 concurrent goroutines (respects MB rate limit).
		const maxConcurrent = 2
		const maxResolved = 3
		sem := make(chan struct{}, maxConcurrent)

		type resolveResult struct {
			artist *tidal.Artist
			idx    int
		}
		resultCh := make(chan resolveResult, len(mbResults))

		for i, mbr := range mbResults {
			go func(i int, name, mbid string) {
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					resultCh <- resolveResult{nil, i}
					return
				}

				cacheKey := strings.ToLower(strings.TrimSpace(name))
				if cached, ok := cache.get(cacheKey); ok {
					slog.Debug("cache hit for artist", "name", name)
					resultCh <- resolveResult{&cached, i}
					return
				}

				isrc, err := mb.GetArtistISRC(mbid)
				if err != nil || isrc == "" {
					slog.Warn("no ISRC for artist", "mbid", mbid, "name", name, "err", err)
					// Fallback: search by artist name directly in Tidal
					artist, _ := c.SearchArtistByName(name)
					if artist != nil {
						imgURL, _ := c.GetArtistImage(artist.ID)
						if imgURL != "" {
							artist.ImageURL = imgURL
						}
						cache.set(cacheKey, *artist)
					}
					resultCh <- resolveResult{artist, i}
					return
				}

				artist, err := c.ResolveArtistByISRC(isrc)
				if err != nil || artist == nil {
					slog.Warn("tidal resolve failed", "isrc", isrc, "err", err)
					resultCh <- resolveResult{nil, i}
					return
				}

				imgURL, _ := c.GetArtistImage(artist.ID)
				if imgURL != "" {
					artist.ImageURL = imgURL
				}
				cache.set(cacheKey, *artist)
				resultCh <- resolveResult{artist, i}
			}(i, mbr.Name, mbr.MBID)
		}

		// Collect results: stop early once maxResolved artists are found, all goroutines finish, or timeout.
		results := make([]*tidal.Artist, len(mbResults))
		resolved := 0
		received := 0
		total := len(mbResults)

	collect:
		for received < total {
			select {
			case res := <-resultCh:
				received++
				results[res.idx] = res.artist
				if res.artist != nil {
					resolved++
					if resolved >= maxResolved {
						break collect
					}
				}
			case <-ctx.Done():
				break collect
			}
		}

		seen := make(map[string]bool)
		artists := make([]tidal.Artist, 0, maxResolved)
		for _, a := range results {
			if a == nil || seen[a.ID] {
				continue
			}
			seen[a.ID] = true
			artists = append(artists, *a)
		}

		writeJSON(w, http.StatusOK, artists)
	}
}

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

func handleCreatePlaylist(c *tidal.TidalClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req tidal.PlaylistRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.ArtistAID == "" || req.ArtistBID == "" {
			writeError(w, http.StatusBadRequest, "artistAId and artistBId are required")
			return
		}
		if req.Count <= 0 {
			req.Count = 20
		}

		var (
			tracksA, tracksB []tidal.Track
			errA, errB       error
		//	wg               sync.WaitGroup
		)
		/*
			wg.Add(2)
			go func() {
				defer wg.Done()
				tracksA, errA = c.GetArtistTracks(req.ArtistAID)
			}()
			go func() {
				defer wg.Done()
				tracksB, errB = c.GetArtistTracks(req.ArtistBID)
			}()
			wg.Wait()
		*/
		tracksA, errA = c.GetArtistTracks(req.ArtistAID, req.Count*2)
		tracksB, errB = c.GetArtistTracks(req.ArtistBID, req.Count*2)
		if errA != nil {
			writeError(w, http.StatusBadGateway, "failed to fetch tracks for artist A: "+errA.Error())
			return
		}
		if errB != nil {
			writeError(w, http.StatusBadGateway, "failed to fetch tracks for artist B: "+errB.Error())
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

func handleSavePlaylist(uc *tidal.UserClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ArtistA string        `json:"artistA"`
			ArtistB string        `json:"artistB"`
			Tracks  []tidal.Track `json:"tracks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.ArtistA == "" || req.ArtistB == "" || len(req.Tracks) == 0 {
			writeError(w, http.StatusBadRequest, "artistA, artistB, and tracks are required")
			return
		}
		id, err := uc.SavePlaylist(req.ArtistA, req.ArtistB, req.Tracks)
		if err != nil {
			slog.Error("save playlist failed", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to save playlist")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"playlist_id": id})
	}
}

func handleTidalLogin(uc *tidal.UserClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		playlistID := r.URL.Query().Get("playlist_id")
		if playlistID == "" {
			writeError(w, http.StatusBadRequest, "missing playlist_id")
			return
		}
		if _, ok := uc.GetPlaylist(playlistID); !ok {
			writeError(w, http.StatusNotFound, "playlist not found or expired")
			return
		}
		loginURL, err := uc.BuildLoginURL(playlistID)
		if err != nil {
			slog.Error("build login URL failed", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to initiate auth")
			return
		}
		http.Redirect(w, r, loginURL, http.StatusFound)
	}
}

func handleTidalCallback(uc *tidal.UserClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		oauthState, ok := uc.GetState(state)
		if !ok {
			slog.Warn("invalid or expired OAuth state", "state", state)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}
		uc.DeleteState(state)

		userToken, err := uc.ExchangeCode(code, oauthState.CodeVerifier)
		if err != nil {
			slog.Error("code exchange failed", "err", err)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		playlist, ok := uc.GetPlaylist(oauthState.PlaylistID)
		if !ok {
			slog.Warn("playlist not found or expired", "playlist_id", oauthState.PlaylistID)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		title := fmt.Sprintf("%s × %s — CieloWave", playlist.ArtistA, playlist.ArtistB)
		playlistID, err := uc.CreatePlaylist(userToken, title)
		if err != nil {
			slog.Error("create playlist failed", "err", err)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		trackIDs := make([]string, len(playlist.Tracks))
		for i, t := range playlist.Tracks {
			trackIDs[i] = t.ID
		}
		if err := uc.AddTracks(userToken, playlistID, trackIDs); err != nil {
			slog.Error("add tracks failed", "err", err)
			http.Redirect(w, r, frontendBase+"?error=auth_failed", http.StatusFound)
			return
		}

		http.Redirect(w, r, frontendBase+"?saved=true", http.StatusFound)
	}
}
