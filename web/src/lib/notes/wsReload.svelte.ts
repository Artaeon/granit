// Coalesces WS `note.changed` bursts into a single trailing-edge
// reload, while preserving every guard the original inline version
// shipped: live editor content (not the lagging `body` mirror) wins
// the "user is typing" check, in-flight saves are skipped, and the
// ~3s own-save quiet window suppresses the file-watcher bounce-back
// of our own PUT. Reloading on every event would flash the editor
// twice per autosave (PUT handler + file watcher).
//
// The page owns the heavy state (note/body/prev/saving/lastSavedAt)
// and the load() function; this controller owns only the timer and
// the WS subscription lifecycle.

import { onWsEvent } from '$lib/ws';

export interface WsReloadDeps {
  /** Currently active note path, or null when no note is loaded. */
  getActivePath: () => string | null;
  /** The "user is currently typing" signal — read CodeMirror's view
   *  state.doc directly so a microtask-lagging bind:value mirror
   *  can't false-positive a clean state. */
  getLiveBody: () => string;
  /** Last saved body — the page's `prev` mirror. */
  getSavedBody: () => string;
  /** True while a save() is in flight. */
  isSaving: () => boolean;
  /** Epoch ms of the last successful save, or null. */
  getLastSavedAt: () => number | null;
  /** Reload the active note from the server. */
  reload: (path: string) => void;
  /** Milliseconds after a successful save during which we ignore
   *  WS-driven reloads — defaults to 3000. */
  ownSaveQuietMs?: number;
  /** Coalesce window for `note.changed` bursts — defaults to 600. */
  coalesceMs?: number;
}

export function installWsReload(deps: WsReloadDeps): () => void {
  const quietMs = deps.ownSaveQuietMs ?? 3000;
  const coalesceMs = deps.coalesceMs ?? 600;
  let timer: ReturnType<typeof setTimeout> | null = null;

  const guardsBlock = (path: string): boolean => {
    const active = deps.getActivePath();
    if (!active || active !== path) return true;
    if (deps.getLiveBody() !== deps.getSavedBody()) return true;
    if (deps.isSaving()) return true;
    const last = deps.getLastSavedAt();
    if (last !== null && Date.now() - last < quietMs) return true;
    return false;
  };

  const schedule = (path: string) => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      // Re-evaluate every guard at fire time — the user could have
      // started typing during the coalesce window, or a save could
      // have started.
      if (guardsBlock(path)) return;
      deps.reload(path);
    }, coalesceMs);
  };

  const off = onWsEvent((ev) => {
    if (ev.type !== 'note.changed') return;
    const active = deps.getActivePath();
    if (!active || ev.path !== active) return;
    // Cheap synchronous-only guards; the timed re-check above covers
    // the rest. The editor-content read suppresses reloads while
    // in-flight typing hasn't propagated to `body`.
    if (deps.getLiveBody() !== deps.getSavedBody()) return;
    if (deps.isSaving()) return;
    const last = deps.getLastSavedAt();
    if (last !== null && Date.now() - last < quietMs) return;
    schedule(active);
  });

  return () => {
    off();
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
  };
}
