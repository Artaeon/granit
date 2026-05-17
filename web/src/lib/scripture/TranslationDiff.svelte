<script lang="ts">
  // TranslationDiff — side-by-side passage view across multiple bible
  // translations. On mount it pings /bible/translations to learn what's
  // bundled; the user toggles individual translations in/out via a chip
  // strip and the passage refetches for the resulting selection.
  //
  // Bundled in granit's repo by default: WEB only. ASV / KJV / BBE
  // appear iff someone has run scripts/fetch-bible-translations.sh and
  // rebuilt the server. The empty-state below points users at that
  // script when only one translation is available.
  //
  // Parent integration: pass bookCode + chapter (+ optional verse range
  // for a narrower slice) and an optional defaultTranslations array to
  // pre-seed the chip selection. The component owns its own loading /
  // error / refetch — drop it into a column and forget about it.

  import { onMount } from 'svelte';
  import { api, type TranslationInfo, type PassageCompareTranslation } from '$lib/api';

  let {
    bookCode,
    chapter,
    verseFrom,
    verseTo,
    defaultTranslations
  }: {
    bookCode: string;
    chapter: number;
    verseFrom?: number;
    verseTo?: number;
    defaultTranslations?: string[];
  } = $props();

  let available = $state<TranslationInfo[]>([]);
  let selected = $state<Set<string>>(new Set());
  let columns = $state<PassageCompareTranslation[]>([]);
  let loadingList = $state(false);
  let loadingPassage = $state(false);
  let error = $state<string | null>(null);

  // selectedKey is a stable cache-buster for the $effect below — Sets
  // aren't deeply tracked by Svelte 5's reactivity, so we derive a
  // joined string from the entries and react to that instead. Sorted
  // so toggle order doesn't trigger duplicate refetches.
  const selectedKey = $derived(Array.from(selected).sort().join(','));

  // Whether the chip selection has at least one translation in it.
  // When the set is empty we skip the passage fetch — server-side an
  // empty list would default to "every translation" which surprises
  // the user (their chips show "nothing selected" → they expect "no
  // columns").
  const hasSelection = $derived(selected.size > 0);

  async function loadTranslations() {
    loadingList = true;
    error = null;
    try {
      const res = await api.bibleTranslations();
      available = res.translations ?? [];
      // Seed selection. defaultTranslations wins when provided AND at
      // least one of its ids is actually bundled (so a stale prop
      // pointing at a missing translation falls through to "all
      // available" rather than rendering empty).
      const init = new Set<string>();
      if (defaultTranslations && defaultTranslations.length) {
        for (const id of defaultTranslations) {
          if (available.some((t) => t.id === id)) init.add(id);
        }
      }
      if (init.size === 0) {
        for (const t of available) init.add(t.id);
      }
      selected = init;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loadingList = false;
    }
  }

  async function loadPassage() {
    if (!bookCode || !chapter || !hasSelection) {
      columns = [];
      return;
    }
    loadingPassage = true;
    error = null;
    try {
      const res = await api.biblePassageCompare({
        book: bookCode,
        chapter,
        verseFrom,
        verseTo,
        translations: Array.from(selected)
      });
      columns = res.translations ?? [];
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loadingPassage = false;
    }
  }

  onMount(() => {
    void loadTranslations();
  });

  // Refetch the passage whenever the selection or the passage
  // coordinates change. Using $effect (not $derived.by) because the
  // body has async I/O — derivations are supposed to be pure.
  $effect(() => {
    // Touch every reactive input so Svelte tracks them.
    void selectedKey;
    void bookCode;
    void chapter;
    void verseFrom;
    void verseTo;
    void loadPassage();
  });

  function toggle(id: string) {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selected = next;
  }

  // When only WEB is bundled the user probably hasn't run the fetch
  // script yet. We don't hide the component — it still shows WEB — but
  // we surface a small footer with the script path so the upgrade path
  // is discoverable.
  const onlyWeb = $derived(available.length <= 1);
</script>

<div class="translation-diff text-text">
  {#if loadingList}
    <div class="text-dim text-xs">Loading translations…</div>
  {:else if error}
    <div class="text-red-300 text-xs">{error}</div>
  {:else if available.length === 0}
    <div class="text-dim text-xs italic">No translations bundled.</div>
  {:else}
    <!-- Translation chip strip — dense, mono-spaced abbreviations so a
         row of 4 fits in a phone-narrow column. -->
    <div class="flex flex-wrap gap-1 mb-2">
      {#each available as t (t.id)}
        {@const on = selected.has(t.id)}
        <button
          type="button"
          class="px-2 py-0.5 text-[11px] font-mono rounded border transition-colors {on
            ? 'bg-accent/20 border-accent text-text'
            : 'bg-transparent border-dim/40 text-dim hover:text-text hover:border-dim'}"
          title={`${t.name}${t.year ? ` · ${t.year}` : ''}`}
          onclick={() => toggle(t.id)}
        >
          {t.abbreviation}
        </button>
      {/each}
    </div>

    {#if !hasSelection}
      <div class="text-dim text-xs italic">Pick at least one translation.</div>
    {:else if loadingPassage}
      <div class="text-dim text-xs">Loading passage…</div>
    {:else if columns.length === 0}
      <div class="text-dim text-xs italic">No data for this passage.</div>
    {:else}
      <!-- Column grid. lg+ lays N translations across; mobile stacks.
           Using min-w-0 on the columns lets long verse text wrap rather
           than triggering horizontal overflow. -->
      <div
        class="grid gap-3"
        style:grid-template-columns="repeat(auto-fit, minmax(14rem, 1fr))"
      >
        {#each columns as col (col.id)}
          <div class="min-w-0 border-l-2 border-dim/30 pl-2">
            <div class="flex items-baseline justify-between gap-2 mb-1">
              <span class="text-[10px] font-mono uppercase tracking-wide text-accent">{col.abbreviation}</span>
              <span class="text-[10px] text-dim truncate" title={col.name}>{col.name}</span>
            </div>
            {#if col.reference}
              <div class="text-[10px] text-dim font-mono mb-1">{col.reference}</div>
            {/if}
            {#if col.verses.length === 0}
              <div class="text-dim text-[11px] italic">—</div>
            {:else}
              <div class="text-[13px] leading-snug space-y-0.5">
                {#each col.verses as v (v.n)}
                  <p class="m-0">
                    <sup class="text-dim text-[10px] font-mono mr-1 select-none">{v.n}</sup
                    >{v.text}
                  </p>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}

    {#if onlyWeb}
      <div class="mt-2 text-[10px] text-dim">
        Only WEB bundled. Run
        <code class="text-text">scripts/fetch-bible-translations.sh</code>
        and rebuild to compare more.
      </div>
    {/if}
  {/if}
</div>
