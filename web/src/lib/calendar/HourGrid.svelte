<script lang="ts">
  import { onMount } from 'svelte';
  import type { CalendarEvent } from '$lib/api';
  import {
    eventDayKey,
    eventStartDate,
    eventTypeColor,
    fmtDateISO,
    fmtTime,
    isAllDay,
    isSameDay,
    layoutDay
  } from './utils';
  import { dragStore, type DraggedTask } from './dragStore';

  let {
    days,
    events,
    onClickEvent,
    onClickSlot,
    onSlotRange,
    onReschedule,
    onResize,
    onTaskDrop
  }: {
    days: Date[];
    events: CalendarEvent[];
    onClickEvent: (ev: CalendarEvent) => void;
    onClickSlot: (date: Date, hour: number, minute: number) => void;
    /** Called on click+drag (or single click → 30min default) on an empty
     *  slot. The page wires this to the unified create modal. */
    onSlotRange?: (start: Date, end: Date) => void;
    onReschedule?: (taskId: string, newStart: Date) => void | Promise<void>;
    /** Called when the user drags the bottom edge of an event to change
     *  its duration. Only fires for scheduled tasks (events.json events
     *  use a different code path: open the editor and adjust there). */
    onResize?: (taskId: string, durationMinutes: number) => void | Promise<void>;
    /** Called when a task from the backlog is dropped on a slot. The
     *  page wires this to api.patchTask({ scheduledStart, durationMinutes }).
     *  Discriminator for "task drop in progress" is the dragStore — when
     *  it's non-null, slot pointer handlers take the task-drop path
     *  instead of slot-drag-to-create. */
    onTaskDrop?: (taskId: string, start: Date, durationMinutes: number) => void | Promise<void>;
  } = $props();

  // Subscribe to the shared drag store. $derived isn't enough — we need
  // to peek at the value inside non-reactive pointer handlers, hence
  // the manual subscribe.
  let pendingTask = $state<DraggedTask | null>(null);
  onMount(() => dragStore.subscribe((v) => (pendingTask = v)));

  // Track the hovered slot during a task drag so we can render a ghost
  // even though slotDrag (used for drag-to-create) is left null. This
  // also gives us the start time on pointerup.
  let taskDragHover = $state<{ dayIdx: number; startMin: number } | null>(null);

  // Safety net: if a desktop drag releases OUTSIDE any slot (e.g. the
  // page header, the backlog itself), the slot's pointerup never fires
  // and the store stays set. A document-level pointerup with a tiny
  // delay clears it. The delay is so the slot's own pointerup can win
  // first if the user dropped inside a column.
  onMount(() => {
    const onDocUp = (e: PointerEvent) => {
      // Microtask defer so the slot's onpointerup (which also clears
      // the store via commitTaskDrop) gets to run first. If it cleared
      // the store, our update here is a no-op.
      queueMicrotask(() => {
        dragStore.update((cur) => {
          if (!cur) return cur;
          // Only clear desktop drags (real pointer ID). Mobile tap-pick
          // (pointerId === -1) survives until a slot consumes it.
          if (cur.pointerId === e.pointerId && cur.pointerId !== -1) return null;
          return cur;
        });
        taskDragHover = null;
      });
    };
    window.addEventListener('pointerup', onDocUp);
    window.addEventListener('pointercancel', onDocUp);
    return () => {
      window.removeEventListener('pointerup', onDocUp);
      window.removeEventListener('pointercancel', onDocUp);
    };
  });

  const HOUR_PX = 48;
  const HOURS = Array.from({ length: 24 }, (_, i) => i);

  let scrollEl: HTMLDivElement | undefined = $state();
  let innerGridEl: HTMLDivElement | undefined = $state();

  onMount(() => {
    if (scrollEl) {
      const n = new Date();
      const minutes = n.getHours() * 60 + n.getMinutes();
      scrollEl.scrollTop = Math.max(0, (minutes - 60) * (HOUR_PX / 60));
    }
  });

  let eventsByDay = $derived.by(() => {
    const m = new Map<string, CalendarEvent[]>();
    for (const ev of events) {
      const key = eventDayKey(ev);
      if (!key) continue;
      if (!m.has(key)) m.set(key, []);
      m.get(key)!.push(ev);
    }
    return m;
  });

  let now = $state(new Date());
  onMount(() => {
    const id = setInterval(() => (now = new Date()), 60_000);
    return () => clearInterval(id);
  });

  function nowMinutes(): number {
    return now.getHours() * 60 + now.getMinutes();
  }

  let railPx = $derived(days.length === 1 ? 48 : 44);
  let minColPx = $derived(days.length === 1 ? 0 : 92);
  let minWidth = $derived(`${railPx + days.length * minColPx}px`);
  let cols = $derived(`${railPx}px repeat(${days.length}, minmax(${minColPx}px, 1fr))`);

  function slotClick(d: Date, e: MouseEvent) {
    const target = e.currentTarget as HTMLDivElement;
    const rect = target.getBoundingClientRect();
    const y = e.clientY - rect.top + target.scrollTop;
    const minutes = Math.floor((y / HOUR_PX) * 60);
    const snapped = Math.round(minutes / 15) * 15;
    onClickSlot(d, Math.floor(snapped / 60), snapped % 60);
  }

  // ----- Drag-to-create (Google Calendar style) -----
  // Pointer-down on empty space in a day column starts a slot selection.
  // Drag extends the selection (15-min snap). Pointer-up calls onSlotRange
  // with the final start/end. A near-zero-distance drag is treated as a
  // single click and yields a 30-minute default range, mirroring Google
  // Calendar's UX.
  interface SlotDrag {
    pointerId: number;
    dayIdx: number;
    startMin: number;
    endMin: number;
    pressStart: number; // performance.now() for click vs. drag heuristic
  }
  let slotDrag = $state<SlotDrag | null>(null);

  function snapMin(yPx: number): number {
    return Math.max(0, Math.min(24 * 60, Math.round((yPx / HOUR_PX) * 60 / 15) * 15));
  }

  // Commit a task-drop at the given slot. Snapped to 15 min.
  function commitTaskDrop(dayIdx: number, slotEl: HTMLElement, clientY: number) {
    if (!pendingTask || !onTaskDrop) return;
    const rect = slotEl.getBoundingClientRect();
    const min = snapMin(clientY - rect.top + slotEl.scrollTop);
    const day = days[dayIdx];
    const start = new Date(day);
    start.setHours(Math.floor(min / 60), min % 60, 0, 0);
    const t = pendingTask;
    // Clear FIRST so the parent's onTaskDrop → load() doesn't race
    // a stale store value into the rerender.
    dragStore.set(null);
    taskDragHover = null;
    void onTaskDrop(t.taskId, start, t.durationMinutes);
  }

  function onSlotPointerDown(e: PointerEvent, dayIdx: number) {
    if (e.pointerType === 'mouse' && e.button !== 0) return;
    // Bail if the press lands on an existing event — those have their own
    // pointer handlers (drag-to-reschedule).
    if ((e.target as HTMLElement)?.closest('button')) return;

    // ─── Mobile tap-pick path ──────────────────────────────
    // pointerId === -1 in the store = "the user tapped a backlog
    // task and is now tapping a slot to drop it". Commit on the
    // pointerdown (we don't need a drag gesture — it's a discrete
    // tap-tap action). The slotDrag flow is bypassed entirely.
    if (pendingTask && pendingTask.pointerId === -1 && onTaskDrop) {
      commitTaskDrop(dayIdx, e.currentTarget as HTMLElement, e.clientY);
      e.preventDefault();
      return;
    }

    // ─── Desktop drag in progress ─────────────────────────
    // pendingTask non-null + real pointerId = the user pressed a
    // backlog row and is now over the grid (no capture, so this
    // pointerdown wouldn't normally fire mid-gesture — but it CAN
    // fire if the user clicked on the row, released, then clicked
    // a slot. Treat it the same as a tap-pick.)
    if (pendingTask && onTaskDrop) {
      commitTaskDrop(dayIdx, e.currentTarget as HTMLElement, e.clientY);
      e.preventDefault();
      return;
    }

    if (!onSlotRange) return; // page doesn't want drag-create — fall through to slotClick
    const target = e.currentTarget as HTMLElement;
    const rect = target.getBoundingClientRect();
    const min = snapMin(e.clientY - rect.top);
    slotDrag = { pointerId: e.pointerId, dayIdx, startMin: min, endMin: min, pressStart: performance.now() };
    target.setPointerCapture(e.pointerId);
    e.preventDefault();
  }

  function onSlotPointerMove(e: PointerEvent, dayIdx: number) {
    // Existing drag-to-create flow — only when slotDrag is active.
    if (slotDrag && slotDrag.pointerId === e.pointerId) {
      const target = e.currentTarget as HTMLElement;
      const rect = target.getBoundingClientRect();
      slotDrag.endMin = snapMin(e.clientY - rect.top);
      return;
    }

    // Task-drop ghost — the backlog set dragStore on pointerdown but
    // didn't capture the pointer, so move events route here naturally
    // as the cursor crosses the grid. We update taskDragHover so the
    // ghost (rendered below) tracks the cursor.
    if (pendingTask && pendingTask.pointerId !== -1) {
      const target = e.currentTarget as HTMLElement;
      const rect = target.getBoundingClientRect();
      const min = snapMin(e.clientY - rect.top);
      taskDragHover = { dayIdx, startMin: min };
    }
  }

  function onSlotPointerUp(e: PointerEvent, dayIdx: number) {
    // Existing drag-to-create commit.
    if (slotDrag && slotDrag.pointerId === e.pointerId) {
      const ds = slotDrag;
      slotDrag = null;
      const target = e.currentTarget as HTMLElement;
      if (target.hasPointerCapture(e.pointerId)) target.releasePointerCapture(e.pointerId);

      let startMin = Math.min(ds.startMin, ds.endMin);
      let endMin = Math.max(ds.startMin, ds.endMin);
      if (endMin - startMin < 15) endMin = startMin + 30;

      const day = days[ds.dayIdx];
      const start = new Date(day);
      start.setHours(Math.floor(startMin / 60), startMin % 60, 0, 0);
      const end = new Date(day);
      end.setHours(Math.floor(endMin / 60), endMin % 60, 0, 0);
      onSlotRange?.(start, end);
      return;
    }

    // Desktop task-drop commit — pointer was never captured, so this
    // pointerup fires naturally on whichever slot is under the cursor.
    if (pendingTask && pendingTask.pointerId !== -1 && onTaskDrop) {
      commitTaskDrop(dayIdx, e.currentTarget as HTMLElement, e.clientY);
    }
  }

  function onSlotPointerCancel(e: PointerEvent) {
    if (slotDrag?.pointerId === e.pointerId) slotDrag = null;
    // Cancel the hover ghost too so a stuck phantom doesn't linger.
    taskDragHover = null;
  }

  // Live-derived bounds for the ghost rectangle while dragging.
  let slotGhost = $derived.by(() => {
    if (!slotDrag) return null;
    const min = Math.min(slotDrag.startMin, slotDrag.endMin);
    const max = Math.max(slotDrag.startMin, slotDrag.endMin);
    return {
      dayIdx: slotDrag.dayIdx,
      startMin: min,
      endMin: max < min + 15 ? min + 30 : max
    };
  });

  function fmtMin(m: number): string {
    return `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`;
  }

  // ----- Drag-to-reschedule -----

  interface DragState {
    ev: CalendarEvent;
    pointerId: number;
    pickOffsetY: number; // px within the event where the user grabbed
    durationMinutes: number;
    startX: number;
    startY: number;
    ghostMinutes: number; // top in minutes
    ghostDayIdx: number;
    moved: boolean;
  }

  let drag = $state<DragState | null>(null);

  function onEventPointerDown(e: PointerEvent, ev: CalendarEvent, dayIdx: number) {
    if (!ev.taskId || !ev.start) return; // only scheduled tasks are draggable
    if (e.pointerType === 'mouse' && e.button !== 0) return;
    const target = e.currentTarget as HTMLElement;
    const rect = target.getBoundingClientRect();
    const start = new Date(ev.start);
    drag = {
      ev,
      pointerId: e.pointerId,
      pickOffsetY: e.clientY - rect.top,
      durationMinutes: ev.durationMinutes ?? Math.round(rect.height / (HOUR_PX / 60)),
      startX: e.clientX,
      startY: e.clientY,
      ghostMinutes: start.getHours() * 60 + start.getMinutes(),
      ghostDayIdx: dayIdx,
      moved: false
    };
    target.setPointerCapture(e.pointerId);
  }

  function onEventPointerMove(e: PointerEvent) {
    if (!drag || drag.pointerId !== e.pointerId) return;
    if (!innerGridEl) return;
    const dx = e.clientX - drag.startX;
    const dy = e.clientY - drag.startY;
    if (Math.abs(dx) > 4 || Math.abs(dy) > 4) drag.moved = true;

    const grid = innerGridEl.getBoundingClientRect();
    const yInGrid = e.clientY - grid.top - drag.pickOffsetY;
    const rawMin = (yInGrid / HOUR_PX) * 60;
    const snapped = Math.max(0, Math.min(24 * 60 - drag.durationMinutes, Math.round(rawMin / 15) * 15));
    drag.ghostMinutes = snapped;

    const xInGrid = e.clientX - grid.left - railPx;
    const colWidth = (grid.width - railPx) / days.length;
    const dayIdx = Math.max(0, Math.min(days.length - 1, Math.floor(xInGrid / colWidth)));
    drag.ghostDayIdx = dayIdx;
  }

  async function onEventPointerUp(e: PointerEvent) {
    if (!drag || drag.pointerId !== e.pointerId) return;
    const ds = drag;
    drag = null;
    const target = e.currentTarget as HTMLElement;
    if (target.hasPointerCapture(e.pointerId)) target.releasePointerCapture(e.pointerId);

    if (!ds.moved) {
      onClickEvent(ds.ev);
      return;
    }
    if (!ds.ev.taskId) return;
    const newDate = new Date(days[ds.ghostDayIdx]);
    newDate.setHours(Math.floor(ds.ghostMinutes / 60), ds.ghostMinutes % 60, 0, 0);
    if (onReschedule) await onReschedule(ds.ev.taskId, newDate);
  }

  function onEventPointerCancel(e: PointerEvent) {
    if (drag?.pointerId === e.pointerId) drag = null;
  }

  // ----- Resize handle (bottom edge of an event) -----
  // Drag the bottom of a scheduled-task event to extend/shorten it.
  // Snapped to 15 min, min 15 min total. Calls onResize(taskId, durMin)
  // which the parent wires to api.patchTask({ durationMinutes }).
  interface ResizeState {
    ev: CalendarEvent;
    pointerId: number;
    startMin: number;       // event's start, minutes from midnight
    initialDurationMin: number;
    pressY: number;
    ghostDuration: number;  // current candidate duration
  }
  let resize = $state<ResizeState | null>(null);

  function onResizePointerDown(e: PointerEvent, ev: CalendarEvent, startMin: number, durationMin: number) {
    if (!ev.taskId || !ev.start) return;
    if (e.pointerType === 'mouse' && e.button !== 0) return;
    e.stopPropagation();
    const target = e.currentTarget as HTMLElement;
    resize = {
      ev,
      pointerId: e.pointerId,
      startMin,
      initialDurationMin: durationMin,
      pressY: e.clientY,
      ghostDuration: durationMin
    };
    target.setPointerCapture(e.pointerId);
  }

  function onResizePointerMove(e: PointerEvent) {
    if (!resize || resize.pointerId !== e.pointerId) return;
    const deltaMin = ((e.clientY - resize.pressY) / HOUR_PX) * 60;
    const raw = resize.initialDurationMin + deltaMin;
    const snapped = Math.max(15, Math.min(24 * 60 - resize.startMin, Math.round(raw / 15) * 15));
    resize.ghostDuration = snapped;
  }

  async function onResizePointerUp(e: PointerEvent) {
    if (!resize || resize.pointerId !== e.pointerId) return;
    const rs = resize;
    resize = null;
    const target = e.currentTarget as HTMLElement;
    if (target.hasPointerCapture(e.pointerId)) target.releasePointerCapture(e.pointerId);
    if (rs.ghostDuration === rs.initialDurationMin) return;
    if (rs.ev.taskId && onResize) await onResize(rs.ev.taskId, rs.ghostDuration);
  }

  function onResizePointerCancel(e: PointerEvent) {
    if (resize?.pointerId === e.pointerId) resize = null;
  }
</script>

<div class="border border-surface1 rounded overflow-hidden bg-base flex flex-col h-full">
  <div class="overflow-x-auto flex flex-col flex-1 min-h-0">
    <div style="min-width: {minWidth}" class="flex-shrink-0">
      <div class="grid bg-mantle border-b border-surface1" style="grid-template-columns: {cols}">
        <div></div>
        {#each days as d}
          {@const isToday = isSameDay(d, new Date())}
          <div class="px-1 py-2 border-l border-surface1 text-center">
            <div class="text-[10px] {isToday ? 'text-primary' : 'text-dim'} uppercase tracking-wider">{d.toLocaleDateString(undefined, { weekday: 'short' })}</div>
            {#if isToday}
              <div class="mt-0.5 inline-flex items-center justify-center w-7 h-7 sm:w-8 sm:h-8 rounded-full bg-primary text-on-primary font-semibold text-base sm:text-lg">{d.getDate()}</div>
            {:else}
              <div class="text-lg sm:text-xl text-text">{d.getDate()}</div>
            {/if}
          </div>
        {/each}
      </div>

      <div class="grid border-b border-surface1 min-h-[28px]" style="grid-template-columns: {cols}">
        <div class="text-[10px] text-dim p-1 text-right">all-day</div>
        {#each days as d}
          {@const list = (eventsByDay.get(fmtDateISO(d)) ?? []).filter(isAllDay)}
          <div class="border-l border-surface1 p-1 space-y-0.5 min-w-0">
            {#each list as ev}
              {@const c = eventTypeColor(ev)}
              <button
                onclick={() => onClickEvent(ev)}
                class="block w-full text-left text-[11px] px-1.5 py-0.5 rounded truncate"
                style="background: {c.bg}; color: {c.fg}; border-left: 2px solid {c.border}; {ev.done ? 'text-decoration: line-through; opacity: 0.7;' : ''}"
              >
                {ev.title}
              </button>
            {/each}
          </div>
        {/each}
      </div>
    </div>

    <div bind:this={scrollEl} class="flex-1 overflow-y-auto relative" style="min-width: {minWidth}">
      <div
        bind:this={innerGridEl}
        class="grid relative"
        style="grid-template-columns: {cols}; height: {HOURS.length * HOUR_PX}px"
      >
        <!-- Hours rail -->
        <div class="relative">
          {#each HOURS as h}
            <div class="text-[10px] text-dim text-right pr-1 border-b border-surface1/50" style="height: {HOUR_PX}px">
              {#if h > 0}
                <span class="-translate-y-2 inline-block">{String(h).padStart(2, '0')}:00</span>
              {/if}
            </div>
          {/each}
        </div>

        <!-- Day columns -->
        {#each days as d, dayIdx}
          {@const isToday = isSameDay(d, new Date())}
          {@const dayEvents = (eventsByDay.get(fmtDateISO(d)) ?? []).filter((e) => !isAllDay(e))}
          {@const layout = layoutDay(dayEvents)}
          <div
            class="relative border-l border-surface1 cursor-cell {isToday ? 'bg-primary/5' : ''}"
            role="button"
            tabindex="0"
            data-day-idx={dayIdx}
            onpointerdown={(e) => onSlotPointerDown(e, dayIdx)}
            onpointermove={(e) => onSlotPointerMove(e, dayIdx)}
            onpointerup={(e) => onSlotPointerUp(e, dayIdx)}
            onpointercancel={onSlotPointerCancel}
            onclick={(e) => { if (!onSlotRange) slotClick(d, e); }}
            onkeydown={(e) => { if (e.key === 'Enter') slotClick(d, e as unknown as MouseEvent); }}
            style="touch-action: none;"
          >
            {#each HOURS as h}
              <div class="border-b border-surface1/40 absolute left-0 right-0 pointer-events-none" style="top: {h * HOUR_PX}px; height: {HOUR_PX}px"></div>
              <div class="border-b border-dashed border-surface1/20 absolute left-0 right-0 pointer-events-none" style="top: {h * HOUR_PX + HOUR_PX / 2}px"></div>
            {/each}

            {#if isToday}
              {@const top = nowMinutes() * (HOUR_PX / 60)}
              <div class="absolute left-0 right-0 z-20 pointer-events-none" style="top: {top}px">
                <div class="h-px bg-error"></div>
                <div class="absolute -left-1 -top-1 w-2 h-2 rounded-full bg-error"></div>
              </div>
            {/if}

            {#if slotGhost && slotGhost.dayIdx === dayIdx}
              {@const top = slotGhost.startMin * (HOUR_PX / 60)}
              {@const height = Math.max((slotGhost.endMin - slotGhost.startMin) * (HOUR_PX / 60), 18)}
              <div
                class="absolute left-0.5 right-0.5 z-20 pointer-events-none rounded border border-primary"
                style="top: {top}px; height: {height}px; background: color-mix(in srgb, var(--color-primary) 24%, transparent);"
              >
                <div class="text-[10px] text-primary font-medium px-1.5 pt-1">
                  {fmtMin(slotGhost.startMin)} – {fmtMin(slotGhost.endMin)}
                  <span class="opacity-70">· {slotGhost.endMin - slotGhost.startMin}m</span>
                </div>
              </div>
            {/if}

            <!-- Task-drop ghost: rendered when a backlog task is being
                 dragged over this column. Distinct from slotGhost (which
                 is for click-and-drag-to-create empty time blocks). -->
            {#if pendingTask && taskDragHover && taskDragHover.dayIdx === dayIdx}
              {@const top = taskDragHover.startMin * (HOUR_PX / 60)}
              {@const dur = pendingTask.durationMinutes}
              {@const height = Math.max(dur * (HOUR_PX / 60), 22)}
              <div
                class="absolute left-0.5 right-0.5 z-20 pointer-events-none rounded border border-secondary ring-1 ring-secondary/40"
                style="top: {top}px; height: {height}px; background: color-mix(in srgb, var(--color-secondary) 22%, transparent);"
              >
                <div class="text-[10px] text-secondary font-medium px-1.5 pt-1 truncate">
                  {fmtMin(taskDragHover.startMin)} · {pendingTask.title}
                </div>
                <div class="text-[10px] opacity-70 px-1.5">drop to schedule · {dur}m</div>
              </div>
            {/if}

            {#each layout as item, layoutIdx (`${item.ev.taskId ?? item.ev.eventId ?? item.ev.title}@${item.ev.start ?? item.startMin}#${layoutIdx}`)}
              {@const c = eventTypeColor(item.ev)}
              {@const isDragging = drag?.ev === item.ev}
              {@const isResizing = resize?.ev === item.ev}
              {@const ghostDur = isResizing ? resize!.ghostDuration : (item.endMin - item.startMin)}
              {@const top = item.startMin * (HOUR_PX / 60)}
              {@const height = Math.max(ghostDur * (HOUR_PX / 60), 18)}
              {@const widthPct = 100 / item.groupSize}
              {@const draggable = !!item.ev.taskId && !!item.ev.start}
              <div
                class="absolute z-10"
                style="top: {top}px; height: {height}px; left: {item.col * widthPct}%; width: calc({widthPct}% - 2px);"
              >
                <button
                  onpointerdown={(e) => draggable && onEventPointerDown(e, item.ev, dayIdx)}
                  onpointermove={onEventPointerMove}
                  onpointerup={onEventPointerUp}
                  onpointercancel={onEventPointerCancel}
                  onclick={(e) => { e.stopPropagation(); if (!drag && !draggable) onClickEvent(item.ev); }}
                  class="absolute inset-0 rounded text-left text-[11px] overflow-hidden hover:brightness-125 transition {draggable ? 'cursor-grab active:cursor-grabbing' : ''} {isDragging ? 'opacity-30' : ''} {isResizing ? 'ring-1 ring-primary' : ''}"
                  style="background: {c.bg}; color: {c.fg}; border-left: 3px solid {c.border}; padding: 2px 4px; touch-action: none;"
                >
                  <div class="font-medium truncate {item.ev.done ? 'line-through opacity-70' : ''}">{item.ev.title}</div>
                  {#if height > 30}
                    <div class="text-[10px] opacity-80 truncate">
                      {fmtTime(eventStartDate(item.ev)!)}
                      {#if isResizing}<span class="font-mono"> · {ghostDur}m</span>{:else if item.ev.location} · {item.ev.location}{/if}
                    </div>
                  {/if}
                </button>
                <!-- Resize grip — bottom 6px, only visible/usable on
                     scheduled tasks. Sits ABOVE the event button so its
                     pointerdown wins. -->
                {#if draggable && height > 22}
                  <div
                    role="separator"
                    aria-label="resize event"
                    class="absolute left-0 right-0 bottom-0 h-1.5 cursor-ns-resize hover:bg-primary/40"
                    onpointerdown={(e) => onResizePointerDown(e, item.ev, item.startMin, item.endMin - item.startMin)}
                    onpointermove={onResizePointerMove}
                    onpointerup={onResizePointerUp}
                    onpointercancel={onResizePointerCancel}
                    style="touch-action: none;"
                  ></div>
                {/if}
              </div>
            {/each}
          </div>
        {/each}

        <!-- Drag ghost — follows cursor across columns -->
        {#if drag}
          {@const c = eventTypeColor(drag.ev)}
          {@const top = drag.ghostMinutes * (HOUR_PX / 60)}
          {@const height = Math.max(drag.durationMinutes * (HOUR_PX / 60), 18)}
          <div
            class="absolute rounded text-left text-[11px] overflow-hidden z-30 pointer-events-none ring-2 ring-primary"
            style="top: {top}px; height: {height}px; left: calc({railPx}px + {drag.ghostDayIdx} * (100% - {railPx}px) / {days.length}); width: calc((100% - {railPx}px) / {days.length} - 4px); background: {c.bg}; color: {c.fg}; border-left: 3px solid {c.border}; padding: 2px 4px; margin-left: 1px;"
          >
            <div class="font-medium truncate">{drag.ev.title}</div>
            <div class="text-[10px] opacity-80">
              {String(Math.floor(drag.ghostMinutes / 60)).padStart(2, '0')}:{String(drag.ghostMinutes % 60).padStart(2, '0')}
              {days[drag.ghostDayIdx].toLocaleDateString(undefined, { weekday: 'short' })}
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>
