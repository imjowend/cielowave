package playlist

import (
	"math/rand/v2"

	"cielowave/backend/internal/tidal"
)

// MixPlaylist se encarga de crear una playlist combinada a partir de los catálogos de canciones de dos artistas.
// El proceso consiste en:
// 1. Eliminar canciones duplicadas por ID de ambos artistas (usando una tabla hash compartida).
// 2. Mezclar el orden de las canciones de cada artista independientemente con el algoritmo Fisher-Yates (usando math/rand/v2).
// 3. Intercalar secuencialmente las canciones (Artista A[0], Artista B[0], Artista A[1], Artista B[1], ...).
// 4. Recortar la lista resultante al límite especificado en 'count'.
func MixPlaylist(tracksA, tracksB []tidal.Track, count int) []tidal.Track {
	// seen almacena los IDs de las canciones que ya hemos procesado para evitar duplicados.
	seen := make(map[string]bool)
	
	// Filtra las canciones duplicadas para cada artista.
	a := dedupe(tracksA, seen)
	b := dedupe(tracksB, seen)

	// Mezcla aleatoriamente las canciones del Artista A de manera aleatoria.
	rand.Shuffle(len(a), func(i, j int) { a[i], a[j] = a[j], a[i] })
	
	// Mezcla aleatoriamente las canciones del Artista B de manera aleatoria.
	rand.Shuffle(len(b), func(i, j int) { b[i], b[j] = b[j], b[i] })

	// Prepara un slice con capacidad para alojar la mezcla total de canciones.
	mixed := make([]tidal.Track, 0, len(a)+len(b))
	ai, bi := 0, 0
	
	// Intercala las canciones de ambos artistas mientras queden canciones disponibles.
	for ai < len(a) || bi < len(b) {
		if ai < len(a) {
			mixed = append(mixed, a[ai])
			ai++
		}
		if bi < len(b) {
			mixed = append(mixed, b[bi])
			bi++
		}
	}

	// Si se definió un límite mayor a cero y el total supera ese límite, recorta el slice.
	if count > 0 && len(mixed) > count {
		mixed = mixed[:count]
	}
	return mixed
}

// dedupe elimina canciones duplicadas de un slice basándose en una tabla hash compartida ('seen').
// Retorna un nuevo slice con elementos únicos y actualiza la tabla hash.
func dedupe(tracks []tidal.Track, seen map[string]bool) []tidal.Track {
	result := make([]tidal.Track, 0, len(tracks))
	for _, t := range tracks {
		if !seen[t.ID] {
			seen[t.ID] = true
			result = append(result, t)
		}
	}
	return result
}
