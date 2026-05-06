<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import MarkdownRenderer from './MarkdownRenderer.svelte';
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';

  // PrintPreview — fullscreen overlay for "save as PDF" of a note.
  //
  // Why a dedicated overlay instead of just window.print():
  //   The note view is full of chrome (sidebars, toolbar, FABs,
  //   editor gutters) that print-default would include. Even with
  //   @media print rules, hiding everything reliably across the
  //   responsive layout is brittle. An overlay gives us a clean
  //   surface where what-you-see-is-what-prints — header at top,
  //   rendered body in the middle, footer at bottom, nothing else.
  //
  // Header + footer are user-editable inline (live preview) and
  // persisted in localStorage so the next print reuses the same
  // values. A "mode" selector flips three professional layouts:
  //   - standard:    A4, 2cm margins, sans-serif body, line under header
  //   - certificate: landscape-ish proportions, centered title, larger
  //                  serif body, decorative footer line — for formal
  //                  documents the user sends to colleagues
  //   - report:      narrower line length, numbered sections, page #
  //                  in footer — for longer multi-page docs
  //
  // The actual print itself is plain window.print(); the OS dialog
  // takes over and the user picks "Save as PDF" or sends to a
  // physical printer. Zero server work, zero new dependencies.

  type Mode = 'standard' | 'certificate' | 'report';

  interface Props {
    open: boolean;
    title: string;
    body: string;
    sourcePath: string;
    onClose: () => void;
  }

  let { open = $bindable(false), title, body, sourcePath, onClose }: Props = $props();

  // Per-vault defaults stored at .granit/print-config.json on the
  // server so an "ACME — Internal" header set on the desktop is
  // already there next time the user prints from their phone. The
  // legacy localStorage keys (granit.print.{header,footer,mode}) are
  // kept as a one-time migration source: if the server has nothing
  // and localStorage does, we adopt those values + push them up so
  // existing users don't lose their settings on first run after this
  // change. localStorage is ALSO updated alongside server writes as a
  // session-fast cache for re-opens before /print-config resolves.
  const HEADER_KEY = 'granit.print.header';
  const FOOTER_KEY = 'granit.print.footer';
  const MODE_KEY = 'granit.print.mode';

  let header = $state('');
  let footer = $state('');
  let mode = $state<Mode>('standard');
  let configOpen = $state(false);
  let savingConfig = $state(false);
  let configDirty = $state(false);

  // Load order: localStorage immediately (so the overlay paints with
  // SOMETHING fast even on a slow server), then await the server. If
  // the server returns non-empty values they overwrite the local
  // copy; if it returns empty AND we have local values, push the
  // local values up (one-time migration).
  let loaded = $state(false);
  onMount(async () => {
    try {
      header = localStorage.getItem(HEADER_KEY) ?? '';
      footer = localStorage.getItem(FOOTER_KEY) ?? '';
      const m = localStorage.getItem(MODE_KEY);
      if (m === 'standard' || m === 'certificate' || m === 'report') mode = m;
    } catch {}
    try {
      const cfg = await api.getPrintConfig();
      const serverHasAny = !!(cfg.header || cfg.footer);
      const localHasAny = !!(header || footer);
      if (serverHasAny) {
        header = cfg.header;
        footer = cfg.footer;
        if (cfg.mode === 'standard' || cfg.mode === 'certificate' || cfg.mode === 'report') {
          mode = cfg.mode;
        }
      } else if (localHasAny) {
        // Migrate localStorage → server so this device's history
        // becomes the vault default. Best-effort: a network error
        // here just means the migration retries next mount.
        try {
          await api.putPrintConfig({ header, footer, mode });
        } catch {}
      }
    } catch {
      // Server unreachable / endpoint not deployed yet — fall back
      // entirely to the localStorage values we already loaded.
    }
    loaded = true;
    configDirty = false;
  });

  // localStorage is the warm cache — every change writes through so
  // the next open paints instantly with the latest values, even
  // before the server confirms. Server save is explicit (button) to
  // avoid round-tripping on every keystroke.
  $effect(() => {
    void header;
    if (!loaded) return;
    try { localStorage.setItem(HEADER_KEY, header); } catch {}
    configDirty = true;
  });
  $effect(() => {
    void footer;
    if (!loaded) return;
    try { localStorage.setItem(FOOTER_KEY, footer); } catch {}
    configDirty = true;
  });
  $effect(() => {
    void mode;
    if (!loaded) return;
    try { localStorage.setItem(MODE_KEY, mode); } catch {}
    configDirty = true;
  });

  async function saveConfigToServer() {
    if (savingConfig) return;
    savingConfig = true;
    try {
      await api.putPrintConfig({ header, footer, mode });
      configDirty = false;
      toast.success('Print defaults saved');
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      savingConfig = false;
    }
  }

  function close() {
    onClose();
  }

  function doPrint() {
    // Defer one tick so the DOM reflects the latest header/footer
    // edits before the print dialog snapshots the page.
    setTimeout(() => window.print(), 16);
  }

  // ESC closes; Mod-P prints from inside the overlay so the user
  // doesn't have to reach for the mouse after editing the header.
  // Capture phase + stopImmediatePropagation so the global Mod-P
  // (CommandPalette quick-switcher) doesn't fire underneath us —
  // otherwise opening the print preview and hitting Mod-P would
  // both print AND open the switcher, which is hostile UX.
  function onKey(e: KeyboardEvent) {
    if (!open) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopImmediatePropagation();
      close();
    } else if ((e.metaKey || e.ctrlKey) && !e.shiftKey && e.key.toLowerCase() === 'p') {
      e.preventDefault();
      e.stopImmediatePropagation();
      doPrint();
    }
  }
  $effect(() => {
    if (!open) return;
    window.addEventListener('keydown', onKey, { capture: true });
    return () => window.removeEventListener('keydown', onKey, { capture: true });
  });

  // Today's date in the user's locale, used as a default placeholder
  // when the footer field is empty so the user can add date fast.
  function todayHuman(): string {
    return new Date().toLocaleDateString(undefined, {
      year: 'numeric', month: 'long', day: 'numeric'
    });
  }
</script>

{#if open}
  <div class="print-overlay" role="dialog" aria-label="Print preview">
    <!-- Toolbar — hidden in print via @media print. Lets the user
         tweak header/footer/mode without leaving the preview. -->
    <header class="print-toolbar">
      <button onclick={close} class="tb-btn" title="Close (Esc)">× Close</button>
      <span class="tb-sep"></span>
      <button
        onclick={() => (configOpen = !configOpen)}
        class="tb-btn {configOpen ? 'tb-active' : ''}"
        title="Edit header / footer"
      >⚙ Configure</button>
      <div class="tb-modes">
        {#each [
          { id: 'standard', label: 'Standard' },
          { id: 'certificate', label: 'Certificate' },
          { id: 'report', label: 'Report' }
        ] as m}
          <button
            onclick={() => (mode = m.id as Mode)}
            class="tb-mode {mode === m.id ? 'tb-active' : ''}"
          >{m.label}</button>
        {/each}
      </div>
      <span class="tb-spacer"></span>
      <button onclick={doPrint} class="tb-btn tb-primary" title="Print (⌘P)">🖨 Print / Save as PDF</button>
    </header>

    {#if configOpen}
      <section class="config-panel">
        <div class="config-row">
          <label for="print-header">Header</label>
          <input
            id="print-header"
            bind:value={header}
            placeholder="e.g. ACME Corp — Internal"
            class="config-input"
          />
        </div>
        <div class="config-row">
          <label for="print-footer">Footer</label>
          <input
            id="print-footer"
            bind:value={footer}
            placeholder={todayHuman()}
            class="config-input"
          />
        </div>
        <div class="config-actions">
          <span class="config-hint">
            Saved on this device by default. Click <strong>Save as vault default</strong>
            to sync the values to <code>.granit/print-config.json</code> so they
            travel across browsers and devices.
          </span>
          <button
            type="button"
            onclick={saveConfigToServer}
            disabled={savingConfig || !configDirty}
            class="config-save"
            title={configDirty ? 'Save header/footer/mode to the vault' : 'No changes to save'}
          >{savingConfig ? 'saving…' : configDirty ? 'Save as vault default' : '✓ saved'}</button>
        </div>
      </section>
    {/if}

    <!-- The printable surface. data-mode toggles the layout. -->
    <main class="print-page" data-mode={mode}>
      {#if mode === 'certificate'}
        <!-- Certificate layout — formal document framing. The double
             border + corner ornaments + formal serif typography give
             the page authenticity at a glance, and the Granit seal
             at the bottom doubles as a verifiable provenance mark
             (a colleague who suspects a forgery can check the repo). -->
        <div class="cert-frame">
          <div class="cert-corner cert-corner-tl">❦</div>
          <div class="cert-corner cert-corner-tr">❦</div>
          <div class="cert-corner cert-corner-bl">❦</div>
          <div class="cert-corner cert-corner-br">❦</div>
          <div class="cert-content">
            {#if header}
              <div class="cert-issuer">{header}</div>
            {/if}
            <div class="cert-eyebrow">— Certificate —</div>
            <h1 class="cert-title">{title}</h1>
            <div class="cert-divider">
              <span class="cert-divider-line"></span>
              <span class="cert-divider-mark">⁕</span>
              <span class="cert-divider-line"></span>
            </div>
            <article class="cert-body">
              <MarkdownRenderer body={body} />
            </article>
            <div class="cert-footer">
              <div class="cert-footer-row">
                <div class="cert-footer-cell">
                  <div class="cert-footer-line"></div>
                  <div class="cert-footer-label">Date</div>
                  <div class="cert-footer-value">{footer || todayHuman()}</div>
                </div>
                <div class="cert-seal">
                  <!-- Round embossed-style seal in pure SVG. Renders
                       crisp at any zoom + survives PDF export at full
                       fidelity (no raster artefacts). The repo URL
                       and a Granit wordmark sit on the inner ring so
                       a colleague verifying the certificate can read
                       the provenance directly off the seal. -->
                  <svg viewBox="0 0 120 120" width="86" height="86" aria-label="Granit seal">
                    <defs>
                      <path id="seal-arc-top" d="M 60 60 m -44 0 a 44 44 0 0 1 88 0" fill="none"/>
                      <path id="seal-arc-bot" d="M 60 60 m 44 0 a 44 44 0 0 1 -88 0" fill="none"/>
                    </defs>
                    <circle cx="60" cy="60" r="54" fill="none" stroke="#8a6d3b" stroke-width="1.5"/>
                    <circle cx="60" cy="60" r="48" fill="none" stroke="#8a6d3b" stroke-width="0.6"/>
                    <circle cx="60" cy="60" r="30" fill="none" stroke="#8a6d3b" stroke-width="1"/>
                    <text font-family="Georgia, serif" font-size="7" letter-spacing="1.5" fill="#8a6d3b">
                      <textPath href="#seal-arc-top" startOffset="50%" text-anchor="middle">GENERATED · WITH · GRANIT</textPath>
                    </text>
                    <text font-family="Georgia, serif" font-size="5" letter-spacing="0.8" fill="#8a6d3b">
                      <textPath href="#seal-arc-bot" startOffset="50%" text-anchor="middle">github.com/artaeon/granit</textPath>
                    </text>
                    <text x="60" y="55" font-family="Georgia, serif" font-size="14" font-weight="700" text-anchor="middle" fill="#8a6d3b" letter-spacing="2">G</text>
                    <text x="60" y="68" font-family="Georgia, serif" font-size="6" letter-spacing="2" text-anchor="middle" fill="#8a6d3b">GRANIT</text>
                    <text x="60" y="76" font-family="Georgia, serif" font-size="3.5" text-anchor="middle" fill="#8a6d3b">{todayHuman()}</text>
                  </svg>
                </div>
              </div>
              <div class="cert-provenance">
                Issued via Granit · open source at <a href="https://github.com/artaeon/granit">github.com/artaeon/granit</a> · {sourcePath}
              </div>
            </div>
          </div>
        </div>
      {:else}
        <header class="print-header">
          {#if header}<div class="print-header-text">{header}</div>{/if}
        </header>
        <article class="print-body">
          <h1 class="doc-title">{title}</h1>
          <div class="doc-meta">{sourcePath} · {todayHuman()}</div>
          <MarkdownRenderer body={body} />
        </article>
        <footer class="print-footer">
          <div class="print-footer-text">
            {footer || todayHuman()}
          </div>
        </footer>
      {/if}
    </main>
  </div>
{/if}

<style>
  /* Overlay container — full viewport, dark wash background so the
     "page" inside reads like a real document. */
  .print-overlay {
    position: fixed;
    inset: 0;
    z-index: 60;
    background: var(--color-mantle);
    overflow-y: auto;
    display: flex;
    flex-direction: column;
  }

  /* Toolbar — sticky top so the user can hit Print after scrolling
     through a long document. Hidden in print via @media print at
     the bottom of this stylesheet. */
  .print-toolbar {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 1rem;
    background: var(--color-base);
    border-bottom: 1px solid var(--color-surface1);
    flex-wrap: wrap;
  }
  .tb-btn {
    padding: 0.25rem 0.75rem;
    border: 1px solid var(--color-surface1);
    border-radius: 0.25rem;
    background: var(--color-surface0);
    color: var(--color-subtext);
    font-size: 0.8125rem;
    cursor: pointer;
  }
  .tb-btn:hover { border-color: var(--color-primary); color: var(--color-text); }
  .tb-active { background: var(--color-primary); color: var(--color-on-primary); border-color: var(--color-primary); }
  .tb-primary { background: var(--color-primary); color: var(--color-on-primary); border-color: var(--color-primary); }
  .tb-primary:hover { opacity: 0.9; }
  .tb-sep { width: 1px; height: 1.5rem; background: var(--color-surface1); }
  .tb-modes { display: inline-flex; border: 1px solid var(--color-surface1); border-radius: 0.25rem; overflow: hidden; }
  .tb-mode {
    padding: 0.25rem 0.75rem;
    background: transparent;
    color: var(--color-subtext);
    font-size: 0.8125rem;
    border: none;
    border-right: 1px solid var(--color-surface1);
    cursor: pointer;
  }
  .tb-mode:last-child { border-right: none; }
  .tb-mode:hover { background: var(--color-surface0); color: var(--color-text); }
  .tb-spacer { flex: 1; }

  .config-panel {
    padding: 0.75rem 1rem;
    background: var(--color-surface0);
    border-bottom: 1px solid var(--color-surface1);
  }
  .config-row {
    display: grid;
    grid-template-columns: 5rem 1fr;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.5rem;
  }
  .config-row label {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-dim);
  }
  .config-input {
    padding: 0.375rem 0.625rem;
    background: var(--color-base);
    border: 1px solid var(--color-surface1);
    border-radius: 0.25rem;
    color: var(--color-text);
    font-size: 0.875rem;
    font-family: inherit;
  }
  .config-input:focus { outline: none; border-color: var(--color-primary); }
  .config-hint {
    font-size: 0.6875rem;
    color: var(--color-dim);
    line-height: 1.5;
    flex: 1;
  }
  .config-actions {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-top: 0.5rem;
  }
  .config-save {
    flex-shrink: 0;
    padding: 0.25rem 0.75rem;
    background: var(--color-secondary);
    color: var(--color-mantle);
    border: none;
    border-radius: 0.25rem;
    font-size: 0.75rem;
    font-weight: 500;
    cursor: pointer;
  }
  .config-save:hover { opacity: 0.9; }
  .config-save:disabled { opacity: 0.5; cursor: not-allowed; }

  /* The page itself — A4-shaped on screen so the user sees what
     prints. White background with shadow to feel like paper. */
  .print-page {
    width: 21cm;
    min-height: 29.7cm;
    margin: 2rem auto;
    padding: 2cm 1.5cm;
    background: white;
    color: #1a1a1a;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
    font-size: 11pt;
    line-height: 1.55;
  }
  .print-page[data-mode="report"] {
    font-size: 10.5pt;
    padding: 2.5cm 2cm;
  }
  .print-page[data-mode="certificate"] {
    font-family: 'Iowan Old Style', Georgia, 'Times New Roman', serif;
    font-size: 12pt;
    padding: 0;
    line-height: 1.6;
    color: #2a2419;
  }

  /* ----- Certificate template ----- */
  /* A formal document framing. The double border + corner ornaments
     give the page a "real certificate" feel without leaning on
     external fonts (which would fail in print or with a CSP). The
     Granit seal at the bottom is pure inline SVG so it survives
     PDF export crisp at any zoom and acts as verifiable provenance. */
  .cert-frame {
    position: relative;
    margin: 1.2cm;
    padding: 1.6cm 1.5cm 1.4cm;
    border: 2px solid #8a6d3b;
    box-shadow: inset 0 0 0 2px #fff, inset 0 0 0 4px #d4b87a;
    background: #fbf7ee;
    min-height: calc(29.7cm - 2.4cm);
    display: flex;
    flex-direction: column;
  }
  .cert-corner {
    position: absolute;
    color: #8a6d3b;
    font-size: 18pt;
    line-height: 1;
  }
  .cert-corner-tl { top: 0.4cm; left: 0.5cm; }
  .cert-corner-tr { top: 0.4cm; right: 0.5cm; transform: rotate(90deg); }
  .cert-corner-bl { bottom: 0.4cm; left: 0.5cm; transform: rotate(-90deg); }
  .cert-corner-br { bottom: 0.4cm; right: 0.5cm; transform: rotate(180deg); }

  .cert-content {
    flex: 1;
    text-align: center;
    display: flex;
    flex-direction: column;
  }
  .cert-issuer {
    font-size: 10pt;
    letter-spacing: 0.3em;
    text-transform: uppercase;
    color: #8a6d3b;
    margin-bottom: 0.5rem;
  }
  .cert-eyebrow {
    font-style: italic;
    font-size: 11pt;
    color: #8a6d3b;
    margin-top: 0.5rem;
    letter-spacing: 0.05em;
  }
  .cert-title {
    font-family: 'Iowan Old Style', Georgia, 'Times New Roman', serif;
    font-size: 26pt;
    font-weight: 700;
    margin: 0.6rem 0 0.2rem;
    color: #2a2419;
    letter-spacing: 0.01em;
    line-height: 1.15;
  }
  .cert-divider {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    margin: 0.5rem 0 1.2rem;
    color: #8a6d3b;
  }
  .cert-divider-line {
    flex: 0 1 25%;
    height: 1px;
    background: linear-gradient(to right, transparent, #8a6d3b, transparent);
  }
  .cert-divider-mark {
    font-size: 14pt;
  }
  .cert-body {
    flex: 1;
    text-align: left;
    font-size: 11.5pt;
    color: #2a2419;
    /* The body of a certificate reads denser than freeform notes;
       a tighter measure (max 16cm) keeps line lengths comfortable. */
    max-width: 16cm;
    margin: 0 auto;
  }
  .cert-body :global(.prose-note),
  .cert-body :global(.prose-note *) {
    color: #2a2419 !important;
  }
  .cert-body :global(.prose-note h1),
  .cert-body :global(.prose-note h2) {
    font-family: 'Iowan Old Style', Georgia, serif !important;
    color: #2a2419 !important;
    text-align: center;
  }
  .cert-footer {
    margin-top: 1.5rem;
    padding-top: 0.8rem;
  }
  .cert-footer-row {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 2rem;
    text-align: left;
  }
  .cert-footer-cell {
    flex: 1;
    min-width: 0;
  }
  .cert-footer-line {
    border-top: 1px solid #8a6d3b;
    margin-bottom: 0.25rem;
    width: 100%;
    max-width: 6cm;
  }
  .cert-footer-label {
    font-size: 8pt;
    letter-spacing: 0.15em;
    text-transform: uppercase;
    color: #8a6d3b;
  }
  .cert-footer-value {
    font-size: 11pt;
    color: #2a2419;
    margin-top: 0.1rem;
  }
  .cert-seal {
    flex-shrink: 0;
    /* The seal sits flush-right; its 86px width keeps it from
       fighting the date column for space on standard A4. */
  }
  .cert-provenance {
    margin-top: 1.2rem;
    padding-top: 0.6rem;
    border-top: 0.5pt solid #d4b87a;
    font-size: 8pt;
    color: #8a6d3b;
    text-align: center;
    font-style: italic;
    letter-spacing: 0.02em;
  }
  .cert-provenance a {
    color: #8a6d3b;
    text-decoration: none;
  }

  .print-header {
    border-bottom: 1px solid #444;
    padding-bottom: 0.5rem;
    margin-bottom: 1.5rem;
    font-size: 9.5pt;
    color: #555;
  }
  .print-page[data-mode="certificate"] .print-header {
    border-bottom: none;
    margin-bottom: 0;
  }
  .print-header-text {
    font-weight: 600;
    letter-spacing: 0.02em;
  }
  .cert-flourish {
    font-size: 1.5rem;
    color: #888;
    margin-bottom: 0.5rem;
  }

  .print-body {
    flex: 1;
  }
  .doc-title {
    font-size: 22pt;
    font-weight: 600;
    margin: 0 0 0.25rem 0;
    color: #1a1a1a;
    line-height: 1.2;
  }
  .doc-meta {
    font-size: 9pt;
    color: #888;
    margin-bottom: 1.5rem;
  }
  .cert-title {
    font-size: 28pt;
    font-weight: 400;
    font-style: italic;
    margin: 1rem 0 0.5rem 0;
    color: #222;
  }
  .cert-rule {
    height: 1px;
    background: linear-gradient(to right, transparent, #888, transparent);
    margin: 0.75rem auto;
    width: 60%;
  }

  .print-footer {
    border-top: 1px solid #444;
    padding-top: 0.5rem;
    margin-top: 1.5rem;
    font-size: 9pt;
    color: #666;
    display: flex;
    justify-content: space-between;
    align-items: baseline;
  }
  .print-page[data-mode="certificate"] .print-footer {
    border-top: none;
    text-align: center;
    justify-content: center;
    margin-top: 2rem;
    font-style: italic;
  }
  .print-footer-text {
    font-variant-numeric: tabular-nums;
  }

  /* MarkdownRenderer overrides for paper. The renderer's container
     class is `.prose-note` (NOT `.prose` — earlier overrides missed
     that and the rendered body inherited the dark-mode tokens, so
     "white text on white paper" was the visible bug in dark mode).
     Force light, paper-friendly styling for every element the body
     can produce; CSS variables get re-bound to literal greys/blacks
     so even tokens we forget to override fall back to readable
     values. */
  .print-page {
    --color-text: #1a1a1a;
    --color-subtext: #404040;
    --color-dim: #6a6a6a;
    --color-surface0: #f6f6f6;
    --color-surface1: #e5e5e5;
    --color-surface2: #d0d0d0;
    --color-base: #ffffff;
    --color-mantle: #ffffff;
    --color-primary: #1a4fb3;
    --color-secondary: #1a4fb3;
  }
  .print-page :global(.prose-note),
  .print-page :global(.prose-note *) {
    color: #1a1a1a !important;
    background: transparent !important;
  }
  .print-page :global(.prose-note h1),
  .print-page :global(.prose-note h2),
  .print-page :global(.prose-note h3),
  .print-page :global(.prose-note h4),
  .print-page :global(.prose-note h5),
  .print-page :global(.prose-note h6) {
    color: #1a1a1a !important;
    page-break-after: avoid;
    break-after: avoid;
  }
  .print-page :global(.prose-note pre),
  .print-page :global(.prose-note code) {
    background: #f4f4f4 !important;
    color: #222 !important;
    border: 1px solid #e5e5e5;
  }
  .print-page :global(.prose-note a) {
    color: #1a4fb3 !important;
    text-decoration: underline;
  }
  .print-page :global(.prose-note blockquote) {
    border-left: 3px solid #888 !important;
    color: #444 !important;
  }
  .print-page :global(.prose-note table),
  .print-page :global(.prose-note th),
  .print-page :global(.prose-note td) {
    border: 1px solid #c0c0c0 !important;
    color: #1a1a1a !important;
  }
  .print-page :global(.prose-note th) {
    background: #f4f4f4 !important;
  }
  .print-page :global(.prose-note hr) {
    border-color: #c0c0c0 !important;
  }
  .print-page :global(.prose-note img) {
    /* Print images at most page-width; otherwise a 4K screenshot
       blows out the layout. Stays !important to override any
       dark-mode filter the screen styles apply. */
    max-width: 100% !important;
    filter: none !important;
  }

  /* THE actual print rules. We hide the overlay's chrome (toolbar,
     config panel, page shadow), reset margins so the OS @page
     handles them, and let the .print-page content flow at native
     dimensions onto the printer. */
  @media print {
    :global(body), :global(html) {
      background: white !important;
      margin: 0;
      padding: 0;
    }
    :global(body > *:not(.print-overlay)) {
      display: none !important;
    }
    .print-overlay {
      position: static !important;
      overflow: visible !important;
      background: white !important;
      display: block !important;
    }
    .print-toolbar, .config-panel { display: none !important; }
    .print-page {
      width: 100% !important;
      min-height: auto !important;
      margin: 0 !important;
      padding: 0 !important;
      box-shadow: none !important;
    }
    @page {
      size: A4;
      margin: 2cm 1.5cm;
    }
    .print-page[data-mode="report"] {
      /* Report mode: tighter margins handled by @page above; nothing
         extra needed at the page level. */
    }
    .print-page[data-mode="certificate"] {
      /* Certificate looks better with extra breathing room on paper. */
    }
    @page :first {
      /* Optional first-page rule kept as a hook for future per-mode
         tweaks; intentionally empty for now. */
    }
  }
</style>
