<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, type CalendarEvent, type CalendarSource, type Project, type EventStatus, todayISO } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { onMount } from 'svelte';
  import { eventStartDate, eventEndDate, fmtTime, eventTypeColor } from './utils';
  import { findEventType } from './eventTypes';
  import TimeInput from './TimeInput.svelte';
  import RecurrenceEditor from './RecurrenceEditor.svelte';
  import EventTypeChips from './EventTypeChips.svelte';
  import RecurringScopePicker from './RecurringScopePicker.svelte';
  import ContentPanel from './ContentPanel.svelte';

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
  // Event type — same catalog as CreateEvent. Empty string means
  // "generic / no type", matching the storage convention.
  let editKind = $state('');
  let editProjectId = $state('');
  // Content-pipeline edit state. Only meaningful when
  // editKind === 'content'; the ContentPanel is rendered conditionally
  // and the patchEvent call sends these fields unconditionally so
  // clearing a status / channel / tag persists rather than being
  // dropped by an omitempty-style round-trip.
  let editStatus = $state<EventStatus | ''>('');
  let editChannels = $state<string[]>([]);
  let editTags = $state<string[]>([]);

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

  // 24-hour HH:MM picker buffers — bindable strings owned here and
  // forwarded to the shared TimeInput component (paired HH+MM selects
  // — native <input type="time"> renders AM/PM on most OS locales
  // regardless of any lang attribute). Strings are read synchronously
  // by the submit handler, avoiding the $effect-driven flush race
  // the user previously reported ("editing the times also not, they
  // get scheduled somewhere").
  let editStartTime = $state('00:00');
  let editEndTime = $state('00:00');

  // Snapshot of date/time fields when edit-mode opens. Used by the
  // save path to detect "user did NOT change time" so we can skip
  // sending start/end to the ICS PATCH endpoint — which preserves
  // floating-time events as floating (the writer emits ZONED UTC
  // for everything it writes, so any round-trip through the wire
  // converts floating → UTC and the displayed wall-clock drifts
  // by the user's offset). Pre-fix the user reported "events shown
  // on wrong times" after renaming a floating-time event with no
  // time change.
  let origEditDate = $state('');
  let origEditStartTime = $state('00:00');
  let origEditEndTime = $state('00:00');

  // ── Recurrence edit state ──────────────────────────────────────
  // RRULE editing handled by the shared RecurrenceEditor component;
  // we only carry the bindable string. The component parses on
  // open (seedFromRRule) and serialises back into editRRule on
  // every internal flip.
  let editRRule = $state('');
  // For recurring events, edit-scope is a per-modal toggle:
  // 'series' rewrites the parent (date / time / rrule all shift),
  // 'instance' writes a per-occurrence override so only the open
  // occurrence changes. Defaults to 'instance' on open — safer
  // because editing one Tuesday rarely should touch every Tuesday.
  // Hidden for ICS events (no override path) and for non-recurring
  // events.
  let editScope = $state<'instance' | 'series'>('instance');

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
  //
  // Source of truth: the calendar feed now stamps each ICS event
  // with editable=true/false directly (based on the file's location),
  // so we trust event.editable when present. Falls back to a sources
  // lookup for backward compatibility with older feed payloads or
  // events that pre-date the editable flag (e.g. in-memory entries
  // built from API responses that don't echo it). Without this, the
  // user's most common bug was: feed picks the writable copy of a
  // duplicated .ics file but EventDetail's source-lookup finds the
  // read-only one first and disables editing.
  let icsWritable = $derived.by(() => {
    if (event?.type !== 'ics_event') return false;
    if (typeof event.editable === 'boolean') return event.editable;
    if (!event.source) return false;
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
    const pad2 = (n: number) => String(n).padStart(2, '0');
    if (event.start) {
      const sd = new Date(event.start);
      let sh = sd.getHours();
      let sm = Math.round(sd.getMinutes() / 5) * 5;
      if (sm === 60) { sh = (sh + 1) % 24; sm = 0; }
      editStartTime = `${pad2(sh)}:${pad2(sm)}`;
    } else {
      editStartTime = '00:00';
    }
    if (event.end) {
      const ed = new Date(event.end);
      let eh = ed.getHours();
      let em = Math.round(ed.getMinutes() / 5) * 5;
      if (em === 60) { eh = (eh + 1) % 24; em = 0; }
      editEndTime = `${pad2(eh)}:${pad2(em)}`;
    } else {
      editEndTime = '00:00';
    }
    // Seed recurrence editor from the source rule. ICS events also
    // carry rrule but their write path goes through ics-events
    // endpoints which don't accept rrule today — show the rule
    // read-only via the picker but disable Save-as-series for ICS.
    editRRule = event.rrule ?? '';
    // Seed the project link from the event so unchanged saves
    // round-trip. Empty when the event isn't linked.
    editProjectId = event.project_id ?? '';
    editKind = event.kind ?? '';
    // Content-pipeline fields. Always seed (even for non-content
    // events) so a user switching the kind to 'content' mid-edit
    // starts from a clean blank rather than stale state from a
    // prior content event in the same session.
    editStatus = (event.status ?? '') as EventStatus | '';
    editChannels = event.channels ? [...event.channels] : [];
    editTags = event.tags ? [...event.tags] : [];
    // Default scope: this-occurrence-only. Users editing 'this
    // Tuesday' through the modal get the same conservative default
    // as the drag-move flow.
    editScope = 'instance';
    // Snapshot the seeded date+time so the save handler can detect
    // a no-op time change and skip the ICS PATCH start/end fields.
    origEditDate = editDate;
    origEditStartTime = editStartTime;
    origEditEndTime = editEndTime;
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
        // ICS recurring + "this occurrence only": detach the
        // occurrence from the series via EXDATE, then create a new
        // standalone VEVENT in the same .ics file carrying the
        // edited properties. Two sequential calls — backend doesn't
        // bundle them, but the order matters: skip FIRST so the
        // user's rendered grid still shows ONE event for that day
        // (the new one) and not both. If create-event fails after
        // skip, the user sees a hole in the series — surfaced as
        // a clear toast so they can re-add manually.
        if (event.rrule && editScope === 'instance' && event.start) {
          // Pick the date of the occurrence currently shown — the
          // backend turns RFC3339 / date-only into the right EXDATE
          // form. Use the ORIGINAL start (event.start), not the
          // edited time, since EXDATE targets the source anchor.
          const skipDate = event.start;
          try {
            await api.skipICSOccurrence(event.source, event.eventId, skipDate);
          } catch (err) {
            toast.error('Skip failed: ' + errorMessage(err));
            return;
          }
          // Build the replacement VEVENT body using the edited fields.
          // No rrule — this is a one-off occurrence, not a new series.
          const start = utcRFC3339FromLocalParts(editDate, editStartTime || '00:00');
          const end = editEndTime
            ? utcRFC3339FromLocalParts(editDate, editEndTime)
            : undefined;
          try {
            await api.createICSEvent(event.source, {
              summary: editTitle,
              start,
              end,
              location: editLocation,
              kind: editKind || undefined
            });
          } catch (err) {
            toast.error(
              'Standalone create failed (skip went through — the original occurrence is now hidden): ' +
                errorMessage(err)
            );
            return;
          }
        } else {
          // ICS series path: rewrite the base VEVENT.
          //
          // Skip start/end on PATCH when the user didn't touch them.
          // The icswriter always emits UTC-zoned timestamps, so any
          // round-trip of an originally-floating event through the
          // wire converts it to UTC — and a UTC+2 user later reading
          // the same file in UTC+3 sees the wall-clock shifted by an
          // hour. Conditionally omitting unchanged times leaves the
          // floating-ness intact for the very common rename-only edit.
          const timeChanged =
            editDate !== origEditDate ||
            editStartTime !== origEditStartTime ||
            editEndTime !== origEditEndTime;
          const patch: Parameters<typeof api.patchICSEvent>[2] = {
            summary: editTitle,
            location: editLocation,
            // Send kind unconditionally so clearing it (editKind='')
            // sends "" through to the backend and removes the
            // X-GRANIT-KIND line.
            kind: editKind
          };
          if (timeChanged) {
            patch.start = utcRFC3339FromLocalParts(editDate, editStartTime || '00:00');
            if (editEndTime) {
              patch.end = utcRFC3339FromLocalParts(editDate, editEndTime);
            }
          }
          await api.patchICSEvent(event.source, event.eventId, patch);
        }
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
            project_id: editProjectId,
            // Same reasoning for kind — empty must clear the type
            // server-side, not be skipped as "no change".
            kind: editKind,
            // Content-pipeline fields. Sent unconditionally for the
            // same reason as project_id / kind — clearing a status or
            // emptying the channel list must overwrite the on-disk
            // value, not be dropped. The backend's apply() helper
            // tolerates either presence-with-empty (clear) or absence
            // (no change); we want the clear semantics.
            status: editStatus,
            channels: editChannels,
            tags: editTags
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

  // Delete prompt state. Recurring events open an inline "what
  // scope?" picker instead of native confirm() dialogs. The previous
  // flow stacked two confirm()s where the safe path was OK and the
  // destructive series-wide delete sat behind Cancel — a user who
  // second-guessed the operation and clicked Cancel to abort would
  // instead trigger the catastrophic path. The inline picker makes
  // the three outcomes (this one / entire series / abort) explicit
  // buttons with no Cancel-bypass trapdoor.
  let deletePrompt = $state<'none' | 'recurring-native' | 'recurring-ics'>('none');

  async function deleteEvent() {
    if (!event?.eventId) return;
    if (event.rrule && event.type === 'ics_event' && event.source) {
      deletePrompt = 'recurring-ics';
      return;
    }
    // All-day recurring events have event.date but NO event.start —
    // the previous gate required event.start and silently fell
    // through to the nuclear delete-whole-series path. Accept either
    // anchor so daily/weekly all-day series stop getting wiped when
    // the user means "skip just today". exDateKey() already handles
    // both shapes (event.start for timed, event.date for all-day).
    if (event.rrule && event.type === 'event' && event.eventId && (event.start || event.date)) {
      deletePrompt = 'recurring-native';
      return;
    }
    // Non-recurring path — single VEVENT, one confirm + DELETE.
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

  async function confirmDeleteOccurrence() {
    if (!event?.eventId) return;
    busy = true;
    try {
      if (deletePrompt === 'recurring-ics' && event.source) {
        // EXDATE the source series at this occurrence's anchor. For
        // timed events that's event.start; for all-day events the
        // feed emits `date` instead. Backend accepts either form.
        const anchor = event.start ?? event.date;
        if (!anchor) {
          toast.error("Can't identify this occurrence — edit the series instead.");
          return;
        }
        await api.skipICSOccurrence(event.source, event.eventId, anchor);
      } else if (deletePrompt === 'recurring-native') {
        const key = exDateKey();
        if (!key) {
          toast.error("Can't identify this occurrence — edit the series instead.");
          return;
        }
        await api.skipEventOccurrence(event.eventId, key);
      } else {
        return;
      }
      deletePrompt = 'none';
      onChanged?.();
      open = false;
      toast.success('this occurrence skipped · series unchanged');
    } catch (err) {
      toast.error('skip failed: ' + errorMessage(err));
    } finally {
      busy = false;
    }
  }

  async function confirmDeleteSeries() {
    if (!event?.eventId) return;
    busy = true;
    try {
      if (deletePrompt === 'recurring-ics' && event.source) {
        await api.deleteICSEvent(event.source, event.eventId);
      } else if (deletePrompt === 'recurring-native') {
        await api.deleteEvent(event.eventId);
      } else {
        return;
      }
      deletePrompt = 'none';
      onChanged?.();
      open = false;
      toast.success('entire series deleted');
    } catch (err) {
      toast.error('delete failed: ' + errorMessage(err));
    } finally {
      busy = false;
    }
  }

  function cancelDeletePrompt() {
    deletePrompt = 'none';
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

  // Duplicate the event one week later. Common workflow: "repeat
  // last Monday's standup format for next Monday" without setting
  // up a full recurring series. Drops the rrule + override key
  // so the duplicate is a fresh standalone event; keeps title /
  // time / location / kind / project_id.
  //
  // Native events (events.json) → POST /events. ICS events
  // (writable source) → POST /calendars/{source}/events. Read-only
  // ICS sources can't be duplicated through this path; the chip
  // hides for those.
  async function duplicateEvent() {
    if (!event) return;
    busy = true;
    try {
      if (event.type === 'ics_event' && event.source) {
        // Shift the start/end by exactly 7 days. Use the floating
        // wire shape we already accept (RFC3339 or YYYY-MM-DD per
        // parseClientTime); add 7d in UTC ms.
        const advance = 7 * 24 * 60 * 60 * 1000;
        let start: string | undefined;
        let end: string | undefined;
        let allDay: boolean | undefined;
        if (event.start) {
          const s = new Date(event.start);
          s.setTime(s.getTime() + advance);
          start = s.toISOString();
        }
        if (event.end) {
          const e = new Date(event.end);
          e.setTime(e.getTime() + advance);
          end = e.toISOString();
        }
        if (event.date && !event.start) {
          // All-day shape: shift the date string by 7 days. parse
          // YYYY-MM-DD locally so DST doesn't introduce drift.
          const [y, m, d] = event.date.split('-').map(Number);
          const shifted = new Date(y, m - 1, d);
          shifted.setDate(shifted.getDate() + 7);
          const yy = shifted.getFullYear();
          const mm = String(shifted.getMonth() + 1).padStart(2, '0');
          const dd = String(shifted.getDate()).padStart(2, '0');
          start = `${yy}-${mm}-${dd}`;
          allDay = true;
        }
        if (!start) {
          toast.error('Could not derive a start date for the duplicate.');
          return;
        }
        await api.createICSEvent(event.source, {
          summary: event.title,
          start,
          end,
          allDay,
          location: event.location,
          kind: event.kind || undefined
        });
        onChanged?.();
        close();
        toast.success('Duplicated +1 week.');
        return;
      }
      if (event.type === 'event' && event.eventId) {
        if (!event.date) {
          toast.error('Event has no date — cannot duplicate.');
          return;
        }
        const [y, m, d] = event.date.split('-').map(Number);
        const shifted = new Date(y, m - 1, d);
        shifted.setDate(shifted.getDate() + 7);
        const yy = shifted.getFullYear();
        const mm = String(shifted.getMonth() + 1).padStart(2, '0');
        const dd = String(shifted.getDate()).padStart(2, '0');
        const newDate = `${yy}-${mm}-${dd}`;
        await api.createEvent({
          title: event.title,
          date: newDate,
          // The feed surfaces ICS-style start/end on `start`/`end`,
          // but events.json events carry the rendered HH:MM via the
          // feed too — derive from the existing start string when
          // present. For all-day events both stay empty.
          start_time: event.start ? new Date(event.start).toTimeString().slice(0, 5) : undefined,
          end_time: event.end ? new Date(event.end).toTimeString().slice(0, 5) : undefined,
          location: event.location,
          color: event.color,
          kind: event.kind,
          project_id: event.project_id
          // Intentionally drops: rrule (the duplicate is one-off),
          // override_key (the original's per-instance state), reminder
          // (let the user re-set if they want it).
        });
        onChanged?.();
        close();
        toast.success('Duplicated +1 week.');
        return;
      }
      toast.info('This event type cannot be duplicated.');
    } catch (e) {
      toast.error('Duplicate failed: ' + errorMessage(e));
    } finally {
      busy = false;
    }
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
          <div class="text-xs uppercase tracking-wider text-dim flex items-center gap-1.5">
            <span>{event.type.replace('_', ' ')}</span>
            {#if event.kind}
              {@const evType = findEventType(event.kind)}
              {#if evType}
              <span aria-hidden="true">·</span>
              <span
                class="inline-flex items-center gap-1 px-1 py-0.5 text-[10px] font-medium border"
                style:color={`var(--color-${evType.color})`}
                style:border-color={`color-mix(in srgb, var(--color-${evType.color}) 45%, transparent)`}
                style:background={`color-mix(in srgb, var(--color-${evType.color}) 12%, transparent)`}
                title={evType.description}
              >
                <span class="font-mono">{evType.glyph}</span>
                <span>{evType.label}</span>
              </span>
              {/if}
            {/if}
          </div>
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
              class="inline-flex items-center gap-1 text-xs px-2 py-0.5 mt-2 rounded-full bg-surface1 text-secondary border border-surface2 hover:bg-surface2"
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
          {#if event.kind === 'content' && (event.status || (event.channels?.length ?? 0) > 0 || (event.tags?.length ?? 0) > 0)}
            <!-- Read-only content panel for the view-mode card. Status
                 + channels + tags only render when at least one is set
                 so a content event with no metadata yet stays clean. -->
            <div class="mt-2 flex flex-wrap items-center gap-1.5 text-[11px]">
              {#if event.status}
                <span class="px-1.5 py-0.5 rounded border border-lavender/40 bg-lavender/15 text-lavender uppercase tracking-wider text-[10px] font-semibold">{event.status}</span>
              {/if}
              {#each event.channels ?? [] as ch (ch)}
                <span class="px-1.5 py-0.5 rounded bg-surface1 text-subtext">{ch}</span>
              {/each}
              {#each event.tags ?? [] as t (t)}
                <span class="px-1.5 py-0.5 rounded bg-surface0 text-dim">#{t}</span>
              {/each}
            </div>
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
          <!-- 24-hour HH:MM picker — shared TimeInput keeps the same
               paired-select markup. Native <input type="time"> respects
               the OS locale, not the element's lang, so a US-locale
               user saw AM/PM on every event edit. The selects always
               show 24-hour values. step=5 matches the seed rounding
               in startEdit so the bound value lines up with an option. -->
          <TimeInput bind:startTime={editStartTime} bind:endTime={editEndTime} step={5} />
          <input bind:value={editLocation} placeholder="location (optional)" class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <!-- Edit scope picker — only relevant for recurring NATIVE
               events. 'this' writes a per-occurrence override (title /
               time / date / location / color of just this one); 'series'
               rewrites the parent so every occurrence shifts. ICS gets
               no scope picker because the patch endpoint has no override
               slot. Defaults to 'this' on open — same conservative
               default as the drag-move flow. -->
          {#if event?.type === 'event' && event?.rrule}
            <fieldset class="border border-surface1 p-2 space-y-1">
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
          <!-- ICS recurring scope picker. ICS events have no
               first-class override path in our schema, but we can
               approximate "this occurrence only" by EXDATE'ing the
               source series and creating a standalone replacement
               VEVENT in the same .ics file — same observable result
               from the user's perspective. The series option keeps
               the existing path (rewrite the base VEVENT). -->
          {#if event?.type === 'ics_event' && event?.rrule && icsWritable}
            <fieldset class="border border-surface1 p-2 space-y-1">
              <legend class="text-[10px] uppercase tracking-wider text-dim px-1">Apply to · ICS</legend>
              <label class="flex items-center gap-2 text-xs text-text cursor-pointer">
                <input type="radio" bind:group={editScope} value="instance" name="ev-edit-scope-ics" />
                <span>Just this occurrence</span>
                <span class="text-[10px] text-dim">— EXDATE + new standalone VEVENT</span>
              </label>
              <label class="flex items-center gap-2 text-xs text-text cursor-pointer">
                <input type="radio" bind:group={editScope} value="series" name="ev-edit-scope-ics" />
                <span>The entire series</span>
                <span class="text-[10px] text-dim">— rewrites the base VEVENT</span>
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
            <RecurrenceEditor
              bind:rrule={editRRule}
              layout="inline"
              minDate={editDate}
              idPrefix="ev-edit"
            />
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
          <!-- Event-type picker. Same catalog + chip shape as
               CreateEvent so the muscle memory is identical. Empty
               state = no type; clicking the active chip clears it. -->
          <div>
            <span class="block text-[11px] uppercase tracking-wider text-dim mb-1.5">Type</span>
            <EventTypeChips bind:kind={editKind} chipSize="compact" />
          </div>
          <!-- Content-pipeline panel — only when the user has selected
               the 'content' kind. Status / channels / tags live here.
               Switching kind away hides the panel; the values stay in
               local state so flipping back-and-forth doesn't lose work
               mid-edit. -->
          {#if editKind === 'content'}
            <ContentPanel
              status={editStatus}
              channels={editChannels}
              tags={editTags}
              onStatusChange={(s) => (editStatus = s)}
              onChannelsChange={(c) => (editChannels = c)}
              onTagsChange={(t) => (editTags = t)}
            />
          {/if}
          <div class="flex items-center gap-2">
            <span class="text-[11px] text-dim uppercase tracking-wider">Color</span>
            {#each colorOptions as c (c.name)}
              <button
                type="button"
                onclick={() => (editColor = c.name)}
                aria-label={c.name}
                title={c.name}
                class="w-5 h-5 rounded-full border-2 {editColor === c.name ? 'border-text' : 'border-surface1'}"
                style="background: {c.hex}"
              ></button>
            {/each}
          </div>
          <div class="flex gap-2">
            <button type="submit" disabled={busy} class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded disabled:opacity-50">save</button>
            <button type="button" onclick={() => (editing = false)} class="px-3 py-1.5 text-sm text-subtext hover:text-text">cancel</button>
            <span class="flex-1"></span>
            <button type="button" onclick={deleteEvent} disabled={busy} class="px-3 py-1.5 text-sm text-error hover:bg-surface0 rounded">delete</button>
          </div>
        </form>
      {:else}
      <!-- Inline delete-scope picker. Replaces the previous two-confirm
           pattern where the destructive 'delete entire series' branch
           sat behind a Cancel keystroke — too easy to trigger by
           reflexively pressing Esc/Cancel to abort. Three explicit
           buttons; nothing happens until one is clicked. -->
      {#if deletePrompt !== 'none'}
        <RecurringScopePicker
          eventTitle={event.title}
          action="delete"
          onChoose={(scope) => {
            if (scope === 'this') void confirmDeleteOccurrence();
            else if (scope === 'series') void confirmDeleteSeries();
          }}
          onCancel={cancelDeletePrompt}
          {busy}
        />
      {/if}
      <div class="flex flex-wrap gap-2 pt-2 border-t border-surface1" class:opacity-40={deletePrompt !== 'none'}>
        {#if event.taskId}
          <button onclick={toggleDone} disabled={busy} class="px-3 py-1.5 text-sm bg-surface0 text-success rounded hover:bg-surface1 disabled:opacity-50">
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
          <!-- Duplicate the event +1 week ahead — common "repeat
               last week's structure for next week" workflow without
               needing to set up a full RRULE. Hidden for tasks /
               deadlines / read-only ICS sources via `editable`. -->
          <button
            onclick={duplicateEvent}
            disabled={busy}
            class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1"
            title="Create a copy of this event one week from now"
          >+1 week</button>
          {#if event.type === 'event' && event.rrule}
            <!-- Skip THIS occurrence only — adds an EXDATE so the
                 expander filters this single instance from future
                 renders. Series stays intact. The text reads as a
                 distinct verb from 'delete' so the user's mental
                 model of cancel-once vs end-series stays clear. -->
            <button
              onclick={skipOccurrence}
              disabled={busy}
              class="px-3 py-1.5 text-sm bg-surface0 text-warning rounded hover:bg-surface1"
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
              class="px-3 py-1.5 text-sm bg-surface0 text-info rounded hover:bg-surface1"
              title="Drop the per-occurrence override and inherit the series defaults"
            >reset this</button>
          {/if}
          <button
            onclick={deleteEvent}
            disabled={busy || deletePrompt !== 'none'}
            class="px-3 py-1.5 text-sm text-error hover:bg-surface0 rounded disabled:opacity-50"
            title={event.rrule ? 'Pick scope: this occurrence or the entire series' : 'Delete this event'}
          >{event.rrule ? 'delete…' : 'delete'}</button>
        {/if}
        {#if event.notePath}
          <button onclick={openNote} class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1">
            open note
          </button>
        {/if}
        <button
          onclick={createMeetingNote}
          disabled={creatingMeetingNote}
          class="px-3 py-1.5 text-sm bg-surface1 text-secondary rounded hover:bg-surface2 disabled:opacity-50"
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

