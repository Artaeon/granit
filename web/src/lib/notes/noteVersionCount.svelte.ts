// Snapshot-count fetcher for the notes editor header.
//
// Surfaces "this note has N saved versions" via a small chip in
// NoteHeader. Refreshes on note swap and after every successful save
// (modTime ticks). The listHistory endpoint is O(1) since the v3
// manifest sidecar, so re-fetching per save is cheap.
//
// Owns its own race-guard generation counter so a stale in-flight
// fetch can't write its result into a fresh note's count.

import { api, type Note } from '$lib/api';

export interface NoteVersionCount {
  readonly versionCount: number;
}

export interface NoteVersionCountOpts {
  /** Currently-loaded note; null between loads. The $effect tracks
   *  both path and modTime so a save bump triggers a refetch. */
  getNote: () => Note | null;
}

export function createNoteVersionCount(
  opts: NoteVersionCountOpts
): NoteVersionCount {
  let versionCount = $state(0);
  let gen = 0;

  $effect(() => {
    const note = opts.getNote();
    const path = note?.path;
    // Track modTime so every save bumps the fetch.
    void note?.modTime;
    if (!path) {
      versionCount = 0;
      return;
    }
    const myGen = ++gen;
    void (async () => {
      try {
        const data = await api.listHistory(path);
        if (myGen === gen) versionCount = data.versions?.length ?? 0;
      } catch {
        if (myGen === gen) versionCount = 0;
      }
    })();
  });

  return {
    get versionCount() { return versionCount; }
  };
}
