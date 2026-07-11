# Metadata real de artista/álbum/año en tracks mezclados — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que `POST /api/playlist` devuelva `artistName`, `albumName` y `releaseDate` reales por cada track (hoy llegan vacíos), y que el frontend muestre el año además de artista y álbum.

**Architecture:** El bug raíz es que `trackAttributes` (backend/internal/tidal/client.go) asume que `album` y `artists` viven anidados dentro de `attributes` de cada track. En la API real de Tidal v2 viven bajo `relationships` (referencias `{id,type}`) y hay que resolverlas contra el array `included` de la respuesta, igual que ya se hace para los tracks en sí. Se valida contra la API real (ver "Evidencia de validación" abajo) antes de escribir el código.

**Tech Stack:** Go 1.2x (backend, stdlib `encoding/json` + `net/http/httptest`), Next.js/TypeScript (frontend).

## Global Constraints

- No se modifica el flujo de autenticación de usuario (PKCE) ni `userclient.go` — fuera de alcance.
- Mantener comentarios en español, siguiendo el estilo existente del repo.
- Los tests nuevos deben poder correr sin credenciales reales (mock con `httptest.NewServer`, patrón ya usado en `client_test.go`).
- No romper los tests existentes (dos de ellos — `TestSearchArtists_SendsLimit10` y `TestSearchArtists_ParsesImageLinks` — ya fallan hoy en `main`, por un bug no relacionado en `SearchArtists`; no son parte de este plan, no los toques).

## Evidencia de validación (ya verificada, no repetir)

Se hizo una llamada real (client-credentials, sin tocar datos de usuario) a:

```
GET https://openapi.tidal.com/v2/artists/6772771/relationships/tracks?countryCode=US&include=tracks,tracks.artists,tracks.albums&collapseBy=FINGERPRINT&page[limit]=2
```

Resultado real (recortado), confirma la forma exacta:

```json
{
  "id": "108093771",
  "type": "tracks",
  "attributes": {
    "title": "Rockstar",
    "isrc": "CA5KR1702134",
    "duration": "PT1M52S"
  },
  "relationships": {
    "artists": { "data": [{ "id": "6772771", "type": "artists" }] },
    "albums":  { "data": [{ "id": "108093770", "type": "albums" }] }
  }
}
```

```json
{ "id": "6772771", "type": "artists", "attributes": { "name": "Maria Becerra" } }
```

```json
{
  "id": "108093770",
  "type": "albums",
  "attributes": { "title": "Rockstar", "releaseDate": "2018-03-09" }
}
```

Conclusiones confirmadas:
1. `include=tracks,tracks.artists,tracks.albums` (notación anidada con punto, no `include=tracks,artists,albums` plano) funciona en una sola llamada — la respuesta trajo 20 tracks + 26 artists + 19 albums en `included`.
2. `attributes` de un track **nunca** tiene `album` ni `artists` anidados — por eso `AlbumName`/`ArtistName` siempre quedaban vacíos.
3. `releaseDate` del álbum viene como string simple `"YYYY-MM-DD"`.
4. No existe ningún campo tipo `main`/`principal` en `relationships.artists.data[]` para marcar el artista protagonista (el código viejo chequeaba `a.Main`, que nunca existió en la respuesta real — era otro campo muerto). Tracks con colaboración (ej. `"She Don't Give a Fo"`) traen dos entradas en `artists.data[]`, en orden; se toma la primera como artista principal, igual que hacía el fallback del código viejo.

---

## File Structure

- Modificar `backend/internal/tidal/client.go`:
  - `jsonAPIResource`: sumar campo `Relationships json.RawMessage`.
  - `trackAttributes`: sacar los campos muertos `Album`/`Artists` (nunca vinieron poblados).
  - Sumar structs nuevos: `trackRelationships`, `artistAttributes`, `albumAttributes`.
  - `GetArtistTracks`: cambiar el `include` de la URL, indexar `included` por `tipo:id`, resolver artista+álbum reales por track.
- Modificar `backend/internal/tidal/models.go`: sumar `ReleaseDate string` a `Track`.
- Modificar `backend/internal/tidal/client_test.go`: nuevo test `TestGetArtistTracks_ResolvesArtistAlbumFromRelationships` con fixture realista (basada en la respuesta real capturada arriba).
- Modificar `frontend/types/index.ts`: sumar `releaseDate?: string` a `Track`.
- Modificar `frontend/components/track-list.tsx`: mostrar el año (derivado de `releaseDate`) junto a artista/álbum.

---

### Task 1: Backend — resolver artista/álbum/año reales vía `relationships`

**Files:**
- Modify: `backend/internal/tidal/client.go:47-52` (jsonAPIResource), `:173-186` (trackAttributes), `:366-464` (GetArtistTracks)
- Modify: `backend/internal/tidal/models.go:10-19` (Track struct)
- Test: `backend/internal/tidal/client_test.go`

**Interfaces:**
- Consumes: nada nuevo de otras tasks.
- Produces: `tidal.Track` con `ArtistID`, `ArtistName`, `AlbumName`, `ReleaseDate` poblados correctamente. `PlaylistResponse.Tracks[].releaseDate` (JSON, vía tag `json:"releaseDate,omitempty"`) — usado por Task 2.

- [ ] **Step 1: Escribir el test que falla, con fixture basada en la respuesta real capturada**

Agregar al final de `backend/internal/tidal/client_test.go`:

```go
func TestGetArtistTracks_ResolvesArtistAlbumFromRelationships(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !containsParam(r.URL.RawQuery, "include=tracks,tracks.artists,tracks.albums") {
			t.Errorf("expected nested include param, got %q", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []any{
				map[string]string{"id": "108093771", "type": "tracks"},
				map[string]string{"id": "108409581", "type": "tracks"},
			},
			"included": []any{
				map[string]any{
					"id":   "108093771",
					"type": "tracks",
					"attributes": map[string]any{
						"title":    "Rockstar",
						"duration": "PT1M52S",
						"isrc":     "CA5KR1702134",
					},
					"relationships": map[string]any{
						"artists": map[string]any{"data": []any{
							map[string]string{"id": "6772771", "type": "artists"},
						}},
						"albums": map[string]any{"data": []any{
							map[string]string{"id": "108093770", "type": "albums"},
						}},
					},
				},
				map[string]any{
					"id":   "108409581",
					"type": "tracks",
					"attributes": map[string]any{
						"title":    "She Don't Give a Fo",
						"duration": "PT3M5S",
						"isrc":     "QZW9L2249675",
					},
					"relationships": map[string]any{
						"artists": map[string]any{"data": []any{
							map[string]string{"id": "6772771", "type": "artists"},
							map[string]string{"id": "9031404", "type": "artists"},
						}},
						"albums": map[string]any{"data": []any{
							map[string]string{"id": "108409580", "type": "albums"},
						}},
					},
				},
				map[string]any{
					"id":         "6772771",
					"type":       "artists",
					"attributes": map[string]any{"name": "Duki"},
				},
				map[string]any{
					"id":         "9031404",
					"type":       "artists",
					"attributes": map[string]any{"name": "Maria Becerra"},
				},
				map[string]any{
					"id":   "108093770",
					"type": "albums",
					"attributes": map[string]any{
						"title":       "Rockstar",
						"releaseDate": "2018-03-09",
					},
				},
				map[string]any{
					"id":   "108409580",
					"type": "albums",
					"attributes": map[string]any{
						"title":       "She Don't Give a Fo (Single)",
						"releaseDate": "2019-11-21",
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestTidalClient(srv.URL)
	tracks, err := c.GetArtistTracks("6772771", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}

	solo := tracks[0]
	if solo.ArtistName != "Duki" || solo.ArtistID != "6772771" {
		t.Errorf("track solista: esperaba artista Duki/6772771, got %q/%q", solo.ArtistName, solo.ArtistID)
	}
	if solo.AlbumName != "Rockstar" {
		t.Errorf("esperaba albumName=Rockstar, got %q", solo.AlbumName)
	}
	if solo.ReleaseDate != "2018-03-09" {
		t.Errorf("esperaba releaseDate=2018-03-09, got %q", solo.ReleaseDate)
	}

	collab := tracks[1]
	if collab.ArtistName != "Duki" || collab.ArtistID != "6772771" {
		t.Errorf("track colab: esperaba primer artista Duki/6772771, got %q/%q", collab.ArtistName, collab.ArtistID)
	}
	if collab.AlbumName != "She Don't Give a Fo (Single)" {
		t.Errorf("esperaba albumName de colab, got %q", collab.AlbumName)
	}
	if collab.ReleaseDate != "2019-11-21" {
		t.Errorf("esperaba releaseDate=2019-11-21, got %q", collab.ReleaseDate)
	}
}
```

- [ ] **Step 2: Correr el test y confirmar que falla**

```bash
cd backend && go test ./internal/tidal/... -run TestGetArtistTracks_ResolvesArtistAlbumFromRelationships -v
```

Esperado: FAIL — `ArtistName`/`AlbumName`/`ReleaseDate` vacíos, y el chequeo de `include=tracks,tracks.artists,tracks.albums` también falla porque hoy la URL manda `include=tracks` a secas.

- [ ] **Step 3: Sumar `ReleaseDate` a `Track`**

En `backend/internal/tidal/models.go`, dentro del struct `Track`:

```go
// Track represents a Tidal track.
type Track struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	DurationSeconds int    `json:"durationSeconds,omitempty"`
	ISRC            string `json:"isrc,omitempty"`
	AlbumName       string `json:"albumName,omitempty"`
	ArtistID        string `json:"artistId,omitempty"`
	ArtistName      string `json:"artistName,omitempty"`
	ReleaseDate     string `json:"releaseDate,omitempty"`
}
```

- [ ] **Step 4: Sumar `Relationships` a `jsonAPIResource` y sacar los campos muertos de `trackAttributes`**

En `backend/internal/tidal/client.go`, reemplazar:

```go
// jsonAPIResource representa un único recurso en el formato estándar de JSON:API.
type jsonAPIResource struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Attributes json.RawMessage `json:"attributes"`
}
```

por:

```go
// jsonAPIResource representa un único recurso en el formato estándar de JSON:API.
type jsonAPIResource struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Attributes    json.RawMessage `json:"attributes"`
	Relationships json.RawMessage `json:"relationships"`
}
```

Y reemplazar:

```go
// trackAttributes modela los atributos de una canción en Tidal v2.
type trackAttributes struct {
	Title    string  `json:"title"`
	Duration flexInt `json:"duration"`
	ISRC     string  `json:"isrc"`
	Album    struct {
		Title string `json:"title"`
	} `json:"album"`
	Artists []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Main bool   `json:"main"`
	} `json:"artists"`
}
```

por:

```go
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
```

- [ ] **Step 5: Actualizar `GetArtistTracks` — include anidado + resolución por relationships**

Reemplazar la línea del path (línea 369):

```go
	path := "/v2/artists/" + url.PathEscape(artistID) + "/relationships/tracks?countryCode=US&include=tracks&collapseBy=FINGERPRINT"
```

por:

```go
	path := "/v2/artists/" + url.PathEscape(artistID) + "/relationships/tracks?countryCode=US&include=tracks,tracks.artists,tracks.albums&collapseBy=FINGERPRINT"
```

Reemplazar el bloque de indexado e iteración (desde `includedByID := make(...)` hasta el cierre del `for _, ref := range tr.Data`):

```go
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
```

- [ ] **Step 6: Correr el test y confirmar que pasa**

```bash
cd backend && go test ./internal/tidal/... -run TestGetArtistTracks_ResolvesArtistAlbumFromRelationships -v
```

Esperado: PASS.

- [ ] **Step 7: Correr toda la suite del paquete (menos los dos tests ya rotos de antes) y el build completo**

```bash
cd backend && go build ./... && go test ./internal/playlist/... -v && go test ./internal/tidal/... -run 'TestGetArtistTracks|TestDuration|TestCollapse' -v
```

Esperado: build limpio, todo PASS (los tests de `SearchArtists` seguirán fallando, son un bug preexistente fuera de este plan).

- [ ] **Step 8: Commit**

```bash
git add backend/internal/tidal/client.go backend/internal/tidal/models.go backend/internal/tidal/client_test.go
git commit -m "fix(tidal): resolver artistName/albumName/releaseDate reales vía relationships"
```

---

### Task 2: Frontend — mostrar el año en la lista de tracks mezclados

**Files:**
- Modify: `frontend/types/index.ts:17-24`
- Modify: `frontend/components/track-list.tsx:36-40`

**Interfaces:**
- Consumes: `releaseDate?: string` presente en cada `Track` devuelto por `POST /api/playlist` (Task 1), formato `"YYYY-MM-DD"`, puede venir ausente en tracks sin álbum resuelto (`omitempty`).
- Produces: nada consumido por otra task.

- [ ] **Step 1: Sumar el campo al tipo `Track` del frontend**

En `frontend/types/index.ts`, reemplazar:

```ts
export interface Track {
  id: string;
  title: string;
  artistId: string;
  artistName: string;
  albumName: string;
  durationSeconds: number;
}
```

por:

```ts
export interface Track {
  id: string;
  title: string;
  artistId: string;
  artistName: string;
  albumName: string;
  durationSeconds: number;
  releaseDate?: string;
}
```

- [ ] **Step 2: Mostrar el año en `TrackList`**

En `frontend/components/track-list.tsx`, reemplazar:

```tsx
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <span className="truncate">{track.artistName}</span>
                  <span className="shrink-0">•</span>
                  <span className="truncate">{track.albumName}</span>
                </div>
```

por:

```tsx
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <span className="truncate">{track.artistName}</span>
                  <span className="shrink-0">•</span>
                  <span className="truncate">{track.albumName}</span>
                  {track.releaseDate && (
                    <>
                      <span className="shrink-0">•</span>
                      <span className="shrink-0">{track.releaseDate.slice(0, 4)}</span>
                    </>
                  )}
                </div>
```

- [ ] **Step 3: Verificar manualmente en el navegador**

```bash
cd frontend && npm run dev
```

Abrir `http://localhost:3000`, mezclar Duki × YSY A, confirmar visualmente que cada tema muestra: título, artista, álbum y año (ej. "Duki • Rockstar • 2018").

- [ ] **Step 4: Commit**

```bash
git add frontend/types/index.ts frontend/components/track-list.tsx
git commit -m "feat(frontend): mostrar año de lanzamiento en la lista de tracks mezclados"
```

---

## Self-Review

**Cobertura del objetivo:** "que artistName/albumName lleguen reales" → Task 1. "agregar año" → Task 1 (backend) + Task 2 (frontend). "álbum" y "artista" ya existían en el tipo/response, solo estaban vacíos por el bug — cubiertos por Task 1.

**Placeholders:** ninguno — todo el código de cada step está completo y es el diff real a aplicar.

**Consistencia de tipos:** `Track.ReleaseDate` (Go, `models.go`) ↔ `releaseDate` (JSON tag) ↔ `Track.releaseDate` (TS) — mismo nombre de campo en los tres lugares, formato string `"YYYY-MM-DD"` consistente con lo observado en la API real.
