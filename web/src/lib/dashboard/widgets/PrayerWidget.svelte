<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type PrayerIntention } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // PrayerWidget — surfaces the user's active prayer intentions on the
  // dashboard, prioritising the ones tied to ventures / projects /
  // goals (the "align life and business to God" use case the dedicated
  // /prayer page is built around). Plain "general" intentions show up
  // too, but underneath the work-tied ones so the front of the card
  // reads as "what are you bringing before God for the work right now".
  //
  // Hidden when there's nothing active, the same as the ventures
  // widget — we don't want to nag the user with an empty prayer card.

  let intentions = $state<PrayerIntention[]>([]);
  let loaded = $state(false);

  async function load() {
    try {
      const r = await api.listPrayer();
      intentions = r.intentions;
    } catch {
      intentions = [];
    } finally {
      loaded = true;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/prayer/intentions.json') load();
    });
  });

  // Sort: work-tied (venture/project/goal) first, then general.
  // Within each bucket, newest by started_at — fresh prayer surfaces.
  let prioritised = $derived.by(() => {
    const active = intentions.filter((p) => p.status === 'praying');
    const tied = active.filter((p) => p.venture || p.project || p.goal);
    const general = active.filter((p) => !(p.venture || p.project || p.goal) && !p.person);
    const persons = active.filter((p) => p.person && !(p.venture || p.project || p.goal));
    const sortByStart = (a: PrayerIntention, b: PrayerIntention) => {
      const ax = a.started_at ?? '';
      const bx = b.started_at ?? '';
      return bx.localeCompare(ax);
    };
    tied.sort(sortByStart);
    persons.sort(sortByStart);
    general.sort(sortByStart);
    return [...tied, ...persons, ...general];
  });

  // Show at most 4 lines on the dashboard so the widget stays tight.
  // The "all →" link goes to the dedicated /prayer page for the rest.
  let visible = $derived(prioritised.slice(0, 4));
  let extra = $derived(Math.max(0, prioritised.length - 4));

  // Compact label for the linkage chip on each row. Picks the most
  // specific tie ("for [venture]") over a less specific one (project
  // / goal) when multiple are set — same priority order /prayer uses.
  function tieLabel(p: PrayerIntention): { icon: string; text: string } | null {
    if (p.venture) return { icon: '🏢', text: p.venture };
    if (p.project) return { icon: '📁', text: p.project };
    if (p.goal) return { icon: '🎯', text: p.goal };
    if (p.person) return { icon: '👤', text: p.person };
    return null;
  }
</script>

{#if loaded && prioritised.length > 0}
  <section class="bg-surface0 border border-surface1 rounded-lg p-4">
    <div class="flex items-baseline justify-between mb-3">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Prayer</h2>
      <a href="/prayer" class="text-xs text-secondary hover:underline">all →</a>
    </div>
    <ul class="space-y-2">
      {#each visible as p (p.id)}
        {@const tie = tieLabel(p)}
        <li class="px-2.5 py-1.5 bg-mantle/30 rounded">
          <div class="text-sm text-text break-words">{p.text}</div>
          {#if tie || p.passage_ref}
            <div class="flex flex-wrap items-center gap-x-2.5 gap-y-0.5 mt-1 text-[11px] text-dim">
              {#if tie}
                <span class="text-secondary">{tie.icon} {tie.text}</span>
              {/if}
              {#if p.passage_ref}
                <span>📖 {p.passage_ref}</span>
              {/if}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
    {#if extra > 0}
      <a href="/prayer" class="block text-xs text-secondary hover:underline mt-2.5 text-center">
        + {extra} more →
      </a>
    {/if}
  </section>
{/if}
