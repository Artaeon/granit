<!--
  ProjectStarterPack — small AI-driven dialog that generates a set
  of starter documents for a project (charter / milestones / risks
  / kickoff agenda). Each doc renders as a card; user reviews,
  saves individually or all at once, dismisses if nothing fits.

  Pure logic (prompt builder + response parser + path defaulting)
  lives in $lib/projects/starterPack.ts and is pinned by 21 tests.
  This component is the streaming-fetch + dialog UX shell.

  Reuses the existing audit-gated chatStream pipeline — no new
  backend feature flag to enable.
-->
<script lang="ts">
	import { api } from '$lib/api';
	import { toast } from '$lib/components/toast';
	import { errorMessage } from '$lib/util/errorMessage';
	import {
		buildStarterPackPrompt,
		parseStarterPackResponse,
		defaultStarterPath,
		type StarterDoc,
		type StarterPackProjectInput,
		type StarterPackRepoContext
	} from './starterPack';
	import { loadStoredString, saveStoredString } from '$lib/util/storage';

	interface Props {
		project: StarterPackProjectInput;
	}
	let { project }: Props = $props();

	type ProposalRow = StarterDoc & { committed?: boolean; savedPath?: string };
	let open = $state(false);
	let busy = $state(false);
	let abort: AbortController | null = null;
	let proposals = $state<ProposalRow[]>([]);
	let raw = $state(''); // surfaced when parse fails so the user can recover
	let parseError = $state(false);
	let savingAll = $state(false);

	// Optional repo scan. The user pastes a local path; we hit the
	// /reposcan endpoint, surface what was found, and pass it as
	// grounding context to the AI on the next Generate. Persisted
	// per-project in localStorage so the user doesn't re-type the
	// path on every visit.
	//
	// The storage key is rebuilt at every read/write rather than
	// captured once. SvelteKit's same-route navigation changes the
	// `project` prop without remounting — a `const KEY = ...` would
	// have stayed pinned to the first project's name, so project B's
	// repo path would land in project A's localStorage slot.
	function repoStorageKey(): string {
		return `granit.starterpack.repo.${project.name}`;
	}
	let repoPath = $state(loadStoredString(repoStorageKey(), ''));
	let repoBusy = $state(false);
	let repoError = $state('');
	let repoContext = $state<StarterPackRepoContext | null>(null);
	// Reload the saved path when the project changes so the input
	// reflects THIS project's history, not the previous one's.
	// lastProjectName is initialised lazily inside the effect (the
	// first run snapshots it, subsequent runs compare). Doing it
	// this way keeps Svelte's compiler from warning about a "state
	// reference only captures initial value" — we genuinely want the
	// frozen snapshot here.
	let lastProjectName: string | null = null;
	$effect(() => {
		if (lastProjectName === null) {
			lastProjectName = project.name;
			return;
		}
		if (project.name !== lastProjectName) {
			lastProjectName = project.name;
			repoPath = loadStoredString(repoStorageKey(), '');
			repoContext = null;
		}
	});
	$effect(() => saveStoredString(repoStorageKey(), repoPath));

	async function scanRepo() {
		const p = repoPath.trim();
		if (!p) {
			repoError = 'enter a path';
			return;
		}
		repoBusy = true;
		repoError = '';
		repoContext = null;
		try {
			const r = await api.scanRepo(p);
			repoContext = {
				name: r.name,
				branch: r.branch,
				readmeName: r.readmeName,
				readmeContent: r.readmeContent,
				manifest: r.manifest,
				manifestContent: r.manifestContent,
				fileTree: r.fileTree,
				recentCommits: r.recentCommits
			};
			toast.success(
				`Scanned ${r.name}${r.isGit ? '' : ' (no .git)'} · ` +
					[
						r.readmeName ? r.readmeName : null,
						r.manifest ? r.manifest : null,
						r.recentCommits && r.recentCommits.length > 0
							? `${r.recentCommits.length} commits`
							: null
					]
						.filter(Boolean)
						.join(' · ')
			);
		} catch (err) {
			repoError = errorMessage(err);
		} finally {
			repoBusy = false;
		}
	}
	function clearRepoContext() {
		repoContext = null;
	}

	async function generate() {
		if (busy) return;
		open = true;
		proposals = [];
		raw = '';
		parseError = false;
		busy = true;
		abort?.abort();
		abort = new AbortController();
		const { system, user } = buildStarterPackPrompt(project, repoContext ?? undefined);
		let buf = '';
		try {
			await api.chatStream(
				[
					{ role: 'system', content: system },
					{ role: 'user', content: user }
				],
				undefined,
				{
					onChunk: (c) => {
						buf += c;
					},
					onDone: () => {
						raw = buf;
						const parsed = parseStarterPackResponse(buf);
						if (parsed.length === 0) {
							parseError = true;
							return;
						}
						proposals = parsed.map((d) => ({ ...d }));
					},
					onError: (err) => toast.error('Starter pack failed: ' + err.message)
				},
				abort.signal
			);
		} finally {
			busy = false;
			abort = null;
		}
	}

	async function saveOne(idx: number) {
		const p = proposals[idx];
		if (!p || p.committed || busy) return;
		const path = defaultStarterPath(project.name, p);
		try {
			await api.createNote({
				path,
				frontmatter: {
					title: p.title,
					project: project.name,
					type: 'project-doc'
				},
				body: p.body
			});
			proposals = proposals.map((x, i) =>
				i === idx ? { ...x, committed: true, savedPath: path } : x
			);
			toast.success(`Saved · ${path}`, {
				action: { label: 'Open', href: `/notes/${encodeURIComponent(path)}` }
			});
		} catch (err) {
			toast.error(`Save failed: ${errorMessage(err)}`);
		}
	}

	async function saveAll() {
		if (savingAll) return;
		savingAll = true;
		try {
			for (let i = 0; i < proposals.length; i++) {
				if (!proposals[i].committed) await saveOne(i);
			}
		} finally {
			savingAll = false;
		}
	}

	function cancel() {
		abort?.abort();
	}
	function dismiss() {
		cancel();
		open = false;
		proposals = [];
		raw = '';
		parseError = false;
	}
</script>

<div class="my-2 space-y-2">
	<!-- Optional repo scan. When the project lives in a local git
		 repo, the user can paste the path and pull README + manifest
		 + recent commits as grounding context for the AI. The next
		 Generate call passes this content to the prompt; without it,
		 the starter pack falls back to the project's description /
		 task list as before. -->
	<div class="flex items-stretch gap-1.5">
		<input
			type="text"
			bind:value={repoPath}
			placeholder="Optional: local git repo path (e.g. ~/Projects/granit)"
			class="flex-1 text-xs px-2 py-1.5 rounded bg-surface0 border border-surface1 text-text placeholder:text-dim focus:outline-none focus:border-primary"
			autocomplete="off"
			spellcheck="false"
		/>
		<button
			type="button"
			onclick={scanRepo}
			disabled={repoBusy || !repoPath.trim()}
			class="text-xs px-2.5 py-1.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary disabled:opacity-50"
			title="Read README + manifest + recent commits from this local repo for grounding"
		>{repoBusy ? 'Scanning…' : 'Scan repo'}</button>
	</div>
	{#if repoError}
		<p class="text-[11px] text-error">{repoError}</p>
	{/if}
	{#if repoContext}
		<div class="text-[11px] text-dim bg-surface0 border border-surface1 rounded px-2 py-1.5 flex items-baseline gap-2">
			<span class="text-success">✓</span>
			<span class="flex-1 truncate" title={repoContext.name}>
				Scanned <strong class="text-text">{repoContext.name}</strong>
				{#if repoContext.branch} · <span class="font-mono">{repoContext.branch}</span>{/if}
				{#if repoContext.readmeName} · {repoContext.readmeName}{/if}
				{#if repoContext.manifest} · {repoContext.manifest}{/if}
				{#if repoContext.recentCommits && repoContext.recentCommits.length > 0} · {repoContext.recentCommits.length} commits{/if}
			</span>
			<button type="button" onclick={clearRepoContext} class="text-dim hover:text-text" title="Drop this scan — next Generate runs without repo context">clear</button>
		</div>
	{/if}
	<div>
		<button
			type="button"
			onclick={generate}
			disabled={busy}
			class="text-xs px-3 py-1.5 rounded bg-surface0 border border-surface1 text-text hover:border-primary disabled:opacity-50 inline-flex items-center gap-1.5"
			title={repoContext
				? 'Generate documents grounded in the scanned repo'
				: 'Generate a starter pack of project documents with AI'}
		>
			<svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
				<path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5z"/>
				<path d="M5 21h14"/>
			</svg>
			{busy ? 'Generating…' : repoContext ? 'Starter pack (with repo)' : 'Starter pack'}
		</button>
	</div>
</div>

{#if open}
	<div class="mt-2 border border-surface1 rounded-md bg-base p-3">
		<header class="flex items-baseline gap-2 mb-2">
			<h3 class="text-sm font-medium text-text flex-1">Starter pack</h3>
			{#if busy}
				<button type="button" onclick={cancel} class="text-[11px] text-warning hover:underline">cancel</button>
			{:else if proposals.length > 0}
				<button
					type="button"
					onclick={generate}
					disabled={busy}
					class="text-[11px] text-secondary hover:underline">regen</button>
				<button
					type="button"
					onclick={saveAll}
					disabled={savingAll || proposals.every((p) => p.committed)}
					class="text-[11px] text-secondary hover:underline">
					{savingAll ? 'saving…' : 'save all'}
				</button>
			{/if}
			<button type="button" onclick={dismiss} class="text-[11px] text-dim hover:text-text">dismiss</button>
		</header>

		{#if busy && proposals.length === 0}
			<p class="text-xs text-dim italic">AI is drafting your starter docs… this usually takes 10–20s.</p>
		{:else if parseError}
			<p class="text-xs text-error mb-2">
				Model returned an unexpected shape. You can regen or copy the raw output below.
			</p>
			<pre class="text-[11px] text-dim bg-surface0 p-2 rounded max-h-48 overflow-auto whitespace-pre-wrap">{raw}</pre>
		{:else if proposals.length > 0}
			<ul class="space-y-2">
				{#each proposals as p, i (p.title + i)}
					<li class="border border-surface1 rounded p-2 bg-surface0">
						<div class="flex items-baseline gap-2 mb-1">
							<button
								type="button"
								onclick={() => saveOne(i)}
								disabled={!!p.committed}
								class="tap-target inline-flex items-center justify-center w-6 h-6 rounded text-[12px] font-medium {p.committed
									? 'bg-surface0 text-success cursor-default'
									: 'bg-surface1 hover:bg-surface2 text-text'}"
								aria-label={p.committed ? 'Saved' : `Save ${p.title}`}
								title={p.committed ? `Saved at ${p.savedPath}` : 'Save this document'}
							>{p.committed ? '✓' : '+'}</button>
							<span class="text-sm font-medium text-text flex-1 truncate" title={p.title}>{p.title}</span>
							{#if p.committed && p.savedPath}
								<a href={`/notes/${encodeURIComponent(p.savedPath)}`} class="text-[10px] text-secondary hover:underline">open ↗</a>
							{:else}
								<span class="text-[10px] text-dim font-mono truncate max-w-[14rem]" title={defaultStarterPath(project.name, p)}>
									{defaultStarterPath(project.name, p)}
								</span>
							{/if}
						</div>
						<details class="text-[11px]">
							<summary class="cursor-pointer text-dim hover:text-text">preview</summary>
							<pre class="mt-1 text-[11px] text-subtext bg-base p-2 rounded max-h-64 overflow-auto whitespace-pre-wrap">{p.body}</pre>
						</details>
					</li>
				{/each}
			</ul>
			<p class="text-[10px] text-dim mt-2 leading-snug">
				Each doc saves under <code class="text-text">Projects/{project.name}/</code> with frontmatter linking back to this project.
			</p>
		{/if}
	</div>
{/if}
