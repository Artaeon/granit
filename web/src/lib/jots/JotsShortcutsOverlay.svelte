<script lang="ts">
  // ? cheat-sheet overlay for /jots. Same backdrop-dismiss pattern as
  // the rest of the app. Esc is handled by the page-level shortcut
  // handler (so a single Esc both closes this overlay and clears
  // filters when it isn't open).
  type Props = { onClose: () => void };
  let { onClose }: Props = $props();
</script>

<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
  onclick={(e) => { if (e.currentTarget === e.target) onClose(); }}
  role="presentation"
>
  <div class="bg-surface0 border border-surface1 rounded shadow-xl max-w-sm w-full p-4 text-text">
    <div class="flex items-baseline mb-2">
      <h3 class="text-xs uppercase tracking-[0.18em] font-semibold">Keyboard shortcuts</h3>
      <button
        type="button"
        onclick={onClose}
        class="ml-auto text-dim hover:text-text text-sm"
        aria-label="close shortcuts"
      >×</button>
    </div>
    <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-[11px]">
      <dt class="font-mono text-text">j</dt><dd class="text-dim">next jot</dd>
      <dt class="font-mono text-text">k</dt><dd class="text-dim">previous jot</dd>
      <dt class="font-mono text-text">g</dt><dd class="text-dim">top of feed</dd>
      <dt class="font-mono text-text">G</dt><dd class="text-dim">end of feed (load more)</dd>
      <dt class="font-mono text-text">c</dt><dd class="text-dim">focus composer</dd>
      <dt class="font-mono text-text">/</dt><dd class="text-dim">focus search</dd>
      <dt class="font-mono text-text">?</dt><dd class="text-dim">toggle this overlay</dd>
      <dt class="font-mono text-text">Esc</dt><dd class="text-dim">clear filters / blur / dismiss</dd>
      <dt class="font-mono text-text">⏎</dt><dd class="text-dim">save jot (in composer)</dd>
      <dt class="font-mono text-text">⇧⏎</dt><dd class="text-dim">newline in composer</dd>
    </dl>
  </div>
</div>
