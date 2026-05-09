<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import {
    api,
    type Venture,
    type Project,
    type Goal,
    type Deadline,
    type PrayerIntention,
    type Note
  } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { daysUntil } from '$lib/deadlines/util';
  import Skeleton from '$lib/components/Skeleton.svelte';

  // Venture detail page — single aggregation view answering "what's
  // the current state of this venture, all in one place?". The
  // /ventures list page is the catalogue; this is the detail.
  //
  // Aggregation is client-side (multiple parallel API calls + filter)
  // because:
  //   - all four sources are already loaded for other pages, so the
  //     browser usually serves them from cache;
  //   - a server-side rollup endpoint would be one more thing to keep
  //     in sync with the underlying schemas;
  //   - the cost is one extra network round-trip vs. four — not the
  //     bottleneck for the data sizes we're dealing with.
  // If perf becomes a problem we can collapse to a /ventures/{name}
  // server-decorated response without touching this page's render
  // logic.
  //
  // Sub-tabs (overview / projects / goals / links / notes) keep the
  // long-tail content out of the initial scan path. The default tab
  // (overview) shows the metric strip + progress + everything in
  // small "preview" form, so power users who want to scan everything
  // at once get it on one screen; deep dives switch tabs.

  type Tab = 'overview' | 'projects' | 'goals' | 'links' | 'notes';

  let venture = $state<Venture | null>(null);
  let projects = $state<Project[]>([]);
  let goals = $state<Goal[]>([]);
  let deadlines = $state<Deadline[]>([]);
  let intentions = $state<PrayerIntention[]>([]);
  let linkedNotes = $state<Note[]>([]);
  let loading = $state(false);
  let notFound = $state(false);
  let tab = $state<Tab>('overview');

  // ----- AI summary state -----
  // The "Summarize" button hits chatStream with a compact JSON snapshot
  // of the venture + its rolled-up projects/goals/deadlines. The model
  // returns prose; we render it as plain paragraphs (no markdown
  // parsing — keeps this page small, and the model is prompted for
  // plain prose). Audit-gated automatically because chatStream goes
  // through /chat/stream → gateChat + auditChat.
  let aiBusy = $state(false);
  let aiText = $state('');
  let aiError = $state('');
  let aiAbort: AbortController | null = null;

  // The route param is the raw venture name (URL-encoded). We
  // case-insensitive-match on the server (ventures.Find) but keep
  // the user's original casing in the displayed name; the lookup
  // here is exact-equality on the decoded segment.
  let name = $derived(decodeURIComponent($page.params.name ?? ''));

  async function load() {
    if (!name) return;
    loading = true;
    notFound = false;
    try {
      // Fetch venture + the four linked-entity lists in parallel.
      // Each list endpoint we already use elsewhere, so the browser
      // typically has the response in cache. Promise.allSettled so
      // a single failing module (deadlines disabled, prayer disabled)
      // doesn't take the whole page down.
      const [vRes, pRes, gRes, dRes, iRes, nRes] = await Promise.allSettled([
        api.getVenture(name),
        api.listProjects(),
        api.listGoals(),
        api.tryListDeadlines(),
        api.listPrayer(),
        // Notes search by venture name — the server's full-text index
        // surfaces notes that mention the name in body or frontmatter.
        // Best-effort; if the search endpoint is unavailable we just
        // hide the Notes tab content.
        api.listNotes({ q: name, limit: 30 })
      ]);

      if (vRes.status !== 'fulfilled') {
        notFound = true;
        return;
      }
      venture = vRes.value;

      // Filter each list by venture (case-insensitive — the server
      // does the same on Find, so a project tagged with lowercase
      // venture name still rolls up to the canonical record).
      const lowerName = name.toLowerCase();
      const allProjects = pRes.status === 'fulfilled' ? pRes.value.projects : [];
      const allGoals = gRes.status === 'fulfilled' ? gRes.value.goals : [];
      const allDeadlines = dRes.status === 'fulfilled' ? (dRes.value ?? []) : [];
      const allIntentions = iRes.status === 'fulfilled' ? iRes.value.intentions : [];
      const allNotes = nRes.status === 'fulfilled' ? nRes.value.notes : [];

      projects = allProjects.filter((p) => (p.venture ?? '').toLowerCase() === lowerName);
      goals = allGoals.filter((g) => (g.venture ?? '').toLowerCase() === lowerName);
      deadlines = allDeadlines.filter((d) => (d.venture ?? '').toLowerCase() === lowerName);
      intentions = allIntentions.filter((p) => (p.venture ?? '').toLowerCase() === lowerName);
      // Notes don't have a structured venture field — full-text match
      // is the best signal we have, and we trust the server's relevance
      // ordering. We trim to the top 12 so the tab stays scannable.
      linkedNotes = allNotes.slice(0, 12);
    } catch (e) {
      toast.error('failed to load venture: ' + (errorMessage(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/ventures.json') load();
      if (ev.type.startsWith('project.')) load();
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') load();
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') load();
      if (ev.type === 'state.changed' && ev.path === '.granit/prayer/intentions.json') load();
    });
    const onVisible = () => {
      if (document.visibilityState === 'visible') load();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
      aiAbort?.abort();
    };
  });

  // Re-fetch when the URL param changes (e.g. user navigates from one
  // venture to another). Reset AI summary too — it's per-venture.
  $effect(() => {
    void name;
    load();
    aiAbort?.abort();
    aiAbort = null;
    aiBusy = false;
    aiText = '';
    aiError = '';
    tab = 'overview';
  });

  // ----- Derived rollups -----

  let activeProjects = $derived(projects.filter((p) => (p.status ?? 'active') === 'active'));
  let pausedProjects = $derived(projects.filter((p) => p.status === 'paused'));
  let activeGoals = $derived(goals.filter((g) => (g.status ?? 'active') === 'active'));
  let activeDeadlines = $derived(
    deadlines.filter((d) => d.status !== 'met' && d.status !== 'cancelled')
  );
  let activeIntentions = $derived(intentions.filter((p) => p.status === 'praying'));

  // Aggregate task counts across the venture's projects — a single
  // "23 open · 7 done" line at the top of the page is the fastest
  // read on overall momentum.
  let aggregateTasksOpen = $derived(
    projects.reduce((acc, p) => acc + ((p.tasksTotal ?? 0) - (p.tasksDone ?? 0)), 0)
  );
  let aggregateTasksDone = $derived(projects.reduce((acc, p) => acc + (p.tasksDone ?? 0), 0));

  // Average progress across active projects (each project's progress
  // is already derived server-side from its goals + tasks).
  let aggregateProgress = $derived.by(() => {
    if (activeProjects.length === 0) return 0;
    const sum = activeProjects.reduce((acc, p) => acc + (p.progress ?? 0), 0);
    return sum / activeProjects.length;
  });

  // Next deadline — the one with the smallest non-negative daysUntil
  // among active deadlines. Used in the hero metric tile.
  let nextDeadline = $derived.by(() => {
    if (activeDeadlines.length === 0) return null;
    return [...activeDeadlines].sort((a, b) => daysUntil(a.date) - daysUntil(b.date))[0];
  });

  function colorVar(c?: string): string {
    const map: Record<string, string> = {
      red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
      blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
      peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
      lavender: 'primary', flamingo: 'error'
    };
    return `var(--color-${map[c ?? ''] ?? 'secondary'})`;
  }

  function statusTone(s?: string): string {
    if (s === 'active') return 'success';
    if (s === 'paused') return 'warning';
    if (s === 'completed') return 'info';
    if (s === 'archived') return 'subtext';
    return 'subtext';
  }

  // Deadline countdown — short-form for the sidebar list. Mirrors the
  // /deadlines page formatter so the language is consistent across surfaces.
  function countdown(d: Deadline): string {
    if (d.status === 'met') return 'met';
    if (d.status === 'cancelled') return 'cancelled';
    const n = daysUntil(d.date);
    if (n === 0) return 'today';
    if (n === 1) return 'tomorrow';
    if (n === -1) return 'yesterday';
    if (n > 1) return `in ${n}d`;
    return `${-n}d ago`;
  }
  function deadlineTone(d: Deadline): string {
    if (d.status === 'met') return 'success';
    if (d.status === 'cancelled') return 'subtext';
    const n = daysUntil(d.date);
    if (n < 0) return 'error';
    if (n <= 3) return 'error';
    if (n <= 7) return 'warning';
    if (n <= 30) return 'info';
    return 'subtext';
  }

  // Tab button helpers — counts on each tab so the user can see what's
  // behind each one without flipping. Tabs hide entirely when the
  // venture has nothing in that bucket and there's nothing actionable
  // there (overview/projects/goals always show; links/notes hide on
  // empty since they're additive).
  let tabs = $derived.by(() => {
    const list: Array<{ id: Tab; label: string; count?: number }> = [
      { id: 'overview', label: 'Overview' },
      { id: 'projects', label: 'Projects', count: projects.length },
      { id: 'goals', label: 'Goals', count: goals.length },
      { id: 'links', label: 'Deadlines & prayer', count: activeDeadlines.length + activeIntentions.length }
    ];
    if (linkedNotes.length > 0) {
      list.push({ id: 'notes', label: 'Notes', count: linkedNotes.length });
    }
    return list;
  });

  // ----- AI summary -----
  // Build a compact JSON snapshot of everything the model needs to write
  // a concise narrative. We omit free-text fields that could explode
  // the prompt (project notes, goal review_log) and cap each list to
  // a sensible number — the model gets the shape, not the entire
  // history. notePath is undefined because the venture isn't a note;
  // chatStream's notePath param is just for note-aware context.
  function buildVentureSnapshot(): string {
    if (!venture) return '{}';
    return JSON.stringify(
      {
        venture: {
          name: venture.name,
          mission: venture.mission || undefined,
          description: venture.description || undefined,
          status: venture.status ?? 'active',
          tags: venture.tags ?? undefined
        },
        projects: projects.slice(0, 20).map((p) => ({
          name: p.name,
          status: p.status ?? 'active',
          kind: p.kind || undefined,
          description: p.description ? p.description.slice(0, 200) : undefined,
          progress: p.progress ?? undefined,
          tasksOpen: (p.tasksTotal ?? 0) - (p.tasksDone ?? 0),
          tasksDone: p.tasksDone ?? 0,
          dueDate: p.due_date || undefined,
          nextAction: p.next_action || undefined
        })),
        goals: goals.slice(0, 20).map((g) => ({
          title: g.title,
          status: g.status ?? 'active',
          targetDate: g.target_date || undefined,
          milestonesDone: (g.milestones ?? []).filter((m) => m.done).length,
          milestonesTotal: (g.milestones ?? []).length
        })),
        deadlines: activeDeadlines.slice(0, 10).map((d) => ({
          title: d.title,
          date: d.date,
          daysUntil: daysUntil(d.date),
          importance: d.importance || undefined
        })),
        prayerIntentions: activeIntentions.slice(0, 10).map((p) => ({
          text: p.text.slice(0, 160),
          startedAt: p.started_at || undefined
        }))
      },
      null,
      2
    );
  }

  async function summarize() {
    if (!venture || aiBusy) return;
    aiAbort?.abort();
    aiAbort = new AbortController();
    aiBusy = true;
    aiError = '';
    aiText = '';
    const snap = buildVentureSnapshot();
    const system =
      'You are a concise venture analyst. The user will give you a JSON snapshot of one venture (mission, projects with progress, goals with milestones, upcoming deadlines, prayer intentions). Write a brief plain-prose status summary in 4-6 sentences. Lead with momentum (where things are moving), name 1-2 specific risks or near-term deadlines, and end with a single suggested next focus. Be specific — reference real names and numbers. No markdown, no bullets, no headers. Plain paragraphs only.';
    const user = `Venture snapshot:\n\n\`\`\`json\n${snap}\n\`\`\`\n\nWrite the status summary now.`;
    let buf = '';
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: (c) => {
            buf += c;
            aiText = buf;
          },
          onDone: () => {},
          onError: (err) => {
            aiError = err.message;
          }
        },
        aiAbort.signal
      );
    } finally {
      aiBusy = false;
      aiAbort = null;
    }
  }

  function dismissAI() {
    aiAbort?.abort();
    aiBusy = false;
    aiText = '';
    aiError = '';
  }

  // Status change for the venture itself — surfaced as a select beside
  // the status badge in the hero so the user can re-bucket from the
  // detail page without bouncing back to /ventures.
  async function changeStatus(next: 'active' | 'paused' | 'archived') {
    if (!venture || (venture.status ?? 'active') === next) return;
    try {
      const updated = await api.patchVenture(venture.name, { status: next });
      venture = updated;
      toast.success(`${venture.name} → ${next}`);
    } catch (err) {
      toast.error('update failed: ' + (errorMessage(err)));
    }
  }

  // Note title fallback — listNotes returns title from frontmatter if
  // present, else basename. We strip ".md" defensively.
  function noteTitle(n: Note): string {
    if (n.title && n.title.trim() !== '') return n.title;
    const base = n.path.split('/').pop() ?? n.path;
    return base.replace(/\.md$/i, '');
  }

  function noteBodyExcerpt(n: Note): string {
    if (!n.body) return '';
    // Find the first occurrence of the venture name (case-insensitive)
    // and return a window around it. Falls back to the head of the
    // body when no match (possible if the match is in frontmatter).
    const lower = n.body.toLowerCase();
    const idx = lower.indexOf(name.toLowerCase());
    const start = idx >= 0 ? Math.max(0, idx - 40) : 0;
    return n.body.slice(start, start + 200).replace(/\s+/g, ' ').trim();
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    {#if loading && !venture}
      <!-- Skeleton hero so the page doesn't reflow when data arrives. -->
      <div class="mb-6 flex items-start gap-3">
        <div class="w-9 h-9 flex-shrink-0"></div>
        <Skeleton class="w-3 h-3 rounded-full mt-3" />
        <div class="flex-1 space-y-2">
          <Skeleton class="h-7 w-1/2" />
          <Skeleton class="h-4 w-3/4" />
          <Skeleton class="h-3 w-2/3" />
        </div>
      </div>
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 mb-6">
        {#each [0, 1, 2, 3] as i (i)}
          <Skeleton class="h-16 rounded" />
        {/each}
      </div>
      <Skeleton class="h-2 w-full rounded-full" />
    {:else if notFound}
      <div class="bg-surface0 border border-error/30 rounded-lg p-6 text-center">
        <p class="text-sm text-text mb-2">No venture named <strong>{name}</strong> found.</p>
        <a href="/ventures" class="text-sm text-secondary hover:underline">← back to ventures</a>
      </div>
    {:else if venture}
      <!-- Header / hero — colored bar, name, mission, description,
           status pill (with inline change), URL, tags. The hero leans
           heavily on whitespace; the metric strip below is where the
           dense data lives. -->
      <header class="mb-6">
        <div class="flex items-start gap-3">
          <a
            href="/ventures"
            aria-label="back to ventures"
            class="flex-shrink-0 w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded -ml-1"
          >
            <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </a>
          <span
            class="w-3 h-3 rounded-full flex-shrink-0 mt-3"
            style="background: {colorVar(venture.color)}"
            aria-hidden="true"
          ></span>
          <div class="flex-1 min-w-0">
            <h1 class="text-2xl sm:text-3xl font-semibold text-text break-words">{venture.name}</h1>
            {#if venture.mission}
              <p class="text-sm sm:text-base text-subtext italic mt-1 break-words">{venture.mission}</p>
            {/if}
            {#if venture.description}
              <p class="text-sm text-subtext mt-2 break-words">{venture.description}</p>
            {/if}
            <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-dim mt-3">
              <!-- Status pill is also the inline change control —
                   tapping the select fires patchVenture. The pill
                   styling stays consistent with the cards page. -->
              <label
                class="px-2 py-0.5 rounded uppercase tracking-wider text-[10px] inline-flex items-center gap-1 cursor-pointer"
                style="background: color-mix(in srgb, var(--color-{statusTone(venture.status)}) 14%, transparent); color: var(--color-{statusTone(venture.status)});"
                title="Change status"
              >
                <span aria-hidden="true">●</span>
                <select
                  value={venture.status ?? 'active'}
                  onchange={(e) => changeStatus((e.currentTarget as HTMLSelectElement).value as 'active' | 'paused' | 'archived')}
                  class="bg-transparent appearance-none outline-none text-[10px] uppercase tracking-wider cursor-pointer"
                  aria-label="Venture status"
                  style="color: inherit;"
                >
                  <option value="active">active</option>
                  <option value="paused">paused</option>
                  <option value="archived">archived</option>
                </select>
              </label>
              {#if venture.url}
                <a
                  href={venture.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-secondary hover:underline truncate font-mono text-[11px]"
                >↗ {venture.url.replace(/^https?:\/\//, '').replace(/\/$/, '')}</a>
              {/if}
              {#if venture.tags && venture.tags.length > 0}
                <span class="flex flex-wrap items-center gap-1">
                  {#each venture.tags as t}
                    <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                  {/each}
                </span>
              {/if}
              {#if venture.created_at}
                <span class="text-[11px]" title="created {venture.created_at}">since {venture.created_at}</span>
              {/if}
            </div>
          </div>
          <!-- AI summary trigger — kept top-right so it's discoverable
               without crowding the title row on small screens. The
               actual summary renders below the metric strip. -->
          <button
            onclick={summarize}
            disabled={aiBusy}
            class="hidden sm:inline-flex flex-shrink-0 items-center gap-1.5 px-2.5 py-1.5 text-xs bg-surface0 border border-surface1 hover:border-primary/40 rounded text-subtext hover:text-primary disabled:opacity-50"
            title="AI status summary"
          >
            <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" stroke-linecap="round" />
            </svg>
            <span>{aiBusy ? 'thinking…' : aiText ? 'regenerate' : 'AI summary'}</span>
          </button>
        </div>
      </header>

      <!-- Aggregate row — at-a-glance momentum signal. Active projects,
           goals, next deadline tile (tone-coded to urgency), prayer count. -->
      <section class="grid grid-cols-2 sm:grid-cols-4 gap-2 mb-6">
        <a
          href={`/projects?venture=${encodeURIComponent(venture.name)}`}
          class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors"
        >
          <div class="text-2xl font-semibold text-text tabular-nums leading-none">{activeProjects.length}</div>
          <div class="text-[11px] text-dim mt-1">Active projects</div>
        </a>
        <a
          href="/goals"
          class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors"
        >
          <div class="text-2xl font-semibold text-text tabular-nums leading-none">{activeGoals.length}</div>
          <div class="text-[11px] text-dim mt-1">Active goals</div>
        </a>
        {#if nextDeadline}
          {@const tone = deadlineTone(nextDeadline)}
          <a
            href={`/deadlines?venture=${encodeURIComponent(venture.name)}`}
            class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors"
            title={nextDeadline.title}
          >
            <div
              class="text-2xl font-semibold tabular-nums leading-none"
              style="color: var(--color-{tone});"
            >{countdown(nextDeadline)}</div>
            <div class="text-[11px] text-dim mt-1 truncate">Next: {nextDeadline.title}</div>
          </a>
        {:else}
          <a
            href={`/deadlines?venture=${encodeURIComponent(venture.name)}&new=1`}
            class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors"
          >
            <div class="text-2xl font-semibold text-text tabular-nums leading-none">—</div>
            <div class="text-[11px] text-dim mt-1">No deadlines</div>
          </a>
        {/if}
        <a
          href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
          class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors"
        >
          <div
            class="text-2xl font-semibold tabular-nums leading-none"
            style="color: {activeIntentions.length > 0 ? 'var(--color-secondary)' : 'var(--color-text)'};"
          >{activeIntentions.length}</div>
          <div class="text-[11px] text-dim mt-1">Praying for</div>
        </a>
      </section>

      <!-- Progress bar — averaged across active projects. Single
           anchor for "how are we doing on this venture". -->
      {#if activeProjects.length > 0}
        <section class="mb-6">
          <div class="flex items-baseline justify-between mb-1.5">
            <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Overall progress</h2>
            <span class="text-xs text-subtext font-mono">
              {Math.round(aggregateProgress * 100)}%
              {#if aggregateTasksOpen + aggregateTasksDone > 0}
                · <span class="text-dim">{aggregateTasksDone}/{aggregateTasksDone + aggregateTasksOpen} tasks</span>
              {/if}
            </span>
          </div>
          <div class="h-2 rounded-full bg-surface0 overflow-hidden">
            <div
              class="h-full transition-all"
              style="width: {Math.round(aggregateProgress * 100)}%; background: {colorVar(venture.color)}"
            ></div>
          </div>
        </section>
      {/if}

      <!-- AI summary panel — appears below the progress bar. Streams
           tokens as they arrive; the user can dismiss with the × or
           regenerate via the trigger button. We intentionally render
           plain prose (no markdown) — the prompt asks for it, and
           skipping a markdown lib keeps the page small. -->
      {#if aiText || aiBusy || aiError}
        <section
          class="mb-6 rounded-lg p-4 border"
          style="border-color: color-mix(in srgb, var(--color-primary) 30%, transparent); background: color-mix(in srgb, var(--color-primary) 4%, transparent);"
        >
          <div class="flex items-baseline justify-between gap-2 mb-2">
            <h2 class="text-xs uppercase tracking-wider text-primary font-medium flex items-center gap-1.5">
              <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" stroke-linecap="round" />
              </svg>
              <span>AI status summary</span>
            </h2>
            <button
              onclick={dismissAI}
              class="text-dim hover:text-text text-sm leading-none"
              aria-label="dismiss summary"
            >×</button>
          </div>
          {#if aiError}
            <p class="text-sm text-error">{aiError}</p>
          {:else if aiText}
            <p class="text-sm text-subtext leading-relaxed whitespace-pre-wrap break-words">{aiText}{#if aiBusy}<span class="inline-block w-1.5 h-3.5 bg-primary/60 align-middle ml-0.5 animate-pulse"></span>{/if}</p>
          {:else}
            <p class="text-sm text-dim italic">analyzing venture state…</p>
          {/if}
        </section>
      {/if}

      <!-- Sub-tabs — overview is the default and folds the four legacy
           sections into one tighter layout. The other tabs are deep
           views: Projects gets a richer per-project row (description
           + progress + task counts), Goals shows milestones with
           target dates, etc. The notes tab only appears when there
           are linked notes (search hit), so a fresh venture stays
           uncluttered. -->
      <nav class="flex flex-wrap gap-1 border-b border-surface1 mb-4 overflow-x-auto" aria-label="Venture sections">
        {#each tabs as t (t.id)}
          <button
            class="px-3 py-2 text-sm border-b-2 -mb-px flex items-center gap-1.5 whitespace-nowrap transition-colors {tab === t.id ? 'border-primary text-text font-medium' : 'border-transparent text-subtext hover:text-text'}"
            onclick={() => (tab = t.id)}
            aria-current={tab === t.id ? 'page' : undefined}
          >
            <span>{t.label}</span>
            {#if t.count !== undefined && t.count > 0}
              <span class="text-[10px] tabular-nums px-1.5 py-0.5 rounded bg-surface1 text-dim">{t.count}</span>
            {/if}
          </button>
        {/each}
      </nav>

      {#if tab === 'overview'}
        <!-- Overview tab — compact preview of every section so the
             user can scan everything at once. Each preview links to
             its dedicated tab for the deep view. -->
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <!-- Projects preview -->
          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Projects · {activeProjects.length}</h2>
              {#if projects.length > 0}
                <button class="text-xs text-secondary hover:underline" onclick={() => (tab = 'projects')}>see all →</button>
              {:else}
                <a
                  href={`/projects?venture=${encodeURIComponent(venture.name)}`}
                  class="text-xs text-secondary hover:underline"
                >+ new →</a>
              {/if}
            </div>
            {#if activeProjects.length === 0 && pausedProjects.length === 0}
              <p class="text-xs text-dim italic px-2.5">No projects linked yet.</p>
            {:else}
              <ul class="space-y-1.5">
                {#each activeProjects.slice(0, 4) as p (p.name)}
                  {@const progress = Math.round((p.progress ?? 0) * 100)}
                  <li>
                    <a
                      href={`/projects?p=${encodeURIComponent(p.name)}`}
                      class="block px-3 py-2 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors group"
                    >
                      <div class="flex items-baseline gap-2">
                        <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(p.color)}"></span>
                        <span class="text-sm text-text flex-1 min-w-0 truncate group-hover:text-primary">{p.name}</span>
                        {#if p.kind}
                          <span class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-primary/10 text-primary flex-shrink-0">{p.kind}</span>
                        {/if}
                      </div>
                      <div class="flex items-center gap-2 mt-1.5">
                        <div class="flex-1 h-1 rounded-full bg-mantle overflow-hidden">
                          <div class="h-full" style="width: {progress}%; background: {colorVar(p.color)}"></div>
                        </div>
                        <span class="text-[10px] text-dim font-mono w-9 text-right">{progress}%</span>
                      </div>
                    </a>
                  </li>
                {/each}
                {#if activeProjects.length > 4}
                  <li>
                    <button class="text-[11px] text-dim hover:text-text px-2" onclick={() => (tab = 'projects')}>
                      + {activeProjects.length - 4} more active
                    </button>
                  </li>
                {/if}
                {#if pausedProjects.length > 0}
                  <li class="text-[11px] text-dim italic px-2 pt-1">
                    + {pausedProjects.length} paused
                  </li>
                {/if}
              </ul>
            {/if}
          </section>

          <!-- Goals preview -->
          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Goals · {activeGoals.length}</h2>
              {#if goals.length > 0}
                <button class="text-xs text-secondary hover:underline" onclick={() => (tab = 'goals')}>see all →</button>
              {:else}
                <a href="/goals" class="text-xs text-secondary hover:underline">+ new →</a>
              {/if}
            </div>
            {#if activeGoals.length === 0}
              <p class="text-xs text-dim italic px-2.5">No goals linked yet.</p>
            {:else}
              <ul class="space-y-1.5">
                {#each activeGoals.slice(0, 4) as g (g.id)}
                  {@const ms = g.milestones ?? []}
                  {@const total = ms.length}
                  {@const done = ms.filter((m) => m.done).length}
                  {@const pct = total === 0 ? (g.status === 'completed' ? 100 : 0) : Math.round((done / total) * 100)}
                  <li>
                    <a
                      href={`/goals?focus=${encodeURIComponent(g.id)}`}
                      class="block px-3 py-2 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors group"
                    >
                      <div class="flex items-baseline gap-2">
                        <span class="text-sm text-text flex-1 min-w-0 truncate group-hover:text-primary">{g.title}</span>
                        {#if g.target_date}
                          <span class="text-[10px] text-dim font-mono flex-shrink-0">🎯 {g.target_date}</span>
                        {/if}
                      </div>
                      {#if total > 0}
                        <div class="flex items-center gap-2 mt-1.5">
                          <div class="flex-1 h-1 rounded-full bg-mantle overflow-hidden">
                            <div class="h-full bg-primary" style="width: {pct}%"></div>
                          </div>
                          <span class="text-[10px] text-dim font-mono">{done}/{total}</span>
                        </div>
                      {/if}
                    </a>
                  </li>
                {/each}
                {#if activeGoals.length > 4}
                  <li>
                    <button class="text-[11px] text-dim hover:text-text px-2" onclick={() => (tab = 'goals')}>
                      + {activeGoals.length - 4} more
                    </button>
                  </li>
                {/if}
              </ul>
            {/if}
          </section>

          <!-- Deadlines preview -->
          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Deadlines · {activeDeadlines.length}</h2>
              <div class="flex items-center gap-2 text-xs">
                <a
                  href={`/deadlines?venture=${encodeURIComponent(venture.name)}&new=1`}
                  class="text-secondary hover:underline"
                >+ add</a>
                <a
                  href={`/deadlines?venture=${encodeURIComponent(venture.name)}`}
                  class="text-dim hover:text-text"
                >all →</a>
              </div>
            </div>
            {#if activeDeadlines.length === 0}
              <p class="text-xs text-dim italic px-2.5">No active deadlines.</p>
            {:else}
              <ul class="space-y-1">
                {#each activeDeadlines.slice(0, 6) as d (d.id)}
                  {@const tone = deadlineTone(d)}
                  <li>
                    <a
                      href={`/deadlines?venture=${encodeURIComponent(venture.name)}#${d.id}`}
                      class="flex items-baseline gap-2 px-2.5 py-1.5 rounded hover:bg-surface0 group"
                      style="border-left: 2px solid var(--color-{tone});"
                    >
                      <span class="text-sm text-text flex-1 truncate group-hover:text-primary">{d.title}</span>
                      {#if d.importance === 'critical'}
                        <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-error/15 text-error flex-shrink-0">crit</span>
                      {:else if d.importance === 'high'}
                        <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-warning/15 text-warning flex-shrink-0">high</span>
                      {/if}
                      <span class="text-xs tabular-nums flex-shrink-0" style="color: var(--color-{tone});">{countdown(d)}</span>
                    </a>
                  </li>
                {/each}
                {#if activeDeadlines.length > 6}
                  <li>
                    <a
                      href={`/deadlines?venture=${encodeURIComponent(venture.name)}`}
                      class="block px-2.5 py-1 text-[11px] text-dim hover:text-text"
                    >+ {activeDeadlines.length - 6} more</a>
                  </li>
                {/if}
              </ul>
            {/if}
          </section>

          <!-- Prayer intentions preview -->
          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Praying for · {activeIntentions.length}</h2>
              <div class="flex items-center gap-2 text-xs">
                <a
                  href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
                  class="text-secondary hover:underline"
                >+ add</a>
                <a
                  href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
                  class="text-dim hover:text-text"
                >all →</a>
              </div>
            </div>
            {#if activeIntentions.length === 0}
              <p class="text-xs text-dim italic px-2.5">
                <a href={`/prayer?venture=${encodeURIComponent(venture.name)}`} class="text-secondary hover:underline">Bring it before God</a> — what are you asking Him for in this venture?
              </p>
            {:else}
              <ul class="space-y-1.5">
                {#each activeIntentions.slice(0, 6) as p (p.id)}
                  <li class="px-2.5 py-1.5 bg-surface0 rounded">
                    <div class="text-sm text-text break-words">{p.text}</div>
                    {#if p.passage_ref || p.started_at}
                      <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 mt-0.5 text-[11px] text-dim">
                        {#if p.passage_ref}<span>📖 {p.passage_ref}</span>{/if}
                        {#if p.started_at}<span>since {p.started_at}</span>{/if}
                      </div>
                    {/if}
                  </li>
                {/each}
                {#if activeIntentions.length > 6}
                  <li class="text-[11px] text-dim px-2 italic">+ {activeIntentions.length - 6} more</li>
                {/if}
              </ul>
            {/if}
          </section>
        </div>

      {:else if tab === 'projects'}
        <!-- Projects tab — full list with description + status pill +
             progress + task counts. Active first, paused below in a
             muted block. -->
        {#if projects.length === 0}
          <div class="text-center py-10 text-sm text-dim">
            No projects linked to this venture yet.
            <div class="mt-2">
              <a
                href={`/projects?venture=${encodeURIComponent(venture.name)}`}
                class="text-secondary hover:underline"
              >Create one →</a>
            </div>
          </div>
        {:else}
          <ul class="space-y-2">
            {#each projects as p (p.name)}
              {@const progress = Math.round((p.progress ?? 0) * 100)}
              {@const tasksTotal = p.tasksTotal ?? 0}
              {@const tasksDone = p.tasksDone ?? 0}
              <li>
                <a
                  href={`/projects?p=${encodeURIComponent(p.name)}`}
                  class="block px-4 py-3 bg-surface0 border border-surface1 rounded-lg hover:border-primary/40 transition-colors group"
                >
                  <div class="flex items-start gap-2">
                    <span class="w-2 h-2 rounded-full flex-shrink-0 mt-1.5" style="background: {colorVar(p.color)}"></span>
                    <div class="flex-1 min-w-0">
                      <div class="flex items-baseline gap-2 flex-wrap">
                        <span class="text-sm font-medium text-text group-hover:text-primary truncate">{p.name}</span>
                        {#if p.kind}
                          <span class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-primary/10 text-primary">{p.kind}</span>
                        {/if}
                        <span
                          class="text-[10px] uppercase tracking-wider"
                          style="color: var(--color-{statusTone(p.status)});"
                        >{p.status ?? 'active'}</span>
                        {#if p.due_date}
                          <span class="text-[10px] text-dim font-mono ml-auto">due {p.due_date}</span>
                        {/if}
                      </div>
                      {#if p.description}
                        <p class="text-xs text-subtext line-clamp-2 mt-1">{p.description}</p>
                      {/if}
                      <div class="flex items-center gap-2 mt-2">
                        <div class="flex-1 h-1 rounded-full bg-mantle overflow-hidden">
                          <div class="h-full" style="width: {progress}%; background: {colorVar(p.color)}"></div>
                        </div>
                        <span class="text-[10px] text-dim font-mono w-9 text-right">{progress}%</span>
                        {#if tasksTotal > 0}
                          <span class="text-[10px] text-dim font-mono whitespace-nowrap">{tasksDone}/{tasksTotal} tasks</span>
                        {/if}
                      </div>
                      {#if p.next_action}
                        <p class="text-[11px] text-secondary mt-1.5">→ {p.next_action}</p>
                      {/if}
                    </div>
                  </div>
                </a>
              </li>
            {/each}
          </ul>
        {/if}

      {:else if tab === 'goals'}
        <!-- Goals tab — every goal with milestone breakdown. -->
        {#if goals.length === 0}
          <div class="text-center py-10 text-sm text-dim">
            No goals linked to this venture yet.
            <div class="mt-2">
              <a href="/goals" class="text-secondary hover:underline">Create one →</a>
            </div>
          </div>
        {:else}
          <ul class="space-y-2">
            {#each goals as g (g.id)}
              {@const ms = g.milestones ?? []}
              {@const total = ms.length}
              {@const done = ms.filter((m) => m.done).length}
              {@const pct = total === 0 ? (g.status === 'completed' ? 100 : 0) : Math.round((done / total) * 100)}
              <li>
                <a
                  href={`/goals?focus=${encodeURIComponent(g.id)}`}
                  class="block px-4 py-3 bg-surface0 border border-surface1 rounded-lg hover:border-primary/40 transition-colors group"
                >
                  <div class="flex items-baseline gap-2 flex-wrap">
                    <span class="text-sm font-medium text-text group-hover:text-primary flex-1 min-w-0 truncate">{g.title}</span>
                    <span
                      class="text-[10px] uppercase tracking-wider"
                      style="color: var(--color-{statusTone(g.status)});"
                    >{g.status ?? 'active'}</span>
                    {#if g.target_date}
                      <span class="text-[10px] text-dim font-mono">🎯 {g.target_date}</span>
                    {/if}
                  </div>
                  {#if g.description}
                    <p class="text-xs text-subtext line-clamp-2 mt-1">{g.description}</p>
                  {/if}
                  {#if total > 0}
                    <div class="flex items-center gap-2 mt-2">
                      <div class="flex-1 h-1 rounded-full bg-mantle overflow-hidden">
                        <div class="h-full bg-primary" style="width: {pct}%"></div>
                      </div>
                      <span class="text-[10px] text-dim font-mono whitespace-nowrap">{done}/{total} milestones</span>
                    </div>
                    <ul class="mt-2 space-y-0.5">
                      {#each ms.slice(0, 4) as m, i (i)}
                        <li class="flex items-center gap-2 text-xs">
                          <span aria-hidden="true" class="w-3 inline-flex justify-center" style="color: {m.done ? 'var(--color-success)' : 'var(--color-dim)'};">{m.done ? '✓' : '○'}</span>
                          <span class={m.done ? 'text-dim line-through' : 'text-subtext'}>{m.text}</span>
                          {#if m.due_date}
                            <span class="ml-auto text-[10px] text-dim font-mono">{m.due_date}</span>
                          {/if}
                        </li>
                      {/each}
                      {#if ms.length > 4}
                        <li class="text-[11px] text-dim ml-5">+ {ms.length - 4} more</li>
                      {/if}
                    </ul>
                  {/if}
                </a>
              </li>
            {/each}
          </ul>
        {/if}

      {:else if tab === 'links'}
        <!-- Links tab — deadlines + prayer side by side, each in full
             list form rather than the truncated overview preview. -->
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Deadlines · {activeDeadlines.length}</h2>
              <a
                href={`/deadlines?venture=${encodeURIComponent(venture.name)}&new=1`}
                class="text-xs text-secondary hover:underline"
              >+ add</a>
            </div>
            {#if activeDeadlines.length === 0}
              <p class="text-xs text-dim italic px-2.5">No active deadlines.</p>
            {:else}
              <ul class="space-y-1">
                {#each activeDeadlines as d (d.id)}
                  {@const tone = deadlineTone(d)}
                  <li>
                    <a
                      href={`/deadlines?venture=${encodeURIComponent(venture.name)}#${d.id}`}
                      class="flex items-baseline gap-2 px-2.5 py-2 rounded hover:bg-surface0 group"
                      style="border-left: 2px solid var(--color-{tone});"
                    >
                      <span class="text-sm text-text flex-1 truncate group-hover:text-primary">{d.title}</span>
                      {#if d.importance === 'critical'}
                        <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-error/15 text-error flex-shrink-0">crit</span>
                      {:else if d.importance === 'high'}
                        <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-warning/15 text-warning flex-shrink-0">high</span>
                      {/if}
                      <span class="text-xs tabular-nums flex-shrink-0" style="color: var(--color-{tone});">{countdown(d)}</span>
                    </a>
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Praying for · {activeIntentions.length}</h2>
              <a
                href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
                class="text-xs text-secondary hover:underline"
              >+ add</a>
            </div>
            {#if activeIntentions.length === 0}
              <p class="text-xs text-dim italic px-2.5">
                <a href={`/prayer?venture=${encodeURIComponent(venture.name)}`} class="text-secondary hover:underline">Bring it before God</a> — what are you asking Him for in this venture?
              </p>
            {:else}
              <ul class="space-y-1.5">
                {#each activeIntentions as p (p.id)}
                  <li class="px-2.5 py-2 bg-surface0 rounded">
                    <div class="text-sm text-text break-words">{p.text}</div>
                    {#if p.passage_ref || p.started_at}
                      <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 mt-0.5 text-[11px] text-dim">
                        {#if p.passage_ref}<span>📖 {p.passage_ref}</span>{/if}
                        {#if p.started_at}<span>since {p.started_at}</span>{/if}
                      </div>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </section>
        </div>

      {:else if tab === 'notes'}
        <!-- Notes tab — full-text-search hits for the venture name.
             Shows an excerpt around the match so the user can see why
             the note surfaced. Best-effort cross-link: notes don't
             have a structured venture field, so a hit on the name in
             body or frontmatter is our signal. -->
        {#if linkedNotes.length === 0}
          <p class="text-sm text-dim italic">No notes mention this venture yet.</p>
        {:else}
          <ul class="space-y-2">
            {#each linkedNotes as n (n.path)}
              {@const excerpt = noteBodyExcerpt(n)}
              <li>
                <a
                  href={`/notes/${encodeURI(n.path)}`}
                  class="block px-4 py-3 bg-surface0 border border-surface1 rounded-lg hover:border-primary/40 transition-colors group"
                >
                  <div class="flex items-baseline gap-2">
                    <span class="text-sm font-medium text-text group-hover:text-primary truncate flex-1 min-w-0">{noteTitle(n)}</span>
                    {#if n.modTime}
                      <span class="text-[10px] text-dim font-mono">{n.modTime.slice(0, 10)}</span>
                    {/if}
                  </div>
                  <p class="text-[11px] text-dim font-mono truncate mt-0.5">{n.path}</p>
                  {#if excerpt}
                    <p class="text-xs text-subtext line-clamp-2 mt-1">…{excerpt}…</p>
                  {/if}
                  {#if n.tags && n.tags.length > 0}
                    <div class="flex flex-wrap gap-1 mt-1.5">
                      {#each n.tags.slice(0, 5) as t}
                        <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                      {/each}
                    </div>
                  {/if}
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
    {/if}
  </div>
</div>
