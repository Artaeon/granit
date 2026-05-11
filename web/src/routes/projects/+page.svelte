<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, type Project, type Task , todayISO } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import ProjectDetail from '$lib/projects/ProjectDetail.svelte';
  import ProjectCreate from '$lib/projects/ProjectCreate.svelte';
  import ProjectTimeline from '$lib/projects/ProjectTimeline.svelte';
  import ProjectHeatmap from '$lib/projects/ProjectHeatmap.svelte';
  import ProjectKanban from '$lib/projects/ProjectKanban.svelte';
  import ProjectAgent from '$lib/projects/ProjectAgent.svelte';
  import ProjectDashboardPanel from '$lib/projects/ProjectDashboardPanel.svelte';
  import type { KanbanStatus } from '$lib/projects/kanbanGroup';
  import { isTypingTarget } from '$lib/util/isTypingTarget';
  import ProjectStatusBar from '$lib/projects/ProjectStatusBar.svelte';
  import VisionContextStrip from '$lib/components/VisionContextStrip.svelte';

  // Project Agent — conversational mutation engine for /projects.
  // Same architecture as TaskAgent: free-text intent → streamed
  // typed actions → accept per row → run-scoped undo. Shared
  // re-stream merge logic lives in $lib/agents/core.
  let agentOpen = $state(false);

  let projects = $state<Project[]>([]);
  let tasks = $state<Task[]>([]);
  let loading = $state(false);

  // ── Per-project momentum derivations ─────────────────────────────
  // Pull all tasks once on the list page so each card can render a
  // tiny 4-week sparkline + "this week" counts. Without this, the
  // ProjectDetail panel (right pane) was the only surface that
  // showed momentum — users browsing the list saw only a flat
  // milestone-progress bar. The data is already loaded for the
  // detail panel; surfacing it on the cards costs zero extra wire
  // calls and answers "which projects are alive" at a glance.
  const SPARK_WEEKS = 4;
  function isoWeekKey(d: Date): string {
    const t = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
    const day = (t.getUTCDay() + 6) % 7;
    t.setUTCDate(t.getUTCDate() - day + 3);
    const firstThu = new Date(Date.UTC(t.getUTCFullYear(), 0, 4));
    const week = 1 + Math.round((t.getTime() - firstThu.getTime()) / (7 * 24 * 60 * 60 * 1000));
    return `${t.getUTCFullYear()}-W${String(week).padStart(2, '0')}`;
  }
  function startOfIsoWeek(d: Date): Date {
    const t = new Date(d);
    const day = (t.getDay() + 6) % 7;
    t.setDate(t.getDate() - day);
    t.setHours(0, 0, 0, 0);
    return t;
  }
  function ymd(d: Date): string {
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }
  // Pre-compute the order of week keys for the sparkline so each
  // card doesn't redo this work.
  const sparkWeekOrder = $derived.by(() => {
    const start = startOfIsoWeek(new Date());
    const order: string[] = [];
    for (let i = SPARK_WEEKS - 1; i >= 0; i--) {
      const d = new Date(start);
      d.setDate(d.getDate() - i * 7);
      order.push(isoWeekKey(d));
    }
    return order;
  });
  // Map: projectName -> { spark: number[], scheduledThisWeek: number }
  const momentumByProject = $derived.by(() => {
    const out = new Map<string, { spark: number[]; scheduledThisWeek: number }>();
    const today = ymd(new Date());
    const monStart = ymd(startOfIsoWeek(new Date()));
    for (const p of projects) {
      out.set(p.name, { spark: new Array(SPARK_WEEKS).fill(0), scheduledThisWeek: 0 });
    }
    for (const t of tasks) {
      // Project membership: explicit projectId OR notePath under a
      // project's folder. Mirrors the matching ProjectDetail uses
      // so the sparkline + the panel's burn-up agree exactly.
      const matched: Project[] = [];
      for (const p of projects) {
        if (t.projectId === p.name) {
          matched.push(p);
          continue;
        }
        const folder = (p.folder ?? '').replace(/\/$/, '');
        if (folder && t.notePath.startsWith(folder + '/')) matched.push(p);
      }
      if (matched.length === 0) continue;
      // Completion → bump the matching week bucket.
      if (t.done && t.completedAt) {
        const k = isoWeekKey(new Date(t.completedAt));
        const idx = sparkWeekOrder.indexOf(k);
        if (idx >= 0) {
          for (const p of matched) {
            const m = out.get(p.name);
            if (m) m.spark[idx]++;
          }
        }
      }
      // Scheduled in current week → bump the count.
      if (!t.done && t.scheduledStart) {
        const day = t.scheduledStart.slice(0, 10);
        if (day >= monStart && day <= today) {
          for (const p of matched) {
            const m = out.get(p.name);
            if (m) m.scheduledThisWeek++;
          }
        }
      }
    }
    return out;
  });
  let q = $state('');
  let statusFilter = $state<'all' | 'active' | 'paused' | 'completed' | 'archived'>('active');
  let createOpen = $state(false);

  // ── Stalled-projects radar ───────────────────────────────────────
  // Scans active projects locally for stall signals — no completion
  // in N days, project mtime older than the threshold — then asks
  // the AI for a one-line "what could unblock this" suggestion per
  // stalled project. The local heuristic is the floor (we never
  // surface anything as stalled that is provably alive); the AI's
  // job is the qualitative "why" and the unblock idea, not the
  // detection itself. This split keeps the AI grounded in real
  // data and means a flaky AI response still produces a usable
  // dashboard (just without the unblock copy).
  //
  // Uses a single chatStream call with all stalled projects bundled
  // — N stalled projects = 1 AI call, not N. The model returns a
  // JSON array we parse and zip back to the list. JSON failure
  // falls back to showing the radar without unblock copy.
  type StalledRow = {
    name: string;
    color?: string;
    venture?: string;
    daysSinceCompletion: number | null;
    daysSinceUpdate: number | null;
    openTasks: number;
    overdueTasks: number;
    unblock?: string;
  };

  const STALL_DAYS = 14;

  let radarOpen = $state(false);
  let radarBusy = $state(false);
  let radarError = $state('');
  let radarRows = $state<StalledRow[]>([]);
  let radarAbort: AbortController | null = null;
  let radarRanAt = $state<string>('');

  // Local stall detection — independent of the AI so the dashboard
  // can render even when the AI is offline / Sabbath-blocked.
  const stalledLocally = $derived.by((): StalledRow[] => {
    const today = new Date();
    const out: StalledRow[] = [];
    for (const p of projects) {
      if ((p.status ?? 'active') !== 'active') continue;
      // Bucket this project's tasks (mirroring detail-panel match
      // logic: explicit projectId OR notePath under folder).
      const folder = (p.folder ?? '').replace(/\/$/, '');
      const matched = tasks.filter(
        (t) => t.projectId === p.name || (folder && t.notePath.startsWith(folder + '/'))
      );
      let lastCompletion: Date | null = null;
      let openCount = 0;
      let overdueCount = 0;
      for (const t of matched) {
        if (t.done && t.completedAt) {
          const d = new Date(t.completedAt);
          if (!Number.isNaN(d.getTime()) && (!lastCompletion || d > lastCompletion)) lastCompletion = d;
        }
        if (!t.done) {
          openCount++;
          if (t.dueDate) {
            const d = new Date(t.dueDate);
            if (!Number.isNaN(d.getTime()) && d.getTime() < today.getTime()) overdueCount++;
          }
        }
      }
      const daysSinceCompletion = lastCompletion
        ? Math.floor((today.getTime() - lastCompletion.getTime()) / 86400000)
        : null;
      const updatedAt = p.updated_at ? new Date(p.updated_at) : null;
      const daysSinceUpdate = updatedAt && !Number.isNaN(updatedAt.getTime())
        ? Math.floor((today.getTime() - updatedAt.getTime()) / 86400000)
        : null;

      // Stall criteria: an active project with EITHER no completions
      // in STALL_DAYS days (incl. never), OR no edits in STALL_DAYS
      // days. A project with overdue tasks but recent activity is
      // not "stalled" — it's just busy and behind. We require BOTH
      // signals to age out before flagging, otherwise a project
      // that just got created shows up as stalled (no completions
      // yet) which is noise.
      const completionStalled =
        daysSinceCompletion === null || daysSinceCompletion >= STALL_DAYS;
      const updateStalled =
        daysSinceUpdate === null || daysSinceUpdate >= STALL_DAYS;
      // Special case: a project with zero tasks at all is dead in
      // a different sense — surface it too.
      const empty = matched.length === 0;
      if ((completionStalled && updateStalled) || empty) {
        out.push({
          name: p.name,
          color: p.color,
          venture: p.venture,
          daysSinceCompletion,
          daysSinceUpdate,
          openTasks: openCount,
          overdueTasks: overdueCount
        });
      }
    }
    // Sort: most-stalled first (highest daysSinceCompletion, then
    // daysSinceUpdate). null = never-completed = sort to top.
    return out.sort((a, b) => {
      const ad = a.daysSinceCompletion ?? 9999;
      const bd = b.daysSinceCompletion ?? 9999;
      if (ad !== bd) return bd - ad;
      const au = a.daysSinceUpdate ?? 9999;
      const bu = b.daysSinceUpdate ?? 9999;
      return bu - au;
    });
  });

  async function runRadar() {
    if (radarBusy) return;
    const stalled = stalledLocally;
    radarBusy = true;
    radarError = '';
    radarRows = stalled.map((r) => ({ ...r })); // render rows immediately, AI fills in unblock
    radarAbort = new AbortController();
    // Local wall-clock HH:MM. toISOString() returns UTC which would
    // show the wrong time for any user not in UTC.
    radarRanAt = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });

    if (stalled.length === 0) {
      radarBusy = false;
      radarAbort = null;
      return;
    }

    // One compact JSON payload — the model gets the project name +
    // signals and returns a parallel array of unblock suggestions.
    // No verbose "tell me about each project" loop; one prompt, one
    // response, N suggestions.
    const payload = stalled.map((r) => ({
      name: r.name,
      days_since_completion: r.daysSinceCompletion,
      days_since_update: r.daysSinceUpdate,
      open_tasks: r.openTasks,
      overdue_tasks: r.overdueTasks
    }));

    const system =
      'You diagnose stalled projects. For each project you receive, return ONE unblock suggestion in <= 14 words. ' +
      'Output STRICT JSON only — no preamble, no fence, no commentary. Schema:\n' +
      '{ "unblocks": [ { "name": string, "unblock": string }, ... ] }\n\n' +
      'Rules:\n' +
      '- The "name" MUST exactly match the input project name.\n' +
      '- Each "unblock" is a verb-led concrete suggestion the user could try this week.\n' +
      '- If a project has 0 tasks, suggest "write down what done looks like, or archive it".\n' +
      '- If a project has many overdue tasks, suggest a 30-min triage / reschedule pass.\n' +
      '- If days_since_completion is null and days_since_update is high, the project may be dead — suggest archiving.\n' +
      '- No corporate sludge: no "synergy", "leverage", "circle back", "let\'s align".\n' +
      '- Never invent details (no fake names, no fake deadlines). You only know what is in the input.';

    const user =
      `Today is ${todayISO()}. Stalled projects:\n\n` +
      '```json\n' +
      JSON.stringify(payload, null, 2) +
      '\n```\n\n' +
      'Return the JSON object with one unblock per project.';

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
          },
          onError: (err) => {
            radarError = err.message;
          }
        },
        radarAbort.signal
      );
      const trimmed = buf.trim();
      if (trimmed) {
        try {
          const cleaned = trimmed.replace(/^```(?:json)?\s*/i, '').replace(/\s*```$/i, '');
          const parsed = JSON.parse(cleaned) as { unblocks?: { name: string; unblock: string }[] };
          if (parsed && Array.isArray(parsed.unblocks)) {
            const byName = new Map(parsed.unblocks.map((u) => [u.name, u.unblock]));
            radarRows = radarRows.map((r) => ({ ...r, unblock: byName.get(r.name) }));
          } else {
            radarError = 'AI returned unexpected shape — radar shown without unblock copy.';
          }
        } catch {
          radarError = 'AI did not return valid JSON — radar shown without unblock copy.';
        }
      }
    } finally {
      radarBusy = false;
      radarAbort = null;
    }
  }
  function cancelRadar() {
    radarAbort?.abort();
  }
  async function archiveProject(name: string) {
    if (!confirm(`Archive "${name}"? It stays in the vault, just out of the active list.`)) return;
    try {
      await api.patchProject(name, { status: 'archived' });
      // Drop the row optimistically; load() reconciles below.
      radarRows = radarRows.filter((r) => r.name !== name);
      await load();
      toast.success(`archived "${name}"`);
    } catch (e) {
      toast.error('archive failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Kanban drag handler. Patches the project's status and refreshes
  // optimistically. Distinct from archiveProject because the kanban
  // is a fluent reclassification surface — no confirm() in the
  // middle of a drag (the user just dragged it, the intent is
  // unambiguous). Failures revert via load().
  async function handleKanbanStatusChange(name: string, status: KanbanStatus) {
    // Optimistic local update so the card "lands" in the new column
    // before the network roundtrip completes.
    projects = projects.map((p) => (p.name === name ? { ...p, status } : p));
    try {
      await api.patchProject(name, { status });
      toast.success(`"${name}" → ${status}`);
    } catch (e) {
      toast.error('status change failed: ' + (e instanceof Error ? e.message : String(e)));
    }
    await load();
  }

  async function load() {
    loading = true;
    try {
      // Two-call parallel load so the sparkline + this-week
      // counts on each card don't wait for projects to finish
      // before tasks even start. Both caches are kept in sync
      // by the WS event subscriptions below.
      const [pr, tr] = await Promise.all([
        api.listProjects(),
        api.listTasks({}).catch(() => ({ tasks: [] as Task[], total: 0 }))
      ]);
      projects = pr.projects;
      tasks = tr.tasks ?? [];
    } catch (e) {
      toast.error('failed to load projects: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    const unsub = onWsEvent((ev) => {
      if (ev.type.startsWith('project.')) load();
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
    // Mobile browsers (and desktop tabs in the background) suspend the
    // WS so events fired while we were away are simply lost. When the
    // tab becomes visible again, force a refetch so we never present a
    // stale list. Cheap to do; cheaper than the user wondering why a
    // project they created on another device isn't showing.
    const onVisible = () => {
      if (document.visibilityState === 'visible') load();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    // 'a' opens the Project Agent — same hotkey contract as
    // /tasks. isTypingTarget guard suppresses while the user is
    // typing in inputs / textareas anywhere on the page.
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (isTypingTarget(e.target)) return;
      if (e.key === 'a') {
        agentOpen = true;
        e.preventDefault();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
      window.removeEventListener('keydown', onKey);
    };
  });

  // Selected project name from query param. Two-pane layout: list left, detail right.
  let selectedName = $derived($page.url.searchParams.get('p') ?? '');
  let selected = $derived(projects.find((p) => p.name === selectedName) ?? null);

  // View modes — list (default; sidebar + detail), timeline (Gantt-ish
  // bars across the full width). Persisted via ?view= so a "show me
  // the project plan" link is shareable. When in timeline mode the
  // sidebar collapses (timeline takes the full surface) and clicking
  // a bar opens the detail drawer-style on top.
  type ViewMode = 'list' | 'kanban' | 'timeline' | 'heatmap';
  let viewMode = $derived<ViewMode>(
    (() => {
      const v = $page.url.searchParams.get('view');
      if (v === 'kanban' || v === 'timeline' || v === 'heatmap') return v;
      return 'list';
    })()
  );
  function setViewMode(v: ViewMode) {
    const params = new URLSearchParams($page.url.searchParams);
    if (v === 'list') params.delete('view');
    else params.set('view', v);
    goto(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }

  // ?venture=<name> scopes the list to a single venture. Cleared via the
  // header chip. Persisted to URL so a "venture roll-up" view is shareable.
  let ventureFilter = $derived($page.url.searchParams.get('venture') ?? '');
  // Distinct venture names — both for the venture group headers and for
  // ProjectCreate's autocomplete datalist.
  let ventures = $derived.by(() => {
    const set = new Set<string>();
    for (const p of projects) {
      const v = (p.venture ?? '').trim();
      if (v) set.add(v);
    }
    return [...set].sort((a, b) => a.localeCompare(b));
  });

  function selectProject(name: string) {
    const params = new URLSearchParams($page.url.searchParams);
    if (name) params.set('p', name);
    else params.delete('p');
    // Closing or switching the project also closes the dashboard
    // overlay — a different project shouldn't keep the prior
    // project's dashboard mounted in the background.
    params.delete('dashboard');
    goto(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }

  // Dashboard overlay — full-screen ProjectDashboardPanel for the
  // selected project. State persists in the URL so a reload (or a
  // shared link) keeps the dashboard open. Pure presentation flag;
  // the panel does its own data load.
  let dashboardOpen = $derived($page.url.searchParams.get('dashboard') === '1');
  function openDashboard() {
    if (!selectedName) return;
    const params = new URLSearchParams($page.url.searchParams);
    params.set('dashboard', '1');
    goto(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }
  function closeDashboard() {
    const params = new URLSearchParams($page.url.searchParams);
    params.delete('dashboard');
    goto(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }
  function clearVentureFilter() {
    const params = new URLSearchParams($page.url.searchParams);
    params.delete('venture');
    goto(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }

  // Kanban feed: all four status columns must always render, so we
  // skip the statusFilter here — venture + search still apply.
  // The sort matches `filtered` so a project's column position is
  // deterministic across views.
  let kanbanFeed = $derived.by(() => {
    let list = projects;
    if (ventureFilter) list = list.filter((p) => (p.venture ?? '') === ventureFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      list = list.filter((p) =>
        p.name.toLowerCase().includes(term) ||
        (p.description ?? '').toLowerCase().includes(term) ||
        (p.tags ?? []).some((t) => t.toLowerCase().includes(term)) ||
        (p.kind ?? '').toLowerCase().includes(term) ||
        (p.venture ?? '').toLowerCase().includes(term)
      );
    }
    // Within a column: priority desc, then name. Status is encoded
    // by the column itself so no status tier needed here.
    return [...list].sort((a, b) => {
      const pa = a.priority ?? 0;
      const pb = b.priority ?? 0;
      if (pa !== pb) return pb - pa;
      return a.name.localeCompare(b.name);
    });
  });

  let filtered = $derived.by(() => {
    let list = projects;
    if (statusFilter !== 'all') list = list.filter((p) => (p.status ?? 'active') === statusFilter);
    if (ventureFilter) list = list.filter((p) => (p.venture ?? '') === ventureFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      list = list.filter((p) =>
        p.name.toLowerCase().includes(term) ||
        (p.description ?? '').toLowerCase().includes(term) ||
        (p.tags ?? []).some((t) => t.toLowerCase().includes(term)) ||
        (p.kind ?? '').toLowerCase().includes(term) ||
        (p.venture ?? '').toLowerCase().includes(term)
      );
    }
    // Sort: active first → priority desc → name
    return [...list].sort((a, b) => {
      const sa = a.status ?? 'active';
      const sb = b.status ?? 'active';
      if (sa !== sb) {
        const order = { active: 0, paused: 1, completed: 2, archived: 3 } as Record<string, number>;
        return (order[sa] ?? 9) - (order[sb] ?? 9);
      }
      const pa = a.priority ?? 0;
      const pb = b.priority ?? 0;
      if (pa !== pb) return pb - pa;
      return a.name.localeCompare(b.name);
    });
  });

  // Group filtered projects by venture, preserving the sort order above.
  // Projects without a venture land in a single 'Unassigned' group at the
  // end — having one named bucket is less noisy than scattering them.
  // When the user has explicitly filtered to a venture we skip the group
  // headers entirely (the URL chip already conveys the scope).
  type Group = { venture: string; projects: typeof projects };
  let grouped = $derived.by((): Group[] => {
    if (ventureFilter) return [{ venture: ventureFilter, projects: filtered }];
    const map = new Map<string, typeof projects>();
    for (const p of filtered) {
      const v = (p.venture ?? '').trim() || '—';
      const arr = map.get(v) ?? [];
      arr.push(p);
      map.set(v, arr);
    }
    const named: Group[] = [];
    let unassigned: Group | null = null;
    for (const [venture, list] of map) {
      const g = { venture, projects: list };
      if (venture === '—') unassigned = g;
      else named.push(g);
    }
    named.sort((a, b) => a.venture.localeCompare(b.venture));
    return unassigned ? [...named, unassigned] : named;
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

  function statusTone(status: string): string {
    if (status === 'active') return 'success';
    if (status === 'paused') return 'warning';
    if (status === 'completed') return 'info';
    if (status === 'archived') return 'subtext';
    return 'subtext';
  }

  async function created(p: Project) {
    createOpen = false;
    // Optimistic insert so the new project shows up immediately even if
    // the listProjects roundtrip is slow. The await load() below
    // reconciles with server-decorated fields (progress, task counts).
    if (!projects.some((x) => x.name === p.name)) {
      projects = [p, ...projects];
    }
    selectProject(p.name);
    await load();
  }

  async function deleted(name: string) {
    selectProject('');
    await load();
    toast.success(`project "${name}" deleted`);
  }
</script>

<div class="h-full flex flex-col">
  <!-- Vision strip sits above the projects layout (sidebar + detail
       split), so the user always sees their season focus without it
       competing with horizontal space. Hidden on mobile when the
       detail pane is open to keep the chrome quiet. -->
  <div class="px-3 sm:px-4 pt-3 flex-shrink-0 {selectedName && viewMode === 'list' ? 'hidden md:block' : ''}">
    <VisionContextStrip />
  </div>

  <!-- Top-level view-mode toggle. Lives outside the sidebar so the
       buttons stay visible in timeline mode (where the sidebar is
       hidden). The list mode is the default — picked by users who
       want the sidebar+detail browsing flow. Timeline gives a
       Gantt-ish whole-portfolio plan view, heatmap a workload grid.
       In chart modes the sidebar's status pills aren't visible, so
       a compact status select rides next to the toggle so the user
       can still scope the chart without bouncing back to list view. -->
  <div class="px-3 sm:px-4 pt-2 flex-shrink-0 flex items-center gap-1.5 flex-wrap {selectedName && viewMode === 'list' ? 'hidden md:flex' : 'flex'}">
    <div class="inline-flex rounded border border-surface1 bg-surface0 overflow-hidden text-xs" role="tablist" aria-label="view mode">
      <button
        role="tab"
        aria-selected={viewMode === 'list'}
        onclick={() => setViewMode('list')}
        class="px-2.5 py-1.5 sm:py-1 min-h-[32px] {viewMode === 'list' ? 'bg-surface1 text-text' : 'text-dim hover:text-text'}"
        title="List + detail (default)"
      >☰ List</button>
      <button
        role="tab"
        aria-selected={viewMode === 'kanban'}
        onclick={() => setViewMode('kanban')}
        class="px-2.5 py-1.5 sm:py-1 min-h-[32px] border-l border-surface1 {viewMode === 'kanban' ? 'bg-surface1 text-text' : 'text-dim hover:text-text'}"
        title="Kanban — drag cards to change status"
      >▤ Board</button>
      <button
        role="tab"
        aria-selected={viewMode === 'timeline'}
        onclick={() => setViewMode('timeline')}
        class="px-2.5 py-1.5 sm:py-1 min-h-[32px] border-l border-surface1 {viewMode === 'timeline' ? 'bg-surface1 text-text' : 'text-dim hover:text-text'}"
        title="Gantt-ish timeline across all projects"
      >▭ Timeline</button>
      <button
        role="tab"
        aria-selected={viewMode === 'heatmap'}
        onclick={() => setViewMode('heatmap')}
        class="px-2.5 py-1.5 sm:py-1 min-h-[32px] border-l border-surface1 {viewMode === 'heatmap' ? 'bg-surface1 text-text' : 'text-dim hover:text-text'}"
        title="Per-project completion volume by week"
      >▦ Heatmap</button>
    </div>
    {#if viewMode === 'timeline' || viewMode === 'heatmap' || viewMode === 'kanban'}
      <!-- The list view's sidebar carries the search box; the chart
           and board views hide the sidebar, so a compact mirror sits
           in the toolbar so the user isn't search-blind here. -->
      <input
        bind:value={q}
        placeholder="filter…"
        class="text-xs px-2 py-1 bg-surface0 border border-surface1 rounded text-text placeholder:text-dim focus:outline-none focus:border-primary min-h-[32px] w-32 sm:w-40"
        aria-label="filter projects"
      />
      <!-- Kanban already splits by status (one column per state),
           so the status select would just empty three columns —
           hide it there. Timeline/heatmap still need it because
           those views show every project on a single canvas. -->
      {#if viewMode !== 'kanban'}
        <select
          value={statusFilter}
          onchange={(e) => (statusFilter = (e.target as HTMLSelectElement).value as typeof statusFilter)}
          class="text-xs px-2 py-1 bg-surface0 border border-surface1 rounded text-subtext min-h-[32px]"
          aria-label="filter by status"
        >
          <option value="active">active</option>
          <option value="paused">paused</option>
          <option value="completed">completed</option>
          <option value="archived">archived</option>
          <option value="all">all</option>
        </select>
      {/if}
      {#if ventureFilter}
        <button
          onclick={clearVentureFilter}
          class="text-xs px-2 py-1 rounded bg-secondary/15 text-secondary hover:bg-secondary/25 min-h-[32px]"
          title="clear venture filter"
        >🏢 {ventureFilter} ×</button>
      {/if}
      <button
        onclick={() => (agentOpen = true)}
        title="Project agent — describe what you want done in plain words"
        class="text-xs px-2.5 py-1 min-h-[32px] rounded border border-surface1 bg-surface0 text-text hover:border-primary inline-flex items-center gap-1"
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5z" />
          <path d="M5 21h14" />
        </svg>
        <span class="hidden sm:inline">Agent</span>
      </button>
      <button
        onclick={() => (createOpen = true)}
        class="ml-auto px-2.5 py-1.5 sm:py-1 min-h-[32px] text-xs bg-primary text-on-primary rounded hover:opacity-90"
      >+ new</button>
    {/if}
  </div>

  <div class="flex-1 min-h-0 flex">
  {#if viewMode === 'list'}
  <!-- List -->
  <aside class="w-full md:w-72 lg:w-80 xl:w-96 flex-shrink-0 border-r border-surface1 bg-mantle/40 flex flex-col {selectedName ? 'hidden md:flex' : ''}">
    <header class="px-3 py-2.5 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
      <h2 class="text-sm font-medium text-text flex-1">Projects</h2>
      <button
        onclick={() => (agentOpen = true)}
        title="Project agent — describe what you want done"
        aria-label="open project agent"
        class="px-2 py-1 text-xs rounded border border-surface1 bg-surface0 text-text hover:border-primary inline-flex items-center gap-1"
      >
        <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5z" />
          <path d="M5 21h14" />
        </svg>
        <span class="hidden sm:inline">AI</span>
      </button>
      <button
        onclick={() => (createOpen = true)}
        class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded hover:opacity-90"
      >+ new</button>
    </header>
    <div class="px-3 py-2 space-y-2 flex-shrink-0">
      <input
        bind:value={q}
        placeholder="filter… (name, kind, venture, tag)"
        class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      <!-- Five status pills. py-1.5 on mobile gives a ~32px tap row;
           desktop tightens back to py-0.5 to keep the dense sidebar
           feel. flex-wrap lets the row break on narrow phones rather
           than crushing each label below readable width. -->
      <div class="flex flex-wrap gap-1 text-xs">
        {#each ['active', 'paused', 'completed', 'archived', 'all'] as s}
          <button
            class="flex-1 min-w-[3.5rem] px-1 py-1.5 sm:py-0.5 rounded {statusFilter === s ? 'bg-surface1 text-text' : 'text-dim hover:text-text hover:bg-surface0'}"
            onclick={() => (statusFilter = s as typeof statusFilter)}
          >{s}</button>
        {/each}
      </div>
      {#if ventureFilter}
        <button
          onclick={clearVentureFilter}
          class="w-full text-left px-2 py-1 text-xs rounded bg-secondary/15 text-secondary hover:bg-secondary/25 flex items-center gap-1.5"
          title="clear venture filter"
        >
          <span>🏢 {ventureFilter}</span>
          <span class="ml-auto text-dim hover:text-text">×</span>
        </button>
      {/if}

      <!-- Stalled-projects radar — collapsible. Local heuristic
           detects active projects with no completion / no edit in
           STALL_DAYS days; the AI adds a one-line unblock per row.
           One toggle button to open + scan; once open, results
           stay visible and the user can rerun. The badge on the
           closed button shows the local count so the user knows
           "the radar would find 4 things" without opening it. -->
      {#if !radarOpen}
        <button
          onclick={() => { radarOpen = true; if (radarRows.length === 0 && !radarBusy) void runRadar(); }}
          class="w-full text-left px-2 py-1.5 text-xs rounded bg-surface0 border border-surface1 hover:border-primary text-subtext flex items-center gap-1.5"
          title="Scan active projects for stalled work"
        >
          <span>📡 Stalled radar</span>
          {#if stalledLocally.length > 0}
            <span class="ml-auto px-1.5 py-0 rounded bg-warning/20 text-warning font-mono text-[10px]">{stalledLocally.length}</span>
          {:else}
            <span class="ml-auto text-dim font-mono text-[10px]">—</span>
          {/if}
        </button>
      {:else}
        <div class="border border-warning/30 bg-warning/5 rounded">
          <div class="px-2 py-1.5 flex items-center gap-1.5 text-xs border-b border-warning/20">
            <span class="text-warning font-medium flex-1">📡 Stalled radar</span>
            {#if radarBusy}
              <button onclick={cancelRadar} class="text-[10px] text-dim hover:text-error">cancel</button>
            {:else}
              <button
                onclick={() => void runRadar()}
                class="text-[10px] text-secondary hover:underline"
                title="rerun the scan"
              >rerun</button>
            {/if}
            <button
              onclick={() => { radarOpen = false; }}
              class="text-[10px] text-dim hover:text-text"
              aria-label="close radar"
            >×</button>
          </div>
          <p class="px-2 pt-1.5 text-[10px] text-dim font-mono">
            scanned {projects.filter((p) => (p.status ?? 'active') === 'active').length} active
            {#if radarRanAt} · ran {radarRanAt}{/if}
          </p>
          {#if radarError}
            <p class="px-2 py-1 text-[10px] text-error">{radarError}</p>
          {/if}
          {#if radarRows.length === 0 && !radarBusy}
            <p class="px-2 py-2 text-xs text-success">Nothing stalled. Active projects all show recent work.</p>
          {:else}
            <ul class="divide-y divide-warning/15">
              {#each radarRows as r (r.name)}
                <li class="px-2 py-1.5 text-xs">
                  <div class="flex items-baseline gap-1.5 mb-0.5">
                    <span class="w-1.5 h-1.5 rounded-full flex-shrink-0" style="background: {colorVar(r.color)}"></span>
                    <button
                      onclick={() => selectProject(r.name)}
                      class="text-text hover:text-primary truncate flex-1 text-left font-medium"
                      title="open {r.name}"
                    >{r.name}</button>
                    <span class="text-[9px] text-dim font-mono flex-shrink-0">
                      {#if r.daysSinceCompletion === null}never done{:else}{r.daysSinceCompletion}d{/if}
                    </span>
                  </div>
                  <p class="text-[10px] text-dim mb-1">
                    {r.openTasks} open
                    {#if r.overdueTasks > 0} · <span class="text-error">{r.overdueTasks} overdue</span>{/if}
                    {#if r.daysSinceUpdate !== null} · edited {r.daysSinceUpdate}d ago{/if}
                  </p>
                  {#if r.unblock}
                    <p class="text-[11px] text-text/90 italic mb-1.5">→ {r.unblock}</p>
                  {:else if radarBusy}
                    <p class="text-[10px] text-dim italic mb-1.5">…</p>
                  {/if}
                  <div class="flex gap-1">
                    <a
                      href={`/calendar?plan=1&project=${encodeURIComponent(r.name)}`}
                      class="text-[10px] px-1.5 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary"
                      title="open the calendar in plan mode for a 30-min unstick session"
                    >schedule unstick →</a>
                    <button
                      onclick={() => void archiveProject(r.name)}
                      class="text-[10px] px-1.5 py-0.5 rounded bg-surface0 border border-surface1 text-dim hover:border-error hover:text-error"
                      title="archive this project"
                    >archive</button>
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}
    </div>
    <div class="flex-1 overflow-y-auto">
      {#if loading && projects.length === 0}
        <div class="p-4 text-sm text-dim">loading…</div>
      {:else if filtered.length === 0}
        <div class="p-4 text-sm text-dim italic">no projects</div>
      {:else}
        {#each grouped as g (g.venture)}
          {#if !ventureFilter && grouped.length > 1}
            <div class="px-3 pt-3 pb-1 sticky top-0 bg-mantle/90 backdrop-blur z-10 flex items-center gap-2 border-b border-surface1/50">
              <span class="text-[10px] uppercase tracking-wider text-dim font-medium flex-1 truncate">
                {g.venture === '—' ? 'no venture' : g.venture}
              </span>
              <span class="text-[10px] text-dim font-mono">{g.projects.length}</span>
            </div>
          {/if}
          <ul class="divide-y divide-surface1">
            {#each g.projects as p (p.name)}
              {@const active = p.name === selectedName}
              {@const progress = p.progress ?? 0}
              <li>
                <button
                  onclick={() => selectProject(p.name)}
                  class="w-full text-left px-3 py-2.5 hover:bg-surface0 {active ? 'bg-surface1' : ''}"
                >
                  <div class="flex items-baseline gap-2 mb-1">
                    <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(p.color)}"></span>
                    <span class="text-sm font-medium text-text flex-1 truncate">{p.name}</span>
                    {#if p.kind}
                      <span class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-primary/10 text-primary flex-shrink-0">{p.kind}</span>
                    {/if}
                    <span
                      class="text-[10px] uppercase tracking-wider flex-shrink-0"
                      style="color: var(--color-{statusTone(p.status ?? 'active')})"
                    >{p.status ?? 'active'}</span>
                  </div>
                  {#if p.description}
                    <p class="text-xs text-subtext line-clamp-2 mb-1.5">{p.description}</p>
                  {/if}
                  <div class="flex items-center gap-2 text-[10px]">
                    <div class="flex-1 h-1 rounded-full bg-surface0 overflow-hidden">
                      <div
                        class="h-full"
                        style="width: {Math.round(progress * 100)}%; background: {colorVar(p.color)}"
                      ></div>
                    </div>
                    <span class="text-dim font-mono w-10 text-right">{Math.round(progress * 100)}%</span>
                    {#if p.tasksTotal != null && p.tasksTotal > 0}
                      <span class="text-dim">{p.tasksDone}/{p.tasksTotal}</span>
                    {/if}
                  </div>

                  <!-- Task-by-status mini-bar — open / scheduled-this-week
                       / done split. The progress bar above answers
                       "how far?", this answers "is the open work being
                       picked up?" — a project with 30 open tasks and
                       0 scheduled tells a different story from one with
                       30 open and 8 scheduled. -->
                  <ProjectStatusBar project={p} tasks={tasks} />

                  {#if momentumByProject.get(p.name)}
                    {@const m = momentumByProject.get(p.name)!}
                    {@const sparkMax = Math.max(...m.spark, 1)}
                    {@const sparkTotal = m.spark.reduce((s, v) => s + v, 0)}
                    {#if sparkTotal > 0 || m.scheduledThisWeek > 0}
                      <!-- 4-week mini-sparkline + this-week count.
                           The list now answers "is this project alive"
                           at a glance — the user can spot stalled
                           projects (flat zero bars) without clicking
                           into each detail panel. Same ISO-week
                           bucketing as the detail burn-up so the
                           per-card view and the per-project view
                           agree. -->
                      <div class="flex items-center gap-2 mt-1.5 text-[10px]">
                        <div class="flex items-end gap-0.5 h-3 flex-shrink-0" aria-hidden="true">
                          {#each m.spark as count, i (i)}
                            {@const isThisWeek = i === SPARK_WEEKS - 1}
                            {@const pct = sparkMax === 0 ? 0 : Math.max(15, Math.round((count / sparkMax) * 100))}
                            <div
                              class="w-1 rounded-sm {isThisWeek ? 'bg-primary' : 'bg-secondary/40'}"
                              style="height: {pct}%"
                            ></div>
                          {/each}
                        </div>
                        {#if sparkTotal > 0}
                          <span class="text-dim font-mono">{sparkTotal} done · 4w</span>
                        {/if}
                        {#if m.scheduledThisWeek > 0}
                          <span class="flex-1"></span>
                          <span class="text-secondary font-mono" title="Tasks scheduled this week">📅 {m.scheduledThisWeek}</span>
                        {/if}
                      </div>
                    {/if}
                  {/if}
                </button>
              </li>
            {/each}
          </ul>
        {/each}
      {/if}
    </div>
  </aside>

  <!-- Detail -->
  <main class="flex-1 min-w-0 {selectedName ? 'block' : 'hidden md:block'}">
    {#if selected}
      <ProjectDetail
        project={selected}
        onClose={() => selectProject('')}
        onUpdated={load}
        onDeleted={deleted}
        onOpenDashboard={openDashboard}
      />
    {:else}
      <div class="h-full flex items-center justify-center text-dim text-sm">
        Select a project from the list, or create a new one.
      </div>
    {/if}
  </main>
  {:else if viewMode === 'kanban'}
    <!-- Kanban — drag-to-change-status board. Sidebar collapses
         (the four columns ARE the navigation). Detail pane opens
         in the same drawer pattern as timeline so clicking a card
         doesn't leave the board. -->
    <main class="flex-1 min-w-0 flex flex-col {selectedName ? 'hidden md:flex' : ''}">
      <ProjectKanban
        projects={kanbanFeed}
        tasks={tasks}
        onSelect={selectProject}
        onStatusChange={handleKanbanStatusChange}
        colorVar={colorVar}
        statusTone={statusTone}
        selectedName={selectedName}
      />
    </main>
    {#if selected}
      <aside class="w-full md:w-[28rem] lg:w-[32rem] flex-shrink-0 border-l border-surface1 bg-base">
        <ProjectDetail
          project={selected}
          onClose={() => selectProject('')}
          onUpdated={load}
          onDeleted={deleted}
        />
      </aside>
    {/if}
  {:else if viewMode === 'timeline'}
    <!-- Timeline view — full-width Gantt-ish chart. Clicking a bar
         flips the URL to ?p=<name>, which keeps the project drawer
         opening on top so the timeline stays the active surface. -->
    <main class="flex-1 min-w-0 flex flex-col {selectedName ? 'hidden md:flex' : ''}">
      <ProjectTimeline
        projects={filtered}
        tasks={tasks}
        onSelect={selectProject}
        colorVar={colorVar}
        statusTone={statusTone}
      />
    </main>
    {#if selected}
      <!-- On desktop the detail pane sits beside the timeline; on
           mobile it covers (the timeline is too dense to share with
           a side pane). -->
      <aside class="w-full md:w-[28rem] lg:w-[32rem] flex-shrink-0 border-l border-surface1 bg-base">
        <ProjectDetail
          project={selected}
          onClose={() => selectProject('')}
          onUpdated={load}
          onDeleted={deleted}
        />
      </aside>
    {/if}
  {:else if viewMode === 'heatmap'}
    <!-- Heatmap — projects × weeks completion grid. Same overlay
         pattern as timeline: full surface chart, optional detail
         pane on the right when a project is selected. -->
    <main class="flex-1 min-w-0 flex flex-col {selectedName ? 'hidden md:flex' : ''}">
      <ProjectHeatmap
        projects={filtered}
        tasks={tasks}
        onSelect={selectProject}
        colorVar={colorVar}
      />
    </main>
    {#if selected}
      <aside class="w-full md:w-[28rem] lg:w-[32rem] flex-shrink-0 border-l border-surface1 bg-base">
        <ProjectDetail
          project={selected}
          onClose={() => selectProject('')}
          onUpdated={load}
          onDeleted={deleted}
        />
      </aside>
    {/if}
  {/if}
  </div>
</div>

{#if dashboardOpen && selected}
  <!-- Project Dashboard overlay — full-screen visual operating
       picture for the selected project. URL-persisted via
       ?dashboard=1 so a reload keeps it open. Sits above every
       other layout (list/kanban/timeline/heatmap) without
       unmounting them, so closing the dashboard lands the user
       back where they came from. -->
  <ProjectDashboardPanel project={selected} onClose={closeDashboard} />
{/if}

<ProjectCreate bind:open={createOpen} ventures={ventures} onCreated={created} />

<!-- Project Agent — operates on whatever's currently visible
     (filtered list, including venture/search/status scope). The
     parent reloads via load() so the kanban + list + timeline
     all reflect the agent's changes immediately. -->
<ProjectAgent
  open={agentOpen}
  projects={filtered}
  todayISO={todayISO()}
  knownVentures={ventures}
  onClose={() => (agentOpen = false)}
  onChanged={load}
/>
