import { writable } from 'svelte/store';
import { loadStoredString, saveStoredString } from '$lib/util/storage';

export type Theme = 'dark' | 'light' | 'system';

const STORAGE_KEY = 'granit.theme';

function readStored(): Theme {
  const v = loadStoredString(STORAGE_KEY, 'system');
  return v === 'dark' || v === 'light' || v === 'system' ? v : 'system';
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
  // Update theme-color so iOS status bar matches the page background.
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) {
    meta.setAttribute('content', e === 'light' ? '#ffffff' : '#000000');
  }
}

function createThemeStore() {
  const initial = readStored();
  const { subscribe, set: rawSet } = writable<Theme>(initial);

  function set(t: Theme) {
    saveStoredString(STORAGE_KEY, t);
    rawSet(t);
    apply(t);
  }

  if (typeof window !== 'undefined') {
    apply(initial);
    const mq = window.matchMedia('(prefers-color-scheme: light)');
    mq.addEventListener('change', () => {
      if (readStored() === 'system') apply('system');
    });
  }

  return { subscribe, set };
}

export const theme = createThemeStore();

export function nextTheme(t: Theme): Theme {
  return t === 'dark' ? 'light' : t === 'light' ? 'system' : 'dark';
}

export function themeLabel(t: Theme): string {
  return t === 'dark' ? 'Dark' : t === 'light' ? 'Light' : 'System';
}
