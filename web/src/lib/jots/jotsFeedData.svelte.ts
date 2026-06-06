// Jots feed data + per-day activity prefetch.
//
// First extraction step out of routes/jots/+page.svelte. Owns the
// paginated jots[] list (cursor / loading / done / error), the WS-
// driven single-date refetch with per-date debouncing, and the
// bounded prefetch queue that pre-warms the inline day-activity
// counts shown under each card.
//
// Also owns the daily folder string read once on mount via getConfig
// — both the WS-scope guard (jotMatches) and jump-to-day routing
// need it, so it lives here next to its single consumer.
//
// The page still owns the IntersectionObserver wiring, the auth
// guard, and the AI / composer / shortcut surfaces. They all read
// `jots` via this controller's getter so derivations (visibleJots,
// streakDays, etc.) stay reactive in the calling context.

import { api, fmtDateISO, type DayActivityItem, type Jot } from '$lib/api';

const PREFETCH_CONCURRENCY = 4;

export interface JotsFeedDataDeps {
  /** Reactive snapshot of the auth store — gates loadMore so we
   *  don't hammer the API before the user is signed in. The page
   *  passes () => !!$auth so the read stays reactive. */
  isAuthed: () => boolean;
  /** Soft-fail prefetch + day-activity loads soft-fail; loadMore
   *  surfaces real errors via this hook so the page can render an
   *  inline error banner. */
  onLoadError?: (message: string) => void;
  /** Optional hook fired with the dates returned by each loadMore()
   *  page so callers can pre-warm related state — currently used
   *  to enqueue day-activity prefetch (which lives here already)
   *  but exposed so future consumers can opt in too. */
  onPageLoaded?: (dates: string[]) => void;
}

export interface JotsFeedDataController {
  readonly jots: Jot[];
  readonly loading: boolean;
  readonly done: boolean;
  readonly error: string;
  readonly dailyFolder: string;

  /** Per-date day-activity cache + loading flags. Plain records —
   *  Svelte 5 tracks reassignment so the patterns below replace the
   *  whole object on each write. */
  readonly dayActivityCache: Record<string, DayActivityItem[]>;
  readonly dayActivityLoading: Record<string, boolean>;

  loadConfig(): Promise<void>;
  loadMore(): Promise<void>;
  /** Refetch a single date and patch the array in-place (insert at
   *  the right sort-desc position when it's new). Soft-fails on
   *  error so a transient WS-driven refetch never blows up the page. */
  refetchJot(date: string): Promise<void>;
  /** Debounced wrapper around refetchJot so a flurry of WS writes
   *  for the same date collapses to one round-trip. */
  scheduleRefetch(date: string): void;
  /** Lazy load of the day-activity items for `date`. Short-circuits
   *  on a cache hit; safe to call repeatedly. */
  loadDayActivity(date: string): Promise<void>;
  /** Match a vault-relative path to a (date, folder) tuple iff the
   *  path is a daily note (`<folder>/YYYY-MM-DD.md`) under the
   *  configured daily folder. Used to scope WS-driven refetches. */
  jotMatches(path: string): { date: string; folder: string } | null;

  /** Cleanup — clears the pending-refetch timers. Call from
   *  onMount's return to avoid stray timers after unmount. */
  dispose(): void;
}

export function createJotsFeedData(deps: JotsFeedDataDeps): JotsFeedDataController {
  let jots = $state<Jot[]>([]);
  let cursor = $state<string | null>(null);
  let loading = $state(false);
  let done = $state(false);
  let error = $state('');
  let dailyFolder = $state('');

  let dayActivityCache = $state<Record<string, DayActivityItem[]>>({});
  let dayActivityLoading = $state<Record<string, boolean>>({});

  // Bounded prefetch queue — keeps in-flight count under
  // PREFETCH_CONCURRENCY so a fresh-load (20 new dates) doesn't fan
  // out 20 parallel requests. Plain non-reactive arrays/counters
  // because the UI doesn't read them.
  let prefetchQueue: string[] = [];
  let prefetchActive = 0;
  function enqueuePrefetch(dates: string[]) {
    for (const d of dates) {
      if (dayActivityCache[d] !== undefined) continue;
      if (dayActivityLoading[d]) continue;
      if (prefetchQueue.includes(d)) continue;
      prefetchQueue.push(d);
    }
    drainPrefetch();
  }
  function drainPrefetch() {
    while (prefetchActive < PREFETCH_CONCURRENCY && prefetchQueue.length > 0) {
      const next = prefetchQueue.shift();
      if (!next) break;
      prefetchActive += 1;
      // loadDayActivity is idempotent — it short-circuits on a cache
      // hit and writes to the same maps the expand-for-details path
      // reads from.
      loadDayActivity(next).finally(() => {
        prefetchActive -= 1;
        drainPrefetch();
      });
    }
  }

  async function loadDayActivity(date: string) {
    if (dayActivityCache[date] !== undefined || dayActivityLoading[date]) return;
    dayActivityLoading = { ...dayActivityLoading, [date]: true };
    try {
      const r = await api.dayActivity(date);
      dayActivityCache = { ...dayActivityCache, [date]: r.items };
    } catch {
      // Soft-fail — empty list keeps the UI honest; user can refresh.
      dayActivityCache = { ...dayActivityCache, [date]: [] };
    } finally {
      const next = { ...dayActivityLoading };
      delete next[date];
      dayActivityLoading = next;
    }
  }

  async function loadConfig() {
    try {
      const c = await api.getConfig();
      dailyFolder = (c.daily_notes_folder ?? '').replace(/\/+$/, '');
    } catch {
      // No config endpoint or failure → assume vault root, which is
      // the default-config behavior anyway.
      dailyFolder = '';
    }
  }

  async function loadMore() {
    if (loading || done || !deps.isAuthed()) return;
    loading = true;
    error = '';
    try {
      const params: { before?: string; limit: number } = { limit: 20 };
      if (cursor) params.before = cursor;
      const r = await api.listJots(params);
      jots = [...jots, ...r.jots];
      cursor = r.nextBefore;
      if (!r.hasMore) done = true;
      const dates = r.jots.map((j) => j.date);
      enqueuePrefetch(dates);
      deps.onPageLoaded?.(dates);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      error = msg;
      deps.onLoadError?.(msg);
    } finally {
      loading = false;
    }
  }

  function nextDateISO(d: string): string {
    const dt = new Date(d + 'T00:00:00');
    dt.setUTCDate(dt.getUTCDate() + 1);
    return fmtDateISO(dt);
  }

  async function refetchJot(date: string) {
    try {
      // /jots is sort-desc + cursor-based; to grab a single date we ask
      // for the page just-after it (before = date+1day) limited to 1.
      const next = nextDateISO(date);
      const r = await api.listJots({ before: next, limit: 1 });
      const fresh = r.jots.find((j) => j.date === date);
      if (!fresh) {
        // The jot was deleted (or never existed) — drop it from the
        // list if it's there.
        jots = jots.filter((j) => j.date !== date);
        return;
      }
      const idx = jots.findIndex((j) => j.date === date);
      if (idx >= 0) {
        jots = [...jots.slice(0, idx), fresh, ...jots.slice(idx + 1)];
      } else {
        // New (today's) daily — prepend, keeping desc order intact.
        const insertAt = jots.findIndex((j) => j.date < date);
        if (insertAt < 0) jots = [...jots, fresh];
        else jots = [...jots.slice(0, insertAt), fresh, ...jots.slice(insertAt)];
      }
    } catch {
      // Soft-fail: a refetch error shouldn't blow up the page.
    }
  }

  // Debounce WS-driven refetches per-date — a flurry of writes (the
  // user typing into a daily) shouldn't trigger a refetch per
  // keystroke. 500ms feels live without thrashing.
  const pendingRefetch = new Map<string, ReturnType<typeof setTimeout>>();
  function scheduleRefetch(date: string) {
    const existing = pendingRefetch.get(date);
    if (existing) clearTimeout(existing);
    const t = setTimeout(() => {
      pendingRefetch.delete(date);
      refetchJot(date);
    }, 500);
    pendingRefetch.set(date, t);
  }

  function jotMatches(path: string): { date: string; folder: string } | null {
    const m = path.match(/^(?:(.+)\/)?(\d{4}-\d{2}-\d{2})\.md$/);
    if (!m) return null;
    const folder = m[1] ?? '';
    if (folder !== dailyFolder) return null;
    return { date: m[2], folder };
  }

  function dispose() {
    for (const t of pendingRefetch.values()) clearTimeout(t);
    pendingRefetch.clear();
  }

  return {
    get jots() { return jots; },
    get loading() { return loading; },
    get done() { return done; },
    get error() { return error; },
    get dailyFolder() { return dailyFolder; },
    get dayActivityCache() { return dayActivityCache; },
    get dayActivityLoading() { return dayActivityLoading; },
    loadConfig,
    loadMore,
    refetchJot,
    scheduleRefetch,
    loadDayActivity,
    jotMatches,
    dispose
  };
}
