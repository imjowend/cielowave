package tidal

import (
	"sync"
	"time"
)

const (
	playlistTTL = 30 * time.Minute
	stateTTL    = 10 * time.Minute
)

// SavedPlaylist holds a generated playlist pending OAuth save.
type SavedPlaylist struct {
	ID        string
	ArtistA   string
	ArtistB   string
	Tracks    []Track
	CreatedAt time.Time
}

// OAuthState holds PKCE state for an in-flight auth flow.
type OAuthState struct {
	CodeVerifier string
	PlaylistID   string
	CreatedAt    time.Time
}

// PlaylistStore is a thread-safe in-memory store for SavedPlaylist with TTL.
type PlaylistStore struct {
	mu      sync.Mutex
	entries map[string]SavedPlaylist
}

func newPlaylistStore() *PlaylistStore {
	return &PlaylistStore{entries: make(map[string]SavedPlaylist)}
}

func (s *PlaylistStore) set(id string, p SavedPlaylist) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[id] = p
}

func (s *PlaylistStore) get(id string) (SavedPlaylist, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.entries[id]
	if !ok || time.Now().After(p.CreatedAt.Add(playlistTTL)) {
		return SavedPlaylist{}, false
	}
	return p, true
}

func (s *PlaylistStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
}

func (s *PlaylistStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, p := range s.entries {
		if now.After(p.CreatedAt.Add(playlistTTL)) {
			delete(s.entries, id)
		}
	}
}

// OAuthStateStore is a thread-safe in-memory store for OAuthState with TTL.
type OAuthStateStore struct {
	mu      sync.Mutex
	entries map[string]OAuthState
}

func newOAuthStateStore() *OAuthStateStore {
	return &OAuthStateStore{entries: make(map[string]OAuthState)}
}

func (s *OAuthStateStore) set(state string, o OAuthState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[state] = o
}

func (s *OAuthStateStore) get(state string) (OAuthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.entries[state]
	if !ok || time.Now().After(o.CreatedAt.Add(stateTTL)) {
		return OAuthState{}, false
	}
	return o, true
}

func (s *OAuthStateStore) delete(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, state)
}

func (s *OAuthStateStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, o := range s.entries {
		if now.After(o.CreatedAt.Add(stateTTL)) {
			delete(s.entries, k)
		}
	}
}
