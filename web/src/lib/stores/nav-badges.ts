// Sidebar badge counts. Two reactive stores driven by the same auth
// + WS lifecycle the layout used to manage inline. Pulled into a
// module so the layout stays focused on shell composition and the
// badge refresh logic gets a tested seam.
//
// overdueTasks: open tasks with a dueDate strictly before today
// (YYYY-MM-DD). todayEvents: calendar feed entries (events.json +
// ICS subscriptions + scheduled tasks) whose date / start lands on
// today. Both refresh on demand and on WS task/event mutations.
// Errors swallow silently — a stale or missing badge is fine; an
// alert spam isn't.

import { writable, get } from 'svelte/store';
import { api } from '$lib/api';
import { onWsEvent } from '$lib/ws';
import { auth } from '$lib/stores/auth';

export const overdueTaskCount = writable<number>(0);
export const todayEventCount = writable<number>(0);

function todayISO(): string {
  const d = new Date();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${m}-${day}`;
}

export async function refreshOverdueTasks(): Promise<void> {
  try {
    const today = todayISO();
    const res = await api.listTasks({ status: 'open', due_before: today });
    overdueTaskCount.set(res.tasks.filter((t) => !t.done && !!t.dueDate && t.dueDate < today).length);
  } catch {
    // leave previous count in place
  }
}

export async function refreshTodayEvents(): Promise<void> {
  try {
    const today = todayISO();
    const feed = await api.calendar(today, today);
    const isToday = (ev: { date?: string; start?: string }) => {
      if (ev.date) return ev.date === today;
      if (ev.start) return ev.start.slice(0, 10) === today;
      return false;
    };
    todayEventCount.set(feed.events.filter(isToday).length);
  } catch {
    // leave previous count in place
  }
}

// Wire badge refresh into auth + WS lifecycle. Returns a cleanup
// function suitable for onMount/$effect tear-down. Auth-gated so a
// logged-out tab doesn't fire API calls; the WS subscription is
// outlived by the auth subscription so a fresh login repopulates
// the badges immediately.
//
// WS bursts collapse via a pending-microtask flag — a plan apply
// that flips many tasks resolves into a single refetch per badge
// rather than one per event.
export function startNavBadges(): () => void {
  let pendingTasks = false;
  let pendingEvents = false;

  const offAuth = auth.subscribe((tok) => {
    if (!tok) {
      overdueTaskCount.set(0);
      todayEventCount.set(0);
      return;
    }
    void refreshOverdueTasks();
    void refreshTodayEvents();
  });

  const offWs = onWsEvent((ev) => {
    if (!get(auth)) return;
    if (ev.type === 'task.changed' || ev.type === 'vault.rescanned') {
      if (!pendingTasks) {
        pendingTasks = true;
        queueMicrotask(() => { pendingTasks = false; void refreshOverdueTasks(); });
      }
    }
    if (
      ev.type === 'event.changed' ||
      ev.type === 'event.removed' ||
      ev.type === 'task.changed' ||
      ev.type === 'vault.rescanned'
    ) {
      if (!pendingEvents) {
        pendingEvents = true;
        queueMicrotask(() => { pendingEvents = false; void refreshTodayEvents(); });
      }
    }
  });

  return () => {
    offAuth();
    offWs();
  };
}
