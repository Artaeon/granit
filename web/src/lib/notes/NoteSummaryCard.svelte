<!--
  NoteSummaryCard — inline AI summary that lives at the top of the
  rendered preview pane. Intent: when the user opens a long note
  they haven't seen in a while, give them a 30-second on-ramp BEFORE
  they start reading.

  Heuristics for when to show:
    - The note body is at least 600 words (below that the user can
      just read the whole thing — a summary is friction).
    - The body doesn't already start with a TL;DR / Summary / tl;dr
      section (we don't summarise something the author already
      summarised — that's noise).

  Cache:
    - First generation is streamed via chatStream and persisted to
      `frontmatter.ai_summary` (object: { text, generated_at, model_hint })
      via the host page's saveFrontmatter callback. Subsequent visits
      render the cached summary instantly with no network call.
    - User can refresh (re-run, replaces the cached version), save
      to body as a TL;DR block (prepends a `> [!summary]` callout
      and clears the cached text), or dismiss for this session.
    - "ai_summary_dismissed" frontmatter key suppresses the card
      forever for this note (user said "don't bug me about this one").

  AI gating: chatStream → /chat/stream pipeline. Sabbath / consent /
  redaction / audit / cost all live there. Don't bypass.
-->
<script lang="ts">
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  interface CachedSummary {
    text: string;
    generated_at: string;
  }

  let {
    notePath,
    title,
    body,
    frontmatter,
    onSaveFrontmatter,
    onPrepend
  }: {
    notePath: string;
    title: string;
    body: string;
    frontmatter: Record<string, unknown>;
    /** Save-frontmatter callback from the host. Merges + persists. */
    onSaveFrontmatter: (next: Record<string, unknown>) => Promise<void> | void;
    /** Prepend a string to the editor body (used for "save as TL;DR"). */
    onPrepend: (markdown: string) => void;
  } = $props();

  // Word count over a fenced-aware body — code blocks get counted as
  // a single token so a heavy code dump doesn't trigger the card.
  function bodyWordCount(src: string): number {
    const stripped = src.replace(/```[\s\S]*?```/g, ' ');
    const t = stripped.trim();
    return t ? t.split(/\s+/).length : 0;
  }
  function startsWithSummary(src: string): boolean {
    // Strip leading frontmatter + whitespace, then check the first
    // ~6 lines for a tl;dr / summary heading or callout. Defensive
    // about case + variations (`TL;DR`, `Summary`, `> [!summary]`).
    let s = src;
    if (s.startsWith('---')) {
      const end = s.indexOf('\n---', 3);
      if (end !== -1) s = s.slice(end + 4);
    }
    const head = s.trimStart().split('\n', 8).join('\n');
    if (/^#{1,3}\s+(tl;?\s*dr|summary|abstract)\b/im.test(head)) return true;
    if (/^>\s*\[!(summary|abstract|tldr|tl;dr)\]/im.test(head)) return true;
    if (/^\*\*?(tl;?\s*dr|summary)\b/im.test(head)) return true;
    return false;
  }

  let wordCount = $derived(bodyWordCount(body));
  let hasOwnSummary = $derived(startsWithSummary(body));
  let cached = $derived.by<CachedSummary | null>(() => {
    const v = frontmatter?.ai_summary;
    if (!v || typeof v !== 'object') return null;
    const t = (v as Record<string, unknown>).text;
    const g = (v as Record<string, unknown>).generated_at;
    if (typeof t !== 'string' || !t.trim()) return null;
    return { text: t, generated_at: typeof g === 'string' ? g : '' };
  });
  let dismissedFrontmatter = $derived(frontmatter?.ai_summary_dismissed === true);

  // Per-session dismissal — distinct from the persistent
  // ai_summary_dismissed frontmatter key. The user can hide it for
  // now without saying "never again". Reset on note change.
  let sessionDismissed = $state(false);
  $effect(() => {
    void notePath;
    sessionDismissed = false;
  });

  // Streaming state.
  let pending = $state(false);
  let response = $state('');
  let error = $state('');
  let abort: AbortController | null = null;
  // Visible flag — drives the "show me a fresh summary" UX even
  // when there's no cached value yet. We auto-show the card under
  // the heuristic conditions below.
  let visible = $derived(
    !sessionDismissed &&
      !dismissedFrontmatter &&
      !hasOwnSummary &&
      (cached !== null || (wordCount >= 600 && body.trim().length > 0))
  );

  function clip(text: string, max = 12000): string {
    const t = text.trim();
    return t.length > max ? t.slice(0, max) + '\n…(truncated)' : t;
  }

  async function generate(): Promise<void> {
    if (pending) return;
    abort?.abort();
    abort = new AbortController();
    pending = true;
    response = '';
    error = '';
    let buf = '';
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              "You write a tight one-paragraph summary of a single note from the user's vault. Voice: declarative, concrete, the writer's own register — match the note's tone (formal, casual, technical) rather than imposing a generic AI cadence. Constraints: 50-90 words; one paragraph; no preamble (\"This note is about…\" is forbidden); start with the strongest claim or core idea, not the topic; if the note argues a position, say what the position is; if the note is exploratory or notes-to-self, say what it's wrestling with. Plain text — no markdown headers, no bullets, no quotes around proper terms. Refuse to invent — if the note is fragmentary, summarise honestly that it's a working draft and what it's circling around. Return only the paragraph."
          },
          {
            role: 'user',
            content: `Note title: ${title}\n\n---\n\n${clip(body)}`
          }
        ],
        notePath || undefined,
        {
          onChunk: (c) => {
            buf += c;
            response = buf;
          },
          onDone: async () => {
            const text = buf.trim();
            if (text) {
              try {
                const next = { ...frontmatter, ai_summary: { text, generated_at: new Date().toISOString() } };
                // If the user previously dismissed this note's
                // summary forever, generating a fresh one is
                // implicit consent to re-enable. Drop the flag.
                if ('ai_summary_dismissed' in next) delete next.ai_summary_dismissed;
                await onSaveFrontmatter(next);
              } catch (e) {
                // Cache write failed — the streamed text is still
                // visible to the user; surface the failure but
                // don't break the UI.
                toast.warning('Summary generated but caching failed.');
                console.error('ai_summary cache failed', e);
              }
            } else {
              error = 'AI returned an empty summary.';
            }
          },
          onError: (err) => { error = err.message; }
        },
        abort.signal
      );
    } finally {
      pending = false;
      abort = null;
    }
  }

  function stop() {
    abort?.abort();
    abort = null;
    pending = false;
  }

  async function refresh() {
    response = '';
    await generate();
  }

  function dismissSession() {
    sessionDismissed = true;
    // Also abort any in-flight stream — user changed their mind.
    stop();
  }

  async function dismissForever() {
    const next = { ...frontmatter };
    next.ai_summary_dismissed = true;
    delete next.ai_summary; // free up the cache too
    try {
      await onSaveFrontmatter(next);
      toast.info("Won't auto-summarise this note again.");
    } catch (e) {
      toast.error('Could not save dismissal — frontmatter write failed.');
    }
  }

  async function saveAsTLDR() {
    const text = (cached?.text || response).trim();
    if (!text) return;
    // Render as an Obsidian-style summary callout. Keeps it visually
    // distinct from regular paragraphs in both Granit and Obsidian,
    // and the renderer styles it with a tinted background so a
    // returning reader finds it instantly. Two newlines after to
    // separate from the rest of the body.
    const block = `> [!summary] TL;DR\n> ${text.replace(/\n/g, '\n> ')}\n\n`;
    onPrepend(block);
    // Drop the cached summary now that it lives in the body —
    // otherwise we'd double-render. Keep ai_summary_generated_at
    // off too so a later refresh reflects the latest state.
    const next = { ...frontmatter };
    delete next.ai_summary;
    try {
      await onSaveFrontmatter(next);
    } catch {
      // Non-critical — the body now has the TL;DR; the stale cache
      // just means the next visit will see the same TL;DR PLUS the
      // card. Refresh would clean it up; not worth toasting an
      // error to the user.
    }
    toast.success('TL;DR added at the top of the note.');
  }

  // Auto-generate once when the card becomes visible AND there's
  // no cache. The user can opt out via dismiss; we don't keep
  // hammering. Tracked by note path so a different note triggers a
  // fresh auto-fire.
  let autoFiredFor = $state<string>('');
  $effect(() => {
    if (!visible) return;
    if (cached !== null) return;
    if (pending) return;
    if (autoFiredFor === notePath) return;
    autoFiredFor = notePath;
    void generate();
  });
</script>

{#if visible}
  <div
    class="ai-summary mb-4 not-prose border border-secondary/30 rounded-md bg-secondary/5"
    role="region"
    aria-label="AI summary"
  >
    <div class="flex items-baseline gap-2 px-3 py-2 border-b border-secondary/20">
      <span class="text-secondary text-[11px] uppercase tracking-wider font-medium flex items-center gap-1.5">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3" aria-hidden="true">
          <path d="M12 3l1.2 4.2L17 9l-3.8 1.8L12 15l-1.2-4.2L7 9l3.8-1.8L12 3z" stroke-linejoin="round"/>
        </svg>
        AI summary
      </span>
      {#if cached?.generated_at}
        <span class="text-[10px] text-dim font-mono" title={cached.generated_at}>
          {new Date(cached.generated_at).toLocaleDateString()}
        </span>
      {:else if pending}
        <span class="text-[10px] text-secondary italic">streaming…</span>
      {/if}
      <span class="flex-1"></span>
      {#if pending}
        <button
          type="button"
          onclick={stop}
          class="text-[10px] text-warning hover:underline"
        >stop</button>
      {:else}
        {#if cached || response}
          <button
            type="button"
            onclick={saveAsTLDR}
            class="text-[10px] text-secondary hover:underline"
            title="Prepend the summary to the note body as a TL;DR callout"
          >save as TL;DR</button>
          <button
            type="button"
            onclick={refresh}
            class="text-[10px] text-secondary hover:underline"
            title="Re-generate the summary"
          >refresh</button>
        {/if}
        <button
          type="button"
          onclick={dismissSession}
          class="text-[10px] text-dim hover:text-text"
          title="Hide the summary card for this session"
        >hide</button>
        <button
          type="button"
          onclick={dismissForever}
          class="text-[10px] text-dim hover:text-error"
          title="Don't auto-summarise this note again (saved to frontmatter)"
        >never</button>
      {/if}
    </div>
    <div class="px-3 py-2 text-[13px] text-text leading-relaxed">
      {#if error}
        <span class="text-error text-xs">{error}</span>
      {:else if cached && !pending && !response}
        <p class="m-0">{cached.text}</p>
      {:else if response}
        <!-- Stream as plain markdown so a model that returned a
             stray bullet doesn't render as `* foo` text. -->
        <div class="prose prose-sm max-w-none">
          <MarkdownRenderer body={response} />
        </div>
      {:else if pending}
        <span class="text-dim italic text-xs flex items-center gap-2">
          <span class="ai-spinner" aria-hidden="true"></span>
          summarising…
        </span>
      {:else}
        <button
          type="button"
          onclick={generate}
          class="text-xs text-secondary hover:underline"
        >Generate a summary now</button>
      {/if}
    </div>
  </div>
{/if}

<style>
  .ai-spinner {
    display: inline-block;
    width: 0.75rem;
    height: 0.75rem;
    border: 2px solid var(--color-surface1);
    border-top-color: var(--color-secondary);
    border-radius: 50%;
    animation: ai-spin 0.7s linear infinite;
  }
  @keyframes ai-spin {
    to { transform: rotate(360deg); }
  }
  /* Keep the summary card from inheriting prose-note margins from
     the surrounding rendered body — the card has its own framing. */
  .ai-summary :global(p:first-child) { margin-top: 0; }
  .ai-summary :global(p:last-child) { margin-bottom: 0; }
</style>
