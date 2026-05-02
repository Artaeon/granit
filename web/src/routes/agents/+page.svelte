<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { api, type AgentPreset, type AgentRun } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import AgentRunPanel from '$lib/agents/AgentRunPanel.svelte';

  // Two-tab page: presets the agent runner can use, plus a timeline of
  // past runs (each run is persisted as an `agent_run` typed-object note
  // by the TUI, so the web sees them automatically).
  type Tab = 'presets' | 'runs';
  let tab = $state<Tab>('runs');

  let presets = $state<AgentPreset[]>([]);
  let runOpen = $state(false);
  let runPreset = $state<AgentPreset | null>(null);

  function startRun(p: AgentPreset) {
    runPreset = p;
    runOpen = true;
  }
  let runs = $state<AgentRun[]>([]);
  let stats = $state<Record<string, Record<string, number>>>({});
  let loading = $state(false);
  let q = $state('');
  let showPrompts = $state(false);

  async function load() {
    loading = true;
    try {
      const [p, r] = await Promise.all([
        api.listAgentPresets(showPrompts),
        api.listAgentRuns(200)
      ]);
      presets = p.presets;
      runs = r.runs;
      stats = r.stats;
    } catch (e) {
      toast.error('failed to load agents: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      // agent runs land on disk as new notes — the WS notifies us.
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  // Filtered to whatever the user typed in the search box.
  let filteredPresets = $derived.by(() => {
    const term = q.trim().toLowerCase();
    if (!term) return presets;
    return presets.filter((p) =>
      p.name.toLowerCase().includes(term) ||
      p.description.toLowerCase().includes(term) ||
      p.id.toLowerCase().includes(term)
    );
  });

  let filteredRuns = $derived.by(() => {
    const term = q.trim().toLowerCase();
    if (!term) return runs;
    return runs.filter((r) =>
      r.preset.toLowerCase().includes(term) ||
      r.goal.toLowerCase().includes(term) ||
      r.title.toLowerCase().includes(term)
    );
  });

  // Aggregate stats for the header strip.
  let totalRuns = $derived(runs.length);
  let okRuns = $derived(runs.filter((r) => r.status === 'ok').length);
  let recentRuns = $derived.by(() => {
    const week = Date.now() - 7 * 24 * 60 * 60 * 1000;
    return runs.filter((r) => {
      const t = new Date(r.started).getTime();
      return !isNaN(t) && t > week;
    }).length;
  });

  function statusTone(status: string): string {
    if (status === 'ok') return 'success';
    if (status === 'budget') return 'warning';
    if (status === 'cancelled') return 'subtext';
    if (status === 'error') return 'error';
    return 'subtext';
  }

  function fmtDate(iso: string): string {
    if (!iso) return '—';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    const now = new Date();
    const sameDay = d.toDateString() === now.toDateString();
    if (sameDay) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
    const days = Math.floor((now.getTime() - d.getTime()) / 86400000);
    if (days < 7) return d.toLocaleDateString(undefined, { weekday: 'short', hour: '2-digit', minute: '2-digit', hour12: false });
    return d.toLocaleDateString();
  }

  function openRun(r: AgentRun) {
    goto(`/notes/${encodeURIComponent(r.path)}`);
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-5xl mx-auto p-4 sm:p-6 lg:p-8">
    <PageHeader title="Agents" subtitle="AI agents you can run, plus the history of past runs" />

    <!-- Stats strip — three numbers people care about at a glance. -->
    <div class="grid grid-cols-3 gap-2 sm:gap-3 mb-4">
      <div class="bg-surface0 border border-surface1 rounded-lg p-3">
        <div class="text-2xl sm:text-3xl font-mono font-semibold text-text tabular-nums">{totalRuns}</div>
        <div class="text-[11px] uppercase tracking-wider text-dim">total runs</div>
      </div>
      <div class="bg-surface0 border border-surface1 rounded-lg p-3">
        <div class="text-2xl sm:text-3xl font-mono font-semibold text-success tabular-nums">{okRuns}</div>
        <div class="text-[11px] uppercase tracking-wider text-dim">completed ok</div>
      </div>
      <div class="bg-surface0 border border-surface1 rounded-lg p-3">
        <div class="text-2xl sm:text-3xl font-mono font-semibold text-primary tabular-nums">{recentRuns}</div>
        <div class="text-[11px] uppercase tracking-wider text-dim">last 7 days</div>
      </div>
    </div>

    <!-- Tabs + search row. -->
    <div class="flex items-center gap-2 flex-wrap mb-4">
      <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
        <button
          class="px-3 py-1.5 {tab === 'runs' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (tab = 'runs')}
        >Runs <span class="text-[10px] opacity-70">{runs.length}</span></button>
        <button
          class="px-3 py-1.5 {tab === 'presets' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (tab = 'presets')}
        >Presets <span class="text-[10px] opacity-70">{presets.length}</span></button>
      </div>
      <input
        bind:value={q}
        placeholder={tab === 'runs' ? 'filter by goal / preset…' : 'filter presets…'}
        class="flex-1 min-w-0 px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      {#if tab === 'presets'}
        <button
          onclick={async () => { showPrompts = !showPrompts; await load(); }}
          class="text-xs px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary"
        >{showPrompts ? 'hide prompts' : 'show prompts'}</button>
      {/if}
    </div>

    {#if loading && runs.length === 0 && presets.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if tab === 'runs'}
      <!-- Runs timeline. -->
      {#if filteredRuns.length === 0}
        <div class="text-sm text-dim italic">
          {q.trim() ? 'no runs match your filter' : 'No agent runs yet. Run one from the granit TUI ("Agents" overlay) and it\'ll show up here.'}
        </div>
      {:else}
        <ul class="divide-y divide-surface1 bg-surface0/40 border border-surface1 rounded-lg">
          {#each filteredRuns as r (r.path)}
            <li>
              <button
                onclick={() => openRun(r)}
                class="w-full text-left px-4 py-3 hover:bg-surface0 transition-colors"
              >
                <div class="flex items-baseline gap-2 mb-1">
                  <span
                    class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium flex-shrink-0"
                    style="color: var(--color-{statusTone(r.status)}); background: color-mix(in srgb, var(--color-{statusTone(r.status)}) 14%, transparent);"
                  >{r.status}</span>
                  <span class="text-sm text-text font-medium truncate flex-1">{r.title || r.goal}</span>
                  <span class="text-[11px] text-dim font-mono flex-shrink-0">{fmtDate(r.started)}</span>
                </div>
                <div class="flex items-baseline gap-2 text-[11px] text-dim">
                  <span class="text-secondary font-mono">{r.preset || '(no preset)'}</span>
                  {#if r.steps}<span>· {r.steps} step{r.steps !== 1 ? 's' : ''}</span>{/if}
                  {#if r.model}<span>· {r.model}</span>{/if}
                  {#if r.goal && r.goal !== r.title}
                    <span class="truncate flex-1 italic">"{r.goal}"</span>
                  {/if}
                </div>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    {:else}
      <!-- Presets gallery. -->
      {#if filteredPresets.length === 0}
        <div class="text-sm text-dim italic">no presets match your filter</div>
      {:else}
        <div class="grid gap-3 grid-cols-1 sm:grid-cols-2">
          {#each filteredPresets as p (p.id)}
            {@const presetStats = stats[p.id]}
            <article class="bg-surface0 border border-surface1 rounded-lg p-4 flex flex-col gap-2">
              <header class="flex items-baseline gap-2 mb-1">
                <h3 class="text-base font-semibold text-text flex-1 truncate">{p.name}</h3>
                {#if p.includeWrite}
                  <span class="text-[10px] px-1.5 py-0.5 rounded bg-error/15 text-error" title="Can create/edit notes/tasks">writes</span>
                {/if}
                <span class="text-[10px] px-1.5 py-0.5 rounded {p.source === 'vault' ? 'bg-info/15 text-info' : 'bg-surface1 text-dim'}" title={p.source === 'vault' ? '.granit/agents/' + p.id + '.json' : 'shipped with granit'}>
                  {p.source}
                </span>
              </header>
              <p class="text-sm text-subtext leading-relaxed">{p.description}</p>

              {#if p.tools.length > 0}
                <div class="flex flex-wrap gap-1 mt-1">
                  {#each p.tools as t}
                    <span class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-surface1 text-subtext">{t}</span>
                  {/each}
                </div>
              {/if}

              {#if showPrompts && p.systemPrompt}
                <details class="mt-2">
                  <summary class="text-xs text-dim cursor-pointer hover:text-text">system prompt</summary>
                  <pre class="text-[11px] text-subtext font-mono whitespace-pre-wrap mt-1 p-2 bg-mantle border border-surface1 rounded max-h-48 overflow-auto">{p.systemPrompt}</pre>
                </details>
              {/if}

              {#if presetStats}
                <footer class="flex items-center gap-2 mt-2 pt-2 border-t border-surface1 text-[11px] text-dim">
                  <span class="text-text font-medium">{presetStats.total ?? 0}</span>
                  <span>runs</span>
                  {#if presetStats.ok}<span class="text-success">· {presetStats.ok} ok</span>{/if}
                  {#if presetStats.error}<span class="text-error">· {presetStats.error} err</span>{/if}
                  {#if presetStats.cancelled}<span class="text-dim">· {presetStats.cancelled} cancelled</span>{/if}
                </footer>
              {/if}

              <div class="flex items-center gap-2 mt-1 pt-2 border-t border-surface1">
                <button
                  onclick={() => startRun(p)}
                  class="px-3 py-1.5 text-xs bg-primary text-mantle rounded font-medium hover:opacity-90"
                >▶ Run</button>
                <code class="text-[10px] text-dim font-mono flex-1">{p.id}</code>
              </div>
            </article>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
</div>

<AgentRunPanel bind:open={runOpen} preset={runPreset} />
