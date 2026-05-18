<script lang="ts">
  // Shared modal framing for the small "edit one record" forms that
  // /people, /objects, /finance, /measurements etc. each implemented
  // from scratch. Every one had its own backdrop + center + click-
  // outside + Escape boilerplate, and the spacing/radius/shadow
  // drifted between modules over time.
  //
  // This component is intentionally framing-only — no form, no save
  // button, no field layout. The consumer keeps the form element and
  // submit handler they already had; they just stop maintaining the
  // backdrop and Escape wiring. Three reasons:
  //   1. Form fields differ wildly (free text vs. date pickers vs.
  //      multi-select), so a "generic form modal" abstraction would
  //      cost more than it saves.
  //   2. Submit semantics differ (some create, some patch, some
  //      route through a draft store) — the consumer is the right
  //      place to own that branching.
  //   3. The footer button row already varies (Cancel/Save, Save+Delete,
  //      single-action quick-create). Reproducing all of those in
  //      props would balloon the API.
  //
  // What we standardise instead: backdrop + centering + click-outside +
  // Escape + the optional title header. Everything else is the
  // consumer's children block.

  import { onMount } from 'svelte';
  import type { Snippet } from 'svelte';

  type MaxWidth = 'xs' | 'sm' | 'md' | 'lg';

  type Props = {
    open: boolean;
    title?: string;
    maxWidth?: MaxWidth;
    onClose: () => void;
    children: Snippet;
  };

  let { open = $bindable(), title, maxWidth = 'md', onClose, children }: Props = $props();

  const widthClass: Record<MaxWidth, string> = {
    xs: 'max-w-xs',
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-lg'
  };

  // Esc-to-close listens on the window when the modal is open so the
  // key still fires when focus lives inside an input/textarea (those
  // swallow keydown otherwise via stopPropagation in the consumer).
  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!open) return;
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
</script>

{#if open}
  <!-- The backdrop dismisses on click; Escape is wired on the window
       above so there's no keyboard parity needed on the backdrop
       element itself. -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={onClose}
    role="dialog"
    aria-modal="true"
    tabindex="-1"
  >
    <!-- Inner card stops the click from bubbling so clicks inside
         the form don't dismiss the modal. svelte-ignore the
         "non-interactive element has handler" warning because the
         handler exists solely to stop propagation, not to act. -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      role="document"
      class="w-full {widthClass[maxWidth]} bg-mantle border border-surface1 rounded-lg shadow-xl"
    >
      {#if title}
        <header class="px-4 py-2.5 border-b border-surface1">
          <h2 class="text-base font-semibold text-text">{title}</h2>
        </header>
      {/if}
      {@render children()}
    </div>
  </div>
{/if}
