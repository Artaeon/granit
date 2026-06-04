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
  import { onDestroy } from 'svelte';
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

  // Reset observedActiveLine when scrollContainer changes identity
  // (note swap, view-mode flip preview ↔ split). Without this, A's
  // last observed heading line briefly persists against B's fresh
  // DOM before the observer's first measure repopulates — visible
  // as a momentary wrong-heading highlight if B has a heading on
  // the same line number. Tracking container identity rather than
  // body keeps the reset out of the per-keystroke hot path that
  // caused the flicker fixed in b43de2ce.
  let lastSeenContainer: HTMLElement | null | undefined = undefined;
  $effect(() => {
    const c = scrollContainer ?? null;
    if (lastSeenContainer !== undefined && lastSeenContainer !== c) {
      observedActiveLine = null;
    }
    lastSeenContainer = c;
  });

  // (Re)attach the IntersectionObserver whenever `body` changes.
  //
  // A prior version tracked a `structuralKey` (the heading-line set)
  // and re-attached only when headings moved, on the assumption that
  // the DOM was stable between heading-set changes. That assumption
  // is wrong: MarkdownRenderer renders via {@html html}, which
  // wholesale-replaces the prose-note container's children on every
  // body change — even formatting-only edits (bold, italics, link
  // toggles) inside an unchanged section. The old DOM elements get
  // detached, new ones with the same data-heading-line attribute
  // take their place, and the observer's frozen refs point at
  // garbage. Result: the active-heading tracker silently stops
  // updating after the first formatting edit. Tracking `body`
  // directly re-attaches against the live DOM.
  //
  // Cost: per body change we tear down the IntersectionObserver,
  // run querySelectorAll over the prose container, and re-observe
  // every heading. Each obs.observe() flushes layout. On an 80-
  // heading note that's 10–40 ms per body change — meaningful on
  // the typing hot path even with bodyForPreview's debounce.
  //
  // Strategy: one effect, two paths.
  //   • Container first appears / changes (mount, note swap, view-
  //     mode flip) → attach IMMEDIATELY so the visible-heading
  //     paint isn't delayed.
  //   • Container stable, body changes (typing) → coalesce the
  //     re-attach at 400 ms. Active heading lies for ≤400 ms after
  //     the user stops typing, then snaps right.
  // Detach lives in a single ref so a re-attach always cleans the
  // prior observer; no two-effect race that could leak observers.
  let lastSeenContainer: HTMLElement | null = null;
  let detachActive: (() => void) | null = null;
  $effect(() => {
    const container = scrollContainer ?? null;
    void body;
    if (!container) {
      detachActive?.();
      detachActive = null;
      lastSeenContainer = null;
      return;
    }
    if (container !== lastSeenContainer) {
      lastSeenContainer = container;
      detachActive?.();
      detachActive = attach(container);
      return;
    }
    const t = setTimeout(() => {
      detachActive?.();
      detachActive = attach(container);
    }, 400);
    return () => clearTimeout(t);
  });
  onDestroy(() => {
    detachActive?.();
    detachActive = null;
  });

  function attach(container: HTMLElement): () => void {

    let raf = 0;
    const visibleLines = new Set<number>();
    const lineToTop = new Map<number, number>();
    // Cache of heading line → element ref captured at observer attach
    // time. Used by the scroll handler to avoid a per-tick
    // querySelector('[data-heading-line=N]') over the prose container,
    // which scaled poorly on long docs (100 headings × ~0.3-0.8 ms
    // layout flush per call at native scroll cadence = 30-80 ms per
    // event). The IntersectionObserver already re-keys lineToTop
    // when visibility changes, so the scroll handler only needs the
    // refs for the rare case where it tops up the cache between
    // visibility transitions.
    const lineToEl = new Map<number, HTMLElement>();

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

    // Scroll handler is rAF-coalesced AS A WHOLE. The previous
    // version did the per-heading getBoundingClientRect walk BEFORE
    // the rAF gate, which made the layout cost scale with scroll
    // event cadence (native rate, often 120 Hz on trackpads).
    let scrollRaf = 0;
    const onScrollOrResize = () => {
      if (scrollRaf) return;
      scrollRaf = requestAnimationFrame(() => {
        scrollRaf = 0;
        // One container rect for the whole walk. Containers scroll
        // independently, so we measure relative to the container's
        // top edge, not the viewport.
        const cTop = container.getBoundingClientRect().top;
        for (const [ln, el] of lineToEl) {
          if (!el.isConnected) continue;
          lineToTop.set(ln, el.getBoundingClientRect().top - cTop);
        }
        if (raf === 0) raf = requestAnimationFrame(recompute);
      });
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
      lineToEl.set(ln, el);
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
      if (scrollRaf) cancelAnimationFrame(scrollRaf);
      // We deliberately don't reset observedActiveLine here: the
      // detach also fires on every body-coalesced re-attach, and
      // resetting per keystroke flashed the active heading from
      // "current" to cursor fallback and back within one frame.
    };
  }
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
