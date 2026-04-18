import Link from "next/link";
import { CheckCircle2, Home } from "lucide-react";
import { Button } from "@/components/ui/button";

export const metadata = {
  title: "Playlist Guardada - CieloWave",
  description: "Tu playlist ha sido añadida a tu cuenta de TIDAL",
};

/**
 * Success page shown after the user successfully authorizes
 * and the playlist is saved to their Tidal account.
 */
export default function SuccessPage() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center px-4">
      <div className="flex max-w-md flex-col items-center gap-6 text-center">
        <div className="flex h-20 w-20 items-center justify-center rounded-full bg-primary/10">
          <CheckCircle2 className="h-10 w-10 text-primary" />
        </div>

        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-bold text-foreground">
            ¡Playlist añadida!
          </h1>
          <p className="text-muted-foreground">
            Tu playlist CieloWave ha sido guardada exitosamente en tu cuenta de
            TIDAL. Ya puedes escucharla desde la app.
          </p>
        </div>

        <Button asChild size="lg" className="mt-4">
          <Link href="/">
            <Home className="h-5 w-5" />
            Volver al inicio
          </Link>
        </Button>
      </div>
    </main>
  );
}
