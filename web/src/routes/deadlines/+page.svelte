<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import {
    api,
    type Deadline,
    type DeadlineCreate,
    type DeadlineImportance,
    type DeadlineStatus,
    type Goal,
    type Project,
    type Task
  } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { onWsEvent } from '$lib/ws';
  import Drawer from '$lib/components/Drawer.svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // Deadlines page — top-level "this matters by date X" markers backed
  // by .granit/deadlines.json. Distinct from Tasks (no checkbox / not
  // a todo) and from Goals (a goal has milestones / progress; a
  // deadline is a single hard moment in time, possibly linked to a
  // goal). The list is grouped by recency so the user can see at a
  // glance what's slipping vs. what's distant background context.

  let deadlines = $state<Deadline[]>([]);
  let goals = $state<Goal[]>([]);
  let projects = $state<Project[]>([]);
  // Open tasks pool used by the "link to tasks" multi-select. Loaded
  // lazily on drawer open so the page paints fast even on big vaults.
  let openTasks = $state<Task[]>([]);
  let tasksLoaded = $state(false);
  let loading = $state(false);
  let busy = $state(false);

  // Selection / drawer state.
  let drawerOpen = $state(false);
  // null = create form; non-null = editing this deadline.
  let editing = $state<Deadline | null>(null);
  // Form buffers — bound to inputs so the drawer can be cancelled
  // cleanly without mutating the on-disk record until Save.
  let fTitle = $state('');
  let fDate = $state('');
  let fDescription = $state('');
  let fImportance = $state<DeadlineImportance>('normal');
  let fStatus = $state<DeadlineStatus>('active');
  let fGoalId = $state('');
  let fProject = $state('');
  let fTaskIds = $state<string[]>([]);

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const [dl, gl, pl] = await Promise.all([
        api.listDeadlines(),
        api.listGoals().catch(() => ({ goals: [] as Goal[], total: 0 })),
        api.listProjects().catch(() => ({ projects: [] as Project[], total: 0 }))
      ]);
      deadlines = dl.deadlines;
      goals = gl.goals;
      projects = pl.projects;
    } catch (e) {
      toast.error('load failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  async function ensureTasksLoaded() {
    if (tasksLoaded) return;
    try {
      const r = await api.listTasks({ status: 'open' });
      openTasks = r.tasks;
    } catch {
      openTasks = [];
    } finally {
      tasksLoaded = true;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      // Refetch on the server's deadlines.json signal AND on note/task
      // changes — the latter two refresh the linked task chips when the
      // underlying task gets renamed or completed.
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') load();
      if (ev.type === 'task.changed' || ev.type === 'note.changed') load();
    });
  });

  // ----- Grouping for the list -----
  //
  // Buckets: "Overdue" | "This week" | "This month" | "Later" | "Done".
  // Cancelled rolls into "Done" (alongside met) so the active surface
  // stays focused; the user can still see them at the bottom.

  type Bucket = 'overdue' | 'this_week' | 'this_month' | 'later' | 'done';
  const bucketOrder: Bucket[] = ['overdue', 'this_week', 'this_month', 'later', 'done'];
  const bucketLabel: Record<Bucket, string> = {
    overdue: 'Overdue',
    this_week: 'This week',
    this_month: 'This month',
    later: 'Later',
    done: 'Met / cancelled'
  };

  function daysUntil(iso: string): number {
    const [y, m, d] = iso.split('-').map(Number);
    if (!y || !m || !d) return 0;
    const target = new Date(y, m - 1, d);
    const t = new Date();
    t.setHours(0, 0, 0, 0);
    return Math.round((target.getTime() - t.getTime()) / 86_400_000);
  }

  function bucketOf(d: Deadline): Bucket {
    if (d.status === 'met' || d.status === 'cancelled') return 'done';
    const days = daysUntil(d.date);
    if (days < 0) return 'overdue';
    if (days <= 7) return 'this_week';
    if (days <= 31) return 'this_month';
    return 'later';
  }

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

  let grouped = $derived.by(() => {
    const out: Record<Bucket, Deadline[]> = {
      overdue: [],
      this_week: [],
      this_month: [],
      later: [],
      done: []
    };
    // The server returns rows already sorted (active+missed first by
    // date asc), so we can just bucket without re-sorting.
    for (const d of deadlines) out[bucketOf(d)].push(d);
    return out;
  });

  // ----- Drawer / form -----

  function openCreate() {
    editing = null;
    fTitle = '';
    fDate = todayISO();
    fDescription = '';
    fImportance = 'normal';
    fStatus = 'active';
    fGoalId = '';
    fProject = '';
    fTaskIds = [];
    drawerOpen = true;
    void ensureTasksLoaded();
  }

  function openEdit(d: Deadline) {
    editing = d;
    fTitle = d.title;
    fDate = d.date;
    fDescription = d.description ?? '';
    fImportance = d.importance ?? 'normal';
    fStatus = d.status ?? 'active';
    fGoalId = d.goal_id ?? '';
    fProject = d.project ?? '';
    fTaskIds = [...(d.task_ids ?? [])];
    drawerOpen = true;
    void ensureTasksLoaded();
  }

  function todayISO(): string {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }

  function isValidDate(s: string): boolean {
    return /^\d{4}-\d{2}-\d{2}$/.test(s);
  }

  async function save() {
    if (!fTitle.trim()) {
      toast.warning('title is required');
      return;
    }
    if (!isValidDate(fDate)) {
      toast.warning('date must be YYYY-MM-DD');
      return;
    }
    busy = true;
    try {
      const payload: DeadlineCreate = {
        title: fTitle.trim(),
        date: fDate,
        description: fDescription.trim() || undefined,
        importance: fImportance,
        status: fStatus,
        goal_id: fGoalId || undefined,
        project: fProject || undefined,
        task_ids: fTaskIds.length ? fTaskIds : undefined
      };
      if (editing) {
        await api.patchDeadline(editing.id, payload);
        toast.success('saved');
      } else {
        await api.createDeadline(payload);
        toast.success('created');
      }
      drawerOpen = false;
      await load();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (!editing) return;
    if (!confirm(`Delete "${editing.title}"? This can't be undone.`)) return;
    busy = true;
    try {
      await api.deleteDeadline(editing.id);
      toast.success('deleted');
      drawerOpen = false;
      await load();
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = false;
    }
  }

  function toggleTaskLink(id: string) {
    if (fTaskIds.includes(id)) fTaskIds = fTaskIds.filter((x) => x !== id);
    else fTaskIds = [...fTaskIds, id];
  }

  function importanceTone(i: DeadlineImportance | string | undefined): string {
    switch (i) {
      case 'critical':
        return 'error';
      case 'high':
        return 'warning';
      default:
        return 'secondary';
    }
  }

  function statusTone(s: DeadlineStatus | string | undefined): string {
    switch (s) {
      case 'met':
        return 'success';
      case 'missed':
        return 'error';
      case 'cancelled':
        return 'subtext';
      default:
        return 'info';
    }
  }

  // Display helpers for the chip row — we look up the linked goal by
  // ID so the chip shows a real title (the on-disk record only stores
  // the ULID). Falls back to "(unknown)" if the goal was deleted but
  // the deadline still references it.
  function goalTitle(id?: string): string {
    if (!id) return '';
    const g = goals.find((x) => x.id === id);
    return g?.title ?? '(unknown)';
  }

  function taskText(id: string): string {
    const t = openTasks.find((x) => x.id === id);
    return t?.text ?? id;
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-4xl mx-auto">
    <PageHeader
      title="Deadlines"
      subtitle="dated commitments — linked to goals, projects, or tasks"
    >
      {#snippet actions()}
        <button
          onclick={openCreate}
          class="px-3 py-1.5 bg-primary text-mantle text-sm font-medium rounded hover:opacity-90"
        >+ New deadline</button>
      {/snippet}
    </PageHeader>

    {#if loading && deadlines.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if deadlines.length === 0}
      <div class="text-sm text-dim italic">
        no deadlines yet. click "+ New deadline" to add the first one.
      </div>
    {:else}
      <div class="space-y-6">
        {#each bucketOrder as b}
          {@const rows = grouped[b]}
          {#if rows.length > 0}
            <section>
              <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">
                {bucketLabel[b]}
                <span class="text-dim/70 ml-1">·{rows.length}</span>
              </h2>
              <ul class="space-y-1.5">
                {#each rows as d (d.id)}
                  {@const it = importanceTone(d.importance)}
                  {@const st = statusTone(d.status)}
                  <li>
                    <button
                      type="button"
                      onclick={() => openEdit(d)}
                      class="w-full text-left bg-surface0 border border-surface1 rounded-lg p-3 hover:border-primary/50 transition-colors flex flex-col gap-1.5"
                    >
                      <div class="flex items-baseline gap-2 flex-wrap">
                        <span class="text-base text-text font-medium flex-1 min-w-0 truncate">
                          {d.title}
                        </span>
                        <span
                          class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded whitespace-nowrap"
                          style="background: color-mix(in srgb, var(--color-{it}) 18%, transparent); color: var(--color-{it});"
                        >{d.importance ?? 'normal'}</span>
                        {#if d.status && d.status !== 'active'}
                          <span
                            class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded whitespace-nowrap"
                            style="background: color-mix(in srgb, var(--color-{st}) 18%, transparent); color: var(--color-{st});"
                          >{d.status}</span>
                        {/if}
                      </div>
                      <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-dim">
                        <span class="font-mono tabular-nums text-subtext">{d.date}</span>
                        <span>· {countdown(d)}</span>
                        {#if d.goal_id}
                          <span class="text-secondary">🎯 {goalTitle(d.goal_id)}</span>
                        {/if}
                        {#if d.project}
                          <span class="text-secondary">📁 {d.project}</span>
                        {/if}
                        {#if d.task_ids && d.task_ids.length > 0}
                          <span>🔗 {d.task_ids.length} task{d.task_ids.length === 1 ? '' : 's'}</span>
                        {/if}
                      </div>
                      {#if d.description}
                        <p class="text-sm text-subtext line-clamp-2">{d.description}</p>
                      {/if}
                    </button>
                  </li>
                {/each}
              </ul>
            </section>
          {/if}
        {/each}
      </div>
    {/if}
  </div>
</div>

<Drawer bind:open={drawerOpen} side="right" responsive width="w-full sm:w-96 md:w-[28rem]">
  <div class="h-full flex flex-col overflow-hidden">
    <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
      <h2 class="text-sm font-semibold text-text flex-1 truncate">
        {editing ? 'Edit deadline' : 'New deadline'}
      </h2>
      <button
        onclick={() => (drawerOpen = false)}
        aria-label="close"
        class="text-dim hover:text-text text-lg leading-none"
      >×</button>
    </header>

    <div class="flex-1 overflow-y-auto p-4 space-y-4">
      <div>
        <label for="d-title" class="block text-xs uppercase tracking-wider text-dim mb-1">Title</label>
        <input
          id="d-title"
          type="text"
          bind:value={fTitle}
          placeholder="e.g. Bar exam"
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        />
      </div>

      <div>
        <label for="d-date" class="block text-xs uppercase tracking-wider text-dim mb-1">Date</label>
        <input
          id="d-date"
          type="date"
          bind:value={fDate}
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        />
      </div>

      <div>
        <label for="d-desc" class="block text-xs uppercase tracking-wider text-dim mb-1">Description</label>
        <textarea
          id="d-desc"
          bind:value={fDescription}
          rows="3"
          placeholder="optional context"
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary resize-y"
        ></textarea>
      </div>

      <div>
        <span class="block text-xs uppercase tracking-wider text-dim mb-1">Importance</span>
        <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
          {#each ['critical', 'high', 'normal'] as i}
            <button
              type="button"
              onclick={() => (fImportance = i as DeadlineImportance)}
              class="flex-1 px-3 py-1.5 capitalize {fImportance === i ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
            >{i}</button>
          {/each}
        </div>
      </div>

      <div>
        <span class="block text-xs uppercase tracking-wider text-dim mb-1">Status</span>
        <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
          {#each ['active', 'missed', 'met', 'cancelled'] as s}
            <button
              type="button"
              onclick={() => (fStatus = s as DeadlineStatus)}
              class="flex-1 px-2 py-1.5 capitalize {fStatus === s ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
            >{s}</button>
          {/each}
        </div>
      </div>

      <div>
        <label for="d-goal" class="block text-xs uppercase tracking-wider text-dim mb-1">Linked goal</label>
        <select
          id="d-goal"
          bind:value={fGoalId}
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        >
          <option value="">— none —</option>
          {#each goals as g (g.id)}
            <option value={g.id}>{g.title}</option>
          {/each}
        </select>
      </div>

      <div>
        <label for="d-project" class="block text-xs uppercase tracking-wider text-dim mb-1">Linked project</label>
        <select
          id="d-project"
          bind:value={fProject}
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        >
          <option value="">— none —</option>
          {#each projects as p (p.name)}
            <option value={p.name}>{p.name}</option>
          {/each}
        </select>
      </div>

      <div>
        <span class="block text-xs uppercase tracking-wider text-dim mb-1">
          Linked tasks <span class="text-dim/70">({fTaskIds.length} selected)</span>
        </span>
        {#if !tasksLoaded}
          <div class="text-xs text-dim italic">loading tasks…</div>
        {:else if openTasks.length === 0}
          <div class="text-xs text-dim italic">no open tasks to link.</div>
        {:else}
          <div class="max-h-48 overflow-y-auto bg-surface0 border border-surface1 rounded">
            {#each openTasks as t (t.id)}
              {@const checked = fTaskIds.includes(t.id)}
              <label class="flex items-start gap-2 px-2 py-1.5 hover:bg-surface1 cursor-pointer text-xs">
                <input
                  type="checkbox"
                  {checked}
                  onchange={() => toggleTaskLink(t.id)}
                  class="mt-0.5"
                />
                <span class="flex-1 text-text truncate">{t.text}</span>
              </label>
            {/each}
          </div>
        {/if}
      </div>
    </div>

    <footer class="px-4 py-3 border-t border-surface1 flex items-center gap-2 flex-shrink-0">
      {#if editing}
        <button
          onclick={remove}
          disabled={busy}
          class="px-3 py-1.5 text-xs text-error hover:bg-error/10 rounded disabled:opacity-50"
        >Delete</button>
      {/if}
      <span class="flex-1"></span>
      <button
        onclick={() => (drawerOpen = false)}
        disabled={busy}
        class="px-3 py-1.5 text-sm text-subtext hover:bg-surface0 rounded disabled:opacity-50"
      >Cancel</button>
      <button
        onclick={save}
        disabled={busy}
        class="px-3 py-1.5 text-sm bg-primary text-mantle rounded hover:opacity-90 disabled:opacity-50"
      >{editing ? 'Save' : 'Create'}</button>
    </footer>
  </div>
</Drawer>
