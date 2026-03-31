import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "CieloWave - Playlist Mixer",
  description: "Mix your favorite artists into the perfect playlist",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="es">
      <body className="font-sans antialiased">{children}</body>
    </html>
  );
}
