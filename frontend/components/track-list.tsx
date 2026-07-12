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
            className={`border-0 p-3 transition-colors hover:bg-blue-600/10 dark:hover:bg-blue-400/10 ${
              isArtistA
                ? "bg-blue-50/90 dark:bg-slate-900/60"
                : "bg-sky-100/80 dark:bg-blue-950/50"
            }`}
          >
            <div className="flex items-center gap-4">
              <span className="w-6 text-center text-sm font-medium text-blue-900/70 dark:text-blue-100/70">
                {index + 1}
              </span>
              <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="truncate font-bold text-blue-950 dark:text-blue-50">
                  {track.title}
                </span>
                <div className="flex items-center gap-2 text-xs text-blue-900/60 dark:text-sky-200/50">
                  {track.artistName && (
                    <span className="truncate">{track.artistName}</span>
                  )}
                  {track.artistName && track.albumName && (
                    <span className="shrink-0">•</span>
                  )}
                  {track.albumName && (
                    <span className="truncate">{track.albumName}</span>
                  )}
                  {track.releaseDate && (track.artistName || track.albumName) && (
                    <span className="shrink-0">•</span>
                  )}
                  {track.releaseDate && (
                    <span className="shrink-0">{track.releaseDate.slice(0, 4)}</span>
                  )}
                </div>
              </div>
              <span className="shrink-0 text-sm text-blue-900/70 dark:text-blue-100/70">
                {formatDuration(track.durationSeconds)}
              </span>
            </div>
          </Card>
        );
      })}
    </div>
  );
}
