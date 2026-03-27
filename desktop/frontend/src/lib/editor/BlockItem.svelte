<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher, tick } from 'svelte'
  import { EditorView } from 'codemirror'
  import { EditorState, RangeSetBuilder } from '@codemirror/state'
  import { keymap, placeholder as cmPlaceholder, Decoration, ViewPlugin } from '@codemirror/view'
  import type { DecorationSet, ViewUpdate } from '@codemirror/view'
  import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
  import { syntaxHighlighting, HighlightStyle } from '@codemirror/language'
  import { tags } from '@lezer/highlight'
  import type { Block } from '../types'

  // Decorations for wikilinks, tags, checkboxes
  const wikilinkDeco = Decoration.mark({ class: 'cm-wikilink' })
  const hashtagDeco = Decoration.mark({ class: 'cm-hashtag' })
  const todoDeco = Decoration.mark({ class: 'cm-todo' })
  const todoDoneDeco = Decoration.mark({ class: 'cm-todo-done' })

  function buildDecos(view: EditorView): DecorationSet {
    const builder = new RangeSetBuilder<Decoration>()
    const doc = view.state.doc.toString()

    // Wikilinks [[...]]
    const wikiRe = /\[\[([^\]]+)\]\]/g
    let m: RegExpExecArray | null
    while ((m = wikiRe.exec(doc)) !== null) {
      builder.add(m.index, m.index + m[0].length, wikilinkDeco)
    }

    // Hashtags #tag
    const tagRe = /#[a-zA-Z][\w\-\/]*/g
    while ((m = tagRe.exec(doc)) !== null) {
      builder.add(m.index, m.index + m[0].length, hashtagDeco)
    }

    // Checkboxes - [ ] and - [x]
    const todoRe = /- \[ \]/g
    while ((m = todoRe.exec(doc)) !== null) {
      builder.add(m.index, m.index + m[0].length, todoDeco)
    }
    const doneRe = /- \[x\]/gi
    while ((m = doneRe.exec(doc)) !== null) {
      builder.add(m.index, m.index + m[0].length, todoDoneDeco)
    }

    return builder.finish()
  }

  const decoPlugin = ViewPlugin.fromClass(
    class {
      decorations: DecorationSet
      constructor(view: EditorView) { this.decorations = buildDecos(view) }
      update(update: ViewUpdate) { if (update.docChanged) this.decorations = buildDecos(update.view) }
    },
    { decorations: v => v.decorations }
  )

  export let block: Block
  export let depth: number = 0
  export let focused: boolean = false

  const dispatch = createEventDispatcher()

  let editorEl: HTMLDivElement
  let view: EditorView
  let internalUpdate = false

  const blockTheme = EditorView.theme({
    '&': {
      backgroundColor: 'transparent',
      color: 'var(--ctp-text)',
      fontSize: '15px',
    },
    '&.cm-focused': { outline: 'none' },
    '.cm-scroller': {
      fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, sans-serif",
      lineHeight: '1.65',
      overflow: 'visible',
    },
    '.cm-content': {
      padding: '0',
      caretColor: 'var(--ctp-blue)',
      minHeight: 'auto',
    },
    '.cm-line': { padding: '1px 0' },
    '.cm-cursor': {
      borderLeftColor: 'var(--ctp-blue)',
      borderLeftWidth: '1.5px',
    },
    '.cm-selectionBackground': {
      backgroundColor: 'color-mix(in srgb, var(--ctp-blue) 15%, transparent) !important',
    },
    '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground': {
      backgroundColor: 'color-mix(in srgb, var(--ctp-blue) 25%, transparent) !important',
    },
    '.cm-placeholder': {
      color: 'var(--ctp-surface2)',
      fontStyle: 'italic',
    },
    '.cm-activeLine': {
      backgroundColor: 'transparent',
    },
    // Wikilink styling via marks
    '.cm-link': {
      color: 'var(--ctp-blue)',
      textDecoration: 'none',
    },
  })

  const blockHighlight = HighlightStyle.define([
    { tag: tags.heading1, color: 'var(--ctp-mauve)', fontWeight: 'bold', fontSize: '1.3em' },
    { tag: tags.heading2, color: 'var(--ctp-blue)', fontWeight: 'bold', fontSize: '1.15em' },
    { tag: tags.heading3, color: 'var(--ctp-sapphire)', fontWeight: 'bold' },
    { tag: tags.emphasis, fontStyle: 'italic', color: 'var(--ctp-subtext1)' },
    { tag: tags.strong, fontWeight: 'bold' },
    { tag: tags.strikethrough, textDecoration: 'line-through', color: 'var(--ctp-overlay0)' },
    { tag: tags.link, color: 'var(--ctp-blue)' },
    { tag: tags.url, color: 'var(--ctp-sapphire)', textDecoration: 'underline' },
    { tag: tags.quote, color: 'var(--ctp-overlay1)', fontStyle: 'italic' },
    { tag: tags.monospace, color: 'var(--ctp-green)', fontFamily: 'ui-monospace, monospace', fontSize: '0.9em', backgroundColor: 'color-mix(in srgb, var(--ctp-surface0) 50%, transparent)', padding: '1px 4px', borderRadius: '3px' },
    { tag: tags.meta, color: 'var(--ctp-overlay0)' },
    { tag: tags.processingInstruction, color: 'var(--ctp-overlay0)' },
    { tag: tags.bool, color: 'var(--ctp-peach)' },
    { tag: tags.number, color: 'var(--ctp-peach)' },
    { tag: tags.string, color: 'var(--ctp-green)' },
    { tag: tags.keyword, color: 'var(--ctp-mauve)' },
    { tag: tags.comment, color: 'var(--ctp-overlay0)', fontStyle: 'italic' },
  ])

  onMount(() => {
    const state = EditorState.create({
      doc: block.content,
      extensions: [
        keymap.of([
          {
            key: 'Enter',
            run: () => {
              const pos = view.state.selection.main.head
              const doc = view.state.doc.toString()
              const before = doc.slice(0, pos)
              const after = doc.slice(pos)
              // Update current block content to text before cursor
              dispatch('split', { before, after })
              return true
            }
          },
          {
            key: 'Backspace',
            run: () => {
              const pos = view.state.selection.main.head
              if (pos === 0 && view.state.selection.main.empty) {
                dispatch('merge-up')
                return true
              }
              return false
            }
          },
          {
            key: 'Tab',
            run: () => { dispatch('indent'); return true }
          },
          {
            key: 'Shift-Tab',
            run: () => { dispatch('outdent'); return true }
          },
          {
            key: 'ArrowUp',
            run: () => {
              const pos = view.state.selection.main.head
              const line = view.state.doc.lineAt(pos)
              if (line.number === 1) {
                dispatch('focus-prev')
                return true
              }
              return false
            }
          },
          {
            key: 'ArrowDown',
            run: () => {
              const pos = view.state.selection.main.head
              const line = view.state.doc.lineAt(pos)
              if (line.number === view.state.doc.lines) {
                dispatch('focus-next')
                return true
              }
              return false
            }
          },
          {
            key: 'Mod-s',
            run: () => { dispatch('save'); return true }
          },
        ]),
        markdown({ base: markdownLanguage }),
        syntaxHighlighting(blockHighlight),
        blockTheme,
        EditorView.updateListener.of(update => {
          if (update.docChanged && !internalUpdate) {
            dispatch('change', update.state.doc.toString())
          }
          if (update.focusChanged && update.view.hasFocus) {
            dispatch('focused')
          }
        }),
        EditorView.lineWrapping,
        decoPlugin,
        EditorView.domEventHandlers({
          click(event: MouseEvent) {
            const target = event.target as HTMLElement
            const wikilink = target.closest('.cm-wikilink')
            if (wikilink) {
              const text = wikilink.textContent || ''
              dispatch('wikilink', text)
              event.preventDefault()
              return true
            }
            return false
          }
        }),
        cmPlaceholder('Type something...'),
      ],
    })
    view = new EditorView({ state, parent: editorEl })
    if (focused) view.focus()
  })

  $: if (view && focused && !view.hasFocus) {
    view.focus()
  }

  $: if (view) {
    const currentDoc = view.state.doc.toString()
    if (block.content !== currentDoc) {
      internalUpdate = true
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: block.content }
      })
      internalUpdate = false
    }
  }

  export function focusAtEnd() {
    if (view) {
      view.focus()
      view.dispatch({ selection: { anchor: view.state.doc.length } })
    }
  }

  export function focusAtStart() {
    if (view) {
      view.focus()
      view.dispatch({ selection: { anchor: 0 } })
    }
  }

  export function focusAtPos(pos: number) {
    if (view) {
      view.focus()
      const p = Math.min(pos, view.state.doc.length)
      view.dispatch({ selection: { anchor: p } })
    }
  }

  onDestroy(() => { if (view) view.destroy() })
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="block-item flex items-start" style="padding-left: {depth * 22}px">
  <!-- Bullet -->
  <button class="bullet-btn flex-shrink-0 mt-[10px] mr-1.5"
    class:has-children={block.children.length > 0}
    class:collapsed={block.collapsed}
    on:click={() => {
      if (block.children.length > 0) dispatch('toggle-collapse')
    }}>
    {#if block.collapsed}
      <svg width="10" height="10" viewBox="0 0 16 16" fill="var(--ctp-blue)">
        <path d="M6 3l5 5-5 5z" />
      </svg>
    {:else if block.children.length > 0}
      <span class="bullet-dot parent"></span>
    {:else}
      <span class="bullet-dot"></span>
    {/if}
  </button>

  <!-- Editor -->
  <div bind:this={editorEl} class="flex-1 min-w-0 block-editor"></div>
</div>

<style>
  .block-item {
    border-radius: 3px;
    transition: background 80ms;
  }
  .block-item:hover {
    background: color-mix(in srgb, var(--ctp-surface0) 15%, transparent);
  }
  .bullet-btn {
    width: 16px;
    height: 16px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    cursor: default;
    transition: all 100ms;
    flex-shrink: 0;
    opacity: 0.7;
  }
  .block-item:hover .bullet-btn {
    opacity: 1;
  }
  .bullet-btn.has-children {
    cursor: pointer;
    opacity: 1;
  }
  .bullet-btn.has-children:hover {
    background: color-mix(in srgb, var(--ctp-blue) 15%, transparent);
  }
  .bullet-btn.collapsed {
    opacity: 1;
  }
  .bullet-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--ctp-overlay0);
    transition: all 100ms;
  }
  .bullet-dot.parent {
    background: var(--ctp-blue);
    width: 6px;
    height: 6px;
  }
  .block-editor :global(.cm-editor) {
    min-height: 22px;
  }
  .block-editor :global(.cm-placeholder) {
    color: var(--ctp-surface2) !important;
  }
  /* Wikilink styling */
  .block-editor :global(.cm-wikilink) {
    color: var(--ctp-blue);
    cursor: pointer;
    border-bottom: 1px solid color-mix(in srgb, var(--ctp-blue) 30%, transparent);
    transition: border-color 100ms;
  }
  .block-editor :global(.cm-wikilink:hover) {
    border-bottom-color: var(--ctp-blue);
  }
  /* Hashtag styling */
  .block-editor :global(.cm-hashtag) {
    color: var(--ctp-teal);
    font-size: 0.93em;
  }
  /* Todo checkbox styling */
  .block-editor :global(.cm-todo) {
    color: var(--ctp-overlay0);
    font-weight: 500;
  }
  .block-editor :global(.cm-todo-done) {
    color: var(--ctp-green);
    text-decoration: line-through;
    opacity: 0.7;
  }
</style>
