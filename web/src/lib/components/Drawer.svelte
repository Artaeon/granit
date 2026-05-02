<script lang="ts">
  // Slide-in drawer used in two modes:
  //   - default (responsive=false): mobile-only — hidden on md+. Use this
  //     for navigation/tree drawers where a permanent desktop sidebar
  //     sits next to the main view, and the drawer is just the phone
  //     fallback.
  //   - responsive=true: visible on all sizes. Use this for detail
  //     panels (agent runs, task details, create-project forms, event
  //     details) that always slide in over the page regardless of width.
  let {
    open = $bindable(false),
    side = 'left',
    responsive = false,
    width = 'w-72',
    children
  }: {
    open?: boolean;
    side?: 'left' | 'right';
    /** When true, the drawer renders at all breakpoints. Default keeps
     *  the legacy mobile-only behavior so consumers paired with a
     *  desktop sidebar don't double-render. */
    responsive?: boolean;
    /** Tailwind width class. Default w-72 fits a tree/menu;
     *  detail panels (agent runs, task forms) override to a wider class
     *  like sm:w-96 md:w-[28rem] for a comfortable reading column. */
    width?: string;
    children: import('svelte').Snippet;
  } = $props();

  function close() { open = false; }
  function onKey(e: KeyboardEvent) { if (e.key === 'Escape') close(); }

  let posClass = $derived(side === 'left' ? 'left-0' : 'right-0');
  let translateClass = $derived(
    side === 'left' ? (open ? 'translate-x-0' : '-translate-x-full') : (open ? 'translate-x-0' : 'translate-x-full')
  );
  // md:hidden hides the drawer on md+ when responsive=false. Empty
  // string lets it render everywhere when responsive=true.
  let hideClass = $derived(responsive ? '' : 'md:hidden');
  let borderSide = $derived(side === 'left' ? 'border-r' : 'border-l');
</script>

<!-- Backdrop -->
{#if open}
  <button
    onclick={close}
    onkeydown={onKey}
    aria-label="close"
    class="{hideClass} fixed inset-0 z-40 bg-black/50 transition-opacity"
  ></button>
{/if}

<!-- Drawer panel -->
<aside
  class="{hideClass} fixed inset-y-0 {posClass} z-50 {width} max-w-[95vw] bg-mantle {borderSide} border-surface1 shadow-2xl transition-transform duration-200 overflow-y-auto {translateClass}"
  aria-hidden={!open}
>
  {@render children()}
</aside>
