// Scripture-of-the-day state for the morning ritual page.
//
// First extraction step out of routes/morning/+page.svelte. Owns the
// three bindings the verse panel needs:
//
//   - scripture          — the currently selected rotation entry
//   - customScripture    — free-text override (text body)
//   - customSource       — free-text override (attribution)
//   - pickerOpen         — whether the picker UI is expanded
//
// Plus the `activeScripture` derivation that the header + the save
// pipeline both read: prefers the custom override when filled, else
// the rotation entry. Picking from the rotation also clears any
// stale custom override — same semantics the inline page had.
//
// Pure state machinery; the page still owns the rendering. The
// persistence layer round-trips by reading `scripture.source` and
// the two custom strings off this controller and restores via the
// `pick()` / `setCustom*()` methods.

import { type Scripture, scriptures, scriptureOfTheDay } from './scriptures';

export interface MorningScriptureController {
  /** Selected rotation entry. Defaults to the deterministic-per-day
   *  pick so a fresh load already has something to read. */
  scripture: Scripture;
  /** Free-text override body. Takes precedence over `scripture` when
   *  non-empty. */
  customScripture: string;
  /** Free-text override attribution. Optional even when
   *  customScripture is set. */
  customSource: string;
  /** Whether the picker UI is expanded. */
  pickerOpen: boolean;

  /** What the header + save pipeline render — custom override wins
   *  over the rotation entry. */
  readonly active: { text: string; source: string };

  /** Pick a rotation entry. Clears any stale custom override so the
   *  picker's two halves don't quietly conflict. */
  pick(s: Scripture): void;
  /** Restore a snapshot — try to match a rotation source first, then
   *  fall back to whatever custom strings were stashed. */
  restore(snap: {
    scriptureSource?: string;
    customScripture?: string;
    customSource?: string;
  }): void;
}

export function createMorningScripture(): MorningScriptureController {
  let scripture = $state<Scripture>(scriptureOfTheDay());
  let customScripture = $state('');
  let customSource = $state('');
  let pickerOpen = $state(false);

  const active = $derived.by(() => {
    if (customScripture.trim()) {
      return { text: customScripture.trim(), source: customSource.trim() };
    }
    return scripture;
  });

  function pick(s: Scripture) {
    scripture = s;
    customScripture = '';
    customSource = '';
  }

  function restore(snap: {
    scriptureSource?: string;
    customScripture?: string;
    customSource?: string;
  }) {
    if (snap.scriptureSource) {
      const m = scriptures.find((x) => x.source === snap.scriptureSource);
      if (m) scripture = m;
    }
    customScripture = snap.customScripture ?? '';
    customSource = snap.customSource ?? '';
  }

  return {
    get scripture() {
      return scripture;
    },
    set scripture(v) {
      scripture = v;
    },
    get customScripture() {
      return customScripture;
    },
    set customScripture(v) {
      customScripture = v;
    },
    get customSource() {
      return customSource;
    },
    set customSource(v) {
      customSource = v;
    },
    get pickerOpen() {
      return pickerOpen;
    },
    set pickerOpen(v) {
      pickerOpen = v;
    },
    get active() {
      return active;
    },
    pick,
    restore
  };
}
