import { Header } from "@/components/header";
import { Footer } from "@/components/footer";
import { PlaylistMixer } from "@/components/playlist-mixer";

export default function Home() {
  return (
    <div className="flex min-h-screen flex-col">
      <Header />
      <main className="flex-1 pb-20">
        <div className="container mx-auto max-w-2xl px-4 py-8">
          <div className="mb-8 text-center">
            <h2 className="mb-2 text-2xl font-bold text-foreground text-balance">
              Mix Your Favorite Artists
            </h2>
            <p className="text-muted-foreground text-pretty">
              Select two artists and create the perfect playlist combining their best tracks
            </p>
          </div>
          <PlaylistMixer />
        </div>
      </main>
      <Footer />
    </div>
  );
}
