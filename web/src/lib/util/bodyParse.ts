// Shared body-parser cache. Each notes-rail panel previously did its
// own full-body split + scan inside a `$derived.by` over `body` —
// Outline, SectionQuestionsPanel, ResearchPanel each ran on every
// keystroke, often duplicated by the desktop-rail / mobile-drawer
// double-mount. On a long note that's:
//
//   2× Outline (split + heading scan)
//   2× SectionQuestionsPanel (split + section walk)
//   2× ResearchPanel × 3 (each: stripFences regex + split + scan)
//
// = ~10 full-doc passes per keystroke. On a 5000-line note typing in
// real time, the main thread can't keep up — the input event queue
// piles up, autosave fires, MORE reactivity hits, and the page goes
// unresponsive.
//
// Fix: derive the canonical line breakdown ONCE per body reference,
// memoised by identity. Panels read from this util instead of running
// their own split. The cost stays linear-in-N but happens once per
// keystroke regardless of how many panels are mounted.
//
// Identity-based cache (not deep equality): every keystroke produces
// a new `body` string by reference, so a single-slot cache is enough
// to coalesce multi-reader requests within the same render pass. We
// also keep the previous slot so a brief flip (panel A → panel B → A)
// still hits cache.

export interface ParsedHeading {
  level: number;
  text: string;
  line: number;
}

export interface ParsedSection {
  line: number;
  level: number;
  title: string;
  body: string;
}

export interface ParsedBody {
  /** Original body — identity check key. Never mutated. */
  source: string;
  /** Line array, fence-aware. Index i = line i+1. */
  lines: string[];
  /** Per-line "is inside a fenced code block" flag — fence-aware
   *  scans (headings, refs, sources) consult this instead of
   *  re-tracking the fence state themselves. */
  inFence: boolean[];
  /** All headings, level 1-6, in document order. Fence-aware. */
  headings: ParsedHeading[];
  /** All sections, level 1-3, with body until the next heading.
   *  Empty bodies filtered out. Used by SectionQuestionsPanel. */
  sections: ParsedSection[];
}

let slot1: ParsedBody | null = null;
let slot2: ParsedBody | null = null;

export function parseBody(body: string): ParsedBody {
  // Identity short-circuit. Reference equality is what we want — every
  // keystroke produces a new body string instance (CodeMirror's
  // doc.toString() allocates), so the slot only matches when the
  // reactive system is reading the same body multiple times within
  // the same render pass.
  if (slot1 && slot1.source === body) return slot1;
  if (slot2 && slot2.source === body) {
    // Promote slot2 to slot1 (LRU-2).
    const tmp = slot2;
    slot2 = slot1;
    slot1 = tmp;
    return slot1;
  }
  const parsed = parse(body);
  slot2 = slot1;
  slot1 = parsed;
  return parsed;
}

function parse(body: string): ParsedBody {
  const lines = body.split('\n');
  const inFence: boolean[] = new Array(lines.length);
  const headings: ParsedHeading[] = [];
  const sections: ParsedSection[] = [];
  let fence = false;
  let cur: ParsedSection | null = null;
  // We collect section bodies as line arrays to avoid quadratic
  // string concatenation on big notes — joined once at the end.
  const sectionBuf: string[][] = [];

  for (let i = 0; i < lines.length; i++) {
    const ln = lines[i];
    const t = ln.trim();
    // Fence markers are detected on the trimmed line (matches what
    // the previous SectionQuestionsPanel implementation did) — close
    // enough to GFM's "fence on its own line" rule for our purposes.
    const isFenceMark = t.startsWith('```') || t.startsWith('~~~');
    if (isFenceMark) {
      fence = !fence;
      inFence[i] = fence;
      if (cur) sectionBuf[sections.length - 1].push(ln);
      continue;
    }
    inFence[i] = fence;
    if (fence) {
      if (cur) sectionBuf[sections.length - 1].push(ln);
      continue;
    }
    // Heading detection runs against the UNTRIMMED line, matching
    // the prior Outline implementation. This makes "  # foo"
    // (leading whitespace) NOT a heading, same as before. The
    // SectionQuestionsPanel did its match against the trimmed line
    // and so was a touch more permissive; the union here uses the
    // stricter rule because the union is read by Outline too and
    // surprising new headings would be visible to the user.
    const hm = /^(#{1,6})\s+(.+?)\s*#*$/.exec(ln);
    if (hm) {
      const level = hm[1].length;
      const text = hm[2].trim();
      headings.push({ level, text, line: i + 1 });
      if (level <= 3) {
        cur = { line: i + 1, level, title: text, body: '' };
        sections.push(cur);
        sectionBuf.push([]);
      }
      continue;
    }
    if (cur) sectionBuf[sections.length - 1].push(ln);
  }

  // Materialise section bodies. Trailing newline parity with the
  // pre-refactor implementations (each appended `ln + '\n'`).
  for (let i = 0; i < sections.length; i++) {
    sections[i].body = sectionBuf[i].length > 0 ? sectionBuf[i].join('\n') + '\n' : '';
  }
  // SectionQuestionsPanel filters empty-body sections. Do that here so
  // every consumer gets the same shape.
  const nonEmpty = sections.filter((s) => s.body.trim().length > 0);

  return {
    source: body,
    lines,
    inFence,
    headings,
    sections: nonEmpty
  };
}
