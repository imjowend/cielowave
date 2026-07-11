# Track colors fix + light/dark/system theme toggle

## Context

CieloWave's frontend (Next.js App Router, Tailwind CSS, HSL CSS variables in
`app/globals.css`) currently:

- Colors mixed tracks by artist using `--artist-a` (blue, hue 221) and
  `--artist-b` (violet, hue 280). The violet doesn't match the site's
  "Cielo"/sky-blue branding (see `--primary: 221 83% 53%` and the
  `CieloWaveIcon`, which is rendered `text-primary`).
- Has no dark mode / theme switching at all. `globals.css` already defines a
  `.dark` class with variables, but nothing toggles it.
- Renders the header (`components/header.tsx`) with the logo+title centered
  (`justify-center`), no room reserved for controls.

## Goals

1. Replace the violet `--artist-b` color with a celeste/cyan tone, so both
   artist colors in the mixed track list sit within the site's blue/celeste
   palette while remaining visually distinguishable from each other.
2. Add a light/dark/system theme toggle, placed in the header, as a
   3-icon segmented control (sun / moon / monitor).
3. Move the header layout so the site title+logo sit on the left and the
   theme toggle sits on the right.

## Non-goals

- No changes to `--primary`, `--secondary`, `--accent`, `--destructive`, or
  any other color token.
- No changes to the artist search/combobox (tracked separately as a bug fix).
- No persistence mechanism beyond what `next-themes` provides out of the box
  (localStorage, handled by the library).

## Design

### 1. Track colors (`app/globals.css`)

Replace `--artist-b` (currently hue 280, violet) with a cyan/celeste hue
(~195), keeping `--artist-a` in the blue family (~221) so the two remain
distinguishable but both read as "celeste":

```css
:root {
  --artist-a: 221 83% 93%;   /* blue */
  --artist-b: 195 83% 90%;   /* celeste/cyan */
}

.dark {
  --artist-a: 221 50% 22%;
  --artist-b: 195 55% 20%;
}
```

Exact lightness/saturation values are tuned during implementation for
sufficient contrast against `--foreground` in both themes; the hues (221 for
A, 195 for B) are the fixed decision here.

No other file references these tokens besides `tailwind.config.ts` (already
wired to `artist-a` / `artist-b` utility classes) and
`components/track-list.tsx` (already consumes `bg-artist-a` / `bg-artist-b`).
Neither needs to change.

### 2. Theme system

- Add dependency: `next-themes`.
- New file `components/theme-provider.tsx`: thin wrapper around
  `next-themes`'s `ThemeProvider`, re-exported as a client component.
- `app/layout.tsx`: wrap `{children}` in `<ThemeProvider attribute="class"
  defaultTheme="system" enableSystem>`. `attribute="class"` matches the
  existing `.dark` class selector already used throughout `globals.css` — no
  changes needed to how dark styles are defined.
- Add `suppressHydrationWarning` to the `<html>` tag in `app/layout.tsx`,
  standard practice with `next-themes` to avoid an SSR/client class mismatch
  warning on first paint.

### 3. Theme toggle component

New file `components/theme-toggle.tsx`:

- Client component using `useTheme()` from `next-themes`.
- Renders a segmented group of 3 icon buttons (Sun, Moon, Monitor from
  `lucide-react`, already a dependency of the project — confirmed in
  `package.json`).
- Each button uses the existing `Button` component
  (`variant="ghost" size="icon"`), with the active option (matching
  `theme` from `useTheme()`) getting `bg-accent text-accent-foreground`
  instead of the default ghost styling.
- Guards against hydration mismatch with the standard `mounted` state
  pattern (render a size-matched placeholder until mounted, then swap to
  the real control).

### 4. Header layout (`components/header.tsx`)

- Change the inner container from
  `flex items-center justify-center` to `flex items-center justify-between`.
- Logo + `<h1>CieloWave</h1>` block stays as the first child (now pinned
  left by `justify-between`).
- Add `<ThemeToggle />` as the second child (pinned right).

## Testing

- Manual verification in the browser (light, dark, and system-follows-OS
  modes) per the `run`/browser-verification workflow: toggle each of the 3
  modes, confirm the header renders correctly at both mobile and desktop
  widths, and confirm track list colors read as celeste/blue (not violet) in
  both themes with adequate text contrast.
- No existing automated test suite covers styling; none added, since this is
  a purely visual change with no logic branching beyond the theme toggle's
  own state (which is exercised manually since it's UI-triggered).
