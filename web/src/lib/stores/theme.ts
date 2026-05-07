import { writable } from 'svelte/store';

export type Theme = 'dark' | 'light' | 'system';

// Palettes layered on top of the dark/light axis. 'default' uses
// the original Tokyo-Night-ish dark + Catppuccin-Latte-ish light
// pair built into app.css's :root. The named palettes set their
// own CSS-variable values via [data-palette='<name>'] selectors
// also in app.css.
export type Palette = 'default' | 'rose-pine' | 'gruvbox' | 'nord' | 'solarized';

export const PALETTES: { id: Palette; label: string; hint: string; swatch: string }[] = [
  { id: 'default',    label: 'Default',    hint: 'Tokyo Night / Catppuccin Latte', swatch: '#bb9af7' },
  { id: 'rose-pine',  label: 'Rosé Pine',  hint: 'Soft, low-saturation pinks',     swatch: '#c4a7e7' },
  { id: 'gruvbox',    label: 'Gruvbox',    hint: 'High-contrast retro',            swatch: '#d3869b' },
  { id: 'nord',       label: 'Nord',       hint: 'Cool arctic blues',              swatch: '#88c0d0' },
  { id: 'solarized',  label: 'Solarized',  hint: 'Schoonover classic',             swatch: '#268bd2' }
];

const STORAGE_KEY = 'granit.theme';
const PALETTE_KEY = 'granit.palette';

function readStored(): Theme {
  if (typeof localStorage === 'undefined') return 'system';
  const v = localStorage.getItem(STORAGE_KEY);
  if (v === 'dark' || v === 'light' || v === 'system') return v;
  return 'system';
}

function readStoredPalette(): Palette {
  if (typeof localStorage === 'undefined') return 'default';
  const v = localStorage.getItem(PALETTE_KEY);
  if (v && PALETTES.some((p) => p.id === v)) return v as Palette;
  return 'default';
}

function effectiveTheme(t: Theme): 'dark' | 'light' {
  if (t === 'dark' || t === 'light') return t;
  if (typeof window === 'undefined') return 'dark';
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
}

function apply(t: Theme, p: Palette) {
  if (typeof document === 'undefined') return;
  const e = effectiveTheme(t);
  document.documentElement.setAttribute('data-theme', e);
  // 'default' palette removes the data-palette attribute entirely
  // so the :root + [data-theme=...] rules in app.css drive the
  // colors. Named palettes set the attribute to enable their own
  // [data-palette='<name>'] override blocks.
  if (p === 'default') {
    document.documentElement.removeAttribute('data-palette');
  } else {
    document.documentElement.setAttribute('data-palette', p);
  }
  // Update theme-color so iOS status bar matches. We only have a
  // value for the default palette — for others the OS chrome stays
  // the default; not worth measuring per-palette here.
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) {
    meta.setAttribute('content', e === 'light' ? '#eff1f5' : '#1a1b26');
  }
}

function createThemeStore() {
  const initial = readStored();
  const initialPalette = readStoredPalette();
  const { subscribe, set: rawSet } = writable<Theme>(initial);

  function set(t: Theme) {
    if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, t);
    rawSet(t);
    apply(t, readStoredPalette());
  }

  // Apply on first load + watch system changes when in 'system' mode
  if (typeof window !== 'undefined') {
    apply(initial, initialPalette);
    const mq = window.matchMedia('(prefers-color-scheme: light)');
    mq.addEventListener('change', () => {
      // Only re-apply if user is in system mode
      const cur = readStored();
      if (cur === 'system') apply('system', readStoredPalette());
    });
  }

  return { subscribe, set };
}

function createPaletteStore() {
  const initial = readStoredPalette();
  const { subscribe, set: rawSet } = writable<Palette>(initial);

  function set(p: Palette) {
    if (typeof localStorage !== 'undefined') localStorage.setItem(PALETTE_KEY, p);
    rawSet(p);
    apply(readStored(), p);
  }
  return { subscribe, set };
}

export const theme = createThemeStore();
export const palette = createPaletteStore();

export function nextTheme(t: Theme): Theme {
  return t === 'dark' ? 'light' : t === 'light' ? 'system' : 'dark';
}

export function themeIcon(t: Theme): string {
  return t === 'dark' ? '☾' : t === 'light' ? '☀' : '◐';
}

export function themeLabel(t: Theme): string {
  return t === 'dark' ? 'Dark' : t === 'light' ? 'Light' : 'System';
}
