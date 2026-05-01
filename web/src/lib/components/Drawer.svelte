<script lang="ts">
  // A mobile-only slide-in drawer. Hidden on md+. The caller is expected to
  // render an additional desktop sidebar separately (with `hidden md:flex`).
  let {
    open = $bindable(false),
    side = 'left',
    children
  }: {
    open?: boolean;
    side?: 'left' | 'right';
    children: import('svelte').Snippet;
  } = $props();

  function close() { open = false; }
  function onKey(e: KeyboardEvent) { if (e.key === 'Escape') close(); }

  let posClass = $derived(side === 'left' ? 'left-0' : 'right-0');
  let translateClass = $derived(
    side === 'left' ? (open ? 'translate-x-0' : '-translate-x-full') : (open ? 'translate-x-0' : 'translate-x-full')
  );
</script>

<!-- Backdrop -->
{#if open}
  <button
    onclick={close}
    onkeydown={onKey}
    aria-label="close"
    class="md:hidden fixed inset-0 z-40 bg-black/50 transition-opacity"
  ></button>
{/if}

<!-- Drawer panel -->
<aside
  class="md:hidden fixed inset-y-0 {posClass} z-50 w-72 max-w-[85vw] bg-mantle border-r border-surface1 shadow-2xl transition-transform duration-200 overflow-y-auto {translateClass}"
  aria-hidden={!open}
>
  {@render children()}
</aside>
