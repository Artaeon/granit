<script lang="ts">
  // The evening shutdown card. Two phases per the brainstorm:
  //
  //   Phase 1 — "Arbeit schließen": four textareas + the close-day
  //     button. Active until the user marks the day closed.
  //
  //   Phase 2 — "Abendroutine": a five-step checklist that appears
  //     once the day is closed. Mirrors the brainstorm exactly:
  //     Essen / Duschen / Bibel / Gebet / Handy. The first item
  //     reuses the food pillar (single source of truth for "did you
  //     eat enough today"); the last reuses shutdown.phoneAway; the
  //     middle three live in shutdown's optional routine flags.
  //
  // Other modules in the app stay reachable during evening mode —
  // this is *soft* gating, not Sabbath-level hiding. The goal is a
  // calm wind-down surface, not a hard lockout.

  import type { ShutdownState } from './dayState';

  type Props = {
    shutdown: ShutdownState;
    /** Today's food pillar state. Drives the "Essen erledigt?"
     *  routine row — clicking that row toggles via onFoodToggle so
     *  the food state stays the single source of truth across the
     *  morning pillar list and the evening routine. */
    foodDone: boolean;
    eveningDone: boolean;
    onShutdownChange: (next: ShutdownState) => void;
    onFoodToggle: (done: boolean) => void;
    onCloseDay: () => void;
  };

  let { shutdown, foodDone, eveningDone, onShutdownChange, onFoodToggle, onCloseDay }: Props = $props();

  let local = $state<ShutdownState>({
    achieved: '',
    tomorrow: '',
    letGo: '',
    phoneAway: false
  });
  $effect(() => {
    local = { ...shutdown };
  });

  function patch<K extends keyof ShutdownState>(field: K, value: ShutdownState[K]) {
    local = { ...local, [field]: value } as ShutdownState;
    // Booleans commit eagerly (no blur event on a checkbox). Text
    // fields commit on blur so they don't fire a save per keystroke.
    if (typeof value === 'boolean') onShutdownChange({ ...local, [field]: value });
  }

  // Phase 2 row helpers: render a checkbox + label + duration hint.
  // The duration is purely informational — comes from the brainstorm
  // ("Duschen / Ordnung — 10 Min") and helps the user calibrate
  // without imposing a timer.
  type RoutineRow = {
    key: 'food' | 'showered' | 'scripture' | 'prayer' | 'phoneAway';
    label: string;
    duration: string;
  };
  const ROUTINE_ROWS: RoutineRow[] = [
    { key: 'food',       label: 'Essen erledigt?',    duration: 'Ja / Nein' },
    { key: 'showered',   label: 'Duschen / Ordnung',  duration: '10 Min' },
    { key: 'scripture',  label: 'Bibel / Lesen',      duration: '10–20 Min' },
    { key: 'prayer',     label: 'Gebet',              duration: '2 Min' },
    { key: 'phoneAway',  label: 'Handy weg',          duration: 'Ja / Nein' }
  ];

  function routineChecked(key: RoutineRow['key']): boolean {
    if (key === 'food') return foodDone;
    if (key === 'phoneAway') return !!local.phoneAway;
    if (key === 'showered')  return !!local.routineShowered;
    if (key === 'scripture') return !!local.routineScripture;
    return !!local.routinePrayer;
  }

  function toggleRoutine(key: RoutineRow['key']) {
    if (key === 'food') {
      onFoodToggle(!foodDone);
      return;
    }
    if (key === 'phoneAway')  return patch('phoneAway',        !local.phoneAway);
    if (key === 'showered')   return patch('routineShowered',  !local.routineShowered);
    if (key === 'scripture')  return patch('routineScripture', !local.routineScripture);
    if (key === 'prayer')     return patch('routinePrayer',    !local.routinePrayer);
  }
</script>

<section class="bg-mantle border border-mauve/40 rounded-lg p-5 space-y-4">
  <header class="space-y-1">
    <h2 class="text-lg font-semibold text-text">Abend-Shutdown</h2>
    <p class="text-sm text-dim">
      Arbeit ist für heute zu. Vier Zeilen, dann Handy weg. Schlaf ist jetzt Training.
    </p>
  </header>

  <div>
    <label for="ev-achieved" class="block text-xs uppercase tracking-wider text-dim mb-1">
      Was hast du heute geschafft?
    </label>
    <textarea
      id="ev-achieved"
      rows="2"
      bind:value={local.achieved}
      onblur={() => onShutdownChange(local)}
      placeholder="das hier zählt heute."
      class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
    ></textarea>
  </div>

  <div>
    <label for="ev-tomorrow" class="block text-xs uppercase tracking-wider text-dim mb-1">
      Was ist morgen wichtig?
    </label>
    <textarea
      id="ev-tomorrow"
      rows="2"
      bind:value={local.tomorrow}
      onblur={() => onShutdownChange(local)}
      placeholder="eine konkrete Aufgabe, kein Stichwort."
      class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
    ></textarea>
  </div>

  <div>
    <label for="ev-letgo" class="block text-xs uppercase tracking-wider text-dim mb-1">
      Was lässt du bewusst liegen?
    </label>
    <textarea
      id="ev-letgo"
      rows="2"
      bind:value={local.letGo}
      onblur={() => onShutdownChange(local)}
      placeholder={'nicht „auf morgen verschieben". Loslassen.'}
      class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
    ></textarea>
  </div>

  <div class="flex items-center justify-end pt-2 border-t border-surface1">
    <button
      type="button"
      onclick={onCloseDay}
      disabled={eveningDone}
      class="px-4 py-2 rounded text-sm font-medium transition-colors
        {eveningDone
          ? 'bg-success/20 text-success border border-success cursor-default'
          : 'bg-primary text-on-primary hover:opacity-90'}"
    >
      {eveningDone ? 'Tag geschlossen' : 'Tag schließen'}
    </button>
  </div>

  {#if eveningDone}
    <!-- Phase 2: the evening routine. Appears only after the day
         is marked closed — matches the brainstorm's sequencing
         where work-shutdown precedes the body wind-down. Five
         binary checkpoints, duration hints next to each so the
         user can calibrate without a timer. -->
    <section class="pt-4 mt-4 border-t border-mauve/40 space-y-2">
      <header class="space-y-1">
        <h3 class="text-sm font-semibold text-text">Abendroutine</h3>
        <p class="text-xs text-dim">Fünf Schritte — Ziel ist Schlaf, nicht Vollständigkeit.</p>
      </header>
      <ul class="space-y-1">
        {#each ROUTINE_ROWS as row (row.key)}
          {@const checked = routineChecked(row.key)}
          <li>
            <button
              type="button"
              onclick={() => toggleRoutine(row.key)}
              aria-pressed={checked}
              class="w-full flex items-center gap-3 px-2 py-1.5 rounded text-sm hover:bg-surface0 transition-colors"
            >
              <span
                class="w-5 h-5 rounded border flex items-center justify-center flex-shrink-0
                  {checked ? 'bg-success border-success' : 'border-surface2'}"
                aria-hidden="true"
              >
                {#if checked}
                  <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle">
                    <path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z" />
                  </svg>
                {/if}
              </span>
              <span class="flex-1 text-left text-text">{row.label}</span>
              <span class="text-[11px] text-dim font-mono flex-shrink-0">{row.duration}</span>
            </button>
          </li>
        {/each}
      </ul>
    </section>
  {/if}
</section>
