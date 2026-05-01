<script lang="ts">
  import type { CalendarEvent } from '$lib/api';
  import { eventStartDate, eventTypeColor, fmtDateISO, fmtTime } from './utils';

  let { events, onClickEvent }: { events: CalendarEvent[]; onClickEvent: (ev: CalendarEvent) => void } = $props();

  let groups = $derived.by(() => {
    const m = new Map<string, CalendarEvent[]>();
    for (const ev of events) {
      const d = eventStartDate(ev);
      const key = d ? fmtDateISO(d) : '';
      if (!key) continue;
      if (!m.has(key)) m.set(key, []);
      m.get(key)!.push(ev);
    }
    const sorted = Array.from(m.entries()).sort(([a], [b]) => a.localeCompare(b));
    for (const [, evs] of sorted) {
      evs.sort((a, b) => {
        const sa = a.start ? new Date(a.start).getTime() : 0;
        const sb = b.start ? new Date(b.start).getTime() : 0;
        return sa - sb;
      });
    }
    return sorted;
  });

  function dateLabel(iso: string): string {
    const [y, m, d] = iso.split('-').map(Number);
    const date = new Date(y, m - 1, d);
    return date.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' });
  }
</script>

<div class="space-y-6 max-w-3xl">
  {#if groups.length === 0}
    <div class="text-sm text-dim italic">no events in range</div>
  {/if}
  {#each groups as [iso, evs]}
    <section>
      <h3 class="text-xs uppercase tracking-wider text-dim mb-2 font-medium border-b border-surface1 pb-1">
        {dateLabel(iso)}
      </h3>
      <ul class="space-y-1">
        {#each evs as ev}
          {@const c = eventTypeColor(ev)}
          <li>
            <button
              onclick={() => onClickEvent(ev)}
              class="w-full text-left flex items-baseline gap-3 py-1 px-2 rounded hover:bg-surface0 group"
            >
              <span class="text-xs font-mono w-14 text-dim flex-shrink-0">
                {ev.start ? fmtTime(new Date(ev.start)) : 'all-day'}
              </span>
              <span class="w-1 h-3 rounded-full flex-shrink-0" style="background: {c.border}"></span>
              <span class="flex-1 text-sm {ev.done ? 'line-through text-dim' : 'text-text'}">{ev.title}</span>
              {#if ev.location}
                <span class="text-xs text-dim">@ {ev.location}</span>
              {/if}
              <span class="text-[10px] uppercase text-dim opacity-0 group-hover:opacity-100">{ev.type.replace('_', ' ')}</span>
            </button>
          </li>
        {/each}
      </ul>
    </section>
  {/each}
</div>
