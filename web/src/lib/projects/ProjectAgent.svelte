<!--
  ProjectAgent — conversational AI surface scoped to /projects.
  Mirror of TaskAgent for the project domain. User types a free-
  text intent; the agent proposes typed actions (set_status,
  set_priority, archive, set_next_action, etc.) on the visible
  project list; each row is an accept/skip card.

  Pure prompt + parse + validate logic lives in projectAgent.ts
  (25 tests). Shared re-stream merge + JSON extraction live in
  $lib/agents/core. This component is the streaming + dialog +
  apply shell.

  Reuses the audit-gated chatStream pipeline — every call goes
  through gateChat/auditChat just like the rest of the AI
  surfaces.
-->
<script lang="ts">
	import { api, type Project } from '$lib/api';
	import { toast } from '$lib/components/toast';
	import { errorMessage } from '$lib/util/errorMessage';
	import { extractJsonBlock } from '$lib/util/jsonExtract';
	import {
		buildProjectAgentPrompt,
		parseProjectAgentResponse,
		validateProjectActions,
		summariseProjectAction,
		computeProjectRevertPatch,
		mergeProjectProposals,
		type ProjectAction,
		type ProjectProposalState,
		type ProjectRevertPatch
	} from './projectAgent';
	import { addIntentToHistory, normaliseHistory } from '$lib/agents/intentHistory';
	import { loadStored, saveStored } from '$lib/util/storage';

	interface Props {
		open: boolean;
		projects: Project[];
		todayISO: string;
		knownVentures?: string[];
		onClose: () => void;
		onChanged?: () => void | Promise<void>;
	}
	let { open, projects, todayISO, knownVentures = [], onClose, onChanged }: Props = $props();

	type ProposalRow = ProjectProposalState;
	type AppliedLog = { projectName: string; summary: string; revert: ProjectRevertPatch };

	let intent = $state('');
	let busy = $state(false);
	let raw = $state('');
	let error = $state('');
	let proposals = $state<ProposalRow[]>([]);
	let applyingAll = $state(false);
	let abort: AbortController | null = null;
	let applied = $state<AppliedLog[]>([]);
	let undoBusy = $state(false);

	// Persisted intent history. Reuses the same pure module the
	// Task Agent uses, with a project-scoped key so the histories
	// don't bleed across surfaces.
	const HISTORY_KEY = 'granit.projects.agent.history';
	let history = $state<string[]>(normaliseHistory(loadStored(HISTORY_KEY, []) as unknown));

	const PROMPTS = [
		'Archive anything obviously dead.',
		'Lower priority on side bets; raise it on the venture-critical ones.',
		'Set a sharp one-line next action on every active project that has none.',
		'Group these under their actual ventures.',
		'Pause anything blocked on someone else.'
	];

	$effect(() => {
		if (!open) {
			abort?.abort();
			abort = null;
			busy = false;
			proposals = [];
			raw = '';
			error = '';
		}
	});

	let inputEl: HTMLTextAreaElement | null = $state(null);
	$effect(() => {
		if (open && inputEl) queueMicrotask(() => inputEl?.focus());
	});

	function reset() {
		raw = '';
		error = '';
		proposals = [];
		applied = [];
	}

	async function run() {
		if (busy || !intent.trim()) return;
		history = addIntentToHistory(history, intent);
		saveStored(HISTORY_KEY, history);
		busy = true;
		reset();
		abort?.abort();
		abort = new AbortController();
		const { system, user } = buildProjectAgentPrompt(projects, intent, todayISO, knownVentures);
		try {
			await api.chatStream(
				[
					{ role: 'system', content: system },
					{ role: 'user', content: user }
				],
				undefined,
				{
					onChunk: (c) => {
						raw += c;
						const block = extractJsonBlock(raw);
						if (!block) return;
						const parsed = parseProjectAgentResponse(block);
						if (parsed.length > 0) {
							const valid = validateProjectActions(parsed, projects);
							proposals = mergeProjectProposals(proposals, valid) as ProposalRow[];
						}
					},
					onError: (err) => {
						error = err.message;
					}
				},
				abort.signal
			);
		} finally {
			busy = false;
			abort = null;
		}
	}

	function cancel() {
		abort?.abort();
	}

	async function applyAction(idx: number, opts: { deferReload?: boolean; silent?: boolean } = {}) {
		const p = proposals[idx];
		if (!p || p.applied || p.applying) return;
		const pre = projects.find((x) => x.name === p.projectName);
		if (!pre) {
			toast.error('Project no longer in scope; refresh and retry.');
			return;
		}
		const revert = computeProjectRevertPatch(p, pre);
		proposals = proposals.map((x, i) => (i === idx ? { ...x, applying: true } : x));
		try {
			await applyOne(p);
			proposals = proposals.map((x, i) =>
				i === idx ? { ...x, applied: true, applying: false } : x
			);
			if (revert) {
				applied = [
					...applied,
					{ projectName: p.projectName, summary: summariseProjectAction(p, pre), revert }
				];
			}
			if (!opts.silent) toast.success(summariseProjectAction(p, pre));
			if (!opts.deferReload) await onChanged?.();
		} catch (err) {
			proposals = proposals.map((x, i) => (i === idx ? { ...x, applying: false } : x));
			toast.error('Apply failed: ' + errorMessage(err));
		}
	}

	async function undoRun() {
		if (undoBusy || applied.length === 0) return;
		undoBusy = true;
		let undone = 0;
		try {
			for (let i = applied.length - 1; i >= 0; i--) {
				const log = applied[i];
				try {
					await api.patchProject(log.projectName, log.revert);
					undone++;
				} catch (err) {
					toast.error(`Undo failed for one project: ${errorMessage(err)}`);
				}
			}
			applied = [];
			proposals = proposals.map((p) => ({ ...p, applied: false }));
			toast.success(`Reverted ${undone} change${undone === 1 ? '' : 's'}`);
			await onChanged?.();
		} finally {
			undoBusy = false;
		}
	}

	function rejectAction(idx: number) {
		const p = proposals[idx];
		if (!p || p.applied) return;
		proposals = proposals.map((x, i) => (i === idx ? { ...x, rejected: true } : x));
	}

	async function applyAll() {
		if (applyingAll) return;
		applyingAll = true;
		const before = applied.length;
		try {
			for (let i = 0; i < proposals.length; i++) {
				const p = proposals[i];
				if (p.applied || p.rejected) continue;
				await applyAction(i, { deferReload: true, silent: true });
			}
			const n = applied.length - before;
			if (n > 0) toast.success(`Applied ${n} change${n === 1 ? '' : 's'}`);
			await onChanged?.();
		} finally {
			applyingAll = false;
		}
	}

	function useHistory(intent_: string) {
		intent = intent_;
	}
	function clearHistory() {
		history = [];
		saveStored(HISTORY_KEY, []);
	}

	// applyOne translates the action enum into a patchProject call.
	// Mirrors agent.ts's structure but for project fields. Kept
	// inline (not in projectAgent.ts) because it touches the API.
	async function applyOne(a: ProjectAction): Promise<void> {
		switch (a.kind) {
			case 'set_status':
				if (a.status) await api.patchProject(a.projectName, { status: a.status });
				return;
			case 'set_priority':
				await api.patchProject(a.projectName, { priority: a.priority ?? 2 });
				return;
			case 'set_due_date':
				if (a.due_date) await api.patchProject(a.projectName, { due_date: a.due_date });
				return;
			case 'clear_due_date':
				await api.patchProject(a.projectName, { due_date: '' });
				return;
			case 'set_next_action':
				if (a.next_action)
					await api.patchProject(a.projectName, { next_action: a.next_action });
				return;
			case 'clear_next_action':
				await api.patchProject(a.projectName, { next_action: '' });
				return;
			case 'set_venture':
				if (a.venture) await api.patchProject(a.projectName, { venture: a.venture });
				return;
			case 'clear_venture':
				await api.patchProject(a.projectName, { venture: '' });
				return;
			case 'change_description':
				if (a.description)
					await api.patchProject(a.projectName, { description: a.description });
				return;
			case 'archive':
				await api.patchProject(a.projectName, { status: 'archived' });
				return;
			case 'unarchive':
				await api.patchProject(a.projectName, { status: 'active' });
				return;
		}
	}

	let pendingCount = $derived(proposals.filter((p) => !p.applied && !p.rejected).length);
	let appliedCount = $derived(proposals.filter((p) => p.applied).length);

	function onKey(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			e.preventDefault();
			void run();
		}
		if (e.key === 'Escape') {
			e.preventDefault();
			onClose();
		}
	}
</script>

{#if open}
	<div
		class="fixed inset-0 z-50 bg-black/40 backdrop-blur-sm flex items-start justify-center pt-8 sm:pt-16 px-3 sm:px-6"
		role="dialog"
		aria-modal="true"
		aria-label="Project agent"
		onclick={(e) => {
			if (e.target === e.currentTarget) onClose();
		}}
		onkeydown={onKey}
		tabindex="-1"
	>
		<div class="w-full max-w-3xl bg-base border border-surface1 rounded-lg shadow-2xl flex flex-col max-h-[90vh]">
			<header class="px-4 py-3 border-b border-surface1 flex items-baseline gap-3 flex-shrink-0">
				<h2 class="text-base font-medium text-text flex-1 inline-flex items-baseline gap-2">
					<svg viewBox="0 0 24 24" class="w-4 h-4 self-center" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5z" />
						<path d="M5 21h14" />
					</svg>
					Project agent
				</h2>
				<span class="text-[11px] text-dim font-mono">{projects.length} project{projects.length === 1 ? '' : 's'} in scope</span>
				<button type="button" onclick={onClose} class="text-sm text-dim hover:text-text px-1" aria-label="close">×</button>
			</header>

			<div class="px-4 py-3 border-b border-surface1 flex-shrink-0 space-y-2">
				<textarea
					bind:value={intent}
					bind:this={inputEl}
					placeholder="Describe the change — &quot;archive everything no one has touched in 6 weeks&quot;, &quot;raise priority on the venture-critical ones&quot;, &quot;set a one-line next action on each active project&quot;"
					class="w-full text-sm px-3 py-2 rounded bg-surface0 border border-surface1 text-text placeholder:text-dim focus:outline-none focus:border-primary resize-none min-h-[64px]"
					rows="2"
					autocomplete="off"
					spellcheck="true"
				></textarea>

				{#if history.length > 0}
					<div class="flex items-baseline gap-1.5 flex-wrap">
						<span class="text-[10px] text-dim uppercase tracking-wide flex-shrink-0">recent:</span>
						{#each history as h, hi (hi + '::' + h)}
							<button
								type="button"
								onclick={() => useHistory(h)}
								class="text-[10px] px-2 py-0.5 rounded bg-secondary/10 border border-secondary/30 text-secondary hover:bg-secondary/20"
								title={h}
							>
								{h.length > 40 ? h.slice(0, 39) + '…' : h}
							</button>
						{/each}
						<button type="button" onclick={clearHistory} class="text-[10px] text-dim hover:text-error ml-1">clear</button>
					</div>
				{/if}

				<div class="flex items-center gap-1.5 flex-wrap">
					{#each PROMPTS as p}
						<button
							type="button"
							onclick={() => (intent = p)}
							class="text-[10px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-dim hover:border-primary hover:text-text"
						>
							{p.slice(0, 32)}{p.length > 32 ? '…' : ''}
						</button>
					{/each}
				</div>

				<div class="flex items-baseline gap-2">
					<p class="text-[10px] text-dim flex-1">
						{#if projects.length === 0}
							No projects in scope — adjust filters first.
						{:else}
							⌘↵ to submit · esc to close. Every action is your call — nothing applies until you accept.
						{/if}
					</p>
					{#if busy}
						<button onclick={cancel} class="text-xs text-warning hover:underline">cancel</button>
					{:else}
						<button
							onclick={() => void run()}
							disabled={!intent.trim() || projects.length === 0}
							class="text-xs px-3 py-1 rounded bg-primary text-on-primary hover:opacity-90 disabled:opacity-50"
						>Run</button>
					{/if}
				</div>
			</div>

			<div class="flex-1 min-h-0 overflow-y-auto px-4 py-3">
				{#if error}
					<p class="text-xs text-error mb-2">{error}</p>
				{/if}

				{#if applied.length > 0 && proposals.length === 0}
					<div class="mb-3 flex items-baseline gap-2 p-2 rounded bg-success/5 border border-success/30 text-[11px]">
						<span class="text-success">✓ {applied.length} change{applied.length === 1 ? '' : 's'} applied</span>
						<button
							onclick={() => void undoRun()}
							disabled={undoBusy}
							class="ml-auto text-xs text-warning hover:underline disabled:opacity-50"
						>{undoBusy ? 'undoing…' : `↶ undo`}</button>
					</div>
				{/if}

				{#if busy && proposals.length === 0}
					<p class="text-xs text-dim italic">Agent is thinking… this usually takes 5-15s.</p>
				{:else if !busy && raw && proposals.length === 0}
					<div class="text-xs text-dim space-y-2">
						<p>No actionable proposals.</p>
						<p class="text-[10px]">Either nothing matches the intent, or the model returned an unparseable response. Try a more concrete phrasing.</p>
						<details class="text-[10px]">
							<summary class="cursor-pointer hover:text-text">raw output</summary>
							<pre class="mt-1 bg-surface0 p-2 rounded max-h-32 overflow-auto whitespace-pre-wrap">{raw}</pre>
						</details>
					</div>
				{:else if proposals.length > 0}
					<div class="mb-2 flex items-baseline gap-2 text-[11px] text-dim">
						<span>{pendingCount} pending</span>
						{#if appliedCount > 0}<span class="text-success">· {appliedCount} applied</span>{/if}
						{#if applied.length > 0}
							<button
								onclick={() => void undoRun()}
								disabled={undoBusy}
								class="ml-auto text-xs text-warning hover:underline disabled:opacity-50"
							>{undoBusy ? 'undoing…' : `↶ undo (${applied.length})`}</button>
						{/if}
						{#if pendingCount > 0}
							<button
								onclick={() => void applyAll()}
								disabled={applyingAll}
								class="{applied.length > 0 ? '' : 'ml-auto'} text-xs text-secondary hover:underline disabled:opacity-50"
							>{applyingAll ? 'applying all…' : 'apply all'}</button>
						{/if}
					</div>
					<ul class="space-y-2">
						{#each proposals as p, i (p.projectName + '::' + p.kind + '::' + i)}
							{@const proj = projects.find((x) => x.name === p.projectName)}
							<li
								class="border rounded p-2 bg-surface0 transition-opacity {p.rejected
									? 'opacity-40 border-surface1'
									: p.applied
									? 'border-success/40 bg-success/5'
									: 'border-surface1 hover:border-primary'}"
							>
								<div class="flex items-baseline gap-2 mb-1">
									<span
										class="text-[9px] font-mono px-1 py-0.5 rounded uppercase tracking-wide {p.applied
											? 'bg-success/20 text-success'
											: p.rejected
											? 'bg-surface1 text-dim'
											: 'bg-surface1 text-subtext'}"
									>{p.kind.replace(/_/g, ' ')}</span>
									<p class="text-sm text-text flex-1">{summariseProjectAction(p, proj)}</p>
									{#if !p.applied && !p.rejected}
										<button
											type="button"
											onclick={() => void applyAction(i)}
											disabled={!!p.applying}
											class="text-[11px] px-2 py-0.5 rounded bg-primary/15 text-primary hover:bg-primary/25 disabled:opacity-50"
										>{p.applying ? '…' : 'accept'}</button>
										<button
											type="button"
											onclick={() => rejectAction(i)}
											class="text-[11px] px-2 py-0.5 rounded text-dim hover:text-text"
										>skip</button>
									{:else if p.applied}
										<span class="text-[11px] text-success">✓ applied</span>
									{:else}
										<span class="text-[11px] text-dim">skipped</span>
									{/if}
								</div>
								<p class="text-[11px] text-dim ml-1">{p.rationale}</p>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="text-xs text-dim italic">Type an intent above and run the agent. Suggestions:</p>
					<ul class="mt-2 space-y-1 text-[11px] text-subtext">
						{#each PROMPTS as p}
							<li>· {p}</li>
						{/each}
					</ul>
				{/if}
			</div>
		</div>
	</div>
{/if}
