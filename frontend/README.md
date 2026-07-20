# CieloWave — Frontend

App en Next.js 15 (App Router) para mezclar las discografías de dos artistas de Tidal
y guardar la playlist resultante en tu cuenta. Consume la API del backend bajo `/api`.

## Requisitos

- **Node.js 18.18+** (mínimo requerido por Next.js 15)
  <!-- TODO: confirmar versión exacta; no hay campo `engines` en package.json -->
- **pnpm** (hay `pnpm-lock.yaml` en el repo)

## Variables de entorno

No requiere variables de entorno. El destino del backend está resuelto en
`next.config.ts` según el entorno (ver siguiente sección).

## Correr en local

```bash
pnpm install
pnpm dev        # http://localhost:3000
```

Requiere el backend corriendo en `http://localhost:8080`.

| Comando       | Descripción              |
|---------------|--------------------------|
| `pnpm dev`    | Servidor de desarrollo   |
| `pnpm build`  | Build de producción      |
| `pnpm start`  | Servidor de producción   |
| `pnpm lint`   | Linter (ESLint)          |

## Cómo pega al backend

`next.config.ts` define un **rewrite** de `/api/:path*` hacia el backend, con destino
según el entorno:

- desarrollo (`NODE_ENV=development`) → `http://localhost:8080`
- producción → `https://cielowave-api.joaquinvasquez.com`

El frontend siempre llama a rutas relativas `/api/...`; Next.js las proxea al backend
que corresponda, así que no hay CORS del lado del cliente.

## Deploy

Deployado en **Vercel**: `cielowave.vercel.app`. El destino de producción del rewrite
está hardcodeado en `next.config.ts` (no depende de una env var de build).
