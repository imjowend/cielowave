package musicbrainz

// ArtistResult is a single artist match from a MusicBrainz search.
type ArtistResult struct {
	MBID  string
	Name  string
	Score int
}

// searchResponse models GET /ws/2/artist/?query=...&fmt=json
type searchResponse struct {
	Artists []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Score int    `json:"score"`
	} `json:"artists"`
}

// recordingsResponse models GET /ws/2/recording?artist={mbid}&inc=isrcs&fmt=json&limit=1
type recordingsResponse struct {
	Recordings []struct {
		ISRCs []string `json:"isrcs"`
	} `json:"recordings"`
}
