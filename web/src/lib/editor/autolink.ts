// Autolink-suggest: when the user types a phrase that matches an
// existing note title, the autocomplete picker offers to wrap the
// phrase in [[…]]. Bridges the gap between "I know what I want to
// link to but I'd rather just type than open the wikilink prompt"
// and the existing [[trigger.
//
// Matching strategy: walk backwards from the cursor by word count
// (longest match wins) and look up the candidate phrase against the
// shared title cache from ./wikilinks. Up to 4 words — anything
// longer is noise (note titles with 5+ words are rare, and the
// scan cost grows with phrase length).
//
// Triggering: regular CodeMirror completion source, gated to:
//   - the cursor is on a word boundary (typed a space, punctuation,
//     or end-of-line) — we don't want to fire mid-word;
//   - we're not inside an open wikilink (wikilinkComplete owns that);
//   - the candidate phrase actually matches a known title.
//
// Reuses the title cache from ./wikilinks so a fresh fetch isn't
// needed; invalidateTitleCache() over there is the single point to
// drop the cache (e.g. after a new-note create).

import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete';
import { ensureTitles } from './wikilinks';

// Maximum phrase length (in words) the matcher considers. Bounded
// because long-phrase scans get more expensive and rarely pay off.
const MAX_PHRASE_WORDS = 4;
// Minimum phrase length (in characters) to even attempt a lookup.
// "go" matching a note titled "Go" is noise; require enough text to
// be a genuine reference.
const MIN_PHRASE_CHARS = 3;

// Inverse word-character class — what counts as a word break. Mirrors
// the wikilink + tag regexes elsewhere in the editor: alphanumerics,
// underscore, hyphen, slash all stay as part of a word.
const wordBreakRe = /[^\w/-]/;

// Match a partially-open wikilink ending at the cursor. If we see one,
// we bow out so wikilinkComplete handles the case without a duplicate
// suggestion.
const openWikilinkRe = /\[\[([^\]\n]*)$/;

// Match a partially-open markdown link `[label](` so we don't fire
// inside a manually-typed link either.
const openMdLinkRe = /\[[^\]]*\]\([^)]*$/;

interface TitleEntry {
  title: string;
  path: string;
}

// Build a case-insensitive index keyed by lowercase title. Maps
// title-lowercase → first matching entry (rare collisions land on
// first-wins, which is consistent with wikilink resolution semantics).
let titleIndex: Map<string, TitleEntry> | null = null;
let titleIndexCache: TitleEntry[] | null = null;

function getIndex(titles: TitleEntry[]): Map<string, TitleEntry> {
  // Rebuild only when the array reference itself changes — the
  // wikilinks module caches at module scope so this stays cheap.
  if (titleIndex && titleIndexCache === titles) return titleIndex;
  titleIndexCache = titles;
  const m = new Map<string, TitleEntry>();
  for (const t of titles) {
    const k = t.title.trim().toLowerCase();
    if (!k || m.has(k)) continue;
    m.set(k, t);
  }
  titleIndex = m;
  return m;
}

// Walk back from `pos` collecting word starts. Returns the offsets at
// which each successive word begins, ordered nearest-to-cursor first.
// Stops at line boundaries because cross-line autolinks are almost
// always wrong (a stray "Stoicera" at the end of one line shouldn't
// merge with a phrase on the next).
function findWordStarts(text: string, pos: number): number[] {
  const out: number[] = [];
  let i = pos;
  while (i > 0 && out.length < MAX_PHRASE_WORDS + 1) {
    // Skip the run of word characters
    while (i > 0 && !wordBreakRe.test(text[i - 1]) && text[i - 1] !== '\n') i--;
    out.push(i);
    // Skip the run of breaks (spaces, punctuation) — but stop at \n
    while (i > 0 && wordBreakRe.test(text[i - 1]) && text[i - 1] !== '\n') i--;
    if (i > 0 && text[i - 1] === '\n') break;
  }
  return out;
}

export async function autolinkComplete(ctx: CompletionContext): Promise<CompletionResult | null> {
  // Word-boundary gate. The cursor must sit either at end-of-doc,
  // before whitespace/punctuation, or before a newline. Inside a word
  // we leave the user alone — they're probably still typing.
  const docText = ctx.state.doc.toString();
  if (ctx.pos < docText.length) {
    const next = docText[ctx.pos];
    if (next && !wordBreakRe.test(next) && next !== '\n') return null;
  }

  // Bow out of any wikilink / markdown-link in progress.
  const before = docText.slice(Math.max(0, ctx.pos - 200), ctx.pos);
  if (openWikilinkRe.test(before)) return null;
  if (openMdLinkRe.test(before)) return null;

  // Build the candidate phrase boundaries (longest first). We try the
  // 4-word phrase, then 3, then 2, then 1 — first match wins.
  const wordStarts = findWordStarts(docText, ctx.pos);
  if (wordStarts.length === 0) return null;

  // Map word-count → start offset. wordStarts[0] is the start of the
  // word the cursor sits at the end of; wordStarts[k-1] is the start
  // of the k-th word back.
  const titles = await ensureTitles();
  const index = getIndex(titles);

  for (let k = Math.min(wordStarts.length, MAX_PHRASE_WORDS); k >= 1; k--) {
    const phraseStart = wordStarts[k - 1];
    const phrase = docText.slice(phraseStart, ctx.pos).trim();
    if (phrase.length < MIN_PHRASE_CHARS) continue;
    // Skip phrases that are themselves already inside a wikilink (e.g.
    // the user just typed inside [[…]] without the open detection
    // catching it because the closing brackets are already there).
    // We check the surrounding 2 chars on each side.
    const leftCtx = docText.slice(Math.max(0, phraseStart - 2), phraseStart);
    const rightCtx = docText.slice(ctx.pos, Math.min(docText.length, ctx.pos + 2));
    if (leftCtx.endsWith('[[') && rightCtx.startsWith(']]')) continue;
    // The phrase must end exactly at the cursor — `findWordStarts`
    // guarantees that. Match against the title index.
    const hit = index.get(phrase.toLowerCase());
    if (!hit) continue;

    return {
      from: phraseStart,
      to: ctx.pos,
      options: [
        {
          label: `[[${hit.title}]]`,
          detail: 'autolink note',
          type: 'note',
          // boost so this option lands above other completion sources
          // that might fire on the same trigger (e.g. word completion
          // from the markdown plugin if it's active).
          boost: 99,
          apply: (view, _completion, applyFrom, applyTo) => {
            view.dispatch({
              changes: { from: applyFrom, to: applyTo, insert: `[[${hit.title}]]` },
              selection: { anchor: applyFrom + 4 + hit.title.length }
            });
          }
        }
      ],
      // Filter pattern matches "any text up to the cursor" — letting
      // the user keep typing inside the phrase without immediately
      // dismissing the suggestion. The completion stays valid while
      // the partial phrase is still a prefix of a known title.
      validFor: /[^\n]*/
    };
  }

  return null;
}
