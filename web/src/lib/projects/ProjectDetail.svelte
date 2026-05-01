<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Project, type ProjectGoal, type Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import GoalEditor from './GoalEditor.svelte';
  import TaskRow from '$lib/components/TaskRow.svelte';

  let { project, onClose, onUpdated, onDeleted }: {
    project: Project;
    onClose: () => void;
    onUpdated: () => void | Promise<void>;
    onDeleted: (name: string) => void | Promise<void>;
  } = $props();

  // Local edit buffer — committed via patch on blur or save.
  let editingDescription = $state(false);
  let descBuf = $state('');
  let editingNextAction = $state(false);
  let nextActionBuf = $state('');
  let editingName = $state(false);
  let nameBuf = $state('');

  let projectTasks = $state<Task[]>([]);
  let loadingTasks = $state(false);
  let showCompletedTasks = $state(false);

  async function loadTasks() {
    loadingTasks = true;
    try {
      // Pull ALL tasks; project membership = matching project field OR
      // notePath under project's folder. Server already does this matching
      // for the projectView decoration so we mirror the same logic here.
      const r = await api.listTasks({});
      const folder = (project.folder ?? '').replace(/\/$/, '');
      projectTasks = r.tasks.filter((t) => {
        if (t.projectId === project.name) return true;
        if (folder && t.notePath.startsWith(folder + '/')) return true;
        return false;
      });
    } catch (e) {
      console.error(e);
    } finally {
      loadingTasks = false;
    }
  }

  $effect(() => {
    void project.name;
    loadTasks();
  });

  async function patch(p: Partial<Project>): Promise<boolean> {
    try {
      await api.patchProject(project.name, p);
      await onUpdated();
      return true;
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
      return false;
    }
  }

  async function commitDescription() {
    editingDescription = false;
    if (descBuf !== (project.description ?? '')) await patch({ description: descBuf });
  }
  async function commitNextAction() {
    editingNextAction = false;
    if (nextActionBuf !== (project.next_action ?? '')) await patch({ next_action: nextActionBuf });
  }
  async function commitName() {
    editingName = false;
    if (nameBuf && nameBuf !== project.name) await patch({ name: nameBuf });
  }

  async function setStatus(status: string) {
    await patch({ status });
  }

  async function setColor(color: string) {
    await patch({ color });
  }

  async function setPriority(priority: number) {
    await patch({ priority });
  }

  async function setDueDate(due_date: string) {
    await patch({ due_date });
  }

  async function setTags(raw: string) {
    const tags = raw.split(',').map((t) => t.trim()).filter(Boolean);
    await patch({ tags });
  }

  async function setFolder(folder: string) {
    await patch({ folder });
  }

  async function updateGoals(goals: ProjectGoal[]) {
    await patch({ goals });
  }

  async function deleteProject() {
    if (!confirm(`Delete project "${project.name}"? Tasks won't be removed.`)) return;
    try {
      await api.deleteProject(project.name);
      await onDeleted(project.name);
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  function colorVar(c?: string): string {
    const map: Record<string, string> = {
      red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
      blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
      peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
      lavender: 'primary', flamingo: 'error'
    };
    return `var(--color-${map[c ?? ''] ?? 'secondary'})`;
  }

  const colorOptions = ['blue', 'green', 'mauve', 'peach', 'red', 'yellow', 'pink', 'lavender', 'teal', 'sapphire', 'flamingo'];
  const categoryOptions = ['development', 'social-media', 'personal', 'business', 'writing', 'research', 'health', 'finance', 'other'];
  const statusOptions = ['active', 'paused', 'completed', 'archived'];
  const priorityLabels = ['none', 'low', 'medium', 'high', 'highest'];

  let progressPct = $derived(Math.round((project.progress ?? 0) * 100));

  let openTasks = $derived(projectTasks.filter((t) => !t.done));
  let doneTasks = $derived(projectTasks.filter((t) => t.done));
</script>

<div class="h-full flex flex-col overflow-hidden">
  <!-- Header -->
  <header class="px-4 py-3 border-b border-surface1 flex-shrink-0 flex items-center gap-2">
    <button
      onclick={onClose}
      aria-label="back"
      class="md:hidden w-9 h-9 -ml-2 flex items-center justify-center text-subtext hover:text-primary"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </button>
    <span class="w-3 h-3 rounded-full flex-shrink-0" style="background: {colorVar(project.color)}"></span>
    {#if editingName}
      <input
        bind:value={nameBuf}
        onblur={commitName}
        onkeydown={(e) => { if (e.key === 'Enter') commitName(); else if (e.key === 'Escape') editingName = false; }}
        autofocus
        class="text-base sm:text-lg font-semibold flex-1 px-1 -mx-1 bg-surface0 border border-primary rounded text-text outline-none"
      />
    {:else}
      <button
        onclick={() => { nameBuf = project.name; editingName = true; }}
        class="text-base sm:text-lg font-semibold text-text truncate flex-1 text-left hover:text-primary"
        title="click to rename"
      >{project.name}</button>
    {/if}
    <select
      value={project.status ?? 'active'}
      onchange={(e) => setStatus((e.target as HTMLSelectElement).value)}
      class="text-xs px-2 py-1 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary"
    >
      {#each statusOptions as s}<option value={s}>{s}</option>{/each}
    </select>
    <button
      onclick={deleteProject}
      title="delete project"
      class="w-9 h-9 flex items-center justify-center text-dim hover:text-error rounded"
      aria-label="delete"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
      </svg>
    </button>
  </header>

  <div class="flex-1 overflow-y-auto">
    <div class="max-w-3xl mx-auto p-4 sm:p-6 space-y-6">
      <!-- Progress bar -->
      <section>
        <div class="flex items-baseline justify-between mb-1.5">
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Progress</h3>
          <span class="text-xs text-subtext font-mono">
            {progressPct}%
            {#if project.tasksTotal != null && project.tasksTotal > 0}
              · <span class="text-dim">{project.tasksDone}/{project.tasksTotal} tasks</span>
            {/if}
          </span>
        </div>
        <div class="h-2 rounded-full bg-surface0 overflow-hidden">
          <div
            class="h-full transition-all"
            style="width: {progressPct}%; background: {colorVar(project.color)}"
          ></div>
        </div>
      </section>

      <!-- Description -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-1.5">Description</h3>
        {#if editingDescription}
          <textarea
            bind:value={descBuf}
            onblur={commitDescription}
            onkeydown={(e) => { if (e.key === 'Escape') editingDescription = false; }}
            autofocus
            rows="3"
            class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
          ></textarea>
        {:else}
          <button
            onclick={() => { descBuf = project.description ?? ''; editingDescription = true; }}
            class="w-full text-left px-3 py-2 text-sm rounded hover:bg-surface0 {project.description ? 'text-text' : 'text-dim italic'}"
          >{project.description || 'click to add a description…'}</button>
        {/if}
      </section>

      <!-- Next Action (highlight chip) -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-1.5">Next action</h3>
        {#if editingNextAction}
          <input
            bind:value={nextActionBuf}
            onblur={commitNextAction}
            onkeydown={(e) => { if (e.key === 'Enter') commitNextAction(); else if (e.key === 'Escape') editingNextAction = false; }}
            autofocus
            class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
          />
        {:else}
          <button
            onclick={() => { nextActionBuf = project.next_action ?? ''; editingNextAction = true; }}
            class="w-full text-left px-3 py-2.5 rounded text-sm border border-warning/30 bg-warning/10 text-warning hover:bg-warning/20 {!project.next_action ? 'italic opacity-70' : 'font-medium'}"
          >→ {project.next_action || 'what\'s the next concrete step?'}</button>
        {/if}
      </section>

      <!-- Goals + milestones -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">Goals & milestones</h3>
        <GoalEditor goals={project.goals ?? []} onChange={updateGoals} />
      </section>

      <!-- Linked tasks -->
      <section>
        <div class="flex items-baseline justify-between mb-2">
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Tasks · {projectTasks.length}</h3>
          <a
            href="/tasks?project={encodeURIComponent(project.name)}"
            class="text-xs text-secondary hover:underline"
          >open in /tasks →</a>
        </div>
        {#if loadingTasks && projectTasks.length === 0}
          <div class="text-xs text-dim">loading…</div>
        {:else if projectTasks.length === 0}
          <div class="text-xs text-dim italic">No tasks linked. Tag a task with <code class="text-secondary">project:{project.name}</code> or place it under <code class="text-secondary">{project.folder || '<no folder>'}</code>.</div>
        {:else}
          <div class="space-y-px">
            {#each openTasks.slice(0, 25) as t (t.id)}
              <TaskRow task={t} onChanged={loadTasks} />
            {/each}
          </div>
          {#if doneTasks.length > 0}
            <button
              onclick={() => (showCompletedTasks = !showCompletedTasks)}
              class="mt-2 text-[11px] text-dim hover:text-text"
            >{showCompletedTasks ? '▾' : '▸'} {doneTasks.length} completed</button>
            {#if showCompletedTasks}
              <div class="space-y-px mt-1 opacity-70">
                {#each doneTasks.slice(0, 25) as t (t.id)}
                  <TaskRow task={t} onChanged={loadTasks} />
                {/each}
              </div>
            {/if}
          {/if}
        {/if}
      </section>

      <!-- Metadata grid -->
      <section class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-3 pt-4 border-t border-surface1">
        <div>
          <label for="prj-folder" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Folder</label>
          <input
            id="prj-folder"
            value={project.folder ?? ''}
            onblur={(e) => setFolder((e.target as HTMLInputElement).value)}
            placeholder="e.g. Projects/foo"
            class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          />
        </div>
        <div>
          <label for="prj-due" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Due date</label>
          <input
            id="prj-due"
            type="date"
            value={project.due_date ?? ''}
            onchange={(e) => setDueDate((e.target as HTMLInputElement).value)}
            class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          />
        </div>
        <div>
          <label for="prj-tags" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Tags</label>
          <input
            id="prj-tags"
            value={(project.tags ?? []).join(', ')}
            onblur={(e) => setTags((e.target as HTMLInputElement).value)}
            placeholder="comma, separated"
            class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          />
        </div>
        <div>
          <label for="prj-cat" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Category</label>
          <select
            id="prj-cat"
            value={project.category ?? ''}
            onchange={(e) => patch({ category: (e.target as HTMLSelectElement).value })}
            class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          >
            <option value="">—</option>
            {#each categoryOptions as c}<option value={c}>{c}</option>{/each}
          </select>
        </div>
        <div>
          <span class="text-[11px] uppercase tracking-wider text-dim block mb-1">Priority</span>
          <div class="flex gap-1">
            {#each priorityLabels as label, i}
              <button
                onclick={() => setPriority(i)}
                class="flex-1 px-1 py-1 text-[11px] rounded {project.priority === i ? 'bg-primary text-mantle' : 'bg-surface0 text-subtext hover:bg-surface1'}"
              >{label}</button>
            {/each}
          </div>
        </div>
        <div>
          <span class="text-[11px] uppercase tracking-wider text-dim block mb-1">Color</span>
          <div class="flex gap-1.5 flex-wrap">
            {#each colorOptions as c}
              <button
                onclick={() => setColor(c)}
                aria-label="color {c}"
                class="w-6 h-6 rounded-full border-2 {project.color === c ? 'border-text' : 'border-surface1'}"
                style="background: {colorVar(c)}"
              ></button>
            {/each}
          </div>
        </div>
      </section>

      <footer class="text-[11px] text-dim pt-2 border-t border-surface1 flex justify-between">
        <span>created {project.created_at || '—'}</span>
        <span>updated {project.updated_at || '—'}</span>
      </footer>
    </div>
  </div>
</div>
