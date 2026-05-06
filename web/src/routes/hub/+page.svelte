<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type HubItem } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // /hub — the personal launch pad. The "single login, find
  // everything I need" page the user described. Three concerns
  // collapsed into one entity (HubItem):
  //
  //   - Pure links — title + url. The day-to-day "URL of staging
  //     dashboard" / "internal admin panel" tier.
  //   - Tool entries — + category + icon. Same shape, just with
  //     organisation metadata so the page groups by category.
  //   - Non-critical credentials — + username + password. NOT a
  //     password manager replacement. The UI is honest: real
  //     secrets belong in bitwarden / 1Password / etc; this is
  //     for the convenience tier ("dev API key for service X").
  //
  // Storage at .granit/hub.json (file-system perms only, no
  // encryption — see the package comment in internal/hub).

  let items = $state<HubItem[]>([]);
  let loading = $state(false);
  let q = $state('');
  let categoryFilter = $state('');

  // Add / edit modal state. editing = null means "create"; an
  // HubItem instance means "edit this".
  let modalOpen = $state(false);
  let editing = $state<HubItem | null>(null);

  // Form buffers — bound to the modal inputs so cancel cleanly
  // discards without mutating the on-disk record.
  let fTitle = $state('');
  let fUrl = $state('');
  let fCategory = $state('');
  let fIcon = $state('');
  let fNotes = $state('');
  let fUsername = $state('');
  let fPassword = $state('');
  let fFavorite = $state(false);
  let saving = $state(false);

  // Per-card "show password" toggle. Map keyed by item ID so
  // expanding one credential doesn't reveal others.
  let revealed = $state<Set<string>>(new Set());

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const r = await api.listHubItems();
      items = r.items;
    } catch (e) {
      toast.error('failed to load hub: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/hub.json') load();
    });
  });

  // Categories with counts, sorted by frequency desc — the most-
  // used categories surface first in the chip row. Items without
  // a category land under "Other".
  let categories = $derived.by(() => {
    const m = new Map<string, number>();
    for (const it of items) {
      const c = (it.category ?? '').trim() || 'Other';
      m.set(c, (m.get(c) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]);
  });

  // Filtered + grouped view. Search matches title / url / category /
  // notes / username (NOT password — that would surface secrets via
  // the search field). Category filter narrows to a single bucket.
  let visibleItems = $derived.by(() => {
    let out = items;
    if (categoryFilter) {
      const cf = categoryFilter.toLowerCase();
      out = out.filter((it) => (it.category ?? 'Other').toLowerCase() === cf);
    }
    const term = q.trim().toLowerCase();
    if (term) {
      out = out.filter((it) =>
        it.title.toLowerCase().includes(term) ||
        (it.url ?? '').toLowerCase().includes(term) ||
        (it.category ?? '').toLowerCase().includes(term) ||
        (it.notes ?? '').toLowerCase().includes(term) ||
        (it.username ?? '').toLowerCase().includes(term)
      );
    }
    return out;
  });

  // Group the visible items by category so the page reads as
  // clusters rather than a flat list. Favorites stay pinned across
  // all groups by sorting them to the top of each bucket.
  type Group = { key: string; items: HubItem[] };
  let grouped = $derived.by((): Group[] => {
    const m = new Map<string, HubItem[]>();
    for (const it of visibleItems) {
      const cat = (it.category ?? '').trim() || 'Other';
      const arr = m.get(cat) ?? [];
      arr.push(it);
      m.set(cat, arr);
    }
    const out: Group[] = [];
    for (const [key, list] of m) {
      list.sort((a, b) => {
        if (!!a.favorite !== !!b.favorite) return a.favorite ? -1 : 1;
        return a.title.localeCompare(b.title);
      });
      out.push({ key, items: list });
    }
    out.sort((a, b) => {
      // Other always last — known categories before the catch-all
      if (a.key === 'Other' && b.key !== 'Other') return 1;
      if (b.key === 'Other' && a.key !== 'Other') return -1;
      return a.key.localeCompare(b.key);
    });
    return out;
  });

  function openCreate() {
    editing = null;
    fTitle = '';
    fUrl = '';
    fCategory = '';
    fIcon = '';
    fNotes = '';
    fUsername = '';
    fPassword = '';
    fFavorite = false;
    modalOpen = true;
  }

  function openEdit(it: HubItem) {
    editing = it;
    fTitle = it.title;
    fUrl = it.url ?? '';
    fCategory = it.category ?? '';
    fIcon = it.icon ?? '';
    fNotes = it.notes ?? '';
    fUsername = it.username ?? '';
    fPassword = it.password ?? '';
    fFavorite = !!it.favorite;
    modalOpen = true;
  }

  async function save() {
    if (!fTitle.trim()) {
      toast.warning('title is required');
      return;
    }
    saving = true;
    const payload: Partial<HubItem> = {
      title: fTitle.trim(),
      url: fUrl.trim() || undefined,
      category: fCategory.trim() || undefined,
      icon: fIcon.trim() || undefined,
      notes: fNotes.trim() || undefined,
      username: fUsername.trim() || undefined,
      password: fPassword || undefined,
      favorite: fFavorite || undefined
    };
    try {
      if (editing) {
        await api.patchHubItem(editing.id, payload);
        toast.success('updated');
      } else {
        await api.createHubItem(payload);
        toast.success('added to hub');
      }
      modalOpen = false;
      await load();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      saving = false;
    }
  }

  async function remove(it: HubItem) {
    if (!confirm(`Remove "${it.title}" from the hub?`)) return;
    try {
      await api.deleteHubItem(it.id);
      await load();
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function toggleFavorite(it: HubItem) {
    try {
      await api.patchHubItem(it.id, { favorite: !it.favorite });
      await load();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  function toggleReveal(id: string) {
    const next = new Set(revealed);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    revealed = next;
  }

  async function copyValue(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label} copied`);
    } catch {
      toast.error(`copy failed (clipboard blocked?)`);
    }
  }

  // First-character fallback when the user didn't pick an icon.
  // Stable per-title so the same item always shows the same letter.
  function fallbackIcon(it: HubItem): string {
    if (it.icon?.trim()) return it.icon.trim();
    return (it.title.trim().charAt(0) || '·').toUpperCase();
  }

  // Hostname extraction for the secondary line on link cards.
  // Falls back to the raw URL if parsing fails.
  function displayHost(url: string): string {
    try {
      const u = new URL(url);
      return u.hostname.replace(/^www\./, '');
    } catch {
      return url;
    }
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    <PageHeader
      title="Hub"
      subtitle="Quick-access links, tools, and small credentials — your single login to the things you use every day."
    >
      {#snippet actions()}
        <button
          onclick={openCreate}
          class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
        >+ Add to hub</button>
      {/snippet}
    </PageHeader>

    <!-- Search + category chips -->
    <div class="space-y-3 mb-6">
      <input
        bind:value={q}
        placeholder="search title, url, notes, username…"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      {#if categories.length > 0}
        <div class="flex flex-wrap gap-1.5">
          <button
            onclick={() => (categoryFilter = '')}
            class="px-2.5 py-0.5 text-xs rounded {categoryFilter === '' ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
          >All <span class="opacity-70 ml-0.5">{items.length}</span></button>
          {#each categories as [c, n]}
            <button
              onclick={() => (categoryFilter = categoryFilter === c ? '' : c)}
              class="px-2.5 py-0.5 text-xs rounded {categoryFilter === c ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            >{c} <span class="opacity-70 ml-0.5">{n}</span></button>
          {/each}
        </div>
      {/if}
    </div>

    {#if loading && items.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if items.length === 0}
      <!-- Empty state — encourages getting started without overwhelming. -->
      <div class="bg-surface0 border border-surface1 rounded-lg p-6 text-center">
        <p class="text-sm text-text mb-2">Your hub is empty.</p>
        <p class="text-xs text-dim mb-4 max-w-md mx-auto">
          Add the URLs you reach for every day — staging dashboards, internal tools,
          docs you re-read, the SaaS you live in. Categorise them so the page reads
          as clusters instead of a wall of links.
        </p>
        <button
          onclick={openCreate}
          class="px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
        >+ Add your first entry</button>
      </div>
    {:else if visibleItems.length === 0}
      <div class="text-sm text-dim italic">No matches.</div>
    {:else}
      <div class="space-y-6">
        {#each grouped as g (g.key)}
          <section>
            <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">
              {g.key} <span class="opacity-60 tabular-nums">· {g.items.length}</span>
            </h2>
            <ul class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
              {#each g.items as it (it.id)}
                {@const hasCred = !!(it.username || it.password)}
                {@const isRevealed = revealed.has(it.id)}
                <li class="bg-surface0 border border-surface1 rounded-lg overflow-hidden hover:border-primary/40 transition-colors group">
                  <div class="p-3 flex items-start gap-2.5">
                    <!-- Icon column. Falls back to the title's first
                         letter when the user didn't pick an icon. -->
                    <div class="w-9 h-9 flex-shrink-0 rounded bg-surface1 flex items-center justify-center text-sm font-medium text-text">
                      {fallbackIcon(it)}
                    </div>
                    <div class="flex-1 min-w-0">
                      <div class="flex items-baseline gap-1.5">
                        {#if it.url}
                          <a
                            href={it.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            class="text-sm text-text font-medium truncate hover:text-primary"
                          >{it.title}</a>
                        {:else}
                          <span class="text-sm text-text font-medium truncate">{it.title}</span>
                        {/if}
                        {#if it.favorite}
                          <span class="text-warning flex-shrink-0" title="favorite" aria-label="favorite">★</span>
                        {/if}
                      </div>
                      {#if it.url}
                        <div class="text-[11px] text-dim font-mono truncate">{displayHost(it.url)}</div>
                      {/if}
                      {#if it.notes}
                        <p class="text-[11px] text-subtext mt-1 line-clamp-2">{it.notes}</p>
                      {/if}
                      {#if hasCred}
                        <div class="mt-2 pt-2 border-t border-surface1 space-y-1">
                          {#if it.username}
                            <div class="flex items-baseline gap-2 text-[11px]">
                              <span class="text-dim w-12 flex-shrink-0">user</span>
                              <span class="text-text font-mono truncate flex-1 min-w-0">{it.username}</span>
                              <button
                                type="button"
                                onclick={() => copyValue(it.username ?? '', 'username')}
                                title="copy username"
                                class="text-dim hover:text-primary opacity-0 group-hover:opacity-100"
                              >⧉</button>
                            </div>
                          {/if}
                          {#if it.password}
                            <div class="flex items-baseline gap-2 text-[11px]">
                              <span class="text-dim w-12 flex-shrink-0">pass</span>
                              <span class="text-text font-mono truncate flex-1 min-w-0">
                                {isRevealed ? it.password : '••••••••'}
                              </span>
                              <button
                                type="button"
                                onclick={() => toggleReveal(it.id)}
                                title={isRevealed ? 'hide password' : 'show password'}
                                class="text-dim hover:text-primary"
                              >{isRevealed ? '◌' : '◎'}</button>
                              <button
                                type="button"
                                onclick={() => copyValue(it.password ?? '', 'password')}
                                title="copy password"
                                class="text-dim hover:text-primary opacity-0 group-hover:opacity-100"
                              >⧉</button>
                            </div>
                          {/if}
                        </div>
                      {/if}
                    </div>
                    <!-- Action menu — favorite + edit + delete. Hidden
                         until card hover so the resting state is clean. -->
                    <div class="flex flex-col gap-0.5 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button
                        onclick={() => toggleFavorite(it)}
                        title={it.favorite ? 'unfavorite' : 'favorite'}
                        aria-label={it.favorite ? 'unfavorite' : 'favorite'}
                        class="text-dim hover:text-warning text-xs leading-none w-5 h-5"
                      >{it.favorite ? '★' : '☆'}</button>
                      <button
                        onclick={() => openEdit(it)}
                        title="edit"
                        aria-label="edit"
                        class="text-dim hover:text-text text-xs leading-none w-5 h-5"
                      >✎</button>
                      <button
                        onclick={() => remove(it)}
                        title="delete"
                        aria-label="delete"
                        class="text-dim hover:text-error text-xs leading-none w-5 h-5"
                      >×</button>
                    </div>
                  </div>
                </li>
              {/each}
            </ul>
          </section>
        {/each}
      </div>
    {/if}

    <footer class="mt-10 pt-4 border-t border-surface1 text-[11px] text-dim">
      Stored in <code class="font-mono">.granit/hub.json</code> with file-system
      restricted access (0o600). Credentials are saved in plain text — use
      <a href="https://bitwarden.com" target="_blank" rel="noopener noreferrer" class="text-secondary hover:underline">Bitwarden</a>
      or another password manager for sensitive secrets. The hub is for the
      convenience tier (URLs, internal tools, low-risk creds).
    </footer>
  </div>
</div>

<!-- Add / edit modal -->
{#if modalOpen}
  <div
    class="fixed inset-0 z-50 flex items-start justify-center pt-12 px-4 bg-mantle/70 backdrop-blur-sm"
    onclick={() => (modalOpen = false)}
    role="presentation"
  >
    <form
      onsubmit={(e) => { e.preventDefault(); save(); }}
      class="w-full max-w-md bg-base border border-surface1 rounded-lg shadow-xl max-h-[90vh] flex flex-col"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-label={editing ? 'Edit hub item' : 'Add to hub'}
    >
      <header class="px-4 py-3 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-sm font-semibold text-text flex-1">{editing ? 'Edit hub item' : 'Add to hub'}</h2>
        <button
          type="button"
          onclick={() => (modalOpen = false)}
          aria-label="close"
          class="text-dim hover:text-text text-lg leading-none"
        >×</button>
      </header>

      <div class="p-4 space-y-3 overflow-y-auto">
        <div>
          <label for="hub-title" class="block text-xs uppercase tracking-wider text-dim mb-1">Title</label>
          <input
            id="hub-title"
            bind:value={fTitle}
            required
            autofocus
            placeholder="e.g. Staging dashboard"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          />
        </div>
        <div>
          <label for="hub-url" class="block text-xs uppercase tracking-wider text-dim mb-1">URL <span class="text-dim/70 normal-case">(optional)</span></label>
          <input
            id="hub-url"
            bind:value={fUrl}
            type="url"
            placeholder="https://…"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary font-mono"
          />
        </div>
        <div class="grid grid-cols-2 gap-2">
          <div>
            <label for="hub-category" class="block text-xs uppercase tracking-wider text-dim mb-1">Category</label>
            <input
              id="hub-category"
              bind:value={fCategory}
              placeholder="Dev / Internal / SaaS …"
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
            />
          </div>
          <div>
            <label for="hub-icon" class="block text-xs uppercase tracking-wider text-dim mb-1">Icon</label>
            <input
              id="hub-icon"
              bind:value={fIcon}
              placeholder="🐙"
              maxlength="4"
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary text-center"
            />
          </div>
        </div>
        <div>
          <label for="hub-notes" class="block text-xs uppercase tracking-wider text-dim mb-1">Notes</label>
          <textarea
            id="hub-notes"
            bind:value={fNotes}
            rows="2"
            placeholder="What is this for?"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          ></textarea>
        </div>

        <!-- Credential block — collapsed visual emphasis until the
             user starts filling it. Honest about the security model. -->
        <details class="border border-surface1 rounded">
          <summary class="px-3 py-2 cursor-pointer text-xs uppercase tracking-wider text-dim hover:bg-surface0">
            Optional credentials
            <span class="text-dim/60 normal-case">— for low-risk values only</span>
          </summary>
          <div class="p-3 space-y-3 border-t border-surface1">
            <div class="text-[11px] text-warning bg-warning/5 border border-warning/30 rounded px-2 py-1.5">
              ⚠ Stored as plain text. Use <a href="https://bitwarden.com" target="_blank" rel="noopener noreferrer" class="underline">Bitwarden</a> or 1Password for real secrets.
            </div>
            <div>
              <label for="hub-user" class="block text-xs uppercase tracking-wider text-dim mb-1">Username</label>
              <input
                id="hub-user"
                bind:value={fUsername}
                autocomplete="off"
                placeholder=""
                class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label for="hub-pass" class="block text-xs uppercase tracking-wider text-dim mb-1">Password</label>
              <input
                id="hub-pass"
                bind:value={fPassword}
                type="text"
                autocomplete="off"
                placeholder=""
                class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary font-mono"
              />
            </div>
          </div>
        </details>

        <label class="flex items-center gap-2 text-sm text-text cursor-pointer">
          <input type="checkbox" bind:checked={fFavorite} class="cursor-pointer" />
          <span>Favorite (pinned to top)</span>
        </label>
      </div>

      <footer class="px-4 py-3 border-t border-surface1 flex items-center gap-2 justify-end">
        {#if editing}
          <button
            type="button"
            onclick={() => { remove(editing!); modalOpen = false; }}
            class="px-3 py-1.5 text-sm text-error hover:bg-error/10 rounded mr-auto"
          >Delete</button>
        {/if}
        <button
          type="button"
          onclick={() => (modalOpen = false)}
          class="px-3 py-1.5 text-sm text-subtext hover:bg-surface0 rounded"
        >Cancel</button>
        <button
          type="submit"
          disabled={saving || !fTitle.trim()}
          class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50"
        >{saving ? 'saving…' : editing ? 'Save' : 'Add'}</button>
      </footer>
    </form>
  </div>
{/if}
