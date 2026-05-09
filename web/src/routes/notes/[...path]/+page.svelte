<script lang="ts">
  import { onMount } from 'svelte';
  import { goto, beforeNavigate } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, type Note , todayISO } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Editor from '$lib/editor/Editor.svelte';
  import NotesTree from '$lib/notes/NotesTree.svelte';
  import Outline from '$lib/notes/Outline.svelte';
  import BacklinksPanel from '$lib/notes/BacklinksPanel.svelte';
  import AnnotationsPanel from '$lib/notes/AnnotationsPanel.svelte';
  import ConcordancePanel from '$lib/notes/ConcordancePanel.svelte';
  import LocalGraph from '$lib/notes/LocalGraph.svelte';
  import FrontmatterEditor from '$lib/notes/FrontmatterEditor.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import DailyQuickAdd from '$lib/notes/DailyQuickAdd.svelte';
  import DailyContext from '$lib/notes/DailyContext.svelte';
  import NoteDeadlinesStrip from '$lib/deadlines/NoteDeadlinesStrip.svelte';
  import Drawer from '$lib/components/Drawer.svelte';
  import { toast } from '$lib/components/toast';
  import { getDraft, setDraft, clearDraft, draftDivergesFromServer } from '$lib/notes/drafts';
  import {
    loadVisitedMap,
    recordVisitedLine,
    clearVisitedFor,
    rememberScroll,
    recallScroll
  } from '$lib/notes/noteHistory';
  import ExtractToNoteDialog from '$lib/notes/ExtractToNoteDialog.svelte';
  import type { ExtractRequest } from '$lib/editor/extract-note';
  import AskAIDialog from '$lib/notes/AskAIDialog.svelte';
  import type { AskAIRequest } from '$lib/editor/ask-ai';
  import PrintPreview from '$lib/notes/PrintPreview.svelte';
  import HistoryPanel from '$lib/notes/HistoryPanel.svelte';
  import ShortcutsHelpOverlay from '$lib/notes/ShortcutsHelpOverlay.svelte';
  import SelectionToolbar from '$lib/editor/SelectionToolbar.svelte';
  import LinkSuggestPanel from '$lib/notes/LinkSuggestPanel.svelte';
  import EditorAIMenu from '$lib/notes/EditorAIMenu.svelte';
  import ResearchPanel from '$lib/notes/ResearchPanel.svelte';
  import ReferenceNotePanel from '$lib/notes/ReferenceNotePanel.svelte';
  import NoteSummaryCard from '$lib/notes/NoteSummaryCard.svelte';
  import AskThisNotePanel from '$lib/notes/AskThisNotePanel.svelte';
  import NoteAudioPlayer from '$lib/notes/NoteAudioPlayer.svelte';
  import NotePresentation from '$lib/notes/NotePresentation.svelte';
  import SectionQuestionsPanel from '$lib/notes/SectionQuestionsPanel.svelte';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { ensurePinnedLoaded } from '$lib/notes/pinnedNotes';

  type ViewMode = 'edit' | 'preview' | 'split';
  const VIEW_KEY = 'granit.note.viewMode';
  let viewMode = $state<ViewMode>('edit');

  // Viewport tracking for the rail/tree mount strategy. Tailwind's
  // lg breakpoint is 1024px (left tree threshold) and xl is 1280px
  // (right info-rail threshold). Previously each rail was rendered
  // TWICE — once in a desktop `<aside class="hidden md:flex">` and
  // once in a `<Drawer>` wrapped by `md:hidden contents`. Both DOM
  // trees were always mounted; CSS just hid one. That meant every
  // panel's $derived/$effect ran twice, doubling the per-keystroke
  // cost of body-derived recomputation in the rail panels — a
  // meaningful chunk of the save-time freeze on long notes. We
  // track each breakpoint here and render the rail / tree to ONLY
  // one location at a time.
  // Initial values from synchronous matchMedia. SvelteKit hydrates
  // this component on the client only after the bundle loads, so
  // window is always defined here — but the typeof guard keeps SSR
  // (if it ever happens) from throwing. The onMount block below
  // wires up live updates; this initializer just avoids a one-frame
  // flash where the wrong layout renders before the listener fires.
  let isLg = $state(
    typeof window !== 'undefined' && window.matchMedia('(min-width: 1024px)').matches
  );
  let isXl = $state(
    typeof window !== 'undefined' && window.matchMedia('(min-width: 1280px)').matches
  );
  // Restore preference once at mount.
  onMount(() => {
    try {
      const v = localStorage.getItem(VIEW_KEY);
      if (v === 'edit' || v === 'preview' || v === 'split') viewMode = v;
    } catch {}
    // Boot the pinned-notes store so the toolbar's pin star (and any
    // other pin-aware surface mounted after this) reflects the
    // server-authoritative list without each component re-fetching.
    ensurePinnedLoaded();
    // Two MQL listeners for the lg + xl breakpoints. matchMedia is
    // ubiquitous in our targets; the older addListener fallback covers
    // ancient Safari just in case.
    if (typeof window === 'undefined') return;
    const lgMql = window.matchMedia('(min-width: 1024px)');
    const xlMql = window.matchMedia('(min-width: 1280px)');
    isLg = lgMql.matches;
    isXl = xlMql.matches;
    const onLg = (e: MediaQueryListEvent) => { isLg = e.matches; };
    const onXl = (e: MediaQueryListEvent) => { isXl = e.matches; };
    function add(mql: MediaQueryList, fn: (e: MediaQueryListEvent) => void) {
      if (typeof mql.addEventListener === 'function') mql.addEventListener('change', fn);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      else (mql as any).addListener?.(fn);
    }
    function remove(mql: MediaQueryList, fn: (e: MediaQueryListEvent) => void) {
      if (typeof mql.removeEventListener === 'function') mql.removeEventListener('change', fn);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      else (mql as any).removeListener?.(fn);
    }
    add(lgMql, onLg);
    add(xlMql, onXl);
    return () => {
      remove(lgMql, onLg);
      remove(xlMql, onXl);
    };
  });
  function setViewMode(m: ViewMode) {
    viewMode = m;
    try { localStorage.setItem(VIEW_KEY, m); } catch {}
  }

  let note = $state<Note | null>(null);
  let body = $state('');
  let saving = $state(false);
  let dirty = $state(false);
  let error = $state('');
  let lastLoadedPath = $state('');

  // Reading progress 0..1 — driven by the editor's onScroll callback
  // (rAF-throttled there). When the doc fits in viewport the
  // denominator is 0 and we clamp to 0; once the user scrolls down a
  // long doc, we tint a 2px line at the top of the editor pane to
  // surface 'how far through am I'. Cheap, no polling.
  let readProgress = $state(0);

  // Preview-pane scroll container. Bound below to the rendered
  // preview viewport (when viewMode === 'preview' or 'split'). The
  // Outline panel uses it as the IntersectionObserver root for
  // active-heading tracking, and the per-heading checkpoint logic
  // below treats every heading that scrolls past the top crosshair
  // as "visited" (persisted per note path).
  let previewContainer = $state<HTMLElement | null>(null);
  // Active heading line in the preview, surfaced upward by Outline.
  // We don't strictly need it on the page, but it lets us drive the
  // visited-checkpoint logic from a single source of truth: every
  // heading the reader passes through with a downward scroll gets
  // ticked as visited.
  // Visited-section tracking — persisted per note path under
  // granit.note.visited. See $lib/notes/noteHistory for the cap
  // logic + LRU trim. The local Set mirrors the on-disk slice for
  // the current note so render reads are O(1).
  let visitedHeadings = $state<Set<number>>(new Set());
  $effect(() => {
    const p = note?.path;
    if (!p) { visitedHeadings = new Set(); return; }
    visitedHeadings = new Set(loadVisitedMap()[p] ?? []);
  });
  function markVisited(line: number) {
    if (!note) return;
    if (visitedHeadings.has(line)) return;
    visitedHeadings = recordVisitedLine(note.path, line);
  }
  function resetVisited() {
    if (!note) return;
    visitedHeadings = new Set();
    clearVisitedFor(note.path);
  }
  // Preview-pane reading progress (0..1). Different surface from the
  // editor's `readProgress` because the user can scroll preview
  // independently in split mode. Also fuels the heading-checkpoint
  // marker — every time the preview scrolls, we tick any heading
  // whose top crossed above the viewport's top quarter.
  let previewProgress = $state(0);
  let previewProgressRaf = 0;
  function onPreviewScroll() {
    if (!previewContainer) return;
    if (previewProgressRaf) return;
    previewProgressRaf = requestAnimationFrame(() => {
      previewProgressRaf = 0;
      const c = previewContainer!;
      const denom = Math.max(1, c.scrollHeight - c.clientHeight);
      previewProgress = Math.max(0, Math.min(1, c.scrollTop / denom));
      // Tick every heading whose top is above the viewport's top
      // quarter (matches Outline's active-heading bias).
      const cTop = c.getBoundingClientRect().top;
      const cutoff = cTop + c.clientHeight * 0.25;
      const els = c.querySelectorAll<HTMLElement>('[data-heading-line]');
      for (const el of els) {
        const top = el.getBoundingClientRect().top;
        if (top <= cutoff) {
          const ln = parseInt(el.dataset.headingLine ?? '', 10);
          if (Number.isFinite(ln)) markVisited(ln);
        }
      }
    });
  }
  // Re-attach the scroll listener whenever the container ref or
  // view mode changes. Using onPreviewScroll directly so the rAF
  // throttle stays per-handler.
  $effect(() => {
    const c = previewContainer;
    if (!c) return;
    c.addEventListener('scroll', onPreviewScroll, { passive: true });
    // Initial tick so a doc that loads with the user at top still
    // marks the first heading visible.
    onPreviewScroll();
    return () => c.removeEventListener('scroll', onPreviewScroll);
  });
  let editor:
    | {
        scrollToLine: (n: number) => void;
        getScrollTop: () => number;
        setScrollTop: (top: number) => void;
        isCompletionActive: () => boolean;
        dispatchChord: (chord: string) => void;
        getDOM: () => HTMLElement | undefined;
        openFind: () => void;
        insertAtCursor: (text: string) => void;
      }
    | undefined = $state();
  // Re-derived after every render so the SelectionToolbar can scope
  // its selection detection to the editor's contentDOM specifically.
  // The CodeMirror DOM exists only after mount, so this stays
  // `undefined` until then and the toolbar simply doesn't render.
  let editorDOM = $derived(editor?.getDOM());

  // Per-note scroll position cache lives in $lib/notes/noteHistory —
  // see the imports at the top. Pixel-accurate (not line-accurate)
  // because line tracking misbehaves once the user changes font size
  // or window width — pixels survive reflow because we restore on
  // the same note (same width, same font) only.

  let treeDrawerOpen = $state(false);
  let infoDrawerOpen = $state(false);

  let pinned = $state<Set<string>>(new Set());
  let pinBusy = $state(false);

  async function loadPinned() {
    try {
      const r = await api.listPinned();
      pinned = new Set(r.pinned.map((p) => p.path));
    } catch {}
  }

  async function togglePin() {
    if (!note) return;
    pinBusy = true;
    try {
      const want = !pinned.has(note.path);
      const r = await api.setPinned(note.path, want);
      pinned = new Set(r.pinned.map((p) => p.path));
    } finally {
      pinBusy = false;
    }
  }

  $effect(() => {
    const path = $page.params.path;
    if (path) load(decodeURIComponent(path));
  });

  let draftRestored = $state(false);

  async function load(p: string, opts: { force?: boolean } = {}) {
    error = '';
    draftRestored = false;
    if (!opts.force && lastLoadedPath === p) return;
    // Reset the per-load draft watermark so the first keystroke on the
    // newly-opened note triggers a draft write. Without this, opening a
    // note whose body happens to equal the previous note's last drafted
    // body would skip the very first draft persistence.
    lastDraftedBody = null;
    // Same-note reloads (WS-triggered note.changed) must not clobber
    // in-flight typing. Snapshot the body before the await; if the user
    // types during the fetch, abort the body overwrite and let the
    // auto-save effect persist their edits. For navigation to a
    // different note (note?.path !== p), we always want to overwrite.
    const isSameNoteReload = note?.path === p;
    const bodyAtStart = body;
    lastLoadedPath = p;
    try {
      const fresh = await api.getNote(p);
      if (isSameNoteReload && body !== bodyAtStart) {
        return;
      }
      const serverBody = fresh.body ?? '';

      // Restore a local draft if it diverges from the server. We ALWAYS
      // prefer the draft when it has unsaved typing, even when the
      // server's modTime is newer — losing the user's work silently is
      // worse than the rare case of overwriting a TUI/other-device edit.
      // The most common reason the server is "newer" while a draft
      // diverges is the user typing during the autosave (the draft was
      // written with the pre-save modTime, then save bumped the server's
      // modTime; the draft's body has the keystrokes that came in after
      // the save fired). Discarding it is exactly the wrong move.
      //
      // We still warn the user when the modTime says they may be
      // working from a stale base, so they can manually reconcile if
      // they actually have a multi-device conflict (the rare case).
      const draft = getDraft(p);
      if (draft && draftDivergesFromServer(draft, serverBody)) {
        const serverNewer = new Date(fresh.modTime) > new Date(draft.baseModTime);
        prev = draft.body;
        body = draft.body;
        note = fresh;
        dirty = true;
        draftRestored = true;
        treeDrawerOpen = false;
        infoDrawerOpen = false;
        if (serverNewer) {
          toast.warning('Restored unsaved draft — server moved forward since your last edit. Your version will overwrite on next save.');
        } else {
          toast.info('Restored unsaved draft');
        }
        save({ silent: true });
        return;
      } else if (draft) {
        // Draft matches server — stale, clean up.
        clearDraft(p);
      }

      note = fresh;
      body = serverBody;
      prev = body;
      dirty = false;
      treeDrawerOpen = false;
      infoDrawerOpen = false;
      // Restore the scroll position (per-note, pixel-accurate). Defer
      // a frame so the editor has finished mounting and the scroller
      // has its content height — without the defer the setScrollTop
      // call lands at 0 because the doc just got swapped.
      const remembered = recallScroll(p);
      if (remembered > 0) {
        requestAnimationFrame(() => {
          editor?.setScrollTop?.(remembered);
        });
      }
      // ?line=<n> — incoming jump from /search. Wins over remembered
      // scroll position so a user clicking a search hit lands on the
      // matched line, not yesterday's reading position. We let the
      // editor mount fully before dispatching the scroll.
      const lineParam = $page.url.searchParams.get('line');
      if (lineParam) {
        const ln = parseInt(lineParam, 10);
        if (Number.isFinite(ln) && ln > 0) {
          requestAnimationFrame(() => editor?.scrollToLine?.(ln));
        }
      }
      // Block-level wikilink target — when arriving via [[Note#H]] the
      // url hash carries the heading text. Scroll to the matching
      // line, overriding any remembered scroll position. Only fires
      // when the hash is non-empty so the regular reopen flow keeps
      // its remembered position. Heading-match is case-insensitive
      // and whitespace-collapsed so "  Plan  " in the hash still
      // matches "## Plan" in the body.
      const rawHash = $page.url.hash ? decodeURIComponent($page.url.hash.slice(1)) : '';
      if (rawHash) {
        const target = rawHash.toLowerCase().replace(/\s+/g, ' ').trim();
        const lines = (body ?? '').split('\n');
        let found = -1;
        let inFence = false;
        for (let i = 0; i < lines.length; i++) {
          const t = lines[i].trim();
          if (t.startsWith('```') || t.startsWith('~~~')) { inFence = !inFence; continue; }
          if (inFence) continue;
          const m = /^(#{1,6})\s+(.+?)\s*$/.exec(t);
          if (m && m[2].toLowerCase().replace(/\s+/g, ' ').trim() === target) {
            found = i + 1; // CodeMirror is 1-based
            break;
          }
        }
        if (found > 0) {
          requestAnimationFrame(() => editor?.scrollToLine?.(found));
        }
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      // If we have a local draft, surface it instead of an error so an
      // offline reload doesn't lose work.
      const draft = getDraft(p);
      if (draft) {
        prev = draft.body;
        body = draft.body;
        note = {
          path: p,
          title: p.split('/').pop()!.replace(/\.md$/, ''),
          modTime: new Date().toISOString(),
          size: draft.body.length,
          frontmatter: {},
          body: draft.body
        } as Note;
        dirty = true;
        draftRestored = true;
        toast.warning('offline — showing your local draft');
        return;
      }
      error = msg;
      note = null;
      body = '';
      prev = '';
      dirty = false;
      // Critical: drop the dedupe guard so a refetch of the SAME path
      // is allowed. Without this, the user lands on a 404/network-error
      // note, the page renders the error banner, and any subsequent
      // navigation back to that URL (browser back, retry click,
      // sidebar re-click) silently no-ops because `lastLoadedPath ===
      // p` returns early. The user concludes the page is frozen and
      // hits reload.
      lastLoadedPath = '';
    }
  }

  let lastSavedAt = $state<number | null>(null);
  let nowTick = $state(Date.now());
  let saveFailed = $state(false);
  // Consecutive save-failure counter. Resets to 0 on any success.
  // Used by the in-page banner below to show a sticky, dismiss-only-
  // by-fixing surface so the user always knows when their edits
  // aren't reaching the server. Previously this was silent after the
  // first toast (lastShownSilentError gated subsequent ones), which
  // is exactly how the freeze went undiagnosed: the autosave kept
  // failing, drafts kept queueing, but the UI looked fine.
  let saveFailCount = $state(0);
  let lastSaveError = $state('');

  // Tick once per second so "saved Ns ago" stays accurate.
  $effect(() => {
    const t = setInterval(() => (nowTick = Date.now()), 1000);
    return () => clearInterval(t);
  });

  // [freeze-hunt] diagnostic flag — flip on at runtime via the
  // browser console with `localStorage.setItem('granit.freeze-hunt','1')`
  // then reload. Logs save-path timing markers + WS reload coalesce
  // hits so we can see what's happening on the next freeze report.
  // No-op when off; flag is read inside save() so toggling takes
  // effect for the next save attempt without a reload.
  function freezeHuntOn(): boolean {
    try {
      return typeof localStorage !== 'undefined'
        && localStorage.getItem('granit.freeze-hunt') === '1';
    } catch {
      return false;
    }
  }

  async function save(opts: { silent?: boolean } = {}): Promise<boolean> {
    if (!note || !dirty || saving) return !dirty;
    saving = true;
    error = '';
    // Capture the body at the start of the save. If the user types
    // during the await, body will diverge from sentBody — we must NOT
    // mark the editor clean in that case, or those keystrokes are lost
    // forever (server only got sentBody, prev=body would mask the gap,
    // and the next typing wouldn't trigger a fresh save). Compare body
    // to sentBody after the await to decide whether more work remains.
    const sentBody = body;
    const hunting = freezeHuntOn();
    const t0 = hunting ? performance.now() : 0;
    if (hunting) console.warn('[freeze-hunt] save:start', { path: note.path, bytes: sentBody.length, silent: !!opts.silent });
    try {
      const updated = await api.putNote(note.path, { frontmatter: note.frontmatter as Record<string, unknown>, body: sentBody });
      if (hunting) console.warn('[freeze-hunt] save:put-returned', { ms: (performance.now() - t0).toFixed(1) });
      note = updated;
      prev = sentBody;
      dirty = body !== sentBody;
      lastSavedAt = Date.now();
      saveFailed = false;
      saveFailCount = 0;
      lastSaveError = '';
      if (!dirty) {
        clearDraft(updated.path);
      } else {
        // User typed during the save. The draft on disk still has
        // the OLD modTime as baseModTime, which would cause the
        // "server has newer content" branch to trip on a mid-edit
        // reload. Refresh the draft synchronously with the post-save
        // modTime so a crash / reload in the next 100ms (debounce
        // window) doesn't fall into that path.
        setDraft(updated.path, body, updated.modTime);
      }
      draftRestored = false;
      if (!opts.silent && !dirty) toast.success('saved');
      if (hunting) {
        // Defer one frame so the post-save reactivity wave has had a
        // chance to fire — the timing here tells us how long the
        // reactive cascade took, which is the suspected freeze
        // surface. Logs the total wall-clock between save start and
        // the next paint after all effects ran.
        requestAnimationFrame(() => {
          console.warn('[freeze-hunt] save:reactive-cascade-done', { totalMs: (performance.now() - t0).toFixed(1) });
        });
      }
      return true;
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      error = msg;
      saveFailed = true;
      saveFailCount++;
      lastSaveError = msg;
      // First failure: toast it. Subsequent: the sticky banner below
      // is the surface; the toast would just nag.
      if (!opts.silent || !lastShownSilentError) {
        toast.error(`save failed: ${msg}`);
        lastShownSilentError = true;
      }
      return false;
    } finally {
      saving = false;
    }
  }
  let lastShownSilentError = $state(false);
  // Reset the "silent error already shown" gate when the user manages a
  // successful interaction (so a second outage shows a toast again).
  $effect(() => {
    if (!saveFailed) lastShownSilentError = false;
  });

  let prev = $state('');
  $effect(() => {
    if (body !== prev) {
      dirty = true;
      prev = body;
    }
  });

  // Auto-save: debounce 2s after last edit. If save fails, the next edit
  // re-triggers the timer so we keep retrying as the user continues typing.
  //
  // Hostile-UX guard: while the autocomplete picker is open (user mid-
  // snippet like /callout, mid-wikilink, or mid-tag), saving causes the
  // editor's doc to be re-set on the WS bounce-back, which closes the
  // picker and interrupts what the user was composing. We back off
  // every 1s in that case and only save once the picker is closed.
  // This pattern preserves a single in-flight timer ref (cleaned up
  // properly on effect re-run) instead of leaking re-scheduled timers.
  $effect(() => {
    void body;
    if (!dirty || saving || !note) return;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const trySave = () => {
      timer = null;
      if (!dirty || saving || !note) return;
      if (editor?.isCompletionActive?.()) {
        // Picker open — back off and re-check in 1s.
        timer = setTimeout(trySave, 1000);
        return;
      }
      save({ silent: true });
    };
    timer = setTimeout(trySave, 2000);
    return () => {
      if (timer) clearTimeout(timer);
    };
  });

  // Persist the body to localStorage on every change — synchronously,
  // no debounce. The previous 600ms (and even the 100ms we tried) had
  // a real bug: during continuous typing the timer reset every
  // keystroke and never fired, so a crash mid-paragraph lost
  // everything since the last typing pause. localStorage.setItem is
  // sub-millisecond for typical note sizes, so writing on every
  // keystroke costs ~30ms/sec at fastest realistic typing speeds —
  // imperceptible. This is the bulletproof guarantee: every visible
  // keystroke is on disk before the next paint.
  //
  // Skip when the body hasn't actually changed since the last write.
  // The effect re-runs whenever `note` is reassigned (every successful
  // save creates a new note reference), and writing the same body to
  // localStorage in that case is wasted work — for a multi-MB note
  // the JSON.stringify + setItem can take 10ms+, which adds up when
  // a save bounces every 2s.
  let lastDraftedBody: string | null = null;
  $effect(() => {
    void body;
    if (!note || !dirty) return;
    if (lastDraftedBody === body) return;
    lastDraftedBody = body;
    setDraft(note.path, body, note.modTime);
  });

  // Force-flush draft on tab hide / before unload. Belt-and-suspenders
  // since we already write synchronously per keystroke — but covers
  // the unlikely case of a body change that hasn't propagated to the
  // $effect yet (e.g. an in-flight CodeMirror dispatch the moment
  // the OS suspends the page). localStorage writes are synchronous,
  // so this guarantees the latest body lands before the page goes away.
  $effect(() => {
    if (typeof window === 'undefined') return;
    const flush = () => {
      if (note && dirty) setDraft(note.path, body, note.modTime);
    };
    const onVis = () => { if (document.visibilityState === 'hidden') flush(); };
    window.addEventListener('beforeunload', flush);
    window.addEventListener('pagehide', flush);
    document.addEventListener('visibilitychange', onVis);
    return () => {
      window.removeEventListener('beforeunload', flush);
      window.removeEventListener('pagehide', flush);
      document.removeEventListener('visibilitychange', onVis);
    };
  });

  // When the network comes back, retry any pending save.
  $effect(() => {
    const onOnline = () => {
      if (saveFailed && dirty && !saving) save({ silent: true });
    };
    window.addEventListener('online', onOnline);
    return () => window.removeEventListener('online', onOnline);
  });

  // Best-effort flush on SPA navigation. We can't synchronously block
  // the navigation (await isn't honored by browser navigations), but the
  // draft layer protects against data loss on the worst case — this just
  // tries to push edits over the wire faster than the 2s debounce would.
  beforeNavigate(() => {
    if (dirty && !saving && note) {
      // Body is already in localStorage via setDraft (debounce 600ms).
      // Fire-and-forget the save; it'll race the navigation but either
      // outcome is safe (draft still on disk).
      void save({ silent: true });
    }
    // Remember the scroll position so navigating back to this note
    // returns to where the user was reading. Saved synchronously so
    // even a forced reload (close tab) catches it.
    if (note && editor?.getScrollTop) {
      rememberScroll(note.path, editor.getScrollTop());
    }
  });

  // Save status label that updates with the live tick.
  let saveStatus = $derived.by(() => {
    void nowTick; // keep it reactive
    if (saving) return 'saving…';
    if (saveFailed && dirty) return 'retry?';
    if (dirty) return 'unsaved';
    if (!lastSavedAt) return 'saved';
    const ago = Math.floor((Date.now() - lastSavedAt) / 1000);
    if (ago < 4) return 'saved';
    if (ago < 60) return `saved ${ago}s ago`;
    if (ago < 3600) return `saved ${Math.floor(ago / 60)}m ago`;
    return 'saved';
  });

  // Brief flash after each successful autosave so the user can SEE
  // that an autosave actually fired. Without this, saves are invisible
  // — the status bar updates silently and the user has no positive
  // confirmation that their work made it to disk. The flash window is
  // 1.2s (long enough to register, short enough not to nag) and
  // doesn't fire when the save was triggered by an explicit Mod-S
  // (those already get a toast.success). The flash is a CSS-driven
  // outline pulse; the existing saveStatus label still drives the
  // text content of the button.
  let saveFlash = $state(false);
  let saveFlashTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    void lastSavedAt;
    if (!lastSavedAt) return;
    saveFlash = true;
    if (saveFlashTimer) clearTimeout(saveFlashTimer);
    saveFlashTimer = setTimeout(() => {
      saveFlash = false;
      saveFlashTimer = null;
    }, 1200);
    return () => {
      if (saveFlashTimer) { clearTimeout(saveFlashTimer); saveFlashTimer = null; }
    };
  });

  // ----- Extract-to-note (Mod-Shift-X) -----
  // The Editor component fires onExtract with the selection + an
  // apply() callback; we show the dialog, the user names the new note,
  // and on confirm we POST /notes then call apply(title) which
  // replaces the original selection with [[title]]. The apply is
  // gated on the API call SUCCEEDING — if create fails, the source
  // note isn't mutated and the user can retry without dead links.
  let extractRequest = $state<ExtractRequest | null>(null);
  let askAIRequest = $state<AskAIRequest | null>(null);
  let printOpen = $state(false);
  let historyOpen = $state(false);
  // Focus mode (Mod-Shift-Z) — hides the app sidebar, info panel,
  // and toolbar so the editor takes the full viewport. Persisted to
  // localStorage so the user's preference survives reloads.
  const FOCUS_KEY = 'granit.note.focus';
  let focusMode = $state(
    typeof localStorage !== 'undefined' && localStorage.getItem(FOCUS_KEY) === '1'
  );
  $effect(() => {
    if (typeof localStorage === 'undefined') return;
    try { localStorage.setItem(FOCUS_KEY, focusMode ? '1' : '0'); } catch {}
  });

  // Reading mode — distraction-free preview with serif typography
  // and narrower max-width. Combo: viewMode='preview' + focusMode=true
  // + a CSS class on the preview pane. Toggle via Mod-Shift-R; flip
  // back to whatever the user had before. We remember the prior
  // view + focus state so toggling reading off restores them.
  const READING_KEY = 'granit.note.reading';
  let readingMode = $state(
    typeof localStorage !== 'undefined' && localStorage.getItem(READING_KEY) === '1'
  );
  let priorView: ViewMode | null = null;
  let priorFocus: boolean | null = null;
  function setReadingMode(on: boolean) {
    if (on === readingMode) return;
    if (on) {
      // Snapshot the user's current state so we can restore it.
      priorView = viewMode;
      priorFocus = focusMode;
      viewMode = 'preview';
      focusMode = true;
    } else if (priorView !== null) {
      viewMode = priorView;
      focusMode = priorFocus ?? false;
      priorView = null;
      priorFocus = null;
    }
    readingMode = on;
    try { localStorage.setItem(READING_KEY, on ? '1' : '0'); } catch {}
  }
  function toggleReadingMode() {
    setReadingMode(!readingMode);
  }
  let helpOpen = $state(false);
  // Audio mode — read-aloud player for the current note. Browser
  // SpeechSynthesis only, no backend. Closed by default; opens via
  // the toolbar button.
  let audioOpen = $state(false);
  // Slideshow / presentation mode — fullscreen deck view of the
  // note, split on H2 boundaries. Closed by default; opens via the
  // toolbar button or Mod-Shift-P.
  let presentationOpen = $state(false);

  // Mobile overflow menu — collapses the secondary header buttons
  // (find, print, slideshow, audio, reading, focus, help) into a
  // single ⋯ trigger on phones. Without this the header overflowed
  // horizontally on narrow viewports; the buttons were just
  // `hidden sm:flex` so mobile users had no way to reach them at
  // all. Positioned with the same viewport-aware fixed-coordinate
  // pattern as EditorAIMenu so it never spills off-screen.
  let overflowOpen = $state(false);
  let overflowMenuEl: HTMLDivElement | undefined = $state();
  let overflowTriggerEl: HTMLButtonElement | undefined = $state();
  let overflowMenuTop = $state(0);
  let overflowMenuLeft = $state(0);
  let overflowMenuWidth = $state(240);

  function repositionOverflow() {
    if (!overflowTriggerEl) return;
    const rect = overflowTriggerEl.getBoundingClientRect();
    const vw = window.innerWidth;
    const margin = 8;
    overflowMenuWidth = Math.min(240, vw - margin * 2);
    let left = rect.right - overflowMenuWidth;
    if (left < margin) left = margin;
    if (left + overflowMenuWidth > vw - margin) left = vw - margin - overflowMenuWidth;
    overflowMenuLeft = left;
    overflowMenuTop = rect.bottom + 4;
  }

  $effect(() => {
    if (!overflowOpen) return;
    repositionOverflow();
    function onDocClick(e: MouseEvent) {
      if (!overflowMenuEl || !overflowTriggerEl) return;
      if (e.target instanceof Node && overflowMenuEl.contains(e.target)) return;
      if (e.target instanceof Node && overflowTriggerEl.contains(e.target)) return;
      overflowOpen = false;
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') overflowOpen = false;
    }
    function onResize() { repositionOverflow(); }
    document.addEventListener('mousedown', onDocClick);
    document.addEventListener('keydown', onKey);
    window.addEventListener('resize', onResize);
    window.addEventListener('scroll', onResize, true);
    return () => {
      document.removeEventListener('mousedown', onDocClick);
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', onResize);
      window.removeEventListener('scroll', onResize, true);
    };
  });

  function handleAskAI(req: AskAIRequest) {
    askAIRequest = req;
  }
  function dismissAskAI() {
    askAIRequest?.cancel();
    askAIRequest = null;
  }

  // Whole-note AI action: opens the AskAIDialog with the entire body
  // pre-filled as the selection, so the user gets summary / extract-
  // tasks / suggest-tags / outline / etc. against the whole note
  // without having to select-all first. The replace/insertAfter
  // callbacks splice into the document at the start (replace = whole
  // body) or after the end (insertAfter = append). The user picks
  // the apply mode in the dialog.
  function askAIWholeNote() {
    askAIRequest = {
      text: body,
      replace: (replacement: string) => { body = replacement; dirty = true; },
      insertAfter: (addition: string) => {
        body = body.replace(/\n*$/, '') + '\n\n' + addition + '\n';
        dirty = true;
      },
      cancel: () => {}
    };
  }

  // Global "?" handler — opens the shortcuts cheat sheet from
  // anywhere on the note view, but ONLY when the user isn't typing
  // into an input or the editor (otherwise they couldn't ever type
  // a literal question mark in their notes). The cheap detection
  // looks at the active element's tag + role; any input/textarea/
  // contenteditable is treated as "user is typing".
  $effect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key !== '?' || e.shiftKey === false) return;
      const el = document.activeElement as HTMLElement | null;
      if (!el) return;
      const tag = el.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea') return;
      if (el.isContentEditable) return;
      // CodeMirror's editable surface is a contenteditable div, so the
      // check above already covers the editor. Outside of that, on the
      // note layout (sidebars, toolbar buttons), `?` is free.
      e.preventDefault();
      helpOpen = true;
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  });

  function handleExtract(req: ExtractRequest) {
    extractRequest = req;
  }

  function dismissExtract() {
    extractRequest?.cancel();
    extractRequest = null;
  }

  async function confirmExtract(args: { title: string; path: string; tags: string[] }) {
    if (!extractRequest || !note) return;
    const { title, path, tags } = args;
    const sourceTitle = note.title || note.path;
    const body = `${extractRequest.text.trim()}

---
*Extracted from [[${sourceTitle}]] on ${todayISO()}*
`;
    // Frontmatter: title + extraction provenance + optional tags.
    // Tags are written only when present so a no-tag extract doesn't
    // get a `tags: []` line cluttering the file.
    const frontmatter: Record<string, unknown> = {
      title,
      extracted_from: note.path,
      extracted_at: new Date().toISOString()
    };
    if (tags.length > 0) frontmatter.tags = tags;
    try {
      await api.createNote({ path, frontmatter, body });
    } catch (e) {
      throw e instanceof Error ? e : new Error(String(e));
    }
    extractRequest.apply(title);
    extractRequest = null;
    await save({ silent: true });
    toast.success(`Extracted to [[${title}]]`);
  }

  async function navigateWikilink(target: string) {
    // Best-effort flush of any pending edit. We never block navigation on
    // the save result — the localStorage draft (setDraft, debounce 600ms)
    // already preserves the body, and beforeNavigate flushes again. If the
    // user is offline, save will fail; the draft is still on disk and gets
    // retried automatically when 'online' fires.
    if (dirty) void save({ silent: true });
    // Block-level wikilink: [[Note#Heading]] — split off the fragment
    // and pass it through the URL hash. The receiving page (i.e. this
    // same component on a fresh mount) reads $page.url.hash and
    // scrolls to the heading after the doc loads.
    const [titleRaw, ...frag] = target.split('#');
    const title = titleRaw.trim();
    const hash = frag.length > 0 ? `#${frag.join('#').trim()}` : '';
    try {
      const list = await api.listNotes({ q: title, limit: 5 });
      const exact = list.notes.find((n) => n.title.toLowerCase() === title.toLowerCase());
      const t = exact ?? list.notes[0];
      if (t) {
        goto(`/notes/${encodeURIComponent(t.path)}${hash}`);
        return;
      }
    } catch {}
    goto(`/notes/${encodeURIComponent(title + '.md')}${hash}`);
  }

  $effect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      // Save scroll position synchronously — beforeunload is the last
      // chance before tab close. We also save on beforeNavigate
      // (SPA-internal nav) so the two cover both paths.
      if (note && editor?.getScrollTop) {
        rememberScroll(note.path, editor.getScrollTop());
      }
      if (dirty) {
        e.preventDefault();
        e.returnValue = '';
      }
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });

  // Pinned set — refresh on mount + on WS changes.
  onMount(() => {
    loadPinned();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') loadPinned();
    });
  });

  // Live-reload current note from WS, but never clobber unsaved edits
  // OR our own just-completed save.
  //
  // The own-save guard (lastSavedAt within ~3s) suppresses the reload
  // that the server fires back after WE save: even when bodies match
  // byte-for-byte, the body=serverBody assignment in load() can
  // disturb the editor's autocomplete state (re-running the value
  // effect, even with an equality guard, occasionally clobbers a
  // mid-snippet picker). Skipping our own bounce-back keeps the
  // composing user's flow intact.
  //
  // Reloads from a cross-device save still come through — the user's
  // own save sets lastSavedAt within milliseconds before the bounce-
  // back, so the 3s window is short enough that an external edit
  // arriving moments later still wins.
  //
  // Two correctness guards on top of the original lastSavedAt window:
  //
  // 1. body !== prev — the SYNCHRONOUS "user typed something we
  //    haven't saved yet" signal. The `dirty` flag is updated by an
  //    $effect that runs in a microtask AFTER body changes, leaving
  //    a small race window where a WS event arriving mid-keystroke
  //    saw `dirty=false` and triggered load(). The reload then
  //    overwrote the user's keystrokes with the server's body
  //    (which itself was the pre-edit version). Comparing body to
  //    prev directly catches the in-flight typing without waiting
  //    for the effect microtask.
  //
  // 2. Coalesce reloads to a single trailing-edge call per ~600ms.
  //    The server fires `note.changed` from BOTH the PUT handler
  //    AND the file-watcher (after the PUT writes the file), so a
  //    single autosave produces ≥2 WS events in close succession.
  //    Without coalescing, we'd schedule two `load()` calls and
  //    flash the editor twice. Same trailing-edge pattern that
  //    NotesTree.svelte adopted in 8cf45ba.
  let wsReloadTimer: ReturnType<typeof setTimeout> | null = null;
  function scheduleWsReload(p: string) {
    if (wsReloadTimer) clearTimeout(wsReloadTimer);
    wsReloadTimer = setTimeout(() => {
      wsReloadTimer = null;
      // Re-evaluate the guards at the moment of reload — the user
      // could have started typing during the 600ms window.
      if (!note || note.path !== p) return;
      if (body !== prev || saving) return;
      if (lastSavedAt && Date.now() - lastSavedAt < 3000) return;
      if (freezeHuntOn()) console.warn('[freeze-hunt] ws-reload:fire', { path: p });
      void load(p, { force: true });
    }, 600);
  }
  onMount(() => {
    const off = onWsEvent((ev) => {
      if (ev.type !== 'note.changed') return;
      if (!note || ev.path !== note.path) return;
      // Cheap synchronous-only guards here; the timed evaluation
      // re-checks the rest at fire time.
      if (body !== prev || saving) return;
      if (lastSavedAt && Date.now() - lastSavedAt < 3000) {
        if (freezeHuntOn()) console.warn('[freeze-hunt] ws-reload:suppress-own-bounce', { ageMs: Date.now() - (lastSavedAt ?? 0) });
        return;
      }
      if (freezeHuntOn()) console.warn('[freeze-hunt] ws-reload:schedule', { path: note.path });
      scheduleWsReload(note.path);
    });
    return () => {
      off();
      if (wsReloadTimer) { clearTimeout(wsReloadTimer); wsReloadTimer = null; }
    };
  });

  let wordCount = $derived.by(() => {
    const t = body.trim();
    return t ? t.split(/\s+/).length : 0;
  });
  let charCount = $derived(body.length);
  let lineCount = $derived(body ? body.split('\n').length : 0);
  // Reading time at ~225 wpm — average silent reading speed. Floor of
  // 1 minute so a short note doesn't read "0 min". Hidden under 50
  // words because "<1 min" on a tiny note is noise.
  let readingMinutes = $derived(Math.max(1, Math.round(wordCount / 225)));

  // Word-count goal — frontmatter `target_words: 1500` turns the
  // status-bar word count into a progress indicator. Common shape
  // for journaling / essay drafts where the user committed to a
  // target. We render a thin progress bar under the count + a
  // percentage label so progress is visible at a glance without
  // taking footer space when no target is set.
  let wordGoal = $derived.by<number | null>(() => {
    const fm = note?.frontmatter as Record<string, unknown> | undefined;
    if (!fm) return null;
    const v = fm.target_words ?? fm.word_goal;
    if (typeof v === 'number' && v > 0) return Math.floor(v);
    if (typeof v === 'string') {
      const n = parseInt(v, 10);
      if (!Number.isNaN(n) && n > 0) return n;
    }
    return null;
  });
  let wordGoalPct = $derived.by(() => {
    if (!wordGoal) return 0;
    return Math.min(100, Math.round((wordCount / wordGoal) * 100));
  });

  // Cursor position state — populated by the Editor's onCursor
  // callback. line:col is 1-indexed (matches what every editor
  // status bar shows). selLen > 0 means the user has a selection;
  // we surface a "{N} selected" badge in that case so the user
  // knows how much they're about to act on.
  let cursorLine = $state(1);
  let cursorCol = $state(1);
  let cursorSelLen = $state(0);

  // Last-saved relative time for the status bar. Re-derived every
  // time `lastSavedAt` ticks; the status bar reads it directly.
  let lastSavedDisplay = $state('—');
  $effect(() => {
    function tick() {
      if (!lastSavedAt) {
        lastSavedDisplay = '—';
        return;
      }
      const sec = Math.round((Date.now() - lastSavedAt) / 1000);
      if (sec < 5) lastSavedDisplay = 'just now';
      else if (sec < 60) lastSavedDisplay = `${sec}s ago`;
      else if (sec < 3600) lastSavedDisplay = `${Math.round(sec / 60)}m ago`;
      else lastSavedDisplay = `${Math.round(sec / 3600)}h ago`;
    }
    tick();
    const id = setInterval(tick, 5000);
    return () => clearInterval(id);
  });

  // Mod-/ to cycle view modes (edit → split → preview → edit). A
  // common shortcut in markdown editors (Typora, Obsidian) — the
  // keymap stays inside the editor so we install a window-level
  // handler that ignores the event when the focused element isn't
  // CodeMirror's editable surface (otherwise typing '/' in a form
  // would cycle the view, hostile UX).
  $effect(() => {
    function onKey(e: KeyboardEvent) {
      const isMac = /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent);
      const mod = isMac ? e.metaKey : e.ctrlKey;
      const el = document.activeElement as HTMLElement | null;
      const tag = el?.tagName?.toLowerCase();
      const inInput = tag === 'input' || tag === 'textarea';

      // Mod-/ — cycle view mode (edit → split → preview).
      if (mod && e.key === '/' && !e.shiftKey && !e.altKey) {
        if (inInput) return;
        e.preventDefault();
        const order: ViewMode[] = ['edit', 'split', 'preview'];
        const idx = order.indexOf(viewMode);
        const next = order[(idx + 1) % order.length];
        setViewMode(next);
        return;
      }

      // Mod-Shift-Z — toggle focus mode. Always live (even with the
      // editor focused) since it's a visibility toggle and doesn't
      // collide with any default editor binding.
      if (mod && e.shiftKey && e.key.toLowerCase() === 'z') {
        e.preventDefault();
        focusMode = !focusMode;
        return;
      }

      // Mod-Shift-R — toggle reading mode (preview + focus + serif
      // typography). The reverse-toggle restores whatever view +
      // focus state the user had before, so it composes with the
      // user's normal setup rather than clobbering it.
      if (mod && e.shiftKey && e.key.toLowerCase() === 'r') {
        e.preventDefault();
        toggleReadingMode();
        return;
      }

      // Mod-Shift-P — open slideshow / presentation mode. The
      // overlay's own Esc handler closes it; we don't need a
      // toggle here.
      if (mod && e.shiftKey && e.key.toLowerCase() === 'p') {
        e.preventDefault();
        if (note) presentationOpen = true;
        return;
      }

      // Mod-Shift-←/→ — jump to previous / next daily note. Only on
      // daily notes (otherwise the chord has no obvious target). Skip
      // when typing into a non-editor input.
      if (mod && e.shiftKey && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
        if (!isDaily || !dailyDate || (inInput && el !== editor?.getDOM())) return;
        e.preventDefault();
        const delta = e.key === 'ArrowLeft' ? -1 : 1;
        void gotoDaily(shiftDate(dailyDate, delta));
        return;
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  function jumpToLine(lineNum: number) {
    editor?.scrollToLine(lineNum);
    infoDrawerOpen = false;
  }

  // jumpToWord — find the first line containing the (case-
  // insensitive) word and scroll the editor to it. Used by the
  // concordance panel: clicking a frequent term audits its usage
  // in context. We scan body lines rather than calling editor.find
  // because the editor exposes a generic openFind() but not
  // "select this word at first occurrence" — a one-time linear
  // scan is cheaper than rebuilding that surface.
  function jumpToWord(word: string) {
    if (!body || !word) return;
    const needle = word.toLowerCase();
    const lines = body.split('\n');
    for (let i = 0; i < lines.length; i++) {
      // \b around the needle so "story" doesn't match "history".
      // Re-create per call so the regex picks up the latest word.
      const re = new RegExp(`\\b${needle.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\$&')}\\b`, 'i');
      if (re.test(lines[i])) {
        jumpToLine(i + 1);
        return;
      }
    }
  }

  async function saveFrontmatter(next: Record<string, unknown>) {
    if (!note) return;
    try {
      const updated = await api.putNote(note.path, { frontmatter: next, body });
      note = updated;
      prev = body;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  // ----- Link-suggester glue -----
  // Tags chip → append to frontmatter.tags (de-duplicated).
  // Link chip → insert markup at the editor cursor; if the editor isn't
  // mounted (e.g. preview view), append to the end of the body so the
  // user still gets a working insertion.
  let existingTagList = $derived.by<string[]>(() => {
    const fm = note?.frontmatter as Record<string, unknown> | undefined;
    if (!fm) return [];
    const t = fm.tags;
    if (Array.isArray(t)) return t.map((x) => String(x));
    if (typeof t === 'string') return t.split(/[,\s]+/).filter(Boolean);
    return [];
  });

  async function addSuggestedTag(tag: string) {
    if (!note) return;
    const clean = tag.trim().replace(/^#/, '').toLowerCase();
    if (!clean) return;
    const fm = { ...(note.frontmatter ?? {}) } as Record<string, unknown>;
    let arr: string[] = [];
    if (Array.isArray(fm.tags)) arr = (fm.tags as unknown[]).map((x) => String(x));
    else if (typeof fm.tags === 'string') arr = fm.tags.split(/[,\s]+/).filter(Boolean);
    if (arr.includes(clean)) {
      toast.success(`#${clean} already on this note`);
      return;
    }
    arr.push(clean);
    fm.tags = arr;
    await saveFrontmatter(fm);
    toast.success(`+ #${clean}`);
  }

  function insertSuggestedLink(markup: string) {
    if (editor?.insertAtCursor) {
      editor.insertAtCursor(' ' + markup + ' ');
    } else {
      // Fallback: append + mark dirty so save picks it up.
      body = body + (body.endsWith('\n') ? '' : '\n') + markup + '\n';
      dirty = true;
    }
    toast.success('link inserted');
  }

  // ── AI menu hooks ──────────────────────────────────────────────
  // The menu surfaces all AI actions under one ✨ button. Wholesale
  // body / title / cursor mutations route through these so the
  // editor's normal dirty-tracking + draft-write flow picks them up.
  async function aiMenuSetTitle(title: string) {
    if (!note) return;
    const fm = { ...(note.frontmatter ?? {}) } as Record<string, unknown>;
    fm.title = title;
    await saveFrontmatter(fm);
  }
  function aiMenuInsertAtTop(text: string) {
    if (editor?.insertAtCursor) {
      // We want the text at the start of the body, NOT at cursor —
      // simplest path is to splice into `body` directly. The editor's
      // bind:value picks it up + the doc-replace path in Editor.svelte
      // preserves selection / scroll near the cursor.
    }
    body = text + body;
    dirty = true;
  }
  function aiMenuReplaceBody(next: string) {
    body = next;
    dirty = true;
  }

  // ----- Daily-note navigation -----
  // A note is "daily" when its basename is YYYY-MM-DD.md OR its frontmatter
  // has type=daily. When daily, expose prev/next-day jumps in the header.
  const dailyDateRe = /(\d{4}-\d{2}-\d{2})\.md$/;
  let dailyDate = $derived.by(() => {
    if (!note) return null;
    const m = note.path.match(dailyDateRe);
    if (m) return m[1];
    if (note.frontmatter && (note.frontmatter as Record<string, unknown>).type === 'daily') {
      const d = (note.frontmatter as Record<string, unknown>).date;
      if (typeof d === 'string') return d.slice(0, 10);
    }
    return null;
  });
  let isDaily = $derived(dailyDate !== null);

  function shiftDate(iso: string, days: number): string {
    const [y, m, d] = iso.split('-').map(Number);
    const dt = new Date(y, m - 1, d);
    dt.setDate(dt.getDate() + days);
    const yy = dt.getFullYear();
    const mm = String(dt.getMonth() + 1).padStart(2, '0');
    const dd = String(dt.getDate()).padStart(2, '0');
    return `${yy}-${mm}-${dd}`;
  }
  async function gotoDaily(date: string) {
    if (dirty) void save({ silent: true });
    try {
      // /api/v1/daily/<date> creates today's note if missing; for past/future
      // dates it just returns the existing note (we won't auto-materialize
      // an empty file for arbitrary historical dates).
      const n = await api.daily(date);
      goto(`/notes/${encodeURIComponent(n.path)}`);
    } catch {
      // If no existing daily for that date, just try the canonical path.
      goto(`/notes/${encodeURIComponent(date + '.md')}`);
    }
  }

  // Folder breadcrumbs — derived once so the template stays
  // declarative. Each crumb carries its own folder filter URL so a
  // mid-path click goes "show me everything in <root>/a/b/" without
  // recomputing the prefix in markup. When the user expands a
  // collapsed deep path we flip `breadcrumbExpanded` to render every
  // segment instead of first/…/last.
  let breadcrumbExpanded = $state(false);
  $effect(() => {
    // Reset on note change so the next note doesn't inherit the
    // expanded state from a previous deep path.
    void note?.path;
    breadcrumbExpanded = false;
  });
  interface Crumb { label: string; href: string }
  let allCrumbs = $derived.by<Crumb[]>(() => {
    if (!note) return [];
    const segs = note.path.split('/').slice(0, -1);
    return segs.map((seg, i) => ({
      label: seg,
      href: `/notes?folder=${encodeURIComponent(segs.slice(0, i + 1).join('/'))}`
    }));
  });
  // When the path has more than 3 folder segments we collapse the
  // middle ones into a clickable ellipsis so the bar stays one-line
  // even on deeply-nested paths (e.g. work/projects/2026/q1/notes).
  // Showing the first two + last keeps the most relevant context
  // (top-level area + immediate parent) without truncating the title.
  const CRUMB_COLLAPSE_THRESHOLD = 4;
  let visibleCrumbs = $derived.by<Crumb[]>(() => {
    if (breadcrumbExpanded) return allCrumbs;
    if (allCrumbs.length <= CRUMB_COLLAPSE_THRESHOLD) return allCrumbs;
    // Keep the first two and the last segment; expansion shows all.
    return [...allCrumbs.slice(0, 2), ...allCrumbs.slice(-1)];
  });
  let crumbsCollapsed = $derived(
    !breadcrumbExpanded && allCrumbs.length > CRUMB_COLLAPSE_THRESHOLD
  );

  let dailyLabel = $derived.by(() => {
    if (!dailyDate) return '';
    const today = new Date();
    const todayIso = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`;
    const yesterday = shiftDate(todayIso, -1);
    const tomorrow = shiftDate(todayIso, 1);
    if (dailyDate === todayIso) return 'today';
    if (dailyDate === yesterday) return 'yesterday';
    if (dailyDate === tomorrow) return 'tomorrow';
    const [y, m, d] = dailyDate.split('-').map(Number);
    const dt = new Date(y, m - 1, d);
    return dt.toLocaleDateString(undefined, { weekday: 'long' });
  });
</script>

{#snippet treeContent()}
  <div class="px-2 pt-2 pb-1 text-xs uppercase tracking-wider text-dim flex-shrink-0">Vault</div>
  <NotesTree currentPath={note?.path} onSelect={() => (treeDrawerOpen = false)} />
{/snippet}

{#snippet infoContent()}
  <div class="p-3 space-y-4 overflow-y-auto h-full">
    <section>
      <h3 class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1.5">
        <span>Outline</span>
        {#if visitedHeadings.size > 0}
          <button
            type="button"
            onclick={resetVisited}
            class="ml-auto text-[9px] tracking-normal normal-case text-dim hover:text-error"
            title="clear visited-section ticks for this note"
            aria-label="reset reading progress"
          >reset ✓</button>
        {/if}
      </h3>
      <Outline
        body={body}
        onJump={jumpToLine}
        cursorLine={cursorLine}
        scrollContainer={viewMode !== 'edit' ? previewContainer : null}
        visited={visitedHeadings}
      />
    </section>
    {#if note}
      <!-- Margin annotations — user-authored marginalia anchored
           to a specific line. Sits near the top of the rail because
           it's an active-reading surface (the user wants their
           comments visible while skimming the source text). -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
            <path d="M14 4h6v6"/>
            <path d="M10 20H4v-6"/>
            <path d="M14 4l-4 4M4 20l6-6"/>
          </svg>
          Margin notes
        </h3>
        <AnnotationsPanel
          notePath={note.path}
          activeLine={cursorLine}
          onJumpToLine={jumpToLine}
        />
      </section>
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Local graph</h3>
        <LocalGraph path={note.path} onNavigate={navigateWikilink} />
      </section>
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Backlinks</h3>
        <BacklinksPanel path={note.path} onNavigate={navigateWikilink} />
      </section>
      <!-- Ask-this-note: multi-turn Q&A scoped to ONLY this note.
           Distinct from the broader AI overlay (which has vault
           RAG); this surface is the "explain section 3 / what's
           the contradiction here" companion. Conversation resets
           on note change so context never bleeds across notes. -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
            <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z"/>
          </svg>
          Ask this note
        </h3>
        <AskThisNotePanel notePath={note.path} title={note.title ?? note.path} body={body} />
      </section>
      <!-- Active-reading study questions, scoped to ONE section.
           Distinct from a whole-note question prompt: per-section
           is tighter and produces sharper questions because the
           AI isn't averaging over 4000 words. -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
            <circle cx="12" cy="12" r="9"/>
            <path d="M9.1 9a3 3 0 015.83 1c0 2-3 3-3 3M12 17h.01" stroke-linecap="round"/>
          </svg>
          Section questions
        </h3>
        <SectionQuestionsPanel notePath={note.path} body={body} />
      </section>
      <!-- Research panel: derives highlights / footnotes / outbound
           URLs from the body so a research-style note becomes a
           navigable index. Renders nothing if the note has none of
           those — saves rail space on a fresh note. -->
      <section>
        <ResearchPanel body={body} onJump={jumpToLine} />
      </section>
      <!-- Concordance: top-N most-frequent content words in the
           note. Editing tool ("am I overusing 'really'?") + reading
           tool ("what does this note keep returning to?"). Clicking
           a term jumps to the first occurrence in context. -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
            <path d="M3 6h18M3 12h12M3 18h6"/>
          </svg>
          Word frequencies
        </h3>
        <ConcordancePanel body={body} onJumpToWord={jumpToWord} />
      </section>
      <!-- Reference note: pin any note to read alongside while
           writing. The classic research move — paper open in one
           pane, summary growing in the other. Persists per-current
           note so reopening picks up where you left off. -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
            <path d="M4 4h12a4 4 0 014 4v12H8a4 4 0 01-4-4V4z"/>
            <path d="M8 8h8M8 12h8M8 16h6"/>
          </svg>
          Reference
        </h3>
        <ReferenceNotePanel currentPath={note.path} currentBody={body} currentTitle={note.title ?? ''} />
      </section>
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
            <path d="M12 3l1.2 4.2L17 9l-3.8 1.8L12 15l-1.2-4.2L7 9l3.8-1.8L12 3z" stroke-linejoin="round"/>
          </svg>
          AI link suggester
        </h3>
        <LinkSuggestPanel
          notePath={note.path}
          body={body}
          existingTags={existingTagList}
          onAddTag={addSuggestedTag}
          onInsertLink={insertSuggestedLink}
        />
      </section>
    {/if}
    {#if note}
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Properties</h3>
        <FrontmatterEditor frontmatter={note.frontmatter ?? {}} onChange={saveFrontmatter} />
      </section>
    {/if}
  </div>
{/snippet}

<div class="h-full flex" class:focus-mode={focusMode} class:reading-mode={readingMode}>
  <!-- Tree — gated on the lg breakpoint. Same double-mount story as
       the right info rail: BOTH the desktop aside and the drawer
       used to render at every viewport with CSS hiding one, which
       wasted a NotesTree mount + its WS subscription per page mount.
       Render only the active one based on the live `isLg` flag. -->
  {#if isLg}
    <aside class="hidden lg:flex lg:flex-col lg:w-64 xl:w-72 border-r border-surface1 bg-mantle/40 flex-shrink-0 focus-hide">
      {@render treeContent()}
    </aside>
  {:else}
    <Drawer bind:open={treeDrawerOpen} side="left" responsive width="w-72 sm:w-80">
      <div class="h-full flex flex-col">
        {@render treeContent()}
      </div>
    </Drawer>
  {/if}

  <!-- Center: editor -->
  <div class="flex-1 flex flex-col min-w-0">
    {#if error && !note}
      <!-- Stuck-on-error escape header. When the load failed and we
           have no note to render, the normal header below is hidden
           too — without this the user has no UI to navigate away
           except a full page reload. Keep it minimal: just a back
           link and the error message. -->
      <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle/85 supports-[backdrop-filter]:bg-mantle/60 supports-[backdrop-filter]:backdrop-blur-md sticky top-0 z-20">
        <a
          href="/notes"
          aria-label="back to notes"
          class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </a>
        <button
          onclick={() => (treeDrawerOpen = true)}
          aria-label="vault tree"
          class="lg:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M3 6h18M3 12h18M3 18h18" stroke-linecap="round" />
          </svg>
        </button>
        <h1 class="text-base font-semibold text-text flex-1 truncate">Couldn't open note</h1>
        <button
          onclick={() => { lastLoadedPath = ''; load(decodeURIComponent($page.params.path ?? '')); }}
          class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded hover:border-primary text-text"
        >Retry</button>
      </header>
      <div class="p-6 text-sm text-error">{error}</div>
    {:else if error}
      <div class="px-4 py-2 text-sm text-error border-b border-error/30 bg-error/10 flex-shrink-0">{error}</div>
    {/if}
    {#if note}
      <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle/85 supports-[backdrop-filter]:bg-mantle/60 supports-[backdrop-filter]:backdrop-blur-md sticky top-0 z-20">
        <!-- Hidden on mobile: the layout's top-bar already shows a back
             arrow to /notes for any subpath, so a second one here pushes
             the view-mode toggle (and save button) off the right edge on
             narrow phones. -->
        <a
          href="/notes"
          aria-label="back to notes"
          class="hidden md:flex w-9 h-9 items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </a>
        <button
          onclick={() => (treeDrawerOpen = true)}
          aria-label="vault tree"
          title="vault tree"
          class="lg:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M3 6h18M3 12h18M3 18h18" stroke-linecap="round" />
          </svg>
        </button>
        {#if isDaily && dailyDate}
          <button
            onclick={() => gotoDaily(shiftDate(dailyDate, -1))}
            aria-label="previous day"
            title="previous day"
            class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
          >‹</button>
        {/if}
        <div class="min-w-0 flex-1">
          <!-- Single-line ellipsis with the full title surfaced via
               the native tooltip + an explicit aria-label so the
               hidden tail is still discoverable on hover (desktop)
               and accessible to screen readers. The h1 itself only
               shows up to one row's worth so the buttons on the
               right never get pushed off the viewport. -->
          <h1
            class="text-base sm:text-lg font-semibold text-text truncate"
            title={note.title}
            aria-label={note.title}
          >
            {note.title}
            {#if dailyLabel}
              <span class="ml-2 text-xs font-normal text-dim uppercase tracking-wider">{dailyLabel}</span>
            {/if}
          </h1>
          <!-- Folder breadcrumbs — each segment is a clickable filter
               link back into the notes index. Deep paths collapse to
               first/…/last with the ellipsis acting as an expand
               toggle so the bar stays one-line even on
               work/projects/2026/q1/notes/foo.md. Tag chips render
               beside the trail when present. -->
          <div class="text-[11px] text-dim flex items-center gap-1 min-w-0 flex-nowrap">
            <a href="/notes" class="hover:text-primary flex-shrink-0">vault</a>
            {#each visibleCrumbs as c, i}
              <span class="text-dim/60 flex-shrink-0">/</span>
              {#if crumbsCollapsed && i === 2}
                <button
                  type="button"
                  onclick={() => (breadcrumbExpanded = true)}
                  class="px-1 rounded hover:bg-surface0 hover:text-text flex-shrink-0 font-mono"
                  title="Show full path ({allCrumbs.length} folders)"
                  aria-label="Expand collapsed folders"
                >…</button>
                <span class="text-dim/60 flex-shrink-0">/</span>
              {/if}
              <a
                href={c.href}
                class="hover:text-primary truncate font-mono {i === visibleCrumbs.length - 1 ? '' : 'flex-shrink'}"
                title={c.label}
              >{c.label}</a>
            {/each}
            {#if (note.frontmatter as Record<string, unknown>)?.tags && Array.isArray((note.frontmatter as Record<string, unknown>).tags)}
              <span class="ml-2 hidden sm:flex items-center gap-1 flex-wrap min-w-0">
                {#each ((note.frontmatter as Record<string, unknown>).tags as string[]).slice(0, 6) as t}
                  <a
                    href="/notes?tag={encodeURIComponent(t)}"
                    class="px-1.5 py-0.5 rounded text-[10px] hover:bg-surface1 flex-shrink-0"
                    style="background: color-mix(in srgb, var(--color-secondary) 14%, transparent); color: var(--color-secondary);"
                  >#{t}</a>
                {/each}
              </span>
            {/if}
          </div>
        </div>
        {#if isDaily && dailyDate}
          <button
            onclick={() => gotoDaily(shiftDate(dailyDate, 1))}
            aria-label="next day"
            title="next day"
            class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
          >›</button>
          <button
            onclick={() => gotoDaily('today')}
            aria-label="today"
            title="jump to today"
            class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-primary hidden sm:inline-flex flex-shrink-0"
          >today</button>
        {/if}
        <button
          onclick={togglePin}
          disabled={pinBusy}
          aria-label={pinned.has(note.path) ? 'unpin' : 'pin'}
          title={pinned.has(note.path) ? 'unpin from dashboard' : 'pin to dashboard'}
          class="w-9 h-9 flex items-center justify-center rounded text-lg disabled:opacity-50
            {pinned.has(note.path) ? 'text-warning' : 'text-dim hover:text-warning'}"
        >
          {pinned.has(note.path) ? '★' : '☆'}
        </button>
        <span class="text-xs text-dim hidden sm:inline">
          {wordCount} words{#if wordCount >= 50} · {readingMinutes} min read{#if viewMode === 'preview' && previewProgress > 0.05 && previewProgress < 0.95} · {Math.max(1, Math.ceil(readingMinutes * (1 - previewProgress)))} left{/if}{/if}
        </span>
        <!-- view-mode toggle -->
        <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs">
          {#each [{m: 'edit', l: 'edit', i: '✎'}, {m: 'split', l: 'split', i: '⊟'}, {m: 'preview', l: 'preview', i: '👁'}] as v}
            <button
              onclick={() => setViewMode(v.m as ViewMode)}
              class="px-2.5 py-1.5 {viewMode === v.m ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
              title={v.l}
            >
              <span class="text-[11px]">{v.i}</span>
            </button>
          {/each}
        </div>
        <!-- mobile: 2-mode toggle (edit/preview only) -->
        <button
          onclick={() => setViewMode(viewMode === 'preview' ? 'edit' : 'preview')}
          aria-label={viewMode === 'preview' ? 'edit source' : 'show preview'}
          class="sm:hidden w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-base"
        >
          {viewMode === 'preview' ? '✎' : '👁'}
        </button>
        <!-- Keyboard shortcut cheat sheet — also bound to "?" on
             anywhere outside an editable surface. Hidden on phones
             where the help is less useful (no physical keyboard). -->
        <button
          onclick={() => (helpOpen = true)}
          title="Keyboard shortcuts (?)"
          aria-label="Keyboard shortcuts"
          class="hidden sm:flex w-9 h-9 items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-base"
        >
          <span class="font-mono text-sm">?</span>
        </button>
        <!-- Export PDF — opens a fullscreen print preview with
             configurable header/footer and three layout modes
             (standard / certificate / report). Hidden on the very
             narrowest viewports because the toolbar is already busy
             on phone; tablet+ shows it. -->
        <button
          onclick={() => (printOpen = true)}
          title="Export as PDF (header + footer)"
          aria-label="Export as PDF"
          class="hidden sm:flex w-9 h-9 items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-base"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" class="w-4 h-4">
            <path d="M6 9V4h12v5"/>
            <rect x="6" y="14" width="12" height="6" rx="1"/>
            <path d="M6 17H4a2 2 0 01-2-2v-3a2 2 0 012-2h16a2 2 0 012 2v3a2 2 0 01-2 2h-2"/>
          </svg>
        </button>
        <!-- Find / replace — opens CodeMirror's built-in search
             panel. Mod-F triggers the same panel via the keymap;
             this button surfaces the feature for users who don't
             know the shortcut. -->
        <button
          onclick={() => editor?.openFind()}
          title="Find / replace (Mod-F)"
          aria-label="Find in note"
          class="hidden sm:flex w-9 h-9 items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-base"
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8">
            <circle cx="11" cy="11" r="7"/>
            <path d="M21 21l-4.5-4.5" stroke-linecap="round"/>
          </svg>
        </button>
        <!-- AI menu. Replaces the single ✨ whole-note button with a
             discoverable dropdown of every AI action: chord-driven
             ones (continue, section, selection) plus three new
             whole-note actions (suggest title, pin TL;DR, tighten).
             Single source of truth for AI affordance discovery in
             the editor. -->
        {#if note}
          <EditorAIMenu
            notePath={note.path}
            body={body}
            onAskWholeNote={askAIWholeNote}
            onChord={(chord) => editor?.dispatchChord(chord)}
            onOpenOverlay={openAIOverlay}
            onSetTitle={aiMenuSetTitle}
            onInsertAtTop={aiMenuInsertAtTop}
            onReplaceBody={aiMenuReplaceBody}
          />
        {/if}
        <!-- Mobile overflow trigger — collapses the secondary buttons
             (find, print, slideshow, audio, reading, focus, help)
             into one menu on phones. Hidden on sm+ where there's
             room for those buttons inline. -->
        <button
          bind:this={overflowTriggerEl}
          onclick={() => (overflowOpen = !overflowOpen)}
          aria-label="More actions"
          aria-haspopup="menu"
          aria-expanded={overflowOpen}
          title="More actions"
          class="sm:hidden w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-lg leading-none"
        >⋯</button>
        <!-- Slideshow — open the note as a fullscreen deck split
             on H2 boundaries. Mod-Shift-P also opens it. Hidden on
             phones because driving a deck on a 4" screen is
             pointless. -->
        <button
          onclick={() => (presentationOpen = true)}
          title="Slideshow (Mod-Shift-P)"
          aria-label="Open slideshow presentation"
          class="hidden sm:flex w-9 h-9 items-center justify-center rounded flex-shrink-0 text-base text-subtext hover:text-primary hover:bg-surface0"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" class="w-4 h-4">
            <rect x="3" y="4" width="18" height="13" rx="1.5"/>
            <path d="M8 21h8M12 17v4" stroke-linecap="round"/>
          </svg>
        </button>
        <!-- Audio mode — TTS read-aloud via the browser's Speech-
             Synthesis API. No backend, no AI cost. Walk-and-listen
             to your own notes. The button is a toggle: opens the
             player strip above the editor; the player has its own
             close button too. -->
        <button
          onclick={() => (audioOpen = !audioOpen)}
          title={audioOpen ? 'Close read-aloud player' : 'Read this note aloud (browser TTS)'}
          aria-label={audioOpen ? 'Close audio player' : 'Open audio player'}
          aria-pressed={audioOpen}
          class="hidden sm:flex w-9 h-9 items-center justify-center rounded flex-shrink-0 text-base
            {audioOpen ? 'bg-secondary/15 text-secondary' : 'text-subtext hover:text-secondary hover:bg-surface0'}"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" class="w-4 h-4">
            <path d="M11 5L6 9H2v6h4l5 4V5z" stroke-linejoin="round"/>
            <path d="M15.5 8.5a5 5 0 010 7" stroke-linecap="round"/>
            <path d="M19 5a9 9 0 010 14" stroke-linecap="round"/>
          </svg>
        </button>
        <!-- Reading mode (Mod-Shift-R) — combo toggle: preview +
             focus + serif typography. Restores the user's prior
             view + focus state on toggle off. The button reads as
             a separate affordance from focus mode because it's a
             different commitment (no editing, full reading
             posture). -->
        <button
          onclick={toggleReadingMode}
          title={readingMode ? 'Exit reading mode (Mod-Shift-R)' : 'Reading mode (Mod-Shift-R)'}
          aria-label={readingMode ? 'Exit reading mode' : 'Enter reading mode'}
          aria-pressed={readingMode}
          class="hidden sm:flex w-9 h-9 items-center justify-center rounded flex-shrink-0 text-base
            {readingMode ? 'bg-primary/15 text-primary' : 'text-subtext hover:text-primary hover:bg-surface0'}"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" class="w-4 h-4">
            <path d="M2 5h7a3 3 0 013 3v11a2 2 0 00-2-2H2V5z"/>
            <path d="M22 5h-7a3 3 0 00-3 3v11a2 2 0 012-2h8V5z"/>
          </svg>
        </button>
        <!-- Focus mode (Mod-Shift-Z) — hides the tree + info panel
             so the editor fills the viewport. Persists across page
             loads. The button shows the current state with a
             tinted background so the user can see they're in focus
             mode at a glance. -->
        <button
          onclick={() => (focusMode = !focusMode)}
          title={focusMode ? 'Exit focus mode (Mod-Shift-Z)' : 'Focus mode (Mod-Shift-Z)'}
          aria-label={focusMode ? 'Exit focus mode' : 'Enter focus mode'}
          aria-pressed={focusMode}
          class="hidden sm:flex w-9 h-9 items-center justify-center rounded flex-shrink-0 text-base
            {focusMode ? 'bg-primary/15 text-primary' : 'text-subtext hover:text-primary hover:bg-surface0'}"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" class="w-4 h-4">
            {#if focusMode}
              <path d="M9 4v4H5M15 4v4h4M9 20v-4H5M15 20v-4h4" stroke-linecap="round" stroke-linejoin="round"/>
            {:else}
              <path d="M4 9V5h4M20 9V5h-4M4 15v4h4M20 15v4h-4" stroke-linecap="round" stroke-linejoin="round"/>
            {/if}
          </svg>
        </button>
        <!-- Version history — opens a fullscreen panel showing all
             prior saved versions of this note with one-click
             restore. Snapshot is taken automatically on every save
             (with content-hash dedup, so autosave bursts don't
             pollute the list). The promise: nothing is ever lost. -->
        <button
          onclick={() => (historyOpen = true)}
          title="Version history — every save creates a backup. Nothing is ever lost."
          aria-label="Version history"
          class="flex w-9 h-9 items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-base"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" class="w-4 h-4">
            <circle cx="12" cy="12" r="9"/>
            <path d="M12 7v5l3 2" stroke-linecap="round"/>
            <path d="M3 12a9 9 0 0114-7.5l1 1" stroke-linecap="round"/>
            <path d="M3 4v4h4" stroke-linecap="round"/>
          </svg>
        </button>
        <button
          onclick={() => save()}
          disabled={(!dirty && !saveFailed) || saving}
          title={saveStatus}
          class="px-3 sm:px-4 py-2 rounded text-sm font-medium disabled:opacity-60 transition-shadow
            {saveFailed ? 'bg-error text-mantle' : dirty || saving ? 'bg-primary text-on-primary' : 'bg-surface1 text-subtext'}
            {saveFlash ? 'save-flash' : ''}"
        >
          {saveStatus}
        </button>
        <button
          onclick={() => (infoDrawerOpen = true)}
          aria-label="outline & backlinks"
          class="xl:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M4 6h16M4 10h10M4 14h16M4 18h10" stroke-linecap="round" />
          </svg>
        </button>
      </header>
      {#if isDaily && note}
        {@const np = note.path}
        <!-- Carryover (yesterday's open) + habit checklist render
             above the quick-add bar so they're the first thing the
             user sees on the daily. Both collapse to a header line
             when the user wants the editor max-screen. -->
        <DailyContext onChanged={async () => { lastLoadedPath = ''; await load(np); }} />
        <DailyQuickAdd notePath={np} dailyDate={dailyDate} onAdded={async () => { lastLoadedPath = ''; await load(np); }} />
      {/if}
      <!-- Audio player strip — visible only when the user has
           toggled the audio button. Sits above the deadline strip
           so the controls are at the top of the reading surface,
           the natural place to find a transport bar. The player
           cleans up on unmount, so flipping the toggle off stops
           any in-flight reading. -->
      {#if audioOpen}
        <NoteAudioPlayer
          body={body}
          title={note.title || note.path}
          onClose={() => (audioOpen = false)}
        />
      {/if}
      <!-- Deadline strip — surfaces project/goal-linked deadlines for
           this note. Renders nothing when frontmatter has neither
           field, or none of the deadlines match. -->
      <NoteDeadlinesStrip frontmatter={note.frontmatter ?? null} />
      <!-- Repeated-save-failure banner. Goes sticky after the 2nd
           consecutive failure — earlier failures are surfaced via
           the per-failure toast. The threshold avoids alarming the
           user on a one-off network blip while still making prolonged
           outages obvious. The banner exposes the actual error and a
           manual "retry now" button so the user has agency rather
           than waiting on the silent autosave loop. Drafts on
           localStorage protect their content meanwhile. -->
      {#if saveFailCount >= 2 && note}
        <div
          role="status"
          class="px-3 sm:px-4 py-2 border-b border-error/40 bg-error/10 text-error text-xs sm:text-sm flex items-center gap-3"
        >
          <span class="flex-shrink-0" aria-hidden="true">⚠</span>
          <span class="flex-1 min-w-0">
            <strong class="font-semibold">Autosave failing</strong> ({saveFailCount} attempt{saveFailCount === 1 ? '' : 's'})
            {#if lastSaveError}<span class="text-error/80"> — {lastSaveError}</span>{/if}.
            Your edits are saved locally and will sync when the server is reachable.
          </span>
          <button
            type="button"
            onclick={() => save({ silent: false })}
            disabled={saving}
            class="px-2.5 py-1 rounded bg-error/20 hover:bg-error/30 text-error font-medium flex-shrink-0 disabled:opacity-50"
          >
            {saving ? 'retrying…' : 'retry now'}
          </button>
        </div>
      {/if}
      <!-- Reading-progress bar — thin tinted strip showing how far
           through the note the user has scrolled. Hidden when
           progress is essentially 0 (note fits in viewport) so it
           doesn't render a visible artifact on short notes. The
           transition smooths the value as we throttle the source
           on rAF. -->
      {@const activeProgress = viewMode === 'preview' ? previewProgress : readProgress}
      {#if activeProgress > 0.005}
        <div
          class="h-[2px] bg-primary/70 transition-[width] duration-100 ease-out"
          style="width: {(activeProgress * 100).toFixed(1)}%"
          aria-hidden="true"
        ></div>
      {/if}
      <div class="flex-1 min-h-0 p-2 sm:p-3">
        {#if viewMode === 'edit'}
          <Editor bind:value={body} bind:this={editor} onSave={save} onNavigate={navigateWikilink} onExtract={handleExtract} onAskAI={handleAskAI} onCursor={(c) => { cursorLine = c.line; cursorCol = c.col; cursorSelLen = c.selLen; }} onScroll={(s) => { const denom = Math.max(1, s.height - s.viewport); readProgress = Math.max(0, Math.min(1, s.top / denom)); }} />
        {:else if viewMode === 'preview'}
          <div class="h-full overflow-y-auto bg-surface0 border border-surface1 rounded px-4 sm:px-6 py-4" bind:this={previewContainer}>
            <div class="max-w-3xl mx-auto">
              {#if note}
                <NoteSummaryCard
                  notePath={note.path}
                  title={note.title || note.path}
                  body={body}
                  frontmatter={(note.frontmatter ?? {}) as Record<string, unknown>}
                  onSaveFrontmatter={saveFrontmatter}
                  onPrepend={(text) => { body = text + body; dirty = true; }}
                />
              {/if}
              <MarkdownRenderer body={body} onWikilink={navigateWikilink} />
            </div>
          </div>
        {:else}
          <!-- split (desktop only) -->
          <div class="h-full grid grid-cols-1 lg:grid-cols-2 gap-2">
            <Editor bind:value={body} bind:this={editor} onSave={save} onNavigate={navigateWikilink} onExtract={handleExtract} onAskAI={handleAskAI} onCursor={(c) => { cursorLine = c.line; cursorCol = c.col; cursorSelLen = c.selLen; }} onScroll={(s) => { const denom = Math.max(1, s.height - s.viewport); readProgress = Math.max(0, Math.min(1, s.top / denom)); }} />
            <div class="h-full overflow-y-auto bg-surface0 border border-surface1 rounded px-4 sm:px-6 py-4 hidden lg:block" bind:this={previewContainer}>
              <MarkdownRenderer body={body} onWikilink={navigateWikilink} />
            </div>
          </div>
        {/if}
      </div>
      <!-- Status bar — always visible (mobile + desktop). The
           previous version was md:hidden, which left desktop users
           with no live word/char/line/cursor readout. The desktop
           layout fits more datapoints; mobile collapses to the
           essentials.

           Order: counts (words · chars · lines) · reading time ·
           cursor (line:col + selection length) · last saved.
           Right side carries autocomplete hint on mobile only —
           desktop has the help button in the header. -->
      <footer
        class="px-3 py-1.5 border-t border-surface1 text-[11px] text-dim flex items-center gap-3 flex-wrap"
        style="padding-bottom: max(0.375rem, env(safe-area-inset-bottom));"
      >
        {#if wordGoal}
          <!-- Word-count goal progress: chip + tiny progress bar
               surfaces a writing target set in frontmatter
               (target_words: 1500). When the goal is hit, palette
               flips to success so the user sees the win. -->
          <span class="inline-flex items-baseline gap-1.5 font-mono tabular-nums">
            <span class={wordCount >= wordGoal ? 'text-success font-semibold' : 'text-text'}>
              {wordCount.toLocaleString()}/{wordGoal.toLocaleString()}
            </span>
            <span class="text-dim">words</span>
            <span class="inline-block w-12 h-1 rounded bg-surface1 overflow-hidden align-middle relative">
              <span
                class="absolute inset-y-0 left-0 {wordCount >= wordGoal ? 'bg-success' : 'bg-primary'}"
                style="width: {wordGoalPct}%"
              ></span>
            </span>
            <span class="text-dim">{wordGoalPct}%</span>
          </span>
        {:else}
          <span class="font-mono tabular-nums">{wordCount} words</span>
        {/if}
        <span class="hidden sm:inline opacity-60">·</span>
        <span class="hidden sm:inline font-mono tabular-nums">{charCount.toLocaleString()} chars</span>
        <span class="hidden md:inline opacity-60">·</span>
        <span class="hidden md:inline font-mono tabular-nums">{lineCount} lines</span>
        {#if wordCount >= 50}
          <span class="opacity-60">·</span>
          <span>{readingMinutes} min read</span>
        {/if}
        {#if viewMode !== 'preview'}
          <span class="hidden sm:inline opacity-60">·</span>
          <span class="hidden sm:inline font-mono tabular-nums">
            Ln {cursorLine}, Col {cursorCol}{#if cursorSelLen > 0} · {cursorSelLen} sel{/if}
          </span>
        {/if}
        <span class="flex-1"></span>
        {#if lastSavedAt}
          <span class="hidden sm:inline">Saved {lastSavedDisplay}</span>
        {/if}
        <span class="md:hidden opacity-60">[[ autocomplete · ⌘-click links</span>
      </footer>
    {:else}
      <div class="p-6 text-sm text-dim">loading…</div>
    {/if}
  </div>

  <!-- Right info panel — gated on viewport. Previously BOTH the
       desktop aside AND the drawer rendered the same `infoContent`
       snippet, with CSS hiding one. Each panel inside that snippet
       therefore mounted twice and ran its $derived/$effect work
       twice on every keystroke. Now the snippet renders to exactly
       one of them based on the live `isXl` flag (matchMedia listener
       set on mount). Saves ~half the per-keystroke recompute when
       editing on any non-xl viewport, and keeps the desktop layout
       unchanged when isXl is true. Focus-mode still hides the rail. -->
  {#if isXl}
    <aside class="hidden xl:flex xl:flex-col xl:w-72 border-l border-surface1 bg-mantle/40 flex-shrink-0 focus-hide">
      {@render infoContent()}
    </aside>
  {:else}
    <Drawer bind:open={infoDrawerOpen} side="right" responsive width="w-80 sm:w-96">
      {@render infoContent()}
    </Drawer>
  {/if}
</div>

<!-- Extract-to-note dialog. Lives at the page root so it overlays
     above the editor + sidebars on every viewport size. The
     ExtractRequest is null when no extraction is in flight, so the
     dialog renders nothing. -->
<ExtractToNoteDialog
  request={extractRequest}
  sourcePath={note?.path ?? ''}
  onConfirm={confirmExtract}
  onDismiss={dismissExtract}
/>

<!-- Print preview overlay. Renders nothing while closed; mounted at
     the page root so its `body > *:not(.print-overlay)` print rule
     reliably hides everything else. -->
{#if note}
  <PrintPreview
    bind:open={printOpen}
    title={note.title || note.path}
    body={body}
    sourcePath={note.path}
    onClose={() => (printOpen = false)}
  />
{/if}

<!-- Version history overlay. Restore returns the body of the chosen
     snapshot; we set `body` so the editor reflects it immediately,
     mark dirty so the next autosave persists the restored content,
     and let the panel's own loadVersions() refresh the list (the
     pre-restore content was itself snapshotted server-side). -->
{#if note}
  <HistoryPanel
    bind:open={historyOpen}
    notePath={note.path}
    currentBody={body}
    onRestore={(restoredBody: string) => {
      body = restoredBody;
      dirty = true;
    }}
  />
{/if}

<!-- Keyboard cheat sheet. Triggered by "?" anywhere outside an
     editable surface, or via the toolbar help button. -->
<ShortcutsHelpOverlay
  bind:open={helpOpen}
  onClose={() => (helpOpen = false)}
/>

<!-- Slideshow / presentation mode — fullscreen deck view. Mounted
     at the page root so it overlays sidebars; component renders
     nothing while closed. -->
{#if note}
  <NotePresentation
    body={body}
    title={note.title || note.path}
    open={presentationOpen}
    onClose={() => (presentationOpen = false)}
  />
{/if}

<!-- Floating selection toolbar — appears above any text selection
     inside the editor. The chord-dispatch path means buttons take
     the same code route as the keyboard shortcuts (single source
     of truth: the keymap). Hidden on mobile via CSS and on print
     surfaces. -->
<SelectionToolbar
  container={editorDOM}
  onCommand={(chord) => editor?.dispatchChord(chord)}
/>

<!-- Ask-AI dialog — fired by Mod-Shift-A or the toolbar's ✨ button.
     The selection is sent to /api/v1/chat with optional preset or
     custom instruction; the response opens with Copy / Replace /
     Insert below action buttons. -->
<AskAIDialog
  request={askAIRequest}
  sourcePath={note?.path ?? ''}
  onDismiss={dismissAskAI}
/>

<!-- Mobile overflow popover — surfaces the secondary header
     actions on phones. Rendered with `position: fixed` and
     viewport-clamped coordinates so it escapes any ancestor
     overflow (the editor / drawer ancestors) and never lands
     off-screen on narrow phones. -->
{#if overflowOpen && note}
  <div
    bind:this={overflowMenuEl}
    role="menu"
    aria-label="More actions"
    class="fixed z-50 bg-mantle border border-surface1 rounded-md shadow-xl py-1 text-sm"
    style="top: {overflowMenuTop}px; left: {overflowMenuLeft}px; width: {overflowMenuWidth}px;"
  >
    <button
      type="button"
      role="menuitem"
      onclick={() => { overflowOpen = false; editor?.openFind(); }}
      class="w-full px-3 py-2 flex items-center gap-2.5 text-text hover:bg-surface0 text-left min-h-[2.25rem]"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <circle cx="11" cy="11" r="7"/>
        <path d="M21 21l-4.5-4.5" stroke-linecap="round"/>
      </svg>
      <span>Find / replace</span>
    </button>
    <button
      type="button"
      role="menuitem"
      onclick={() => { overflowOpen = false; printOpen = true; }}
      class="w-full px-3 py-2 flex items-center gap-2.5 text-text hover:bg-surface0 text-left min-h-[2.25rem]"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <path d="M6 9V4h12v5"/>
        <rect x="6" y="14" width="12" height="6" rx="1"/>
        <path d="M6 17H4a2 2 0 01-2-2v-3a2 2 0 012-2h16a2 2 0 012 2v3a2 2 0 01-2 2h-2"/>
      </svg>
      <span>Export PDF</span>
    </button>
    <button
      type="button"
      role="menuitem"
      onclick={() => { overflowOpen = false; presentationOpen = true; }}
      class="w-full px-3 py-2 flex items-center gap-2.5 text-text hover:bg-surface0 text-left min-h-[2.25rem]"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <rect x="3" y="4" width="18" height="13" rx="1.5"/>
        <path d="M8 21h8M12 17v4" stroke-linecap="round"/>
      </svg>
      <span>Slideshow</span>
    </button>
    <button
      type="button"
      role="menuitemcheckbox"
      aria-checked={audioOpen}
      onclick={() => { overflowOpen = false; audioOpen = !audioOpen; }}
      class="w-full px-3 py-2 flex items-center gap-2.5 hover:bg-surface0 text-left min-h-[2.25rem]
        {audioOpen ? 'text-secondary' : 'text-text'}"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <path d="M11 5L6 9H2v6h4l5 4V5z" stroke-linejoin="round"/>
        <path d="M15.5 8.5a5 5 0 010 7" stroke-linecap="round"/>
        <path d="M19 5a9 9 0 010 14" stroke-linecap="round"/>
      </svg>
      <span>{audioOpen ? 'Close audio' : 'Read aloud'}</span>
    </button>
    <button
      type="button"
      role="menuitemcheckbox"
      aria-checked={readingMode}
      onclick={() => { overflowOpen = false; toggleReadingMode(); }}
      class="w-full px-3 py-2 flex items-center gap-2.5 hover:bg-surface0 text-left min-h-[2.25rem]
        {readingMode ? 'text-primary' : 'text-text'}"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <path d="M2 5h7a3 3 0 013 3v11a2 2 0 00-2-2H2V5z"/>
        <path d="M22 5h-7a3 3 0 00-3 3v11a2 2 0 012-2h8V5z"/>
      </svg>
      <span>{readingMode ? 'Exit reading mode' : 'Reading mode'}</span>
    </button>
    <button
      type="button"
      role="menuitemcheckbox"
      aria-checked={focusMode}
      onclick={() => { overflowOpen = false; focusMode = !focusMode; }}
      class="w-full px-3 py-2 flex items-center gap-2.5 hover:bg-surface0 text-left min-h-[2.25rem]
        {focusMode ? 'text-primary' : 'text-text'}"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        {#if focusMode}
          <path d="M9 4v4H5M15 4v4h4M9 20v-4H5M15 20v-4h4" stroke-linecap="round" stroke-linejoin="round"/>
        {:else}
          <path d="M4 9V5h4M20 9V5h-4M4 15v4h4M20 15v4h-4" stroke-linecap="round" stroke-linejoin="round"/>
        {/if}
      </svg>
      <span>{focusMode ? 'Exit focus mode' : 'Focus mode'}</span>
    </button>
    <div class="border-t border-surface1 my-1"></div>
    <button
      type="button"
      role="menuitem"
      onclick={() => { overflowOpen = false; helpOpen = true; }}
      class="w-full px-3 py-2 flex items-center gap-2.5 text-text hover:bg-surface0 text-left min-h-[2.25rem]"
    >
      <span class="w-4 h-4 flex-shrink-0 flex items-center justify-center font-mono text-sm">?</span>
      <span>Keyboard shortcuts</span>
    </button>
  </div>
{/if}

<style>
  /* Save-button success flash — a 1.2s outline pulse fires whenever
     lastSavedAt updates, so the user gets positive visual confirmation
     that their autosave actually landed. Uses an outline ring (not a
     colour swap) so the existing dirty/saved/error palette on the
     button still reads through underneath. */
  @keyframes save-flash {
    0%   { box-shadow: 0 0 0 0 rgb(var(--color-success-rgb, 34 197 94) / 0.55); }
    60%  { box-shadow: 0 0 0 6px rgb(var(--color-success-rgb, 34 197 94) / 0); }
    100% { box-shadow: 0 0 0 0 rgb(var(--color-success-rgb, 34 197 94) / 0); }
  }
  .save-flash {
    animation: save-flash 1.2s ease-out 1;
  }
  /* Focus mode: hide the side asides (tree on the left, info on the
     right) so the editor pane fills the available width. The header
     and footer stay — they're tightly bound to the editing flow
     (save state, word count, daily-nav buttons). */
  .focus-mode .focus-hide {
    display: none !important;
  }
  /* Touch tap-target floor — on coarse-pointer devices (phones /
     tablets without a precise mouse) every header button needs at
     least 36px tall so the interactive surface meets the WCAG
     pointer-target guidance. Desktop with a fine pointer keeps the
     denser 28-32px sizes the toolbar was designed around. */
  @media (pointer: coarse) {
    :global(header button),
    :global(header a) {
      min-height: 2.25rem;
    }
  }
  /* Reading mode: serif typography + narrower max-width on the
     preview pane so the user lands in a "I'm reading a book"
     posture, not a "I'm editing in a text box" one. We compose with
     focus-mode's hidden asides so reading mode reads as a single
     centered column. The :global selectors are needed because
     Svelte's scoped CSS would otherwise miss the MarkdownRenderer's
     internal elements. */
  .reading-mode :global(.prose-note),
  .reading-mode :global([class*="MarkdownRenderer"]) {
    font-family: 'Source Serif 4', 'Iowan Old Style', 'Georgia', serif;
  }
  .reading-mode :global(.prose-note) {
    max-width: 64ch;
    margin-left: auto;
    margin-right: auto;
    font-size: 1.05rem;
    line-height: 1.7;
  }
  .reading-mode :global(.prose-note h1) { font-size: 1.7rem; line-height: 1.25; }
  .reading-mode :global(.prose-note h2) { font-size: 1.35rem; line-height: 1.3; margin-top: 1.6em; }
  .reading-mode :global(.prose-note h3) { font-size: 1.15rem; margin-top: 1.4em; }
  .reading-mode :global(.prose-note p) { margin: 0.85em 0; }
  .reading-mode :global(.prose-note blockquote) {
    font-style: italic;
    border-left-width: 3px;
    padding-left: 1.1em;
  }
</style>
