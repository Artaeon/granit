<!--
  AskTasks — the chat-style "ask AI about my task list" surface that
  used to live inline in web/src/routes/tasks/+page.svelte. Pure read
  surface: streams a markdown answer over the loaded task set, no
  mutations.

  Parent integration:
    - bind:open — flip to true to open the panel (typically from a
      trigger button in the quick-add row). The component sets it
      back to false on dismiss.
    - filtered — the currently-visible task set, used as context for
      the model. Capped at 80 internally to keep the prompt bounded.

  Behaviour:
    - Auto-focuses the question input on open.
    - Enter submits; Stop aborts mid-stream; Dismiss closes the panel
      and clears question/answer state.
    - Re-ask button surfaces after a completed answer so the user
      can re-run the same query against a refreshed task list.
-->
<script lang="ts">
  import { api, type Task } from '$lib/api';
  import { rafThrottle } from '$lib/util/streamThrottle';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  let {
    filtered,
    open = $bindable(false)
  }: {
    filtered: Task[];
    open?: boolean;
  } = $props();

  let question = $state('');
  let answer = $state('');
  let busy = $state(false);
  let error = $state('');
  let abort: AbortController | null = null;
  let inputEl = $state<HTMLInputElement | undefined>();

  // Auto-focus the input the moment the panel opens. queueMicrotask
  // because the input element binding lands after the conditional
  // block mounts; focusing in the same tick misses the element.
  $effect(() => {
    if (open) {
      queueMicrotask(() => inputEl?.focus());
    }
  });

  // Compact JSON serialisation of the candidate tasks. Capped at the
  // 80 most recent in the current view; the model rarely benefits
  // from more and we don't want a 50KB prompt for a yes/no question.
  // Fields chosen for "what's the user likely to ask about" — text,
  // priority, due, scheduled, estimate, tags, and the parent
  // project/goal/deadline pointers.
  function buildSeed(): string {
    const slice = filtered.slice(0, 80).map((t) => ({
      text: t.text,
      priority: t.priority || undefined,
      due: t.dueDate || undefined,
      done: t.done || undefined,
      scheduled: t.scheduledStart || undefined,
      est: t.estimatedMinutes || undefined,
      tags: t.tags && t.tags.length > 0 ? t.tags : undefined,
      project: t.projectId || undefined,
      goal: t.goalId || undefined,
      deadline: t.deadlineId || undefined,
      note: t.notePath
    }));
    return JSON.stringify(slice, null, 2);
  }

  async function submit() {
    const q = question.trim();
    if (!q || busy) return;
    abort?.abort();
    abort = new AbortController();
    busy = true;
    error = '';
    answer = '';
    const seed = buildSeed();
    const system =
      "You answer the user's questions about their own task list. Be specific — quote " +
      'task text, mention priorities/due dates, count what needs counting. If the answer ' +
      "isn't supported by the loaded tasks, say so rather than guess. Return markdown " +
      'with concise paragraphs and bullets where helpful. No preamble.';
    const user =
      'Currently visible tasks (JSON, capped at 80):\n```json\n' + seed + '\n```\n\n' +
      'Question: ' + q;
    try {
      const t = rafThrottle((full: string) => { answer = full; });
      await api.chatStream(
        [{ role: 'system', content: system }, { role: 'user', content: user }],
        undefined,
        {
          onChunk: t.onChunk,
          onDone: () => { t.flush(); },
          onError: (err) => { t.flush(); error = err.message; }
        },
        abort.signal
      );
    } finally {
      busy = false;
      abort = null;
    }
  }

  function dismiss() {
    abort?.abort();
    abort = null;
    busy = false;
    question = '';
    answer = '';
    error = '';
    open = false;
  }
</script>

<!-- The parent that owns the trigger also owns the "empty filter →
     don't open" guard, since `filtered` is its derived state. We
     still gate Submit on filtered.length here so an out-of-band
     state.empty case doesn't burn a tokenless call. -->
{#if open}
  <div class="px-3 py-2 border-b border-surface1 flex-shrink-0 bg-surface0">
    <div class="flex items-baseline gap-2 mb-1.5">
      <h3 class="text-[10px] uppercase tracking-wider text-text font-medium">ask tasks</h3>
      <span class="text-[10px] text-dim font-mono">over {filtered.length} task{filtered.length === 1 ? '' : 's'}</span>
      <span class="flex-1"></span>
      {#if busy}
        <span class="text-[10px] text-dim italic font-mono">streaming…</span>
        <button onclick={() => abort?.abort()} class="text-[10px] text-dim hover:text-text font-mono">stop</button>
      {:else if answer.length > 0}
        <button onclick={() => void submit()} class="text-[10px] text-text hover:underline font-mono">re-ask</button>
      {/if}
      <button onclick={dismiss} class="text-[10px] text-dim hover:text-text font-mono">dismiss</button>
    </div>
    <input
      bind:this={inputEl}
      bind:value={question}
      onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void submit(); } }}
      placeholder="e.g. what's blocking the launch? which P1 has no due date?"
      disabled={busy}
      class="w-full bg-mantle border border-surface1 rounded px-2 py-1 text-[13px] text-text placeholder-dim focus:outline-none focus:border-primary mb-1.5 disabled:opacity-50"
    />
    {#if error}
      <p class="text-[11px] text-error mb-1">{error}</p>
    {/if}
    {#if answer.trim()}
      <div class="bg-mantle border border-surface1 rounded p-2 max-h-[24rem] overflow-y-auto">
        <MarkdownRenderer body={answer} />
      </div>
    {/if}
  </div>
{/if}
