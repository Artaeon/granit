<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import { EditorView, basicSetup } from 'codemirror'
  import { EditorState } from '@codemirror/state'
  import { keymap } from '@codemirror/view'
  import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
  import { syntaxHighlighting, HighlightStyle } from '@codemirror/language'
  import { tags } from '@lezer/highlight'

  export let content = ''
  export let dirty = false

  const dispatch = createEventDispatcher()

  let editorEl: HTMLDivElement
  let view: EditorView
  let saveTimeout: ReturnType<typeof setTimeout>
  let internalUpdate = false

  const granitTheme = EditorView.theme({
    '&': {
      backgroundColor: 'var(--ctp-base)',
      color: 'var(--ctp-text)',
      fontSize: '16px',
      height: '100%',
    },
    '&.cm-focused': { outline: 'none' },
    '.cm-scroller': {
      fontFamily: "'Lyon-Text', Georgia, 'YuMincho', 'Yu Mincho', 'Hiragino Mincho ProN', 'Hiragino Mincho Pro', serif",
      lineHeight: '1.75',
      overflow: 'auto',
    },
    '.cm-content': {
      padding: '48px 96px',
      caretColor: 'var(--ctp-text)',
      minHeight: '100%',
      maxWidth: '900px',
      margin: '0 auto',
    },
    '.cm-gutters': {
      display: 'none',
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'transparent',
      color: 'var(--ctp-peach)',
      fontWeight: '600',
    },
    '.cm-activeLine': {
      backgroundColor: 'transparent',
    },
    '.cm-cursor, .cm-dropCursor': {
      borderLeftColor: 'var(--ctp-text)',
      borderLeftWidth: '2px',
    },
    '.cm-selectionBackground': {
      backgroundColor: 'color-mix(in srgb, var(--ctp-blue) 20%, transparent) !important',
    },
    '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground': {
      backgroundColor: 'color-mix(in srgb, var(--ctp-blue) 30%, transparent) !important',
    },
    '.cm-matchingBracket': {
      backgroundColor: 'color-mix(in srgb, var(--ctp-yellow) 25%, transparent)',
      outline: '1px solid color-mix(in srgb, var(--ctp-yellow) 50%, transparent)',
    },
    '.cm-searchMatch': {
      backgroundColor: 'color-mix(in srgb, var(--ctp-yellow) 30%, transparent)',
      borderRadius: '2px',
    },
    '.cm-searchMatch.cm-searchMatch-selected': {
      backgroundColor: 'color-mix(in srgb, var(--ctp-peach) 40%, transparent)',
    },
    '.cm-foldPlaceholder': {
      backgroundColor: 'var(--ctp-surface0)',
      color: 'var(--ctp-overlay0)',
      border: '1px solid var(--ctp-surface1)',
      borderRadius: '4px',
      padding: '0 6px',
      margin: '0 4px',
    },
    '.cm-tooltip': {
      backgroundColor: 'var(--ctp-surface0)',
      color: 'var(--ctp-text)',
      border: '1px solid var(--ctp-surface1)',
      borderRadius: '8px',
      boxShadow: '0 8px 24px rgba(0,0,0,0.3)',
    },
    '.cm-panels': { backgroundColor: 'var(--ctp-mantle)', color: 'var(--ctp-text)' },
    '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--ctp-surface0)' },
    '.cm-panel.cm-search': { backgroundColor: 'var(--ctp-mantle)', padding: '8px 12px' },
    '.cm-panel.cm-search input': {
      backgroundColor: 'var(--ctp-surface0)', color: 'var(--ctp-text)',
      border: '1px solid var(--ctp-surface1)', borderRadius: '4px', padding: '2px 8px',
    },
    '.cm-panel.cm-search button': {
      backgroundColor: 'var(--ctp-surface0)', color: 'var(--ctp-text)',
      border: '1px solid var(--ctp-surface1)', borderRadius: '4px', padding: '2px 8px',
      cursor: 'pointer',
    },
    '.cm-panel.cm-search button:hover': { backgroundColor: 'var(--ctp-surface1)' },
    '.cm-panel.cm-search label': { color: 'var(--ctp-subtext0)' },
    '.cm-lineNumbers .cm-gutterElement': { padding: '0 8px 0 12px', minWidth: '32px' },
  })

  const granitHighlight = HighlightStyle.define([
    { tag: tags.heading1, color: 'var(--ctp-text)', fontWeight: 'bold', fontSize: '2em' },
    { tag: tags.heading2, color: 'var(--ctp-text)', fontWeight: 'bold', fontSize: '1.5em' },
    { tag: tags.heading3, color: 'var(--ctp-text)', fontWeight: 'bold', fontSize: '1.25em' },
    { tag: tags.heading4, color: 'var(--ctp-text)', fontWeight: 'bold' },
    { tag: tags.heading5, color: 'var(--ctp-text)', fontWeight: 'bold' },
    { tag: tags.heading6, color: 'var(--ctp-text)', fontWeight: 'bold' },
    { tag: tags.emphasis, fontStyle: 'italic', color: 'var(--ctp-subtext1)' },
    { tag: tags.strong, fontWeight: 'bold' },
    { tag: tags.strikethrough, textDecoration: 'line-through', color: 'var(--ctp-overlay0)' },
    { tag: tags.link, color: 'var(--ctp-blue)' },
    { tag: tags.url, color: 'var(--ctp-sapphire)', textDecoration: 'underline' },
    { tag: tags.quote, color: 'var(--ctp-overlay1)', fontStyle: 'italic' },
    { tag: tags.monospace, color: 'var(--ctp-green)' },
    { tag: tags.meta, color: 'var(--ctp-overlay0)' },
    { tag: tags.processingInstruction, color: 'var(--ctp-overlay0)' },
    { tag: tags.contentSeparator, color: 'var(--ctp-surface2)' },
    { tag: tags.atom, color: 'var(--ctp-green)' },
    { tag: tags.bool, color: 'var(--ctp-peach)' },
    { tag: tags.number, color: 'var(--ctp-peach)' },
    { tag: tags.string, color: 'var(--ctp-green)' },
    { tag: tags.keyword, color: 'var(--ctp-mauve)' },
    { tag: tags.comment, color: 'var(--ctp-overlay0)', fontStyle: 'italic' },
    { tag: tags.variableName, color: 'var(--ctp-text)' },
    { tag: tags.function(tags.variableName), color: 'var(--ctp-blue)' },
    { tag: tags.definition(tags.variableName), color: 'var(--ctp-blue)' },
    { tag: tags.typeName, color: 'var(--ctp-yellow)' },
    { tag: tags.className, color: 'var(--ctp-yellow)' },
    { tag: tags.propertyName, color: 'var(--ctp-blue)' },
    { tag: tags.operator, color: 'var(--ctp-sky)' },
    { tag: tags.punctuation, color: 'var(--ctp-overlay1)' },
  ])

  onMount(() => {
    const state = EditorState.create({
      doc: content,
      extensions: [
        basicSetup,
        markdown({ base: markdownLanguage }),
        syntaxHighlighting(granitHighlight),
        granitTheme,
        keymap.of([
          { key: 'Mod-s', run: () => { clearTimeout(saveTimeout); dispatch('save'); return true } },
          { key: 'Tab', run: (v) => { v.dispatch(v.state.replaceSelection('  ')); return true } },
        ]),
        EditorView.updateListener.of(update => {
          if (update.docChanged && !internalUpdate) {
            const newContent = update.state.doc.toString()
            dispatch('change', newContent)
            clearTimeout(saveTimeout)
            saveTimeout = setTimeout(() => dispatch('save'), 2000)
          }
          if (update.selectionSet || update.docChanged) {
            const pos = update.state.selection.main.head
            const line = update.state.doc.lineAt(pos)
            dispatch('cursor', { line: line.number, col: pos - line.from + 1 })
          }
        }),
        EditorView.lineWrapping,
      ],
    })
    view = new EditorView({ state, parent: editorEl })
  })

  // React to content changes from parent (tab switches)
  $: if (view) {
    const currentDoc = view.state.doc.toString()
    if (content !== currentDoc) {
      internalUpdate = true
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: content }
      })
      internalUpdate = false
    }
  }

  onDestroy(() => {
    clearTimeout(saveTimeout)
    if (view) view.destroy()
  })

  function wrapSelection(before: string, after: string) {
    if (!view) return
    const { from, to } = view.state.selection.main
    const selected = view.state.sliceDoc(from, to)
    view.dispatch({
      changes: { from, to, insert: before + selected + after },
      selection: { anchor: from + before.length, head: from + before.length + selected.length }
    })
    view.focus()
  }

  function insertAtCursor(text: string) {
    if (!view) return
    const pos = view.state.selection.main.head
    view.dispatch({
      changes: { from: pos, insert: text },
      selection: { anchor: pos + text.length }
    })
    view.focus()
  }

  // Exported scroll/cursor position helpers for tab restore
  export function getScrollPos(): number {
    if (!view) return 0
    return view.scrollDOM.scrollTop
  }

  export function setScrollPos(pos: number) {
    if (!view) return
    // Defer to next tick so the editor has rendered the new content
    requestAnimationFrame(() => {
      if (view) view.scrollDOM.scrollTop = pos
    })
  }

  export function getCursorPos(): number {
    if (!view) return 0
    return view.state.selection.main.head
  }

  export function setCursorPos(pos: number) {
    if (!view) return
    const docLen = view.state.doc.length
    const safePos = Math.min(pos, docLen)
    view.dispatch({ selection: { anchor: safePos } })
  }

  // SVG icon paths (16x16 viewBox)
  const icons = {
    bold: 'M4 2h5a3 3 0 0 1 0 6H4zm0 6h6a3 3 0 0 1 0 6H4z',
    italic: 'M6 2h6M4 14h6M10 2L6 14',
    strike: 'M2 8h12M4 3c0 0 0 3 4 3s4-3 4-3M4 13c0 0 0-3 4-3s4 3 4 3',
    code: 'M5 4L1 8l4 4M11 4l4 4-4 4',
    h1: 'M2 2v12M10 2v12M2 8h8M13 4v8l2-2',
    h2: 'M1 2v12M8 2v12M1 8h7M11 4h3a2 2 0 0 1 0 4h-2l3 4',
    h3: 'M1 2v12M8 2v12M1 8h7M12 4h2a1.5 1.5 0 0 1 0 3h-1a1.5 1.5 0 0 1 0 3h-2',
    link: 'M7 3H4a3 3 0 0 0 0 6h3M9 3h3a3 3 0 0 1 0 6H9M5 8h6',
    task: 'M3 1h10v14H3zM6 5l1.5 1.5L10 4M6 9h4',
    quote: 'M3 4h10M3 8h6M3 12h8',
    list: 'M4 4h10M4 8h10M4 12h10M1 4h1M1 8h1M1 12h1',
    table: 'M1 2h14v12H1zM1 6h14M1 10h14M6 2v12M11 2v12',
    hr: 'M2 8h12',
  }

  const toolGroups = [
    [
      { icon: icons.bold, action: () => wrapSelection('**', '**'), title: 'Bold (Ctrl+B)' },
      { icon: icons.italic, action: () => wrapSelection('*', '*'), title: 'Italic (Ctrl+I)' },
      { icon: icons.strike, action: () => wrapSelection('~~', '~~'), title: 'Strikethrough' },
      { icon: icons.code, action: () => wrapSelection('`', '`'), title: 'Inline Code' },
    ],
    [
      { icon: icons.h1, action: () => insertAtCursor('# '), title: 'Heading 1' },
      { icon: icons.h2, action: () => insertAtCursor('## '), title: 'Heading 2' },
      { icon: icons.h3, action: () => insertAtCursor('### '), title: 'Heading 3' },
    ],
    [
      { icon: icons.link, action: () => wrapSelection('[[', ']]'), title: 'Wikilink' },
      { icon: icons.task, action: () => insertAtCursor('- [ ] '), title: 'Task' },
      { icon: icons.quote, action: () => insertAtCursor('> '), title: 'Blockquote' },
      { icon: icons.list, action: () => insertAtCursor('- '), title: 'Bullet List' },
      { icon: icons.table, action: () => insertAtCursor('| Col 1 | Col 2 |\n|--------|--------|\n|        |        |\n'), title: 'Table' },
      { icon: icons.hr, action: () => insertAtCursor('\n---\n'), title: 'Horizontal Rule' },
    ],
  ]
</script>

<div class="flex flex-col h-full bg-ctp-base">
  <!-- Toolbar -->
  <div class="flex items-center gap-0.5 px-2 py-0.5 border-b border-ctp-surface0/20 bg-ctp-base">
    {#each toolGroups as group, gi}
      {#if gi > 0}
        <span class="text-ctp-surface1 text-[10px] mx-1">&middot;</span>
      {/if}
      {#each group as tool}
        <button on:click={tool.action} title={tool.title}
          class="w-7 h-7 flex items-center justify-center text-ctp-overlay1 hover:text-ctp-text hover:bg-ctp-surface0/70
                 rounded-md transition-all duration-75" data-tooltip={tool.title}>
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">
            <path d="{tool.icon}" />
          </svg>
        </button>
      {/each}
    {/each}
  </div>

  <!-- CodeMirror editor -->
  <div bind:this={editorEl} class="flex-1 overflow-hidden cm-editor-wrapper"></div>
</div>

<style>
  .cm-editor-wrapper :global(.cm-editor) {
    height: 100%;
  }
</style>
