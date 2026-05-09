// Shared color palette for the marginalia + book-highlight system.
// Three surfaces consume this:
//
//   - AnnotationsPanel — the editor right-rail margin notes
//   - RecentAnnotationsWidget — the dashboard "what did I think
//     about" tile
//   - Books reader — passage highlights inside chapter HTML
//
// Each surface used to ship its own color helpers, with the
// literal `'yellow' | 'blue' | 'green' | 'pink'` union retyped
// inline in ~3 places. That made it easy for one site to drift
// (e.g. the books reader's toolbar swatches once didn't match
// the marginalia panel) and for a future palette change to miss
// a surface. Centralising means one edit, one source of truth.

/** The four user-facing color tokens. Stable across saves — the
 *  string lands in the on-disk annotation + highlight records, so
 *  renaming requires a migration. */
export type AnnotationColor = 'yellow' | 'blue' | 'green' | 'pink';

/** Ordered for the picker swatch row — same visual order across
 *  the editor selection toolbar, the books-reader toolbar, and
 *  the AnnotationsPanel composer. */
export const ANNOTATION_COLORS: AnnotationColor[] = ['yellow', 'blue', 'green', 'pink'];

/** Default color when the user picks nothing or stored data is
 *  empty / unknown. Yellow because it's the most common physical
 *  highlighter color — least surprise as a fallback. */
export const DEFAULT_ANNOTATION_COLOR: AnnotationColor = 'yellow';

/** Type-guard used at the boundary where a string from the wire /
 *  storage becomes a typed AnnotationColor. Everywhere downstream
 *  can then trust the value without re-validating. */
export function isAnnotationColor(s: string | undefined | null): s is AnnotationColor {
  return s === 'yellow' || s === 'blue' || s === 'green' || s === 'pink';
}

/** Coerce an arbitrary string (or undefined) to an AnnotationColor,
 *  defaulting on miss. Lets callers go from API JSON → typed value
 *  in one step. */
export function asAnnotationColor(s: string | undefined | null): AnnotationColor {
  return isAnnotationColor(s) ? s : DEFAULT_ANNOTATION_COLOR;
}

/** Tailwind class for a left-border tint at 400 weight. Used by
 *  the annotation cards in AnnotationsPanel + the bookmarks list
 *  in the books reader. Pass the raw string from storage —
 *  unknown / missing values fall to yellow. */
export function annotationBorderClass(c?: string): string {
  switch (c) {
    case 'blue':  return 'border-l-blue-400';
    case 'green': return 'border-l-green-400';
    case 'pink':  return 'border-l-pink-400';
    default:      return 'border-l-yellow-400';
  }
}

/** Tailwind class for a solid background at 400 weight. Used by
 *  the dashboard widget's column-bar accent. */
export function annotationBarClass(c?: string): string {
  switch (c) {
    case 'blue':  return 'bg-blue-400';
    case 'green': return 'bg-green-400';
    case 'pink':  return 'bg-pink-400';
    default:      return 'bg-yellow-400';
  }
}

/** Tailwind classes for the color-picker swatch buttons — solid
 *  background at 300 weight (lighter, reads as a swatch rather
 *  than an active state) plus black text for readability when
 *  the swatch is large enough to carry a label. */
export function annotationSwatchClass(c: AnnotationColor): string {
  switch (c) {
    case 'blue':  return 'bg-blue-300 text-black';
    case 'green': return 'bg-green-300 text-black';
    case 'pink':  return 'bg-pink-300 text-black';
    default:      return 'bg-yellow-300 text-black';
  }
}

/** Hex value for the inline style="background: …" of the small
 *  selected-state swatch ring in composers. The Tailwind class
 *  approach can't quite reach the dynamic style we want
 *  (border-2 border-text vs border-transparent on selection),
 *  so the swatch fill is hex-coded. Kept here so a future palette
 *  shift updates one map instead of a literal in three components. */
export function annotationSwatchHex(c: AnnotationColor): string {
  switch (c) {
    case 'blue':  return '#bfdbfe';
    case 'green': return '#bbf7d0';
    case 'pink':  return '#fbcfe8';
    default:      return '#fde68a';
  }
}
