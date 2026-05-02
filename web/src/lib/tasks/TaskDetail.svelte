<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, type Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import Drawer from '$lib/components/Drawer.svelte';

  // TaskDetail is the side-drawer that pops open when the user clicks
  // a task card. Editable fields not already inline-editable on the card:
  // recurrence, free-form notes, dependency text. Read-only summary of
  // metadata (created/completed/updated) for context.

  let {
    open = $bindable(false),
    task,
    onChanged
  }: {
    open?: boolean;
    task: Task | null;
    onChanged?: () => void | Promise<void>;
  } = $props();

  let notesBuf = $state('');
  let recurrenceBuf = $state('');
  let busy = $state(false);

  // Resync local buffers whenever the modal opens for a different task.
  $effect(() => {
    if (open && task) {
      notesBuf = task.notes ?? '';
      recurrenceBuf = task.recurrence ?? '';
    }
  });

  async function patch(patch: Parameters<typeof api.patchTask>[1]) {
    if (!task) return;
    busy = true;
    try {
      await api.patchTask(task.id, patch);
      await onChanged?.();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = false;
    }
  }

  async function commitNotes() {
    if (!task) return;
    if (notesBuf === (task.notes ?? '')) return;
    await patch({ notes: notesBuf });
  }

  async function setRecurrence(r: string) {
    recurrenceBuf = r;
    await patch({ recurrence: r });
  }

  async function setPriority(p: number) { await patch({ priority: p }); }
  async function toggleDone() { if (task) await patch({ done: !task.done }); }
  async function setTriage(state: NonNullable<Task['triage']>) { await patch({ triage: state }); }

  function close() { open = false; }
  function openNote() {
    if (!task) return;
    goto(`/notes/${encodeURIComponent(task.notePath)}`);
    close();
  }

  function fmtDate(s?: string): string {
    if (!s) return '—';
    const d = new Date(s);
    return d.toLocaleString();
  }

  const recurrenceOptions: { value: string; label: string }[] = [
    { value: '', label: 'none' },
    { value: 'daily', label: 'daily' },
    { value: 'weekly', label: 'weekly' },
    { value: 'monthly', label: 'monthly' },
    { value: '3x-week', label: '3× / week' }
  ];
  const triageStates: NonNullable<Task['triage']>[] = ['inbox', 'triaged', 'scheduled', 'done', 'dropped', 'snoozed'];
</script>

<Drawer bind:open side="right" responsive width="w-full sm:w-96 md:w-[28rem]">
  {#if task}
    <div class="h-full flex flex-col overflow-hidden">
      <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <h2 class="text-sm font-semibold text-text flex-1 truncate">Task details</h2>
        <button onclick={close} aria-label="close" class="text-dim hover:text-text">×</button>
      </header>

      <div class="flex-1 overflow-y-auto p-4 space-y-4">
        <!-- Title + done toggle -->
        <section class="flex items-start gap-2">
          <button
            onclick={toggleDone}
            disabled={busy}
            class="w-5 h-5 mt-0.5 rounded border flex items-center justify-center flex-shrink-0
              {task.done ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
            aria-label={task.done ? 'mark not done' : 'mark done'}
          >
            {#if task.done}
              <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
            {/if}
          </button>
          <div class="flex-1 min-w-0">
            <h3 class="text-base font-medium text-text break-words {task.done ? 'line-through text-dim' : ''}">{task.text}</h3>
            <a href="/notes/{encodeURIComponent(task.notePath)}" onclick={openNote} class="text-xs text-secondary hover:underline font-mono">
              {task.notePath}
            </a>
          </div>
        </section>

        <!-- Priority pills -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Priority</h4>
          <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs">
            {#each [{ p: 0, label: 'none' }, { p: 1, label: 'P1' }, { p: 2, label: 'P2' }, { p: 3, label: 'P3' }] as o}
              <button
                onclick={() => setPriority(o.p)}
                disabled={busy}
                class="flex-1 px-3 py-1.5 {task.priority === o.p ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
              >{o.label}</button>
            {/each}
          </div>
        </section>

        <!-- Triage row -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Triage</h4>
          <div class="grid grid-cols-3 gap-1 text-xs">
            {#each triageStates as st}
              <button
                onclick={() => setTriage(st)}
                disabled={busy}
                class="px-2 py-1 rounded {(task.triage || 'inbox') === st ? 'bg-primary text-mantle' : 'bg-surface0 text-subtext hover:bg-surface1'}"
              >{st}</button>
            {/each}
          </div>
        </section>

        <!-- Recurrence -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Recurrence</h4>
          <div class="flex flex-wrap gap-1 text-xs">
            {#each recurrenceOptions as o}
              <button
                onclick={() => setRecurrence(o.value)}
                disabled={busy}
                class="px-2.5 py-1 rounded {recurrenceBuf === o.value ? 'bg-info text-mantle' : 'bg-surface0 text-subtext hover:bg-surface1'}"
              >{o.label}</button>
            {/each}
          </div>
          <p class="text-[10px] text-dim mt-1">Writes a <code>#daily</code>/<code>#weekly</code>/etc. tag onto the task line.</p>
        </section>

        <!-- Free-form notes -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Notes</h4>
          <textarea
            bind:value={notesBuf}
            onblur={commitNotes}
            placeholder="any details, links, context…"
            rows="4"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          ></textarea>
          <p class="text-[10px] text-dim mt-1">Stored in the task sidecar — not in the markdown.</p>
        </section>

        <!-- Read-only metadata -->
        <section class="pt-4 border-t border-surface1">
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-2">Metadata</h4>
          <dl class="text-xs space-y-1">
            <div class="flex gap-2"><dt class="text-dim w-24">ID</dt><dd class="text-text font-mono">{task.id}</dd></div>
            {#if task.createdAt}
              <div class="flex gap-2"><dt class="text-dim w-24">Created</dt><dd class="text-text">{fmtDate(task.createdAt)}</dd></div>
            {/if}
            {#if task.completedAt}
              <div class="flex gap-2"><dt class="text-dim w-24">Completed</dt><dd class="text-text">{fmtDate(task.completedAt)}</dd></div>
            {/if}
            {#if task.updatedAt}
              <div class="flex gap-2"><dt class="text-dim w-24">Updated</dt><dd class="text-text">{fmtDate(task.updatedAt)}</dd></div>
            {/if}
            {#if task.estimatedMinutes}
              <div class="flex gap-2"><dt class="text-dim w-24">Estimate</dt><dd class="text-text">{task.estimatedMinutes} min</dd></div>
            {/if}
            {#if task.dependsOn && task.dependsOn.length}
              <div class="flex gap-2"><dt class="text-dim w-24">Depends on</dt><dd class="text-text">{task.dependsOn.join(', ')}</dd></div>
            {/if}
          </dl>
        </section>
      </div>
    </div>
  {/if}
</Drawer>
