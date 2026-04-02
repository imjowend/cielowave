"use client";

import { useState, useCallback } from "react";
import { Loader2, RefreshCw, Music } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { ArtistCombobox } from "@/components/artist-combobox";
import { TrackList } from "@/components/track-list";
import type { Artist, Track, PlaylistResponse } from "@/types";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export function PlaylistMixer() {
  const [artistA, setArtistA] = useState<Artist | null>(null);
  const [artistB, setArtistB] = useState<Artist | null>(null);
  const [songCount, setSongCount] = useState(20);
  const [tracks, setTracks] = useState<Track[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [hasGenerated, setHasGenerated] = useState(false);

  const generatePlaylist = useCallback(async () => {
    if (!artistA || !artistB) return;

    setLoading(true);
    try {
      const response = await fetch(`${API_URL}/api/playlist`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          artistAId: artistA.id,
          artistBId: artistB.id,
          count: songCount,
        }),
      });

      if (response.ok) {
        const data: PlaylistResponse = await response.json();
        setTracks(data.tracks || []);
        setTotalCount(data.totalCount);
        setHasGenerated(true);
      }
    } catch (error) {
      console.error("Failed to generate playlist:", error);
    } finally {
      setLoading(false);
    }
  }, [artistA, artistB, songCount]);

  const canGenerate = artistA && artistB && !loading;

  return (
    <div className="flex flex-col gap-8">
      {/* Artist Selection */}
      <section className="flex flex-col gap-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <ArtistCombobox
            label="Artist A"
            value={artistA}
            onSelect={setArtistA}
          />
          <ArtistCombobox
            label="Artist B"
            value={artistB}
            onSelect={setArtistB}
          />
        </div>
      </section>

      {/* Song Count Slider */}
      <section className="flex flex-col gap-4">
        <div className="flex items-center justify-between">
          <label className="text-sm font-medium text-muted-foreground">
            Songs in playlist
          </label>
          <span className="text-lg font-semibold text-primary">{songCount}</span>
        </div>
        <Slider
          min={10}
          max={50}
          step={1}
          value={[songCount]}
          onValueChange={(value) => setSongCount(value[0])}
          className="w-full"
        />
        <div className="flex justify-between text-xs text-muted-foreground">
          <span>10</span>
          <span>50</span>
        </div>
      </section>

      {/* Generate Button */}
      <Button
        size="lg"
        className="w-full text-base font-semibold"
        disabled={!canGenerate}
        onClick={generatePlaylist}
      >
        {loading ? (
          <>
            <Loader2 className="h-5 w-5 animate-spin" />
            Generating...
          </>
        ) : (
          <>
            <Music className="h-5 w-5" />
            Generate Playlist
          </>
        )}
      </Button>

      {/* Playlist Result */}
      {hasGenerated && tracks.length > 0 && artistA && artistB && (
        <section className="flex flex-col gap-4">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 className="text-lg font-semibold text-foreground text-balance">
                Your CieloWave — {artistA.name} × {artistB.name}
              </h2>
              <p className="text-sm text-muted-foreground">
                {totalCount} tracks
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={generatePlaylist}
              disabled={loading}
            >
              <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
              Regenerate
            </Button>
          </div>
          <TrackList tracks={tracks} artistA={artistA} artistB={artistB} />
        </section>
      )}
    </div>
  );
}
