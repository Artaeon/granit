// Static preset catalog for the InlineAIMenu. Extracted out of the
// menu component because the data was half the file: ~350 lines of
// prompt strings carrying no UI logic, plus the type definitions
// every consumer needs. Living next door (same folder) keeps the
// import path short while letting the menu component focus on
// behaviour (keyboard nav, streaming, accept/reject) rather than
// scrolling past 25 preset definitions every time someone opens
// it.
//
// Adding a preset is a one-block edit here. The menu picks them
// up via the exported PRESETS list; categories must be one of
// PresetCategory and will render in CATEGORY_ORDER.
//
// Why a .ts module and not a .json: prompts span multiple string
// fragments joined with `+`; JSON would force everything onto one
// line. The TS module also lets the type system catch a typo'd
// category at compile time.

export type PresetCategory = 'writing' | 'research' | 'planning' | 'learning';

export interface Preset {
  id: string;
  label: string;
  /** Hint shown faded next to the label — what kind of action this
   *  is at a glance ("rewrite", "generate", etc.). */
  hint: string;
  /** Category groups the list by intent — writing, research,
   *  planning, learning. Headers render between groups so the menu
   *  reads like a tools palette, not a flat dump. */
  category: PresetCategory;
  /** Available in cursor mode (no selection)? */
  cursor: boolean;
  /** Available in selection mode? */
  selection: boolean;
  /** System prompt for the LLM. The user message is built at
   *  submission time using the current selection or body. */
  systemForCursor?: string;
  systemForSelection?: string;
  /** Whether this preset operates on the whole note body when in
   *  cursor mode. False means the action only makes sense on a
   *  selection (chip is hidden in cursor mode). */
  wholeNote?: boolean;
}

export const CATEGORY_LABELS: Record<PresetCategory, string> = {
  writing: 'Writing',
  research: 'Research',
  planning: 'Planning',
  learning: 'Learning'
};

// Render order for category groups — most-used first so the menu's
// top-of-list under the prompt input is always the most common pick.
export const CATEGORY_ORDER: readonly PresetCategory[] = [
  'writing',
  'planning',
  'research',
  'learning'
];

export const PRESETS: Preset[] = [
  // ── Writing ─────────────────────────────────────────────────
  {
    id: 'continue',
    label: 'Continue writing',
    hint: 'extend at cursor',
    category: 'writing',
    cursor: true,
    selection: false,
    systemForCursor:
      "You are a writing assistant continuing the user's prose at the cursor. " +
      'Match their voice, register, and structure. Continue naturally — write 1-3 sentences ' +
      '(or 1-2 short paragraphs at most). Do NOT repeat what came before. ' +
      'Do NOT introduce headers or lists unless the surrounding text already uses them. ' +
      'Return ONLY the continuation text — no preamble, no quotes, no commentary.'
  },
  {
    id: 'improve',
    label: 'Improve writing',
    hint: 'rewrite clearer + tighter',
    category: 'writing',
    cursor: true,
    selection: true,
    wholeNote: true,
    systemForCursor:
      'Rewrite the following note for clarity and concision. Preserve voice, structure, ' +
      'and all the author\'s claims. Drop filler; sharpen verbs; prefer concrete nouns. ' +
      'Return the polished note in full, ready to paste back. No preamble.',
    systemForSelection:
      'Improve the writing of the following passage — clearer, tighter, same voice. ' +
      'Preserve every claim. Return only the rewritten text, no preamble.'
  },
  {
    id: 'fix-grammar',
    label: 'Fix grammar & spelling',
    hint: 'proofread only',
    category: 'writing',
    cursor: true,
    selection: true,
    wholeNote: true,
    systemForCursor:
      'Fix grammar, spelling, and punctuation in the following note. Preserve voice, ' +
      'structure, and meaning exactly. Do NOT shorten, expand, or rephrase. ' +
      'Return the corrected note in full. No preamble.',
    systemForSelection:
      'Fix grammar, spelling, and punctuation in the following. Preserve voice and ' +
      'meaning exactly. Return only the corrected text, no preamble.'
  },
  {
    id: 'shorter',
    label: 'Make shorter',
    hint: 'tighten without losing meaning',
    category: 'writing',
    cursor: true,
    selection: true,
    wholeNote: true,
    systemForCursor:
      'Tighten the following note into the shortest faithful version. Drop redundancy ' +
      'and filler; keep meaning and structure. Return the full shortened note, no preamble.',
    systemForSelection:
      'Tighten the following into the shortest faithful version. Drop redundancy ' +
      'and filler; keep meaning. Return only the shortened text, no preamble.'
  },
  {
    id: 'longer',
    label: 'Make longer',
    hint: 'expand with detail',
    category: 'writing',
    cursor: true,
    selection: true,
    wholeNote: true,
    systemForCursor:
      'Expand the following note with relevant detail and examples. Preserve every claim ' +
      'and the existing structure; add depth, not filler. Return the full expanded note, ' +
      'no preamble.',
    systemForSelection:
      'Expand the following into a fuller paragraph (2-4 sentences) without padding or ' +
      'repetition. Stay in the same voice. Return only the expanded text, no preamble.'
  },
  {
    id: 'match-tone',
    label: 'Match surrounding tone',
    hint: 'rewrite selection in same voice',
    category: 'writing',
    cursor: false,
    selection: true,
    systemForSelection:
      'Rewrite the following passage so its voice, register, and rhythm match the surrounding ' +
      'text. If the user message includes BEFORE / AFTER context blocks, treat those as the ' +
      'reference voice. If no surrounding text was provided (the selection IS the whole note), ' +
      'preserve the passage\'s own existing voice and just smooth out any inconsistencies. ' +
      'Preserve meaning. Return only the rewritten text, no preamble.'
  },
  {
    id: 'translate-en',
    label: 'Translate → English',
    hint: 'natural English',
    category: 'writing',
    cursor: false,
    selection: true,
    systemForSelection:
      'Translate the following into clear, natural English. Preserve markdown formatting. ' +
      'Return only the translation, no preamble.'
  },
  {
    id: 'translate-de',
    label: 'Translate → German',
    hint: 'natürliches Deutsch',
    category: 'writing',
    cursor: false,
    selection: true,
    systemForSelection:
      'Translate the following into clear, natural German. Preserve markdown formatting. ' +
      'Return only the translation, no preamble.'
  },

  // ── Planning ────────────────────────────────────────────────
  {
    id: 'extract-tasks',
    label: 'Extract tasks',
    hint: 'pull action items out',
    category: 'planning',
    cursor: true,
    selection: true,
    wholeNote: true,
    systemForCursor:
      'Read the note and surface every actionable thing in it — implicit or explicit. ' +
      'Return ONLY a markdown task list using `- [ ]` checkboxes. One task per line, ' +
      'each starting with a verb. Skip vague intentions; only items the reader could pick ' +
      'up and start. No preamble.',
    systemForSelection:
      'List every actionable thing in this passage as a markdown `- [ ]` task list. ' +
      'One task per line, each starting with a verb. No preamble.'
  },
  {
    id: 'next-steps',
    label: 'Suggest next steps',
    hint: 'what to do after this note',
    category: 'planning',
    cursor: true,
    selection: false,
    wholeNote: true,
    systemForCursor:
      'Read the note and propose 3-5 concrete next steps for the reader — what to write ' +
      'next, who to talk to, what to read, what to decide. Be specific to the content. ' +
      'Return a markdown bullet list, no preamble.'
  },
  {
    id: 'outline',
    label: 'Outline this note',
    hint: 'H2/H3 structure',
    category: 'planning',
    cursor: true,
    selection: false,
    wholeNote: true,
    systemForCursor:
      'Read the following note and produce a markdown outline of its sections (H2 / H3 ' +
      'headings only, no body text). Use existing section titles when present, propose ' +
      'new ones when there are none. Return only the outline.'
  },
  {
    id: 'summarize',
    label: 'Summarize',
    hint: 'bullet TL;DR',
    category: 'planning',
    cursor: true,
    selection: true,
    wholeNote: true,
    systemForCursor:
      'Summarize the following note in 3-5 concise bullet points. Lead each bullet with ' +
      'the concrete claim. Return ONLY the bullet list, no preamble.',
    systemForSelection:
      'Summarize the following in 3 concise bullet points. Return ONLY the bullets, no preamble.'
  },

  // ── Research ────────────────────────────────────────────────
  {
    id: 'define',
    label: 'Define',
    hint: 'short definition + example',
    category: 'research',
    cursor: false,
    selection: true,
    systemForSelection:
      'Treat the selection as a term or concept the reader wants defined. Return: ' +
      '(1) a one-sentence definition in plain language, (2) the etymology or origin if ' +
      'illuminating, (3) one concrete example. Markdown, no preamble.'
  },
  {
    id: 'steel-man',
    label: 'Steel-man this',
    hint: 'strongest version of the argument',
    category: 'research',
    cursor: false,
    selection: true,
    systemForSelection:
      'Present the strongest version of the argument in this passage — the form a careful ' +
      'opponent would have to actually engage with. Preserve the original claim; tighten the ' +
      'reasoning; supply the best evidence or principle that supports it. Return a short ' +
      'markdown paragraph, no preamble.'
  },
  {
    id: 'counter-argue',
    label: 'Counter-argue',
    hint: 'best objection to this claim',
    category: 'research',
    cursor: false,
    selection: true,
    systemForSelection:
      'Treat the selection as a claim. Give the single strongest objection a thoughtful, ' +
      'charitable critic would raise. Be specific — name the assumption it depends on, the ' +
      'evidence it ignores, or the alternative explanation it doesn\'t address. Return a ' +
      'short markdown paragraph, no preamble.'
  },
  {
    id: 'questions',
    label: 'Open questions',
    hint: 'what this note doesn\'t answer',
    category: 'research',
    cursor: true,
    selection: false,
    wholeNote: true,
    systemForCursor:
      'Read the following note and list 3-5 open questions it raises but doesn\'t answer. ' +
      'Each question should be specific to the content. Return a short markdown bullet ' +
      'list, no preamble.'
  },
  {
    id: 'gaps',
    label: 'Find gaps',
    hint: 'what\'s missing or assumed',
    category: 'research',
    cursor: true,
    selection: false,
    wholeNote: true,
    systemForCursor:
      'Read the following note as a critical editor. What is missing, unclear, or assumed ' +
      'without evidence? Return 3-6 specific gaps as a markdown bullet list — each gap ' +
      'names what\'s missing and what would close it. No preamble.'
  },
  {
    id: 'brainstorm',
    label: 'Brainstorm ideas',
    hint: 'generate options at cursor',
    category: 'research',
    cursor: true,
    selection: false,
    systemForCursor:
      'Brainstorm 5-7 concrete ideas, angles, or directions relevant to what comes before ' +
      'the cursor in this note. Format as a markdown bullet list. Be specific, not generic. ' +
      'Return only the bullets, no preamble.'
  },

  // ── Learning ────────────────────────────────────────────────
  {
    id: 'explain',
    label: 'Explain this',
    hint: 'unpack for a smart non-expert',
    category: 'learning',
    cursor: false,
    selection: true,
    systemForSelection:
      'Explain the following clearly. Assume the reader knows the broad surrounding ' +
      'context but not this specific topic. Return a short markdown explanation: ' +
      'definition, intuition, one concrete example. No preamble.'
  },
  {
    id: 'eli5',
    label: 'Explain like I\'m 5',
    hint: 'simplest possible analogy',
    category: 'learning',
    cursor: false,
    selection: true,
    systemForSelection:
      'Explain the following using language a curious child would follow. Use one concrete ' +
      'analogy from everyday life — not a list, not a definition dump. Be accurate; ' +
      'simplify but don\'t lie. Return a single short paragraph, no preamble.'
  },
  {
    id: 'quiz-me',
    label: 'Quiz me on this note',
    hint: '5 retrieval-practice questions',
    category: 'learning',
    cursor: true,
    selection: false,
    wholeNote: true,
    systemForCursor:
      'Read the note and write 5 retrieval-practice questions that test understanding of ' +
      'the core ideas — not trivia. After each question, give the answer on the next line ' +
      'prefixed with `> `. Markdown, no preamble.'
  },
  {
    id: 'connect',
    label: 'Connect to my other notes',
    hint: 'which linked notes relate, how',
    category: 'learning',
    cursor: true,
    selection: false,
    wholeNote: true,
    systemForCursor:
      'Looking at this note AND the linked notes provided as context (if any), name the ' +
      'specific concepts, claims, or examples that connect them. For each connection: name ' +
      'the linked note in [[wikilink]] form, then one sentence on what connects. If no ' +
      'linked notes were provided, say so plainly. Markdown bullet list, no preamble.'
  },
  {
    // Flashcards — Q/A pairs in a deliberately simple two-line
    // format so the user can read them top-to-bottom AND the
    // scheduler (lib/util/scheduleFlashcards.ts) can parse them
    // for spaced-rep review-task creation. Same shape every time:
    //   Q: question?
    //   A: answer.
    // Blank line between cards. quiz-me asks "do you understand?"
    // and gives 5 questions with `> ` answers; this asks "what
    // should you drill?" and gives more cards with crisper Q/A —
    // suited to spaced-repetition review rather than one-shot
    // self-quizzing.
    id: 'flashcards',
    label: 'Generate flashcards',
    hint: 'Q/A pairs for spaced review',
    category: 'learning',
    cursor: true,
    selection: true,
    wholeNote: true,
    systemForCursor:
      'Read the following note and write flashcards for the concepts a learner would want ' +
      'to drill. Produce 6-10 cards. For each card use EXACTLY this two-line shape with a ' +
      'blank line separating cards:\n\nQ: <single specific question, one sentence>\nA: <single ' +
      'concise answer, one sentence; longer only when essential>\n\nFocus on durable concepts ' +
      '(definitions, mechanisms, "why X causes Y"), not surface trivia. Skip cards that ' +
      'paraphrase what a sibling card already covers. Plain markdown, no preamble, no numbering, ' +
      'no headers.',
    systemForSelection:
      'Read the following passage and write flashcards for it. Produce 3-6 cards. For each card ' +
      'use EXACTLY this two-line shape with a blank line separating cards:\n\nQ: <single specific ' +
      'question>\nA: <single concise answer>\n\nFocus on durable concepts, not surface trivia. ' +
      'Plain markdown, no preamble, no numbering, no headers.'
  },
  {
    // Extract references — scans the body for every external
    // reference (URLs, bible refs, book titles in quotes, "author
    // (year)" citation style, etc.) and emits a deduped References
    // section. Whole-note only because the value is in pulling
    // scattered refs from throughout the document into one place.
    // Lives under research because the user reaches for it during
    // sourcing / fact-checking work, not while writing first drafts.
    id: 'extract-references',
    label: 'Extract references',
    hint: 'build a References section',
    category: 'research',
    cursor: true,
    selection: false,
    wholeNote: true,
    systemForCursor:
      'Scan the following note for EVERY external reference: URLs, bible passages (e.g. ' +
      'John 3:16, Rom. 8:28-30), book or paper titles (italicised, in quotes, or otherwise ' +
      'flagged), author-year citations (Smith 2019, "Smith et al."), and other named sources. ' +
      'Produce ONE markdown section starting with `## References` followed by a deduplicated ' +
      'bullet list. For each entry: render the canonical form (e.g. "Romans 8:28-30", not ' +
      '"rom 8:28-30") and, when an author or context can be inferred, add a brief ' +
      'parenthetical hint. Group by kind only when there are ≥3 of that kind (Scripture, Books, ' +
      'Web, Other). Return ONLY the `## References` section — nothing before or after.'
  }
];
