<script lang="ts">
  // Vault maintenance — surfaces the two AI-assisted hygiene moves
  // wired in handlers_vault_maintenance.go:
  //   Weekly digest — streamed suggestions (merge / retitle /
  //                   missing-tags) over the last N days.
  //   Orphans       — deterministic list of notes with zero
  //                   incoming wikilinks; AI-proposed backlink
  //                   candidates per orphan are opt-in (one extra
  //                   request, AI cost per click).
  //
  // Both share the same suggestion + apply pattern: AI proposes,
  // user accepts each one explicitly. No batch-apply, no silent
  // edits — these features cross-write the vault and the consent
  // posture is "every change confirmed."
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import {
    api,
    type MaintenanceSuggestion,
    type OrphanNote,
    type BacklinkSuggestion
  } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';

  type Tab = 'digest' | 'orphans';
  let tab = $state<Tab>('digest');

  // ── Weekly digest state ───────────────────────────────────────────
  // suggestions accumulates as the SSE stream arrives. dismissedKeys
  // is client-side only — dismissals don't round-trip to the server
  // (a dismissed merge proposal is gone for this session; no
  // persistent "don't suggest again" memory).
  let lookbackDays = $state<number>(7);
  let digestRunning = $state(false);
  let digestController: AbortController | null = $state(null);
  let suggestions = $state<MaintenanceSuggestion[]>([]);
  let dismissedKeys = $state<Set<string>>(new Set());
  let appliedKeys = $state<Set<string>>(new Set());
  let digestError = $state<string | null>(null);

  // Stable key for each suggestion so the {#each} can dedup as the
  // stream lands. The server's prompt isn't deterministic — running
  // the same digest twice in a row may yield variant suggestions —
  // but within one run we want each card to render exactly once.
  function suggestionKey(s: MaintenanceSuggestion): string {
    switch (s.kind) {
      case 'merge':
        return 'merge|' + s.notes.slice().sort().join('|');
      case 'retitle':
        return 'retitle|' + s.note + '|' + s.suggestedTitle;
      case 'missing-tags':
        return 'tags|' + s.note + '|' + s.suggestedTags.join(',');
      case 'add-backlink':
        return 'bl|' + s.fromNotePath + '|' + s.toNotePath;
    }
  }

  function startDigest() {
    if (digestRunning) return;
    suggestions = [];
    dismissedKeys = new Set();
    appliedKeys = new Set();
    digestError = null;
    digestRunning = true;
    digestController = api.maintenanceWeeklyDigest(
      {
        onSuggestion: (s) => {
          // Dedup by key on arrival — cheaper than re-key in the
          // template and avoids the "card flashes in twice" race
          // when an SSE flush happens to deliver near-duplicates.
          const key = suggestionKey(s);
          if (suggestions.some((x) => suggestionKey(x) === key)) return;
          suggestions = [...suggestions, s];
        },
        onDone: () => {
          digestRunning = false;
          digestController = null;
          if (suggestions.length === 0) {
            toast.info('Digest complete — no suggestions for this window.');
          }
        },
        onError: (err) => {
          digestRunning = false;
          digestController = null;
          digestError = err.message;
        }
      },
      { lookbackDays }
    );
  }

  function stopDigest() {
    digestController?.abort();
    digestController = null;
    digestRunning = false;
  }

  async function accept(s: MaintenanceSuggestion) {
    const key = suggestionKey(s);
    try {
      const r = await api.maintenanceApply(s);
      if (r.next === 'merge-prep') {
        // Cross-note merge is refused server-side for safety. Send
        // the user to the longest note (server returned its path)
        // so they can manually pull the others in.
        toast.info('Merges need manual review — opening the target note.');
        if (r.target) location.href = '/notes/' + encodeURIComponent(r.target);
        return;
      }
      appliedKeys = new Set([...appliedKeys, key]);
      toast.success('Applied.');
    } catch (e) {
      toast.error('Apply failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  function dismiss(s: MaintenanceSuggestion) {
    dismissedKeys = new Set([...dismissedKeys, suggestionKey(s)]);
  }

  let visibleSuggestions = $derived(
    suggestions.filter((s) => {
      const k = suggestionKey(s);
      return !dismissedKeys.has(k) && !appliedKeys.has(k);
    })
  );

  // ── Orphans state ─────────────────────────────────────────────────
  // orphans is the deterministic list (one round trip). Per-orphan AI
  // suggestions are opt-in: clicking "Suggest backlinks" re-fetches
  // THE ENTIRE LIST with ?suggest=1 to keep things simple. A more
  // surgical per-orphan refetch is possible but the deterministic
  // scan is cheap and the AI call latency dominates either way.
  let orphans = $state<OrphanNote[]>([]);
  let orphansLoading = $state(false);
  let orphansLoaded = $state(false);
  let orphansSuggesting = $state(false);
  let orphansError = $state<string | null>(null);

  async function loadOrphans(withSuggest = false) {
    if (withSuggest) orphansSuggesting = true;
    else orphansLoading = true;
    orphansError = null;
    try {
      const r = await api.maintenanceOrphans({ suggest: withSuggest });
      orphans = r.orphans;
      orphansLoaded = true;
    } catch (e) {
      orphansError = e instanceof Error ? e.message : String(e);
    } finally {
      orphansLoading = false;
      orphansSuggesting = false;
    }
  }

  async function addBacklink(orphan: OrphanNote, source: BacklinkSuggestion) {
    try {
      await api.maintenanceApply({
        kind: 'add-backlink',
        fromNotePath: source.from,
        toNotePath: orphan.path,
        anchorText: source.excerpt
      });
      toast.success(`Linked from ${source.from}`);
      // Drop the consumed suggestion from this orphan's list so the
      // user doesn't accidentally add the same backlink twice.
      orphans = orphans.map((o) =>
        o.path === orphan.path
          ? {
              ...o,
              suggestedBacklinks: (o.suggestedBacklinks ?? []).filter((b) => b.from !== source.from)
            }
          : o
      );
    } catch (e) {
      toast.error('Add backlink failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  function fmtRelative(iso: string): string {
    try {
      const d = new Date(iso);
      const days = Math.floor((Date.now() - d.getTime()) / 86_400_000);
      if (days < 1) return 'today';
      if (days < 2) return 'yesterday';
      if (days < 7) return `${days}d ago`;
      if (days < 30) return `${Math.floor(days / 7)}w ago`;
      if (days < 365) return `${Math.floor(days / 30)}mo ago`;
      return `${Math.floor(days / 365)}y ago`;
    } catch {
      return '';
    }
  }

  onMount(() => {
    if (!$auth) return;
    // Don't auto-run the AI digest (would burn tokens on every visit).
    // Do pre-fetch the deterministic orphan list so the user lands
    // with data on the other tab.
    loadOrphans(false);
  });
</script>

<svelte:head>
  <title>Maintenance · Granit</title>
</svelte:head>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    <PageHeader
      title="Maintenance"
      subtitle="AI proposes vault hygiene; you accept each change explicitly."
    />

    <!-- Tab strip -->
    <nav class="flex gap-1 mb-4 text-sm">
      <button
        type="button"
        onclick={() => (tab = 'digest')}
        class="px-3 py-1.5 rounded {tab === 'digest' ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1 border border-surface1'}"
      >Weekly digest <span class="opacity-70 ml-1">{visibleSuggestions.length}</span></button>
      <button
        type="button"
        onclick={() => { tab = 'orphans'; if (!orphansLoaded && !orphansLoading) loadOrphans(false); }}
        class="px-3 py-1.5 rounded {tab === 'orphans' ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1 border border-surface1'}"
      >Orphans <span class="opacity-70 ml-1">{orphans.length}</span></button>
    </nav>

    {#if tab === 'digest'}
      <!-- Digest controls — lookback selector + run / stop. Lookback
           is the same as the server's default of 7 days unless the
           user changes it. -->
      <div class="flex items-center gap-2 mb-4 flex-wrap">
        <label class="text-xs text-dim">Lookback:</label>
        {#each [7, 14, 30] as days}
          <button
            type="button"
            onclick={() => (lookbackDays = days)}
            disabled={digestRunning}
            class="text-xs px-2 py-1 rounded border {lookbackDays === days ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'} disabled:opacity-50"
          >{days}d</button>
        {/each}
        <span class="flex-1"></span>
        {#if digestRunning}
          <button
            type="button"
            onclick={stopDigest}
            class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded text-warning hover:border-warning"
          >Stop</button>
        {:else}
          <button
            type="button"
            onclick={startDigest}
            class="px-4 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90"
          >Run digest</button>
        {/if}
      </div>

      {#if digestError}
        <div class="mb-3 p-3 bg-surface0 border border-error/40 rounded text-sm text-error">
          {digestError}
        </div>
      {/if}

      {#if digestRunning && suggestions.length === 0}
        <div class="text-sm text-dim italic flex items-center gap-2">
          <span class="inline-block w-2 h-2 rounded-full bg-primary animate-pulse"></span>
          AI is scanning the last {lookbackDays} days…
        </div>
      {/if}

      {#if visibleSuggestions.length === 0 && !digestRunning && !digestError}
        <p class="text-sm text-dim italic">
          Click <span class="text-text">Run digest</span> to scan the last {lookbackDays} days for hygiene suggestions.
        </p>
      {/if}

      <ul class="space-y-2">
        {#each visibleSuggestions as s (suggestionKey(s))}
          <li class="bg-surface0 border border-surface1 rounded p-3 group">
            <div class="flex items-start gap-3">
              <div class="flex-1 min-w-0">
                <!-- Kind badge + per-kind body -->
                {#if s.kind === 'merge'}
                  <div class="flex items-baseline gap-2 flex-wrap mb-1">
                    <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-warning">merge</span>
                    <span class="text-xs text-dim">{s.notes.length} notes</span>
                  </div>
                  <div class="text-sm text-text">
                    {#each s.notes as n}
                      <a href="/notes/{encodeURIComponent(n)}" class="font-mono hover:text-primary mr-2 break-all">{n}</a>
                    {/each}
                  </div>
                  {#if s.reason}<p class="text-xs text-subtext mt-1">{s.reason}</p>{/if}
                {:else if s.kind === 'retitle'}
                  <div class="flex items-baseline gap-2 flex-wrap mb-1">
                    <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-secondary">retitle</span>
                    <a href="/notes/{encodeURIComponent(s.note)}" class="text-xs font-mono text-dim hover:text-primary break-all">{s.note}</a>
                  </div>
                  <div class="text-sm">
                    {#if s.currentTitle}<span class="text-dim line-through mr-2">{s.currentTitle}</span>{/if}
                    <span class="text-text font-medium">{s.suggestedTitle}</span>
                  </div>
                  {#if s.reason}<p class="text-xs text-subtext mt-1">{s.reason}</p>{/if}
                {:else if s.kind === 'missing-tags'}
                  <div class="flex items-baseline gap-2 flex-wrap mb-1">
                    <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-info">tags</span>
                    <a href="/notes/{encodeURIComponent(s.note)}" class="text-xs font-mono text-dim hover:text-primary break-all">{s.note}</a>
                  </div>
                  <div class="flex flex-wrap gap-1">
                    {#each s.suggestedTags as t}
                      <span class="text-xs px-1.5 py-0.5 rounded bg-surface1 text-secondary">#{t}</span>
                    {/each}
                  </div>
                  {#if s.reason}<p class="text-xs text-subtext mt-1">{s.reason}</p>{/if}
                {:else if s.kind === 'add-backlink'}
                  <div class="flex items-baseline gap-2 flex-wrap mb-1">
                    <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-info">backlink</span>
                  </div>
                  <div class="text-sm text-text">
                    Link <a href="/notes/{encodeURIComponent(s.fromNotePath)}" class="font-mono hover:text-primary">{s.fromNotePath}</a>
                    → <a href="/notes/{encodeURIComponent(s.toNotePath)}" class="font-mono hover:text-primary">{s.toNotePath}</a>
                  </div>
                {/if}
              </div>
              <div class="flex items-center gap-1 flex-shrink-0">
                <button
                  type="button"
                  onclick={() => accept(s)}
                  class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded hover:opacity-90"
                >Accept</button>
                <button
                  type="button"
                  onclick={() => dismiss(s)}
                  class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-dim hover:text-text hover:border-overlay0"
                >Dismiss</button>
              </div>
            </div>
          </li>
        {/each}
      </ul>
    {:else if tab === 'orphans'}
      <div class="flex items-center gap-2 mb-4 flex-wrap">
        <button
          type="button"
          onclick={() => loadOrphans(false)}
          disabled={orphansLoading}
          class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50"
        >{orphansLoading ? 'refreshing…' : 'Refresh'}</button>
        <button
          type="button"
          onclick={() => loadOrphans(true)}
          disabled={orphansLoading || orphansSuggesting || orphans.length === 0}
          class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded hover:opacity-90 disabled:opacity-50"
          title="Re-fetch with AI-suggested backlink sources for each orphan"
        >{orphansSuggesting ? 'suggesting…' : 'Suggest backlinks (AI)'}</button>
      </div>

      {#if orphansError}
        <div class="mb-3 p-3 bg-surface0 border border-error/40 rounded text-sm text-error">
          {orphansError}
        </div>
      {/if}

      {#if orphans.length === 0 && orphansLoaded}
        <p class="text-sm text-dim italic">No orphans — every note has at least one incoming wikilink.</p>
      {/if}

      <ul class="space-y-2">
        {#each orphans as o (o.path)}
          <li class="bg-surface0 border border-surface1 rounded p-3">
            <div class="flex items-baseline justify-between gap-3 flex-wrap mb-1">
              <a href="/notes/{encodeURIComponent(o.path)}" class="text-sm text-text font-medium hover:text-primary truncate">
                {o.title}
              </a>
              <span class="text-[11px] text-dim flex-shrink-0">{fmtRelative(o.modTime)}</span>
            </div>
            <div class="text-[11px] text-dim font-mono truncate mb-2">{o.path}</div>
            {#if o.suggestedBacklinks && o.suggestedBacklinks.length > 0}
              <div class="space-y-1">
                {#each o.suggestedBacklinks as b}
                  <div class="flex items-baseline gap-2 text-xs">
                    <button
                      type="button"
                      onclick={() => addBacklink(o, b)}
                      class="px-2 py-0.5 rounded bg-primary text-on-primary hover:opacity-90 flex-shrink-0"
                      title="Add a wikilink from {b.from} to {o.title}"
                    >+ link from</button>
                    <a href="/notes/{encodeURIComponent(b.from)}" class="font-mono text-subtext hover:text-primary truncate">{b.from}</a>
                  </div>
                  {#if b.excerpt}
                    <p class="text-[11px] text-dim italic pl-1 truncate" title={b.excerpt}>"{b.excerpt}"</p>
                  {/if}
                {/each}
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>
