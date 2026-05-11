<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EditorState } from '@codemirror/state';
  import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection, dropCursor, rectangularSelection, crosshairCursor } from '@codemirror/view';
  import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
  import { markdown, markdownLanguage } from '@codemirror/lang-markdown';
  import { syntaxHighlighting, indentOnInput, foldGutter, foldKeymap, bracketMatching } from '@codemirror/language';
  import { autocompletion, completionKeymap, closeBrackets, closeBracketsKeymap, completionStatus } from '@codemirror/autocomplete';
  import { searchKeymap, highlightSelectionMatches, openSearchPanel } from '@codemirror/search';

  import { theme, mdHighlight } from './theme';
  import { wikilinkDecoration, wikilinkClickHandler, wikilinkComplete } from './wikilinks';
  import { snippetComplete } from './snippets';
  import { tagComplete } from './tags';
  import { markdownShortcuts, smartPaste } from './markdown-shortcuts';
  import { imagePasteAndDrop } from './image-upload';
  import { autolinkComplete } from './autolink';
  import { extractToNoteKeymap, type ExtractRequest } from './extract-note';
  import { askAIKeymap, type AskAIRequest } from './ask-ai';
  import { checkboxShortcuts } from './checkbox-shortcuts';
  import { headingShortcuts } from './heading-shortcuts';
  import { continueWritingExtension } from './continue-writing';

  let {
    value = $bindable(''),
    onSave,
    onNavigate,
    onExtract,
    onAskAI,
    onCursor,
    onScroll,
    placeholder = ''
  }: {
    value?: string;
    onSave?: () => void;
    onNavigate?: (target: string) => void;
    /**
     * Mod-Shift-X handler. Fires only when the editor has a non-empty
     * selection. The host page should show a dialog asking for the
     * new note title, then call req.apply(title) on confirm or
     * req.cancel() on dismiss. Implementation in $lib/editor/extract-note.
     */
    onExtract?: (req: ExtractRequest) => void;
    /**
     * Mod-Shift-A handler. Fires only when the editor has a non-empty
     * selection. The host page shows the Ask-AI dialog, calls
     * /api/v1/chat with the selection, and on accept invokes one of
     * req.replace / req.insertAfter to splice the AI response into
     * the document. Implementation in $lib/editor/ask-ai.
     */
    onAskAI?: (req: AskAIRequest) => void;
    /**
     * Cursor position callback — fires on every selection change with
     * the 1-indexed line number, 1-indexed column, and the selected
     * length (0 when there's no selection). Used by the host page's
     * status bar to display "Ln 12, Col 4" / "12 selected". Receiving
     * a callback is preferred over polling because polling on every
     * frame is expensive on long documents.
     */
    onCursor?: (info: { line: number; col: number; selLen: number }) => void;
    /**
     * Scroll callback — fires (rAF-throttled) whenever the user
     * scrolls the editor's scrollable area. Host uses this to drive
     * a reading-progress bar without polling. Idle by default.
     */
    onScroll?: (info: { top: number; height: number; viewport: number }) => void;
    placeholder?: string;
  } = $props();

  let containerEl: HTMLDivElement | undefined = $state();
  let view: EditorView | undefined;
  let internalChange = false;
  // Watermark of the last `value` we synced into the CodeMirror state.
  // Used by both the updateListener (user typing) and the external
  // $effect below to short-circuit no-op syncs in O(1) without
  // materializing the doc string. See the long comment on the $effect
  // for the full story.
  let lastAppliedValue: string | null = null;

  function setupView() {
    if (!containerEl) return;
    const state = EditorState.create({
      doc: value,
      extensions: [
        lineNumbers(),
        foldGutter(),
        history(),
        drawSelection(),
        dropCursor(),
        EditorState.allowMultipleSelections.of(true),
        indentOnInput(),
        bracketMatching(),
        closeBrackets(),
        autocompletion({
          // Order: wikilinks ([[…]]) → snippets (/…) → tags (#…) →
          // autolink (phrases that match a known note title). Each
          // source is scoped to its own trigger / context so they
          // don't compete; autolink lands last because it runs on
          // every word boundary and we'd rather the trigger-character
          // sources resolve first when both could match.
          override: [wikilinkComplete, snippetComplete, tagComplete, autolinkComplete],
          activateOnTyping: true,
          closeOnBlur: true
        }),
        rectangularSelection(),
        crosshairCursor(),
        highlightActiveLine(),
        highlightActiveLineGutter(),
        highlightSelectionMatches(),
        EditorView.lineWrapping,
        markdown({ base: markdownLanguage, codeLanguages: [] }),
        syntaxHighlighting(mdHighlight),
        wikilinkDecoration,
        wikilinkClickHandler((target) => onNavigate?.(target)),
        // Extract-to-note (Mod-Shift-X). Registered above the default
        // keymap so the chord isn't shadowed by anything else. The
        // keybind only fires when there's a selection — empty
        // selections fall through so the user can re-bind the chord
        // for something else later if needed.
        extractToNoteKeymap((req) => onExtract?.(req)),
        // Ask-AI (Mod-Shift-A). Same shape as extract: selection
        // required, hands the request up to the host so the modal
        // UX + /chat call live in the page (where the toast +
        // settings nav live too).
        askAIKeymap((req) => onAskAI?.(req)),
        // Continue Writing (Mod-Alt-Space at empty selection).
        // Streams an AI continuation as ghost text after the cursor;
        // Tab accepts, Esc rejects. Lives BEFORE the main keymap so
        // its Tab handler gets first look at the chord — when no
        // ghost is active it returns false, falling through to the
        // default Tab indent.
        continueWritingExtension(),
        theme,
        // Markdown shortcuts come BEFORE defaultKeymap so Mod-b /
        // Mod-i / Mod-k aren't shadowed by CodeMirror's defaults. Same
        // story for the search keymap below — `searchKeymap` brings
        // Mod-f for the find panel, which composes nicely.
        keymap.of([
          ...markdownShortcuts,
          ...checkboxShortcuts,
          ...headingShortcuts,
          ...closeBracketsKeymap,
          ...defaultKeymap,
          ...historyKeymap,
          ...foldKeymap,
          ...completionKeymap,
          ...searchKeymap,
          indentWithTab,
          {
            key: 'Mod-s',
            preventDefault: true,
            run: () => {
              onSave?.();
              return true;
            }
          }
        ]),
        // Image paste/drop registered BEFORE smartPaste — image
        // clipboards never carry text/plain so the two never compete,
        // but ordering matters in case the OS clipboard contains both
        // (some screenshot tools include a text fallback).
        imagePasteAndDrop,
        // Smart paste: URL-while-selected → markdown link. Falls
        // through to default paste otherwise.
        smartPaste,
        EditorView.updateListener.of((u) => {
          if (u.docChanged) {
            internalChange = true;
            const next = u.state.doc.toString();
            value = next;
            // Keep the external-sync watermark in lockstep with the
            // user's own edits so the next parent reactivity ping
            // (with this same string echoed back as `value`) hits
            // the O(1) identity short-circuit and never materializes
            // the doc again.
            lastAppliedValue = next;
            queueMicrotask(() => (internalChange = false));
          }
          // Fire cursor info on selection or doc changes. Both
          // mutate the cursor position from the user's perspective
          // (typing moves the caret; clicking moves it; deletion
          // can shift it). selectionSet is the most precise signal
          // for "cursor moved" but doesn't catch typing-induced
          // drift, so we OR with docChanged.
          if ((u.selectionSet || u.docChanged) && onCursor) {
            const sel = u.state.selection.main;
            const line = u.state.doc.lineAt(sel.head);
            onCursor({
              line: line.number,
              col: sel.head - line.from + 1,
              selLen: Math.abs(sel.to - sel.from)
            });
          }
        })
      ]
    });
    view = new EditorView({ state, parent: containerEl });
    // rAF-throttled scroll fanout to the host. Native 'scroll'
    // events can fire 60+×/s on a fast wheel; collapsing to one
    // emit per animation frame keeps the host's progress bar
    // smooth without thrashing layout.
    if (onScroll) {
      let pending = false;
      const fire = () => {
        if (!view) return;
        const el = view.scrollDOM;
        onScroll!({ top: el.scrollTop, height: el.scrollHeight, viewport: el.clientHeight });
      };
      view.scrollDOM.addEventListener(
        'scroll',
        () => {
          if (pending) return;
          pending = true;
          requestAnimationFrame(() => {
            pending = false;
            fire();
          });
        },
        { passive: true }
      );
      // Fire once on mount so the initial 0% paints without
      // requiring the user to scroll.
      requestAnimationFrame(fire);
    }
  }

  onMount(setupView);

  // External value changes — replace doc, but preserve cursor +
  // selection across the dispatch so a server-side body sync (after
  // autosave returns a normalised body, or after a WS reload from
  // another device) doesn't yank the cursor back to position 0
  // mid-type. The user reported "saving bug with reloading" — the
  // pre-fix version replaced the doc cleanly but lost cursor
  // state, which felt like the editor "broke" until a reload
  // re-mounted everything.
  //
  // Three-layer guard:
  //   1. internalChange flag suppresses the dispatch when the
  //      change came from the user's own typing (the updateListener
  //      already updated `value` to match the doc).
  //   2. lastAppliedValue identity check — when a parent reactivity
  //      ping re-passes the SAME string we already applied (common
  //      after autosave bounces and other state updates), bail
  //      WITHOUT materializing the doc. Previously we called
  //      doc.toString() which is O(N) and allocates the full doc
  //      string on every ping; on a 1MB note that was a perceptible
  //      ~5–10ms hitch right after every save returned. We never
  //      need the actual doc text here — we only need to know
  //      whether `value` matches what we last set, which a closure
  //      reference comparison answers in O(1).
  //   3. Fallback toString() comparison only when the reference
  //      check missed (parent rebuilt an equal string). Cheap on
  //      small docs; only runs when the cheap path didn't already
  //      bail.
  // When the dispatch DOES fire (genuinely external value change),
  // we clamp the original selection to the new doc length so the
  // cursor lands at a sensible position instead of jumping to 0.
  $effect(() => {
    const v = value;
    if (!view || internalChange) return;
    if (lastAppliedValue !== null && lastAppliedValue === v) return;
    // Only fall back to the expensive toString() when the cheap
    // identity check missed. Catches parent re-renders that produce
    // a new string instance with the same content.
    if (view.state.doc.toString() === v) {
      lastAppliedValue = v;
      return;
    }
    const sel = view.state.selection.main;
    const len = v.length;
    const anchor = Math.min(sel.anchor, len);
    const head = Math.min(sel.head, len);
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: v },
      selection: { anchor, head }
    });
    lastAppliedValue = v;
  });

  onDestroy(() => view?.destroy());

  export function focus() { view?.focus(); }
  /**
   * Authoritative read of the current document content, materialised
   * from CodeMirror's own state — NOT the `value` prop. The parent's
   * bound `value` is updated through Svelte's microtask scheduler, so
   * a long reactive cascade (post-autosave note prop mutations
   * triggering panel rerenders) can leave the parent's mirror lagging
   * the editor's actual content by 10s of ms during heavy typing.
   *
   * Why this matters: load() and the WS reload guards used to compare
   * the parent's `body` to `prev` to decide "is it safe to overwrite
   * the editor". With a stale `body`, that check could falsely
   * conclude "no in-flight edits" and overwrite the editor with a
   * shorter server body — discarding everything the user typed after
   * the most recent autosave. Reading the doc directly is immune to
   * that race because CodeMirror's view state is updated
   * synchronously inside dispatch(), with no queue in front of it.
   */
  export function getContent(): string {
    return view ? view.state.doc.toString() : value;
  }
  export function scrollToLine(lineNum: number) {
    if (!view) return;
    const line = view.state.doc.line(Math.max(1, Math.min(lineNum, view.state.doc.lines)));
    view.dispatch({
      selection: { anchor: line.from },
      effects: EditorView.scrollIntoView(line.from, { y: 'start', yMargin: 32 })
    });
    view.focus();
  }

  // Sticky-scroll-position helpers. Parent reads getScrollTop() before
  // navigating away (saved to localStorage keyed by note path) and
  // calls setScrollTop() after the doc is loaded so the user lands
  // back where they were. We use the actual scroller element rather
  // than CodeMirror's per-line tracking because the scroll-to-pixel
  // form survives reflow (font-size change, line wrapping) much better
  // than scroll-to-line for prose-heavy notes.
  export function getScrollTop(): number {
    if (!view) return 0;
    return view.scrollDOM.scrollTop;
  }
  /** Reading-progress helper. Returns the scroll metrics in one
   *  call so the host can compute scrollTop / (scrollHeight -
   *  viewport) for a 0..1 progress fraction. Cheap; safe to call
   *  on every scroll event without measuring the layout twice. */
  export function getScrollMetrics(): { top: number; height: number; viewport: number } {
    if (!view) return { top: 0, height: 0, viewport: 0 };
    const el = view.scrollDOM;
    return { top: el.scrollTop, height: el.scrollHeight, viewport: el.clientHeight };
  }
  export function setScrollTop(top: number) {
    if (!view || top <= 0) return;
    // Defer one frame so the doc has a chance to lay out before we
    // try to scroll into a tall position. Without this, replacing the
    // doc and immediately scrolling can land at 0 because the scroller
    // hasn't computed its content height yet.
    requestAnimationFrame(() => {
      if (view) view.scrollDOM.scrollTop = top;
    });
  }

  // True when the autocomplete picker is currently open (the user is
  // mid-snippet or wikilink). Parent uses this to pause auto-save —
  // saving + the resulting WS roundtrip can reset the editor doc,
  // which closes the picker, which interrupts the user. Hostile UX.
  export function isCompletionActive(): boolean {
    if (!view) return false;
    return completionStatus(view.state) !== null;
  }

  // Dispatch a chord like "mod+b" / "mod+shift+x" into the editor.
  // Used by the floating SelectionToolbar so toolbar buttons take the
  // same code path as keyboard shortcuts — single source of truth
  // (the keymap), no parallel implementations to drift.
  export function dispatchChord(chord: string) {
    if (!view) return;
    const parts = chord.toLowerCase().split('+');
    const key = parts[parts.length - 1];
    const mod = parts.includes('mod');
    const shift = parts.includes('shift');
    const alt = parts.includes('alt');
    const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent);
    const ev = new KeyboardEvent('keydown', {
      key,
      ctrlKey: mod && !isMac,
      metaKey: mod && isMac,
      shiftKey: shift,
      altKey: alt,
      bubbles: true,
      cancelable: true
    });
    view.contentDOM.dispatchEvent(ev);
  }

  /** Insert a string at the current cursor position. If a selection
   *  is active, replaces it. Used by the link-suggester to slot
   *  [[wiki]] markup at the user's working point without yanking
   *  focus out of the doc. Mirrors a CM edit with the dispatch path
   *  the keymap uses, so undo / redo treat it like any other edit. */
  export function insertAtCursor(text: string) {
    if (!view) return;
    const { from, to } = view.state.selection.main;
    view.dispatch({
      changes: { from, to, insert: text },
      selection: { anchor: from + text.length },
      scrollIntoView: true
    });
    view.focus();
  }

  /** The editor view's host element — exposed so an overlay (like
   *  SelectionToolbar) can scope its selection-detection to the
   *  editor instead of the whole document. */
  export function getDOM(): HTMLElement | undefined {
    return view?.contentDOM;
  }

  /** Open CodeMirror's built-in find/replace panel. Same panel as
   *  Mod-F triggers via the keymap; exposed as an export so a
   *  toolbar button on the host page can invoke it for users who
   *  don't know the shortcut. */
  export function openFind() {
    if (!view) return;
    openSearchPanel(view);
  }
</script>

<div bind:this={containerEl} class="cm-host h-full overflow-hidden bg-surface0 border border-surface1 rounded"></div>

<style>
  .cm-host :global(.cm-editor) { height: 100%; }
  .cm-host :global(.cm-scroller) { font-size: 14px; line-height: 1.55; }
  @media (max-width: 767px) {
    .cm-host :global(.cm-scroller) { font-size: 16px; }
    .cm-host :global(.cm-gutters) { display: none; }
    /* Mobile keyboards take half the screen — the autocomplete
       popup placed below the cursor often lands UNDER the keyboard
       and is unreachable. Constrain max-height so CM6 picks the
       above-cursor placement when there's no room below, and clamp
       max-width so the popup never spills past the viewport when
       the cursor is near the right edge. */
    .cm-host :global(.cm-tooltip.cm-tooltip-autocomplete) {
      max-width: calc(100vw - 1rem);
      max-height: 40dvh;
    }
    /* Search panel — the find/replace bar — gets pushed off-screen
       in landscape on small phones because of its default fixed
       width. Make it span the editor width with a tap-friendly
       button row. */
    .cm-host :global(.cm-panel.cm-search) {
      flex-wrap: wrap;
    }
    .cm-host :global(.cm-panel.cm-search input) {
      min-height: 36px;
    }
    .cm-host :global(.cm-panel.cm-search button) {
      min-height: 36px;
      min-width: 36px;
    }
  }
</style>
