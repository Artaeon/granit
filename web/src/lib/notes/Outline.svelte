<!--
  Outline (TOC) — derives a heading list from the source body and
  jumps the user to the matching line on click. Two modes of "active
  heading" tracking are supported:

    1. Editor-mode: caller passes `cursorLine` and we light up the
       heading that owns the line the cursor is on (largest line
       number ≤ cursorLine). Cheap, no DOM.

    2. Preview-mode: caller passes `scrollContainer` (a ref to the
       preview pane). We attach an IntersectionObserver to every
       `[data-heading-line]` element it contains, and the heading
       closest to the top of the viewport wins. rAF-throttled so a
       fast scroll doesn't churn renders. Re-attaches when the body
       changes (the rendered DOM is wholesale-replaced by `{@html}`).

  Active heading gets a primary-tinted accent bar + bg so the user
  always knows where they are in a long doc.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import { parseBody, type ParsedHeading } from '$lib/util/bodyParse';

  let {
    body,
    onJump,
    cursorLine,
    scrollContainer,
    visited
  }: {
    body: string;
    onJump?: (line: number) => void;
    /** When set, the heading whose line ≤ cursorLine is highlighted. */
    cursorLine?: number;
    /** Preview pane element. When set, IntersectionObserver tracks
     *  which heading the reader is currently on. */
    scrollContainer?: HTMLElement | null;
    /** Optional per-line "this section was visited" set — drives a
     *  small ✓ tick next to read sections. Provided by the host
     *  page so the persistence layer (localStorage per note path)
     *  lives there rather than here. */
    visited?: Set<number>;
  } = $props();

  type Heading = ParsedHeading;

  // Single shared body parse — Outline, SectionQuestionsPanel, and
  // ResearchPanel each used to run their own full-body split on
  // every keystroke. Even worse, the `infoContent` snippet on the
  // notes route renders into BOTH the desktop aside and the mobile
  // drawer, so on every viewport both copies are mounted and each
  // panel ran twice. The shared parseBody util slots by reference
  // identity so multi-reader requests within one render pass collapse
  // to a single linear scan.
  let headings = $derived(parseBody(body).headings);

  // Active heading from the cursor (edit mode) — pick the heading
  // with the greatest line ≤ cursorLine. Cheap linear scan since
  // headings.length is small (rarely > 50 in practice).
  let cursorActiveLine = $derived.by<number | null>(() => {
    if (cursorLine === undefined) return null;
    let active: number | null = null;
    for (const h of headings) {
      if (h.line <= cursorLine) active = h.line;
      else break;
    }
    return active;
  });

  // Active heading from the preview's IntersectionObserver. Updated
  // imperatively as the user scrolls; reading from the resulting
  // $state inside our derived `activeLine` below means we stay
  // reactive without spinning observers on every body change.
  let observedActiveLine = $state<number | null>(null);

  let activeLine = $derived(observedActiveLine ?? cursorActiveLine);

  // (Re)attach the IntersectionObserver only when the heading
  // STRUCTURE changes — i.e. the set of source line numbers — not on
  // every keystroke. Previously this effect tracked the headings
  // array reference, which a body-derived $derived rebuilds for
  // every body change. On a long preview-mode note that meant
  // disconnecting + reconnecting the observer + re-querying every
  // [data-heading-line] element on every keystroke; the DOM walk
  // alone could blow the per-keystroke budget on slow devices.
  //
  // structuralKey collapses the heading list to a join of line
  // numbers — same key string for the same set of lines, regardless
  // of whether the underlying array got a fresh reference. That
  // matches what the observer actually cares about (the targets it
  // observes are keyed by data-heading-line). When the user edits
  // text WITHIN a section without adding/removing/moving a heading,
  // the key stays identical and the effect doesn't re-run.
  let structuralKey = $derived(headings.map((h) => h.line).join(','));
  $effect(() => {
    const container = scrollContainer ?? null;
    void structuralKey; // dependency — re-run only on heading shape change
    if (!container) return;

    let raf = 0;
    const visibleLines = new Set<number>();
    const lineToTop = new Map<number, number>();

    const recompute = () => {
      raf = 0;
      // Pick the heading with the smallest absolute distance to the
      // viewport top among those visible — i.e. the one most
      // "current" right now. If nothing is visible (between two
      // headings, mid-section), fall back to the largest line whose
      // top is above the viewport top — the section the user is in.
      let best: number | null = null;
      let bestDist = Infinity;
      for (const ln of visibleLines) {
        const top = lineToTop.get(ln);
        if (top === undefined) continue;
        const d = Math.abs(top);
        if (d < bestDist) {
          bestDist = d;
          best = ln;
        }
      }
      if (best === null) {
        // Nothing visible — pick the most recent heading above the
        // top edge.
        let above: number | null = null;
        let aboveTop = -Infinity;
        for (const [ln, top] of lineToTop.entries()) {
          if (top <= 0 && top > aboveTop) {
            aboveTop = top;
            above = ln;
          }
        }
        best = above;
      }
      observedActiveLine = best;
    };

    const onScrollOrResize = () => {
      // Update top offsets cheaply (reading offsetTop is fast; we
      // skip layout-trashing getBoundingClientRect on every tick).
      // Containers scroll independently, so we measure relative to
      // the container's top edge, not the viewport.
      const cTop = container.getBoundingClientRect().top;
      for (const [ln] of lineToTop) {
        const el = container.querySelector(
          `[data-heading-line="${ln}"]`
        ) as HTMLElement | null;
        if (!el) continue;
        lineToTop.set(ln, el.getBoundingClientRect().top - cTop);
      }
      if (raf === 0) raf = requestAnimationFrame(recompute);
    };

    const obs = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const ln = parseInt(
            (entry.target as HTMLElement).dataset.headingLine ?? '',
            10
          );
          if (!Number.isFinite(ln)) continue;
          if (entry.isIntersecting) visibleLines.add(ln);
          else visibleLines.delete(ln);
          // Refresh the cached top so recompute picks the correct
          // "closest to top" heading. We can't trust observer's
          // boundingClientRect because it's frozen at fire time.
          const el = entry.target as HTMLElement;
          lineToTop.set(ln, el.getBoundingClientRect().top - container.getBoundingClientRect().top);
        }
        if (raf === 0) raf = requestAnimationFrame(recompute);
      },
      {
        root: container,
        // 0% threshold + a generous topmargin pulls the "active"
        // line to the top quarter of the viewport — feels natural
        // when a heading is at eye level.
        rootMargin: '0px 0px -75% 0px',
        threshold: [0]
      }
    );

    // Capture every heading currently in the DOM. Re-query when
    // headings change (new doc / fresh render) so the observer
    // never holds detached nodes.
    const els = container.querySelectorAll<HTMLElement>('[data-heading-line]');
    for (const el of els) {
      const ln = parseInt(el.dataset.headingLine ?? '', 10);
      if (!Number.isFinite(ln)) continue;
      lineToTop.set(ln, 0);
      obs.observe(el);
    }

    container.addEventListener('scroll', onScrollOrResize, { passive: true });
    window.addEventListener('resize', onScrollOrResize);
    // Initial measure.
    onScrollOrResize();

    return () => {
      obs.disconnect();
      container.removeEventListener('scroll', onScrollOrResize);
      window.removeEventListener('resize', onScrollOrResize);
      if (raf) cancelAnimationFrame(raf);
      // Reset so the next mount starts clean (no flash of the previous
      // doc's active line).
      untrack(() => {
        observedActiveLine = null;
      });
    };
  });
</script>

{#if headings.length === 0}
  <div class="text-xs text-dim italic px-2 py-1">no headings</div>
{:else}
  <ul class="space-y-px text-sm">
    {#each headings as h}
      {@const isActive = activeLine === h.line}
      {@const isVisited = visited?.has(h.line) ?? false}
      <li>
        <button
          type="button"
          onclick={() => onJump?.(h.line)}
          class="w-full text-left py-1 px-2 rounded truncate flex items-baseline gap-1.5 transition-colors
            {isActive ? 'bg-surface1 text-primary border-l-2 border-primary -ml-px' : 'text-text hover:bg-surface0'}"
          style="padding-left: {0.5 + (h.level - 1) * 0.75}rem; font-size: {h.level === 1 ? '0.875rem' : '0.8125rem'}; opacity: {isActive ? 1 : 1 - (h.level - 1) * 0.08};"
        >
          <span class="truncate flex-1">{h.text}</span>
          {#if isVisited && !isActive}
            <span
              class="text-success/80 text-[10px] flex-shrink-0"
              title="visited"
              aria-label="visited"
            >✓</span>
          {/if}
        </button>
      </li>
    {/each}
  </ul>
{/if}
