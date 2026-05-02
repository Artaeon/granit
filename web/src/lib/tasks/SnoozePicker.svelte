<script lang="ts">
  import { onMount } from 'svelte';

  let {
    anchor,
    onPick,
    onClose
  }: {
    anchor?: HTMLElement;
    onPick: (untilISO: string) => void;
    onClose: () => void;
  } = $props();

  // SnoozePicker writes a "YYYY-MM-DDThh:mm" timestamp — that's the format
  // granit's parser reads back into Task.SnoozedUntil. Built-in presets
  // mirror the TUI's snooze options (1h / tonight / tomorrow / next week);
  // a custom date+time field lets the user pick anything else.

  let containerEl: HTMLDivElement | undefined = $state();

  onMount(() => {
    const onDoc = (e: MouseEvent) => {
      if (!containerEl) return;
      if (containerEl.contains(e.target as Node)) return;
      if (anchor && anchor.contains(e.target as Node)) return;
      onClose();
    };
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('click', onDoc);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('click', onDoc);
      document.removeEventListener('keydown', onKey);
    };
  });

  function fmt(d: Date): string {
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  function nowPlus(hours: number): string {
    const d = new Date();
    d.setHours(d.getHours() + hours, 0, 0, 0);
    return fmt(d);
  }

  function tonight(hour = 19): string {
    const d = new Date();
    d.setHours(hour, 0, 0, 0);
    if (d < new Date()) d.setDate(d.getDate() + 1);
    return fmt(d);
  }

  function tomorrow(hour = 9): string {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    d.setHours(hour, 0, 0, 0);
    return fmt(d);
  }

  function nextWeek(hour = 9): string {
    const d = new Date();
    d.setDate(d.getDate() + 7);
    d.setHours(hour, 0, 0, 0);
    return fmt(d);
  }

  function nextMonday(hour = 9): string {
    const d = new Date();
    const dow = d.getDay();
    const offset = ((1 - dow + 7) % 7) || 7;
    d.setDate(d.getDate() + offset);
    d.setHours(hour, 0, 0, 0);
    return fmt(d);
  }

  let customDate = $state(fmt(tomorrowDate()));
  function tomorrowDate(): Date {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    d.setHours(9, 0, 0, 0);
    return d;
  }

  const presets: { label: string; sub: string; fn: () => string }[] = [
    { label: 'In 1 hour', sub: '', fn: () => nowPlus(1) },
    { label: 'In 3 hours', sub: '', fn: () => nowPlus(3) },
    { label: 'Tonight', sub: '19:00', fn: () => tonight() },
    { label: 'Tomorrow', sub: '09:00', fn: () => tomorrow() },
    { label: 'Next Monday', sub: '09:00', fn: () => nextMonday() },
    { label: 'Next week', sub: '7 days', fn: () => nextWeek() }
  ];
</script>

<div
  bind:this={containerEl}
  class="absolute z-40 mt-1 bg-mantle border border-surface1 rounded shadow-lg w-56 p-1 right-0"
  role="menu"
>
  <div class="text-[10px] uppercase tracking-wider text-dim px-2 py-1.5">Snooze until…</div>
  <ul class="text-sm">
    {#each presets as p}
      <li>
        <button
          onclick={() => onPick(p.fn())}
          class="w-full text-left px-2 py-1.5 rounded hover:bg-surface0 flex items-baseline justify-between gap-2"
        >
          <span class="text-text">{p.label}</span>
          <span class="text-[10px] text-dim">{p.sub}</span>
        </button>
      </li>
    {/each}
  </ul>
  <div class="border-t border-surface1 mt-1 pt-1.5 px-2 pb-2 space-y-1">
    <label for="snooze-custom" class="block text-[10px] uppercase tracking-wider text-dim">Custom</label>
    <input
      id="snooze-custom"
      type="datetime-local"
      bind:value={customDate}
      class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-text text-xs"
    />
    <button
      onclick={() => onPick(customDate)}
      disabled={!customDate}
      class="w-full px-2 py-1 bg-primary text-mantle rounded text-xs disabled:opacity-50"
    >Snooze</button>
  </div>
</div>
