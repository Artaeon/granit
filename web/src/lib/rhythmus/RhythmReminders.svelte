<script lang="ts">
  // Invisible component — mounted once at the app shell. Ticks every
  // 30 seconds, checks the configured reminder list, and fires a
  // toast (always) + a browser notification (when permission granted)
  // when the current local time crosses a reminder's HH:MM.
  //
  // Why a component and not a module-level singleton:
  //   1. Lifecycle — onMount + onDestroy give us a clean teardown
  //      when the layout re-renders (e.g. on logout).
  //   2. Reactivity — the rhythmus config store is read via
  //      auto-subscription, so a settings edit propagates here
  //      without a page reload.
  //
  // Dedup rules:
  //   - Each reminder fires once per local day. We persist
  //     "lastFiredOn" per reminder id in localStorage so a tab
  //     reload doesn't re-fire what already fired this morning.
  //   - A 30-minute grace window after the configured time covers
  //     "the user opened the app at 10:05" — they still get the
  //     10:00 reminder. Past that window the reminder is considered
  //     missed; we don't catch up retroactively.
  //   - Two tabs are coordinated through the same localStorage key:
  //     whichever tab fires first stamps lastFiredOn; the other
  //     reads the stamp on its next tick and skips.
  //
  // Sabbath: completely silent. The day is a rule, not a list of
  // tasks the user should be reminded to do.

  import { onMount } from 'svelte';
  import { toast } from '$lib/components/toast';
  import { sabbath } from '$lib/stores/sabbath';
  import { rhythmusConfig } from './minima';
  import { fmtDateISO } from '$lib/util/date';

  const STORAGE_PREFIX = 'granit.rhythmus.reminder.fired.';
  const GRACE_MINUTES = 30;

  function lastFiredKey(id: string): string {
    return STORAGE_PREFIX + id;
  }

  function getLastFiredOn(id: string): string {
    try {
      return localStorage.getItem(lastFiredKey(id)) ?? '';
    } catch {
      return '';
    }
  }

  function setLastFiredOn(id: string, date: string): void {
    try {
      localStorage.setItem(lastFiredKey(id), date);
    } catch {
      // Quota or private mode — fall through. Worst case the reminder
      // fires twice in a day.
    }
  }

  function parseHHMM(hhmm: string): { h: number; m: number } | null {
    const m = hhmm.match(/^(\d{1,2}):(\d{2})$/);
    if (!m) return null;
    const h = parseInt(m[1], 10);
    const mm = parseInt(m[2], 10);
    if (!Number.isFinite(h) || !Number.isFinite(mm)) return null;
    if (h < 0 || h > 23 || mm < 0 || mm > 59) return null;
    return { h, m: mm };
  }

  // Minutes-of-day comparison keeps the math out of date objects —
  // wall-clock 10:00 is always 600 minutes-into-the-day regardless
  // of DST transitions (which never happen mid-day anyway).
  function minutesOfDay(d: Date): number {
    return d.getHours() * 60 + d.getMinutes();
  }

  function fireReminder(label: string): void {
    toast.info(label);
    // Best-effort browser notification on top of the in-app toast.
    // Quiet about failure: missing permission is fine; the toast is
    // the load-bearing surface.
    try {
      if (typeof Notification === 'undefined') return;
      if (Notification.permission !== 'granted') return;
      const n = new Notification('Granit', { body: label, silent: false });
      // Auto-close after 8s so a missed notification doesn't pile up.
      window.setTimeout(() => n.close(), 8_000);
    } catch {
      // Some browsers throw on construct in non-https; ignore.
    }
  }

  function tick(now: Date): void {
    if ($sabbath) return;
    const today = fmtDateISO(now);
    const nowMin = minutesOfDay(now);
    for (const r of $rhythmusConfig.reminders) {
      if (!r.enabled) continue;
      const t = parseHHMM(r.time);
      if (!t) continue;
      const remMin = t.h * 60 + t.m;
      // Inside the [time, time + GRACE_MINUTES) window?
      if (nowMin < remMin) continue;
      if (nowMin >= remMin + GRACE_MINUTES) continue;
      if (getLastFiredOn(r.id) === today) continue;
      // Stamp BEFORE firing so a sibling tab that races sees the
      // stamp and skips. Firing twice is the failure mode we want
      // to avoid, not "fired zero times because of the race".
      setLastFiredOn(r.id, today);
      fireReminder(r.label || '');
    }
  }

  onMount(() => {
    // Initial tick on mount so a freshly-opened tab catches any
    // reminder whose window is still open. The setInterval handles
    // every subsequent crossing.
    tick(new Date());
    const id = window.setInterval(() => tick(new Date()), 30_000);
    return () => window.clearInterval(id);
  });
</script>
