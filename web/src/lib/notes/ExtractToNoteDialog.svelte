<script lang="ts">
  import { onMount, tick } from 'svelte';
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
  $effect(() => {
    if (request) {
      title = suggestTitle(request.text);
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
          // Select-all on focus so the user can immediately overtype
          // the suggestion. The previous behaviour put the cursor at
          // the end of the suggestion, which meant the user had to
          // Cmd-A first to replace it — friction the report flagged.
          titleEl.select();
        }
      });
    } else {
      title = '';
      folder = '';
      tags = [];
      tagBuf = '';
      path = '';
      busy = false;
      error = '';
      advancedOpen = false;
      pathTouched = false;
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
  <div
    class="fixed inset-0 z-50 flex items-start justify-center pt-16 px-4 bg-mantle/70 backdrop-blur-sm"
    onclick={dismiss}
    onkeydown={onKey}
    role="presentation"
  >
    <form
      onsubmit={submit}
      class="w-full max-w-lg bg-base border border-surface1 rounded-lg shadow-xl"
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

        <div>
          <label for="extract-folder" class="block text-xs uppercase tracking-wider text-dim mb-1">Folder</label>
          <input
            id="extract-folder"
            bind:this={folderEl}
            bind:value={folder}
            list="extract-folder-options"
            placeholder="(vault root)"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
            autocomplete="off"
            spellcheck="false"
          />
          {#if folderOptions.length > 0}
            <datalist id="extract-folder-options">
              {#each folderOptions as f}<option value={f}></option>{/each}
            </datalist>
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
      <footer class="px-4 py-3 border-t border-surface1 flex items-center gap-2 justify-end">
        <button
          type="button"
          onclick={dismiss}
          disabled={busy}
          class="px-3 py-1.5 text-sm text-subtext hover:bg-surface0 rounded disabled:opacity-50"
        >Cancel</button>
        <button
          type="submit"
          disabled={busy || !title.trim()}
          class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50"
        >{busy ? 'Extracting…' : 'Extract'}</button>
      </footer>
    </form>
  </div>
{/if}
