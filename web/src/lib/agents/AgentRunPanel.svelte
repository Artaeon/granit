<script lang="ts">
  import { goto } from '$app/navigation';
  import { onDestroy } from 'svelte';
  import { api, type AgentPreset } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import Drawer from '$lib/components/Drawer.svelte';

  // Slide-in panel that runs an agent and streams its transcript live.
  // Open with `runner.start(preset)` from a parent (the /agents page or
  // the daily note's "Plan my day" button). Subscribes to agent.event +
  // agent.complete WS frames keyed by the runId we got from POST /run.

  let {
    open = $bindable(false),
    preset = null as AgentPreset | null
  }: {
    open?: boolean;
    preset?: AgentPreset | null;
  } = $props();

  type StreamLine = { step: number; kind: string; text: string };

  let goal = $state('');
  let runId = $state<string | null>(null);
  let lines = $state<StreamLine[]>([]);
  let finalAnswer = $state('');
  let status = $state<'idle' | 'running' | 'ok' | 'error' | 'budget' | 'cancelled'>('idle');
  let resultPath = $state<string | null>(null);
  let starting = $state(false);
  let unsub: (() => void) | null = null;

  $effect(() => {
    if (!open) return;
    // Reset on each open so the panel starts clean.
    if (status !== 'running') {
      goal = '';
      runId = null;
      lines = [];
      finalAnswer = '';
      status = 'idle';
      resultPath = null;
    }
  });

  onDestroy(() => unsub?.());

  function subscribe(rid: string) {
    unsub?.();
    unsub = onWsEvent((ev) => {
      if (ev.type === 'agent.event' && ev.id === rid) {
        lines = [...lines, { step: ev.data.step, kind: ev.data.kind, text: ev.data.text }];
      } else if (ev.type === 'agent.complete' && ev.id === rid) {
        status = (ev.data.status as typeof status) ?? 'ok';
        finalAnswer = ev.data.finalAnswer ?? '';
        resultPath = ev.path ?? null;
      }
    });
  }

  async function run() {
    if (!preset) return;
    starting = true;
    try {
      const r = await api.runAgent(preset.id, goal);
      runId = r.runId;
      status = 'running';
      lines = [];
      finalAnswer = '';
      resultPath = null;
      subscribe(r.runId);
    } catch (e) {
      toast.error('failed to start: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      starting = false;
    }
  }

  function viewTranscript() {
    if (!resultPath) return;
    open = false;
    goto(`/notes/${encodeURIComponent(resultPath)}`);
  }

  // ReAct events the runtime emits — we color them so the user can scan
  // a long transcript at a glance.
  function tone(kind: string): string {
    switch (kind) {
      case 'thought': return 'subtext';
      case 'tool_call': return 'primary';
      case 'tool_result': return 'info';
      case 'final_answer': return 'success';
      case 'error': return 'error';
      case 'prompt_sent':
      case 'response_received':
      default:
        return 'dim';
    }
  }

  function kindLabel(kind: string): string {
    return kind.replace(/_/g, ' ');
  }
</script>

<Drawer bind:open side="right">
  {#if preset}
    <div class="h-full flex flex-col overflow-hidden">
      <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <h2 class="text-base font-semibold text-text flex-1 truncate">{preset.name}</h2>
        <button onclick={() => (open = false)} aria-label="close" class="text-dim hover:text-text">×</button>
      </header>

      <div class="px-4 py-3 border-b border-surface1 flex-shrink-0">
        <p class="text-xs text-subtext">{preset.description}</p>
      </div>

      {#if status === 'idle'}
        <form
          onsubmit={(e) => { e.preventDefault(); run(); }}
          class="px-4 py-3 border-b border-surface1 space-y-2 flex-shrink-0"
        >
          <label for="goal" class="block text-[11px] uppercase tracking-wider text-dim">Goal {#if !preset.includeWrite}<span class="text-dim/70">(optional)</span>{/if}</label>
          <textarea
            id="goal"
            bind:value={goal}
            rows="3"
            placeholder={preset.id === 'plan-my-day' ? 'leave blank — preset reads today automatically' : 'what should the agent do?'}
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          ></textarea>
          <button
            type="submit"
            disabled={starting}
            class="w-full px-3 py-2.5 bg-primary text-mantle rounded text-sm font-medium disabled:opacity-50"
          >{starting ? 'starting…' : `Run ${preset.name}`}</button>
          <p class="text-[10px] text-dim italic">
            Uses your granit AI config (provider · {preset.includeWrite ? 'may write to vault' : 'read-only'}).
          </p>
        </form>
      {/if}

      <div class="flex-1 overflow-y-auto p-3 space-y-2">
        {#if status !== 'idle'}
          <div class="flex items-baseline gap-2">
            <span
              class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium"
              style="color: var(--color-{status === 'ok' ? 'success' : status === 'error' ? 'error' : status === 'budget' ? 'warning' : 'primary'}); background: color-mix(in srgb, var(--color-{status === 'ok' ? 'success' : status === 'error' ? 'error' : status === 'budget' ? 'warning' : 'primary'}) 14%, transparent);"
            >{status}</span>
            {#if status === 'running'}
              <span class="text-xs text-dim animate-pulse">running…</span>
            {/if}
            <span class="flex-1"></span>
            {#if resultPath}
              <button onclick={viewTranscript} class="text-xs text-secondary hover:underline">view transcript →</button>
            {/if}
          </div>

          {#if finalAnswer}
            <div class="rounded p-3 bg-success/10 border-l-3 border-success mt-2">
              <div class="text-[10px] uppercase tracking-wider text-success mb-1">Answer</div>
              <p class="text-sm text-text whitespace-pre-wrap">{finalAnswer}</p>
            </div>
          {/if}

          {#if lines.length > 0}
            <ol class="space-y-1.5 mt-3">
              {#each lines as ln, i (i)}
                <li class="text-xs flex gap-2">
                  <span class="font-mono text-dim flex-shrink-0 w-7">#{ln.step}</span>
                  <span
                    class="text-[10px] uppercase tracking-wider w-20 flex-shrink-0 font-medium"
                    style="color: var(--color-{tone(ln.kind)})"
                  >{kindLabel(ln.kind)}</span>
                  <span class="text-subtext flex-1 break-words whitespace-pre-wrap">{ln.text}</span>
                </li>
              {/each}
            </ol>
          {:else if status === 'running'}
            <p class="text-xs text-dim italic mt-3">waiting for the model…</p>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</Drawer>
