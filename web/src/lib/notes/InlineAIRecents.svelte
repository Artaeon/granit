<!--
  InlineAIRecents — the recent-prompts row above the action list.

  Two strips: top is per-note history (up to 3 pills), bottom is
  cross-source recents from the Cmd+J chat overlay (so prompts the
  user wrote in conversation are one click away here). Clicking a
  pill fills the parent's prompt input and immediately fires it as
  a custom Ask — the parent passes a single `run(text)` callback
  that does both.

  Hidden when there's nothing to show OR the user is mid-type — see
  the parent's {#if} guard.
-->
<script lang="ts">
  interface Props {
    /** Per-note recents, most-recent first. Sliced to 3 here. */
    history: string[];
    /** Chat-overlay recents the per-note list hasn't already seen. */
    crossRecents: { prompt: string }[];
    /** Apply a recent prompt — typically prefill the parent's input
     *  and run it as a custom Ask in one shot. */
    run: (prompt: string) => void;
  }
  let { history, crossRecents, run }: Props = $props();
</script>

<div class="px-2 py-1 border-b border-surface1 space-y-0.5">
  {#if history.length > 0}
    <div class="flex flex-wrap items-center gap-1">
      <span class="text-[10px] text-dim font-mono uppercase tracking-wider">recent:</span>
      {#each history.slice(0, 3) as h, i (h + ':' + i)}
        <button
          type="button"
          onclick={() => run(h)}
          class="text-[11px] px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-text max-w-[12rem] truncate"
          title={h}
        >{h}</button>
      {/each}
    </div>
  {/if}
  {#if crossRecents.length > 0}
    <div class="flex flex-wrap items-center gap-1">
      <span class="text-[10px] text-dim font-mono uppercase tracking-wider" title="from the Cmd+J chat sidebar">from chat:</span>
      {#each crossRecents as r, i (r.prompt + ':' + i)}
        <button
          type="button"
          onclick={() => run(r.prompt)}
          class="text-[11px] px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-subtext max-w-[12rem] truncate"
          title={r.prompt}
        >↗ {r.prompt}</button>
      {/each}
    </div>
  {/if}
</div>
