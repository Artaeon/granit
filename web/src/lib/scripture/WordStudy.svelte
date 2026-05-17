<script lang="ts">
  // WordStudy — dense lexicon card for a single Strong's code.
  //
  // Fetches the lexicon entry lazily on mount (and whenever the parent
  // swaps `strongsCode` for a new lookup) and renders code + lemma +
  // transliteration + definition + KJV gloss + derivation in a tight
  // power-UI layout. Loading + error + missing-data states are all
  // handled inline so the card is self-contained.
  //
  // Parent integration: drop this anywhere — sidebar, popover, sheet —
  // and pass `strongsCode`. Optional `onClose` renders a close button
  // when provided; omit it for inline embeds.

  import { api, ApiError, type StrongsEntry } from '$lib/api';

  let {
    strongsCode,
    onClose
  }: {
    strongsCode: string;
    onClose?: () => void;
  } = $props();

  let entry = $state<StrongsEntry | null>(null);
  let loading = $state(false);
  let error = $state<string | null>(null);
  // notBundled is the "lexicon JSON wasn't shipped" case — distinct from
  // "lexicon shipped but this code is missing". We tell them apart via
  // the /status endpoint so the hint text can be helpful instead of generic.
  let notBundled = $state(false);

  async function load(code: string) {
    if (!code) return;
    loading = true;
    error = null;
    notBundled = false;
    entry = null;
    try {
      entry = await api.strongsEntry(code);
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        // 404 could mean either "code missing" or "lexicon not bundled".
        // Probe /status to disambiguate so we can show the right hint.
        try {
          const status = await api.strongsStatus();
          if (!status.lexicon) {
            notBundled = true;
          } else {
            error = `No lexicon entry for ${code}`;
          }
        } catch {
          error = e.message;
        }
      } else {
        error = e instanceof Error ? e.message : String(e);
      }
    } finally {
      loading = false;
    }
  }

  // Re-fetch whenever the parent passes a new code. $effect runs on
  // every change to strongsCode so the card stays in sync without the
  // parent having to re-mount it.
  $effect(() => {
    load(strongsCode);
  });

  // language() picks Greek vs Hebrew styling from the code prefix.
  function language(code: string): 'greek' | 'hebrew' | 'unknown' {
    const c = code.trim().toUpperCase();
    if (c.startsWith('G')) return 'greek';
    if (c.startsWith('H')) return 'hebrew';
    return 'unknown';
  }
</script>

<div class="bg-mantle border border-surface1 rounded p-3 text-sm space-y-2">
  <div class="flex items-baseline justify-between gap-2">
    <div class="flex items-baseline gap-2 min-w-0">
      <span
        class="text-[10px] uppercase tracking-wider font-mono px-1.5 py-0.5 rounded border"
        class:border-blue-400={language(strongsCode) === 'greek'}
        class:text-blue-300={language(strongsCode) === 'greek'}
        class:border-amber-400={language(strongsCode) === 'hebrew'}
        class:text-amber-300={language(strongsCode) === 'hebrew'}
        class:border-surface2={language(strongsCode) === 'unknown'}
        class:text-dim={language(strongsCode) === 'unknown'}
      >{strongsCode}</span>
      {#if entry?.lemma}
        <span class="text-text text-base leading-none">{entry.lemma}</span>
      {/if}
      {#if entry?.translit}
        <span class="text-dim italic text-xs">{entry.translit}</span>
      {/if}
    </div>
    {#if onClose}
      <button
        type="button"
        onclick={onClose}
        class="text-dim hover:text-text text-xs px-1.5 py-0.5 rounded hover:bg-surface0"
        aria-label="Close word study"
      >Close</button>
    {/if}
  </div>

  {#if loading}
    <div class="text-dim text-xs">Loading…</div>
  {:else if notBundled}
    <div class="text-xs text-dim space-y-1">
      <div class="text-amber-300">Strong's lexicon not bundled.</div>
      <div>Run <code class="text-text">scripts/fetch-strongs.sh</code> and rebuild to enable word study.</div>
    </div>
  {:else if error}
    <div class="text-red-300 text-xs">{error}</div>
  {:else if entry}
    {#if entry.strongs_def}
      <div>
        <div class="text-[10px] uppercase tracking-wider text-dim">Definition</div>
        <div class="text-text leading-snug">{entry.strongs_def}</div>
      </div>
    {/if}
    {#if entry.kjv_def}
      <div>
        <div class="text-[10px] uppercase tracking-wider text-dim">KJV gloss</div>
        <div class="text-text leading-snug">{entry.kjv_def}</div>
      </div>
    {/if}
    {#if entry.derivation}
      <div>
        <div class="text-[10px] uppercase tracking-wider text-dim">Derivation</div>
        <div class="text-dim leading-snug text-xs">{entry.derivation}</div>
      </div>
    {/if}
    {#if !entry.strongs_def && !entry.kjv_def && !entry.derivation}
      <div class="text-dim text-xs italic">Lexicon entry has no definition text.</div>
    {/if}
  {/if}
</div>
