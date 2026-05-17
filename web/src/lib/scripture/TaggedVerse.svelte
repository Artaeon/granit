<script lang="ts">
  // TaggedVerse — renders a Strong's-tagged chapter (or a single verse
  // within it) with every word as a clickable token. Clicking a word
  // that carries a Strong's code calls `onSelectStrong(code)`; the
  // parent decides what to show (typically a WordStudy card alongside).
  //
  // The component fetches the whole chapter once on mount because the
  // tagged JSON is already loaded server-side and one chapter is
  // negligible payload — much cheaper than round-tripping per word.
  // Setting `verseNum` narrows the render to a single verse without
  // re-fetching when the parent steps through verses.
  //
  // Parent integration: pass bookCode + chapter (+ optional verseNum
  // for a one-verse mode) and a handler. The component handles its own
  // loading / error / not-bundled states.

  import { api, ApiError, type TaggedVerse, type TaggedWord } from '$lib/api';

  let {
    bookCode,
    chapter,
    verseNum,
    onSelectStrong
  }: {
    bookCode: string;
    chapter: number;
    verseNum?: number;
    onSelectStrong: (code: string) => void;
  } = $props();

  let verses = $state<TaggedVerse[]>([]);
  let loading = $state(false);
  let error = $state<string | null>(null);
  let notBundled = $state(false);

  async function load(book: string, ch: number) {
    if (!book || !ch) return;
    loading = true;
    error = null;
    notBundled = false;
    verses = [];
    try {
      const res = await api.taggedChapter(book, ch);
      verses = res.verses ?? [];
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        // Same trick as WordStudy — disambiguate "not bundled" from
        // "chapter genuinely missing" via /status.
        try {
          const status = await api.strongsStatus();
          if (!status.tagged) {
            notBundled = true;
          } else {
            error = `Tagged ${book} ${ch} not found`;
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

  $effect(() => {
    load(bookCode, chapter);
  });

  // shown is the verse list narrowed to verseNum when set. We use
  // $derived rather than a manual filter so the render stays in sync
  // automatically when either input changes.
  const shown = $derived(
    verseNum != null ? verses.filter((v) => v.n === verseNum) : verses
  );

  // tappable() decides whether a word is interactive. Empty Strong's
  // codes are common upstream — glue words, punctuation — and we render
  // those as plain text so the user can't tap a dead link.
  function tappable(w: TaggedWord): boolean {
    return !!w.strongs && w.strongs.trim() !== '';
  }
</script>

<div class="tagged-verse text-text leading-relaxed">
  {#if loading}
    <div class="text-dim text-xs">Loading tagged text…</div>
  {:else if notBundled}
    <div class="text-xs text-dim space-y-1">
      <div class="text-warning">Tagged bible not bundled.</div>
      <div>Run <code class="text-text">scripts/fetch-strongs.sh</code> and rebuild to enable word study.</div>
    </div>
  {:else if error}
    <div class="text-error text-xs">{error}</div>
  {:else if shown.length === 0}
    <div class="text-dim text-xs italic">No tagged verses to display.</div>
  {:else}
    {#each shown as v (v.n)}
      <div class="verse">
        <sup class="text-dim text-[10px] mr-1 select-none">{v.n}</sup>
        {#each v.words as w, i}
          {#if tappable(w)}
            <button
              type="button"
              class="word tappable"
              onclick={() => onSelectStrong(w.strongs!)}
              title={w.strongs}
            >
              <span class="word-text">{w.text}</span>
              <sup class="strongs-tag">{w.strongs}</sup>
            </button>{i < v.words.length - 1 ? ' ' : ''}
          {:else}
            <span class="word">{w.text}</span>{i < v.words.length - 1 ? ' ' : ''}
          {/if}
        {/each}
      </div>
    {/each}
  {/if}
</div>

<style>
  /* Inline-word styling: words flow as text with a hover affordance.
     The Strong's code only appears on hover/focus so the verse reads
     cleanly at rest. */
  .tagged-verse :global(.verse) {
    display: inline;
  }
  .word {
    display: inline;
  }
  .tappable {
    display: inline;
    background: none;
    border: none;
    padding: 0;
    margin: 0;
    color: inherit;
    font: inherit;
    cursor: pointer;
    position: relative;
  }
  .tappable:hover .word-text,
  .tappable:focus-visible .word-text {
    text-decoration: underline;
    text-decoration-style: dotted;
    text-decoration-thickness: 1px;
    text-underline-offset: 2px;
  }
  .strongs-tag {
    font-size: 9px;
    color: var(--color-dim, #888);
    margin-left: 1px;
    opacity: 0;
    transition: opacity 0.12s;
    font-family: ui-monospace, SFMono-Regular, monospace;
  }
  .tappable:hover .strongs-tag,
  .tappable:focus-visible .strongs-tag {
    opacity: 1;
  }
</style>
