<script lang="ts">
  import { api, type CalendarEvent } from '$lib/api';
  import { toast } from '$lib/components/toast';

  let {
    open = $bindable(false),
    date,
    existingEvents = [],
    onCreated
  }: {
    open?: boolean;
    date: Date;
    /** Events shown on the surrounding calendar — used for conflict
     *  detection. Defaults to empty so callers that don't care about
     *  conflicts (e.g. unit tests) don't have to provide it. */
    existingEvents?: CalendarEvent[];
    onCreated: () => void | Promise<void>;
  } = $props();

  let title = $state('');
  let dateISO = $state('');
  let startTime = $state('');
  let endTime = $state('');
  let location = $state('');
  // Empty default lets eventTypeColor rotate by title hash so a fresh
  // calendar fills with distinct hues automatically.
  let color = $state('');
  let saving = $state(false);

  // ── Conflict detection ──────────────────────────────────────────
  // Compute the events on the chosen date that overlap the picked
  // [startTime, endTime] window. Pure derivation from existingEvents
  // — cheap at any realistic vault size. We only flag overlaps when
  // BOTH the new event and the candidate have explicit start+end
  // times; all-day events on the same date are by-definition not a
  // scheduling clash (they're parallel context).
  function pickHM(rfc: string | undefined): string {
    if (!rfc) return '';
    // RFC3339 like "2026-05-09T14:00:00+02:00" — slice the time part
    // in the device's local presentation. The /calendar page stores
    // events in the user's local zone already.
    const m = /T(\d{2}:\d{2})/.exec(rfc);
    return m ? m[1] : '';
  }
  function eventDateKey(e: CalendarEvent): string {
    if (e.date) return e.date;
    if (e.start) return e.start.slice(0, 10);
    return '';
  }
  function rangesOverlap(aStart: string, aEnd: string, bStart: string, bEnd: string): boolean {
    // String compare on HH:MM is order-preserving since both are
    // zero-padded 24-hour. aStart < bEnd && bStart < aEnd is the
    // half-open-interval overlap test.
    return aStart < bEnd && bStart < aEnd;
  }
  const conflicts = $derived.by(() => {
    if (!startTime || !endTime || !dateISO) return [];
    if (startTime >= endTime) return []; // bad range; let the form's own validation handle it
    const out: { title: string; start: string; end: string }[] = [];
    for (const e of existingEvents) {
      if (eventDateKey(e) !== dateISO) continue;
      const eStart = pickHM(e.start);
      const eEnd = pickHM(e.end);
      if (!eStart || !eEnd) continue;
      if (rangesOverlap(startTime, endTime, eStart, eEnd)) {
        out.push({ title: e.title, start: eStart, end: eEnd });
      }
      if (out.length >= 3) break; // cap so a busy day doesn't tower
    }
    return out;
  });

  $effect(() => {
    if (open && date) {
      dateISO = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
    }
  });

  function close() {
    open = false;
    title = '';
    startTime = '';
    endTime = '';
    location = '';
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!title.trim()) return;
    saving = true;
    try {
      await api.createEvent({
        title: title.trim(),
        date: dateISO,
        start_time: startTime,
        end_time: endTime,
        location: location.trim(),
        color
      });
      close();
      await onCreated();
      toast.success('event created');
    } catch (err) {
      toast.error('create failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      saving = false;
    }
  }

  const colorOptions = ['red', 'yellow', 'orange', 'green', 'blue', 'purple', 'cyan'];
  function tone(c: string): string {
    const map: Record<string, string> = { red: 'error', yellow: 'warning', orange: 'accent', green: 'success', blue: 'secondary', purple: 'primary', cyan: 'info' };
    return `var(--color-${map[c] ?? 'info'})`;
  }
</script>

{#if open}
  <div class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onclick={close} role="dialog" tabindex="-1" onkeydown={(e) => { if (e.key === 'Escape') close(); }}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} class="w-full max-w-md bg-mantle border border-surface1 rounded-lg" role="document">
      <header class="px-5 py-3 border-b border-surface1 flex items-center gap-2">
        <h2 class="text-base font-semibold text-text flex-1">New event</h2>
        <button onclick={close} class="text-dim hover:text-text" aria-label="close">×</button>
      </header>
      <form onsubmit={submit} class="p-5 space-y-3">
        <input bind:value={title} required autofocus placeholder="title" class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary" />
        <div class="grid grid-cols-3 gap-2">
          <input type="date" bind:value={dateISO} required class="px-2 py-2 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <input type="time" bind:value={startTime} placeholder="start" class="px-2 py-2 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <input type="time" bind:value={endTime} placeholder="end" class="px-2 py-2 bg-surface0 border border-surface1 rounded text-sm text-text" />
        </div>
        {#if conflicts.length > 0}
          <!-- Conflict warning. Inline, non-blocking — overlaps are
               sometimes intentional (back-to-back meetings the user
               wants flagged but not refused), so we surface the clash
               and let the user decide. -->
          <div class="px-3 py-2 bg-warning/10 border border-warning/30 rounded text-[11px]">
            <div class="text-warning font-semibold mb-1">⚠ Overlaps {conflicts.length} existing event{conflicts.length === 1 ? '' : 's'}:</div>
            <ul class="space-y-0.5">
              {#each conflicts as c}
                <li class="text-warning/90 font-mono">{c.start}–{c.end} · {c.title}</li>
              {/each}
            </ul>
          </div>
        {/if}
        <input bind:value={location} placeholder="location (optional)" class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text" />
        <div class="flex items-center gap-2">
          <span class="text-[11px] text-dim uppercase tracking-wider">Color</span>
          {#each colorOptions as c}
            <button
              type="button"
              onclick={() => (color = c)}
              aria-label={c}
              class="w-6 h-6 rounded-full border-2 {color === c ? 'border-text' : 'border-surface1'}"
              style="background: {tone(c)}"
            ></button>
          {/each}
        </div>
        <button type="submit" disabled={!title.trim() || saving} class="w-full px-4 py-2.5 bg-primary text-on-primary rounded font-medium disabled:opacity-50">{saving ? 'creating…' : 'Create event'}</button>
      </form>
    </div>
  </div>
{/if}
