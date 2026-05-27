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
  // Mobile overflow menu state. On phones the bar collapses to four
  // primary icons + a ⋯ button that opens this sheet for the rest of
  // the actions. Closed by tapping the backdrop or running an action.
  let mobileMoreOpen = $state(false);
  let mobileSnoozeOpen = $state(false);
  let mobileSnoozeAnchor: HTMLElement | undefined = $state();

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

  // Bulk due date — accepts an ISO date string from the date input.
  // Empty string clears the due date.
  async function bulkDueDate(date: string) {
    await applyAll(date ? `due → ${date}` : 'due cleared',
      (id) => api.patchTask(id, { dueDate: date }));
  }
  async function bulkDelete() {
    if (!confirm(`Delete ${ids.length} task${ids.length === 1 ? '' : 's'}? This cannot be undone.`)) return;
    await applyAll('deleted', (id) => api.deleteTask(id));
  }
</script>

<!-- Desktop bar — unchanged layout, just gated to sm+. The mobile
     variant below is a fixed floating strip pinned to the bottom of
     the viewport with icon-only buttons. -->
<div class="hidden sm:flex px-3 py-2 border-b border-surface1 bg-surface1 flex-wrap items-center gap-2 flex-shrink-0">
  <span class="text-sm font-medium text-primary">{count} selected</span>
  <button onclick={onClear} class="text-xs text-dim hover:text-text">clear</button>
  <span class="flex-1"></span>

  <button onclick={bulkDone} disabled={busy} class="px-2.5 py-1 text-xs bg-surface0 text-success rounded hover:bg-surface1 disabled:opacity-50">✓ done</button>
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
      class="px-2.5 py-1 text-xs bg-surface0 text-warning rounded hover:bg-surface1 disabled:opacity-50"
    >snooze</button>
    {#if snoozeOpen}
      <SnoozePicker anchor={snoozeAnchor} onPick={bulkSnooze} onClose={() => (snoozeOpen = false)} />
    {/if}
  </span>

  <button onclick={bulkUnsnooze} disabled={busy} class="px-2.5 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2 disabled:opacity-50 hidden sm:inline-block">wake</button>

  <span class="hidden sm:inline-block w-px h-4 bg-surface1"></span>

  <!-- Bulk-set due date. Triggers on input change, then clears the
       value so the same date can be applied twice in a row if the
       user wants. The "clear" button next to it bulk-removes the
       due date. -->
  <label class="hidden sm:flex items-center gap-1 text-xs text-dim" title="Bulk-set due date for selected tasks">
    due
    <input
      type="date"
      onchange={(e) => {
        const v = (e.target as HTMLInputElement).value;
        if (v) { void bulkDueDate(v); (e.target as HTMLInputElement).value = ''; }
      }}
      disabled={busy}
      class="bg-surface0 border border-surface1 rounded px-1.5 py-0.5 text-xs text-text"
    />
  </label>
  <button onclick={() => bulkDueDate('')} disabled={busy} class="px-2 py-1 text-[11px] bg-surface1 text-subtext rounded hover:bg-surface2 disabled:opacity-50 hidden md:inline-block" title="Clear due date">clear due</button>

  <span class="hidden sm:inline-block w-px h-4 bg-surface1"></span>

  <button onclick={bulkDelete} disabled={busy} class="px-2.5 py-1 text-xs bg-surface0 text-error rounded hover:bg-surface1 disabled:opacity-50">🗑 delete</button>
</div>

<!-- Mobile floating bar. Pinned to the viewport bottom so it doesn't
     compete with the toolbar at the top for vertical space. Four
     primary actions inline as 40×40 icon buttons (done · snooze ·
     delete · more) plus the selection-count chip on the left. The ⋯
     button opens a sheet with the remaining actions (re-open,
     priority, triage, wake, due date, clear due). All actions wire
     through the same handlers as the desktop bar — this is purely a
     presentational pivot. -->
<div class="sm:hidden fixed left-0 right-0 bottom-0 z-30 bg-surface1 border-t border-surface2 flex items-center gap-1 px-2 py-1.5 pb-[calc(0.375rem+env(safe-area-inset-bottom,0px))] shadow-2xl">
  <span class="text-xs font-medium text-primary px-1.5 tabular-nums">{count}</span>
  <button onclick={onClear} class="text-[10px] text-dim hover:text-text px-1" aria-label="clear selection">clear</button>
  <span class="flex-1"></span>
  <button
    onclick={bulkDone}
    disabled={busy}
    class="w-10 h-10 flex items-center justify-center text-success bg-surface0 rounded disabled:opacity-50 active:bg-surface2"
    title="mark done"
    aria-label="mark done"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12l5 5L20 7"/></svg>
  </button>
  <button
    bind:this={mobileSnoozeAnchor}
    onclick={() => (mobileSnoozeOpen = !mobileSnoozeOpen)}
    disabled={busy}
    class="w-10 h-10 flex items-center justify-center text-warning bg-surface0 rounded disabled:opacity-50 active:bg-surface2"
    title="snooze"
    aria-label="snooze"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
  </button>
  {#if mobileSnoozeOpen}
    <SnoozePicker anchor={mobileSnoozeAnchor} onPick={(u) => { mobileSnoozeOpen = false; void bulkSnooze(u); }} onClose={() => (mobileSnoozeOpen = false)} />
  {/if}
  <button
    onclick={bulkDelete}
    disabled={busy}
    class="w-10 h-10 flex items-center justify-center text-error bg-surface0 rounded disabled:opacity-50 active:bg-surface2"
    title="delete"
    aria-label="delete selected"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/></svg>
  </button>
  <button
    onclick={() => (mobileMoreOpen = true)}
    class="w-10 h-10 flex items-center justify-center text-subtext bg-surface0 rounded active:bg-surface2"
    title="more actions"
    aria-label="more actions"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><circle cx="5" cy="12" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="19" cy="12" r="2"/></svg>
  </button>
</div>

<!-- Mobile overflow sheet — slides up from the bottom with the
     actions that don't fit inline. Tapping the backdrop closes;
     each action closes the sheet on success. -->
{#if mobileMoreOpen}
  <button
    type="button"
    onclick={() => (mobileMoreOpen = false)}
    aria-label="close more actions"
    class="sm:hidden fixed inset-0 z-40 bg-black/50"
  ></button>
  <div
    class="sm:hidden fixed left-0 right-0 bottom-0 z-50 bg-mantle border-t border-surface1 rounded-t-xl shadow-2xl pb-[calc(0.75rem+env(safe-area-inset-bottom,0px))] max-h-[80vh] overflow-y-auto"
    role="dialog"
    aria-label="More bulk actions"
  >
    <div class="flex justify-center pt-2 pb-1" aria-hidden="true">
      <span class="block w-10 h-1 bg-surface2 rounded-full"></span>
    </div>
    <div class="px-3 py-2 space-y-3">
      <div>
        <h3 class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Status</h3>
        <div class="grid grid-cols-2 gap-2">
          <button onclick={() => { mobileMoreOpen = false; void bulkOpen(); }} disabled={busy} class="px-3 py-2 text-sm bg-surface0 text-subtext rounded border border-surface1 active:bg-surface1 disabled:opacity-50">re-open</button>
          <button onclick={() => { mobileMoreOpen = false; void bulkUnsnooze(); }} disabled={busy} class="px-3 py-2 text-sm bg-surface0 text-subtext rounded border border-surface1 active:bg-surface1 disabled:opacity-50">wake</button>
        </div>
      </div>
      <div>
        <h3 class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Priority</h3>
        <div class="grid grid-cols-4 gap-2">
          <button onclick={() => { mobileMoreOpen = false; void bulkPriority(1); }} disabled={busy} class="px-2 py-2 text-sm bg-surface0 text-error rounded border border-surface1 active:bg-surface1 disabled:opacity-50 font-mono">P1</button>
          <button onclick={() => { mobileMoreOpen = false; void bulkPriority(2); }} disabled={busy} class="px-2 py-2 text-sm bg-surface0 text-warning rounded border border-surface1 active:bg-surface1 disabled:opacity-50 font-mono">P2</button>
          <button onclick={() => { mobileMoreOpen = false; void bulkPriority(3); }} disabled={busy} class="px-2 py-2 text-sm bg-surface0 text-info rounded border border-surface1 active:bg-surface1 disabled:opacity-50 font-mono">P3</button>
          <button onclick={() => { mobileMoreOpen = false; void bulkPriority(0); }} disabled={busy} class="px-2 py-2 text-sm bg-surface0 text-dim rounded border border-surface1 active:bg-surface1 disabled:opacity-50 font-mono">P0</button>
        </div>
      </div>
      <div>
        <h3 class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Triage</h3>
        <div class="grid grid-cols-3 gap-2">
          {#each ['inbox', 'triaged', 'scheduled', 'done', 'dropped', 'snoozed'] as st (st)}
            <button
              onclick={() => { mobileMoreOpen = false; void bulkTriage(st as Parameters<typeof bulkTriage>[0]); }}
              disabled={busy}
              class="px-2 py-2 text-sm bg-surface0 text-subtext rounded border border-surface1 active:bg-surface1 disabled:opacity-50"
            >{st}</button>
          {/each}
        </div>
      </div>
      <div>
        <h3 class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Due date</h3>
        <div class="flex items-center gap-2">
          <input
            type="date"
            onchange={(e) => {
              const v = (e.target as HTMLInputElement).value;
              if (v) { mobileMoreOpen = false; void bulkDueDate(v); }
            }}
            disabled={busy}
            class="flex-1 bg-surface0 border border-surface1 rounded px-3 py-2 text-text"
          />
          <button
            onclick={() => { mobileMoreOpen = false; void bulkDueDate(''); }}
            disabled={busy}
            class="px-3 py-2 text-sm bg-surface0 text-subtext rounded border border-surface1 active:bg-surface1 disabled:opacity-50"
            title="Clear due date"
          >clear</button>
        </div>
      </div>
    </div>
  </div>
{/if}
