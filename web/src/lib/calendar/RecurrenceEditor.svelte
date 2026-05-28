<script lang="ts">
  // Shared RRULE editor — used by CreateEvent (block layout, full
  // feature set including BYDAY picker) and EventDetail (inline
  // compact layout, basic presets only). The recurrence model is
  // identical in both cases; only the surrounding form layout
  // differs, so `layout` selects which markup tree renders.
  //
  // The component owns parse + serialise: it reads/writes a single
  // RFC 5545 RRULE string through `rrule` ($bindable), and seeds its
  // internal preset state from that string on prop change. Callers
  // never need to touch the FREQ/INTERVAL/BYDAY/UNTIL tokens.

  type Repeat =
    | 'none'
    | 'daily'
    | 'weekdays'
    | 'weekly'
    | 'biweekly'
    | 'monthly'
    | 'yearly'
    | 'bydays'
    | 'custom';

  type WD = 'MO' | 'TU' | 'WE' | 'TH' | 'FR' | 'SA' | 'SU';

  let {
    rrule = $bindable(''),
    layout = 'block',
    /** Anchor date (YYYY-MM-DD) — used as `min` for the Until picker
     *  so the user can't pick a date earlier than the event itself. */
    minDate = '',
    /** Optional id prefix to keep label `for=` attributes unique when
     *  multiple editors render on the same page. */
    idPrefix = 'rrule'
  }: {
    rrule?: string;
    layout?: 'block' | 'inline';
    minDate?: string;
    idPrefix?: string;
  } = $props();

  // ── Picker state ────────────────────────────────────────────────
  let repeat = $state<Repeat>('none');
  let untilDate = $state(''); // YYYY-MM-DD; empty = forever
  let customRule = $state('');
  let bydaysSet = $state<Set<WD>>(new Set<WD>(['MO', 'TU', 'WE', 'TH', 'FR']));

  // Track the rrule we last serialised so the parse-back $effect can
  // distinguish "external change" (re-seed) from "self-write" (skip).
  // Without this, every internal flip would round-trip through the
  // parser and risk drift on edge cases.
  let lastSerialised = $state('');

  // Parse the incoming RRULE into our preset state. Runs whenever
  // the bound `rrule` changes from outside this component. The
  // suppress-on-self-write guard keeps us from re-parsing our own
  // emit (which would be lossy for 'custom' / unsupported shapes).
  $effect(() => {
    const incoming = rrule ?? '';
    if (incoming === lastSerialised) return;
    seedFromRRule(incoming);
  });

  function seedFromRRule(s: string) {
    if (!s) {
      repeat = 'none';
      untilDate = '';
      customRule = '';
      return;
    }
    const parts: Record<string, string> = {};
    for (const seg of s.split(';')) {
      const [k, v] = seg.split('=', 2);
      if (k && v !== undefined) parts[k.trim().toUpperCase()] = v.trim();
    }
    let until = '';
    if (parts.UNTIL) {
      // RFC 5545 UNTIL is YYYYMMDDTHHMMSSZ — pull the date prefix.
      const m = /^(\d{4})(\d{2})(\d{2})/.exec(parts.UNTIL);
      if (m) until = `${m[1]}-${m[2]}-${m[3]}`;
    }
    const freq = parts.FREQ ?? '';
    const interval = parts.INTERVAL ?? '';
    const byday = parts.BYDAY ?? '';
    untilDate = until;
    customRule = '';
    if (freq === 'DAILY' && !interval && !byday) {
      repeat = 'daily';
    } else if (freq === 'WEEKLY' && byday === 'MO,TU,WE,TH,FR' && !interval) {
      repeat = 'weekdays';
    } else if (freq === 'WEEKLY' && !byday && (interval === '' || interval === '1')) {
      repeat = 'weekly';
    } else if (freq === 'WEEKLY' && !byday && interval === '2') {
      repeat = 'biweekly';
    } else if (freq === 'WEEKLY' && byday && interval === '' && byday !== 'MO,TU,WE,TH,FR') {
      // A specific weekday subset that ISN'T the canonical
      // Mon-Fri — surface via the BYDAY picker.
      repeat = 'bydays';
      bydaysSet = new Set<WD>(byday.split(',').filter(isWD));
    } else if (freq === 'MONTHLY' && !interval && !byday) {
      repeat = 'monthly';
    } else if (freq === 'YEARLY' && !interval) {
      repeat = 'yearly';
    } else {
      repeat = 'custom';
      customRule = s;
    }
  }
  function isWD(s: string): s is WD {
    return s === 'MO' || s === 'TU' || s === 'WE' || s === 'TH' || s === 'FR' || s === 'SA' || s === 'SU';
  }

  function untilSuffix(): string {
    if (!untilDate || !/^\d{4}-\d{2}-\d{2}$/.test(untilDate)) return '';
    return `;UNTIL=${untilDate.replace(/-/g, '')}T235959Z`;
  }
  function bydaysSuffix(): string {
    if (bydaysSet.size === 0) return '';
    const order: WD[] = ['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU'];
    return order.filter((d) => bydaysSet.has(d)).join(',');
  }

  // Serialise the picker state back into RRULE form. Pure derivation
  // so every internal flip emits a fresh string the parent observes.
  let serialised = $derived.by((): string => {
    switch (repeat) {
      case 'none': return '';
      case 'daily': return 'FREQ=DAILY' + untilSuffix();
      case 'weekdays': return 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR' + untilSuffix();
      case 'weekly': return 'FREQ=WEEKLY' + untilSuffix();
      case 'biweekly': return 'FREQ=WEEKLY;INTERVAL=2' + untilSuffix();
      case 'monthly': return 'FREQ=MONTHLY' + untilSuffix();
      case 'yearly': return 'FREQ=YEARLY' + untilSuffix();
      case 'bydays': {
        const days = bydaysSuffix();
        if (!days) return 'FREQ=WEEKLY' + untilSuffix();
        return 'FREQ=WEEKLY;BYDAY=' + days + untilSuffix();
      }
      case 'custom': return customRule.trim();
    }
  });

  // Push our serialised value back to the bindable rrule when the
  // picker state changes. Tag lastSerialised so the inbound effect
  // doesn't immediately re-parse our own emit.
  $effect(() => {
    const out = serialised;
    if (out !== rrule) {
      lastSerialised = out;
      rrule = out;
    }
  });

  function toggleBYDay(code: WD) {
    const next = new Set(bydaysSet);
    if (next.has(code)) next.delete(code);
    else next.add(code);
    bydaysSet = next;
  }

  const WEEKDAYS: { code: WD; label: string; isWeekend: boolean }[] = [
    { code: 'MO', label: 'Mon', isWeekend: false },
    { code: 'TU', label: 'Tue', isWeekend: false },
    { code: 'WE', label: 'Wed', isWeekend: false },
    { code: 'TH', label: 'Thu', isWeekend: false },
    { code: 'FR', label: 'Fri', isWeekend: false },
    { code: 'SA', label: 'Sat', isWeekend: true },
    { code: 'SU', label: 'Sun', isWeekend: true }
  ];
</script>

{#if layout === 'inline'}
  <!-- EventDetail's compact inline layout: label + select on one
       baseline-aligned row, optional until input inline, no BYDAY
       picker (basic presets only). -->
  <div class="flex items-baseline gap-2 flex-wrap">
    <label class="text-[11px] text-dim uppercase tracking-wider" for="{idPrefix}-repeat">Repeat</label>
    <select
      id="{idPrefix}-repeat"
      bind:value={repeat}
      class="bg-surface0 border border-surface1 rounded px-2 py-1 text-sm text-text"
    >
      <option value="none">Does not repeat</option>
      <option value="daily">Every day</option>
      <option value="weekdays">Every weekday (Mon–Fri)</option>
      <option value="weekly">Every week</option>
      <option value="biweekly">Every 2 weeks</option>
      <option value="monthly">Every month</option>
      <option value="yearly">Every year</option>
      <option value="custom">Custom RRULE…</option>
    </select>
    {#if repeat !== 'none' && repeat !== 'custom'}
      <label class="text-[11px] text-dim flex items-center gap-1.5">
        until
        <input
          type="date"
          bind:value={untilDate}
          min={minDate}
          class="bg-surface0 border border-surface1 rounded px-2 py-1 text-sm text-text"
        />
      </label>
    {/if}
  </div>
  {#if repeat === 'custom'}
    <input
      bind:value={customRule}
      placeholder="FREQ=MONTHLY;BYDAY=1MO"
      spellcheck="false"
      class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text font-mono"
    />
  {/if}
  {#if serialised}
    <p class="text-[10px] text-dim font-mono"><span class="text-secondary">→</span> {serialised}</p>
  {/if}
{:else}
  <!-- CreateEvent's full-form block layout: stacked labels, larger
       inputs, BYDAY weekday grid + presets row. -->
  <div>
    <label class="block text-[11px] uppercase tracking-wider text-dim mb-1.5" for="{idPrefix}-repeat">Repeat</label>
    <select
      id="{idPrefix}-repeat"
      bind:value={repeat}
      class="w-full px-3 py-3 sm:py-2.5 bg-surface0 border border-surface1 rounded-lg text-base sm:text-sm text-text focus:outline-none focus:border-primary"
    >
      <option value="none">Does not repeat</option>
      <option value="daily">Every day</option>
      <option value="weekdays">Every weekday (Mon–Fri)</option>
      <option value="weekly">Every week</option>
      <option value="biweekly">Every 2 weeks</option>
      <option value="bydays">Specific weekdays…</option>
      <option value="monthly">Every month</option>
      <option value="yearly">Every year</option>
      <option value="custom">Custom RRULE…</option>
    </select>
  </div>
  {#if repeat === 'bydays'}
    <div>
      <span class="block text-[11px] uppercase tracking-wider text-dim mb-1.5">Days of week</span>
      <div class="flex items-center gap-1 flex-wrap">
        {#each WEEKDAYS as wd (wd.code)}
          {@const on = bydaysSet.has(wd.code)}
          <button
            type="button"
            onclick={() => toggleBYDay(wd.code)}
            aria-pressed={on}
            title={wd.label + (wd.isWeekend ? ' (weekend)' : '')}
            class="min-w-[2.75rem] px-2 py-1.5 text-xs font-medium border transition-colors {on ? 'bg-primary text-on-primary border-primary' : wd.isWeekend ? 'bg-surface0 text-dim border-surface1 hover:border-secondary' : 'bg-surface0 text-text border-surface1 hover:border-primary'}"
          >{wd.label}</button>
        {/each}
      </div>
      <div class="flex items-center gap-1.5 mt-1.5 text-[10px]">
        <button
          type="button"
          onclick={() => (bydaysSet = new Set<WD>(['MO', 'TU', 'WE', 'TH', 'FR']))}
          class="px-1.5 py-0.5 text-dim hover:text-primary border border-dashed border-surface1 hover:border-primary"
        >Mon–Fri</button>
        <button
          type="button"
          onclick={() => (bydaysSet = new Set<WD>(['SA', 'SU']))}
          class="px-1.5 py-0.5 text-dim hover:text-primary border border-dashed border-surface1 hover:border-primary"
        >Sat–Sun</button>
        <button
          type="button"
          onclick={() => (bydaysSet = new Set<WD>(['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU']))}
          class="px-1.5 py-0.5 text-dim hover:text-primary border border-dashed border-surface1 hover:border-primary"
        >All</button>
        <button
          type="button"
          onclick={() => (bydaysSet = new Set<WD>())}
          class="px-1.5 py-0.5 text-dim hover:text-error border border-dashed border-surface1 hover:border-error"
        >Clear</button>
        <span class="text-dim ml-2">{bydaysSet.size} day{bydaysSet.size === 1 ? '' : 's'} selected</span>
      </div>
    </div>
  {/if}
  {#if repeat !== 'none'}
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
      {#if repeat !== 'custom'}
        <div>
          <label class="block text-[11px] uppercase tracking-wider text-dim mb-1.5" for="{idPrefix}-until">Until (optional)</label>
          <input
            id="{idPrefix}-until"
            type="date"
            bind:value={untilDate}
            min={minDate}
            placeholder="forever"
            class="w-full px-3 py-3 sm:py-2.5 bg-surface0 border border-surface1 rounded-lg text-base sm:text-sm text-text focus:outline-none focus:border-primary"
          />
        </div>
      {:else}
        <div class="sm:col-span-2">
          <label class="block text-[11px] uppercase tracking-wider text-dim mb-1.5" for="{idPrefix}-rrule">RRULE</label>
          <input
            id="{idPrefix}-rrule"
            bind:value={customRule}
            placeholder='e.g. FREQ=MONTHLY;BYDAY=1MO;UNTIL=20271231T235959Z'
            spellcheck="false"
            class="w-full px-3 py-3 sm:py-2.5 bg-surface0 border border-surface1 rounded-lg text-sm text-text font-mono focus:outline-none focus:border-primary"
          />
          <p class="text-[10px] text-dim mt-1 leading-snug">
            RFC 5545 — FREQ + INTERVAL + BYDAY + UNTIL. Power-user shape; presets above cover most cases.
          </p>
        </div>
      {/if}
      {#if serialised}
        <div class="sm:col-span-2 px-2.5 py-1.5 bg-mantle border border-surface1 rounded text-[11px] text-dim font-mono">
          <span class="text-secondary">→</span> {serialised}
        </div>
      {/if}
    </div>
  {/if}
{/if}
