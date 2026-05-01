<script lang="ts">
  let { body, onJump }: { body: string; onJump?: (line: number) => void } = $props();

  interface Heading {
    level: number;
    text: string;
    line: number;
  }

  let headings = $derived.by((): Heading[] => {
    const out: Heading[] = [];
    const lines = body.split('\n');
    let inFence = false;
    for (let i = 0; i < lines.length; i++) {
      const ln = lines[i];
      if (ln.match(/^```/)) inFence = !inFence;
      if (inFence) continue;
      const m = ln.match(/^(#{1,6})\s+(.+?)\s*#*$/);
      if (m) {
        out.push({ level: m[1].length, text: m[2].trim(), line: i + 1 });
      }
    }
    return out;
  });
</script>

{#if headings.length === 0}
  <div class="text-xs text-dim italic px-2 py-1">no headings</div>
{:else}
  <ul class="space-y-px text-sm">
    {#each headings as h}
      <li>
        <button
          type="button"
          onclick={() => onJump?.(h.line)}
          class="w-full text-left py-1 px-2 rounded text-text hover:bg-surface0 truncate"
          style="padding-left: {0.5 + (h.level - 1) * 0.75}rem; font-size: {h.level === 1 ? '0.875rem' : '0.8125rem'}; opacity: {1 - (h.level - 1) * 0.08};"
        >
          {h.text}
        </button>
      </li>
    {/each}
  </ul>
{/if}
