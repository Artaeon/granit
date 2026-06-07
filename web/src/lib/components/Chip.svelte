<!--
  Chip — the toggle-pill primitive (sibling to Button).

  Filter/preset/segment pills recur all over the app (tasks' All/Today/
  Overdue/P1/No-date/Done, goals' status filters, calendar event-types,
  notes quick-filters). Each hand-rolled the same long conditional class
  string: an idle look (surface0 + tinted text + hover border) that
  fills with a semantic colour when active. Chip captures that in two
  axes — `tone` (the semantic colour) and `active` (filled vs. outline)
  — so a filter pill is one short tag instead of a 120-char ternary.

  tone:
    neutral  the default / "All"-style pill — fills primary when active
    warning  amber  (e.g. due today)
    error    red    (e.g. overdue, P1)
    info     blue   (e.g. no date)
    success  green  (e.g. done)
    muted    grey   (e.g. archived / retired)

  Count badges, dots, and icons stay as children — the caller still owns
  those, and can read `active` to flip their own colours.

  Keeps the zero-radius aesthetic and adds the focus-visible ring the
  hand-rolled chips were missing.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  type Tone = 'neutral' | 'warning' | 'error' | 'info' | 'success' | 'muted';
  type Size = 'sm' | 'md';

  let {
    tone = 'neutral',
    size = 'md',
    active = false,
    disabled = false,
    title = undefined,
    onclick = undefined,
    class: extra = '',
    children,
    ...rest
  }: {
    tone?: Tone;
    /** md = default filter-row pill; sm = compact (narrow sidebars). */
    size?: Size;
    active?: boolean;
    disabled?: boolean;
    title?: string;
    onclick?: (e: MouseEvent) => void;
    class?: string;
    children?: Snippet;
    [key: string]: unknown;
  } = $props();

  const sizes: Record<Size, string> = {
    sm: 'gap-1 px-2 py-0.5 text-[11px]',
    md: 'gap-1.5 px-2.5 py-1 text-xs'
  };
  const base =
    'inline-flex items-center rounded-md border font-medium whitespace-nowrap transition-colors select-none ' +
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 ' +
    'disabled:opacity-40 disabled:pointer-events-none';

  // Idle = outline with tinted text; active = filled with the tone.
  const idle: Record<Tone, string> = {
    neutral: 'bg-surface0 text-subtext border-surface1 hover:border-primary hover:text-text',
    warning: 'bg-surface0 text-warning border-surface1 hover:border-warning',
    error: 'bg-surface0 text-error border-surface1 hover:border-error',
    info: 'bg-surface0 text-info border-surface1 hover:border-info',
    success: 'bg-surface0 text-success border-surface1 hover:border-success',
    muted: 'bg-surface0 text-dim border-surface1 hover:border-surface2 hover:text-subtext'
  };
  const filled: Record<Tone, string> = {
    neutral: 'bg-primary text-on-primary border-primary',
    warning: 'bg-warning text-mantle border-warning',
    error: 'bg-error text-mantle border-error',
    info: 'bg-info text-mantle border-info',
    success: 'bg-success text-mantle border-success',
    muted: 'bg-surface2 text-text border-surface2'
  };

  let cls = $derived(`${base} ${sizes[size]} ${active ? filled[tone] : idle[tone]} ${extra}`);
</script>

<button type="button" {title} {disabled} {onclick} aria-pressed={active} class={cls} {...rest}>
  {@render children?.()}
</button>
