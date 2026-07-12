export function BackgroundGlow() {
  return (
    <div
      aria-hidden="true"
      className="pointer-events-none fixed inset-0 -z-10 overflow-hidden"
    >
      {/* Orbe 1 — azul cielo: brillo principal (claro) / urbano (oscuro) */}
      <div className="absolute -left-32 -top-40 h-[32rem] w-[32rem] rounded-full bg-[#82C8E5]/15 blur-3xl dark:bg-[#82C8E5]/10" />

      {/* Orbe 2 — profundidad: glacial (claro) / azul real (oscuro) */}
      <div className="absolute -right-40 top-1/3 h-[28rem] w-[28rem] rounded-full bg-[#A5D8F3]/10 blur-3xl dark:bg-[#1D4ED8]/[0.07]" />

      {/* Orbe 3 — lujo dorado, estrellas (claro) / punto de luz blanco (oscuro) */}
      <div className="absolute right-12 top-12 h-48 w-48 rounded-full bg-[#D4AF37]/5 blur-3xl dark:bg-white/[0.04]" />
    </div>
  );
}
