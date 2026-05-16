<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, fmtDateISO, type DayActivityItem, type Jot, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { rafThrottle } from '$lib/util/streamThrottle';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { toast } from '$lib/components/toast';

  // Amplenote-style infinite-scroll feed of every daily note. The page
  // talks to /api/v1/jots which paginates server-side — fetching N
  // dailies one-by-one would round-trip N times per page; the dedicated
  // endpoint keeps it to one round-trip per page no matter how many
  // years of dailies the user has accumulated.

  let jots = $state<Jot[]>([]);
  let cursor = $state<string | null>(null);
  let loading = $state(false);
  let done = $state(false);
  let error = $state('');

  // Daily folder, pulled from the user's config so jump-to-day knows
  // where to navigate. Read once on mount; settings changes mid-session
  // are rare and a refresh recovers cleanly.
  let dailyFolder = $state('');

  // Inline search state
  let searchText = $state('');
  let searchResults = $state<Note[]>([]);
  let searching = $state(false);

  // Hashtag filter — when non-empty, jots must mention EVERY active
  // tag (AND filter). Clicking a `#tag` chip toggles membership.
  // Persisted in the URL hash so a refresh or shared link lands the
  // user on the same filtered view. Supports both the new form
  // `#tags=a,b,c` and the legacy single-tag form `#tag=foo` from
  // earlier versions of this page.
  function readTagsFromHash(): string[] {
    if (typeof window === 'undefined') return [];
    const h = window.location.hash;
    const multi = h.match(/^#tags=(.+)$/);
    if (multi) {
      return decodeURIComponent(multi[1])
        .split(',')
        .map((s) => s.trim().toLowerCase())
        .filter(Boolean);
    }
    const single = h.match(/^#tag=(.+)$/);
    if (single) return [decodeURIComponent(single[1]).toLowerCase()];
    return [];
  }
  let activeTags = $state<string[]>(readTagsFromHash());

  // Quick filters — orthogonal to the tag set. "open tasks" hides jots
  // whose daily has zero unchecked checkboxes; "timeframe" caps the
  // feed to dailies within the last N days from today.
  type Timeframe = 'all' | '7d' | '30d';
  let filterOpenTasks = $state(false);
  let filterTimeframe = $state<Timeframe>('all');

  let hasAnyFilter = $derived(
    activeTags.length > 0 || filterOpenTasks || filterTimeframe !== 'all'
  );

  // All distinct hashtags found across the loaded jots, ordered by
  // frequency desc then alpha. Cheap derivation — for a few hundred
  // jots × a few tags each, the linear scan is invisible.
  let allTags = $derived.by(() => {
    const counts = new Map<string, number>();
    const tagRe = /(?:^|\s)#([\p{L}\p{N}_-]+)/gu;
    for (const j of jots) {
      const seen = new Set<string>();
      for (const m of (j.body ?? '').matchAll(tagRe)) {
        const t = m[1].toLowerCase();
        if (seen.has(t)) continue;
        seen.add(t);
        counts.set(t, (counts.get(t) ?? 0) + 1);
      }
    }
    return [...counts.entries()]
      .sort((a, b) => (b[1] - a[1]) || a[0].localeCompare(b[0]))
      .map(([t]) => t);
  });

  // Filtered feed = jots that satisfy every active filter dimension:
  //   1. every active tag must appear in the body (AND)
  //   2. if filterOpenTasks, jot.openTasks > 0
  //   3. if filterTimeframe != 'all', jot.date is within the window
  // When no filter is active, returns the full list verbatim.
  let visibleJots = $derived.by(() => {
    let out = jots;
    if (filterOpenTasks) out = out.filter((j) => j.openTasks > 0);
    if (filterTimeframe !== 'all') {
      const days = filterTimeframe === '7d' ? 7 : 30;
      const cutoff = new Date(today);
      cutoff.setDate(cutoff.getDate() - (days - 1));
      const cutoffISO = fmtDateISO(cutoff);
      out = out.filter((j) => j.date >= cutoffISO);
    }
    if (activeTags.length === 0) return out;
    // Build one regex per tag; jot must match all of them.
    const escaped = activeTags.map((t) => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
    const regexes = escaped.map((e) => new RegExp(`(?:^|\\s)#${e}\\b`, 'i'));
    return out.filter((j) => {
      const body = j.body ?? '';
      return regexes.every((re) => re.test(body));
    });
  });

  function writeTagsHash() {
    if (typeof window === 'undefined') return;
    const next = activeTags.length > 0
      ? `#tags=${activeTags.map(encodeURIComponent).join(',')}`
      : '';
    // Replace, not push — tag filtering shouldn't pollute browser
    // history with one entry per chip click.
    history.replaceState(null, '', window.location.pathname + window.location.search + next);
  }

  function toggleTag(t: string) {
    const lower = t.toLowerCase();
    if (activeTags.includes(lower)) {
      activeTags = activeTags.filter((x) => x !== lower);
    } else {
      activeTags = [...activeTags, lower];
    }
    writeTagsHash();
  }
  function clearAllFilters() {
    activeTags = [];
    filterOpenTasks = false;
    filterTimeframe = 'all';
    writeTagsHash();
  }

  // Sentinel + observer for infinite scroll.
  let sentinel: HTMLDivElement | undefined = $state();
  let observer: IntersectionObserver | null = null;

  // ─── per-day activity (lazy, on-expand) ──────────────────────────
  // The Jots feed shows one daily-note body per entry; under it a
  // collapsed <details> block surfaces every OTHER thing created on
  // that same day (notes, tasks created/completed, events, habits,
  // prayer, hub items). Each block fetches lazily on first open so
  // a long scroll doesn't N+1 the API.
  // Per-date cache + loading flags as plain records — Svelte 5
  // tracks property additions and re-renders on reassignment, so
  // this is the simplest reactive pattern for "memo by string key".
  let dayActivityCache = $state<Record<string, DayActivityItem[]>>({});
  let dayActivityLoading = $state<Record<string, boolean>>({});

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

  // Group activity items by their Kind so the renderer can render a
  // labelled bucket per category instead of one undifferentiated
  // list. Order of buckets is fixed so the layout stays stable
  // across re-renders even as items shift.
  type Bucket = { kind: string; label: string; items: DayActivityItem[] };
  const KIND_LABELS: Record<string, string> = {
    note_created: 'Notes created',
    task_created: 'Tasks created',
    task_completed: 'Tasks completed',
    event: 'Calendar',
    habit: 'Habits',
    prayer: 'Prayer',
    hub_item: 'Hub',
    jot: 'Jots'
  };
  const BUCKET_ORDER: string[] = [
    'event',
    'task_created',
    'task_completed',
    'note_created',
    'jot',
    'habit',
    'prayer',
    'hub_item'
  ];

  // Inline-header counts: a compact summary of the day-activity items
  // bucketed by Kind, used to render a chip strip in each jot's header
  // without expanding the <details> block. Picked the four buckets that
  // matter at-a-glance: events, tasks created, tasks completed, notes
  // created. Habits/prayer/hub are surfaced inside the expanded panel.
  type ActivitySummary = {
    events: number;
    tasksCreated: number;
    tasksDone: number;
    notes: number;
    total: number;
  };
  function summarize(items: DayActivityItem[] | undefined): ActivitySummary {
    const s: ActivitySummary = { events: 0, tasksCreated: 0, tasksDone: 0, notes: 0, total: 0 };
    if (!items) return s;
    for (const it of items) {
      s.total += 1;
      switch (it.kind) {
        case 'event': s.events += 1; break;
        case 'task_created': s.tasksCreated += 1; break;
        case 'task_completed': s.tasksDone += 1; break;
        case 'note_created': s.notes += 1; break;
      }
    }
    return s;
  }

  // Eager-but-bounded prefetch of dayActivity for newly-loaded jots so
  // the inline header counts populate without each card needing to be
  // scrolled into view. Caps concurrency at 4 to avoid hammering the
  // server when a fresh-load brings in 20 dates at once; pages 2+ only
  // add another 20 each, so total in-flight stays small.
  let prefetchQueue: string[] = [];
  let prefetchActive = 0;
  const PREFETCH_CONCURRENCY = 4;
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

  function bucketize(items: DayActivityItem[]): Bucket[] {
    const groups = new Map<string, DayActivityItem[]>();
    for (const it of items) {
      const arr = groups.get(it.kind) ?? [];
      arr.push(it);
      groups.set(it.kind, arr);
    }
    const out: Bucket[] = [];
    for (const k of BUCKET_ORDER) {
      const arr = groups.get(k);
      if (arr && arr.length > 0) {
        out.push({ kind: k, label: KIND_LABELS[k] ?? k, items: arr });
      }
    }
    // Stray kinds the server might add later get appended at the end
    // so a future "measurement" entry surfaces without a UI release.
    for (const [k, arr] of groups) {
      if (BUCKET_ORDER.indexOf(k) === -1 && arr.length > 0) {
        out.push({ kind: k, label: KIND_LABELS[k] ?? k, items: arr });
      }
    }
    return out;
  }

  function activityHref(it: DayActivityItem): string {
    if (it.path) return `/notes/${encodeURIComponent(it.path)}`;
    if (it.kind === 'event') return '/calendar';
    if (it.kind === 'prayer') return '/prayer';
    if (it.kind === 'hub_item') return '/hub';
    if (it.kind === 'habit') return '/habits';
    if (it.kind === 'task_created' || it.kind === 'task_completed') return '/tasks';
    return '#';
  }

  function activityTime(at: string): string {
    const d = new Date(at);
    if (Number.isNaN(d.getTime())) return '';
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');
    return `${hh}:${mm}`;
  }

  // ─── AI: theme detection across loaded jots ──────────────────────
  // Reads the last 20-30 loaded jot bodies and surfaces 3-5 recurring
  // themes (topics, people, projects, struggles, joys). Each theme
  // becomes a clickable chip that runs the existing search flow,
  // turning vague pattern recognition into navigable surfaces.
  type Theme = { label: string; query: string };
  let aiBusy = $state(false);
  let aiAbort: AbortController | null = null;
  let aiThemes = $state<Theme[]>([]);
  let aiError = $state('');
  let aiRaw = $state('');

  function buildJotsSeed(): string {
    // Cap at 30 jots × ~1200 chars each. The model needs enough
    // signal to spot recurrence without blowing the prompt out.
    const slice = jots.slice(0, 30).map((j) => ({
      date: j.date,
      body: (j.body ?? '').slice(0, 1200)
    }));
    return JSON.stringify(slice, null, 2);
  }

  async function detectThemes() {
    if (aiBusy || jots.length < 5) {
      if (jots.length < 5) toast.info('Load a few more jots first.');
      return;
    }
    aiAbort?.abort();
    aiAbort = new AbortController();
    aiBusy = true;
    aiError = '';
    aiThemes = [];
    aiRaw = '';
    let buf = '';
    const seed = buildJotsSeed();
    const system = 'You analyse recent daily-note entries and surface 3-5 recurring themes. A theme is a topic, person, project, struggle, or joy that shows up across multiple entries. Return STRICTLY a JSON array, no fences, no prose: [{"label": "<short title, 1-3 words, lowercase>", "query": "<single-word search term that finds the theme>"}]. Pick search terms that actually appear in the entries (a hashtag, a name, a recurring word) — not synonyms.';
    const user = `Recent jots:\n\`\`\`json\n${seed}\n\`\`\`\n\nGive me 3-5 themes.`;
    try {
      // rAF throttle — aiRaw is rendered live as a preview.
      const jotsT = rafThrottle((full) => { aiRaw = full; });
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: jotsT.onChunk,
          onDone: () => {
            jotsT.flush();
            let cleaned = aiRaw.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const arr = JSON.parse(cleaned) as Theme[];
              if (Array.isArray(arr)) aiThemes = arr.filter((x) => x.label && x.query);
            } catch {
              aiError = 'Model didn\'t return parseable JSON.';
            }
          },
          onError: (err) => { jotsT.flush(); aiError = err.message; }
        },
        aiAbort.signal
      );
    } finally {
      aiBusy = false;
      aiAbort = null;
    }
  }
  function dismissThemes() {
    aiAbort?.abort();
    aiBusy = false;
    aiThemes = [];
    aiError = '';
    aiRaw = '';
  }
  function applyTheme(t: Theme) {
    searchText = t.query;
    runSearch();
  }

  // ── jot path / regex ──────────────────────────────────────────────
  // Mirrors the server's filter — a vault-relative path is a daily note
  // iff it's `<folder>/YYYY-MM-DD.md` or just `YYYY-MM-DD.md` (when no
  // folder is configured). Used to scope WS-driven refetches.
  function jotMatches(path: string): { date: string; folder: string } | null {
    const m = path.match(/^(?:(.+)\/)?(\d{4}-\d{2}-\d{2})\.md$/);
    if (!m) return null;
    const folder = m[1] ?? '';
    if (folder !== dailyFolder) return null;
    return { date: m[2], folder };
  }

  async function loadMore() {
    if (loading || done || !$auth) return;
    loading = true;
    error = '';
    try {
      const params: { before?: string; limit: number } = { limit: 20 };
      if (cursor) params.before = cursor;
      const r = await api.listJots(params);
      jots = [...jots, ...r.jots];
      cursor = r.nextBefore;
      if (!r.hasMore) done = true;
      // Queue inline-count prefetch for the new dates so headers
      // populate before the user scrolls each into view.
      enqueuePrefetch(r.jots.map((j) => j.date));
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  // Refetch a single jot by date and patch it into the array (or
  // prepend if it didn't exist before — i.e. today's daily was just
  // created).
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

  function nextDateISO(d: string): string {
    const dt = new Date(d + 'T00:00:00');
    dt.setUTCDate(dt.getUTCDate() + 1);
    return fmtDateISO(dt);
  }

  // ── header date formatting ────────────────────────────────────────
  // "Today" / "Yesterday" / weekday for ±6 days / full date otherwise.
  function relativeLabel(date: string, today: Date): string {
    const d = new Date(date + 'T00:00:00');
    const diff = Math.round((d.getTime() - today.getTime()) / 86400000);
    if (diff === 0) return 'Today';
    if (diff === -1) return 'Yesterday';
    if (diff === 1) return 'Tomorrow';
    if (diff > -7 && diff < 7) {
      return d.toLocaleDateString(undefined, { weekday: 'long' });
    }
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }

  function fullLabel(date: string): string {
    const d = new Date(date + 'T00:00:00');
    return d.toLocaleDateString(undefined, {
      weekday: 'long',
      year: 'numeric',
      month: 'long',
      day: 'numeric'
    });
  }

  // Midnight today, recomputed reactively via $derived.by — used as the
  // anchor for relative-date labels ("Today" / "Yesterday" / etc).
  let today = $derived.by(() => {
    const d = new Date();
    return new Date(d.getFullYear(), d.getMonth(), d.getDate());
  });

  // ── header stats: streak + loaded counters ────────────────────────
  // All derived from the loaded `jots` array — no extra round-trips.
  // The streak window is bounded by what's currently loaded; if the
  // user scrolls past the streak edge it extends naturally as more
  // pages arrive.

  // Current daily-note streak: consecutive calendar days ending today
  // (or yesterday — Amplenote-style grace so you don't lose a streak
  // before today's daily is written) that have a loaded jot.
  let streakDays = $derived.by(() => {
    if (jots.length === 0) return 0;
    const have = new Set(jots.map((j) => j.date));
    // Walk back from today. Allow today to be missing as long as
    // yesterday is present, so the badge keeps the previous count
    // through the morning before today's note exists.
    const anchor = new Date(today);
    const todayISO = fmtDateISO(anchor);
    let cur = new Date(anchor);
    if (!have.has(todayISO)) {
      cur.setDate(cur.getDate() - 1);
      if (!have.has(fmtDateISO(cur))) return 0;
    }
    let count = 0;
    while (have.has(fmtDateISO(cur))) {
      count += 1;
      cur.setDate(cur.getDate() - 1);
    }
    return count;
  });

  // Total word count across all loaded jot bodies. Whitespace split is
  // good enough — this is a glanceable metric, not an editor stat.
  let loadedWords = $derived.by(() => {
    let n = 0;
    for (const j of jots) {
      const body = j.body ?? '';
      if (!body) continue;
      const matches = body.match(/\S+/g);
      if (matches) n += matches.length;
    }
    return n;
  });

  function formatCount(n: number): string {
    if (n >= 1000) return `${(n / 1000).toFixed(n >= 10_000 ? 0 : 1)}k`;
    return String(n);
  }

  // ── handlers ──────────────────────────────────────────────────────
  function handleWikilink(target: string) {
    // Naive: same logic as note-detail page — try as-is, else treat as
    // a title and append .md. The server will 404 for missing notes;
    // the user lands on whatever the route can resolve.
    const path = target.endsWith('.md') ? target : target + '.md';
    goto(`/notes/${encodeURIComponent(path)}`);
  }

  function jumpToDate(e: Event) {
    const v = (e.target as HTMLInputElement).value;
    if (!v) return;
    const path = dailyFolder ? `${dailyFolder}/${v}.md` : `${v}.md`;
    goto(`/notes/${encodeURIComponent(path)}`);
  }

  async function runSearch() {
    const q = searchText.trim();
    if (!q) {
      searchResults = [];
      return;
    }
    searching = true;
    try {
      const r = await api.listNotes({ folder: dailyFolder, q, limit: 10 });
      searchResults = r.notes;
    } catch (e) {
      toast.error('search failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      searching = false;
    }
  }

  function clearSearch() {
    searchText = '';
    searchResults = [];
  }

  function openToday() {
    const t = new Date();
    const ds = `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`;
    const path = dailyFolder ? `${dailyFolder}/${ds}.md` : `${ds}.md`;
    goto(`/notes/${encodeURIComponent(path)}`);
  }

  // Quick-jot composer — Amplenote-style "fire a thought into today"
  // without leaving the feed. Appends a timestamped line under a
  // `## Jots` section in today's daily, creating the section on first
  // use. The WS note.changed event then re-fetches today's jot in
  // the feed automatically.
  let composerText = $state('');
  let composerBusy = $state(false);
  let composerEl = $state<HTMLTextAreaElement | undefined>();

  function appendUnderJotsSection(body: string, line: string): string {
    // Find the `## Jots` heading; if present, splice the line in just
    // below it (after any existing list items the user has there). If
    // missing, append the section to the end of the document.
    const lines = body.split('\n');
    const idx = lines.findIndex((l) => /^##\s+Jots\b/i.test(l.trim()));
    if (idx === -1) {
      const sep = body.endsWith('\n') ? '' : '\n';
      return body + `${sep}\n## Jots\n${line}\n`;
    }
    // Walk past the heading to the end of the section (next `## ` or EOF).
    let end = lines.length;
    for (let i = idx + 1; i < lines.length; i++) {
      if (/^##\s+/.test(lines[i].trim())) {
        end = i;
        break;
      }
    }
    // Insert before `end`, trimming trailing empty lines so the new
    // line sits flush with the section content.
    let insertAt = end;
    while (insertAt > idx + 1 && lines[insertAt - 1].trim() === '') insertAt--;
    lines.splice(insertAt, 0, line);
    return lines.join('\n');
  }

  async function submitJot() {
    const text = composerText.trim();
    if (!text || composerBusy) return;
    composerBusy = true;
    try {
      const note = await api.daily('today');
      const t = new Date();
      const hh = String(t.getHours()).padStart(2, '0');
      const mm = String(t.getMinutes()).padStart(2, '0');
      // Multi-line input collapses to "; " separators so the appended
      // line stays a single bullet. Original line breaks are preserved
      // by markdown viewers since the line ends with a bullet.
      const flat = text.replace(/\n+/g, '; ');
      const newBody = appendUnderJotsSection(note.body ?? '', `- ${hh}:${mm} — ${flat}`);
      await api.putNote(note.path, {
        frontmatter: note.frontmatter ?? undefined,
        body: newBody
      });
      composerText = '';
      toast.success('jot saved');
      // WS will re-fetch; queue an immediate optimistic refetch too in
      // case the WS round-trip lags.
      const today = `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`;
      scheduleRefetch(today);
      composerEl?.focus();
    } catch (e) {
      toast.error('failed to add jot: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      composerBusy = false;
    }
  }

  // ── lifecycle ─────────────────────────────────────────────────────
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

  onMount(() => {
    // Resolve the daily folder so jump-to-day + WS scoping work.
    api
      .getConfig()
      .then((c) => {
        dailyFolder = (c.daily_notes_folder ?? '').replace(/\/+$/, '');
      })
      .catch(() => {
        // No config endpoint or failure → assume vault root, which is
        // the default-config behavior anyway.
        dailyFolder = '';
      });

    loadMore();

    // Hook up the IntersectionObserver once the sentinel is in the DOM.
    const setupObserver = () => {
      if (!sentinel) return;
      observer = new IntersectionObserver(
        (entries) => {
          for (const e of entries) {
            if (e.isIntersecting) loadMore();
          }
        },
        { rootMargin: '400px' }
      );
      observer.observe(sentinel);
    };
    // microtask delay so the bind:this has resolved
    queueMicrotask(setupObserver);

    const offWs = onWsEvent((ev) => {
      if (ev.type !== 'note.changed' && ev.type !== 'note.removed') return;
      const m = jotMatches(ev.path);
      if (!m) return;
      scheduleRefetch(m.date);
    });

    return () => {
      observer?.disconnect();
      offWs();
      for (const t of pendingRefetch.values()) clearTimeout(t);
      pendingRefetch.clear();
    };
  });
</script>

<div class="h-full overflow-y-auto" id="jots-scroll">
  <div class="max-w-3xl mx-auto px-3 sm:px-5 lg:px-6 pt-2 pb-6">
    <!-- Title strip: tight one-row header — title + live counters.
         Replaces the old two-line h1+subtitle block, freeing ~40px
         of vertical for actual content. -->
    <header
      class="flex items-baseline gap-2 mb-1.5 text-[11px] text-dim border-b border-surface1 pb-1.5"
    >
      <span class="text-[13px] font-semibold uppercase tracking-[0.18em] text-text">Jots</span>
      {#if streakDays > 0}
        <span
          class="font-mono text-text"
          title="consecutive days ending today with a daily note loaded"
        >{streakDays}d streak</span>
      {/if}
      {#if jots.length > 0}
        <span class="opacity-50">·</span>
        <span class="font-mono" title="loaded across all pages">
          {formatCount(jots.length)} jots
          {#if allTags.length > 0}
            · {formatCount(allTags.length)} tags
          {/if}
          {#if loadedWords > 0}
            · {formatCount(loadedWords)} words
          {/if}
        </span>
      {/if}
      <span class="ml-auto opacity-60 hidden sm:inline font-mono">? for shortcuts</span>
    </header>

    <!-- Toolbar (sticky) — compact one-row controls. Wraps on narrow
         viewports but stays as dense as possible at desktop width. -->
    <div
      class="sticky top-0 z-20 -mx-3 sm:-mx-5 lg:-mx-6 px-3 sm:px-5 lg:px-6 py-1.5 mb-2 bg-base border-b border-surface1"
    >
      <div class="flex flex-wrap items-center gap-1.5">
        <input
          type="date"
          onchange={jumpToDate}
          title="jump to date"
          class="bg-mantle border border-surface1 rounded px-1.5 py-0.5 text-[11px] text-text focus:outline-none focus:border-primary"
        />
        <div class="flex-1 min-w-[10rem] flex items-center gap-0.5">
          <input
            type="text"
            bind:value={searchText}
            onkeydown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                runSearch();
              } else if (e.key === 'Escape') {
                clearSearch();
              }
            }}
            placeholder="search jots…"
            class="flex-1 bg-mantle border border-surface1 rounded px-1.5 py-0.5 text-[11px] text-text placeholder-dim focus:outline-none focus:border-primary"
          />
          {#if searchText}
            <button
              type="button"
              onclick={clearSearch}
              aria-label="clear search"
              class="text-[11px] text-dim hover:text-text px-1"
            >×</button>
          {/if}
        </div>
        {#if jots.length >= 5}
          <button
            type="button"
            onclick={detectThemes}
            disabled={aiBusy}
            class="text-[11px] px-1.5 py-0.5 rounded bg-surface1 text-text border border-surface2 hover:bg-surface2 disabled:opacity-50"
            title="ask AI to find recurring themes in your jots"
          >{aiBusy ? 'reading…' : 'themes'}</button>
        {/if}
        <button
          type="button"
          onclick={openToday}
          title="open today's daily note"
          class="text-[11px] px-1.5 py-0.5 rounded bg-surface0 text-subtext hover:bg-surface1"
        >today</button>
      </div>

      {#if aiBusy || aiThemes.length > 0 || aiError}
        <div class="mt-1.5 p-1.5 bg-surface1 border border-surface2 rounded">
          <div class="flex items-baseline gap-2 mb-1">
            <h3 class="text-[10px] uppercase tracking-wider text-text font-medium">recurring themes</h3>
            <span class="flex-1"></span>
            {#if aiBusy}
              <span class="text-[10px] text-dim italic">analysing…</span>
            {:else if aiThemes.length > 0}
              <button onclick={detectThemes} class="text-[10px] text-secondary hover:underline">regenerate</button>
            {/if}
            <button onclick={dismissThemes} class="text-[10px] text-dim hover:text-text">dismiss</button>
          </div>
          {#if aiError}
            <p class="text-[11px] text-error">{aiError}</p>
          {:else if aiThemes.length > 0}
            <div class="flex flex-wrap gap-1.5">
              {#each aiThemes as t (t.label)}
                <button
                  type="button"
                  onclick={() => applyTheme(t)}
                  class="text-[11px] px-2 py-0.5 rounded-full bg-mantle border border-surface1 hover:border-primary text-text"
                  title={`search: ${t.query}`}
                >{t.label}</button>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <!-- Filter strip: quick filters + hashtag chips. Tags are AND-
           combined — clicking adds a tag to the filter set, clicking
           again removes it. Quick filters (open tasks, last 7d/30d)
           are orthogonal and stack on top of the tag filter. -->
      {#if allTags.length > 0 || jots.length > 0}
        <div class="flex flex-wrap items-center gap-1 mt-1.5 text-[11px]">
          {#if hasAnyFilter}
            <button
              type="button"
              onclick={clearAllFilters}
              class="px-1.5 py-0.5 rounded bg-surface1 text-text hover:bg-surface2"
              title="clear every active filter"
            >clear ({visibleJots.length}/{jots.length})</button>
          {/if}
          <!-- Quick filters: orthogonal toggles, distinguished from
               tag chips by a leading dot so the user can tell them
               apart at a glance. -->
          <button
            type="button"
            onclick={() => (filterOpenTasks = !filterOpenTasks)}
            class="px-1.5 py-0.5 rounded {filterOpenTasks ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            title="only jots whose daily has open tasks"
          >· open tasks</button>
          <button
            type="button"
            onclick={() => (filterTimeframe = filterTimeframe === '7d' ? 'all' : '7d')}
            class="px-1.5 py-0.5 rounded {filterTimeframe === '7d' ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            title="last 7 days only"
          >· 7d</button>
          <button
            type="button"
            onclick={() => (filterTimeframe = filterTimeframe === '30d' ? 'all' : '30d')}
            class="px-1.5 py-0.5 rounded {filterTimeframe === '30d' ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            title="last 30 days only"
          >· 30d</button>
          {#if allTags.length > 0}
            <span class="text-dim opacity-50 mx-0.5">|</span>
          {/if}
          {#each allTags.slice(0, 24) as t}
            <button
              type="button"
              onclick={() => toggleTag(t)}
              class="px-1.5 py-0.5 rounded {activeTags.includes(t) ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            >#{t}</button>
          {/each}
          {#if allTags.length > 24}
            <span class="text-dim">+{allTags.length - 24} more</span>
          {/if}
        </div>
      {/if}

      {#if searchResults.length > 0}
        <div class="mt-1.5 bg-mantle border border-surface1 rounded p-1.5 max-h-64 overflow-y-auto">
          <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5 px-1">
            {searchResults.length} match{searchResults.length === 1 ? '' : 'es'}
          </div>
          <ul class="space-y-0.5">
            {#each searchResults as n (n.path)}
              <li>
                <a
                  href="/notes/{encodeURIComponent(n.path)}"
                  class="block px-2 py-1 rounded text-sm text-text hover:bg-surface0"
                >
                  <span class="font-medium">{n.title}</span>
                  <span class="text-xs text-dim ml-2">{n.path}</span>
                </a>
              </li>
            {/each}
          </ul>
        </div>
      {:else if searchText && !searching}
        <div class="mt-2 text-xs text-dim italic px-1">no matches — press Enter to search</div>
      {/if}
    </div>

    <!-- Quick-jot composer — Amplenote-style fire-and-forget input
         that appends a timestamped line to today's daily. The user
         doesn't navigate; the new content lands in the feed below.
         Single-row by default; expands as the user types thanks to
         the bottom-resize textarea and rows=1. -->
    <div class="mb-3 bg-surface0 border border-surface1 rounded focus-within:border-primary transition-colors">
      <div class="flex items-start gap-1.5 px-2 py-1.5">
        <textarea
          bind:this={composerEl}
          bind:value={composerText}
          onkeydown={(e) => {
            // Enter (without shift) submits; Shift+Enter inserts a newline.
            // Cmd/Ctrl+Enter also submits as a power-user convenience.
            if (e.key === 'Enter' && (!e.shiftKey || e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              submitJot();
            }
          }}
          placeholder="jot a thought — Enter saves, Shift+Enter newline"
          rows="1"
          disabled={composerBusy}
          class="flex-1 bg-transparent text-sm text-text placeholder-dim focus:outline-none resize-y disabled:opacity-50 leading-snug"
        ></textarea>
        <button
          type="button"
          onclick={submitJot}
          disabled={composerBusy || !composerText.trim()}
          class="text-[11px] px-2 py-1 rounded bg-primary text-on-primary font-medium hover:opacity-90 disabled:opacity-40 shrink-0"
        >{composerBusy ? '…' : 'add'}</button>
      </div>
    </div>

    {#if error}
      <div class="text-sm text-error mb-4 p-3 bg-surface0 border border-error rounded">
        {error}
      </div>
    {/if}

    <!-- First-paint skeleton: 3 placeholder cards while the first page lands. -->
    {#if jots.length === 0 && loading}
      <div class="space-y-3">
        {#each [0, 1, 2] as _}
          <div class="bg-surface0 border border-surface1 rounded p-2.5">
            <Skeleton class="h-4 w-36 mb-2" />
            <Skeleton class="h-4 w-full mb-1" />
            <Skeleton class="h-4 w-5/6 mb-1" />
            <Skeleton class="h-4 w-3/4" />
          </div>
        {/each}
      </div>
    {:else if jots.length === 0 && done}
      <EmptyState
        icon="📓"
        title="No daily notes yet"
        description="Once you start writing dailies, they show up here — newest at the top, infinite scroll all the way back."
      >
        {#snippet action()}
          <button
            onclick={openToday}
            class="px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium"
          >
            Open today's daily
          </button>
        {/snippet}
      </EmptyState>
    {:else}
      {#if hasAnyFilter && visibleJots.length === 0}
        <p class="text-xs text-dim italic mb-3">
          No jots match the active filter{activeTags.length + (filterOpenTasks ? 1 : 0) + (filterTimeframe !== 'all' ? 1 : 0) === 1 ? '' : 's'}.
          {#if activeTags.length > 0}
            Tags: {#each activeTags as t, i}<span class="text-text">#{t}</span>{i < activeTags.length - 1 ? ', ' : ''}{/each}.
          {/if}
          Keep scrolling to load older dailies, or
          <button type="button" onclick={clearAllFilters} class="underline hover:text-text">clear filters</button>.
        </p>
      {/if}
      <ul class="space-y-3">
        {#each visibleJots as jot (jot.path)}
          <li data-jot-date={jot.date}>
            <article>
              <header
                class="sticky top-[2.5rem] z-10 -mx-1 px-1 py-1 bg-base flex items-baseline gap-2 mb-1.5 border-b border-surface1/60"
              >
                <h2 class="text-sm font-semibold text-text">
                  {relativeLabel(jot.date, today)}
                </h2>
                <span class="text-[11px] text-dim hidden sm:inline font-mono">{jot.date}</span>
                {#if jot.openTasks > 0}
                  <span
                    class="text-[10px] px-1 py-0.5 rounded bg-surface1 text-text font-mono"
                    title="{jot.openTasks} open task{jot.openTasks === 1 ? '' : 's'} in this daily"
                  >{jot.openTasks}☐</span>
                {/if}
                <!-- Inline activity counts. Reads from the prefetched
                     dayActivityCache populated as soon as the page
                     loads; shows nothing while the request is in
                     flight so the header doesn't shift. -->
                {@const sum = summarize(dayActivityCache[jot.date])}
                {#if sum.total > 0}
                  <span class="flex items-baseline gap-1 text-[10px] font-mono text-dim">
                    {#if sum.events > 0}
                      <span class="text-text" title="{sum.events} calendar event{sum.events === 1 ? '' : 's'}">{sum.events}cal</span>
                    {/if}
                    {#if sum.tasksDone > 0}
                      <span class="text-text" title="{sum.tasksDone} task{sum.tasksDone === 1 ? '' : 's'} completed">{sum.tasksDone}✓</span>
                    {/if}
                    {#if sum.tasksCreated > 0}
                      <span title="{sum.tasksCreated} task{sum.tasksCreated === 1 ? '' : 's'} created">+{sum.tasksCreated}</span>
                    {/if}
                    {#if sum.notes > 0}
                      <span title="{sum.notes} note{sum.notes === 1 ? '' : 's'} created">{sum.notes}n</span>
                    {/if}
                  </span>
                {/if}
                <a
                  href="/notes/{encodeURIComponent(jot.path)}"
                  class="ml-auto text-[11px] text-text hover:underline opacity-70 hover:opacity-100"
                >open →</a>
              </header>
              <div class="bg-surface0 border border-surface1 rounded p-2.5">
                {#if jot.body.trim()}
                  <MarkdownRenderer body={jot.body} onWikilink={handleWikilink} />
                {:else}
                  <p class="text-xs text-dim italic">empty</p>
                {/if}
              </div>

              <!-- What happened that day — collapsed Amplenote-style
                   overview of every item created/completed/touched on
                   this date across the vault. Loads lazily on first
                   open so long scrolls don't N+1 the API. -->
              <details
                class="mt-1 bg-surface0 border border-surface1 rounded text-sm"
                ontoggle={(e) => {
                  if ((e.currentTarget as HTMLDetailsElement).open) {
                    loadDayActivity(jot.date);
                  }
                }}
              >
                <summary class="cursor-pointer px-2 py-1 text-[10px] uppercase tracking-[0.18em] text-dim hover:text-text select-none">
                  what happened that day
                </summary>
                <div class="px-2.5 pb-2.5 pt-1">
                  {#if dayActivityLoading[jot.date] && dayActivityCache[jot.date] === undefined}
                    <p class="text-xs text-dim italic">loading…</p>
                  {:else if (dayActivityCache[jot.date]?.length ?? 0) === 0}
                    {#if dayActivityCache[jot.date] !== undefined}
                      <p class="text-xs text-dim italic">No tracked activity on this day.</p>
                    {/if}
                  {:else}
                    {#each bucketize(dayActivityCache[jot.date] ?? []) as bucket (bucket.kind)}
                      <div class="mb-2 last:mb-0">
                        <h4 class="text-[10px] uppercase tracking-[0.18em] text-text font-medium mb-0.5">
                          {bucket.label} <span class="text-dim font-normal">({bucket.items.length})</span>
                        </h4>
                        <ul>
                          {#each bucket.items as it (it.kind + ':' + (it.target_id ?? it.path ?? it.title) + ':' + it.at)}
                            <li class="flex items-baseline gap-1.5 text-[11px] leading-relaxed">
                              <span class="text-dim font-mono w-9 shrink-0">{activityTime(it.at)}</span>
                              <a
                                href={activityHref(it)}
                                class="text-text hover:underline truncate"
                              >{it.title}</a>
                              {#if it.detail}
                                <span class="text-dim truncate">· {it.detail}</span>
                              {/if}
                            </li>
                          {/each}
                        </ul>
                      </div>
                    {/each}
                  {/if}
                </div>
              </details>
            </article>
          </li>
        {/each}
      </ul>

      <!-- Sentinel: when this enters the viewport, load the next page. -->
      <div bind:this={sentinel} class="h-12 mt-4 flex items-center justify-center text-xs text-dim">
        {#if loading}
          loading more…
        {:else if done}
          {jots.length} jot{jots.length === 1 ? '' : 's'} · end of feed
        {/if}
      </div>
    {/if}
  </div>
</div>
