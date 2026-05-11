<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';

  // Daily note widget — opens the day's daily, surfaces a body
  // preview (first 3 non-heading lines) plus a footer of ambient
  // signals (word count, jots count, tasks count) so the user can
  // tell at a glance whether they've already engaged with today's
  // note. WS reload coalesced — the editor's autosave fires on
  // every keystroke; without coalescing the widget refetches per
  // character.

  let daily = $state<Note | null>(null);
  let loading = $state(false);

  async function load() {
    loading = true;
    try {
      daily = await api.daily('today');
    } finally {
      loading = false;
    }
  }

  const reload = createCoalescedReload(load, 600);
  onMount(() => {
    void load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' && daily && ev.path === daily.path) reload.trigger();
    });
  });
  onDestroy(reload.cancel);

  // Stripped body (no frontmatter, no headings) for stats + preview.
  let strippedBody = $derived.by(() => {
    if (!daily?.body) return '';
    let body = daily.body;
    if (body.startsWith('---')) {
      const end = body.indexOf('\n---', 3);
      if (end !== -1) body = body.slice(end + 4);
    }
    return body.trim();
  });

  // Three-line preview from the first non-heading paragraphs.
  let preview = $derived.by(() => {
    if (!strippedBody) return '';
    const lines = strippedBody
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l && !l.startsWith('#') && !l.startsWith('---'))
      .slice(0, 3);
    return lines.join(' · ');
  });

  // Ambient signals — count tasks (`- [ ]` / `- [x]`), jots
  // (lines under `## Jots` heading), word count. Cheap regex pass
  // so the widget stays glance-fast.
  let stats = $derived.by(() => {
    if (!strippedBody) return { words: 0, tasks: 0, jots: 0 };
    const tasks = (strippedBody.match(/^[-*]\s+\[[ xX]\]/gm) ?? []).length;
    // Jots = lines under `## Jots` heading until the next ## heading
    let jots = 0;
    const sections = strippedBody.split(/^##\s+/m);
    for (const s of sections) {
      if (/^Jots\b/i.test(s)) {
        jots = (s.match(/^[-*]\s+\d{1,2}:\d{2}/gm) ?? s.match(/^[-*]\s+/gm) ?? []).length;
        break;
      }
    }
    const words = strippedBody.split(/\s+/).filter(Boolean).length;
    return { words, tasks, jots };
  });
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4 flex flex-col">
  <div class="flex items-baseline justify-between mb-2 gap-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Daily note</h2>
    {#if daily}
      <a href="/notes/{encodeURIComponent(daily.path)}" class="text-xs text-secondary hover:underline">edit →</a>
    {/if}
  </div>
  {#if loading && !daily}
    <div class="space-y-2">
      <div class="h-4 bg-surface1 rounded animate-pulse w-2/3"></div>
      <div class="h-3 bg-surface1 rounded animate-pulse w-3/4"></div>
      <div class="h-3 bg-surface1 rounded animate-pulse w-1/2"></div>
    </div>
  {:else if daily}
    <a href="/notes/{encodeURIComponent(daily.path)}" class="block hover:opacity-90 group">
      <div class="text-base font-medium text-text group-hover:text-primary transition-colors truncate">
        {daily.title}
      </div>
      <div class="text-[11px] text-dim font-mono truncate mt-0.5">{daily.path}</div>
      {#if preview}
        <p class="text-sm text-subtext mt-2 line-clamp-2">{preview}</p>
      {:else}
        <p class="text-sm text-dim italic mt-2">empty — tap to start writing</p>
      {/if}
    </a>
    {#if stats.words > 0 || stats.tasks > 0 || stats.jots > 0}
      <div class="flex items-baseline gap-3 mt-3 pt-2 border-t border-surface1 text-[11px] text-dim font-mono tabular-nums">
        {#if stats.words > 0}<span>{stats.words} words</span>{/if}
        {#if stats.tasks > 0}<span class="text-secondary">{stats.tasks} ☐</span>{/if}
        {#if stats.jots > 0}<span class="text-info">{stats.jots} jots</span>{/if}
      </div>
    {/if}
  {/if}
</section>
