<script lang="ts">
  import { api, type NoteAnnotation } from '$lib/api';
  import { errorMessage } from '$lib/util/errorMessage';
  import { toast } from '$lib/components/toast';
  import { onWsEvent } from '$lib/ws';
  import { onMount } from 'svelte';

  // AnnotationsPanel — right-side margin column rendering the
  // user's marginalia on the active note. One card per annotation,
  // anchored to its line via the `data-line` attribute the editor
  // exposes, with hover-to-highlight in the editor body.
  //
  // The panel is read-write: a "+" button at the top opens an
  // inline composer for a new note; clicking an existing one
  // toggles edit mode in place; the × deletes (with no
  // confirmation — the action is reversible by re-adding from the
  // anchor text shown in the card header).

  type Props = {
    notePath: string;
    /** Currently-focused line in the editor — bumped to scroll the
     *  matching annotation card into view. */
    activeLine?: number;
    /** Optional callback fired when the user clicks an annotation
     *  card so the editor can scroll its line into view. */
    onJumpToLine?: (line: number) => void;
  };
  let { notePath, activeLine, onJumpToLine }: Props = $props();

  const COLORS = ['yellow', 'blue', 'green', 'pink'] as const;
  type Color = (typeof COLORS)[number];
  const DEFAULT_COLOR: Color = 'yellow';

  let items = $state<NoteAnnotation[]>([]);
  let loading = $state(false);
  let composing = $state(false);
  let composerText = $state('');
  let composerColor = $state<Color>(DEFAULT_COLOR);
  let composerLine = $state(1);
  let composerAnchor = $state('');
  let editingId = $state<string | null>(null);
  let editText = $state('');
  let editColor = $state<Color>(DEFAULT_COLOR);

  async function load() {
    if (!notePath) return;
    loading = true;
    try {
      const r = await api.listAnnotations(notePath);
      items = r.annotations;
    } catch (e) {
      // Silent — the panel is auxiliary; failing it shouldn't
      // bother the user mid-edit. The next reload retries.
      console.warn('[annotations] load failed:', errorMessage(e));
    } finally {
      loading = false;
    }
  }

  // Reload when the active note changes. WS broadcasts on
  // .granit/annotations.json fire from any tab — we re-read on
  // each one so cross-tab edits surface here too.
  $effect(() => {
    void notePath;
    load();
  });

  onMount(() => {
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/annotations.json') {
        load();
      }
    });
  });

  /** Open the composer pre-filled with the line + its anchor text.
   *  Public so the parent can wire a "annotate this line" action
   *  from a context menu / shortcut. */
  export function openComposer(lineNum: number, anchorText: string) {
    composerLine = Math.max(1, lineNum);
    composerAnchor = anchorText.slice(0, 80);
    composerText = '';
    composerColor = DEFAULT_COLOR;
    composing = true;
  }

  async function saveNew(e?: Event) {
    e?.preventDefault();
    const text = composerText.trim();
    if (!text) return;
    try {
      const created = await api.createAnnotation({
        notePath,
        lineNum: composerLine,
        anchorText: composerAnchor,
        text,
        color: composerColor
      });
      items = [...items, created].sort((a, b) => a.lineNum - b.lineNum);
      composing = false;
      composerText = '';
    } catch (err) {
      toast.error('Couldn\'t save annotation: ' + errorMessage(err));
    }
  }

  function startEdit(a: NoteAnnotation) {
    editingId = a.id;
    editText = a.text;
    editColor = (a.color as Color) || DEFAULT_COLOR;
  }
  function cancelEdit() {
    editingId = null;
    editText = '';
  }
  async function saveEdit() {
    if (!editingId) return;
    const text = editText.trim();
    if (!text) return;
    try {
      const updated = await api.patchAnnotation(editingId, { text, color: editColor });
      items = items.map((x) => (x.id === updated.id ? updated : x));
      editingId = null;
    } catch (err) {
      toast.error('Couldn\'t update: ' + errorMessage(err));
    }
  }

  async function remove(a: NoteAnnotation) {
    try {
      await api.deleteAnnotation(a.id);
      items = items.filter((x) => x.id !== a.id);
    } catch (err) {
      toast.error('Couldn\'t delete: ' + errorMessage(err));
    }
  }

  function jumpTo(a: NoteAnnotation) {
    onJumpToLine?.(a.lineNum);
  }

  function colorClass(c?: string): string {
    switch (c) {
      case 'blue':  return 'border-l-blue-400';
      case 'green': return 'border-l-green-400';
      case 'pink':  return 'border-l-pink-400';
      default:      return 'border-l-yellow-400';
    }
  }

  // Highlight the card that matches the active editor line. We
  // don't auto-scroll on every cursor move (jarring) — only when
  // the user clicks the "scroll to active line" affordance via
  // the parent.
  let activeCardId = $derived.by(() => {
    if (activeLine == null) return null;
    const hit = items.find((x) => x.lineNum === activeLine);
    return hit?.id ?? null;
  });
</script>

<div class="space-y-2">
  <div class="flex justify-end -mt-6 mb-1">
    <button
      onclick={() => openComposer(activeLine ?? 1, '')}
      class="text-[11px] px-2 py-0.5 rounded bg-surface1 hover:bg-surface2 text-subtext normal-case tracking-normal"
      title="Add a margin note for the current line"
    >
      + Add
    </button>
  </div>
    {#if composing}
      <form onsubmit={saveNew} class="border border-primary/50 rounded-lg p-2 bg-surface0">
        <div class="text-[10px] uppercase tracking-wider text-dim mb-1">Line {composerLine}</div>
        {#if composerAnchor}
          <p class="text-xs text-dim italic mb-2 line-clamp-2">"{composerAnchor}"</p>
        {/if}
        <textarea
          bind:value={composerText}
          placeholder="Your note about this line…"
          rows="3"
          class="w-full px-2 py-1 bg-mantle border border-surface1 rounded text-text text-sm focus:outline-none focus:border-primary resize-none"
          autofocus
        ></textarea>
        <div class="flex items-center gap-2 mt-2">
          <div class="flex gap-1">
            {#each COLORS as c (c)}
              <button
                type="button"
                class="w-5 h-5 rounded-full border-2 {composerColor === c ? 'border-text' : 'border-transparent'}"
                style="background: {c === 'yellow' ? '#fde68a' : c === 'blue' ? '#bfdbfe' : c === 'green' ? '#bbf7d0' : '#fbcfe8'}"
                onclick={() => (composerColor = c)}
                title={c}
              ></button>
            {/each}
          </div>
          <div class="flex-1"></div>
          <button
            type="button"
            onclick={() => (composing = false)}
            class="text-xs px-2 py-1 text-dim hover:text-text"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={!composerText.trim()}
            class="text-xs px-2 py-1 bg-primary text-on-primary rounded disabled:opacity-50"
          >
            Save
          </button>
        </div>
      </form>
    {/if}

    {#if items.length === 0 && !composing && !loading}
      <p class="text-xs text-dim italic px-2 py-6 text-center leading-relaxed">
        No margin notes yet. Click <span class="text-text">+ Add</span> to mark a line with a thought, question, or counter-argument — your reasoning over time, kept beside the source.
      </p>
    {/if}

    {#each items as a (a.id)}
      <article
        class="border border-surface1 border-l-4 {colorClass(a.color)} rounded-lg p-2 bg-surface0 hover:border-primary transition-colors {activeCardId === a.id ? 'ring-2 ring-primary/40' : ''}"
      >
        {#if editingId === a.id}
          <div class="text-[10px] uppercase tracking-wider text-dim mb-1">
            Line {a.lineNum}
          </div>
          {#if a.anchorText}
            <p class="text-xs text-dim italic mb-2 line-clamp-2">"{a.anchorText}"</p>
          {/if}
          <textarea
            bind:value={editText}
            rows="3"
            class="w-full px-2 py-1 bg-mantle border border-surface1 rounded text-text text-sm focus:outline-none focus:border-primary resize-none"
          ></textarea>
          <div class="flex items-center gap-2 mt-2">
            <div class="flex gap-1">
              {#each COLORS as c (c)}
                <button
                  type="button"
                  class="w-5 h-5 rounded-full border-2 {editColor === c ? 'border-text' : 'border-transparent'}"
                  style="background: {c === 'yellow' ? '#fde68a' : c === 'blue' ? '#bfdbfe' : c === 'green' ? '#bbf7d0' : '#fbcfe8'}"
                  onclick={() => (editColor = c)}
                ></button>
              {/each}
            </div>
            <div class="flex-1"></div>
            <button
              type="button"
              onclick={cancelEdit}
              class="text-xs px-2 py-1 text-dim hover:text-text"
            >
              Cancel
            </button>
            <button
              type="button"
              onclick={saveEdit}
              disabled={!editText.trim()}
              class="text-xs px-2 py-1 bg-primary text-on-primary rounded disabled:opacity-50"
            >
              Save
            </button>
          </div>
        {:else}
          <div class="flex items-baseline justify-between mb-1">
            <button
              onclick={() => jumpTo(a)}
              class="text-[10px] uppercase tracking-wider text-dim hover:text-primary"
              title="Jump to this line in the editor"
            >
              Line {a.lineNum}
            </button>
            <div class="flex items-center gap-1 opacity-0 hover:opacity-100 group-hover:opacity-100 focus-within:opacity-100">
              <button
                onclick={() => startEdit(a)}
                class="text-[11px] text-dim hover:text-text"
                title="Edit"
              >
                edit
              </button>
              <button
                onclick={() => remove(a)}
                class="text-[11px] text-dim hover:text-error"
                title="Delete"
              >
                ×
              </button>
            </div>
          </div>
          {#if a.anchorText}
            <p class="text-xs text-dim italic mb-1.5 line-clamp-2">"{a.anchorText}"</p>
          {/if}
          <p class="text-sm text-text whitespace-pre-wrap leading-snug">{a.text}</p>
        {/if}
      </article>
    {/each}
</div>
