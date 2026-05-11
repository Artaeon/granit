<!--
  SectionQuestionsPanel — pick a heading from this note, get 3 sharp
  study questions specifically about THAT section. Distinct from the
  whole-note "Question" preset in AskAIDialog: that one asks the AI
  about the entire body, which on a 4000-word note dilutes the
  questions into vague generalities. Per-section is tighter: the
  questions actually probe the argument or definitions of one part.

  Use case: active reading. The reader hits a section they want to
  test their understanding of, picks it from the dropdown, gets
  three questions, and answers them in their head (or writes the
  answers as a follow-up note). Lower friction than re-selecting
  the section and using the whole AskAIDialog.

  AI gating: chatStream → /chat/stream pipeline. Same shape as the
  rest of the rail's AI features — Sabbath / redaction / audit /
  cost all centralised there.
-->
<script lang="ts">
  import { api } from '$lib/api';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import { parseBody, type ParsedSection } from '$lib/util/bodyParse';

  type Section = ParsedSection;

  let {
    notePath,
    body
  }: { notePath: string; body: string } = $props();

  // Shared parse cache — the body is split + scanned ONCE per keystroke
  // across Outline / SectionQuestionsPanel / ResearchPanel rather than
  // each running its own full pass (multiplied by the desktop +
  // mobile-drawer double-mount of the rail). On a long note that was
  // the bulk of the per-keystroke main-thread cost.
  //
  // Debounced 250ms — the section dropdown only matters when the
  // user pauses to interact. While they're typing inside a section
  // we don't need to re-derive on every keystroke, especially since
  // an in-progress edit can briefly produce nonsense intermediate
  // headings ("# part" while typing "## partial"). The panel's
  // pickedLine binding is preserved across debounce updates: if the
  // user picked a section on line 42 and then types in section 12,
  // line 42 is still selected.
  let debouncedBody = $state('');
  let primed = false;
  $effect(() => {
    const next = body;
    if (!primed) {
      primed = true;
      debouncedBody = next;
      return;
    }
    const id = setTimeout(() => { debouncedBody = next; }, 250);
    return () => clearTimeout(id);
  });
  let sections = $derived(debouncedBody.trim() ? parseBody(debouncedBody).sections : []);

  let pickedLine = $state<number | null>(null);
  let questions = $state('');
  let pending = $state(false);
  let error = $state('');
  let abort: AbortController | null = null;

  // Reset on note change so a fresh note starts blank rather than
  // rendering questions about the previous note's section.
  $effect(() => {
    void notePath;
    pickedLine = null;
    questions = '';
    error = '';
    pending = false;
    abort?.abort();
    abort = null;
  });

  let pickedSection = $derived(
    pickedLine !== null ? sections.find((s) => s.line === pickedLine) ?? null : null
  );

  function clip(t: string, max = 4000): string {
    const tt = t.trim();
    return tt.length > max ? tt.slice(0, max) + '\n…(truncated)' : tt;
  }

  async function generate() {
    const sec = pickedSection;
    if (!sec || pending) return;
    abort?.abort();
    abort = new AbortController();
    pending = true;
    error = '';
    questions = '';
    let buf = '';
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'You are an active-reading study companion. Given ONE section of a longer note, generate exactly 3 sharp study questions about that section. The questions test understanding of the argument, definitions, claims, or examples in THIS SECTION ONLY — not the rest of the note. Mix question types: one comprehension question (does the reader understand the main claim), one application question (could the reader apply or transfer the idea), one critical question (where could the argument be challenged or where does it depend on a hidden assumption). Avoid vague "what is the author saying" prompts; cite a specific term or sentence. Format: a numbered markdown list, one question per line, no preamble, no closing summary.'
          },
          {
            role: 'user',
            content: `Section heading: ${sec.title}\n\n---\n\n${clip(sec.body)}`
          }
        ],
        notePath || undefined,
        {
          onChunk: (c) => { buf += c; questions = buf; },
          onDone: () => {
            if (!buf.trim()) error = 'AI returned an empty response.';
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

  function clear() {
    stop();
    questions = '';
    error = '';
    pickedLine = null;
  }
</script>

<div class="text-sm space-y-1.5">
  {#if sections.length === 0}
    <div class="text-[11px] text-dim italic px-2 py-1">
      Add ## headings to enable per-section questions.
    </div>
  {:else}
    <select
      bind:value={pickedLine}
      onchange={() => { questions = ''; error = ''; }}
      class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text focus:outline-none focus:border-primary"
      aria-label="pick a section"
    >
      <option value={null} disabled selected={pickedLine === null}>pick a section…</option>
      {#each sections as s (s.line)}
        <option value={s.line}>
          {'·'.repeat(Math.max(0, s.level - 1))} {s.title}
        </option>
      {/each}
    </select>

    {#if pickedSection}
      <div class="flex items-center gap-1.5 px-1">
        {#if pending}
          <button
            type="button"
            onclick={stop}
            class="text-[11px] px-2 py-0.5 rounded bg-surface0 text-error border border-error hover:bg-surface0"
          >stop</button>
          <span class="text-[10px] text-secondary italic">streaming…</span>
        {:else}
          <button
            type="button"
            onclick={generate}
            class="text-[11px] px-2 py-0.5 rounded bg-surface1 hover:bg-surface2 text-primary border border-surface2 inline-flex items-center gap-1"
            title="Generate 3 study questions about this section"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-2.5 h-2.5">
              <path d="M12 3l1.2 4.2L17 9l-3.8 1.8L12 15l-1.2-4.2L7 9l3.8-1.8L12 3z" stroke-linejoin="round"/>
            </svg>
            {questions ? 'regenerate' : 'questions'}
          </button>
        {/if}
        {#if questions || error}
          <button
            type="button"
            onclick={clear}
            class="text-[10px] text-dim hover:text-error ml-auto"
          >clear</button>
        {/if}
        {#if pickedSection}
          <span class="text-[10px] text-dim ml-auto" title={`section starts at line ${pickedSection.line}`}>
            ~{Math.round(pickedSection.body.length / 4)} tok
          </span>
        {/if}
      </div>
    {/if}

    {#if error}
      <div class="text-[11px] text-error bg-surface0 border border-error rounded px-2 py-1">
        {error}
      </div>
    {:else if questions}
      <div class="bg-mantle border border-surface1 rounded px-2 py-1.5 text-[12px] questions-prose">
        <MarkdownRenderer body={questions} />
      </div>
    {/if}
  {/if}
</div>

<style>
  .questions-prose :global(ol),
  .questions-prose :global(ul) { margin: 0.2rem 0; padding-left: 1.25rem; }
  .questions-prose :global(li) { margin: 0.3rem 0; }
  .questions-prose :global(p) { margin: 0.3rem 0; }
</style>
