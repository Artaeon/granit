// Four create-dialog state slots + their openers, bundled together
// because they're all "an open flag + seeded values + the function
// that opens with those values":
//
//   • QuickCreateScheduled  (createOpen / createDate/Hour/Minute) —
//     fired by clickSlot from the hour-grid views.
//   • CreateEvent           (createEventOpen / createEventDate) —
//     long-form event dialog from the toolbar.
//   • UnifiedCreate         (unifiedOpen / unifiedStart/End/Kind) —
//     fired by drag-select (kind='task') and by FindTime gap pick
//     (kind='event'). The dialog itself routes by kind.
//   • FindTime              (findTimeOpen) — no seed, just the
//                                            open flag.
//
// Keeping them in one controller saves the parent twelve loose
// $state declarations and three near-identical opener functions.

export interface CalendarCreateDialogsController {
  createOpen: boolean;
  createDate: Date;
  createHour: number;
  createMinute: number;

  createEventOpen: boolean;
  createEventDate: Date;

  unifiedOpen: boolean;
  unifiedStart: Date;
  unifiedEnd: Date;
  unifiedKind: 'task' | 'event';

  findTimeOpen: boolean;

  /** Open QuickCreateScheduled at a specific slot — what the hour
   *  grids call when the user taps an empty slot. */
  clickSlot(date: Date, hour: number, minute: number): void;
  /** Open UnifiedCreate for a drag-selected range (defaults to
   *  task kind). */
  onSlotRange(start: Date, end: Date): void;
  /** Open UnifiedCreate seeded from a FindTime gap pick (event
   *  kind, end derived from duration). */
  onFindTimePick(start: Date, durationMinutes: number): void;
}

export function createCalendarCreateDialogs(): CalendarCreateDialogsController {
  let createOpen = $state(false);
  let createDate = $state(new Date());
  let createHour = $state(9);
  let createMinute = $state(0);

  let createEventOpen = $state(false);
  let createEventDate = $state(new Date());

  let unifiedOpen = $state(false);
  let unifiedStart = $state(new Date());
  let unifiedEnd = $state(new Date());
  let unifiedKind = $state<'task' | 'event'>('task');

  let findTimeOpen = $state(false);

  function clickSlot(date: Date, hour: number, minute: number) {
    createDate = date;
    createHour = hour;
    createMinute = minute;
    createOpen = true;
  }

  function onSlotRange(start: Date, end: Date) {
    unifiedStart = start;
    unifiedEnd = end;
    unifiedKind = 'task';
    unifiedOpen = true;
  }

  function onFindTimePick(start: Date, durationMinutes: number) {
    unifiedStart = start;
    unifiedEnd = new Date(start.getTime() + durationMinutes * 60_000);
    unifiedKind = 'event';
    unifiedOpen = true;
  }

  return {
    get createOpen() { return createOpen; },
    set createOpen(v) { createOpen = v; },
    get createDate() { return createDate; },
    set createDate(v) { createDate = v; },
    get createHour() { return createHour; },
    set createHour(v) { createHour = v; },
    get createMinute() { return createMinute; },
    set createMinute(v) { createMinute = v; },

    get createEventOpen() { return createEventOpen; },
    set createEventOpen(v) { createEventOpen = v; },
    get createEventDate() { return createEventDate; },
    set createEventDate(v) { createEventDate = v; },

    get unifiedOpen() { return unifiedOpen; },
    set unifiedOpen(v) { unifiedOpen = v; },
    get unifiedStart() { return unifiedStart; },
    set unifiedStart(v) { unifiedStart = v; },
    get unifiedEnd() { return unifiedEnd; },
    set unifiedEnd(v) { unifiedEnd = v; },
    get unifiedKind() { return unifiedKind; },
    set unifiedKind(v) { unifiedKind = v; },

    get findTimeOpen() { return findTimeOpen; },
    set findTimeOpen(v) { findTimeOpen = v; },

    clickSlot,
    onSlotRange,
    onFindTimePick
  };
}
