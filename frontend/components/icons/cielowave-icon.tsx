"use client";

import { cn } from "@/lib/utils";

interface CieloWaveIconProps {
  className?: string;
  size?: number;
}

export function CieloWaveIcon({ className, size = 32 }: CieloWaveIconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={cn("text-primary", className)}
      aria-hidden="true"
    >
      {/* Cloud */}
      <path
        d="M48 28C48 28 48 20 40 18C38 12 32 8 24 10C16 12 14 20 14 24C8 24 4 30 6 36C8 42 14 42 16 42H46C52 42 56 36 54 30C52 26 48 28 48 28Z"
        fill="currentColor"
        fillOpacity="0.9"
      />
      {/* Waves */}
      <path
        d="M8 50C12 46 18 46 22 50C26 54 32 54 36 50C40 46 46 46 50 50C54 54 58 54 60 52"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
        fill="none"
      />
      <path
        d="M4 58C8 54 14 54 18 58C22 62 28 62 32 58C36 54 42 54 46 58"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
        fill="none"
        opacity="0.6"
      />
    </svg>
  );
}
