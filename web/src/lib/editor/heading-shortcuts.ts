// Heading-level + block transform shortcuts. Mirrors what Notion
// and the markdown-editor lineage (iA Writer, Bear, Logseq) bind
// for fast inline structure work — Mod-Alt-1 .. 6 sets the line
// to a heading of that level, Mod-Shift-7 toggles a bullet, and
// Mod-Shift-9 wraps in a blockquote. Each command operates on the
// CURRENT line for empty selections, or the smallest set of lines
// covering the selection for ranges.
//
// All commands are no-ops on read-only docs (CodeMirror's editor
// state already short-circuits readOnly, but checking lets us
// return false explicitly so the chord can fall through to the
// next handler in the keymap stack).

import { EditorView, type KeyBinding } from '@codemirror/view';
import { EditorSelection } from '@codemirror/state';

// Strip an existing markdown line prefix (heading hashes, bullets,
// blockquote arrow, numbered list) off `text` and return the bare
// content. The caller usually replaces the stripped portion with a
// new prefix.
function stripLinePrefix(text: string): string {
  return text
    .replace(/^(\s*)(#{1,6}\s+)/, '$1')
    .replace(/^(\s*)([-*+]\s+)/, '$1')
    .replace(/^(\s*)(>\s*)/, '$1')
    .replace(/^(\s*)(\d+\.\s+)/, '$1');
}

// Operate on the line(s) the cursor / selection spans. The provided
// `transform` receives the bare line content and returns the new
// content (with whatever prefix the command wants). Multi-line
// selections get each line transformed independently — same as
// Notion / VS Code's "format multiple lines" behaviour.
function transformLines(
  view: EditorView,
  transform: (bareText: string, indent: string) => string
): boolean {
  if (view.state.readOnly) return false;
  view.dispatch(
    view.state.changeByRange((range) => {
      const startLine = view.state.doc.lineAt(range.from);
      const endLine = view.state.doc.lineAt(range.to);
      const newLines: string[] = [];
      for (let n = startLine.number; n <= endLine.number; n++) {
        const line = view.state.doc.line(n);
        const indent = line.text.match(/^\s*/)?.[0] ?? '';
        const bare = stripLinePrefix(line.text).slice(indent.length);
        newLines.push(transform(bare, indent));
      }
      const insert = newLines.join('\n');
      return {
        changes: { from: startLine.from, to: endLine.to, insert },
        // Place the cursor at the end of the transformed range —
        // matches the "I just promoted this paragraph to H1" UX
        // where the user wants to keep typing the heading.
        range: EditorSelection.cursor(startLine.from + insert.length)
      };
    })
  );
  return true;
}

function setHeading(view: EditorView, level: number): boolean {
  const hashes = '#'.repeat(level);
  return transformLines(view, (bare, indent) => {
    if (!bare.trim()) return `${indent}${hashes} `;
    return `${indent}${hashes} ${bare}`;
  });
}

function clearBlock(view: EditorView): boolean {
  // Mod-Alt-0 — strips any heading / list / quote prefix off the
  // line. Useful for "promote this back to plain text" without
  // selecting and re-typing.
  return transformLines(view, (bare, indent) => `${indent}${bare}`);
}

function toggleBullet(view: EditorView): boolean {
  return transformLines(view, (bare, indent) => {
    if (!bare.trim()) return `${indent}- `;
    return `${indent}- ${bare}`;
  });
}

function toggleQuote(view: EditorView): boolean {
  return transformLines(view, (bare, indent) => {
    if (!bare.trim()) return `${indent}> `;
    return `${indent}> ${bare}`;
  });
}

function toggleInlineCode(view: EditorView): boolean {
  if (view.state.readOnly) return false;
  view.dispatch(
    view.state.changeByRange((range) => {
      if (range.from === range.to) {
        // Empty selection: insert backticks and place the cursor
        // between them.
        return {
          changes: { from: range.from, insert: '``' },
          range: EditorSelection.cursor(range.from + 1)
        };
      }
      const text = view.state.sliceDoc(range.from, range.to);
      // If the selection is already wrapped, strip — same toggle UX
      // as Mod-B / Mod-I.
      if (text.length > 2 && text.startsWith('`') && text.endsWith('`')) {
        const inner = text.slice(1, -1);
        return {
          changes: { from: range.from, to: range.to, insert: inner },
          range: EditorSelection.range(range.from, range.from + inner.length)
        };
      }
      const insert = `\`${text}\``;
      return {
        changes: { from: range.from, to: range.to, insert },
        range: EditorSelection.range(range.from + 1, range.from + 1 + text.length)
      };
    })
  );
  return true;
}

export const headingShortcuts: readonly KeyBinding[] = [
  { key: 'Mod-Alt-1', preventDefault: true, run: (v) => setHeading(v, 1) },
  { key: 'Mod-Alt-2', preventDefault: true, run: (v) => setHeading(v, 2) },
  { key: 'Mod-Alt-3', preventDefault: true, run: (v) => setHeading(v, 3) },
  { key: 'Mod-Alt-4', preventDefault: true, run: (v) => setHeading(v, 4) },
  { key: 'Mod-Alt-5', preventDefault: true, run: (v) => setHeading(v, 5) },
  { key: 'Mod-Alt-6', preventDefault: true, run: (v) => setHeading(v, 6) },
  { key: 'Mod-Alt-0', preventDefault: true, run: clearBlock },
  // Mod-Shift-8 = "*" on most layouts — bullet list. Mod-Shift-9 is
  // the natural pair for "blockquote" because shift-9 = "(" which
  // doesn't conflict with anything else and reads as "wrap this".
  { key: 'Mod-Shift-8', preventDefault: true, run: toggleBullet },
  { key: 'Mod-Shift-9', preventDefault: true, run: toggleQuote },
  // Mod-` for inline code — natural muscle memory: backtick is the
  // markdown marker, modifier+backtick is "wrap selection in it".
  { key: 'Mod-`', preventDefault: true, run: toggleInlineCode }
];
