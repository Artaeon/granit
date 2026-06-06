<script lang="ts">
  // Main editor / preview / split pane for the notes route.
  //
  // Owns:
  //   • the {#if viewMode === 'edit'/preview'/split'} switch
  //   • the previewBody snippet that splits daily-note bodies on the
  //     "## Day Activity" anchor so the inline activity widget can
  //     render between the two halves of the markdown
  //   • the SummaryCard mount on the preview pane (one mount, ever)
  //   • the previewContainer scroll element + its bind:this passthrough
  //
  // Editor.bind:this and previewContainer bind:this both pass through
  // to the parent via `bindEditor` + `bindPreviewContainer` callbacks —
  // Svelte 5 doesn't (yet) thread bind:this through component props,
  // so we expose the refs via a one-shot setter the parent stores in
  // its own $state slot.

  import type { Note } from '$lib/api';
  import type { Extension } from '@codemirror/state';
  import type { ExtractRequest } from '$lib/editor/extract-note';
  import type { EditorHandle } from '$lib/notes/editorHandle';
  import Editor from '$lib/editor/Editor.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import DayActivityInline from '$lib/notes/DayActivityInline.svelte';
  import NoteSummaryCard from '$lib/notes/NoteSummaryCard.svelte';

  type ViewMode = 'edit' | 'preview' | 'split';

  interface Props {
    note: Note;
    viewMode: ViewMode;
    /** Live unthrottled body — bound bidirectionally so keystrokes
     *  flow back to the page's pipe.body. */
    body: string;
    /** rAF-coalesced mirror for preview / summary card. */
    bodyForPreview: string;
    /** Daily-note segment split, or null when not daily. */
    dayActivitySegments: { before: string; after: string } | null;
    dailyDate: string | null;
    /** Extra CodeMirror extensions (inline-AI bridges). */
    editorAIExtensions: Extension[];
    onSave: () => Promise<boolean>;
    onNavigateWikilink: (target: string) => Promise<void>;
    onExtract: (req: ExtractRequest) => void;
    onCursor: (c: { line: number; col: number; selLen: number }) => void;
    onScroll: (s: { top: number; height: number; viewport: number }) => void;
    onSaveFrontmatter: (next: Record<string, unknown>) => Promise<boolean>;
    onPrepend: (text: string) => void;
    /** Pushed by the component on mount; the page stores the handle
     *  for cross-surface use. */
    bindEditor: (h: EditorHandle | undefined) => void;
    /** Pushed by the component when the preview scroll element
     *  mounts — used by the route's IntersectionObserver root. */
    bindPreviewContainer: (el: HTMLElement | null) => void;
  }

  let {
    note,
    viewMode,
    body = $bindable(),
    bodyForPreview,
    dayActivitySegments,
    dailyDate,
    editorAIExtensions,
    onSave,
    onNavigateWikilink,
    onExtract,
    onCursor,
    onScroll,
    onSaveFrontmatter,
    onPrepend,
    bindEditor,
    bindPreviewContainer
  }: Props = $props();

  // Local refs the parent reads via the bind* callbacks.
  let editor = $state<EditorHandle | undefined>();
  let previewContainer = $state<HTMLElement | null>(null);
  $effect(() => bindEditor(editor));
  $effect(() => bindPreviewContainer(previewContainer));
</script>

{#snippet previewBody()}
  {#if dayActivitySegments && dailyDate}
    <MarkdownRenderer body={dayActivitySegments.before} onWikilink={onNavigateWikilink} />
    <DayActivityInline date={dailyDate} />
    <MarkdownRenderer body={dayActivitySegments.after} onWikilink={onNavigateWikilink} />
  {:else}
    <!-- Throttled body — rAF-coalesced via bodyForPreview above.
         Multiple keystrokes in one frame produce one parse, not 5+. -->
    <MarkdownRenderer body={bodyForPreview} onWikilink={onNavigateWikilink} />
  {/if}
{/snippet}

<div class="flex-1 min-h-0 p-2 sm:p-3">
  {#if viewMode === 'edit'}
    <Editor bind:value={body} bind:this={editor} onSave={onSave} onNavigate={onNavigateWikilink} onExtract={onExtract} {onCursor} {onScroll} extraExtensions={editorAIExtensions} />
  {:else if viewMode === 'preview'}
    <div class="h-full overflow-y-auto bg-surface0 border border-surface1 rounded px-4 sm:px-6 py-4" bind:this={previewContainer}>
      <div class="max-w-3xl mx-auto">
        <NoteSummaryCard
          notePath={note.path}
          title={note.title || note.path}
          body={bodyForPreview}
          frontmatter={(note.frontmatter ?? {}) as Record<string, unknown>}
          {onSaveFrontmatter}
          {onPrepend}
        />
        {@render previewBody()}
      </div>
    </div>
  {:else}
    <!-- split (desktop only) -->
    <div class="h-full grid grid-cols-1 lg:grid-cols-2 gap-2">
      <Editor bind:value={body} bind:this={editor} onSave={onSave} onNavigate={onNavigateWikilink} onExtract={onExtract} {onCursor} {onScroll} extraExtensions={editorAIExtensions} />
      <div class="h-full overflow-y-auto bg-surface0 border border-surface1 rounded px-4 sm:px-6 py-4 hidden lg:block" bind:this={previewContainer}>
        {@render previewBody()}
      </div>
    </div>
  {/if}
</div>
