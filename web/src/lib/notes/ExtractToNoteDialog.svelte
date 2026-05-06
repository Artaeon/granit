<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { suggestTitle, slugifyTitle, type ExtractRequest } from '$lib/editor/extract-note';

  // ExtractToNoteDialog — small modal that pops when the user hits
  // Mod-Shift-X with a selection. Asks for a title, computes the
  // target path, and on confirm calls onConfirm(title, path); the
  // parent does the API round-trip and the in-document replacement.
  //
  // Designed minimal — no folder picker, no fancy preview. The user
  // can edit the path field directly if they want a non-default
  // location. Enter confirms, Escape cancels — same shape as the
  // command palette / quick-input dialogs across the app.

  interface Props {
    request: ExtractRequest | null;
    /** Called when user confirms; parent handles create + replace. */
    onConfirm: (title: string, path: string) => Promise<void> | void;
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
  let path = $state('');
  let busy = $state(false);
  let error = $state('');
  let titleEl: HTMLInputElement | undefined = $state();

  // Default folder picker — daily notes (YYYY-MM-DD.md under any
  // folder) extract to the same folder so meeting-jot extractions
  // stay close to their source. Anything else lands at vault root,
  // which is the path the new-note dialog uses by default. The user
  // can edit either field before confirming.
  function defaultFolder(src: string): string {
    const m = src.match(/^(.+?)\/\d{4}-\d{2}-\d{2}\.md$/);
    return m ? m[1] + '/' : '';
  }

  // Auto-fill title + path when a new request lands. We tick() before
  // focusing so the input is mounted and the user's keypress that
  // triggered the dialog (Mod-Shift-X) doesn't get re-captured.
  $effect(() => {
    if (request) {
      title = suggestTitle(request.text);
      const folder = defaultFolder(sourcePath);
      const slug = slugifyTitle(title) || 'untitled';
      path = `${folder}${slug}.md`;
      error = '';
      tick().then(() => titleEl?.focus());
    } else {
      title = '';
      path = '';
      busy = false;
      error = '';
    }
  });

  // Re-derive path when the title changes (only if the user hasn't
  // hand-edited path yet — once they touch path we leave it alone so
  // their custom value isn't clobbered).
  let pathTouched = $state(false);
  $effect(() => {
    void title;
    if (request && !pathTouched) {
      const folder = defaultFolder(sourcePath);
      const slug = slugifyTitle(title) || 'untitled';
      path = `${folder}${slug}.md`;
    }
  });

  function onPathInput() {
    pathTouched = true;
  }

  async function submit(e?: SubmitEvent) {
    e?.preventDefault();
    if (busy || !request) return;
    const t = title.trim();
    if (!t) {
      error = 'Title is required';
      return;
    }
    const cleanPath = path.trim();
    if (!cleanPath.endsWith('.md')) {
      error = 'Path must end in .md';
      return;
    }
    // The backend already rejects ".." and absolute paths (handlers_
    // notes.go), but failing here is faster + the dialog gets to keep
    // the user's draft instead of round-tripping for a 400.
    if (cleanPath.includes('..') || cleanPath.startsWith('/')) {
      error = 'Path must stay inside the vault';
      return;
    }
    busy = true;
    error = '';
    try {
      await onConfirm(t, path.trim());
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
    class="fixed inset-0 z-50 flex items-start justify-center pt-20 px-4 bg-mantle/70 backdrop-blur-sm"
    onclick={dismiss}
    onkeydown={onKey}
    role="presentation"
  >
    <form
      onsubmit={submit}
      class="w-full max-w-md bg-base border border-surface1 rounded-lg shadow-xl"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-label="Extract selection to new note"
    >
      <header class="px-4 py-3 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-sm font-semibold text-text flex-1">Extract to new note</h2>
        <span class="text-[11px] text-dim">{request.text.length} char{request.text.length === 1 ? '' : 's'}</span>
      </header>
      <div class="p-4 space-y-3">
        <div>
          <label for="extract-title" class="block text-xs uppercase tracking-wider text-dim mb-1">Title</label>
          <input
            id="extract-title"
            bind:this={titleEl}
            bind:value={title}
            placeholder="Note title"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
            autocomplete="off"
            spellcheck="false"
          />
        </div>
        <div>
          <label for="extract-path" class="block text-xs uppercase tracking-wider text-dim mb-1">Path</label>
          <input
            id="extract-path"
            bind:value={path}
            oninput={onPathInput}
            placeholder="folder/title.md"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-xs font-mono text-subtext focus:outline-none focus:border-primary"
            autocomplete="off"
            spellcheck="false"
          />
        </div>
        {#if error}
          <div class="text-xs text-error">{error}</div>
        {/if}
        <p class="text-[11px] text-dim leading-relaxed">
          The selection will be moved into a new note and replaced here with
          <code class="font-mono">[[{title || 'Title'}]]</code>. The new note
          links back to <code class="font-mono">{sourcePath}</code> via frontmatter.
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
