<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { api, type EmailSignature } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // Email signature library. List on the left, editor + live
  // iframe-sandboxed preview on the right (stacks on mobile).
  // Copy button puts the HTML on the clipboard so you can paste
  // it into your mail client's signature field. The default
  // marker promotes one signature visually so the eye lands
  // there first.

  let signatures = $state<EmailSignature[]>([]);
  let loading = $state(false);
  let selectedId = $state<string | null>(null);
  let q = $state('');
  let filterCategory = $state('');

  // Edit buffer — when selectedId is set we mirror the chosen
  // signature here so edits don't write through on every
  // keystroke. Save explicitly via the form's submit button.
  let buf = $state<EmailSignature | null>(null);
  let dirty = $state(false);

  let selected = $derived(signatures.find((s) => s.id === selectedId) ?? null);
  let nameInputEl: HTMLInputElement | undefined = $state();

  // Re-seed buffer ONLY when selectedId changes. Reading `signatures`
  // tracks it as a dep, which means a WS-driven reload (firing on
  // every save / on another device's edit) would re-seed buf and
  // clobber the user's in-progress typing — that's the "auto
  // closes" feeling: the editor visibly resets to server state.
  // untrack() reads signatures without subscribing, so the effect
  // only re-fires on real user-driven selection changes.
  $effect(() => {
    const id = selectedId;
    untrack(() => {
      const found = signatures.find((s) => s.id === id) ?? null;
      buf = found ? { ...found } : null;
      dirty = false;
    });
  });

  function markDirty() { dirty = true; }

  let categories = $derived.by(() => {
    const set = new Set<string>();
    for (const s of signatures) {
      if (s.category) set.add(s.category);
    }
    return [...set].sort();
  });

  let filtered = $derived.by(() => {
    const needle = q.trim().toLowerCase();
    return signatures.filter((s) => {
      if (filterCategory && (s.category ?? '') !== filterCategory) return false;
      if (!needle) return true;
      return (
        s.name.toLowerCase().includes(needle) ||
        (s.category ?? '').toLowerCase().includes(needle) ||
        s.html.toLowerCase().includes(needle)
      );
    });
  });

  async function load() {
    loading = true;
    try {
      const r = await api.listEmailSignatures();
      signatures = r.signatures;
    } catch (e) {
      toast.error('failed to load: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/email-signatures.json') load();
    });
  });

  async function createNew() {
    try {
      const created = await api.createEmailSignature({
        name: 'Untitled signature',
        html: '<p>Best regards,<br/>Your Name</p>',
        plain_text: 'Best regards,\nYour Name'
      });
      await load();
      selectedId = created.id;
      // Defer focus + select-all so the user lands in "name the
      // signature" mode immediately. Without this, the editor
      // appeared briefly and the user saw a placeholder name —
      // looked like the page hadn't done anything.
      tick().then(() => {
        nameInputEl?.focus();
        nameInputEl?.select();
      });
    } catch (e) {
      toast.error('create failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function saveBuf() {
    if (!buf || !selectedId) return;
    try {
      await api.patchEmailSignature(selectedId, {
        name: buf.name,
        html: buf.html,
        plain_text: buf.plain_text,
        category: buf.category,
        is_default: buf.is_default
      });
      dirty = false;
      await load();
      toast.success('saved');
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function setDefault(id: string) {
    try {
      await api.patchEmailSignature(id, { is_default: true });
      await load();
      toast.success('default updated');
    } catch (e) {
      toast.error('update failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function removeSelected() {
    if (!selectedId) return;
    if (!confirm(`Delete "${selected?.name ?? 'this signature'}"? This cannot be undone.`)) return;
    try {
      await api.deleteEmailSignature(selectedId);
      selectedId = null;
      await load();
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function copyHtml() {
    if (!buf) return;
    try {
      // Two paths: rich-clipboard (HTML + text) so a paste into
      // a rich-text email field renders, plus a plain fallback.
      // navigator.clipboard.write isn't available in every browser;
      // catch + fall back to writeText.
      if (typeof ClipboardItem !== 'undefined' && navigator.clipboard?.write) {
        const blobHtml = new Blob([buf.html], { type: 'text/html' });
        const blobText = new Blob([buf.plain_text || stripTags(buf.html)], { type: 'text/plain' });
        await navigator.clipboard.write([
          new ClipboardItem({ 'text/html': blobHtml, 'text/plain': blobText })
        ]);
      } else {
        await navigator.clipboard.writeText(buf.html);
      }
      toast.success('Copied HTML to clipboard');
    } catch {
      toast.error('Copy failed (clipboard blocked?)');
    }
  }

  async function copyText() {
    if (!buf) return;
    try {
      await navigator.clipboard.writeText(buf.plain_text || stripTags(buf.html));
      toast.success('Copied plain text to clipboard');
    } catch {
      toast.error('Copy failed (clipboard blocked?)');
    }
  }

  // Cheap HTML → text fallback when the user didn't fill in
  // plain_text. Strips tags + collapses whitespace; not perfect
  // but good enough for "I forgot to fill in the fallback".
  function stripTags(html: string): string {
    return html
      .replace(/<br\s*\/?>/gi, '\n')
      .replace(/<\/(p|div|h[1-6]|li)>/gi, '\n')
      .replace(/<[^>]+>/g, '')
      .replace(/\n{3,}/g, '\n\n')
      .trim();
  }

  // Live preview — sandboxed iframe. srcdoc carries the user's
  // HTML; sandbox attribute set without allow-scripts so any
  // <script> inside the user's signature is silently dropped by
  // the browser. allow-same-origin lets the iframe inherit our
  // CSS reset baseline; we don't need it for cross-origin
  // anything else here.
  const previewSrcdoc = $derived.by(() => {
    if (!buf) return '';
    return `<!doctype html><html><head><meta charset="utf-8"><style>
      body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
             font-size: 14px; line-height: 1.5; color: #1a1a1a; padding: 16px; }
      a { color: #1a4fb3; }
      img { max-width: 100%; }
    </style></head><body>${buf.html}</body></html>`;
  });
</script>

<svelte:head><title>Email signatures · Granit</title></svelte:head>

<div class="flex flex-col h-full">
  <PageHeader title="Email signatures" subtitle="HTML signature library — preview, copy, manage">
    {#snippet actions()}
      <button
        onclick={createNew}
        class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90"
      >+ New</button>
    {/snippet}
  </PageHeader>

  <div class="flex-1 flex flex-col md:flex-row min-h-0">
    <!-- List pane: search + category filter + clickable rows.
         On mobile we hide the list when a signature is selected
         (focus on the editor); desktop keeps both panes side-by-
         side. Same pattern /projects uses so the muscle memory
         carries across the app. -->
    <aside class="md:w-72 lg:w-80 md:border-r border-surface1 md:overflow-y-auto flex-shrink-0 {selectedId ? 'hidden md:block' : 'block'}">
      <div class="p-3 border-b border-surface1 space-y-2">
        <input
          bind:value={q}
          placeholder="Search…"
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        />
        {#if categories.length > 0}
          <select
            bind:value={filterCategory}
            class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text"
          >
            <option value="">All categories</option>
            {#each categories as c}
              <option value={c}>{c}</option>
            {/each}
          </select>
        {/if}
      </div>
      {#if loading}
        <div class="p-4 text-xs text-dim italic">Loading…</div>
      {:else if filtered.length === 0}
        <div class="p-4 text-xs text-dim italic">
          {signatures.length === 0 ? 'No signatures yet — hit New to add one.' : 'No matches.'}
        </div>
      {:else}
        <ul class="divide-y divide-surface1">
          {#each filtered as s (s.id)}
            <li>
              <button
                onclick={() => (selectedId = s.id)}
                class="w-full text-left px-3 py-2.5 flex items-baseline gap-2 hover:bg-surface0 transition-colors {selectedId === s.id ? 'bg-surface0' : ''}"
              >
                <div class="flex-1 min-w-0">
                  <div class="flex items-baseline gap-2">
                    <span class="text-sm text-text truncate">{s.name}</span>
                    {#if s.is_default}
                      <span class="text-[10px] uppercase tracking-wider text-primary">default</span>
                    {/if}
                  </div>
                  {#if s.category}
                    <div class="text-[11px] text-dim">{s.category}</div>
                  {/if}
                </div>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </aside>

    <!-- Detail pane: form + sandboxed preview. On mobile, hidden
         until a signature is picked (or created); desktop always
         shows it with an empty state. -->
    <div class="flex-1 min-w-0 overflow-y-auto p-4 sm:p-6 {selectedId ? 'block' : 'hidden md:block'}">
      {#if !buf}
        <div class="h-full flex flex-col items-center justify-center text-center text-dim text-sm">
          <p class="max-w-sm">Pick a signature on the left to preview, edit, or copy. Or hit + New to start a fresh one.</p>
        </div>
      {:else}
        <div class="space-y-4">
          <!-- Mobile back arrow — returns to the list on small
               screens. Hidden on desktop where both panes are
               always visible side-by-side. -->
          <button
            type="button"
            onclick={() => (selectedId = null)}
            class="md:hidden -ml-1 mb-2 inline-flex items-center gap-1 text-sm text-subtext hover:text-text"
          >
            <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6"/></svg>
            Back to list
          </button>

          <div class="flex items-baseline gap-2 flex-wrap">
            <input
              bind:this={nameInputEl}
              bind:value={buf.name}
              oninput={markDirty}
              placeholder="Signature name"
              class="flex-1 min-w-0 text-base font-semibold px-2 py-1.5 bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-primary"
            />
            {#if !buf.is_default}
              <button
                onclick={() => selected && setDefault(selected.id)}
                class="text-xs text-secondary hover:underline"
              >Make default</button>
            {:else}
              <span class="text-xs text-primary">✓ default</span>
            {/if}
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <label class="block">
              <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Category</span>
              <input
                value={buf.category ?? ''}
                oninput={(e) => { buf!.category = (e.target as HTMLInputElement).value; markDirty(); }}
                placeholder='e.g. "Work" or "Stoicera"'
                list="sig-cat-options"
                class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
              />
              <datalist id="sig-cat-options">
                {#each categories as c}
                  <option value={c}></option>
                {/each}
              </datalist>
            </label>
            <label class="block">
              <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Default for vault</span>
              <label class="flex items-center gap-2 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text cursor-pointer">
                <input
                  type="checkbox"
                  checked={!!buf.is_default}
                  onchange={(e) => { buf!.is_default = (e.target as HTMLInputElement).checked; markDirty(); }}
                  class="w-4 h-4 accent-primary"
                />
                <span class="text-subtext">Use unless another is picked</span>
              </label>
            </label>
          </div>

          <div>
            <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">HTML</span>
            <textarea
              bind:value={buf.html}
              oninput={markDirty}
              rows="10"
              placeholder='<p>Best regards,<br/>Your Name</p>'
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text font-mono focus:outline-none focus:border-primary"
            ></textarea>
          </div>

          <div>
            <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Plain-text fallback (optional)</span>
            <textarea
              bind:value={buf.plain_text}
              oninput={markDirty}
              rows="4"
              placeholder="Leave blank — granit will derive one from the HTML on Copy plain"
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text font-mono focus:outline-none focus:border-primary"
            ></textarea>
          </div>

          <!-- Live preview. iframe with srcdoc + sandbox attribute
               (no allow-scripts) so any <script> tags in the user's
               HTML are silently dropped by the browser. The trust
               line lives at this iframe boundary. -->
          <div>
            <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Preview</span>
            <iframe
              title="Signature preview"
              srcdoc={previewSrcdoc}
              sandbox="allow-same-origin"
              class="w-full bg-white rounded border border-surface1"
              style="height: 220px"
            ></iframe>
          </div>

          <div class="flex items-center gap-2 flex-wrap pt-2 border-t border-surface1">
            <button
              onclick={copyHtml}
              class="px-3 py-2 text-sm bg-secondary/15 text-secondary border border-secondary/30 rounded hover:bg-secondary/25"
            >📋 Copy HTML</button>
            <button
              onclick={copyText}
              class="px-3 py-2 text-sm bg-surface0 border border-surface1 text-subtext rounded hover:border-primary"
            >Copy plain</button>
            <span class="flex-1"></span>
            <button
              onclick={removeSelected}
              class="px-3 py-2 text-sm text-dim hover:text-error"
            >Delete</button>
            <button
              onclick={saveBuf}
              disabled={!dirty}
              class="px-4 py-2 text-sm bg-primary text-on-primary rounded font-medium disabled:opacity-50"
            >{dirty ? 'Save' : 'Saved'}</button>
          </div>
        </div>
      {/if}
    </div>
  </div>
</div>
