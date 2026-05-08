<script lang="ts">
  import { tick } from 'svelte';
  import { api, type HubItem } from '$lib/api';
  import { toast } from '$lib/components/toast';

  // HubImportDialog — paste a Netscape-format bookmark export
  // (the HTML file Chrome / Firefox / Safari produce when you
  // export bookmarks) and batch-create hub entries from it.
  //
  // The Netscape format is a loose <DT><A HREF=…>title</A> tree,
  // possibly nested under <H3>folder</H3> headings. We preserve
  // the folder names as categories so a "Dev tools" folder in
  // the user's browser becomes the same category in the hub —
  // their existing organisation carries over instead of falling
  // into one big "Imported" bucket.
  //
  // The "import the world" temptation is real but disastrous: a
  // browser bookmark file usually carries hundreds of stale
  // entries. We show a checkbox per parsed link so the user
  // picks DELIBERATELY what crosses the threshold to the hub.
  // Default state: all unchecked. Bulk select-all is one click
  // away for the user who really does want everything.

  interface Props {
    open: boolean;
    onClose: () => void;
    onImported: () => void;
  }

  let { open = $bindable(false), onClose, onImported }: Props = $props();

  type Parsed = { title: string; url: string; category: string; selected: boolean };

  let html = $state('');
  let parsed = $state<Parsed[]>([]);
  let busy = $state(false);
  let progress = $state({ done: 0, total: 0 });
  let error = $state('');
  let textareaEl: HTMLTextAreaElement | undefined = $state();
  // Default-category override — when set, applies to every parsed
  // entry whose folder context is empty (rare in practice; some
  // browsers export top-level bookmarks without a containing H3).
  let fallbackCategory = $state('Imported');

  $effect(() => {
    if (open) {
      html = '';
      parsed = [];
      busy = false;
      progress = { done: 0, total: 0 };
      error = '';
      tick().then(() => textareaEl?.focus());
    }
  });

  // Parse a Netscape bookmark HTML blob. The format is:
  //   <H3>Folder name</H3>
  //   <DL><p>
  //     <DT><A HREF="https://…" ADD_DATE="…">Title</A>
  //     <DT><A HREF="https://…">Another</A>
  //     <DT><H3>Sub-folder</H3>
  //     <DL><p> … </DL>
  //   </DL>
  //
  // We don't build a real DOM — we walk regex matches in document
  // order, keeping a stack of folder names so each <A> picks up
  // its enclosing folder as its category. Matches both <H3> opens
  // and </DL> closes to push/pop the stack. Loose matching
  // tolerates the format quirks that vary across browsers (case,
  // attribute order, missing closing tags).
  function parseBookmarks(input: string): Parsed[] {
    const out: Parsed[] = [];
    const stack: string[] = [];
    // Token regex: <H3>title</H3>, </DL>, or <A HREF="…">title</A>
    const tokenRe = /<\/dl>|<h3[^>]*>([\s\S]*?)<\/h3>|<a\s[^>]*href="([^"]+)"[^>]*>([\s\S]*?)<\/a>/gi;
    let m: RegExpExecArray | null;
    while ((m = tokenRe.exec(input)) !== null) {
      const matched = m[0].toLowerCase();
      if (matched.startsWith('</dl')) {
        if (stack.length > 0) stack.pop();
        continue;
      }
      if (m[1] !== undefined) {
        // Opened a folder — push its name on the stack.
        stack.push(decodeHtml(m[1]).trim());
        continue;
      }
      if (m[2] !== undefined && m[3] !== undefined) {
        const url = decodeHtml(m[2]).trim();
        const title = decodeHtml(m[3]).replace(/<[^>]+>/g, '').trim();
        if (!url || !title) continue;
        // Skip non-http URLs (javascript: bookmarklets, file://,
        // chrome://, etc) — they wouldn't work in a vault context
        // anyway and tend to be browser-internal pollution.
        if (!/^https?:\/\//i.test(url)) continue;
        const cat = stack.length > 0 ? stack[stack.length - 1] : '';
        out.push({ title, url, category: cat, selected: false });
      }
    }
    return out;
  }

  // Minimal HTML entity decoder — covers the entities Netscape
  // exports actually use. A full DOM parser would handle every
  // edge case but adds complexity; a small decoder for the 90%
  // path is enough for bookmark imports.
  function decodeHtml(s: string): string {
    return s
      .replace(/&amp;/g, '&')
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'")
      .replace(/&#x27;/g, "'");
  }

  function preview() {
    error = '';
    const result = parseBookmarks(html);
    if (result.length === 0) {
      error = 'No bookmarks found. Paste the HTML file from your browser\'s bookmark export.';
      parsed = [];
      return;
    }
    parsed = result;
  }

  function selectAll() {
    parsed = parsed.map((p) => ({ ...p, selected: true }));
  }
  function selectNone() {
    parsed = parsed.map((p) => ({ ...p, selected: false }));
  }
  function selectCategory(cat: string) {
    parsed = parsed.map((p) => (p.category === cat ? { ...p, selected: !p.selected } : p));
  }

  // Group the parsed list by category so the user's existing
  // browser-folder structure surfaces as visual clusters in the
  // preview — same readability win as on the main hub page.
  let groups = $derived.by(() => {
    const m = new Map<string, Parsed[]>();
    for (const p of parsed) {
      const k = p.category || fallbackCategory || 'Imported';
      const arr = m.get(k) ?? [];
      arr.push(p);
      m.set(k, arr);
    }
    return [...m.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  });

  let selectedCount = $derived(parsed.filter((p) => p.selected).length);

  async function doImport() {
    const picked = parsed.filter((p) => p.selected);
    if (picked.length === 0) {
      toast.warning('Select at least one bookmark to import');
      return;
    }
    busy = true;
    error = '';
    progress = { done: 0, total: picked.length };
    let successes = 0;
    let failures = 0;
    // Sequential rather than parallel — vault writes go through
    // the same atomicio rewrite path, so concurrent createHubItem
    // calls would serialize anyway. Sequential keeps the progress
    // counter honest.
    for (const p of picked) {
      try {
        await api.createHubItem({
          title: p.title,
          url: p.url,
          category: p.category || fallbackCategory || 'Imported'
        });
        successes++;
      } catch {
        failures++;
      }
      progress = { done: progress.done + 1, total: picked.length };
    }
    busy = false;
    if (failures > 0) {
      toast.warning(`Imported ${successes}, ${failures} failed`);
    } else {
      toast.success(`Imported ${successes} bookmark${successes === 1 ? '' : 's'}`);
    }
    onImported();
    onClose();
  }
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 flex items-end sm:items-start justify-center sm:pt-12 sm:px-4 bg-mantle/70 backdrop-blur-sm"
    onclick={onClose}
    role="presentation"
  >
    <section
      class="w-full sm:max-w-2xl bg-base border border-surface1 rounded-t-xl sm:rounded-lg shadow-xl max-h-[92dvh] sm:max-h-[90vh] flex flex-col"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-label="Import browser bookmarks"
    >
      <header class="px-4 py-3 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-sm font-semibold text-text flex-1">Import browser bookmarks</h2>
        {#if parsed.length > 0}
          <span class="text-[11px] text-dim">{selectedCount} of {parsed.length} selected</span>
        {/if}
        <button
          onclick={onClose}
          aria-label="close"
          class="text-dim hover:text-text text-lg leading-none ml-2"
        >×</button>
      </header>

      <div class="flex-1 overflow-y-auto p-4 space-y-3">
        {#if parsed.length === 0}
          <p class="text-xs text-dim leading-relaxed">
            Export bookmarks from your browser as HTML, then paste the
            file's contents below. Folder names are kept as categories
            so your existing organisation carries over.
          </p>
          <details class="text-[11px] text-dim border border-surface1 rounded">
            <summary class="px-3 py-1.5 cursor-pointer hover:bg-surface0">How to export</summary>
            <div class="px-3 py-2 border-t border-surface1 space-y-1.5">
              <div><strong class="text-subtext">Chrome / Edge:</strong> Bookmarks → Bookmark manager → ⋮ → Export bookmarks</div>
              <div><strong class="text-subtext">Firefox:</strong> Library → Bookmarks → Show All → Import &amp; Backup → Export Bookmarks to HTML</div>
              <div><strong class="text-subtext">Safari:</strong> File → Export → Bookmarks</div>
            </div>
          </details>
          <textarea
            bind:this={textareaEl}
            bind:value={html}
            placeholder="Paste the HTML file's contents here…"
            class="w-full h-44 px-3 py-2 bg-surface0 border border-surface1 rounded text-xs font-mono text-text focus:outline-none focus:border-primary"
            spellcheck="false"
          ></textarea>
          {#if error}<div class="text-xs text-error">{error}</div>{/if}
          <div class="flex justify-end gap-2">
            <button
              type="button"
              onclick={preview}
              disabled={!html.trim()}
              class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50"
            >Preview</button>
          </div>
        {:else}
          <!-- Preview / select view. The user sees what's about to
               land in their hub before any write happens. -->
          <div class="flex items-center gap-2 flex-wrap">
            <button type="button" onclick={selectAll} class="text-xs px-2 py-1 bg-surface0 border border-surface1 rounded hover:border-primary">Select all</button>
            <button type="button" onclick={selectNone} class="text-xs px-2 py-1 bg-surface0 border border-surface1 rounded hover:border-primary">Select none</button>
            <span class="flex-1"></span>
            <label class="text-[11px] text-dim flex items-center gap-1.5">
              Default category for un-foldered:
              <input
                bind:value={fallbackCategory}
                class="px-2 py-0.5 bg-surface0 border border-surface1 rounded text-xs text-text w-28"
              />
            </label>
          </div>
          <div class="space-y-3">
            {#each groups as [cat, list] (cat)}
              <div class="border border-surface1 rounded">
                <header class="px-3 py-1.5 border-b border-surface1 flex items-baseline gap-2 bg-surface0/50">
                  <button
                    type="button"
                    onclick={() => selectCategory(cat === fallbackCategory && parsed.find((p) => !p.category)?.category === '' ? '' : cat)}
                    class="text-xs uppercase tracking-wider text-dim font-medium hover:text-text"
                  >{cat}</button>
                  <span class="text-[10px] text-dim/70 tabular-nums">· {list.length}</span>
                  <span class="text-[10px] text-dim/70 ml-auto tabular-nums">{list.filter((l) => l.selected).length} picked</span>
                </header>
                <ul class="divide-y divide-surface1">
                  {#each list as p (p.url)}
                    <li>
                      <label class="flex items-center gap-2 px-3 py-1.5 hover:bg-surface0 cursor-pointer">
                        <input type="checkbox" bind:checked={p.selected} class="cursor-pointer flex-shrink-0" />
                        <div class="flex-1 min-w-0">
                          <div class="text-sm text-text truncate">{p.title}</div>
                          <div class="text-[11px] text-dim font-mono truncate">{p.url}</div>
                        </div>
                      </label>
                    </li>
                  {/each}
                </ul>
              </div>
            {/each}
          </div>
          {#if busy}
            <div class="text-xs text-dim italic">
              Importing… {progress.done} / {progress.total}
            </div>
          {/if}
        {/if}
      </div>

      <footer class="px-4 py-3 border-t border-surface1 flex items-center gap-2 justify-end">
        {#if parsed.length > 0}
          <button
            type="button"
            onclick={() => { parsed = []; html = ''; }}
            disabled={busy}
            class="px-3 py-1.5 text-sm text-subtext hover:bg-surface0 rounded mr-auto disabled:opacity-50"
          >← Back</button>
          <button
            type="button"
            onclick={onClose}
            disabled={busy}
            class="px-3 py-1.5 text-sm text-subtext hover:bg-surface0 rounded disabled:opacity-50"
          >Cancel</button>
          <button
            type="button"
            onclick={doImport}
            disabled={busy || selectedCount === 0}
            class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50"
          >{busy ? `Importing ${progress.done}/${progress.total}…` : `Import ${selectedCount} bookmark${selectedCount === 1 ? '' : 's'}`}</button>
        {:else}
          <button
            type="button"
            onclick={onClose}
            class="px-3 py-1.5 text-sm text-subtext hover:bg-surface0 rounded"
          >Cancel</button>
        {/if}
      </footer>
    </section>
  </div>
{/if}
