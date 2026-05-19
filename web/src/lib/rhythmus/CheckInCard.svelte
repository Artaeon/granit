<script lang="ts">
  // Morning check-in card: three questions, no commitment beyond
  // answering them. Shown when the day's mode is null (i.e. the
  // user hasn't picked one yet today). Once a mode is chosen, the
  // card collapses out and the pillar list takes over.
  //
  // Why this isn't a wizard: the three questions are tiny and the
  // user might want to revisit / change them mid-morning. Three
  // controls in one quiet card lets the morning self regulate
  // without a "Next →" / "Back" affordance that implies a flow.
  //
  // The three questions map to the next-action engine inputs:
  //   - fatigue → body branch (10 min vs. walk)
  //   - eaten   → "eat first" priority
  //   - MIT     → focus block target
  // Anything else (whether spirit was done, etc.) belongs on the
  // pillar rows, not here.

  import type { DayMode } from './dayState';
  import ModePicker from './ModePicker.svelte';

  type Props = {
    mode: DayMode | null;
    fatigue: number;
    eaten: boolean;
    mit: string;
    onModeChange: (next: DayMode) => void;
    onFatigueChange: (next: number) => void;
    onEatenChange: (next: boolean) => void;
    onMitChange: (next: string) => void;
  };

  let {
    mode,
    fatigue,
    eaten,
    mit,
    onModeChange,
    onFatigueChange,
    onEatenChange,
    onMitChange
  }: Props = $props();

  let mitLocal = $state('');
  $effect(() => {
    mitLocal = mit;
  });
</script>

<section class="bg-mantle border border-surface1 rounded-lg p-5 space-y-4">
  <header class="space-y-1">
    <h2 class="text-lg font-semibold text-text">Morgen-Check-in</h2>
    <p class="text-sm text-dim">
      Drei Fragen — dann kennt die App deinen Tag und reduziert die Erwartungen entsprechend.
    </p>
  </header>

  <div>
    <label for="fatigue-slider" class="block text-xs uppercase tracking-wider text-dim mb-1.5">
      Wie müde bist du? <span class="text-text font-medium ml-1 tabular-nums">{fatigue}/5</span>
    </label>
    <input
      id="fatigue-slider"
      type="range"
      min="1"
      max="5"
      step="1"
      value={fatigue}
      oninput={(e) => onFatigueChange(parseInt((e.target as HTMLInputElement).value, 10))}
      class="w-full accent-primary h-9"
      aria-label="Müdigkeit 1 bis 5"
    />
    <div class="flex justify-between text-[10px] text-dim mt-0.5">
      <span>ausgeruht</span>
      <span>fertig</span>
    </div>
  </div>

  <div role="group" aria-labelledby="eaten-group-label">
    <div id="eaten-group-label" class="block text-xs uppercase tracking-wider text-dim mb-1.5">Schon gegessen?</div>
    <div class="inline-flex gap-1.5">
      <button
        type="button"
        onclick={() => onEatenChange(true)}
        aria-pressed={eaten}
        class="px-4 py-2 min-h-9 rounded border text-sm transition-colors {eaten ? 'bg-success/15 border-success text-success font-medium' : 'bg-surface0 border-surface1 text-subtext hover:border-primary'}"
      >Ja</button>
      <button
        type="button"
        onclick={() => onEatenChange(false)}
        aria-pressed={!eaten}
        class="px-4 py-2 min-h-9 rounded border text-sm transition-colors {!eaten ? 'bg-warning/15 border-warning text-warning font-medium' : 'bg-surface0 border-surface1 text-subtext hover:border-primary'}"
      >Noch nicht</button>
    </div>
  </div>

  <div>
    <label for="mit-input" class="block text-xs uppercase tracking-wider text-dim mb-1.5">
      Eine wichtigste Aufgabe heute
    </label>
    <input
      id="mit-input"
      type="text"
      bind:value={mitLocal}
      onblur={() => onMitChange(mitLocal)}
      placeholder="z. B. Kundenprojekt fertigstellen"
      class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
    />
    <p class="text-[11px] text-dim mt-1">
      Nur eine. Alles andere wird heute eine Option, kein Druck.
    </p>
  </div>

  <div class="pt-2 border-t border-surface1">
    <ModePicker value={mode} onChange={onModeChange} />
    <p class="text-[11px] text-dim mt-2">
      Modus jederzeit umstellbar. Wenn der Tag bricht, wechsel auf <em>Chaotisch</em> oder <em>Notfall</em> —
      die Karte reduziert die Erwartungen entsprechend.
    </p>
  </div>
</section>
