<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import { EditorView, basicSetup } from 'codemirror'
  import { EditorState } from '@codemirror/state'
  import { keymap } from '@codemirror/view'
  import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
  import { languages } from '@codemirror/language-data'
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
      fontSize: '14px',
      height: '100%',
    },
    '&.cm-focused': { outline: 'none' },
    '.cm-scroller': {
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      lineHeight: '1.75',
      overflow: 'auto',
    },
    '.cm-content': {
      padding: '24px 32px',
      caretColor: 'var(--ctp-text)',
      minHeight: '100%',
    },
    '.cm-gutters': {
      backgroundColor: 'color-mix(in srgb, var(--ctp-mantle) 60%, transparent)',
      color: 'var(--ctp-surface2)',
      border: 'none',
      borderRight: '1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent)',
      minWidth: '48px',
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'transparent',
      color: 'var(--ctp-peach)',
      fontWeight: '600',
    },
    '.cm-activeLine': {
      backgroundColor: 'color-mix(in srgb, var(--ctp-surface0) 20%, transparent)',
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
    { tag: tags.heading1, color: 'var(--ctp-mauve)', fontWeight: 'bold', fontSize: '1.4em' },
    { tag: tags.heading2, color: 'var(--ctp-blue)', fontWeight: 'bold', fontSize: '1.2em' },
    { tag: tags.heading3, color: 'var(--ctp-sapphire)', fontWeight: 'bold', fontSize: '1.1em' },
    { tag: tags.heading4, color: 'var(--ctp-teal)', fontWeight: 'bold' },
    { tag: tags.heading5, color: 'var(--ctp-green)', fontWeight: 'bold' },
    { tag: tags.heading6, color: 'var(--ctp-yellow)', fontWeight: 'bold' },
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
        markdown({ base: markdownLanguage, codeLanguages: languages }),
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

  const toolGroups = [
    [
      { label: 'B', action: () => wrapSelection('**', '**'), title: 'Bold (Ctrl+B)', cls: 'font-bold' },
      { label: 'I', action: () => wrapSelection('*', '*'), title: 'Italic (Ctrl+I)', cls: 'italic' },
      { label: 'S', action: () => wrapSelection('~~', '~~'), title: 'Strikethrough', cls: '' },
      { label: '</>', action: () => wrapSelection('`', '`'), title: 'Code', cls: '' },
    ],
    [
      { label: 'H1', action: () => wrapSelection('# ', '\n'), title: 'Heading 1', cls: '' },
      { label: 'H2', action: () => wrapSelection('## ', '\n'), title: 'Heading 2', cls: '' },
      { label: 'H3', action: () => wrapSelection('### ', '\n'), title: 'Heading 3', cls: '' },
    ],
    [
      { label: '[[', action: () => wrapSelection('[[', ']]'), title: 'Wikilink', cls: '' },
      { label: '- [ ]', action: () => insertAtCursor('- [ ] '), title: 'Task', cls: '' },
      { label: '>', action: () => insertAtCursor('> '), title: 'Quote', cls: '' },
    ],
  ]
</script>

<div class="flex flex-col h-full bg-ctp-base">
  <!-- Toolbar -->
  <div class="flex items-center gap-0.5 px-3 py-1.5 border-b border-ctp-surface0/50 bg-ctp-mantle/50">
    {#each toolGroups as group, gi}
      {#if gi > 0}
        <div class="w-px h-4 bg-ctp-surface0 mx-1"></div>
      {/if}
      {#each group as tool}
        <button on:click={tool.action} title={tool.title}
          class="px-2 py-1 text-[11px] text-ctp-overlay0 hover:text-ctp-text hover:bg-ctp-surface0
                 rounded-md transition-all duration-75 {tool.cls}">
          {tool.label}
        </button>
      {/each}
    {/each}

    <div class="flex-1"></div>

    <div class="flex items-center gap-1.5 text-[10px]">
      {#if dirty}
        <span class="flex items-center gap-1 text-ctp-peach">
          <span class="w-1.5 h-1.5 rounded-full bg-ctp-peach animate-pulse"></span>
          Modified
        </span>
      {:else}
        <span class="text-ctp-green flex items-center gap-1">
          <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 8l3 3 7-7" /></svg>
          Saved
        </span>
      {/if}
    </div>
  </div>

  <!-- CodeMirror editor -->
  <div bind:this={editorEl} class="flex-1 overflow-hidden cm-editor-wrapper"></div>
</div>

<style>
  .cm-editor-wrapper :global(.cm-editor) {
    height: 100%;
  }
</style>
