<!--
  TaskAgent — conversational AI surface scoped to the /tasks page.
  User types a free-text intent, the agent proposes typed actions
  (set priority, archive, schedule, mark done, …) on the currently
  filtered tasks. User accepts/rejects each card; apply runs the
  PATCH per action.

  Pure prompt + parse + validate logic lives in agent.ts (23
  tests). This component is the streaming + dialog + apply shell.
  Reuses the audit-gated chatStream pipeline — every call goes
  through gateChat/auditChat just like the rest of the app.

  Open/close + the input box stay in the parent (the +page.svelte
  toolbar drives this). When `open` flips false we cancel any
  in-flight stream so a half-streamed proposal doesn't keep
  arriving after the user closes the dialog.
-->
<script lang="ts">
	import { api, type Task } from '$lib/api';
	import { toast } from '$lib/components/toast';
	import { errorMessage } from '$lib/util/errorMessage';
	import { extractJsonBlock } from '$lib/util/jsonExtract';
	import {
		buildAgentPrompt,
		parseAgentResponse,
		validateActions,
		summariseAction,
		computeRevertPatch,
		mergeProposals,
		type TaskAction,
		type TaskRevertPatch
	} from './agent';
	import { addIntentToHistory, normaliseHistory } from '$lib/agents/intentHistory';
	import { loadStored, saveStored } from '$lib/util/storage';

	interface Props {
		open: boolean;
		tasks: Task[]; // the scope the agent sees — caller's filtered list
		todayISO: string;
		availableProjects?: string[];
		onClose: () => void;
		onChanged?: () => void | Promise<void>;
	}
	let { open, tasks, todayISO, availableProjects = [], onClose, onChanged }: Props = $props();

	type ProposalRow = TaskAction & { applied?: boolean; applying?: boolean; rejected?: boolean };

	// Each applied action stashes its revert patch + the original
	// task id, so the dialog can offer a single "Undo run" button
	// that walks the list backwards. Session-scoped (resets on
	// close) — long-term undo would conflict with edits the user
	// made manually after the agent run.
	type AppliedLog = { taskId: string; summary: string; revert: TaskRevertPatch };

	let intent = $state('');
	let busy = $state(false);
	let raw = $state('');
	let error = $state('');
	let proposals = $state<ProposalRow[]>([]);
	let applyingAll = $state(false);
	let abort: AbortController | null = null;
	let applied = $state<AppliedLog[]>([]);
	let undoBusy = $state(false);

	// Persisted intent history. Loaded once when the dialog first
	// mounts (we don't reload on every open — would stomp on the
	// in-memory dedup). Saved on every successful run().
	const HISTORY_KEY = 'granit.tasks.agent.history';
	let history = $state<string[]>(normaliseHistory(loadStored(HISTORY_KEY, []) as unknown));

	// Suggestion chips so the user doesn't stare at a blank box.
	// Curated for the high-leverage cases: cleanup, scheduling,
	// prioritisation. Click → fill the textarea so the user can
	// edit before submitting.
	const PROMPTS = [
		'Archive anything obviously dead or no longer relevant.',
		'Lower priority on admin chores; raise priority on anything blocking.',
		'Schedule the highest-leverage open work for tomorrow morning.',
		'Snooze anything not actionable this week.',
		'Mark everything that looks already done.'
	];

	$effect(() => {
		// When the parent closes the dialog, drop any in-flight
		// stream so we don't keep burning tokens / arriving stale
		// proposals after the user moved on. Also clear the per-
		// run UI state (proposals / raw / error) so reopening
		// doesn't surface ghost rows from the previous run. We
		// DELIBERATELY keep the `applied` log so the user can
		// still undo a closed-dialog run on reopen — closing
		// shouldn't strand changes.
		if (!open) {
			abort?.abort();
			abort = null;
			busy = false;
			proposals = [];
			raw = '';
			error = '';
		}
	});

	// Auto-focus the intent textarea when the dialog opens. Makes
	// the keyboard shortcut ('a') feel responsive — the user hits
	// 'a' and is immediately typing. Bound to the textarea element
	// below; we trigger focus only on the open→true edge so
	// re-renders during streaming don't keep stealing focus from
	// the accept/skip buttons.
	let inputEl: HTMLTextAreaElement | null = $state(null);
	$effect(() => {
		if (open && inputEl) {
			// Defer to next microtask so the textarea is in the DOM
			// before we ask the browser to focus it.
			queueMicrotask(() => inputEl?.focus());
		}
	});

	function reset() {
		raw = '';
		error = '';
		proposals = [];
		// Reset undo log on a fresh run — undoing after a new run
		// would be confusing (which set of changes is being
		// reverted?). User has to undo BEFORE running the next
		// intent.
		applied = [];
	}

	async function run() {
		if (busy || !intent.trim()) return;
		// Persist the intent BEFORE we start the stream — even a
		// cancelled run is a valid history entry the user might
		// want to retry. Computed via the pure helper, then
		// flushed to localStorage.
		history = addIntentToHistory(history, intent);
		saveStored(HISTORY_KEY, history);
		busy = true;
		reset();
		abort?.abort();
		abort = new AbortController();
		const { system, user } = buildAgentPrompt(tasks, intent, todayISO, availableProjects);
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
						// Try to parse the JSON block as it streams. The
						// closing brace usually only arrives at the end,
						// so this typically populates once — but fast
						// local models can populate mid-stream and that's
						// the nicer UX (see plan-day for the precedent).
						const block = extractJsonBlock(raw);
						if (!block) return;
						const parsed = parseAgentResponse(block);
						if (parsed.length > 0) {
							const valid = validateActions(parsed, tasks);
							// mergeProposals preserves applied/rejected rows even
							// if the new parse no longer mentions them — protects
							// the audit trail from disappearing when an accept
							// triggers a parent reload that filters the task out
							// of scope. Pure helper, see agent.ts tests.
							proposals = mergeProposals(proposals, valid) as ProposalRow[];
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
		// Snapshot the pre-state so undo has something to revert to.
		// If the task isn't on the current list any more (raced with
		// a delete), skip — we wouldn't be able to revert anyway.
		const preTask = tasks.find((t) => t.id === p.taskId);
		if (!preTask) {
			toast.error('Task no longer in scope; refresh and retry.');
			return;
		}
		const revert = computeRevertPatch(p, preTask);
		proposals = proposals.map((x, i) => (i === idx ? { ...x, applying: true } : x));
		try {
			await applyOne(p);
			proposals = proposals.map((x, i) =>
				i === idx ? { ...x, applied: true, applying: false } : x
			);
			if (revert) {
				applied = [
					...applied,
					{ taskId: p.taskId, summary: summariseAction(p, preTask), revert }
				];
			}
			// silent suppresses the per-item toast — applyAll uses
			// this to surface ONE summary toast instead of N noisy
			// success toasts when batch-applying.
			if (!opts.silent) toast.success(summariseAction(p, preTask));
			// deferReload lets applyAll batch a single onChanged call
			// after the whole loop completes; per-item reloads would
			// re-fetch + re-broadcast N times for an N-action batch.
			if (!opts.deferReload) await onChanged?.();
		} catch (err) {
			proposals = proposals.map((x, i) => (i === idx ? { ...x, applying: false } : x));
			toast.error('Apply failed: ' + errorMessage(err));
		}
	}

	// Undo every action applied since the last run() / dialog open.
	// Walks backwards so re-application order matches reverse order
	// of original application — important for paired operations
	// like "set priority then schedule" where the schedule lookup
	// might want the original priority back first.
	async function undoRun() {
		if (undoBusy || applied.length === 0) return;
		undoBusy = true;
		let undone = 0;
		try {
			for (let i = applied.length - 1; i >= 0; i--) {
				const log = applied[i];
				try {
					await api.patchTask(log.taskId, log.revert);
					undone++;
				} catch (err) {
					toast.error(`Undo failed for one task: ${errorMessage(err)}`);
				}
			}
			applied = [];
			// Reset the proposal flags so the cards become re-acceptable
			// (the user might want to redo a subset they just undid).
			proposals = proposals.map((p) => ({ ...p, applied: false }));
			toast.success(`Reverted ${undone} change${undone === 1 ? '' : 's'}`);
			await onChanged?.();
		} finally {
			undoBusy = false;
		}
	}

	function useHistory(intent_: string) {
		intent = intent_;
	}

	function clearHistory() {
		history = [];
		saveStored(HISTORY_KEY, []);
	}

	function rejectAction(idx: number) {
		// Guard against rejecting an already-applied row — could only
		// happen via a race the UI doesn't expose (the buttons hide
		// once applied), but defensive against future markup edits.
		// An applied row marked rejected would render as "skipped"
		// despite having mutated the backing task, which would lie
		// about the state of the world.
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
			// One reload + one summary toast for the whole batch.
			const n = applied.length - before;
			if (n > 0) toast.success(`Applied ${n} change${n === 1 ? '' : 's'}`);
			await onChanged?.();
		} finally {
			applyingAll = false;
		}
	}

	// Single-action applier. Translates the action enum into a
	// patchTask call — kept inline (not in agent.ts) because it
	// reaches the live API. agent.ts stays pure for tests.
	async function applyOne(a: TaskAction): Promise<void> {
		switch (a.kind) {
			case 'set_priority':
				await api.patchTask(a.taskId, { priority: a.priority ?? 2 });
				return;
			case 'set_due':
				if (a.dueDate) await api.patchTask(a.taskId, { dueDate: a.dueDate });
				return;
			case 'clear_due':
				await api.patchTask(a.taskId, { dueDate: '' });
				return;
			case 'schedule':
				if (a.scheduledStart) {
					await api.patchTask(a.taskId, {
						scheduledStart: a.scheduledStart,
						...(a.durationMinutes ? { durationMinutes: a.durationMinutes } : {})
					});
				}
				return;
			case 'clear_schedule':
				await api.patchTask(a.taskId, { clearSchedule: true });
				return;
			case 'mark_done':
				await api.patchTask(a.taskId, { done: true });
				return;
			case 'archive':
				// Matches the stale-review accept path: done=true,
				// triage='dropped'. Surface remains visible in
				// completed/archived but out of the active flow.
				await api.patchTask(a.taskId, { done: true, triage: 'dropped' });
				return;
			case 'unarchive':
				await api.patchTask(a.taskId, { done: false, triage: 'inbox' });
				return;
			case 'snooze':
				if (a.snoozedUntil) await api.patchTask(a.taskId, { snoozedUntil: a.snoozedUntil });
				return;
			case 'set_project':
				await api.patchTask(a.taskId, { projectId: a.projectId ?? '' });
				return;
			case 'change_text':
				if (a.text) await api.patchTask(a.taskId, { text: a.text });
				return;
		}
	}

	let pendingCount = $derived(proposals.filter((p) => !p.applied && !p.rejected).length);
	let appliedCount = $derived(proposals.filter((p) => p.applied).length);

	function onKey(e: KeyboardEvent) {
		// Cmd/Ctrl-Enter submits the intent (matches chat dialog
		// convention across the app).
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
	<!-- Modal overlay. Click-outside dismisses (rare for AI surfaces
		 but consistent with the rest of the app's modals). -->
	<div
		class="fixed inset-0 z-50 bg-black/40 flex items-start justify-center pt-8 sm:pt-16 px-3 sm:px-6"
		role="dialog"
		aria-modal="true"
		aria-label="Task agent"
		onclick={(e) => {
			if (e.target === e.currentTarget) onClose();
		}}
		onkeydown={onKey}
		tabindex="-1"
	>
		<div class="w-full max-w-3xl bg-base border border-surface1 rounded-lg shadow-2xl flex flex-col max-h-[90vh]">
			<!-- Header -->
			<header class="px-4 py-3 border-b border-surface1 flex items-baseline gap-3 flex-shrink-0">
				<h2 class="text-base font-medium text-text flex-1 inline-flex items-baseline gap-2">
					<svg viewBox="0 0 24 24" class="w-4 h-4 self-center" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5z" />
						<path d="M5 21h14" />
					</svg>
					Task agent
				</h2>
				<span class="text-[11px] text-dim font-mono">{tasks.length} task{tasks.length === 1 ? '' : 's'} in scope</span>
				<button type="button" onclick={onClose} class="text-sm text-dim hover:text-text px-1" aria-label="close">×</button>
			</header>

			<!-- Input + suggestion chips -->
			<div class="px-4 py-3 border-b border-surface1 flex-shrink-0 space-y-2">
				<textarea
					bind:value={intent}
					bind:this={inputEl}
					placeholder="Tell the agent what to do — &quot;archive anything obviously stale&quot;, &quot;raise priority on blockers&quot;, &quot;schedule the report for Friday morning&quot;"
					class="w-full text-sm px-3 py-2 rounded bg-surface0 border border-surface1 text-text placeholder:text-dim focus:outline-none focus:border-primary resize-none min-h-[64px]"
					rows="2"
					autocomplete="off"
					spellcheck="true"
				></textarea>

				<!-- Recent intents — one-click reuse. Distinct from the
					 starter prompts so the user knows what's theirs vs.
					 what's a template. Shown only when there's actual
					 history; otherwise the starter chips below carry the
					 first-run onboarding load. -->
				{#if history.length > 0}
					<div class="flex items-baseline gap-1.5 flex-wrap">
						<span class="text-[10px] text-dim uppercase tracking-wide flex-shrink-0">recent:</span>
						{#each history as h, hi (hi + '::' + h)}
							<button
								type="button"
								onclick={() => useHistory(h)}
								class="text-[10px] px-2 py-0.5 rounded bg-surface1 border border-surface2 text-secondary hover:bg-surface2"
								title={h}
							>
								{h.length > 40 ? h.slice(0, 39) + '…' : h}
							</button>
						{/each}
						<button
							type="button"
							onclick={clearHistory}
							class="text-[10px] text-dim hover:text-error ml-1"
							title="clear history"
						>clear</button>
					</div>
				{/if}

				<div class="flex items-center gap-1.5 flex-wrap">
					{#each PROMPTS as p}
						<button
							type="button"
							onclick={() => (intent = p)}
							class="text-[10px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-dim hover:border-primary hover:text-text"
							title="use this as the starting intent"
						>
							{p.slice(0, 32)}{p.length > 32 ? '…' : ''}
						</button>
					{/each}
				</div>
				<div class="flex items-baseline gap-2">
					<p class="text-[10px] text-dim flex-1">
						{#if tasks.length === 0}
							No tasks in scope — pick a non-empty filter or clear your bulk-selection.
						{:else}
							⌘↵ to submit · esc to close. Every action is your call — nothing applies until you accept.
						{/if}
					</p>
					{#if busy}
						<button onclick={cancel} class="text-xs text-warning hover:underline">cancel</button>
					{:else}
						<button
							onclick={() => void run()}
							disabled={!intent.trim() || tasks.length === 0}
							class="text-xs px-3 py-1 rounded bg-primary text-on-primary hover:opacity-90 disabled:opacity-50"
							title={tasks.length === 0
								? 'Agent needs at least one task in scope'
								: 'Submit the intent'}
						>Run</button>
					{/if}
				</div>
			</div>

			<!-- Body -->
			<div class="flex-1 min-h-0 overflow-y-auto px-4 py-3">
				{#if error}
					<p class="text-xs text-error mb-2">{error}</p>
				{/if}

				<!-- Standalone undo banner. Surfaces whenever the user
					 has applied changes but the proposal list is empty
					 (closed-and-reopened the dialog, or a re-stream
					 dropped pending rows). Without this the undo
					 button only lives inside the proposals header and
					 disappears with the rows. -->
				{#if applied.length > 0 && proposals.length === 0}
					<div class="mb-3 flex items-baseline gap-2 p-2 rounded bg-surface0 border border-success text-[11px]">
						<span class="text-success">✓ {applied.length} change{applied.length === 1 ? '' : 's'} applied</span>
						<button
							onclick={() => void undoRun()}
							disabled={undoBusy}
							class="ml-auto text-xs text-warning hover:underline disabled:opacity-50"
							title="Revert every change applied in this run"
						>{undoBusy ? 'undoing…' : `↶ undo`}</button>
					</div>
				{/if}

				{#if busy && proposals.length === 0}
					<p class="text-xs text-dim italic">Agent is thinking… this usually takes 5-15s.</p>
				{:else if !busy && raw && proposals.length === 0}
					<div class="text-xs text-dim space-y-2">
						<p>No actionable proposals.</p>
						<p class="text-[10px]">The model returned an empty (or unparseable) action list. Either there's nothing to do for that intent, or the response was off-shape — try rephrasing more concretely.</p>
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
								title="Revert every change applied in this run"
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
						{#each proposals as p, i (p.taskId + '::' + p.kind + '::' + i)}
							{@const task = tasks.find((t) => t.id === p.taskId)}
							<li
								class="border rounded p-2 bg-surface0 transition-opacity {p.rejected
									? 'opacity-40 border-surface1'
									: p.applied
									? 'border-success bg-surface0'
									: 'border-surface1 hover:border-primary'}"
							>
								<div class="flex items-baseline gap-2 mb-1">
									<span
										class="text-[9px] font-mono px-1 py-0.5 rounded uppercase tracking-wide {p.applied
											? 'bg-surface0 text-success'
											: p.rejected
											? 'bg-surface1 text-dim'
											: 'bg-surface1 text-subtext'}"
									>{p.kind.replace(/_/g, ' ')}</span>
									<p class="text-sm text-text flex-1">{summariseAction(p, task)}</p>
									{#if !p.applied && !p.rejected}
										<button
											type="button"
											onclick={() => void applyAction(i)}
											disabled={!!p.applying}
											class="text-[11px] px-2 py-0.5 rounded bg-surface1 text-primary hover:bg-primary hover:text-on-primary disabled:opacity-50"
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
