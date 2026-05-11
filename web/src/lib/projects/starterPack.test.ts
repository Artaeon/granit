import { describe, expect, it } from 'vitest';
import {
	buildStarterPackPrompt,
	parseStarterPackResponse,
	defaultStarterPath
} from './starterPack';

describe('buildStarterPackPrompt', () => {
	it('returns a system + user pair', () => {
		const { system, user } = buildStarterPackPrompt({ name: 'Granite' });
		expect(system).toMatch(/JSON array/);
		expect(system).toMatch(/Charter/);
		expect(system).toMatch(/Milestones/);
		expect(system).toMatch(/Risks/);
		expect(system).toMatch(/Kickoff/);
		expect(user).toContain('Project name: Granite');
	});

	it('includes optional fields when present', () => {
		const { user } = buildStarterPackPrompt({
			name: 'Granite',
			description: 'a knowledge manager',
			kind: 'side-project',
			venture: 'Artaeon',
			tags: ['svelte', 'go'],
			next_action: 'ship the chat overlay'
		});
		expect(user).toContain('Description: a knowledge manager');
		expect(user).toContain('Kind: side-project');
		expect(user).toContain('Venture: Artaeon');
		expect(user).toContain('Tags: svelte, go');
		expect(user).toContain('Stated next action: ship the chat overlay');
	});

	it('omits optional fields cleanly when empty', () => {
		const { user } = buildStarterPackPrompt({ name: 'Solo' });
		// No empty "Description: " / "Tags: " lines.
		expect(user).not.toMatch(/Description: $/m);
		expect(user).not.toMatch(/Tags: $/m);
		expect(user).not.toContain('Kind:');
		expect(user).not.toContain('Venture:');
	});

	it('produces deterministic output for the same input (test-friendly)', () => {
		const input = { name: 'Granite', description: 'x' };
		const a = buildStarterPackPrompt(input);
		const b = buildStarterPackPrompt(input);
		expect(a).toEqual(b);
	});
});

describe('parseStarterPackResponse', () => {
	const sample = JSON.stringify([
		{ title: 'Charter', body: '## Why\n- ship\n' },
		{ title: 'Milestones', body: '## Milestones\n- **M1**\n' },
		{ title: 'Risks', body: '## Risks\n- known' },
		{ title: 'Kickoff agenda', body: '## Agenda\n- 0-5min' }
	]);

	it('parses a clean JSON array', () => {
		const got = parseStarterPackResponse(sample);
		expect(got).toHaveLength(4);
		expect(got[0].title).toBe('Charter');
		expect(got[3].title).toBe('Kickoff agenda');
	});

	it('strips ```json / ``` fences the model adds anyway', () => {
		const fenced = '```json\n' + sample + '\n```';
		expect(parseStarterPackResponse(fenced)).toHaveLength(4);
	});

	it('handles leading/trailing prose ("Here you go:")', () => {
		const messy =
			'Here are your four starter documents:\n\n' + sample + '\n\nLet me know if you want a different format.';
		expect(parseStarterPackResponse(messy)).toHaveLength(4);
	});

	it('drops entries missing title or body, keeps the valid ones', () => {
		const mixed = JSON.stringify([
			{ title: 'Charter', body: 'real' },
			{ title: '', body: 'no title' },
			{ title: 'No body' }, // missing body
			{ body: 'no title field' }, // missing title
			{ title: 'Risks', body: 'real too' }
		]);
		const got = parseStarterPackResponse(mixed);
		expect(got).toHaveLength(2);
		expect(got.map((d) => d.title)).toEqual(['Charter', 'Risks']);
	});

	it('ignores unknown fields per entry (forward-compat)', () => {
		const extra = JSON.stringify([
			{ title: 'Charter', body: 'b', extraField: 'ignored', model: 'opus-4.7' }
		]);
		expect(parseStarterPackResponse(extra)[0].title).toBe('Charter');
	});

	it('keeps the folder override when supplied per-entry', () => {
		const withFolder = JSON.stringify([
			{ title: 'Charter', body: 'b', folder: 'Specs/CharterBox' }
		]);
		expect(parseStarterPackResponse(withFolder)[0].folder).toBe('Specs/CharterBox');
	});

	it('drops the folder field when it is not a non-empty string', () => {
		// Defensive — model occasionally emits null or empty.
		const bad = JSON.stringify([
			{ title: 'A', body: 'b', folder: '' },
			{ title: 'B', body: 'b', folder: null }
		]);
		const got = parseStarterPackResponse(bad);
		expect(got).toHaveLength(2);
		expect(got[0].folder).toBeUndefined();
		expect(got[1].folder).toBeUndefined();
	});

	it('returns [] on completely malformed JSON', () => {
		expect(parseStarterPackResponse('not json at all')).toEqual([]);
		expect(parseStarterPackResponse('')).toEqual([]);
		expect(parseStarterPackResponse('[not closed')).toEqual([]);
	});

	it('returns [] when the top-level shape is an object, not an array', () => {
		expect(parseStarterPackResponse('{"docs":[]}')).toEqual([]);
	});

	it('handles whitespace-only bodies as invalid', () => {
		const bad = JSON.stringify([{ title: 'X', body: '   \n\t' }]);
		expect(parseStarterPackResponse(bad)).toEqual([]);
	});
});

describe('defaultStarterPath', () => {
	it('lands docs under Projects/<safe-name>/<safe-title>.md', () => {
		const got = defaultStarterPath('Granite', { title: 'Charter', body: '' });
		expect(got).toBe('Projects/Granite/Charter.md');
	});

	it('slugifies titles with punctuation safely', () => {
		const got = defaultStarterPath('Granite', {
			title: 'Risks: known unknowns!',
			body: ''
		});
		expect(got).toBe('Projects/Granite/Risks-known-unknowns.md');
	});

	it('slugifies the project name too', () => {
		const got = defaultStarterPath('My App: v2!', { title: 'Charter', body: '' });
		// "My App: v2!" → "My-App-v2" via safe()
		expect(got).toBe('Projects/My-App-v2/Charter.md');
	});

	it('honors a per-doc folder override', () => {
		const got = defaultStarterPath('Granite', {
			title: 'Charter',
			body: '',
			folder: 'Specs'
		});
		expect(got).toBe('Specs/Charter.md');
	});

	it('strips trailing slash from a supplied folder', () => {
		const got = defaultStarterPath('Granite', {
			title: 'Charter',
			body: '',
			folder: 'Specs/Project/'
		});
		expect(got).toBe('Specs/Project/Charter.md');
	});

	it('falls back to "untitled" when title slugifies to empty', () => {
		const got = defaultStarterPath('Granite', { title: '!!!', body: '' });
		expect(got).toBe('Projects/Granite/untitled.md');
	});

	it('caps overly long titles at 60 chars', () => {
		const got = defaultStarterPath('Granite', {
			title: 'a'.repeat(120),
			body: ''
		});
		// safe() caps at 60, then ".md" is appended.
		expect(got).toMatch(/^Projects\/Granite\/a{60}\.md$/);
	});
});
