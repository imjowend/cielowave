package tidal

import (
	"testing"
	"time"
)

func TestPlaylistStore_SetGet_Hit(t *testing.T) {
	s := newPlaylistStore()
	p := SavedPlaylist{ID: "1", ArtistA: "Duki", ArtistB: "Nicki", CreatedAt: time.Now()}
	s.set("1", p)
	got, ok := s.get("1")
	if !ok {
		t.Fatal("expected hit, got miss")
	}
	if got.ArtistA != "Duki" {
		t.Fatalf("expected ArtistA=Duki, got %q", got.ArtistA)
	}
}

func TestPlaylistStore_GetExpired(t *testing.T) {
	s := newPlaylistStore()
	p := SavedPlaylist{ID: "1", CreatedAt: time.Now().Add(-31 * time.Minute)}
	s.set("1", p)
	_, ok := s.get("1")
	if ok {
		t.Fatal("expected miss for expired entry, got hit")
	}
}

func TestPlaylistStore_Delete(t *testing.T) {
	s := newPlaylistStore()
	p := SavedPlaylist{ID: "1", CreatedAt: time.Now()}
	s.set("1", p)
	s.delete("1")
	_, ok := s.get("1")
	if ok {
		t.Fatal("expected miss after delete, got hit")
	}
}

func TestPlaylistStore_Cleanup(t *testing.T) {
	s := newPlaylistStore()
	s.set("expired", SavedPlaylist{ID: "expired", CreatedAt: time.Now().Add(-31 * time.Minute)})
	s.set("live", SavedPlaylist{ID: "live", CreatedAt: time.Now()})
	s.cleanup()
	if _, ok := s.get("expired"); ok {
		t.Fatal("cleanup should have removed expired entry")
	}
	if _, ok := s.get("live"); !ok {
		t.Fatal("cleanup should not remove live entry")
	}
}

func TestOAuthStateStore_SetGet_Hit(t *testing.T) {
	s := newOAuthStateStore()
	o := OAuthState{CodeVerifier: "abc", PlaylistID: "p1", CreatedAt: time.Now()}
	s.set("state1", o)
	got, ok := s.get("state1")
	if !ok {
		t.Fatal("expected hit, got miss")
	}
	if got.CodeVerifier != "abc" {
		t.Fatalf("expected CodeVerifier=abc, got %q", got.CodeVerifier)
	}
}

func TestOAuthStateStore_GetExpired(t *testing.T) {
	s := newOAuthStateStore()
	o := OAuthState{CreatedAt: time.Now().Add(-11 * time.Minute)}
	s.set("state1", o)
	_, ok := s.get("state1")
	if ok {
		t.Fatal("expected miss for expired state, got hit")
	}
}

func TestOAuthStateStore_Delete(t *testing.T) {
	s := newOAuthStateStore()
	s.set("state1", OAuthState{CreatedAt: time.Now()})
	s.delete("state1")
	_, ok := s.get("state1")
	if ok {
		t.Fatal("expected miss after delete, got hit")
	}
}

func TestOAuthStateStore_Cleanup(t *testing.T) {
	s := newOAuthStateStore()
	s.set("old", OAuthState{CreatedAt: time.Now().Add(-11 * time.Minute)})
	s.set("new", OAuthState{CreatedAt: time.Now()})
	s.cleanup()
	if _, ok := s.get("old"); ok {
		t.Fatal("cleanup should have removed expired state")
	}
	if _, ok := s.get("new"); !ok {
		t.Fatal("cleanup should not remove live state")
	}
}
