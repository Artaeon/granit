// Calendar detail-drawer state + the click-on-an-event router.
//
// clickEvent decides what happens when the user picks an event card:
//
//   • goal_target → jump to /goals?focus=<id>. The detail modal
//     would imply edit/reschedule semantics the backend doesn't
//     support (a goal's target_date is a property of the goal, not
//     a standalone event), so the user is bounced to the source
//     of truth.
//
//   • meal_slot → toggle done in-place via api.patchMeal. Meal
//     slots are synthesized markers — no editable event exists
//     server-side — so the detail modal would offer reschedule/
//     delete actions that don't apply. Mirrors the dashboard widget
//     interaction so a tick anywhere syncs to both surfaces via
//     the daily note.
//
//   • everything else → open the EventDetail drawer + publish to the
//     workspace context bus so an adjacent AI pane can surface this
//     event as context.

import { goto } from '$app/navigation';
import { api, type CalendarEvent } from '$lib/api';
import { errorMessage } from '$lib/util/errorMessage';
import { toast } from '$lib/components/toast';
import { workspaceContext } from '$lib/workspace/workspaceContext.svelte';
import { fmtDateISO } from './utils';
import type { CalendarDataController } from './calendarData.svelte';

export interface CalendarDetailController {
  selected: CalendarEvent | null;
  detailOpen: boolean;
  /** Route an event click: goal_target → /goals, meal_slot → toggle,
   *  everything else → open the EventDetail drawer + publish to
   *  workspaceContext. */
  clickEvent(ev: CalendarEvent): void;
  /** Flip the done state of a meal_slot via api.patchMeal, then
   *  reload calendar data. */
  toggleMealEvent(ev: CalendarEvent): Promise<void>;
}

export interface CalendarDetailDeps {
  dataCtl: CalendarDataController;
}

export function createCalendarDetail(deps: CalendarDetailDeps): CalendarDetailController {
  let selected = $state<CalendarEvent | null>(null);
  let detailOpen = $state(false);

  function clickEvent(ev: CalendarEvent) {
    if (ev.type === 'goal_target' && ev.eventId) {
      goto(`/goals?focus=${encodeURIComponent(ev.eventId)}`);
      return;
    }
    if (ev.type === 'meal_slot' && ev.start) {
      void toggleMealEvent(ev);
      return;
    }
    selected = ev;
    detailOpen = true;
    workspaceContext.publish({
      paneKind: 'calendar',
      itemId: ev.eventId ?? `${ev.date ?? ''}|${ev.title ?? ''}`,
      label: ev.title ?? 'untitled event',
      excerpt: ev.date ?? undefined
    });
  }

  async function toggleMealEvent(ev: CalendarEvent) {
    if (!ev.start) return;
    const start = new Date(ev.start);
    if (Number.isNaN(start.getTime())) return;
    const hh = String(start.getHours()).padStart(2, '0');
    const mm = String(start.getMinutes()).padStart(2, '0');
    const dateISO = ev.date ?? fmtDateISO(start);
    try {
      // The (time, date) tuple is enough to identify the slot — a
      // day rarely has two meals at the same minute, and the API's
      // ApplyPatch matches on time-alone when name is empty. We
      // deliberately DON'T pass ev.title because it carries the
      // rendered "Breakfast — Haferflocken" combined label which
      // doesn't match the slot's bare Name field.
      await api.patchMeal({
        time: `${hh}:${mm}`,
        date: dateISO,
        done: !ev.done
      });
      await deps.dataCtl.load();
    } catch (e) {
      toast.error('Toggle meal failed: ' + errorMessage(e));
    }
  }

  return {
    get selected() {
      return selected;
    },
    set selected(v) {
      selected = v;
    },
    get detailOpen() {
      return detailOpen;
    },
    set detailOpen(v) {
      detailOpen = v;
    },
    clickEvent,
    toggleMealEvent
  };
}
