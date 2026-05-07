<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Virtue, type VirtueCheck, type HabitInfo } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import Heatmap from '$lib/components/Heatmap.svelte';

  // /virtues — character formation tracker. The "kingdom in me"
  // dimension that complements the project / venture / goal
  // surface. Each virtue is a quality the user is intentionally
  // cultivating, with a dated history of weekly self-checks
  // captured during the Sunday review rhythm.
  //
  // Layout: status tabs + add-form + cards (one per virtue) with
  // an inline weekly-check button. Click into a card to expand
  // the check history. Keeping the rate-it-now action one tap
  // away on the list page is the whole point — virtue tracking
  // dies fast when it requires navigation each Sunday.

  let virtues = $state<Virtue[]>([]);
  let loading = $state(false);
  let q = $state('');
  let statusFilter = $state<'all' | 'active' | 'paused' | 'archived'>('active');
  let createOpen = $state(false);

  // Create form state.
  let nName = $state('');
  let nDescription = $state('');
  let nAnchor = $state('');
  let nSeason = $state('');
  let nColor = $state('blue');
  let saving = $state(false);

  // Per-virtue check form state — keyed by virtue id. We store
  // score + note buffers so the user can compose a check inline
  // without losing it on a re-render.
  let openCheck = $state<string | null>(null);
  let checkScore = $state<Record<string, number>>({});
  let checkNote = $state<Record<string, string>>({});
  let savingCheck = $state(false);

  // Per-virtue history-expanded toggle.
  let expanded = $state<Set<string>>(new Set());

  const colorOptions = ['blue', 'green', 'mauve', 'peach', 'red', 'yellow', 'pink', 'lavender', 'teal', 'sapphire'];

  function colorVar(c?: string): string {
    const map: Record<string, string> = {
      red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
      blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
      peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
      lavender: 'primary', flamingo: 'error'
    };
    return `var(--color-${map[c ?? ''] ?? 'secondary'})`;
  }

  // Habit catalogue — drives the "linked habits" multi-select on
  // the create/edit forms and the streak chip rendered on each
  // virtue card. Best-effort: a missing habits module leaves
  // `habitsByName` empty and the linkage UI just shows names without
  // streak metadata. Reused across mount + edit.
  let habits = $state<HabitInfo[]>([]);
  let habitsByName = $derived.by(() => {
    const m = new Map<string, HabitInfo>();
    for (const h of habits) m.set(h.name.toLowerCase(), h);
    return m;
  });

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      // Fetch virtues + habits in parallel; habits failure shouldn't
      // block the virtues page (the linkage UI degrades gracefully
      // to plain name strings).
      const [v, h] = await Promise.all([
        api.listVirtues(),
        api.listHabits().catch(() => ({ habits: [] as HabitInfo[], today: '', days: 0 }))
      ]);
      virtues = v.virtues;
      habits = h.habits;
    } catch (e) {
      toast.error('failed to load virtues: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/virtues.json') load();
    });
    const onVisible = () => {
      if (document.visibilityState === 'visible') load();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
    };
  });

  // ----- Filter / sort -----

  let filtered = $derived.by(() => {
    let list = virtues;
    if (statusFilter !== 'all') list = list.filter((v) => (v.status ?? 'active') === statusFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      list = list.filter(
        (v) =>
          v.name.toLowerCase().includes(term) ||
          (v.description ?? '').toLowerCase().includes(term) ||
          (v.anchor ?? '').toLowerCase().includes(term) ||
          (v.season ?? '').toLowerCase().includes(term)
      );
    }
    return [...list].sort((a, b) => {
      const sa = a.status ?? 'active';
      const sb = b.status ?? 'active';
      if (sa !== sb) {
        const order = { active: 0, paused: 1, archived: 2 } as Record<string, number>;
        return (order[sa] ?? 9) - (order[sb] ?? 9);
      }
      return a.name.localeCompare(b.name);
    });
  });

  // ----- Helpers -----

  // Latest check by week-start lex order (YYYY-MM-DD sorts correctly).
  function latestCheck(v: Virtue): VirtueCheck | null {
    const cs = v.checks ?? [];
    if (cs.length === 0) return null;
    return cs.reduce((acc, c) => (c.week_start > acc.week_start ? c : acc), cs[0]);
  }

  // Score → tone mapping. 5 is success, 4 info, 3 warning, 1-2 error,
  // 0 / unset → dim. Mirrors the daily-task priority palette so a
  // colour scan across the page reads consistently.
  function scoreTone(score: number): string {
    if (score >= 5) return 'success';
    if (score === 4) return 'info';
    if (score === 3) return 'warning';
    if (score >= 1) return 'error';
    return 'dim';
  }

  // ----- Create -----

  function resetCreate() {
    nName = '';
    nDescription = '';
    nAnchor = '';
    nSeason = '';
    nColor = 'blue';
  }

  async function submitCreate(e?: SubmitEvent) {
    e?.preventDefault();
    if (!nName.trim()) return;
    saving = true;
    try {
      const v = await api.createVirtue({
        name: nName.trim(),
        description: nDescription.trim() || undefined,
        anchor: nAnchor.trim() || undefined,
        season: nSeason.trim() || undefined,
        color: nColor,
        status: 'active'
      });
      // Optimistic prepend — load() reconciles below.
      if (!virtues.some((x) => x.id === v.id)) {
        virtues = [v, ...virtues];
      }
      resetCreate();
      createOpen = false;
      toast.success(`virtue "${v.name}" created`);
      await load();
    } catch (err) {
      toast.error('create failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      saving = false;
    }
  }

  // ----- Status toggle -----

  async function setStatus(v: Virtue, status: 'active' | 'paused' | 'archived') {
    try {
      await api.patchVirtue(v.id, { status });
      await load();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // ----- Linked habits editor -----
  // One open-at-a-time linkage editor. Tracking by virtue id so a
  // tap on a different card auto-closes the previous one (no
  // multiple open editors stacking up). Toggle a habit on/off to
  // add/remove it from the linkage list; the patch is debounced by
  // a button rather than auto-save so the user can de-select +
  // re-select without firing a write per click.
  let linkingId = $state<string | null>(null);
  let linkBuf = $state<string[]>([]);
  let linkSaving = $state(false);

  function openLinkEditor(v: Virtue) {
    linkingId = v.id;
    linkBuf = [...(v.linked_habits ?? [])];
  }
  function cancelLinkEditor() {
    linkingId = null;
  }
  function toggleLinkBuf(name: string) {
    const idx = linkBuf.findIndex((n) => n.toLowerCase() === name.toLowerCase());
    if (idx >= 0) linkBuf = [...linkBuf.slice(0, idx), ...linkBuf.slice(idx + 1)];
    else linkBuf = [...linkBuf, name];
  }
  async function saveLinkEditor(v: Virtue) {
    linkSaving = true;
    try {
      await api.patchVirtue(v.id, { linked_habits: linkBuf });
      linkingId = null;
      await load();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      linkSaving = false;
    }
  }

  // ----- Weekly check -----

  function openCheckFor(v: Virtue) {
    openCheck = v.id;
    // Default to the latest score so the user can quickly nudge
    // up/down rather than starting from 0.
    const last = latestCheck(v);
    if (checkScore[v.id] === undefined) {
      checkScore = { ...checkScore, [v.id]: last?.score ?? 3 };
    }
    if (checkNote[v.id] === undefined) {
      checkNote = { ...checkNote, [v.id]: '' };
    }
  }

  async function submitCheck(v: Virtue) {
    const score = checkScore[v.id] ?? 3;
    const note = checkNote[v.id] ?? '';
    savingCheck = true;
    try {
      await api.logVirtueCheck(v.id, { score, note: note.trim() || undefined });
      // Reset the buffer for next week.
      checkScore = { ...checkScore, [v.id]: 3 };
      checkNote = { ...checkNote, [v.id]: '' };
      openCheck = null;
      toast.success(`checked ${v.name}: ${score}/5`);
      await load();
    } catch (err) {
      toast.error('save failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      savingCheck = false;
    }
  }

  function toggleHistory(id: string) {
    const next = new Set(expanded);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    expanded = next;
  }

  function fmtDate(s: string): string {
    if (!s) return '';
    const d = new Date(s);
    if (isNaN(d.getTime())) return s;
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }

  // ----- Stat strip -----
  // Glance summary tuned to the Sunday review rhythm:
  //   active        — all non-paused, non-archived virtues
  //   checkedRecent — has a check within the last 7 days
  //   needCheck     — active virtues with no check OR last check
  //                   more than 7 days old. The page leads with
  //                   "what's waiting on me?" so the count ties
  //                   directly to the user's next action.
  let stats = $derived.by(() => {
    const now = Date.now();
    const sevenDaysAgo = now - 7 * 24 * 3600 * 1000;
    let active = 0, checkedRecent = 0, needCheck = 0;
    for (const v of virtues) {
      const status = v.status ?? 'active';
      if (status !== 'active') continue;
      active++;
      const last = latestCheck(v);
      const lastTs = last ? new Date(last.week_start).getTime() : 0;
      if (lastTs >= sevenDaysAgo) checkedRecent++;
      else needCheck++;
    }
    return { active, checkedRecent, needCheck };
  });
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-3xl mx-auto">
    <PageHeader
      title="Virtues"
      subtitle="What God is forming in you. Name them, anchor them in scripture, check honestly each Sunday."
    >
      {#snippet actions()}
        <button
          onclick={() => (createOpen = !createOpen)}
          class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
        >{createOpen ? 'cancel' : '+ New virtue'}</button>
      {/snippet}
    </PageHeader>

    <!-- Stat strip — small one-line "shape of the Sunday review"
         summary. needCheck is the actionable count the user reads
         first, so it leads with a warning tone; checkedRecent is
         the success counterpart so the user feels the win when
         they're caught up. -->
    {#if stats.active > 0}
      <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1 mb-4 text-xs">
        <span class="text-text font-medium tabular-nums">{stats.active} active</span>
        {#if stats.needCheck > 0}
          <span class="text-warning tabular-nums">{stats.needCheck} {stats.needCheck === 1 ? 'waiting on check' : 'waiting on check'}</span>
        {/if}
        {#if stats.checkedRecent > 0}
          <span class="text-success/80 tabular-nums">{stats.checkedRecent} checked this week</span>
        {/if}
      </div>
    {/if}

    {#if createOpen}
      <form onsubmit={submitCreate} class="bg-surface0 border border-surface1 rounded-lg p-4 mb-6 space-y-3">
        <div>
          <label for="nv-name" class="text-xs uppercase tracking-wider text-dim block mb-1">Name</label>
          <input
            id="nv-name"
            bind:value={nName}
            required
            autofocus
            placeholder="Patience · Humility · Generosity · Courage · Presence · Discipline"
            class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text"
          />
        </div>
        <div>
          <label for="nv-anchor" class="text-xs uppercase tracking-wider text-dim block mb-1">Anchor</label>
          <input
            id="nv-anchor"
            bind:value={nAnchor}
            placeholder='e.g. "Love is patient, love is kind" — 1 Cor 13:4'
            class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text"
          />
          <p class="text-[11px] text-dim mt-1">
            Free-form: a scripture ref, a teacher's quote, your own commitment sentence. The line you'll re-read each week.
          </p>
        </div>
        <div>
          <label for="nv-desc" class="text-xs uppercase tracking-wider text-dim block mb-1">Description (optional)</label>
          <textarea
            id="nv-desc"
            bind:value={nDescription}
            rows="2"
            placeholder="What does living this virtue look like for you, in this season?"
            class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text"
          ></textarea>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label for="nv-season" class="text-xs uppercase tracking-wider text-dim block mb-1">Season (optional)</label>
            <input
              id="nv-season"
              bind:value={nSeason}
              placeholder="e.g. Lent 2026, Q3 deep work, fatherhood"
              class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text"
            />
          </div>
          <div>
            <span class="text-xs uppercase tracking-wider text-dim block mb-1">Color</span>
            <div class="flex flex-wrap gap-1.5">
              {#each colorOptions as c}
                <button
                  type="button"
                  onclick={() => (nColor = c)}
                  aria-label="color {c}"
                  class="w-6 h-6 rounded-full border-2 {nColor === c ? 'border-text' : 'border-surface1'}"
                  style="background: {colorVar(c)}"
                ></button>
              {/each}
            </div>
          </div>
        </div>
        <button
          type="submit"
          disabled={!nName.trim() || saving}
          class="w-full px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
        >{saving ? 'saving…' : 'Create'}</button>
      </form>
    {/if}

    <!-- Status tabs + search -->
    <div class="flex flex-wrap items-center gap-2 mb-4">
      <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
        {#each ['all', 'active', 'paused', 'archived'] as s}
          <button
            class="px-3 py-1.5 capitalize {statusFilter === s ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (statusFilter = s as typeof statusFilter)}
          >{s}</button>
        {/each}
      </div>
      <input
        bind:value={q}
        placeholder="search name, anchor, season…"
        class="flex-1 min-w-0 px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text"
      />
    </div>

    {#if loading && virtues.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if filtered.length === 0 && statusFilter === 'active'}
      <div class="bg-surface0 border border-surface1 rounded-lg p-6 text-center">
        <p class="text-sm text-text mb-1">No active virtues yet.</p>
        <p class="text-xs text-dim">
          Pick 2–3 to cultivate this season. Anchor each one in scripture.
          The Sunday review is where you check honestly — not a performance review,
          a noticing-what-God-is-forming.
        </p>
      </div>
    {:else if filtered.length === 0}
      <div class="text-sm text-dim italic">no virtues match this filter.</div>
    {:else}
      <ul class="space-y-3">
        {#each filtered as v (v.id)}
          {@const last = latestCheck(v)}
          {@const lastTone = last ? scoreTone(last.score) : 'dim'}
          {@const isExpanded = expanded.has(v.id)}
          {@const isCheckOpen = openCheck === v.id}
          {@const isLinking = linkingId === v.id}
          <li
            class="bg-surface0 border border-surface1 rounded-lg overflow-hidden"
            style="border-left: 3px solid {colorVar(v.color)};"
          >
            <div class="p-4">
              <div class="flex items-start gap-3">
                <div class="flex-1 min-w-0">
                  <div class="flex items-baseline gap-2 flex-wrap">
                    <h2 class="text-base sm:text-lg font-semibold text-text">{v.name}</h2>
                    {#if v.season}
                      <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-subtext">{v.season}</span>
                    {/if}
                    <span class="text-[10px] uppercase tracking-wider text-dim">{v.status ?? 'active'}</span>
                  </div>
                  {#if v.anchor}
                    <p class="text-sm text-secondary italic mt-1 break-words">"{v.anchor}"</p>
                  {/if}
                  {#if v.description}
                    <p class="text-sm text-subtext mt-1 break-words">{v.description}</p>
                  {/if}
                </div>
                {#if last}
                  <button
                    type="button"
                    onclick={() => toggleHistory(v.id)}
                    class="flex-shrink-0 flex flex-col items-center px-3 py-1.5 rounded hover:bg-mantle/40"
                    title="show history"
                  >
                    <span
                      class="text-2xl font-semibold tabular-nums leading-none"
                      style="color: var(--color-{lastTone});"
                    >{last.score}</span>
                    <span class="text-[10px] text-dim mt-0.5">last check</span>
                  </button>
                {/if}
              </div>

              <!-- Linked habits — surfaces what daily practices feed
                   this virtue. Each linked habit shows its current
                   streak (resolved against the live habits API) so
                   the user reads "discipline · 12d streak from
                   morning prayer" at a glance. Names that don't
                   resolve to a current habit (deleted, renamed)
                   render plain — never an error. -->
              <div class="mt-3 pt-3 border-t border-surface1">
                <div class="flex items-baseline justify-between gap-2 mb-1.5">
                  <span class="text-[11px] uppercase tracking-wider text-dim">
                    Habits feeding this virtue
                    {#if (v.linked_habits ?? []).length > 0}
                      <span class="text-dim/70 ml-0.5">· {v.linked_habits!.length}</span>
                    {/if}
                  </span>
                  {#if isLinking}
                    <div class="flex items-center gap-2 text-xs">
                      <button
                        type="button"
                        onclick={cancelLinkEditor}
                        class="text-dim hover:text-text"
                      >cancel</button>
                      <button
                        type="button"
                        onclick={() => saveLinkEditor(v)}
                        disabled={linkSaving}
                        class="px-2 py-0.5 bg-primary text-on-primary rounded font-medium disabled:opacity-50"
                      >{linkSaving ? '…' : 'save'}</button>
                    </div>
                  {:else}
                    <button
                      type="button"
                      onclick={() => openLinkEditor(v)}
                      class="text-[11px] text-secondary hover:underline"
                    >{(v.linked_habits ?? []).length === 0 ? '+ link habits' : 'edit'}</button>
                  {/if}
                </div>

                {#if isLinking}
                  {#if habits.length === 0}
                    <p class="text-xs text-dim italic">
                      No habits to link yet. <a href="/habits" class="text-secondary hover:underline">Add one in /habits</a>, then come back.
                    </p>
                  {:else}
                    <div class="flex flex-wrap gap-1.5">
                      {#each habits as h (h.name)}
                        {@const linked = linkBuf.some((n) => n.toLowerCase() === h.name.toLowerCase())}
                        <button
                          type="button"
                          onclick={() => toggleLinkBuf(h.name)}
                          class="inline-flex items-baseline gap-1.5 px-2 py-1 rounded text-xs border transition-colors
                            {linked
                              ? 'bg-primary/15 border-primary text-primary'
                              : 'bg-mantle/40 border-surface1 text-subtext hover:border-primary/40'}"
                        >
                          <span>{h.name}</span>
                          <span class="text-[10px] opacity-70">🔥 {h.currentStreak}d</span>
                        </button>
                      {/each}
                    </div>
                    <p class="text-[11px] text-dim mt-2">Tap a habit to toggle. Save to commit.</p>
                  {/if}
                {:else if (v.linked_habits ?? []).length > 0}
                  <ul class="flex flex-wrap gap-1.5">
                    {#each v.linked_habits ?? [] as hname}
                      {@const h = habitsByName.get(hname.toLowerCase())}
                      <li class="inline-flex items-baseline gap-1.5 px-2 py-1 bg-mantle/40 border border-surface1 rounded text-xs">
                        <span class="text-text">{hname}</span>
                        {#if h}
                          <span class="text-dim text-[10px]" title="current streak">🔥 {h.currentStreak}d</span>
                        {:else}
                          <span class="text-dim/60 text-[10px]" title="not in today's daily note">—</span>
                        {/if}
                      </li>
                    {/each}
                  </ul>
                {:else}
                  <p class="text-[11px] text-dim italic">
                    No habits linked yet. Daily practices that cultivate this virtue.
                  </p>
                {/if}
              </div>

              <!-- Inline check controls -->
              <div class="mt-3 pt-3 border-t border-surface1">
                {#if !isCheckOpen}
                  <div class="flex items-center justify-between gap-2 text-xs">
                    <span class="text-dim">
                      {#if last}
                        last checked {fmtDate(last.week_start)}
                      {:else}
                        no checks yet — log your first reflection
                      {/if}
                    </span>
                    <div class="flex items-center gap-1">
                      <button
                        onclick={() => openCheckFor(v)}
                        class="px-2.5 py-1 bg-primary text-on-primary rounded text-xs font-medium hover:opacity-90"
                      >+ check this week</button>
                      <select
                        value={v.status ?? 'active'}
                        onchange={(e) => setStatus(v, (e.target as HTMLSelectElement).value as 'active' | 'paused' | 'archived')}
                        class="px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-subtext hover:border-primary"
                        aria-label="status"
                      >
                        <option value="active">active</option>
                        <option value="paused">paused</option>
                        <option value="archived">archived</option>
                      </select>
                    </div>
                  </div>
                {:else}
                  <form
                    onsubmit={(e) => { e.preventDefault(); submitCheck(v); }}
                    class="space-y-2"
                  >
                    <div class="flex flex-wrap items-center gap-1.5">
                      <span class="text-xs text-dim mr-1">Score</span>
                      {#each [1, 2, 3, 4, 5] as n}
                        <button
                          type="button"
                          onclick={() => (checkScore = { ...checkScore, [v.id]: n })}
                          class="w-9 h-9 rounded text-sm font-semibold border tabular-nums
                            {(checkScore[v.id] ?? 3) === n
                              ? 'bg-primary text-on-primary border-primary'
                              : 'bg-surface0 text-subtext border-surface1 hover:border-primary/40'}"
                        >{n}</button>
                      {/each}
                      <span class="text-[11px] text-dim ml-1">honest, not punishing</span>
                    </div>
                    <textarea
                      value={checkNote[v.id] ?? ''}
                      oninput={(e) => (checkNote = { ...checkNote, [v.id]: (e.target as HTMLTextAreaElement).value })}
                      rows="2"
                      placeholder="What did you notice this week? Where was the grace? Where the friction?"
                      class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text placeholder-dim"
                    ></textarea>
                    <div class="flex items-center gap-2">
                      <button
                        type="button"
                        onclick={() => (openCheck = null)}
                        class="px-2.5 py-1 text-xs text-subtext hover:text-text"
                      >cancel</button>
                      <span class="flex-1"></span>
                      <button
                        type="submit"
                        disabled={savingCheck}
                        class="px-3 py-1.5 bg-primary text-on-primary rounded text-xs font-medium disabled:opacity-50"
                      >{savingCheck ? 'saving…' : 'save check'}</button>
                    </div>
                  </form>
                {/if}
              </div>

              {#if isExpanded && (v.checks?.length ?? 0) > 0}
                <!-- Year heatmap. Virtue checks are weekly, so we
                     paint each week's 7 days with the same score —
                     gives the visual a familiar habit-grid feel
                     without inventing fake daily resolution. -->
                <div class="mt-3 pt-3 border-t border-surface1">
                  <Heatmap
                    cells={(v.checks ?? []).flatMap((c) => {
                      const start = new Date(c.week_start);
                      const out = [];
                      for (let d = 0; d < 7; d++) {
                        const dt = new Date(start);
                        dt.setDate(start.getDate() + d);
                        out.push({
                          date: `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, '0')}-${String(dt.getDate()).padStart(2, '0')}`,
                          value: c.score
                        });
                      }
                      return out;
                    })}
                    maxValue={5}
                    legendLabels={['1', '2', '3', '4', '5']}
                  />
                </div>
                <ul class="mt-3 pt-3 border-t border-surface1 space-y-1.5">
                  {#each v.checks ?? [] as c (c.week_start)}
                    {@const tone = scoreTone(c.score)}
                    <li class="flex items-baseline gap-3 text-sm">
                      <span class="text-xs text-dim font-mono tabular-nums w-20 flex-shrink-0">{c.week_start}</span>
                      <span
                        class="font-semibold tabular-nums w-5 text-center flex-shrink-0"
                        style="color: var(--color-{tone});"
                      >{c.score}</span>
                      <span class="flex-1 text-subtext break-words">{c.note || '—'}</span>
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}

    <footer class="mt-10 pt-4 border-t border-surface1 text-[11px] text-dim">
      Synced via <code class="font-mono">.granit/virtues.json</code> — same file the granit TUI reads.
    </footer>
  </div>
</div>
