<script lang="ts">
  // Visual shell for the AI Alignment Audit surface on /goals.
  // Surfaces clusters of unlinked tasks that aren't advancing any
  // active goal. Page owns the state, this owns the chrome.
  export interface AuditFinding {
    cluster: string;
    count: number;
    sample: string[];
    observation: string;
    question: string;
  }

  type Props = {
    findings: AuditFinding[];
    dismissed: Set<string>;
    busy: boolean;
    error: string;
    orphanOpenCount: number;
    orphanDoneCount: number;
    linkedCount: number;
    activeGoalsCount: number;
    onAbort: () => void;
    onRetry: () => void;
    onClose: () => void;
    onDismissFinding: (f: AuditFinding) => void;
  };

  let {
    findings,
    dismissed,
    busy,
    error,
    orphanOpenCount,
    orphanDoneCount,
    linkedCount,
    activeGoalsCount,
    onAbort,
    onRetry,
    onClose,
    onDismissFinding
  }: Props = $props();
</script>

<section class="mb-5 bg-surface0 border border-surface1 rounded-lg overflow-hidden">
  <header class="px-4 py-2.5 border-b border-surface1 flex items-center gap-2 flex-wrap">
    <span class="text-sm font-medium text-text">Alignment audit</span>
    <span class="text-[11px] text-dim">
      {orphanOpenCount} unlinked open ·
      {orphanDoneCount} unlinked done in 14d ·
      {linkedCount} linked
    </span>
    <span class="flex-1"></span>
    {#if busy}
      <button
        type="button"
        onclick={onAbort}
        class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
      >Stop</button>
    {:else}
      <button
        type="button"
        onclick={onRetry}
        class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
        title="re-roll the audit"
      >↻ retry</button>
    {/if}
    <a
      href="/tasks"
      class="text-xs text-secondary hover:underline"
      title="Open /tasks to re-link work to a goal"
    >Open tasks →</a>
    <button
      type="button"
      onclick={onClose}
      class="text-xs text-dim hover:text-text px-1"
    >Dismiss</button>
  </header>

  {#if error}
    <div class="px-4 py-2 text-xs text-error bg-surface0 border-b border-error">{error}</div>
  {/if}

  <div class="p-3 space-y-2">
    {#if busy && findings.length === 0}
      <div class="text-xs text-dim italic px-2 py-3 flex items-center gap-2">
        <span class="inline-block w-1.5 h-3 bg-surface0 animate-pulse rounded-sm"></span>
        clustering {orphanOpenCount + orphanDoneCount} unlinked tasks against {activeGoalsCount} active goals…
      </div>
    {:else if findings.length === 0 && !error}
      <div class="text-xs text-dim italic px-2 py-3">No findings yet.</div>
    {:else}
      {#each findings as f (f.cluster + f.observation)}
        {#if !dismissed.has(f.cluster)}
          <article class="p-3 bg-mantle border border-surface1 rounded border-l-4" style="border-left-color: var(--color-warning);">
            <div class="flex items-baseline gap-2 mb-1 flex-wrap">
              <span class="text-sm font-medium text-text flex-1 min-w-0 break-words">{f.cluster}</span>
              <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface0 text-warning tabular-nums">
                {f.count} task{f.count === 1 ? '' : 's'}
              </span>
            </div>
            <p class="text-sm text-subtext leading-snug">{f.observation}</p>
            {#if f.sample.length > 0}
              <ul class="mt-1.5 space-y-0.5 text-[11px] text-dim font-mono">
                {#each f.sample as s}
                  <li class="truncate">— {s}</li>
                {/each}
              </ul>
            {/if}
            <p class="text-sm text-text italic mt-2">"{f.question}"</p>
            <div class="flex items-center gap-2 mt-2 text-[11px]">
              <button
                type="button"
                onclick={() => onDismissFinding(f)}
                class="px-2 py-0.5 bg-surface1 text-subtext rounded hover:bg-surface2"
                title="It was intentional / I've heard you. Hide this finding."
              >Intentional</button>
              <a
                href="/tasks"
                class="ml-auto text-secondary hover:underline"
              >Re-link in /tasks →</a>
            </div>
          </article>
        {/if}
      {/each}
    {/if}
  </div>
</section>
