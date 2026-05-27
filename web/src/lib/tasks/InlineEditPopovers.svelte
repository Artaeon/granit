<!-- Approach (c): the three popover chips (tags / project / goal) live in
     a contiguous slice of the chip-row, so we move BOTH the chips and the
     absolutely-positioned popovers into this sub-component. The chips
     flow naturally into TaskCard's flex-wrap container; the popovers
     rely on TaskCard's .task-card being position:relative as their
     positioning context. openPopover is $bindable so TaskCard can flip
     its overflow class while a popover is open (Fix A3). -->
<script lang="ts">
  import { api, type Task, type Project, type Goal } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { loadProjectsOnce, loadGoalsOnce, loadTagsOnce } from './inlineEditCache';

  type PopoverKind = 'tags' | 'project' | 'goal';

  let {
    task = $bindable(),
    onChanged,
    openPopover = $bindable<PopoverKind | null>(null)
  }: {
    task: Task;
    onChanged?: (t: Task) => void;
    openPopover?: PopoverKind | null;
  } = $props();

  let projectsList = $state<Project[]>([]);
  let projectsLoading = $state(false);
  let goalsList = $state<Goal[]>([]);
  let goalsLoading = $state(false);
  let knownTags = $state<{ tag: string; count: number }[]>([]);
  let tagInputBuf = $state('');
  let projectQuery = $state('');
  let goalQuery = $state('');
  let busy = $state(false);

  function closePopovers() {
    openPopover = null;
    tagInputBuf = '';
    projectQuery = '';
    goalQuery = '';
  }

  async function openTagsPopover(e: Event) {
    e.stopPropagation();
    openPopover = openPopover === 'tags' ? null : 'tags';
    if (openPopover === 'tags' && knownTags.length === 0) {
      try { knownTags = await loadTagsOnce(); } catch { /* best-effort */ }
    }
  }

  async function openProjectPopover(e: Event) {
    e.stopPropagation();
    openPopover = openPopover === 'project' ? null : 'project';
    if (openPopover === 'project' && projectsList.length === 0) {
      projectsLoading = true;
      try { projectsList = await loadProjectsOnce(); } catch { /* best-effort */ }
      finally { projectsLoading = false; }
    }
  }

  async function openGoalPopover(e: Event) {
    e.stopPropagation();
    openPopover = openPopover === 'goal' ? null : 'goal';
    if (openPopover === 'goal' && goalsList.length === 0) {
      goalsLoading = true;
      try { goalsList = await loadGoalsOnce(); } catch { /* best-effort */ }
      finally { goalsLoading = false; }
    }
  }

  // Click-outside + Escape handling for the inline popovers. Matches
  // the SnoozePicker pattern but lives directly on this component so
  // the popover markup can stay inline.
  $effect(() => {
    if (!openPopover) return;
    const onDoc = (ev: MouseEvent) => {
      const target = ev.target as Node | null;
      if (!target) return;
      // Buttons / popover panels carry data-inline-pop so they don't
      // self-dismiss. Any click that does NOT land on one of them
      // closes the popover.
      let n: Node | null = target;
      while (n) {
        if (n instanceof HTMLElement && n.dataset.inlinePop) return;
        n = n.parentNode;
      }
      closePopovers();
    };
    const onKey = (ev: KeyboardEvent) => { if (ev.key === 'Escape') closePopovers(); };
    document.addEventListener('click', onDoc);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('click', onDoc);
      document.removeEventListener('keydown', onKey);
    };
  });

  // Tag editing rewrites task.text: tags are #tag tokens inline. The
  // server's parser re-extracts them on next read, so we PATCH text
  // and let the round-trip drop the new tag onto task.tags.
  function stripTagToken(text: string, tag: string): string {
    const re = new RegExp(`(?:^|\\s)#${tag.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\$&')}(?=\\s|$)`, 'gu');
    return text.replace(re, ' ').replace(/\s+/g, ' ').trim();
  }
  function appendTagToken(text: string, tag: string): string {
    return (text.trim() + ' #' + tag).replace(/\s+/g, ' ').trim();
  }

  async function addTag(raw: string) {
    const t = raw.trim().replace(/^#/, '').replace(/[^\p{L}\p{N}_/-]/gu, '').slice(0, 20);
    if (!t) return;
    if ((task.tags ?? []).includes(t)) { tagInputBuf = ''; return; }
    busy = true;
    try {
      const nextText = appendTagToken(task.text, t);
      const updated = await api.patchTask(task.id, { text: nextText });
      task = updated;
      onChanged?.(updated);
      tagInputBuf = '';
    } catch {
      toast.error('failed to add tag');
    } finally {
      busy = false;
    }
  }
  async function removeTag(tag: string) {
    busy = true;
    try {
      const nextText = stripTagToken(task.text, tag);
      const updated = await api.patchTask(task.id, { text: nextText });
      task = updated;
      onChanged?.(updated);
    } catch {
      toast.error('failed to remove tag');
    } finally {
      busy = false;
    }
  }

  async function setProject(name: string | null) {
    const prev = task;
    const projectId = name ?? '';
    task = { ...task, projectId: projectId || undefined };
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { projectId });
      task = updated;
      onChanged?.(updated);
    } catch {
      task = prev;
      toast.error('failed to set project');
    } finally {
      busy = false;
      closePopovers();
    }
  }

  async function setGoal(id: string | null) {
    const prev = task;
    const goalId = id ?? '';
    task = { ...task, goalId: goalId || undefined };
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { goalId });
      task = updated;
      onChanged?.(updated);
    } catch {
      task = prev;
      toast.error('failed to set goal');
    } finally {
      busy = false;
      closePopovers();
    }
  }

  let filteredProjects = $derived.by(() => {
    const q = projectQuery.trim().toLowerCase();
    if (!q) return projectsList;
    return projectsList.filter((p) => p.name.toLowerCase().includes(q));
  });
  let filteredGoals = $derived.by(() => {
    const q = goalQuery.trim().toLowerCase();
    if (!q) return goalsList;
    return goalsList.filter((g) => (g.title ?? g.id).toLowerCase().includes(q) || g.id.toLowerCase().includes(q));
  });
</script>

<!-- Tags chip — click to open inline editor; even with no tags set,
     a hover-only "+ tag" affordance appears so the user never has
     to open the detail drawer just to stick a tag on a task. -->
{#if task.tags && task.tags.length > 0}
  <button
    type="button"
    data-inline-pop
    onclick={openTagsPopover}
    title="Edit tags"
    aria-label="Edit tags"
    class="text-dim hover:text-text inline-flex items-center"
  >{task.tags.map((t) => '#' + t).join(' ')}</button>
{:else}
  <button
    type="button"
    data-inline-pop
    onclick={openTagsPopover}
    title="Add tag"
    aria-label="Add tag"
    class="text-[10px] font-mono px-1.5 rounded text-dim opacity-0 group-hover:opacity-100 hover:text-text hover:bg-surface1 border border-surface2 flex-shrink-0 disabled:opacity-50 cursor-pointer transition-opacity"
  >+ tag</button>
{/if}
{#if task.projectId}
  <button
    type="button"
    data-inline-pop
    onclick={openProjectPopover}
    class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] text-secondary bg-surface1 hover:brightness-110"
    title="Change project (current: {task.projectId})"
    aria-label="Change project"
  >
    <span aria-hidden="true">📁</span>
    <span class="truncate max-w-[8rem]">{task.projectId}</span>
  </button>
{:else}
  <button
    type="button"
    data-inline-pop
    onclick={openProjectPopover}
    title="Set project"
    aria-label="Set project"
    class="text-[10px] font-mono px-1.5 rounded text-dim opacity-0 group-hover:opacity-100 hover:text-text hover:bg-surface1 border border-surface2 flex-shrink-0 cursor-pointer transition-opacity"
  >+ project</button>
{/if}
{#if task.goalId}
  <button
    type="button"
    data-inline-pop
    onclick={openGoalPopover}
    class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] text-info bg-surface0 font-mono hover:brightness-110"
    title="Change goal (current: {task.goalId})"
    aria-label="Change goal"
  >
    <span aria-hidden="true">🎯</span>{task.goalId}
  </button>
{:else}
  <button
    type="button"
    data-inline-pop
    onclick={openGoalPopover}
    title="Set goal"
    aria-label="Set goal"
    class="text-[10px] font-mono px-1.5 rounded text-dim opacity-0 group-hover:opacity-100 hover:text-text hover:bg-surface1 border border-surface2 flex-shrink-0 cursor-pointer transition-opacity"
  >+ goal</button>
{/if}

<!-- Inline-edit popovers. Anchored absolute under the card so they
     don't escape the task list scroll container; data-inline-pop
     keeps click-outside from collapsing them on internal clicks.
     Only one renders at a time (openPopover is a single enum). The
     positioning context is TaskCard's .task-card (position:relative). -->
{#if openPopover === 'tags'}
  <div
    data-inline-pop
    class="absolute z-40 left-2 top-full mt-1 bg-mantle border border-surface1 rounded shadow-lg p-2 w-64"
    role="dialog"
    aria-label="Edit tags"
  >
    <div class="flex flex-wrap gap-1 mb-2 min-h-[1.25rem]">
      {#each task.tags ?? [] as t (t)}
        <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] bg-surface1 text-text">
          #{t}
          <button
            type="button"
            data-inline-pop
            onclick={() => removeTag(t)}
            disabled={busy}
            aria-label="Remove tag {t}"
            class="text-dim hover:text-error leading-none"
          >×</button>
        </span>
      {/each}
      {#if !task.tags || task.tags.length === 0}
        <span class="text-[10px] text-dim italic">no tags yet</span>
      {/if}
    </div>
    <form
      data-inline-pop
      onsubmit={(e) => { e.preventDefault(); void addTag(tagInputBuf); }}
    >
      <input
        data-inline-pop
        bind:value={tagInputBuf}
        maxlength="20"
        placeholder="new tag, Enter to add"
        class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text placeholder-dim focus:outline-none focus:border-primary"
      />
    </form>
    {#if knownTags.length > 0}
      {@const used = new Set(task.tags ?? [])}
      {@const q = tagInputBuf.trim().toLowerCase()}
      {@const suggested = knownTags
        .filter((kt) => !used.has(kt.tag) && (!q || kt.tag.toLowerCase().includes(q)))
        .slice(0, 8)}
      {#if suggested.length > 0}
        <div class="mt-2 pt-2 border-t border-surface1 flex flex-wrap gap-1">
          {#each suggested as kt (kt.tag)}
            <button
              type="button"
              data-inline-pop
              onclick={() => void addTag(kt.tag)}
              disabled={busy}
              class="text-[10px] px-1.5 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary hover:text-text disabled:opacity-50"
              title="{kt.count} task{kt.count === 1 ? '' : 's'} use this tag"
            >#{kt.tag}</button>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
{/if}
{#if openPopover === 'project'}
  <div
    data-inline-pop
    class="absolute z-40 left-2 top-full mt-1 bg-mantle border border-surface1 rounded shadow-lg w-64 max-h-72 overflow-hidden flex flex-col"
    role="dialog"
    aria-label="Pick project"
  >
    <div class="p-2 border-b border-surface1">
      <input
        data-inline-pop
        bind:value={projectQuery}
        placeholder="filter projects…"
        class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text placeholder-dim focus:outline-none focus:border-primary"
      />
    </div>
    <div class="overflow-y-auto flex-1 py-1">
      <button
        type="button"
        data-inline-pop
        onclick={() => void setProject(null)}
        disabled={busy}
        class="w-full text-left px-3 py-1 text-xs text-dim hover:bg-surface0 hover:text-text disabled:opacity-50 flex items-center gap-2"
      >
        <span class="text-base leading-none">—</span>
        <span>no project</span>
      </button>
      {#if projectsLoading}
        <div class="px-3 py-2 text-[10px] text-dim italic">loading…</div>
      {:else if filteredProjects.length === 0}
        <div class="px-3 py-2 text-[10px] text-dim italic">no matches</div>
      {:else}
        {#each filteredProjects as p (p.name)}
          <button
            type="button"
            data-inline-pop
            onclick={() => void setProject(p.name)}
            disabled={busy}
            class="w-full text-left px-3 py-1 text-xs hover:bg-surface0 disabled:opacity-50 flex items-center justify-between gap-2 {task.projectId === p.name ? 'text-primary' : 'text-text'}"
          >
            <span class="truncate">{p.name}</span>
            {#if task.projectId === p.name}
              <span class="text-[10px] text-primary">✓</span>
            {/if}
          </button>
        {/each}
      {/if}
    </div>
  </div>
{/if}
{#if openPopover === 'goal'}
  <div
    data-inline-pop
    class="absolute z-40 left-2 top-full mt-1 bg-mantle border border-surface1 rounded shadow-lg w-64 max-h-72 overflow-hidden flex flex-col"
    role="dialog"
    aria-label="Pick goal"
  >
    <div class="p-2 border-b border-surface1">
      <input
        data-inline-pop
        bind:value={goalQuery}
        placeholder="filter goals…"
        class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text placeholder-dim focus:outline-none focus:border-primary"
      />
    </div>
    <div class="overflow-y-auto flex-1 py-1">
      <button
        type="button"
        data-inline-pop
        onclick={() => void setGoal(null)}
        disabled={busy}
        class="w-full text-left px-3 py-1 text-xs text-dim hover:bg-surface0 hover:text-text disabled:opacity-50 flex items-center gap-2"
      >
        <span class="text-base leading-none">—</span>
        <span>no goal</span>
      </button>
      {#if goalsLoading}
        <div class="px-3 py-2 text-[10px] text-dim italic">loading…</div>
      {:else if filteredGoals.length === 0}
        <div class="px-3 py-2 text-[10px] text-dim italic">no matches</div>
      {:else}
        {#each filteredGoals as g (g.id)}
          <button
            type="button"
            data-inline-pop
            onclick={() => void setGoal(g.id)}
            disabled={busy}
            class="w-full text-left px-3 py-1 text-xs hover:bg-surface0 disabled:opacity-50 flex flex-col {task.goalId === g.id ? 'text-primary' : 'text-text'}"
          >
            <span class="flex items-center justify-between gap-2">
              <span class="truncate">{g.title || g.id}</span>
              {#if task.goalId === g.id}
                <span class="text-[10px] text-primary flex-shrink-0">✓</span>
              {/if}
            </span>
            {#if g.title && g.title !== g.id}
              <span class="text-[9px] text-dim font-mono truncate">{g.id}</span>
            {/if}
          </button>
        {/each}
      {/if}
    </div>
  </div>
{/if}
