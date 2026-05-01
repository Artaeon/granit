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
          override: [wikilinkComplete],
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
        keymap.of([
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
