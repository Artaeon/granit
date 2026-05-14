import { describe, expect, it } from 'vitest';
import {
  BRIEFING_SYSTEM_PROMPT,
  buildBriefingUserPrompt
} from './briefingPrompt';
import type { CalendarEvent, Task, Goal, Deadline } from '$lib/api';

// The system prompt is load-bearing: it shapes the entire output
// for the morning page's primary AI surface. A drift here (e.g.
// someone changes "3 paragraphs" → "4 paragraphs" without
// updating the template that splits on \n{2,}) would silently
// break the visual layout.

describe('BRIEFING_SYSTEM_PROMPT', () => {
  it('mandates 3 short paragraphs', () => {
    const lc = BRIEFING_SYSTEM_PROMPT.toLowerCase();
    expect(lc).toContain('three short paragraphs');
  });

  it('forbids inventing data', () => {
    expect(BRIEFING_SYSTEM_PROMPT.toLowerCase()).toContain('never invent');
  });

  it('caps length at 60-110 words', () => {
    expect(BRIEFING_SYSTEM_PROMPT).toContain('60-110 words');
  });

  it('mentions paragraph contracts (shape / focus / watch)', () => {
    // Each paragraph has a specific job; loose paraphrase here so a
    // tightening edit doesn't fail the test, but the three roles
    // must be present for the page's rendering contract to hold.
    const lc = BRIEFING_SYSTEM_PROMPT.toLowerCase();
    expect(lc).toContain('shape of the day');
    expect(lc).toContain('focus on');
    expect(lc).toContain('watch for');
  });
});

const TODAY = '2026-05-15';

function emptyInputs(): Parameters<typeof buildBriefingUserPrompt>[0] {
  return { todayISO: TODAY, events: [], tasks: [], goals: [], deadlines: [] };
}

describe('buildBriefingUserPrompt', () => {
  it('leads with the date', () => {
    const got = buildBriefingUserPrompt(emptyInputs());
    expect(got.startsWith(`It's the morning of ${TODAY}`)).toBe(true);
  });

  it('falls back to a no-data note when everything is empty', () => {
    const got = buildBriefingUserPrompt(emptyInputs()).toLowerCase();
    expect(got).toContain('no data');
  });

  it("shows 'nothing scheduled' for an empty calendar", () => {
    const tasks: Task[] = [{
      id: 't1', text: 'Write the post', notePath: 'a.md', lineNum: 1, done: false,
      priority: 1, createdAt: '2026-05-14T00:00:00Z', tags: []
    } as Task];
    const got = buildBriefingUserPrompt({ ...emptyInputs(), tasks });
    expect(got.toLowerCase()).toContain('nothing scheduled');
    expect(got).toContain('Write the post');
  });

  it('formats events with HH:MM in 24h + optional kind + location', () => {
    const events: CalendarEvent[] = [
      {
        type: 'event',
        title: '1:1 with Sam',
        start: '2026-05-15T09:00:00',
        end: '2026-05-15T09:30:00',
        location: 'Zoom',
        kind: 'meeting'
      },
      {
        type: 'ics_event',
        title: 'Deep work',
        start: '2026-05-15T14:00:00',
        end: '2026-05-15T15:30:00',
        kind: 'focus'
      }
    ];
    const got = buildBriefingUserPrompt({ ...emptyInputs(), events });
    expect(got).toContain('09:00');
    expect(got).toContain('1:1 with Sam');
    expect(got).toContain('[meeting]');
    expect(got).toContain('@ Zoom');
    expect(got).toContain('14:00');
    expect(got).toContain('[focus]');
    // No PM — the morning brief must stay 24h regardless of OS locale.
    expect(got).not.toMatch(/\bPM\b/);
    expect(got).not.toMatch(/\bAM\b/);
  });

  it('marks all-day events explicitly', () => {
    const events: CalendarEvent[] = [
      { type: 'event', title: 'Holiday', date: '2026-05-15' }
    ];
    const got = buildBriefingUserPrompt({ ...emptyInputs(), events });
    expect(got).toContain('all-day');
    expect(got).toContain('Holiday');
  });

  it('caps each list so a noisy backlog does not blow up the prompt', () => {
    const tasks: Task[] = Array.from({ length: 20 }, (_, i) => ({
      id: `t${i}`, text: `task ${i}`, notePath: 'a.md', lineNum: i, done: false,
      priority: 0, createdAt: '2026-05-14T00:00:00Z', tags: []
    })) as Task[];
    const got = buildBriefingUserPrompt({ ...emptyInputs(), tasks });
    // First 8 should appear; later ones should not.
    expect(got).toContain('task 0');
    expect(got).toContain('task 7');
    expect(got).not.toContain('task 8');
    expect(got).not.toContain('task 19');
  });

  it('annotates tasks with priority + due + estimate when present', () => {
    const tasks: Task[] = [
      {
        id: 't1', text: 'Ship the launch', notePath: 'a.md', lineNum: 1, done: false,
        priority: 1, dueDate: '2026-05-15', estimatedMinutes: 90,
        createdAt: '2026-05-14T00:00:00Z', tags: []
      } as Task
    ];
    const got = buildBriefingUserPrompt({ ...emptyInputs(), tasks });
    expect(got).toContain('P1');
    expect(got).toContain('Ship the launch');
    expect(got).toContain('(due 2026-05-15)');
    expect(got).toContain('~90m');
  });

  it('includes active goals (capped at 3)', () => {
    const goals: Goal[] = Array.from({ length: 5 }, (_, i) => ({
      id: `g${i}`, title: `goal ${i}`, status: 'active',
      created_at: '2026-05-01', updated_at: '2026-05-01'
    })) as Goal[];
    const got = buildBriefingUserPrompt({ ...emptyInputs(), goals });
    expect(got).toContain('goal 0');
    expect(got).toContain('goal 2');
    expect(got).not.toContain('goal 3');
    expect(got).not.toContain('goal 4');
  });

  it('includes deadlines with their countdown', () => {
    const deadlines = [
      { d: { id: 'd1', title: 'Q3 report', date: '2026-05-18', importance: 'critical', status: 'active', created_at: '', updated_at: '' } as Deadline, days: 3 },
      { d: { id: 'd2', title: 'Tax filing', date: '2026-05-21', importance: 'high', status: 'active', created_at: '', updated_at: '' } as Deadline, days: 6 }
    ];
    const got = buildBriefingUserPrompt({ ...emptyInputs(), deadlines });
    expect(got).toContain('Q3 report');
    expect(got).toContain('in 3d');
    expect(got).toContain('(critical)');
    expect(got).toContain('Tax filing');
    expect(got).toContain('in 6d');
  });
});
