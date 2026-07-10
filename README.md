# CieloWave

Un mezclador de listas de reproducción de música potenciado por un backend en Go y un frontend en Next.js, integrando las APIs de Tidal y MusicBrainz.

## Estructura del Proyecto

```text
cielowave/
├── backend/    # Servidor API REST en Go
│   ├── internal/
│   │   ├── musicbrainz/  # Cliente de MusicBrainz con limitador de tasa (rate-limit)
│   │   ├── playlist/     # Lógica para mezclar e intercalar playlists
│   │   └── tidal/        # Cliente para la OpenAPI v2 de Tidal
│   └── main.go           # Punto de entrada y enrutamiento HTTP del backend
└── frontend/   # Interfaz web en Next.js (Frontend)
```

---

## 🔍 Arquitectura y Pipeline de Resolución de Artistas

Debido a que el plan de desarrollador estándar de Tidal restringe las búsquedas de catálogo mediante texto (retornando errores `401 Client does not have required access tier` o `404 Not Found` al autenticarse con el token de *Client Credentials*), CieloWave implementa un **pipeline de resolución en múltiples etapas** para buscar y asociar artistas:

```mermaid
graph TD
    A[El usuario busca un artista ej., 'Coldplay'] --> B(Buscar en la API de MusicBrainz)
    B --> C{Obtener el MBID del artista}
    C --> D[Obtener grabaciones desde MusicBrainz]
    D --> E{Extraer el primer código ISRC}
    E --> F[Consultar en la OpenAPI v2 de Tidal /v2/tracks?filter[isrc]=...]
    F --> G{Resolver el ID de artista de Tidal}
    G --> H[Obtener imagen de perfil en alta resolución en Tidal /v2/artworks]
    H --> I[Guardar en artistCache con un TTL de 1 hora]
```

1. **Búsqueda en MusicBrainz:** Evita las restricciones de búsqueda de Tidal consultando la base de datos libre y gratuita de MusicBrainz para encontrar el ID único del artista (`MBID`).
2. **Puente de ISRC:** El backend recupera las grabaciones del artista de MusicBrainz y extrae el **ISRC** (International Standard Recording Code, código de identificación estándar de grabaciones).
3. **Resolución en Tidal:** Consultamos el endpoint `/v2/tracks` de Tidal filtrando por el código ISRC (lo cual está permitido bajo el flujo *Client Credentials*). Esto nos devuelve la pista exacta en Tidal y el ID de artista asociado.
4. **Resolución de Imagen:** El backend consulta las relaciones de arte de perfil en Tidal para resolver y descargar la URL de la imagen en su máxima resolución.
5. **Caché en Memoria:** El artista resuelto se almacena en caché en el backend durante 1 hora para agilizar búsquedas consecutivas y evitar golpear los límites de tasa de las APIs externas.

---

## Instrucciones de Configuración

### Prerrequisitos

- Go 1.26+
- Node.js 18+ (para el frontend en Next.js)
- Una cuenta de desarrollador en Tidal con credenciales de API activas

### 1. Configuración del Backend

1. Ve al directorio del backend:
   ```bash
   cd backend
   ```
2. Crea tu archivo de configuración de entorno desde la plantilla:
   ```bash
   cp .env.example .env
   ```
3. Introduce tus credenciales de desarrollador de Tidal en el archivo `.env`:
   ```env
   TIDAL_CLIENT_ID=tu_client_id
   TIDAL_CLIENT_SECRET=tu_client_secret
   PORT=8080
   ```
4. Ejecuta el servidor en Go:
   ```bash
   go run main.go
   ```

### 2. Configuración del Frontend

1. Ve al directorio del frontend:
   ```bash
   cd frontend
   ```
2. Instala las dependencias necesarias:
   ```bash
   npm install
   ```
3. Inicia el servidor de desarrollo de Next.js:
   ```bash
   npm run dev
   ```
4. Accede a la aplicación en tu navegador web en `http://localhost:3000`.
