<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Virtue, type VirtueCheck } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';

  // VirtuesWeeklyCheck — drop-in component for the Sunday review
  // rhythm. Surfaces only ACTIVE virtues with their latest check
  // and a 1–5 button row to log this week's score in one tap. The
  // /virtues page is the long-form home for editing virtue records;
  // this is the "do the weekly thing" surface, designed to take
  // <30 seconds on a Sunday.
  //
  // Designed to be embedded on /review (Sunday rhythm) but also
  // reusable on /examen or any other reflection surface in future.

  let virtues = $state<Virtue[]>([]);
  let loaded = $state(false);

  // Per-virtue draft note (the score lives on the button click,
  // the note is keyed by virtue id and submitted on a "+ note"
  // expand).
  let openNote = $state<string | null>(null);
  let noteBuf = $state<Record<string, string>>({});
  let busy = $state<Record<string, boolean>>({});

  async function load() {
    try {
      const r = await api.listVirtues();
      virtues = r.virtues.filter((v) => (v.status ?? 'active') === 'active');
    } catch {
      virtues = [];
    } finally {
      loaded = true;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/virtues.json') load();
    });
  });

  // The week label we render — Monday-of-current-week using local
  // time. The server canonicalises any submitted check to its
  // own MondayOf so the label is purely informational; the
  // backend won't be confused if a user crossing midnight UTC sees
  // a "stale" Monday for a few minutes.
  function thisWeekMonday(): string {
    const d = new Date();
    const offset = (d.getDay() + 6) % 7; // sunday=0, monday=1 → 0
    d.setDate(d.getDate() - offset);
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${y}-${m}-${day}`;
  }
  const week = thisWeekMonday();

  // Latest check for a virtue (newest week-start wins). Used to
  // render the "you scored X last time" pill so the user sees
  // continuity rather than starting from blank each Sunday.
  function latestCheck(v: Virtue): VirtueCheck | null {
    const cs = v.checks ?? [];
    if (cs.length === 0) return null;
    return cs.reduce((acc, c) => (c.week_start > acc.week_start ? c : acc), cs[0]);
  }

  // Check this week is "already done" when a check matching the
  // current Monday exists. Lets us show a "✓ checked" state on the
  // already-done row instead of inviting a re-rate.
  function checkedThisWeek(v: Virtue): VirtueCheck | null {
    const cs = v.checks ?? [];
    return cs.find((c) => c.week_start === week) ?? null;
  }

  function scoreTone(score: number): string {
    if (score >= 5) return 'success';
    if (score === 4) return 'info';
    if (score === 3) return 'warning';
    if (score >= 1) return 'error';
    return 'dim';
  }

  async function submitScore(v: Virtue, score: number) {
    if (busy[v.id]) return;
    busy = { ...busy, [v.id]: true };
    try {
      await api.logVirtueCheck(v.id, {
        score,
        note: (noteBuf[v.id] ?? '').trim() || undefined
      });
      noteBuf = { ...noteBuf, [v.id]: '' };
      openNote = null;
      toast.success(`${v.name}: ${score}/5`);
      await load();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = { ...busy, [v.id]: false };
    }
  }

  function colorVar(c?: string): string {
    const map: Record<string, string> = {
      red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
      blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
      peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
      lavender: 'primary', flamingo: 'error'
    };
    return `var(--color-${map[c ?? ''] ?? 'secondary'})`;
  }
</script>

{#if loaded && virtues.length > 0}
  <section class="bg-surface0 border border-surface1 rounded-lg p-4 space-y-3">
    <div class="flex items-baseline justify-between">
      <h3 class="text-xs uppercase tracking-wider text-dim font-medium">
        Virtues this week
      </h3>
      <a href="/virtues" class="text-xs text-secondary hover:underline">manage →</a>
    </div>
    <p class="text-[11px] text-subtext italic">
      Honest noticing — what God is forming. One tap to score, optional note.
    </p>
    <ul class="space-y-2">
      {#each virtues as v (v.id)}
        {@const last = latestCheck(v)}
        {@const thisWeek = checkedThisWeek(v)}
        {@const isOpen = openNote === v.id}
        <li
          class="bg-mantle/40 rounded p-3"
          style="border-left: 2px solid {colorVar(v.color)};"
        >
          <div class="flex items-start gap-3 flex-wrap">
            <div class="flex-1 min-w-0">
              <div class="text-sm text-text font-medium">{v.name}</div>
              {#if v.anchor}
                <div class="text-[11px] text-secondary italic mt-0.5 break-words">{v.anchor}</div>
              {/if}
              {#if last && last.week_start !== week}
                <div class="text-[11px] text-dim mt-1">
                  last week: <span style="color: var(--color-{scoreTone(last.score)});">{last.score}/5</span>
                  {#if last.note}<span class="ml-1">· {last.note}</span>{/if}
                </div>
              {/if}
            </div>
            <!-- Score buttons. If this week's check exists, the
                 already-picked score is highlighted; clicking a
                 different one updates (the server upserts in place). -->
            <div class="flex flex-wrap items-center gap-1 flex-shrink-0">
              {#each [1, 2, 3, 4, 5] as n}
                {@const picked = thisWeek?.score === n}
                <button
                  type="button"
                  onclick={() => submitScore(v, n)}
                  disabled={busy[v.id]}
                  class="w-8 h-8 rounded text-sm font-semibold tabular-nums border transition-colors
                    {picked
                      ? 'bg-primary text-on-primary border-primary'
                      : 'bg-surface0 text-subtext border-surface1 hover:border-primary/40'}"
                  title="{n}/5"
                >{n}</button>
              {/each}
              <button
                type="button"
                onclick={() => (openNote = isOpen ? null : v.id)}
                class="text-[11px] text-dim hover:text-text px-1.5"
                title="add reflection note"
              >{isOpen ? '×' : '+ note'}</button>
            </div>
          </div>
          {#if isOpen}
            <textarea
              value={noteBuf[v.id] ?? ''}
              oninput={(e) => (noteBuf = { ...noteBuf, [v.id]: (e.target as HTMLTextAreaElement).value })}
              rows="2"
              placeholder="What did you notice this week? Where the grace, where the friction?"
              class="mt-2 w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
            ></textarea>
            <p class="text-[11px] text-dim mt-1">Tap a number above to save with this note.</p>
          {:else if thisWeek}
            <p class="text-[11px] text-success mt-1">
              ✓ checked this week: {thisWeek.score}/5
              {#if thisWeek.note}<span class="text-subtext ml-1">— {thisWeek.note}</span>{/if}
            </p>
          {/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}
