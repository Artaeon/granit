<!--
  AIDraftBadge — renders the small "from chat about X" chip on
  notes saved through the sidebar chat's "save as note" flow.
  The save path stamps frontmatter:
    type: 'ai-draft'
    mode: <agent mode id>
    project: <name>?      // if PM mode / project page
    goal:    <id>?        // if Goal Manager / goal page
    calendar_window: true?  // if Calendar Manager / calendar page
    captured_at: ISO
    tags: ['ai-draft', mode]

  This component reads those fields and renders a clickable
  badge that bounces the user back to the source page — closes
  the cross-linking loop so a drafted brief discovered weeks
  later still carries a path to the project / goal it came
  from.

  Self-contained: takes a frontmatter Record and the API helpers
  for the link labels. No fetches — purely a presentation
  surface. The host (notes editor header) decides when to render
  this (e.g. only when frontmatter.type === 'ai-draft').
-->
<script lang="ts">
	import { findMode } from '$lib/ai/agents';

	interface Props {
		/** Note frontmatter — typed as Record so the consumer
		 *  doesn't need to know the full Note shape. */
		frontmatter: Record<string, unknown> | undefined;
		/** Optional: resolved project name for display. Only used
		 *  when frontmatter.project is set. */
		projectLabel?: string;
		/** Optional: resolved goal title for display. Only used
		 *  when frontmatter.goal is set. */
		goalLabel?: string;
	}
	let { frontmatter, projectLabel, goalLabel }: Props = $props();

	// Reactive derivations on the frontmatter fields. Strings are
	// trimmed to defend against trailing newlines from manual
	// YAML edits.
	let isAIDraft = $derived(
		!!frontmatter && (frontmatter.type === 'ai-draft' || frontmatter.type === 'chat')
	);
	let modeId = $derived(
		typeof frontmatter?.mode === 'string' ? frontmatter.mode.trim() : ''
	);
	let mode = $derived(modeId ? findMode(modeId) : null);
	let projectName = $derived(
		typeof frontmatter?.project === 'string' ? frontmatter.project.trim() : ''
	);
	let goalId = $derived(
		typeof frontmatter?.goal === 'string' ? frontmatter.goal.trim() : ''
	);
	let isCalendar = $derived(frontmatter?.calendar_window === true);

	// Pick the link target — project wins (most specific entity);
	// then goal; then calendar; then nothing (still render the
	// "AI draft · <mode>" pill so the user knows the provenance
	// even without a back-link).
	let href = $derived(
		projectName
			? `/projects/${encodeURIComponent(projectName)}`
			: goalId
			? `/goals?focus=${encodeURIComponent(goalId)}`
			: isCalendar
			? '/calendar'
			: ''
	);
	let sourceLabel = $derived(
		projectName
			? `project: ${projectLabel || projectName}`
			: goalId
			? `goal: ${goalLabel || goalId}`
			: isCalendar
			? 'calendar'
			: ''
	);
</script>

{#if isAIDraft}
	{#if href}
		<a
			{href}
			class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full border border-surface2 bg-surface1 text-primary text-[11px] hover:bg-surface2 hover:border-primary transition-colors min-h-[24px]"
			title="Saved from the AI sidebar — click to open the {sourceLabel.split(':')[0]}"
		>
			<span aria-hidden="true">{mode?.glyph ?? '✨'}</span>
			<span>
				<span class="text-dim">from chat ·</span>
				<span class="font-medium">{sourceLabel}</span>
			</span>
		</a>
	{:else}
		<!-- No specific source (saved without a context in scope) —
			 still surface the AI-draft provenance with the mode label
			 so the file's origin is obvious. -->
		<span
			class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full border border-surface1 bg-surface0 text-dim text-[11px] min-h-[24px]"
			title="Saved from the AI sidebar — no specific source context"
		>
			<span aria-hidden="true">{mode?.glyph ?? '✨'}</span>
			<span>AI draft{mode ? ` · ${mode.label}` : ''}</span>
		</span>
	{/if}
{/if}
