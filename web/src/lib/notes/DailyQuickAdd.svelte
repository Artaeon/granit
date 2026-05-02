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

  // Plan-my-day shortcut. Looks up the preset on demand (cheap — local
  // catalog lives in localStorage cache after the first fetch) so the
  // composer stays light when the user doesn't open it.
  let planOpen = $state(false);
  let planPreset = $state<AgentPreset | null>(null);
  let planLoading = $state(false);

  async function openPlanMyDay() {
    if (planPreset) {
      planOpen = true;
      return;
    }
    planLoading = true;
    try {
      const r = await api.listAgentPresets();
      const p = r.presets.find((p) => p.id === 'plan-my-day');
      if (!p) {
        toast.error('plan-my-day preset not found — vault override may have shadowed it');
        return;
      }
      planPreset = p;
      planOpen = true;
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      planLoading = false;
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

    <!-- Plan-my-day fires the AI agent that reads today's calendar +
         tasks + projects and writes a ## Plan section to this note. -->
    <button
      type="button"
      onclick={openPlanMyDay}
      disabled={planLoading}
      class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-primary disabled:opacity-50 hidden sm:inline-flex items-center gap-1"
      title="Use AI to draft today's schedule"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/><path d="M5 5l2 2M19 5l-2 2"/>
      </svg>
      {planLoading ? '…' : 'Plan my day'}
    </button>
  </form>
</div>

<AgentRunPanel bind:open={planOpen} preset={planPreset} />
