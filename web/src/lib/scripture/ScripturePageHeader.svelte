<script lang="ts">
  // Slim page header for /scripture. Same shape as TasksPageHeader /
  // GoalsPageHeader / NotesPageHeader — single ~44px row carrying
  // title + catalogue count + a segmented mode picker + a small set
  // of actions. The previous chrome was a multi-line PageHeader strip
  // followed by a 6-tab pill row that wrapped awkwardly on mobile.
  //
  // Contemplative carve-out: scripture is a contemplative practice.
  // The header itself is intentionally calm; no XP / streak / score
  // gamification rendered here. The StreakBadge that survives is
  // descriptive (consecutive days you've opened the bible) — kept on
  // the page surface, not promoted into the chrome.

  type Mode = 'read' | 'memo' | 'browse' | 'bible' | 'bookmarks' | 'intentions';

  type Props = {
    mode: Mode;
    catalogueCount: number;       // verses in the curated set (Browse badge)
    prayingCount: number;          // currently-praying intentions (Prayer badge)
    onSelectMode: (m: Mode) => void;
    onQuickCapture: () => void;    // promote whatever's visible into Read mode
  };

  let { mode, catalogueCount, prayingCount, onSelectMode, onQuickCapture }: Props = $props();

  // Primary mode picker — icon-segmented control on desktop, label-
  // only <select> on mobile. The icons stay simple line-art glyphs
  // (no Lucide dep) to match the rest of the slim-header family.
  const MODES: { key: Mode; label: string; title: string; icon: string }[] = [
    {
      key: 'read',
      label: 'Read',
      title: 'verse of the day + AI commentary (1)',
      // Open book.
      icon: 'M3 5h7v14H3z M14 5h7v14h-7z M10 5v14 M14 5v14'
    },
    {
      key: 'memo',
      label: 'Memo',
      title: 'cloze-deletion memorization drill (2)',
      // Speech-bubble with dots = recite.
      icon: 'M4 5h16v10H8l-4 4z M8 10h.01 M12 10h.01 M16 10h.01'
    },
    {
      key: 'browse',
      label: 'Browse',
      title: 'catalogue verses + topical filter + AI search (3)',
      // List rows.
      icon: 'M4 6h16 M4 12h16 M4 18h16'
    },
    {
      key: 'bible',
      label: 'Bible',
      title: 'full bible reader — random passage, search, book picker (4)',
      // Cross.
      icon: 'M10 4h4v6h6v4h-6v6h-4v-6H4v-4h6z'
    },
    {
      key: 'bookmarks',
      label: 'Marks',
      title: 'saved bookmarks (5)',
      // Bookmark ribbon.
      icon: 'M6 4h12v16l-6-4-6 4z'
    },
    {
      key: 'intentions',
      label: 'Prayer',
      title: 'prayer intentions — currently-praying / answered / archived (6)',
      // Praying-hands stylised as joined chevrons.
      icon: 'M8 4l4 8 M16 4l-4 8 M12 12v8 M6 20h12'
    }
  ];
</script>

<div class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle">
  <h1 class="text-base sm:text-lg font-semibold text-text leading-none">Scripture</h1>

  <!-- Catalogue count chip — verses in the curated rotation. Stays
       visible regardless of mode so the user has an at-a-glance sense
       of vault depth. -->
  <span class="inline-flex items-center gap-1 text-[11px] font-mono tabular-nums">
    <span class="px-1.5 py-0.5 bg-surface0 border border-surface1 rounded text-dim">
      <span class="text-text font-semibold">{catalogueCount}</span>
    </span>
  </span>

  <span class="flex-1"></span>

  <!-- Icon-segmented mode picker. Active mode = primary background,
       inactive = subtext on surface0. Badge dots ride on the modes
       that carry a live count (Browse: vault size, Prayer: currently
       praying). -->
  <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden">
    {#each MODES as m (m.key)}
      <button
        type="button"
        onclick={() => onSelectMode(m.key)}
        title={m.title}
        aria-label={m.label}
        aria-pressed={mode === m.key}
        class="px-2 py-1.5 inline-flex items-center gap-1 text-xs {mode === m.key ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d={m.icon} />
        </svg>
        <span class="hidden md:inline">{m.label}</span>
        {#if m.key === 'browse' && catalogueCount > 0 && mode !== 'browse'}
          <span class="text-[10px] tabular-nums font-mono opacity-70">{catalogueCount}</span>
        {/if}
        {#if m.key === 'intentions' && prayingCount > 0 && mode !== 'intentions'}
          <span class="text-[10px] tabular-nums font-mono text-secondary">{prayingCount}</span>
        {/if}
      </button>
    {/each}
  </div>

  <!-- Mobile fallback: every mode in a single <select>. Avoids the
       6-icon row collapsing into something untappable on a phone. -->
  <select
    class="sm:hidden bg-surface0 border border-surface1 rounded px-2 py-1 text-xs text-text"
    value={mode}
    onchange={(e) => onSelectMode((e.currentTarget as HTMLSelectElement).value as Mode)}
    aria-label="mode"
  >
    {#each MODES as m (m.key)}
      <option value={m.key}>{m.label}</option>
    {/each}
  </select>

  <!-- Primary action: "Another verse" / quick capture into Read. The
       button is mode-aware — the page wires it to the relevant
       per-mode action (anotherOne in read, startDrill in memo, etc.).
       Always visible so the chrome carries one obvious next step. -->
  <button
    type="button"
    onclick={onQuickCapture}
    aria-label="Quick action"
    title="Re-roll: next verse / next drill / random passage depending on the mode"
    class="px-2 py-1.5 text-xs bg-primary text-on-primary rounded hover:opacity-90 inline-flex items-center gap-1"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3 12a9 9 0 1 0 3-6.7 M3 4v5h5" />
    </svg>
    <span class="hidden md:inline">Next</span>
  </button>
</div>
