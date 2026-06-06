// Edit-form controller for EventDetail.svelte.
//
// Owns the entire edit buffer state — title, date, location, color,
// kind, project link, content-pipeline fields, time pickers,
// recurrence rule, and the per-modal edit scope. Exposes one
// startEdit() to seed from the live event, and one saveEdit() that
// dispatches the right write path:
//
//   - ICS event + recurring + scope=instance
//       -> skipICSOccurrence (EXDATE) THEN createICSEvent (standalone)
//       Two sequential calls. Order matters: skip first so the
//       rendered grid shows ONE event for that day (the new one) and
//       not both. If create-event fails after skip, surface a clear
//       toast so the user can re-add manually.
//   - ICS event + series (or non-recurring)
//       -> patchICSEvent. Skip start/end on PATCH when the user
//       didn't touch them — the icswriter always emits UTC-zoned
//       timestamps, so any round-trip of an originally-floating
//       event converts it to UTC and a different-timezone reader
//       sees the wall-clock drift. Conditionally omitting unchanged
//       times preserves floating-ness for the very common rename-only edit.
//   - native event + recurring + scope=instance
//       -> overrideEventOccurrence at the anchor (exDateKey). Series
//       base + rrule untouched, so editing a single Tuesday doesn't
//       shift every Tuesday. Carries title/location/color too.
//   - native event + series (or non-recurring)
//       -> patchEvent. Sends rrule / project_id / kind / status /
//       channels / tags UNCONDITIONALLY so clearing a field (e.g.
//       editProjectId='') overwrites the on-disk value rather than
//       being silently dropped by an omitempty round-trip.
//
// 24-hour HH:MM strings are owned here as bindable state; the modal's
// TimeInput component binds directly to ctl.editStartTime /
// ctl.editEndTime. Strings are read synchronously by the submit
// handler — avoids the $effect-driven flush race that earlier
// surfaced as "editing the times also not, they get scheduled
// somewhere".

import { api, type CalendarEvent, type EventStatus } from '$lib/api';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';
import { utcRFC3339FromLocalParts, exDateKey } from './eventDetailHelpers';

export type EditScope = 'instance' | 'series';

export interface EventDetailEditController {
  /** True while the inline edit form is open. */
  editing: boolean;

  // Buffer fields — bindable via getter/setter pairs so the modal's
  // <input bind:value={editCtl.editTitle}> works as in plain $state.
  editTitle: string;
  editDate: string;
  editLocation: string;
  editColor: string;
  editKind: string;
  editProjectId: string;
  editStatus: EventStatus | '';
  editChannels: string[];
  editTags: string[];
  editStartTime: string;
  editEndTime: string;
  editRRule: string;
  editScope: EditScope;

  /** Seed buffers from the live event and flip editing -> true. */
  startEdit(): void;
  /** Cancel without saving — flips editing -> false. */
  cancelEdit(): void;
  /** Submit handler — preventDefault + dispatch + close on success. */
  saveEdit(e: SubmitEvent): Promise<void>;
}

export type EventDetailEditDeps = {
  getEvent: () => CalendarEvent | null;
  setBusy: (v: boolean) => void;
  close: () => void;
  onChanged?: () => void;
};

export function createEventDetailEdit(deps: EventDetailEditDeps): EventDetailEditController {
  let editing = $state(false);
  let editTitle = $state('');
  let editDate = $state('');
  let editLocation = $state('');
  let editColor = $state('cyan');
  let editKind = $state('');
  let editProjectId = $state('');
  let editStatus = $state<EventStatus | ''>('');
  let editChannels = $state<string[]>([]);
  let editTags = $state<string[]>([]);

  // 24-hour HH:MM picker buffers — paired with the shared TimeInput
  // component. Native <input type="time"> renders AM/PM on most OS
  // locales regardless of any lang attribute; the TimeInput's paired
  // selects sidestep that.
  let editStartTime = $state('00:00');
  let editEndTime = $state('00:00');

  // Snapshot of date/time fields when edit-mode opens. Used by the
  // save path to detect "user did NOT change time" so we can skip
  // sending start/end to the ICS PATCH endpoint — preserves
  // floating-time events as floating.
  let origEditDate = $state('');
  let origEditStartTime = $state('00:00');
  let origEditEndTime = $state('00:00');

  let editRRule = $state('');
  let editScope = $state<EditScope>('instance');

  function startEdit() {
    const event = deps.getEvent();
    if (!event) return;
    editTitle = event.title;
    editDate = event.date ?? (event.start ? event.start.slice(0, 10) : '');
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
    editProjectId = event.project_id ?? '';
    editKind = event.kind ?? '';
    // Content-pipeline fields. Always seed (even for non-content
    // events) so a user switching the kind to 'content' mid-edit
    // starts from a clean blank rather than stale state.
    editStatus = (event.status ?? '') as EventStatus | '';
    editChannels = event.channels ? [...event.channels] : [];
    editTags = event.tags ? [...event.tags] : [];
    // Default scope: this-occurrence-only. Same conservative default
    // as the drag-move flow.
    editScope = 'instance';
    // Snapshot the seeded date+time so the save handler can detect
    // a no-op time change and skip the ICS PATCH start/end fields.
    origEditDate = editDate;
    origEditStartTime = editStartTime;
    origEditEndTime = editEndTime;
    editing = true;
  }

  function cancelEdit() {
    editing = false;
  }

  async function saveEdit(e: SubmitEvent) {
    e.preventDefault();
    const event = deps.getEvent();
    if (!event) return;
    deps.setBusy(true);
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
          const key = exDateKey(event);
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
      deps.onChanged?.();
      deps.close();
      toast.success(
        event?.rrule && event.type === 'event' && editScope === 'instance'
          ? 'this occurrence updated'
          : 'event updated'
      );
    } catch (err) {
      toast.error('save failed: ' + errorMessage(err));
    } finally {
      deps.setBusy(false);
    }
  }

  return {
    get editing() { return editing; },
    set editing(v) { editing = v; },
    get editTitle() { return editTitle; },
    set editTitle(v) { editTitle = v; },
    get editDate() { return editDate; },
    set editDate(v) { editDate = v; },
    get editLocation() { return editLocation; },
    set editLocation(v) { editLocation = v; },
    get editColor() { return editColor; },
    set editColor(v) { editColor = v; },
    get editKind() { return editKind; },
    set editKind(v) { editKind = v; },
    get editProjectId() { return editProjectId; },
    set editProjectId(v) { editProjectId = v; },
    get editStatus() { return editStatus; },
    set editStatus(v) { editStatus = v; },
    get editChannels() { return editChannels; },
    set editChannels(v) { editChannels = v; },
    get editTags() { return editTags; },
    set editTags(v) { editTags = v; },
    get editStartTime() { return editStartTime; },
    set editStartTime(v) { editStartTime = v; },
    get editEndTime() { return editEndTime; },
    set editEndTime(v) { editEndTime = v; },
    get editRRule() { return editRRule; },
    set editRRule(v) { editRRule = v; },
    get editScope() { return editScope; },
    set editScope(v) { editScope = v; },
    startEdit,
    cancelEdit,
    saveEdit
  };
}
