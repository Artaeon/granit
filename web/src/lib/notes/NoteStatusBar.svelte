<script lang="ts">
  // Note editor footer status bar — counts (words / chars / lines),
  // reading time, cursor position, last-saved relative time, optional
  // word-goal progress chip. Lifted out of routes/notes/[...path]/+page
  // as part of the 2026-05-28 god-file decomposition. No behaviour
  // change: every derived value is computed by the caller and handed
  // in as a prop so the editor page stays the single source of truth
  // for cursor / save / word-goal state.
  //
  // Visible at every breakpoint; mobile drops the chars / lines / cursor
  // details and shows the autocomplete hint instead.

  interface Props {
    wordCount: number;
    charCount: number;
    lineCount: number;
    readingMinutes: number;
    wordGoal: number | null;
    wordGoalPct: number;
    cursorLine: number;
    cursorCol: number;
    cursorSelLen: number;
    viewMode: 'edit' | 'preview' | 'split';
    lastSavedAt: number | null;
    lastSavedDisplay: string;
  }

  let {
    wordCount,
    charCount,
    lineCount,
    readingMinutes,
    wordGoal,
    wordGoalPct,
    cursorLine,
    cursorCol,
    cursorSelLen,
    viewMode,
    lastSavedAt,
    lastSavedDisplay
  }: Props = $props();
</script>

<footer
  class="px-3 py-1.5 border-t border-surface1 text-[11px] text-dim flex items-center gap-3 flex-wrap"
  style="padding-bottom: max(0.375rem, env(safe-area-inset-bottom));"
>
  {#if wordGoal}
    <!-- Word-count goal progress: chip + tiny progress bar
         surfaces a writing target set in frontmatter
         (target_words: 1500). When the goal is hit, palette
         flips to success so the user sees the win. -->
    <span class="inline-flex items-baseline gap-1.5 font-mono tabular-nums">
      <span class={wordCount >= wordGoal ? 'text-success font-semibold' : 'text-text'}>
        {wordCount.toLocaleString()}/{wordGoal.toLocaleString()}
      </span>
      <span class="text-dim">words</span>
      <span class="inline-block w-12 h-1 rounded bg-surface1 overflow-hidden align-middle relative">
        <span
          class="absolute inset-y-0 left-0 {wordCount >= wordGoal ? 'bg-success' : 'bg-primary'}"
          style="width: {wordGoalPct}%"
        ></span>
      </span>
      <span class="text-dim">{wordGoalPct}%</span>
    </span>
  {:else}
    <span class="font-mono tabular-nums">{wordCount} words</span>
  {/if}
  <span class="hidden sm:inline opacity-60">·</span>
  <span class="hidden sm:inline font-mono tabular-nums">{charCount.toLocaleString()} chars</span>
  <span class="hidden md:inline opacity-60">·</span>
  <span class="hidden md:inline font-mono tabular-nums">{lineCount} lines</span>
  {#if wordCount >= 50}
    <span class="opacity-60">·</span>
    <span>{readingMinutes} min read</span>
  {/if}
  {#if viewMode !== 'preview'}
    <span class="hidden sm:inline opacity-60">·</span>
    <span class="hidden sm:inline font-mono tabular-nums">
      Ln {cursorLine}, Col {cursorCol}{#if cursorSelLen > 0} · {cursorSelLen} sel{/if}
    </span>
  {/if}
  <span class="flex-1"></span>
  {#if lastSavedAt}
    <span class="hidden sm:inline">Saved {lastSavedDisplay}</span>
  {/if}
  <span class="md:hidden opacity-60">[[ autocomplete · ⌘-click links</span>
</footer>
