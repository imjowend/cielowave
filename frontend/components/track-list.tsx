"use client";

import { Card } from "@/components/ui/card";
import { formatDuration } from "@/lib/utils";
import type { Track, Artist } from "@/types";

interface TrackListProps {
  tracks: Track[];
  artistA: Artist;
  artistB: Artist;
}

export function TrackList({ tracks, artistA, artistB }: TrackListProps) {
  return (
    <div className="flex flex-col gap-2">
      {tracks.map((track, index) => {
        const isArtistA = track.artistId === artistA.id;

        return (
          <Card
            key={`${track.id}-${index}`}
            className={`border-0 p-3 transition-colors ${
              isArtistA
                ? "bg-artist-a"
                : "bg-artist-b"
            }`}
          >
            <div className="flex items-center gap-4">
              <span className="w-6 text-center text-sm font-medium text-muted-foreground">
                {index + 1}
              </span>
              <div className="flex min-w-0 flex-1 flex-col">
                <span className="truncate font-medium text-foreground">
                  {track.title}
                </span>
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <span className="truncate">{track.artistName}</span>
                  <span className="shrink-0">•</span>
                  <span className="truncate">{track.albumName}</span>
                </div>
              </div>
              <span className="shrink-0 text-sm text-muted-foreground">
                {formatDuration(track.durationSeconds)}
              </span>
            </div>
          </Card>
        );
      })}
    </div>
  );
}
