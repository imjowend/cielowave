package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"cielowave/backend/internal/playlist"
	"cielowave/backend/internal/tidal"

	"github.com/joho/godotenv"
)

func main() {
	// Cargar variables de entorno desde .env (si existe), con fallback a variables del sistema
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment")
	}

	// Carga las variables de entorno necesarias para el cliente de Tidal
	clientID := os.Getenv("TIDAL_CLIENT_ID")
	clientSecret := os.Getenv("TIDAL_CLIENT_SECRET")

	// Carga el puerto del servidor, con valor por defecto 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Inicializa el cliente de Tidal
	client, err := tidal.NewTidalClient(clientID, clientSecret)
	if err != nil {
		log.Fatalf("failed to initialize Tidal client: %v", err)
	}

	// Configura las rutas del servidor HTTP
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /api/artists", handleSearchArtists(client))
	mux.HandleFunc("GET /api/artists/{id}/tracks", handleGetArtistTracks(client))
	mux.HandleFunc("POST /api/playlist", handleCreatePlaylist(client))

	// Inicia el servidor HTTP con middleware CORS
	log.Printf("CieloWave backend listening on :%s", port)
	if err := http.ListenAndServe(":"+port, corsMiddleware(mux)); err != nil {
		log.Fatalf("server error: %v", err)
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

func handleSearchArtists(c *tidal.TidalClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			writeError(w, http.StatusBadRequest, "missing query parameter: q")
			return
		}
		artists, err := c.SearchArtists(q)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
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
