// Natural-language quick-create above the calendar grid:
//
//   "lunch tomorrow 12pm 1h" -> event
//
// Reuses the deterministic regex parser in calendar/quickCreate.ts
// (no LLM call), so the flow stays fast, offline-friendly, and
// predictable. Live preview shows what we recognised so the user
// can see whether the parse worked before hitting Enter — saves
// the "type, submit, get a wrong event, delete, retry" loop.

import { api } from '$lib/api';
import { errorMessage } from '$lib/util/errorMessage';
import { toast } from '$lib/components/toast';
import { parseEventInput, type ParseResult } from './quickCreate';
import type { CalendarDataController } from './calendarData.svelte';

export interface CalendarQuickEventController {
  quickInput: string;
  readonly quickBusy: boolean;
  readonly quickParse: ParseResult | null;
  submit(): Promise<void>;
}

export interface CalendarQuickEventDeps {
  dataCtl: CalendarDataController;
}

export function createCalendarQuickEvent(deps: CalendarQuickEventDeps): CalendarQuickEventController {
  let quickInput = $state('');
  let quickBusy = $state(false);
  const quickParse = $derived<ParseResult | null>(
    quickInput.trim() ? parseEventInput(quickInput) : null
  );

  async function submit() {
    if (!quickParse?.ok || !quickParse.event || quickBusy) return;
    quickBusy = true;
    try {
      const ev = quickParse.event;
      await api.createEvent({
        title: ev.title,
        date: ev.date,
        start_time: ev.startTime,
        end_time: ev.endTime
      });
      quickInput = '';
      toast.success('event created');
      await deps.dataCtl.load();
    } catch (err) {
      toast.error('create failed: ' + errorMessage(err));
    } finally {
      quickBusy = false;
    }
  }

  return {
    get quickInput() {
      return quickInput;
    },
    set quickInput(v) {
      quickInput = v;
    },
    get quickBusy() {
      return quickBusy;
    },
    get quickParse() {
      return quickParse;
    },
    submit
  };
}
