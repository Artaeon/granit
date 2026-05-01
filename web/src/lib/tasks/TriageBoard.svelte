<script lang="ts">
  import type { Task } from '$lib/api';
  import TaskCard from './TaskCard.svelte';

  let { tasks, onChanged }: { tasks: Task[]; onChanged: () => void | Promise<void> } = $props();

  // Six columns mirroring TUI's TriageState. Order matches the user's
  // mental model of triage flow: anything new lands in inbox; the user
  // routes it to triaged (will do), scheduled (committed time), done,
  // dropped (deliberately not doing), or snoozed (later).
  const cols = [
    { key: 'inbox', label: 'Inbox', tone: 'subtext', help: 'untriaged' },
    { key: 'triaged', label: 'Triaged', tone: 'info', help: 'in-flight' },
    { key: 'scheduled', label: 'Scheduled', tone: 'primary', help: 'has a time' },
    { key: 'snoozed', label: 'Snoozed', tone: 'warning', help: 'waking soon' },
    { key: 'done', label: 'Done', tone: 'success', help: 'finished' },
    { key: 'dropped', label: 'Dropped', tone: 'dim', help: 'not doing' }
  ] as const;

  let grouped = $derived.by(() => {
    const m: Record<string, Task[]> = { inbox: [], triaged: [], scheduled: [], snoozed: [], done: [], dropped: [] };
    for (const t of tasks) {
      const k = t.triage || (t.done ? 'done' : 'inbox');
      (m[k] ??= []).push(t);
    }
    return m;
  });
</script>

<div class="flex gap-3 overflow-x-auto pb-3 h-full">
  {#each cols as c}
    {@const list = grouped[c.key] ?? []}
    <section class="w-72 flex-shrink-0 flex flex-col bg-surface0/50 border border-surface1 rounded">
      <header class="px-3 py-2 border-b border-surface1 flex items-baseline gap-2">
        <h3 class="text-xs uppercase tracking-wider font-medium" style="color: var(--color-{c.tone})">{c.label}</h3>
        <span class="text-[10px] text-dim">{list.length}</span>
        <span class="ml-auto text-[10px] text-dim italic">{c.help}</span>
      </header>
      <div class="flex-1 overflow-y-auto p-2 space-y-2 min-h-[8rem]">
        {#each list as t (t.id)}
          <TaskCard task={t} compact onChanged={onChanged} />
        {/each}
        {#if list.length === 0}
          <div class="text-[11px] text-dim italic text-center py-4">empty</div>
        {/if}
      </div>
    </section>
  {/each}
</div>
