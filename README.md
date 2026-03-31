# CieloWave

A music playlist mixer powered by the Tidal API.

## Description

CieloWave is a tool that blends and generates playlists using Tidal's music catalog.

## Project Structure

```
cielowave/
├── backend/   # Go API server
└── frontend/  # Web UI (coming soon)
```

## Setup Instructions

### Prerequisites

- Go 1.21+
- A Tidal developer account with API credentials

### Backend

```bash
cd backend
cp .env.example .env
# Fill in your Tidal credentials in .env
go run main.go
```

### Frontend

_Coming soon — will be added from v0._
