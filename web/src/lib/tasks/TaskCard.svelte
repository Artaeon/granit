<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, type Task } from '$lib/api';
  import { inlineMd } from '$lib/util/inlineMd';
  import { cleanTaskText } from '$lib/util/taskParse';
  import { toast } from '$lib/components/toast';
  import SnoozePicker from './SnoozePicker.svelte';
  import { activeTimer, minutesByTaskId, fmtDuration } from '$lib/stores/timer';

  let {
    task = $bindable(),
    compact = false,
    onChanged,
    selectedIds = $bindable(new Set<string>()),
    onOpenDetail,
    onContextMenu,
    hasChildren = false,
    childCount = 0,
    collapsed = false,
    onToggleCollapse
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
    /** True when this task has at least one child (descendant) — page
     *  computes this from indent + line ordering and passes it down.
     *  Drives whether the chevron renders. */
    hasChildren?: boolean;
    /** Number of direct + transitive children — surfaced as "(N)" in
     *  the chevron's title attribute so the user sees how much they're
     *  hiding before they collapse. */
    childCount?: number;
    /** Whether this task's children are currently collapsed. */
    collapsed?: boolean;
    /** Click handler for the chevron — page flips its collapsedIds set. */
    onToggleCollapse?: () => void;
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

  // Optimistic local update — flip the field immediately so the UI
  // doesn't sit waiting on a 100-400ms round-trip + parent refetch
  // before the user sees their click. The server response is still
  // authoritative; we replace the local task with whatever it returns.
  // On failure we'd ideally roll back, but the parent's refetch (via
  // onChanged) will reconcile within ~half a second either way, so a
  // toast + leave-as-is keeps the code simple. If a future user
  // complaint about ghost-updates surfaces, swap in explicit rollback.
  async function toggle(e: Event) {
    e.stopPropagation();
    const prev = task;
    task = { ...task, done: !task.done };
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { done: task.done });
      task = updated;
      onChanged?.(updated);
    } catch (err) {
      task = prev;
      toast.error('failed to toggle task');
    } finally {
      busy = false;
    }
  }

  async function cycleTriage(e: Event) {
    e.stopPropagation();
    const nxt = nextTriage(task.triage);
    const prev = task;
    task = { ...task, triage: nxt };
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { triage: nxt });
      task = updated;
      onChanged?.(updated);
    } catch (err) {
      task = prev;
      toast.error('failed to update triage');
    } finally {
      busy = false;
    }
  }

  async function applySnooze(until: string) {
    snoozePickerOpen = false;
    const prev = task;
    task = { ...task, snoozedUntil: until };
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { snoozedUntil: until });
      task = updated;
      onChanged?.(updated);
    } catch (err) {
      task = prev;
      toast.error('failed to snooze');
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
    const prev = task;
    task = { ...task, snoozedUntil: undefined };
    busy = true;
    try {
      const updated = await api.patchTask(task.id, { snoozedUntil: '' });
      task = updated;
      onChanged?.(updated);
    } catch (err) {
      task = prev;
      toast.error('failed to unsnooze');
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

  // Swipe-to-action state. Tracks horizontal drag distance and
  // surfaces a colored backing layer behind the card showing what
  // will fire if the user releases now.
  //   • Swipe right → toggle done (green ✓)
  //   • Swipe left  → snooze (amber 💤)
  // Threshold is 80px — below that the card snaps back. Vertical
  // movement (scroll intent) cancels the swipe so list-scrolling
  // isn't accidentally hijacked.
  const SWIPE_THRESHOLD = 80;
  let swipeStartX = 0;
  let swipeStartY = 0;
  let swipeOffset = $state(0);
  let swipeActive = $state(false);

  function onTouchStart(e: TouchEvent) {
    const t0 = e.touches[0];
    if (!t0) return;
    swipeStartX = t0.clientX;
    swipeStartY = t0.clientY;
    swipeOffset = 0;
    swipeActive = false;
    if (onContextMenu) {
      longPressTimer = setTimeout(() => {
        onContextMenu?.(task, t0.clientX, t0.clientY);
        longPressTimer = null;
      }, 500);
    }
  }
  function onTouchMove(e: TouchEvent) {
    const t0 = e.touches[0];
    if (!t0) return;
    const dx = t0.clientX - swipeStartX;
    const dy = t0.clientY - swipeStartY;
    // Once the user has moved ~10px, decide whether this is a swipe
    // (horizontal) or a scroll (vertical) and lock in. If vertical
    // wins we cancel the swipe and let the list scroll naturally.
    if (!swipeActive && (Math.abs(dx) > 10 || Math.abs(dy) > 10)) {
      if (Math.abs(dx) > Math.abs(dy)) {
        swipeActive = true;
        // Cancel long-press; user is swiping, not holding.
        if (longPressTimer) { clearTimeout(longPressTimer); longPressTimer = null; }
      } else {
        // Vertical scroll — abort the swipe entirely.
        swipeStartX = NaN;
        return;
      }
    }
    if (!swipeActive || Number.isNaN(swipeStartX)) return;
    // Cap the visual offset at 1.5× threshold so the card doesn't
    // fly off-screen on a vigorous swipe — the action is committed
    // at threshold either way.
    swipeOffset = Math.max(-SWIPE_THRESHOLD * 1.5, Math.min(SWIPE_THRESHOLD * 1.5, dx));
    // Don't preventDefault on the touchmove event itself — the user
    // may still pan vertically after the lock-in moment if their
    // gesture changes direction.
  }
  function onTouchEnd(e: TouchEvent) {
    if (longPressTimer) {
      clearTimeout(longPressTimer);
      longPressTimer = null;
    }
    if (!swipeActive) return;
    const finalOffset = swipeOffset;
    swipeOffset = 0;
    swipeActive = false;
    // Commit the action if past threshold. Right = done; left =
    // snooze for 1 day (a sensible default). The user can still
    // open the SnoozePicker explicitly from the context menu / card
    // button if they want a custom date.
    if (finalOffset > SWIPE_THRESHOLD) {
      e.preventDefault();
      void toggle(new Event('swipe-done'));
    } else if (finalOffset < -SWIPE_THRESHOLD) {
      e.preventDefault();
      const tomorrow = new Date();
      tomorrow.setDate(tomorrow.getDate() + 1);
      tomorrow.setHours(9, 0, 0, 0);
      void applySnooze(tomorrow.toISOString());
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

  // Overdue / today derived states. Drives the soft red / amber tint
  // on the card background — a passive cue that doesn't compete with
  // the priority border but signals urgency at a glance.
  let isOverdue = $derived.by(() => {
    if (task.done || !task.dueDate) return false;
    return task.dueDate < new Date().toISOString().slice(0, 10);
  });
  let isDueToday = $derived.by(() => {
    if (task.done || !task.dueDate) return false;
    return task.dueDate === new Date().toISOString().slice(0, 10);
  });
</script>

<!-- Outer container handles touch events + hosts the swipe-action
     backing layer behind the card. Position is relative so the
     backing can overlay full-bleed; overflow hidden so a swipe
     past the edge doesn't bleed past the card border. -->
<div
  class="task-card-wrap relative overflow-hidden rounded"
  style="margin-left: {indentPx}px;"
  ontouchstart={onTouchStart}
  ontouchmove={onTouchMove}
  ontouchend={onTouchEnd}
  ontouchcancel={onTouchEnd}
  role="presentation"
>
  <!-- Action backing — visible only while swiping. Right swipe
       reveals "✓ Done" on the LEFT (the user is dragging right, so
       the action label appears on the side they swept FROM). Left
       swipe reveals "💤 Snooze" on the right side. -->
  {#if swipeActive && swipeOffset !== 0}
    <div
      class="absolute inset-0 flex items-center px-3 text-sm font-semibold pointer-events-none
        {swipeOffset > 0 ? 'justify-start bg-success/30 text-success' : 'justify-end bg-warning/30 text-warning'}"
      aria-hidden="true"
    >
      {#if swipeOffset > 0}
        <span class="flex items-center gap-1.5">
          <span class="text-lg">✓</span>
          {Math.abs(swipeOffset) >= SWIPE_THRESHOLD ? (task.done ? 'Reopen' : 'Done') : 'Swipe right…'}
        </span>
      {:else}
        <span class="flex items-center gap-1.5">
          {Math.abs(swipeOffset) >= SWIPE_THRESHOLD ? 'Snooze 1d' : 'Swipe left…'}
          <span class="text-lg">💤</span>
        </span>
      {/if}
    </div>
  {/if}
<div
  class="task-card bg-surface0 border-l-2 {priorityClass(task.priority)} border border-surface1 rounded p-2 transition-all group relative
    {isSelected ? 'ring-1 ring-primary' : 'hover:border-primary/40 hover:bg-surface0/80'}
    {isOverdue ? 'task-card--overdue' : ''}
    {isDueToday ? 'task-card--today' : ''}
    {task.done ? 'task-card--done' : ''}"
  class:opacity-60={task.done}
  class:opacity-50={snoozed}
  class:task-card--swiping={swipeActive}
  style="transform: translateX({swipeOffset}px);"
  oncontextmenu={onContextMenuEvent}
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
          <!-- Subtask collapse chevron — only renders when the task
               has at least one child. Click flips collapsed state on
               the page-level set; the page filters descendants out
               of the rendered list. The chevron animates by rotating
               the same SVG so the visual state matches the data. -->
          {#if hasChildren && onToggleCollapse}
            <button
              onclick={(e) => { e.stopPropagation(); onToggleCollapse?.(); }}
              class="flex-shrink-0 mt-0.5 w-4 h-4 flex items-center justify-center text-dim hover:text-text rounded transition-transform"
              style:transform={collapsed ? 'rotate(-90deg)' : 'rotate(0deg)'}
              title={collapsed ? `Expand (${childCount} subtask${childCount === 1 ? '' : 's'} hidden)` : `Collapse (${childCount} subtask${childCount === 1 ? '' : 's'})`}
              aria-expanded={!collapsed}
              aria-label={collapsed ? 'Expand subtasks' : 'Collapse subtasks'}
            >
              <svg viewBox="0 0 16 16" class="w-3 h-3" fill="currentColor"><path d="M4 6l4 5 4-5z"/></svg>
            </button>
          {/if}
          <button
            onclick={onTitleClick}
            class="text-sm text-left flex-1 min-w-0 break-words {task.done ? 'line-through text-dim' : 'text-text'}"
            title="click to edit · cmd/ctrl-click to open details · right-click for actions"
          >
            {#if (task.indent ?? 0) > 0}
              <span class="text-dim opacity-60 mr-1">↳</span>
            {/if}
            {@html inlineMd(cleanTaskText(task.text))}
            {#if hasChildren && collapsed}
              <span class="ml-1 text-[10px] text-dim font-mono">+{childCount}</span>
            {/if}
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
</div>

<style>
  /* Soft urgency tints. The priority border (left rail) is the
     primary signal; these card-wide tints are secondary, set low
     enough that they don't fight the border but high enough that a
     scanning eye can pick out overdue rows in a long list. */
  .task-card--overdue {
    background: color-mix(in srgb, var(--color-error) 6%, var(--color-surface0));
  }
  .task-card--overdue:hover {
    background: color-mix(in srgb, var(--color-error) 10%, var(--color-surface0));
  }
  .task-card--today {
    background: color-mix(in srgb, var(--color-warning) 5%, var(--color-surface0));
  }
  /* Done state: strikethrough already comes from the title element;
     here we add a subtle hatched background so the card visually
     "fades out" without losing its shape. Animation is applied once
     the .task-card--done class arrives so the user sees the change. */
  .task-card--done {
    background-image: linear-gradient(
      135deg,
      transparent 0,
      transparent 8px,
      color-mix(in srgb, var(--color-text) 4%, transparent) 8px,
      color-mix(in srgb, var(--color-text) 4%, transparent) 16px
    );
    background-size: 16px 16px;
    animation: task-done-pop 280ms ease-out;
  }
  @keyframes task-done-pop {
    0%   { transform: scale(1);    opacity: 1;   }
    35%  { transform: scale(0.985); opacity: 0.7; }
    100% { transform: scale(1);    opacity: 0.6; }
  }
  /* Reduce-motion respect: skip the animation but keep the visual
     change so accessibility users still get the state cue. */
  @media (prefers-reduced-motion: reduce) {
    .task-card--done { animation: none; }
  }
  /* Swipe transform — when actively swiping, no transition so the
     card tracks the finger 1:1; on release the .task-card--swiping
     class is removed and the default transition kicks in to snap
     the card back to position. */
  .task-card-wrap .task-card {
    transition: transform 200ms ease-out, background-color 150ms;
    will-change: transform;
  }
  .task-card-wrap .task-card.task-card--swiping {
    transition: none;
  }
</style>
