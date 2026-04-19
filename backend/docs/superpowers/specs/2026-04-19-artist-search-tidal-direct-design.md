# Design: Artist Search — Tidal Direct

**Date:** 2026-04-19  
**Status:** Approved

## Problem

`handleSearchArtists` routes through MusicBrainz → ISRC → Tidal, causing two issues:
- **Slow**: 3+ sequential/parallel external API calls per query
- **Inaccurate**: MusicBrainz results don't always map to the correct Tidal artist (e.g. "Daft Punk" → "Stardust")

## Solution

Use Tidal's native search API directly. Drop MusicBrainz entirely.

## Changes

### `internal/tidal/client.go` — `SearchArtists`

- Add `limit=10` to the search query: `GET /v2/searchresults/{query}?countryCode=US&include=artists&limit=10`
- Update `artistAttributes` to parse the first `imageLinks[0].href` as the image URL
- Remove the parallel `GetArtistImage` calls (separate `/v2/artworks/` round-trips) — image URLs are already in the search response
- Return artists in Tidal's natural order; sorting is the caller's responsibility

### `main.go` — `handleSearchArtists`

- Simplify handler to: `SearchArtists(q)` → sort → slice to 5
- Sort with `sort.SliceStable`: artists whose name begins with `q` (case-insensitive) come first; ties preserve Tidal order
- Remove `artistCache` struct, `artistCacheEntry`, `newArtistCache()`, and `get`/`set` methods
- Remove `mb *musicbrainz.MusicBrainzClient` and `cache *artistCache` parameters from the handler
- Remove all goroutine/semaphore/resolution logic (ISRC resolution, `resultCh`, `sem`)
- Remove `mbClient` and `cache` initialization from `main()`
- Remove `cielowave/backend/internal/musicbrainz` import

## Data Flow (after)

```
GET /api/artists?q=daft+punk
  → Tidal GET /v2/searchresults/daft+punk?countryCode=US&include=artists&limit=10
  → sort: starts-with-query first, then Tidal order
  → slice to top 5
  → JSON: [{ id, name, imageUrl }, ...]
```

## Response Shape

```json
[
  { "id": "123", "name": "Daft Punk", "imageUrl": "https://..." },
  ...
]
```

## Out of Scope

- The `POST /api/playlist/save` + `GET /api/auth/tidal/login` + `GET /api/auth/tidal/callback` flow is already fully implemented and requires no changes.
- The `musicbrainz` package itself is not deleted (may be used elsewhere or in tests) — only its usage in `main.go` is removed.
