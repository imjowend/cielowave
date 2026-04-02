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
	DurationSeconds int    `json:"durationSeconds,omitempty"`
	ISRC            string `json:"isrc,omitempty"`
	AlbumName       string `json:"albumName,omitempty"`
	ArtistID        string `json:"artistId,omitempty"`
	ArtistName      string `json:"artistName,omitempty"`
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
