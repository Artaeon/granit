<script lang="ts">
  import { api, todayISO, type Task } from '$lib/api';
  import { inlineMd } from '$lib/util/inlineMd';
  import { toast } from '$lib/components/toast';

  let { task = $bindable(), onChanged }: { task: Task; onChanged?: (t: Task) => void } = $props();
  let busy = $state(false);

  // Optimistic toggle — flip local state instantly, replace with the
  // server's authoritative response when it arrives. Rollback on
  // error so the user never sees a desync.
  async function toggle() {
    const prev = task;
    task = { ...task, done: !task.done };
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { done: task.done });
      task = updated;
      onChanged?.(updated);
    } catch (e) {
      task = prev;
      toast.error('failed to toggle task');
    } finally {
      busy = false;
    }
  }

  function priorityClass(p: number): string {
    if (p === 1) return 'text-error';
    if (p === 2) return 'text-warning';
    if (p === 3) return 'text-info';
    return 'text-dim';
  }

  // Relative date label + tone for the due-date chip.
  //   today          → primary
  //   tomorrow       → info
  //   overdue        → error
  //   within 7 days  → warning
  //   further out    → subtext (neutral)
  // Done tasks always use a muted neutral.
  function dueChip(dueISO: string, done: boolean): { label: string; tone: string; icon: string } | null {
    if (!dueISO) return null;
    const today = todayISO();
    const [y, m, d] = dueISO.split('-').map(Number);
    const due = new Date(y, m - 1, d);
    const t = new Date();
    t.setHours(0, 0, 0, 0);
    const diffMs = due.getTime() - t.getTime();
    const diffDays = Math.round(diffMs / 86_400_000);

    let label = '';
    if (diffDays === 0) label = 'today';
    else if (diffDays === 1) label = 'tomorrow';
    else if (diffDays === -1) label = 'yesterday';
    else if (diffDays > 0 && diffDays < 7) label = due.toLocaleDateString(undefined, { weekday: 'short' });
    else if (diffDays < 0) label = `${Math.abs(diffDays)}d ago`;
    else label = due.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });

    let tone = 'subtext';
    if (done) tone = 'subtext';
    else if (dueISO < today) tone = 'error';
    else if (diffDays === 0) tone = 'primary';
    else if (diffDays === 1) tone = 'info';
    else if (diffDays > 0 && diffDays <= 7) tone = 'warning';

    return { label, tone, icon: dueISO < today && !done ? '!' : '' };
  }

  function fmtTime(iso: string): string {
    const d = new Date(iso);
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
  }

  let due = $derived(task.dueDate ? dueChip(task.dueDate, task.done) : null);
  let scheduled = $derived(task.scheduledStart ? fmtTime(task.scheduledStart) : null);
</script>

<div class="flex items-baseline gap-2 py-1 group">
  <button
    onclick={toggle}
    disabled={busy}
    class="w-4 h-4 rounded border flex-shrink-0 flex items-center justify-center transition-colors
      {task.done ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
    aria-label={task.done ? 'mark not done' : 'mark done'}
  >
    {#if task.done}
      <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
    {/if}
  </button>

  {#if task.priority > 0}
    <span class="text-xs font-mono {priorityClass(task.priority)}">!{task.priority}</span>
  {/if}

  <span class="flex-1 text-sm min-w-0 truncate {task.done ? 'line-through text-dim' : 'text-text'}">{@html inlineMd(task.text)}</span>

  {#if scheduled}
    <span
      class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium tabular-nums"
      style="background: color-mix(in srgb, var(--color-primary) 14%, transparent); color: var(--color-primary);"
      title="scheduled at {scheduled}"
    >
      <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round">
        <circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>
      </svg>
      {scheduled}
    </span>
  {/if}

  {#if due}
    <span
      class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium whitespace-nowrap"
      style="background: color-mix(in srgb, var(--color-{due.tone}) 14%, transparent); color: var(--color-{due.tone}); border: 1px solid color-mix(in srgb, var(--color-{due.tone}) 30%, transparent);"
      title="due {task.dueDate}"
    >
      {#if due.icon}<span class="font-bold">{due.icon}</span>{/if}
      <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round">
        <rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>
      </svg>
      {due.label}
    </span>
  {/if}

  {#if task.tags && task.tags.length > 0}
    <span class="text-xs text-dim hidden group-hover:inline">
      {task.tags.map((t) => '#' + t).join(' ')}
    </span>
  {/if}

  <a href="/notes/{encodeURIComponent(task.notePath)}" class="text-xs text-dim hover:text-secondary opacity-0 group-hover:opacity-100" aria-label="open note">
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M7 17L17 7M9 7h8v8"/>
    </svg>
  </a>
</div>
