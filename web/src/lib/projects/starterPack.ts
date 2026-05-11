// Project starter-pack generator — pure prompt builder + response
// parser. The Svelte component (ProjectStarterPack.svelte) handles
// the streaming fetch, dialog UX, and per-doc save dispatch; the
// logic-y bits live here so they can be tested without rendering.
//
// The "starter pack" is a small set of project-bootstrap documents
// (charter / milestones / risk register / kickoff agenda). One AI
// call generates all of them as a structured JSON array; the user
// reviews the cards and saves each individually or all at once.
// Cheaper than four separate prompts because the model can keep
// internal consistency across the docs in a single response.

/** StarterDoc — one proposed document the AI returned. Title +
 *  body are required; folder defaults to Projects/<name>/ when not
 *  set. */
export interface StarterDoc {
	title: string;
	body: string;
	folder?: string;
}

/** Minimal project shape this module needs — title + a few optional
 *  hints used to seed the AI. We don't import the full Project type
 *  from $lib/api so this module stays test-time-friendly (no SDK
 *  imports). */
export interface StarterPackProjectInput {
	name: string;
	description?: string;
	kind?: string;
	venture?: string;
	tags?: string[];
	next_action?: string;
}

/** Optional grounding context from a local git repo scan — the
 *  reposcan handler produces this shape. When the user has linked
 *  a project to a repo on disk, we fold the README / manifest /
 *  recent commits into the user prompt so the AI grounds its
 *  starter docs in the actual codebase instead of generic
 *  templates. Every field is optional — a partial scan (e.g. no
 *  manifest detected) still helps.
 *
 *  Per-field byte caps live on the backend (reposcan.Context); by
 *  the time it reaches this module the content is already bounded,
 *  so the prompt builder doesn't re-clamp. */
export interface StarterPackRepoContext {
	name?: string;
	branch?: string;
	readmeName?: string;
	readmeContent?: string;
	manifest?: string;
	manifestContent?: string;
	fileTree?: string[];
	recentCommits?: string[];
}

/** buildStarterPackPrompt assembles the system + user pair the
 *  chatStream call submits. Pure — same inputs always produce the
 *  same prompt, easy to test for regressions ("the model gets a
 *  reasonable set of fields, in a stable order, and is told to
 *  return strict JSON"). */
export function buildStarterPackPrompt(
	project: StarterPackProjectInput,
	repo?: StarterPackRepoContext
): {
	system: string;
	user: string;
} {
	const system = `You generate a starter pack of bootstrapping documents for a project the user is just kicking off in Granit.

Return STRICTLY a JSON array — no prose, no fences, no preamble. Shape:
[
  {"title": "<3-6 words>", "body": "<markdown body, 60-200 words>"},
  ...
]

Generate exactly these four documents in this order:

1. **Charter** — one-page project charter. Sections: ## Why · ## Scope · ## Out of scope · ## Definition of done · ## Stakeholders. Each section: 1-3 short bullets. Keep grounded in the inputs — do NOT invent stakeholders or deadlines that aren't in the brief.

2. **Milestones** — 3-5 outcome-oriented milestones (NOT tasks). Sections: ## Milestones, with each milestone as a bullet "**M1 — <name>** · <one-sentence outcome statement>". No dates unless the user supplied a target_date.

3. **Risks** — risk register. Sections: ## Risks, each risk as "- **<short risk>** — likelihood · impact · one-line mitigation". 3-5 risks. Pick risks that match the project's domain; skip generic "scope creep" / "team burnout" unless they actually fit.

4. **Kickoff agenda** — 30-minute kickoff meeting agenda. Sections: ## Agenda with timed bullets ("0-5min: Welcome + context", etc.). Cover: shared context, charter walkthrough, risks, next concrete action, time for questions.

Hard rules:
- No "synergy", "leverage", "robust", "best-in-class", "stakeholders aligning", "drive value".
- No invented people, companies, dates, or technologies. Use what's in the brief.
- Bodies use plain markdown headings + bullets; no horizontal rules; no frontmatter.
- Bodies are 60-200 words each. Total response stays under 1200 words.`;

	const lines: string[] = [`Project name: ${project.name}`];
	if (project.description) lines.push(`Description: ${project.description}`);
	if (project.kind) lines.push(`Kind: ${project.kind}`);
	if (project.venture) lines.push(`Venture: ${project.venture}`);
	if (project.tags && project.tags.length > 0) lines.push(`Tags: ${project.tags.join(', ')}`);
	if (project.next_action) lines.push(`Stated next action: ${project.next_action}`);

	// When a repo scan was supplied, weave the extracted README +
	// manifest + recent commits + file tree into the user prompt
	// as a separate "Repository scan" block. The AI is told to
	// PRIORITISE this content as the source of truth (it's real
	// data) over generic templates, so the resulting charter
	// mentions the actual tech stack, the milestones reflect what's
	// already shipped (from recent commits), and the risks call out
	// real dependencies / file-tree complexity.
	if (repo && hasUsefulRepoContent(repo)) {
		const repoLines: string[] = ['', '— Repository scan (PRIORITISE these facts over generic templates) —'];
		if (repo.name) repoLines.push(`Repo: ${repo.name}` + (repo.branch ? ` (branch: ${repo.branch})` : ''));
		if (repo.readmeContent) {
			repoLines.push('', `${repo.readmeName || 'README'}:`, '```', repo.readmeContent, '```');
		}
		if (repo.manifestContent) {
			repoLines.push('', `${repo.manifest || 'manifest'}:`, '```', repo.manifestContent, '```');
		}
		if (repo.fileTree && repo.fileTree.length > 0) {
			repoLines.push('', 'Top-level file tree:', repo.fileTree.map((e) => `- ${e}`).join('\n'));
		}
		if (repo.recentCommits && repo.recentCommits.length > 0) {
			repoLines.push(
				'',
				`Recent commits (newest first, ${repo.recentCommits.length}):`,
				repo.recentCommits.map((c) => `- ${c}`).join('\n')
			);
		}
		lines.push(repoLines.join('\n'));
	}

	const user = `Generate the starter pack for this project. Return the JSON array as instructed.\n\n${lines.join('\n')}`;

	return { system, user };
}

// hasUsefulRepoContent — defensive check before we splice the repo
// block into the prompt. A scan that returned only Name + IsGit
// (no README, no manifest, no commits, no tree) adds tokens without
// information; the model is better off going off the user-supplied
// description alone. Keeps the prompt slim when the repo is
// effectively empty (a freshly-init'd repo with one .gitignore).
function hasUsefulRepoContent(repo: StarterPackRepoContext): boolean {
	if (repo.readmeContent && repo.readmeContent.trim()) return true;
	if (repo.manifestContent && repo.manifestContent.trim()) return true;
	if (repo.recentCommits && repo.recentCommits.length > 0) return true;
	if (repo.fileTree && repo.fileTree.length > 2) return true;
	return false;
}

/** parseStarterPackResponse extracts the doc array from the model's
 *  reply. Tolerant of:
 *  - leading/trailing prose ("Here you go:")
 *  - ```json fences the model adds anyway
 *  - extra fields per doc (model decides to add "summary": we just
 *    ignore unknown keys)
 *  - malformed entries inside an otherwise-valid array (drops the
 *    bad ones, keeps the good)
 *  Returns [] when nothing usable is in the response — caller shows
 *  a "model returned an unexpected shape" toast. */
export function parseStarterPackResponse(raw: string): StarterDoc[] {
	let s = raw.trim();
	// Strip ```json or ``` fences the model wraps the array in.
	if (s.startsWith('```')) {
		s = s.replace(/^```(?:json)?\s*/i, '').replace(/```\s*$/, '').trim();
	}
	// Some models prefix prose ("Here are your four documents:") then
	// the JSON. Slice from the first '[' to the matching last ']'.
	const firstBracket = s.indexOf('[');
	const lastBracket = s.lastIndexOf(']');
	if (firstBracket >= 0 && lastBracket > firstBracket) {
		s = s.slice(firstBracket, lastBracket + 1);
	}
	let parsed: unknown;
	try {
		parsed = JSON.parse(s);
	} catch {
		return [];
	}
	if (!Array.isArray(parsed)) return [];
	const out: StarterDoc[] = [];
	for (const entry of parsed) {
		if (!entry || typeof entry !== 'object') continue;
		const e = entry as Record<string, unknown>;
		if (typeof e.title !== 'string' || !e.title.trim()) continue;
		if (typeof e.body !== 'string' || !e.body.trim()) continue;
		out.push({
			title: e.title.trim(),
			body: e.body,
			folder: typeof e.folder === 'string' && e.folder.trim() ? e.folder.trim() : undefined
		});
	}
	return out;
}

/** defaultStarterPath builds the conventional save path for a
 *  starter-pack doc — Projects/<safe-project-name>/<safe-title>.md.
 *  Trims/slugifies both halves so titles like "Risks: known
 *  unknowns" land at a valid filename. */
export function defaultStarterPath(projectName: string, doc: StarterDoc): string {
	const folder = doc.folder ?? `Projects/${safe(projectName)}`;
	const fname = safe(doc.title) || 'document';
	return `${folder.replace(/\/+$/, '')}/${fname}.md`;
}

function safe(s: string): string {
	return s
		.replace(/[^\w\s-]/g, '')
		.replace(/\s+/g, '-')
		.replace(/-+/g, '-')
		.replace(/^-|-$/g, '')
		.slice(0, 60) || 'untitled';
}
