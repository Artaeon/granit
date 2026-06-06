<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { api } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import {
    colorVar,
    statusTone,
    countdown,
    deadlineTone,
    noteTitle,
    noteBodyExcerpt
  } from '$lib/ventures/venturesDetailHelpers';
  import { createVenturesDetailAISummary } from '$lib/ventures/venturesDetailAISummary.svelte';
  import {
    createVenturesDetailData,
    type VenturesDetailTab
  } from '$lib/ventures/venturesDetailData.svelte';
  import VentureHero from '$lib/ventures/VentureHero.svelte';
  import VentureMetricStrip from '$lib/ventures/VentureMetricStrip.svelte';

  // Venture detail page — single aggregation view answering "what's
  // the current state of this venture, all in one place?". The
  // /ventures list page is the catalogue; this is the detail.
  //
  // Aggregation is client-side (multiple parallel API calls + filter)
  // because:
  //   - all four sources are already loaded for other pages, so the
  //     browser usually serves them from cache;
  //   - a server-side rollup endpoint would be one more thing to keep
  //     in sync with the underlying schemas;
  //   - the cost is one extra network round-trip vs. four — not the
  //     bottleneck for the data sizes we're dealing with.
  // If perf becomes a problem we can collapse to a /ventures/{name}
  // server-decorated response without touching this page's render
  // logic.
  //
  // Sub-tabs (overview / projects / goals / links / notes) keep the
  // long-tail content out of the initial scan path. The default tab
  // (overview) shows the metric strip + progress + everything in
  // small "preview" form, so power users who want to scan everything
  // at once get it on one screen; deep dives switch tabs.

  let tab = $state<VenturesDetailTab>('overview');

  // The route param is the raw venture name (URL-encoded). We
  // case-insensitive-match on the server (ventures.Find) but keep
  // the user's original casing in the displayed name; the lookup
  // here is exact-equality on the decoded segment.
  let name = $derived(decodeURIComponent($page.params.name ?? ''));

  // ----- Data controller -----
  // Owns the loaded state arrays, the parallel load(), and every
  // derived rollup the page renders against. The page reads through
  // $derived getters so existing template bindings stay
  // identifier-level (no `data.X` dotted access in markup churn).
  const data = createVenturesDetailData({ getName: () => name });
  let venture = $derived(data.venture);
  let projects = $derived(data.projects);
  let goals = $derived(data.goals);
  let linkedNotes = $derived(data.linkedNotes);
  let loading = $derived(data.loading);
  let notFound = $derived(data.notFound);
  let activeProjects = $derived(data.activeProjects);
  let pausedProjects = $derived(data.pausedProjects);
  let activeGoals = $derived(data.activeGoals);
  let activeDeadlines = $derived(data.activeDeadlines);
  let activeIntentions = $derived(data.activeIntentions);
  let aggregateTasksOpen = $derived(data.aggregateTasksOpen);
  let aggregateTasksDone = $derived(data.aggregateTasksDone);
  let aggregateProgress = $derived(data.aggregateProgress);
  let nextDeadline = $derived(data.nextDeadline);
  let tabs = $derived(data.tabs);

  // ----- AI summary state -----
  // The "Summarize" button hits chatStream with a compact JSON snapshot
  // of the venture + its rolled-up projects/goals/deadlines. The model
  // returns prose; we render it as plain paragraphs (no markdown
  // parsing — keeps this page small, and the model is prompted for
  // plain prose). Audit-gated automatically because chatStream goes
  // through /chat/stream → gateChat + auditChat.
  const aiSummary = createVenturesDetailAISummary({
    getVenture: () => data.venture,
    getProjects: () => data.projects,
    getGoals: () => data.goals,
    getActiveDeadlines: () => data.activeDeadlines,
    getActiveIntentions: () => data.activeIntentions
  });
  let aiBusy = $derived(aiSummary.busy);
  let aiText = $derived(aiSummary.text);
  let aiError = $derived(aiSummary.error);

  onMount(() => {
    data.load();
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/ventures.json') data.load();
      if (ev.type.startsWith('project.')) data.load();
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') data.load();
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') data.load();
      if (ev.type === 'state.changed' && ev.path === '.granit/prayer/intentions.json') data.load();
    });
    const onVisible = () => {
      if (document.visibilityState === 'visible') data.load();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
      aiSummary.dismiss();
    };
  });

  // Re-fetch when the URL param changes (e.g. user navigates from one
  // venture to another). Reset AI summary too — it's per-venture.
  $effect(() => {
    void name;
    data.load();
    aiSummary.dismiss();
    tab = 'overview';
  });

  // Status change for the venture itself — surfaced as a select beside
  // the status badge in the hero so the user can re-bucket from the
  // detail page without bouncing back to /ventures.
  async function changeStatus(next: 'active' | 'paused' | 'archived') {
    const v = data.venture;
    if (!v || (v.status ?? 'active') === next) return;
    try {
      const updated = await api.patchVenture(v.name, { status: next });
      data.venture = updated;
      toast.success(`${updated.name} → ${next}`);
    } catch (err) {
      toast.error('update failed: ' + errorMessage(err));
    }
  }

</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    {#if loading && !venture}
      <!-- Skeleton hero so the page doesn't reflow when data arrives. -->
      <div class="mb-4 flex items-start gap-3">
        <div class="w-9 h-9 flex-shrink-0"></div>
        <Skeleton class="w-3 h-3 rounded-full mt-3" />
        <div class="flex-1 space-y-2">
          <Skeleton class="h-7 w-1/2" />
          <Skeleton class="h-4 w-3/4" />
          <Skeleton class="h-3 w-2/3" />
        </div>
      </div>
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 mb-4">
        {#each [0, 1, 2, 3] as i (i)}
          <Skeleton class="h-16 rounded" />
        {/each}
      </div>
      <Skeleton class="h-2 w-full rounded-full" />
    {:else if notFound}
      <div class="bg-surface0 border border-error rounded-lg p-6 text-center">
        <p class="text-sm text-text mb-2">No venture named <strong>{name}</strong> found.</p>
        <a href="/ventures" class="text-sm text-secondary hover:underline">← back to ventures</a>
      </div>
    {:else if venture}
      <!-- Header / hero — colored bar, name, mission, description,
           status pill (with inline change), URL, tags. The hero leans
           heavily on whitespace; the metric strip below is where the
           dense data lives. -->
      <VentureHero
        {venture}
        {aiBusy}
        {aiText}
        onChangeStatus={changeStatus}
        onSummarize={aiSummary.summarize}
      />

      <!-- Metric strip — aggregate row + progress bar + AI summary
           panel. All three sit between the hero and the sub-nav,
           share the "what's the current state" intent, and extract
           together as one above-tabs band. -->
      <VentureMetricStrip
        {venture}
        activeProjectsCount={activeProjects.length}
        activeGoalsCount={activeGoals.length}
        activeIntentionsCount={activeIntentions.length}
        {nextDeadline}
        {aggregateProgress}
        {aggregateTasksOpen}
        {aggregateTasksDone}
        showProgressBar={activeProjects.length > 0}
        {aiBusy}
        {aiText}
        {aiError}
        onDismissAI={aiSummary.dismiss}
      />

      <!-- Sub-tabs — overview is the default and folds the four legacy
           sections into one tighter layout. The other tabs are deep
           views: Projects gets a richer per-project row (description
           + progress + task counts), Goals shows milestones with
           target dates, etc. The notes tab only appears when there
           are linked notes (search hit), so a fresh venture stays
           uncluttered. -->
      <nav class="flex flex-wrap gap-1 border-b border-surface1 mb-4 overflow-x-auto" aria-label="Venture sections">
        {#each tabs as t (t.id)}
          <button
            class="px-3 py-2 text-sm border-b-2 -mb-px flex items-center gap-1.5 whitespace-nowrap transition-colors {tab === t.id ? 'border-primary text-text font-medium' : 'border-transparent text-subtext hover:text-text'}"
            onclick={() => (tab = t.id)}
            aria-current={tab === t.id ? 'page' : undefined}
          >
            <span>{t.label}</span>
            {#if t.count !== undefined && t.count > 0}
              <span class="text-[10px] tabular-nums px-1.5 py-0.5 rounded bg-surface1 text-dim">{t.count}</span>
            {/if}
          </button>
        {/each}
      </nav>

      {#if tab === 'overview'}
        <!-- Overview tab — compact preview of every section so the
             user can scan everything at once. Each preview links to
             its dedicated tab for the deep view. -->
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <!-- Projects preview -->
          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Projects · {activeProjects.length}</h2>
              {#if projects.length > 0}
                <button class="text-xs text-secondary hover:underline" onclick={() => (tab = 'projects')}>see all →</button>
              {:else}
                <a
                  href={`/projects?venture=${encodeURIComponent(venture.name)}`}
                  class="text-xs text-secondary hover:underline"
                >+ new →</a>
              {/if}
            </div>
            {#if activeProjects.length === 0 && pausedProjects.length === 0}
              <a
                href={`/projects?venture=${encodeURIComponent(venture.name)}`}
                class="block px-3 py-4 border border-dashed border-surface1 rounded text-center text-xs text-dim hover:border-primary hover:text-text transition-colors"
              >
                No projects linked yet.
                <span class="block mt-0.5 text-secondary">+ create one for this venture</span>
              </a>
            {:else}
              <ul class="space-y-1.5">
                {#each activeProjects.slice(0, 4) as p (p.name)}
                  {@const progress = Math.round((p.progress ?? 0) * 100)}
                  <li>
                    <a
                      href={`/projects?p=${encodeURIComponent(p.name)}`}
                      class="block px-3 py-2 bg-surface0 border border-surface1 rounded hover:border-primary transition-colors group"
                    >
                      <div class="flex items-baseline gap-2">
                        <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(p.color)}"></span>
                        <span class="text-sm text-text flex-1 min-w-0 truncate group-hover:text-primary">{p.name}</span>
                        {#if p.kind}
                          <span class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-primary flex-shrink-0">{p.kind}</span>
                        {/if}
                      </div>
                      <div class="flex items-center gap-2 mt-1.5">
                        <div class="flex-1 h-1 rounded-full bg-mantle overflow-hidden">
                          <div class="h-full" style="width: {progress}%; background: {colorVar(p.color)}"></div>
                        </div>
                        <span class="text-[10px] text-dim font-mono w-9 text-right">{progress}%</span>
                      </div>
                    </a>
                  </li>
                {/each}
                {#if activeProjects.length > 4}
                  <li>
                    <button class="text-[11px] text-dim hover:text-text px-2" onclick={() => (tab = 'projects')}>
                      + {activeProjects.length - 4} more active
                    </button>
                  </li>
                {/if}
                {#if pausedProjects.length > 0}
                  <li class="text-[11px] text-dim italic px-2 pt-1">
                    + {pausedProjects.length} paused
                  </li>
                {/if}
              </ul>
            {/if}
          </section>

          <!-- Goals preview -->
          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Goals · {activeGoals.length}</h2>
              {#if goals.length > 0}
                <button class="text-xs text-secondary hover:underline" onclick={() => (tab = 'goals')}>see all →</button>
              {:else}
                <a href="/goals" class="text-xs text-secondary hover:underline">+ new →</a>
              {/if}
            </div>
            {#if activeGoals.length === 0}
              <a
                href="/goals"
                class="block px-3 py-4 border border-dashed border-surface1 rounded text-center text-xs text-dim hover:border-primary hover:text-text transition-colors"
              >
                No goals linked yet.
                <span class="block mt-0.5 text-secondary">+ link this venture to a goal</span>
              </a>
            {:else}
              <ul class="space-y-1.5">
                {#each activeGoals.slice(0, 4) as g (g.id)}
                  {@const ms = g.milestones ?? []}
                  {@const total = ms.length}
                  {@const done = ms.filter((m) => m.done).length}
                  {@const pct = total === 0 ? (g.status === 'completed' ? 100 : 0) : Math.round((done / total) * 100)}
                  <li>
                    <a
                      href={`/goals?focus=${encodeURIComponent(g.id)}`}
                      class="block px-3 py-2 bg-surface0 border border-surface1 rounded hover:border-primary transition-colors group"
                    >
                      <div class="flex items-baseline gap-2">
                        <span class="text-sm text-text flex-1 min-w-0 truncate group-hover:text-primary">{g.title}</span>
                        {#if g.target_date}
                          <span class="text-[10px] text-dim font-mono flex-shrink-0">🎯 {g.target_date}</span>
                        {/if}
                      </div>
                      {#if total > 0}
                        <div class="flex items-center gap-2 mt-1.5">
                          <div class="flex-1 h-1 rounded-full bg-mantle overflow-hidden">
                            <div class="h-full bg-primary" style="width: {pct}%"></div>
                          </div>
                          <span class="text-[10px] text-dim font-mono">{done}/{total}</span>
                        </div>
                      {/if}
                    </a>
                  </li>
                {/each}
                {#if activeGoals.length > 4}
                  <li>
                    <button class="text-[11px] text-dim hover:text-text px-2" onclick={() => (tab = 'goals')}>
                      + {activeGoals.length - 4} more
                    </button>
                  </li>
                {/if}
              </ul>
            {/if}
          </section>

          <!-- Deadlines preview -->
          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Deadlines · {activeDeadlines.length}</h2>
              <div class="flex items-center gap-2 text-xs">
                <a
                  href={`/deadlines?venture=${encodeURIComponent(venture.name)}&new=1`}
                  class="text-secondary hover:underline"
                >+ add</a>
                <a
                  href={`/deadlines?venture=${encodeURIComponent(venture.name)}`}
                  class="text-dim hover:text-text"
                >all →</a>
              </div>
            </div>
            {#if activeDeadlines.length === 0}
              <p class="text-xs text-dim italic px-2.5">No active deadlines.</p>
            {:else}
              <ul class="space-y-1">
                {#each activeDeadlines.slice(0, 6) as d (d.id)}
                  {@const tone = deadlineTone(d)}
                  <li>
                    <a
                      href={`/deadlines?venture=${encodeURIComponent(venture.name)}#${d.id}`}
                      class="flex items-baseline gap-2 px-2.5 py-1.5 rounded hover:bg-surface0 group"
                      style="border-left: 2px solid var(--color-{tone});"
                    >
                      <span class="text-sm text-text flex-1 truncate group-hover:text-primary">{d.title}</span>
                      {#if d.importance === 'critical'}
                        <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-surface0 text-error flex-shrink-0">crit</span>
                      {:else if d.importance === 'high'}
                        <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-surface0 text-warning flex-shrink-0">high</span>
                      {/if}
                      <span class="text-xs tabular-nums flex-shrink-0" style="color: var(--color-{tone});">{countdown(d)}</span>
                    </a>
                  </li>
                {/each}
                {#if activeDeadlines.length > 6}
                  <li>
                    <a
                      href={`/deadlines?venture=${encodeURIComponent(venture.name)}`}
                      class="block px-2.5 py-1 text-[11px] text-dim hover:text-text"
                    >+ {activeDeadlines.length - 6} more</a>
                  </li>
                {/if}
              </ul>
            {/if}
          </section>

          <!-- Prayer intentions preview -->
          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Praying for · {activeIntentions.length}</h2>
              <div class="flex items-center gap-2 text-xs">
                <a
                  href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
                  class="text-secondary hover:underline"
                >+ add</a>
                <a
                  href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
                  class="text-dim hover:text-text"
                >all →</a>
              </div>
            </div>
            {#if activeIntentions.length === 0}
              <p class="text-xs text-dim italic px-2.5">
                <a href={`/prayer?venture=${encodeURIComponent(venture.name)}`} class="text-secondary hover:underline">Bring it before God</a> — what are you asking Him for in this venture?
              </p>
            {:else}
              <ul class="space-y-1.5">
                {#each activeIntentions.slice(0, 6) as p (p.id)}
                  <li class="px-2.5 py-1.5 bg-surface0 rounded">
                    <div class="text-sm text-text break-words">{p.text}</div>
                    {#if p.passage_ref || p.started_at}
                      <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 mt-0.5 text-[11px] text-dim">
                        {#if p.passage_ref}<span>📖 {p.passage_ref}</span>{/if}
                        {#if p.started_at}<span>since {p.started_at}</span>{/if}
                      </div>
                    {/if}
                  </li>
                {/each}
                {#if activeIntentions.length > 6}
                  <li class="text-[11px] text-dim px-2 italic">+ {activeIntentions.length - 6} more</li>
                {/if}
              </ul>
            {/if}
          </section>
        </div>

      {:else if tab === 'projects'}
        <!-- Projects tab — full list with description + status pill +
             progress + task counts. Active first, paused below in a
             muted block. -->
        {#if projects.length === 0}
          <div class="text-center py-6 text-sm text-dim">
            No projects linked to this venture yet.
            <div class="mt-2">
              <a
                href={`/projects?venture=${encodeURIComponent(venture.name)}`}
                class="text-secondary hover:underline"
              >Create one →</a>
            </div>
          </div>
        {:else}
          <ul class="space-y-2">
            {#each projects as p (p.name)}
              {@const progress = Math.round((p.progress ?? 0) * 100)}
              {@const tasksTotal = p.tasksTotal ?? 0}
              {@const tasksDone = p.tasksDone ?? 0}
              <li>
                <a
                  href={`/projects?p=${encodeURIComponent(p.name)}`}
                  class="block px-4 py-3 bg-surface0 border border-surface1 rounded-lg hover:border-primary transition-colors group"
                >
                  <div class="flex items-start gap-2">
                    <span class="w-2 h-2 rounded-full flex-shrink-0 mt-1.5" style="background: {colorVar(p.color)}"></span>
                    <div class="flex-1 min-w-0">
                      <div class="flex items-baseline gap-2 flex-wrap">
                        <span class="text-sm font-medium text-text group-hover:text-primary truncate">{p.name}</span>
                        {#if p.kind}
                          <span class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-primary">{p.kind}</span>
                        {/if}
                        <span
                          class="text-[10px] uppercase tracking-wider"
                          style="color: var(--color-{statusTone(p.status)});"
                        >{p.status ?? 'active'}</span>
                        {#if p.due_date}
                          <span class="text-[10px] text-dim font-mono ml-auto">due {p.due_date}</span>
                        {/if}
                      </div>
                      {#if p.description}
                        <p class="text-xs text-subtext line-clamp-2 mt-1">{p.description}</p>
                      {/if}
                      <div class="flex items-center gap-2 mt-2">
                        <div class="flex-1 h-1 rounded-full bg-mantle overflow-hidden">
                          <div class="h-full" style="width: {progress}%; background: {colorVar(p.color)}"></div>
                        </div>
                        <span class="text-[10px] text-dim font-mono w-9 text-right">{progress}%</span>
                        {#if tasksTotal > 0}
                          <span class="text-[10px] text-dim font-mono whitespace-nowrap">{tasksDone}/{tasksTotal} tasks</span>
                        {/if}
                      </div>
                      {#if p.next_action}
                        <p class="text-[11px] text-secondary mt-1.5">→ {p.next_action}</p>
                      {/if}
                    </div>
                  </div>
                </a>
              </li>
            {/each}
          </ul>
        {/if}

      {:else if tab === 'goals'}
        <!-- Goals tab — every goal with milestone breakdown. -->
        {#if goals.length === 0}
          <div class="text-center py-6 text-sm text-dim">
            No goals linked to this venture yet.
            <div class="mt-2">
              <a href="/goals" class="text-secondary hover:underline">Create one →</a>
            </div>
          </div>
        {:else}
          <ul class="space-y-2">
            {#each goals as g (g.id)}
              {@const ms = g.milestones ?? []}
              {@const total = ms.length}
              {@const done = ms.filter((m) => m.done).length}
              {@const pct = total === 0 ? (g.status === 'completed' ? 100 : 0) : Math.round((done / total) * 100)}
              <li>
                <a
                  href={`/goals?focus=${encodeURIComponent(g.id)}`}
                  class="block px-4 py-3 bg-surface0 border border-surface1 rounded-lg hover:border-primary transition-colors group"
                >
                  <div class="flex items-baseline gap-2 flex-wrap">
                    <span class="text-sm font-medium text-text group-hover:text-primary flex-1 min-w-0 truncate">{g.title}</span>
                    <span
                      class="text-[10px] uppercase tracking-wider"
                      style="color: var(--color-{statusTone(g.status)});"
                    >{g.status ?? 'active'}</span>
                    {#if g.target_date}
                      <span class="text-[10px] text-dim font-mono">🎯 {g.target_date}</span>
                    {/if}
                  </div>
                  {#if g.description}
                    <p class="text-xs text-subtext line-clamp-2 mt-1">{g.description}</p>
                  {/if}
                  {#if total > 0}
                    <div class="flex items-center gap-2 mt-2">
                      <div class="flex-1 h-1 rounded-full bg-mantle overflow-hidden">
                        <div class="h-full bg-primary" style="width: {pct}%"></div>
                      </div>
                      <span class="text-[10px] text-dim font-mono whitespace-nowrap">{done}/{total} milestones</span>
                    </div>
                    <ul class="mt-2 space-y-0.5">
                      {#each ms.slice(0, 4) as m, i (i)}
                        <li class="flex items-center gap-2 text-xs">
                          <span aria-hidden="true" class="w-3 inline-flex justify-center" style="color: {m.done ? 'var(--color-success)' : 'var(--color-dim)'};">{m.done ? '✓' : '○'}</span>
                          <span class={m.done ? 'text-dim line-through' : 'text-subtext'}>{m.text}</span>
                          {#if m.due_date}
                            <span class="ml-auto text-[10px] text-dim font-mono">{m.due_date}</span>
                          {/if}
                        </li>
                      {/each}
                      {#if ms.length > 4}
                        <li class="text-[11px] text-dim ml-5">+ {ms.length - 4} more</li>
                      {/if}
                    </ul>
                  {/if}
                </a>
              </li>
            {/each}
          </ul>
        {/if}

      {:else if tab === 'links'}
        <!-- Links tab — deadlines + prayer side by side, each in full
             list form rather than the truncated overview preview. -->
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Deadlines · {activeDeadlines.length}</h2>
              <a
                href={`/deadlines?venture=${encodeURIComponent(venture.name)}&new=1`}
                class="text-xs text-secondary hover:underline"
              >+ add</a>
            </div>
            {#if activeDeadlines.length === 0}
              <p class="text-xs text-dim italic px-2.5">No active deadlines.</p>
            {:else}
              <ul class="space-y-1">
                {#each activeDeadlines as d (d.id)}
                  {@const tone = deadlineTone(d)}
                  <li>
                    <a
                      href={`/deadlines?venture=${encodeURIComponent(venture.name)}#${d.id}`}
                      class="flex items-baseline gap-2 px-2.5 py-2 rounded hover:bg-surface0 group"
                      style="border-left: 2px solid var(--color-{tone});"
                    >
                      <span class="text-sm text-text flex-1 truncate group-hover:text-primary">{d.title}</span>
                      {#if d.importance === 'critical'}
                        <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-surface0 text-error flex-shrink-0">crit</span>
                      {:else if d.importance === 'high'}
                        <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-surface0 text-warning flex-shrink-0">high</span>
                      {/if}
                      <span class="text-xs tabular-nums flex-shrink-0" style="color: var(--color-{tone});">{countdown(d)}</span>
                    </a>
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          <section>
            <div class="flex items-baseline justify-between mb-2">
              <h2 class="text-sm font-medium text-text">Praying for · {activeIntentions.length}</h2>
              <a
                href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
                class="text-xs text-secondary hover:underline"
              >+ add</a>
            </div>
            {#if activeIntentions.length === 0}
              <p class="text-xs text-dim italic px-2.5">
                <a href={`/prayer?venture=${encodeURIComponent(venture.name)}`} class="text-secondary hover:underline">Bring it before God</a> — what are you asking Him for in this venture?
              </p>
            {:else}
              <ul class="space-y-1.5">
                {#each activeIntentions as p (p.id)}
                  <li class="px-2.5 py-2 bg-surface0 rounded">
                    <div class="text-sm text-text break-words">{p.text}</div>
                    {#if p.passage_ref || p.started_at}
                      <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 mt-0.5 text-[11px] text-dim">
                        {#if p.passage_ref}<span>📖 {p.passage_ref}</span>{/if}
                        {#if p.started_at}<span>since {p.started_at}</span>{/if}
                      </div>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </section>
        </div>

      {:else if tab === 'notes'}
        <!-- Notes tab — full-text-search hits for the venture name.
             Shows an excerpt around the match so the user can see why
             the note surfaced. Best-effort cross-link: notes don't
             have a structured venture field, so a hit on the name in
             body or frontmatter is our signal. -->
        {#if linkedNotes.length === 0}
          <p class="text-sm text-dim italic">No notes mention this venture yet.</p>
        {:else}
          <ul class="space-y-2">
            {#each linkedNotes as n (n.path)}
              {@const excerpt = noteBodyExcerpt(n, name)}
              <li>
                <a
                  href={`/notes/${encodeURI(n.path)}`}
                  class="block px-4 py-3 bg-surface0 border border-surface1 rounded-lg hover:border-primary transition-colors group"
                >
                  <div class="flex items-baseline gap-2">
                    <span class="text-sm font-medium text-text group-hover:text-primary truncate flex-1 min-w-0">{noteTitle(n)}</span>
                    {#if n.modTime}
                      <span class="text-[10px] text-dim font-mono">{n.modTime.slice(0, 10)}</span>
                    {/if}
                  </div>
                  <p class="text-[11px] text-dim font-mono truncate mt-0.5">{n.path}</p>
                  {#if excerpt}
                    <p class="text-xs text-subtext line-clamp-2 mt-1">…{excerpt}…</p>
                  {/if}
                  {#if n.tags && n.tags.length > 0}
                    <div class="flex flex-wrap gap-1 mt-1.5">
                      {#each n.tags.slice(0, 5) as t}
                        <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                      {/each}
                    </div>
                  {/if}
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
    {/if}
  </div>
</div>
