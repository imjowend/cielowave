# CieloWave

Mezclador de playlists que combina las discografías de dos artistas de Tidal en una
sola lista intercalada y la guarda en tu cuenta de Tidal.

## Stack

- **Backend:** Go (stdlib `net/http`), integración con la API de Tidal
- **Frontend:** Next.js 15 + React 19 + TypeScript + Tailwind CSS
- **Deploy:** VPS propio (Traefik) + Vercel

## Estructura del repo

Cada subcarpeta tiene su propio README con el detalle técnico:

- [`backend/`](./backend/README.md) — API HTTP en Go (búsqueda de artistas, mezcla de
  tracks, guardado de playlists vía OAuth2 PKCE)
- [`frontend/`](./frontend/README.md) — interfaz web en Next.js

## Deploy

- **Backend:** VPS propio, enrutado vía Traefik en `cielowave-api.joaquinvasquez.com`
- **Frontend:** Vercel en `cielowave.vercel.app`

## Estado

**Terminado.** Funcionalidades completas: búsqueda de artistas, mezcla/deduplicación
de tracks, guardado de playlist en Tidal vía OAuth2 PKCE.
