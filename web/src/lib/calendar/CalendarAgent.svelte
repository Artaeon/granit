<!--
  CalendarAgent — conversational AI surface scoped to /calendar.
  Fourth instance of the shared agent architecture
  (Tasks/Projects/Goals/Calendar). Same dialog UX, same apply +
  run-scoped undo lifecycle, same audit-gated chatStream pipeline.

  Scope (deliberately narrow — see calendarAgent.ts):
   - Native events only. ICS-sourced events are filtered out
     in the parent BEFORE being passed in.
   - Series-level edits only — no per-occurrence overrides.
   - No deletes, no rrule edits.

  Pure prompt / parse / validate / revert lives in
  calendarAgent.ts (22 tests). Shared re-stream + extraction in
  $lib/agents/core.
-->
<script lang="ts">
	import { api, type CalendarEventEntry } from '$lib/api';
	import { toast } from '$lib/components/toast';
	import { errorMessage } from '$lib/util/errorMessage';
	import { extractJsonBlock } from '$lib/util/jsonExtract';
	import {
		buildCalendarAgentPrompt,
		parseCalendarAgentResponse,
		validateCalendarActions,
		summariseCalendarAction,
		computeCalendarRevertPatch,
		mergeCalendarProposals,
		type CalendarAction,
		type CalendarProposalState,
		type CalendarRevertPatch
	} from './calendarAgent';
	import { addIntentToHistory, normaliseHistory } from '$lib/agents/intentHistory';
	import { loadStored, saveStored } from '$lib/util/storage';
	import { rafThrottle } from '$lib/util/streamThrottle';

	interface Props {
		open: boolean;
		events: CalendarEventEntry[]; // NATIVE events only — caller filters ICS out
		todayISO: string;
		knownProjects?: string[];
		onClose: () => void;
		onChanged?: () => void | Promise<void>;
	}
	let { open, events, todayISO, knownProjects = [], onClose, onChanged }: Props = $props();

	type ProposalRow = CalendarProposalState;
	type AppliedLog = { eventId: string; summary: string; revert: CalendarRevertPatch };

	let intent = $state('');
	let busy = $state(false);
	let raw = $state('');
	let error = $state('');
	let proposals = $state<ProposalRow[]>([]);
	let applyingAll = $state(false);
	let abort: AbortController | null = null;
	let applied = $state<AppliedLog[]>([]);
	let undoBusy = $state(false);

	const HISTORY_KEY = 'granit.calendar.agent.history';
	let history = $state<string[]>(normaliseHistory(loadStored(HISTORY_KEY, []) as unknown));

	const PROMPTS = [
		'Move morning meetings to the afternoon.',
		'Rename anything that\'s still labelled "untitled".',
		'Link my standup events to the right project.',
		'Tint client meetings blue.',
		'Clear the location on holdovers from last quarter.'
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
		const { system, user } = buildCalendarAgentPrompt(events, intent, todayISO, knownProjects);
		try {
			// rAF throttle — same shape as TaskAgent + GoalAgent.
			const calT = rafThrottle((full) => {
				raw = full;
				const block = extractJsonBlock(full);
				if (!block) return;
				const parsed = parseCalendarAgentResponse(block);
				if (parsed.length > 0) {
					const valid = validateCalendarActions(parsed, events, todayISO);
					proposals = mergeCalendarProposals(proposals, valid) as ProposalRow[];
				}
			});
			await api.chatStream(
				[
					{ role: 'system', content: system },
					{ role: 'user', content: user }
				],
				undefined,
				{
					onChunk: calT.onChunk,
					onDone: () => { calT.flush(); },
					onError: (err) => { calT.flush(); error = err.message; }
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
		const pre = events.find((e) => e.id === p.eventId);
		if (!pre) {
			toast.error('Event no longer in scope; refresh and retry.');
			return;
		}
		const revert = computeCalendarRevertPatch(p, pre);
		proposals = proposals.map((x, i) => (i === idx ? { ...x, applying: true } : x));
		try {
			await applyOne(p);
			proposals = proposals.map((x, i) =>
				i === idx ? { ...x, applied: true, applying: false } : x
			);
			if (revert) {
				applied = [
					...applied,
					{ eventId: p.eventId, summary: summariseCalendarAction(p, pre), revert }
				];
			}
			if (!opts.silent) toast.success(summariseCalendarAction(p, pre));
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
					await api.patchEvent(log.eventId, log.revert as Partial<CalendarEventEntry>);
					undone++;
				} catch (err) {
					toast.error(`Undo failed for one event: ${errorMessage(err)}`);
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

	// applyOne translates the action enum into a patchEvent call.
	// set_event_project maps to project_id on the server. clear_*
	// variants send the empty string which the omitempty JSON tag
	// drops on the next read.
	async function applyOne(a: CalendarAction): Promise<void> {
		switch (a.kind) {
			case 'rename_event':
				if (a.title) await api.patchEvent(a.eventId, { title: a.title });
				return;
			case 'move_event_to_date':
				if (a.date) await api.patchEvent(a.eventId, { date: a.date });
				return;
			case 'set_event_time':
				if (a.start_time && a.end_time)
					await api.patchEvent(a.eventId, {
						start_time: a.start_time,
						end_time: a.end_time
					});
				return;
			case 'set_event_color':
				if (a.color) await api.patchEvent(a.eventId, { color: a.color });
				return;
			case 'set_event_location':
				if (a.location) await api.patchEvent(a.eventId, { location: a.location });
				return;
			case 'clear_event_location':
				await api.patchEvent(a.eventId, { location: '' });
				return;
			case 'set_event_project':
				if (a.project) await api.patchEvent(a.eventId, { project_id: a.project });
				return;
			case 'clear_event_project':
				await api.patchEvent(a.eventId, { project_id: '' });
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
		class="fixed inset-0 z-50 bg-black/60 flex items-center justify-center p-3 sm:p-6" style="padding-right: calc(0.75rem + var(--ai-pinned-w, 0px));"
		role="dialog"
		aria-modal="true"
		aria-label="Calendar agent"
		onclick={(e) => {
			if (e.target === e.currentTarget) onClose();
		}}
		onkeydown={onKey}
		tabindex="-1"
	>
		<div class="w-full max-w-3xl bg-base border border-surface1 rounded-xl shadow-2xl flex flex-col max-h-[88vh]">
			<header class="px-3 py-2 border-b border-surface1 flex items-baseline gap-3 flex-shrink-0">
				<h2 class="text-base font-medium text-text flex-1 inline-flex items-baseline gap-2">
					<svg viewBox="0 0 24 24" class="w-4 h-4 self-center" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5z" />
						<path d="M5 21h14" />
					</svg>
					Calendar agent
				</h2>
				<span class="text-[11px] text-dim font-mono">{events.length} event{events.length === 1 ? '' : 's'} in scope</span>
				<button type="button" onclick={onClose} class="text-sm text-dim hover:text-text px-1" aria-label="close">×</button>
			</header>

			<div class="px-3 py-2 border-b border-surface1 flex-shrink-0 space-y-2">
				<textarea
					bind:value={intent}
					bind:this={inputEl}
					placeholder="Describe the change — &quot;move morning meetings to afternoon&quot;, &quot;rename untitled events&quot;, &quot;tint client meetings blue&quot;"
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
								class="text-[10px] px-2 py-0.5 rounded bg-surface1 border border-surface2 text-secondary hover:bg-surface2"
								title={h}
							>{h.length > 40 ? h.slice(0, 39) + '…' : h}</button>
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
						>{p.slice(0, 32)}{p.length > 32 ? '…' : ''}</button>
					{/each}
				</div>

				<div class="flex items-baseline gap-2">
					<p class="text-[10px] text-dim flex-1">
						{#if events.length === 0}
							No native events in scope — ICS events are read-only and excluded.
						{:else}
							⌘↵ to submit · esc to close. Per-occurrence overrides go through the event-detail UI.
						{/if}
					</p>
					{#if busy}
						<button onclick={cancel} class="text-xs text-warning hover:underline">cancel</button>
					{:else}
						<button
							onclick={() => void run()}
							disabled={!intent.trim() || events.length === 0}
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
					<div class="mb-3 flex items-baseline gap-2 p-2 rounded bg-surface0 border border-success text-[11px]">
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
						{#each proposals as p, i (p.eventId + '::' + p.kind + '::' + i)}
							{@const ev = events.find((x) => x.id === p.eventId)}
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
									<p class="text-sm text-text flex-1">{summariseCalendarAction(p, ev)}</p>
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
