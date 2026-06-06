// Delete / skip / reset controller for EventDetail.svelte.
//
// Three observable verbs:
//   - deleteEvent — entry point. Non-recurring goes through a single
//     confirm()+DELETE. Recurring opens the inline scope picker
//     (deletePrompt != 'none') so the user picks "this occurrence" or
//     "the entire series" with explicit buttons. The previous flow
//     stacked two confirm()s with the destructive series-wide branch
//     behind Cancel — too easy to trigger by reflex.
//   - skipOccurrence — one-click "skip just this occurrence" on a
//     native recurring event. ICS gets a toast.info since the ICS skip
//     would need to round-trip the source .ics file (not on the patch
//     path today).
//   - resetOccurrence — drop a per-instance override and surface the
//     series default again. Visible only when event.override_key is set.
//
// The scope picker is driven by deletePrompt ('none' | 'recurring-native'
// | 'recurring-ics'); the modal's RecurringScopePicker reads through
// confirmDeleteOccurrence / confirmDeleteSeries / cancelDeletePrompt.
//
// busy is owned externally so it shares state with the save / duplicate
// flows — only one of those should be in flight at a time, and the
// shared flag drives the global disabled state in the action row.

import { api } from '$lib/api';
import type { CalendarEvent } from '$lib/api';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';
import { exDateKey } from './eventDetailHelpers';

export type DeletePrompt = 'none' | 'recurring-native' | 'recurring-ics';

export interface EventDetailDeleteController {
  /** Which scope picker is open — drives RecurringScopePicker rendering. */
  readonly deletePrompt: DeletePrompt;
  /** Top-level delete entry. Recurring -> opens scope picker; else
   *  fires confirm() + DELETE inline. */
  deleteEvent(): Promise<void>;
  /** Scope picker -> "this occurrence" branch. EXDATE for ICS or
   *  per-occurrence skip for native. */
  confirmDeleteOccurrence(): Promise<void>;
  /** Scope picker -> "entire series" branch. Same DELETE path as
   *  non-recurring, just gated by the explicit user choice. */
  confirmDeleteSeries(): Promise<void>;
  /** Scope picker -> Cancel. Resets state without touching the event. */
  cancelDeletePrompt(): void;
  /** One-click skip for native recurring events. Uses confirm() since
   *  there's only one outcome (the action row already disambiguates
   *  from full delete via the 'skip this' button copy). */
  skipOccurrence(): Promise<void>;
  /** Drop a per-instance override; surfaces only when override_key is set. */
  resetOccurrence(): Promise<void>;
}

export type EventDetailDeleteDeps = {
  /** Live read of the event prop — re-read on each verb so a parent
   *  swap between mount and click doesn't act on stale data. */
  getEvent: () => CalendarEvent | null;
  /** Shared busy flag — owned by the parent so save / duplicate /
   *  delete take turns through the same gate. */
  setBusy: (v: boolean) => void;
  /** Close-the-modal callback. */
  close: () => void;
  /** Fired after any successful mutation so the parent reloads. */
  onChanged?: () => void;
};

export function createEventDetailDelete(deps: EventDetailDeleteDeps): EventDetailDeleteController {
  let deletePrompt = $state<DeletePrompt>('none');

  async function deleteEvent() {
    const event = deps.getEvent();
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
    deps.setBusy(true);
    try {
      if (event.type === 'ics_event' && event.source) {
        await api.deleteICSEvent(event.source, event.eventId);
      } else {
        await api.deleteEvent(event.eventId);
      }
      deps.onChanged?.();
      deps.close();
      toast.success('event deleted');
    } catch (err) {
      toast.error('delete failed: ' + errorMessage(err));
    } finally {
      deps.setBusy(false);
    }
  }

  async function confirmDeleteOccurrence() {
    const event = deps.getEvent();
    if (!event?.eventId) return;
    deps.setBusy(true);
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
        const key = exDateKey(event);
        if (!key) {
          toast.error("Can't identify this occurrence — edit the series instead.");
          return;
        }
        await api.skipEventOccurrence(event.eventId, key);
      } else {
        return;
      }
      deletePrompt = 'none';
      deps.onChanged?.();
      deps.close();
      toast.success('this occurrence skipped · series unchanged');
    } catch (err) {
      toast.error('skip failed: ' + errorMessage(err));
    } finally {
      deps.setBusy(false);
    }
  }

  async function confirmDeleteSeries() {
    const event = deps.getEvent();
    if (!event?.eventId) return;
    deps.setBusy(true);
    try {
      if (deletePrompt === 'recurring-ics' && event.source) {
        await api.deleteICSEvent(event.source, event.eventId);
      } else if (deletePrompt === 'recurring-native') {
        await api.deleteEvent(event.eventId);
      } else {
        return;
      }
      deletePrompt = 'none';
      deps.onChanged?.();
      deps.close();
      toast.success('entire series deleted');
    } catch (err) {
      toast.error('delete failed: ' + errorMessage(err));
    } finally {
      deps.setBusy(false);
    }
  }

  function cancelDeletePrompt() {
    deletePrompt = 'none';
  }

  // Reset a per-occurrence override back to series defaults. The
  // server side accepts an empty override body at the same key as
  // a clear; this surfaces it as a one-click action when an event
  // carries the override_key marker.
  async function resetOccurrence() {
    const event = deps.getEvent();
    if (!event?.eventId || !event.override_key) return;
    if (!confirm(`Reset "${event.title}" on this date back to the series defaults?`)) return;
    deps.setBusy(true);
    try {
      await api.overrideEventOccurrence(event.eventId, event.override_key, {});
      deps.onChanged?.();
      deps.close();
      toast.success('Occurrence reset to series defaults');
    } catch (err) {
      toast.error('reset failed: ' + errorMessage(err));
    } finally {
      deps.setBusy(false);
    }
  }

  async function skipOccurrence() {
    const event = deps.getEvent();
    if (!event?.eventId || !event.rrule) return;
    if (event.type !== 'event') {
      toast.info("Skipping ICS occurrences isn't supported yet — edit the source calendar.");
      return;
    }
    const key = exDateKey(event);
    if (!key) return;
    if (!confirm(`Skip just this occurrence of "${event.title}"? The rest of the series stays.`)) return;
    deps.setBusy(true);
    try {
      await api.skipEventOccurrence(event.eventId, key);
      deps.onChanged?.();
      deps.close();
      toast.success('Occurrence cancelled · series unchanged');
    } catch (err) {
      toast.error('skip failed: ' + errorMessage(err));
    } finally {
      deps.setBusy(false);
    }
  }

  return {
    get deletePrompt() { return deletePrompt; },
    deleteEvent,
    confirmDeleteOccurrence,
    confirmDeleteSeries,
    cancelDeletePrompt,
    skipOccurrence,
    resetOccurrence
  };
}
