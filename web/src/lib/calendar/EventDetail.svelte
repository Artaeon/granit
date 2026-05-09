<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, type CalendarEvent, type CalendarSource, type Project , todayISO } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { onMount } from 'svelte';
  import { eventStartDate, eventEndDate, fmtTime, eventTypeColor } from './utils';

  let {
    open = $bindable(false),
    event,
    onChanged
  }: {
    open?: boolean;
    event: CalendarEvent | null;
    onChanged?: () => void;
  } = $props();

  let busy = $state(false);
  let editing = $state(false);
  let editTitle = $state('');
  let editDate = $state('');
  let editLocation = $state('');
  let editColor = $state('cyan');
  let editProjectId = $state('');

  // Project list — loaded once on mount so the picker is populated by
  // the time the user clicks 'edit'. Failure degrades silently to "No
  // project" only.
  let projects = $state<Project[]>([]);
  async function loadProjects() {
    try {
      const r = await api.listProjects();
      projects = r.projects ?? [];
    } catch {
      projects = [];
    }
  }
  onMount(loadProjects);

  // 24-hour HH:MM picker buffers — same pattern as UnifiedCreate.
  // Native <input type="time"> renders AM/PM on most OS locales
  // regardless of any lang attribute. Custom HH + MM selects
  // guarantee 24-hour display everywhere.
  let editStartH = $state(0);
  let editStartM = $state(0);
  let editEndH = $state(0);
  let editEndM = $state(0);
  // editStartTime / editEndTime are DERIVED, not state-with-effect.
  // The previous $effect-driven sync had a flush race: the user
  // could change a select and immediately click Save, and the
  // submit handler would read a stale time string that hadn't
  // picked up the new H/M yet. Events landed at the previous
  // time, not the one the user just chose. The user reported this
  // as "editing the times also not, they get scheduled somewhere".
  // $derived is read synchronously on access — no flush race.
  let editStartTime = $derived(
    `${String(editStartH).padStart(2, '0')}:${String(editStartM).padStart(2, '0')}`
  );
  let editEndTime = $derived(
    `${String(editEndH).padStart(2, '0')}:${String(editEndM).padStart(2, '0')}`
  );

  // ── Recurrence edit state ──────────────────────────────────────
  // Mirrors the picker in CreateEvent. We seed from the source
  // event's rrule on edit-start by parsing the FREQ + INTERVAL +
  // BYDAY tokens; arbitrary RRULEs that don't match a preset land
  // in 'custom' and round-trip verbatim.
  type Repeat = 'none' | 'daily' | 'weekdays' | 'weekly' | 'biweekly' | 'monthly' | 'yearly' | 'custom';
  let editRepeat = $state<Repeat>('none');
  let editUntilDate = $state('');
  let editCustomRule = $state('');
  // For recurring events, edit-scope is a per-modal toggle:
  // 'series' rewrites the parent (date / time / rrule all shift),
  // 'instance' writes a per-occurrence override so only the open
  // occurrence changes. Defaults to 'instance' on open — safer
  // because editing one Tuesday rarely should touch every Tuesday.
  // Hidden for ICS events (no override path) and for non-recurring
  // events.
  let editScope = $state<'instance' | 'series'>('instance');

  function parseRepeatFromRRule(rrule: string): { repeat: Repeat; until: string; custom: string } {
    if (!rrule) return { repeat: 'none', until: '', custom: '' };
    const parts: Record<string, string> = {};
    for (const seg of rrule.split(';')) {
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
    if (freq === 'DAILY' && !interval && !byday) return { repeat: 'daily', until, custom: '' };
    if (freq === 'WEEKLY' && byday === 'MO,TU,WE,TH,FR' && !interval) return { repeat: 'weekdays', until, custom: '' };
    if (freq === 'WEEKLY' && !byday && (interval === '' || interval === '1')) return { repeat: 'weekly', until, custom: '' };
    if (freq === 'WEEKLY' && !byday && interval === '2') return { repeat: 'biweekly', until, custom: '' };
    if (freq === 'MONTHLY' && !interval && !byday) return { repeat: 'monthly', until, custom: '' };
    if (freq === 'YEARLY' && !interval) return { repeat: 'yearly', until, custom: '' };
    return { repeat: 'custom', until: '', custom: rrule };
  }
  function untilSuffix(date: string): string {
    if (!date || !/^\d{4}-\d{2}-\d{2}$/.test(date)) return '';
    return `;UNTIL=${date.replace(/-/g, '')}T235959Z`;
  }
  let editRRule = $derived.by((): string => {
    switch (editRepeat) {
      case 'none': return '';
      case 'daily': return 'FREQ=DAILY' + untilSuffix(editUntilDate);
      case 'weekdays': return 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR' + untilSuffix(editUntilDate);
      case 'weekly': return 'FREQ=WEEKLY' + untilSuffix(editUntilDate);
      case 'biweekly': return 'FREQ=WEEKLY;INTERVAL=2' + untilSuffix(editUntilDate);
      case 'monthly': return 'FREQ=MONTHLY' + untilSuffix(editUntilDate);
      case 'yearly': return 'FREQ=YEARLY' + untilSuffix(editUntilDate);
      case 'custom': return editCustomRule.trim();
    }
  });

  // Calendar sources — needed to know if an ICS event came from a
  // writable .ics file. Loaded once on mount; refreshed on demand.
  let sources = $state<CalendarSource[]>([]);
  async function loadSources() {
    try {
      const r = await api.listCalendarSources();
      sources = r.sources;
    } catch {
      sources = [];
    }
  }
  onMount(loadSources);

  // ICS events from a writable calendar are editable through the
  // ics-events endpoints; events.json events keep their existing path.
  let icsWritable = $derived.by(() => {
    if (event?.type !== 'ics_event' || !event.source) return false;
    const src = sources.find((s) => s.source === event.source);
    return !!src?.writable;
  });
  let editable = $derived((event?.type === 'event' && !!event?.eventId) || icsWritable);

  function startEdit() {
    if (!event) return;
    editTitle = event.title;
    editDate = event.date ?? (event.start ? event.start.slice(0, 10) : '');
    // editStartTime / editEndTime are derived from H+M now — seed
    // the H/M state instead. Direct string assignment to the
    // derived bindings would throw at runtime in Svelte 5.
    editLocation = event.location ?? '';
    editColor = event.color ?? 'cyan';
    // Seed the 24-hour selects from the event's start/end so the
    // picker shows the current value when edit-mode opens. Round
    // minutes to the nearest 5 to align with the select options;
    // the underlying time string still carries the exact value
    // until the user changes it.
    if (event.start) {
      const sd = new Date(event.start);
      editStartH = sd.getHours();
      editStartM = Math.round(sd.getMinutes() / 5) * 5;
      if (editStartM === 60) { editStartH = (editStartH + 1) % 24; editStartM = 0; }
    } else {
      editStartH = 0;
      editStartM = 0;
    }
    if (event.end) {
      const ed = new Date(event.end);
      editEndH = ed.getHours();
      editEndM = Math.round(ed.getMinutes() / 5) * 5;
      if (editEndM === 60) { editEndH = (editEndH + 1) % 24; editEndM = 0; }
    } else {
      editEndH = 0;
      editEndM = 0;
    }
    // Seed recurrence editor from the source rule. ICS events also
    // carry rrule but their write path goes through ics-events
    // endpoints which don't accept rrule today — show the rule
    // read-only via the picker but disable Save-as-series for ICS.
    const parsed = parseRepeatFromRRule(event.rrule ?? '');
    editRepeat = parsed.repeat;
    editUntilDate = parsed.until;
    editCustomRule = parsed.custom;
    // Seed the project link from the event so unchanged saves
    // round-trip. Empty when the event isn't linked.
    editProjectId = event.project_id ?? '';
    // Default scope: this-occurrence-only. Users editing 'this
    // Tuesday' through the modal get the same conservative default
    // as the drag-move flow.
    editScope = 'instance';
    editing = true;
  }

  // Build a UTC RFC3339 (Z-suffixed) string from YYYY-MM-DD + HH:MM
  // interpreted as the user's LOCAL wall clock. The Date constructor
  // (with separate y/mo/d/h/mi args) treats the inputs as local time,
  // and `.toISOString()` then renders the resulting instant in UTC —
  // so a EU user typing 14:30 sends "12:30:00Z" in summer. The
  // backend's parseClientTime accepts RFC3339, and the icswriter
  // emits UTC Z, so the round-trip preserves wall-clock intent.
  // Used only for the ICS write path; events.json takes separate
  // date + HH:MM fields and stores them verbatim (see the patchEvent
  // branch below). Name is utc* (not local*) because the OUTPUT is
  // a UTC instant — the misleading earlier name made it look like a
  // floating-time helper.
  function utcRFC3339FromLocalParts(date: string, time: string): string {
    const [y, mo, d] = date.split('-').map(Number);
    const [h, mi] = time.split(':').map(Number);
    return new Date(y, mo - 1, d, h, mi, 0, 0).toISOString();
  }

  async function saveEdit(e: SubmitEvent) {
    e.preventDefault();
    if (!event) return;
    busy = true;
    try {
      if (event.type === 'ics_event' && event.source && event.eventId) {
        await api.patchICSEvent(event.source, event.eventId, {
          summary: editTitle,
          start: utcRFC3339FromLocalParts(editDate, editStartTime || '00:00'),
          end: editEndTime ? utcRFC3339FromLocalParts(editDate, editEndTime) : undefined,
          location: editLocation
        });
      } else if (event.eventId) {
        // Recurring + 'this only' scope: write a per-occurrence
        // override on the original anchor. Series base + rrule are
        // not touched, so editing a single Tuesday doesn't shift
        // every Tuesday. The override carries title/location/color
        // too, so a user renaming "this Tuesday's standup" also
        // gets the rename surfaced for that one cell only.
        if (event.rrule && event.type === 'event' && editScope === 'instance') {
          const key = exDateKey();
          if (!key) {
            toast.error('Cannot identify this occurrence — try editing the series.');
            return;
          }
          await api.overrideEventOccurrence(event.eventId, key, {
            date: editDate,
            start_time: editStartTime,
            end_time: editEndTime,
            title: editTitle,
            location: editLocation,
            color: editColor
          });
        } else {
          await api.patchEvent(event.eventId, {
            title: editTitle,
            date: editDate,
            start_time: editStartTime,
            end_time: editEndTime,
            location: editLocation,
            color: editColor,
            // Send rrule unconditionally so editing a recurring event
            // back to non-recurring (editRepeat='none' → '') correctly
            // clears the rule rather than leaving the old one in place.
            rrule: editRRule,
            // Send project_id unconditionally too — clearing the link
            // (editProjectId='') must overwrite a previously-linked
            // project on disk, not be silently dropped by omitempty
            // round-tripping through Partial<>.
            project_id: editProjectId
          });
        }
      } else {
        return;
      }
      editing = false;
      onChanged?.();
      open = false;
      toast.success(
        event?.rrule && event.type === 'event' && editScope === 'instance'
          ? 'this occurrence updated'
          : 'event updated'
      );
    } catch (err) {
      toast.error('save failed: ' + (errorMessage(err)));
    } finally {
      busy = false;
    }
  }

  async function deleteEvent() {
    if (!event?.eventId) return;
    if (!confirm(`Delete event "${event.title}"?`)) return;
    busy = true;
    try {
      if (event.type === 'ics_event' && event.source) {
        await api.deleteICSEvent(event.source, event.eventId);
      } else {
        await api.deleteEvent(event.eventId);
      }
      onChanged?.();
      open = false;
      toast.success('event deleted');
    } catch (err) {
      toast.error('delete failed: ' + (errorMessage(err)));
    } finally {
      busy = false;
    }
  }

  // Skip just THIS occurrence of a recurring event — the user's
  // "team meeting cancelled this week, but keep the series" move.
  // Adds an EXDATE entry to the source event; the expander filters
  // it out the next time the calendar renders. The expander
  // (internal/serveapi/ics.go isExcluded) compares against:
  //   - YYYY-MM-DD for all-day events
  //   - YYYY-MM-DDTHH:MM:SS in UTC for timed events
  // We must send the UTC format so the next render's exclusion
  // check actually matches. Only native recurring events get this;
  // ICS skip would need to round-trip the source .ics file which
  // isn't on the patch path today.
  function exDateKey(): string {
    if (!event) return '';
    // For an already-overridden occurrence, the canonical anchor is
    // surfaced as event.override_key by the calendar feed. Prefer it
    // — re-deriving from event.start would point at the OVERRIDDEN
    // time, not the series anchor, and the EXDATE/Override map keys
    // by anchor.
    if (event.override_key) return event.override_key;
    if (event.start) {
      // event.start is the floating-ISO emit from handleCalendar
      // ("2026-05-09T08:00:00", no Z, no offset). The server keys
      // overrides + EXDATEs by the same wall-clock digits — slicing
      // the leading 19 chars matches that shape directly.
      // Round-tripping through new Date(...).toISOString() would
      // re-anchor the wall-clock to the client zone and then re-emit
      // in UTC, shifting the key by the client offset (e.g. on UTC+2,
      // 08:00 floating → 06:00 UTC). The skip / reset endpoints would
      // then store at the wrong anchor and the EXDATE would no longer
      // match the expander's emitted occurrence.
      return event.start.slice(0, 19);
    }
    return event.date ?? '';
  }
  // Reset a per-occurrence override back to series defaults. The
  // server side accepts an empty override body at the same key as
  // a clear; this surfaces it as a one-click action when an event
  // carries the override_key marker.
  async function resetOccurrence() {
    if (!event?.eventId || !event.override_key) return;
    if (!confirm(`Reset "${event.title}" on this date back to the series defaults?`)) return;
    busy = true;
    try {
      await api.overrideEventOccurrence(event.eventId, event.override_key, {});
      onChanged?.();
      open = false;
      toast.success('Occurrence reset to series defaults');
    } catch (err) {
      toast.error('reset failed: ' + (errorMessage(err)));
    } finally {
      busy = false;
    }
  }

  async function skipOccurrence() {
    if (!event?.eventId || !event.rrule) return;
    if (event.type !== 'event') {
      toast.info('Skipping ICS occurrences isn\'t supported yet — edit the source calendar.');
      return;
    }
    const key = exDateKey();
    if (!key) return;
    if (!confirm(`Skip just this occurrence of "${event.title}"? The rest of the series stays.`)) return;
    busy = true;
    try {
      await api.skipEventOccurrence(event.eventId, key);
      onChanged?.();
      open = false;
      toast.success('Occurrence cancelled · series unchanged');
    } catch (err) {
      toast.error('skip failed: ' + (errorMessage(err)));
    } finally {
      busy = false;
    }
  }

  const colorOptions = ['red', 'yellow', 'orange', 'green', 'blue', 'purple', 'cyan'];

  async function toggleDone() {
    if (!event?.taskId) return;
    busy = true;
    try {
      await api.patchTask(event.taskId, { done: !event.done });
      onChanged?.();
      open = false;
    } finally {
      busy = false;
    }
  }
  async function clearSchedule() {
    if (!event?.taskId) return;
    busy = true;
    try {
      await api.patchTask(event.taskId, { clearSchedule: true });
      onChanged?.();
      open = false;
    } finally {
      busy = false;
    }
  }

  function openNote() {
    if (event?.notePath) goto(`/notes/${encodeURIComponent(event.notePath)}`);
    open = false;
  }

  // Create a meeting note for this event and navigate to it. The
  // note lands at Meetings/<YYYY-MM-DD> · <slug-of-title>.md with
  // frontmatter that captures the event metadata so the note is
  // searchable + tag-filterable + later linkable from the daily.
  // If today's daily exists we also append a backlink line so the
  // user has a one-click trail from "what did I do today" to the
  // meeting note. Failures fall back to a toast.
  let creatingMeetingNote = $state(false);
  async function createMeetingNote() {
    if (!event || creatingMeetingNote) return;
    creatingMeetingNote = true;
    try {
      const date = (event.start ?? event.date ?? new Date().toISOString()).slice(0, 10);
      const slug = (event.title || 'meeting')
        .toLowerCase()
        .replace(/[^a-z0-9\s-]/g, '')
        .replace(/\s+/g, '-')
        .slice(0, 60);
      const path = `Meetings/${date} · ${slug}.md`;
      const startTimeStr = event.start ? new Date(event.start).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', hour12: false }) : '';
      const endTimeStr = event.end ? new Date(event.end).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', hour12: false }) : '';
      const fm: Record<string, unknown> = {
        type: 'meeting',
        date,
        title: event.title,
        tags: ['meeting'],
        // Round-trips so a future feature can link the note back to
        // the source event without a fuzzy search.
        sourceEvent: event.eventId ?? undefined,
        sourceCalendar: event.source ?? undefined
      };
      if (event.location) fm.location = event.location;
      if (startTimeStr) fm.start = startTimeStr;
      if (endTimeStr) fm.end = endTimeStr;
      // Strip undefined keys so the YAML serializer doesn't emit
      // `key: ~` lines for nothing.
      for (const k of Object.keys(fm)) if (fm[k] === undefined) delete fm[k];

      const body =
        `# ${event.title}\n\n` +
        (event.location ? `**Location:** ${event.location}\n` : '') +
        (startTimeStr || endTimeStr ? `**Time:** ${startTimeStr}${endTimeStr ? '–' + endTimeStr : ''}\n` : '') +
        `\n## Attendees\n- \n\n## Agenda\n- \n\n## Notes\n\n\n## Action items\n- [ ] \n`;

      await api.createNote({ path, frontmatter: fm, body });

      // Append a backlink to today's daily — best-effort.
      try {
        const today = todayISO();
        if (date === today) {
          const daily = await api.daily('today');
          const dailyBody = (daily.body ?? '') + `\n- [[${path}|${event.title}]] (meeting)\n`;
          await api.putNote(daily.path, { frontmatter: daily.frontmatter ?? {}, body: dailyBody });
        }
      } catch {}

      toast.success('Meeting note created');
      goto(`/notes/${encodeURIComponent(path)}`);
      open = false;
    } catch (err) {
      toast.error('Failed to create meeting note: ' + (errorMessage(err)));
    } finally {
      creatingMeetingNote = false;
    }
  }

  function close() { open = false; }
</script>

{#if open && event}
  {@const c = eventTypeColor(event)}
  {@const start = eventStartDate(event)}
  {@const end = eventEndDate(event)}
  <div
    class="fixed inset-0 z-50 bg-black/40 flex items-end sm:items-center justify-center sm:p-4"
    onclick={close}
    onkeydown={(e) => { if (e.key === 'Escape') close(); }}
    role="dialog"
    tabindex="-1"
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      class="w-full max-w-md bg-mantle border border-surface1 rounded-t-lg sm:rounded-lg p-5 space-y-3 max-h-[90dvh] overflow-y-auto pb-[calc(1.25rem+env(safe-area-inset-bottom,0px))] sm:pb-5"
      role="document"
    >
      <div class="flex items-start gap-3">
        <div class="w-1 self-stretch rounded-full" style="background: {c.border}"></div>
        <div class="flex-1">
          <div class="text-xs uppercase tracking-wider text-dim">{event.type.replace('_', ' ')}</div>
          <h2 class="text-lg font-semibold text-text {event.done ? 'line-through opacity-70' : ''}">{event.title}</h2>
          {#if start}
            <div class="text-sm text-subtext mt-1">
              {start.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' })}
              {#if event.start} · {fmtTime(start)}{#if end} – {fmtTime(end)}{/if}{/if}
            </div>
          {:else if event.date}
            <div class="text-sm text-subtext mt-1">{event.date}</div>
          {/if}
          {#if event.location}
            <div class="text-sm text-dim mt-1">@ {event.location}</div>
          {/if}
          {#if event.project_id}
            <!-- Project chip — links to the project page. Same surface
                 that drives the calendar's per-project filter; clicking
                 here jumps to the project's detail view. -->
            <a
              href={`/projects/${encodeURIComponent(event.project_id)}`}
              class="inline-flex items-center gap-1 text-xs px-2 py-0.5 mt-2 rounded-full bg-secondary/15 text-secondary border border-secondary/30 hover:bg-secondary/25"
              title="open project"
            >
              <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M3 7h7l2 2h9v11H3z" stroke-linejoin="round"/>
              </svg>
              {event.project_id}
            </a>
          {/if}
          {#if event.notePath}
            <div class="text-xs text-dim mt-2 font-mono truncate">{event.notePath}</div>
          {/if}
        </div>
      </div>

      {#if editing && editable}
        <form onsubmit={saveEdit} class="space-y-2 pt-2 border-t border-surface1">
          <input bind:value={editTitle} required placeholder="title" class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <!-- Stack on mobile: a 3-column row crushes the date input below
               usable width on phones. The date gets its own row, then
               start/end share a row. -->
          <input type="date" bind:value={editDate} required class="w-full px-2 py-2 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <!-- 24-hour HH:MM picker — paired selects, same pattern as
               UnifiedCreate. Native <input type="time"> respects the
               OS locale, not the element's lang, so a US-locale user
               saw AM/PM on every event edit. These selects always
               show 24-hour values. -->
          <div class="grid grid-cols-2 gap-2">
            <div>
              <span class="block text-[10px] uppercase tracking-wider text-dim mb-1">Start (24h)</span>
              <div class="flex items-center bg-surface0 border border-surface1 rounded overflow-hidden focus-within:border-primary">
                <select
                  bind:value={editStartH}
                  aria-label="start hour"
                  class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
                >
                  {#each Array.from({ length: 24 }, (_, i) => i) as h}
                    <option value={h}>{String(h).padStart(2, '0')}</option>
                  {/each}
                </select>
                <span class="text-dim px-1">:</span>
                <select
                  bind:value={editStartM}
                  aria-label="start minute"
                  class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
                >
                  {#each Array.from({ length: 12 }, (_, i) => i * 5) as m}
                    <option value={m}>{String(m).padStart(2, '0')}</option>
                  {/each}
                </select>
              </div>
            </div>
            <div>
              <span class="block text-[10px] uppercase tracking-wider text-dim mb-1">End (24h)</span>
              <div class="flex items-center bg-surface0 border border-surface1 rounded overflow-hidden focus-within:border-primary">
                <select
                  bind:value={editEndH}
                  aria-label="end hour"
                  class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
                >
                  {#each Array.from({ length: 24 }, (_, i) => i) as h}
                    <option value={h}>{String(h).padStart(2, '0')}</option>
                  {/each}
                </select>
                <span class="text-dim px-1">:</span>
                <select
                  bind:value={editEndM}
                  aria-label="end minute"
                  class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
                >
                  {#each Array.from({ length: 12 }, (_, i) => i * 5) as m}
                    <option value={m}>{String(m).padStart(2, '0')}</option>
                  {/each}
                </select>
              </div>
            </div>
          </div>
          <input bind:value={editLocation} placeholder="location (optional)" class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <!-- Edit scope picker — only relevant for recurring NATIVE
               events. 'this' writes a per-occurrence override (title /
               time / date / location / color of just this one); 'series'
               rewrites the parent so every occurrence shifts. ICS gets
               no scope picker because the patch endpoint has no override
               slot. Defaults to 'this' on open — same conservative
               default as the drag-move flow. -->
          {#if event?.type === 'event' && event?.rrule}
            <fieldset class="border border-surface1 rounded p-2 space-y-1">
              <legend class="text-[10px] uppercase tracking-wider text-dim px-1">Apply to</legend>
              <label class="flex items-center gap-2 text-xs text-text cursor-pointer">
                <input type="radio" bind:group={editScope} value="instance" name="ev-edit-scope" />
                <span>Just this occurrence</span>
                <span class="text-[10px] text-dim">— series base unchanged</span>
              </label>
              <label class="flex items-center gap-2 text-xs text-text cursor-pointer">
                <input type="radio" bind:group={editScope} value="series" name="ev-edit-scope" />
                <span>The entire series</span>
                <span class="text-[10px] text-dim">— shifts every instance</span>
              </label>
            </fieldset>
          {/if}
          <!-- Repeat picker — same shape as CreateEvent so the muscle
               memory is identical. ICS events DO get this shown for
               read-only feedback (the rrule of the source series),
               but the patch path for ICS doesn't currently accept
               rrule changes — surfacing the rule still helps the
               user understand what's recurring. The picker is
               disabled when the user is editing a single occurrence
               (recurrence is a series-level concept). -->
          {#if event?.type === 'event' && (!event?.rrule || editScope === 'series')}
            <div class="flex items-baseline gap-2 flex-wrap">
              <label class="text-[11px] text-dim uppercase tracking-wider" for="ev-edit-repeat">Repeat</label>
              <select
                id="ev-edit-repeat"
                bind:value={editRepeat}
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
              {#if editRepeat !== 'none' && editRepeat !== 'custom'}
                <label class="text-[11px] text-dim flex items-center gap-1.5">
                  until
                  <input
                    type="date"
                    bind:value={editUntilDate}
                    min={editDate}
                    class="bg-surface0 border border-surface1 rounded px-2 py-1 text-sm text-text"
                  />
                </label>
              {/if}
            </div>
            {#if editRepeat === 'custom'}
              <input
                bind:value={editCustomRule}
                placeholder="FREQ=MONTHLY;BYDAY=1MO"
                spellcheck="false"
                class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text font-mono"
              />
            {/if}
            {#if editRRule}
              <p class="text-[10px] text-dim font-mono"><span class="text-secondary">→</span> {editRRule}</p>
            {/if}
          {/if}
          <!-- Project link picker — drives the calendar's project
               filter + colour-by-project overlay. Hidden when no
               projects exist (fresh vault); ICS events get it too
               so a writable .ics calendar can carry project links
               via the events.json sidecar (the link is on the
               native event record, not in the ICS payload). -->
          {#if event?.type === 'event' && projects.length > 0}
            <div class="flex items-center gap-2 flex-wrap">
              <label class="text-[11px] text-dim uppercase tracking-wider" for="ev-edit-project">Project</label>
              <select
                id="ev-edit-project"
                bind:value={editProjectId}
                class="bg-surface0 border border-surface1 rounded px-2 py-1 text-sm text-text"
              >
                <option value="">No project</option>
                {#each projects as p (p.name)}
                  <option value={p.name}>{p.name}</option>
                {/each}
              </select>
            </div>
          {/if}
          <div class="flex items-center gap-2">
            <span class="text-[11px] text-dim uppercase tracking-wider">Color</span>
            {#each colorOptions as c}
              <button
                type="button"
                onclick={() => (editColor = c)}
                aria-label={c}
                class="w-5 h-5 rounded-full border-2 {editColor === c ? 'border-text' : 'border-surface1'}"
                style="background: var(--color-{c === 'red' ? 'error' : c === 'yellow' ? 'warning' : c === 'orange' ? 'accent' : c === 'green' ? 'success' : c === 'blue' ? 'secondary' : c === 'purple' ? 'primary' : 'info'})"
              ></button>
            {/each}
          </div>
          <div class="flex gap-2">
            <button type="submit" disabled={busy} class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded disabled:opacity-50">save</button>
            <button type="button" onclick={() => (editing = false)} class="px-3 py-1.5 text-sm text-subtext hover:text-text">cancel</button>
            <span class="flex-1"></span>
            <button type="button" onclick={deleteEvent} disabled={busy} class="px-3 py-1.5 text-sm text-error hover:bg-error/10 rounded">delete</button>
          </div>
        </form>
      {:else}
      <div class="flex flex-wrap gap-2 pt-2 border-t border-surface1">
        {#if event.taskId}
          <button onclick={toggleDone} disabled={busy} class="px-3 py-1.5 text-sm bg-success/20 text-success rounded hover:bg-success/30 disabled:opacity-50">
            {event.done ? 'mark not done' : 'mark done'}
          </button>
          {#if event.start}
            <button onclick={clearSchedule} disabled={busy} class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1">
              unschedule
            </button>
          {/if}
        {/if}
        {#if editable}
          <button onclick={startEdit} class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1">edit</button>
          {#if event.type === 'event' && event.rrule}
            <!-- Skip THIS occurrence only — adds an EXDATE so the
                 expander filters this single instance from future
                 renders. Series stays intact. The text reads as a
                 distinct verb from 'delete' so the user's mental
                 model of cancel-once vs end-series stays clear. -->
            <button
              onclick={skipOccurrence}
              disabled={busy}
              class="px-3 py-1.5 text-sm bg-warning/15 text-warning rounded hover:bg-warning/25"
              title="Cancel just this occurrence — keep the rest of the series"
            >skip this</button>
          {/if}
          {#if event.type === 'event' && event.override_key}
            <!-- This occurrence has a per-instance override (set via
                 drag-move or the 'just this' edit scope). One-click
                 to drop the override and surface the series default
                 again. Hidden when override_key is empty (plain
                 occurrence or non-recurring event) so the action
                 row doesn't grow buttons that wouldn't do anything. -->
            <button
              onclick={resetOccurrence}
              disabled={busy}
              class="px-3 py-1.5 text-sm bg-info/15 text-info rounded hover:bg-info/25"
              title="Drop the per-occurrence override and inherit the series defaults"
            >reset this</button>
          {/if}
          <button onclick={deleteEvent} disabled={busy} class="px-3 py-1.5 text-sm text-error hover:bg-error/10 rounded">
            {event.type === 'event' && event.rrule ? 'delete series' : 'delete'}
          </button>
        {/if}
        {#if event.notePath}
          <button onclick={openNote} class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1">
            open note
          </button>
        {/if}
        <button
          onclick={createMeetingNote}
          disabled={creatingMeetingNote}
          class="px-3 py-1.5 text-sm bg-secondary/15 text-secondary rounded hover:bg-secondary/25 disabled:opacity-50"
          title="Create a meeting note for this event with frontmatter"
        >
          {creatingMeetingNote ? 'creating…' : '✎ meeting note'}
        </button>
        <span class="flex-1"></span>
        <button onclick={close} class="px-3 py-1.5 text-sm text-subtext hover:text-text">close</button>
      </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  /* Native <select> dropdown panel is rendered with OS chrome and
     defaults to white-on-white in dark mode. Same fix as
     UnifiedCreate: hint color-scheme + explicit option colors so
     the dropdown is readable on every browser/OS. */
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
