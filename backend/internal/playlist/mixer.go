package playlist

import (
	"math/rand/v2"

	"cielowave/backend/internal/tidal"
)

// MixPlaylist deduplicates tracks from both artists by ID, shuffles each list
// independently with Fisher-Yates, interleaves them (A[0], B[0], A[1], B[1], ...),
// and trims the result to count tracks.
func MixPlaylist(tracksA, tracksB []tidal.Track, count int) []tidal.Track {
	seen := make(map[string]bool)
	a := dedupe(tracksA, seen)
	b := dedupe(tracksB, seen)

	rand.Shuffle(len(a), func(i, j int) { a[i], a[j] = a[j], a[i] })
	rand.Shuffle(len(b), func(i, j int) { b[i], b[j] = b[j], b[i] })

	mixed := make([]tidal.Track, 0, len(a)+len(b))
	ai, bi := 0, 0
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

	if count > 0 && len(mixed) > count {
		mixed = mixed[:count]
	}
	return mixed
}

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
