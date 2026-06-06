<!--
  AIOverlayContextChips — the grounding-toggle row beneath the body.

  Two mutually-exclusive shapes:

    • On /notes/<path>  — show the note attach chip. The server-side
                          notePath expander on /chat/stream injects
                          the note's body into the system prompt; we
                          surface the path so the user knows what
                          we're sending. RAG is the sibling toggle.

    • Anywhere else     — show the snapshot chip. On non-note routes
                          the AI gets the Context Engine's snapshot
                          (events / tasks / recent notes / goals /
                          deadlines) so freeform questions have data
                          to lean on. Snapshot status shows
                          "loading…" / "today's vault" / "unavailable"
                          with a retry button when load fails.

  RAG is always the second toggle — its checked state lives on the
  AIContextManager; the parent owns the setter through this
  component's onSetRag callback so the manager's policy + persistence
  + side-effects fire normally.
-->
<script lang="ts">
  type Props = {
    /** From parent's currentNotePath derived — drives the
     *  note-vs-snapshot branch. Null when not on a /notes/ route. */
    currentNotePath: string | null;
    attachNote: boolean;
    attachSnapshot: boolean;
    snapshotLoading: boolean;
    snapshotData: unknown;
    rag: boolean;
    onSetRag: (v: boolean) => void;
    onLoadSnapshot: () => void;
  };

  let {
    currentNotePath,
    attachNote = $bindable(),
    attachSnapshot = $bindable(),
    snapshotLoading,
    snapshotData,
    rag,
    onSetRag,
    onLoadSnapshot
  }: Props = $props();
</script>

{#if currentNotePath}
  <div class="border-t border-surface1 px-4 py-2 flex items-center gap-2 flex-shrink-0 text-[11px] flex-wrap">
    <label class="flex items-center gap-1.5 cursor-pointer flex-1 min-w-[10rem]">
      <input
        type="checkbox"
        bind:checked={attachNote}
        class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0"
      />
      <span class="text-dim flex-shrink-0">attach</span>
      <span class="text-subtext font-mono truncate" title={currentNotePath}>{currentNotePath}</span>
    </label>
    <label class="flex items-center gap-1.5 cursor-pointer flex-shrink-0" title="Search the vault for relevant notes and include their excerpts as grounding context">
      <input
        type="checkbox"
        checked={rag}
        onchange={(e) => onSetRag(e.currentTarget.checked)}
        class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0"
      />
      <span class="text-dim">RAG</span>
    </label>
  </div>
{:else}
  <div class="border-t border-surface1 px-4 py-2 flex items-center gap-3 flex-shrink-0 text-[11px] flex-wrap">
    <label class="flex items-center gap-1.5 cursor-pointer flex-1 min-w-[10rem]">
      <input
        type="checkbox"
        bind:checked={attachSnapshot}
        disabled={snapshotLoading}
        class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0 disabled:opacity-50"
      />
      <span class="text-dim flex-shrink-0">snapshot</span>
      <span class="text-subtext font-mono truncate">
        {#if snapshotLoading}
          loading…
        {:else if snapshotData}
          today's vault
        {:else}
          unavailable
        {/if}
      </span>
      {#if !snapshotLoading && !snapshotData}
        <button
          type="button"
          onclick={(e) => { e.preventDefault(); onLoadSnapshot(); }}
          class="text-secondary hover:underline ml-1"
        >retry</button>
      {/if}
    </label>
    <label class="flex items-center gap-1.5 cursor-pointer flex-shrink-0" title="Search the vault for relevant notes per question and include their excerpts as grounding context">
      <input
        type="checkbox"
        checked={rag}
        onchange={(e) => onSetRag(e.currentTarget.checked)}
        class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0"
      />
      <span class="text-dim">RAG</span>
    </label>
  </div>
{/if}
