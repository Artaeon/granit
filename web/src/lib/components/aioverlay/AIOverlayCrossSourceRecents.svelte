<!--
  AIOverlayCrossSourceRecents — the "from notes:" chip row that
  surfaces recent prompts the user wrote in the inline AI menu on
  a note. Lets a user pick up where they left off in a different
  context without retyping.

  Visibility gate sits in the parent (only on a fresh thread with
  an empty composer); this component just renders the chips when
  given a non-empty list. Clicking a chip loads the prompt into the
  composer for review — we don't auto-send because the inline
  menu's note context may not apply over here.
-->
<script lang="ts">
  import type { RecentPrompt } from '$lib/ai/recentPrompts';

  type Props = {
    prompts: RecentPrompt[];
    onPick: (prompt: string) => void;
  };
  let { prompts, onPick }: Props = $props();
</script>

{#if prompts.length > 0}
  <div class="border-t border-surface1 px-4 py-1.5 flex flex-wrap items-center gap-1 text-[11px] flex-shrink-0">
    <span class="text-dim self-center" title="recent prompts from the inline AI menu in your notes">from notes:</span>
    {#each prompts as r, i (r.prompt + ':' + i)}
      <button
        type="button"
        onclick={() => onPick(r.prompt)}
        class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-subtext max-w-[14rem] truncate"
        title={r.notePath ? `from ${r.notePath}: ${r.prompt}` : r.prompt}
      >↗ {r.prompt}</button>
    {/each}
  </div>
{/if}
