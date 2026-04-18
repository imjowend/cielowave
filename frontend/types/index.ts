/**
 * Represents an artist from the Tidal catalog.
 * Used in the artist search combobox.
 */
export interface Artist {
  id: string;
  name: string;
  imageUrl?: string;
}

/**
 * API response type for GET /api/artists?q=...
 * Returns an array of up to 5 artists matching the search query.
 */
export type ArtistsApiResponse = Artist[];

export interface Track {
  id: string;
  title: string;
  artistId: string;
  artistName: string;
  albumName: string;
  durationSeconds: number;
}

export interface PlaylistResponse {
  tracks: Track[];
  totalCount: number;
}
