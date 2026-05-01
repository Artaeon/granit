<script lang="ts">
  import type { Task } from '$lib/api';

  let { task, compact = false }: { task: Task; compact?: boolean } = $props();

  const today = new Date().toISOString().slice(0, 10);

  function dueClass(d?: string): string {
    if (!d) return 'text-dim';
    if (d < today) return 'text-error';
    if (d === today) return 'text-warning';
    return 'text-dim';
  }

  function priorityBadge(p: number): { label: string; cls: string } | null {
    if (p === 1) return { label: 'P1', cls: 'bg-error/20 text-error' };
    if (p === 2) return { label: 'P2', cls: 'bg-warning/20 text-warning' };
    if (p === 3) return { label: 'P3', cls: 'bg-info/20 text-info' };
    return null;
  }

  let badge = $derived(priorityBadge(task.priority));
  let scheduledFmt = $derived.by(() => {
    if (!task.scheduledStart) return '';
    return new Date(task.scheduledStart).toLocaleString([], {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    });
  });
</script>

{#if badge}
  <span class="text-[10px] font-mono px-1.5 rounded {badge.cls} flex-shrink-0">{badge.label}</span>
{/if}
{#if task.dueDate}
  <span class="text-xs {dueClass(task.dueDate)} flex-shrink-0">{compact ? task.dueDate : 'due ' + task.dueDate}</span>
{/if}
{#if task.scheduledStart}
  <span class="text-xs text-info flex-shrink-0">⏰ {scheduledFmt}</span>
{/if}
{#if task.tags && task.tags.length > 0 && !compact}
  <span class="text-xs text-dim flex-shrink-0">{task.tags.map((t) => '#' + t).join(' ')}</span>
{/if}
{#if task.triage && task.triage !== 'done' && !compact}
  <span class="text-[10px] uppercase px-1.5 rounded bg-surface1 text-subtext flex-shrink-0">{task.triage}</span>
{/if}
