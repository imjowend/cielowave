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
import type { Artist } from "@/types";

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
  const [loading, setLoading] = useState(false);
  const [showSlowMessage, setShowSlowMessage] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const slowMessageRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const searchArtists = useCallback(async (query: string) => {
    if (!query.trim() || query.trim().length < 2) {
      setArtists([]);
      setLoading(false);
      setShowSlowMessage(false);
      if (slowMessageRef.current) {
        clearTimeout(slowMessageRef.current);
      }
      return;
    }

    setLoading(true);
    setShowSlowMessage(false);

    // Show "slow search" message after 2 seconds
    slowMessageRef.current = setTimeout(() => {
      setShowSlowMessage(true);
    }, 2000);

    try {
      const response = await fetch(
        `${API_URL}/api/artists?q=${encodeURIComponent(query)}`
      );
      if (response.ok) {
        const data = await response.json();
        setArtists(data || []);
      }
    } catch (error) {
      console.error("Failed to search artists:", error);
      setArtists([]);
    } finally {
      setLoading(false);
      setShowSlowMessage(false);
      if (slowMessageRef.current) {
        clearTimeout(slowMessageRef.current);
      }
    }
  }, []);

  useEffect(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }

    debounceRef.current = setTimeout(() => {
      searchArtists(search);
    }, 500);

    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
    };
  }, [search, searchArtists]);

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
              <span className="text-muted-foreground">Search artist...</span>
            )}
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[--radix-popover-trigger-width] p-0">
          <Command shouldFilter={false}>
            <CommandInput
              placeholder="Search artist..."
              value={search}
              onValueChange={setSearch}
            />
            <CommandList>
              {loading ? (
                <div className="flex flex-col gap-2 py-6 text-center">
                  <div className="text-sm text-muted-foreground">
                    Searching in Tidal catalog...
                  </div>
                  {showSlowMessage && (
                    <div className="text-xs text-muted-foreground/70">
                      Resolving artists, this may take a moment...
                    </div>
                  )}
                </div>
              ) : search.trim() && artists.length === 0 ? (
                <CommandEmpty>No artists found.</CommandEmpty>
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
