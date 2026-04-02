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

// recordingsResponse models GET /ws/2/recording/?query=arid:{mbid}&fmt=json&limit=10
type recordingsResponse struct {
	Recordings []struct {
		ISRCs []string `json:"isrcs"`
	} `json:"recordings"`
}

// releasesResponse models GET /ws/2/release/?artist={mbid}&fmt=json&limit=1&inc=recordings+isrcs
type releasesResponse struct {
	Releases []struct {
		Media []struct {
			Tracks []struct {
				Recording struct {
					ISRCs []string `json:"isrcs"`
				} `json:"recording"`
			} `json:"tracks"`
		} `json:"media"`
	} `json:"releases"`
}
