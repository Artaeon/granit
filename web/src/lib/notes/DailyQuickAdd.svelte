<script lang="ts">
  import { api, type AgentPreset } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import AgentRunPanel from '$lib/agents/AgentRunPanel.svelte';

  // DailyQuickAdd is a slim composer that lives at the top of every
  // daily note page. Single input + Enter creates a task in this note's
  // ## Tasks section; the user never has to leave the page or open a
  // modal for the most common write of the day.
  //
  // Parses TUI shorthand inline:
  //   "review PR !1 due:2026-05-03 #work" → priority 1, due date, tag

  let {
    notePath,
    dailyDate,
    onAdded
  }: {
    notePath: string;
    dailyDate: string | null;
    onAdded: () => void | Promise<void>;
  } = $props();

  let kind = $state<'task' | 'event'>('task');
  let text = $state('');
  let busy = $state(false);

  // Daily AI shortcuts. Plan-my-day, Summarize today, Reflect on today
  // — three presets that all read+write the daily note. Looked up on
  // demand and cached in agentCache so subsequent opens skip the
  // /agents/presets round-trip.
  let panelOpen = $state(false);
  let panelPreset = $state<AgentPreset | null>(null);
  let panelLoading = $state<string | null>(null); // preset id while fetching
  const agentCache = new Map<string, AgentPreset>();

  async function openPreset(id: 'plan-my-day' | 'summarize-day' | 'reflect-on-day') {
    const cached = agentCache.get(id);
    if (cached) {
      panelPreset = cached;
      panelOpen = true;
      return;
    }
    panelLoading = id;
    try {
      const r = await api.listAgentPresets();
      const p = r.presets.find((p) => p.id === id);
      if (!p) {
        toast.error(`${id} preset not found — vault override may have shadowed it`);
        return;
      }
      agentCache.set(id, p);
      panelPreset = p;
      panelOpen = true;
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      panelLoading = null;
    }
  }

  // Event-only buffers (only revealed when kind === 'event')
  let evStart = $state('09:00');
  let evEnd = $state('10:00');

  function parseShorthand(raw: string): { text: string; priority?: number; due?: string; tags?: string[] } {
    let t = raw.trim();
    let priority: number | undefined;
    let due: string | undefined;
    const tags: string[] = [];

    // !1 / !2 / !3
    t = t.replace(/(?:^|\s)!([1-3])(?=\s|$)/g, (_, p) => { priority = Number(p); return ''; }).trim();
    // due:YYYY-MM-DD
    t = t.replace(/(?:^|\s)due:(\d{4}-\d{2}-\d{2})(?=\s|$)/g, (_, d) => { due = d; return ''; }).trim();
    // #tag
    t = t.replace(/(?:^|\s)#([A-Za-z0-9_/-]+)(?=\s|$)/g, (_, tag) => { tags.push(tag); return ''; }).trim();

    return { text: t, priority, due, tags: tags.length ? tags : undefined };
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    busy = true;
    try {
      if (kind === 'task') {
        const { text: clean, priority, due, tags } = parseShorthand(text);
        if (!clean) return;
        await api.createTask({
          notePath,
          text: clean,
          priority,
          dueDate: due,
          tags,
          section: '## Tasks'
        });
        text = '';
      } else {
        if (!dailyDate) return;
        await api.createEvent({
          title: text.trim(),
          date: dailyDate,
          start_time: evStart,
          end_time: evEnd,
          color: 'cyan'
        });
        text = '';
      }
      await onAdded();
      toast.success(kind === 'task' ? 'task added' : 'event added');
    } catch (err) {
      toast.error('failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      busy = false;
    }
  }
</script>

<div class="px-3 py-2 border-b border-surface1 bg-surface0/40 flex-shrink-0">
  <form onsubmit={submit} class="flex items-center gap-2 max-w-3xl mx-auto">
    <!-- Kind toggle. Compact two-state pill — task vs event. -->
    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-[11px] flex-shrink-0">
      <button
        type="button"
        onclick={() => (kind = 'task')}
        class="px-2 py-1.5 {kind === 'task' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
      >task</button>
      <button
        type="button"
        onclick={() => (kind = 'event')}
        class="px-2 py-1.5 {kind === 'event' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
      >event</button>
    </div>

    <input
      bind:value={text}
      placeholder={kind === 'task' ? '+ task   (try: review PR !1 due:2026-05-08 #work)' : '+ event title'}
      class="flex-1 min-w-0 px-2.5 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
    />

    {#if kind === 'event'}
      <input type="time" bind:value={evStart} class="w-22 px-2 py-1.5 bg-mantle border border-surface1 rounded text-xs text-text" />
      <span class="text-dim text-xs">–</span>
      <input type="time" bind:value={evEnd} class="w-22 px-2 py-1.5 bg-mantle border border-surface1 rounded text-xs text-text" />
    {/if}

    <button
      type="submit"
      disabled={busy || !text.trim()}
      class="px-3 py-1.5 text-xs bg-primary text-mantle rounded font-medium disabled:opacity-50"
    >{busy ? '…' : 'add'}</button>

    <!-- Daily AI band. Three presets that read this note + write back
         appended sections (## Plan / ## Summary / ## Reflection).
         Hidden on phone widths to keep the input bar uncluttered;
         visible from sm: up. -->
    <div class="hidden sm:flex items-center gap-1">
      <button
        type="button"
        onclick={() => openPreset('plan-my-day')}
        disabled={panelLoading !== null}
        class="px-2.5 py-1.5 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-primary disabled:opacity-50 inline-flex items-center gap-1"
        title="Use AI to draft today's schedule"
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>
        </svg>
        {panelLoading === 'plan-my-day' ? '…' : 'Plan'}
      </button>
      <button
        type="button"
        onclick={() => openPreset('summarize-day')}
        disabled={panelLoading !== null}
        class="px-2.5 py-1.5 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-secondary hover:text-secondary disabled:opacity-50 inline-flex items-center gap-1"
        title="Recap today's done tasks + jots into a ## Summary section"
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <path d="M4 6h16M4 12h10M4 18h16"/>
        </svg>
        {panelLoading === 'summarize-day' ? '…' : 'Summarize'}
      </button>
      <button
        type="button"
        onclick={() => openPreset('reflect-on-day')}
        disabled={panelLoading !== null}
        class="px-2.5 py-1.5 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-warning hover:text-warning disabled:opacity-50 inline-flex items-center gap-1"
        title="Write a thoughtful ## Reflection on today"
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <path d="M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"/>
          <circle cx="12" cy="12" r="3"/>
        </svg>
        {panelLoading === 'reflect-on-day' ? '…' : 'Reflect'}
      </button>
    </div>
  </form>
</div>

<AgentRunPanel bind:open={panelOpen} preset={panelPreset} />
