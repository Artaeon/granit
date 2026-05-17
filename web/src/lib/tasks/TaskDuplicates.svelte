<!--
  TaskDuplicates — surfaces likely-duplicate task pairs from the
  GET /api/v1/tasks/duplicates endpoint. Same component shape as
  AIStaleVerdicts: a header row with run / re-scan / dismiss, then
  a list of accept/skip cards. Backend is deterministic (Jaccard
  token similarity) — no AI cost, no streaming, single fetch.

  Each pair renders both tasks side-by-side with a similarity
  badge. The user picks which one to keep; the other is dropped
  (done=true, triage='dropped' — same semantics as the triage flow
  uses). "Skip" dismisses the pair locally without mutating either
  task — appropriate when the user looks at it and decides they
  actually want both.

  Props
    onReload   Parent's task-list reload — called after a successful
               merge so the surviving task list reflects the drop.
-->
<script lang="ts">
  import { api, type Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';

  let { onReload }: { onReload: () => void | Promise<void> } = $props();

  interface DupPair { a: Task; b: Task; similarity: number }

  let pairs = $state<DupPair[]>([]);
  // `attempted` flips true after the FIRST run regardless of
  // success/failure. Previously this was `attempted` which only
  // tracked success — pairing it with the auto-run effect meant
  // a persistent fetch error (attempted stays false, finally flips
  // loading false) re-triggered the effect → infinite re-run loop.
  // `attempted` decouples "have we tried" from "did it succeed",
  // so the auto-run is a single shot whatever the outcome.
  let attempted = $state(false);
  let loading = $state(false);
  let error = $state('');
  let scanned = $state(0);
  let mergingId = $state<string>(''); // task ID currently being dropped

  // Auto-run on first mount so the user lands on a populated view
  // rather than an empty pane with a "run" button. Subsequent
  // sessions persist nothing — the scan is fast enough that a
  // fresh fetch on revisit is acceptable and the data needs to be
  // fresh anyway.
  $effect(() => {
    if (!attempted && !loading) {
      void run();
    }
  });

  async function run() {
    loading = true;
    error = '';
    try {
      const r = await api.taskDuplicates();
      pairs = r.pairs;
      scanned = r.scanned;
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
      attempted = true;
    }
  }

  // Drop `loser`, keep `winner`. Done=true + triage='dropped' mirrors
  // the triage-flow "drop this task" semantics, so the dropped task
  // moves to the same "completed/archived" bucket every other surface
  // uses. The pair vanishes from the panel either way.
  async function merge(pair: DupPair, winnerId: string) {
    const loser = pair.a.id === winnerId ? pair.b : pair.a;
    mergingId = loser.id;
    try {
      await api.patchTask(loser.id, { done: true, triage: 'dropped' });
      // Drop the current pair AND every other stale pair that
      // references the just-dropped task. Otherwise a transitive
      // duplicate "A pairs with B and B pairs with C" would leave a
      // dangling B-C card after the user merges A-B; the next click
      // would "merge" against a task that's already gone, which
      // succeeds (idempotent patch) but reads as a confusing no-op.
      pairs = pairs.filter(
        (p) => p !== pair && p.a.id !== loser.id && p.b.id !== loser.id
      );
      await onReload();
      toast.success('Merged — kept the other task.');
    } catch (e) {
      toast.error('Merge failed: ' + errorMessage(e));
    } finally {
      mergingId = '';
    }
  }

  function skip(pair: DupPair) {
    pairs = pairs.filter((p) => p !== pair);
  }

  function fmtPct(sim: number): string {
    return `${Math.round(sim * 100)}%`;
  }
</script>

<div class="flex items-baseline gap-3 mb-4 flex-wrap">
  <p class="text-sm text-dim flex-1">
    Likely-duplicate task pairs by text similarity. Pick one to keep — the other is dropped.
  </p>
  {#if loading}
    <span class="text-[11px] text-dim italic flex-shrink-0">scanning…</span>
  {:else if attempted}
    <span class="text-[11px] text-dim font-mono tabular-nums flex-shrink-0">
      {pairs.length} pair{pairs.length === 1 ? '' : 's'} · scanned {scanned}
    </span>
    <button
      onclick={() => void run()}
      class="px-3 py-1.5 text-xs bg-surface1 text-secondary rounded hover:bg-surface2 flex-shrink-0"
      title="Re-scan open tasks"
    >↻ re-scan</button>
  {/if}
</div>

{#if error}
  <div class="mb-5 p-3 bg-surface0 border border-error rounded text-xs text-error">
    {error}
  </div>
{/if}

{#if attempted && pairs.length === 0 && !error}
  <div class="p-6 bg-surface0 border border-surface1 rounded text-center text-sm text-dim">
    No duplicate pairs found. Nice list.
  </div>
{/if}

{#if pairs.length > 0}
  <ul class="space-y-3">
    {#each pairs as pair (pair.a.id + '|' + pair.b.id)}
      {@const merging = mergingId === pair.a.id || mergingId === pair.b.id}
      <li class="bg-surface1 border border-surface2 rounded p-3">
        <div class="flex items-center justify-between mb-2 gap-2">
          <span class="text-[10px] uppercase tracking-wider text-secondary font-semibold">
            similarity <span class="font-mono tabular-nums">{fmtPct(pair.similarity)}</span>
          </span>
          <button
            onclick={() => skip(pair)}
            disabled={merging}
            class="text-[10px] text-dim hover:text-text disabled:opacity-50"
            title="Not duplicates — dismiss this pair without changing either task"
          >not duplicates</button>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
          {#each [pair.a, pair.b] as t (t.id)}
            <div class="p-2 bg-mantle border border-surface1 rounded flex flex-col gap-1.5">
              <div class="text-sm text-text break-words">{t.text}</div>
              <div class="flex items-baseline gap-2 text-[10px] text-dim font-mono">
                {#if t.priority}<span>P{t.priority}</span>{/if}
                {#if t.dueDate}<span>due {t.dueDate}</span>{/if}
                {#if t.notePath}<span class="truncate" title={t.notePath}>{t.notePath}</span>{/if}
              </div>
              <button
                onclick={() => void merge(pair, t.id)}
                disabled={merging}
                class="px-2 py-1 text-xs bg-success/15 text-success rounded hover:bg-success/25 disabled:opacity-50"
                title="Keep this task; drop the other"
              >{mergingId === t.id ? '…' : 'keep this'}</button>
            </div>
          {/each}
        </div>
      </li>
    {/each}
  </ul>
  <p class="text-[10px] text-dim italic mt-3">
    Deterministic scan — no AI cost. Threshold is 60% Jaccard token overlap. Drop = done + triage=dropped, same as the triage flow.
  </p>
{/if}
