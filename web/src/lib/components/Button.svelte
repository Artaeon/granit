<!--
  Button — the one button primitive for granit's UI.

  Before this, ~187 files hand-rolled near-identical Tailwind class
  strings for every button (bg-surface0 / border-surface1 / hover…),
  which is why toolbars drifted into a mix of black-filled, gray-filled,
  outlined, and plain-text buttons with no clear hierarchy. This
  component collapses that into a small, named set so an action always
  reads the same everywhere.

  Variants (intent, not colour):
    primary    the single strongest call-to-action on a surface
    secondary  a normal bordered button — the default
    ghost      low-emphasis / toolbar action (no border until hover)
    danger     destructive (delete / close)

  active=true marks a toggle/segmented button as selected — it adopts
  the primary fill so "where am I" is obvious. (Ignored by primary /
  danger, which have no on/off notion.)

  Keeps the Power-UI aesthetic: zero border-radius (the global radius
  tokens already resolve `rounded` to 0), dense padding, and now a
  consistent focus-visible ring every button was previously missing.

  Renders an <a> when `href` is set, otherwise a <button>. Content is a
  snippet, so callers compose their own icon + label inside.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';
  type Size = 'sm' | 'md';

  let {
    variant = 'secondary',
    size = 'md',
    active = false,
    iconOnly = false,
    href = undefined,
    type = 'button',
    disabled = false,
    title = undefined,
    class: extra = '',
    onclick = undefined,
    children,
    ...rest
  }: {
    variant?: Variant;
    size?: Size;
    /** Selected state for toggles / segmented controls — adopts the
     *  primary fill. No-op for primary/danger. */
    active?: boolean;
    /** Square padding for single-glyph icon buttons. */
    iconOnly?: boolean;
    href?: string;
    type?: 'button' | 'submit' | 'reset';
    disabled?: boolean;
    title?: string;
    class?: string;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
    [key: string]: unknown;
  } = $props();

  // Layout + behaviour shared by every button. `inline-flex` is the
  // default display, but it's omitted when the caller passes their own
  // display utility (e.g. `hidden sm:inline-flex` for responsive
  // visibility) so the override actually wins — Tailwind can't reliably
  // arbitrate two same-specificity `display` rules otherwise.
  const layout = 'items-center justify-center gap-1.5 font-medium leading-none whitespace-nowrap transition-colors select-none ' +
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 focus-visible:ring-offset-0 ' +
    'disabled:opacity-40 disabled:pointer-events-none';
  let hasDisplay = $derived(/(^|\s)(hidden|block|inline-block|inline|flex|inline-flex|grid|contents)(\s|$)/.test(extra));
  let base = $derived(hasDisplay ? layout : `inline-flex ${layout}`);

  const sizes: Record<Size, string> = {
    sm: iconOnly ? 'text-xs p-1' : 'text-xs px-2 py-1',
    md: iconOnly ? 'text-sm p-1.5' : 'text-xs px-2.5 py-1.5'
  };

  // Resting look per variant. `active` short-circuits secondary/ghost to
  // the selected (primary-fill) look.
  const variants: Record<Variant, string> = {
    primary: 'bg-primary text-on-primary hover:opacity-90',
    secondary: 'bg-surface0 border border-surface1 text-subtext hover:bg-surface1 hover:text-text',
    ghost: 'text-subtext hover:bg-surface1 hover:text-text',
    danger: 'text-error hover:bg-error/10'
  };
  const activeCls = 'bg-primary text-on-primary border border-transparent hover:opacity-90';

  let look = $derived(
    active && (variant === 'secondary' || variant === 'ghost') ? activeCls : variants[variant]
  );
  let cls = $derived(`${base} ${sizes[size]} ${look} ${extra}`);
</script>

{#if href}
  <a {href} {title} class={cls} aria-disabled={disabled || undefined} {...rest}>
    {@render children?.()}
  </a>
{:else}
  <button {type} {title} {disabled} {onclick} class={cls} {...rest}>
    {@render children?.()}
  </button>
{/if}
