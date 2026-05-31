<script lang="ts">
  import { EVENT_STATUSES, type CalendarEvent } from '$lib/api';
  import { eventStartDate, fmtTime } from './utils';

  // Pipeline overlay — kanban-style status grouping rendered on top of
  // the month grid when the user wants the "what's in the funnel"
  // view. Toggled by a header button; the underlying grid stays
  // visible underneath the panel so dates remain reachable with one
  // click on the backdrop.
  //
  // Six canonical columns from EVENT_STATUSES plus a leading 'Unset'
  // column for events that haven't been promoted from the create
  // form yet. Columns scale to equal widths on desktop; on mobile
  // they stack vertically since a horizontal scrolling kanban on a
  // touch screen tends to hide the rightmost columns from skim
  // readers. Each card shows title + date + the first channel chip
  // (additional channels are visible in EventDetail).
  //
  // The overlay reads events from props rather than fetching its
  // own — keeps the calendar page as the single source of truth
  // for filter / project / date-range state. Closing fires onClose
  // back to the parent; clicking a card fires onClickEvent which
  // the parent wires to the existing EventDetail modal so a
  // pipeline click and a grid click open the same surface.

  interface Props {
    /** Already filtered events from the calendar page. The overlay
     *  picks the kind==='content' subset itself rather than asking
     *  the parent to pre-filter — keeps the contract narrow. */
    events: CalendarEvent[];
    onClickEvent: (ev: CalendarEvent) => void;
    onClose: () => void;
  }

  let { events, onClickEvent, onClose }: Props = $props();

  type ColumnKey = '' | (typeof EVENT_STATUSES)[number];

  interface Column {
    key: ColumnKey;
    label: string;
    events: CalendarEvent[];
  }

  let columns = $derived.by<Column[]>(() => {
    const groups = new Map<ColumnKey, CalendarEvent[]>();
    // Seed every canonical column so empty stages still render —
    // a zero-count Drafting column is meaningful "nothing in flight"
    // signal that an absent column would hide.
    groups.set('', []);
    for (const s of EVENT_STATUSES) groups.set(s, []);
    for (const ev of events) {
      if (ev.kind !== 'content') continue;
      const key = (ev.status ?? '') as ColumnKey;
      // Normalise unknown values into 'Unset' so a hand-edited
      // status doesn't create a stray column the user can't action.
      const bucket = groups.has(key) ? key : '';
      groups.get(bucket)!.push(ev);
    }
    // Sort each column by date so the soonest item floats to the top
    // — the user reads down the column the same way they'd scan a
    // backlog.
    const result: Column[] = [];
    for (const [key, evs] of groups) {
      evs.sort((a, b) => {
        const da = eventStartDate(a)?.getTime() ?? Number.MAX_SAFE_INTEGER;
        const db = eventStartDate(b)?.getTime() ?? Number.MAX_SAFE_INTEGER;
        return da - db;
      });
      result.push({
        key,
        label: key === '' ? 'Unset' : key[0].toUpperCase() + key.slice(1),
        events: evs
      });
    }
    return result;
  });

  let totalContent = $derived(events.filter((e) => e.kind === 'content').length);

  // Tints — match the ContentPanel status palette so the picker
  // and the kanban read as one feature.
  function colHeaderClass(key: ColumnKey): string {
    switch (key) {
      case 'idea':
        return 'text-text border-overlay0 bg-surface1/60';
      case 'drafting':
        return 'text-blue border-blue/40 bg-blue/10';
      case 'review':
        return 'text-yellow border-yellow/40 bg-yellow/10';
      case 'scheduled':
        return 'text-lavender border-lavender/40 bg-lavender/10';
      case 'published':
        return 'text-green border-green/40 bg-green/10';
      case 'archived':
        return 'text-dim border-overlay0 bg-surface0/60';
      default:
        return 'text-dim border-overlay0 bg-surface0/60';
    }
  }

  function shortDate(ev: CalendarEvent): string {
    const d = eventStartDate(ev);
    if (!d) return ev.date ?? '';
    // Month short + day — keeps cards readable without crowding;
    // user clicks through for the full datetime.
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }
</script>

<!-- Floating panel — sits above the month grid but the backdrop
     stays semi-transparent so the grid is still glanceable
     underneath. Click outside the panel to close. -->
<div
  class="absolute inset-0 z-30 bg-black/40 flex items-stretch p-3 sm:p-4"
  role="dialog"
  aria-label="Content pipeline"
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
        <span class="text-sm font-semibold">Pipeline</span>
        <span class="text-[11px] text-dim">{totalContent} {totalContent === 1 ? 'item' : 'items'}</span>
      </div>
      <span class="flex-1"></span>
      <button
        type="button"
        onclick={onClose}
        class="text-xs text-subtext hover:text-text px-2 py-1 rounded hover:bg-surface0"
        title="Close pipeline overlay"
      >Close · Esc</button>
    </header>

    <div class="flex-1 overflow-auto p-3">
      <div class="grid gap-2 grid-cols-1 sm:grid-cols-2 lg:grid-cols-7">
        {#each columns as col (col.key)}
          <section
            class="flex flex-col min-h-0 rounded-md border {colHeaderClass(col.key)}"
            aria-label="{col.label} ({col.events.length})"
          >
            <header class="flex items-center justify-between px-2 py-1.5 border-b border-current/30 flex-shrink-0">
              <span class="text-[11px] uppercase tracking-wider font-semibold">{col.label}</span>
              <span class="text-[10px] tabular-nums font-mono opacity-80">{col.events.length}</span>
            </header>
            <ul class="flex-1 min-h-0 overflow-y-auto p-1.5 space-y-1.5">
              {#each col.events as ev (ev.eventId ?? ev.title + ':' + ev.date)}
                <li>
                  <button
                    type="button"
                    onclick={() => onClickEvent(ev)}
                    class="block w-full text-left bg-mantle hover:bg-surface0 border border-surface1 hover:border-lavender/60 rounded-md px-2 py-1.5 transition-colors"
                    title={ev.title}
                  >
                    <div class="text-[12px] font-medium text-text leading-tight truncate">{ev.title}</div>
                    <div class="mt-0.5 flex items-center gap-1.5 text-[10px] text-dim">
                      <span class="font-mono">{shortDate(ev)}</span>
                      {#if ev.start}
                        {@const d = eventStartDate(ev)}
                        {#if d}<span>· {fmtTime(d)}</span>{/if}
                      {/if}
                      {#if ev.channels && ev.channels.length > 0}
                        <span class="flex-shrink-0">·</span>
                        <span class="truncate text-lavender">{ev.channels[0]}{ev.channels.length > 1 ? ` +${ev.channels.length - 1}` : ''}</span>
                      {/if}
                    </div>
                  </button>
                </li>
              {/each}
              {#if col.events.length === 0}
                <li class="text-[10px] text-dim italic px-1 py-0.5">empty</li>
              {/if}
            </ul>
          </section>
        {/each}
      </div>
    </div>
  </div>
</div>
