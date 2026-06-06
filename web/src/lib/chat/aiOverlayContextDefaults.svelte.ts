// Path → mode auto-suggestion. When the overlay opens for the first
// time on a route, pick the mode that fits the page so the user
// doesn't have to flip the mode picker themselves (e.g. /chat opens
// with 'analyze', /notes opens with 'write').
//
// Strict guard: only suggest when the user is parked on the default
// 'general' mode. A deliberate pick from the user — even one they
// made on a previous route — never gets clobbered. The applied
// flag resets every time the overlay closes so re-opening on a
// different route gets a fresh suggestion run.

import { tick } from 'svelte';
import { AGENT_MODES } from '$lib/ai/agents';
import { suggestedModeForPath } from './contextDefaults';

export interface AIContextDefaultsOptions {
  /** True when the overlay is open. The installer re-runs the suggest
   *  logic on every open→reopen, with an internal "applied this
   *  open" gate so it doesn't re-fire mid-session. */
  isOpen: () => boolean;
  /** Current route pathname, e.g. $page.url.pathname. */
  getPathname: () => string;
  /** True when the user is on the default 'general' mode and a
   *  suggestion is safe to apply. */
  isOnGeneralMode: () => boolean;
  /** Apply the suggested mode (the AIContextManager's selectMode). */
  selectMode: (id: string) => void;
}

export function installAIContextDefaults(opts: AIContextDefaultsOptions): void {
  let applied = $state(false);

  function apply() {
    if (applied) return;
    if (typeof window === 'undefined') return;
    const suggested = suggestedModeForPath(opts.getPathname());
    if (suggested && opts.isOnGeneralMode()) {
      const target = AGENT_MODES.find((m) => m.id === suggested);
      if (target) opts.selectMode(target.id);
    }
    applied = true;
  }

  // Apply on every fresh open so the user navigating /tasks → open
  // chat → /goals → open chat gets the right mode each time. The
  // flag resets when the overlay closes.
  $effect(() => {
    if (opts.isOpen()) {
      // tick() lets the open transition settle before we possibly
      // flip the mode (avoids a single-frame flash of the wrong
      // header label).
      tick().then(() => apply());
    } else {
      applied = false;
    }
  });
}
