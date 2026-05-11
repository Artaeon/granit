<script lang="ts">
  // ConcordancePanel — top-N most-frequent meaningful words in the
  // current note, rendered as a clickable list with frequency
  // counts. The classic concordance affordance: a research /
  // editing tool that surfaces the words a writer keeps returning
  // to without them realising it.
  //
  // Use cases:
  //   - "Am I overusing 'really'?" — surfaces hedge words you
  //     reach for unconsciously
  //   - "What terms do I keep returning to?" — fingerprint a
  //     research note's actual vocabulary
  //   - Click a row → editor jumps to the first occurrence so the
  //     user can audit the usage in context
  //
  // Pure client-side: tokenises the note body in the browser, no
  // round-trip to the server. Cheap enough — the typical note is
  // a few thousand words and the regex split + counter pass takes
  // well under a millisecond.

  type Props = {
    body: string;
    /** Called when the user clicks a row. The parent (notes page)
     *  uses the editor's find-or-jump affordance to surface the
     *  occurrence in context. */
    onJumpToWord?: (word: string) => void;
  };
  let { body = '', onJumpToWord }: Props = $props();

  // English stopwords — kept short on purpose. We want to filter
  // the obvious connective tissue ("the", "and") so the user sees
  // their actual content vocabulary, but not so aggressively that
  // we hide a word the user is genuinely overusing. Words shorter
  // than `MIN_LEN` are also filtered (catches "is", "of", "to",
  // ...) so this list only has to cover the longer-than-3 stopwords.
  const STOPWORDS = new Set([
    'about', 'after', 'again', 'against', 'also', 'because',
    'been', 'being', 'between', 'both', 'before', 'could',
    'does', 'doing', 'down', 'during', 'each', 'every',
    'from', 'further', 'have', 'having', 'here', 'into',
    'just', 'like', 'more', 'most', 'much', 'only', 'other',
    'over', 'same', 'should', 'some', 'such', 'than',
    'that', 'their', 'them', 'then', 'there', 'these', 'they',
    'this', 'those', 'through', 'under', 'until', 'very',
    'were', 'what', 'when', 'where', 'which', 'while', 'with',
    'would', 'your', 'yours', 'really', 'into', 'whose',
    'still', 'even', 'whether', 'within', 'without', 'across',
    'whilst', 'thus', 'hence', 'though', 'simply',
    // Common code/markdown bleed when stripping isn't perfect.
    'http', 'https', 'com', 'www', 'true', 'false', 'null'
  ]);
  const MIN_LEN = 4;
  const TOP_N = 30;

  // Strip code blocks, inline code, frontmatter, and link markup
  // so the count reflects prose, not infrastructure. The user
  // really doesn't want to learn that "function" appears 42 times
  // because they pasted a code listing.
  function tokenisable(raw: string): string {
    let s = raw;
    // YAML frontmatter — only at the very top of the file.
    s = s.replace(/^---[\s\S]*?\n---\n?/, '');
    // Fenced code blocks (``` …  ``` or ~~~ … ~~~).
    s = s.replace(/```[\s\S]*?```/g, ' ');
    s = s.replace(/~~~[\s\S]*?~~~/g, ' ');
    // Inline code `…`.
    s = s.replace(/`[^`]*`/g, ' ');
    // Link / image markdown — keep the visible text, drop the URL.
    s = s.replace(/!?\[([^\]]*)\]\([^)]*\)/g, ' $1 ');
    // Wikilinks [[Path|Display]] or [[Path]].
    s = s.replace(/\[\[([^|\]]*)(?:\|([^\]]*))?\]\]/g, ' $2 $1 ');
    // HTML tags.
    s = s.replace(/<[^>]+>/g, ' ');
    return s;
  }

  // Tokenise on non-letter/digit boundaries. Keep apostrophes
  // inside words ("don't", "user's") so they don't fragment.
  function tokenise(s: string): string[] {
    return s
      .toLowerCase()
      .split(/[^a-z0-9'’-]+/)
      .filter(Boolean);
  }

  let entries = $derived.by((): { word: string; count: number }[] => {
    if (!body) return [];
    const tokens = tokenise(tokenisable(body));
    const counts = new Map<string, number>();
    for (const t of tokens) {
      const w = t.replace(/^['’-]+|['’-]+$/g, ''); // trim outer punctuation
      if (w.length < MIN_LEN) continue;
      if (STOPWORDS.has(w)) continue;
      if (/^\d+$/.test(w)) continue; // pure numbers are noise
      counts.set(w, (counts.get(w) ?? 0) + 1);
    }
    const arr = Array.from(counts.entries()).map(([word, count]) => ({ word, count }));
    arr.sort((a, b) => (b.count - a.count) || a.word.localeCompare(b.word));
    return arr.slice(0, TOP_N);
  });

  // Total non-stopword count powers the percentage column. Cheap —
  // re-derived from the same pass via the entries length, but we
  // recompute on body change, which is fine.
  let totalWords = $derived.by((): number => {
    if (!body) return 0;
    return tokenise(tokenisable(body)).filter((w) => w.length >= MIN_LEN && !STOPWORDS.has(w)).length;
  });

  // Bar-width relative to the top entry — gives a visual cue of
  // ratio without the user having to compare numbers.
  let topCount = $derived(entries[0]?.count ?? 1);

  function pct(n: number): string {
    if (totalWords === 0) return '0%';
    return `${((n / totalWords) * 100).toFixed(1)}%`;
  }
</script>

{#if entries.length === 0}
  <p class="text-xs text-dim italic px-2 py-3 text-center">
    Concordance appears once the note has more than a handful of
    content words. Add some prose and refresh.
  </p>
{:else}
  <div class="text-[11px] text-dim mb-2 px-1">
    {entries.length} most-frequent terms · {totalWords} content words total
  </div>
  <ul class="space-y-0.5">
    {#each entries as e (e.word)}
      <li>
        <button
          type="button"
          onclick={() => onJumpToWord?.(e.word)}
          class="w-full text-left px-2 py-1 rounded hover:bg-surface1 flex items-center gap-2 group"
          title="Jump to first occurrence in editor"
        >
          <span class="flex-1 truncate text-sm text-text">{e.word}</span>
          <span class="text-[11px] text-dim tabular-nums w-12 text-right">
            {pct(e.count)}
          </span>
          <span class="text-xs text-subtext tabular-nums w-8 text-right">
            ×{e.count}
          </span>
          <!-- Inline bar — width relative to topCount so the leading
               row spans the bar fully, every other row scales down. -->
          <span
            class="block h-1 bg-primary rounded-full flex-shrink-0"
            style="width: {Math.max(8, (e.count / topCount) * 56)}px"
          ></span>
        </button>
      </li>
    {/each}
  </ul>
{/if}
