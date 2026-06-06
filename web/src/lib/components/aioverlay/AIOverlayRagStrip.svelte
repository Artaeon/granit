<!--
  AIOverlayRagStrip — grounding attribution. Shows which vault notes
  the assistant pulled into the last turn's prompt so the user can
  verify what shaped the answer. Click any title to open the actual
  note. Empty list → component renders nothing.

  Distinct from per-message Sources expanders inside ChatMessageList
  (those carry per-turn hits): this row reflects the MOST RECENT
  turn's hits and lives near the composer so the user knows what
  context the NEXT send would inherit if they pin / regen.
-->
<script lang="ts">
  import type { RagHit } from '$lib/chat/rag';

  type Props = { hits: RagHit[] };
  let { hits }: Props = $props();
</script>

{#if hits.length > 0}
  <div class="border-t border-surface1 px-4 py-1.5 flex items-center gap-1.5 flex-wrap text-[11px] flex-shrink-0">
    <span class="text-dim">retrieved:</span>
    {#each hits as h (h.path)}
      <a
        href="/notes/{encodeURIComponent(h.path)}"
        class="text-secondary hover:underline truncate max-w-[12rem]"
        title={h.path}
      >{h.title}</a>
    {/each}
  </div>
{/if}
