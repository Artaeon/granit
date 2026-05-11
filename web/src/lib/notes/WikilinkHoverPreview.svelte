<!--
  WikilinkHoverPreview — a single floating tooltip that any
  MarkdownRenderer instance can host. Listens to mouseover/mouseout
  on the supplied `host` element, detects `[data-wikilink]`, fetches
  the target note (cached per session), and shows the first
  paragraph as a tooltip near the link.

  Cross-surface design: every place wikilinks render (notes preview,
  embed cards, reference panel, future hover-on-editor) can drop one
  of these instances next to its prose container and gain the same
  behaviour. No prop-drilling of fetch state — the module cache
  shares the result across instances.

  Note resolution: the wikilink target is the human title, not the
  vault path. We hit /api/v1/notes?q=<title>&limit=5 (same listNotes
  surface the wikilink-click navigation uses) and pick the exact-
  title match when present, else the first hit.
-->
<script lang="ts" module>
	import { api } from '$lib/api';
	import { extractFirstParagraph, stripInlineMarkdown } from './wikilinkPreview';

	// Module-scoped promise cache. Lives for the duration of the page
	// session — opening a note → hovering five wikilinks → going back
	// and re-hovering the same wikilinks hits the cache, no refetch.
	// Memory is bounded by the wikilink set the user actually hovers,
	// typically dozens at most.
	export interface PreviewData {
		title: string;
		path: string;
		body: string; // already stripped of frontmatter + capped
	}
	const cache = new Map<string, Promise<PreviewData | null>>();

	export function fetchPreview(rawTarget: string): Promise<PreviewData | null> {
		// Strip the optional block-level fragment ([[Note#Heading]]).
		// We always preview the same Note regardless of which heading
		// the wikilink targets — the heading-jump is a navigation cue,
		// not a separate document.
		const target = rawTarget.split('#')[0].trim();
		if (!target) return Promise.resolve(null);
		const cached = cache.get(target);
		if (cached) return cached;
		const p = (async (): Promise<PreviewData | null> => {
			try {
				const r = await api.listNotes({ q: target, limit: 5 });
				const exact = r.notes.find(
					(n) => n.title.toLowerCase() === target.toLowerCase()
				);
				const hit = exact ?? r.notes[0];
				if (!hit) return null;
				// We don't have the body in the list response. Fetch.
				const note = await api.getNote(hit.path);
				const raw = extractFirstParagraph(note.body ?? '');
				return {
					title: note.title ?? hit.title,
					path: note.path,
					body: stripInlineMarkdown(raw)
				};
			} catch {
				return null;
			}
		})();
		cache.set(target, p);
		return p;
	}
</script>

<script lang="ts">
	import { onDestroy } from 'svelte';

	interface Props {
		/** The container element whose descendants' [data-wikilink]
		 *  elements should trigger the preview. */
		host: HTMLElement | undefined;
		/** Hover delay before the preview shows (ms). Default 280ms —
		 *  long enough that mousing over inline links while reading
		 *  doesn't flash tooltips, short enough that a deliberate
		 *  hover-to-preview feels responsive. */
		showDelay?: number;
	}
	let { host, showDelay = 280 }: Props = $props();

	let preview = $state<PreviewData | null>(null);
	let loading = $state(false);
	// Position state for the tooltip. We render it position:fixed and
	// place it just below the hovered link, flipping above when the
	// link is in the bottom 30% of the viewport.
	let top = $state(0);
	let left = $state(0);
	let visible = $state(false);
	let currentTarget = $state<string>('');

	let showTimer: ReturnType<typeof setTimeout> | null = null;
	let hideTimer: ReturnType<typeof setTimeout> | null = null;
	let lastEl: HTMLElement | null = null;

	function clearTimers() {
		if (showTimer) {
			clearTimeout(showTimer);
			showTimer = null;
		}
		if (hideTimer) {
			clearTimeout(hideTimer);
			hideTimer = null;
		}
	}

	function position(el: HTMLElement) {
		const rect = el.getBoundingClientRect();
		const vw = window.innerWidth;
		const vh = window.innerHeight;
		const tipWidth = Math.min(360, vw - 24);
		// Below by default; flip above when the link sits low.
		const wantBelow = rect.bottom < vh * 0.7;
		top = wantBelow ? rect.bottom + 6 : Math.max(8, rect.top - 6 - 200);
		// Anchor to the link's left edge, clamp to viewport.
		let l = rect.left;
		if (l + tipWidth > vw - 12) l = vw - tipWidth - 12;
		if (l < 12) l = 12;
		left = l;
	}

	async function showFor(el: HTMLElement) {
		const target = el.getAttribute('data-wikilink') ?? '';
		if (!target) return;
		currentTarget = target;
		position(el);
		visible = true;
		loading = true;
		preview = null;
		const data = await fetchPreview(target);
		// If the hover moved on to a different link mid-fetch, drop
		// this result — the one in flight for the NEW link will land.
		if (currentTarget !== target) return;
		preview = data;
		loading = false;
		if (el.isConnected) position(el); // re-measure post-content
	}

	function onOver(e: MouseEvent) {
		const t = e.target as HTMLElement | null;
		if (!t) return;
		const el = t.closest('[data-wikilink]') as HTMLElement | null;
		if (!el) return;
		if (el === lastEl && visible) return; // already showing
		lastEl = el;
		clearTimers();
		showTimer = setTimeout(() => {
			showTimer = null;
			void showFor(el);
		}, showDelay);
	}

	function onOut(e: MouseEvent) {
		const t = e.target as HTMLElement | null;
		const related = e.relatedTarget as HTMLElement | null;
		// Only hide when leaving the wikilink AND not entering the
		// tooltip itself (so the user can mouse INTO the tooltip to
		// read longer content without it vanishing).
		const wl = t?.closest('[data-wikilink]');
		if (!wl) return;
		if (related && (related.closest('[data-wikilink-tooltip]') || related.closest('[data-wikilink]'))) {
			return;
		}
		clearTimers();
		hideTimer = setTimeout(() => {
			hideTimer = null;
			visible = false;
			lastEl = null;
		}, 120);
	}

	function onTooltipEnter() {
		// Reading the tooltip — cancel the hide.
		if (hideTimer) {
			clearTimeout(hideTimer);
			hideTimer = null;
		}
	}
	function onTooltipLeave() {
		hideTimer = setTimeout(() => {
			hideTimer = null;
			visible = false;
			lastEl = null;
		}, 120);
	}

	// Attach / detach the listeners as `host` changes (the parent's
	// element ref binds late on mount). Cleanup on unmount removes both.
	$effect(() => {
		const h = host;
		if (!h) return;
		h.addEventListener('mouseover', onOver);
		h.addEventListener('mouseout', onOut);
		return () => {
			h.removeEventListener('mouseover', onOver);
			h.removeEventListener('mouseout', onOut);
		};
	});

	onDestroy(clearTimers);
</script>

{#if visible}
	<div
		data-wikilink-tooltip
		role="tooltip"
		onmouseenter={onTooltipEnter}
		onmouseleave={onTooltipLeave}
		style:top="{top}px"
		style:left="{left}px"
		class="fixed z-50 max-w-[360px] w-[360px] pointer-events-auto bg-surface0 border border-surface1 rounded-md shadow-xl p-3 text-xs"
	>
		{#if preview}
			<div class="flex items-baseline gap-1.5 mb-1">
				<span class="font-semibold text-text truncate flex-1" title={preview.title}>
					{preview.title}
				</span>
				<a
					href="/notes/{encodeURIComponent(preview.path)}"
					class="text-[10px] text-secondary hover:underline flex-shrink-0"
					title="Open this note"
				>open ↗</a>
			</div>
			{#if preview.body}
				<p class="text-subtext leading-snug whitespace-pre-wrap break-words">
					{preview.body}
				</p>
			{:else}
				<p class="text-dim italic">_(this note is empty)_</p>
			{/if}
		{:else if loading}
			<p class="text-dim italic">Loading {currentTarget}…</p>
		{:else}
			<p class="text-dim italic">No note matches "{currentTarget}" yet — click to create.</p>
		{/if}
	</div>
{/if}
