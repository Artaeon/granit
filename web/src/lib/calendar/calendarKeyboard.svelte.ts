// Page-scoped keyboard shortcuts for the calendar.
//
// Mirrors Google Calendar's default bindings so muscle memory carries
// over: t = today, j/n = next, k/p = prev, d/w/W/m/y/a = view, ? = help,
// f = find time, Shift+A = open agent.
//
// The handler short-circuits when:
//   • the user is typing in any text field;
//   • a modifier key is held (Mod/Ctrl/Alt — we don't fight system
//     shortcuts);
//   • a create / detail / find-time dialog is open and owns its own
//     keyboard surface.
//
// showShortcutHelp lives in the controller because '?' toggles it
// and the same controller surfaces it for the template's ShortcutHelp
// modal.

import type { CalendarViewStateController, View } from './calendarViewState.svelte';
import type { CalendarCreateDialogsController } from './calendarCreateDialogs.svelte';
import type { CalendarDetailController } from './calendarDetail.svelte';

export interface CalendarKeyboardController {
  showShortcutHelp: boolean;
  /** window keydown handler — pass to <svelte:window onkeydown={...}>. */
  onKeydown(e: KeyboardEvent): void;
}

export interface CalendarKeyboardDeps {
  viewCtl: CalendarViewStateController;
  dlgCtl: CalendarCreateDialogsController;
  detCtl: CalendarDetailController;
  /** Open the calendar agent (Shift+A). The parent owns the agentOpen
   *  state so this is a callback rather than a controller field. */
  openAgent: () => void;
}

function isTextField(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  const tag = el.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable;
}

export function createCalendarKeyboard(deps: CalendarKeyboardDeps): CalendarKeyboardController {
  let showShortcutHelp = $state(false);
  const { viewCtl, dlgCtl, detCtl } = deps;

  function onKeydown(e: KeyboardEvent) {
    if (isTextField(e.target)) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    // Don't fight the create / detail drawers — they own their own
    // keyboard surface (Escape to close, Enter to submit).
    if (dlgCtl.createOpen || dlgCtl.createEventOpen || dlgCtl.unifiedOpen || detCtl.detailOpen || dlgCtl.findTimeOpen) return;
    switch (e.key) {
      case 't': viewCtl.gotoToday(); break;
      case 'j': case 'n': viewCtl.next(); break;
      case 'k': case 'p': viewCtl.prev(); break;
      case 'd': viewCtl.view = 'day' as View; break;
      case 'w': viewCtl.view = 'week' as View; break;
      case 'W': viewCtl.view = 'workweek' as View; break;
      case 'm': viewCtl.view = 'month' as View; break;
      case 'y': viewCtl.view = 'year' as View; break;
      // 'a' = agenda view (matches Google Calendar). Shift+A opens
      // the calendar agent.
      case 'a': viewCtl.view = 'agenda' as View; break;
      case 'A': deps.openAgent(); break;
      case 'f': dlgCtl.findTimeOpen = true; break;
      case '?': showShortcutHelp = !showShortcutHelp; break;
      default: return;
    }
    e.preventDefault();
  }

  return {
    get showShortcutHelp() {
      return showShortcutHelp;
    },
    set showShortcutHelp(v) {
      showShortcutHelp = v;
    },
    onKeydown
  };
}

// Horizontal touch-swipe to navigate weeks/days. Triggered on the
// main grid container; a horizontal swipe of >60px (with vertical
// movement <40px so we don't hijack scroll) counts. Mobile users
// can flick between weeks the same way they would on Google
// Calendar / iOS Calendar.

export interface CalendarSwipeHandlers {
  onTouchStart(e: TouchEvent): void;
  onTouchEnd(e: TouchEvent): void;
}

export function createCalendarSwipe(viewCtl: CalendarViewStateController): CalendarSwipeHandlers {
  let touchStartX = 0;
  let touchStartY = 0;
  let touchActive = false;

  function onTouchStart(e: TouchEvent) {
    if (e.touches.length !== 1) { touchActive = false; return; }
    touchStartX = e.touches[0].clientX;
    touchStartY = e.touches[0].clientY;
    touchActive = true;
  }

  function onTouchEnd(e: TouchEvent) {
    if (!touchActive) return;
    touchActive = false;
    if (e.changedTouches.length !== 1) return;
    const dx = e.changedTouches[0].clientX - touchStartX;
    const dy = e.changedTouches[0].clientY - touchStartY;
    if (Math.abs(dy) > 40) return; // mostly vertical → let scroll happen
    if (Math.abs(dx) < 60) return; // too short
    if (dx > 0) viewCtl.prev(); else viewCtl.next();
  }

  return { onTouchStart, onTouchEnd };
}
