<!--
  AIOverlayMentionedRefs — the chip strip just above the composer
  that surfaces the @-mentioned entities that will be attached to the
  next send. Each chip carries the kind label + truncated title +
  an × to drop one. Cleared automatically by the parent after the
  message goes out, so this component never sees a "stale ref"
  state — empty array → renders nothing.
-->
<script lang="ts">
  import type { MentionRef } from '$lib/components/MentionPicker.svelte';

  type Props = {
    refs: MentionRef[];
    onRemove: (idx: number) => void;
  };
  let { refs, onRemove }: Props = $props();
</script>

{#if refs.length > 0}
  <div class="border-t border-surface1 px-4 py-1.5 flex flex-wrap gap-1 text-[11px] flex-shrink-0">
    <span class="text-dim self-center">refs:</span>
    {#each refs as r, i (r.kind + ':' + r.id + ':' + i)}
      <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-secondary text-on-primary">
        <span class="text-[9px] uppercase tracking-wider">{r.kind}</span>
        <span class="truncate max-w-[10rem]" title={r.title}>{r.title}</span>
        <button
          type="button"
          onclick={() => onRemove(i)}
          class="hover:text-error leading-none"
          aria-label="Remove reference"
        >×</button>
      </span>
    {/each}
  </div>
{/if}
