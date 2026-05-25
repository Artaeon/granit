<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { auth } from '$lib/stores/auth';
  import { api, type VisionsStore, type VisionDoc, type Vision } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import { loadDraft, clearDraft, makeDraftWriter, loadDraftSavedAt } from '$lib/util/draftAutosave';

  // /vision — multi-document vision catalogue. One tab per doc
  // (Hauptvision / Kurzversion / Mission / Stoicera / Körper /
  // Glaube + user-defined custom keys), each with its own edit
  // history. Edits require a Reason — the whole point of the
  // feature is making vision changes intentional and reviewable.
  //
  // Legacy values + season focus from the old single-record vision
  // live in a compact sidecar strip at the top of the page; they
  // don't fit the multi-doc narrative model so they stay as
  // structured sidecar data the user can edit inline.

  let store = $state<VisionsStore | null>(null);
  let legacy = $state<Vision | null>(null);
  let activeKey = $state('main');
  let loading = $state(false);

  // Per-tab UI mode: read | edit | history. Tracked per-key so
  // switching tabs doesn't reset edit-in-progress on another tab.
  // BUT: we only carry ONE form buffer at a time (editContent /
  // editReason are single values, not per-tab maps), so switching
  // AWAY from an edit-mode tab implicitly drops it back to read —
  // the draft survives in localStorage and re-entry restores it.
  // Without this rule we'd hit the bug where coming back to a
  // previously-edited tab shows the other tab's buffer content.
  type Mode = 'read' | 'edit' | 'history';
  let mode = $state<Record<string, Mode>>({});
  function setMode(key: string, m: Mode) {
    mode = { ...mode, [key]: m };
  }
  function getMode(key: string): Mode {
    return mode[key] ?? 'read';
  }
  function switchActiveKey(next: string) {
    if (editingKey && editingKey !== next) {
      // Flush any pending debounced draft write before we drop the
      // edit-mode flag — the user's last keystrokes need to land in
      // localStorage so re-entering edit mode restores them.
      draftWriter.flushNow();
      setMode(editingKey, 'read');
      editingKey = null;
    }
    activeKey = next;
  }

  // Edit-form state. Bound to whichever tab is currently in edit
  // mode. Reset on entry/cancel.
  let editContent = $state('');
  let editReason = $state('');
  let saving = $state(false);
  // Draft autosave — protects the in-progress edit against reload
  // / accidental navigation. The vision PUT requires a non-empty
  // reason so we can't auto-persist to the server; localStorage
  // draft is the next best thing. Keyed per-doc so switching tabs
  // while editing two surfaces in parallel doesn't clobber either.
  let editingKey = $state<string | null>(null);
  let draftSavedAt = $state<string | null>(null);
  // Snapshot of the form state at startEdit time. Used by the
  // save-$effect below to:
  //  1. Skip writes when the user hasn't actually touched the form
  //     yet (otherwise an opened-but-untouched tab leaves a same-
  //     as-server draft in localStorage forever — wasteful noise).
  //  2. Preserve the previously-loaded draftSavedAt timestamp
  //     until the first real keystroke. Without this, re-entering
  //     a 12-minute-old draft would immediately overwrite the
  //     indicator with "just now" — defeating the whole point of
  //     showing the user when the safety net last fired.
  let editEntryContent = $state('');
  let editEntryReason = $state('');
  const draftWriter = makeDraftWriter(500);
  function draftKeyFor(key: string): string {
    return `vision.edit.${key}`;
  }
  $effect(() => {
    if (!editingKey) return;
    // Untouched entry — content + reason match the snapshot recorded
    // at startEdit. Don't write a draft yet; the user hasn't done
    // anything to capture.
    if (editContent === editEntryContent && editReason === editEntryReason) return;
    draftWriter.save(draftKeyFor(editingKey), { content: editContent, reason: editReason });
    // Bump the visible "draft saved · Xs ago" timestamp only on
    // genuine changes, not on entry-snapshot equality.
    draftSavedAt = new Date().toISOString();
  });
  // Flush any pending draft write before the user leaves the page
  // (close tab, hot reload, navigation). flushNow bypasses the
  // debounce so the last few keystrokes don't vanish.
  onDestroy(() => draftWriter.flushNow());

  // "Add custom domain" dialog state. Simple inline form, not a
  // modal — appears below the tab strip when toggled.
  let creatingCustom = $state(false);
  let newKey = $state('');
  let newLabel = $state('');

  // "Add new vision domain" toggles the creator panel below the tabs.
  function startCreate() {
    creatingCustom = true;
    newKey = '';
    newLabel = '';
  }
  function cancelCreate() {
    creatingCustom = false;
  }
  async function submitCreate() {
    const key = newKey.trim();
    const label = newLabel.trim();
    if (!key || !label) {
      toast.error('key and label required');
      return;
    }
    try {
      const created = await api.createVisionDoc({ key, label });
      toast.success(`added "${label}"`);
      creatingCustom = false;
      await load();
      activeKey = created.key;
    } catch (e) {
      toast.error('failed: ' + errorMessage(e));
    }
  }

  // Sidecar (legacy) edit state. Toggled separately from per-doc edit.
  // Draft-protected like the per-doc editor — losing a freshly-typed
  // 90-day season focus to an accidental reload would be the kind
  // of small frustration that compounds.
  let editingLegacy = $state(false);
  let legacyForm = $state({ valuesText: '', season_focus: '' });
  const LEGACY_DRAFT_KEY = 'vision.legacy';
  // Save the legacy form as a draft on change while editing.
  $effect(() => {
    if (!editingLegacy) return;
    draftWriter.save(LEGACY_DRAFT_KEY, legacyForm);
  });
  function startLegacyEdit() {
    if (!legacy) return;
    const draft = loadDraft<typeof legacyForm | null>(LEGACY_DRAFT_KEY, null);
    if (draft && (draft.valuesText !== '' || draft.season_focus !== '')) {
      legacyForm = draft;
    } else {
      legacyForm = {
        valuesText: (legacy.values ?? []).join('\n'),
        season_focus: legacy.season_focus ?? ''
      };
    }
    editingLegacy = true;
  }
  function cancelLegacy() {
    clearDraft(LEGACY_DRAFT_KEY);
    draftWriter.cancel();
    editingLegacy = false;
  }
  async function saveLegacy() {
    const values = legacyForm.valuesText.split(/[\n,]+/).map((v) => v.trim()).filter(Boolean);
    try {
      const next = await api.putVision({
        ...legacy,
        values,
        season_focus: legacyForm.season_focus.trim()
      });
      legacy = next;
      clearDraft(LEGACY_DRAFT_KEY);
      draftWriter.cancel();
      editingLegacy = false;
      toast.success('values + season saved');
    } catch (e) {
      toast.error('failed: ' + errorMessage(e));
    }
  }

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const [s, l] = await Promise.all([api.listVisions(), api.getVision()]);
      store = s;
      legacy = l;
      // ?tab= URL param wins for the initial activation — used by
      // ProjectDetail's "Vision anlegen" CTA to jump straight to the
      // matching tab. If the param points at an unknown key (stale
      // link / deleted doc) we fall through to the existing activeKey,
      // and then to the first doc if THAT is also unknown.
      const wantedTab = $page.url.searchParams.get('tab');
      if (wantedTab && store.docs.find((d) => d.key === wantedTab)) {
        activeKey = wantedTab;
      } else if (!store.docs.find((d) => d.key === activeKey) && store.docs.length > 0) {
        activeKey = store.docs[0].key;
      }
    } catch (e) {
      toast.error('failed: ' + errorMessage(e));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && (ev.path === '.granit/visions.json' || ev.path === '.granit/vision.json')) {
        load();
      }
    });
  });

  let activeDoc = $derived<VisionDoc | null>(
    store?.docs.find((d) => d.key === activeKey) ?? null
  );

  // Split the catalogue into "top-level" docs (Hauptvision / Mission /
  // Stoicera / etc.) and "project" docs (key starts with 'project:').
  // The two clusters render as separate tab groups so a vault with
  // 12 projects doesn't bury the 6 core domains under a wall of
  // project tabs.
  let topLevelDocs = $derived(store?.docs.filter((d) => !d.key.startsWith('project:')) ?? []);
  let projectDocs = $derived(store?.docs.filter((d) => d.key.startsWith('project:')) ?? []);

  function startEdit(d: VisionDoc) {
    // Restore a previous in-progress draft if one exists for this
    // doc — wins over the server's saved content because it
    // represents the user's most recent intent. Falls through to
    // server content (or empty) when no draft is present.
    const draft = loadDraft<{ content: string; reason: string } | null>(draftKeyFor(d.key), null);
    if (draft && (draft.content !== '' || draft.reason !== '')) {
      editContent = draft.content;
      editReason = draft.reason;
      draftSavedAt = loadDraftSavedAt(draftKeyFor(d.key));
    } else {
      editContent = d.content ?? '';
      editReason = '';
      draftSavedAt = null;
    }
    // Snapshot the entry state so the save-$effect can skip writes
    // until the user actually changes something. Also preserves
    // the loaded draftSavedAt indicator until the first real edit.
    editEntryContent = editContent;
    editEntryReason = editReason;
    editingKey = d.key;
    setMode(d.key, 'edit');
  }
  function cancelEdit(d: VisionDoc) {
    // Cancel = explicit "throw away the draft". User clicked
    // cancel because they want the in-progress text gone.
    clearDraft(draftKeyFor(d.key));
    draftWriter.cancel();
    editContent = '';
    editReason = '';
    editingKey = null;
    draftSavedAt = null;
    setMode(d.key, 'read');
  }
  async function saveEdit(d: VisionDoc) {
    const reason = editReason.trim();
    if (!reason) {
      toast.error('reason required — say why you\'re changing this');
      return;
    }
    saving = true;
    try {
      await api.putVisionDoc(d.key, { content: editContent, reason });
      // Server confirmed the change; the draft is obsolete.
      clearDraft(draftKeyFor(d.key));
      draftWriter.cancel();
      editingKey = null;
      draftSavedAt = null;
      toast.success('saved');
      setMode(d.key, 'read');
      await load();
    } catch (e) {
      // Don't clear the draft on save failure — the user's work
      // should survive a transient server error so they can retry.
      toast.error('failed: ' + errorMessage(e));
    } finally {
      saving = false;
    }
  }
  // Human-readable "draft saved Xs ago" string for the form footer.
  let draftAgo = $derived.by<string>(() => {
    if (!draftSavedAt) return '';
    const ms = Date.now() - new Date(draftSavedAt).getTime();
    if (ms < 0) return 'just now';
    const sec = Math.floor(ms / 1000);
    if (sec < 5) return 'draft saved · just now';
    if (sec < 60) return `draft saved · ${sec}s ago`;
    const min = Math.floor(sec / 60);
    if (min < 60) return `draft saved · ${min}m ago`;
    return 'draft saved earlier';
  });

  async function togglePin(d: VisionDoc) {
    try {
      if (d.pinned) {
        // Already pinned — clicking again would unpin. Backend
        // currently only supports pin (single-slot semantics).
        // Pin a different doc to move the spotlight.
        toast.info('already pinned to today — pin another doc to move it');
        return;
      }
      await api.pinVisionDoc(d.key);
      toast.success(`"${d.label}" is now on the today view`);
      await load();
    } catch (e) {
      toast.error('failed: ' + errorMessage(e));
    }
  }

  async function restoreHistory(d: VisionDoc, idx: number) {
    const entry = d.history?.[idx];
    if (!entry) return;
    if (!confirm(`Replace "${d.label}" with the version from ${new Date(entry.when).toLocaleString()}?\n\nThe current text will be moved into history.`)) return;
    try {
      await api.putVisionDoc(d.key, {
        content: entry.content,
        reason: `restored from ${new Date(entry.when).toLocaleDateString()}`
      });
      toast.success('restored');
      setMode(d.key, 'read');
      await load();
    } catch (e) {
      toast.error('failed: ' + errorMessage(e));
    }
  }

  function fmtDate(iso: string): string {
    try {
      const d = new Date(iso);
      return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
    } catch {
      return iso;
    }
  }
</script>

<svelte:head><title>Vision · granit</title></svelte:head>

<div class="h-full overflow-y-auto">
  <div class="max-w-4xl mx-auto p-3 sm:p-4 lg:p-6">
    <PageHeader title="Vision" />

    {#if loading && !store}
      <div class="text-sm text-dim">loading…</div>
    {:else if !$auth}
      <div class="text-sm text-dim">not authorized</div>
    {:else if store}

      <!-- Sidecar: legacy values + season focus. Compact, editable
           inline. Kept above the tabs because they're stable
           reference data (values change yearly, season changes ~90
           days) — read them first, then go into the narrative tabs. -->
      {#if legacy}
        <section class="mb-5 bg-mantle border border-surface1 rounded-lg p-3 sm:p-4">
          {#if editingLegacy}
            <div class="space-y-3">
              <div>
                <label for="leg-values" class="block text-[11px] uppercase tracking-wider text-dim font-medium mb-1">Werte (one per line)</label>
                <textarea
                  id="leg-values"
                  bind:value={legacyForm.valuesText}
                  rows="4"
                  class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary font-mono"
                  placeholder="Treue&#10;Wahrheit&#10;Demut"
                ></textarea>
              </div>
              <div>
                <label for="leg-season" class="block text-[11px] uppercase tracking-wider text-dim font-medium mb-1">Season-Focus (this 90 days)</label>
                <input
                  id="leg-season"
                  type="text"
                  bind:value={legacyForm.season_focus}
                  class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
                  placeholder="e.g. build the daily-loop foundation"
                />
              </div>
              <div class="flex gap-2 justify-end">
                <button type="button" onclick={cancelLegacy} class="px-3 py-1.5 text-xs text-subtext hover:text-text">cancel</button>
                <button type="button" onclick={saveLegacy} class="px-3 py-1.5 text-xs bg-primary text-on-primary rounded font-medium">save</button>
              </div>
            </div>
          {:else}
            <div class="flex items-start gap-3">
              <div class="flex-1 min-w-0 space-y-1.5">
                {#if (legacy.values ?? []).length > 0}
                  <div class="flex flex-wrap items-baseline gap-1.5">
                    <span class="text-[11px] uppercase tracking-wider text-dim mr-1">Werte</span>
                    {#each legacy.values ?? [] as v}
                      <span class="text-sm text-text">{v}</span>
                      <span class="text-dim/50">·</span>
                    {/each}
                  </div>
                {/if}
                {#if legacy.season_focus}
                  <div class="flex items-baseline gap-2">
                    <span class="text-[11px] uppercase tracking-wider text-dim mr-1">Season</span>
                    <span class="text-sm text-text">{legacy.season_focus}</span>
                    {#if legacy.season_day && legacy.season_total}
                      <span class="text-xs text-dim">Day {legacy.season_day} of {legacy.season_total}</span>
                    {/if}
                  </div>
                {/if}
                {#if (legacy.values ?? []).length === 0 && !legacy.season_focus}
                  <p class="text-xs text-dim italic">No values or season focus set yet.</p>
                {/if}
              </div>
              <button type="button" onclick={startLegacyEdit} class="text-xs text-secondary hover:underline flex-shrink-0">edit</button>
            </div>
          {/if}
        </section>
      {/if}

      <!-- Tab strip: one tab per vision doc, plus a "+ new" trigger.
           Project visions render in a separate cluster after a
           vertical divider so they don't crowd the core domains.
           Pinned doc shows a 📌 next to its label. -->
      <div class="border-b border-surface1 mb-4 flex flex-wrap items-end gap-1">
        {#each topLevelDocs as d (d.key)}
          <button
            type="button"
            onclick={() => switchActiveKey(d.key)}
            class="px-3 py-1.5 text-sm border-b-2 -mb-px transition-colors
              {activeKey === d.key
                ? 'border-primary text-text font-medium'
                : 'border-transparent text-dim hover:text-text'}"
            title={d.pinned ? 'Currently pinned to the today view' : ''}
          >
            {d.label}
            {#if d.pinned}<span class="ml-1 text-success" aria-label="pinned to today">📌</span>{/if}
          </button>
        {/each}
        {#if projectDocs.length > 0}
          <span class="self-stretch border-l border-surface1 mx-1.5" aria-hidden="true"></span>
          <span class="self-end pb-1.5 pl-1 text-[11px] text-dim uppercase tracking-wider">Projekte</span>
          {#each projectDocs as d (d.key)}
            <button
              type="button"
              onclick={() => switchActiveKey(d.key)}
              class="px-3 py-1.5 text-sm border-b-2 -mb-px transition-colors
                {activeKey === d.key
                  ? 'border-primary text-text font-medium'
                  : 'border-transparent text-dim hover:text-text'}"
              title={d.pinned ? 'Currently pinned to the today view' : ''}
            >
              {d.label}
              {#if d.pinned}<span class="ml-1 text-success" aria-label="pinned to today">📌</span>{/if}
            </button>
          {/each}
        {/if}
        <span class="flex-1"></span>
        <button
          type="button"
          onclick={startCreate}
          class="px-3 py-1.5 text-xs text-secondary hover:underline"
        >+ neuer Bereich</button>
      </div>

      <!-- Create-new-doc inline form. Sits between tabs and active
           panel so creating a domain leaves the user looking right
           at the spot it'll land. -->
      {#if creatingCustom}
        <section class="mb-4 bg-mantle border border-surface1 rounded-lg p-3 sm:p-4 space-y-3">
          <h3 class="text-sm font-medium text-text">Neuen Vision-Bereich anlegen</h3>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label for="new-key" class="block text-[11px] uppercase tracking-wider text-dim font-medium mb-1">Key (URL-safe)</label>
              <input
                id="new-key"
                type="text"
                bind:value={newKey}
                placeholder="family"
                class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label for="new-label" class="block text-[11px] uppercase tracking-wider text-dim font-medium mb-1">Label (display name)</label>
              <input
                id="new-label"
                type="text"
                bind:value={newLabel}
                placeholder="Familie"
                class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
              />
            </div>
          </div>
          <div class="flex gap-2 justify-end">
            <button type="button" onclick={cancelCreate} class="px-3 py-1.5 text-xs text-subtext hover:text-text">cancel</button>
            <button type="button" onclick={submitCreate} class="px-3 py-1.5 text-xs bg-primary text-on-primary rounded font-medium">create</button>
          </div>
        </section>
      {/if}

      <!-- Active tab panel: read / edit / history per-doc mode. -->
      {#if activeDoc}
        {@const m = getMode(activeDoc.key)}
        <section class="bg-mantle border border-surface1 rounded-lg p-4 sm:p-5">
          <header class="flex items-baseline gap-3 mb-4 pb-3 border-b border-surface1">
            <h2 class="text-sm uppercase tracking-wider text-dim font-medium flex-1">
              {activeDoc.label}
            </h2>
            {#if m === 'read'}
              <button type="button" onclick={() => startEdit(activeDoc)} class="text-xs text-secondary hover:underline">edit</button>
              <button type="button" onclick={() => togglePin(activeDoc)} class="text-xs text-secondary hover:underline" disabled={activeDoc.pinned}>
                {activeDoc.pinned ? 'pinned' : 'pin to today'}
              </button>
              {#if (activeDoc.history?.length ?? 0) > 0}
                <button type="button" onclick={() => setMode(activeDoc.key, 'history')} class="text-xs text-secondary hover:underline">
                  history ({activeDoc.history?.length})
                </button>
              {/if}
            {:else}
              <button type="button" onclick={() => setMode(activeDoc.key, 'read')} class="text-xs text-subtext hover:text-text">back</button>
            {/if}
          </header>

          {#if m === 'edit'}
            <div class="space-y-4">
              <div>
                <label for="content-area" class="block text-[11px] uppercase tracking-wider text-dim font-medium mb-1">Content (markdown)</label>
                <textarea
                  id="content-area"
                  bind:value={editContent}
                  rows="14"
                  class="w-full px-3 py-3 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary font-mono leading-relaxed"
                  placeholder={`Schreibe deine ${activeDoc.label.toLowerCase()}…`}
                ></textarea>
              </div>
              <div>
                <label for="reason-area" class="block text-[11px] uppercase tracking-wider text-dim font-medium mb-1">
                  Warum änderst du das? <span class="text-error">*</span>
                </label>
                <textarea
                  id="reason-area"
                  bind:value={editReason}
                  rows="2"
                  class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
                  placeholder="e.g. clarified my mission after the weekly review"
                ></textarea>
                <p class="text-[11px] text-dim mt-1">Erscheint in der Historie. Pflichtfeld — der Sinn der Funktion.</p>
              </div>
              <div class="flex items-baseline gap-2 justify-end">
                {#if draftAgo}
                  <span class="text-[11px] text-dim flex-1" title="Drafts are stored locally in this browser. Cleared on save or cancel.">{draftAgo}</span>
                {/if}
                <button type="button" onclick={() => cancelEdit(activeDoc)} class="px-3 py-1.5 text-xs text-subtext hover:text-text">cancel</button>
                <button
                  type="button"
                  onclick={() => saveEdit(activeDoc)}
                  disabled={saving || !editReason.trim()}
                  class="px-4 py-1.5 text-sm bg-primary text-on-primary rounded font-medium disabled:opacity-50"
                >{saving ? 'saving…' : 'save'}</button>
              </div>
            </div>
          {:else if m === 'history'}
            <div class="space-y-3">
              <p class="text-xs text-dim">Newest first. Click "restore" to swap a past version back into the live doc (current text becomes the next history entry).</p>
              {#each activeDoc.history ?? [] as h, idx (h.when + idx)}
                <details class="border border-surface1 rounded">
                  <summary class="flex items-baseline gap-2 px-3 py-2 cursor-pointer hover:bg-surface0">
                    <span class="text-xs text-dim font-mono tabular-nums">{fmtDate(h.when)}</span>
                    <span class="flex-1 text-sm text-text truncate">{h.reason || '(no reason recorded)'}</span>
                    <button
                      type="button"
                      onclick={(e) => { e.preventDefault(); e.stopPropagation(); restoreHistory(activeDoc, idx); }}
                      class="text-xs text-warning hover:underline flex-shrink-0"
                    >restore</button>
                  </summary>
                  <div class="px-3 py-3 border-t border-surface1 prose-vision">
                    {#if h.content}
                      <MarkdownRenderer body={h.content} />
                    {:else}
                      <p class="text-xs text-dim italic">(previous version was empty)</p>
                    {/if}
                  </div>
                </details>
              {/each}
            </div>
          {:else}
            <!-- read mode -->
            {#if activeDoc.content}
              <div class="prose-vision">
                <MarkdownRenderer body={activeDoc.content} />
              </div>
              <p class="mt-4 pt-3 border-t border-surface1 text-[11px] text-dim">
                Updated {fmtDate(activeDoc.updated_at)}
              </p>
            {:else}
              <div class="py-8 text-center">
                <p class="text-sm text-dim mb-3">No content yet for <em>{activeDoc.label}</em>.</p>
                <button
                  type="button"
                  onclick={() => startEdit(activeDoc)}
                  class="px-4 py-2 text-sm bg-primary text-on-primary rounded font-medium"
                >Start writing</button>
              </div>
            {/if}
          {/if}
        </section>
      {/if}
    {/if}
  </div>
</div>

<style>
  /* Slightly larger leading + serif-style body for the vision read
     view. Visions are documents the user re-reads, not lists to scan. */
  :global(.prose-vision) {
    line-height: 1.7;
    font-size: 0.95rem;
  }
  :global(.prose-vision p) {
    margin: 0 0 0.85em 0;
  }
  :global(.prose-vision h1, .prose-vision h2, .prose-vision h3) {
    margin-top: 1em;
    margin-bottom: 0.4em;
  }
</style>
