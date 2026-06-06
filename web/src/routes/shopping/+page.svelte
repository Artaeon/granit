<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type ShoppingItem, type ShoppingTotals } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';
  import {
    type Cadence,
    CADENCE_OPTIONS,
    CATEGORY_SUGGESTIONS,
    normalizeCadence,
    cadenceMonthlyFactor,
    fmtMoney,
    lineTotal,
    categoryLabel
  } from '$lib/shopping/shoppingHelpers';

  // /shopping — three-view page over a single Item collection:
  //   Plan: status=planned items, grouped by category (the active
  //         "go shopping" view). This is the default landing.
  //   Standards: items.standard=true, regardless of current status,
  //         so the user can re-plan recurring needs (flip a bought
  //         standard back to planned with one click) or audit their
  //         "what do I always buy" list.
  //   Bought: status=bought, newest-first, with this-month total.
  //         The history view — useful for budgeting + the
  //         occasional "did I already buy that?" check.
  //
  // The "kept simple" philosophy: free-form categories with a
  // canonical default set, optional URLs (one click → open),
  // optional prices (totals roll up only when prices are set),
  // and a single status lifecycle.

  type View = 'plan' | 'standards' | 'bought';
  const VIEW_KEY = 'granit.shopping.view';
  let view = $state<View>(loadStoredString(VIEW_KEY, 'plan') as View);
  $effect(() => saveStoredString(VIEW_KEY, view));

  let items = $state<ShoppingItem[]>([]);
  let totals = $state<ShoppingTotals | null>(null);
  let loading = $state(false);

  // Quick-add form. The full edit surface (description, notes,
  // category override) lives in the inline edit on each row to
  // keep the add-flow as fast as a single line of typing.
  let nName = $state('');
  let nCategory = $state('groceries');
  let nPrice = $state<number | ''>('');
  let nQuantity = $state<number | ''>('');
  let nUrl = $state('');
  let nStandard = $state(false);
  let saving = $state(false);

  // Per-row edit state. When the user clicks "edit" on a row we
  // populate these buffers; saving commits a PATCH, cancel discards.
  let editingId = $state<string | null>(null);
  let eName = $state('');
  let eCategory = $state('');
  let ePrice = $state<number | ''>('');
  let eQuantity = $state<number | ''>('');
  let eUrl = $state('');
  let eStandard = $state(false);
  let eCadence = $state<Cadence>('');
  let eNotes = $state('');

  // ----- Load / sync -----

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const [r, t] = await Promise.all([
        api.listShopping(),
        api.shoppingTotals().catch(() => null)
      ]);
      items = r.items;
      totals = t;
    } catch (e) {
      toast.error('failed to load shopping: ' + (errorMessage(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/shopping.json') load();
    });
    const onVisible = () => {
      if (document.visibilityState === 'visible') load();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
    };
  });

  // ----- Filtering by view -----

  let viewItems = $derived.by(() => {
    if (view === 'plan') return items.filter((i) => i.status === 'planned');
    if (view === 'standards') return items.filter((i) => i.standard);
    return items.filter((i) => i.status === 'bought');
  });

  // Group by category for the Plan view. Items without a category
  // land in an explicit "uncategorised" bucket so the user can spot
  // (and fix) the gap. For Standards view we group the same way.
  // Bought view is flat (chronological).
  let grouped = $derived.by(() => {
    if (view === 'bought') return null;
    const map = new Map<string, ShoppingItem[]>();
    for (const it of viewItems) {
      const k = (it.category ?? '').trim() || '—';
      const arr = map.get(k) ?? [];
      arr.push(it);
      map.set(k, arr);
    }
    // Sort categories: canonical order first (matching server
    // CategorySuggestions), then any custom categories alphabetically,
    // and the "uncategorised" bucket last.
    const known = new Map<string, number>();
    CATEGORY_SUGGESTIONS.forEach((c, i) => known.set(c, i));
    const keys = [...map.keys()].sort((a, b) => {
      if (a === '—') return 1;
      if (b === '—') return -1;
      const ka = known.has(a) ? known.get(a)! : 100 + a.charCodeAt(0);
      const kb = known.has(b) ? known.get(b)! : 100 + b.charCodeAt(0);
      return ka - kb;
    });
    return keys.map((k) => ({ category: k, items: map.get(k)! }));
  });

  // Totals for the active view. Plan view shows planned-sum; bought
  // view shows this-month spend. Standards view shows count only —
  // a price total there is misleading because most standards are
  // ongoing not one-shot purchases.
  let viewTotal = $derived.by(() => {
    if (!totals) return null;
    if (view === 'plan') return totals.planned_sum;
    if (view === 'bought') return totals.bought_month_sum;
    return null;
  });

  // ----- Quick-add -----

  function resetCreate() {
    nName = '';
    nPrice = '';
    nQuantity = '';
    nUrl = '';
    nStandard = false;
    // Keep the category sticky — the user adding 5 groceries shouldn't
    // re-pick "groceries" each time.
  }

  async function addItem(e?: SubmitEvent) {
    e?.preventDefault();
    if (!nName.trim()) return;
    saving = true;
    try {
      const it = await api.createShoppingItem({
        name: nName.trim(),
        category: nCategory.trim() || undefined,
        price: typeof nPrice === 'number' ? nPrice : undefined,
        quantity: typeof nQuantity === 'number' ? nQuantity : undefined,
        url: nUrl.trim() || undefined,
        standard: nStandard,
        status: 'planned'
      });
      // Optimistic prepend; load() reconciles ordering + totals.
      items = [it, ...items];
      resetCreate();
      await load();
    } catch (err) {
      toast.error('add failed: ' + (errorMessage(err)));
    } finally {
      saving = false;
    }
  }

  // ----- Status transitions -----

  async function setStatus(it: ShoppingItem, status: 'planned' | 'bought' | 'skipped') {
    try {
      const updated = await api.patchShoppingItem(it.id, { status });
      items = items.map((x) => (x.id === it.id ? updated : x));
      // Refresh totals — the rollup may have changed.
      totals = await api.shoppingTotals().catch(() => totals);
    } catch (e) {
      toast.error('save failed: ' + (errorMessage(e)));
    }
  }

  async function toggleStandard(it: ShoppingItem) {
    try {
      const updated = await api.patchShoppingItem(it.id, { standard: !it.standard });
      items = items.map((x) => (x.id === it.id ? updated : x));
    } catch (e) {
      toast.error('save failed: ' + (errorMessage(e)));
    }
  }

  async function removeItem(it: ShoppingItem) {
    if (!confirm(`Delete "${it.name}"?`)) return;
    try {
      await api.deleteShoppingItem(it.id);
      items = items.filter((x) => x.id !== it.id);
      totals = await api.shoppingTotals().catch(() => totals);
    } catch (e) {
      toast.error('delete failed: ' + (errorMessage(e)));
    }
  }

  // ----- Inline edit -----

  function startEdit(it: ShoppingItem) {
    editingId = it.id;
    eName = it.name;
    eCategory = it.category ?? '';
    ePrice = it.price ?? '';
    eQuantity = it.quantity ?? '';
    eUrl = it.url ?? '';
    eStandard = !!it.standard;
    // Server may return any string but the picker only honours
    // canonical values; everything else round-trips as "no schedule"
    // via NormalizeCadence on save.
    eCadence = normalizeCadence(it.cadence);
    eNotes = it.notes ?? '';
  }
  function cancelEdit() {
    editingId = null;
  }
  async function commitEdit() {
    if (!editingId) return;
    try {
      const updated = await api.patchShoppingItem(editingId, {
        name: eName.trim(),
        category: eCategory.trim() || undefined,
        price: typeof ePrice === 'number' ? ePrice : undefined,
        quantity: typeof eQuantity === 'number' ? eQuantity : undefined,
        url: eUrl.trim() || undefined,
        standard: eStandard,
        // Cadence only meaningful when standard=true; we still send
        // it on non-standard items so a later "mark standard" picks
        // up an existing user intent without re-asking.
        cadence: eCadence,
        notes: eNotes.trim() || undefined
      });
      items = items.map((x) => (x.id === updated.id ? updated : x));
      editingId = null;
      totals = await api.shoppingTotals().catch(() => totals);
    } catch (e) {
      toast.error('save failed: ' + (errorMessage(e)));
    }
  }

</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-3xl mx-auto">
    <header class="mb-5">
      <h1 class="text-2xl sm:text-3xl font-semibold text-text">Shopping</h1>
      <p class="text-sm text-dim mt-1">
        Simple plan-to-buy with categories, links, optional prices.
        Standards are recurring needs you re-plan each cycle.
      </p>
    </header>

    <!-- Totals strip — rolls up alongside /finance overview. Three
         cards on phones (2 mobile rows), four on tablet+ when there's
         a recurring projection to surface. -->
    {#if totals}
      {@const showRecurring = (totals.recurring_standards_count ?? 0) > 0}
      <div class="grid grid-cols-2 {showRecurring ? 'lg:grid-cols-4 sm:grid-cols-3' : 'sm:grid-cols-3'} gap-2 mb-5">
        <div class="px-3 py-2 bg-surface0 border border-surface1 rounded">
          <div class="text-2xl font-semibold text-text tabular-nums leading-none">{totals.planned_count}</div>
          <div class="text-[11px] text-dim mt-1 truncate">Planned · {fmtMoney(totals.planned_sum)}</div>
        </div>
        <div class="px-3 py-2 bg-surface0 border border-surface1 rounded">
          <div class="text-2xl font-semibold text-text tabular-nums leading-none">{totals.bought_month_count}</div>
          <div class="text-[11px] text-dim mt-1 truncate">Bought this month · {fmtMoney(totals.bought_month_sum)}</div>
        </div>
        {#if showRecurring}
          <a
            href="/finance"
            class="px-3 py-2 bg-surface0 border border-surface1 rounded hover:border-primary block group"
            title="recurring standards projected per month — read in /finance overview"
          >
            <div class="text-2xl font-semibold tabular-nums leading-none text-secondary group-hover:text-primary">
              {fmtMoney(totals.recurring_monthly_estimate)}
            </div>
            <div class="text-[11px] text-dim mt-1 truncate">~ recurring/month · {totals.recurring_standards_count} standards</div>
          </a>
        {/if}
        <a
          href="/finance"
          class="hidden sm:flex px-3 py-2 bg-surface0 border border-surface1 rounded text-xs text-secondary hover:border-primary items-center justify-center"
        >view in /finance →</a>
      </div>
    {/if}

    <!-- Quick add form. Mobile-first grid layout: name takes its own
         row (so a long item title isn't crammed against numeric
         inputs), then category | price | qty share a 3-col row, then
         url (full width) and the standard toggle + add button on the
         action row. On sm+ the layout collapses to two rows: name +
         category + price + qty + add on row 1; url + standard on row 2.
         Avoids flex-wrap reflowing into a tower of half-empty rows on
         narrow phones. -->
    <form onsubmit={addItem} class="bg-surface0 border border-surface1 rounded-lg p-3 sm:p-4 mb-5 space-y-2.5">
      <div class="grid grid-cols-3 gap-2 sm:flex sm:flex-wrap sm:items-stretch">
        <input
          bind:value={nName}
          required
          placeholder="add to plan…"
          class="col-span-3 sm:flex-1 sm:min-w-[14rem] px-3 py-2 bg-mantle border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        />
        <select
          bind:value={nCategory}
          class="col-span-3 sm:col-auto sm:w-auto px-2 py-2 bg-mantle border border-surface1 rounded text-sm text-text"
          aria-label="category"
        >
          {#each CATEGORY_SUGGESTIONS as c}<option value={c}>{c}</option>{/each}
        </select>
        <input
          type="number"
          bind:value={nPrice}
          step="0.01"
          min="0"
          placeholder="€"
          class="sm:w-20 px-2 py-2 bg-mantle border border-surface1 rounded text-sm text-text tabular-nums"
          aria-label="price"
        />
        <input
          type="number"
          bind:value={nQuantity}
          step="1"
          min="1"
          placeholder="qty"
          class="sm:w-16 px-2 py-2 bg-mantle border border-surface1 rounded text-sm text-text tabular-nums"
          aria-label="quantity"
        />
        <button
          type="submit"
          disabled={!nName.trim() || saving}
          class="px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
        >{saving ? '…' : '+ add'}</button>
      </div>
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <input
          bind:value={nUrl}
          type="url"
          placeholder="product link (optional)"
          class="flex-1 min-w-[10rem] px-2 py-1.5 bg-mantle border border-surface1 rounded text-xs text-text font-mono placeholder-dim focus:outline-none focus:border-primary"
        />
        <label class="flex items-center gap-1.5 text-dim cursor-pointer flex-shrink-0">
          <input type="checkbox" bind:checked={nStandard} class="accent-primary" />
          standard (recurring need)
        </label>
      </div>
    </form>

    <!-- View tabs -->
    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm mb-4">
      {#each [
        { v: 'plan', label: 'Plan', count: items.filter((i) => i.status === 'planned').length },
        { v: 'standards', label: 'Standards', count: items.filter((i) => i.standard).length },
        { v: 'bought', label: 'Bought', count: items.filter((i) => i.status === 'bought').length }
      ] as o}
        <button
          type="button"
          class="flex-1 px-3 py-2 capitalize transition-colors {view === o.v ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (view = o.v as View)}
        >
          {o.label}
          <span class="ml-1 text-xs opacity-70 tabular-nums">{o.count}</span>
        </button>
      {/each}
    </div>

    {#if viewTotal !== null && viewTotal > 0}
      <p class="text-xs text-dim mb-3">
        {view === 'plan' ? 'Planned total' : 'This month total'}:
        <span class="text-text font-medium">{fmtMoney(viewTotal)}</span>
      </p>
    {/if}

    {#if loading && items.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if viewItems.length === 0}
      {#if view === 'plan'}
        <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
          <p class="text-sm text-text mb-1">Nothing on the plan yet.</p>
          <p class="text-xs text-dim">
            Add items above. Mark frequently-bought ones as <em>standard</em> so you can re-plan them quickly each cycle.
          </p>
        </div>
      {:else if view === 'standards'}
        <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
          <p class="text-sm text-text mb-1">No standards yet.</p>
          <p class="text-xs text-dim">
            Mark items as standard to keep a clean catalogue of your recurring needs (groceries, basics, things you always restock).
          </p>
        </div>
      {:else}
        <div class="text-sm text-dim italic">No bought items yet.</div>
      {/if}
    {:else if view === 'bought'}
      <!-- Flat chronological list for the bought history -->
      <ul class="space-y-1">
        {#each viewItems as it (it.id)}
          {@const total = lineTotal(it)}
          <li class="bg-surface0 border border-surface1 rounded p-3">
            <div class="flex items-baseline gap-2 flex-wrap">
              <span class="text-sm text-text flex-1 min-w-0 break-words">{it.name}</span>
              {#if it.url}
                <a
                  href={it.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-[11px] text-secondary hover:underline truncate font-mono max-w-[10rem]"
                >↗ link</a>
              {/if}
              {#if total > 0}
                <span class="text-xs text-dim tabular-nums">{fmtMoney(total)}</span>
              {/if}
              <span class="text-[11px] text-dim font-mono">{it.bought_at ?? '—'}</span>
              <button
                type="button"
                onclick={() => setStatus(it, 'planned')}
                title="re-plan (move back to plan)"
                class="text-[11px] text-secondary hover:underline"
              >re-plan</button>
            </div>
            {#if it.category}
              <div class="text-[11px] text-dim mt-1">{it.category}{#if it.standard} · standard{/if}</div>
            {/if}
          </li>
        {/each}
      </ul>
    {:else if grouped}
      <!-- Grouped (Plan / Standards) -->
      <div class="space-y-5">
        {#each grouped as g (g.category)}
          <section>
            <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-2 pb-1 border-b border-surface1">
              {categoryLabel(g.category)}
              <span class="ml-1 text-dim/70">· {g.items.length}</span>
            </h2>
            <ul class="space-y-1.5">
              {#each g.items as it (it.id)}
                {@const total = lineTotal(it)}
                {@const editing = editingId === it.id}
                <li class="bg-surface0 border border-surface1 rounded p-3">
                  {#if editing}
                    <!-- Edit form: same mobile-first grid as quick-add.
                         Name takes its own row; category/price/qty share
                         a 3-col row on mobile and inline on sm+. URL +
                         notes are full-width below regardless of viewport. -->
                    <div class="space-y-2">
                      <div class="grid grid-cols-3 gap-2 sm:flex sm:flex-wrap sm:items-stretch">
                        <input
                          bind:value={eName}
                          required
                          placeholder="name"
                          class="col-span-3 sm:flex-1 sm:min-w-[12rem] px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text"
                        />
                        <input
                          bind:value={eCategory}
                          list="cat-suggestions"
                          placeholder="category"
                          class="col-span-3 sm:col-auto sm:w-32 px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text"
                        />
                        <datalist id="cat-suggestions">
                          {#each CATEGORY_SUGGESTIONS as c}<option value={c}></option>{/each}
                        </datalist>
                        <input
                          type="number"
                          bind:value={ePrice}
                          step="0.01"
                          min="0"
                          placeholder="€"
                          class="sm:w-20 px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text tabular-nums"
                        />
                        <input
                          type="number"
                          bind:value={eQuantity}
                          step="1"
                          min="1"
                          placeholder="qty"
                          class="sm:w-16 px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text tabular-nums"
                        />
                      </div>
                      <input
                        bind:value={eUrl}
                        type="url"
                        placeholder="product link (optional)"
                        class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-xs text-text font-mono"
                      />
                      <textarea
                        bind:value={eNotes}
                        rows="2"
                        placeholder="notes (optional)"
                        class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-xs text-text"
                      ></textarea>
                      <!-- Cadence picker — only meaningful for standards
                           but we let the user set it on any item; if
                           they later mark standard, the cadence is
                           already in place. Hidden when neither
                           standard nor an existing cadence is set so
                           the form stays compact for simple items. -->
                      {#if eStandard || eCadence}
                        <div class="flex items-center gap-2 flex-wrap">
                          <label class="text-xs text-dim flex items-center gap-1.5 flex-shrink-0">
                            recurs
                            <select
                              bind:value={eCadence}
                              class="px-2 py-1 bg-mantle border border-surface1 rounded text-xs text-text"
                            >
                              {#each CADENCE_OPTIONS as o}<option value={o.value}>{o.label}</option>{/each}
                            </select>
                          </label>
                          {#if eStandard && eCadence && typeof ePrice === 'number' && ePrice > 0}
                            {@const qty = typeof eQuantity === 'number' && eQuantity > 0 ? eQuantity : 1}
                            {@const monthly = ePrice * qty * cadenceMonthlyFactor(eCadence)}
                            <span class="text-[11px] text-secondary">≈ {fmtMoney(monthly)}/month</span>
                          {/if}
                        </div>
                      {/if}
                      <div class="flex items-center gap-3 flex-wrap">
                        <label class="flex items-center gap-1.5 text-xs text-dim cursor-pointer">
                          <input type="checkbox" bind:checked={eStandard} class="accent-primary" />
                          standard
                        </label>
                        <span class="flex-1"></span>
                        <button
                          type="button"
                          onclick={cancelEdit}
                          class="px-3 py-1.5 text-sm text-subtext hover:text-text"
                        >cancel</button>
                        <button
                          type="button"
                          onclick={commitEdit}
                          class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium"
                        >save</button>
                      </div>
                    </div>
                  {:else}
                    <!-- Two-row layout: top row = checkbox + name + price/url
                         (the high-frequency information); bottom row = chips +
                         action icons. Always-visible action icons replace the
                         hover-only ⋯ menu so touch users don't have to hunt. -->
                    <div class="flex items-start gap-2.5">
                      <button
                        type="button"
                        onclick={() => setStatus(it, it.status === 'bought' ? 'planned' : 'bought')}
                        title={it.status === 'bought' ? 'mark not bought' : 'mark bought'}
                        aria-label="toggle bought"
                        class="w-6 h-6 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center transition-colors
                          {it.status === 'bought' ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
                      >
                        {#if it.status === 'bought'}
                          <svg viewBox="0 0 12 12" class="w-3.5 h-3.5 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                        {/if}
                      </button>
                      <div class="flex-1 min-w-0">
                        <div class="flex items-baseline gap-2 flex-wrap">
                          <span class="text-sm text-text break-words {it.status === 'bought' ? 'line-through text-dim' : ''}">
                            {it.name}
                            {#if it.quantity && it.quantity > 1}<span class="text-dim text-xs"> ×{it.quantity}</span>{/if}
                          </span>
                          {#if total > 0}
                            <span class="text-xs text-dim tabular-nums flex-shrink-0">{fmtMoney(total)}</span>
                          {/if}
                        </div>
                        {#if it.notes}
                          <p class="text-[11px] text-dim mt-0.5 break-words">{it.notes}</p>
                        {/if}
                        <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 mt-1">
                          {#if it.standard}
                            <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-secondary">
                              standard{#if it.cadence} · {it.cadence}{/if}
                            </span>
                          {/if}
                          {#if it.url}
                            <a
                              href={it.url}
                              target="_blank"
                              rel="noopener noreferrer"
                              class="text-[11px] text-secondary hover:underline truncate font-mono max-w-full sm:max-w-[14rem]"
                              title={it.url}
                            >↗ {it.url.replace(/^https?:\/\//, '').replace(/\/$/, '')}</a>
                          {/if}
                        </div>
                      </div>
                      <!-- Action buttons: always-visible icon row aligned
                           to the right edge. 8x8 hit boxes meet the 32×32
                           touch-target floor for a comfortable phone tap. -->
                      <div class="flex items-center gap-0.5 flex-shrink-0 -mr-1">
                        <button
                          type="button"
                          onclick={() => toggleStandard(it)}
                          aria-label={it.standard ? 'unmark standard' : 'mark standard'}
                          title={it.standard ? 'unmark standard' : 'mark as recurring need'}
                          class="w-8 h-8 flex items-center justify-center rounded text-base transition-colors
                            {it.standard ? 'text-secondary hover:bg-surface1' : 'text-dim hover:text-secondary hover:bg-surface1'}"
                        >{it.standard ? '★' : '☆'}</button>
                        <button
                          type="button"
                          onclick={() => startEdit(it)}
                          aria-label="edit"
                          title="edit"
                          class="w-8 h-8 flex items-center justify-center rounded text-dim hover:text-text hover:bg-surface1"
                        >
                          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
                            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
                          </svg>
                        </button>
                        <button
                          type="button"
                          onclick={() => setStatus(it, 'skipped')}
                          aria-label="skip"
                          title="skip — won't buy this cycle"
                          class="w-8 h-8 flex items-center justify-center rounded text-dim hover:text-warning hover:bg-surface1"
                        >
                          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                            <circle cx="12" cy="12" r="10"/><path d="m4.93 4.93 14.14 14.14"/>
                          </svg>
                        </button>
                        <button
                          type="button"
                          onclick={() => removeItem(it)}
                          aria-label="delete"
                          title="delete"
                          class="w-8 h-8 flex items-center justify-center rounded text-dim hover:text-error hover:bg-surface1"
                        >
                          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                            <path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
                          </svg>
                        </button>
                      </div>
                    </div>
                  {/if}
                </li>
              {/each}
            </ul>
          </section>
        {/each}
      </div>
    {/if}

    <footer class="mt-10 pt-4 border-t border-surface1 text-[11px] text-dim">
      Synced via <code class="font-mono">.granit/shopping.json</code> — single record per item, standards re-plan in place.
    </footer>
  </div>
</div>
