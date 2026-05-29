<script lang="ts">
  // Slim page header for /jots. Stream AA — mirrors the tasks/notes
  // page-header pattern: a single dense bar with title + live counters
  // (streak / jots / tags / words) and a `?` shortcuts toggle.
  //
  // Everything else that used to live in the old multi-row header
  // (date jump, search, AI buttons, "today", filter chips, composer)
  // now lives in sibling components rendered immediately below this
  // strip so the chrome stays narrow.

  type Props = {
    streakDays: number;
    jotsCount: number;
    tagsCount: number;
    loadedWords: number;
    onToggleHelp: () => void;
  };

  let {
    streakDays,
    jotsCount,
    tagsCount,
    loadedWords,
    onToggleHelp
  }: Props = $props();

  // Compact formatter — k-suffix for ≥1000. Glanceable, not editor-stat.
  function fmt(n: number): string {
    if (n >= 1000) return `${(n / 1000).toFixed(n >= 10_000 ? 0 : 1)}k`;
    return String(n);
  }
</script>

<header
  class="flex items-baseline gap-2 mb-1.5 text-[11px] text-dim border-b border-surface1 pb-1.5"
>
  <span class="text-[13px] font-semibold uppercase tracking-[0.18em] text-text">Jots</span>
  {#if streakDays > 0}
    <span
      class="font-mono text-text"
      title="consecutive days ending today with a daily note loaded"
    >{streakDays}d streak</span>
  {/if}
  {#if jotsCount > 0}
    <span class="opacity-50">·</span>
    <span class="font-mono" title="loaded across all pages">
      {fmt(jotsCount)} jots
      {#if tagsCount > 0}
        · {fmt(tagsCount)} tags
      {/if}
      {#if loadedWords > 0}
        · {fmt(loadedWords)} words
      {/if}
    </span>
  {/if}
  <button
    type="button"
    onclick={onToggleHelp}
    class="ml-auto opacity-60 hidden sm:inline font-mono hover:opacity-100 hover:text-text"
    title="show keyboard shortcuts (press ?)"
  >? shortcuts</button>
</header>
