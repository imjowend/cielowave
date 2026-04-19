"use client";

import * as React from "react";
import { useState, useCallback, useEffect, useRef } from "react";
import { Check, ChevronsUpDown, User } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Spinner } from "@/components/ui/spinner";
import { useDebounce } from "@/hooks/useDebounce";
import type { Artist } from "@/types";

// Maximum number of artist results to display
const MAX_RESULTS = 5;
// Minimum characters required to trigger a search
const MIN_SEARCH_LENGTH = 3;

interface ArtistComboboxProps {
  label: string;
  value: Artist | null;
  onSelect: (artist: Artist | null) => void;
}

const API_URL = "";

export function ArtistCombobox({ label, value, onSelect }: ArtistComboboxProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [artists, setArtists] = useState<Artist[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [showSlowMessage, setShowSlowMessage] = useState(false);
  const slowMessageRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Debounce the search term with 300ms delay
  const debouncedSearchTerm = useDebounce(search, 300);

  // Perform API search when debounced search term changes
  useEffect(() => {
    const searchArtists = async () => {
      // Only search if debounced term meets minimum length requirement
      if (!debouncedSearchTerm.trim() || debouncedSearchTerm.trim().length < MIN_SEARCH_LENGTH) {
        setArtists([]);
        setIsLoading(false);
        setShowSlowMessage(false);
        if (slowMessageRef.current) {
          clearTimeout(slowMessageRef.current);
        }
        return;
      }

      setIsLoading(true);
      setShowSlowMessage(false);

      // Show "slow search" message after 2 seconds
      slowMessageRef.current = setTimeout(() => {
        setShowSlowMessage(true);
      }, 2000);

      try {
        const response = await fetch(
          `${API_URL}/api/artists?q=${encodeURIComponent(debouncedSearchTerm)}`
        );
        if (response.ok) {
          const data: Artist[] = await response.json();
          // Limit results to MAX_RESULTS (prepared for when backend returns more)
          setArtists((data || []).slice(0, MAX_RESULTS));
        }
      } catch (error) {
        console.error("Failed to search artists:", error);
        setArtists([]);
      } finally {
        setIsLoading(false);
        setShowSlowMessage(false);
        if (slowMessageRef.current) {
          clearTimeout(slowMessageRef.current);
        }
      }
    };

    searchArtists();

    // Cleanup slow message timeout on unmount or when search term changes
    return () => {
      if (slowMessageRef.current) {
        clearTimeout(slowMessageRef.current);
      }
    };
  }, [debouncedSearchTerm]);

  const handleSelect = useCallback(
    (artist: Artist) => {
      onSelect(artist);
      setOpen(false);
      setSearch("");
    },
    [onSelect]
  );

  return (
    <div className="flex flex-col gap-2">
      <label className="text-sm font-medium text-muted-foreground">
        {label}
      </label>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            role="combobox"
            aria-expanded={open}
            aria-haspopup="listbox"
            aria-label={`${label}: ${value?.name || "Search artist"}`}
            className="h-14 w-full justify-between bg-input hover:bg-secondary"
          >
            {value ? (
              <div className="flex items-center gap-3">
                <div className="flex h-9 w-9 items-center justify-center rounded-full bg-secondary">
                  {value.imageUrl ? (
                    <img
                      src={value.imageUrl}
                      alt={value.name}
                      className="h-9 w-9 rounded-full object-cover"
                    />
                  ) : (
                    <User className="h-5 w-5 text-muted-foreground" />
                  )}
                </div>
                <span className="truncate font-medium">{value.name}</span>
              </div>
            ) : (
              <span className="text-muted-foreground">Buscar artista...</span>
            )}
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[--radix-popover-trigger-width] p-0">
          <Command shouldFilter={false}>
            <CommandInput
              placeholder="Escribe el nombre..."
              value={search}
              onValueChange={setSearch}
              aria-label={`Buscar ${label}`}
              aria-describedby={`${label}-hint`}
            />
            <span id={`${label}-hint`} className="sr-only">
              Escribe al menos {MIN_SEARCH_LENGTH} caracteres para buscar
            </span>
            <CommandList aria-busy={isLoading} aria-live="polite">
              {isLoading ? (
                <div className="flex flex-col items-center gap-3 py-6 text-center">
                  <Spinner size="md" />
                  <div className="text-sm text-muted-foreground">
                    Buscando en el catálogo de TIDAL...
                  </div>
                  {showSlowMessage && (
                    <div className="text-xs text-muted-foreground/70">
                      Encontrando artistas, un momento...
                    </div>
                  )}
                </div>
              ) : debouncedSearchTerm.trim().length >= MIN_SEARCH_LENGTH && artists.length === 0 ? (
                <CommandEmpty>No encontramos ese artista. Intenta con otro nombre.</CommandEmpty>
              ) : (
                <CommandGroup>
                  {artists.map((artist) => (
                    <CommandItem
                      key={artist.id}
                      value={artist.id}
                      onSelect={() => handleSelect(artist)}
                    >
                      <div className="flex items-center gap-3">
                        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-secondary">
                          {artist.imageUrl ? (
                            <img
                              src={artist.imageUrl}
                              alt={artist.name}
                              className="h-8 w-8 rounded-full object-cover"
                            />
                          ) : (
                            <User className="h-4 w-4 text-muted-foreground" />
                          )}
                        </div>
                        <span>{artist.name}</span>
                      </div>
                      <Check
                        className={cn(
                          "ml-auto h-4 w-4",
                          value?.id === artist.id ? "opacity-100" : "opacity-0"
                        )}
                      />
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}
