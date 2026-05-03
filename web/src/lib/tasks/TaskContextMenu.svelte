<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Task, type Project, type Goal, type Deadline } from '$lib/api';
  import { toast } from '$lib/components/toast';

  // TaskContextMenu is the right-click / long-press menu for a kanban
  // (or list) task. Discoverable surface for actions that already exist
  // server-side but were buried behind hover affordances: open detail,
  // mark done, set priority, link to project / goal / deadline, delete.
  //
  // Reuses api.patchTask / api.deleteTask — we don't add server endpoints,
  // we just give the existing ones a discoverable home. Submenus are
  // lazy-loaded on first open so the menu itself is cheap to mount.

  let {
    task,
    x,
    y,
    onClose,
    onChanged,
    onOpenDetail
  }: {
    task: Task;
    x: number;
    y: number;
    onClose: () => void;
    onChanged?: () => void | Promise<void>;
    onOpenDetail?: (t: Task) => void;
  } = $props();

  let panel: HTMLDivElement | undefined = $state();
  let submenu = $state<'priority' | 'project' | 'goal' | 'deadline' | null>(null);
  let projects = $state<Project[]>([]);
  let goals = $state<Goal[]>([]);
  let deadlines = $state<Deadline[]>([]);
  let loadingSub = $state(false);

  // Position-clamp inside the viewport so the menu doesn't render off
  // the right edge or below the fold. Initial position is the click
  // coords; the onMount measurement clamps once the panel is in the
  // DOM and we know its actual size.
  let pos = $state<{ left: number; top: number }>({ left: 0, top: 0 });
  $effect(() => {
    pos = { left: x, top: y };
  });
  onMount(() => {
    queueMicrotask(() => {
      if (!panel) return;
      const rect = panel.getBoundingClientRect();
      const vw = window.innerWidth;
      const vh = window.innerHeight;
      let left = pos.left;
      let top = pos.top;
      if (left + rect.width > vw - 8) left = Math.max(8, vw - rect.width - 8);
      if (top + rect.height > vh - 8) top = Math.max(8, vh - rect.height - 8);
      pos = { left, top };
    });
  });

  function handleOutside(e: MouseEvent) {
    if (panel && !panel.contains(e.target as Node)) onClose();
  }
  function handleKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose();
  }
  onMount(() => {
    // Defer outside-click registration to the next frame: the same
    // mousedown that opened the menu would otherwise immediately close
    // it. setTimeout(0) lands us after the current event loop tick.
    setTimeout(() => {
      window.addEventListener('mousedown', handleOutside);
      window.addEventListener('keydown', handleKey);
    }, 0);
    return () => {
      window.removeEventListener('mousedown', handleOutside);
      window.removeEventListener('keydown', handleKey);
    };
  });

  async function ensureSub(kind: 'project' | 'goal' | 'deadline') {
    submenu = kind;
    loadingSub = true;
    try {
      if (kind === 'project' && projects.length === 0) {
        const r = await api.listProjects();
        projects = r.projects;
      } else if (kind === 'goal' && goals.length === 0) {
        const r = await api.listGoals();
        goals = r.goals;
      } else if (kind === 'deadline' && deadlines.length === 0) {
        const r = await api.listDeadlines();
        deadlines = r.deadlines;
      }
    } catch (e) {
      toast.error('failed to load: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loadingSub = false;
    }
  }

  async function applyPatch(patch: Parameters<typeof api.patchTask>[1], successMsg?: string) {
    try {
      await api.patchTask(task.id, patch);
      if (successMsg) toast.success(successMsg);
      await onChanged?.();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      onClose();
    }
  }

  async function setPriority(p: number) { await applyPatch({ priority: p }); }
  async function markDone() { await applyPatch({ done: !task.done }); }
  async function linkProject(name: string) { await applyPatch({ projectId: name }, name ? `linked to ${name}` : 'project unlinked'); }
  async function linkGoal(id: string) { await applyPatch({ goalId: id }, id ? `linked to goal ${id}` : 'goal unlinked'); }
  async function linkDeadline(id: string) { await applyPatch({ deadlineId: id }, id ? 'deadline linked' : 'deadline unlinked'); }

  async function deleteTask() {
    if (!confirm(`Delete task "${task.text}"?`)) return;
    try {
      await api.deleteTask(task.id);
      toast.success('task deleted');
      await onChanged?.();
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      onClose();
    }
  }
</script>

<div
  bind:this={panel}
  class="fixed z-50 bg-mantle border border-surface1 rounded shadow-xl py-1 text-sm min-w-[12rem] max-w-[16rem]"
  style="left: {pos.left}px; top: {pos.top}px;"
  role="menu"
  aria-label="task actions"
>
  {#if submenu === null}
    {#if onOpenDetail}
      <button
        class="w-full text-left px-3 py-1.5 text-text hover:bg-surface0"
        onclick={() => { onOpenDetail(task); onClose(); }}
        role="menuitem"
      >Open detail…</button>
    {/if}
    <button
      class="w-full text-left px-3 py-1.5 text-text hover:bg-surface0"
      onclick={markDone}
      role="menuitem"
    >{task.done ? 'Mark not done' : 'Mark done'}</button>

    <div class="my-1 border-t border-surface1"></div>

    <button
      class="w-full flex items-center justify-between px-3 py-1.5 text-text hover:bg-surface0"
      onclick={() => (submenu = 'priority')}
      role="menuitem"
    >
      <span>Set priority</span>
      <span class="text-dim text-xs">›</span>
    </button>
    <button
      class="w-full flex items-center justify-between px-3 py-1.5 text-text hover:bg-surface0"
      onclick={() => ensureSub('project')}
      role="menuitem"
    >
      <span>Link project</span>
      <span class="text-dim text-xs">›</span>
    </button>
    <button
      class="w-full flex items-center justify-between px-3 py-1.5 text-text hover:bg-surface0"
      onclick={() => ensureSub('goal')}
      role="menuitem"
    >
      <span>Link goal</span>
      <span class="text-dim text-xs">›</span>
    </button>
    <button
      class="w-full flex items-center justify-between px-3 py-1.5 text-text hover:bg-surface0"
      onclick={() => ensureSub('deadline')}
      role="menuitem"
    >
      <span>Link deadline</span>
      <span class="text-dim text-xs">›</span>
    </button>

    <div class="my-1 border-t border-surface1"></div>

    <button
      class="w-full text-left px-3 py-1.5 text-error hover:bg-error/10"
      onclick={deleteTask}
      role="menuitem"
    >Delete task</button>
  {:else if submenu === 'priority'}
    <div class="px-3 py-1 text-[10px] uppercase tracking-wider text-dim">priority</div>
    {#each [{ p: 1, label: 'P1 high' }, { p: 2, label: 'P2 medium' }, { p: 3, label: 'P3 low' }, { p: 0, label: 'no priority' }] as o}
      <button
        class="w-full text-left px-3 py-1.5 text-text hover:bg-surface0 {task.priority === o.p ? 'bg-surface0/60 text-primary' : ''}"
        onclick={() => setPriority(o.p)}
        role="menuitem"
      >{o.label}</button>
    {/each}
    <div class="my-1 border-t border-surface1"></div>
    <button class="w-full text-left px-3 py-1.5 text-dim hover:bg-surface0" onclick={() => (submenu = null)}>‹ back</button>
  {:else if submenu === 'project'}
    <div class="px-3 py-1 text-[10px] uppercase tracking-wider text-dim">project</div>
    {#if loadingSub}
      <div class="px-3 py-1.5 text-dim italic text-xs">loading…</div>
    {:else}
      <button
        class="w-full text-left px-3 py-1.5 text-dim hover:bg-surface0 italic"
        onclick={() => linkProject('')}
        role="menuitem"
      >(none)</button>
      <div class="max-h-64 overflow-y-auto">
        {#each projects as p}
          <button
            class="w-full text-left px-3 py-1.5 text-text hover:bg-surface0 truncate {task.projectId === p.name ? 'bg-surface0/60 text-primary' : ''}"
            onclick={() => linkProject(p.name)}
            role="menuitem"
            title={p.description}
          >{p.name}</button>
        {/each}
      </div>
    {/if}
    <div class="my-1 border-t border-surface1"></div>
    <button class="w-full text-left px-3 py-1.5 text-dim hover:bg-surface0" onclick={() => (submenu = null)}>‹ back</button>
  {:else if submenu === 'goal'}
    <div class="px-3 py-1 text-[10px] uppercase tracking-wider text-dim">goal</div>
    {#if loadingSub}
      <div class="px-3 py-1.5 text-dim italic text-xs">loading…</div>
    {:else}
      <button
        class="w-full text-left px-3 py-1.5 text-dim hover:bg-surface0 italic"
        onclick={() => linkGoal('')}
        role="menuitem"
      >(none)</button>
      <div class="max-h-64 overflow-y-auto">
        {#each goals as g}
          <button
            class="w-full text-left px-3 py-1.5 text-text hover:bg-surface0 truncate {task.goalId === g.id ? 'bg-surface0/60 text-primary' : ''}"
            onclick={() => linkGoal(g.id)}
            role="menuitem"
            title={g.description ?? ''}
          >
            <span class="text-dim font-mono text-[10px] mr-1">{g.id}</span>
            {g.title}
          </button>
        {/each}
      </div>
    {/if}
    <div class="my-1 border-t border-surface1"></div>
    <button class="w-full text-left px-3 py-1.5 text-dim hover:bg-surface0" onclick={() => (submenu = null)}>‹ back</button>
  {:else if submenu === 'deadline'}
    <div class="px-3 py-1 text-[10px] uppercase tracking-wider text-dim">deadline</div>
    {#if loadingSub}
      <div class="px-3 py-1.5 text-dim italic text-xs">loading…</div>
    {:else}
      <button
        class="w-full text-left px-3 py-1.5 text-dim hover:bg-surface0 italic"
        onclick={() => linkDeadline('')}
        role="menuitem"
      >(none)</button>
      <div class="max-h-64 overflow-y-auto">
        {#each deadlines as d}
          <button
            class="w-full text-left px-3 py-1.5 text-text hover:bg-surface0 truncate {task.deadlineId === d.id ? 'bg-surface0/60 text-primary' : ''}"
            onclick={() => linkDeadline(d.id)}
            role="menuitem"
            title={d.description ?? ''}
          >
            <span class="text-dim font-mono text-[10px] mr-1">{d.date}</span>
            {d.title}
          </button>
        {/each}
      </div>
    {/if}
    <div class="my-1 border-t border-surface1"></div>
    <button class="w-full text-left px-3 py-1.5 text-dim hover:bg-surface0" onclick={() => (submenu = null)}>‹ back</button>
  {/if}
</div>
