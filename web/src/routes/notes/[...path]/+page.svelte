<script lang="ts">
  import { onMount } from 'svelte';
  import { goto, beforeNavigate } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Editor from '$lib/editor/Editor.svelte';
  import NotesTree from '$lib/notes/NotesTree.svelte';
  import Outline from '$lib/notes/Outline.svelte';
  import BacklinksPanel from '$lib/notes/BacklinksPanel.svelte';
  import LocalGraph from '$lib/notes/LocalGraph.svelte';
  import FrontmatterEditor from '$lib/notes/FrontmatterEditor.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import DailyQuickAdd from '$lib/notes/DailyQuickAdd.svelte';
  import DailyContext from '$lib/notes/DailyContext.svelte';
  import NoteDeadlinesStrip from '$lib/deadlines/NoteDeadlinesStrip.svelte';
  import Drawer from '$lib/components/Drawer.svelte';
  import { toast } from '$lib/components/toast';
  import { getDraft, setDraft, clearDraft, draftDivergesFromServer } from '$lib/notes/drafts';
  import ExtractToNoteDialog from '$lib/notes/ExtractToNoteDialog.svelte';
  import type { ExtractRequest } from '$lib/editor/extract-note';
  import AskAIDialog from '$lib/notes/AskAIDialog.svelte';
  import type { AskAIRequest } from '$lib/editor/ask-ai';
  import PrintPreview from '$lib/notes/PrintPreview.svelte';
  import HistoryPanel from '$lib/notes/HistoryPanel.svelte';
  import ShortcutsHelpOverlay from '$lib/notes/ShortcutsHelpOverlay.svelte';
  import SelectionToolbar from '$lib/editor/SelectionToolbar.svelte';

  type ViewMode = 'edit' | 'preview' | 'split';
  const VIEW_KEY = 'granit.note.viewMode';
  let viewMode = $state<ViewMode>('edit');
  // Restore preference once at mount.
  onMount(() => {
    try {
      const v = localStorage.getItem(VIEW_KEY);
      if (v === 'edit' || v === 'preview' || v === 'split') viewMode = v;
    } catch {}
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
  let editor:
    | {
        scrollToLine: (n: number) => void;
        getScrollTop: () => number;
        setScrollTop: (top: number) => void;
        isCompletionActive: () => boolean;
        dispatchChord: (chord: string) => void;
        getDOM: () => HTMLElement | undefined;
        openFind: () => void;
      }
    | undefined = $state();
  // Re-derived after every render so the SelectionToolbar can scope
  // its selection detection to the editor's contentDOM specifically.
  // The CodeMirror DOM exists only after mount, so this stays
  // `undefined` until then and the toolbar simply doesn't render.
  let editorDOM = $derived(editor?.getDOM());

  // Per-note scroll position cache. Pixel-accurate (not line-accurate)
  // because line tracking misbehaves once the user changes font size or
  // window width — pixels survive reflow because we restore on the
  // same note (same width, same font) only.
  // localStorage'd so a page reload, tab close, or device handoff
  // also lands the user back at the right spot.
  const SCROLL_KEY = 'granit.note.scroll';
  function loadScrollMap(): Record<string, number> {
    if (typeof localStorage === 'undefined') return {};
    try {
      return (JSON.parse(localStorage.getItem(SCROLL_KEY) ?? '{}') as Record<string, number>) || {};
    } catch {
      return {};
    }
  }
  function saveScrollMap(m: Record<string, number>) {
    try { localStorage.setItem(SCROLL_KEY, JSON.stringify(m)); } catch {}
  }
  function rememberScroll(path: string, top: number) {
    if (top <= 0) return;
    const m = loadScrollMap();
    m[path] = top;
    // Cap the map at the 200 most-recently-visited notes so we don't
    // grow localStorage indefinitely. Cheap heuristic: when oversized,
    // drop a random 50; the user's recently-viewed notes still land.
    const keys = Object.keys(m);
    if (keys.length > 200) {
      const drop = keys.slice(0, keys.length - 150);
      for (const k of drop) delete m[k];
    }
    saveScrollMap(m);
  }
  function recallScroll(path: string): number {
    return loadScrollMap()[path] ?? 0;
  }

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

  async function load(p: string) {
    error = '';
    draftRestored = false;
    if (lastLoadedPath === p) return;
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

  // Tick once per second so "saved Ns ago" stays accurate.
  $effect(() => {
    const t = setInterval(() => (nowTick = Date.now()), 1000);
    return () => clearInterval(t);
  });

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
    try {
      const updated = await api.putNote(note.path, { frontmatter: note.frontmatter as Record<string, unknown>, body: sentBody });
      note = updated;
      prev = sentBody;
      dirty = body !== sentBody;
      lastSavedAt = Date.now();
      saveFailed = false;
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
      return true;
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      error = msg;
      saveFailed = true;
      // Even silent auto-saves toast the failure once — we'd rather warn
      // the user than have them assume their work is safe.
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
  $effect(() => {
    void body;
    if (!note || !dirty) return;
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
  let helpOpen = $state(false);

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
*Extracted from [[${sourceTitle}]] on ${new Date().toISOString().slice(0, 10)}*
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
  onMount(() =>
    onWsEvent((ev) => {
      if (ev.type !== 'note.changed') return;
      if (!note || ev.path !== note.path) return;
      if (dirty || saving) return;
      if (lastSavedAt && Date.now() - lastSavedAt < 3000) return;
      lastLoadedPath = '';
      load(note.path);
    })
  );

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
      <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Outline</h3>
      <Outline body={body} onJump={jumpToLine} />
    </section>
    {#if note}
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Local graph</h3>
        <LocalGraph path={note.path} onNavigate={navigateWikilink} />
      </section>
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Backlinks</h3>
        <BacklinksPanel path={note.path} onNavigate={navigateWikilink} />
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

<div class="h-full flex" class:focus-mode={focusMode}>
  <!-- Tree (desktop only). Hidden in focus mode so the editor takes
       the full viewport — toggle with Mod-Shift-Z or the focus
       button in the header. -->
  <aside class="hidden lg:flex lg:flex-col lg:w-64 xl:w-72 border-r border-surface1 bg-mantle/40 flex-shrink-0 focus-hide">
    {@render treeContent()}
  </aside>

  <!-- Tree drawer (mobile + tablet) -->
  <Drawer bind:open={treeDrawerOpen} side="left">
    <div class="h-full flex flex-col">
      {@render treeContent()}
    </div>
  </Drawer>

  <!-- Center: editor -->
  <div class="flex-1 flex flex-col min-w-0">
    {#if error && !note}
      <!-- Stuck-on-error escape header. When the load failed and we
           have no note to render, the normal header below is hidden
           too — without this the user has no UI to navigate away
           except a full page reload. Keep it minimal: just a back
           link and the error message. -->
      <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0">
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
      <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0">
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
          <h1 class="text-base sm:text-lg font-semibold text-text truncate">
            {note.title}
            {#if dailyLabel}
              <span class="ml-2 text-xs font-normal text-dim uppercase tracking-wider">{dailyLabel}</span>
            {/if}
          </h1>
          <!-- Folder breadcrumbs — each segment is a clickable filter
               link back into the notes index. Plus tag chips pulled
               from the frontmatter when present. -->
          <div class="text-[11px] text-dim flex items-center gap-1 truncate">
            <a href="/notes" class="hover:text-primary">vault</a>
            {#each note.path.split('/').slice(0, -1) as seg, i}
              <span class="text-dim/60">/</span>
              <a href="/notes?folder={encodeURIComponent(note.path.split('/').slice(0, i + 1).join('/'))}" class="hover:text-primary truncate font-mono">{seg}</a>
            {/each}
            {#if (note.frontmatter as Record<string, unknown>)?.tags && Array.isArray((note.frontmatter as Record<string, unknown>).tags)}
              <span class="ml-2 flex items-center gap-1 flex-wrap">
                {#each ((note.frontmatter as Record<string, unknown>).tags as string[]).slice(0, 6) as t}
                  <a
                    href="/notes?tag={encodeURIComponent(t)}"
                    class="px-1.5 py-0.5 rounded text-[10px] hover:bg-surface1"
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
          {wordCount} words{#if wordCount >= 50} · {readingMinutes} min read{/if}
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
        <!-- Whole-note AI. Opens the AskAIDialog with the entire
             body pre-filled, so the user can summarise / extract
             tasks / suggest tags / outline against the whole note
             without selecting first. The dialog still supports the
             selection-based shortcut (Mod-Shift-A) — this is just
             the no-selection entry point. -->
        <button
          onclick={askAIWholeNote}
          title="Ask AI about this note"
          aria-label="Ask AI about this note"
          class="hidden sm:flex w-9 h-9 items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-base"
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5L12 3z" stroke-linejoin="round"/>
            <path d="M19 14l.7 2.1L22 17l-2.3.9L19 20l-.7-2.1L16 17l2.3-.9L19 14z" stroke-linejoin="round"/>
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
          title="Version history"
          aria-label="Version history"
          class="hidden sm:flex w-9 h-9 items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-base"
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
          class="px-3 sm:px-4 py-2 rounded text-sm font-medium disabled:opacity-60
            {saveFailed ? 'bg-error text-mantle' : dirty || saving ? 'bg-primary text-on-primary' : 'bg-surface1 text-subtext'}"
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
      <!-- Deadline strip — surfaces project/goal-linked deadlines for
           this note. Renders nothing when frontmatter has neither
           field, or none of the deadlines match. -->
      <NoteDeadlinesStrip frontmatter={note.frontmatter ?? null} />
      <div class="flex-1 min-h-0 p-2 sm:p-3">
        {#if viewMode === 'edit'}
          <Editor bind:value={body} bind:this={editor} onSave={save} onNavigate={navigateWikilink} onExtract={handleExtract} onAskAI={handleAskAI} onCursor={(c) => { cursorLine = c.line; cursorCol = c.col; cursorSelLen = c.selLen; }} />
        {:else if viewMode === 'preview'}
          <div class="h-full overflow-y-auto bg-surface0 border border-surface1 rounded px-4 sm:px-6 py-4">
            <div class="max-w-3xl mx-auto">
              <MarkdownRenderer body={body} onWikilink={navigateWikilink} />
            </div>
          </div>
        {:else}
          <!-- split (desktop only) -->
          <div class="h-full grid grid-cols-1 lg:grid-cols-2 gap-2">
            <Editor bind:value={body} bind:this={editor} onSave={save} onNavigate={navigateWikilink} onExtract={handleExtract} onAskAI={handleAskAI} onCursor={(c) => { cursorLine = c.line; cursorCol = c.col; cursorSelLen = c.selLen; }} />
            <div class="h-full overflow-y-auto bg-surface0 border border-surface1 rounded px-4 sm:px-6 py-4 hidden lg:block">
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
      <footer class="px-3 py-1.5 border-t border-surface1 text-[11px] text-dim flex items-center gap-3 flex-wrap">
        <span class="font-mono tabular-nums">{wordCount} words</span>
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

  <!-- Right info panel (desktop xl+) — also hidden in focus mode. -->
  <aside class="hidden xl:flex xl:flex-col xl:w-72 border-l border-surface1 bg-mantle/40 flex-shrink-0 focus-hide">
    {@render infoContent()}
  </aside>

  <!-- Info drawer (mobile + tablet + desktop without xl) -->
  <Drawer bind:open={infoDrawerOpen} side="right">
    {@render infoContent()}
  </Drawer>
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

<style>
  /* Focus mode: hide the side asides (tree on the left, info on the
     right) so the editor pane fills the available width. The header
     and footer stay — they're tightly bound to the editing flow
     (save state, word count, daily-nav buttons). */
  .focus-mode .focus-hide {
    display: none !important;
  }
</style>
