<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import {
    api,
    type BookDetail,
    type BookSidecar,
    type BookHighlight,
    type BookBookmark,
    type BookTOCEntry
  } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { errorMessage } from '$lib/util/errorMessage';
  import { toast } from '$lib/components/toast';
  import { loadStored, saveStored, loadStoredString, saveStoredString } from '$lib/util/storage';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import {
    ANNOTATION_COLORS,
    annotationSwatchClass,
    type AnnotationColor
  } from '$lib/notes/annotationColors';

  // Reader page — chapter pager with progress save, asset
  // resolution, and highlights overlay.
  //
  // Architecture:
  //   - Detail (spine + TOC) loads once on mount.
  //   - Chapter HTML loads on demand and is parsed into a fragment.
  //     We walk it to find <img> / <link rel="stylesheet"> refs the
  //     server rewrote through /asset/, fetch each as a blob URL,
  //     and patch the DOM in place. This is the "auth on binary"
  //     workaround — <img> can't carry a Bearer header otherwise.
  //   - Scroll progress is throttled (2 s) → PUT /progress.
  //   - Highlights are saved on text-selection + colour pick;
  //     re-rendering wraps each highlight's quoted span in a
  //     <mark> via plain text-search at chapter render time.

  // Reader prefs — per-device, font-size + theme + max-width. The
  // user might want a tighter column on a 4K monitor and a wider
  // one on a phone with bigger type. Stored as one blob so a
  // settings change saves once.
  type ReaderPrefs = {
    fontSize: 'sm' | 'md' | 'lg' | 'xl';
    theme: 'paper' | 'sepia' | 'dark';
    maxWidth: 'narrow' | 'wide';
    lineHeight: 'tight' | 'normal' | 'loose';
  };
  const PREFS_KEY = 'granit.books.reader.prefs';
  const DEFAULT_PREFS: ReaderPrefs = {
    fontSize: 'md',
    theme: 'paper',
    maxWidth: 'narrow',
    lineHeight: 'normal'
  };
  let prefs = $state<ReaderPrefs>({ ...DEFAULT_PREFS, ...loadStored<Partial<ReaderPrefs>>(PREFS_KEY, {}) });
  $effect(() => saveStored(PREFS_KEY, prefs));

  // Sidebar visibility — TOC + highlights panel. Hidden by default
  // on mobile (per-device key so the desktop user keeps it open).
  const SIDEBAR_KEY = 'granit.books.reader.sidebar';
  let sidebarOpen = $state(loadStoredString(SIDEBAR_KEY, '1') === '1');
  $effect(() => saveStoredString(SIDEBAR_KEY, sidebarOpen ? '1' : '0'));

  let bookId = $derived($page.params.id ?? '');
  let detail = $state<BookDetail | null>(null);
  let sidecar = $state<BookSidecar | null>(null);
  let chapterIdx = $state(0);
  let chapterHTML = $state('');
  let chapterLoading = $state(false);
  let error = $state('');

  // Asset blob URLs we've created — revoke on chapter change so
  // the browser doesn't accumulate object URLs across reads.
  let chapterAssetURLs: string[] = [];

  // Container for the rendered chapter; binding the ref lets the
  // post-render DOM patch (asset blob URLs, highlight wrapping,
  // selection capture) run against the same element.
  let chapterEl: HTMLElement | undefined = $state();

  let progressSaveTimer: ReturnType<typeof setTimeout> | null = null;

  async function loadDetail() {
    error = '';
    try {
      const [d, sc] = await Promise.all([
        api.getBook(bookId),
        api.getBookSidecar(bookId)
      ]);
      detail = d;
      sidecar = sc;
      // Resume where the user left off.
      const startCh = sc?.progress?.chapterIdx ?? 0;
      await jumpToChapter(Math.min(startCh, d.chapters.length - 1));
      // Restore scroll position after the chapter renders.
      await tick();
      const frac = sc?.progress?.scrollFraction ?? 0;
      if (frac > 0 && chapterEl) {
        const el = chapterEl;
        requestAnimationFrame(() => {
          el.scrollTop = frac * Math.max(1, el.scrollHeight - el.clientHeight);
        });
      }
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function jumpToChapter(idx: number) {
    if (!detail) return;
    if (idx < 0) idx = 0;
    if (idx >= detail.chapters.length) idx = detail.chapters.length - 1;
    chapterIdx = idx;
    chapterLoading = true;
    chapterHTML = '';
    revokeChapterAssets();
    try {
      const r = await api.getBookChapter(bookId, idx);
      chapterHTML = r.html;
      await tick();
      await patchChapterAssets();
      applyHighlights();
      // New chapter starts at top; flush progress immediately so
      // a quick navigate doesn't appear stuck on the old chapter.
      flushProgress(0);
    } catch (e) {
      error = errorMessage(e);
    } finally {
      chapterLoading = false;
    }
  }

  // Walk the rendered chapter for <img>/<link rel="stylesheet">
  // pointing at our asset endpoint, fetch each as a blob URL, and
  // swap the src/href so the browser-native auth bypass works.
  // The server already rewrote relative refs to /api/v1/books/<id>
  // /asset/<resolved-path> — we just need to peel off the path
  // segment after `/asset/` and round-trip it through the
  // authenticated fetch.
  async function patchChapterAssets() {
    if (!chapterEl) return;
    const imgs = chapterEl.querySelectorAll<HTMLImageElement>('img[src]');
    const links = chapterEl.querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"][href]');
    const work: Promise<void>[] = [];
    for (const img of imgs) {
      const src = img.getAttribute('src') ?? '';
      if (!src.includes('/asset/')) continue;
      const rel = src.split('/asset/')[1];
      work.push(
        api.bookAssetBlobURL(bookId, decodeURIComponent(rel)).then((url) => {
          if (url) {
            img.src = url;
            chapterAssetURLs.push(url);
          }
        })
      );
    }
    for (const link of links) {
      const href = link.getAttribute('href') ?? '';
      if (!href.includes('/asset/')) continue;
      const rel = href.split('/asset/')[1];
      work.push(
        api.bookAssetBlobURL(bookId, decodeURIComponent(rel)).then((url) => {
          if (url) {
            link.href = url;
            chapterAssetURLs.push(url);
          }
        })
      );
    }
    await Promise.all(work);
  }

  function revokeChapterAssets() {
    for (const url of chapterAssetURLs) URL.revokeObjectURL(url);
    chapterAssetURLs = [];
  }

  // ── Progress saving ─────────────────────────────────────────────
  // Throttled 2 s — the user can scroll a lot in a chapter and
  // saving every event would burn writes. The trailing-edge model
  // means a quick scroll-stop-scroll always lands one save at the
  // current position.
  function scheduleSaveProgress() {
    if (progressSaveTimer) return;
    progressSaveTimer = setTimeout(() => {
      progressSaveTimer = null;
      flushProgress(currentScrollFraction());
    }, 2000);
  }
  function currentScrollFraction(): number {
    if (!chapterEl) return 0;
    const denom = Math.max(1, chapterEl.scrollHeight - chapterEl.clientHeight);
    return Math.max(0, Math.min(1, chapterEl.scrollTop / denom));
  }
  async function flushProgress(frac: number) {
    if (!detail) return;
    try {
      await api.putBookProgress(bookId, {
        chapterIdx,
        scrollFraction: frac
      });
    } catch {
      // Silently — losing one progress save isn't worth a toast,
      // the next scroll/jump will re-try.
    }
  }
  function onChapterScroll() {
    scheduleSaveProgress();
  }
  // Save on visibility-hidden / page-leave so a tab close doesn't
  // lose the current spot. Synchronous-ish — we use sendBeacon if
  // the regular fetch is in flight.
  function onVisibilityChange() {
    if (document.visibilityState === 'hidden') {
      flushProgress(currentScrollFraction());
    }
  }

  // ── Highlights ──────────────────────────────────────────────────
  // Selection-driven: when the user releases a text selection
  // inside the chapter, a small floating toolbar offers four
  // colour swatches. Clicking one POSTS a highlight + re-renders.
  let toolbar = $state<{ x: number; y: number; text: string } | null>(null);

  function onChapterPointerUp() {
    if (!chapterEl) return;
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed) {
      toolbar = null;
      return;
    }
    const text = sel.toString().trim();
    if (text.length < 2) {
      toolbar = null;
      return;
    }
    if (!chapterEl.contains(sel.anchorNode)) {
      toolbar = null;
      return;
    }
    const range = sel.getRangeAt(0);
    const r = range.getBoundingClientRect();
    const containerRect = chapterEl.getBoundingClientRect();
    toolbar = {
      x: r.left + r.width / 2 - containerRect.left,
      y: r.top - containerRect.top - 36,
      text
    };
  }

  // ── Bookmarks ───────────────────────────────────────────────────
  // Lightweight "remember this spot" markers — chapter + scroll
  // fraction + an optional label. Distinct from highlights:
  // bookmarks aren't tied to a quoted passage, they're navigation
  // anchors ("come back here").
  async function addBookmark() {
    if (!detail) return;
    const chapter = detail.chapters[chapterIdx];
    const defaultLabel = chapter ? chapter.label : `Chapter ${chapterIdx + 1}`;
    const label = prompt('Bookmark label (optional):', defaultLabel);
    if (label === null) return; // cancelled
    try {
      const b = await api.createBookBookmark(bookId, {
        chapterIdx,
        scrollFraction: currentScrollFraction(),
        label: label.trim() || defaultLabel
      });
      sidecar = sidecar
        ? { ...sidecar, bookmarks: [...(sidecar.bookmarks ?? []), b] }
        : sidecar;
      toast.success('Bookmark saved');
    } catch (e) {
      toast.error('Bookmark failed: ' + errorMessage(e));
    }
  }

  async function jumpToBookmark(b: BookBookmark) {
    if (b.chapterIdx !== chapterIdx) {
      await jumpToChapter(b.chapterIdx);
    }
    if (b.scrollFraction && chapterEl) {
      const el = chapterEl;
      requestAnimationFrame(() => {
        el.scrollTop = b.scrollFraction! * Math.max(1, el.scrollHeight - el.clientHeight);
      });
    }
  }

  async function deleteBookmark(bid: string, e: Event) {
    e.stopPropagation();
    try {
      await api.deleteBookBookmark(bookId, bid);
      sidecar = sidecar
        ? { ...sidecar, bookmarks: (sidecar.bookmarks ?? []).filter((b) => b.id !== bid) }
        : sidecar;
    } catch (err) {
      toast.error('Delete failed: ' + errorMessage(err));
    }
  }

  async function highlight(color: AnnotationColor) {
    if (!toolbar) return;
    const text = toolbar.text;
    const sel = window.getSelection();
    let prefix = '';
    let suffix = '';
    if (sel && sel.rangeCount > 0 && chapterEl) {
      const flat = chapterEl.textContent ?? '';
      const idx = flat.indexOf(text);
      if (idx >= 0) {
        prefix = flat.slice(Math.max(0, idx - 30), idx);
        suffix = flat.slice(idx + text.length, idx + text.length + 30);
      }
    }
    try {
      const h = await api.createBookHighlight(bookId, {
        chapterIdx,
        text,
        prefix,
        suffix,
        color
      });
      sidecar = sidecar
        ? { ...sidecar, highlights: [...(sidecar.highlights ?? []), h] }
        : sidecar;
      applyHighlights();
      toolbar = null;
      window.getSelection()?.removeAllRanges();
    } catch (e) {
      toast.error('Highlight failed: ' + errorMessage(e));
    }
  }

  // Wrap saved highlights for the current chapter in <mark> tags.
  // Anchored via prefix + text + suffix (30 chars each side, saved
  // at highlight time): we collapse the chapter to its flat text,
  // search for prefix+text+suffix as a contiguous span, and pick
  // the offset where text sits inside that match. Falls back to
  // first-occurrence anchoring when the prefix/suffix are absent
  // (older highlights) or when the chapter has been edited
  // underneath them.
  function applyHighlights() {
    if (!chapterEl || !sidecar?.highlights) return;
    for (const h of sidecar.highlights) {
      if (h.chapterIdx !== chapterIdx) continue;
      // Skip if already wrapped — saves repeated work on re-render.
      if (chapterEl.querySelector(`mark[data-hid="${h.id}"]`)) continue;
      const offset = locateHighlight(chapterEl, h.text, h.prefix ?? '', h.suffix ?? '');
      if (offset < 0) continue;
      wrapRange(chapterEl, offset, h.text.length, h.color, h.id);
    }
  }

  /**
   * Find the absolute character offset (within the flattened
   * chapter text) of a saved highlight, using prefix + text + suffix
   * as a disambiguator. Returns -1 if nothing matches.
   *
   * Strategy: assemble `prefix + text + suffix` and search for it
   * verbatim. If found, the highlight starts at `match + prefix.length`.
   * If not found (chapter changed since highlight, or anchors are
   * empty), fall back to the first occurrence of `text` alone.
   */
  function locateHighlight(root: HTMLElement, text: string, prefix: string, suffix: string): number {
    const flat = root.textContent ?? '';
    if (text.length < 2 || !flat.includes(text)) return -1;
    if (prefix || suffix) {
      const composite = prefix + text + suffix;
      const idx = flat.indexOf(composite);
      if (idx >= 0) return idx + prefix.length;
    }
    return flat.indexOf(text);
  }

  /**
   * Wrap (offset, offset+length) of the flattened chapter text in
   * a <mark>. Walks the DOM accumulating text-node lengths; when
   * we cross `offset` we split the text node, advance through any
   * intervening nodes (so a highlight crossing tag boundaries
   * still wraps cleanly), and stop at offset+length.
   */
  function wrapRange(root: HTMLElement, offset: number, length: number, color: string, id: string) {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
    let pos = 0;
    let remaining = length;
    let node: Node | null;
    const toWrap: Text[] = [];
    while ((node = walker.nextNode())) {
      const t = node as Text;
      const len = t.textContent?.length ?? 0;
      if (len === 0) continue;
      const startInNode = offset - pos;
      const endInNode = offset + length - pos;
      if (endInNode <= 0) {
        pos += len;
        continue;
      }
      if (startInNode >= len) {
        pos += len;
        continue;
      }
      // This node intersects [offset, offset+length). Split as
      // needed so the wrap target is exactly the matching text.
      let target = t;
      if (startInNode > 0) {
        target = t.splitText(startInNode);
      }
      const remainingInTarget = Math.min(remaining, target.textContent?.length ?? 0);
      if (remainingInTarget < (target.textContent?.length ?? 0)) {
        target.splitText(remainingInTarget);
      }
      toWrap.push(target);
      remaining -= remainingInTarget;
      pos += len;
      if (remaining <= 0) break;
    }
    if (toWrap.length === 0) return;
    for (let i = 0; i < toWrap.length; i++) {
      const t = toWrap[i];
      const mark = document.createElement('mark');
      // Only the first segment carries the data-hid so the de-dup
      // check above stays correct (a multi-node highlight isn't
      // counted as N marks).
      if (i === 0) mark.dataset.hid = id;
      mark.className = `bookmark-color-${color}`;
      mark.textContent = t.textContent;
      t.replaceWith(mark);
    }
  }

  // Chapter-list label resolved through the current TOC (or the
  // pre-baked Chapter N fallback).
  let currentChapterLabel = $derived(
    detail?.chapters[chapterIdx]?.label ?? `Chapter ${chapterIdx + 1}`
  );

  // ── Layout ──────────────────────────────────────────────────────
  let articleClass = $derived.by(() => {
    const sizeMap = { sm: 'text-base', md: 'text-lg', lg: 'text-xl', xl: 'text-2xl' };
    const widthMap = { narrow: 'max-w-[68ch]', wide: 'max-w-[88ch]' };
    const lineMap = { tight: 'leading-snug', normal: 'leading-relaxed', loose: 'leading-loose' };
    return `${sizeMap[prefs.fontSize]} ${widthMap[prefs.maxWidth]} ${lineMap[prefs.lineHeight]} mx-auto reader-prose`;
  });
  let outerThemeClass = $derived(
    prefs.theme === 'sepia'
      ? 'bg-[#f5ecd7] text-[#3a2f1c]'
      : prefs.theme === 'dark'
        ? 'bg-mantle text-text'
        : 'bg-surface0 text-text'
  );

  // Toolbar swatch class — delegated to the shared annotationColors
  // helper so the books-reader, notes-margin, and dashboard surfaces
  // all stay in lockstep on a future palette change.
  const fmtSelectedColor = annotationSwatchClass;

  onMount(() => {
    loadDetail();
    document.addEventListener('visibilitychange', onVisibilityChange);
    return onWsEvent((ev) => {
      if (
        ev.type === 'state.changed' &&
        ev.path === `.granit/books/${bookId}.json`
      ) {
        // Reload sidecar but don't disturb the active scroll
        // position — only re-apply highlights on top of the
        // current rendered chapter.
        api.getBookSidecar(bookId).then((sc) => {
          sidecar = sc;
          applyHighlights();
        });
      }
    });
  });

  onDestroy(() => {
    if (progressSaveTimer) clearTimeout(progressSaveTimer);
    flushProgress(currentScrollFraction());
    revokeChapterAssets();
    document.removeEventListener('visibilitychange', onVisibilityChange);
  });

  // Nested TOC walker for the sidebar — recursive markup is the
  // shortest path to a clickable hierarchy.
  function walkToc(toc: BookTOCEntry[] | undefined): { entry: BookTOCEntry; depth: number }[] {
    const out: { entry: BookTOCEntry; depth: number }[] = [];
    function recur(es: BookTOCEntry[], d: number) {
      for (const e of es) {
        out.push({ entry: e, depth: d });
        if (e.Children) recur(e.Children, d + 1);
      }
    }
    if (toc) recur(toc, 0);
    return out;
  }
  let flatToc = $derived(walkToc(detail?.toc));
</script>

<svelte:head>
  <title>{detail?.title ?? 'Reading'} — Granit</title>
</svelte:head>

{#if !$auth}
  <div class="p-6 text-dim">Sign in to read.</div>
{:else if error}
  <div class="bg-error/10 border border-error/30 text-error rounded p-4 m-4 text-sm">
    {error}
  </div>
{:else if !detail}
  <div class="p-6">
    <Skeleton class="h-8 w-1/3 mb-4" />
    <Skeleton class="h-4 w-2/3 mb-2" />
    <Skeleton class="h-4 w-1/2" />
  </div>
{:else}
  <div class="h-full flex flex-col {outerThemeClass}">
    <!-- Top bar — title + chapter context + reader-prefs popover. -->
    <header class="flex-shrink-0 border-b border-surface1/50 px-3 sm:px-4 py-2 flex items-center gap-2 backdrop-blur-sm">
      <button
        onclick={() => goto('/books')}
        class="text-dim hover:text-text text-sm flex-shrink-0"
        title="Back to shelf"
      >
        ←
      </button>
      <div class="min-w-0 flex-1">
        <div class="text-sm font-medium truncate">{detail.title}</div>
        <div class="text-xs text-dim truncate">
          {currentChapterLabel} · {chapterIdx + 1} / {detail.chapters.length}
        </div>
      </div>
      <button
        onclick={addBookmark}
        class="text-xs px-2 py-1 rounded hover:bg-surface1/50 text-subtext flex items-center gap-1"
        title="Bookmark current spot"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="w-4 h-4">
          <path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"/>
        </svg>
        <span class="hidden sm:inline">Bookmark</span>
      </button>
      <button
        onclick={() => (sidebarOpen = !sidebarOpen)}
        class="text-xs px-2 py-1 rounded hover:bg-surface1/50 text-subtext"
        title="Toggle table of contents"
      >
        {sidebarOpen ? 'Hide TOC' : 'TOC'}
      </button>
      <details class="relative">
        <summary class="text-xs px-2 py-1 rounded hover:bg-surface1/50 text-subtext cursor-pointer list-none">
          Aa
        </summary>
        <div class="absolute right-0 top-full mt-1 z-20 bg-surface0 border border-surface1 rounded-lg shadow-lg p-3 w-56 text-sm space-y-3 text-text">
          <div>
            <div class="text-xs uppercase text-dim mb-1">Size</div>
            <div class="flex gap-1">
              {#each ['sm', 'md', 'lg', 'xl'] as s (s)}
                <button
                  class="flex-1 px-2 py-1 rounded {prefs.fontSize === s ? 'bg-primary text-on-primary' : 'bg-surface1 hover:bg-surface2'}"
                  onclick={() => (prefs = { ...prefs, fontSize: s as ReaderPrefs['fontSize'] })}
                >
                  {s.toUpperCase()}
                </button>
              {/each}
            </div>
          </div>
          <div>
            <div class="text-xs uppercase text-dim mb-1">Theme</div>
            <div class="flex gap-1">
              {#each ['paper', 'sepia', 'dark'] as t (t)}
                <button
                  class="flex-1 px-2 py-1 rounded text-xs capitalize {prefs.theme === t ? 'bg-primary text-on-primary' : 'bg-surface1 hover:bg-surface2'}"
                  onclick={() => (prefs = { ...prefs, theme: t as ReaderPrefs['theme'] })}
                >
                  {t}
                </button>
              {/each}
            </div>
          </div>
          <div>
            <div class="text-xs uppercase text-dim mb-1">Width</div>
            <div class="flex gap-1">
              {#each ['narrow', 'wide'] as w (w)}
                <button
                  class="flex-1 px-2 py-1 rounded text-xs capitalize {prefs.maxWidth === w ? 'bg-primary text-on-primary' : 'bg-surface1 hover:bg-surface2'}"
                  onclick={() => (prefs = { ...prefs, maxWidth: w as ReaderPrefs['maxWidth'] })}
                >
                  {w}
                </button>
              {/each}
            </div>
          </div>
          <div>
            <div class="text-xs uppercase text-dim mb-1">Line height</div>
            <div class="flex gap-1">
              {#each ['tight', 'normal', 'loose'] as l (l)}
                <button
                  class="flex-1 px-2 py-1 rounded text-xs capitalize {prefs.lineHeight === l ? 'bg-primary text-on-primary' : 'bg-surface1 hover:bg-surface2'}"
                  onclick={() => (prefs = { ...prefs, lineHeight: l as ReaderPrefs['lineHeight'] })}
                >
                  {l}
                </button>
              {/each}
            </div>
          </div>
        </div>
      </details>
    </header>

    <div class="flex-1 flex min-h-0">
      <!-- Sidebar: TOC + highlights -->
      {#if sidebarOpen}
        <aside class="w-72 border-r border-surface1/50 overflow-y-auto flex-shrink-0 hidden md:block">
          <div class="p-3">
            <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Contents</h3>
            <ul class="space-y-0.5">
              {#each flatToc as { entry, depth } (entry.Title + entry.SpineIdx)}
                <li>
                  <button
                    onclick={() => entry.SpineIdx >= 0 && jumpToChapter(entry.SpineIdx)}
                    style="padding-left: {0.25 + depth * 0.75}rem"
                    class="block w-full text-left text-sm py-1 rounded hover:bg-surface1/50 truncate {entry.SpineIdx === chapterIdx ? 'text-primary font-medium' : 'text-subtext'}"
                    disabled={entry.SpineIdx < 0}
                    title={entry.Title}
                  >
                    {entry.Title}
                  </button>
                </li>
              {/each}
              {#if flatToc.length === 0}
                <!-- TOC absent — render the spine as the navigation. -->
                {#each detail.chapters as ch (ch.index)}
                  <li>
                    <button
                      onclick={() => jumpToChapter(ch.index)}
                      class="block w-full text-left text-sm py-1 px-1 rounded hover:bg-surface1/50 truncate {ch.index === chapterIdx ? 'text-primary font-medium' : 'text-subtext'}"
                    >
                      {ch.label}
                    </button>
                  </li>
                {/each}
              {/if}
            </ul>

            {#if (sidecar?.bookmarks ?? []).length > 0}
              <h3 class="text-xs uppercase tracking-wider text-dim mt-6 mb-2">Bookmarks</h3>
              <ul class="space-y-1">
                {#each sidecar?.bookmarks ?? [] as b (b.id)}
                  <li class="group flex items-center rounded hover:bg-surface1/50">
                    <button
                      class="flex-1 min-w-0 text-left text-xs leading-snug px-2 py-1.5 flex items-center gap-2"
                      onclick={() => jumpToBookmark(b)}
                    >
                      <span class="text-primary flex-shrink-0">▸</span>
                      <span class="flex-1 truncate text-text">{b.label}</span>
                      <span class="text-dim flex-shrink-0">ch {b.chapterIdx + 1}</span>
                    </button>
                    <button
                      type="button"
                      class="opacity-0 group-hover:opacity-100 text-dim hover:text-error px-2 py-1.5 flex-shrink-0"
                      title="Remove bookmark"
                      onclick={(e) => deleteBookmark(b.id, e)}
                    >
                      ×
                    </button>
                  </li>
                {/each}
              </ul>
            {/if}

            {#if (sidecar?.highlights ?? []).length > 0}
              <h3 class="text-xs uppercase tracking-wider text-dim mt-6 mb-2">Highlights</h3>
              <ul class="space-y-2">
                {#each sidecar?.highlights ?? [] as h (h.id)}
                  <li>
                    <button
                      class="w-full text-left text-xs leading-snug p-2 rounded hover:bg-surface1/50 border-l-2 border-{h.color}-300"
                      onclick={() => jumpToChapter(h.chapterIdx)}
                    >
                      <div class="text-text line-clamp-3">"{h.text}"</div>
                      <div class="text-dim mt-1">ch {h.chapterIdx + 1}</div>
                    </button>
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
        </aside>
      {/if}

      <!-- Reader column -->
      <main class="flex-1 min-w-0 relative">
        <div
          bind:this={chapterEl}
          onscroll={onChapterScroll}
          onpointerup={onChapterPointerUp}
          class="h-full overflow-y-auto px-4 sm:px-8 py-6"
        >
          {#if chapterLoading}
            <div class={articleClass}>
              <Skeleton class="h-6 w-1/3 mb-4" />
              <Skeleton class="h-4 w-full mb-2" />
              <Skeleton class="h-4 w-11/12 mb-2" />
              <Skeleton class="h-4 w-9/12 mb-6" />
              <Skeleton class="h-4 w-full mb-2" />
              <Skeleton class="h-4 w-10/12" />
            </div>
          {:else}
            <article class={articleClass}>
              {@html chapterHTML}
            </article>
            <nav class="flex items-center justify-between mt-12 mb-4 max-w-[88ch] mx-auto">
              <button
                onclick={() => jumpToChapter(chapterIdx - 1)}
                disabled={chapterIdx <= 0}
                class="text-sm px-3 py-2 rounded bg-surface1 hover:bg-surface2 disabled:opacity-30 disabled:cursor-not-allowed"
              >
                ← Previous
              </button>
              <div class="text-xs text-dim">
                {chapterIdx + 1} / {detail.chapters.length}
              </div>
              <button
                onclick={() => jumpToChapter(chapterIdx + 1)}
                disabled={chapterIdx >= detail.chapters.length - 1}
                class="text-sm px-3 py-2 rounded bg-surface1 hover:bg-surface2 disabled:opacity-30 disabled:cursor-not-allowed"
              >
                Next →
              </button>
            </nav>
          {/if}
        </div>

        <!-- Floating selection toolbar — appears on text-selection -->
        {#if toolbar}
          <div
            class="absolute z-10 flex gap-1 bg-surface0 border border-surface1 rounded-lg shadow-lg px-2 py-1.5"
            style="left: {toolbar.x}px; top: {toolbar.y}px; transform: translateX(-50%);"
          >
            {#each ANNOTATION_COLORS as c (c)}
              <button
                class="w-6 h-6 rounded {fmtSelectedColor(c)} hover:scale-110 transition-transform"
                title="Highlight {c}"
                onclick={() => highlight(c)}
              ></button>
            {/each}
          </div>
        {/if}
      </main>
    </div>
  </div>
{/if}

<style>
  /* Reader-prose typography — applies inside the chapter article
     so EPUB content lands close to native long-form reading. We
     deliberately don't import Tailwind's prose plugin here; the
     EPUB ships its own headings, lists, blockquotes, and we want
     them to inherit a typographic baseline rather than be
     overridden by an opinionated stylesheet. */
  :global(.reader-prose h1),
  :global(.reader-prose h2),
  :global(.reader-prose h3) {
    font-family: var(--font-serif, Georgia, serif);
    font-weight: 600;
    margin-top: 2em;
    margin-bottom: 0.6em;
    line-height: 1.2;
  }
  :global(.reader-prose h1) { font-size: 1.6em; }
  :global(.reader-prose h2) { font-size: 1.35em; }
  :global(.reader-prose h3) { font-size: 1.15em; }
  :global(.reader-prose p) {
    margin: 0 0 1em;
    text-indent: 1.25em;
    hyphens: auto;
  }
  :global(.reader-prose p:first-of-type),
  :global(.reader-prose h1 + p),
  :global(.reader-prose h2 + p),
  :global(.reader-prose h3 + p) {
    text-indent: 0;
  }
  :global(.reader-prose blockquote) {
    margin: 1.5em 0;
    padding-left: 1em;
    border-left: 3px solid currentColor;
    opacity: 0.85;
    font-style: italic;
  }
  :global(.reader-prose img) {
    max-width: 100%;
    height: auto;
    display: block;
    margin: 1.5em auto;
  }
  :global(.reader-prose a) {
    color: inherit;
    text-decoration: underline;
    text-decoration-thickness: 1px;
    text-underline-offset: 2px;
  }
  :global(.reader-prose ul),
  :global(.reader-prose ol) {
    margin: 1em 0;
    padding-left: 1.6em;
  }
  :global(.bookmark-color-yellow) { background: #fde68a; color: #1f2937; }
  :global(.bookmark-color-blue) { background: #bfdbfe; color: #1f2937; }
  :global(.bookmark-color-green) { background: #bbf7d0; color: #1f2937; }
  :global(.bookmark-color-pink) { background: #fbcfe8; color: #1f2937; }
</style>
