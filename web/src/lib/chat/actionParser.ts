// Action / follow-up parsers used by the AI chat overlay.
//
// Lifted out of AIOverlay.svelte so the parsing rules are testable
// independently of the 2000-line component. The parsers run on every
// streamed-in assistant chunk's final form; a malformed block must
// produce no chip rather than crash the render.
//
// The AGENT_CAPABILITIES_SYSTEM prompt (in AIOverlay.svelte) tells the
// model exactly which two channels exist:
//   1. <followups>line1\nline2\n…</followups> — short prompts to chip
//   2. ```granit-action {"type":"task|event|note|remember", …} ```
//      — vault actions to commit
//
// stripStructuredBlocks removes both from the rendered markdown so
// the user sees a clean reply with chips below.

/** task action — text required; everything else optional. */
export interface ActionTask {
	type: 'task';
	text: string;
	dueDate?: string;
	priority?: number;
	notePath?: string;
}
/** calendar event — title+start required; end/location optional. */
export interface ActionEvent {
	type: 'event';
	title: string;
	start: string;
	end?: string;
	location?: string;
}
/** new note — title+body required; folder optional. */
export interface ActionNote {
	type: 'note';
	title: string;
	body: string;
	folder?: string;
}
/** long-term memory fact — content required; tags optional. */
export interface ActionRemember {
	type: 'remember';
	content: string;
	tags?: string[];
}

export type ParsedAction = ActionTask | ActionEvent | ActionNote | ActionRemember;

const FOLLOWUPS_RE = /<followups>([\s\S]*?)<\/followups>/i;
const ACTION_FENCE_RE = /```granit-action\s*\n([\s\S]*?)```/g;

/** parseFollowups extracts the up-to-3 follow-up prompts from a single
 *  <followups>…</followups> block. Returns [] when the block is absent
 *  or empty. Each line is trimmed; leading list markers ("- ", "* ")
 *  are stripped so models that emit bulleted lists still parse cleanly.
 *  Caps individual prompts at 200 chars to defend against the model
 *  going off-script with a paragraph instead of a prompt. */
export function parseFollowups(content: string): string[] {
	const m = content.match(FOLLOWUPS_RE);
	if (!m) return [];
	return m[1]
		.split(/\r?\n/)
		.map((l) => l.replace(/^[-*]\s+/, '').trim())
		.filter((l) => l.length > 0 && l.length < 200)
		.slice(0, 3);
}

/** parseActions extracts every well-formed granit-action JSON block.
 *  Malformed JSON, missing required fields, or unknown types drop
 *  out of the returned array — the assistant occasionally streams a
 *  half-token block before completing it, and we render conservatively
 *  rather than flashing error chips that disappear once the stream
 *  finishes. The fence regex already requires a CLOSING ``` though,
 *  so anything that gets matched here is structurally complete; a
 *  parse / validation failure at that point is a real model-output
 *  spec violation and gets a one-line warning so the action drop is
 *  traceable. */
export function parseActions(content: string): ParsedAction[] {
	const out: ParsedAction[] = [];
	// New RegExp each call — global lastIndex isn't safe to share.
	const re = new RegExp(ACTION_FENCE_RE.source, ACTION_FENCE_RE.flags);
	let m: RegExpExecArray | null;
	while ((m = re.exec(content)) !== null) {
		const body = m[1].trim();
		let obj: unknown;
		try {
			obj = JSON.parse(body);
		} catch (err) {
			console.warn('[granit] dropped malformed granit-action JSON:', err instanceof Error ? err.message : err, '\n', body.slice(0, 240));
			continue;
		}
		const action = validateAction(obj);
		if (action) {
			out.push(action);
		} else {
			console.warn('[granit] dropped granit-action with unknown type or missing required fields:\n', body.slice(0, 240));
		}
	}
	return out;
}

/** stripStructuredBlocks removes the follow-ups block and every action
 *  fence from a message so the rendered markdown reads as prose. The
 *  blocks live elsewhere in the UI (chips below the message body). */
export function stripStructuredBlocks(content: string): string {
	let out = content.replace(/<followups>[\s\S]*?<\/followups>/gi, '');
	out = out.replace(/```granit-action\s*\n[\s\S]*?```/g, '');
	// Collapse the trailing whitespace the strip can leave behind.
	return out.replace(/\n{3,}/g, '\n\n').trimEnd();
}

/** actionKey produces a stable identifier for an action so a UI
 *  caller can dedupe commits across regens. The signature blends the
 *  action's identifying fields with the message index so two messages
 *  proposing the same task get separate keys (intentional — the user
 *  may want to commit both). */
export function actionKey(msgIdx: number, a: ParsedAction): string {
	const sig =
		a.type === 'task'
			? `${a.text}|${a.dueDate ?? ''}`
			: a.type === 'event'
				? `${a.title}|${a.start}`
				: a.type === 'note'
					? `${a.title}|${a.folder ?? ''}`
					: a.content;
	return `${msgIdx}:${a.type}:${sig}`;
}

// validateAction is the gate between "untrusted JSON from the model"
// and "typed ParsedAction we let through to the UI". Per-type field
// presence checks; rejects anything else. Returns null on invalid so
// the caller can simply skip the entry without an exception.
function validateAction(obj: unknown): ParsedAction | null {
	if (!obj || typeof obj !== 'object') return null;
	const o = obj as Record<string, unknown>;
	const type = o.type;
	if (typeof type !== 'string') return null;
	if (type === 'task') {
		if (typeof o.text !== 'string' || !o.text.trim()) return null;
		return {
			type: 'task',
			text: o.text,
			dueDate: typeof o.dueDate === 'string' ? o.dueDate : undefined,
			priority: typeof o.priority === 'number' ? o.priority : undefined,
			notePath: typeof o.notePath === 'string' ? o.notePath : undefined
		};
	}
	if (type === 'event') {
		if (typeof o.title !== 'string' || !o.title) return null;
		if (typeof o.start !== 'string' || !o.start) return null;
		return {
			type: 'event',
			title: o.title,
			start: o.start,
			end: typeof o.end === 'string' ? o.end : undefined,
			location: typeof o.location === 'string' ? o.location : undefined
		};
	}
	if (type === 'note') {
		if (typeof o.title !== 'string' || !o.title) return null;
		if (typeof o.body !== 'string') return null;
		return {
			type: 'note',
			title: o.title,
			body: o.body,
			folder: typeof o.folder === 'string' ? o.folder : undefined
		};
	}
	if (type === 'remember') {
		if (typeof o.content !== 'string' || !o.content.trim()) return null;
		const tags = Array.isArray(o.tags)
			? o.tags.filter((t): t is string => typeof t === 'string')
			: undefined;
		return { type: 'remember', content: o.content, tags };
	}
	return null;
}
