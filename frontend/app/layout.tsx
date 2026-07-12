import type { Metadata } from "next";
import "./globals.css";
import { ThemeProvider } from "@/components/theme-provider";
import { BackgroundGlow } from "@/components/background-glow";

export const metadata: Metadata = {
  title: "CieloWave - Playlist Mixer",
  description: "Mix your favorite artists into the perfect playlist",
  icons: {
    icon: "/icon.svg",
  },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="es" suppressHydrationWarning>
      <body className="font-sans antialiased">
        <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
          <BackgroundGlow />
          {children}
        </ThemeProvider>
      </body>
    </html>
  );
}
