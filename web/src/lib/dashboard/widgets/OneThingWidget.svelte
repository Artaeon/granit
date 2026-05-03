<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // OneThingWidget surfaces the "Next week's one thing" answer from
  // the user's most recent weekly review. Closes the loop between
  // the review ritual and the rest of the week — Sunday's
  // commitment becomes Monday morning's first sight.
  //
  // Reads from Reviews/*.md (the same files /review writes), parses
  // out the canonical heading, displays the prose. Empty when there
  // are no reviews yet — points at /review with a single CTA.
  //
  // Uses the existing notes API; no new endpoint or state required.
  // The folder filter on listNotes keeps the call cheap (one tight
  // call to one folder, limit 1, sorted newest first).

  let oneThing = $state<string>('');
  let reviewPath = $state<string>('');
  let reviewWeek = $state<string>('');
  let loaded = $state(false);

  // Extract the "Next week's one thing" section from a review
  // markdown body. Mirror of the parser in /review's page so a
  // section the form rendered is the same one this widget reads.
  function extractOneThing(body: string): string {
    // The canonical heading is the longest of the five — anchor
    // on it explicitly rather than scanning all headings.
    const match = body.match(
      /^##\s+Next week['’]?s one thing\s*$([\s\S]*?)(?=\n##\s+|$)/m
    );
    if (!match) return '';
    const text = match[1].trim();
    // Empty-section sentinel from /review's encoder.
    if (text === '_(empty)_') return '';
    return text;
  }

  async function load() {
    try {
      const list = await api.listNotes({ folder: 'Reviews', limit: 1 });
      const note = list.notes[0];
      if (!note) {
        oneThing = '';
        loaded = true;
        return;
      }
      // listNotes is metadata-only — fetch the body for parsing.
      const full = await api.getNote(note.path);
      oneThing = extractOneThing(full.body ?? '');
      reviewPath = note.path;
      reviewWeek = note.path.match(/(\d{4}-W\d{2})\.md$/)?.[1] ?? '';
      loaded = true;
    } catch {
      // Soft-fail: a missing Reviews/ folder isn't an error, it's
      // just "no reviews yet" — the empty state below handles it.
      oneThing = '';
      loaded = true;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' && ev.path?.startsWith('Reviews/')) load();
      if (ev.type === 'note.removed' && ev.path?.startsWith('Reviews/')) load();
    });
  });
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4 hover:border-primary/40 transition-colors">
  {#if !loaded}
    <div class="text-xs text-dim">loading…</div>
  {:else if oneThing}
    <a href="/review" class="block group">
      <div class="flex items-baseline gap-2 mb-2">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium">This week's one thing</h2>
        <span class="flex-1"></span>
        {#if reviewWeek}
          <span class="text-[11px] text-dim font-mono">from {reviewWeek}</span>
        {/if}
        <span class="text-xs text-dim group-hover:text-primary transition-colors">review →</span>
      </div>
      <p class="text-base text-text leading-snug">
        {oneThing}
      </p>
    </a>
  {:else}
    <div class="flex items-center gap-3">
      <span class="text-2xl">🎯</span>
      <div class="flex-1 min-w-0">
        <p class="text-sm text-text font-medium">No weekly commitment yet</p>
        <p class="text-xs text-dim">Run the weekly review to set what you'll focus on this week.</p>
      </div>
      <a href="/review" class="text-xs px-3 py-1.5 bg-primary text-on-primary rounded font-medium hover:opacity-90 flex-shrink-0">
        Start review →
      </a>
    </div>
  {/if}
</section>
