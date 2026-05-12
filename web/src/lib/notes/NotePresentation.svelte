<!--
  NotePresentation — fullscreen slide-deck view of the current
  note. Splits the body on `## ` headings (H2); each section
  becomes a slide. The H1 (if present) is the title slide.

  Why H2 and not H1: most notes use exactly one H1 as the doc
  title, then H2s for sections. Splitting on H1 would produce a
  single-slide "deck" most of the time. The user can opt-in to
  finer or coarser splits later by changing the heading structure
  of their notes.

  Controls:
    - → / Space / Enter / PageDown — next slide
    - ← / PageUp — previous slide
    - Home — first; End — last
    - Esc — exit
    - F or double-click — toggle fullscreen on the host element
    - N or T — toggle speaker notes (gist if cached, else nothing)

  Slide rendering goes through MarkdownRenderer so wikilinks /
  callouts / images all render the same as in the regular preview.
  We just isolate the slice of the body for this slide and render
  it large.

  Fullscreen is opt-in (toggle via F) rather than forced, because
  triggering Fullscreen API on open requires a fresh user
  activation event in some browsers; opening from a button click
  works, but opening from a keyboard shortcut after focus has
  drifted may not. Keeping it on a button keeps the UX reliable.
-->
<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  let {
    body,
    title,
    open,
    onClose
  }: {
    body: string;
    title: string;
    open: boolean;
    onClose: () => void;
  } = $props();

  function stripFrontmatter(src: string): string {
    if (!src.startsWith('---')) return src;
    const end = src.indexOf('\n---', 3);
    if (end === -1) return src;
    return src.slice(end + 4).replace(/^\r?\n/, '');
  }

  // A slide is a heading line + the lines that follow until the
  // next ## (or EOF). The first slide is the H1 title block + any
  // intro paragraphs before the first ##; if no H1 exists, we use
  // the note title as the synthesised first slide's title.
  type Slide = { title: string; body: string; level: number };

  let slides = $derived.by<Slide[]>(() => {
    const src = stripFrontmatter(body);
    if (!src.trim()) return [];
    const lines = src.split('\n');
    const out: Slide[] = [];
    let cur: Slide | null = null;
    let inFence = false;
    for (let i = 0; i < lines.length; i++) {
      const ln = lines[i];
      const t = ln.trim();
      if (t.startsWith('```') || t.startsWith('~~~')) {
        inFence = !inFence;
        if (cur) cur.body += ln + '\n';
        else if (out.length === 0) {
          // Pre-heading content — start an implicit slide with the
          // note title.
          cur = { title: title, body: ln + '\n', level: 1 };
        } else {
          // Shouldn't happen — the loop always has cur set after
          // the first slide is opened.
          if (cur) (cur as Slide).body += ln + '\n';
        }
        continue;
      }
      if (inFence) {
        if (cur) cur.body += ln + '\n';
        continue;
      }
      const m = /^(#{1,2})\s+(.+?)\s*#*$/.exec(t);
      if (m) {
        // Flush the previous slide.
        if (cur) out.push(cur);
        cur = { title: m[2].trim(), body: '', level: m[1].length };
        continue;
      }
      if (!cur) {
        // Pre-heading content — build the title slide. Use the
        // note's title (the heading H1 if present at the very top
        // would normally have been picked up by the regex above
        // already; this branch handles notes that begin with body
        // text rather than a heading).
        cur = { title: title, body: ln + '\n', level: 1 };
      } else {
        cur.body += ln + '\n';
      }
    }
    if (cur) out.push(cur);
    if (out.length === 0) {
      out.push({ title: title, body: src, level: 1 });
    }
    return out;
  });

  let position = $state(0);
  // Reset on open + on body change so re-opening lands on slide 1
  // rather than wherever the previous session left off.
  $effect(() => {
    void open;
    void slides;
    position = 0;
  });

  let total = $derived(slides.length);
  let current = $derived(slides[position] ?? null);

  function next() {
    if (position < total - 1) position++;
  }
  function prev() {
    if (position > 0) position--;
  }
  function first() { position = 0; }
  function last() { position = total - 1; }

  let host: HTMLElement | undefined = $state();
  let isFullscreen = $state(false);
  let notesOpen = $state(false);

  function toggleFullscreen() {
    if (!host) return;
    if (document.fullscreenElement) {
      void document.exitFullscreen();
    } else {
      void host.requestFullscreen?.().catch(() => {});
    }
  }

  // Track fullscreen state so the F-toggle button state updates.
  $effect(() => {
    const onChange = () => {
      isFullscreen = document.fullscreenElement === host;
    };
    document.addEventListener('fullscreenchange', onChange);
    return () => document.removeEventListener('fullscreenchange', onChange);
  });

  // Keyboard navigation. Only active when `open` is true so we
  // don't fight other shortcuts on the page.
  $effect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      // Don't intercept while the user is typing into a focused
      // input — there shouldn't be inputs here, but be defensive.
      const el = document.activeElement as HTMLElement | null;
      const tag = el?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea') return;
      if (e.key === 'Escape') { e.preventDefault(); close(); return; }
      if (e.key === 'ArrowRight' || e.key === 'PageDown' || e.key === ' ' || e.key === 'Enter') {
        e.preventDefault(); next(); return;
      }
      if (e.key === 'ArrowLeft' || e.key === 'PageUp') {
        e.preventDefault(); prev(); return;
      }
      if (e.key === 'Home') { e.preventDefault(); first(); return; }
      if (e.key === 'End') { e.preventDefault(); last(); return; }
      if (e.key.toLowerCase() === 'f') { e.preventDefault(); toggleFullscreen(); return; }
      if (e.key.toLowerCase() === 'n' || e.key.toLowerCase() === 't') {
        e.preventDefault(); notesOpen = !notesOpen; return;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  function close() {
    if (document.fullscreenElement === host) {
      void document.exitFullscreen().catch(() => {});
    }
    onClose();
  }

  // Defensive cleanup: if the parent unmounts the deck without
  // calling onClose first (e.g. user navigates away mid-presentation),
  // exit fullscreen so the next page isn't stuck in a fullscreen
  // shell that no longer has a host. Browsers usually exit
  // automatically when the fullscreen element leaves the DOM, but
  // some Safari builds wedge — explicit exit is cheap insurance.
  onDestroy(() => {
    if (typeof document !== 'undefined' && document.fullscreenElement === host) {
      void document.exitFullscreen().catch(() => {});
    }
  });

  // Focus the host on open so keystrokes work without an extra
  // click. tick() to wait for the conditional render.
  $effect(() => {
    if (!open) return;
    void tick().then(() => host?.focus?.());
  });
</script>

{#if open}
  <div
    bind:this={host}
    role="dialog"
    aria-modal="true"
    aria-label="Slideshow presentation of the note"
    tabindex="-1"
    class="fixed inset-0 z-50 bg-mantle text-text flex flex-col outline-none"
  >
    <!-- Top bar — slide counter, fullscreen toggle, exit. The
         double-click toggle on the slide canvas is set up below. -->
    <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 text-xs">
      <span class="font-mono tabular-nums text-dim">
        {position + 1} / {total}
      </span>
      <span class="text-dim/60">·</span>
      <span class="text-subtext truncate min-w-0">{title}</span>
      <span class="flex-1"></span>
      <button
        type="button"
        onclick={() => (notesOpen = !notesOpen)}
        class="px-2 py-1 rounded text-[11px] {notesOpen ? 'bg-surface1 text-secondary' : 'text-subtext hover:bg-surface0'}"
        title="Toggle speaker notes (N)"
      >notes</button>
      <button
        type="button"
        onclick={toggleFullscreen}
        class="px-2 py-1 rounded text-[11px] {isFullscreen ? 'bg-surface1 text-primary' : 'text-subtext hover:bg-surface0'}"
        title="Fullscreen (F)"
      >{isFullscreen ? 'exit fullscreen' : 'fullscreen'}</button>
      <button
        type="button"
        onclick={close}
        class="px-2 py-1 rounded text-[11px] text-subtext hover:bg-surface0"
        title="Exit slideshow (Esc)"
      >× close</button>
    </header>

    <!-- Slide canvas — large heading, generous padding, body
         renders through MarkdownRenderer so wikilinks/callouts/
         images stay consistent with the regular preview. We use a
         max-width so a wide screen doesn't stretch text into
         ribbons; centering keeps the slide readable. Double-click
         toggles fullscreen — common UX in deck tools. -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="flex-1 overflow-y-auto flex items-center justify-center px-4 sm:px-12 py-6 cursor-pointer"
      ondblclick={toggleFullscreen}
      onclick={(e) => {
        // Click anywhere except a link / interactive element to
        // advance. Keeps the deck navigable with just a clicker
        // (touchpad clicks count too) without hijacking link
        // clicks inside slide content.
        const t = e.target as HTMLElement;
        if (t.closest('a, button, input, textarea, select, [contenteditable], .slide-no-advance')) return;
        next();
      }}
    >
      <article class="max-w-4xl w-full">
        {#if current}
          <!-- The slide title gets its own heading style (large,
               in the primary color). We don't render the heading
               inside the body content because we already split it
               out at parse time. -->
          {#if current.level === 1 && position === 0}
            <h1 class="text-4xl sm:text-5xl font-bold mb-8 text-primary leading-tight">{current.title}</h1>
          {:else}
            <h2 class="text-3xl sm:text-4xl font-semibold mb-4 text-text leading-tight">{current.title}</h2>
          {/if}
          <div class="slide-body text-lg sm:text-xl leading-relaxed">
            <MarkdownRenderer body={current.body} />
          </div>
        {:else}
          <p class="text-dim italic">no slides — note has no headings.</p>
        {/if}
      </article>
    </div>

    <!-- Speaker notes drawer at the bottom. We don't have an
         AI-generated gist per slide hooked up yet (would require a
         per-section summary cache); for now the panel surfaces a
         hint shortcut + room for future AI-generated speaker
         notes. The toggle (N or T) keeps the affordance present
         without taking permanent screen space. -->
    {#if notesOpen}
      <aside class="border-t border-surface1 bg-surface0 px-3 sm:px-12 py-2 max-h-32 overflow-y-auto text-xs text-subtext">
        <span class="text-dim text-[10px] uppercase tracking-wider mr-2">speaker notes</span>
        <span class="italic">No saved notes for this slide. Tip: use ## headings to split sections; this slide is "{current?.title ?? '—'}".</span>
      </aside>
    {/if}

    <!-- Bottom navigation. Three big touch targets so a clicker /
         tap-and-go workflow on tablets works. -->
    <nav class="flex items-center gap-2 px-3 sm:px-12 py-2 border-t border-surface1 text-xs">
      <button
        type="button"
        onclick={prev}
        disabled={position === 0}
        class="px-3 py-1.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary disabled:opacity-30 slide-no-advance"
      >‹ prev</button>
      <span class="flex-1"></span>
      <span class="text-[10px] text-dim hidden sm:inline">
        ← / → · Space · F fullscreen · Esc exit
      </span>
      <span class="flex-1"></span>
      <button
        type="button"
        onclick={next}
        disabled={position >= total - 1}
        class="px-3 py-1.5 rounded bg-primary text-on-primary hover:opacity-90 disabled:opacity-30 slide-no-advance"
      >next ›</button>
    </nav>
  </div>
{/if}

<style>
  /* Make slide bodies render with bigger spacing and lifted
     headings — closer to a deck cadence than a wall of prose. */
  .slide-body :global(p) { margin: 0.7em 0; }
  .slide-body :global(ul),
  .slide-body :global(ol) { margin: 0.7em 0; padding-left: 1.5em; }
  .slide-body :global(li) { margin: 0.3em 0; }
  .slide-body :global(h3) { font-size: 1.5em; margin-top: 1em; }
  .slide-body :global(h4) { font-size: 1.2em; margin-top: 0.8em; }
  .slide-body :global(blockquote) { font-size: 1.05em; }
  .slide-body :global(img) { max-height: 50vh; object-fit: contain; }
  .slide-body :global(pre) { font-size: 0.85em; }
</style>
