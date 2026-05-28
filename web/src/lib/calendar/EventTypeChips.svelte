<script lang="ts">
  // Shared event-kind picker — a row of chip buttons sourced from
  // EVENT_TYPES. Used by CreateEvent + EventDetail. Tapping the
  // active chip clears it; an explicit clear button appears beside
  // the row once a chip is on, matching the previous inline shape
  // both callsites used. Each chip carries the type's glyph (tinted
  // with the catalog colour when inactive) so the picker doubles as
  // a legend for the calendar grid's chip prefixes.

  import { EVENT_TYPES, findEventType } from './eventTypes';

  let {
    kind = $bindable(''),
    /** Slightly wider chip padding for full-form layouts (CreateEvent)
     *  vs the tight inline edit form (EventDetail). Default matches
     *  EventDetail's px-2 py-1. */
    chipSize = 'compact',
    onChange
  }: {
    kind?: string;
    chipSize?: 'compact' | 'comfy';
    onChange?: (next: string) => void;
  } = $props();

  // Picking a chip with `defaultDurationMin` doesn't auto-shift any
  // time field here — that's the caller's concern (CreateEvent does it
  // by reading kind in its pickKind handler). The chips emit `onChange`
  // for callers that want to react beyond the simple bind:kind path.
  function setKind(next: string) {
    const value = kind === next ? '' : next;
    kind = value;
    onChange?.(value);
  }
  function clear() {
    kind = '';
    onChange?.('');
  }

  // Tag class — picker chip padding matches the original two callsites:
  // CreateEvent used px-2 py-1.5 (comfy); EventDetail used px-2 py-1
  // (compact). Keeping both shapes here means neither callsite shifts
  // visually under the extraction.
  let chipClass = $derived(
    chipSize === 'comfy' ? 'px-2 py-1.5' : 'px-2 py-1'
  );
  let clearChipClass = $derived(
    chipSize === 'comfy' ? 'px-1.5 py-1' : 'px-1.5 py-0.5'
  );
</script>

<div class="flex items-center gap-1 flex-wrap">
  {#each EVENT_TYPES as t (t.id)}
    {@const on = kind === t.id}
    <button
      type="button"
      onclick={() => setKind(t.id)}
      aria-pressed={on}
      title={t.description}
      class="inline-flex items-center gap-1.5 {chipClass} text-xs font-medium border transition-colors {on ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 text-text border-surface1 hover:border-primary'}"
    >
      <span
        class="inline-flex items-center justify-center w-4 h-4 text-[10px] font-bold font-mono leading-none"
        style:background={on ? 'transparent' : `color-mix(in srgb, var(--color-${t.color}) 22%, transparent)`}
        style:color={on ? undefined : `var(--color-${t.color})`}
      >{t.glyph}</span>
      <span>{t.label}</span>
    </button>
  {/each}
  {#if kind}
    <button
      type="button"
      onclick={clear}
      class="text-[10px] text-dim hover:text-error {clearChipClass} border border-dashed border-surface1 hover:border-error"
    >clear</button>
  {/if}
</div>
