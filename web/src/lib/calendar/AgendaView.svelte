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

  // Relative-aware label so the agenda reads naturally — "Today" /
  // "Tomorrow" / "Yesterday" / weekday for the next 6 days, full date
  // otherwise. Matches what the daily-note header shows and what users
  // expect from Google / iOS Calendar.
  function dateLabel(iso: string): string {
    const [y, m, d] = iso.split('-').map(Number);
    const date = new Date(y, m - 1, d);
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const days = Math.round((date.getTime() - today.getTime()) / 86_400_000);
    const dow = date.toLocaleDateString(undefined, { weekday: 'long' });
    if (days === 0) return `Today · ${dow}`;
    if (days === 1) return `Tomorrow · ${dow}`;
    if (days === -1) return `Yesterday · ${dow}`;
    if (days > 1 && days < 7) return `${dow} · ${date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`;
    return date.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' });
  }

  function isTodayIso(iso: string): boolean {
    const [y, m, d] = iso.split('-').map(Number);
    const a = new Date(y, m - 1, d);
    const t = new Date();
    return a.getFullYear() === t.getFullYear() && a.getMonth() === t.getMonth() && a.getDate() === t.getDate();
  }
</script>

<div class="space-y-6 max-w-3xl">
  {#if groups.length === 0}
    <div class="text-center py-16 px-4">
      <svg viewBox="0 0 64 64" class="w-12 h-12 mx-auto text-dim opacity-40 mb-3" fill="none" stroke="currentColor" stroke-width="2">
        <rect x="10" y="14" width="44" height="42" rx="3"/>
        <path d="M10 24h44M22 8v10M42 8v10" stroke-linecap="round"/>
      </svg>
      <p class="text-sm text-subtext mb-1">No events in this range</p>
      <p class="text-xs text-dim">Click + drag on the day/week grid to create one, or use the <span class="text-text font-medium">+ New</span> button.</p>
    </div>
  {/if}
  {#each groups as [iso, evs]}
    {@const isToday = isTodayIso(iso)}
    <section>
      <h3 class="text-xs uppercase tracking-wider {isToday ? 'text-primary' : 'text-dim'} mb-2 font-medium border-b border-surface1 pb-1 flex items-center gap-2">
        <span>{dateLabel(iso)}</span>
        {#if isToday}<span class="w-1.5 h-1.5 rounded-full bg-primary"></span>{/if}
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
