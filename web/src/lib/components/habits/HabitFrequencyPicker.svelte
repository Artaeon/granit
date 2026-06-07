<script lang="ts">
  // Frequency picker — a compact inline popover with preset chips
  // (Daily / Weekdays / Weekends / 3×/wk / 5×/wk) and a "Custom days"
  // row of 7 toggleable weekday letters. Closes on save / cancel /
  // outside click. Designed to share row real-estate with the other
  // inline chips on a habit card; matches the existing target /
  // stack-edit popover density.

  import { formatFrequency, WEEKDAY_KEYS, WEEKDAY_DISPLAY } from '$lib/habits/habitsFrequencyFormat';
  import {
    canonicaliseFrequency,
    type FrequencyEditController
  } from '$lib/habits/habitsFrequencyEdit.svelte';

  let {
    name,
    frequency,
    ctl
  }: {
    name: string;
    frequency: string | undefined;
    ctl: FrequencyEditController;
  } = $props();

  let open = $derived(ctl.editing === name);

  // Preset chips. Value is the canonical cadence string the backend
  // expects; label is what the chip shows.
  const PRESETS = [
    { v: 'daily', label: 'Daily' },
    { v: 'weekdays', label: 'Weekdays' },
    { v: 'weekends', label: 'Weekends' },
    { v: '3x-week', label: '3×/wk' },
    { v: '5x-week', label: '5×/wk' }
  ] as const;

  // Currently selected weekdays parsed from the draft. Returns a Set
  // of 3-letter tokens for O(1) toggle lookup in the row.
  let pickedDays = $derived.by<Set<string>>(() => {
    const raw = ctl.draft.trim().toLowerCase();
    if (!raw || !raw.includes(',')) return new Set();
    const tokens = raw.split(',').map((t) => t.trim()).filter(Boolean);
    return new Set(tokens.filter((t) => WEEKDAY_KEYS.includes(t)));
  });

  function selectPreset(v: string): void {
    ctl.draft = v;
  }

  function toggleDay(key: string): void {
    const next = new Set(pickedDays);
    if (next.has(key)) next.delete(key); else next.add(key);
    if (next.size === 0) {
      ctl.draft = '';
      return;
    }
    // Canonicalise (sorted, deduped) at the controller boundary so
    // the picker UI always reads back consistently.
    ctl.draft = canonicaliseFrequency([...next].join(','));
  }

  function clear(): void {
    ctl.draft = '';
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      ctl.cancel();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      void ctl.submit(name);
    }
  }

  let humanLabel = $derived(formatFrequency(frequency));
  let draftPreview = $derived(formatFrequency(ctl.draft) || '(none)');
</script>

{#if open}
  <!-- Popover row. Lives inline in the card's chip cluster so it
       inherits the surrounding padding; keyboard handlers live on
       the wrapper so any focused child (chip, weekday button) can
       Esc-cancel / Enter-submit. -->
  <div
    role="group"
    aria-label="edit frequency for {name}"
    onkeydown={onKeydown}
    class="w-full mt-1.5 p-2 bg-surface0 border border-primary rounded text-[11px] flex flex-col gap-1.5"
  >
    <div class="flex flex-wrap items-center gap-1">
      <span class="text-dim uppercase tracking-wider text-[10px] mr-1">cadence</span>
      {#each PRESETS as p (p.v)}
        {@const active = ctl.draft.trim().toLowerCase() === p.v}
        <button
          type="button"
          onclick={() => selectPreset(p.v)}
          class="px-1.5 py-0.5 rounded border text-[11px] transition-colors
            {active
              ? 'bg-primary text-on-primary border-primary'
              : 'bg-surface1 text-subtext border-surface2 hover:text-text hover:bg-surface2'}"
        >{p.label}</button>
      {/each}
    </div>

    <div class="flex flex-wrap items-center gap-1">
      <span class="text-dim uppercase tracking-wider text-[10px] mr-1">days</span>
      {#each WEEKDAY_KEYS as key, i (key)}
        {@const active = pickedDays.has(key)}
        <button
          type="button"
          onclick={() => toggleDay(key)}
          class="w-6 h-6 rounded text-[10px] font-medium border transition-colors
            {active
              ? 'bg-primary text-on-primary border-primary'
              : 'bg-surface1 text-subtext border-surface2 hover:text-text hover:bg-surface2'}"
          title={WEEKDAY_DISPLAY[i]}
          aria-pressed={active}
          aria-label={WEEKDAY_DISPLAY[i]}
        >{WEEKDAY_DISPLAY[i].charAt(0)}</button>
      {/each}
    </div>

    <div class="flex items-center gap-1.5">
      <span class="text-dim">→</span>
      <span class="text-text font-medium">{draftPreview}</span>
      {#if ctl.error}
        <span class="text-[10px] text-error ml-1">{ctl.error}</span>
      {/if}
      <span class="flex-1"></span>
      <button
        type="button"
        onclick={clear}
        class="px-1.5 py-0.5 text-dim hover:text-text underline text-[11px]"
        title="clear cadence"
      >clear</button>
      <button
        type="button"
        onclick={() => void ctl.submit(name)}
        disabled={ctl.busy}
        class="px-2 py-0.5 bg-primary text-on-primary rounded text-[11px] disabled:opacity-50"
      >save</button>
      <button
        type="button"
        onclick={() => ctl.cancel()}
        class="px-1.5 py-0.5 text-dim hover:text-text text-[11px]"
      >cancel</button>
    </div>
  </div>
{:else if humanLabel}
  <button
    type="button"
    onclick={() => ctl.start(name, frequency)}
    class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] border border-surface2 bg-surface0 text-subtext hover:text-text hover:bg-surface1"
    title="cadence — click to edit"
  >
    <span aria-hidden="true">↻</span>
    <span>{humanLabel}</span>
  </button>
{:else}
  <button
    type="button"
    onclick={() => ctl.start(name, frequency)}
    class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border bg-surface1 text-dim border-surface2 hover:text-text"
    title="set a cadence"
  >+ cadence</button>
{/if}
