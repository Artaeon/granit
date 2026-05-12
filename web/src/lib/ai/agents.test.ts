import { describe, expect, it } from 'vitest';
import {
  AGENT_MODES,
  GENERIC_MODES,
  CONTEXTUAL_MODES,
  PERSONAS,
  findMode
} from './agents';

// AGENT_MODES is the canonical posture catalog the AIOverlay sidebar
// + /chat page both render. These tests act as a contract: future
// edits that accidentally regress key invariants (Researcher must
// emit wikilinks; every mode must have a non-empty system prompt;
// findMode must always return SOMETHING so callers don't have to
// null-check) will fail loudly instead of silently breaking the UX.

describe('AGENT_MODES catalog', () => {
  it('every mode has an id, label, glyph, tagline, and system prompt', () => {
    for (const m of AGENT_MODES) {
      expect(m.id, `mode missing id`).toBeTruthy();
      expect(m.label, `${m.id} missing label`).toBeTruthy();
      expect(m.glyph, `${m.id} missing glyph`).toBeTruthy();
      expect(m.tagline, `${m.id} missing tagline`).toBeTruthy();
      expect(m.system.length, `${m.id} system prompt too short`).toBeGreaterThan(40);
    }
  });

  it('mode IDs are unique', () => {
    const ids = AGENT_MODES.map((m) => m.id);
    const dupes = ids.filter((id, i) => ids.indexOf(id) !== i);
    expect(dupes).toEqual([]);
  });

  it('mode glyphs are short enough to fit in a chip (<=3 chars)', () => {
    for (const m of AGENT_MODES) {
      expect(m.glyph.length, `${m.id} glyph "${m.glyph}" too long`).toBeLessThanOrEqual(3);
    }
  });

  it('GENERIC_MODES / CONTEXTUAL_MODES / PERSONAS partition the catalog', () => {
    const total = GENERIC_MODES.length + CONTEXTUAL_MODES.length + PERSONAS.length;
    expect(total).toBe(AGENT_MODES.length);
  });

  it('findMode returns the requested mode when it exists', () => {
    const r = findMode('researcher');
    expect(r.id).toBe('researcher');
  });

  it('findMode falls back to the first mode on unknown ID (never null)', () => {
    const fallback = findMode('does-not-exist-xyz');
    expect(fallback).toBeTruthy();
    expect(fallback.id).toBe(AGENT_MODES[0].id);
  });
});

describe('Researcher mode contract', () => {
  // The Researcher mode is the one used by the AI-research workflow:
  // user types a topic, model returns an outline with wikilink
  // headings, user clicks each [[Chapter]] to generate it. If the
  // prompt regresses to plain `## Foo` headings (without
  // [[brackets]]), the click-to-generate workflow silently breaks —
  // the user clicks a heading and nothing happens because there's
  // no link to navigate to. This test guards against that.

  const r = findMode('researcher');

  it('exists in the catalog', () => {
    expect(r.id).toBe('researcher');
  });

  it('system prompt mandates [[wikilink]] format in chapter headings', () => {
    // The prompt must contain BOTH the literal brackets AND the
    // word "wikilink" or similar context, so a refactor that
    // accidentally drops the format example still trips a test.
    expect(r.system).toContain('[[');
    expect(r.system).toContain(']]');
    // Defence against someone changing only the example without
    // updating the rule text — the rule must explicitly mention
    // wrapping in brackets.
    expect(r.system.toLowerCase()).toMatch(/\bbrackets?\b/);
  });

  it('system prompt instructs no-preamble output', () => {
    // The chapter-writer downstream relies on the outline starting
    // with `# Topic`, not `Sure! Here's an outline:`. The prompt
    // must say so explicitly.
    expect(r.system.toLowerCase()).toMatch(/no preamble|only.*outline|no sign-?off/);
  });

  it('system prompt asks for 5-9 chapters', () => {
    // Calibration: too few and the outline isn't useful; too many
    // and the user is overwhelmed. Magic numbers in the prompt are
    // load-bearing — if a future edit cranks it to "20 chapters",
    // we'd want to know.
    expect(r.system).toMatch(/5-9|5–9|five to nine|5 to 9/i);
  });
});
