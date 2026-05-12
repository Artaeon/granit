<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type AIAuditEntry } from '$lib/api';

  // AIUsageWidget — streamlined dashboard tile for today's AI
  // spend. The full breakdown lives in /settings (audit log,
  // per-feature buckets, week rollup); this widget surfaces the
  // three numbers that actually move daily decisions: how many
  // calls, how many tokens, how much money. Cost-conscious LLM
  // use needs ambient awareness, not buried-three-clicks-deep
  // awareness.
  //
  // No WS subscription — AI calls don't emit a granular event
  // the registry knows about, and the audit endpoint is cheap.
  // Refreshes on mount and via tab-visibility change so a user
  // returning to the dashboard sees fresh numbers without a
  // hard reload.

  let entries = $state<AIAuditEntry[]>([]);
  let loaded = $state(false);
  let error = $state(false);

  async function load() {
    try {
      const r = await api.getAIAudit();
      entries = r.entries ?? [];
      error = false;
    } catch {
      // AI audit may be 404 on builds without AI features
      // wired up — soft-fail to a "no usage" state rather than
      // a scary error tile on the home page.
      entries = [];
      error = true;
    } finally {
      loaded = true;
    }
  }

  onMount(() => {
    load();
    const onVis = () => {
      if (document.visibilityState === 'visible') load();
    };
    document.addEventListener('visibilitychange', onVis);
    return () => document.removeEventListener('visibilitychange', onVis);
  });

  // Roll up entries since local-midnight. Mirrors the "today"
  // bucket in /settings so the headline number on the dashboard
  // matches what the user sees in the audit page.
  const today = $derived.by(() => {
    const start = new Date();
    start.setHours(0, 0, 0, 0);
    const startMs = start.getTime();
    let count = 0;
    let tokens = 0;
    let microCents = 0;
    let errors = 0;
    for (const e of entries) {
      const t = new Date(e.timestamp).getTime();
      if (Number.isNaN(t) || t < startMs) continue;
      count++;
      tokens += (e.prompt_tokens ?? 0) + (e.completion_tokens ?? 0);
      microCents += e.cost_micro_cents ?? 0;
      if (e.error) errors++;
    }
    return { count, tokens, microCents, errors };
  });

  function formatTokens(n: number): string {
    if (n < 1000) return `${n}`;
    if (n < 1_000_000) return `${(n / 1000).toFixed(1)}k`;
    return `${(n / 1_000_000).toFixed(2)}M`;
  }

  // Cost lives in micro-cents (1/1_000_000 of a cent) on the
  // audit entries — convert to dollars and trim trailing zeros
  // so "$0.0030" reads as "$0.003" and a clean nickel reads as
  // "$0.05" instead of "$0.0500". Same formatter as /settings.
  function formatCost(microCents: number): string {
    const dollars = microCents / 100_000_000;
    if (dollars === 0) return '$0';
    if (dollars < 0.0001) return '<$0.0001';
    let s = dollars.toFixed(4);
    s = s.replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '');
    return '$' + s;
  }
</script>

<div class="bg-surface0 border border-surface1 rounded-lg p-3">
  <header class="flex items-baseline gap-2 mb-3">
    <h3 class="text-sm font-medium text-text">AI usage</h3>
    <span class="flex-1"></span>
    <a href="/settings" class="text-[11px] text-secondary hover:underline">details →</a>
  </header>

  {#if !loaded}
    <div class="text-xs text-dim italic">Loading…</div>
  {:else if error}
    <p class="text-xs text-dim italic">AI audit unavailable.</p>
  {:else if today.count === 0}
    <p class="text-xs text-dim italic">No AI calls today.</p>
  {:else}
    <div class="flex items-baseline gap-2 mb-2">
      <span class="text-2xl font-semibold text-text">{formatCost(today.microCents)}</span>
      <span class="text-xs text-dim">today</span>
      {#if today.errors > 0}
        <span class="flex-1"></span>
        <span class="text-[11px] text-error" title="{today.errors} failed call{today.errors === 1 ? '' : 's'} today">
          {today.errors} err
        </span>
      {/if}
    </div>
    <div class="flex items-baseline gap-3 text-[11px] text-dim font-mono">
      <span>{today.count} call{today.count === 1 ? '' : 's'}</span>
      <span>·</span>
      <span>{formatTokens(today.tokens)} tok</span>
    </div>
  {/if}
</div>
