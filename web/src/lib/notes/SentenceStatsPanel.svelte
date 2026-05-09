<script lang="ts">
  // SentenceStatsPanel — diagnostic for the rhythm of a note's
  // prose. Surfaces sentence-length distribution, average,
  // longest sentence, and a "rhythm" signal (variance ÷ mean)
  // that flags monotone writing.
  //
  // Why it matters for research / writing:
  //   - Long-only writing reads dense; short-only reads choppy;
  //     varied length is the rhythm of good prose. The panel
  //     exposes the user's actual distribution so they can
  //     audit the music of their own paragraphs.
  //   - Very long sentences (40+ words) are usually a sign that
  //     a clause should split into two. The "longest" row
  //     surfaces the worst offender so the user can act on it.
  //   - Pure client-side derivation. No round-trip; no AI. The
  //     panel updates as the body changes.

  type Props = {
    body: string;
    /** Optional callback so a click on a "long sentence" warning
     *  can scroll the editor to its line. */
    onJumpToLine?: (line: number) => void;
  };
  let { body = '', onJumpToLine }: Props = $props();

  // Sentence detection:
  //   - Strip frontmatter, code blocks, link/image markup, HTML
  //     so we count prose, not infrastructure.
  //   - Split on terminal punctuation (.!?) followed by whitespace
  //     or end-of-string. We don't try to handle every edge case
  //     ("Mr. Smith" splits into two; "etc." too); the alternative
  //     would be a 50KB sentence-tokeniser dependency for what's
  //     a diagnostic panel, not a load-bearing decision system.
  //   - Drop sentences shorter than `MIN_WORDS` (mostly fragments
  //     from list items / headings).
  const MIN_WORDS = 3;

  // Find the line number of a sentence in the body — used by the
  // "long sentence" warning so clicking jumps the editor there.
  type Sentence = { text: string; words: number; line: number };

  function tokenisable(raw: string): string {
    let s = raw;
    s = s.replace(/^---[\s\S]*?\n---\n?/, '');
    s = s.replace(/```[\s\S]*?```/g, ' ');
    s = s.replace(/~~~[\s\S]*?~~~/g, ' ');
    s = s.replace(/`[^`]*`/g, ' ');
    s = s.replace(/!?\[([^\]]*)\]\([^)]*\)/g, ' $1 ');
    s = s.replace(/\[\[([^|\]]*)(?:\|([^\]]*))?\]\]/g, ' $2 $1 ');
    s = s.replace(/<[^>]+>/g, ' ');
    return s;
  }

  // Split prose into sentences with line numbers preserved. We
  // walk the body line-by-line so the editor jump can use the
  // line of the sentence's last terminal punctuation.
  function extractSentences(raw: string): Sentence[] {
    const cleaned = tokenisable(raw);
    const out: Sentence[] = [];
    // Track absolute line numbers — `cleaned` has the same line
    // shape as the source (we replaced inline tokens with spaces,
    // not line breaks), so a position in `cleaned` maps 1:1 to
    // the same position in `raw`.
    const lineStarts: number[] = [0];
    for (let i = 0; i < raw.length; i++) {
      if (raw[i] === '\n') lineStarts.push(i + 1);
    }
    function lineForOffset(off: number): number {
      // Binary search would be faster on a huge note; the linear
      // scan is fine for the typical few-hundred-line body.
      let lineNum = 1;
      for (let i = 0; i < lineStarts.length; i++) {
        if (lineStarts[i] > off) break;
        lineNum = i + 1;
      }
      return lineNum;
    }
    // Iterate by terminal-punct boundary. The lookahead `(?=\s|$)`
    // requires whitespace or end after the punctuation so
    // "section 1.2" doesn't split.
    const re = /[.!?](?=\s|$)/g;
    let last = 0;
    let m: RegExpExecArray | null;
    while ((m = re.exec(cleaned))) {
      const end = m.index + 1;
      const text = cleaned.slice(last, end).trim();
      const wordCount = text.split(/\s+/).filter(Boolean).length;
      if (text && wordCount >= MIN_WORDS) {
        out.push({ text, words: wordCount, line: lineForOffset(last) });
      }
      last = end;
    }
    // Trailing fragment without terminal punctuation — dropped on
    // purpose. A bullet list item or heading without a period
    // doesn't read as a sentence; counting it would distort stats.
    return out;
  }

  let sentences = $derived(extractSentences(body));
  let lengths = $derived(sentences.map((s) => s.words));

  let stats = $derived.by(() => {
    if (lengths.length === 0) return null;
    const sum = lengths.reduce((a, b) => a + b, 0);
    const mean = sum / lengths.length;
    const sorted = [...lengths].sort((a, b) => a - b);
    const median = sorted[Math.floor(sorted.length / 2)];
    const max = sorted[sorted.length - 1];
    // Population stdev — small enough N that the n vs n-1 choice
    // doesn't really matter; we just want the rhythm signal.
    const variance = lengths.reduce((acc, x) => acc + (x - mean) ** 2, 0) / lengths.length;
    const stdev = Math.sqrt(variance);
    // Rhythm score: stdev ÷ mean (coefficient of variation). 0.3+
    // is varied prose; <0.15 is monotone. The label is more useful
    // than the raw number.
    const cv = mean > 0 ? stdev / mean : 0;
    let rhythmLabel = 'varied';
    let rhythmTone = 'success';
    if (cv < 0.15) {
      rhythmLabel = 'monotone';
      rhythmTone = 'warning';
    } else if (cv < 0.25) {
      rhythmLabel = 'flat';
      rhythmTone = 'subtle';
    }
    return { count: lengths.length, mean, median, max, cv, rhythmLabel, rhythmTone };
  });

  // Bucket lengths into 5 bands for the histogram strip. The
  // bands are deliberately literary — under 8 (terse), 8-15
  // (medium), 16-25 (long), 26-40 (heavy), 40+ (run-on).
  let buckets = $derived.by(() => {
    const b = [0, 0, 0, 0, 0];
    for (const len of lengths) {
      if (len < 8) b[0]++;
      else if (len < 16) b[1]++;
      else if (len < 26) b[2]++;
      else if (len < 41) b[3]++;
      else b[4]++;
    }
    return b;
  });
  const BUCKET_LABELS = ['<8', '8–15', '16–25', '26–40', '40+'];
  const BUCKET_HINTS = ['terse', 'medium', 'long', 'heavy', 'run-on'];

  // Surface the longest sentences (40+ words) so the user can
  // act on the worst offenders. Capped at 3 to avoid filling
  // the panel with one note's worth of prose.
  let longSentences = $derived(
    sentences
      .filter((s) => s.words > 40)
      .sort((a, b) => b.words - a.words)
      .slice(0, 3)
  );

  function rhythmClass(tone: string): string {
    return tone === 'warning'
      ? 'text-warning'
      : tone === 'subtle'
        ? 'text-subtext'
        : 'text-success';
  }

  function pct(n: number, total: number): number {
    return total === 0 ? 0 : Math.round((n / total) * 100);
  }
</script>

{#if !stats}
  <p class="text-xs text-dim italic px-2 py-3 text-center">
    Sentence stats appear once the note has a few full sentences. Add some prose and refresh.
  </p>
{:else}
  <!-- Top row: averages + count + rhythm signal -->
  <div class="grid grid-cols-3 gap-2 mb-3 text-xs">
    <div class="bg-surface1/40 rounded px-2 py-1.5">
      <div class="text-dim text-[10px] uppercase tracking-wider">Sentences</div>
      <div class="text-text font-medium tabular-nums">{stats.count}</div>
    </div>
    <div class="bg-surface1/40 rounded px-2 py-1.5">
      <div class="text-dim text-[10px] uppercase tracking-wider">Avg words</div>
      <div class="text-text font-medium tabular-nums">{stats.mean.toFixed(1)}</div>
    </div>
    <div class="bg-surface1/40 rounded px-2 py-1.5">
      <div class="text-dim text-[10px] uppercase tracking-wider">Rhythm</div>
      <div class="font-medium {rhythmClass(stats.rhythmTone)}">{stats.rhythmLabel}</div>
    </div>
  </div>

  <!-- Histogram strip — 5 buckets, percentage bars + counts. The
       BUCKET_HINTS row underneath is the literary read on each
       band so the user remembers what 16-25 actually means. -->
  <div class="space-y-1 mb-3">
    {#each buckets as count, i (i)}
      <div class="flex items-center gap-2 text-[11px]">
        <span class="w-12 text-right tabular-nums text-dim">{BUCKET_LABELS[i]}</span>
        <div class="flex-1 h-2 bg-surface1 rounded-full overflow-hidden">
          <div
            class="h-full bg-primary/40"
            style="width: {pct(count, stats.count)}%"
          ></div>
        </div>
        <span class="w-8 text-right tabular-nums text-text">{count}</span>
        <span class="w-12 text-dim text-[10px]">{BUCKET_HINTS[i]}</span>
      </div>
    {/each}
  </div>

  {#if longSentences.length > 0}
    <!-- "These ran on" — clickable list of >40-word sentences,
         longest first. Click → jump editor to the line so the
         user can split the clause in place. -->
    <div class="mt-4">
      <h4 class="text-[10px] uppercase tracking-wider text-dim mb-1">Run-on candidates</h4>
      <ul class="space-y-1">
        {#each longSentences as s (s.line + s.text.slice(0, 20))}
          <li>
            <button
              type="button"
              onclick={() => onJumpToLine?.(s.line)}
              class="w-full text-left px-2 py-1.5 rounded bg-warning/5 border border-warning/20 hover:border-warning/50 text-[11px]"
              title="Jump to this sentence"
            >
              <div class="flex items-baseline justify-between mb-0.5">
                <span class="text-warning font-medium">{s.words} words</span>
                <span class="text-dim">L{s.line}</span>
              </div>
              <p class="text-text line-clamp-2 leading-snug">{s.text.replace(/\s+/g, ' ')}</p>
            </button>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
{/if}
