"use client";

import * as React from "react";
import { useTheme } from "next-themes";
import { Monitor, Moon, Sun } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

const OPTIONS = [
  { value: "light", label: "Modo claro", icon: Sun },
  { value: "dark", label: "Modo oscuro", icon: Moon },
  { value: "system", label: "Usar el tema del sistema", icon: Monitor },
] as const;

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = React.useState(false);

  React.useEffect(() => {
    setMounted(true);
  }, []);

  if (!mounted) {
    return <div className="h-10 w-[108px]" aria-hidden="true" />;
  }

  return (
    <div className="flex items-center gap-1 rounded-md border border-border p-1">
      {OPTIONS.map(({ value, label, icon: Icon }) => (
        <Button
          key={value}
          type="button"
          variant="ghost"
          size="icon"
          aria-label={label}
          aria-pressed={theme === value}
          className={cn(
            "h-8 w-8",
            theme === value
              ? "bg-accent text-accent-foreground"
              : "text-muted-foreground hover:text-foreground"
          )}
          onClick={() => setTheme(value)}
        >
          <Icon className="h-4 w-4" />
        </Button>
      ))}
    </div>
  );
}
