// Preset + library list controller for InlineAIMenu.
//
// Owns three reactive pieces and one fetcher:
//
//   visiblePresets   — the static PRESETS catalog filtered by current
//                      cursor state (selection-only chips hide in
//                      cursor mode and vice versa) and by the user's
//                      query string, stable-sorted by category so the
//                      rendered list aligns with the grouped headers.
//   highlightedIdx   — keyboard-navigation cursor. Resets to 0 whenever
//                      visiblePresets changes (mode flip, filter typed)
//                      so Up/Down always start from the top of the new
//                      list rather than landing on a now-hidden row.
//   libraryAll /     — user-saved prompts fetched once from the backend.
//   libraryFiltered    The filtered slice mirrors the preset query +
//                      cursor-state rules so the library section follows
//                      the same affordances.
//
// loadLibrary is exposed as a method (rather than auto-running in the
// factory) so the menu can decide when to fetch — typically once on
// mount. A silent empty list looks like "user has no saved prompts"
// rather than "fetch failed", which is why the catch logs a warn
// breadcrumb but doesn't bubble the error to the UI.
//
// runLibraryEntry isn't part of this controller — it routes through
// the menu's custom-prompt path (prefilling the input + firing the
// same code path as a typed Ask), which is component-level concern.

import { api, type AIPromptEntry } from '$lib/api';
import {
  CATEGORY_ORDER,
  PRESETS,
  type Preset
} from './inline-ai-presets';

export interface PresetFilterOpts {
  /** Reactive — true when the trigger event carried a non-empty
   *  selection. Switches the chips between cursor-mode and
   *  selection-mode rules for both presets and library entries. */
  getHasSelection: () => boolean;
  /** Reactive — current prompt-input query string. Used as a fuzzy
   *  substring filter against label + hint (presets) and label +
   *  prompt (library). */
  getQuery: () => string;
}

export interface PresetFilterController {
  /** Filtered + category-sorted preset list. Drives the main action
   *  list rendering and keyboard nav. */
  readonly visiblePresets: Preset[];
  /** Filtered library entries (user-saved prompts). Rendered as a
   *  distinct group after the curated categories. */
  readonly libraryFiltered: AIPromptEntry[];
  /** Keyboard-nav cursor into visiblePresets. */
  highlightedIdx: number;
  /** Fetch the user's saved prompt library. Safe to call multiple
   *  times — the latest result wins. Failures swallow into an empty
   *  list and log a console.warn breadcrumb. */
  loadLibrary(): Promise<void>;
}

export function createPresetFilterController(
  opts: PresetFilterOpts
): PresetFilterController {
  let libraryAll = $state<AIPromptEntry[]>([]);
  let highlightedIdx = $state(0);

  // Filter presets by mode + query. Selection mode hides cursor-only
  // chips and vice versa. Stable sort by CATEGORY_ORDER so the flat
  // list lines up with the grouped headers in the menu render.
  let visiblePresets = $derived.by(() => {
    const hasSel = opts.getHasSelection();
    const filtered = PRESETS.filter((p) => (hasSel ? p.selection : p.cursor));
    const q = opts.getQuery().trim().toLowerCase();
    const matched = q
      ? filtered.filter(
          (p) =>
            p.label.toLowerCase().includes(q) ||
            p.hint.toLowerCase().includes(q)
        )
      : filtered;
    return matched
      .slice()
      .sort(
        (a, b) =>
          CATEGORY_ORDER.indexOf(a.category) -
          CATEGORY_ORDER.indexOf(b.category)
      );
  });

  let libraryFiltered = $derived.by(() => {
    const hasSel = opts.getHasSelection();
    const q = opts.getQuery().trim().toLowerCase();
    return libraryAll.filter((e) => {
      // Scope filter — strict per cursor state, but 'either' shows in both.
      if (hasSel && e.scope === 'cursor') return false;
      if (!hasSel && e.scope === 'selection') return false;
      // Text filter — same fuzzy substring as the preset list.
      if (
        q &&
        !e.label.toLowerCase().includes(q) &&
        !e.prompt.toLowerCase().includes(q)
      )
        return false;
      return true;
    });
  });

  // Reset the keyboard-nav cursor whenever the visible list shape
  // changes. Without this, a query typed mid-flight can leave
  // highlightedIdx pointing past the end of the new list and the
  // first Up/Down keypress lands on nothing.
  $effect(() => {
    void visiblePresets;
    highlightedIdx = 0;
  });

  async function loadLibrary() {
    try {
      const lib = await api.getAIPrompts();
      libraryAll = lib.entries ?? [];
    } catch (e) {
      libraryAll = [];
      // Library is a discoverability surface — a silent empty list
      // looks like "user has no saved prompts" rather than "fetch
      // failed". A console.warn at least leaves breadcrumbs for the
      // user when they ask why their library is gone.
      // eslint-disable-next-line no-console
      console.warn('inline-ai library:', e instanceof Error ? e.message : String(e));
    }
  }

  return {
    get visiblePresets() {
      return visiblePresets;
    },
    get libraryFiltered() {
      return libraryFiltered;
    },
    get highlightedIdx() {
      return highlightedIdx;
    },
    set highlightedIdx(v: number) {
      highlightedIdx = v;
    },
    loadLibrary
  };
}
