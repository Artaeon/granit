<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';

  // Carryover + habits band that lives at the very top of every daily
  // note. Two collapsible groups so a clean day takes minimal space.

  let { onChanged }: { onChanged: () => void | Promise<void> } = $props();

  type Ctx = Awaited<ReturnType<typeof api.dailyContext>>;
  let ctx = $state<Ctx | null>(null);
  let busy = $state<string | null>(null); // task id of in-flight done toggle
  let collapsed = $state<{ carry: boolean; habits: boolean }>({ carry: false, habits: false });

  async function load() {
    try {
      ctx = await api.dailyContext();
    } catch {
      ctx = null;
    }
  }

  onMount(() => {
    load();
    // Refresh on any note write to today's daily, since habit checks
    // are inferred from the daily's body text.
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed' || ev.type === 'task.changed') load();
    });
  });

  async function markCarryDone(id: string) {
    busy = id;
    try {
      await api.patchTask(id, { done: true });
      // Optimistic-y: drop the row immediately so the UI doesn't
      // show a "done" item awaiting the WS round-trip.
      if (ctx) ctx = { ...ctx, carryover: ctx.carryover.filter((c) => c.id !== id) };
      await onChanged();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = null;
    }
  }

  function priorityTone(p?: number): string {
    if (p === 1) return 'error';
    if (p === 2) return 'warning';
    if (p === 3) return 'info';
    return 'subtext';
  }

  function relDue(due?: string): string {
    if (!due) return '';
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const [y, m, d] = due.split('-').map(Number);
    const t = new Date(y, m - 1, d).getTime();
    const days = Math.round((today.getTime() - t) / 86_400_000);
    if (days === 1) return 'yesterday';
    if (days > 1) return `${days}d ago`;
    return due;
  }
</script>

{#if ctx && (ctx.carryover.length > 0 || ctx.habits.length > 0)}
  <div class="px-3 py-2 border-b border-surface1 bg-surface0/30 flex-shrink-0 space-y-2">
    <div class="max-w-3xl mx-auto space-y-2">
      {#if ctx.carryover.length > 0}
        <section>
          <button
            onclick={() => (collapsed = { ...collapsed, carry: !collapsed.carry })}
            class="text-[11px] uppercase tracking-wider text-warning hover:text-text flex items-center gap-1"
          >
            <span>{collapsed.carry ? '▸' : '▾'}</span>
            Carryover · {ctx.carryover.length}
            <span class="text-dim font-normal normal-case ml-2 text-[10px] italic">open tasks due before today</span>
          </button>
          {#if !collapsed.carry}
            <ul class="mt-1 space-y-px pl-2">
              {#each ctx.carryover as c (c.id)}
                <li class="flex items-center gap-2 text-sm group">
                  <button
                    onclick={() => markCarryDone(c.id)}
                    disabled={busy === c.id}
                    class="w-3.5 h-3.5 rounded border border-surface2 hover:border-success flex items-center justify-center flex-shrink-0"
                    aria-label="mark done"
                    title="mark done"
                  ></button>
                  {#if c.priority}
                    <span class="text-[10px] font-mono px-1 rounded" style="color: var(--color-{priorityTone(c.priority)}); background: color-mix(in srgb, var(--color-{priorityTone(c.priority)}) 12%, transparent);">P{c.priority}</span>
                  {/if}
                  <span class="text-text flex-1 truncate">{c.text}</span>
                  {#if c.dueDate}
                    <span class="text-[11px] text-warning whitespace-nowrap">{relDue(c.dueDate)}</span>
                  {/if}
                  <a
                    href="/notes/{encodeURIComponent(c.notePath)}"
                    class="text-[11px] text-dim hover:text-secondary opacity-0 group-hover:opacity-100"
                    title="open source note"
                    aria-label="open source note"
                  >↗</a>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/if}

      {#if ctx.habits.length > 0}
        <section>
          <button
            onclick={() => (collapsed = { ...collapsed, habits: !collapsed.habits })}
            class="text-[11px] uppercase tracking-wider text-info hover:text-text flex items-center gap-1"
          >
            <span>{collapsed.habits ? '▸' : '▾'}</span>
            Habits · {ctx.habits.filter((h) => h.done).length}/{ctx.habits.length}
            <span class="text-dim font-normal normal-case ml-2 text-[10px] italic">today</span>
          </button>
          {#if !collapsed.habits}
            <ul class="mt-1 flex flex-wrap gap-1.5 pl-2">
              {#each ctx.habits as h}
                <li
                  class="text-xs px-2 py-1 rounded inline-flex items-center gap-1.5 border"
                  style="border-color: color-mix(in srgb, var(--color-{h.done ? 'success' : 'surface1'}) 60%, transparent); background: {h.done ? 'color-mix(in srgb, var(--color-success) 12%, transparent)' : 'transparent'}; color: var(--color-{h.done ? 'success' : 'subtext'});"
                  title={h.done ? 'done today' : 'not yet — write "- [x] ' + h.text + '" in the daily'}
                >
                  {#if h.done}
                    <svg viewBox="0 0 12 12" class="w-3 h-3"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                  {:else}
                    <span class="w-3 h-3 rounded-sm border border-surface2 inline-block"></span>
                  {/if}
                  <span>{h.text}</span>
                </li>
              {/each}
            </ul>
            <p class="text-[10px] text-dim italic mt-1.5 pl-2">
              Habits sync from <code>config.json</code>'s <code>daily_recurring_tasks</code>. Tick by writing
              <code>- [x] {'{habit}'}</code> in the daily note.
            </p>
          {/if}
        </section>
      {/if}
    </div>
  </div>
{/if}
