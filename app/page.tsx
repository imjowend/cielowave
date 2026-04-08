'use client'

import { useState, useEffect } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Slider } from '@/components/ui/slider'
import { Music, Music2, AlertCircle, CheckCircle2, Loader2 } from 'lucide-react'
import { toast } from 'sonner'

interface Track {
  id: string
  title: string
  duration: number
}

interface Playlist {
  artistA: string
  artistB: string
  tracks: Track[]
}

export default function Home() {
  const router = useRouter()
  const searchParams = useSearchParams()

  const [artistA, setArtistA] = useState('')
  const [artistB, setArtistB] = useState('')
  const [numTracks, setNumTracks] = useState([10])
  const [playlist, setPlaylist] = useState<Playlist | null>(null)
  const [loading, setLoading] = useState(false)
  const [saveState, setSaveState] = useState<'idle' | 'loading' | 'redirecting'>('idle')
  const [showFeedback, setShowFeedback] = useState(false)
  const [feedbackType, setFeedbackType] = useState<'success' | 'error' | null>(null)

  // Handle OAuth feedback from URL params
  useEffect(() => {
    const saved = searchParams.get('saved')
    const error = searchParams.get('error')

    if (saved === 'true') {
      setFeedbackType('success')
      setShowFeedback(true)
      toast.success('¡Playlist guardada en tu Tidal! 🎵', {
        position: 'top-center',
      })
      // Clean up URL
      router.replace('/')
    } else if (error === 'auth_failed') {
      setFeedbackType('error')
      setShowFeedback(true)
      toast.error(
        'No se pudo conectar con Tidal. Intentá de nuevo.',
        { position: 'top-center' }
      )
      // Clean up URL
      router.replace('/')
    }
  }, [searchParams, router])

  const generatePlaylist = async () => {
    if (!artistA.trim() || !artistB.trim()) {
      toast.error('Por favor completa ambos artistas')
      return
    }

    setLoading(true)
    try {
      const response = await fetch('/api/playlist', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          artistA: artistA.trim(),
          artistB: artistB.trim(),
          numTracks: numTracks[0],
        }),
      })

      if (!response.ok) {
        throw new Error('Error generating playlist')
      }

      const data = await response.json()
      setPlaylist(data)
      toast.success('¡Playlist generada! 🎵')
    } catch (error) {
      toast.error('Error al generar la playlist')
      console.error(error)
    } finally {
      setLoading(false)
    }
  }

  const saveTidal = async () => {
    if (!playlist) return

    setSaveState('loading')
    try {
      const response = await fetch('/api/playlist/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          artistA: playlist.artistA,
          artistB: playlist.artistB,
          tracks: playlist.tracks,
        }),
      })

      if (!response.ok) {
        throw new Error('Error saving to Tidal')
      }

      const data = await response.json()
      const playlistId = data.playlist_id

      // Update to redirecting state
      setSaveState('redirecting')

      // Redirect to backend OAuth login
      window.location.href = `/api/auth/tidal/login?playlist_id=${playlistId}`
    } catch (error) {
      setSaveState('idle')
      toast.error('Error al guardar en Tidal')
      console.error(error)
    }
  }

  return (
    <main className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4 md:p-8">
      <div className="max-w-2xl mx-auto">
        {/* Header */}
        <div className="text-center mb-8">
          <div className="flex items-center justify-center gap-2 mb-4">
            <Music className="w-8 h-8 text-indigo-600" />
            <h1 className="text-4xl font-bold text-gray-900">CieloWave</h1>
          </div>
          <p className="text-gray-600">Crea playlists únicamente de dos artistas</p>
        </div>

        {/* Main Card */}
        <Card className="p-8 mb-6 shadow-lg">
          {/* Artist Inputs */}
          <div className="space-y-4 mb-6">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Artista 1
              </label>
              <Input
                placeholder="Ej: The Beatles"
                value={artistA}
                onChange={(e) => setArtistA(e.target.value)}
                disabled={loading}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Artista 2
              </label>
              <Input
                placeholder="Ej: David Bowie"
                value={artistB}
                onChange={(e) => setArtistB(e.target.value)}
                disabled={loading}
              />
            </div>
          </div>

          {/* Tracks Slider */}
          <div className="mb-8">
            <label className="block text-sm font-medium text-gray-700 mb-4">
              Cantidad de canciones: {numTracks[0]}
            </label>
            <Slider
              min={1}
              max={50}
              step={1}
              value={numTracks}
              onValueChange={setNumTracks}
              disabled={loading}
              className="w-full"
            />
          </div>

          {/* Generate Button */}
          <Button
            onClick={generatePlaylist}
            disabled={loading}
            size="lg"
            className="w-full bg-indigo-600 hover:bg-indigo-700 text-white mb-4"
          >
            {loading ? (
              <>
                <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                Generando...
              </>
            ) : (
              <>
                <Music2 className="w-4 h-4 mr-2" />
                Generar Playlist
              </>
            )}
          </Button>
        </Card>

        {/* Playlist Display */}
        {playlist && (
          <Card className="p-8 shadow-lg">
            <div className="mb-6">
              <h2 className="text-2xl font-bold text-gray-900 mb-2">
                Tu Playlist
              </h2>
              <p className="text-gray-600">
                {playlist.artistA} & {playlist.artistB}
              </p>
            </div>

            {/* Tracks List */}
            <div className="space-y-2 mb-8 max-h-96 overflow-y-auto">
              {playlist.tracks.map((track, index) => (
                <div
                  key={track.id}
                  className="flex items-center justify-between p-3 bg-gray-50 rounded-lg hover:bg-gray-100 transition"
                >
                  <div className="flex items-center gap-3 flex-1">
                    <span className="text-sm font-medium text-gray-500 w-6">
                      {index + 1}
                    </span>
                    <span className="text-gray-900">{track.title}</span>
                  </div>
                  <span className="text-sm text-gray-500">
                    {Math.floor(track.duration / 60)}:
                    {String(track.duration % 60).padStart(2, '0')}
                  </span>
                </div>
              ))}
            </div>

            {/* Save to Tidal Button */}
            <Button
              onClick={saveTidal}
              disabled={saveState !== 'idle'}
              variant="outline"
              size="lg"
              className="w-full border-indigo-600 text-indigo-600 hover:bg-indigo-50"
            >
              {saveState === 'idle' && (
                <>
                  <Music className="w-4 h-4 mr-2" />
                  Guardar en Tidal
                </>
              )}
              {saveState === 'loading' && (
                <>
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                  Guardando...
                </>
              )}
              {saveState === 'redirecting' && (
                <>
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                  Conectando con Tidal...
                </>
              )}
            </Button>
          </Card>
        )}
      </div>
    </main>
  )
}
