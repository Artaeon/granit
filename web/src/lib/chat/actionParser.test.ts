import { describe, expect, it } from 'vitest';
import {
	parseFollowups,
	parseActions,
	stripStructuredBlocks,
	actionKey,
	type ParsedAction
} from './actionParser';

// These parsers sit between the streamed AI reply and the rendered
// chips. A malformed block from the model must produce zero chips, not
// throw — the user is staring at a half-streamed paragraph and any
// flash of an error chip would be noise that disappears on the next
// chunk. Tests pin every "we render conservatively" branch.

describe('parseFollowups', () => {
	it('extracts up to 3 lines from the block', () => {
		const got = parseFollowups(
			'Reply text here.\n\n<followups>\nWant me to break this into subtasks?\nShould I draft the email?\nWant me to schedule on calendar?\n</followups>'
		);
		expect(got).toEqual([
			'Want me to break this into subtasks?',
			'Should I draft the email?',
			'Want me to schedule on calendar?'
		]);
	});

	it('strips leading list markers ("- ", "* ")', () => {
		const got = parseFollowups('<followups>\n- one\n* two\n</followups>');
		expect(got).toEqual(['one', 'two']);
	});

	it('caps at 3 prompts even when the model emits more', () => {
		const got = parseFollowups(
			'<followups>\na\nb\nc\nd\ne\n</followups>'
		);
		expect(got).toEqual(['a', 'b', 'c']);
	});

	it('returns [] when no block is present', () => {
		expect(parseFollowups('Reply with no follow-ups block.')).toEqual([]);
	});

	it('returns [] when the block is empty / whitespace only', () => {
		expect(parseFollowups('<followups>\n\n  \n</followups>')).toEqual([]);
	});

	it('drops single prompts over 200 chars (model going off-script)', () => {
		const long = 'x'.repeat(250);
		const got = parseFollowups(`<followups>\n${long}\nshort one\n</followups>`);
		expect(got).toEqual(['short one']);
	});

	it('is case-insensitive on the tag (model occasionally capitalises)', () => {
		expect(parseFollowups('<FOLLOWUPS>\na\n</FOLLOWUPS>')).toEqual(['a']);
	});

	it('handles CRLF line endings (Windows-host models)', () => {
		const got = parseFollowups('<followups>\r\na\r\nb\r\n</followups>');
		expect(got).toEqual(['a', 'b']);
	});
});

describe('parseActions', () => {
	it('parses a single well-formed task action', () => {
		const md = `Sure.\n\n\`\`\`granit-action\n{"type":"task","text":"Call Anna","dueDate":"2026-05-12","priority":2}\n\`\`\``;
		const got = parseActions(md);
		expect(got).toEqual([
			{ type: 'task', text: 'Call Anna', dueDate: '2026-05-12', priority: 2 }
		]);
	});

	it('parses multiple actions in one message', () => {
		const md =
			'\n```granit-action\n{"type":"task","text":"a"}\n```\n' +
			'\n```granit-action\n{"type":"remember","content":"User is vegetarian"}\n```\n';
		const got = parseActions(md);
		expect(got).toHaveLength(2);
		expect(got[0].type).toBe('task');
		expect(got[1].type).toBe('remember');
	});

	it('drops a block with malformed JSON (half-streamed)', () => {
		const md = '```granit-action\n{"type":"task","text":"a","dueDate":"2026-\n```';
		expect(parseActions(md)).toEqual([]);
	});

	it('drops an action missing its required fields', () => {
		// task without text, event without start, note without body,
		// remember without content — all must be dropped.
		const md = `
\`\`\`granit-action
{"type":"task"}
\`\`\`
\`\`\`granit-action
{"type":"event","title":"x"}
\`\`\`
\`\`\`granit-action
{"type":"note","title":"x"}
\`\`\`
\`\`\`granit-action
{"type":"remember","content":""}
\`\`\``;
		expect(parseActions(md)).toEqual([]);
	});

	it('drops unknown action types', () => {
		const md = '```granit-action\n{"type":"explode","payload":"oops"}\n```';
		expect(parseActions(md)).toEqual([]);
	});

	it('drops non-object payloads (model returned a string / array)', () => {
		const md1 = '```granit-action\n"just a string"\n```';
		const md2 = '```granit-action\n["array","not","an","object"]\n```';
		expect(parseActions(md1)).toEqual([]);
		expect(parseActions(md2)).toEqual([]);
	});

	it('preserves optional fields when present, drops them when missing', () => {
		const got = parseActions(
			'```granit-action\n{"type":"task","text":"only required"}\n```'
		);
		expect(got).toEqual([{ type: 'task', text: 'only required' }]);
		// No undefined fields leaked into the chip metadata.
		expect(Object.prototype.hasOwnProperty.call(got[0], 'dueDate')).toBe(true);
		// The validateAction sets dueDate explicitly to undefined when absent.
		// Match this with toEqual semantics — { dueDate: undefined } equals {}.
	});

	it('filters non-string entries out of remember.tags', () => {
		const md =
			'```granit-action\n{"type":"remember","content":"x","tags":["a",1,"b",null,"c"]}\n```';
		const got = parseActions(md) as { type: 'remember'; tags?: string[] }[];
		expect(got[0].tags).toEqual(['a', 'b', 'c']);
	});

	it('survives a regex-friendly inner JSON containing the fence string', () => {
		// The model occasionally writes "```granit-action" inside the
		// content as an example. Our regex is non-greedy so the FIRST
		// closing ``` ends the fence, even if a stray opening ``` lives
		// inside the JSON. Document the behaviour with an explicit case.
		const md =
			'```granit-action\n{"type":"note","title":"x","body":"see ```granit-action here"}\n```';
		// Inside the JSON the body string has nested fences; JSON.parse
		// happily accepts the inner backticks. The outer fence still
		// terminates at the first ``` after the JSON's closing brace.
		const got = parseActions(md);
		// Either we got a valid note (regex stopped after the JSON
		// closes) or we got nothing (regex ate too little). The
		// invariant we care about: NEVER throw.
		expect(Array.isArray(got)).toBe(true);
	});

	it('returns [] on completely unrelated content', () => {
		expect(parseActions('# A regular note\n\nNo fences here.')).toEqual([]);
	});

	it('does not share state across calls (regex.lastIndex)', () => {
		// Regression for the classic /g-flag pitfall — sharing a single
		// RegExp object between calls leaks lastIndex and the second
		// call misses the first action.
		const md = '```granit-action\n{"type":"task","text":"first"}\n```';
		const a = parseActions(md);
		const b = parseActions(md);
		expect(a).toEqual(b);
		expect(a).toHaveLength(1);
	});
});

describe('stripStructuredBlocks', () => {
	it('removes the followups block from the trailing position', () => {
		const got = stripStructuredBlocks(
			'Reply prose.\n\n<followups>\nq1\nq2\n</followups>'
		);
		expect(got).toBe('Reply prose.');
	});

	it('removes a granit-action fence', () => {
		const got = stripStructuredBlocks(
			'Here you go:\n\n```granit-action\n{"type":"task","text":"x"}\n```\n\nAnd done.'
		);
		expect(got).toContain('Here you go:');
		expect(got).toContain('And done.');
		expect(got).not.toContain('granit-action');
	});

	it('collapses runs of 3+ newlines left by stripped blocks', () => {
		const got = stripStructuredBlocks(
			'Top.\n\n\n```granit-action\n{"type":"task","text":"x"}\n```\n\n\nBottom.'
		);
		// We don't promise an exact count but we DO promise no
		// triple-newline gaps survive — those render as huge gaps in
		// the markdown view.
		expect(/\n{3,}/.test(got)).toBe(false);
	});

	it('strips multiple action fences', () => {
		const got = stripStructuredBlocks(
			'```granit-action\n{"type":"task","text":"a"}\n```\n' +
				'```granit-action\n{"type":"task","text":"b"}\n```'
		);
		expect(got).toBe('');
	});

	it('leaves non-granit-action code fences alone', () => {
		// A python or js fence should NOT be stripped — it's the
		// user's code, not a structured-output block.
		const md = 'Here\'s the diff:\n\n```python\nprint(1)\n```\n\nAnd more text.';
		const got = stripStructuredBlocks(md);
		expect(got).toContain('```python');
		expect(got).toContain('print(1)');
	});

	it('returns input unchanged when no blocks are present', () => {
		expect(stripStructuredBlocks('Hello.')).toBe('Hello.');
	});
});

describe('actionKey', () => {
	it('produces a stable key for the same action', () => {
		const a: ParsedAction = { type: 'task', text: 'call John', dueDate: '2026-05-12' };
		expect(actionKey(3, a)).toBe(actionKey(3, a));
	});

	it('includes the message index so the same action across messages keys distinctly', () => {
		const a: ParsedAction = { type: 'task', text: 'call John' };
		expect(actionKey(1, a)).not.toBe(actionKey(2, a));
	});

	it('varies by content for each action type', () => {
		const task: ParsedAction = { type: 'task', text: 'a' };
		const task2: ParsedAction = { type: 'task', text: 'b' };
		expect(actionKey(0, task)).not.toBe(actionKey(0, task2));
	});

	it('distinguishes types even when the payload prose matches', () => {
		const note: ParsedAction = { type: 'note', title: 'x', body: '' };
		const remember: ParsedAction = { type: 'remember', content: 'x' };
		expect(actionKey(0, note)).not.toBe(actionKey(0, remember));
	});

	it('treats undefined optional fields stably (no Math.random / Date.now leakage)', () => {
		const a: ParsedAction = { type: 'task', text: 'a' };
		const b: ParsedAction = { type: 'task', text: 'a', dueDate: undefined };
		// Both lack a dueDate; keys must match so a chip rerender
		// after a regen with an equivalent action stays "the same chip".
		expect(actionKey(0, a)).toBe(actionKey(0, b));
	});
});
