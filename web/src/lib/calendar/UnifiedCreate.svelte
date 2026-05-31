<script lang="ts">
  import { api, buildRRULE, EVENT_STATUSES, type CalendarSource, type ICSRecurrenceFreq } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { fmtDateISO } from './utils';
  import TimeInput from './TimeInput.svelte';

  // Unified create modal — task or event — invoked from drag-to-create on
  // the calendar grid OR from the sidebar buttons. Single dialog so the
  // user picks "task vs event" at the point of creation; same start/duration
  // can flow through either branch.

  let {
    open = $bindable(false),
    start,        // Date the user dragged from
    end,          // Date the user dragged to (or start+30min if no drag)
    defaultKind = 'task',
    defaultNotePath,
    onCreated
  }: {
    open?: boolean;
    start: Date;
    end: Date;
    defaultKind?: 'task' | 'event' | 'content';
    defaultNotePath?: string;
    onCreated: () => void | Promise<void>;
  } = $props();

  // Initial value is a static fallback; the $effect below re-seeds from
  // `defaultKind` every time the modal opens, so a prop change after
  // instantiation propagates correctly. Reading the prop directly here
  // would warn about capturing only the initial value.
  //
  // 'content' is a third preset: same wire shape as 'event' but
  // pre-fills kind='content' + Idea status + a channel chip input.
  // Routes through createEvent under the hood — content just IS an
  // event with extra metadata, no separate endpoint.
  let kind = $state<'task' | 'event' | 'content'>('task');
  // Content-only seed: status defaults to 'idea' so a fresh content
  // event starts at the top of the funnel. Channels is freeform —
  // single chip is enough at create-time, more get added in
  // EventDetail later.
  let contentStatus = $state<string>('idea');
  let contentChannel = $state<string>('');
  let title = $state('');
  let dateISO = $state('');
  let notePath = $state('');
  let priority = $state(0);
  let location = $state('');
  // Empty default → eventTypeColor falls through to the per-title hash
  // rotate, so a series of drag-created events get distinct hues without
  // the user picking each one. Picking a color here flips it explicit.
  let color = $state('');
  // Reminder offset in minutes. 0 = no reminder. The push scheduler
  // fires a Web Push at (event.start - remindMinsBefore). Common
  // useful values; user can pick any of the chips or 0 to skip.
  let remindMinsBefore = $state(0);
  let saving = $state(false);
  let titleEl: HTMLInputElement | undefined = $state();

  // Calendar picker — "" means events.json (granit-native), any other
  // value is a writable .ics filename. Submit gates on this.
  let calendarTarget = $state<string>('');
  let writableSources = $state<CalendarSource[]>([]);

  // Recurrence (event branch only). FREQ="" disables recurrence.
  let recurFreq = $state<ICSRecurrenceFreq>('');
  let recurInterval = $state<number>(1);
  let recurCount = $state<number | ''>('');
  let recurUntil = $state<string>('');
  let recurByDay = $state<Set<string>>(new Set());

  async function loadSources() {
    try {
      const r = await api.listCalendarSources();
      writableSources = r.sources.filter((s) => s.writable);
    } catch {
      writableSources = [];
    }
  }

  // 24-hour HH:MM picker — bindable strings owned here and forwarded
  // to the shared TimeInput (paired HH+MM selects in 5-min steps).
  // Native <input type="time"> renders AM/PM on most OS locales
  // regardless of the element's lang attribute, so we keep the
  // 24-hour selects. Strings are read synchronously by the submit
  // handler — no flush race, which previously surfaced as "the times
  // … get scheduled somewhere" because the submit observed stale
  // startTime values that hadn't picked up the latest H/M yet.
  let startTime = $state('00:00');
  let endTime = $state('00:00');

  // Track the previous start in minutes so we can shift the end when
  // the user changes start. Without this, picking 14:00-15:00 then
  // changing start to 15:00 leaves end at 15:00 → 0-minute event →
  // endBeforeStart gate fires and the user can't save. Auto-shifting
  // keeps the duration the user already chose.
  function hhmmToMin(s: string): number {
    const [h, m] = s.split(':').map(Number);
    return (h || 0) * 60 + (m || 0);
  }
  function minToHHMM(n: number): string {
    const wrapped = ((n % (24 * 60)) + 24 * 60) % (24 * 60);
    const h = Math.floor(wrapped / 60);
    const m = wrapped % 60;
    return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
  }
  let prevStartMinutes = $state(0);
  let suppressShift = $state(false);
  $effect(() => {
    const cur = hhmmToMin(startTime);
    const endCur = hhmmToMin(endTime);
    if (suppressShift) {
      // Skip shift on programmatic resets (open / kind-switch /
      // duration-preset button).
      prevStartMinutes = cur;
      suppressShift = false;
      return;
    }
    if (cur !== prevStartMinutes && endCur > 0) {
      const dur = endCur - prevStartMinutes;
      if (dur > 0) {
        endTime = minToHHMM(cur + dur);
      }
    }
    prevStartMinutes = cur;
  });

  // Re-seed every time the modal opens — `start`/`end` reflect the most
  // recent drag, so the buffer must follow.
  $effect(() => {
    if (!open) return;
    kind = defaultKind;
    // Reset content-only fields on every open so a previous
    // content event's channel doesn't bleed into a fresh form.
    contentStatus = 'idea';
    contentChannel = '';
    dateISO = fmtDateISO(start);
    // Suppress the auto-shift effect during the open-time reseed so
    // we don't drag the end time to follow start when the user
    // hasn't even seen the form yet.
    suppressShift = true;
    // Seed startTime / endTime from the prop's start/end. Round
    // minutes to the nearest 5 so they line up with the picker's
    // options. The grid drag-create already snaps to 15min so this
    // rounding is usually a no-op; defends against modal re-opens
    // with hand-picked starts.
    const pad2 = (n: number) => String(n).padStart(2, '0');
    let sh = start.getHours();
    let sm = Math.round(start.getMinutes() / 5) * 5;
    if (sm === 60) { sh = (sh + 1) % 24; sm = 0; }
    let eh = end.getHours();
    let em = Math.round(end.getMinutes() / 5) * 5;
    if (em === 60) { eh = (eh + 1) % 24; em = 0; }
    startTime = `${pad2(sh)}:${pad2(sm)}`;
    endTime = `${pad2(eh)}:${pad2(em)}`;
    title = '';
    notePath = defaultNotePath ?? `Jots/${dateISO}.md`;
    priority = 0;
    location = '';
    color = '';
    remindMinsBefore = 0;
    calendarTarget = '';
    recurFreq = '';
    recurInterval = 1;
    recurCount = '';
    recurUntil = '';
    recurByDay = new Set();
    loadSources();
    setTimeout(() => titleEl?.focus(), 50);
  });

  function toggleByDay(d: string) {
    const next = new Set(recurByDay);
    if (next.has(d)) next.delete(d);
    else next.add(d);
    recurByDay = next;
  }

  // RFC3339 in the user's local zone — events.json + ICS endpoints both
  // accept this shape; the server's parser keeps the offset.
  //
  // CRITICAL: build the Date from `dateISO` (the input the user can
  // actively change), NOT from the `start` prop captured at modal-
  // open. Earlier this read `new Date(start)` and silently dropped
  // the user's date-picker edits — drag-create on Wednesday, change
  // the date to Friday, hit save, the event landed on Wednesday.
  // The user reported this as "random times / not in the calendar".
  function localRFC3339(timeStr: string): string {
    const [h, m] = parseHHMM(timeStr);
    const [y, mo, d] = dateISO.split('-').map(Number);
    if (!y || !mo || !d) return new Date().toISOString();
    const dt = new Date(y, mo - 1, d, h, m, 0, 0);
    return dt.toISOString();
  }

  // Parse "HH:MM" → [hours, minutes]. Tolerates "9:5" and similar
  // sloppy inputs; clamps to a valid 24-hour clock so a typo can't
  // produce nonsense like 25:99. Returning a clamped value is safer
  // than throwing because the form's submit gate already validates
  // upstream — this is just defence in depth.
  function parseHHMM(s: string): [number, number] {
    const parts = (s ?? '').split(':');
    let h = Number(parts[0]) || 0;
    let m = Number(parts[1]) || 0;
    if (h < 0) h = 0; if (h > 23) h = 23;
    if (m < 0) m = 0; if (m > 59) m = 59;
    return [h, m];
  }

  function close() { open = false; }

  function durationMinutes(): number {
    const [sh, sm] = parseHHMM(startTime);
    const [eh, em] = parseHHMM(endTime);
    return Math.max(15, (eh * 60 + em) - (sh * 60 + sm));
  }

  // True when end-time is at-or-before start-time. The submit
  // button uses this to gate creation so the user can never persist
  // an inverted range; the form surfaces a clear inline message
  // explaining what's wrong.
  let endBeforeStart = $derived.by(() => {
    if (!startTime || !endTime) return false;
    const [sh, sm] = parseHHMM(startTime);
    const [eh, em] = parseHHMM(endTime);
    return eh * 60 + em <= sh * 60 + sm;
  });

  // Live human-readable preview of the committed datetime range —
  // surfaced in the form so the user can verify what will actually
  // be saved before they hit Create. This is the most reliable way
  // to defuse "the time picker rendered AM but I expected PM" type
  // confusion: even when the input UI lies, the preview line is
  // built directly from the stored 24-hour values.
  let rangePreview = $derived.by(() => {
    if (!dateISO || !startTime || !endTime) return '';
    const [y, mo, d] = dateISO.split('-').map(Number);
    if (!y || !mo || !d) return '';
    const dt = new Date(y, mo - 1, d);
    const dayLabel = dt.toLocaleDateString(undefined, {
      weekday: 'short', month: 'short', day: 'numeric'
    });
    return `${dayLabel} · ${startTime} → ${endTime}`;
  });

  function startISO(): string {
    return localRFC3339(startTime);
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!title.trim()) return;
    saving = true;
    try {
      if (kind === 'task') {
        await api.createTask({
          notePath,
          text: title.trim(),
          priority: priority || undefined,
          scheduledStart: startISO(),
          durationMinutes: durationMinutes(),
          section: '## Tasks'
        });
      } else if (calendarTarget) {
        // Route through the writable-ICS endpoint. Recurrence is
        // event-only; tasks have their own recurrence story.
        const rrule = buildRRULE({
          freq: recurFreq,
          interval: recurInterval,
          count: typeof recurCount === 'number' ? recurCount : undefined,
          until: recurUntil || undefined,
          byDay: recurFreq === 'WEEKLY' ? Array.from(recurByDay) : undefined
        });
        await api.createICSEvent(calendarTarget, {
          summary: title.trim(),
          start: localRFC3339(startTime),
          end: localRFC3339(endTime),
          location: location.trim() || undefined,
          rrule: rrule || undefined
        });
      } else if (kind === 'content') {
        // Content event — same wire shape as a native event, with
        // the content-pipeline metadata pre-attached. Channel is
        // optional at create-time (the user can fill it in
        // EventDetail later if they're still scoping); status
        // defaults to 'idea' to match the natural top-of-funnel
        // starting point.
        const ch = contentChannel.trim();
        await api.createEvent({
          title: title.trim(),
          date: dateISO,
          start_time: startTime,
          end_time: endTime,
          location: location.trim(),
          color,
          remind_minutes_before: remindMinsBefore || undefined,
          kind: 'content',
          status: contentStatus,
          channels: ch ? [ch] : []
        });
      } else {
        await api.createEvent({
          title: title.trim(),
          date: dateISO,
          start_time: startTime,
          end_time: endTime,
          location: location.trim(),
          color,
          remind_minutes_before: remindMinsBefore || undefined
        });
      }
      close();
      await onCreated();
      toast.success(
        kind === 'task' ? 'task scheduled' : kind === 'content' ? 'content scheduled' : 'event created'
      );
    } catch (err) {
      toast.error('save failed: ' + (errorMessage(err)));
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
</script>

{#if open}
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
      class="w-full max-w-md bg-mantle border border-surface1 rounded-t-lg sm:rounded-lg shadow-xl max-h-[90dvh] overflow-y-auto"
      role="document"
    >
      <header class="px-5 py-3 border-b border-surface1 flex items-center gap-2">
        <h2 class="text-base font-semibold text-text flex-1">New {kind}</h2>
        <button onclick={close} class="text-dim hover:text-text" aria-label="close">×</button>
      </header>

      <!-- Type toggle: task / event / content. Big and obvious so the
           user can flip presets after a drag without re-doing the time
           selection. Content is a fast-path for production calendars —
           same wire shape as event, but with the pipeline metadata
           pre-attached so the user doesn't have to flip kind +
           ContentPanel manually after creation. -->
      <div class="px-5 pt-4">
        <div class="flex bg-surface0 border border-surface1 rounded-lg overflow-hidden">
          <button
            type="button"
            onclick={() => (kind = 'task')}
            class="flex-1 px-3 py-2 text-sm flex items-center justify-center gap-2 {kind === 'task' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          >
            <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="3" y="5" width="6" height="6" rx="1"/><path d="m4.5 8 1 1 2-2M13 7h8M13 17h8M3 17h6"/></svg>
            Task
          </button>
          <button
            type="button"
            onclick={() => (kind = 'event')}
            class="flex-1 px-3 py-2 text-sm flex items-center justify-center gap-2 {kind === 'event' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          >
            <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/></svg>
            Event
          </button>
          <button
            type="button"
            onclick={() => (kind = 'content')}
            class="flex-1 px-3 py-2 text-sm flex items-center justify-center gap-2 {kind === 'content' ? 'bg-lavender text-on-primary' : 'text-subtext hover:bg-surface1'}"
          >
            <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 5h16M4 12h10M4 19h16"/><circle cx="18" cy="12" r="3"/></svg>
            Content
          </button>
        </div>
      </div>

      <form onsubmit={submit} class="p-5 pb-[calc(1.25rem+env(safe-area-inset-bottom,0px))] sm:pb-5 space-y-3">
        <input
          bind:this={titleEl}
          bind:value={title}
          required
          placeholder={kind === 'task' ? 'what needs doing?' : kind === 'content' ? 'what are you publishing?' : 'event title'}
          class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded-lg text-sm text-text focus:outline-none focus:border-primary"
        />

        {#if kind === 'content'}
          <!-- Content-only fast-path: a single channel + status seed
               so the user gets a usable production-pipeline event in
               one form-fill. More channels and tags can be added in
               EventDetail later — this surface is the "capture, don't
               scope" optimisation. -->
          <label class="block">
            <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Channel</span>
            <input
              type="text"
              bind:value={contentChannel}
              placeholder="twitter, linkedin, blog..."
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded-lg text-sm text-text focus:outline-none focus:border-lavender"
            />
          </label>
          <div>
            <div class="block text-[11px] uppercase tracking-wider text-dim mb-1">Status</div>
            <div class="flex flex-wrap gap-1">
              {#each EVENT_STATUSES as s (s)}
                <button
                  type="button"
                  onclick={() => (contentStatus = s)}
                  class="text-[11px] px-2 py-1 rounded border transition-colors {contentStatus === s ? 'bg-lavender/15 border-lavender/40 text-lavender' : 'bg-surface0 border-surface1 text-subtext hover:bg-surface1'}"
                >{s[0].toUpperCase() + s.slice(1)}</button>
              {/each}
            </div>
          </div>
        {/if}

        <!-- Date + time row — shared between task & event branches.
             Each input gets an explicit label so a 12-hour-locale
             time picker can't confuse the user about which slot is
             which. The duration + range preview below ARE the
             trustworthy display: built directly from the stored
             24-hour values, so even if the OS-rendered picker shows
             AM/PM, the preview line confirms what gets persisted. -->
        <label class="block">
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Date</span>
          <input
            type="date"
            bind:value={dateISO}
            required
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded-lg text-sm text-text focus:outline-none focus:border-primary"
          />
        </label>
        <!-- Custom 24-hour time picker — shared TimeInput keeps the
             paired-select markup. Native <input type="time"> renders
             AM/PM on most OS locales regardless of the element's lang
             attribute, so we use HH + MM selects: every browser, every
             OS, always 24-hour. step=5 keeps the existing 5-minute
             granularity (most calendar interactions snap to 15, so 5
             covers every realistic pick without 60 menu items). -->
        <TimeInput bind:startTime bind:endTime step={5} />
        <!-- Duration quick-pick. Sets the end-time to start +
             selected duration. Saves users from clicking through
             two select dropdowns when they want a standard slot. -->
        <div class="flex items-center gap-1.5 flex-wrap">
          <span class="text-[11px] uppercase tracking-wider text-dim mr-1">Duration</span>
          {#each [
            { mins: 15, label: '15m' },
            { mins: 30, label: '30m' },
            { mins: 45, label: '45m' },
            { mins: 60, label: '1h' },
            { mins: 90, label: '1.5h' },
            { mins: 120, label: '2h' }
          ] as preset}
            {@const active = hhmmToMin(endTime) - hhmmToMin(startTime) === preset.mins}
            <button
              type="button"
              onclick={() => {
                const startMin = hhmmToMin(startTime);
                suppressShift = true;
                endTime = minToHHMM(startMin + preset.mins);
              }}
              class="px-2 py-1 text-xs rounded border transition-colors
                {active ? 'bg-surface1 border-primary text-primary' : 'bg-surface0 border-surface1 text-subtext hover:border-primary'}"
            >{preset.label}</button>
          {/each}
        </div>
        <!-- Range preview + duration — the user-facing source of
             truth. Shows exactly what will be saved (24-hour, the
             actual stored values). Tinted error when end ≤ start
             so the inverted-range case is obvious before submit. -->
        <div
          class="flex items-baseline justify-between text-[11px] -mt-1 px-1"
          class:text-error={endBeforeStart}
          class:text-dim={!endBeforeStart}
        >
          {#if endBeforeStart}
            <span>End must be after start</span>
            <span class="font-mono">{startTime} → {endTime}</span>
          {:else}
            <span class="truncate">{rangePreview}</span>
            <span class="font-mono tabular-nums flex-shrink-0">{durationMinutes()} min</span>
          {/if}
        </div>

        {#if kind === 'task'}
          <input
            bind:value={notePath}
            placeholder="note path (vault-relative .md)"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded-lg text-sm text-text font-mono"
          />
          <div class="flex items-center gap-2">
            <span class="text-xs text-dim flex-shrink-0">Priority</span>
            <div class="flex bg-surface0 border border-surface1 rounded-lg overflow-hidden text-xs">
              {#each [{ p: 0, label: 'none' }, { p: 1, label: 'P1' }, { p: 2, label: 'P2' }, { p: 3, label: 'P3' }] as o}
                <button
                  type="button"
                  onclick={() => (priority = o.p)}
                  class="px-3 py-1.5 {priority === o.p ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
                >{o.label}</button>
              {/each}
            </div>
          </div>
        {:else}
          {#if kind !== 'content'}
            <!-- Calendar picker. Default ("") = events.json (granit-native);
                 any other value routes through the new ICS endpoints.
                 Wrapping the select inside the label is the simplest
                 valid label-association pattern (no for/id pair needed).
                 Hidden for the content preset since content events
                 always live in events.json (the pipeline metadata
                 doesn't fit ICS's VEVENT vocabulary). -->
            <label class="block">
              <span class="block text-[11px] text-dim uppercase tracking-wider mb-1">Calendar</span>
              <select
                bind:value={calendarTarget}
                class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded-lg text-sm text-text"
              >
                <option value="">events.json (default)</option>
                {#each writableSources as src}
                  <option value={src.source}>{src.source}</option>
                {/each}
              </select>
            </label>
          {/if}

          <input
            bind:value={location}
            placeholder="location (optional)"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded-lg text-sm text-text"
          />

          {#if !calendarTarget}
            <!-- Color only matters for events.json. ICS events get
                 colored per-source on the grid. -->
            <div class="flex items-center gap-2">
              <span class="text-[11px] text-dim uppercase tracking-wider">Color</span>
              {#each colorOptions as c (c.name)}
                <button
                  type="button"
                  onclick={() => (color = c.name)}
                  aria-label={c.name}
                  title={c.name}
                  class="w-6 h-6 rounded-full border-2 {color === c.name ? 'border-text' : 'border-surface1'}"
                  style="background: {c.hex}"
                ></button>
              {/each}
            </div>
            <!-- Reminder offset. Server scheduler fires a Web Push
                 at (event.start - remindMinsBefore). Available on
                 events.json events only since ICS round-trip would
                 need VALARM support, which we haven't built yet. -->
            <div class="flex items-center gap-1.5 flex-wrap">
              <span class="text-[11px] uppercase tracking-wider text-dim mr-1">Remind</span>
              {#each [
                { mins: 0,  label: 'off' },
                { mins: 5,  label: '5m' },
                { mins: 15, label: '15m' },
                { mins: 30, label: '30m' },
                { mins: 60, label: '1h' },
                { mins: 1440, label: '1d' }
              ] as preset}
                {@const active = remindMinsBefore === preset.mins}
                <button
                  type="button"
                  onclick={() => (remindMinsBefore = preset.mins)}
                  class="px-2 py-1 text-xs rounded border transition-colors
                    {active ? 'bg-surface1 border-primary text-primary' : 'bg-surface0 border-surface1 text-subtext hover:border-primary'}"
                >{preset.label}</button>
              {/each}
            </div>
          {:else}
            <!-- Recurrence — only available on the ICS branch since
                 events.json doesn't support RRULE. -->
            <div class="flex items-center gap-2">
              <span class="text-[11px] text-dim uppercase tracking-wider w-20">Repeats</span>
              <select
                bind:value={recurFreq}
                class="flex-1 px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text"
              >
                <option value="">never</option>
                <option value="DAILY">daily</option>
                <option value="WEEKLY">weekly</option>
                <option value="MONTHLY">monthly</option>
                <option value="YEARLY">yearly</option>
              </select>
            </div>
            {#if recurFreq}
              <div class="flex items-center gap-2">
                <span class="text-[11px] text-dim uppercase tracking-wider w-20">Every</span>
                <input
                  type="number"
                  min="1"
                  bind:value={recurInterval}
                  class="w-16 px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text"
                />
                <span class="text-xs text-dim">{recurFreq.toLowerCase()}{recurInterval > 1 ? '' : ''}(s)</span>
              </div>
              {#if recurFreq === 'WEEKLY'}
                <div class="flex items-center gap-1">
                  <span class="text-[11px] text-dim uppercase tracking-wider w-20">On</span>
                  {#each [['MO', 'M'], ['TU', 'T'], ['WE', 'W'], ['TH', 'T'], ['FR', 'F'], ['SA', 'S'], ['SU', 'S']] as [v, l]}
                    <button
                      type="button"
                      onclick={() => toggleByDay(v)}
                      class="w-7 h-7 rounded text-xs font-medium {recurByDay.has(v) ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext border border-surface1'}"
                    >{l}</button>
                  {/each}
                </div>
              {/if}
              <div class="grid grid-cols-2 gap-2">
                <input
                  type="number"
                  min="1"
                  placeholder="count (optional)"
                  bind:value={recurCount}
                  class="px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text"
                />
                <input
                  type="date"
                  placeholder="until (optional)"
                  bind:value={recurUntil}
                  class="px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text"
                />
              </div>
            {/if}
          {/if}
        {/if}

        <button
          type="submit"
          disabled={!title.trim() || saving || endBeforeStart}
          class="w-full px-4 py-2.5 bg-primary text-on-primary rounded-lg font-medium disabled:opacity-50"
        >{saving ? 'creating…' : `Create ${kind}`}</button>
      </form>
    </div>
  </div>
{/if}

