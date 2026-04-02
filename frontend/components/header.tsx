import { Cloud, Waves } from "lucide-react";

export function Header() {
  return (
    <header className="sticky top-0 z-50 w-full border-b border-border bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto flex h-16 items-center justify-center px-4">
        <div className="flex items-center gap-3">
          <div className="relative flex items-center">
            <Cloud className="h-7 w-7 text-primary" />
            <Waves className="absolute -bottom-1 left-2 h-4 w-4 text-primary" />
          </div>
          <h1 className="text-xl font-bold tracking-tight text-foreground">
            CieloWave
          </h1>
        </div>
      </div>
    </header>
  );
}
