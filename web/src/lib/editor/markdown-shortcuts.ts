// Markdown formatting shortcuts for the CodeMirror editor: Ctrl-B
// wraps the selection in **, Ctrl-I in *, Ctrl-K wraps as a markdown
// link. Cross-platform — `Mod-` resolves to Cmd on macOS and Ctrl on
// everything else, matching the rest of the app's keymaps.
//
// Each command is a no-op when the doc is read-only (CodeMirror
// short-circuits via state.facet(EditorState.readOnly)).

import { EditorView, type KeyBinding } from '@codemirror/view';
import { EditorSelection } from '@codemirror/state';

// Toggle a symmetric inline marker (** for bold, * for italic) around
// each selection. If the selection already starts and ends with the
// marker we strip it (a second Ctrl-B unbolds), otherwise we wrap.
// Empty selections insert the marker pair and place the cursor between
// them — same UX as Obsidian / iA Writer / VS Code's markdown mode.
function toggleWrap(view: EditorView, marker: string): boolean {
  if (view.state.readOnly) return false;
  view.dispatch(
    view.state.changeByRange((range) => {
      const text = view.state.sliceDoc(range.from, range.to);
      // Already wrapped → strip. Use length compare to avoid false
      // positives on short selections like "**" itself.
      if (
        text.length >= marker.length * 2 &&
        text.startsWith(marker) &&
        text.endsWith(marker)
      ) {
        const inner = text.slice(marker.length, text.length - marker.length);
        return {
          changes: { from: range.from, to: range.to, insert: inner },
          range: EditorSelection.range(range.from, range.from + inner.length)
        };
      }
      // Empty selection → insert pair and put the cursor in the middle.
      if (range.from === range.to) {
        const insert = marker + marker;
        return {
          changes: { from: range.from, insert },
          range: EditorSelection.cursor(range.from + marker.length)
        };
      }
      const insert = marker + text + marker;
      return {
        changes: { from: range.from, to: range.to, insert },
        range: EditorSelection.range(range.from + marker.length, range.from + marker.length + text.length)
      };
    })
  );
  return true;
}

// Ctrl-K: turn selection into a markdown link. If the clipboard is
// not pre-staged with a URL (we can't read clipboard from a keybind
// without async permission flows that are awkward in CM), we use the
// pattern [selection]() and place the cursor inside the parens so the
// user pastes the URL there. Empty selection inserts [](text url) and
// puts the cursor on the link text.
function makeLink(view: EditorView): boolean {
  if (view.state.readOnly) return false;
  view.dispatch(
    view.state.changeByRange((range) => {
      const text = view.state.sliceDoc(range.from, range.to);
      if (range.from === range.to) {
        const insert = '[]()';
        return {
          changes: { from: range.from, insert },
          range: EditorSelection.cursor(range.from + 1) // between the brackets
        };
      }
      const insert = `[${text}]()`;
      // Cursor inside the parens — user pastes URL there. The brackets
      // already wrap their selected text.
      return {
        changes: { from: range.from, to: range.to, insert },
        range: EditorSelection.cursor(range.from + insert.length - 1)
      };
    })
  );
  return true;
}

export const markdownShortcuts: readonly KeyBinding[] = [
  { key: 'Mod-b', preventDefault: true, run: (v) => toggleWrap(v, '**') },
  { key: 'Mod-i', preventDefault: true, run: (v) => toggleWrap(v, '*') },
  // Single `_` for italic too — some users prefer underscores. Same
  // toggle behaviour, accepts the alt shortcut.
  { key: 'Mod-Shift-i', preventDefault: true, run: (v) => toggleWrap(v, '_') },
  { key: 'Mod-k', preventDefault: true, run: makeLink }
];

// Smart paste — multi-format detection for the editor:
//
//  1. URL pasted while text is selected → [selection](url)
//  2. CSV/TSV with ≥2 rows + consistent column count → markdown table
//  3. Code-shaped text (language signals at start) → fenced code block
//  4. Anything else → fall through to CodeMirror's default paste
//
// Conservative on purpose: each detector only fires on strong
// signals so plain prose with a comma here and there isn't reformatted
// against the user's wish. When in doubt, we let the default paste
// handle it. Shift+paste / Mod+V on Mac → both come through here; the
// user can always undo (Mod+Z) if a detection fired they didn't want.

const httpUrl = /^(https?:\/\/[^\s]+)$/;

// Code-likeness heuristic. Returns a guessed language if the text
// looks like code, or null if it's prose. We only fire when the
// FIRST non-blank line carries a strong signal — so a user pasting
// prose with a brace in it doesn't get wrapped.
function guessCodeLanguage(text: string): string | null {
  const t = text.trim();
  if (t.length < 20 || t.length > 8000) return null; // too short / too long
  const firstNonBlank = t.split('\n').find((l) => l.trim().length > 0) ?? '';
  const head = firstNonBlank.trim();
  // Strong language hints — start tokens that prose almost never uses.
  const hints: { lang: string; re: RegExp }[] = [
    { lang: 'go', re: /^(package\s+\w+|func\s+\w+\s*\(|import\s*\(|import\s+"[^"]+")/ },
    { lang: 'ts', re: /^(import\s+.+from\s+['"]|export\s+(function|const|class|interface|type|default))/ },
    { lang: 'js', re: /^(const|let|var)\s+\w+\s*=\s*(function|\(|async)/ },
    { lang: 'py', re: /^(from\s+\w+\s+import|import\s+\w+|def\s+\w+\(|class\s+\w+(\(|:))/ },
    { lang: 'rust', re: /^(use\s+\w+::|fn\s+\w+\s*\(|pub\s+(fn|struct|enum|mod|use))/ },
    { lang: 'sh', re: /^(#!\/(bin|usr)\/.+sh|cd\s+\S+|sudo\s+\S+|export\s+\w+=)/ },
    { lang: 'sql', re: /^(SELECT\s|INSERT\s+INTO|UPDATE\s+\w+|CREATE\s+(TABLE|INDEX|VIEW))/i },
    { lang: 'json', re: /^[{\[]/ },
    { lang: 'yaml', re: /^(---|[\w-]+:\s*[\w-])/ }
  ];
  for (const { lang, re } of hints) {
    if (re.test(head)) {
      // For json / yaml we want extra confidence — both shapes also
      // match plain prose accidentally. Require the doc to actually
      // parse for json; for yaml require multiple key: lines.
      if (lang === 'json') {
        try { JSON.parse(t); return 'json'; } catch { return null; }
      }
      if (lang === 'yaml') {
        const keyLines = t.split('\n').filter((l) => /^[\w-]+:\s/.test(l)).length;
        return keyLines >= 2 ? 'yaml' : null;
      }
      return lang;
    }
  }
  return null;
}

// Tabular-likeness heuristic. Returns a separator if the text is a
// CSV/TSV with consistent column counts ≥2 across ≥2 rows. We try
// tab first (cheapest signal: copying from a spreadsheet pastes
// tabs), then comma, then semicolon (Excel locales).
function detectTableSeparator(text: string): { sep: string; rows: string[][] } | null {
  const lines = text.replace(/\r/g, '').split('\n').filter((l) => l.length > 0);
  if (lines.length < 2) return null;
  for (const sep of ['\t', ',', ';']) {
    const rows = lines.map((l) => splitCsv(l, sep));
    const cols = rows[0].length;
    if (cols < 2) continue;
    if (rows.every((r) => r.length === cols)) {
      // Reject false positives: comma-separated prose like "I went to
      // Paris, Rome, Berlin." would match. Require average column
      // length to be short (under 60 chars) — real table cells are
      // typically short, prose sentences aren't.
      const avg = rows.flat().reduce((s, c) => s + c.length, 0) / (rows.length * cols);
      if (avg > 60) continue;
      return { sep, rows };
    }
  }
  return null;
}

// Tiny CSV row splitter — handles double-quoted cells (RFC 4180),
// escaped quotes ("") inside, and the chosen separator. Doesn't
// handle multi-line quoted cells, which is a bigger format we're
// not trying to support in a paste detector.
function splitCsv(line: string, sep: string): string[] {
  if (sep === '\t') return line.split('\t');
  const out: string[] = [];
  let cur = '';
  let inQuote = false;
  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (inQuote) {
      if (ch === '"') {
        if (line[i + 1] === '"') { cur += '"'; i++; } else { inQuote = false; }
      } else {
        cur += ch;
      }
    } else if (ch === '"') {
      inQuote = true;
    } else if (ch === sep) {
      out.push(cur);
      cur = '';
    } else {
      cur += ch;
    }
  }
  out.push(cur);
  return out.map((c) => c.trim());
}

function rowsToMarkdownTable(rows: string[][]): string {
  if (rows.length === 0) return '';
  const cols = rows[0].length;
  const escape = (s: string) => s.replace(/\|/g, '\\|');
  const head = `| ${rows[0].map(escape).join(' | ')} |`;
  const sep = `| ${Array(cols).fill('---').join(' | ')} |`;
  const body = rows.slice(1).map((r) => `| ${r.map(escape).join(' | ')} |`).join('\n');
  return body ? `${head}\n${sep}\n${body}` : `${head}\n${sep}`;
}

export const smartPaste = EditorView.domEventHandlers({
  paste(event, view) {
    if (view.state.readOnly) return false;
    const clip = event.clipboardData?.getData('text/plain') ?? '';
    if (!clip) return false;
    const trimmed = clip.trim();
    const sel = view.state.selection.main;

    // 1) URL with selection → markdown link.
    if (httpUrl.test(trimmed) && sel.from !== sel.to) {
      const selectedText = view.state.sliceDoc(sel.from, sel.to);
      // Don't auto-wrap if the selection itself looks like a URL — the
      // user may be pasting a URL replacement, in which case we want
      // the plain paste behaviour.
      if (!httpUrl.test(selectedText.trim())) {
        event.preventDefault();
        const insert = `[${selectedText}](${trimmed})`;
        view.dispatch({
          changes: { from: sel.from, to: sel.to, insert },
          selection: EditorSelection.cursor(sel.from + insert.length)
        });
        return true;
      }
    }

    // The remaining detectors only fire when we're pasting AT a
    // cursor (no selection) — replacing a selection with a fenced
    // code block or a markdown table would be confusing. The user
    // wanted to overwrite the selected text, not transform paste.
    if (sel.from !== sel.to) return false;

    // Don't fire detectors inside an existing fenced code block —
    // the user's clearly pasting code into code.
    const lineFrom = view.state.doc.lineAt(sel.from);
    const before = view.state.sliceDoc(0, lineFrom.from);
    const fenceCount = (before.match(/^```/gm) || []).length;
    const insideFence = fenceCount % 2 === 1;
    if (insideFence) return false;

    // 2) Tabular paste → markdown table.
    const table = detectTableSeparator(trimmed);
    if (table) {
      event.preventDefault();
      const md = rowsToMarkdownTable(table.rows);
      // Surround with blank lines so the table renders as its own
      // block. The leading newline is dropped if we're already at the
      // start of a fresh line.
      const atLineStart = sel.from === lineFrom.from;
      const lead = atLineStart ? '' : '\n\n';
      const insert = `${lead}${md}\n`;
      view.dispatch({
        changes: { from: sel.from, to: sel.from, insert },
        selection: EditorSelection.cursor(sel.from + insert.length)
      });
      return true;
    }

    // 3) Code-shaped paste → fenced code block.
    const lang = guessCodeLanguage(trimmed);
    if (lang) {
      event.preventDefault();
      const atLineStart = sel.from === lineFrom.from;
      const lead = atLineStart ? '' : '\n\n';
      const insert = `${lead}\`\`\`${lang}\n${trimmed}\n\`\`\`\n`;
      view.dispatch({
        changes: { from: sel.from, to: sel.from, insert },
        selection: EditorSelection.cursor(sel.from + insert.length)
      });
      return true;
    }

    // Otherwise — let the default paste handler run.
    return false;
  }
});
