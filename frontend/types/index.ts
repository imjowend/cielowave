export interface Artist {
  id: string;
  name: string;
  imageURL?: string;
}

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
