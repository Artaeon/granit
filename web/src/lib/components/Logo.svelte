<script lang="ts">
  // Granit logomark — chiseled monogram G.
  //
  // The G is drawn as a single filled path (no curves, all straight
  // strokes) so it reads as "carved into stone" rather than "set in
  // type". Mason-wedge notch in the bottom-right of the imaginary
  // slab is the asymmetric signature that distinguishes the mark
  // from every other G-logo on the planet — survives down to 16px
  // where the notch is the only thing the eye can resolve.
  //
  // Renders with currentColor on the letter so it themes-respond —
  // the host element's text color drives the fill. The notch and
  // background are SCOPED to the static icon.svg / maskable.svg
  // variants (where we have a defined backdrop); in-app uses (like
  // the sidebar brand chip) drop the notch + backdrop and let the
  // parent container paint the chip color.
  //
  // The same geometry ships as the SPA favicon + apple-touch-icon
  // PNGs so the mark is consistent across browser tab, install
  // icon, and in-app chip.

  let {
    class: cls = '',
    size = '1em',
    label = 'Granit'
  }: {
    /** Tailwind classes applied to the SVG (e.g. "w-7 h-7"). */
    class?: string;
    /** Inline width/height when no class sets size. */
    size?: string;
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
  <!-- G letter. Single fill path so the counter (negative space
       inside) and outer silhouette stay inseparable. fill="current
       Color" means the host's text color drives the mark. -->
  <path
    fill="currentColor"
    d="M 130 96 L 382 96 L 382 184 L 218 184 L 218 328 L 312 328 L 312 280 L 250 280 L 250 224 L 382 224 L 382 416 L 130 416 Z"
  />
</svg>
