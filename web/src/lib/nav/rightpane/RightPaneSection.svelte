<script lang="ts">
  // Shared shell for right-pane content components that follow the
  // (header + scrollable body + footer link) pattern. Tasks / Goals /
  // Habits / Today all wrap to this; Calendar / Notes / Vision have
  // bespoke structures and stay verbatim.
  //
  // Slots/snippets:
  //   headerTrailing — optional snippet rendered after the title +
  //                    badge inside the header (right side, post the
  //                    flex spacer). Tasks uses it for the add-task
  //                    form; Habits could use it for a status pill.
  //   children       — the body. Receives a scrollable parent
  //                    (flex-1 overflow-y-auto min-h-0) so callers
  //                    can render lists or grids without re-wiring.
  //
  // Footer: always a single bordered link row. `footerHref` is
  // required to keep the surface predictable; if a content needs a
  // button instead, fall back to a bespoke layout.
  import type { Snippet } from 'svelte';

  let {
    title,
    badge,
    headerTrailing,
    footerLabel,
    footerHref,
    bodyClass,
    children
  }: {
    title: string;
    badge?: string;
    headerTrailing?: Snippet;
    footerLabel: string;
    footerHref: string;
    /** Tailwind classes for the body wrapper; defaults to standard padding + scroll. */
    bodyClass?: string;
    children: Snippet;
  } = $props();
</script>

<div class="flex flex-col h-full text-sm min-h-0">
  <header class="flex items-baseline gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0">
    <h3 class="text-xs uppercase tracking-wider text-dim font-medium">{title}</h3>
    {#if badge}
      <span class="text-[10px] tabular-nums text-dim">{badge}</span>
    {/if}
    <span class="flex-1"></span>
    {#if headerTrailing}
      {@render headerTrailing()}
    {/if}
  </header>

  <div class={bodyClass ?? 'flex-1 overflow-y-auto px-2 py-2 min-h-0'}>
    {@render children()}
  </div>

  <footer class="border-t border-surface1 px-3 py-1.5 flex-shrink-0">
    <a href={footerHref} class="text-xs text-secondary hover:underline">{footerLabel} →</a>
  </footer>
</div>
