<script lang="ts">
  import { goto } from '$app/navigation';
  import { onDestroy } from 'svelte';
  import { api, type AgentPreset } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { classifyAiError } from '$lib/util/aiErrors';
  import Drawer from '$lib/components/Drawer.svelte';

  // Slide-in panel that runs an agent and streams its transcript live.
  // Open with `runner.start(preset)` from a parent (the /agents page or
  // the daily note's "Plan my day" button). Subscribes to agent.event +
  // agent.complete WS frames keyed by the runId we got from POST /run.

  let {
    open = $bindable(false),
    preset = null as AgentPreset | null,
    initialGoal = ''
  }: {
    open?: boolean;
    preset?: AgentPreset | null;
    /** Pre-fill the goal textarea when the panel opens. Used by the
     *  scripture page to feed the verse + source straight in so the
     *  user can hit Run without typing. */
    initialGoal?: string;
  } = $props();

  type StreamLine = { step: number; kind: string; text: string };

  // WebCitation is one extracted hit from a web_search tool_result.
  // We parse the plain-text observation the agent renders (`1. Title
  // \n URL: …\n snippet`) into a small struct so a chip strip can
  // surface the cited sources below the final answer. Mirrors the
  // Result shape on the server side, minus the snippet (we keep that
  // off the chip to stay compact — full snippet is still in the
  // transcript above for users who want to read it).
  type WebCitation = {
    step: number;
    title: string;
    url: string;
    provider?: string;
  };

  let goal = $state('');
  let runId = $state<string | null>(null);
  let lines = $state<StreamLine[]>([]);
  let finalAnswer = $state('');
  let status = $state<'idle' | 'running' | 'ok' | 'error' | 'budget' | 'cancelled'>('idle');
  let resultPath = $state<string | null>(null);
  let starting = $state(false);
  let unsub: (() => void) | null = null;

  // Fallback timer for the case where the WS hub drops our agent.complete
  // frame (slow client, queue full). After this many ms in 'running'
  // without a complete event, we fetch /agents/runs and resolve from the
  // persisted note. The runner caps its own ctx at 5 min, so 6 min is
  // a safe ceiling — anything that took longer is dead anyway.
  const FALLBACK_TIMEOUT_MS = 6 * 60 * 1000;
  let fallbackTimer: ReturnType<typeof setTimeout> | null = null;
  let runStartedAt = 0;

  // Budget + step caps. Budget is in main-currency units (€/$/etc.)
  // so users type "0.25" instead of "25_000_000". We multiply by
  // 100_000_000 for the wire format. 0 means "no cap" — only the
  // server's iteration ceiling applies. Per-preset defaults: deep-
  // research gets 0.25 + 20 steps, others stay free.
  let budgetMain = $state(0);
  let maxSteps = $state(0);
  // Cost telemetry from agent.complete. microCents == -1 (not set)
  // means the model wasn't priced (Ollama / unknown OpenAI snapshot).
  let costMicroCents = $state<number | null>(null);
  let promptTokens = $state(0);
  let completionTokens = $state(0);

  // Adjust defaults when the preset changes so the deep-research entry
  // arrives with sensible caps prefilled.
  $effect(() => {
    if (!preset) return;
    if (preset.id === 'deep-research') {
      if (budgetMain === 0) budgetMain = 0.25; // €0.25 cap
      if (maxSteps === 0) maxSteps = 20;
    }
  });

  $effect(() => {
    if (!open) return;
    // Reset on each open so the panel starts clean. Seed the goal
    // textarea with the caller-supplied initialGoal (scripture page
    // uses this to pre-fill the verse) so the user can just hit Run.
    if (status !== 'running') {
      goal = initialGoal;
      runId = null;
      lines = [];
      finalAnswer = '';
      status = 'idle';
      resultPath = null;
      costMicroCents = null;
      promptTokens = 0;
      completionTokens = 0;
    }
  });

  onDestroy(() => {
    unsub?.();
    if (fallbackTimer) clearTimeout(fallbackTimer);
  });

  function subscribe(rid: string) {
    unsub?.();
    unsub = onWsEvent((ev) => {
      if (ev.type === 'agent.event' && ev.id === rid) {
        lines = [...lines, { step: ev.data.step, kind: ev.data.kind, text: ev.data.text }];
      } else if (ev.type === 'agent.complete' && ev.id === rid) {
        status = (ev.data.status as typeof status) ?? 'ok';
        finalAnswer = ev.data.finalAnswer ?? '';
        resultPath = ev.path ?? null;
        // Cost telemetry only present when the model is priced.
        // microCents ≥ 0 ⇒ valid; absent ⇒ Ollama / unknown model.
        if (typeof ev.data.microCents === 'number') {
          costMicroCents = ev.data.microCents;
          promptTokens = ev.data.promptTokens ?? 0;
          completionTokens = ev.data.completionTokens ?? 0;
        }
        if (fallbackTimer) {
          clearTimeout(fallbackTimer);
          fallbackTimer = null;
        }
      }
    });
  }

  // Fallback resolver. Fires when the WS complete frame never arrived.
  // Walks /agents/runs (newest first) for a record whose preset matches
  // and whose started timestamp is within a short window of when we
  // POSTed /run — close enough to be the same run without needing the
  // server to expose runId in the persisted note.
  async function resolveFromPersisted() {
    if (!preset || !runStartedAt) return;
    if (status !== 'running') return; // already resolved
    try {
      const r = await api.listAgentRuns(50);
      const presetID = preset.id;
      const startMs = runStartedAt;
      // Server writes started in RFC3339; the run's ISO timestamp will
      // be within a few seconds of when we POSTed /run. ±60s window
      // tolerates clock skew between the device + server.
      const match = r.runs.find((run) => {
        if (run.preset !== presetID) return false;
        const t = Date.parse(run.started);
        if (Number.isNaN(t)) return false;
        return Math.abs(t - startMs) <= 60_000;
      });
      if (!match) {
        status = 'error';
        finalAnswer = 'lost connection to the run — refresh /agents to find the transcript';
        return;
      }
      // Persisted note exists ⇒ run finished. Use its frontmatter
      // status; the body has the answer + transcript for follow-up.
      const s = match.status || 'ok';
      status = (s === 'budget' || s === 'error' || s === 'ok' ? s : 'ok') as typeof status;
      resultPath = match.path;
      finalAnswer = `Run finished — open transcript for the full answer (${match.steps} steps)`;
    } catch {
      status = 'error';
      finalAnswer = 'lost connection — open /agents to find the transcript';
    } finally {
      if (fallbackTimer) {
        clearTimeout(fallbackTimer);
        fallbackTimer = null;
      }
    }
  }

  async function run() {
    if (!preset) return;
    starting = true;
    try {
      // Main units → micro-cents (×100_000_000). 0 stays 0 ⇒ "no budget".
      const budgetMicroCents = budgetMain > 0 ? Math.round(budgetMain * 100_000_000) : 0;
      const r = await api.runAgent(preset.id, goal, {
        maxSteps: maxSteps > 0 ? maxSteps : undefined,
        budgetMicroCents: budgetMicroCents > 0 ? budgetMicroCents : undefined
      });
      runId = r.runId;
      status = 'running';
      lines = [];
      finalAnswer = '';
      resultPath = null;
      costMicroCents = null;
      promptTokens = 0;
      completionTokens = 0;
      runStartedAt = Date.now();
      subscribe(r.runId);
      // Arm the fallback. Cleared by the WS complete handler; only fires
      // if the WS frame got dropped on a slow client.
      if (fallbackTimer) clearTimeout(fallbackTimer);
      fallbackTimer = setTimeout(resolveFromPersisted, FALLBACK_TIMEOUT_MS);
    } catch (e) {
      // Map provider-specific noise (missing model, refused dial, bad
      // key) to a one-line headline + Open-Settings CTA. The raw error
      // is still available behind the toast's "details" expand and on
      // the console, so power users debugging a custom config can see
      // exactly what the provider returned.
      const raw = e instanceof Error ? e.message : String(e);
      console.error('[runAgent] failed:', raw);
      const hint = classifyAiError(raw);
      toast.error(hint.headline, { action: hint.cta, details: hint.raw });
    } finally {
      starting = false;
    }
  }

  function formatCost(microCents: number | null): string {
    if (microCents === null || microCents < 0) return '—';
    // 1 micro-cent = 1/1_000_000 of a cent ⇒ 100_000_000 mc = 1.00 main unit.
    const main = microCents / 100_000_000;
    if (main > 0 && main < 0.0001) return '<0.0001';
    return main.toFixed(4).replace(/0+$/, '').replace(/\.$/, '');
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

  // parseWebSearchResult turns the agent's plain-text observation
  // (emitted by tools_web.go's renderWebHits) into a list of
  // citations. We pair tool_call events naming "web_search" with the
  // tool_result that follows in the same step, so an unrelated
  // tool_result in the same step (which shouldn't normally happen,
  // but the parser stays robust) doesn't get misclassified.
  //
  // Format we parse:
  //
  //   1. Title text
  //      URL: https://example.com
  //      snippet line
  //
  //   2. Other title
  //      URL: https://other.example.com
  //      another snippet
  //
  //   (source: duckduckgo · 2 results)
  function parseWebSearchResult(text: string, step: number): WebCitation[] {
    const out: WebCitation[] = [];
    if (!text) return out;
    // Capture an optional trailing provider line.
    let provider: string | undefined;
    const sourceMatch = text.match(/\(source:\s*([^·)]+)/);
    if (sourceMatch) provider = sourceMatch[1].trim();
    // Hits are blocks separated by a blank line. Walk line-by-line so
    // the same regex handles 1 result or 10 without backtracking
    // through the whole string.
    let current: { title?: string; url?: string } | null = null;
    for (const raw of text.split('\n')) {
      const line = raw.trim();
      const heading = line.match(/^\d+\.\s+(.+)$/);
      if (heading) {
        if (current?.url) out.push({ step, title: current.title ?? current.url, url: current.url, provider });
        current = { title: heading[1].trim() };
        continue;
      }
      const urlMatch = line.match(/^URL:\s*(https?:\/\/\S+)/i);
      if (urlMatch && current) {
        current.url = urlMatch[1];
        continue;
      }
      // Title line where the heading itself is a URL (renderer
      // falls back to URL when title is empty).
      if (current && !current.url) {
        const bareURL = (current.title ?? '').match(/^(https?:\/\/\S+)/i);
        if (bareURL) current.url = bareURL[1];
      }
    }
    if (current?.url) out.push({ step, title: current.title ?? current.url, url: current.url, provider });
    return out;
  }

  // Walk the live event stream and pair `web_search` tool_call events
  // with the tool_result that follows them in the same step. The
  // resulting flat list backs the citations chip strip below the
  // final answer. Re-derives on every new line so an in-progress run
  // shows chips the instant the result lands.
  let webCitations = $derived.by(() => {
    const out: WebCitation[] = [];
    for (let i = 0; i < lines.length; i++) {
      const ln = lines[i];
      // A web_search call is rendered like:
      //   web_search(query="foo", limit="5")
      // Match the prefix instead of the full string so future args
      // (region, safesearch) don't break the detector.
      if (ln.kind !== 'tool_call') continue;
      if (!ln.text.trimStart().startsWith('web_search(')) continue;
      // Look forward for the result in the same step.
      for (let j = i + 1; j < lines.length; j++) {
        if (lines[j].step !== ln.step) break;
        if (lines[j].kind !== 'tool_result') continue;
        out.push(...parseWebSearchResult(lines[j].text, ln.step));
        break;
      }
    }
    return out;
  });

  // Derive a stable favicon URL for a citation chip. Google's
  // public favicon endpoint is the lowest-effort path that works
  // for ~every public site without us shipping a favicon scraper.
  // Falls back to a transparent SVG dot inside the chip when the
  // request fails (browser surfaces a broken-image icon otherwise).
  function faviconURL(u: string): string {
    try {
      const host = new URL(u).host;
      return `https://www.google.com/s2/favicons?sz=32&domain=${encodeURIComponent(host)}`;
    } catch {
      return '';
    }
  }

  // Truncate the hostname for chip rendering so a long subdomain
  // doesn't push the chip wider than the drawer.
  function chipHost(u: string): string {
    try {
      const h = new URL(u).host.replace(/^www\./, '');
      if (h.length > 28) return h.slice(0, 28) + '…';
      return h;
    } catch {
      return u;
    }
  }
</script>

<Drawer bind:open side="right" responsive width="w-full sm:w-96 md:w-[32rem] lg:w-[36rem]">
  {#if preset}
    <div class="h-full flex flex-col overflow-hidden">
      <header class="px-3 py-2 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <h2 class="text-base font-semibold text-text flex-1 truncate">{preset.name}</h2>
        <button
          onclick={() => (open = false)}
          aria-label="close"
          class="w-9 h-9 -mr-2 flex items-center justify-center text-dim hover:text-text hover:bg-surface0 rounded text-xl leading-none"
        >×</button>
      </header>

      <div class="px-3 py-2 border-b border-surface1 flex-shrink-0">
        <p class="text-xs text-subtext">{preset.description}</p>
      </div>

      {#if status === 'idle'}
        <form
          onsubmit={(e) => { e.preventDefault(); run(); }}
          class="px-3 py-2 border-b border-surface1 space-y-2 flex-shrink-0"
        >
          <label for="goal" class="block text-[11px] uppercase tracking-wider text-dim">Goal {#if !preset.includeWrite}<span class="text-dim/70">(optional)</span>{/if}</label>
          <textarea
            id="goal"
            bind:value={goal}
            rows="3"
            placeholder={preset.id === 'plan-my-day' ? 'leave blank — preset reads today automatically' : 'what should the agent do?'}
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          ></textarea>
          <div class="grid grid-cols-2 gap-2">
            <div>
              <label for="budget" class="block text-[11px] uppercase tracking-wider text-dim mb-1">
                Budget <span class="text-dim/70 lowercase normal-case">(€/$ — 0 = no cap)</span>
              </label>
              <input
                id="budget"
                type="number"
                min="0"
                step="0.05"
                bind:value={budgetMain}
                class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
              />
            </div>
            <div>
              <label for="maxsteps" class="block text-[11px] uppercase tracking-wider text-dim mb-1">
                Max steps <span class="text-dim/70 lowercase normal-case">(0 = preset default)</span>
              </label>
              <input
                id="maxsteps"
                type="number"
                min="0"
                max="50"
                step="1"
                bind:value={maxSteps}
                class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
              />
            </div>
          </div>
          <button
            type="submit"
            disabled={starting}
            class="w-full px-3 py-2.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
          >{starting ? 'starting…' : `Run ${preset.name}`}</button>
          <p class="text-[10px] text-dim italic">
            Uses your granit AI config (provider · {preset.includeWrite ? 'may write to vault' : 'read-only'}).
            Budget is enforced for OpenAI runs only — Ollama is free.
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
              <button
                onclick={viewTranscript}
                class="text-xs text-secondary hover:underline px-2 py-1 -mr-1 rounded hover:bg-surface1"
              >view transcript →</button>
            {/if}
          </div>

          {#if costMicroCents !== null && costMicroCents >= 0}
            <div class="flex items-baseline gap-3 text-[11px] text-dim mt-1">
              <span>Tokens <span class="text-subtext font-medium">{promptTokens}</span> in / <span class="text-subtext font-medium">{completionTokens}</span> out</span>
              <span>·</span>
              <span>Cost <span class="text-subtext font-medium">{formatCost(costMicroCents)}</span></span>
            </div>
          {/if}

          {#if finalAnswer}
            <div class="rounded p-3 bg-surface0 border-l-3 border-success mt-2">
              <div class="text-[10px] uppercase tracking-wider text-success mb-1">Answer</div>
              <p class="text-sm text-text whitespace-pre-wrap">{finalAnswer}</p>
            </div>
          {/if}

          {#if webCitations.length > 0}
            <!-- Web citations chip strip. Surfaces every URL the agent
                 pulled in via web_search so the user can spot-check
                 sources before trusting the answer. One chip per hit;
                 favicons load from Google's S2 endpoint (free, public,
                 no key needed). The strip is dedup'd on URL so two
                 web_search calls returning the same hit don't show
                 twice. -->
            {@const seen = new Set<string>()}
            {@const unique = webCitations.filter((c) => {
              if (seen.has(c.url)) return false;
              seen.add(c.url);
              return true;
            })}
            <div class="mt-2 pt-2 border-t border-surface1">
              <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5">
                Sources <span class="normal-case lowercase">— from: {unique.length} {unique.length === 1 ? 'source' : 'sources'}</span>
              </div>
              <div class="flex flex-wrap gap-1.5">
                {#each unique as c (c.url)}
                  <a
                    href={c.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    title={c.title || c.url}
                    class="inline-flex items-center gap-1.5 px-2 py-1 bg-surface0 border border-surface1 hover:border-primary rounded text-[11px] text-subtext hover:text-primary transition-colors"
                  >
                    {#if faviconURL(c.url)}
                      <img
                        src={faviconURL(c.url)}
                        alt=""
                        width="14"
                        height="14"
                        loading="lazy"
                        class="w-3.5 h-3.5 rounded-sm flex-shrink-0"
                        onerror={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden'; }}
                      />
                    {/if}
                    <span class="truncate max-w-[180px]">{chipHost(c.url)}</span>
                  </a>
                {/each}
              </div>
            </div>
          {/if}

          {#if lines.length > 0}
            <ol class="space-y-2 mt-3">
              {#each lines as ln, i (i)}
                <li class="text-xs">
                  <!-- Header line: step + kind badge. Wrapping to its
                       own row keeps long event text fully readable on
                       phones (the previous side-by-side layout
                       squeezed the text column to ~140px on narrow
                       drawers). -->
                  <div class="flex items-center gap-2 mb-0.5">
                    <span class="font-mono text-dim flex-shrink-0">#{ln.step}</span>
                    <span
                      class="text-[10px] uppercase tracking-wider font-medium"
                      style="color: var(--color-{tone(ln.kind)})"
                    >{kindLabel(ln.kind)}</span>
                  </div>
                  <p class="text-subtext break-words whitespace-pre-wrap pl-6 border-l border-surface1 ml-1.5">{ln.text}</p>
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
