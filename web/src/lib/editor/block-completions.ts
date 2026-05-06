// Built-in block-type completions that share the `/` trigger with
// the user's snippet list. Surfaces all the structural moves a
// power user would otherwise reach for via a separate keymap chord
// or a memorised markdown shape:
//
//     /h1 .. /h6     promote line to a heading of that level
//     /clear         strip heading / list / quote prefix
//     /code          insert a triple-backtick fenced code block
//     /divider       insert a thematic break (---)
//     /bullet        bullet list line
//     /numbered      numbered list line
//     /quote         blockquote line
//     /checkbox      task checkbox line
//     /table         markdown table skeleton (3-col, 1 row)
//     /note          callout — info tone, "> [!note]"
//     /warning       callout — warning tone
//     /tip           callout — tip tone
//     /danger        callout — danger tone
//
// The MarkdownRenderer already understands the Obsidian-style
// callout shape (> [!note], > [!warning], etc), so the inserted
// text renders correctly in the preview pane on the same edit.
//
// Two completion shapes:
//   - "static" entries replace the `/typed` range with a literal
//     string. Cursor lands at the position the snippet author
//     specified via the special $| sentinel (or end of inserted
//     text if no sentinel).
//   - "transform" entries operate on the full LINE the cursor is
//     on, stripping any existing markdown prefix and applying a
//     new one. Used for the heading / list / quote completions so
//     `/h1` over an existing `## Foo` line cleanly produces `# Foo`
//     without piling up hashes.

import type { Completion, CompletionContext, CompletionResult } from '@codemirror/autocomplete';
import type { EditorView } from '@codemirror/view';
import { EditorSelection } from '@codemirror/state';

interface StaticBlock {
  trigger: string;       // includes the leading '/'
  description: string;
  /**
   * Inserted text. Use $| to mark the desired cursor position
   * (omit the sentinel for cursor-at-end). Newlines are preserved.
   */
  insert: string;
}

interface TransformBlock {
  trigger: string;
  description: string;
  /**
   * Receives the bare line content (existing markdown prefix
   * stripped) plus the line's leading whitespace, returns the new
   * full-line content. Used for prefix-mutating commands like
   * /h1 / /quote / /bullet.
   */
  transform: (bareText: string, indent: string) => string;
}

type BlockEntry = StaticBlock | TransformBlock;

function isTransform(b: BlockEntry): b is TransformBlock {
  return 'transform' in b;
}

// Strip any existing markdown line prefix so a transform
// completion can re-apply a fresh one. Mirrors the helper inside
// heading-shortcuts.ts — kept here as a private copy because that
// module also uses it and we don't want a circular import / a
// shared util module just for two callers.
function stripLinePrefix(text: string): string {
  return text
    .replace(/^(\s*)(#{1,6}\s+)/, '$1')
    .replace(/^(\s*)([-*+]\s+\[[ xX]\]\s+)/, '$1') // checkbox first — more specific
    .replace(/^(\s*)([-*+]\s+)/, '$1')
    .replace(/^(\s*)(>\s*\[![a-z]+\]\s*)/, '$1') // callout first
    .replace(/^(\s*)(>\s*)/, '$1')
    .replace(/^(\s*)(\d+\.\s+)/, '$1');
}

// Headings — six levels share the same shape. Generated once at
// module scope so the completion list is stable.
const HEADINGS: TransformBlock[] = [1, 2, 3, 4, 5, 6].map((level) => ({
  trigger: `/h${level}`,
  description: `Heading ${level}`,
  transform: (bare, indent) => {
    const hashes = '#'.repeat(level);
    return bare.trim() ? `${indent}${hashes} ${bare}` : `${indent}${hashes} `;
  }
}));

const BLOCKS: BlockEntry[] = [
  ...HEADINGS,
  {
    trigger: '/clear',
    description: 'Strip heading / list / quote prefix',
    transform: (bare, indent) => `${indent}${bare}`
  },
  {
    trigger: '/bullet',
    description: 'Bullet list item',
    transform: (bare, indent) => bare.trim() ? `${indent}- ${bare}` : `${indent}- `
  },
  {
    trigger: '/numbered',
    description: 'Numbered list item',
    transform: (bare, indent) => bare.trim() ? `${indent}1. ${bare}` : `${indent}1. `
  },
  {
    trigger: '/quote',
    description: 'Blockquote',
    transform: (bare, indent) => bare.trim() ? `${indent}> ${bare}` : `${indent}> `
  },
  {
    trigger: '/checkbox',
    description: 'Task checkbox',
    transform: (bare, indent) => bare.trim() ? `${indent}- [ ] ${bare}` : `${indent}- [ ] `
  },
  {
    trigger: '/code',
    description: 'Fenced code block',
    insert: '```\n$|\n```\n'
  },
  {
    trigger: '/divider',
    description: 'Thematic break (horizontal rule)',
    insert: '\n---\n\n$|'
  },
  {
    trigger: '/embed',
    description: 'Embed another note (![[Title]])',
    // ![[…]] is the Obsidian-style embed shape the MarkdownRenderer
    // hydrates into an inline embed card. Cursor lands inside the
    // brackets so the user immediately starts the wikilink picker.
    insert: '![[$|]]'
  },
  {
    trigger: '/table',
    description: 'Markdown table (3 columns)',
    insert: '| Column 1 | Column 2 | Column 3 |\n| --- | --- | --- |\n| $| | | |\n'
  },
  {
    trigger: '/note',
    description: 'Callout — note',
    insert: '> [!note]\n> $|\n'
  },
  {
    trigger: '/warning',
    description: 'Callout — warning',
    insert: '> [!warning]\n> $|\n'
  },
  {
    trigger: '/tip',
    description: 'Callout — tip',
    insert: '> [!tip]\n> $|\n'
  },
  {
    trigger: '/danger',
    description: 'Callout — danger',
    insert: '> [!danger]\n> $|\n'
  }
];

function applyStatic(view: EditorView, body: StaticBlock, from: number, to: number) {
  // $| marks the desired cursor position. Splitting on it keeps the
  // template authorable as plain strings without an AST.
  const sentinelIdx = body.insert.indexOf('$|');
  const insert = sentinelIdx >= 0
    ? body.insert.slice(0, sentinelIdx) + body.insert.slice(sentinelIdx + 2)
    : body.insert;
  const cursorOffset = sentinelIdx >= 0 ? sentinelIdx : insert.length;
  view.dispatch({
    changes: { from, to, insert },
    selection: EditorSelection.cursor(from + cursorOffset)
  });
}

function applyTransform(view: EditorView, body: TransformBlock, from: number, to: number) {
  // Drop the typed `/trigger` first so it doesn't leak into the
  // line-transform's bare-text input. We replace the matched range
  // with empty, then transform the line's content.
  const line = view.state.doc.lineAt(from);
  const indent = line.text.match(/^\s*/)?.[0] ?? '';
  // The line WITHOUT the typed slash-trigger and any prefix.
  const beforeTrigger = view.state.sliceDoc(line.from, from);
  const afterTrigger = view.state.sliceDoc(to, line.to);
  // The user's selection / cursor sits AFTER the slash — so
  // `beforeTrigger + afterTrigger` is what their line looks like
  // with the trigger erased. Strip any existing markdown prefix
  // off THAT, then apply the new prefix from the transform.
  const merged = beforeTrigger + afterTrigger;
  const bare = stripLinePrefix(merged).slice(indent.length);
  const newLine = body.transform(bare, indent);
  view.dispatch({
    changes: { from: line.from, to: line.to, insert: newLine },
    selection: EditorSelection.cursor(line.from + newLine.length)
  });
}

// Build CodeMirror Completion records for the slash-block list,
// filtered by the user's typed prefix. Mirrors the user-snippet
// completion shape so both lists slot into the same picker.
export function blockCompletionsFor(typed: string): Completion[] {
  const lower = typed.toLowerCase();
  const filtered = BLOCKS.filter((b) => b.trigger.toLowerCase().startsWith(lower));
  return filtered.map((b) => ({
    label: b.trigger,
    detail: b.description,
    type: 'keyword',
    // boost: built-ins sit slightly below user snippets when scores
    // tie — the user's own /meeting / /standup are the more
    // valuable picks if both could match.
    boost: -10,
    apply: (view: EditorView, _completion: Completion, applyFrom: number, applyTo: number) => {
      if (isTransform(b)) {
        applyTransform(view, b, applyFrom, applyTo);
      } else {
        applyStatic(view, b, applyFrom, applyTo);
      }
    }
  }));
}

// CompletionSource that can stand alone OR be merged into another.
// snippets.ts merges these into its own list so the two share a
// single picker; exported separately too in case a future caller
// wants only the built-ins.
export async function blockCompletionSource(ctx: CompletionContext): Promise<CompletionResult | null> {
  const before = ctx.state.sliceDoc(Math.max(0, ctx.pos - 32), ctx.pos);
  const m = /(?:^|\s)(\/[a-z0-9-]*)$/i.exec(before);
  if (!m) return null;
  const typed = m[1];
  const options = blockCompletionsFor(typed);
  if (options.length === 0) return null;
  return { from: ctx.pos - typed.length, options, validFor: /^\/[a-z0-9-]*$/i };
}
