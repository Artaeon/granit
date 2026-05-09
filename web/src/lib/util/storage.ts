// Component-local localStorage helpers. The `persistedWritable`
// helper covers the global-store case, but component-internal
// state (view-mode toggles, per-page filter prefs, custom
// debounce timers, draft buffers) wants the simpler shape:
//
//   let view = $state<View>(loadStored(VIEW_KEY, 'list'));
//   $effect(() => saveStored(VIEW_KEY, view));
//
// instead of:
//
//   let view = $state<View>(
//     (typeof localStorage !== 'undefined' &&
//       (localStorage.getItem(VIEW_KEY) as View)) || 'list'
//   );
//   $effect(() => {
//     if (typeof localStorage === 'undefined') return;
//     try { localStorage.setItem(VIEW_KEY, view); } catch {}
//   });
//
// Both helpers are SSR-safe (return / no-op when localStorage is
// undefined during prerender) and quota-safe (silent catch on the
// write path). loadStored takes an optional validator so older
// stored shapes can be filtered out without poisoning the state.

/**
 * Read + parse JSON from localStorage. Returns `defaultValue` for:
 * - SSR (no localStorage)
 * - missing key
 * - JSON parse failure
 * - validator throw
 *
 * The validator runs on the parsed value; throw to fall back to
 * default. Pass `(v) => v as T` to accept any shape (the default).
 */
export function loadStored<T>(
  key: string,
  defaultValue: T,
  validate?: (raw: unknown) => T
): T {
  if (typeof localStorage === 'undefined') return defaultValue;
  try {
    const raw = localStorage.getItem(key);
    if (raw === null) return defaultValue;
    const parsed = JSON.parse(raw) as unknown;
    return validate ? validate(parsed) : (parsed as T);
  } catch {
    return defaultValue;
  }
}

/**
 * Write JSON to localStorage. SSR-safe (no-op) and quota-safe
 * (silent catch — the in-memory state is still correct, only
 * persistence degrades). Accepts undefined to remove the key,
 * matching the "clear this preference" intent.
 */
export function saveStored<T>(key: string, value: T | undefined): void {
  if (typeof localStorage === 'undefined') return;
  try {
    if (value === undefined) localStorage.removeItem(key);
    else localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // quota / privacy mode — silently drop.
  }
}

/**
 * Read a string directly from localStorage with no JSON parse —
 * useful for prefs that are themselves strings (a current view
 * name, a stored URL, the active theme id) where the JSON quote
 * marks would be visual clutter in the storage inspector.
 */
export function loadStoredString(key: string, defaultValue: string): string {
  if (typeof localStorage === 'undefined') return defaultValue;
  try {
    return localStorage.getItem(key) ?? defaultValue;
  } catch {
    return defaultValue;
  }
}

/**
 * Write a raw string to localStorage. Quota-safe. Pass undefined
 * to remove the key.
 */
export function saveStoredString(key: string, value: string | undefined): void {
  if (typeof localStorage === 'undefined') return;
  try {
    if (value === undefined) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // quota / privacy mode — silently drop.
  }
}
