import { describe, it, expect } from 'vitest';
import { parseFlashcards } from './scheduleFlashcards';

// parseFlashcards is the parser that turns the InlineAIMenu
// "flashcards" preset output into structured Q/A pairs for the
// spaced-rep scheduler. The format the preset emits is:
//
//   Q: question?
//   A: answer.
//
//   Q: ...
//   A: ...
//
// These tests pin the parser's behaviour on the shapes it
// actually has to consume — verbatim AI output, hand-edited
// markdown, and the edge cases that almost-pass.

describe('parseFlashcards', () => {
  it('returns [] for empty input', () => {
    expect(parseFlashcards('')).toEqual([]);
  });

  it('parses a minimal Q:/A: pair', () => {
    const out = parseFlashcards('Q: what is X?\nA: it is Y.');
    expect(out).toEqual([{ question: 'what is X?', answer: 'it is Y.' }]);
  });

  it('parses multiple cards separated by blank lines', () => {
    const body = `Q: what is one?
A: a number.

Q: what is two?
A: another number.`;
    expect(parseFlashcards(body)).toEqual([
      { question: 'what is one?', answer: 'a number.' },
      { question: 'what is two?', answer: 'another number.' }
    ]);
  });

  it('tolerates markdown emphasis on the markers', () => {
    const body = `**Q:** what is one?
**A:** a number.

*Q:* what is two?
*A:* another number.`;
    const out = parseFlashcards(body);
    expect(out).toHaveLength(2);
    expect(out[0].question).toBe('what is one?');
    expect(out[1].question).toBe('what is two?');
  });

  it('tolerates bullet prefixes on the markers', () => {
    const body = `- Q: bullet one?
- A: yes.
* Q: bullet two?
* A: also yes.`;
    const out = parseFlashcards(body);
    expect(out).toHaveLength(2);
  });

  it('tolerates numbering on the markers', () => {
    const body = `1. Q: numbered one?
1. A: yes.

2. Q: numbered two?
2. A: also yes.`;
    const out = parseFlashcards(body);
    expect(out).toHaveLength(2);
  });

  it('allows a single blank line between Q and A', () => {
    const body = `Q: question?

A: answer.`;
    expect(parseFlashcards(body)).toEqual([{ question: 'question?', answer: 'answer.' }]);
  });

  it('skips a Q with no following A', () => {
    const body = `Q: orphan question
some other text
Q: real question?
A: real answer.`;
    const out = parseFlashcards(body);
    expect(out).toHaveLength(1);
    expect(out[0].question).toBe('real question?');
  });

  it('does not consume an A line as a Q', () => {
    // Walks the A line forward; the next iteration shouldn't treat
    // it as a fresh Q candidate.
    const body = `Q: q1?
A: a1.
Q: q2?
A: a2.`;
    expect(parseFlashcards(body)).toHaveLength(2);
  });

  it('preserves trailing punctuation in Q and A bodies', () => {
    const body = `Q: question with comma, and period.
A: answer with semicolon; and period.`;
    expect(parseFlashcards(body)).toEqual([
      { question: 'question with comma, and period.', answer: 'answer with semicolon; and period.' }
    ]);
  });

  it('handles multibyte text in Q/A bodies (German / Greek / CJK)', () => {
    const body = `Q: Was bedeutet "ἀγάπη"?
A: 爱 — the highest form of love.`;
    expect(parseFlashcards(body)).toEqual([
      { question: 'Was bedeutet "ἀγάπη"?', answer: '爱 — the highest form of love.' }
    ]);
  });

  it('drops a card whose Q body is whitespace', () => {
    const body = `Q:   \nA: lonely answer.`;
    expect(parseFlashcards(body)).toEqual([]);
  });

  it('drops a card whose A body is whitespace', () => {
    const body = `Q: lonely question?\nA:   `;
    expect(parseFlashcards(body)).toEqual([]);
  });

  it('parses CRLF line endings the same as LF', () => {
    const body = 'Q: cross-platform?\r\nA: yes.\r\n\r\nQ: again?\r\nA: yes.';
    expect(parseFlashcards(body)).toHaveLength(2);
  });

  it('ignores lines that look like Q but lack the colon/punctuation marker', () => {
    // "Quick brown fox" must NOT match the Q regex.
    const body = `Quick brown fox jumps.
Q: real?
A: yes.`;
    expect(parseFlashcards(body)).toHaveLength(1);
  });
});
