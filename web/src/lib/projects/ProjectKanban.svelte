<!--
  ProjectKanban — board view of projects grouped by status. Four
  fixed columns: active / paused / completed / archived. Cards are
  draggable; dropping into a different column emits an
  onStatusChange callback so the parent can PATCH the project and
  refresh.

  Why fixed columns (not config-driven like the tasks Kanban):
  project status is a SHORT canonical lifecycle, not a user-defined
  pipeline. Custom statuses would fragment the schema for little
  gain. Empty columns ARE the point — they're the drop target for
  "park this", "archive this".

  All grouping logic lives in kanbanGroup.ts (pure, unit-tested).
  This file is DOM + drag UX only.
-->
<script lang="ts">
	import type { Project, Task } from '$lib/api';
	import { groupByStatus, statusLabel, type KanbanStatus } from './kanbanGroup';
	import ProjectStatusBar from './ProjectStatusBar.svelte';

	interface Props {
		projects: Project[];
		tasks: Task[];
		onSelect: (name: string) => void;
		onStatusChange: (name: string, status: KanbanStatus) => void | Promise<void>;
		colorVar: (c?: string) => string;
		statusTone: (status: string) => string;
		selectedName?: string;
	}
	let { projects, tasks, onSelect, onStatusChange, colorVar, statusTone, selectedName = '' }: Props =
		$props();

	let buckets = $derived(groupByStatus(projects));

	// Drag state lives in the component, not the URL — drag is
	// ephemeral. Tracking just the source name (not the source
	// column) is enough; the drop handler knows its own target.
	let draggingName = $state<string | null>(null);
	let dragOver = $state<KanbanStatus | null>(null);

	// Track the source column so the same-column hover doesn't show
	// a misleading drop-target highlight (the drop handler is a
	// no-op there — the highlight would imply something happens).
	let dragSource = $state<KanbanStatus | null>(null);

	function onDragStart(e: DragEvent, name: string) {
		draggingName = name;
		const src = projects.find((p) => p.name === name);
		dragSource = ((src?.status ?? 'active') as KanbanStatus);
		if (e.dataTransfer) {
			e.dataTransfer.effectAllowed = 'move';
			// Some browsers require setData for drag to start at all.
			e.dataTransfer.setData('text/plain', name);
		}
	}
	function onDragEnd() {
		draggingName = null;
		dragOver = null;
		dragSource = null;
	}
	function onColumnDragOver(e: DragEvent, status: KanbanStatus) {
		if (!draggingName) return;
		e.preventDefault(); // allow drop
		if (e.dataTransfer) {
			// Show 'none' for same-column to make the no-op explicit
			// (cursor reflects "you can't drop here usefully").
			e.dataTransfer.dropEffect = dragSource === status ? 'none' : 'move';
		}
		// Only flip the highlight for a foreign column — same-column
		// hovering should look untouched (a re-order would be misleading).
		dragOver = dragSource === status ? null : status;
	}
	function onColumnDragLeave(status: KanbanStatus) {
		if (dragOver === status) dragOver = null;
	}
	async function onColumnDrop(e: DragEvent, target: KanbanStatus) {
		e.preventDefault();
		const name = draggingName ?? e.dataTransfer?.getData('text/plain') ?? '';
		draggingName = null;
		dragOver = null;
		dragSource = null;
		if (!name) return;
		const src = projects.find((p) => p.name === name);
		const current = (src?.status ?? 'active') as KanbanStatus;
		if (current === target) return; // same-column drop is a no-op
		await onStatusChange(name, target);
	}

	// Per-project task counts: open / done. Computed once per render
	// so each card render doesn't re-walk the global task list.
	let countsByProject = $derived.by(() => {
		const out = new Map<string, { open: number; done: number; overdue: number }>();
		const today = new Date().toISOString().slice(0, 10);
		for (const p of projects) out.set(p.name, { open: 0, done: 0, overdue: 0 });
		for (const t of tasks) {
			for (const p of projects) {
				const folder = (p.folder ?? '').replace(/\/$/, '');
				const isMember = t.projectId === p.name || (folder && t.notePath.startsWith(folder + '/'));
				if (!isMember) continue;
				const m = out.get(p.name);
				if (!m) continue;
				if (t.done) m.done++;
				else {
					m.open++;
					if (t.dueDate && t.dueDate < today) m.overdue++;
				}
			}
		}
		return out;
	});

	function priorityBadge(p: number | undefined): string | null {
		if (!p) return null;
		if (p >= 3) return 'P1';
		if (p === 2) return 'P2';
		return 'P3';
	}
</script>

<div class="flex-1 min-h-0 overflow-x-auto overflow-y-hidden">
	<!-- The board grid. min-w on each column keeps cards readable
		 on narrow screens, the parent scrolls horizontally so all
		 four columns are reachable even on a phone. -->
	<div class="flex gap-2 px-3 sm:px-4 py-3 h-full min-w-fit">
		{#each buckets as bucket (bucket.status)}
			{@const isDropTarget = dragOver === bucket.status && draggingName !== null}
			<section
				class="flex flex-col flex-shrink-0 w-[18rem] sm:w-[20rem] rounded border bg-mantle/40 transition-colors {isDropTarget
					? 'border-primary bg-primary/5'
					: 'border-surface1'}"
				aria-label="{statusLabel(bucket.status)} column"
				ondragover={(e) => onColumnDragOver(e, bucket.status)}
				ondragleave={() => onColumnDragLeave(bucket.status)}
				ondrop={(e) => onColumnDrop(e, bucket.status)}
				role="list"
			>
				<header class="px-3 py-2 border-b border-surface1 flex items-baseline gap-2 flex-shrink-0">
					<span
						class="w-1.5 h-1.5 rounded-full flex-shrink-0"
						style="background: var(--color-{statusTone(bucket.status)})"
					></span>
					<h3 class="text-xs font-medium text-text uppercase tracking-wide">
						{statusLabel(bucket.status)}
					</h3>
					<span class="text-[10px] text-dim font-mono ml-auto">{bucket.projects.length}</span>
				</header>

				<div class="flex-1 min-h-0 overflow-y-auto p-2 space-y-2">
					{#if bucket.projects.length === 0}
						<p class="text-[11px] text-dim italic px-1 py-3 text-center">
							{#if isDropTarget}drop here →{:else}empty{/if}
						</p>
					{:else}
						{#each bucket.projects as p (p.name)}
							{@const c = countsByProject.get(p.name)}
							{@const pb = priorityBadge(p.priority)}
							{@const isSel = p.name === selectedName}
							{@const isDragging = draggingName === p.name}
							<article
								role="listitem"
								class="group border rounded bg-surface0 hover:border-primary transition-colors cursor-pointer {isSel
									? 'border-primary ring-1 ring-primary/30'
									: 'border-surface1'} {isDragging ? 'opacity-40' : ''}"
								draggable="true"
								ondragstart={(e) => onDragStart(e, p.name)}
								ondragend={onDragEnd}
								onclick={() => onSelect(p.name)}
								onkeydown={(e) => {
									if (e.key === 'Enter' || e.key === ' ') {
										e.preventDefault();
										onSelect(p.name);
									}
								}}
								tabindex="0"
								title="Drag to change status · click to open"
							>
								<div class="px-2.5 py-2">
									<div class="flex items-baseline gap-1.5 mb-1">
										<span
											class="w-2 h-2 rounded-full flex-shrink-0"
											style="background: {colorVar(p.color)}"
										></span>
										<h4 class="text-sm font-medium text-text flex-1 truncate" title={p.name}>
											{p.name}
										</h4>
										{#if pb}
											<span
												class="text-[9px] font-mono px-1 rounded {p.priority && p.priority >= 3
													? 'bg-error/15 text-error'
													: p.priority === 2
													? 'bg-warning/15 text-warning'
													: 'bg-surface1 text-dim'}"
												title="priority {p.priority}"
											>
												{pb}
											</span>
										{/if}
									</div>
									{#if p.venture}
										<p class="text-[10px] text-dim mb-1 truncate" title="venture {p.venture}">
											🏢 {p.venture}
										</p>
									{/if}
									{#if p.description}
										<p class="text-[11px] text-subtext line-clamp-2 mb-1.5">{p.description}</p>
									{/if}
									{#if c && c.open + c.done > 0}
										<div class="text-[10px] text-dim flex items-baseline gap-1.5">
											<span>{c.open} open</span>
											{#if c.overdue > 0}<span class="text-error">· {c.overdue} overdue</span>{/if}
											<span class="ml-auto">{c.done} done</span>
										</div>
										<ProjectStatusBar tasks={tasks} project={p} />
									{:else}
										<p class="text-[10px] text-dim italic">no tasks linked</p>
									{/if}
									{#if p.due_date}
										<p class="text-[10px] text-dim mt-1 font-mono">due {p.due_date}</p>
									{/if}
								</div>
							</article>
						{/each}
					{/if}
				</div>
			</section>
		{/each}
	</div>
</div>
