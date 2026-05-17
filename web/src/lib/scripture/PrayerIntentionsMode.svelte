<!--
  PrayerIntentionsMode — the prayer-list panel that used to live as
  the {:else if mode === 'intentions'} branch inside
  web/src/routes/scripture/+page.svelte. Same extraction shape as
  BibleBookmarksMode: own JSON file (.granit/prayer/intentions.json),
  own lazy load, own WS subscription, own CRUD.

  Lifecycle map: each intention is in one of three statuses —
  praying → answered → archived. The component renders three
  collapsible buckets and a quick-add composer at the top.

  Props
    active        true when the parent's mode === 'intentions'. The
                  component lazy-loads on first true; subsequent
                  reloads come through the WS listener.
    prayingCount  $bindable — surfaces the count of currently-
                  praying items so the parent's tab badge stays
                  live without a duplicate fetch.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type PrayerIntention } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { onWsEvent } from '$lib/ws';

  let {
    active,
    prayingCount = $bindable(0)
  }: {
    active: boolean;
    prayingCount?: number;
  } = $props();

  let intentions = $state<PrayerIntention[]>([]);
  let loaded = $state(false);
  let newText = $state('');
  let newCategory = $state('');

  async function load() {
    try {
      const r = await api.listPrayer();
      intentions = r.intentions;
      loaded = true;
    } catch (e) {
      toast.error('failed to load prayers: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Lazy load on first activation; subsequent updates ride the WS.
  $effect(() => {
    if (active && !loaded) {
      void load();
    }
  });

  // WS listener kept alive even when not active so a TUI / second-
  // tab edit refreshes the panel for the next visit. Only fires
  // when we've already loaded once — first paint happens through
  // the $effect above.
  onMount(() =>
    onWsEvent((ev) => {
      if (ev.type !== 'state.changed') return;
      if (ev.path === '.granit/prayer/intentions.json' && loaded) {
        void load();
      }
    })
  );

  async function add() {
    const text = newText.trim();
    if (!text) return;
    try {
      await api.createPrayer({
        text,
        category: newCategory.trim() || undefined
      });
      // Keep the category — most users add several with the same
      // category in a row.
      newText = '';
      await load();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function setStatus(p: PrayerIntention, status: 'praying' | 'answered' | 'archived') {
    try {
      await api.patchPrayer(p.id, { status });
      await load();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Note-edit commits on blur. Suppress the round-trip when nothing
  // changed — same posture as BibleBookmarksMode.
  async function saveAnswer(p: PrayerIntention, answer: string) {
    if (answer === (p.answer ?? '')) return;
    try {
      await api.patchPrayer(p.id, { answer });
      await load();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function remove(p: PrayerIntention) {
    if (!confirm('Remove this intention from history?')) return;
    try {
      await api.deletePrayer(p.id);
      await load();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Pre-grouped derivations so the template stays declarative.
  let prayingNow = $derived(intentions.filter((p) => p.status === 'praying'));
  let answered = $derived(intentions.filter((p) => p.status === 'answered'));
  let archived = $derived(intentions.filter((p) => p.status === 'archived'));

  // Mirror the praying count up to the parent's tab badge. The
  // bindable assignment lives inside an $effect so the writer (this
  // component) and the reader (the parent badge) stay in sync
  // without a notification dance.
  $effect(() => {
    prayingCount = prayingNow.length;
  });
</script>

{#if !loaded}
  <div class="text-sm text-dim">loading prayer list…</div>
{:else}
  <!-- Quick-add composer at top — Enter submits, category persists. -->
  <form onsubmit={(e) => { e.preventDefault(); void add(); }} class="bg-surface0 border border-surface1 rounded-lg p-3 mb-5">
    <input
      bind:value={newText}
      placeholder="What are you praying for?"
      class="w-full bg-transparent text-text placeholder-dim focus:outline-none text-base"
    />
    <div class="flex items-center gap-2 mt-2">
      <input
        bind:value={newCategory}
        placeholder="category (optional, e.g. Family / Self / World)"
        class="flex-1 bg-mantle border border-surface1 rounded px-2 py-1 text-xs text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      <button
        type="submit"
        disabled={!newText.trim()}
        class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90 disabled:opacity-50"
      >Add</button>
    </div>
  </form>

  {#if intentions.length === 0}
    <p class="text-sm text-dim italic">No intentions yet. The list above this line is your prayer list — add what's on your heart.</p>
  {:else}
    {#if prayingNow.length > 0}
      <h3 class="text-xs uppercase tracking-wider text-dim mt-2 mb-2">Praying for</h3>
      <ul class="space-y-2 mb-5">
        {#each prayingNow as p (p.id)}
          <li class="bg-surface0 border border-surface1 rounded-lg p-3">
            <div class="flex items-baseline gap-3 flex-wrap">
              <p class="text-text flex-1 min-w-0 break-words">{p.text}</p>
              {#if p.category}
                <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-subtext">{p.category}</span>
              {/if}
              <button onclick={() => setStatus(p, 'answered')} class="text-xs text-success hover:underline" title="Mark as answered">✓ answered</button>
              <button onclick={() => setStatus(p, 'archived')} class="text-xs text-dim hover:text-text" title="Archive">archive</button>
              <button onclick={() => remove(p)} class="text-xs text-dim hover:text-error" aria-label="delete">×</button>
            </div>
            {#if p.started_at}<p class="text-[11px] text-dim mt-1">since {p.started_at}</p>{/if}
          </li>
        {/each}
      </ul>
    {/if}
    {#if answered.length > 0}
      <h3 class="text-xs uppercase tracking-wider text-dim mt-2 mb-2">Answered ✓</h3>
      <ul class="space-y-2 mb-5">
        {#each answered as p (p.id)}
          <li class="bg-surface0 border border-success rounded-lg p-3">
            <div class="flex items-baseline gap-3 flex-wrap">
              <p class="text-text flex-1 min-w-0 break-words">{p.text}</p>
              {#if p.category}
                <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-subtext">{p.category}</span>
              {/if}
              <button onclick={() => setStatus(p, 'praying')} class="text-xs text-dim hover:text-text" title="Move back to praying">↺</button>
              <button onclick={() => remove(p)} class="text-xs text-dim hover:text-error" aria-label="delete">×</button>
            </div>
            <p class="text-[11px] text-dim mt-1">
              {#if p.started_at}from {p.started_at}{/if}
              {#if p.answered_at}· answered {p.answered_at}{/if}
            </p>
            <textarea
              value={p.answer ?? ''}
              placeholder="How was it answered? (optional)"
              onblur={(e) => saveAnswer(p, (e.currentTarget as HTMLTextAreaElement).value)}
              rows="2"
              class="w-full mt-2 bg-mantle border border-surface1 rounded px-2 py-1.5 text-xs text-text placeholder-dim resize-y focus:outline-none focus:border-primary"
            ></textarea>
          </li>
        {/each}
      </ul>
    {/if}
    {#if archived.length > 0}
      <h3 class="text-xs uppercase tracking-wider text-dim mt-2 mb-2">Archived</h3>
      <ul class="space-y-2 opacity-60">
        {#each archived as p (p.id)}
          <li class="bg-surface0 border border-surface1 rounded-lg p-3 flex items-baseline gap-3 flex-wrap">
            <p class="text-text flex-1 min-w-0 break-words text-sm">{p.text}</p>
            <button onclick={() => setStatus(p, 'praying')} class="text-xs text-dim hover:text-text">↺</button>
            <button onclick={() => remove(p)} class="text-xs text-dim hover:text-error" aria-label="delete">×</button>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
  <p class="text-[11px] text-dim italic mt-4">
    Synced via <code>.granit/prayer/intentions.json</code> — same file the granit TUI reads.
  </p>
{/if}
