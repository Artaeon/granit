<script lang="ts">
  // Dev-only logo comparison page. Renders the current icon + 3
  // candidate variants side-by-side at four sizes (16 / 64 / 192 /
  // install) so visual selection isn't a guessing game. The four-
  // panel split (dark/light × small/large) catches the cases that
  // matter:
  //
  //   - 16px (favicon) — does the silhouette read at all?
  //   - 64px (sidebar brand) — is the design legible day-to-day?
  //   - 192px (PWA install) — does it look intentional on the home
  //     screen next to native apps?
  //   - 512px (splash) — does the gemstone tell its story?
  //
  // Route is intentionally NOT linked from the nav — it's a one-time
  // decision surface. Reach it with /dev/logos when you want to choose.

  type Variant = { name: string; file: string; description: string };
  const variants: Variant[] = [
    {
      name: 'Current',
      file: '/icon.svg',
      description: '4-facet hexagonal stone. Already-shipped baseline.'
    },
    {
      name: 'A — Dimensional Stone',
      file: '/logo-variants/dimensional-stone.svg',
      description:
        'Same geometry as current, but wider tonal range (cap-left near-bright, body-right near-black) + thinner outline. Reads as carved gemstone.'
    },
    {
      name: 'B — Monogram G',
      file: '/logo-variants/monogram-g.svg',
      description:
        'Chiseled G letter with a stone-mason wedge notch in the bottom-right corner. Scales perfectly at 16px favicon — the notch is the differentiator at every size.'
    },
    {
      name: 'C — Cornerstone',
      file: '/logo-variants/cornerstone.svg',
      description:
        'Three stacked stone blocks forming a foundation. The top block (the cornerstone) bears a recessed Greek-cross incision — references Eph 2:20 without screaming denomination. Fits kingdom-building purpose.'
    }
  ];

  const sizes: { px: number; label: string }[] = [
    { px: 16, label: '16px · favicon' },
    { px: 64, label: '64px · sidebar' },
    { px: 192, label: '192px · install' },
    { px: 256, label: '256px · splash' }
  ];
</script>

<svelte:head>
  <title>Granit · Logo variants</title>
</svelte:head>

<div class="p-6 max-w-6xl mx-auto">
  <header class="mb-6">
    <h1 class="text-2xl font-semibold text-text">Logo variants</h1>
    <p class="text-sm text-dim mt-1">
      Pick a row, then tell Claude which one to ship. Each row renders the
      SVG at four sizes against both light and dark backdrops so the
      decision isn't guesswork.
    </p>
  </header>

  {#each variants as v (v.file)}
    <section class="mb-6 bg-surface0 border border-surface1 rounded-lg overflow-hidden">
      <header class="px-4 py-3 border-b border-surface1 flex items-baseline gap-3">
        <h2 class="text-base font-semibold text-text">{v.name}</h2>
        <code class="text-[11px] text-dim font-mono">{v.file}</code>
      </header>
      <p class="px-4 py-2 text-sm text-subtext border-b border-surface1">
        {v.description}
      </p>
      <!-- Two-column grid: dark backdrop on the left, light on the
           right. Within each column, the four sizes line up with
           labels underneath. The icon renders via <img> (not inline
           SVG) so the comparison hits the same code path the browser
           uses for favicons + install icons — including the dark/light
           media-query inside the SVG, which only triggers when the SVG
           is loaded as an image and the OS preference matches the
           backdrop. -->
      <div class="grid grid-cols-1 md:grid-cols-2">
        <div class="p-5 bg-[#0a0a0a]">
          <p class="text-[10px] uppercase tracking-wider text-white/50 mb-3 font-medium">Dark backdrop</p>
          <div class="flex items-end gap-6 flex-wrap">
            {#each sizes as s (s.px)}
              <div class="flex flex-col items-center gap-2">
                <img src={v.file} alt="{v.name} at {s.px}px" width={s.px} height={s.px} style="image-rendering: -webkit-optimize-contrast;" />
                <span class="text-[10px] text-white/40 font-mono">{s.label}</span>
              </div>
            {/each}
          </div>
        </div>
        <div class="p-5 bg-[#fafafa]">
          <p class="text-[10px] uppercase tracking-wider text-black/50 mb-3 font-medium">Light backdrop</p>
          <div class="flex items-end gap-6 flex-wrap">
            {#each sizes as s (s.px)}
              <div class="flex flex-col items-center gap-2">
                <img src={v.file} alt="{v.name} at {s.px}px" width={s.px} height={s.px} style="image-rendering: -webkit-optimize-contrast;" />
                <span class="text-[10px] text-black/40 font-mono">{s.label}</span>
              </div>
            {/each}
          </div>
        </div>
      </div>
    </section>
  {/each}

  <footer class="mt-8 px-4 py-3 border border-surface1 rounded text-xs text-dim">
    <p>
      To activate a variant, tell Claude the letter (A / B / C) or "Keep
      current". Claude will copy the SVG into <code>web/static/icon.svg</code>
      + render the PNG sizes + sync the splash markup in
      <code>app.html</code> so the install icon, favicon, and pre-paint
      splash all match.
    </p>
  </footer>
</div>
