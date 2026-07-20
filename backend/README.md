# CieloWave — Backend

API HTTP en Go que integra la API de Tidal. Busca artistas y su catálogo de tracks
usando *Client Credentials*, mezcla e intercala las canciones de dos artistas en una
playlist, y permite guardar esa playlist en la cuenta Tidal del usuario mediante un
flujo OAuth2 PKCE. Las playlists generadas y los estados OAuth se mantienen en un
store en memoria (temporal).

## Requisitos

- **Go 1.26**
- Credenciales de una app de desarrollador de Tidal (Client ID + Secret)

## Variables de entorno

El backend carga un `.env` si existe (vía `godotenv`) y lee estas variables:

| Variable              | Requerida | Propósito                                                        |
|-----------------------|-----------|------------------------------------------------------------------|
| `TIDAL_CLIENT_ID`     | sí        | Client ID de la app de Tidal (búsqueda y catálogo)               |
| `TIDAL_CLIENT_SECRET` | sí        | Client Secret de la app de Tidal                                 |
| `TIDAL_REDIRECT_URI`  | sí        | URI de callback del flujo OAuth2 PKCE (el server aborta si falta)|
| `PORT`                | no        | Puerto HTTP de escucha (default `8080`)                          |

## Correr en local

```bash
cd backend
cp .env.example .env   # completar con credenciales reales
go run .
# Servidor en http://localhost:8080
```

## Buildear / deployar

```bash
docker compose up -d --build   # desde la raíz del repo
```

En producción el enrutamiento (TLS, CORS, rate-limit) lo maneja **Traefik** vía labels
en `docker-compose.yml`; el backend solo expone HTTP plano en el puerto 8080.

## Integración externa

- **Tidal API** — dos modos de acceso:
  - *Client Credentials*: búsqueda de artistas y obtención de tracks de catálogo.
  - *OAuth2 PKCE (user token)*: login del usuario, creación de la playlist en su
    cuenta y alta de tracks.

## Endpoints

| Método | Ruta                        | Descripción                                   |
|--------|-----------------------------|-----------------------------------------------|
| GET    | `/health`                   | Health check                                  |
| GET    | `/api/artists?q=`           | Buscar artistas en Tidal (top 5 por prefijo)  |
| GET    | `/api/artists/{id}/tracks`  | Tracks de catálogo de un artista              |
| POST   | `/api/playlist`             | Crear playlist mezclada (no persistida)       |
| POST   | `/api/playlist/save`        | Guardar playlist en memoria y devolver un ID  |
| GET    | `/api/auth/tidal/login`     | Iniciar OAuth2 PKCE con Tidal                 |
| GET    | `/api/auth/tidal/callback`  | Callback OAuth2 — crea la playlist en Tidal   |

## Estructura

```
main.go                  → entrypoint, rutas HTTP y handlers
internal/
  tidal/
    client.go            → TidalClient (búsqueda y tracks, Client Credentials)
    userclient.go        → UserClient (OAuth2 PKCE, guardar playlists)
    store.go             → store en memoria (playlists + estados OAuth)
    models.go            → tipos (Artist, Track, PlaylistRequest, ...)
  playlist/
    mixer.go             → MixPlaylist: intercala y desduplica tracks
```
