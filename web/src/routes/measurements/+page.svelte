<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type MeasurementSeries, type MeasurementEntry } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // /measurements is the numeric companion to /habits — habits are
  // yes/no toggles, measurements are values (weight, sleep, push-ups,
  // mood). One Series = one metric definition; Entries are the logged
  // values over time. Page shows current value cards + a quick-log
  // composer; click a card to drill into the series detail.

  let series = $state<MeasurementSeries[]>([]);
  let latest = $state<Record<string, MeasurementEntry>>({});
  let loading = $state(false);

  // Detail view state — when a series is selected, we fetch + render
  // its full entry history. Inline rather than a separate route to
  // keep the click-back path snappy.
  let selectedId = $state<string | null>(null);
  let detailEntries = $state<MeasurementEntry[]>([]);

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const r = await api.listMeasurementSeries();
      series = r.series;
      latest = r.latest;
    } catch (e) {
      toast.error('failed to load: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }
  async function loadDetail(id: string) {
    selectedId = id;
    try {
      const r = await api.listMeasurementEntries({ series: id });
      detailEntries = r.entries;
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type !== 'state.changed') return;
      if (!ev.path?.startsWith('.granit/measurements/')) return;
      load();
      if (selectedId) loadDetail(selectedId);
    });
  });

  // Format a value with its series unit, e.g. "78.5 kg".
  function fmtValue(v: number | null | undefined, unit: string): string {
    if (v === null || v === undefined || !Number.isFinite(v)) return '—';
    // 1 decimal where it matters, integer when round.
    const s = Math.abs(v) >= 100 || Number.isInteger(v) ? v.toFixed(0) : v.toFixed(1);
    return unit ? `${s} ${unit}` : s;
  }

  // Trend vs the prior entry — the detail view's per-row delta and
  // the index card's "↑/↓ vs last" hint. Direction-aware: for a
  // "down" series (weight, body fat) a decrease is good (success
  // colour); for "up" (push-ups, miles) an increase is good.
  function trendDelta(s: MeasurementSeries, current: number, prev: number | null): { delta: number; tone: string } | null {
    if (prev === null || prev === undefined) return null;
    const delta = current - prev;
    if (delta === 0) return { delta: 0, tone: 'text-dim' };
    const goodDirection = (s.direction ?? 'up') === 'down' ? -1 : 1;
    const tone = (delta > 0 ? 1 : -1) === goodDirection ? 'text-success' : 'text-warning';
    return { delta, tone };
  }

  // Target progress hint — "X away from target" with sign.
  function targetHint(s: MeasurementSeries, current: number): string {
    if (s.target === null || s.target === undefined) return '';
    const delta = current - s.target;
    if (delta === 0) return 'at target';
    const sign = delta > 0 ? '+' : '−';
    const abs = Math.abs(delta);
    const fmt = abs >= 100 || Number.isInteger(abs) ? abs.toFixed(0) : abs.toFixed(1);
    return `${sign}${fmt} ${s.unit} vs target`;
  }

  // ── Series modal (create/edit) ─────────────────────────────────────
  let modalOpen = $state(false);
  let editingId = $state<string | null>(null);
  let form = $state({ name: '', unit: '', target: '', direction: 'up' as 'up' | 'down', notes: '' });
  function openCreate() {
    editingId = null;
    form = { name: '', unit: '', target: '', direction: 'up', notes: '' };
    modalOpen = true;
  }
  function openEdit(s: MeasurementSeries) {
    editingId = s.id;
    form = {
      name: s.name,
      unit: s.unit,
      target: s.target !== undefined && s.target !== null ? String(s.target) : '',
      direction: ((s.direction as 'up' | 'down') ?? 'up'),
      notes: s.notes ?? ''
    };
    modalOpen = true;
  }
  async function submitForm() {
    if (!form.name.trim()) return;
    const body: Partial<MeasurementSeries> = {
      name: form.name.trim(),
      unit: form.unit.trim(),
      direction: form.direction,
      notes: form.notes.trim() || undefined
    };
    if (form.target.trim() !== '') {
      body.target = parseFloat(form.target);
    } else {
      // Patch must explicitly clear the target; create can omit.
      if (editingId) (body as Record<string, unknown>).target = null;
    }
    try {
      if (editingId) {
        await api.patchMeasurementSeries(editingId, body);
      } else {
        await api.createMeasurementSeries(body);
      }
      modalOpen = false;
      editingId = null;
      await load();
      toast.success(editingId ? 'updated' : 'added');
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function deleteSeries(s: MeasurementSeries) {
    if (!confirm(`Delete "${s.name}" and all its entries?`)) return;
    try {
      await api.deleteMeasurementSeries(s.id);
      if (selectedId === s.id) {
        selectedId = null;
        detailEntries = [];
      }
      await load();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // ── Quick-log: log a value against the selected series ─────────────
  let logValue = $state('');
  let logDate = $state(new Date().toISOString().slice(0, 10));
  async function submitLog(seriesId: string) {
    const v = parseFloat(logValue);
    if (!Number.isFinite(v)) return;
    try {
      await api.createMeasurementEntry({
        series_id: seriesId,
        date: logDate,
        value: v
      });
      logValue = '';
      logDate = new Date().toISOString().slice(0, 10);
      toast.success('logged');
      await load();
      if (selectedId === seriesId) await loadDetail(seriesId);
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function deleteEntry(e: MeasurementEntry) {
    if (!confirm(`Delete this entry from ${e.date}?`)) return;
    try {
      await api.deleteMeasurementEntry(e.id);
      await load();
      if (selectedId) await loadDetail(selectedId);
    } catch (err) {
      toast.error('failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }

  let activeSeries = $derived(series.filter((s) => !s.archived));
  let archivedSeries = $derived(series.filter((s) => s.archived));
  let selectedSeries = $derived(series.find((s) => s.id === selectedId) ?? null);
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-4xl mx-auto p-4 sm:p-6 lg:p-8">
    <PageHeader title="Measurements" subtitle="Numeric tracking — weight, sleep, exercise, mood, anything you want a number for" />

    <div class="flex items-baseline gap-3 flex-wrap mb-4">
      <span class="text-xs text-dim">{activeSeries.length} active{archivedSeries.length > 0 ? ` · ${archivedSeries.length} archived` : ''}</span>
      <span class="flex-1"></span>
      <button onclick={openCreate} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New metric</button>
    </div>

    {#if loading && series.length === 0}
      <p class="text-sm text-dim">loading…</p>
    {:else if series.length === 0}
      <div class="bg-surface0 border border-surface1 rounded-lg p-6 text-center">
        <p class="text-sm text-text">No metrics yet.</p>
        <p class="text-xs text-dim mt-1">Add what you want to track — weight, sleep hours, push-ups, mood — and log values over time.</p>
      </div>
    {:else}
      <!-- Card grid: one card per active series. Click selects for
           detail view + quick-log; current value + trend hints render
           inline. -->
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 mb-6">
        {#each activeSeries as s (s.id)}
          {@const cur = latest[s.id]}
          {@const isSelected = selectedId === s.id}
          <button
            type="button"
            onclick={() => loadDetail(s.id)}
            class="bg-surface0 border rounded-lg p-3 text-left transition-colors hover:border-primary/40 {isSelected ? 'border-primary' : 'border-surface1'}"
          >
            <div class="flex items-baseline gap-2">
              <h3 class="font-medium text-text truncate flex-1">{s.name}</h3>
              <span class="text-[10px] text-dim">{s.direction ?? 'up'}{s.target !== null && s.target !== undefined ? ` → ${fmtValue(s.target, s.unit)}` : ''}</span>
            </div>
            <p class="text-2xl font-semibold text-text mt-2 font-mono">
              {cur ? fmtValue(cur.value, s.unit) : '—'}
            </p>
            <p class="text-[11px] text-dim mt-1">
              {#if cur}
                {cur.date}
                {#if s.target !== null && s.target !== undefined}
                  · <span class="{(s.direction === 'down' ? cur.value > s.target : cur.value < s.target) ? 'text-warning' : 'text-success'}">{targetHint(s, cur.value)}</span>
                {/if}
              {:else}
                no entries yet
              {/if}
            </p>
          </button>
        {/each}
      </div>

      <!-- Detail panel for the selected series -->
      {#if selectedSeries}
        <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-6">
          <header class="flex items-baseline gap-3 mb-3 flex-wrap">
            <h2 class="text-base font-semibold text-text">{selectedSeries.name}</h2>
            <span class="text-xs text-dim">{detailEntries.length} entries</span>
            <span class="flex-1"></span>
            <button onclick={() => openEdit(selectedSeries!)} class="text-xs text-dim hover:text-text">edit</button>
            <button onclick={() => deleteSeries(selectedSeries!)} class="text-xs text-dim hover:text-error">delete</button>
            <button onclick={() => { selectedId = null; detailEntries = []; }} class="text-xs text-dim hover:text-text">close</button>
          </header>

          <!-- Quick-log form -->
          <form onsubmit={(e) => { e.preventDefault(); submitLog(selectedSeries!.id); }} class="flex flex-wrap gap-2 mb-4">
            <input type="date" bind:value={logDate} class="bg-mantle border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
            <input type="number" step="any" bind:value={logValue} placeholder="value ({selectedSeries.unit})" class="flex-1 min-w-32 bg-mantle border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" required />
            <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">Log {selectedSeries.unit}</button>
          </form>

          <!-- Entry list with deltas. Walks newest → oldest; each row
               shows the delta vs the previous (older) entry, coloured
               by direction-vs-goal. -->
          {#if detailEntries.length === 0}
            <p class="text-sm text-dim italic">No entries yet — log your first above.</p>
          {:else}
            <ul class="divide-y divide-surface1 text-sm">
              {#each detailEntries as e, i (e.id)}
                {@const prev = detailEntries[i + 1]?.value ?? null}
                {@const tr = trendDelta(selectedSeries!, e.value, prev)}
                <li class="py-1.5 flex items-baseline gap-3">
                  <span class="text-xs text-dim font-mono w-24 flex-shrink-0">{e.date}</span>
                  <span class="text-text font-mono">{fmtValue(e.value, selectedSeries!.unit)}</span>
                  {#if tr}
                    <span class="text-xs font-mono {tr.tone}">
                      {tr.delta > 0 ? '+' : tr.delta < 0 ? '' : '±'}{tr.delta.toFixed(Math.abs(tr.delta) >= 10 ? 0 : 1)}
                    </span>
                  {/if}
                  {#if e.notes}
                    <span class="text-[11px] text-subtext flex-1 min-w-0 truncate">{e.notes}</span>
                  {:else}
                    <span class="flex-1"></span>
                  {/if}
                  <button onclick={() => deleteEntry(e)} aria-label="delete" class="text-xs text-dim hover:text-error">×</button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/if}

      {#if archivedSeries.length > 0}
        <h3 class="text-xs uppercase tracking-wider text-dim mt-4 mb-2">Archived</h3>
        <ul class="space-y-1 opacity-60">
          {#each archivedSeries as s (s.id)}
            <li class="text-xs flex items-baseline gap-3 px-3 py-1.5 bg-surface0 border border-surface1 rounded">
              <span class="text-text">{s.name}</span>
              <span class="text-dim">{s.unit}</span>
              <span class="flex-1"></span>
              <button onclick={() => openEdit(s)} class="text-dim hover:text-text">edit</button>
              <button onclick={() => deleteSeries(s)} class="text-dim hover:text-error">delete</button>
            </li>
          {/each}
        </ul>
      {/if}
    {/if}

    <p class="text-[11px] text-dim italic mt-6">
      Synced via <code>.granit/measurements/{'{series,entries}'}.json</code> — same files the granit TUI reads.
    </p>
  </div>
</div>

<!-- ── Series modal ────────────────────────────────────────────────── -->
{#if modalOpen}
  <div class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onclick={() => (modalOpen = false)} role="dialog" tabindex="-1" onkeydown={(e) => { if (e.key === 'Escape') modalOpen = false; }}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} onsubmit={(e) => { e.preventDefault(); submitForm(); }} class="w-full max-w-sm bg-mantle border border-surface1 rounded-lg shadow-xl p-4 space-y-3">
      <h2 class="text-base font-semibold text-text">{editingId ? 'Edit metric' : 'New metric'}</h2>
      <input bind:value={form.name} required placeholder="Name (Weight, Sleep, Push-ups…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <div class="grid grid-cols-2 gap-2">
        <input bind:value={form.unit} required placeholder="Unit (kg, hours, count…)" class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
        <select bind:value={form.direction} class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
          <option value="up">Higher is better</option>
          <option value="down">Lower is better</option>
        </select>
      </div>
      <input type="number" step="any" bind:value={form.target} placeholder="Target (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
      <textarea bind:value={form.notes} rows="2" placeholder="Notes (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text resize-y focus:outline-none focus:border-primary"></textarea>
      <div class="flex justify-end gap-2 pt-2">
        <button type="button" onclick={() => (modalOpen = false)} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
        <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">{editingId ? 'Save' : 'Add'}</button>
      </div>
    </form>
  </div>
{/if}
