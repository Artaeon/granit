<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { suggestTitle, slugifyTitle, type ExtractRequest } from '$lib/editor/extract-note';
  import { api } from '$lib/api';

  // ExtractToNoteDialog — modal for the Mod-Shift-X "extract to new
  // note" flow. Asks for a title, folder, and tags; computes the
  // target path; on confirm calls onConfirm(title, path, tags); the
  // parent does the API round-trip and the in-document replacement.
  //
  // UX choices:
  //   - Title pre-fill is a tight single sentence (or heading) so a
  //     long meeting block doesn't dump 200 chars into the field.
  //     The input auto-selects on focus so the user can immediately
  //     overtype with a real title.
  //   - Folder is a free-text input with a datalist of existing
  //     vault folders — the user picks from history OR types
  //     somewhere new. Daily-note sources default to the source's
  //     own folder so meeting jots cluster naturally.
  //   - Tags are chips: type a name, Enter or comma confirms, ×
  //     removes. They land in the new note's frontmatter so a
  //     future tag search picks the extract up.
  //   - Path is shown read-only as a "preview" line so the user
  //     can see what's going to disk; an Advanced toggle reveals
  //     the path field for direct override.

  interface Props {
    request: ExtractRequest | null;
    /** Called when user confirms; parent handles create + replace. */
    onConfirm: (args: { title: string; path: string; tags: string[] }) => Promise<void> | void;
    /** Called when the dialog closes without confirmation. */
    onDismiss: () => void;
    /**
     * Path of the note the selection came from — used to compute a
     * sensible default folder for the extracted note (same folder as
     * the source if the source is a daily note, otherwise vault root).
     */
    sourcePath: string;
  }

  let { request, onConfirm, onDismiss, sourcePath }: Props = $props();

  let title = $state('');
  let folder = $state('');
  let tags = $state<string[]>([]);
  let tagBuf = $state('');
  let path = $state('');
  let advancedOpen = $state(false);
  let pathTouched = $state(false);
  let busy = $state(false);
  let error = $state('');
  let titleEl: HTMLInputElement | undefined = $state();
  let folderEl: HTMLInputElement | undefined = $state();
  // Folder picker state — a real combobox dropdown. The previous
  // <datalist> implementation relied on the browser's autocomplete
  // surface, which the user described as a "fake select" because
  // the popup didn't appear reliably (Safari, mobile Firefox).
  // Rolling our own keeps the behaviour consistent everywhere.
  let folderPickerOpen = $state(false);
  let folderPickerEl: HTMLDivElement | undefined = $state();

  // Folder discovery — derive distinct folder paths from existing
  // notes so the datalist offers concrete choices ("Notes/Meetings",
  // "Projects/Granit") instead of a blank field. Cached at module
  // scope of this component instance; refetched each open since the
  // list is small.
  let folderOptions = $state<string[]>([]);
  async function loadFolders() {
    try {
      const r = await api.listNotes({ limit: 5000 });
      const set = new Set<string>();
      for (const n of r.notes) {
        const i = n.path.lastIndexOf('/');
        if (i > 0) set.add(n.path.slice(0, i));
      }
      folderOptions = [...set].sort((a, b) => a.localeCompare(b));
    } catch {
      folderOptions = [];
    }
  }

  function defaultFolder(src: string): string {
    const m = src.match(/^(.+?)\/\d{4}-\d{2}-\d{2}\.md$/);
    return m ? m[1] : '';
  }

  function buildPath(f: string, t: string): string {
    const slug = slugifyTitle(t) || 'untitled';
    const cleanF = f.trim().replace(/\/+$/, '');
    return cleanF ? `${cleanF}/${slug}.md` : `${slug}.md`;
  }

  // Auto-fill on every new request. The dialog re-uses the same
  // component instance across opens, so resetting in `else` keeps
  // a stale draft from leaking into the next extraction.
  //
  // CRITICAL: this effect must ONLY re-run on `request` transitions —
  // never when `sourcePath` changes. The user reported "title cannot
  // be changed!" The root cause:
  //
  //   - sourcePath is bound to the parent's `note?.path`
  //   - the parent reassigns `note` every time autosave succeeds
  //     (note = updated inside save())
  //   - Reading sourcePath inside the effect body registered it as
  //     a reactive dep, so every parent autosave during the user's
  //     typing re-ran THIS effect and reset title to suggestTitle
  //
  // Wrapping the body in untrack() — except for the explicit
  // `request` read up top — means the effect tracks request alone.
  // Open/close transitions still re-init state; in-flight parent
  // saves are invisible to the dialog.
  $effect(() => {
    const r = request; // the only reactive dep we want
    if (r) {
      untrack(() => {
        title = suggestTitle(r.text);
        folder = defaultFolder(sourcePath);
        tags = [];
        tagBuf = '';
        path = buildPath(folder, title);
        pathTouched = false;
        advancedOpen = false;
        error = '';
        void loadFolders();
        tick().then(() => {
          if (titleEl) {
            titleEl.focus();
            // Cursor at end via setSelectionRange — works on every
            // browser without the "select all" side-effect.
            const len = titleEl.value.length;
            titleEl.setSelectionRange(len, len);
          }
        });
      });
    } else {
      untrack(() => {
        title = '';
        folder = '';
        tags = [];
        tagBuf = '';
        path = '';
        busy = false;
        error = '';
        advancedOpen = false;
        pathTouched = false;
      });
    }
  });

  // Re-derive path when title or folder change — but only if the
  // user hasn't manually edited the Advanced path field. Once they
  // do, their value is the source of truth.
  $effect(() => {
    void title;
    void folder;
    if (request && !pathTouched) {
      path = buildPath(folder, title);
    }
  });

  function onPathInput() {
    pathTouched = true;
  }

  // Folder dropdown — opens on input focus / click and closes on
  // outside click. Differentiates between TWO interactions:
  //
  //   1. User clicks the chevron / focuses the input → see ALL
  //      folders in the vault. No filtering. This is the bug the
  //      user reported: with the previous always-filter behaviour,
  //      the field was pre-seeded to "Jots" so the dropdown only
  //      showed Jots-* folders — they couldn't see the rest of the
  //      vault.
  //
  //   2. User actively types in the search box at the top of the
  //      panel → filter live. The search box is INSIDE the dropdown,
  //      separate from the folder field below it, so picking a folder
  //      doesn't replace the user's search query mid-search.
  //
  // The folder field stays as the SELECTED VALUE (or a custom path
  // the user types directly).
  let folderSearch = $state('');
  let filteredFolders = $derived.by(() => {
    const q = folderSearch.trim().toLowerCase();
    if (!q) return folderOptions;
    return folderOptions.filter((f) => f.toLowerCase().includes(q));
  });
  function pickFolder(f: string) {
    folder = f;
    folderPickerOpen = false;
    folderSearch = '';
    folderEl?.focus();
  }
  function onFolderFocus() {
    folderPickerOpen = true;
    folderSearch = '';
  }
  function onFolderClick() {
    folderPickerOpen = true;
    folderSearch = '';
  }
  function onFolderKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      folderPickerOpen = false;
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      folderPickerOpen = true;
    }
  }
  // Document-level listener to close the picker when the user
  // clicks outside the input + popup. Mounted only while the
  // picker is open so we don't hold a listener forever.
  $effect(() => {
    if (!folderPickerOpen) return;
    const onClick = (e: MouseEvent) => {
      const t = e.target as Node | null;
      if (!t) return;
      if (folderPickerEl?.contains(t)) return;
      if (folderEl?.contains(t)) return;
      folderPickerOpen = false;
    };
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  });

  // Tag chip helpers — same UX as FrontmatterEditor: Enter / comma
  // confirms, Backspace on empty buffer pops the last chip.
  function addTag() {
    const t = tagBuf.trim().replace(/^#/, '');
    if (!t) return;
    if (tags.some((x) => x.toLowerCase() === t.toLowerCase())) {
      tagBuf = '';
      return;
    }
    tags = [...tags, t];
    tagBuf = '';
  }
  function removeTag(i: number) {
    tags = tags.filter((_, idx) => idx !== i);
  }
  function onTagKey(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      addTag();
    } else if (e.key === 'Backspace' && tagBuf === '' && tags.length > 0) {
      e.preventDefault();
      removeTag(tags.length - 1);
    }
  }

  async function submit(e?: SubmitEvent) {
    e?.preventDefault();
    if (busy || !request) return;
    const t = title.trim();
    if (!t) {
      error = 'Title is required';
      titleEl?.focus();
      return;
    }
    // Flush any in-flight tag buffer so a user who typed "agile"
    // and hit Enter to submit (without Enter on the chip first)
    // doesn't lose the tag.
    if (tagBuf.trim()) addTag();
    const cleanPath = path.trim();
    if (!cleanPath.endsWith('.md')) {
      error = 'Path must end in .md';
      return;
    }
    if (cleanPath.includes('..') || cleanPath.startsWith('/')) {
      error = 'Path must stay inside the vault';
      return;
    }
    busy = true;
    error = '';
    try {
      await onConfirm({ title: t, path: cleanPath, tags });
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  function dismiss() {
    if (busy) return;
    onDismiss();
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      dismiss();
    }
  }
</script>

{#if request}
  <!-- Mobile slides up from the bottom (rounded-t-xl, items-end);
       desktop centers near the top with a comfortable margin. dvh
       (dynamic viewport) accounts for iOS Safari's bottom bar so a
       long dialog doesn't get clipped when the bar shows. -->
  <div
    class="fixed inset-0 z-50 flex items-end sm:items-start justify-center sm:pt-16 sm:px-4 bg-black/60"
    onclick={dismiss}
    onkeydown={onKey}
    role="presentation"
  >
    <form
      onsubmit={submit}
      class="w-full sm:max-w-lg bg-base border border-surface1 rounded-t-xl sm:rounded-lg shadow-xl max-h-[92dvh] sm:max-h-[88vh] overflow-y-auto"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-label="Extract selection to new note"
    >
      <header class="px-4 py-3 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-sm font-semibold text-text flex-1">Extract to new note</h2>
        <span class="text-[11px] text-dim">{request.text.length} char{request.text.length === 1 ? '' : 's'} selected</span>
      </header>
      <div class="p-4 space-y-3">
        <div>
          <label for="extract-title" class="block text-xs uppercase tracking-wider text-dim mb-1">Title</label>
          <input
            id="extract-title"
            bind:this={titleEl}
            bind:value={title}
            placeholder="Note title (e.g. Board sync)"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
            autocomplete="off"
            spellcheck="false"
          />
          <p class="text-[10px] text-dim mt-1">Pre-filled from your selection — overtype to change.</p>
        </div>

        <div class="relative">
          <label for="extract-folder" class="block text-xs uppercase tracking-wider text-dim mb-1">Folder</label>
          <div class="relative">
            <input
              id="extract-folder"
              bind:this={folderEl}
              bind:value={folder}
              onfocus={onFolderFocus}
              onclick={onFolderClick}
              onkeydown={onFolderKeydown}
              placeholder="(vault root)"
              class="w-full px-3 py-2 pr-9 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary cursor-pointer"
              autocomplete="off"
              spellcheck="false"
              role="combobox"
              aria-expanded={folderPickerOpen}
              aria-controls="extract-folder-listbox"
            />
            <button
              type="button"
              onclick={() => { folderPickerOpen = !folderPickerOpen; if (folderPickerOpen) folderSearch = ''; folderEl?.focus(); }}
              aria-label={folderPickerOpen ? 'close folder picker' : 'open folder picker'}
              class="absolute right-2 top-1/2 -translate-y-1/2 text-dim hover:text-text px-1 leading-none"
            >▾</button>
          </div>
          {#if folderPickerOpen}
            <div
              bind:this={folderPickerEl}
              id="extract-folder-listbox"
              role="listbox"
              class="absolute z-10 left-0 right-0 mt-1 bg-base border border-surface1 rounded shadow-lg max-h-72 overflow-hidden flex flex-col"
            >
              <!-- Search box at the top — separate from the folder
                   field below so picking doesn't kill the user's
                   search query mid-search. -->
              <div class="p-1.5 border-b border-surface1 flex-shrink-0">
                <input
                  bind:value={folderSearch}
                  placeholder="search folders…"
                  class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text focus:outline-none focus:border-primary"
                  autocomplete="off"
                  spellcheck="false"
                />
              </div>
              <div class="overflow-y-auto flex-1">
                <button
                  type="button"
                  onclick={() => pickFolder('')}
                  role="option"
                  aria-selected={folder === ''}
                  class="w-full text-left px-3 py-1.5 text-xs text-dim hover:bg-surface0 italic border-b border-surface1"
                >(vault root)</button>
                {#if folderOptions.length === 0}
                  <div class="px-3 py-2 text-xs text-dim italic">
                    No folders in the vault yet. Type a name in the field above to create one.
                  </div>
                {:else}
                  {#each filteredFolders as f}
                    <button
                      type="button"
                      onclick={() => pickFolder(f)}
                      role="option"
                      aria-selected={folder === f}
                      class="w-full text-left px-3 py-1.5 text-sm font-mono hover:bg-surface0 truncate {folder === f ? 'text-primary bg-primary/5' : 'text-text'}"
                    >{f}</button>
                  {/each}
                  {#if filteredFolders.length === 0}
                    <div class="px-3 py-2 text-xs text-dim italic">
                      No matching folder for "{folderSearch.trim()}". Use the field above to create a new one.
                    </div>
                  {/if}
                {/if}
              </div>
              <div class="px-3 py-1.5 text-[10px] text-dim border-t border-surface1 flex-shrink-0">
                {filteredFolders.length} of {folderOptions.length} folder{folderOptions.length === 1 ? '' : 's'}
              </div>
            </div>
          {/if}
        </div>

        <div>
          <span class="block text-xs uppercase tracking-wider text-dim mb-1">Tags</span>
          <div class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded flex flex-wrap items-center gap-1.5 focus-within:border-primary">
            {#each tags as tag, i (tag + ':' + i)}
              <span class="inline-flex items-center gap-1 px-1.5 py-0.5 bg-secondary/15 text-secondary rounded text-[11px]">
                <span>#{tag}</span>
                <button
                  type="button"
                  onclick={() => removeTag(i)}
                  aria-label="remove tag {tag}"
                  class="text-secondary/70 hover:text-error leading-none"
                >×</button>
              </span>
            {/each}
            <input
              bind:value={tagBuf}
              onkeydown={onTagKey}
              onblur={() => { if (tagBuf.trim()) addTag(); }}
              placeholder={tags.length === 0 ? 'add tags…' : '+'}
              class="flex-1 min-w-[6rem] bg-transparent text-sm text-text placeholder-dim focus:outline-none"
              autocomplete="off"
              spellcheck="false"
            />
          </div>
        </div>

        <!-- Path preview line — non-editable by default so the user
             reads it as confirmation, not a thing to fiddle with.
             "Advanced" reveals the editable path field for the rare
             case the user wants something the folder picker can't
             express (a different filename slug, a deeper nesting). -->
        <div class="text-[11px] text-dim flex items-center gap-2 pt-1">
          <span class="font-mono text-subtext truncate flex-1">{path || '…'}</span>
          <button
            type="button"
            onclick={() => (advancedOpen = !advancedOpen)}
            class="text-secondary hover:underline flex-shrink-0"
          >{advancedOpen ? 'hide path' : 'edit path'}</button>
        </div>
        {#if advancedOpen}
          <input
            bind:value={path}
            oninput={onPathInput}
            placeholder="folder/title.md"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-xs font-mono text-subtext focus:outline-none focus:border-primary"
            autocomplete="off"
            spellcheck="false"
          />
        {/if}

        {#if error}
          <div class="text-xs text-error">{error}</div>
        {/if}
        <p class="text-[11px] text-dim leading-relaxed pt-1 border-t border-surface1">
          On confirm: the selection becomes a note at <code class="font-mono">{path || '…'}</code>
          and the source location is replaced with <code class="font-mono">[[{title || 'Title'}]]</code>.
          The new note links back to <code class="font-mono">{sourcePath}</code> via frontmatter.
        </p>
      </div>
      <footer
        class="px-4 py-3 border-t border-surface1 flex items-center gap-2 justify-end"
        style="padding-bottom: max(0.75rem, env(safe-area-inset-bottom));"
      >
        <button
          type="button"
          onclick={dismiss}
          disabled={busy}
          class="px-3 py-2 sm:py-1.5 text-sm text-subtext hover:bg-surface0 rounded disabled:opacity-50 min-h-[44px] sm:min-h-0"
        >Cancel</button>
        <button
          type="submit"
          disabled={busy || !title.trim()}
          class="px-3 py-2 sm:py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50 min-h-[44px] sm:min-h-0"
        >{busy ? 'Extracting…' : 'Extract'}</button>
      </footer>
    </form>
  </div>
{/if}
