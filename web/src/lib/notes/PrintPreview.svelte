<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import PrintToolbar from './printpreview/PrintToolbar.svelte';
  import PrintConfigPanel from './printpreview/PrintConfigPanel.svelte';
  import PrintRenderPane from './printpreview/PrintRenderPane.svelte';
  import {
    CERT_STRINGS,
    computeHash,
    type Mode,
    type CertVariant,
    type CertLang
  } from './printpreview/strings';
  // All print styling (overlay chrome, toolbar, config panel, paper
  // surface, signature footer, @media print) lives in print.css —
  // a side-effect CSS module so the rules are unscoped and the
  // subcomponents in /printpreview/ can pick them up. Keeping print
  // CSS in one place makes the load-bearing @media print block easy
  // to audit; scattering it across components is the original sin.
  import './printpreview/print.css';

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
  // Layout (post-decomposition):
  //   • PrintToolbar    — slim top row: Close · Title · Mode · Sign ·
  //                       More ▾ · Print primary
  //   • PrintConfigPanel — collapsible Document settings (header,
  //                        footer, signer, purpose, save button).
  //                        Starts COLLAPSED so the first thing the
  //                        user sees is the Print button.
  //   • PrintRenderPane — the actual paper surface with the four
  //                        templates (standard / report / letterhead /
  //                        memo) and the optional signature footer.
  //
  // The actual print itself is plain window.print(); the OS dialog
  // takes over and the user picks "Save as PDF" or sends to a
  // physical printer. Zero server work, zero new dependencies.

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
  // Document settings panel starts COLLAPSED. The vast majority of
  // prints reuse the persisted defaults; the user only needs the
  // panel when overriding header/footer for this one document.
  let configOpen = $state(false);
  let savingConfig = $state(false);
  // Baseline snapshot of the last known-saved values. configDirty
  // is derived by comparing the live state against this baseline,
  // so writing into header/footer/mode during the onMount server
  // load doesn't falsely mark the form as dirty (which it used to:
  // the localStorage write-through effects flipped configDirty=true
  // immediately after `loaded=true; configDirty=false`, because the
  // server-load reassignment had changed the tracked values). Reset
  // the baseline after every successful save so the "save" button
  // disables again.
  let baselineHeader = $state('');
  let baselineFooter = $state('');
  let baselineMode = $state<Mode>('standard');
  let configDirty = $derived(
    header !== baselineHeader || footer !== baselineFooter || mode !== baselineMode
  );

  // "Sign document" — appends a tamper-detection footer to the
  // printed document with a SHA-256 of the body, generated-at
  // timestamp, and a one-line provenance. Like the integrity stamp
  // a signed PDF carries: not a legal e-signature, but a verifiable
  // claim that the document was generated through Granit and that
  // the bytes haven't been altered since.
  const SIG_KEY = 'granit.print.signature';
  const SIG_SIGNER_KEY = 'granit.print.signer';
  const SIG_PURPOSE_KEY = 'granit.print.purpose';
  const CERT_VARIANT_KEY = 'granit.print.certVariant';
  const CERT_LANG_KEY = 'granit.print.certLang';

  // Default to 'compact' — the user's pain point was that the
  // standard signature footer pushes content off the page. Compact
  // is small enough to sit at the foot of any A4 page without
  // disrupting pagination.
  let certVariant = $state<CertVariant>('compact');
  let certLang = $state<CertLang>('en');
  let signatureOn = $state(false);
  let signatureHash = $state('');
  let signatureTimestamp = $state('');
  let signer = $state('');
  let purpose = $state('');

  // A short, stable document identifier derived from the hash
  // (first 8 chars of SHA-256, uppercase, hyphenated).
  let docID = $derived.by(() => {
    if (!signatureHash) return '…';
    const h = signatureHash.slice(0, 8).toUpperCase();
    return `${h.slice(0, 4)}-${h.slice(4, 8)}`;
  });

  let str = $derived(CERT_STRINGS[certLang]);

  // Word + char counts for the signature block — small but
  // useful integrity datapoints alongside the hash.
  let docWords = $derived(body.trim() ? body.trim().split(/\s+/).length : 0);
  let docChars = $derived(body.length);
  let docLines = $derived(body ? body.split('\n').length : 0);

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
      // 'certificate' was a previous template mode that's been
      // removed — silently coerce any saved value to 'standard'.
      if (m === 'standard' || m === 'report' || m === 'letterhead' || m === 'memo') mode = m;
      else if (m === 'certificate') mode = 'standard';
      signatureOn = localStorage.getItem(SIG_KEY) === '1';
      signer = localStorage.getItem(SIG_SIGNER_KEY) ?? '';
      purpose = localStorage.getItem(SIG_PURPOSE_KEY) ?? '';
      const cv = localStorage.getItem(CERT_VARIANT_KEY);
      if (cv === 'standard' || cv === 'compact') certVariant = cv;
      const cl = localStorage.getItem(CERT_LANG_KEY);
      if (cl === 'en' || cl === 'de') certLang = cl;
    } catch {}
    try {
      const cfg = await api.getPrintConfig();
      const serverHasAny = !!(cfg.header || cfg.footer);
      const localHasAny = !!(header || footer);
      if (serverHasAny) {
        header = cfg.header;
        footer = cfg.footer;
        if (cfg.mode === 'standard' || cfg.mode === 'report' || cfg.mode === 'letterhead' || cfg.mode === 'memo') {
          mode = cfg.mode;
        } else if (cfg.mode === 'certificate') {
          mode = 'standard';
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
    // Snapshot the post-load values as the baseline. configDirty
    // is now `derived(header !== baselineHeader || …)`, so until
    // the user edits a field the form reads as clean.
    baselineHeader = header;
    baselineFooter = footer;
    baselineMode = mode;
  });

  // localStorage is the warm cache — every change writes through so
  // the next open paints instantly with the latest values, even
  // before the server confirms. Server save is explicit (button) to
  // avoid round-tripping on every keystroke.
  // Pure write-through to localStorage. configDirty is derived from
  // baseline comparison (above), so these effects no longer carry
  // any dirty-tracking responsibility.
  $effect(() => {
    void header;
    if (!loaded) return;
    try { localStorage.setItem(HEADER_KEY, header); } catch {}
  });
  $effect(() => {
    void footer;
    if (!loaded) return;
    try { localStorage.setItem(FOOTER_KEY, footer); } catch {}
  });
  $effect(() => {
    void mode;
    if (!loaded) return;
    try { localStorage.setItem(MODE_KEY, mode); } catch {}
  });
  $effect(() => {
    void signatureOn;
    if (!loaded) return;
    try { localStorage.setItem(SIG_KEY, signatureOn ? '1' : '0'); } catch {}
  });
  $effect(() => {
    void signer;
    if (!loaded) return;
    try { localStorage.setItem(SIG_SIGNER_KEY, signer); } catch {}
  });
  $effect(() => {
    void purpose;
    if (!loaded) return;
    try { localStorage.setItem(SIG_PURPOSE_KEY, purpose); } catch {}
  });
  $effect(() => {
    void certVariant;
    if (!loaded) return;
    try { localStorage.setItem(CERT_VARIANT_KEY, certVariant); } catch {}
  });
  $effect(() => {
    void certLang;
    if (!loaded) return;
    try { localStorage.setItem(CERT_LANG_KEY, certLang); } catch {}
  });

  // Document signature: SHA-256 of the rendered body + a frozen
  // generated-at timestamp. Recomputed when the body or signature
  // toggle changes — re-hashing on every keystroke is fine because
  // SubtleCrypto is fast for small docs (microseconds), and a fresh
  // hash is the whole point: the signature claims THIS exact body
  // is what got produced.
  //
  // Timestamp is captured at signature-on rather than per-render so
  // the user reads the same "Generated at" line every time the
  // overlay opens — useful when they're previewing and want to know
  // the moment they signed off.
  $effect(() => {
    if (!signatureOn) {
      signatureHash = '';
      signatureTimestamp = '';
      return;
    }
    if (!signatureTimestamp) {
      signatureTimestamp = new Date().toISOString();
    }
    const snap = body;
    void computeHash(snap).then((h) => {
      // Late-arriving hash from a stale snapshot — only commit if
      // the body hasn't changed since we kicked off the digest.
      if (snap === body) signatureHash = h;
    });
  });

  // Teleport: when the overlay opens, move its DOM node to be a
  // direct child of document.body. SvelteKit wraps content in a
  // `<div style="display: contents">` (see web/src/app.html), so an
  // overlay rendered inline inside a page component is nested at
  // body > div > … > .print-overlay. Multiple previous attempts at
  // print isolation failed because of this nesting (CSS `body > *`
  // selectors hit the wrapper, not the overlay). Moving the node
  // out of its component subtree solves it: in print mode we can
  // now reliably hide every direct child of body except the
  // overlay, and content paginates normally.
  let overlayEl: HTMLDivElement | undefined = $state();
  $effect(() => {
    if (!open || !overlayEl) return;
    const original = overlayEl.parentNode;
    document.body.appendChild(overlayEl);
    return () => {
      // On close, move the node back so Svelte's lifecycle can
      // reconcile it cleanly. If the original parent has already
      // been unmounted (e.g. the user navigated away), Svelte's
      // {#if open} cleanup will tear it down regardless.
      try {
        if (original && original.isConnected) {
          original.appendChild(overlayEl!);
        }
      } catch {}
    };
  });

  async function saveConfigToServer() {
    if (savingConfig) return;
    savingConfig = true;
    try {
      await api.putPrintConfig({ header, footer, mode });
      // Re-snapshot baseline so configDirty derives back to false.
      baselineHeader = header;
      baselineFooter = footer;
      baselineMode = mode;
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
</script>

{#if open}
  <!-- Teleport target. The actual overlay is moved to be a direct
       child of document.body via the effect above — without that,
       SvelteKit's `<div style="display: contents">` wrapper sits
       between body and our overlay, which broke every previous
       attempt at print isolation. -->
  <div bind:this={overlayEl} class="print-overlay" role="dialog" aria-label="Print preview">
    <PrintToolbar
      bind:mode
      bind:signatureOn
      bind:certVariant
      bind:certLang
      bind:configOpen
      {configDirty}
      {savingConfig}
      onClose={close}
      onPrint={doPrint}
      onSaveVaultDefault={saveConfigToServer}
    />

    {#if configOpen}
      <PrintConfigPanel
        {mode}
        bind:header
        bind:footer
        bind:signer
        bind:purpose
        {signatureOn}
        {certLang}
        {savingConfig}
        {configDirty}
        onSave={saveConfigToServer}
      />
    {/if}

    <PrintRenderPane
      {mode}
      {title}
      {body}
      {sourcePath}
      {header}
      {footer}
      {signatureOn}
      {certVariant}
      {certLang}
      {str}
      {signer}
      {purpose}
      {signatureHash}
      {signatureTimestamp}
      {docID}
      {docWords}
      {docChars}
      {docLines}
    />
  </div>
{/if}

