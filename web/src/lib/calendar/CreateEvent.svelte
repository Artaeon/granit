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
  let remindMinutes = $state(0); // 0 = no reminder
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
    color = '';
    remindMinutes = 0;
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
        color,
        remind_minutes_before: remindMinutes || undefined
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

  // Reminder presets — same set the notification scheduler honors.
  // 0 means no reminder; the API call drops the field via undefined.
  const reminderOptions: { value: number; label: string }[] = [
    { value: 0, label: 'No reminder' },
    { value: 5, label: '5 min before' },
    { value: 10, label: '10 min before' },
    { value: 15, label: '15 min before' },
    { value: 30, label: '30 min before' },
    { value: 60, label: '1 hr before' },
    { value: 1440, label: '1 day before' }
  ];
</script>

{#if open}
  <!-- Container: mobile slides up from bottom (items-end + rounded-t),
       desktop centers (items-center + rounded). max-h-[90dvh] with
       overflow-y-auto on the inner card so a long form scrolls
       inside the sheet on narrow viewports. dvh (dynamic viewport
       height) accounts for the iOS Safari address-bar height
       changes — vh would clip when the address bar is visible. -->
  <div
    class="fixed inset-0 z-50 bg-black/40 backdrop-blur-sm flex items-end sm:items-center justify-center sm:p-4"
    onclick={close}
    role="dialog"
    tabindex="-1"
    onkeydown={(e) => { if (e.key === 'Escape') close(); }}
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      class="w-full sm:max-w-md bg-mantle border border-surface1 rounded-t-xl sm:rounded-lg shadow-2xl max-h-[90dvh] overflow-y-auto"
      role="document"
    >
      <!-- Mobile drag-handle visual hint. Doesn't actually drag —
           the sheet is dismissed via tap-outside or the X — but it
           signals "this is a sheet" the way iOS / Android apps do. -->
      <div class="sm:hidden flex justify-center pt-2 pb-1">
        <span class="block w-10 h-1 rounded-full bg-surface2"></span>
      </div>
      <header class="px-5 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <h2 class="text-base font-semibold text-text flex-1">New event</h2>
        <button onclick={close} class="text-dim hover:text-text text-2xl leading-none w-8 h-8 -mr-2 flex items-center justify-center" aria-label="close">×</button>
      </header>
      <form onsubmit={submit} class="p-4 sm:p-5 pb-[calc(1rem+env(safe-area-inset-bottom,0px))] sm:pb-5 space-y-4">
        <!-- Title — bigger touch target than the rest since it's
             the only required field besides the date. -->
        <div>
          <label class="block text-[11px] uppercase tracking-wider text-dim mb-1.5" for="ev-title">Title</label>
          <input
            id="ev-title"
            bind:value={title}
            required
            placeholder="Event title"
            class="w-full px-3 py-3 sm:py-2.5 bg-surface0 border border-surface1 rounded-lg text-base sm:text-sm text-text focus:outline-none focus:border-primary"
          />
        </div>

        <!-- Date on its own row — the picker on iOS Safari pushes
             the keyboard up, and a 3-col grid was unreadable.
             Time inputs on a 2-col grid below for tap precision. -->
        <div>
          <label class="block text-[11px] uppercase tracking-wider text-dim mb-1.5" for="ev-date">Date</label>
          <input
            id="ev-date"
            type="date"
            bind:value={dateISO}
            required
            class="w-full px-3 py-3 sm:py-2.5 bg-surface0 border border-surface1 rounded-lg text-base sm:text-sm text-text focus:outline-none focus:border-primary"
          />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-[11px] uppercase tracking-wider text-dim mb-1.5" for="ev-start">Start</label>
            <input
              id="ev-start"
              type="time"
              bind:value={startTime}
              class="w-full px-3 py-3 sm:py-2.5 bg-surface0 border border-surface1 rounded-lg text-base sm:text-sm text-text font-mono tabular-nums focus:outline-none focus:border-primary"
            />
          </div>
          <div>
            <label class="block text-[11px] uppercase tracking-wider text-dim mb-1.5" for="ev-end">End</label>
            <input
              id="ev-end"
              type="time"
              bind:value={endTime}
              class="w-full px-3 py-3 sm:py-2.5 bg-surface0 border border-surface1 rounded-lg text-base sm:text-sm text-text font-mono tabular-nums focus:outline-none focus:border-primary"
            />
          </div>
        </div>

        {#if conflicts.length > 0}
          <!-- Conflict warning. Inline, non-blocking — overlaps are
               sometimes intentional (back-to-back meetings the user
               wants flagged but not refused), so we surface the clash
               and let the user decide. -->
          <div class="px-3 py-2 bg-warning/10 border border-warning/30 rounded-lg text-xs">
            <div class="text-warning font-semibold mb-1">⚠ Overlaps {conflicts.length} existing event{conflicts.length === 1 ? '' : 's'}</div>
            <ul class="space-y-0.5">
              {#each conflicts as c}
                <li class="text-warning/90 font-mono">{c.start}–{c.end} · {c.title}</li>
              {/each}
            </ul>
          </div>
        {/if}

        <div>
          <label class="block text-[11px] uppercase tracking-wider text-dim mb-1.5" for="ev-loc">Location</label>
          <input
            id="ev-loc"
            bind:value={location}
            placeholder="Where (optional)"
            class="w-full px-3 py-3 sm:py-2.5 bg-surface0 border border-surface1 rounded-lg text-base sm:text-sm text-text focus:outline-none focus:border-primary"
          />
        </div>

        <div>
          <label class="block text-[11px] uppercase tracking-wider text-dim mb-1.5" for="ev-rem">Reminder</label>
          <select
            id="ev-rem"
            bind:value={remindMinutes}
            class="w-full px-3 py-3 sm:py-2.5 bg-surface0 border border-surface1 rounded-lg text-base sm:text-sm text-text focus:outline-none focus:border-primary"
          >
            {#each reminderOptions as opt}
              <option value={opt.value}>{opt.label}</option>
            {/each}
          </select>
        </div>

        <div>
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1.5">Color</span>
          <!-- Color swatches sized for touch — 32px (w-8 h-8) on
               mobile, can shrink to 24px on desktop where pixel
               targeting is more forgiving. -->
          <div class="flex items-center gap-2 flex-wrap">
            {#each colorOptions as c}
              <button
                type="button"
                onclick={() => (color = c)}
                aria-label={c}
                class="w-8 h-8 sm:w-7 sm:h-7 rounded-full border-2 transition-transform {color === c ? 'border-text scale-110' : 'border-surface1'}"
                style="background: {tone(c)}"
              ></button>
            {/each}
            {#if color}
              <button
                type="button"
                onclick={() => (color = '')}
                class="text-[11px] text-dim hover:text-text px-2"
                aria-label="clear color"
              >clear</button>
            {/if}
          </div>
        </div>

        <button
          type="submit"
          disabled={!title.trim() || saving}
          class="w-full px-4 py-3 sm:py-2.5 bg-primary text-on-primary rounded-lg font-medium disabled:opacity-50 text-base sm:text-sm"
        >{saving ? 'Creating…' : 'Create event'}</button>
      </form>
    </div>
  </div>
{/if}
