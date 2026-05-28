<script lang="ts">
  // Smart-section grouped task list. Stream N.
  // Each section is tinted + bordered by urgency:
  //   OVERDUE → red tint + error border-l-2 + animated pulse dot
  //   TODAY   → amber tint + warning border-l-2
  //   TOMORROW → primary dot, no tint
  //   THIS WEEK → dim, no tint
  //   LATER → dim, collapsible (default collapsed)
  //   NO DATE → dim, collapsible (default collapsed)
  //   DONE → success, collapsible (default collapsed)
  //
  // Section collapse state persists to localStorage via the parent
  // (collapsedSections record) so the user's choice survives reloads.
  // Empty sections render as a single-line muted header.
  import TaskCard from '$lib/tasks/TaskCard.svelte';
  import type { Task } from '$lib/api';
  import { focusOnMount } from '$lib/util/focusOnMount';

  export type ListGroup = {
    key: string;
    label: string;
    tasks: Task[];
    deepLink?: string;
  };

  type Props = {
    groups: ListGroup[];
    filtered: Task[];
    cursorIdx: number;
    compactCards: boolean;
    childCount: Map<string, number>;
    collapsedIds: Set<string>;
    collapsedSections: Record<string, boolean>;
    groupAddKey: string | null;
    groupAddText: string;
    groupAddBusy: boolean;
    selectedIds: Set<string>;
    isHiddenByCollapse: (id: string, c: Set<string>) => boolean;
    onToggleSection: (key: string) => void;
    onToggleCollapse: (id: string) => void;
    onChanged: () => void | Promise<void>;
    onOpenDetail: (t: Task) => void;
    onContextMenu: (t: Task, x: number, y: number) => void;
    onStartGroupAdd: (key: string) => void;
    onCancelGroupAdd: () => void;
    onSubmitGroupAdd: (key: string) => void;
    onGroupAddTextChange: (v: string) => void;
  };

  let {
    groups,
    filtered,
    cursorIdx,
    compactCards,
    childCount,
    collapsedIds,
    collapsedSections,
    groupAddKey,
    groupAddText = $bindable(),
    groupAddBusy,
    selectedIds = $bindable(),
    isHiddenByCollapse,
    onToggleSection,
    onToggleCollapse,
    onChanged,
    onOpenDetail,
    onContextMenu,
    onStartGroupAdd,
    onCancelGroupAdd,
    onSubmitGroupAdd
  }: Props = $props();

  // Visual style table per bucket key. Sectioning is keyed by the
  // listGroups output, which uses these stable keys for the 'due'
  // group: overdue / today / tomorrow / this_week / later / no_date /
  // done. Non-'due' groupings (priority / tag / project / goal /
  // deadline / note) fall through to neutral styling.
  function sectionStyle(key: string) {
    switch (key) {
      case 'overdue':
        return {
          dot: 'bg-error',
          label: 'text-error',
          tint: 'bg-error/[0.06]',
          border: 'border-error',
          pulse: true,
          collapseDefault: false
        };
      case 'today':
        return {
          dot: 'bg-warning',
          label: 'text-warning',
          tint: 'bg-warning/[0.06]',
          border: 'border-warning',
          pulse: false,
          collapseDefault: false
        };
      case 'tomorrow':
        return {
          dot: 'bg-primary',
          label: 'text-text',
          tint: '',
          border: 'border-surface1',
          pulse: false,
          collapseDefault: false
        };
      case 'this_week':
        return {
          dot: 'bg-secondary',
          label: 'text-text',
          tint: '',
          border: 'border-surface1',
          pulse: false,
          collapseDefault: false
        };
      case 'later':
        return {
          dot: 'bg-surface2',
          label: 'text-dim',
          tint: '',
          border: 'border-surface1',
          pulse: false,
          collapseDefault: true
        };
      case 'no_date':
        return {
          dot: 'bg-info',
          label: 'text-dim',
          tint: '',
          border: 'border-surface1',
          pulse: false,
          collapseDefault: true
        };
      case 'done':
        return {
          dot: 'bg-success',
          label: 'text-success',
          tint: '',
          border: 'border-surface1',
          pulse: false,
          collapseDefault: true
        };
      default:
        return {
          dot: 'bg-surface2',
          label: 'text-text',
          tint: '',
          border: 'border-surface1',
          pulse: false,
          collapseDefault: false
        };
    }
  }

  // Resolve effective collapse state. The parent persists explicit
  // user toggles; absence falls through to the per-section default
  // (overdue/today/tomorrow/this_week open, later/no_date/done
  // collapsed). The "explicit ? value : default" pattern means a user
  // who explicitly opens 'later' keeps it open across reloads but a
  // first-time visitor sees the recommended initial shape.
  function isCollapsed(key: string): boolean {
    const explicit = collapsedSections[key];
    if (explicit === true) return true;
    if (explicit === false) return false;
    return sectionStyle(key).collapseDefault;
  }
</script>

<div class="space-y-3 max-w-3xl">
  {#each groups as g (g.key)}
    {@const s = sectionStyle(g.key)}
    {@const collapsed = isCollapsed(g.key)}
    {@const empty = g.tasks.length === 0}
    <section class="rounded {s.tint} {s.tint ? `border-l-2 ${s.border} pl-2` : ''}">
      <!-- Section header. text-sm font-semibold per Stream N spec —
           bigger than the previous text-xs uppercase tracking-wider
           so the visual hierarchy actually lands. Bulk action sits
           on the right; collapse chevron on the left. -->
      <header class="flex items-center gap-2 py-2 {empty ? 'opacity-60' : ''}">
        <button
          type="button"
          onclick={() => onToggleSection(g.key)}
          class="inline-flex items-center gap-2 group flex-1 min-w-0 text-left"
          aria-expanded={!collapsed}
          aria-controls={`sect-${g.key}`}
          title={collapsed ? 'Expand section' : 'Collapse section'}
        >
          <!-- Chevron rotates 90° when expanded. -->
          <svg
            viewBox="0 0 24 24"
            class="w-3 h-3 text-dim flex-shrink-0 transition-transform {collapsed ? '' : 'rotate-90'}"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M9 6l6 6-6 6" />
          </svg>
          <span class="w-2 h-2 rounded-full flex-shrink-0 {s.dot} {s.pulse ? 'animate-pulse' : ''}" aria-hidden="true"></span>
          <h2 class="text-sm font-semibold {s.label} truncate">
            {g.label}
          </h2>
          <span class="text-xs text-dim font-mono tabular-nums flex-shrink-0">{g.tasks.length}</span>
          {#if g.key === 'overdue' && g.tasks.length > 0}
            <span class="ml-0.5 px-1 py-0 bg-error/10 text-error text-[9px] tracking-wider rounded uppercase font-bold flex-shrink-0" title="These tasks are past their due date">
              past due
            </span>
          {/if}
        </button>
        {#if g.deepLink}
          <a
            href={g.deepLink}
            class="text-[10px] text-secondary hover:underline tracking-normal flex-shrink-0"
            title="open {g.label}"
          >open ↗</a>
        {/if}
        {#if !empty}
          <button
            type="button"
            onclick={() => onStartGroupAdd(g.key)}
            class="text-[11px] text-dim hover:text-primary tracking-normal font-mono flex-shrink-0 px-1.5 py-0.5 rounded hover:bg-surface0"
            title="add a task to this group ({g.label})"
            aria-label="add task to {g.label}"
          >+</button>
        {/if}
      </header>

      {#if !collapsed && !empty}
        <div id={`sect-${g.key}`}>
          {#if groupAddKey === g.key}
            <div class="mb-1.5 flex items-center gap-1.5">
              <input
                type="text"
                value={groupAddText}
                oninput={(e) => (groupAddText = (e.currentTarget as HTMLInputElement).value)}
                onkeydown={(e) => {
                  if (e.key === 'Enter') { e.preventDefault(); onSubmitGroupAdd(g.key); }
                  else if (e.key === 'Escape') { e.preventDefault(); onCancelGroupAdd(); }
                }}
                onblur={() => { if (!groupAddText.trim() && !groupAddBusy) onCancelGroupAdd(); }}
                placeholder="new task in {g.label}…"
                use:focusOnMount
                disabled={groupAddBusy}
                class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1 text-sm text-text placeholder-dim focus:outline-none focus:border-primary disabled:opacity-50"
              />
              <button
                type="button"
                onclick={() => onSubmitGroupAdd(g.key)}
                disabled={groupAddBusy || !groupAddText.trim()}
                class="text-[11px] px-2 py-1 rounded bg-primary text-on-primary font-medium hover:opacity-90 disabled:opacity-40"
              >{groupAddBusy ? '…' : 'add'}</button>
            </div>
          {/if}
          <div class="space-y-1.5">
            {#each g.tasks.filter((tt) => !isHiddenByCollapse(tt.id, collapsedIds)) as t (t.id)}
              <div
                data-task-id={t.id}
                class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}
              >
                <TaskCard
                  task={t}
                  compact={compactCards}
                  hasChildren={(childCount.get(t.id) ?? 0) > 0}
                  childCount={childCount.get(t.id) ?? 0}
                  collapsed={collapsedIds.has(t.id)}
                  onToggleCollapse={() => onToggleCollapse(t.id)}
                  onChanged={onChanged}
                  bind:selectedIds
                  onOpenDetail={onOpenDetail}
                  onContextMenu={onContextMenu}
                />
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </section>
  {/each}
</div>
