import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

// Mock the api module before importing nav-badges so the inner
// closure captures the mocked listTasks/calendar. The auth store
// pulls getToken from the same module, so we keep the originals
// around with importOriginal.
vi.mock('$lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api')>();
  return {
    ...actual,
    api: {
      ...actual.api,
      listTasks: vi.fn(),
      calendar: vi.fn()
    }
  };
});

// onWsEvent is touched by startNavBadges; the test doesn't drive
// the WS path, but the import has to resolve. A noop subscriber is
// enough.
vi.mock('$lib/ws', () => ({
  onWsEvent: vi.fn(() => () => undefined)
}));

import { api } from '$lib/api';
import {
  overdueTaskCount,
  todayEventCount,
  refreshOverdueTasks,
  refreshTodayEvents
} from './nav-badges';

function ymd(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

beforeEach(() => {
  vi.clearAllMocks();
  overdueTaskCount.set(0);
  todayEventCount.set(0);
});

afterEach(() => {
  vi.useRealTimers();
});

describe('refreshOverdueTasks', () => {
  it('counts open tasks with a dueDate strictly before today', async () => {
    const today = ymd(new Date());
    const yesterday = ymd(new Date(Date.now() - 86_400_000));
    (api.listTasks as ReturnType<typeof vi.fn>).mockResolvedValue({
      tasks: [
        { id: '1', done: false, dueDate: yesterday },
        { id: '2', done: false, dueDate: today }, // today is NOT overdue
        { id: '3', done: true, dueDate: yesterday }, // done doesn't count
        { id: '4', done: false }, // no dueDate doesn't count
        { id: '5', done: false, dueDate: yesterday }
      ]
    });
    await refreshOverdueTasks();
    expect(get(overdueTaskCount)).toBe(2);
  });

  it('keeps the previous count on API failure (no flash to zero)', async () => {
    overdueTaskCount.set(7);
    (api.listTasks as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('boom'));
    await refreshOverdueTasks();
    expect(get(overdueTaskCount)).toBe(7);
  });
});

describe('refreshTodayEvents', () => {
  it('counts events whose date or start lands on today', async () => {
    const today = ymd(new Date());
    const tomorrow = ymd(new Date(Date.now() + 86_400_000));
    (api.calendar as ReturnType<typeof vi.fn>).mockResolvedValue({
      events: [
        { date: today },
        { date: tomorrow },
        { start: `${today}T09:00:00Z` },
        { start: `${tomorrow}T09:00:00Z` },
        { /* neither field — should not count */ }
      ]
    });
    await refreshTodayEvents();
    expect(get(todayEventCount)).toBe(2);
  });

  it('keeps the previous count on API failure', async () => {
    todayEventCount.set(3);
    (api.calendar as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('nope'));
    await refreshTodayEvents();
    expect(get(todayEventCount)).toBe(3);
  });
});
