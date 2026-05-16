import { describe, expect, it, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

// Module-under-test imports are deferred to inside each `it` block —
// the store reads localStorage at module-evaluation time, so a top-
// level import would memoise the empty initial state and we'd never
// see the per-test reset reflected in the store's initial value.
// vi.resetModules() between tests forces a fresh module evaluation
// so the persistedWritable initializer re-reads the cleared store.

function clearStorage() {
  const keys: string[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i);
    if (k) keys.push(k);
  }
  for (const k of keys) localStorage.removeItem(k);
}

beforeEach(() => {
  clearStorage();
  vi.resetModules();
});

describe('open-note store — lastOpenNote', () => {
  it('starts null when localStorage is empty', async () => {
    const mod = await import('./open-note');
    expect(get(mod.lastOpenNote)).toBeNull();
  });

  it('recordOpenNote round-trips through localStorage', async () => {
    const mod = await import('./open-note');
    mod.recordOpenNote({ path: 'work/projects/granit.md', title: 'Granit notes' });
    const v = get(mod.lastOpenNote);
    expect(v?.path).toBe('work/projects/granit.md');
    expect(v?.title).toBe('Granit notes');
    expect(typeof v?.openedAt).toBe('string');
    // Reloading the module re-reads localStorage; the entry should survive.
    vi.resetModules();
    const fresh = await import('./open-note');
    expect(get(fresh.lastOpenNote)?.path).toBe('work/projects/granit.md');
  });

  it('falls back to a path-derived title when none is supplied', async () => {
    const mod = await import('./open-note');
    mod.recordOpenNote({ path: 'Jots/2025-05-16.md', title: '' });
    expect(get(mod.lastOpenNote)?.title).toBe('2025-05-16');
  });

  it('updateOpenNoteScroll only writes when the path matches', async () => {
    const mod = await import('./open-note');
    mod.recordOpenNote({ path: 'a.md', title: 'A' });
    mod.updateOpenNoteScroll('a.md', 420);
    expect(get(mod.lastOpenNote)?.scrollPos).toBe(420);
    mod.updateOpenNoteScroll('b.md', 999);
    // Stale path: previous scroll preserved, no clobber.
    expect(get(mod.lastOpenNote)?.path).toBe('a.md');
    expect(get(mod.lastOpenNote)?.scrollPos).toBe(420);
  });

  it('clearOpenNote drops the stored entry', async () => {
    const mod = await import('./open-note');
    mod.recordOpenNote({ path: 'a.md', title: 'A' });
    mod.clearOpenNote();
    expect(get(mod.lastOpenNote)).toBeNull();
  });
});

describe('open-note store — pinnedTrayNotes', () => {
  it('pinOpenNote appends and is idempotent on the same path', async () => {
    const mod = await import('./open-note');
    mod.pinOpenNote({ path: 'a.md', title: 'A' });
    mod.pinOpenNote({ path: 'b.md', title: 'B' });
    mod.pinOpenNote({ path: 'a.md', title: 'A' });
    const v = get(mod.pinnedTrayNotes);
    // Re-pinning 'a.md' moves it to the end; no duplicates.
    expect(v.map((e) => e.path)).toEqual(['b.md', 'a.md']);
  });

  it('caps the pin list at TRAY_PIN_CAP entries', async () => {
    const mod = await import('./open-note');
    for (let i = 0; i < mod.TRAY_PIN_CAP + 2; i++) {
      mod.pinOpenNote({ path: `note-${i}.md`, title: `N${i}` });
    }
    const v = get(mod.pinnedTrayNotes);
    expect(v).toHaveLength(mod.TRAY_PIN_CAP);
    // Oldest entries are evicted, newest stays.
    expect(v[v.length - 1].path).toBe(`note-${mod.TRAY_PIN_CAP + 1}.md`);
  });

  it('unpinOpenNote is a no-op for absent paths', async () => {
    const mod = await import('./open-note');
    mod.pinOpenNote({ path: 'a.md', title: 'A' });
    mod.unpinOpenNote('not-pinned.md');
    expect(get(mod.pinnedTrayNotes).map((e) => e.path)).toEqual(['a.md']);
    mod.unpinOpenNote('a.md');
    expect(get(mod.pinnedTrayNotes)).toHaveLength(0);
  });

  it('isTrayPinned reflects the current store state', async () => {
    const mod = await import('./open-note');
    expect(mod.isTrayPinned('a.md')).toBe(false);
    mod.pinOpenNote({ path: 'a.md', title: 'A' });
    expect(mod.isTrayPinned('a.md')).toBe(true);
  });
});

describe('open-note store — trayEnabled', () => {
  it('defaults to true', async () => {
    const mod = await import('./open-note');
    expect(get(mod.trayEnabled)).toBe(true);
  });

  it('round-trips a false value', async () => {
    const mod = await import('./open-note');
    mod.trayEnabled.set(false);
    expect(get(mod.trayEnabled)).toBe(false);
    vi.resetModules();
    const fresh = await import('./open-note');
    expect(get(fresh.trayEnabled)).toBe(false);
  });
});
