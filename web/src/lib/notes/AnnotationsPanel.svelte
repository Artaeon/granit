<script lang="ts">
  import { api, type NoteAnnotation } from '$lib/api';
  import { errorMessage } from '$lib/util/errorMessage';
  import { classifyAiError } from '$lib/util/aiErrors';
  import { toast } from '$lib/components/toast';
  import { onWsEvent } from '$lib/ws';
  import { onMount } from 'svelte';
  import { focusOnMount } from '$lib/util/focusOnMount';
  import {
    ANNOTATION_COLORS,
    DEFAULT_ANNOTATION_COLOR,
    annotationBorderClass,
    annotationSwatchHex,
    asAnnotationColor,
    type AnnotationColor
  } from '$lib/notes/annotationColors';

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
    /** Optional callback fired with the loaded count whenever the
     *  in-store annotation set changes. The parent uses this to
     *  render a count badge on the section header without re-
     *  fetching or duplicating the load. */
    onCountChange?: (count: number) => void;
  };
  let { notePath, activeLine, onJumpToLine, onCountChange }: Props = $props();

  const COLORS = ANNOTATION_COLORS;
  type Color = AnnotationColor;
  const DEFAULT_COLOR: Color = DEFAULT_ANNOTATION_COLOR;

  let items = $state<NoteAnnotation[]>([]);
  // Insert a new annotation and keep `items` sorted by line. Used by
  // both saveNew (manual compose) and acceptSuggestion (AI accept).
  function insertSortedByLine(created: NoteAnnotation): void {
    items = [...items, created].sort((a, b) => a.lineNum - b.lineNum);
  }
  // Re-fire onCountChange whenever the in-memory list shape moves.
  // Bumping it from $effect rather than from each mutation site
  // keeps every code path that sets `items` consistent — load, AI
  // accept, manual save, edit, delete, WS-driven refresh all flow
  // through the same notification.
  $effect(() => {
    onCountChange?.(items.length);
  });
  let loading = $state(false);
  let composing = $state(false);
  let composerText = $state('');
  let composerColor = $state<Color>(DEFAULT_COLOR);
  let composerLine = $state(1);
  let composerAnchor = $state('');
  let editingId = $state<string | null>(null);
  let editText = $state('');
  let editColor = $state<Color>(DEFAULT_COLOR);

  // AI suggestion state — held separately from `items` so the user
  // sees clearly which rows are model-proposed (and dismissable as
  // a batch) vs already in the store. Each suggestion carries the
  // same shape the create endpoint expects so accepting is a one-
  // call POST.
  type Suggestion = {
    lineNum: number;
    anchorText: string;
    text: string;
    color: string;
  };
  let aiBusy = $state(false);
  let aiAbort: AbortController | null = null;
  let aiError = $state('');
  let aiSuggestions = $state<Suggestion[]>([]);
  let acceptingIdx = $state<number | null>(null);

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

  // Set when a WS reload fires while the user is mid-edit / composing.
  // load() would otherwise replace items[] wholesale and leave the
  // user's editText / composerText pointing at a row whose live
  // server text has just changed — saving back would clobber the
  // remote edit. Defer the reload; flush it when the user finishes.
  let pendingReload = false;
  function isBusyLocally(): boolean {
    return editingId !== null || composing;
  }
  function maybeReload() {
    if (isBusyLocally()) {
      pendingReload = true;
      return;
    }
    pendingReload = false;
    void load();
  }
  function flushPendingReload() {
    if (pendingReload) maybeReload();
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
        maybeReload();
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
      insertSortedByLine(created);
      composing = false;
      composerText = '';
      flushPendingReload();
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
    flushPendingReload();
  }
  async function saveEdit() {
    if (!editingId) return;
    const text = editText.trim();
    if (!text) return;
    try {
      const updated = await api.patchAnnotation(editingId, { text, color: editColor });
      items = items.map((x) => (x.id === updated.id ? updated : x));
      editingId = null;
      flushPendingReload();
    } catch (err) {
      toast.error('Couldn\'t update: ' + errorMessage(err));
    }
  }

  async function runSuggest() {
    if (aiBusy) return;
    aiBusy = true;
    aiError = '';
    aiSuggestions = [];
    aiAbort = new AbortController();
    try {
      const r = await api.aiAnnotateNote(notePath, aiAbort.signal);
      aiSuggestions = (r.annotations ?? []).map((a) => ({
        lineNum: a.lineNum,
        anchorText: a.anchorText,
        text: a.text,
        color: asAnnotationColor(a.color)
      }));
      if (aiSuggestions.length === 0) {
        if (r.warning) {
          aiError = r.warning;
        } else {
          aiError = 'No suggestions returned — the note may be too short.';
        }
      }
    } catch (err) {
      // AbortError is the user clicking Cancel — silent.
      if (err instanceof DOMException && err.name === 'AbortError') {
        aiError = '';
      } else {
        const msg = errorMessage(err);
        const hint = classifyAiError(msg);
        aiError = hint.headline;
        if (/disabled in AI preferences/i.test(msg)) {
          aiError = 'Enable "Annotate note" in Settings → AI features first.';
        }
      }
    } finally {
      aiBusy = false;
      aiAbort = null;
    }
  }
  function cancelSuggest() {
    aiAbort?.abort();
  }
  function dismissSuggestions() {
    aiSuggestions = [];
    aiError = '';
  }
  async function acceptSuggestion(idx: number) {
    if (acceptingIdx !== null) return;
    const s = aiSuggestions[idx];
    if (!s) return;
    acceptingIdx = idx;
    try {
      const created = await api.createAnnotation({
        notePath,
        lineNum: s.lineNum,
        anchorText: s.anchorText,
        text: s.text,
        color: s.color
      });
      insertSortedByLine(created);
      // Drop the accepted suggestion so the row vanishes.
      aiSuggestions = aiSuggestions.filter((_, i) => i !== idx);
    } catch (err) {
      toast.error('Couldn\'t accept: ' + errorMessage(err));
    } finally {
      acceptingIdx = null;
    }
  }
  function skipSuggestion(idx: number) {
    aiSuggestions = aiSuggestions.filter((_, i) => i !== idx);
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

  // Border-class delegated to the shared annotationColors helper —
  // same palette used by the dashboard widget + the books reader's
  // bookmarks list, so a future palette shift updates one file.
  const colorClass = annotationBorderClass;

  // Highlight the card that matches the active editor line. We
  // don't auto-scroll on every cursor move (jarring) — only when
  // the user clicks the "scroll to active line" affordance via
  // the parent.
  let activeCardId = $derived(
    activeLine == null ? null : items.find((x) => x.lineNum === activeLine)?.id ?? null
  );
</script>

<div class="space-y-2">
  <div class="flex justify-end -mt-4 mb-1 gap-1">
    <!-- AI suggest — calls the audit-gated annotate-note feature.
         Cancellable while in flight. Disabled while suggestions are
         already on screen (the user reviews + clears those first
         before re-running, so a 4-row "skip" pile doesn't bury a
         fresh batch). -->
    {#if aiBusy}
      <button
        onclick={cancelSuggest}
        class="text-[11px] px-2 py-0.5 rounded bg-surface1 hover:bg-surface2 text-warning normal-case tracking-normal"
        title="Cancel the in-flight suggestion request"
      >
        cancel
      </button>
    {:else}
      <button
        onclick={runSuggest}
        disabled={aiSuggestions.length > 0}
        class="text-[11px] px-2 py-0.5 rounded bg-surface1 hover:bg-surface2 text-subtext normal-case tracking-normal disabled:opacity-50 disabled:cursor-not-allowed"
        title="Ask AI to propose 3-5 margin notes for this note"
      >
        ✨ AI suggest
      </button>
    {/if}
    <button
      onclick={() => openComposer(activeLine ?? 1, '')}
      class="text-[11px] px-2 py-0.5 rounded bg-surface1 hover:bg-surface2 text-subtext normal-case tracking-normal"
      title="Add a margin note for the current line"
    >
      + Add
    </button>
  </div>

  <!-- AI suggestion review surface — distinct from the in-store
       items below so the user can accept or skip a batch without
       confusing them with already-saved annotations. -->
  {#if aiBusy && aiSuggestions.length === 0}
    <div class="border border-surface2 bg-surface1 rounded-lg p-3 text-xs text-subtext flex items-center gap-2">
      <svg viewBox="0 0 24 24" class="w-3 h-3 animate-spin text-primary" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="9" stroke-opacity="0.25"/>
        <path d="M21 12a9 9 0 0 0-9-9" stroke-linecap="round"/>
      </svg>
      Reading the note + drafting suggestions…
    </div>
  {/if}
  {#if aiError}
    <div class="border border-warning bg-surface0 rounded-lg p-2 text-xs text-warning flex items-baseline gap-2">
      <span class="flex-1">{aiError}</span>
      <button onclick={dismissSuggestions} class="text-dim hover:text-text">×</button>
    </div>
  {/if}
  {#if aiSuggestions.length > 0}
    <div class="border border-surface2 bg-surface1 rounded-lg p-2">
      <div class="flex items-baseline justify-between mb-2">
        <span class="text-[10px] uppercase tracking-wider text-primary">
          {aiSuggestions.length} AI suggestion{aiSuggestions.length === 1 ? '' : 's'}
        </span>
        <button
          onclick={dismissSuggestions}
          class="text-[11px] text-dim hover:text-text"
          title="Dismiss all suggestions"
        >
          dismiss all
        </button>
      </div>
      <ul class="space-y-1.5">
        {#each aiSuggestions as s, idx (idx)}
          <li class="border border-surface1 border-l-4 {colorClass(s.color)} rounded p-2 bg-surface0">
            <div class="text-[10px] uppercase tracking-wider text-dim mb-1">
              Line {s.lineNum}
            </div>
            {#if s.anchorText}
              <p class="text-xs text-dim italic mb-1.5 line-clamp-2">"{s.anchorText}"</p>
            {/if}
            <p class="text-sm text-text mb-2 leading-snug">{s.text}</p>
            <div class="flex justify-end gap-1">
              <button
                onclick={() => skipSuggestion(idx)}
                disabled={acceptingIdx !== null}
                class="text-xs px-2 py-1 text-dim hover:text-text"
              >
                skip
              </button>
              <button
                onclick={() => acceptSuggestion(idx)}
                disabled={acceptingIdx !== null}
                class="text-xs px-2 py-1 bg-primary text-on-primary rounded disabled:opacity-50"
              >
                {acceptingIdx === idx ? 'Saving…' : '+ Accept'}
              </button>
            </div>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
    {#if composing}
      <form onsubmit={saveNew} class="border border-primary rounded-lg p-2 bg-surface0">
        <div class="text-[10px] uppercase tracking-wider text-dim mb-1">Line {composerLine}</div>
        {#if composerAnchor}
          <p class="text-xs text-dim italic mb-2 line-clamp-2">"{composerAnchor}"</p>
        {/if}
        <textarea
          bind:value={composerText}
          placeholder="Your note about this line…"
          rows="3"
          class="w-full px-2 py-1 bg-mantle border border-surface1 rounded text-text text-sm focus:outline-none focus:border-primary resize-none"
          use:focusOnMount
        ></textarea>
        <div class="flex items-center gap-2 mt-2">
          <div class="flex gap-1">
            {#each COLORS as c (c)}
              <button
                type="button"
                class="w-5 h-5 rounded-full border-2 {composerColor === c ? 'border-text' : 'border-transparent'}"
                style="background: {annotationSwatchHex(c)}"
                onclick={() => (composerColor = c)}
                title={c}
              ></button>
            {/each}
          </div>
          <div class="flex-1"></div>
          <button
            type="button"
            onclick={() => { composing = false; flushPendingReload(); }}
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
                  aria-label="Highlight colour {c}"
                  aria-pressed={editColor === c}
                  class="w-5 h-5 rounded-full border-2 {editColor === c ? 'border-text' : 'border-transparent'}"
                  style="background: {annotationSwatchHex(c)}"
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
