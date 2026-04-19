# CieloWave — Backend

API HTTP en Go que conecta con la API de Tidal para buscar artistas, mezclar tracks y guardar playlists en cuentas de usuario vía OAuth2 PKCE.

## Stack

- **Go 1.26** — stdlib únicamente (`net/http`, `log/slog`)
- **godotenv** — carga de variables de entorno desde `.env`
- Integración con **Tidal API** (client credentials + OAuth2 PKCE)

## Estructura

```
main.go                     → Entrypoint, rutas HTTP, handlers
internal/
  tidal/
    client.go               → TidalClient (búsqueda, tracks) — client credentials
    userclient.go           → UserClient (OAuth2 PKCE, guardar playlists)
    models.go               → Tipos: Artist, Track, PlaylistRequest, etc.
    store.go                → Store en memoria para playlists y estados OAuth
  playlist/
    mixer.go                → MixPlaylist: mezcla y desduplicación de tracks
```

## Endpoints

| Método | Ruta                          | Descripción                              |
|--------|-------------------------------|------------------------------------------|
| GET    | `/health`                     | Health check                             |
| GET    | `/api/artists?q=`             | Buscar artistas en Tidal                 |
| GET    | `/api/artists/{id}/tracks`    | Obtener tracks de un artista             |
| POST   | `/api/playlist`               | Crear playlist mezclada (no guardada)    |
| POST   | `/api/playlist/save`          | Guardar playlist en memoria temporalmente|
| GET    | `/api/auth/tidal/login`       | Iniciar OAuth2 PKCE con Tidal            |
| GET    | `/api/auth/tidal/callback`    | Callback OAuth2 — crea playlist en Tidal |

## Variables de entorno

Crea un archivo `.env` en la raíz del backend:

```env
TIDAL_CLIENT_ID=tu_client_id
TIDAL_CLIENT_SECRET=tu_client_secret
TIDAL_REDIRECT_URI=http://localhost:8080/api/auth/tidal/callback
PORT=8080                   # opcional, default 8080
```

## Correr en local

```bash
go run main.go
# Servidor en http://localhost:8080
```

## Tests

```bash
# Correr todos los tests
go test ./...

# Con verbose output
go test -v ./...

# Solo un paquete
go test ./internal/playlist/...
go test ./internal/tidal/...
go test .                   # tests del paquete main
```

Los tests usan exclusivamente `net/http/httptest` — no requieren credenciales reales ni servidor externo. Los `UserClient` y `TidalClient` exponen helpers (`OverrideAuthURL`, `OverrideAPIBase`, `NewTidalClientForTest`) para inyectar servidores mock en tests.

### Cobertura

```bash
go test -cover ./...
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```
