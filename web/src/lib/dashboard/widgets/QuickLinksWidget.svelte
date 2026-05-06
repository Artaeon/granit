<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type HubItem } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // QuickLinksWidget — surface the user's hub favorites on the
  // home dashboard so the most-used links are one click from the
  // morning view. Span 1 — the widget is meant to fit alongside
  // Now / Streaks / Top deadlines, not dominate. Caps at 5 items
  // because the dashboard is glance-fast: more than 5 turns into
  // a list and the user is better served opening /hub directly.
  //
  // Visit clicks stamp last_visited_at via the existing /visit
  // endpoint so the hub page's "recently used" cue stays accurate
  // even when the user lives in the dashboard.

  let items = $state<HubItem[] | null>(null);
  let loaded = $state(false);

  async function load() {
    try {
      const r = await api.listHubItems();
      items = r.items.filter((it) => !!it.favorite);
    } catch {
      items = [];
    } finally {
      loaded = true;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/hub.json') load();
    });
  });

  let visible = $derived(items ? items.slice(0, 5) : []);
  let extra = $derived(items ? Math.max(0, items.length - 5) : 0);

  function fallbackIcon(it: HubItem): string {
    if (it.icon?.trim()) return it.icon.trim();
    return (it.title.trim().charAt(0) || '·').toUpperCase();
  }

  function faviconUrl(it: HubItem, size = 32): string | null {
    if (it.icon?.trim()) return null;
    if (!it.url) return null;
    try {
      const u = new URL(it.url);
      return `https://www.google.com/s2/favicons?domain=${encodeURIComponent(u.hostname)}&sz=${size}`;
    } catch {
      return null;
    }
  }

  function open(it: HubItem) {
    if (!it.url) return;
    window.open(it.url, '_blank', 'noopener,noreferrer');
    // Fire-and-forget visit stamp so /hub's "recently used" cue
    // reflects dashboard clicks too. A failure here doesn't matter
    // — the user got to their destination.
    void api.visitHubItem(it.id).catch(() => {});
  }
</script>

{#if loaded}
  <section class="bg-surface0 border border-surface1 rounded-lg p-4">
    <div class="flex items-baseline justify-between mb-3">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Quick links</h2>
      <a href="/hub" class="text-xs text-secondary hover:underline">all →</a>
    </div>
    {#if visible.length === 0}
      <div class="text-sm text-dim italic">
        Star items in <a href="/hub" class="text-secondary hover:underline">Hub</a> to surface them here.
      </div>
    {:else}
      <ul class="space-y-1.5">
        {#each visible as it (it.id)}
          {@const fav = faviconUrl(it)}
          <li>
            <button
              type="button"
              onclick={() => open(it)}
              class="w-full text-left px-2 py-1.5 rounded hover:bg-surface1 flex items-center gap-2.5 group"
              aria-label={it.url ? `Open ${it.title}` : it.title}
            >
              <div class="w-6 h-6 flex-shrink-0 rounded bg-mantle/40 flex items-center justify-center text-[11px] font-medium text-text overflow-hidden">
                {#if fav}
                  <img
                    src={fav}
                    alt=""
                    class="w-4 h-4"
                    loading="lazy"
                    onerror={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none'; }}
                  />
                {:else}
                  {fallbackIcon(it)}
                {/if}
              </div>
              <div class="flex-1 min-w-0">
                <div class="text-sm text-text truncate group-hover:text-primary">{it.title}</div>
                {#if it.category}
                  <div class="text-[10px] text-dim truncate">{it.category}</div>
                {/if}
              </div>
              {#if it.url}
                <span class="text-dim text-xs opacity-0 group-hover:opacity-100" aria-hidden="true">↗</span>
              {/if}
            </button>
          </li>
        {/each}
      </ul>
      {#if extra > 0}
        <a href="/hub" class="block text-xs text-secondary hover:underline mt-2 text-center">
          + {extra} more favorite{extra === 1 ? '' : 's'} →
        </a>
      {/if}
    {/if}
  </section>
{/if}
