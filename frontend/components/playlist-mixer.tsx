"use client";

import { useState, useCallback } from "react";
import { Loader2, RefreshCw, Music, ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { ArtistCombobox } from "@/components/artist-combobox";
import { TrackList } from "@/components/track-list";
import { QRSaveTidal } from "@/components/qr-save-tidal";
import type { Artist, Track, PlaylistResponse } from "@/types";

const API_URL = "";

export function PlaylistMixer() {
  const [artistA, setArtistA] = useState<Artist | null>(null);
  const [artistB, setArtistB] = useState<Artist | null>(null);
  const [songCount, setSongCount] = useState(20);
  const [tracks, setTracks] = useState<Track[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [hasGenerated, setHasGenerated] = useState(false);
  const [showQR, setShowQR] = useState(false);
  const [playlistId, setPlaylistId] = useState<string | null>(null);

  // Build the Tidal authorization URL with playlist state
  const tidalAuthUrl = playlistId
    ? `${API_URL}/api/auth/tidal/login?playlist_id=${playlistId}`
    : null;

  const generatePlaylist = useCallback(async () => {
    if (!artistA || !artistB) return;

    setLoading(true);
    setShowQR(false); // Reset QR visibility on new generation
    setPlaylistId(null); // Reset playlist ID for new generation
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

        // Save playlist to backend and get the server-generated ID
        try {
          const saveResponse = await fetch(`${API_URL}/api/playlist/save`, {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({
              artistA: artistA.name,
              artistB: artistB.name,
              tracks: data.tracks,
            }),
          });

          if (saveResponse.ok) {
            const saveData = await saveResponse.json();
            setPlaylistId(saveData.playlist_id);
          } else {
            console.error("Failed to save playlist:", saveResponse.statusText);
            setPlaylistId(null);
          }
        } catch (saveError) {
          console.error("Failed to save playlist:", saveError);
          setPlaylistId(null);
        }
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
            label="Primer artista"
            value={artistA}
            onSelect={setArtistA}
          />
          <ArtistCombobox
            label="Segundo artista"
            value={artistB}
            onSelect={setArtistB}
          />
        </div>
      </section>

      {/* Song Count Slider */}
      <section className="flex flex-col gap-4">
        <div className="flex items-center justify-between">
          <label className="text-sm font-medium text-muted-foreground">
            Canciones en tu playlist
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
            Creando tu mezcla...
          </>
        ) : (
          <>
            <Music className="h-5 w-5" />
            Crear mi playlist
          </>
        )}
      </Button>

      {/* Playlist Result */}
      {hasGenerated && tracks.length > 0 && artistA && artistB && (
        <section className="flex flex-col gap-4">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 className="text-lg font-semibold text-foreground text-balance">
                Tu CieloWave — {artistA.name} × {artistB.name}
              </h2>
              <p className="text-sm text-muted-foreground">
                {totalCount} canciones listas para ti
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={generatePlaylist}
              disabled={loading}
            >
              <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
              Mezclar de nuevo
            </Button>
          </div>
          <TrackList tracks={tracks} artistA={artistA} artistB={artistB} />

          {/* Save to TIDAL Section */}
          {tidalAuthUrl && (
            <div className="mt-6 flex flex-col gap-4 rounded-lg border border-border bg-card/50 p-4">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <h3 className="font-semibold text-foreground">
                    Llévala contigo
                  </h3>
                  <p className="text-sm text-muted-foreground">
                    Guarda tu mezcla en TIDAL y escúchala donde quieras
                  </p>
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setShowQR(!showQR)}
                  >
                    {showQR ? "Ocultar QR" : "Mostrar QR"}
                  </Button>
                  <Button
                    size="sm"
                    onClick={() => {
                      window.location.href = tidalAuthUrl;
                    }}
                  >
                    <ExternalLink className="h-4 w-4" />
                    Añadir a mi TIDAL
                  </Button>
                </div>
              </div>

              {/* QR Code for mobile scanning */}
              {showQR && (
                <div className="flex justify-center pt-2">
                  <QRSaveTidal authUrl={tidalAuthUrl} />
                </div>
              )}
            </div>
          )}
        </section>
      )}
    </div>
  );
}
