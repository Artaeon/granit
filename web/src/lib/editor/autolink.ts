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

// Normalise a string for alias matching. Accent strip + lowercase +
// trailing-morphology strip lets "cafe" match "Café", "projects" match
// "Project", "running" match a (hypothetical) "Run" stub, etc.
//
// NFD decomposes precomposed accented chars into base + combining mark
// so the Unicode-property regex can drop just the marks. Plural/verb
// suffix strips are coarse — we don't try to be a full lemmatiser
// (that's a lossy game). The 3-char-base guard prevents nonsense like
// "is" → "i" or "as" → "a" that would explode the false-positive rate.
//
// Applied to both the user's phrase and the cached titles so that
// matching is symmetric — e.g. title "Projects" and phrase "project"
// both reduce to "project".
function normaliseForAlias(s: string): string {
  let out = s.trim().toLowerCase().normalize('NFD').replace(/\p{Diacritic}/gu, '');
  if (out.length < MIN_PHRASE_CHARS) return out;
  // Verb-form strips first. -ing is longer than -ed and longer than
  // any plural suffix, so try it first to avoid a -s strip on "-ings"
  // (e.g. "meetings" should land on "meeting", not "meetinge").
  if (out.length >= MIN_PHRASE_CHARS + 3 && out.endsWith('ing')) {
    out = out.slice(0, -3);
  } else if (out.length >= MIN_PHRASE_CHARS + 2 && out.endsWith('ed')) {
    out = out.slice(0, -2);
  } else if (out.length >= MIN_PHRASE_CHARS + 3 && out.endsWith('ies')) {
    // "stories" → "story". Ordering matters: must run before -es so
    // "stories" doesn't first lose its "es" → "stori".
    out = out.slice(0, -3) + 'y';
  } else if (out.length >= MIN_PHRASE_CHARS + 2 && out.endsWith('es')) {
    out = out.slice(0, -2);
  } else if (out.length >= MIN_PHRASE_CHARS + 1 && out.endsWith('s')) {
    out = out.slice(0, -1);
  }
  return out;
}

// Build a case-insensitive index keyed by lowercase title. Maps
// title-lowercase → first matching entry (rare collisions land on
// first-wins, which is consistent with wikilink resolution semantics).
//
// `aliasIndex` is the parallel map keyed by normaliseForAlias(title) —
// used only when exact lookup misses. Both rebuild together on cache
// invalidation so they never drift apart.
let titleIndex: Map<string, TitleEntry> | null = null;
let aliasIndex: Map<string, TitleEntry> | null = null;
let titleIndexCache: TitleEntry[] | null = null;

function getIndex(titles: TitleEntry[]): {
  exact: Map<string, TitleEntry>;
  alias: Map<string, TitleEntry>;
} {
  // Rebuild only when the array reference itself changes — the
  // wikilinks module caches at module scope so this stays cheap.
  if (titleIndex && aliasIndex && titleIndexCache === titles) {
    return { exact: titleIndex, alias: aliasIndex };
  }
  titleIndexCache = titles;
  const exact = new Map<string, TitleEntry>();
  const alias = new Map<string, TitleEntry>();
  for (const t of titles) {
    const k = t.title.trim().toLowerCase();
    if (!k) continue;
    if (!exact.has(k)) exact.set(k, t);
    // Index every title by its normalised form too, even when the
    // normalised form equals the lowercase form — otherwise a title
    // like "Project" (normalised "project", lowercase "project") would
    // never resolve from a phrase "projects" (normalised "project"),
    // because the lookup-side normalisation strips suffixes the
    // title-side never had. First-wins on collisions, same as exact.
    const a = normaliseForAlias(t.title);
    if (a && !alias.has(a)) alias.set(a, t);
  }
  titleIndex = exact;
  aliasIndex = alias;
  return { exact, alias };
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
  const { exact: exactIndex, alias: aliasIdx } = getIndex(titles);

  // Returns true if the phrase at [phraseStart, ctx.pos) sits inside an
  // already-closed wikilink (e.g. user clicked back into [[…]] and is
  // editing the target). Open-bracket detection upstream catches the
  // open case; this catches the closed case the upstream regex misses.
  const phraseInsideWikilink = (phraseStart: number): boolean => {
    const leftCtx = docText.slice(Math.max(0, phraseStart - 2), phraseStart);
    const rightCtx = docText.slice(ctx.pos, Math.min(docText.length, ctx.pos + 2));
    return leftCtx.endsWith('[[') && rightCtx.startsWith(']]');
  };

  // Shape of a successful completion result. Same options spec for both
  // exact and alias hits — only the label and boost differ — so we
  // build it once here rather than duplicate the CodeMirror payload.
  const buildResult = (
    phraseStart: number,
    hit: TitleEntry,
    isAlias: boolean
  ): CompletionResult => ({
    from: phraseStart,
    to: ctx.pos,
    options: [
      {
        label: isAlias ? `[[${hit.title}]] (alias match)` : `[[${hit.title}]]`,
        detail: isAlias ? 'autolink note (alias)' : 'autolink note',
        type: 'note',
        // Alias hits sit a notch below exact so that if both ever
        // surface in the same picker frame (e.g. via merged sources)
        // the exact match still wins — the two-pass structure below
        // already prevents simultaneous emission, but the boost gap
        // is a cheap safety net.
        boost: isAlias ? 90 : 99,
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
  });

  // Pass 1: exact lookup across all candidate phrase lengths. Run the
  // whole longest-first sweep before any alias attempt so an exact hit
  // at a SHORTER phrase still beats an alias hit at a longer phrase —
  // otherwise the loop could "downgrade" a clean exact match just
  // because a 4-word alias also resolves, which is exactly the
  // surprise behaviour we want to avoid.
  const maxK = Math.min(wordStarts.length, MAX_PHRASE_WORDS);
  for (let k = maxK; k >= 1; k--) {
    const phraseStart = wordStarts[k - 1];
    const phrase = docText.slice(phraseStart, ctx.pos).trim();
    if (phrase.length < MIN_PHRASE_CHARS) continue;
    if (phraseInsideWikilink(phraseStart)) continue;
    const hit = exactIndex.get(phrase.toLowerCase());
    if (!hit) continue;
    return buildResult(phraseStart, hit, false);
  }

  // Pass 2: alias lookup. Only reached if no exact match was found at
  // any phrase length. Same length-first ordering — longest matching
  // alias wins, since longer phrases carry more intent.
  for (let k = maxK; k >= 1; k--) {
    const phraseStart = wordStarts[k - 1];
    const phrase = docText.slice(phraseStart, ctx.pos).trim();
    if (phrase.length < MIN_PHRASE_CHARS) continue;
    if (phraseInsideWikilink(phraseStart)) continue;
    const normalised = normaliseForAlias(phrase);
    // Re-check the length floor on the NORMALISED form too — a phrase
    // like "is" survives the raw-length gate post-trim only because of
    // surrounding word-break logic, and we never want a sub-3-char
    // alias key driving a suggestion.
    if (normalised.length < MIN_PHRASE_CHARS) continue;
    const hit = aliasIdx.get(normalised);
    if (!hit) continue;
    return buildResult(phraseStart, hit, true);
  }

  return null;
}
