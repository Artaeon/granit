<script lang="ts">
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import SnoozePicker from './SnoozePicker.svelte';

  let { count, ids, onClear, onChanged }: {
    count: number;
    ids: string[];
    onClear: () => void;
    onChanged: () => void | Promise<void>;
  } = $props();

  let busy = $state(false);
  let snoozeOpen = $state(false);
  let snoozeAnchor: HTMLElement | undefined = $state();

  // applyAll runs `op` against every selected task, surfacing a single toast
  // with the success/failure tally at the end. Errors don't abort the loop —
  // a partial failure (e.g. one task no longer exists) shouldn't block the rest.
  async function applyAll(label: string, op: (id: string) => Promise<unknown>) {
    busy = true;
    let ok = 0, fail = 0;
    try {
      for (const id of ids) {
        try { await op(id); ok++; } catch { fail++; }
      }
      if (fail === 0) toast.success(`${label}: ${ok}`);
      else toast.warning(`${label}: ${ok} ok, ${fail} failed`);
      await onChanged();
    } finally {
      busy = false;
    }
  }

  async function bulkDone() { await applyAll('marked done', (id) => api.patchTask(id, { done: true })); }
  async function bulkOpen() { await applyAll('reopened', (id) => api.patchTask(id, { done: false })); }
  async function bulkPriority(p: number) { await applyAll(`priority → P${p || '—'}`, (id) => api.patchTask(id, { priority: p })); }
  async function bulkTriage(state: 'inbox' | 'triaged' | 'scheduled' | 'done' | 'dropped' | 'snoozed') {
    await applyAll(`triage → ${state}`, (id) => api.patchTask(id, { triage: state }));
  }
  async function bulkSnooze(until: string) {
    snoozeOpen = false;
    await applyAll('snoozed', (id) => api.patchTask(id, { snoozedUntil: until }));
  }
  async function bulkUnsnooze() {
    await applyAll('woken', (id) => api.patchTask(id, { snoozedUntil: '' }));
  }
</script>

<div class="px-3 py-2 border-b border-surface1 bg-primary/10 flex flex-wrap items-center gap-2 flex-shrink-0">
  <span class="text-sm font-medium text-primary">{count} selected</span>
  <button onclick={onClear} class="text-xs text-dim hover:text-text">clear</button>
  <span class="flex-1"></span>

  <button onclick={bulkDone} disabled={busy} class="px-2.5 py-1 text-xs bg-success/20 text-success rounded hover:bg-success/30 disabled:opacity-50">✓ done</button>
  <button onclick={bulkOpen} disabled={busy} class="px-2.5 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2 disabled:opacity-50 hidden sm:inline-block">re-open</button>

  <span class="hidden sm:inline-block w-px h-4 bg-surface1"></span>

  <select
    onchange={(e) => { const v = (e.target as HTMLSelectElement).value; if (v !== '') { bulkPriority(Number(v)); (e.target as HTMLSelectElement).value = ''; } }}
    disabled={busy}
    class="text-xs bg-surface0 border border-surface1 rounded px-2 py-1 text-text hidden sm:inline-block"
  >
    <option value="">priority…</option>
    <option value="1">P1</option>
    <option value="2">P2</option>
    <option value="3">P3</option>
    <option value="0">none</option>
  </select>

  <select
    onchange={(e) => { const v = (e.target as HTMLSelectElement).value; if (v !== '') { bulkTriage(v as Parameters<typeof bulkTriage>[0]); (e.target as HTMLSelectElement).value = ''; } }}
    disabled={busy}
    class="text-xs bg-surface0 border border-surface1 rounded px-2 py-1 text-text"
  >
    <option value="">triage…</option>
    <option value="inbox">inbox</option>
    <option value="triaged">triaged</option>
    <option value="scheduled">scheduled</option>
    <option value="done">done</option>
    <option value="dropped">dropped</option>
    <option value="snoozed">snoozed</option>
  </select>

  <span class="relative inline-block">
    <button
      bind:this={snoozeAnchor}
      onclick={() => (snoozeOpen = !snoozeOpen)}
      disabled={busy}
      class="px-2.5 py-1 text-xs bg-warning/20 text-warning rounded hover:bg-warning/30 disabled:opacity-50"
    >snooze</button>
    {#if snoozeOpen}
      <SnoozePicker anchor={snoozeAnchor} onPick={bulkSnooze} onClose={() => (snoozeOpen = false)} />
    {/if}
  </span>

  <button onclick={bulkUnsnooze} disabled={busy} class="px-2.5 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2 disabled:opacity-50 hidden sm:inline-block">wake</button>
</div>
