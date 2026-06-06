// Calendar event-mutation dispatch — move / resize / drop-task /
// reschedule.
//
// Each function receives the full event (or task id + new time) and
// routes to the right patch endpoint by event kind:
//
//   move    — events.json (patchEvent), writable ICS (patchICSEvent),
//             tasks (patchTask via reschedule path; called separately).
//             Recurring events go through askRecurringScope first;
//             the user picks "this occurrence" → per-instance override
//             (events.json) or skip + create-standalone (ICS), or
//             "the whole series" → rewrite the base.
//
//   resize  — same branching shape as move but only end_time / end
//             change. Tasks → patchTask({durationMinutes}).
//
//   dropTask     — patchTask({scheduledStart, durationMinutes}) for a
//                  fresh schedule landing on the grid.
//   reschedule   — patchTask({scheduledStart}) only; keeps existing
//                  duration. Used when the user drags an already-
//                  scheduled task to a new time.
//
// Every successful path ends with dataCtl.load() so the optimistic
// drag visual reconciles with the server's authoritative state. On
// failure the load also fires so the bar/card snaps back to where it
// was before the user reached for it.

import { goto } from '$app/navigation';
import { api, todayISO, type CalendarEvent } from '$lib/api';
import { errorMessage } from '$lib/util/errorMessage';
import { toast } from '$lib/components/toast';
import type { CalendarDataController } from './calendarData.svelte';
import type { RecurringScope, RecurringAction } from './calendarRecurringScope.svelte';

export interface CalendarEventMutationsDeps {
  dataCtl: CalendarDataController;
  /** From calendarRecurringScope — resolves with the user's pick or
   *  null on cancel. */
  askRecurringScope: (title: string, action: RecurringAction) => Promise<RecurringScope | null>;
}

export interface CalendarEventMutationsController {
  moveEvent(ev: CalendarEvent, newStart: Date): Promise<void>;
  resizeEvent(ev: CalendarEvent, durationMinutes: number): Promise<void>;
  dropTask(id: string, start: Date, dur: number): Promise<void>;
  reschedule(taskId: string, newStart: Date): Promise<void>;
}

// Format helper for success toasts.
function fmt(d: Date): string {
  return (
    d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' }) +
    ' ' +
    String(d.getHours()).padStart(2, '0') +
    ':' +
    String(d.getMinutes()).padStart(2, '0')
  );
}

// Local-date helper that matches the events.json `date` field shape.
function localDateStr(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function hhmm(d: Date): string {
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

export function createCalendarEventMutations(
  deps: CalendarEventMutationsDeps
): CalendarEventMutationsController {
  const { dataCtl, askRecurringScope } = deps;

  async function moveEvent(ev: CalendarEvent, newStart: Date) {
    try {
      // Surface a clear toast when the user drag-released an event we
      // can't actually patch — the most common cause is a legacy
      // events.json entry without an ID (created before the ID-mint
      // path was added) or an ICS event whose source isn't writable.
      // Without this, the drag visually "works" (ghost moves with the
      // pointer) but the event snaps back on release with no feedback.
      if (!ev.eventId && !ev.taskId) {
        toast.error("This event is missing an ID and can't be moved. Try editing it in the detail view to mint one.");
        return;
      }
      if (ev.type === 'ics_event' && ev.source) {
        // ICS-specific gate: even if eventId is set, the source must
        // be writable. The HourGrid filters this in isMovable, but a
        // stale writableSources prop can let a drag fire that the
        // server then rejects with 403. Catch it here with a clear
        // message instead of a generic patchICSEvent failure toast.
        //
        // Prefer the server-stamped event.editable flag — it tracks
        // the actual file's location at feed time and survives
        // duplicate-filename scenarios where two .ics files share a
        // name but only one is writable. Falls back to dataCtl.calSources
        // lookup for legacy entries that pre-date the flag.
        const w =
          typeof ev.editable === 'boolean'
            ? ev.editable
            : !!dataCtl.calSources.find((s) => s.source === ev.source)?.writable;
        if (!w) {
          toast.error(`Read-only calendar (${ev.source}) — move the file to <vault>/calendars/ to enable edits.`);
          return;
        }
      }
      // Recurring-series UX: a recurring event has TWO valid drag
      // semantics — "this occurrence only" (per-instance override) or
      // "the whole series" (rewrite the base). The picker resolves to
      // 'this' | 'series' | null; null = user cancelled, so we abort
      // and let the drag visually revert via dataCtl.load().
      let recurringMode: 'series' | 'instance' | null = null;
      if (ev.rrule && ev.eventId) {
        const scope = await askRecurringScope(ev.title ?? 'this event', 'move');
        if (!scope) {
          await dataCtl.load();
          return;
        }
        recurringMode = scope === 'this' ? 'instance' : 'series';
      }
      // ICS "just this one" — fire skip + create-standalone in the
      // same order EventDetail uses. Falls through on failure with a
      // toast so the user can retry; the source occurrence is only
      // EXDATE'd AFTER the standalone create succeeds wouldn't be
      // safer here because the user already accepted the move, and a
      // partial state (extra event, no skip) leaves both visible —
      // less confusing than (skip done, no replacement) which leaves
      // a hole.
      if (recurringMode === 'instance' && ev.type === 'ics_event' && ev.eventId && ev.source && ev.start) {
        try {
          // Skip the ORIGINAL anchor date so the series no longer
          // renders the occurrence the user dragged.
          await api.skipICSOccurrence(ev.source, ev.eventId, ev.start);
        } catch (e) {
          toast.error('Move (skip) failed: ' + errorMessage(e));
          return;
        }
        const dur = ev.durationMinutes ?? 60;
        const endD = new Date(newStart.getTime() + dur * 60_000);
        try {
          await api.createICSEvent(ev.source, {
            summary: ev.title,
            start: newStart.toISOString(),
            end: endD.toISOString(),
            location: ev.location ?? undefined
          });
        } catch (e) {
          toast.error(
            'Move (create standalone) failed — the original occurrence is now hidden: ' + errorMessage(e)
          );
          return;
        }
        await dataCtl.load();
        toast.success(`Moved this occurrence to ${fmt(newStart)}`);
        return;
      }
      // Per-instance override path: write a single override entry
      // keyed by the occurrence's UTC ANCHOR (the series-base time
      // for this occurrence, NOT the currently rendered time). When
      // an override is already active, `ev.start` reflects the
      // overridden time, so re-deriving the key from `ev.start`
      // would mint a fresh override at the wrong anchor and the
      // original override would stay buried in the map. Use
      // ev.override_key when present (canonical anchor surfaced by
      // the calendar feed); fall back to ev.start for first-time
      // overrides where the rendered time IS the anchor.
      if (recurringMode === 'instance' && ev.type === 'event' && ev.eventId && ev.start) {
        const dateStr = localDateStr(newStart);
        const startTime = hhmm(newStart);
        const dur = ev.durationMinutes ?? 30;
        const startMinOnly = newStart.getHours() * 60 + newStart.getMinutes();
        const maxEndMinOnly = 24 * 60 - 1;
        const endMin = Math.min(startMinOnly + dur, maxEndMinOnly);
        const endTime = `${String(Math.floor(endMin / 60)).padStart(2, '0')}:${String(endMin % 60).padStart(2, '0')}`;
        // Slice the floating ISO directly instead of round-tripping
        // through Date+toISOString. The backend emits start/end as
        // floating wall-clock ("2026-05-09T08:00:00", no Z, no
        // offset) and keys overrides by the same wall-clock digits.
        // new Date(...).toISOString() would re-anchor those digits
        // to the client zone and emit a UTC-shifted key — on a
        // UTC+2 client the key would land 2hr ahead of the anchor,
        // and the override would silently mint at the wrong slot.
        // The leading 19 chars of `ev.start` always carry the
        // YYYY-MM-DDTHH:MM:SS shape the server expects.
        const key = ev.override_key ?? ev.start.slice(0, 19);
        await api.overrideEventOccurrence(ev.eventId, key, {
          date: dateStr,
          start_time: startTime,
          end_time: endTime
        });
        await dataCtl.load();
        toast.success(`Moved this occurrence to ${fmt(newStart)}`);
        return;
      }
      if (ev.type === 'event' && ev.eventId) {
        const dateStr = localDateStr(newStart);
        const startTime = hhmm(newStart);
        // Preserve duration: take the event's old duration in minutes,
        // add to the new start to compute the new end. The event used
        // to span 14:30-16:00; dragging to 09:15 should produce
        // 09:15-10:45, not collapse to a zero-length event.
        //
        // Midnight clamp: events.json carries one `date` plus HH:MM
        // start/end strings — the schema can't represent an event whose
        // end falls on the next calendar day. Without this clamp,
        // dragging a 60-min event to 23:30 would emit end_time="00:30",
        // which the backend's validateEventTimes refuses ("end_time
        // must be after start_time"); the move looked successful on the
        // grid, then reverted on reload — exactly the "places it
        // somewhere else" symptom the user reported. Clamp to 23:59 so
        // the move always lands; the user can extend it manually if
        // they want a true cross-midnight event (today not supported).
        const dur = ev.durationMinutes ?? 30;
        const startMin = newStart.getHours() * 60 + newStart.getMinutes();
        const maxEndMin = 24 * 60 - 1;
        const endMin = Math.min(startMin + dur, maxEndMin);
        const endTime = `${String(Math.floor(endMin / 60)).padStart(2, '0')}:${String(endMin % 60).padStart(2, '0')}`;
        await api.patchEvent(ev.eventId, { date: dateStr, start_time: startTime, end_time: endTime });
      } else if (ev.type === 'ics_event' && ev.eventId && ev.source) {
        const dur = ev.durationMinutes ?? 60;
        const endD = new Date(newStart.getTime() + dur * 60_000);
        await api.patchICSEvent(ev.source, ev.eventId, {
          start: newStart.toISOString(),
          end: endD.toISOString()
        });
      } else {
        // The event doesn't match any of the known dispatch branches
        // (event / ics_event / task). Surface so the user doesn't see
        // a silent failure.
        toast.error(`Can't move this event type (${ev.type ?? 'unknown'}).`);
        return;
      }
      await dataCtl.load();
      toast.success(`Moved to ${fmt(newStart)}`);
    } catch (e) {
      toast.error('Move failed: ' + errorMessage(e));
    }
  }

  async function resizeEvent(ev: CalendarEvent, durationMinutes: number) {
    try {
      // Mirrors moveEvent's recurring chooser. null = user cancelled —
      // dataCtl.load() snaps the resize visual back to its original
      // duration.
      let recurringMode: 'series' | 'instance' | null = null;
      if (ev.rrule && !ev.taskId && ev.eventId) {
        const scope = await askRecurringScope(ev.title ?? 'this event', 'resize');
        if (!scope) {
          await dataCtl.load();
          return;
        }
        recurringMode = scope === 'this' ? 'instance' : 'series';
      }
      // ICS "just this one" resize — skip + create-standalone with the
      // edited duration. Same failure ordering as moveEvent.
      if (recurringMode === 'instance' && ev.type === 'ics_event' && ev.eventId && ev.source && ev.start) {
        try {
          await api.skipICSOccurrence(ev.source, ev.eventId, ev.start);
        } catch (e) {
          toast.error('Resize (skip) failed: ' + errorMessage(e));
          return;
        }
        const startD = new Date(ev.start);
        const endD = new Date(startD.getTime() + durationMinutes * 60_000);
        try {
          await api.createICSEvent(ev.source, {
            summary: ev.title,
            start: startD.toISOString(),
            end: endD.toISOString(),
            location: ev.location ?? undefined
          });
        } catch (e) {
          toast.error(
            'Resize (create standalone) failed — the original occurrence is now hidden: ' + errorMessage(e)
          );
          return;
        }
        await dataCtl.load();
        return;
      }
      if (recurringMode === 'instance' && ev.type === 'event' && ev.eventId && ev.start) {
        const startD = new Date(ev.start);
        const startMin = startD.getHours() * 60 + startD.getMinutes();
        const maxEndMin = 24 * 60 - 1;
        const endMin = Math.min(startMin + durationMinutes, maxEndMin);
        const endTime = `${String(Math.floor(endMin / 60)).padStart(2, '0')}:${String(endMin % 60).padStart(2, '0')}`;
        // See moveEvent: prefer the surfaced override_key over re-
        // deriving from ev.start so we don't mint a fresh override at
        // an already-overridden time. Slice the floating ISO directly
        // instead of new Date(...).toISOString() — see the long-form
        // note in moveEvent for why round-tripping through Date
        // silently shifts the key by the client offset.
        const key = ev.override_key ?? ev.start.slice(0, 19);
        // Resize keeps the start_time on the original occurrence date
        // unchanged — only end_time shifts. We still send start_time so
        // the override carries a complete (start, end) pair and the
        // expander doesn't have to merge with the series.
        const startTime = hhmm(startD);
        await api.overrideEventOccurrence(ev.eventId, key, {
          start_time: startTime,
          end_time: endTime
        });
        await dataCtl.load();
        return;
      }
      if (ev.taskId) {
        await api.patchTask(ev.taskId, { durationMinutes });
      } else if (ev.type === 'event' && ev.eventId && ev.start) {
        // events.json is keyed on date + HH:MM strings. The schema
        // can't represent an event ending on the next calendar day, so
        // a resize that would push end past 23:59 must clamp. Without
        // the clamp, the backend's validateEventTimes refuses
        // ("end_time must be after start_time", string compare on
        // HH:MM) and the resize silently reverts — that's part of the
        // "drag make it longer ... places it somewhere else" report.
        const startD = new Date(ev.start);
        const startMin = startD.getHours() * 60 + startD.getMinutes();
        const maxEndMin = 24 * 60 - 1;
        const endMin = Math.min(startMin + durationMinutes, maxEndMin);
        const endTime = `${String(Math.floor(endMin / 60)).padStart(2, '0')}:${String(endMin % 60).padStart(2, '0')}`;
        await api.patchEvent(ev.eventId, { end_time: endTime });
      } else if (ev.type === 'ics_event' && ev.eventId && ev.source && ev.start) {
        // Same editable gate as moveEvent — read-only sources can't be
        // resized either. Prefer the server-stamped flag.
        const w =
          typeof ev.editable === 'boolean'
            ? ev.editable
            : !!dataCtl.calSources.find((s) => s.source === ev.source)?.writable;
        if (!w) {
          toast.error(`Read-only calendar (${ev.source}) — can't resize this event.`);
          return;
        }
        // ICS uses RFC3339 — full timestamps, so cross-midnight is
        // representable. No clamp needed; the writer will normalize to
        // UTC on emit.
        const startD = new Date(ev.start);
        const endD = new Date(startD.getTime() + durationMinutes * 60_000);
        await api.patchICSEvent(ev.source, ev.eventId, { end: endD.toISOString() });
      }
      await dataCtl.load();
    } catch (e) {
      // Surface a toast so the user sees the resize failed instead of
      // watching the bar snap back silently. Mirrors moveEvent's error
      // path — every drag gesture should give clear feedback on
      // outcome.
      toast.error('Resize failed: ' + errorMessage(e));
    }
  }

  async function dropTask(id: string, start: Date, dur: number) {
    try {
      await api.patchTask(id, {
        scheduledStart: start.toISOString(),
        durationMinutes: dur
      });
      await dataCtl.load();
      toast.success('scheduled');
    } catch (e) {
      toast.error('schedule failed: ' + errorMessage(e));
    }
  }

  async function reschedule(taskId: string, newStart: Date) {
    try {
      await api.patchTask(taskId, { scheduledStart: newStart.toISOString() });
      await dataCtl.load();
      toast.success(`Rescheduled to ${fmt(newStart)}`);
    } catch (e) {
      toast.error('Reschedule failed: ' + errorMessage(e));
    }
  }

  return { moveEvent, resizeEvent, dropTask, reschedule };
}

// ─────────────────────────────────────────────────────────────────
// EventDetail-side dispatchers — append-only additions for the
// "Duplicate +1 week" and "Create meeting note" actions in the
// detail modal. They live here next to the grid-side mutations
// above so all "modify a calendar event from anywhere in the UI"
// glue is in one file.
//

// Pure-ts dispatchers for the bigger "do something with an event"
// side effects that EventDetail.svelte previously inlined. No runes
// here — callers own busy / open / onChanged state and pass the
// event in. Each function returns true on success so the caller can
// decide whether to close the modal + fire onChanged.
//
// Two concerns live here today:
//   1. duplicateEventPlusOneWeek — shifts an event 7 days forward
//      and writes it back as a fresh standalone (no rrule, no
//      override_key, no reminder).
//   2. createMeetingNoteForEvent — generates a Meetings/<date> ·
//      <slug>.md note with frontmatter capturing event metadata,
//      and appends a backlink to today's daily when relevant.
//
// Future passes can land patchEventDispatch / deleteEventDispatch /
// skipOccurrenceDispatch here too — the goal is one obvious place
// for "the modal asked the backend to do X" so the view layer just
// owns form state + button wiring.

/** Duplicate the event one week later. Common workflow: "repeat
 *  last Monday's standup format for next Monday" without setting
 *  up a full recurring series. Drops the rrule + override key
 *  so the duplicate is a fresh standalone event; keeps title /
 *  time / location / kind / project_id.
 *
 *  Native events (events.json) → POST /events. ICS events
 *  (writable source) → POST /calendars/{source}/events. Read-only
 *  ICS sources can't be duplicated through this path; the chip
 *  hides for those.
 *
 *  Returns true on success, false on any failure or skip-path. The
 *  caller decides whether to close the modal + fire onChanged. */
export async function duplicateEventPlusOneWeek(event: CalendarEvent | null): Promise<boolean> {
  if (!event) return false;
  try {
    if (event.type === 'ics_event' && event.source) {
      // Shift the start/end by exactly 7 calendar days. Use local
      // setDate (NOT +ms) so a duplicate across a DST boundary
      // lands at the same wall-clock time the user expects — adding
      // 7×86400000ms shifts the wall clock by ±1h whenever the
      // interval crosses spring-forward or fall-back. The native-
      // events branch below already does this correctly; this is
      // the matching fix for the ICS branch.
      let start: string | undefined;
      let end: string | undefined;
      let allDay: boolean | undefined;
      if (event.start) {
        const s = new Date(event.start);
        s.setDate(s.getDate() + 7);
        start = s.toISOString();
      }
      if (event.end) {
        const e = new Date(event.end);
        e.setDate(e.getDate() + 7);
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
        return false;
      }
      await api.createICSEvent(event.source, {
        summary: event.title,
        start,
        end,
        allDay,
        location: event.location,
        kind: event.kind || undefined
      });
      toast.success('Duplicated +1 week.');
      return true;
    }
    if (event.type === 'event' && event.eventId) {
      if (!event.date) {
        toast.error('Event has no date — cannot duplicate.');
        return false;
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
      toast.success('Duplicated +1 week.');
      return true;
    }
    toast.info('This event type cannot be duplicated.');
    return false;
  } catch (e) {
    toast.error('Duplicate failed: ' + errorMessage(e));
    return false;
  }
}

/** Create a meeting note for this event and navigate to it. The
 *  note lands at Meetings/<YYYY-MM-DD> · <slug-of-title>.md with
 *  frontmatter that captures the event metadata so the note is
 *  searchable + tag-filterable + later linkable from the daily.
 *  If today's daily exists we also append a backlink line so the
 *  user has a one-click trail from "what did I do today" to the
 *  meeting note. Failures fall back to a toast.
 *
 *  Returns true on success (caller closes the modal); false on
 *  failure (modal stays open so the user can retry). */
export async function createMeetingNoteForEvent(event: CalendarEvent | null): Promise<boolean> {
  if (!event) return false;
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
    return true;
  } catch (err) {
    toast.error('Failed to create meeting note: ' + errorMessage(err));
    return false;
  }
}
