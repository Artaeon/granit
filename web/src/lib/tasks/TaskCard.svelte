<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, type Task } from '$lib/api';
  import { inlineMd } from '$lib/util/inlineMd';
  import SnoozePicker from './SnoozePicker.svelte';
  import { activeTimer, minutesByTaskId, fmtDuration } from '$lib/stores/timer';

  let {
    task = $bindable(),
    compact = false,
    onChanged,
    selectedIds = $bindable(new Set<string>()),
    onOpenDetail,
    onContextMenu
  }: {
    task: Task;
    compact?: boolean;
    onChanged?: (t: Task) => void;
    selectedIds?: Set<string>;
    /** Open the detail drawer for this task. Bound to the explicit
     *  "open detail" button + the right-click menu — clicking the
     *  title itself enters inline edit. */
    onOpenDetail?: (t: Task) => void;
    /** Right-click / long-press hook. The page wires a TaskContextMenu
     *  to this so the user gets a discoverable surface for triage / link
     *  actions without losing the existing hover affordances. */
    onContextMenu?: (t: Task, x: number, y: number) => void;
  } = $props();

  import { tick } from 'svelte';

  let editing = $state(false);
  let editText = $state(task.text);
  let editPriority = $state(task.priority);
  let editDue = $state(task.dueDate ?? '');
  let busy = $state(false);
  let editInputEl: HTMLInputElement | undefined = $state();
  let snoozePickerOpen = $state(false);
  let snoozePickerAnchor: HTMLElement | undefined = $state();

  function priorityClass(p: number): string {
    if (p === 1) return 'border-error';
    if (p === 2) return 'border-warning';
    if (p === 3) return 'border-info';
    return 'border-surface1';
  }

  function priorityBadge(p: number): { label: string; cls: string } | null {
    if (p === 1) return { label: 'P1', cls: 'bg-error/20 text-error' };
    if (p === 2) return { label: 'P2', cls: 'bg-warning/20 text-warning' };
    if (p === 3) return { label: 'P3', cls: 'bg-info/20 text-info' };
    return null;
  }

  // Snooze active = SnoozedUntil exists AND is in the future.
  function isSnoozed(t: Task): boolean {
    if (!t.snoozedUntil) return false;
    const sn = new Date(t.snoozedUntil);
    if (isNaN(sn.getTime())) return false;
    return sn.getTime() > Date.now();
  }

  function relSnooze(iso: string): string {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    const diff = d.getTime() - Date.now();
    const mins = Math.round(diff / 60_000);
    if (mins < 60) return `${mins}m`;
    const hrs = Math.round(mins / 60);
    if (hrs < 24) return `${hrs}h`;
    const days = Math.round(hrs / 24);
    if (days < 7) return `${days}d`;
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

  // Triage cycle order matches granit's UX: inbox → triaged → scheduled → done → dropped → snoozed → inbox
  const triageOrder: Array<NonNullable<Task['triage']>> = ['inbox', 'triaged', 'scheduled', 'done', 'dropped', 'snoozed'];
  function nextTriage(cur?: string): NonNullable<Task['triage']> {
    const i = triageOrder.indexOf((cur as NonNullable<Task['triage']>) || 'inbox');
    return triageOrder[(i + 1) % triageOrder.length];
  }
  function triageTone(t?: string): string {
    if (t === 'inbox') return 'subtext';
    if (t === 'triaged') return 'info';
    if (t === 'scheduled') return 'primary';
    if (t === 'done') return 'success';
    if (t === 'dropped') return 'dim';
    if (t === 'snoozed') return 'warning';
    return 'subtext';
  }

  async function toggle(e: Event) {
    e.stopPropagation();
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { done: !task.done });
      task = updated;
      onChanged?.(updated);
    } finally {
      busy = false;
    }
  }

  async function cycleTriage(e: Event) {
    e.stopPropagation();
    const nxt = nextTriage(task.triage);
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { triage: nxt });
      task = updated;
      onChanged?.(updated);
    } finally {
      busy = false;
    }
  }

  async function applySnooze(until: string) {
    snoozePickerOpen = false;
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { snoozedUntil: until });
      task = updated;
      onChanged?.(updated);
    } finally {
      busy = false;
    }
  }

  // Time tracking: clock in/out for this task. The Server's active-
  // timer state is the source of truth — we just call the endpoint
  // and the WS frame back updates the global activeTimer store.
  async function toggleClock(e: Event) {
    e.stopPropagation();
    busy = true;
    try {
      const isRunning = $activeTimer && $activeTimer.taskId === task.id;
      if (isRunning) {
        await api.clockOut();
      } else {
        await api.clockIn({ notePath: task.notePath, taskText: task.text, taskId: task.id });
      }
    } finally {
      busy = false;
    }
  }

  async function unsnooze(e: Event) {
    e.stopPropagation();
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { snoozedUntil: '' });
      task = updated;
      onChanged?.(updated);
    } finally {
      busy = false;
    }
  }

  function toggleSelect(e: Event) {
    e.stopPropagation();
    const next = new Set(selectedIds);
    if (next.has(task.id)) next.delete(task.id);
    else next.add(task.id);
    selectedIds = next;
  }

  function startEdit(e: Event) {
    e.stopPropagation();
    editing = true;
    editText = task.text;
    editPriority = task.priority;
    editDue = task.dueDate ?? '';
    tick().then(() => editInputEl?.focus());
  }

  async function saveEdit(e: Event) {
    e?.preventDefault();
    e?.stopPropagation();
    if (!editText.trim()) {
      editing = false;
      return;
    }
    busy = true;
    try {
      const patch: Parameters<typeof api.patchTask>[1] = {};
      if (editText !== task.text) patch.text = editText.trim();
      if (editPriority !== task.priority) patch.priority = editPriority;
      if (editDue !== (task.dueDate ?? '')) patch.dueDate = editDue;
      if (Object.keys(patch).length > 0) {
        const updated = await api.patchTask(task.id, patch);
        task = updated;
        onChanged?.(updated);
      }
      editing = false;
    } finally {
      busy = false;
    }
  }

  function cancelEdit() { editing = false; }

  // Title-click handler. Default behavior is inline edit (the user's
  // expected gesture from Notion/Things/Reminders). Holding cmd/ctrl
  // or middle-click escapes to onOpenDetail when wired, so power users
  // can still jump to the drawer without leaving the keyboard.
  function onTitleClick(e: MouseEvent) {
    e.stopPropagation();
    if ((e.metaKey || e.ctrlKey) && onOpenDetail) {
      onOpenDetail(task);
      return;
    }
    startEdit(e);
  }

  // Long-press surfaces the context menu on touch devices, where
  // there's no native right-click. 500ms threshold matches the
  // platform conventions.
  let longPressTimer: ReturnType<typeof setTimeout> | null = null;
  function onTouchStart(e: TouchEvent) {
    if (!onContextMenu) return;
    const t0 = e.touches[0];
    if (!t0) return;
    const x = t0.clientX;
    const y = t0.clientY;
    longPressTimer = setTimeout(() => {
      onContextMenu?.(task, x, y);
      longPressTimer = null;
    }, 500);
  }
  function onTouchEnd() {
    if (longPressTimer) {
      clearTimeout(longPressTimer);
      longPressTimer = null;
    }
  }
  function onContextMenuEvent(e: MouseEvent) {
    if (!onContextMenu) return;
    e.preventDefault();
    onContextMenu(task, e.clientX, e.clientY);
  }

  function openNote(e: Event) {
    e.stopPropagation();
    goto(`/notes/${encodeURIComponent(task.notePath)}`);
  }

  let dueClass = $derived.by(() => {
    if (!task.dueDate) return 'text-dim';
    const today = new Date().toISOString().slice(0, 10);
    if (task.dueDate < today) return 'text-error';
    if (task.dueDate === today) return 'text-warning';
    return 'text-dim';
  });

  // Relative-aware due-date label. Reads naturally — "today", "tomorrow",
  // "yesterday", "in 3 days", "+2w", or a localised date for far-out
  // dates. Matches what users expect from Things / Reminders / Todoist
  // and keeps the task row uncluttered (was rendering raw "2026-05-15"
  // even for tomorrow).
  function dueLabel(due: string): string {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const [y, m, d] = due.split('-').map(Number);
    const date = new Date(y, m - 1, d);
    const days = Math.round((date.getTime() - today.getTime()) / 86_400_000);
    if (days === 0) return 'today';
    if (days === 1) return 'tomorrow';
    if (days === -1) return 'yesterday';
    if (days < 0 && days >= -6) return `${-days}d ago`;
    if (days > 0 && days <= 6) return `in ${days}d`;
    if (days < 0 && days >= -28) return `${Math.round(-days / 7)}w ago`;
    if (days > 0 && days <= 28) return `in ${Math.round(days / 7)}w`;
    return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }
  // Icon char per due-state — nothing fancy, just enough to let the eye
  // distinguish "overdue" (⚠) from "soon" (📅) at a glance.
  function dueIcon(due: string): string {
    const today = new Date().toISOString().slice(0, 10);
    if (due < today) return '⚠';
    if (due === today) return '⏰';
    return '📅';
  }

  let badge = $derived(priorityBadge(task.priority));
  let isSelected = $derived(selectedIds.has(task.id));
  let snoozed = $derived(isSnoozed(task));
  // Whether THIS task is the currently-running clock-in. Drives the
  // play/stop icon swap on the toggle button.
  let isThisRunning = $derived(!!$activeTimer && $activeTimer.taskId === task.id);
  // Indent for subtasks. Tasks with Indent>0 are nested under a parent
  // within the same note. Cap at 4 levels so deep trees don't push
  // content off-screen.
  let indentPx = $derived(Math.min(task.indent ?? 0, 4) * 18);
</script>

<div
  class="bg-surface0 border-l-2 {priorityClass(task.priority)} border border-surface1 rounded p-2 transition-colors group
    {isSelected ? 'ring-1 ring-primary' : 'hover:border-primary/40'}"
  class:opacity-60={task.done}
  class:opacity-50={snoozed}
  style="margin-left: {indentPx}px;"
  oncontextmenu={onContextMenuEvent}
  ontouchstart={onTouchStart}
  ontouchend={onTouchEnd}
  ontouchmove={onTouchEnd}
  ontouchcancel={onTouchEnd}
  role="article"
>
  {#if !editing}
    <div class="flex items-start gap-2">
      <!-- Bulk-select checkbox: visible on hover, or when at least one is selected. -->
      <label
        class="cursor-pointer mt-0.5 flex-shrink-0 transition-opacity
          {selectedIds.size > 0 ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'}"
        title="select for bulk action"
      >
        <input type="checkbox" checked={isSelected} onchange={toggleSelect} class="w-3.5 h-3.5 accent-primary cursor-pointer" />
      </label>

      <button
        onclick={toggle}
        disabled={busy}
        class="w-4 h-4 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center
          {task.done ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
        aria-label={task.done ? 'mark not done' : 'mark done'}
      >
        {#if task.done}
          <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
        {/if}
      </button>

      <div class="flex-1 min-w-0">
        <div class="flex items-baseline gap-2">
          <button
            onclick={onTitleClick}
            class="text-sm text-left flex-1 min-w-0 break-words {task.done ? 'line-through text-dim' : 'text-text'}"
            title="click to edit · cmd/ctrl-click to open details · right-click for actions"
          >
            {#if (task.indent ?? 0) > 0}
              <span class="text-dim opacity-60 mr-1">↳</span>
            {/if}
            {@html inlineMd(task.text)}
          </button>
          {#if badge}
            <span class="text-[10px] font-mono px-1.5 rounded {badge.cls} flex-shrink-0">{badge.label}</span>
          {/if}
          {#if task.estimatedMinutes}
            <span class="text-[10px] font-mono text-dim flex-shrink-0" title="estimate">{task.estimatedMinutes}m</span>
          {/if}
        </div>

        {#if !compact}
          <div class="flex flex-wrap items-center gap-1.5 mt-1.5 text-xs">
            {#if task.dueDate}
              <span
                class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] {dueClass} bg-surface1/40"
                title="due {task.dueDate}"
              >
                <span class="text-[9px]" aria-hidden="true">{dueIcon(task.dueDate)}</span>
                {dueLabel(task.dueDate)}
              </span>
            {/if}
            {#if task.scheduledStart}
              <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] text-primary bg-primary/10">
                <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>
                {new Date(task.scheduledStart).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })}
              </span>
            {/if}
            {#if snoozed && task.snoozedUntil}
              <button
                onclick={unsnooze}
                class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] text-warning bg-warning/10 hover:bg-warning/20"
                title="click to wake now"
              >
                <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
                {relSnooze(task.snoozedUntil)}
              </button>
            {/if}
            {#if task.tags && task.tags.length > 0}
              <span class="text-dim">{task.tags.map((t) => '#' + t).join(' ')}</span>
            {/if}
            {#if task.projectId}
              <span
                class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] text-secondary bg-secondary/10"
                title="project: {task.projectId}"
              >
                <span aria-hidden="true">📁</span>
                <span class="truncate max-w-[8rem]">{task.projectId}</span>
              </span>
            {/if}
            {#if task.goalId}
              <span
                class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] text-info bg-info/10 font-mono"
                title="goal: {task.goalId}"
              >
                <span aria-hidden="true">🎯</span>{task.goalId}
              </span>
            {/if}
            {#if task.deadlineId}
              <span
                class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] text-warning bg-warning/10"
                title="deadline: {task.deadlineId}"
              >
                <span aria-hidden="true">⏰</span>
                <span class="font-mono">deadline</span>
              </span>
            {/if}
            <button
              onclick={cycleTriage}
              class="text-[10px] uppercase px-1.5 py-0.5 rounded transition-colors"
              style="color: var(--color-{triageTone(task.triage)}); background: color-mix(in srgb, var(--color-{triageTone(task.triage)}) 12%, transparent);"
              title="click to cycle triage state"
            >{task.triage || 'inbox'}</button>
            {#if task.dependsOn && task.dependsOn.length > 0}
              <span class="text-[10px] text-dim" title="depends on {task.dependsOn.join(', ')}">↳ {task.dependsOn.length} dep{task.dependsOn.length !== 1 ? 's' : ''}</span>
            {/if}
            {#if task.recurrence}
              <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] text-info bg-info/10" title="recurring {task.recurrence}">
                <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 12a9 9 0 0 1 15-6.7L21 8M3 16l3 3 3-3M21 12a9 9 0 0 1-15 6.7L3 16"/></svg>
                {task.recurrence}
              </span>
            {/if}
            {#if task.notes}
              <span class="text-[10px] text-dim" title={task.notes}>📝</span>
            {/if}
            {#if $minutesByTaskId[task.id]}
              <span class="text-[10px] text-dim font-mono" title="total tracked">{fmtDuration($minutesByTaskId[task.id] * 60)}</span>
            {/if}
            <span class="flex-1"></span>
            <button
              onclick={toggleClock}
              disabled={busy}
              class="opacity-0 group-hover:opacity-100 disabled:opacity-50 {isThisRunning ? 'text-success !opacity-100' : 'text-dim hover:text-success'}"
              title={isThisRunning ? 'stop tracking' : 'start tracking'}
              aria-label={isThisRunning ? 'stop tracking' : 'start tracking'}
            >
              {#if isThisRunning}
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="currentColor"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>
              {:else}
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="currentColor"><path d="M5 4l14 8-14 8z"/></svg>
              {/if}
            </button>
            <button
              bind:this={snoozePickerAnchor}
              onclick={(e) => { e.stopPropagation(); snoozePickerOpen = !snoozePickerOpen; }}
              aria-label="snooze"
              class="text-dim hover:text-warning opacity-0 group-hover:opacity-100"
              title="snooze"
            >
              <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
              </svg>
            </button>
            <button onclick={startEdit} class="text-dim hover:text-text opacity-0 group-hover:opacity-100">edit</button>
            {#if onOpenDetail}
              <button
                onclick={(e) => { e.stopPropagation(); onOpenDetail!(task); }}
                aria-label="open details"
                title="open details"
                class="text-dim hover:text-primary opacity-0 group-hover:opacity-100"
              >
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 8v4M12 16h.01"/></svg>
              </button>
            {/if}
            <button onclick={openNote} class="text-dim hover:text-secondary opacity-0 group-hover:opacity-100" aria-label="open note">↗</button>
          </div>
        {/if}
      </div>
    </div>
    {#if snoozePickerOpen}
      <SnoozePicker
        anchor={snoozePickerAnchor}
        onClose={() => (snoozePickerOpen = false)}
        onPick={applySnooze}
      />
    {/if}
  {:else}
    <div role="presentation" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
    <form onsubmit={saveEdit} class="space-y-2">
      <input
        bind:value={editText}
        bind:this={editInputEl}
        onkeydown={(e) => { if (e.key === 'Escape') { e.preventDefault(); cancelEdit(); } }}
        class="w-full px-2 py-2 bg-mantle border border-surface1 rounded text-base sm:text-sm text-text focus:outline-none focus:border-primary"
      />
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <select bind:value={editPriority} class="bg-mantle border border-surface1 rounded px-2 py-1 text-text text-sm">
          <option value={0}>no priority</option>
          <option value={1}>P1</option>
          <option value={2}>P2</option>
          <option value={3}>P3</option>
        </select>
        <input type="date" bind:value={editDue} class="bg-mantle border border-surface1 rounded px-2 py-1 text-text text-sm" />
        <span class="flex-1 min-w-0"></span>
        <button type="button" onclick={cancelEdit} class="px-3 py-1 text-dim hover:text-text">cancel</button>
        <button type="submit" disabled={busy} class="px-3 py-1 bg-primary text-on-primary rounded disabled:opacity-50">save</button>
      </div>
    </form>
    </div>
  {/if}
</div>
