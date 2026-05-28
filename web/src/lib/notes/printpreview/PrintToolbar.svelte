<script lang="ts">
  // Slim PrintPreview toolbar. Mirrors the calendar HeaderToolbar
  // pattern — a single row carrying only what the user reaches for
  // every print: Close · Title · Mode segmented · Sign chip ·
  // spacer · More ▾ · Print (primary). Less-common controls (open
  // the Document settings panel, signature variant, language, save
  // vault default) live in the More dropdown.
  //
  // The first thing a user sees on opening is therefore the Print
  // button — not 20 form fields. Defaults handle the rest.
  import { focusOnMount } from '$lib/util/focusOnMount';
  import type { Mode, CertVariant, CertLang } from './strings';

  let {
    mode = $bindable('standard'),
    signatureOn = $bindable(false),
    certVariant = $bindable('compact'),
    certLang = $bindable('en'),
    configOpen = $bindable(false),
    configDirty,
    savingConfig,
    onClose,
    onPrint,
    onSaveVaultDefault
  }: {
    mode?: Mode;
    signatureOn?: boolean;
    certVariant?: CertVariant;
    certLang?: CertLang;
    configOpen?: boolean;
    configDirty: boolean;
    savingConfig: boolean;
    onClose: () => void;
    onPrint: () => void;
    onSaveVaultDefault: () => void;
  } = $props();

  let moreOpen = $state(false);
  function toggleMore() { moreOpen = !moreOpen; }
  function closeMore() { moreOpen = false; }

  function onMoreKey(e: KeyboardEvent) {
    if (e.key === 'Escape') { closeMore(); }
  }

  // Click-outside guard for the More dropdown. Stays on window
  // because the toolbar is fixed and the menu can overlap any
  // sibling node in the overlay.
  function onWindowClick(e: MouseEvent) {
    if (!moreOpen) return;
    const target = e.target as HTMLElement | null;
    if (!target?.closest('[data-print-more]')) closeMore();
  }

  const MODES: { id: Mode; label: string; title: string }[] = [
    { id: 'standard',   label: 'Standard',   title: 'Standard A4 — title + body + footer' },
    { id: 'letterhead', label: 'Letterhead', title: 'Letterhead — sender block + serif body' },
    { id: 'memo',       label: 'Memo',       title: 'Memo — TO/FROM/DATE/RE block at top' },
    { id: 'report',     label: 'Report',     title: 'Report — tighter type, denser layout' }
  ];

  // Signing on bumps the title to make the state legible at a glance —
  // and toggling it on auto-opens Document settings so the Signer field
  // surfaces without the user hunting for it.
  function toggleSign() {
    signatureOn = !signatureOn;
    if (signatureOn) configOpen = true;
  }
</script>

<svelte:window onclick={onWindowClick} />

<header class="print-toolbar">
  <!-- Close × — leftmost so Esc-equivalent is muscle-memory. -->
  <button onclick={onClose} class="tb-icon-btn" title="Close (Esc)" aria-label="close print preview">
    <svg viewBox="0 0 24 24" class="tb-icon" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M6 6l12 12 M18 6l-12 12"/>
    </svg>
  </button>

  <h2 class="tb-title">Print Preview</h2>

  <!-- Mode segmented — primary surface for picking the template.
       Stays visible because the layout difference is the single
       most-visible choice the user makes on this screen. -->
  <div class="tb-modes" role="radiogroup" aria-label="Print template">
    {#each MODES as m (m.id)}
      <button
        onclick={() => (mode = m.id)}
        class="tb-mode {mode === m.id ? 'tb-active' : ''}"
        title={m.title}
        aria-pressed={mode === m.id}
      >{m.label}</button>
    {/each}
  </div>

  <!-- Sign chip — toggles signature footer on/off. Self-explanatory
       state via the label change. -->
  <button
    onclick={toggleSign}
    class="tb-chip {signatureOn ? 'tb-active' : ''}"
    title={signatureOn ? 'Document is signed (SHA-256 footer added)' : 'Add signature footer (SHA-256 + timestamp)'}
    aria-pressed={signatureOn}
  >{signatureOn ? 'Signed' : 'Sign'}</button>

  <span class="tb-spacer"></span>

  <!-- More dropdown — Document settings toggle + signature options +
       vault-default save. Single overflow surface so the row stays
       slim regardless of viewport width. -->
  <div class="tb-more-wrap" data-print-more>
    <button
      type="button"
      onclick={toggleMore}
      aria-haspopup="true"
      aria-expanded={moreOpen}
      title="More options (Document settings, signature variant, language, save defaults)"
      class="tb-icon-btn {moreOpen ? 'tb-active' : ''}"
    >
      <svg viewBox="0 0 24 24" class="tb-icon" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
        <circle cx="5" cy="12" r="1" />
        <circle cx="12" cy="12" r="1" />
        <circle cx="19" cy="12" r="1" />
      </svg>
    </button>
    {#if moreOpen}
      <div
        role="menu"
        class="tb-menu"
        onkeydown={onMoreKey}
        use:focusOnMount
        tabindex="-1"
      >
        <!-- Document settings toggle — the panel itself is the
             editor for header / footer / signer / purpose. -->
        <button
          type="button"
          role="menuitem"
          onclick={() => { configOpen = !configOpen; closeMore(); }}
          class="tb-menu-item"
        >
          <span>Document settings</span>
          <span class="tb-menu-state">{configOpen ? 'open' : 'collapsed'}</span>
        </button>

        {#if signatureOn}
          <div class="tb-menu-sep"></div>
          <div class="tb-menu-section">Signature variant</div>
          <div class="tb-menu-radio">
            <button
              type="button"
              onclick={() => (certVariant = 'compact')}
              class="tb-menu-pill {certVariant === 'compact' ? 'tb-active' : ''}"
              title="Slim trust stamp — fits on the same page"
            >Compact</button>
            <button
              type="button"
              onclick={() => (certVariant = 'standard')}
              class="tb-menu-pill {certVariant === 'standard' ? 'tb-active' : ''}"
              title="Full trust certificate footer"
            >Standard</button>
          </div>

          <div class="tb-menu-section">Signature language</div>
          <div class="tb-menu-radio">
            <button
              type="button"
              onclick={() => (certLang = 'en')}
              class="tb-menu-pill {certLang === 'en' ? 'tb-active' : ''}"
              title="English"
            >English</button>
            <button
              type="button"
              onclick={() => (certLang = 'de')}
              class="tb-menu-pill {certLang === 'de' ? 'tb-active' : ''}"
              title="Deutsch"
            >Deutsch</button>
          </div>
        {/if}

        <div class="tb-menu-sep"></div>
        <button
          type="button"
          role="menuitem"
          onclick={() => { onSaveVaultDefault(); closeMore(); }}
          disabled={savingConfig || !configDirty}
          class="tb-menu-item"
          title={configDirty ? 'Save header/footer/mode to .granit/print-config.json' : 'No changes to save'}
        >
          <span>Save as vault default</span>
          <span class="tb-menu-state">{savingConfig ? 'saving…' : configDirty ? 'unsaved' : 'saved'}</span>
        </button>
      </div>
    {/if}
  </div>

  <!-- Print — primary CTA. Always the rightmost, always tinted.
       The single action 90% of overlay opens are working toward. -->
  <button onclick={onPrint} class="tb-primary" title="Print / Save as PDF (⌘P)">
    <svg viewBox="0 0 24 24" class="tb-icon" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M6 9V2h12v7 M6 18H4a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2h-2 M6 14h12v8H6z"/>
    </svg>
    <span>Print</span>
  </button>
</header>
