<script lang="ts">
  // Granit logomark — faceted hexagonal stone.
  //
  // Six-vertex outline + three internal facet lines (cap base, cap
  // median, body median) so the silhouette reads as a four-facet cut
  // gem rather than a flat hexagon with a hat. Picks up on the
  // "granit" name without being literal about granite.
  //
  // Renders with currentColor so it themes-respond — the host
  // element's text color drives the stroke. Pair with a tinted
  // background (the sidebar chip uses bg-primary/15) for the
  // "logo plate" look, or use bare for a flat mark.
  //
  // The same geometry ships as the SPA favicon + apple-touch-icon
  // PNGs so the mark is consistent across browser tab, install
  // icon, and in-app chip.

  let {
    class: cls = '',
    size = '1em',
    strokeWidth = 32,
    label = 'Granit'
  }: {
    /** Tailwind classes applied to the SVG (e.g. "w-7 h-7"). */
    class?: string;
    /** Inline width/height when no class sets size. */
    size?: string;
    /** Stroke width in 512×512 viewBox units; tune up for small
     *  rendered sizes so the silhouette stays confident. */
    strokeWidth?: number;
    /** Accessible label. Set to '' for purely decorative use. */
    label?: string;
  } = $props();
</script>

<svg
  xmlns="http://www.w3.org/2000/svg"
  viewBox="0 0 512 512"
  class={cls}
  width={cls ? undefined : size}
  height={cls ? undefined : size}
  role={label ? 'img' : 'presentation'}
  aria-label={label || undefined}
  aria-hidden={label ? undefined : 'true'}
>
  <g fill="none" stroke="currentColor" stroke-width={strokeWidth} stroke-linejoin="round" stroke-linecap="round">
    <!-- Outer stone outline: top apex, top shoulders, bottom shoulders, bottom apex. -->
    <path d="M 256 76 L 432 184 L 432 360 L 256 436 L 80 360 L 80 184 Z" />
    <!-- Cap base — splits the top facet from the body. -->
    <path d="M 80 184 L 432 184" />
    <!-- Cap median — divides the top into two triangular facets. -->
    <path d="M 256 76 L 256 184" />
    <!-- Body median — mirrors the cap median through the body so the
         silhouette reads as four facets total (two cap + two body)
         instead of two-cap + flat-body. Subtle but turns it from
         "hexagon with hat" into "cut gemstone". -->
    <path d="M 256 184 L 256 436" />
  </g>
</svg>
