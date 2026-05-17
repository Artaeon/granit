<!--
  AIStaleVerdicts — the "AI verdicts on stale tasks" surface that
  used to live inside web/src/routes/tasks/+page.svelte. Extracted
  because tasks/+page.svelte was 3323 lines and this feature is a
  self-contained AI flow with its own state, prompt, parser, and
  apply path.

  Owns:
    - The trigger button (run / cancel / re-scan / dismiss).
    - The error banner.
    - The verdict panel (per-row accept-archive / defer / acknowledge,
      bulk "archive all N").

  Does NOT own:
    - The task list itself (the parent's "stale" view still renders
      cards under this component).
    - The isStale predicate or the filter that produces candidates
      (passed in so the parent stays the single source of truth on
      what's considered "stale" — that 7-day threshold is also used
      for sidebar counts, tab badges, list filters).

  Props
    candidates    Tasks that should be evaluated when the user clicks
                  ✨ AI verdicts. Capped at 25 internally to keep the
                  prompt bounded; the parent passes the already-
                  filtered set.
    allTasks      The full task list — needed by validateStaleVerdicts
                  to confirm each verdict's taskId actually exists.
    onReload      Called after a successful defer / archive so the
                  parent re-fetches and the task drops out of the
                  stale view.
-->
<script lang="ts">
  import { api } from '$lib/api';
  import type { Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { rafThrottle } from '$lib/util/streamThrottle';
  import { extractJsonBlock } from '$lib/util/jsonExtract';
  import {
    buildStaleVerdictPrompt,
    validateStaleVerdicts,
    type StaleVerdict
  } from '$lib/tasks/aiPrompts';

  let {
    candidates,
    allTasks,
    onReload
  }: {
    candidates: Task[];
    allTasks: Task[];
    onReload: () => void | Promise<void>;
  } = $props();

  function todayISO(): string {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }

  let busy = $state(false);
  let error = $state('');
  let raw = $state('');
  let verdicts = $state<StaleVerdict[]>([]);
  let abort: AbortController | null = null;
  let applyingId = $state<string>('');

  async function run() {
    if (busy) return;
    busy = true;
    error = '';
    raw = '';
    verdicts = [];
    abort = new AbortController();
    const slice = candidates.slice(0, 25);
    if (slice.length === 0) {
      busy = false;
      toast.info('No stale tasks to evaluate.');
      return;
    }
    const { system, user } = buildStaleVerdictPrompt(slice, todayISO());
    // rAF coalescer — JSON parser runs at most once per frame as the
    // model streams. Without it a 5KB response triggers ~80 parses.
    const t = rafThrottle((full) => {
      raw = full;
      const block = extractJsonBlock(full);
      if (!block) return;
      try {
        const parsed = JSON.parse(block) as { verdicts?: StaleVerdict[] };
        if (Array.isArray(parsed.verdicts)) {
          verdicts = validateStaleVerdicts(parsed.verdicts, allTasks);
        }
      } catch {
        // Partial JSON during stream — wait for more chunks.
      }
    });
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
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
  function cancel() { abort?.abort(); }
  function dismiss() {
    raw = '';
    error = '';
    verdicts = [];
  }

  // archive → done=true + triage='dropped' (matches the "drop this task"
  //           semantics from the triage flow)
  // defer   → snoozedUntil = today + 14 days (drops from stale view AND
  //           open list until then)
  // keep    → no-op; just remove the verdict from the panel so the user
  //           can move on
  async function apply(v: StaleVerdict) {
    applyingId = v.taskId;
    try {
      if (v.verdict === 'archive') {
        await api.patchTask(v.taskId, { done: true, triage: 'dropped' });
      } else if (v.verdict === 'defer') {
        const t = new Date();
        t.setDate(t.getDate() + 14);
        await api.patchTask(v.taskId, {
          snoozedUntil: t.toISOString(),
          triage: 'snoozed'
        });
      }
      verdicts = verdicts.filter((x) => x.taskId !== v.taskId);
      if (v.verdict !== 'keep') await onReload();
    } catch (e) {
      toast.error('Apply failed: ' + errorMessage(e));
    } finally {
      applyingId = '';
    }
  }
  function skip(taskId: string) {
    verdicts = verdicts.filter((x) => x.taskId !== taskId);
  }
  let archiveCount = $derived(verdicts.filter((v) => v.verdict === 'archive').length);
  async function archiveAll() {
    const items = verdicts.filter((v) => v.verdict === 'archive');
    for (const v of items) await apply(v);
  }
</script>

<div class="flex items-baseline gap-3 mb-4">
  <p class="text-sm text-dim flex-1">
    Tasks that haven't been touched in 7+ days. Drop, snooze, or do them.
  </p>
  {#if busy}
    <button
      onclick={cancel}
      class="px-3 py-1.5 text-xs bg-surface0 text-warning rounded hover:bg-surface1 flex-shrink-0"
      title="Cancel the in-flight verdict scan"
    >✨ thinking… cancel</button>
  {:else if verdicts.length > 0 || error || raw}
    <button
      onclick={() => void run()}
      class="px-3 py-1.5 text-xs bg-surface1 text-secondary rounded hover:bg-surface2 flex-shrink-0"
      title="Re-evaluate stale tasks"
    >↻ re-scan</button>
    <button
      onclick={dismiss}
      class="px-3 py-1.5 text-xs text-dim hover:text-error flex-shrink-0"
    >dismiss</button>
  {:else}
    <button
      onclick={() => void run()}
      disabled={candidates.length === 0}
      class="px-3 py-1.5 text-xs bg-surface1 text-secondary rounded hover:bg-surface2 disabled:opacity-50 flex-shrink-0"
      title="AI verdict on each stale task: keep, defer 2 weeks, or archive"
    >✨ AI verdicts</button>
  {/if}
</div>

{#if error}
  <div class="mb-5 p-3 bg-surface0 border border-error rounded text-xs text-error">
    {error}
  </div>
{/if}

{#if verdicts.length > 0}
  <!-- Verdict panel. Each row: keep / defer / archive with a one-line
       rationale. Accept-archive sets the task done + triage='dropped';
       defer snoozes 14 days; keep is a no-op (just dismisses the row).
       User stays in control — every action goes through `apply` which
       round-trips through patchTask + onReload. -->
  <div class="mb-5 p-3 bg-surface1 border border-surface2 rounded">
    <div class="flex items-center mb-2">
      <div class="text-xs uppercase tracking-wider text-secondary font-semibold flex-1">
        AI verdicts ({verdicts.length})
      </div>
      {#if archiveCount > 1}
        <button
          onclick={() => void archiveAll()}
          disabled={!!applyingId}
          class="text-[10px] text-error hover:underline mr-2 disabled:opacity-50"
          title="Archive all {archiveCount} dead-weight tasks"
        >archive all {archiveCount}</button>
      {/if}
      <button
        onclick={dismiss}
        class="text-[10px] text-dim hover:text-error"
        title="Drop verdicts without applying"
      >discard</button>
    </div>
    <ul class="space-y-2">
      {#each verdicts as v (v.taskId)}
        {@const t = allTasks.find((x) => x.id === v.taskId)}
        {#if t}
          <!-- Static class lookup so Tailwind's purge keeps them.
               Dynamic `text-{x}` would survive in dev but get
               tree-shaken in prod. -->
          {@const verdictClass = v.verdict === 'archive' ? 'text-error' : v.verdict === 'defer' ? 'text-warning' : 'text-success'}
          <li class="flex items-start gap-2 text-xs">
            <div class="flex-1 min-w-0">
              <div class="text-text">{t.text}</div>
              <div class="text-dim mt-0.5">
                <span class="font-mono uppercase tracking-wider {verdictClass}">{v.verdict}</span>
                {#if v.rationale}<span class="italic"> — {v.rationale}</span>{/if}
              </div>
            </div>
            <button
              onclick={() => void apply(v)}
              disabled={applyingId === v.taskId}
              class="px-2 py-0.5 rounded flex-shrink-0
                {v.verdict === 'archive' ? 'bg-surface0 text-error hover:bg-surface1' :
                 v.verdict === 'defer' ? 'bg-surface0 text-warning hover:bg-surface1' :
                 'bg-surface0 text-success hover:bg-surface1'}
                disabled:opacity-50"
              title={v.verdict === 'archive'
                ? 'Drop the task — done=true, triage=dropped'
                : v.verdict === 'defer'
                ? 'Snooze 2 weeks'
                : 'Acknowledge — keep on the list'}
            >{applyingId === v.taskId ? '…' :
              v.verdict === 'archive' ? 'archive' :
              v.verdict === 'defer' ? 'defer 2w' : 'acknowledge'}</button>
            <button
              onclick={() => skip(v.taskId)}
              disabled={!!applyingId}
              class="px-2 py-0.5 text-dim hover:text-text flex-shrink-0 disabled:opacity-50"
            >skip</button>
          </li>
        {/if}
      {/each}
    </ul>
    <p class="text-[10px] text-dim italic mt-2 pt-2 border-t border-surface1">
      Verdicts are advisory — every action requires your accept. Defer = snooze 14 days; archive = done + dropped.
    </p>
  </div>
{/if}
