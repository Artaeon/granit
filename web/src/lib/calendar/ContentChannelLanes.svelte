<script lang="ts">
  import type { CalendarEvent } from '$lib/api';
  import { eventStartDate, fmtDateISO, fmtTime } from './utils';

  // Channel lanes — horizontal swim-lane grouping for content events
  // in week / workweek view. Each lane represents one channel
  // (channels[0] of each content event); 'Unset' catches content
  // events with no channel filled in yet so a half-scoped event
  // doesn't fall out of view. Days run left-to-right across the
  // week, lanes stack top-to-bottom.
  //
  // Reads like a Trello board rotated to the day-grid axis: a single
  // glance shows "Twitter has 3 posts mid-week, LinkedIn has one
  // Friday, the blog draft is unscheduled".
  //
  // The overlay sits absolute over the HourGrid so the underlying
  // hour columns stay visually present; closing returns to the same
  // scroll position with no reflow cost. Clicking a card opens the
  // same EventDetail the grid uses — pipeline click and grid click
  // are interchangeable surfaces.

  interface Props {
    /** The current week's days (Date[] of length 5 or 7). */
    days: Date[];
    /** Already-filtered event list from the calendar page. The
     *  overlay picks the kind==='content' subset itself rather than
     *  asking the parent to pre-filter. */
    events: CalendarEvent[];
    onClickEvent: (ev: CalendarEvent) => void;
    onClose: () => void;
  }

  let { days, events, onClickEvent, onClose }: Props = $props();

  interface LaneCard {
    ev: CalendarEvent;
    dayIdx: number;
  }

  interface Lane {
    channel: string;
    label: string;
    cards: LaneCard[];
  }

  let lanes = $derived.by<Lane[]>(() => {
    const dayKeys = days.map(fmtDateISO);
    const dayIndexByKey = new Map(dayKeys.map((k, i) => [k, i]));
    // Channel ordering: stable by first-seen so a freshly-added
    // 'Podcast' lane lands at the bottom instead of shuffling the
    // existing layout each render.
    const order: string[] = [];
    const byChannel = new Map<string, LaneCard[]>();
    for (const ev of events) {
      if (ev.kind !== 'content') continue;
      const start = eventStartDate(ev);
      const k = start ? fmtDateISO(start) : (ev.date ?? '');
      const idx = dayIndexByKey.get(k);
      if (idx === undefined) continue;
      const channel = ev.channels?.[0]?.trim() ?? '';
      const key = channel || '';
      if (!byChannel.has(key)) {
        byChannel.set(key, []);
        order.push(key);
      }
      byChannel.get(key)!.push({ ev, dayIdx: idx });
    }
    // Sort cards within a lane by day index, then by start time, so
    // the lane reads left-to-right in chronological order matching
    // the day-grid axis.
    for (const cards of byChannel.values()) {
      cards.sort((a, b) => {
        if (a.dayIdx !== b.dayIdx) return a.dayIdx - b.dayIdx;
        const ta = eventStartDate(a.ev)?.getTime() ?? 0;
        const tb = eventStartDate(b.ev)?.getTime() ?? 0;
        return ta - tb;
      });
    }
    // 'Unset' lane sinks to the bottom so the channels-the-user-knows
    // sit at the top of their attention.
    const sortedKeys = [...order].sort((a, b) => {
      if (a === '' && b !== '') return 1;
      if (b === '' && a !== '') return -1;
      return 0;
    });
    return sortedKeys.map((k) => ({
      channel: k,
      label: k || 'Unset',
      cards: byChannel.get(k) ?? []
    }));
  });

  let totalContent = $derived(events.filter((e) => e.kind === 'content').length);

  function shortTime(ev: CalendarEvent): string {
    const d = eventStartDate(ev);
    return d ? fmtTime(d) : '';
  }
</script>

<!-- Floating overlay — same chrome as ContentPipelineOverlay so the
     two views read as the same feature in different shapes. -->
<div
  class="absolute inset-0 z-30 bg-black/40 flex items-stretch p-3 sm:p-4"
  role="dialog"
  aria-label="Content channel lanes"
  tabindex="-1"
  onclick={onClose}
  onkeydown={(e) => { if (e.key === 'Escape') onClose(); }}
>
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    onclick={(e) => e.stopPropagation()}
    onkeydown={(e) => e.stopPropagation()}
    class="flex-1 bg-mantle border border-surface1 rounded-lg shadow-xl flex flex-col overflow-hidden"
    role="document"
  >
    <header class="flex items-center gap-2 px-4 py-2 border-b border-surface1 flex-shrink-0">
      <div class="flex items-center gap-1.5 text-text">
        <span
          class="inline-flex items-center justify-center w-5 h-5 rounded text-[10px] font-mono font-semibold"
          style="background: color-mix(in srgb, var(--color-lavender) 18%, transparent); color: var(--color-lavender);"
          aria-hidden="true"
        >C</span>
        <span class="text-sm font-semibold">Channels</span>
        <span class="text-[11px] text-dim">{totalContent} {totalContent === 1 ? 'item' : 'items'} · {lanes.length} {lanes.length === 1 ? 'channel' : 'channels'}</span>
      </div>
      <span class="flex-1"></span>
      <button
        type="button"
        onclick={onClose}
        class="text-xs text-subtext hover:text-text px-2 py-1 rounded hover:bg-surface0"
        title="Close channel lanes"
      >Close · Esc</button>
    </header>

    <div class="flex-1 overflow-auto">
      <!-- Day header row — sticks to the top so a long lane list
           keeps the day labels in view while scrolling. Mirrors the
           weekday header shape used by HourGrid for visual continuity. -->
      <div
        class="grid sticky top-0 z-10 bg-mantle border-b border-surface1"
        style="grid-template-columns: 7rem repeat({days.length}, minmax(0, 1fr));"
      >
        <div class="px-2 py-1.5 text-[10px] uppercase tracking-wider text-dim">Channel</div>
        {#each days as d (d.toISOString())}
          <div class="px-2 py-1.5 text-[11px] text-subtext border-l border-surface1">
            <div class="font-semibold">{d.toLocaleDateString(undefined, { weekday: 'short' })}</div>
            <div class="text-dim text-[10px]">{d.getDate()}</div>
          </div>
        {/each}
      </div>

      {#if lanes.length === 0}
        <div class="p-6 text-sm text-dim text-center italic">
          No content events scheduled in this window. Create one via the + button.
        </div>
      {:else}
        {#each lanes as lane (lane.channel)}
          <div
            class="grid border-b border-surface1 hover:bg-surface0/40"
            style="grid-template-columns: 7rem repeat({days.length}, minmax(0, 1fr));"
          >
            <div class="px-2 py-1.5 flex items-center gap-1.5 text-[12px] {lane.channel ? 'text-lavender' : 'text-dim italic'} border-r border-surface1 truncate">
              {#if lane.channel}
                <span class="w-1.5 h-1.5 rounded-full flex-shrink-0" style="background: var(--color-lavender)"></span>
              {/if}
              <span class="truncate">{lane.label}</span>
              <span class="ml-auto text-[10px] font-mono tabular-nums text-dim">{lane.cards.length}</span>
            </div>
            {#each days as _d, dayIdx (dayIdx)}
              {@const dayCards = lane.cards.filter((c) => c.dayIdx === dayIdx)}
              <div class="px-1 py-1 border-l border-surface1 min-h-[2.25rem] space-y-1">
                {#each dayCards as card (card.ev.eventId ?? card.ev.title + ':' + (card.ev.start ?? ''))}
                  <button
                    type="button"
                    onclick={() => onClickEvent(card.ev)}
                    class="block w-full text-left bg-mantle hover:bg-surface1 border border-surface1 hover:border-lavender/60 rounded px-1.5 py-1 transition-colors"
                    title={card.ev.title}
                  >
                    <div class="text-[11px] font-medium text-text leading-tight truncate">{card.ev.title}</div>
                    <div class="flex items-center gap-1 text-[10px] text-dim mt-px">
                      {#if card.ev.start}<span class="font-mono">{shortTime(card.ev)}</span>{:else}<span class="italic">all-day</span>{/if}
                      {#if card.ev.status}
                        <span class="flex-shrink-0">·</span>
                        <span class="uppercase tracking-wider text-[9px] font-semibold text-lavender">{card.ev.status}</span>
                      {/if}
                    </div>
                  </button>
                {/each}
              </div>
            {/each}
          </div>
        {/each}
      {/if}
    </div>
  </div>
</div>
