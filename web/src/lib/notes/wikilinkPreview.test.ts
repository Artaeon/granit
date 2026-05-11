import { describe, expect, it } from 'vitest';
import {
	stripFrontmatter,
	extractFirstParagraph,
	stripInlineMarkdown
} from './wikilinkPreview';

describe('stripFrontmatter', () => {
	it('removes a well-formed yaml frontmatter block', () => {
		const got = stripFrontmatter(
			'---\ntitle: Foo\ntype: note\n---\nFirst paragraph.\n\nMore prose.'
		);
		expect(got).toBe('First paragraph.\n\nMore prose.');
	});

	it('leaves the body unchanged when no frontmatter present', () => {
		const got = stripFrontmatter('No leading triple-dash.\n\nJust prose.');
		expect(got).toBe('No leading triple-dash.\n\nJust prose.');
	});

	it('returns body unchanged on malformed (open-ended) frontmatter', () => {
		// A leading "---" without a closing "---" must not consume the
		// whole note. Surface the body verbatim so the user sees
		// SOMETHING in the preview.
		const got = stripFrontmatter('---\ntitle: unfinished\n\nLooks like prose now.');
		expect(got).toBe('---\ntitle: unfinished\n\nLooks like prose now.');
	});

	it('handles empty body', () => {
		expect(stripFrontmatter('')).toBe('');
	});
});

describe('extractFirstParagraph', () => {
	it('skips the leading H1 and returns the first prose paragraph', () => {
		const body = '# Title\n\nFirst paragraph here.\n\nSecond paragraph.';
		expect(extractFirstParagraph(body)).toBe('First paragraph here.');
	});

	it('skips frontmatter', () => {
		const body =
			'---\ntype: note\n---\n# Heading\n\nThe actual content.\n\nMore.';
		expect(extractFirstParagraph(body)).toBe('The actual content.');
	});

	it('skips deeper headings without ending the paragraph search', () => {
		// "## Foo" should not be returned as the preview — keep looking.
		const body = '## Section A\n\nReal content under section.';
		expect(extractFirstParagraph(body)).toBe('Real content under section.');
	});

	it('returns empty string when the note is only headings + code', () => {
		const body = '# Just a title\n\n```\nprint(1)\n```\n';
		expect(extractFirstParagraph(body)).toBe('');
	});

	it('skips list markers as the lead line but keeps in-paragraph content', () => {
		// First content line is a list — that's not "the gist of this
		// note", so we skip it and look for prose.
		const body = '- a bullet\n- another\n\nThis is the real lead.';
		expect(extractFirstParagraph(body)).toBe('This is the real lead.');
	});

	it('skips empty lines until a paragraph starts', () => {
		const body = '\n\n\n\nFirst real line.\n\nNext para.';
		expect(extractFirstParagraph(body)).toBe('First real line.');
	});

	it('collapses multi-line paragraphs into one space-joined string', () => {
		const body = '# Title\n\nLine one\nline two\nline three.\n\nNext paragraph.';
		expect(extractFirstParagraph(body)).toBe('Line one line two line three.');
	});

	it('caps long paragraphs at ~400 chars and snaps to word boundary', () => {
		const long = ('word '.repeat(200)).trim();
		const got = extractFirstParagraph('# T\n\n' + long);
		expect(got.length).toBeLessThanOrEqual(401);
		expect(got.endsWith('…') || got.length < 400).toBe(true);
		// And it never breaks a word in half.
		expect(got).not.toMatch(/wor…$/);
	});

	it('skips a fenced code block entirely', () => {
		const body = '# T\n\n```\ncode here\n```\n\nProse after the fence.';
		expect(extractFirstParagraph(body)).toBe('Prose after the fence.');
	});

	it('handles CRLF endings (Windows-host notes)', () => {
		const body = '# T\r\n\r\nFirst real paragraph.\r\n\r\nNext.';
		expect(extractFirstParagraph(body)).toBe('First real paragraph.');
	});

	it('returns empty on completely empty input', () => {
		expect(extractFirstParagraph('')).toBe('');
		expect(extractFirstParagraph('\n\n\n')).toBe('');
	});
});

describe('stripInlineMarkdown', () => {
	it('unwraps bold, italic, inline code, strikethrough', () => {
		expect(stripInlineMarkdown('**bold** and *italic* and `code` and ~~gone~~.')).toBe(
			'bold and italic and code and gone.'
		);
	});

	it('keeps the alias side of a wikilink', () => {
		expect(stripInlineMarkdown('See [[Note Title|alias]] and [[Plain]].')).toBe(
			'See alias and Plain.'
		);
	});

	it('drops image embeds entirely', () => {
		expect(stripInlineMarkdown('text ![[diagram.png]] more text')).toBe(
			'text more text'
		);
	});

	it('drops footnote refs', () => {
		expect(stripInlineMarkdown('Claim[^1] with note[^citation].')).toBe(
			'Claim with note.'
		);
	});

	it('unwraps markdown links to just the link text', () => {
		expect(stripInlineMarkdown('See [docs](https://example.com).')).toBe('See docs.');
	});

	it('collapses runs of whitespace left by removals', () => {
		expect(stripInlineMarkdown('a    b\n\nc')).toBe('a b c');
	});

	it('returns input verbatim when no markdown to strip', () => {
		expect(stripInlineMarkdown('plain prose.')).toBe('plain prose.');
	});
});
