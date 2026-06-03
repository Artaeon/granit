<script lang="ts">
  /**
   * Version history panel for a single note. Renders as a fullscreen
   * overlay (same shape as PrintPreview) so it can host a side-by-
   * side current-vs-version diff without fighting the editor's
   * layout.
   *
   * Three columns at desktop:
   *   1. Version list (newest first, with timestamp + size + hash)
   *   2. Selected version's body, read-only
   *   3. Current live body for comparison
   *
   * On mobile we collapse to a stacked layout: list, then preview.
   *
   * The user's mandate: "make sure there is file history as well for
   * rollback and nothing is ever lost!!!" — so the Restore button
   * does NOT confirm before writing. The pre-restore content is
   * itself snapshotted server-side before the restore overwrites it,
   * so a misclicked restore is one click away from being undone via
   * the same panel. A confirm dialog would be friction without
   * adding safety.
   */
  import { onMount } from 'svelte';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { api, type NoteVersion } from '$lib/api';
  import { lineDiff, diffStats, type DiffLine } from '$lib/util/lineDiff';
  import { relativeTime } from '$lib/util/relativeTime';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';

  // Inline relative-time formatter — there's no shared helper yet
  // and the panel only needs five buckets ("just now", N seconds /
  // minutes / hours / days). Pulling in date-fns or rolling a full
  // i18n-aware Intl.RelativeTimeFormat would be overkill for one
  // call site.
  // Use the shared relative-time helper; falls back to the original
  // ISO if unparseable to preserve the previous behaviour.
  function fmtRelativeTime(iso: string): string {
    return relativeTime(iso) || iso;
  }

  type Version = NoteVersion;

  let {
    open = $bindable(false),
    notePath = '',
    currentBody = '',
    onRestore
  }: {
    open?: boolean;
    notePath?: string;
    currentBody?: string;
    /**
     * Fires after a successful restore so the parent page can refresh
     * its body / dirty state. The newly-live body is passed in; the
     * parent should set its `body` and `note` accordingly. The panel
     * stays open so the user can see the restore landed and pick a
     * different version if needed.
     */
    onRestore?: (newBody: string) => void;
  } = $props();

  let versions = $state<Version[]>([]);
  let loading = $state(false);
  let loadError = $state('');
  let selectedTs = $state<string | null>(null);
  let selectedBody = $state('');
  let bodyLoading = $state(false);
  let restoring = $state(false);
  // 'split' = side-by-side full bodies (the original layout); 'diff'
  // = LCS line-diff against the live body so the user can see exactly
  // what changed between the snapshot and now. Persisted across opens
  // since the same user usually wants the same view.
  type View = 'split' | 'diff';
  const VIEW_KEY = 'granit.history.view';
  let view = $state<View>('split');
  onMount(() => {
    const v = loadStoredString(VIEW_KEY, '');
    if (v === 'split' || v === 'diff') view = v;
  });
  function setView(v: View) {
    view = v;
    saveStoredString(VIEW_KEY, v);
  }
  // Diff selectedBody → currentBody: "what changed since this snapshot
  // up to now". Reads naturally with + meaning "added since" and −
  // meaning "removed since". Memoised via $derived so the LCS only
  // re-runs when one of the bodies changes.
  let diffLines = $derived<DiffLine[]>(
    selectedBody && currentBody ? lineDiff(selectedBody, currentBody) : []
  );
  let stats = $derived(diffStats(diffLines));

  // (Re)load the version list every time the panel opens or the
  // notePath changes. We don't re-fetch on every save — the panel's
  // not visible while editing, and refreshing it on close-and-reopen
  // is plenty.
  $effect(() => {
    if (!open) return;
    if (!notePath) return;
    void loadVersions();
  });

  async function loadVersions() {
    loading = true;
    loadError = '';
    versions = [];
    selectedTs = null;
    selectedBody = '';
    try {
      const data = await api.listHistory(notePath);
      versions = data.versions ?? [];
      if (versions.length > 0) {
        await selectVersion(versions[0].timestamp);
      }
    } catch (err) {
      loadError = err instanceof Error ? err.message : 'failed to load history';
    } finally {
      loading = false;
    }
  }

  async function selectVersion(ts: string) {
    selectedTs = ts;
    bodyLoading = true;
    selectedBody = '';
    try {
      const data = await api.getHistoryVersion(notePath, ts);
      selectedBody = data.body ?? '';
    } catch (err) {
      toast.error(`Couldn't load version: ${errorMessage(err)}`);
    } finally {
      bodyLoading = false;
    }
  }

  // Confirmation gate. Restore is a destructive action — the live
  // body gets overwritten. The pre-restore body IS snapshotted
  // automatically (the backend's restore handler self-snaps before
  // overwrite), so the action is reversible — but the user has no
  // way to know that without a prompt. A single Esc/Cancel exits;
  // the confirm button issues the actual restore.
  let confirmingRestore = $state(false);

  function requestRestore() {
    if (!selectedTs || restoring) return;
    confirmingRestore = true;
  }

  function cancelRestore() {
    confirmingRestore = false;
  }

  async function restoreSelected() {
    if (!selectedTs || restoring) return;
    confirmingRestore = false;
    restoring = true;
    try {
      const restored = await api.restoreHistoryVersion(notePath, selectedTs);
      toast.success('Restored. Pre-restore content is in history.');
      onRestore?.(restored.body ?? '');
      await loadVersions();
    } catch (err) {
      toast.error(`Restore failed: ${errorMessage(err)}`);
    } finally {
      restoring = false;
    }
  }

  function fmtSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
  }

  function fmtTimestampFull(iso: string): string {
    try {
      return new Date(iso).toLocaleString(undefined, {
        year: 'numeric', month: 'short', day: 'numeric',
        hour: '2-digit', minute: '2-digit', second: '2-digit'
      });
    } catch {
      return iso;
    }
  }

  function close() {
    open = false;
  }

  // Esc to close, ↑/↓ to walk the version list when focus is on the
  // panel. We attach the listener while open is true and detach on
  // close so the editor's keymap isn't competing with us when the
  // panel isn't visible.
  $effect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        // Esc inside the confirm prompt cancels the prompt, not the
        // whole panel — so a user mid-double-take can back out
        // without losing their selection state.
        if (confirmingRestore) {
          cancelRestore();
          return;
        }
        close();
        return;
      }
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        const idx = versions.findIndex((v) => v.timestamp === selectedTs);
        if (idx === -1) return;
        const next = e.key === 'ArrowDown'
          ? Math.min(idx + 1, versions.length - 1)
          : Math.max(idx - 1, 0);
        if (next !== idx) {
          e.preventDefault();
          void selectVersion(versions[next].timestamp);
        }
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 bg-base flex flex-col"
    role="dialog"
    aria-label="Version history"
    aria-modal="true"
  >
    <header class="flex-shrink-0 flex items-center gap-3 px-3 py-2 border-b border-surface1 bg-mantle">
      <h2 class="text-base font-semibold text-text">
        <span class="text-dim font-normal">History ·</span> {notePath}
      </h2>
      <span class="text-xs text-dim">
        {versions.length} {versions.length === 1 ? 'version' : 'versions'}
      </span>
      {#if versions.length > 0}
        <!-- View toggle: side-by-side bodies vs. LCS diff. The diff
             reads "this snapshot → live body" so the user sees what
             they've added/removed since the snapshot. -->
        <div class="ml-2 inline-flex items-center text-[11px] rounded border border-surface1 bg-surface0 overflow-hidden">
          <button
            type="button"
            onclick={() => setView('split')}
            class="px-2 py-1 {view === 'split' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            title="Side-by-side full bodies"
          >Split</button>
          <button
            type="button"
            onclick={() => setView('diff')}
            class="px-2 py-1 {view === 'diff' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            title="LCS line diff: what changed since this snapshot"
          >Diff</button>
        </div>
        {#if view === 'diff' && (stats.added > 0 || stats.removed > 0)}
          <span class="text-[11px] tabular-nums">
            <span class="text-success">+{stats.added}</span>
            <span class="text-error ml-1">−{stats.removed}</span>
          </span>
        {/if}
      {/if}
      <div class="flex-1"></div>
      <button
        onclick={requestRestore}
        disabled={!selectedTs || restoring}
        class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 rounded text-sm font-medium bg-primary text-on-primary disabled:opacity-50"
        title="Replace the current note with this version"
      >
        {restoring ? 'Restoring…' : 'Restore'}
        <span class="hidden sm:inline"> this version</span>
      </button>
      <button
        onclick={close}
        aria-label="Close history"
        class="w-11 h-11 sm:w-9 sm:h-9 flex items-center justify-center text-subtext hover:text-text hover:bg-surface0 rounded"
      >
        <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M6 6l12 12M6 18L18 6" stroke-linecap="round"/>
        </svg>
      </button>
    </header>

    {#if loading}
      <div class="p-8 text-center text-dim text-sm">Loading history…</div>
    {:else if loadError}
      <div class="p-8 text-center text-error text-sm">{loadError}</div>
    {:else if versions.length === 0}
      <div class="flex-1 flex items-center justify-center p-8 text-center">
        <div class="max-w-md">
          <div class="text-base text-text mb-2">No history yet</div>
          <div class="text-sm text-dim">
            History snapshots are taken automatically every time you save a
            note. Once you've saved at least once after this update, you'll
            see versions here. Identical consecutive saves are deduplicated,
            so the list stays clean.
          </div>
        </div>
      </div>
    {:else}
      <div class="flex-1 min-h-0 grid grid-cols-1 md:grid-cols-[18rem_1fr] lg:grid-cols-[18rem_1fr_1fr] gap-0">
        <!-- Version list — left column on desktop, top section on
             mobile. Newest first. On mobile the list is constrained
             to ~40dvh so the selected version body still gets
             primary real estate; if the user has many versions they
             scroll inside the constrained list rather than past it. -->
        <aside class="border-b md:border-b-0 md:border-r border-surface1 overflow-y-auto bg-mantle max-h-[40dvh] md:max-h-none">
          <ul class="divide-y divide-surface1">
            {#each versions as v (v.timestamp)}
              {@const sel = selectedTs === v.timestamp}
              <li>
                <button
                  onclick={() => selectVersion(v.timestamp)}
                  class="w-full text-left px-3 py-2.5 flex flex-col gap-0.5 transition-colors
                    {sel ? 'bg-surface1 text-text' : 'hover:bg-surface0 text-subtext'}"
                  aria-current={sel ? 'true' : undefined}
                >
                  <span class="text-sm font-medium">{fmtRelativeTime(v.timestamp)}</span>
                  <span class="text-[11px] text-dim font-mono">{fmtTimestampFull(v.timestamp)}</span>
                  <span class="text-[11px] text-dim flex gap-2">
                    <span>{fmtSize(v.size)}</span>
                    <span class="font-mono opacity-70">{v.hash}</span>
                  </span>
                </button>
              </li>
            {/each}
          </ul>
        </aside>

        {#if view === 'diff'}
          <!-- Diff view spans the remaining columns. Each line is
               coloured by type: + green (added since snapshot),
               − red (removed since snapshot), eq dim. The gutter
               carries +/-/space so it's still readable in
               monochrome / when red-green colour-blind. -->
          <section class="md:col-span-1 lg:col-span-2 overflow-y-auto bg-base">
            <div class="px-3 py-2 border-b border-surface1 sticky top-0 bg-base">
              <div class="text-[11px] uppercase tracking-wider text-dim font-semibold">Diff</div>
              <div class="text-sm text-text">
                {#if selectedTs}
                  <span class="text-dim">snapshot:</span> {fmtTimestampFull(selectedTs)} <span class="text-dim">→ live body</span>
                {:else}
                  <span class="text-dim">no version selected</span>
                {/if}
              </div>
            </div>
            <div class="px-2 py-2">
              {#if bodyLoading}
                <div class="text-sm text-dim px-2">Loading…</div>
              {:else if diffLines.length === 0}
                <div class="text-sm text-dim italic px-2">no differences</div>
              {:else}
                <pre class="text-[12px] font-mono leading-5 whitespace-pre-wrap break-words m-0"><!--
               -->{#each diffLines as l, i (i)}<!--
                 -->{#if l.type === 'add'}<span class="block bg-surface0 text-success"><span class="inline-block w-4 text-right pr-2 select-none opacity-60">+</span>{l.text || ' '}</span>{:else if l.type === 'del'}<span class="block bg-surface0 text-error line-through opacity-90"><span class="inline-block w-4 text-right pr-2 select-none opacity-60 no-underline">−</span>{l.text || ' '}</span>{:else}<span class="block text-dim"><span class="inline-block w-4 text-right pr-2 select-none opacity-50">·</span>{l.text || ' '}</span>{/if}<!--
               -->{/each}<!--
             --></pre>
              {/if}
            </div>
          </section>
        {:else}
          <!-- Selected version body (read-only) -->
          <section class="overflow-y-auto border-r border-surface1 bg-base">
            <div class="px-3 py-2 border-b border-surface1 sticky top-0 bg-base">
              <div class="text-[11px] uppercase tracking-wider text-dim font-semibold">Selected version</div>
              {#if selectedTs}
                <div class="text-sm text-text">{fmtTimestampFull(selectedTs)}</div>
              {/if}
            </div>
            <div class="px-4 py-3">
              {#if bodyLoading}
                <div class="text-sm text-dim">Loading…</div>
              {:else}
                <pre class="text-sm font-mono whitespace-pre-wrap break-words text-text">{selectedBody}</pre>
              {/if}
            </div>
          </section>

          <!-- Current live body for comparison. Hidden on narrow
               viewports where the side-by-side wouldn't fit anyway. -->
          <section class="hidden lg:block overflow-y-auto bg-base">
            <div class="px-3 py-2 border-b border-surface1 sticky top-0 bg-base">
              <div class="text-[11px] uppercase tracking-wider text-dim font-semibold">Current</div>
              <div class="text-sm text-text">Live (unsaved changes excluded)</div>
            </div>
            <div class="px-4 py-3">
              <pre class="text-sm font-mono whitespace-pre-wrap break-words text-text">{currentBody}</pre>
            </div>
          </section>
        {/if}
      </div>
    {/if}

    {#if confirmingRestore}
      <!-- Restore confirmation. Renders inside the history dialog so
           the keyboard nav (Esc) still bubbles to the parent close
           handler. Backdrop is a sibling element with pointer-events
           so a click outside the prompt dismisses without restoring. -->
      <div
        class="absolute inset-0 z-10 flex items-center justify-center bg-base/70 backdrop-blur-sm"
        role="dialog"
        aria-modal="true"
        aria-label="Confirm restore"
        onclick={(e) => { if (e.target === e.currentTarget) cancelRestore(); }}
      >
        <div class="bg-surface0 border border-surface1 rounded-lg shadow-lg p-5 max-w-sm w-full mx-4">
          <h3 class="text-base font-semibold text-text mb-2">Overwrite current note?</h3>
          <p class="text-sm text-subtext leading-relaxed mb-4">
            Replaces your current body with the selected version. Your
            current body is auto-backed-up first, so you can restore it
            from this list afterwards.
          </p>
          <div class="flex items-center justify-end gap-2">
            <button
              onclick={cancelRestore}
              class="px-3 py-1.5 rounded text-sm text-subtext hover:text-text hover:bg-surface1"
            >Cancel</button>
            <button
              onclick={restoreSelected}
              autofocus
              class="px-3 py-1.5 rounded text-sm font-medium bg-primary text-on-primary"
            >Restore</button>
          </div>
        </div>
      </div>
    {/if}
  </div>
{/if}
