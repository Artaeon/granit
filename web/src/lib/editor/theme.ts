import { EditorView } from '@codemirror/view';
import { HighlightStyle } from '@codemirror/language';
import { tags } from '@lezer/highlight';

// Editor chrome — the panel, gutters, cursor, selection.
export const theme = EditorView.theme(
  {
    '&': {
      backgroundColor: 'transparent',
      color: 'var(--color-text)',
      height: '100%'
    },
    '&.cm-focused': { outline: 'none' },
    '.cm-content': {
      fontFamily: 'var(--font-mono)',
      caretColor: 'var(--color-primary)',
      padding: '12px 0'
    },
    '.cm-line': { padding: '0 12px' },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--color-primary)' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
      backgroundColor: 'var(--color-surface1)'
    },
    '.cm-activeLine': { backgroundColor: 'rgba(255,255,255,0.025)' },
    '.cm-activeLineGutter': { backgroundColor: 'transparent', color: 'var(--color-subtext)' },
    '.cm-gutters': {
      backgroundColor: 'transparent',
      color: 'var(--color-dim)',
      border: 'none'
    },
    '.cm-foldPlaceholder': {
      backgroundColor: 'var(--color-surface0)',
      color: 'var(--color-dim)',
      border: '1px solid var(--color-surface1)'
    },
    '.cm-scroller': { overflow: 'auto', fontFamily: 'var(--font-mono)' },
    // Search/match highlight
    '.cm-searchMatch': { backgroundColor: 'rgba(187,154,247,0.25)' },
    '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: 'rgba(187,154,247,0.5)' },
    // Autocomplete popup
    '.cm-tooltip.cm-tooltip-autocomplete': {
      backgroundColor: 'var(--color-mantle)',
      border: '1px solid var(--color-surface1)',
      borderRadius: '6px',
      boxShadow: '0 8px 24px rgba(0,0,0,0.4)'
    },
    '.cm-tooltip-autocomplete > ul': { fontFamily: 'var(--font-sans)', fontSize: '13px' },
    '.cm-tooltip-autocomplete > ul > li': { padding: '6px 10px' },
    '.cm-tooltip-autocomplete > ul > li[aria-selected]': {
      backgroundColor: 'var(--color-surface1)',
      color: 'var(--color-primary)'
    },
    '.cm-completionDetail': { color: 'var(--color-dim)', fontStyle: 'normal' },
    '.cm-completionLabel': { color: 'var(--color-text)' },
    '.cm-completionMatchedText': { color: 'var(--color-primary)', textDecoration: 'none' },
    // Wikilink decoration
    '.cm-wikilink': { color: 'var(--color-secondary)' },
    '.cm-wikilink:hover': { textDecoration: 'underline', cursor: 'pointer' }
  },
  { dark: true }
);

// Markdown syntax highlight — maps Lezer tags to colors from our palette.
export const mdHighlight = HighlightStyle.define([
  { tag: tags.heading1, color: 'var(--color-primary)', fontWeight: '700', fontSize: '1.4em' },
  { tag: tags.heading2, color: 'var(--color-secondary)', fontWeight: '700', fontSize: '1.2em' },
  { tag: tags.heading3, color: 'var(--color-info)', fontWeight: '600' },
  { tag: tags.heading4, color: 'var(--color-info)', fontWeight: '600' },
  { tag: tags.heading5, color: 'var(--color-info)' },
  { tag: tags.heading6, color: 'var(--color-info)' },
  { tag: tags.strong, fontWeight: '700', color: 'var(--color-text)' },
  { tag: tags.emphasis, fontStyle: 'italic' },
  { tag: tags.strikethrough, textDecoration: 'line-through', color: 'var(--color-dim)' },
  { tag: tags.link, color: 'var(--color-secondary)' },
  { tag: tags.url, color: 'var(--color-secondary)', textDecoration: 'underline' },
  { tag: tags.monospace, color: 'var(--color-accent)', backgroundColor: 'var(--color-surface0)' },
  { tag: tags.quote, color: 'var(--color-dim)', fontStyle: 'italic' },
  { tag: tags.list, color: 'var(--color-text)' },
  { tag: tags.meta, color: 'var(--color-dim)' },
  { tag: tags.processingInstruction, color: 'var(--color-dim)' },
  { tag: tags.contentSeparator, color: 'var(--color-dim)' },
  { tag: tags.atom, color: 'var(--color-warning)' },
  { tag: tags.keyword, color: 'var(--color-primary)' }
]);
