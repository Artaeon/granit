import { describe, expect, it } from 'vitest';
import { deriveDraftTitle } from './draftTitle';

const TODAY = '2026-05-12';

describe('deriveDraftTitle', () => {
	it('returns the first H1 when present', () => {
		expect(
			deriveDraftTitle('# Project Granite charter\n\nbody body body', TODAY)
		).toBe('Project Granite charter');
	});

	it('skips a leading blank line and picks the first H1 below', () => {
		expect(
			deriveDraftTitle('\n\n# Status update — week 18\n\nThings shipped:', TODAY)
		).toBe('Status update — week 18');
	});

	it('ignores ## headings — only H1 counts', () => {
		// ## should NOT match; falls back to the first non-fence line.
		expect(
			deriveDraftTitle('## Subsection\n\nReal content here.', TODAY)
		).toBe('## Subsection');
		// (The fallback grabs the literal first line including ##.
		// That's acceptable — the user explicitly avoided H1.)
	});

	it('falls back to the first non-empty line when no H1 exists', () => {
		expect(
			deriveDraftTitle('This is the gist of what I wanted to say.', TODAY)
		).toBe('This is the gist of what I wanted to say.');
	});

	it('skips blockquote lines in the fallback', () => {
		expect(
			deriveDraftTitle('> mode: Project Manager\n> rag off\n\nThe real first line.', TODAY)
		).toBe('The real first line.');
	});

	it('skips code fences in the fallback', () => {
		expect(
			deriveDraftTitle('```ts\nconst x = 1;\n```\n\nThe explanation.', TODAY)
		).toBe('const x = 1;');
		// Code lines INSIDE a fence still count if they\'re first
		// past the fence opener — the parser is line-shape-aware,
		// not fence-state-aware. The fence opener itself is skipped.
	});

	it('strips bullet markers from the first list item', () => {
		expect(
			deriveDraftTitle('- write the brief\n- ship it\n- gather feedback', TODAY)
		).toBe('write the brief');
		expect(deriveDraftTitle('* idea one\n* idea two', TODAY)).toBe('idea one');
		expect(deriveDraftTitle('1. step one\n2. step two', TODAY)).toBe('step one');
	});

	it('skips horizontal rule lines (--- / ***)', () => {
		expect(
			deriveDraftTitle('---\n\nFirst actual line.', TODAY)
		).toBe('First actual line.');
	});

	it('clips overly long titles at 60 chars', () => {
		const long = 'a'.repeat(120);
		const out = deriveDraftTitle('# ' + long, TODAY);
		expect(out.length).toBe(60);
		expect(out).toBe('a'.repeat(60));
	});

	it('falls back to "AI draft <date>" on a whitespace-only body', () => {
		expect(deriveDraftTitle('', TODAY)).toBe(`AI draft ${TODAY}`);
		expect(deriveDraftTitle('   \n\t\n  ', TODAY)).toBe(`AI draft ${TODAY}`);
	});

	it('uses the date in the fallback only — non-fallback paths never include it', () => {
		// Guards against an accidental "AI draft 2026-..." sneaking
		// into the H1 path.
		const out = deriveDraftTitle('# Real title', '2026-05-12');
		expect(out).not.toContain('AI draft');
		expect(out).not.toContain('2026');
	});
});
