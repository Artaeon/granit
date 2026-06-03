<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // BacklinksPanel — right-rail surface in the notes editor showing
  // every note that wikilinks INTO the current one. Each row carries
  // the source title PLUS the per-mention context (line number +
  // inline snippet centred on the link) so the user reads the
  // backlink the way they'd read a citation: "where is this referenced
  // and what was being said around it" — not just a flat list of
  // titles. Clicking a context navigates to the source and scrolls to
  // that exact line via the ?line=N query param the notes page reads
  // on mount.
  //
  // WS-driven freshness: any note.changed could rewrite a wikilink to
  // or from the current note. Refetching on every note write would
  // burn requests, so we coalesce with a small trailing-edge debounce
  // and only fire when the panel is mounted (the editor route owns
  // this component — unmount → no refetch).

  let { path, onNavigate }: { path: string; onNavigate?: (target: string) => void } = $props();

  // Backwards-compatible response shape. `contexts` is the new
  // server-emitted field (handlers_notes.go::findBacklinkContexts);
  // pre-this-build vaults / older deployments may return entries
  // without it, in which case the panel falls back to the bare title
  // list it always rendered before.
  interface BacklinkContext {
    line: number;
    snippet: string;
  }
  interface BacklinkEntry {
    path: string;
    title: string;
    contexts?: BacklinkContext[];
  }
  interface LinksData {
    outgoing: string[];
    backlinks: BacklinkEntry[];
  }

  let data = $state<LinksData | null>(null);
  let loading = $state(false);
  // Surfaced when load() rejects with a non-empty result vs. when the
  // server genuinely reports zero backlinks. The previous empty-state
  // ("No notes link here yet.") fired on transient backend failures
  // too, hiding the error and looking like a successful empty query.
  let loadError = $state<string | null>(null);
  // Per-source expansion state. By default the first source is open;
  // subsequent ones collapse with an aggregate count so the panel
  // doesn't dominate the rail on hub-style notes. Tracked by path so
  // toggling survives a reload that reorders sources.
  let expanded = $state<Record<string, boolean>>({});

  // Generation counter — every load() call captures the current value
  // and only commits if it's still the latest when the response
  // arrives. Rapid note switching can land an old fetch into the new
  // note's UI; the counter guards against it. Same pattern as the
  // chat-stream / inline-AI / preview-render generation guards.
  let loadGen = 0;

  async function load() {
    if (!path) return;
    const myGen = ++loadGen;
    loading = true;
    try {
      const fresh = await api.req<LinksData>(`/links/${encodeURI(path)}`);
      if (myGen !== loadGen) return;
      data = fresh;
      loadError = null;
      // Auto-expand the first source so the user sees the most-recent
      // context immediately. Subsequent sources stay collapsed until
      // tapped — the rail isn't infinite vertical real estate.
      if (data?.backlinks?.length && expanded[data.backlinks[0].path] === undefined) {
        expanded = { ...expanded, [data.backlinks[0].path]: true };
      }
    } catch (e) {
      if (myGen !== loadGen) return;
      // Don't wipe the previous data on a transient failure — the user
      // keeps seeing what they had + an error chip with a retry path.
      // Empty-state fallback was the silent-failure-as-no-backlinks
      // bug.
      loadError = e instanceof Error ? e.message : 'failed to load';
    } finally {
      if (myGen === loadGen) loading = false;
    }
  }

  // Coalesce note.changed bursts (one event per editor autosave keystroke
  // on the current note OR any source). A trailing-edge debounce: keystrokes
  // queue a single refetch 400ms after the last note.changed.
  let reloadTimer: ReturnType<typeof setTimeout> | undefined;
  function scheduleReload() {
    if (reloadTimer) clearTimeout(reloadTimer);
    reloadTimer = setTimeout(() => {
      reloadTimer = undefined;
      void load();
    }, 400);
  }

  $effect(() => {
    void path;
    load();
  });

  onMount(() =>
    onWsEvent((ev) => {
      // note.changed: could be a source rewriting its wikilink to us,
      // or the current note's title changing (which would change
      // resolution rules globally). Either way, refetch.
      // note.removed: a source vanished → drop its entry from the panel.
      if (ev.type === 'note.changed' || ev.type === 'note.removed') {
        scheduleReload();
      }
    })
  );

  function toggle(srcPath: string) {
    expanded = { ...expanded, [srcPath]: !expanded[srcPath] };
  }

  // Total mention count across all sources — drives the rail heading
  // and lets a power user gauge "how connected" the current note is
  // at a glance.
  let mentionCount = $derived.by(() => {
    if (!data?.backlinks) return 0;
    let n = 0;
    for (const bl of data.backlinks) {
      n += bl.contexts?.length ?? 1;
    }
    return n;
  });

  function backlinkHref(srcPath: string, line?: number): string {
    const base = `/notes/${encodeURIComponent(srcPath)}`;
    return line ? `${base}?line=${line}` : base;
  }

  // Title hint for the empty-state code example. Strips the directory
  // and `.md` extension so the user sees the same string they'd type
  // inside `[[...]]` from another note. Falls back to "this note" on
  // the (unusual) empty-path edge case so the hint still reads.
  let emptyStateTitleHint = $derived.by(() => {
    if (!path) return 'this note';
    const base = path.split('/').pop() ?? path;
    return base.replace(/\.md$/, '');
  });
</script>

{#if loadError && !data}
  <div class="text-xs px-2 py-1 flex items-center gap-2">
    <span class="text-error">⚠</span>
    <span class="flex-1 text-error/90 truncate" title={loadError}>Couldn't load backlinks.</span>
    <button
      type="button"
      onclick={() => void load()}
      class="px-1.5 py-0.5 text-[10px] text-dim hover:text-text border border-surface1 rounded"
    >retry</button>
  </div>
{:else if loading && !data}
  <div class="text-xs text-dim italic px-2 py-1">loading…</div>
{:else if data}
  {#if loadError}
    <!-- Soft error: we still have prior data to show, but the latest
         refetch failed. A thin inline chip surfaces it without
         clobbering the panel. -->
    <div class="text-[10px] text-error/80 px-2 py-1 mb-1 flex items-center gap-1.5">
      <span aria-hidden="true">⚠</span>
      <span class="flex-1 truncate" title={loadError}>refresh failed (showing previous results)</span>
      <button
        type="button"
        onclick={() => void load()}
        class="text-dim hover:text-text underline-offset-2 hover:underline"
      >retry</button>
    </div>
  {/if}
  {#if data.backlinks.length > 0}
    {#if mentionCount > data.backlinks.length}
      <!-- Show aggregate only when contexts add detail beyond the
           source count (e.g. 3 sources, 7 mentions). On per-source
           equality the count is redundant. -->
      <div class="text-[10px] uppercase tracking-wider text-dim mb-2 px-2 tabular-nums">
        {mentionCount} mention{mentionCount === 1 ? '' : 's'} across {data.backlinks.length} note{data.backlinks.length === 1 ? '' : 's'}
      </div>
    {/if}
    <ul class="space-y-2">
      {#each data.backlinks as bl (bl.path)}
        {@const ctxs = bl.contexts ?? []}
        {@const isOpen = expanded[bl.path] ?? false}
        <li class="text-sm">
          <!-- Source header. Click toggles the context list open/closed.
               When the source has no contexts at all (legacy response
               or an exotic resolution path where the regex missed),
               the row degrades to the original "click to navigate"
               behaviour so the user still has a way through. -->
          {#if ctxs.length > 0}
            <button
              type="button"
              onclick={() => toggle(bl.path)}
              class="w-full flex items-center gap-1.5 px-2 py-1 rounded text-left text-text hover:bg-surface0 transition-colors"
              aria-expanded={isOpen}
            >
              <svg viewBox="0 0 24 24" class="w-3 h-3 text-dim flex-shrink-0 transition-transform {isOpen ? 'rotate-90' : ''}" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="9 6 15 12 9 18"/>
              </svg>
              <span class="flex-1 min-w-0 truncate">← {bl.title || bl.path}</span>
              {#if ctxs.length > 1}
                <span class="text-[10px] text-dim font-mono tabular-nums flex-shrink-0">{ctxs.length}</span>
              {/if}
            </button>
            {#if isOpen}
              <ul class="ml-4 mt-0.5 space-y-0.5 border-l border-surface1 pl-2.5">
                <!-- Key by line+index, not bare line: two `[[Target]]`
                     mentions on the same source line produce duplicate
                     ctx.line values from findBacklinkContexts, which
                     Svelte's keyed-each rejects as a duplicate key
                     and either throws or silently drops a row. -->
                {#each ctxs as ctx, i (`${ctx.line}:${i}`)}
                  <li>
                    <a
                      href={backlinkHref(bl.path, ctx.line)}
                      class="block px-1.5 py-1 rounded text-xs text-subtext hover:bg-surface0 hover:text-text transition-colors leading-snug"
                      title="open {bl.title} at line {ctx.line}"
                    >
                      <span class="text-[10px] text-dim font-mono tabular-nums mr-1.5">L{ctx.line}</span>
                      <span class="text-text/80">{ctx.snippet}</span>
                    </a>
                  </li>
                {/each}
              </ul>
            {/if}
          {:else}
            <a
              href={backlinkHref(bl.path)}
              class="block px-2 py-1 text-text hover:bg-surface0 rounded truncate transition-colors"
            >
              ← {bl.title || bl.path}
            </a>
          {/if}
        </li>
      {/each}
    </ul>
  {:else}
    <div class="text-xs text-dim italic px-2 py-1 leading-relaxed">
      No notes link here yet.
      <span class="block text-[11px] mt-1 text-dim/70">
        Write <code class="font-mono text-[10px] bg-surface0 px-1 py-0.5 rounded">[[{emptyStateTitleHint}]]</code> in another note to create a backlink.
      </span>
    </div>
  {/if}
  {#if data.outgoing.length > 0}
    <div class="text-[10px] uppercase tracking-wider text-dim mt-4 mb-1 px-2">
      Outgoing
      <span class="text-dim/70 tabular-nums normal-case tracking-normal">· {data.outgoing.length}</span>
    </div>
    <ul class="space-y-px">
      {#each data.outgoing as link (link)}
        <li>
          <button
            type="button"
            onclick={() => onNavigate?.(link)}
            class="block w-full text-left px-2 py-1 text-sm text-secondary hover:bg-surface0 rounded truncate transition-colors"
          >
            → {link}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
{/if}
