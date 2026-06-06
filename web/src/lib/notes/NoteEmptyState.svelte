<script lang="ts">
  // Empty / not-found / error-stuck states for the notes editor.
  //
  // Three render branches in priority order:
  //
  //   1. notFound — server returned 404 for the requested path.
  //      Shows the would-be title with a one-click "Create"
  //      affordance — far better than the bare "loading…" or the
  //      generic error banner the page used to render.
  //
  //   2. errorNoNote — load failed and we have no note to render.
  //      The normal header is hidden too, so without this the user
  //      has no UI to navigate away except a full page reload.
  //      Renders a back link, a vault-drawer trigger (mobile), the
  //      error message, and a Retry button.
  //
  //   3. (no branch) — caller renders nothing. The page falls back
  //      to its loading state.

  interface Props {
    notFound: boolean;
    error: string;
    /** When true, a note is loaded — the component renders nothing
     *  and the caller's main editor takes over. */
    hasNote: boolean;
    /** Title inferred from the path (.md stripped, basename only). */
    notFoundTitle: string;
    /** Raw URL path param (already URI-decoded for display). */
    rawPath: string;
    creatingNote: boolean;
    onCreate: () => void;
    onOpenTreeDrawer: () => void;
    onRetry: () => void;
  }

  const {
    notFound,
    error,
    hasNote,
    notFoundTitle,
    rawPath,
    creatingNote,
    onCreate,
    onOpenTreeDrawer,
    onRetry
  }: Props = $props();

  // Computed render selector — keeps the {#if} ladder in the
  // template shallow and the branch names explicit.
  const branch = $derived(
    notFound && !hasNote
      ? 'not-found'
      : error && !hasNote
        ? 'error-no-note'
        : error
          ? 'error-strip'
          : 'none'
  );
</script>

{#if branch === 'not-found'}
  <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle sticky top-0 z-20">
    <a
      href="/notes"
      aria-label="back to notes"
      class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </a>
    <h1 class="text-base font-semibold text-text flex-1 truncate">{notFoundTitle || 'New note'}</h1>
  </header>
  <div class="flex-1 flex items-center justify-center p-8">
    <div class="max-w-md text-center">
      <div class="text-base text-text mb-1">This note doesn't exist yet</div>
      <div class="text-sm text-dim mb-5">
        Create <code class="text-subtext">{rawPath}</code>
        with an empty body and start writing.
      </div>
      <button
        onclick={onCreate}
        disabled={creatingNote}
        class="px-4 py-2 rounded bg-primary text-on-primary text-sm font-medium hover:opacity-90 disabled:opacity-60"
      >
        {creatingNote ? 'Creating…' : 'Create note'}
      </button>
    </div>
  </div>
{:else if branch === 'error-no-note'}
  <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle sticky top-0 z-20">
    <a
      href="/notes"
      aria-label="back to notes"
      class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </a>
    <button
      onclick={onOpenTreeDrawer}
      aria-label="vault tree"
      class="lg:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded flex-shrink-0"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M3 6h18M3 12h18M3 18h18" stroke-linecap="round" />
      </svg>
    </button>
    <h1 class="text-base font-semibold text-text flex-1 truncate">Couldn't open note</h1>
    <button
      onclick={onRetry}
      class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded hover:border-primary text-text"
    >Retry</button>
  </header>
  <div class="p-6 text-sm text-error">{error}</div>
{:else if branch === 'error-strip'}
  <div class="px-4 py-2 text-sm text-error border-b border-error bg-surface0 flex-shrink-0">{error}</div>
{/if}
