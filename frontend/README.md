# CieloWave — Frontend

Next.js 15 app que permite mezclar playlists de dos artistas de Tidal y guardarlas en tu cuenta.

## Stack

- **Next.js 15** (App Router) + **React 19**
- **TypeScript**
- **Tailwind CSS** + **Radix UI** (componentes accesibles)
- **pnpm** como gestor de paquetes

## Estructura

```
app/          → Rutas (App Router)
components/   → UI: PlaylistMixer, ArtistCombobox, TrackList, QR, Header, Footer
hooks/        → useDebounce
lib/          → utils (cn helper)
types/        → Tipos compartidos
```

## Correr en local

```bash
pnpm install
pnpm dev        # http://localhost:3000
```

> Requiere que el backend esté corriendo en `http://localhost:8080` (o configurar `NEXT_PUBLIC_API_URL`).

## Scripts

| Comando        | Descripción                   |
|----------------|-------------------------------|
| `pnpm dev`     | Servidor de desarrollo        |
| `pnpm build`   | Build de producción           |
| `pnpm start`   | Servidor de producción        |
| `pnpm lint`    | Linter (ESLint)               |

## Tests

El frontend no tiene suite de tests configurada actualmente. Para agregar tests se recomienda [Vitest](https://vitest.dev/) + [Testing Library](https://testing-library.com/).

## Deploy

El frontend está deployado en **Vercel**: [cielowave.vercel.app](https://cielowave.vercel.app)
