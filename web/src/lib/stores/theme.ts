import { writable } from 'svelte/store';

export type Theme = 'dark' | 'light' | 'system';

const STORAGE_KEY = 'granit.theme';

function readStored(): Theme {
  if (typeof localStorage === 'undefined') return 'system';
  const v = localStorage.getItem(STORAGE_KEY);
  if (v === 'dark' || v === 'light' || v === 'system') return v;
  return 'system';
}

function effectiveTheme(t: Theme): 'dark' | 'light' {
  if (t === 'dark' || t === 'light') return t;
  if (typeof window === 'undefined') return 'dark';
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
}

function apply(t: Theme) {
  if (typeof document === 'undefined') return;
  const e = effectiveTheme(t);
  document.documentElement.setAttribute('data-theme', e);
  // Update theme-color so iOS status bar matches
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) {
    meta.setAttribute('content', e === 'light' ? '#eff1f5' : '#1a1b26');
  }
}

function createThemeStore() {
  const initial = readStored();
  const { subscribe, set: rawSet } = writable<Theme>(initial);

  function set(t: Theme) {
    if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, t);
    rawSet(t);
    apply(t);
  }

  // Apply on first load + watch system changes when in 'system' mode
  if (typeof window !== 'undefined') {
    apply(initial);
    const mq = window.matchMedia('(prefers-color-scheme: light)');
    mq.addEventListener('change', () => {
      // Only re-apply if user is in system mode
      const cur = readStored();
      if (cur === 'system') apply('system');
    });
  }

  return { subscribe, set };
}

export const theme = createThemeStore();

export function nextTheme(t: Theme): Theme {
  return t === 'dark' ? 'light' : t === 'light' ? 'system' : 'dark';
}

export function themeIcon(t: Theme): string {
  return t === 'dark' ? '☾' : t === 'light' ? '☀' : '◐';
}

export function themeLabel(t: Theme): string {
  return t === 'dark' ? 'Dark' : t === 'light' ? 'Light' : 'System';
}
