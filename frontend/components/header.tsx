import { CieloWaveIcon } from "@/components/icons/cielowave-icon";

export function Header() {
  return (
    <header className="sticky top-0 z-50 w-full border-b border-border bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto flex h-16 items-center justify-center px-4">
        <div className="flex items-center gap-2.5">
          <CieloWaveIcon size={36} className="text-primary" />
          <h1 className="text-xl font-bold tracking-tight text-foreground">
            CieloWave
          </h1>
        </div>
      </div>
    </header>
  );
}
