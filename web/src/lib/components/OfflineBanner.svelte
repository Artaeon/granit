<script lang="ts">
  import { fly } from 'svelte/transition';
  import { isOnline, wasOffline } from '$lib/stores/online';
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { flushDrafts, queuedDraftCount, listDrafts } from '$lib/notes/drafts';

  // Auto-flush queued note drafts on offline → online transition.
  // Runs once per back-online flip; the wasOffline subscriber sets
  // a flag on flip so we don't fire on every render.
  let queued = $state(0);
  let flushing = $state(false);
  let lastFlushReport = $state<{ ok: number; fail: number } | null>(null);
  let armed = false; // becomes true when we go offline; flips back on flush

  // The note the user is currently editing — derived from the URL.
  // We exclude this path from the queued-draft count and from
  // flushDrafts(), because the open editor owns its own draft
  // lifecycle (per-keystroke setDraft → clearDraft on save). If we
  // counted it here, the blue "N drafts pending" pill would flicker
  // on every successful autosave bounce (the editor briefly re-writes
  // a draft when the user typed during the save's await). Worse, a
  // manual retry click would PUT the localStorage draft body and race
  // with the editor's live body — the older snapshot can win and
  // overwrite recent keystrokes.
  let currentNotePath = $derived.by<string | null>(() => {
    const route = $page.route?.id ?? '';
    if (!route.startsWith('/notes/')) return null;
    const param = $page.params?.path;
    if (!param) return null;
    try { return decodeURIComponent(param); } catch { return param; }
  });

  function refreshCount() {
    // Count only drafts NOT belonging to the currently-open note.
    // listDrafts walks the index; cheap (≤12 entries by design).
    const open = currentNotePath;
    queued = listDrafts().filter((d) => d.path !== open).length;
  }
  // Re-evaluate on path change so navigating between notes immediately
  // updates the badge without waiting for the polling tick.
  $effect(() => {
    void currentNotePath;
    refreshCount();
  });
  onMount(() => {
    refreshCount();
    // The drafts module mutates localStorage from another component
    // (the note editor). storage events fire across-tab; same-tab
    // updates need a polling refresh on a timer. 3s is plenty —
    // this number is a soft hint, not a real-time gauge.
    const id = setInterval(refreshCount, 3000);
    return () => clearInterval(id);
  });

  // Track offline → online transitions to fire flush exactly once
  // per real reconnect. Subscribing rather than $effect because the
  // store is a writable boolean and we want a level-triggered hook
  // (off→on) not edge-triggered (changed at all).
  let prevOnline = true;
  isOnline.subscribe((on) => {
    if (!on) {
      armed = true;
    } else if (armed && !prevOnline) {
      armed = false;
      void doFlush();
    }
    prevOnline = on;
  });

  async function doFlush() {
    if (flushing) return;
    if (queuedDraftCount() === 0) return;
    flushing = true;
    try {
      // Pass the active note's path so flushDrafts skips it. The open
      // editor's own autosave loop owns that draft; concurrent PUTs
      // here would race and the older draft body could clobber live
      // keystrokes (root cause of "my last few edits disappeared
      // after clicking retry"). flushDrafts already had this hook;
      // we just weren't using it.
      const open = currentNotePath ?? undefined;
      const report = await flushDrafts(async (path, body) => {
        const updated = await api.putNote(path, body);
        return { modTime: updated.modTime };
      }, open);
      lastFlushReport = { ok: report.succeeded, fail: report.failed };
      if (report.succeeded > 0) {
        toast.success(`Synced ${report.succeeded} draft${report.succeeded === 1 ? '' : 's'}`);
      }
      if (report.failed > 0) {
        toast.error(`${report.failed} draft${report.failed === 1 ? '' : 's'} couldn't sync — will retry`);
      }
    } finally {
      flushing = false;
      refreshCount();
      // Auto-clear the report after 4s so the banner doesn't sit on
      // a stale message.
      if (lastFlushReport) setTimeout(() => (lastFlushReport = null), 4000);
    }
  }
</script>

{#if !$isOnline}
  <div
    in:fly={{ y: -10, duration: 180 }}
    class="md:hidden fixed top-12 inset-x-0 z-30 px-3 py-1.5 bg-warning text-on-primary text-xs flex items-center gap-2"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M2 2l20 20M8.5 16.5a5 5 0 0 1 7 0M5 12.55a11 11 0 0 1 14.08-1.4M1.42 9a16 16 0 0 1 4.5-3.07M9 19.5h.01" />
    </svg>
    <span class="flex-1">
      Offline — viewing cached data.
      {#if queued > 0}
        {queued} draft{queued === 1 ? '' : 's'} will sync when you're back.
      {:else}
        Edits queue for retry.
      {/if}
    </span>
  </div>
  <div
    in:fly={{ x: 10, duration: 180 }}
    class="hidden md:flex fixed top-3 right-3 z-30 px-3 py-1.5 rounded-lg bg-warning text-on-primary text-xs items-center gap-2"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M2 2l20 20M8.5 16.5a5 5 0 0 1 7 0M5 12.55a11 11 0 0 1 14.08-1.4M1.42 9a16 16 0 0 1 4.5-3.07" />
    </svg>
    <span>Offline</span>
    {#if queued > 0}
      <span class="px-1.5 py-0.5 rounded-full bg-mantle text-warning font-mono text-[10px]" title={`${queued} drafts queued`}>{queued}</span>
    {/if}
  </div>
{:else if $wasOffline || flushing || lastFlushReport}
  <div
    in:fly={{ x: 10, duration: 180 }}
    class="fixed top-3 right-3 z-30 px-3 py-1.5 rounded-lg bg-success text-on-primary text-xs flex items-center gap-2"
  >
    {#if flushing}
      <span>↻ Syncing {queued} draft{queued === 1 ? '' : 's'}…</span>
    {:else if lastFlushReport && lastFlushReport.ok > 0}
      <span>✓ Back online · synced {lastFlushReport.ok}</span>
    {:else}
      <span>✓ Back online</span>
    {/if}
  </div>
{:else if queued > 0 && $isOnline}
  <!-- Drafts pending while online: typically because the server
       returned 5xx earlier, the WS reconnected but the auto-flush
       failed. Quiet pill with a manual retry so the user isn't
       stuck. -->
  <button
    type="button"
    onclick={doFlush}
    disabled={flushing}
    class="fixed top-3 right-3 z-30 px-3 py-1.5 rounded-lg bg-info text-on-primary text-xs flex items-center gap-2 hover:opacity-90"
    title={`${queued} drafts queued — click to retry now`}
  >
    <span>{queued} draft{queued === 1 ? '' : 's'} pending</span>
    <span class="opacity-70">retry</span>
  </button>
{/if}
