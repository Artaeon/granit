<script lang="ts">
  // Right-side info rail for the note editor — Outline, Margin notes,
  // Backlinks, Research, and three collapsible advanced surfaces
  // (AI link suggester, Reference note, Properties).
  //
  // Extracted from routes/notes/[...path]/+page on 2026-05-28.
  // Was a `{#snippet infoContent()}` rendered into either the desktop
  // aside or the drawer. Now a component instead so the file stays
  // declarative on what's IN the rail vs HOW it mounts (the latter
  // still lives in the page so the matchMedia gating is preserved).
  //
  // annotationCount is $bindable so the parent (which surfaces the
  // count in the section header badge in the desktop aside) stays
  // the single source of truth for the value — AnnotationsPanel's
  // onCountChange writes through.

  import type { Note } from '$lib/api';
  import Outline from '$lib/notes/Outline.svelte';
  import BacklinksPanel from '$lib/notes/BacklinksPanel.svelte';
  import AnnotationsPanel from '$lib/notes/AnnotationsPanel.svelte';
  import FrontmatterEditor from '$lib/notes/FrontmatterEditor.svelte';
  import ResearchPanel from '$lib/notes/ResearchPanel.svelte';
  import ReferenceNotePanel from '$lib/notes/ReferenceNotePanel.svelte';
  import LinkSuggestPanel from '$lib/notes/LinkSuggestPanel.svelte';

  interface Props {
    note: Note | null;
    body: string;
    viewMode: 'edit' | 'preview' | 'split';
    previewContainer: HTMLElement | null;
    visitedHeadings: Set<number>;
    cursorLine: number;
    annotationCount: number;
    existingTagList: string[];
    onJumpToLine: (line: number) => void;
    onNavigateWikilink: (target: string) => void;
    onResetVisited: () => void;
    onSaveFrontmatter: (next: Record<string, unknown>) => void | Promise<unknown>;
    onAddSuggestedTag: (tag: string) => void | Promise<void>;
    onInsertSuggestedLink: (markup: string) => void;
  }

  let {
    note,
    body,
    viewMode,
    previewContainer,
    visitedHeadings,
    cursorLine,
    annotationCount = $bindable(),
    existingTagList,
    onJumpToLine,
    onNavigateWikilink,
    onResetVisited,
    onSaveFrontmatter,
    onAddSuggestedTag,
    onInsertSuggestedLink
  }: Props = $props();
</script>

<!-- Right rail — pruned from 12 sections to 6. Removed surfaces
     that duplicated features available elsewhere:
       * Local graph    → Backlinks already lists what's relevant
       * Ask this note  → AIOverlay covers single-note Q&A via the
                          "attach note" toggle on the composer
       * Section questions → EditorAIBar's More menu has Outline +
                             Open questions verbs
       * Word frequencies + Sentence rhythm → niche editorial tools;
                             EditorAIBar covers tighten/critique
     The kept set is the active-reading + navigation kernel: where
     am I in the note (Outline), my marginalia (Margin notes),
     what else links here (Backlinks), and three collapsible
     advanced surfaces under details/summary so they don't crowd
     the rail until the user reaches for them. -->
<div class="p-3 space-y-4 overflow-y-auto h-full">
  <section>
    <h3 class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1.5">
      <span>Outline</span>
      {#if visitedHeadings.size > 0}
        <button
          type="button"
          onclick={onResetVisited}
          class="ml-auto text-[9px] tracking-normal normal-case text-dim hover:text-error"
          title="clear visited-section ticks for this note"
          aria-label="reset reading progress"
        >reset</button>
      {/if}
    </h3>
    <Outline
      body={body}
      onJump={onJumpToLine}
      cursorLine={cursorLine}
      scrollContainer={viewMode !== 'edit' ? previewContainer : null}
      visited={visitedHeadings}
    />
  </section>
  {#if note}
    <section>
      <h3 class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1.5">
        <span>Margin notes</span>
        {#if annotationCount > 0}
          <span class="ml-auto normal-case tracking-normal text-[10px] px-1.5 py-0.5 rounded-full bg-surface1 text-text tabular-nums">{annotationCount}</span>
        {/if}
      </h3>
      <AnnotationsPanel
        notePath={note.path}
        activeLine={cursorLine}
        onJumpToLine={onJumpToLine}
        onCountChange={(n) => (annotationCount = n)}
      />
    </section>
    <section>
      <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Backlinks</h3>
      <BacklinksPanel path={note.path} onNavigate={onNavigateWikilink} />
    </section>
    <!-- Research panel auto-hides when the body has no highlights /
         footnotes / outbound URLs. Keep visible (no wrapping
         details) so it just appears the moment the note picks up
         any of those affordances. -->
    <ResearchPanel body={body} onJump={onJumpToLine} />
    <!-- Advanced surfaces — collapsed by default. Each `<details>`
         opens independently and remembers nothing across reloads
         on purpose; defaulting closed is the whole point. -->
    <details class="group">
      <summary class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1.5 cursor-pointer hover:text-text select-none">
        <svg viewBox="0 0 24 24" class="w-3 h-3 transition-transform group-open:rotate-90" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 6 15 12 9 18"/></svg>
        <span>AI link suggester</span>
      </summary>
      <div class="mt-2">
        <LinkSuggestPanel
          notePath={note.path}
          body={body}
          existingTags={existingTagList}
          onAddTag={onAddSuggestedTag}
          onInsertLink={onInsertSuggestedLink}
        />
      </div>
    </details>
    <details class="group">
      <summary class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1.5 cursor-pointer hover:text-text select-none">
        <svg viewBox="0 0 24 24" class="w-3 h-3 transition-transform group-open:rotate-90" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 6 15 12 9 18"/></svg>
        <span>Reference note</span>
      </summary>
      <div class="mt-2">
        <ReferenceNotePanel currentPath={note.path} currentBody={body} currentTitle={note.title ?? ''} />
      </div>
    </details>
    <details class="group">
      <summary class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-1.5 cursor-pointer hover:text-text select-none">
        <svg viewBox="0 0 24 24" class="w-3 h-3 transition-transform group-open:rotate-90" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 6 15 12 9 18"/></svg>
        <span>Properties</span>
      </summary>
      <div class="mt-2">
        <FrontmatterEditor frontmatter={note.frontmatter ?? {}} onChange={onSaveFrontmatter} />
      </div>
    </details>
  {/if}
</div>
