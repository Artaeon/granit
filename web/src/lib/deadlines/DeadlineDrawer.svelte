<script lang="ts">
  // Create/edit deadline drawer. Stream BB. Lifted from the
  // /deadlines page so the page concerns itself with layout +
  // shortcuts, not form-field plumbing. The drawer owns no state of
  // its own — every field is a $bindable so the parent retains the
  // single source of truth (matches how the prior inline form
  // worked, just hosted in its own file).
  import Drawer from '$lib/components/Drawer.svelte';
  import type {
    Deadline,
    DeadlineImportance,
    DeadlineStatus,
    Goal,
    Project,
    Task,
    Venture
  } from '$lib/api';

  type Props = {
    open: boolean;
    editing: Deadline | null;
    busy: boolean;
    fTitle: string;
    fDate: string;
    fDescription: string;
    fImportance: DeadlineImportance;
    fStatus: DeadlineStatus;
    fGoalId: string;
    fProject: string;
    fVenture: string;
    fTaskIds: string[];
    goals: Goal[];
    projects: Project[];
    ventures: Venture[];
    openTasks: Task[];
    tasksLoaded: boolean;
    onSave: () => void | Promise<void>;
    onDelete: () => void | Promise<void>;
    onClose: () => void;
    onToggleTaskLink: (id: string) => void;
  };

  let {
    open = $bindable(),
    editing,
    busy,
    fTitle = $bindable(),
    fDate = $bindable(),
    fDescription = $bindable(),
    fImportance = $bindable(),
    fStatus = $bindable(),
    fGoalId = $bindable(),
    fProject = $bindable(),
    fVenture = $bindable(),
    fTaskIds,
    goals,
    projects,
    ventures,
    openTasks,
    tasksLoaded,
    onSave,
    onDelete,
    onClose,
    onToggleTaskLink
  }: Props = $props();
</script>

<Drawer bind:open side="right" responsive width="w-full sm:w-96 md:w-[28rem]">
  <div class="h-full flex flex-col overflow-hidden">
    <header class="px-3 py-2 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
      <h2 class="text-sm font-semibold text-text flex-1 truncate">
        {editing ? 'Edit deadline' : 'New deadline'}
      </h2>
      <button
        onclick={onClose}
        aria-label="close"
        class="text-dim hover:text-text text-lg leading-none"
      >×</button>
    </header>

    <div class="flex-1 overflow-y-auto p-2 sm:p-3 space-y-3">
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
              class="flex-1 px-3 py-1.5 capitalize {fImportance === i ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
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
              class="flex-1 px-2 py-1.5 capitalize {fStatus === s ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
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
        <label for="d-venture" class="block text-xs uppercase tracking-wider text-dim mb-1">Linked venture</label>
        <input
          id="d-venture"
          bind:value={fVenture}
          list="d-ventures-list"
          placeholder="venture name (optional)"
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        />
        {#if ventures.length > 0}
          <datalist id="d-ventures-list">
            {#each ventures as v}<option value={v.name}></option>{/each}
          </datalist>
        {/if}
        <p class="text-[11px] text-dim mt-1">
          Free-text — links the deadline to a Venture record so it shows up on /ventures and the venture's project rollup.
        </p>
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
                  onchange={() => onToggleTaskLink(t.id)}
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
          onclick={onDelete}
          disabled={busy}
          class="px-3 py-1.5 text-xs text-error hover:bg-surface0 rounded disabled:opacity-50"
        >Delete</button>
      {/if}
      <span class="flex-1"></span>
      <button
        onclick={onClose}
        disabled={busy}
        class="px-3 py-1.5 text-sm text-subtext hover:bg-surface0 rounded disabled:opacity-50"
      >Cancel</button>
      <button
        onclick={onSave}
        disabled={busy}
        class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded hover:opacity-90 disabled:opacity-50"
      >{editing ? 'Save' : 'Create'}</button>
    </footer>
  </div>
</Drawer>
