<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type CalendarEvent, type Project } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { findEventType } from './eventTypes';
  import RecurrenceEditor from './RecurrenceEditor.svelte';
  import EventTypeChips from './EventTypeChips.svelte';

  let {
    open = $bindable(false),
    date,
    existingEvents = [],
    defaultProjectId = '',
    onCreated
  }: {
    open?: boolean;
    date: Date;
    /** Events shown on the surrounding calendar — used for conflict
     *  detection. Defaults to empty so callers that don't care about
     *  conflicts (e.g. unit tests) don't have to provide it. */
    existingEvents?: CalendarEvent[];
    /** Pre-fill the project picker — e.g. when the calendar page is
     *  filtered to one project, a fresh create should default to that
     *  project. Empty = no default link. */
    defaultProjectId?: string;
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
  // Event type — meeting / focus / personal / travel / break /
  // blocker / '' (generic). Drives the chip glyph + default tint on
  // the calendar grid. When the user toggles a type AND hasn't yet
  // set start/end, the form pre-fills the default duration so the
  // common "drop a 60-min focus block" workflow is one click.
  let kind = $state('');
  let remindMinutes = $state(0); // 0 = no reminder
  let projectId = $state('');
  let saving = $state(false);
  // Called by EventTypeChips' onChange. EventTypeChips already handles
  // the "tap same chip = clear" toggle locally and emits the resulting
  // kind ('' on clear). This handler only owns the duration-defaulting
  // side-effect: when the user picks a duration-bearing type AND start
  // is set AND end is still empty, pre-fill end so the common "60-min
  // focus block" workflow is one click. We don't fire on clears.
  function onKindChange(next: string) {
    if (!next) return;
    const def = findEventType(next);
    if (def?.defaultDurationMin && startTime && !endTime) {
      endTime = addMinutesToHHMM(startTime, def.defaultDurationMin);
    }
  }
  // Add minutes to an HH:MM 24h string. Clamps to 23:59 so a long
  // type pushed past midnight doesn't underflow into the previous
  // day (events.json schema is single-date; cross-midnight is a
  // separate constraint enforced at submit time).
  function addMinutesToHHMM(hhmm: string, addMin: number): string {
    const [h, m] = hhmm.split(':').map(Number);
    if (Number.isNaN(h) || Number.isNaN(m)) return hhmm;
    const total = Math.min(h * 60 + m + addMin, 23 * 60 + 59);
    const hh = Math.floor(total / 60);
    const mm = total - hh * 60;
    return `${String(hh).padStart(2, '0')}:${String(mm).padStart(2, '0')}`;
  }

  // Project list — loaded once on mount, kept until the page reloads.
  // Failure degrades silently (the picker shows the "(no project)"
  // option only) so a missing /projects endpoint doesn't block event
  // creation.
  let projects = $state<Project[]>([]);
  onMount(async () => {
    try {
      const r = await api.listProjects();
      projects = r.projects ?? [];
    } catch {
      projects = [];
    }
  });

  // ── Recurrence picker ─────────────────────────────────────────
  // Stored as RFC 5545 RRULE strings — the shared RecurrenceEditor
  // owns parse + serialise + the BYDAY picker. We only carry the
  // bindable string and the AI parse maps natural-language preset
  // names into the same RRULE shape so the editor seeds correctly.
  let rrule = $state('');

  // Map AI-parsed preset names ("weekly", "weekdays", …) to an
  // RRULE string so RecurrenceEditor's parser picks the matching
  // preset back up. Keeps the AI prompt simple (still emits the
  // friendly preset name) while letting the editor own the canonical
  // shape. Unknown values map to '' (no recurrence).
  function presetToRRule(preset: string): string {
    switch (preset) {
      case 'daily': return 'FREQ=DAILY';
      case 'weekdays': return 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR';
      case 'weekly': return 'FREQ=WEEKLY';
      case 'biweekly': return 'FREQ=WEEKLY;INTERVAL=2';
      case 'monthly': return 'FREQ=MONTHLY';
      case 'yearly': return 'FREQ=YEARLY';
      default: return '';
    }
  }

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
      // Seed the project picker from the parent's hint each time the
      // sheet opens (e.g. /calendar?project=Foo deep-link or the
      // calendar page's "filter by project" pre-fill). After open,
      // the user owns the value.
      if (!projectId) projectId = defaultProjectId;
    }
  });

  function close() {
    open = false;
    title = '';
    startTime = '';
    endTime = '';
    location = '';
    color = '';
    kind = '';
    remindMinutes = 0;
    projectId = '';
    aiInput = '';
    aiBusy = false;
    aiError = '';
    rrule = '';
  }

  // ─── AI natural-language parse ───────────────────────────────────
  // 'dinner Friday 7pm with Alice at the Olive Garden' → fills the
  // form. Strict JSON output through the audit-gated chat pipeline.
  // The user always reviews + clicks Save — we don't auto-create.
  let aiInput = $state('');
  let aiBusy = $state(false);
  let aiError = $state('');
  let aiAbort: AbortController | null = null;

  function todayStr(): string {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }

  async function aiParse() {
    const text = aiInput.trim();
    if (!text || aiBusy) return;
    aiAbort?.abort();
    aiAbort = new AbortController();
    aiBusy = true;
    aiError = '';
    let buf = '';
    try {
      // Lazy-load api so this widget tree-shakes without it for users
      // who never touch the AI input.
      const { api } = await import('$lib/api');
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'You parse natural-language event descriptions into JSON. Today\'s date is "' + todayStr() + '" (the user\'s local day). Return STRICTLY a JSON object, no fences, no prose: {"title": "<short event title, no leading verb like "schedule">", "date": "YYYY-MM-DD", "start": "HH:MM" (24h, optional — empty string for all-day), "end": "HH:MM" (24h, optional — derive from start + 1h if user says a time but no end), "location": "<optional>", "repeat": "none" | "daily" | "weekdays" | "weekly" | "biweekly" | "monthly" | "yearly" (optional, default "none")}. Resolve relative dates (today / tomorrow / Friday / next week) against today\'s date. Use 24h. If a time is given without end, default end = start + 1 hour. If no time at all, leave start and end empty (all-day). Pick a repeat value when the user says things like "every Monday", "weekly", "every weekday", "monthly", "every year" — otherwise leave it "none".'
          },
          { role: 'user', content: text }
        ],
        undefined,
        {
          onChunk: (c) => { buf += c; },
          onDone: () => {
            let cleaned = buf.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const parsed = JSON.parse(cleaned) as {
                title?: string;
                date?: string;
                start?: string;
                end?: string;
                location?: string;
                repeat?: string;
              };
              if (parsed.title) title = parsed.title;
              if (parsed.date && /^\d{4}-\d{2}-\d{2}$/.test(parsed.date)) dateISO = parsed.date;
              if (parsed.start && /^\d{2}:\d{2}$/.test(parsed.start)) startTime = parsed.start;
              if (parsed.end && /^\d{2}:\d{2}$/.test(parsed.end)) endTime = parsed.end;
              if (parsed.location) location = parsed.location;
              const validRepeats = ['none', 'daily', 'weekdays', 'weekly', 'biweekly', 'monthly', 'yearly'];
              if (parsed.repeat && validRepeats.includes(parsed.repeat)) {
                rrule = presetToRRule(parsed.repeat);
              }
              aiInput = '';
            } catch {
              aiError = 'Could not parse — try again or fill the fields manually.';
            }
          },
          onError: (err) => { aiError = err.message; }
        },
        aiAbort.signal
      );
    } finally {
      aiBusy = false;
      aiAbort = null;
    }
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
        remind_minutes_before: remindMinutes || undefined,
        rrule: rrule || undefined,
        project_id: projectId || undefined,
        kind: kind || undefined
      });
      close();
      await onCreated();
      toast.success('event created');
    } catch (err) {
      toast.error('create failed: ' + (errorMessage(err)));
    } finally {
      saving = false;
    }
  }

  const colorOptions: { name: string; hex: string }[] = [
    { name: 'red', hex: '#ff3b30' },
    { name: 'orange', hex: '#ff9500' },
    { name: 'yellow', hex: '#ffcc00' },
    { name: 'green', hex: '#34c759' },
    { name: 'mint', hex: '#00c7be' },
    { name: 'teal', hex: '#5ac8fa' },
    { name: 'blue', hex: '#007aff' },
    { name: 'indigo', hex: '#5856d6' },
    { name: 'purple', hex: '#af52de' },
    { name: 'pink', hex: '#ff2d55' },
    { name: 'brown', hex: '#a2845e' },
    { name: 'gray', hex: '#8e8e93' }
  ];

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
    class="fixed inset-0 z-50 bg-black/40 flex items-end sm:items-center justify-center sm:p-4"
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
        <!-- AI parse — type 'dinner Friday 7pm with Alice at the
             Olive Garden' and it fills the fields below. The user
             always reviews + clicks Save; we never auto-create. The
             input lives at the top because the parse populates the
             whole form, so it's the natural starting point. -->
        <div class="rounded-lg bg-surface1 border border-surface2 p-2.5">
          <label class="block text-[10px] uppercase tracking-wider text-primary mb-1.5 inline-flex items-center gap-1" for="ev-ai">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
              <path d="M12 3l1.2 4.2L17 9l-3.8 1.8L12 15l-1.2-4.2L7 9l3.8-1.8L12 3z" stroke-linejoin="round"/>
            </svg>
            Quick parse with AI
          </label>
          <div class="flex gap-2">
            <input
              id="ev-ai"
              bind:value={aiInput}
              onkeydown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); void aiParse(); } }}
              placeholder='e.g. "dinner Friday 7pm with Alice at the Olive Garden"'
              disabled={aiBusy}
              class="flex-1 px-2.5 py-2 bg-mantle border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary disabled:opacity-60"
            />
            <button
              type="button"
              onclick={aiParse}
              disabled={!aiInput.trim() || aiBusy}
              class="px-3 py-2 bg-primary text-on-primary rounded text-sm disabled:opacity-50"
            >{aiBusy ? '…' : 'Parse'}</button>
          </div>
          {#if aiError}
            <p class="text-[11px] text-error mt-1.5">{aiError}</p>
          {:else}
            <p class="text-[10px] text-dim mt-1 leading-snug">
              Date / time / location resolve from natural language. Review the fields below and click Create.
            </p>
          {/if}
        </div>

        <!-- Event type. Optional — leaving it generic still creates
             a valid event, just without the glyph prefix + auto-tint
             on the grid. Tapping the same chip twice clears it.
             Picking a type with a defaultDurationMin pre-fills the
             end time if start is set + end is still empty (so a
             one-click "60-min focus block at 14:00" workflow exists
             from the create form). -->
        <div>
          <!-- Not a real form-label: the group below is a row of
               toggle buttons, not a single input. Span keeps the
               visual without lying about the relationship. -->
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1.5">Type <span class="text-dim normal-case tracking-normal">(optional)</span></span>
          <EventTypeChips bind:kind onChange={onKindChange} chipSize="comfy" />
        </div>

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
          <div class="px-3 py-2 bg-surface0 border border-warning rounded-lg text-xs">
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

        <!-- Project link (optional). The calendar page surfaces a
             chip + colour overlay for events with a project_id, and
             a per-project filter folds linked events alongside the
             project's tasks — so a user can use the calendar as a
             project-management surface. Hidden when no projects
             exist (fresh vault). -->
        {#if projects.length > 0}
          <div>
            <label class="block text-[11px] uppercase tracking-wider text-dim mb-1.5" for="ev-project">Project</label>
            <select
              id="ev-project"
              bind:value={projectId}
              class="w-full px-3 py-3 sm:py-2.5 bg-surface0 border border-surface1 rounded-lg text-base sm:text-sm text-text focus:outline-none focus:border-primary"
            >
              <option value="">No project</option>
              {#each projects as p (p.name)}
                <option value={p.name}>{p.name}</option>
              {/each}
            </select>
          </div>
        {/if}

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

        <!-- Repeat picker. Shared RecurrenceEditor owns the preset
             dropdown, the BYDAY weekday grid, the Until input, and the
             RRULE preview line. Common-case presets cover ~95% of
             calendar use; 'Custom' lets a power user paste a raw rule. -->
        <RecurrenceEditor
          bind:rrule
          layout="block"
          minDate={dateISO}
          idPrefix="ev"
        />

        <div>
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1.5">Color</span>
          <!-- Color swatches sized for touch — 32px (w-8 h-8) on
               mobile, can shrink to 24px on desktop where pixel
               targeting is more forgiving. -->
          <div class="flex items-center gap-2 flex-wrap">
            {#each colorOptions as c (c.name)}
              <button
                type="button"
                onclick={() => (color = c.name)}
                aria-label={c.name}
                title={c.name}
                class="w-8 h-8 sm:w-7 sm:h-7 rounded-full border-2 transition-transform {color === c.name ? 'border-text scale-110' : 'border-surface1'}"
                style="background: {c.hex}"
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
