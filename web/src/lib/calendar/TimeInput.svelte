<script lang="ts">
  // Shared 24-hour HH:MM time-range picker used in EventDetail +
  // UnifiedCreate. Two paired selects per slot (hours 0-23 + minutes
  // 0-55 at `step` granularity) instead of <input type="time">
  // because every browser's native time input renders AM/PM on most
  // OS locales, ignoring the element's lang attribute. Paired selects
  // guarantee 24-hour display everywhere.
  //
  // The component owns NO date logic — start/end are bindable HH:MM
  // strings, period. Hour + minute splits are internal and reflect
  // back through the bindable strings on every change.

  let {
    startTime = $bindable('00:00'),
    endTime = $bindable('00:00'),
    allDay = false,
    step = 15
  }: {
    startTime?: string;
    endTime?: string;
    allDay?: boolean;
    step?: number;
  } = $props();

  // Split HH:MM safely — tolerates undefined / partial inputs by
  // clamping to 0 rather than throwing. Defence in depth; callers
  // should always pass valid strings.
  function splitHM(s: string): [number, number] {
    const parts = (s ?? '').split(':');
    let h = Number(parts[0]);
    let m = Number(parts[1]);
    if (!Number.isFinite(h) || h < 0) h = 0;
    if (h > 23) h = 23;
    if (!Number.isFinite(m) || m < 0) m = 0;
    if (m > 59) m = 59;
    return [h, m];
  }

  // Round a minute to the nearest `step` so the value lines up with
  // the available options. The select would otherwise show a blank
  // current value when the bound minute (e.g. 27) doesn't match any
  // option in a step=15 picker.
  function roundToStep(m: number): number {
    const r = Math.round(m / step) * step;
    return r >= 60 ? 0 : r;
  }

  // Derived hour/minute from the bindable HH:MM strings. Reading
  // through $derived (not $state + $effect) avoids a flush race:
  // changing a select and immediately submitting the parent form
  // reads the up-to-date string synchronously.
  let startH = $derived(splitHM(startTime)[0]);
  let startM = $derived(roundToStep(splitHM(startTime)[1]));
  let endH = $derived(splitHM(endTime)[0]);
  let endM = $derived(roundToStep(splitHM(endTime)[1]));

  function pad(n: number): string {
    return String(n).padStart(2, '0');
  }

  function setStartH(h: number) {
    startTime = `${pad(h)}:${pad(startM)}`;
  }
  function setStartM(m: number) {
    startTime = `${pad(startH)}:${pad(m)}`;
  }
  function setEndH(h: number) {
    endTime = `${pad(h)}:${pad(endM)}`;
  }
  function setEndM(m: number) {
    endTime = `${pad(endH)}:${pad(m)}`;
  }

  // Build the minute options once per step value. Default step=15
  // gives 00,15,30,45 — covers the calendar's snap granularity.
  // step=5 gives every 5min, matching UnifiedCreate's pre-extraction
  // shape; callers pass step={5} to keep that behaviour.
  let minuteOptions = $derived.by(() => {
    const out: number[] = [];
    for (let m = 0; m < 60; m += step) out.push(m);
    return out;
  });
  const hourOptions = Array.from({ length: 24 }, (_, i) => i);
</script>

{#if !allDay}
  <div class="grid grid-cols-2 gap-2">
    <div>
      <span class="block text-[10px] uppercase tracking-wider text-dim mb-1">Start (24h)</span>
      <div class="flex items-center bg-surface0 border border-surface1 rounded overflow-hidden focus-within:border-primary">
        <select
          value={startH}
          onchange={(e) => setStartH(Number((e.target as HTMLSelectElement).value))}
          aria-label="start hour"
          class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
        >
          {#each hourOptions as h}
            <option value={h}>{pad(h)}</option>
          {/each}
        </select>
        <span class="text-dim px-1">:</span>
        <select
          value={startM}
          onchange={(e) => setStartM(Number((e.target as HTMLSelectElement).value))}
          aria-label="start minute"
          class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
        >
          {#each minuteOptions as m}
            <option value={m}>{pad(m)}</option>
          {/each}
        </select>
      </div>
    </div>
    <div>
      <span class="block text-[10px] uppercase tracking-wider text-dim mb-1">End (24h)</span>
      <div class="flex items-center bg-surface0 border border-surface1 rounded overflow-hidden focus-within:border-primary">
        <select
          value={endH}
          onchange={(e) => setEndH(Number((e.target as HTMLSelectElement).value))}
          aria-label="end hour"
          class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
        >
          {#each hourOptions as h}
            <option value={h}>{pad(h)}</option>
          {/each}
        </select>
        <span class="text-dim px-1">:</span>
        <select
          value={endM}
          onchange={(e) => setEndM(Number((e.target as HTMLSelectElement).value))}
          aria-label="end minute"
          class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
        >
          {#each minuteOptions as m}
            <option value={m}>{pad(m)}</option>
          {/each}
        </select>
      </div>
    </div>
  </div>
{/if}

<style>
  /* Native <select> dropdown panel uses OS chrome and defaults to
     white-on-white in dark themes. Same fix as the pre-extraction
     callsites: hint color-scheme + explicit option colors so the
     dropdown is readable on every browser/OS. */
  .time-select {
    color-scheme: dark;
    background: var(--color-surface0);
  }
  .time-select option {
    background: var(--color-base);
    color: var(--color-text);
  }
  :global([data-theme="light"]) .time-select {
    color-scheme: light;
  }
</style>