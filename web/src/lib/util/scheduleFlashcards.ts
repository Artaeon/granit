// Parses Q/A flashcard pairs from a note body and schedules each
// one as a 5-step spaced-repetition review series on the calendar.
//
// Same review cadence the scripture memory-verse drill uses
// (web/src/routes/scripture/+page.svelte → scheduleVerseReview):
// reviews land on day 1, 3, 7, 14, and 30 from today at 09:00. Each
// review surfaces on the calendar via the existing task_scheduled
// row plumbing — no new event type, no new schema. Tasks anchor on
// today's daily note (the task store requires a real notePath; we
// can't predict that a future daily note will exist).
//
// Card format expected — matches the InlineAIMenu "flashcards"
// preset output exactly:
//
//   Q: What is X?
//   A: Y.
//
//   Q: ...
//   A: ...
//
// Cards are detected by a Q: line immediately followed by an A:
// line. Blank lines between cards are encouraged but not required;
// the parser just walks paragraph-by-paragraph. We tolerate optional
// leading punctuation ("**Q:**", "Q.") so the user can hand-edit
// the cards into their preferred markdown emphasis without breaking
// the round trip.
//
// Returns the count of cards detected + the count of reviews
// successfully scheduled. The caller renders a toast off that.

import { api } from '$lib/api';

export interface Flashcard {
  question: string;
  answer: string;
}

const REVIEW_INTERVALS_DAYS = [1, 3, 7, 14, 30];

// Match a Q line — case-insensitive, optional markdown emphasis,
// optional numbering. Same for A. The body captures everything
// after the marker until end-of-line.
const Q_LINE = /^\s*(?:[-*]\s+)?(?:\d+\.\s*)?(?:\*\*?)?\s*Q[:.\s]+(.+?)(?:\*\*?)?\s*$/i;
const A_LINE = /^\s*(?:[-*]\s+)?(?:\d+\.\s*)?(?:\*\*?)?\s*A[:.\s]+(.+?)(?:\*\*?)?\s*$/i;

export function parseFlashcards(body: string): Flashcard[] {
  if (!body) return [];
  const lines = body.split(/\r?\n/);
  const cards: Flashcard[] = [];
  for (let i = 0; i < lines.length - 1; i++) {
    const qMatch = lines[i].match(Q_LINE);
    if (!qMatch) continue;
    // The A line MUST be the next non-blank line. We tolerate one
    // blank line between Q and A — some authors put a gap for
    // readability when the answer is the only thing on the line.
    let j = i + 1;
    while (j < lines.length && lines[j].trim() === '') j++;
    if (j >= lines.length) break;
    const aMatch = lines[j].match(A_LINE);
    if (!aMatch) continue;
    const question = qMatch[1].trim();
    const answer = aMatch[1].trim();
    if (question && answer) {
      cards.push({ question, answer });
    }
    i = j; // jump past the A line so we don't try to read it as a Q
  }
  return cards;
}

export interface ScheduleResult {
  cards: number;
  scheduled: number;
  failed: number;
}

// Schedules a 5-session review series for every card detected in
// `body`. Anchors all tasks on the user's daily note for today
// (api.daily('today') ensures it exists). Each review task carries
// the question + answer in its text so the calendar hover already
// shows what's being drilled.
//
// Returns the counts so the caller can render a toast. Partial
// failures (some tasks fail to create) leave the rest intact —
// better to schedule SOME of the series than to abort everything.
export async function scheduleFlashcards(body: string): Promise<ScheduleResult> {
  const cards = parseFlashcards(body);
  if (cards.length === 0) {
    return { cards: 0, scheduled: 0, failed: 0 };
  }
  const today = await api.daily('today');
  const base = new Date();
  base.setHours(9, 0, 0, 0);
  let scheduled = 0;
  let failed = 0;
  for (const card of cards) {
    for (const days of REVIEW_INTERVALS_DAYS) {
      const at = new Date(base.getTime() + days * 86_400_000);
      const label = card.question.length > 60
        ? card.question.slice(0, 60) + '…'
        : card.question;
      try {
        await api.createTask({
          notePath: today.path,
          text: `Flashcard · ${label} (day ${days})\nQ: ${card.question}\nA: ${card.answer}`,
          scheduledStart: at.toISOString(),
          durationMinutes: 3,
          tags: ['flashcard']
        });
        scheduled++;
      } catch {
        failed++;
      }
    }
  }
  return { cards: cards.length, scheduled, failed };
}
