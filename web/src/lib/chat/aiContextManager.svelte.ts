// AI-context manager — owns the per-thread "what does the model see"
// surface for AIOverlay.
//
// Three concerns, one cluster:
//
//   1. Mode (posture + persona)
//      ‒ modeId / mode (derived) / lastPersonaId / lastPersona
//      ‒ autoMode + lastAutoSwitchedFor (page-context auto-switch)
//      ‒ the $effect that drives the auto-switch based on the
//        current page (project > goal > calendar > none)
//      ‒ selectMode (user picks) + restoreMode (history reloads
//        a thread that had a different mode)
//
//   2. RAG (toggle)
//      ‒ rag boolean + setRag / toggleRag
//      ‒ Seeded from the loaded mode's default; each selectMode call
//        re-seeds from the new mode's default; the user is free to
//        override mid-thread via the slash router / RAG chip.
//
//   3. Long-term AI memory
//      ‒ aiMemoryFacts + aiMemoryLoaded + loadAIMemory
//      ‒ Refreshed on WS broadcasts of ai-memory.json changes (the
//        WS subscription lives in the parent so it can chain with
//        other granitmeta listeners).
//
// Why these three together: they're the three things AIOverlay's
// prelude assembler reads to decide what gets prepended to every
// turn. Pulling them into one module keeps the "model context"
// surface searchable and avoids three separate single-concern
// managers when the responsibilities co-evolve.
//
// What stays in AIOverlay (intentionally NOT moved):
//   ‒ lastRagHits / perTurnRagHits / expandedSources — post-send
//     UI state about which sources WERE used. Written by the
//     session manager during streaming; this manager doesn't need
//     them.
//   ‒ snapshotData / snapshotLoading / attachSnapshot / attachNote
//     — surface-specific attach toggles that the user flips per
//     session. Different lifecycle from mode / rag / memory.
//
// Auto-switch invariant the tests pin (kept from the inlined
// version): a manual mode pick clears autoMode, so re-entering the
// same page-context doesn't yank the user back. Without
// lastAutoSwitchedFor we'd reauto-switch every time the effect
// re-runs against a still-matching page-context key.

import {
  PERSONAS,
  findMode,
  loadModeId,
  persistModeId,
  type AgentMode
} from '$lib/ai/agents';
import { api, type AIMemoryFact } from '$lib/api';

/** Read-only access to parent-derived page context. The manager calls
 *  these inside the auto-switch $effect; the parent's $derived chain
 *  drives the page state. */
export interface AIContextSources {
  currentProjectName: () => string | null;
  currentGoalId: () => string | null;
  onCalendarPage: () => boolean;
}

export interface AIContextManagerOptions {
  sources: AIContextSources;
  /** aria-live readout for screen readers — mode flips need an
   *  announcement that's polite-channel (not a toast). */
  announce: (msg: string) => void;
}

export interface AIContextManager {
  // ── Mode ─────────────────────────────────────────────────────
  readonly modeId: string;
  readonly mode: AgentMode;
  readonly autoMode: '' | 'project' | 'goal' | 'calendar';
  readonly autoPMActive: boolean;
  readonly lastPersonaId: string;
  readonly lastPersona: AgentMode | null;
  /** User-initiated mode pick. Persists, resets RAG to the mode's
   *  default, clears autoMode (so re-entering the page-context
   *  won't yank), updates lastPersonaId if the new mode is a
   *  persona, and announces over aria-live. */
  selectMode(id: string): void;
  /** Quiet variant for history-thread restores. No announce, no RAG
   *  reset — just set + persist. RAG is a session-level toggle, not
   *  per-thread, so a thread reload preserves the current RAG choice. */
  restoreMode(id: string): void;

  // ── RAG ──────────────────────────────────────────────────────
  readonly rag: boolean;
  setRag(v: boolean): void;
  /** Toggle + return new value for the slash router toast. */
  toggleRag(): boolean;

  // ── Memory ───────────────────────────────────────────────────
  readonly aiMemoryFacts: AIMemoryFact[];
  readonly aiMemoryLoaded: boolean;
  loadAIMemory(): Promise<void>;
}

export function createAIContextManager(
  opts: AIContextManagerOptions
): AIContextManager {
  // ── Mode ──────────────────────────────────────────────────────
  let modeId = $state<string>(loadModeId());
  const mode = $derived(findMode(modeId));
  let autoMode = $state<'' | 'project' | 'goal' | 'calendar'>('');
  // Page-context key we already auto-switched FOR. Without this
  // the auto-switch effect would loop: a manual selectMode clears
  // autoMode, the effect re-runs, sees `modeId !== target` and
  // yanks the user right back. With the guard we auto-switch at
  // most once per page-context entry — after the user accepts or
  // overrides, no more nagging until they leave the scope.
  let lastAutoSwitchedFor = $state<string>('');
  const autoPMActive = $derived(autoMode !== '');

  // Persistent "last persona" so the inline persona chip above the
  // composer keeps showing a meaningful label even when the active
  // mode is a generic posture. Seeded from the loaded mode if it's
  // already a persona; otherwise the first persona in PERSONAS.
  let lastPersonaId = $state<string>(
    findMode(loadModeId()).kind === 'persona'
      ? loadModeId()
      : PERSONAS[0]?.id ?? ''
  );
  const lastPersona = $derived(lastPersonaId ? findMode(lastPersonaId) : null);

  // ── RAG ──────────────────────────────────────────────────────
  // Initial seed uses the module helper (not the modeId state) so
  // Svelte's analyzer doesn't flag reading state in an initializer.
  // Later changes flow through selectMode.
  let rag = $state(findMode(loadModeId()).ragDefault);

  // ── Memory ────────────────────────────────────────────────────
  let aiMemoryFacts = $state<AIMemoryFact[]>([]);
  let aiMemoryLoaded = $state(false);

  // ── Auto-switch effect ─────────────────────────────────────────
  // Precedence: most-specific entity wins (Project > Goal >
  // Calendar > none). First entry into a scope flips to the matching
  // agent mode + remembers the key. Leaving the scope reverts to
  // the user's persisted mode if and only if we're still in an
  // auto-set mode — a manual pick while inside a scope captures
  // ownership for the rest of that scope.
  $effect(() => {
    const project = opts.sources.currentProjectName();
    const goal = opts.sources.currentGoalId();
    const calendar = opts.sources.onCalendarPage();
    const inProject = !!project;
    const inGoal = !inProject && !!goal;
    const inCalendar = !inProject && !inGoal && calendar;
    const key = inProject
      ? `project:${project}`
      : inGoal
      ? `goal:${goal}`
      : inCalendar
      ? 'calendar'
      : '';

    if (key) {
      if (lastAutoSwitchedFor !== key) {
        const targetMode = inProject
          ? 'project-manager'
          : inGoal
          ? 'goal-manager'
          : 'calendar-manager';
        if (modeId !== targetMode) {
          autoMode = inProject ? 'project' : inGoal ? 'goal' : 'calendar';
          modeId = targetMode;
        }
        lastAutoSwitchedFor = key;
      }
    } else {
      if (
        autoMode &&
        (modeId === 'project-manager' ||
          modeId === 'goal-manager' ||
          modeId === 'calendar-manager')
      ) {
        autoMode = '';
        modeId = loadModeId();
        // Only reset the auto-switch guard when WE owned the mode
        // (autoMode was set and we reverted it). If the user had
        // manually picked a mode inside the scope (selectMode clears
        // autoMode), `lastAutoSwitchedFor` still pins the scope key
        // — preserving it across the leave keeps the re-entry guard
        // honest, so coming back doesn't yank them out of their
        // chosen mode.
        lastAutoSwitchedFor = '';
      }
    }
  });

  function selectMode(id: string): void {
    if (id === modeId) return;
    // User is taking control — clear the contextual auto-switch so
    // re-entering this scope doesn't yank them back, and so leaving
    // it doesn't trigger the "revert" branch (which only fires when
    // autoMode is set).
    autoMode = '';
    modeId = id;
    persistModeId(id);
    const m = findMode(id);
    rag = m.ragDefault;
    if (m.kind === 'persona') lastPersonaId = id;
    opts.announce(`Mode: ${m.label}. ${m.tagline}`);
  }

  function restoreMode(id: string): void {
    if (id === modeId) return;
    modeId = id;
    persistModeId(id);
  }

  function setRag(v: boolean): void {
    rag = v;
  }

  function toggleRag(): boolean {
    rag = !rag;
    return rag;
  }

  async function loadAIMemory(): Promise<void> {
    try {
      const r = await api.listAIMemory();
      aiMemoryFacts = r.facts;
      aiMemoryLoaded = true;
    } catch {
      // Silent — an empty / failed memory load shouldn't block the
      // chat. The user simply doesn't get memory-augmented replies
      // until the next successful fetch.
    }
  }

  return {
    get modeId() { return modeId; },
    get mode() { return mode; },
    get autoMode() { return autoMode; },
    get autoPMActive() { return autoPMActive; },
    get lastPersonaId() { return lastPersonaId; },
    get lastPersona() { return lastPersona; },
    selectMode,
    restoreMode,
    get rag() { return rag; },
    setRag,
    toggleRag,
    get aiMemoryFacts() { return aiMemoryFacts; },
    get aiMemoryLoaded() { return aiMemoryLoaded; },
    loadAIMemory
  };
}
