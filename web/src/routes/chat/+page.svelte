<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api, type ChatMessage , todayISO } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { classifyAiError } from '$lib/util/aiErrors';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import { loadStored, saveStored } from '$lib/util/storage';
  import {
    AGENT_MODES,
    GENERIC_MODES,
    PERSONAS,
    findMode,
    loadModeId,
    persistModeId
  } from '$lib/ai/agents';

  // Multi-turn chat with the configured LLM. History lives in localStorage
  // (one current conversation; the user "saves" via "save as note" to keep
  // a long-term record). Server is stateless — we ship the whole history
  // every turn.

  const STORAGE_KEY = 'granit.chat.current';
  type Stored = { messages: ChatMessage[]; updatedAt: number; modeId?: string };

  let messages = $state<ChatMessage[]>([]);
  let input = $state('');
  let busy = $state(false);
  let scrollEl: HTMLDivElement | undefined = $state();
  let inputEl: HTMLTextAreaElement | undefined = $state();

  // Mode (posture / persona) — same catalogue + storage as AIOverlay
  // so a conversation started in the sidebar can be continued in the
  // full page with the same mode applied. Persisted to localStorage
  // via persistModeId so the choice survives reloads.
  let modeId = $state<string>(loadModeId());
  let mode = $derived(findMode(modeId));
  // Sticky mode picker — collapsed-by-default chip strip that the
  // user expands when they want to switch. Mirrors the AIOverlay's
  // compact mode-pill UX (no full picker dialog on a 320px phone).
  let modePickerOpen = $state(false);

  // Restore the in-progress conversation on first paint so a refresh
  // doesn't lose context. Save after every message exchange.
  onMount(() => {
    const stored = loadStored<Stored | null>(STORAGE_KEY, null);
    if (stored && Array.isArray(stored.messages)) messages = stored.messages;
    if (stored?.modeId) modeId = stored.modeId;
    inputEl?.focus();
  });

  $effect(() => {
    void messages;
    void modeId;
    saveStored<Stored>(STORAGE_KEY, { messages, modeId, updatedAt: Date.now() });
  });

  // When the user changes mode, persist for the next page-load (so
  // AIOverlay sees it too — same store key).
  function pickMode(id: string) {
    modeId = id;
    persistModeId(id);
    modePickerOpen = false;
    inputEl?.focus();
  }

  // Auto-scroll to bottom on new messages.
  $effect(() => {
    void messages.length;
    void busy;
    tick().then(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  });

  async function send(e?: Event) {
    e?.preventDefault();
    const text = input.trim();
    if (!text || busy) return;
    busy = true;
    const userMsg: ChatMessage = { role: 'user', content: text };
    messages = [...messages, userMsg];
    input = '';
    try {
      // Prepend the mode's system prompt on every turn (matches the
      // AIOverlay's pattern). The server is stateless and we ship the
      // whole history each request, so we add it fresh each time —
      // letting the user change mode mid-conversation simply switches
      // the posture for subsequent turns without contaminating prior
      // ones with conflicting system messages.
      const sys: ChatMessage = { role: 'system', content: mode.system };
      const r = await api.chat([sys, ...messages]);
      messages = [...messages, r.message];
    } catch (err) {
      // Surface the server error inline so the user can see what went
      // wrong (e.g. "no API key set"). Drop the user's message back into
      // the input so they can retry without re-typing. The classifier
      // turns raw "ollama: 404 …" noise into a one-line headline plus
      // an Open-Settings CTA; raw is still available behind "details".
      const msg = errorMessage(err);
      console.error('[chat] send failed:', msg);
      const hint = classifyAiError(msg);
      toast.error(hint.headline, { action: hint.cta, details: hint.raw });
      messages = messages.slice(0, -1);
      input = text;
    } finally {
      busy = false;
      tick().then(() => inputEl?.focus());
    }
  }

  function reset() {
    if (messages.length > 0 && !confirm('Clear the current conversation?')) return;
    messages = [];
    input = '';
  }

  // Save the conversation as a markdown note so the user has a permanent
  // record. Uses /api/v1/notes — the same path the editor uses.
  async function saveAsNote() {
    if (messages.length === 0) return;
    const stamp = new Date().toISOString().slice(0, 16).replace('T', ' ');
    const title = `Chat ${stamp}`;
    const path = `Chats/${stamp.replace(/[: ]/g, '-')}.md`;
    const body =
      `---\ntype: chat\ndate: ${todayISO()}\n---\n\n# ${title}\n\n` +
      messages
        .map((m) => `## ${m.role === 'user' ? 'You' : m.role === 'assistant' ? 'Assistant' : 'System'}\n\n${m.content}`)
        .join('\n\n');
    try {
      await api.createNote({ path, body });
      toast.success(`saved to ${path}`);
    } catch (e) {
      toast.error('save failed: ' + (errorMessage(e)));
    }
  }

  function onKey(e: KeyboardEvent) {
    // Enter sends; Shift+Enter inserts newline. Mirrors the convention
    // every chat UI uses since Slack made it standard.
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }

  // Auto-grow the composer up to ~40dvh so a multi-line draft is
  // visible end-to-end without the user manually resizing. Same
  // pattern as the AIOverlay's autosize — height:auto reset → read
  // scrollHeight → clamp → write back. Falls back to internal scroll
  // once we hit the cap. Re-runs on every input mutation via $effect
  // so paste / programmatic writes also resize correctly.
  function autosizeInput() {
    if (!inputEl) return;
    // Cap to 40% of the VISIBLE viewport — not innerHeight — so the
    // textarea can't grow taller than the room left above the keyboard
    // on mobile. When the keyboard is up, visualViewport.height is the
    // authoritative "what the user sees" measurement.
    const visible = (typeof window !== 'undefined' && window.visualViewport)
      ? window.visualViewport.height
      : window.innerHeight;
    const cap = Math.max(120, Math.floor(visible * 0.4));
    inputEl.style.height = 'auto';
    const next = Math.min(cap, inputEl.scrollHeight);
    inputEl.style.height = next + 'px';
    inputEl.style.overflowY = inputEl.scrollHeight > cap ? 'auto' : 'hidden';
  }
  $effect(() => {
    void input;
    tick().then(() => autosizeInput());
  });

  // ── iOS / mobile keyboard fix (rewritten) ───────────────────────
  // Approach: bind the chat root container's height directly to the
  // visualViewport height. The native flex column then naturally
  // places the composer at the visible bottom — no transforms, no
  // lifting math, no double-shrinkage from dvh + translateY (which
  // was the bug in the previous attempt).
  //
  // Why visualViewport and not 100dvh? iOS Safari < 16.4 reports dvh
  // values that DON'T shrink for the keyboard — only for the URL
  // chrome. dvh is consistent enough across the rest of the app, but
  // a chat composer must sit exactly above the keyboard, and that's
  // what visualViewport reports authoritatively on every supported
  // browser (iOS Safari, Chrome Android, Firefox mobile, all desktop).
  //
  // SSR-safe: viewportHeight starts null; the inline style falls back
  // to "100dvh" until the effect runs. No flicker on hydration.
  let viewportHeight = $state<number | null>(null);
  let keyboardOpen = $state(false);
  $effect(() => {
    if (typeof window === 'undefined') return;
    const vv = window.visualViewport;
    if (!vv) return;
    function update() {
      if (!vv) return;
      viewportHeight = vv.height;
      // 120px threshold separates the keyboard from chrome shrinks
      // (Safari's URL bar can collapse VV by 40-80px without the
      // keyboard being up).
      const obscured = Math.max(0, window.innerHeight - vv.height);
      const wasOpen = keyboardOpen;
      keyboardOpen = obscured > 120;
      // When the keyboard rises, close any open mode-picker so it
      // doesn't crowd the messages area, and scroll to bottom so the
      // most recent message is visible right above the keyboard.
      if (keyboardOpen && !wasOpen) {
        modePickerOpen = false;
        tick().then(() => {
          if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
        });
      }
    }
    vv.addEventListener('resize', update);
    vv.addEventListener('scroll', update);
    update();
    return () => {
      vv.removeEventListener('resize', update);
      vv.removeEventListener('scroll', update);
    };
  });

  // Computed style string for the chat root container. Falls back to
  // 100dvh when visualViewport isn't available (SSR, very old browsers).
  let rootStyle = $derived(
    viewportHeight !== null ? `height: ${viewportHeight}px` : 'height: 100dvh'
  );
</script>

<!--
  Root: height is bound to visualViewport.height so the flex chain
  always knows the exact visible region. When the keyboard opens, the
  effect updates `viewportHeight` → this inline style shrinks → the
  composer naturally lands at the new bottom via flex layout. No
  transforms, no offset math.
  min-h-0 on the message scroll area lets flex shrinking propagate;
  without it the scroller would expand to its content size and push
  the composer off-screen on small viewports.
-->
<div class="flex flex-col overflow-hidden w-full" style={rootStyle}>
  <!-- Use PageHeader's actions snippet so the title/subtitle and
       chrome buttons share the same flex-wrap rules. Avoids the
       previous nested-<header> + arm-twisted flex layout that
       caused the subtitle to crowd the buttons on narrow screens. -->
  <div class="px-3 pt-2 border-b border-surface1 flex-shrink-0 max-w-3xl w-full mx-auto">
    <PageHeader title="Chat" subtitle={mode.tagline}>
      {#snippet actions()}
        <!-- Mode chip: shows current glyph + label; tap to expand the
             picker below. Mirrors AIOverlay's compact-mode-pill UX —
             a thin row of selectable chips appears underneath when
             open, instead of a full modal dialog.  -->
        <button
          onclick={() => (modePickerOpen = !modePickerOpen)}
          title="switch mode (posture / persona)"
          class="text-xs px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary inline-flex items-center gap-1.5"
        >
          <span class="font-mono text-primary">{mode.glyph}</span>
          <span class="hidden sm:inline">{mode.label}</span>
          <svg viewBox="0 0 24 24" class="w-3 h-3 transition-transform {modePickerOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </button>
        {#if messages.length > 0}
          <button onclick={saveAsNote} class="text-xs px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary">
            Save as note
          </button>
          <button onclick={reset} class="text-xs px-2.5 py-1.5 text-dim hover:text-error">
            Clear
          </button>
        {/if}
      {/snippet}
    </PageHeader>
    {#if modePickerOpen}
      <!-- Expanded picker: two compact rows (modes + personas). Each
           chip is a tap-target on mobile (~36px high via py-2). The
           current mode is highlighted with primary text + a bottom
           rule. Hover hint surfaces the tagline so the user can
           preview what each posture does. -->
      <div class="pb-2 pt-1 space-y-1">
        <div class="flex flex-wrap gap-1">
          {#each GENERIC_MODES as m}
            <button
              onclick={() => pickMode(m.id)}
              title={m.tagline}
              class="text-[11px] px-2 py-1 rounded inline-flex items-center gap-1 transition-colors
                {modeId === m.id ? 'bg-primary/15 text-primary border border-primary' : 'bg-surface0 border border-surface1 text-subtext hover:border-primary'}"
            >
              <span class="font-mono">{m.glyph}</span>
              <span>{m.label}</span>
            </button>
          {/each}
        </div>
        {#if PERSONAS.length > 0}
          <div class="flex flex-wrap gap-1 pt-0.5 border-t border-surface1/40">
            <span class="text-[10px] uppercase tracking-wider text-dim self-center mr-1">Personas</span>
            {#each PERSONAS as m}
              <button
                onclick={() => pickMode(m.id)}
                title={m.tagline}
                class="text-[11px] px-2 py-1 rounded inline-flex items-center gap-1 transition-colors
                  {modeId === m.id ? 'bg-primary/15 text-primary border border-primary' : 'bg-surface0 border border-surface1 text-subtext hover:border-primary'}"
              >
                <span class="font-mono">{m.glyph}</span>
                <span>{m.label}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>

  <div bind:this={scrollEl} class="flex-1 min-h-0 overflow-y-auto">
    <div class="max-w-3xl mx-auto px-4 py-6">
      {#if messages.length === 0}
        <div class="text-center py-6">
          <p class="text-sm text-dim">Say hi. The model has no vault context unless you mention notes by name or paste excerpts.</p>
          <p class="text-[11px] text-dim/70 italic mt-2">
            Conversations are stored locally — nothing leaves the server (except your messages going to the AI provider).
          </p>
        </div>
      {/if}

      <ol class="space-y-4">
        {#each messages as m, i (i)}
          <li class="flex gap-3 {m.role === 'user' ? 'justify-end' : ''}">
            {#if m.role !== 'user'}
              <div class="w-8 h-8 rounded-full bg-surface1 text-primary flex items-center justify-center flex-shrink-0 text-xs font-mono mt-1">AI</div>
            {/if}
            <div
              class="max-w-[85%] sm:max-w-[75%] px-4 py-2.5 rounded-lg whitespace-pre-wrap break-words text-sm leading-relaxed
                {m.role === 'user' ? 'bg-primary text-on-primary' : m.role === 'system' ? 'bg-surface0 text-dim border border-surface1 italic' : 'bg-surface0 text-text border border-surface1'}"
            >{m.content}</div>
            {#if m.role === 'user'}
              <div class="w-8 h-8 rounded-full bg-surface1 text-subtext flex items-center justify-center flex-shrink-0 text-xs font-mono mt-1">You</div>
            {/if}
          </li>
        {/each}
        {#if busy}
          <li class="flex gap-3">
            <div class="w-8 h-8 rounded-full bg-surface1 text-primary flex items-center justify-center flex-shrink-0 text-xs font-mono mt-1">AI</div>
            <div class="px-4 py-2.5 rounded-lg bg-surface0 border border-surface1">
              <div class="flex gap-1 items-center text-dim">
                <span class="w-1.5 h-1.5 rounded-full bg-current animate-bounce"></span>
                <span class="w-1.5 h-1.5 rounded-full bg-current animate-bounce" style="animation-delay: 0.15s"></span>
                <span class="w-1.5 h-1.5 rounded-full bg-current animate-bounce" style="animation-delay: 0.3s"></span>
              </div>
            </div>
          </li>
        {/if}
      </ol>
    </div>
  </div>

  <!--
    Bottom composer. Because the root container's height is bound to
    visualViewport.height (see effect in <script>), this flex-shrink-0
    band ALWAYS lands at the visible bottom. When the keyboard opens,
    the root shrinks → composer moves up with it. No transforms.

    pb-[env(safe-area-inset-bottom)] keeps the iPhone home indicator
    off the textarea when keyboard is down. When keyboard is up, the
    home indicator is hidden anyway, so the .keyboard-open class
    zeroes the padding to avoid a phantom gap above the keyboard.
  -->
  <div
    class="composer-bar border-t border-surface1 bg-mantle flex-shrink-0 pb-[env(safe-area-inset-bottom,0px)]"
    class:keyboard-open={keyboardOpen}
  >
    <form onsubmit={send} class="max-w-3xl mx-auto p-3 flex gap-2 items-end">
      <textarea
        bind:this={inputEl}
        bind:value={input}
        onkeydown={onKey}
        oninput={autosizeInput}
        placeholder="Send a message…   (Enter to send, Shift+Enter for newline)"
        rows="1"
        class="flex-1 min-w-0 px-3 py-2 bg-surface0 border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-none transition-colors"
        style="min-height: 2.5rem;"
      ></textarea>
      <button
        type="submit"
        disabled={busy || !input.trim()}
        class="tap-target px-4 py-3 sm:py-2 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50 hover:bg-primary/90 active:bg-primary/80 transition-colors"
      >Send</button>
    </form>
  </div>
</div>

<style>
  /* When the keyboard is open, remove the safe-area padding — the
     home indicator only shows when the keyboard is down. Saves a
     few pixels of phantom gap above the keyboard. The composer's
     vertical position is driven by the root container's height
     binding to visualViewport.height in <script>, not by transforms. */
  .composer-bar.keyboard-open {
    padding-bottom: 0;
  }
</style>
