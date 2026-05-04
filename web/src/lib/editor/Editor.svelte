<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EditorState } from '@codemirror/state';
  import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection, dropCursor, rectangularSelection, crosshairCursor } from '@codemirror/view';
  import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
  import { markdown, markdownLanguage } from '@codemirror/lang-markdown';
  import { syntaxHighlighting, indentOnInput, foldGutter, foldKeymap, bracketMatching } from '@codemirror/language';
  import { autocompletion, completionKeymap, closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete';
  import { searchKeymap, highlightSelectionMatches } from '@codemirror/search';

  import { theme, mdHighlight } from './theme';
  import { wikilinkDecoration, wikilinkClickHandler, wikilinkComplete } from './wikilinks';
  import { snippetComplete } from './snippets';
  import { tagComplete } from './tags';
  import { markdownShortcuts, smartPaste } from './markdown-shortcuts';

  let {
    value = $bindable(''),
    onSave,
    onNavigate,
    placeholder = ''
  }: {
    value?: string;
    onSave?: () => void;
    onNavigate?: (target: string) => void;
    placeholder?: string;
  } = $props();

  let containerEl: HTMLDivElement | undefined = $state();
  let view: EditorView | undefined;
  let internalChange = false;

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
          // Order: wikilinks ([[…]]) → snippets (/…) → tags (#…). All
          // three are scoped by their trigger character so they don't
          // step on each other; CodeMirror runs each source and merges.
          override: [wikilinkComplete, snippetComplete, tagComplete],
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
        theme,
        // Markdown shortcuts come BEFORE defaultKeymap so Mod-b /
        // Mod-i / Mod-k aren't shadowed by CodeMirror's defaults. Same
        // story for the search keymap below — `searchKeymap` brings
        // Mod-f for the find panel, which composes nicely.
        keymap.of([
          ...markdownShortcuts,
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
        // Smart paste: URL-while-selected → markdown link. Falls
        // through to default paste otherwise.
        smartPaste,
        EditorView.updateListener.of((u) => {
          if (u.docChanged) {
            internalChange = true;
            value = u.state.doc.toString();
            queueMicrotask(() => (internalChange = false));
          }
        })
      ]
    });
    view = new EditorView({ state, parent: containerEl });
  }

  onMount(setupView);

  // External value changes — replace doc.
  $effect(() => {
    const v = value;
    if (view && !internalChange && view.state.doc.toString() !== v) {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: v } });
    }
  });

  onDestroy(() => view?.destroy());

  export function focus() { view?.focus(); }
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
</script>

<div bind:this={containerEl} class="cm-host h-full overflow-hidden bg-surface0 border border-surface1 rounded"></div>

<style>
  .cm-host :global(.cm-editor) { height: 100%; }
  .cm-host :global(.cm-scroller) { font-size: 14px; line-height: 1.55; }
  @media (max-width: 767px) {
    .cm-host :global(.cm-scroller) { font-size: 16px; }
    .cm-host :global(.cm-gutters) { display: none; }
  }
</style>
