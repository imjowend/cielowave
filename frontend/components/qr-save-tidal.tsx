"use client";

import { QRCodeSVG } from "qrcode.react";

interface QRSaveTidalProps {
  /** URL that initiates the Tidal OAuth flow with playlist state */
  authUrl: string;
  /** Size of the QR code in pixels */
  size?: number;
}

/**
 * Displays a QR code that allows users to scan and authorize
 * saving the generated playlist to their Tidal account.
 */
export function QRSaveTidal({ authUrl, size = 180 }: QRSaveTidalProps) {
  return (
    <div className="flex flex-col items-center gap-4 rounded-lg border border-border bg-card p-6">
      <div className="rounded-lg bg-white p-3">
        <QRCodeSVG
          value={authUrl}
          size={size}
          level="M"
          includeMargin={false}
          aria-label="QR code to save playlist to Tidal"
        />
      </div>
      <div className="text-center">
        <p className="text-sm font-medium text-foreground">
          Escanea para guardar
        </p>
        <p className="text-xs text-muted-foreground">
          Abre la cámara de tu móvil y escanea el código QR
        </p>
      </div>
    </div>
  );
}
