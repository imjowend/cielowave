package tidal

// Artist represents a Tidal artist.
type Artist struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl,omitempty"`
}

// Track represents a Tidal track.
type Track struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	DurationSeconds int    `json:"durationSeconds,omitzero"`
	ArtistID        string `json:"artistId"`
	ArtistName      string `json:"artistName"`
	AlbumName       string `json:"albumName"`
	ISRC            string `json:"isrc,omitempty"`
}

// PlaylistRequest is the request body for POST /api/playlist.
type PlaylistRequest struct {
	ArtistAID string `json:"artistAId"`
	ArtistBID string `json:"artistBId"`
	Count     int    `json:"count"`
}

// PlaylistResponse is the response body for POST /api/playlist.
type PlaylistResponse struct {
	Tracks     []Track `json:"tracks"`
	TotalCount int     `json:"totalCount"`
}
