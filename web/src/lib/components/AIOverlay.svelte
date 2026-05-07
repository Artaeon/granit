<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api, type ChatMessage } from '$lib/api';
  import { sabbath } from '$lib/stores/sabbath';
  import { toast } from '$lib/components/toast';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  // AIOverlay — global AI panel. Slides in from the right on
  // desktop, becomes a bottom sheet on mobile. Triggered with
  // Mod+J from anywhere (and Esc to close). The body is split
  // into two modes:
  //   1. Quick actions  — four buttons that dispatch the existing
  //      Tier 1 features (briefing / triage / deadlines /
  //      synopsis). Result renders inline as markdown or a JSON
  //      block.
  //   2. Chat — a streaming conversation against the configured
  //      LLM via /api/v1/chat/stream. History is in-memory only
  //      so the overlay stays disposable; the dedicated /chat page
  //      is the place for long-running threads + saving.
  //
  // The component listens for its own keyboard shortcut so the
  // layout doesn't have to know it exists — drop a single
  // <AIOverlay /> in +layout.svelte and you're done.

  let open = $state(false);
  let panelEl: HTMLDivElement | undefined = $state();
  let inputEl: HTMLTextAreaElement | undefined = $state();
  let scrollEl: HTMLDivElement | undefined = $state();

  let busy = $state(false);
  let abort: AbortController | null = null;

  // Status pill — what model the chat / actions will route to.
  let statusInfo = $state<{ provider: string; model: string; sabbath: boolean } | null>(null);

  // Quick-action result. Cleared every time the user fires a new
  // action OR sends a chat message (chat takes over the body).
  let quickTitle = $state('');
  let quickResult = $state('');

  // Chat history — local to the overlay, not persisted. The full
  // /chat page handles persistence + save-as-note for serious
  // threads; this is a quick-question surface.
  let messages = $state<ChatMessage[]>([]);
  let input = $state('');

  function close() {
    abort?.abort();
    open = false;
  }
  function toggle() {
    open = !open;
    if (open) {
      void loadStatus();
      tick().then(() => inputEl?.focus());
    }
  }

  // Global Mod+J shortcut + Esc to close. We don't fire when the
  // user is typing in another input — a writer hitting Mod+J in
  // their text editor probably means "newline literal", not "open
  // AI." (Mod+J is a terminal/Vim convention for line-join, but
  // we're inside a webapp and the conflict is rare; the input
  // exclusion handles the common case of code editors etc.)
  function onKey(e: KeyboardEvent) {
    if (open && e.key === 'Escape') {
      e.preventDefault();
      close();
      return;
    }
    if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'j') {
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) {
        // Inside our own panel? Toggle anyway, otherwise let the
        // caret keep its keystroke.
        if (panelEl && t.closest('[data-ai-overlay]')) {
          // Allow.
        } else {
          return;
        }
      }
      e.preventDefault();
      toggle();
    }
  }

  onMount(() => {
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  async function loadStatus() {
    try {
      const s = await api.getAIStatus();
      statusInfo = {
        provider: s.global_provider,
        model: s.global_model,
        sabbath: !!s.sabbath_active
      };
    } catch {
      statusInfo = null;
    }
  }

  // ── Quick actions ──────────────────────────────────────────────
  // Each one: cancel any in-flight call, fire the API, render
  // markdown (briefing / synopsis) or a JSON block of proposals
  // (triage / deadlines). Proposals are NOT applied from here —
  // the dedicated tasks page is the place for that flow because
  // it has the full task context. The overlay just shows the
  // model's suggestions so the user can decide whether to navigate
  // there. Keeps the overlay simple.
  async function runBriefing() {
    await runQuick('Daily briefing', async (s) => {
      const r = await api.aiDailyBriefing(s);
      return r.markdown;
    });
  }
  async function runSynopsis() {
    await runQuick('Weekly synopsis', async (s) => {
      const r = await api.aiWeeklyReview(s);
      return r.markdown;
    });
  }
  async function runTriage() {
    await runQuick('Inbox triage', async (s) => {
      const r = await api.aiInboxTriage(s);
      const props = r.proposals ?? [];
      if (props.length === 0) return '_No untriaged tasks to review._';
      const lines = props.map(
        (p) =>
          `- **${p.priority === 0 ? 'drop' : `P${p.priority}`}** · ${p.schedule} · ${p.rationale} _(${p.id})_`
      );
      return `${lines.length} suggestion${lines.length === 1 ? '' : 's'} — open /tasks → inbox to apply:\n\n${lines.join('\n')}`;
    });
  }
  async function runDeadlines() {
    await runQuick('Detect deadlines', async (s) => {
      const r = await api.aiDeadlineDetect(s);
      const props = r.proposals ?? [];
      if (props.length === 0) return '_No clear deadlines detected._';
      const lines = props.map((p) => `- **${p.due_date}** · ${p.rationale} _(${p.id})_`);
      return `${lines.length} deadline${lines.length === 1 ? '' : 's'} detected — open /tasks → inbox to apply:\n\n${lines.join('\n')}`;
    });
  }

  async function runQuick(title: string, fn: (signal: AbortSignal) => Promise<string>) {
    if (busy) return;
    abort?.abort();
    abort = new AbortController();
    busy = true;
    quickTitle = title;
    quickResult = '_running…_';
    messages = []; // chat clears when a quick action runs
    try {
      quickResult = await fn(abort.signal);
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        quickResult = '_cancelled_';
      } else {
        const msg = err instanceof Error ? err.message : String(err);
        quickResult = /disabled in AI preferences/i.test(msg)
          ? `_${msg}_  \n\n[Open settings →](/settings)`
          : `_failed:_ ${msg}`;
      }
    } finally {
      busy = false;
      abort = null;
    }
  }

  // ── Chat ──────────────────────────────────────────────────────
  // Streaming via /api/v1/chat/stream so the user sees tokens
  // arriving — important on slow local LLMs where a 30s wait
  // with no signal feels broken. Cancel button aborts mid-stream.
  async function send(e?: Event) {
    e?.preventDefault();
    const text = input.trim();
    if (!text || busy) return;
    quickTitle = '';
    quickResult = '';
    busy = true;
    abort?.abort();
    abort = new AbortController();
    const userMsg: ChatMessage = { role: 'user', content: text };
    const history = [...messages, userMsg];
    messages = [...history, { role: 'assistant', content: '' }];
    input = '';
    let acc = '';
    const idx = messages.length - 1;
    try {
      await api.chatStream(
        history,
        undefined,
        {
          onChunk: (c) => {
            acc += c;
            // Reassign through map so $state picks up the change.
            messages = messages.map((m, i) => (i === idx ? { ...m, content: acc } : m));
          },
          onError: (err) => {
            messages = messages.map((m, i) =>
              i === idx ? { ...m, content: `_error:_ ${err.message}` } : m
            );
          }
        },
        abort.signal
      );
    } finally {
      busy = false;
      abort = null;
      tick().then(() => {
        if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
      });
    }
  }

  function cancelInflight() {
    abort?.abort();
  }

  function clearChat() {
    if (messages.length === 0) return;
    messages = [];
    quickTitle = '';
    quickResult = '';
  }

  $effect(() => {
    void messages.length;
    void quickResult;
    tick().then(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  });

  function onInputKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      void send();
    }
  }
</script>

{#if open}
  <!-- Backdrop. Click-to-close on mobile; on desktop the panel
       sits next to content rather than over it, so the backdrop
       is hidden by md:hidden — desktop users dismiss with Esc or
       the close button. -->
  <button
    type="button"
    aria-label="close AI overlay"
    onclick={close}
    class="md:hidden fixed inset-0 z-40 bg-black/40 backdrop-blur-sm"
  ></button>

  <div
    bind:this={panelEl}
    data-ai-overlay
    role="dialog"
    aria-label="AI assistant"
    class="fixed z-50 flex flex-col bg-base border-surface1 shadow-2xl
           inset-x-0 bottom-0 max-h-[85vh] rounded-t-xl border-t
           md:inset-y-0 md:right-0 md:left-auto md:bottom-auto md:top-0 md:h-full md:w-[420px] md:max-h-none md:rounded-none md:border-l md:border-t-0"
  >
    <!-- Header. Mobile gets a drag-handle visual hint at the very
         top; both layouts get title + status pill + close. -->
    <div class="md:hidden flex justify-center pt-2 pb-1">
      <span class="block w-10 h-1 rounded-full bg-surface2"></span>
    </div>
    <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
      <span class="text-base">✨</span>
      <h2 class="text-sm font-semibold text-text">AI assistant</h2>
      {#if statusInfo}
        <span
          class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-surface1 text-subtext truncate"
          title="Default backend (per-feature overrides apply individually)"
        >{statusInfo.provider} · {statusInfo.model}</span>
      {/if}
      <span class="flex-1"></span>
      {#if busy}
        <button
          onclick={cancelInflight}
          class="px-2 py-1 text-[11px] text-warning hover:underline"
          title="Cancel the in-flight request"
        >cancel</button>
      {/if}
      <button
        onclick={close}
        aria-label="close"
        class="text-dim hover:text-text px-2 py-1 text-lg leading-none"
      >×</button>
    </header>

    {#if statusInfo?.sabbath || $sabbath}
      <div class="mx-4 mt-3 px-3 py-2 text-[11px] bg-warning/10 border border-warning/30 rounded text-warning">
        🕯️ Sabbath mode — AI requests are paused today.
      </div>
    {/if}

    <!-- Quick actions row. Wraps on small viewports so it never
         pushes the body off-screen. -->
    <div class="px-4 py-3 border-b border-surface1 flex flex-wrap gap-1.5 flex-shrink-0">
      <button
        onclick={runBriefing}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Briefing</button>
      <button
        onclick={runSynopsis}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Weekly synopsis</button>
      <button
        onclick={runTriage}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Triage</button>
      <button
        onclick={runDeadlines}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Deadlines</button>
      <span class="flex-1"></span>
      {#if messages.length > 0 || quickResult}
        <button
          onclick={clearChat}
          class="px-2 py-1 text-[11px] text-dim hover:text-error"
          title="Clear the overlay"
        >clear</button>
      {/if}
    </div>

    <!-- Body — quick-action result OR chat thread. Mutually
         exclusive: firing a quick action clears the chat, sending
         a chat message clears the quick result. Keeps the overlay
         single-purpose at any moment. -->
    <div bind:this={scrollEl} class="flex-1 overflow-y-auto px-4 py-3">
      {#if quickResult}
        <div class="text-[10px] uppercase tracking-wider text-secondary mb-2">{quickTitle}</div>
        <div class="prose prose-sm max-w-none">
          <MarkdownRenderer body={quickResult} />
        </div>
      {:else if messages.length > 0}
        <ul class="space-y-3">
          {#each messages as m, i (i)}
            <li>
              <div class="text-[10px] uppercase tracking-wider {m.role === 'user' ? 'text-secondary' : 'text-primary'} mb-0.5">
                {m.role === 'user' ? 'you' : 'assistant'}
              </div>
              {#if m.role === 'user'}
                <div class="text-sm text-text whitespace-pre-wrap">{m.content}</div>
              {:else}
                <div class="prose prose-sm max-w-none">
                  <MarkdownRenderer body={m.content || '_…_'} />
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {:else}
        <div class="text-xs text-dim leading-relaxed">
          <p class="mb-2">Quick actions above run the configured AI features. Or type a question below.</p>
          <p class="text-[11px]">Press <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">Mod+J</kbd> anywhere to open this. <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">Esc</kbd> to close.</p>
        </div>
      {/if}
    </div>

    <!-- Chat input. Sits at the bottom, growable up to a few rows.
         Enter sends, Shift+Enter inserts a newline. Disabled
         during Sabbath since the request would just be refused. -->
    <form
      onsubmit={send}
      class="border-t border-surface1 px-4 py-3 flex items-end gap-2 flex-shrink-0"
    >
      <textarea
        bind:this={inputEl}
        bind:value={input}
        onkeydown={onInputKey}
        rows="2"
        placeholder={$sabbath ? 'Sabbath active — AI paused' : 'Ask anything…'}
        disabled={busy || $sabbath}
        class="flex-1 bg-surface0 border border-surface1 rounded px-3 py-2 text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-none disabled:opacity-60"
      ></textarea>
      <button
        type="submit"
        disabled={busy || !input.trim() || $sabbath}
        class="px-3 py-2 text-sm bg-primary text-on-primary rounded font-medium disabled:opacity-40"
      >Send</button>
    </form>
  </div>
{/if}
