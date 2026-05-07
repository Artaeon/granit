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

  type Mode = 'standard' | 'certificate' | 'report' | 'letterhead' | 'memo';

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
  // "Add Granit signature" — appends a tamper-detection footer to
  // the printed document with a SHA-256 of the body, generated-at
  // timestamp, and a one-line provenance. Like the integrity stamp
  // a signed PDF carries: not a legal e-signature, but a verifiable
  // claim that the document was generated through Granit and that
  // the bytes haven't been altered since.
  //
  // Persisted alongside header/footer/mode so the user's choice
  // sticks across exports.
  const SIG_KEY = 'granit.print.signature';
  const SIG_SIGNER_KEY = 'granit.print.signer';
  const SIG_PURPOSE_KEY = 'granit.print.purpose';
  // Certificate-specific options. Variant controls the visual size
  // (standard = full A4 frame; compact = half-page, denser body).
  // Language swaps the eyebrow / issuer / "Signed at" / locale for
  // the timestamp formatter so users can issue German certificates.
  const CERT_VARIANT_KEY = 'granit.print.certVariant';
  const CERT_LANG_KEY = 'granit.print.certLang';
  type CertVariant = 'standard' | 'compact';
  type CertLang = 'en' | 'de';
  let certVariant = $state<CertVariant>('standard');
  let certLang = $state<CertLang>('en');
  let signatureOn = $state(false);
  let signatureHash = $state('');
  let signatureTimestamp = $state('');
  // Optional human-readable claim fields. Equivalent to the
  // "Reason" / "Signer" lines a real signed-PDF carries — useful
  // for a "this report was prepared by Raphael Lugmayr · Q3 review"
  // attestation. Both persist in localStorage so the user doesn't
  // re-type them on every export.
  let signer = $state('');
  let purpose = $state('');
  // A short, stable document identifier derived from the hash
  // (first 8 chars of SHA-256, uppercase, hyphenated). Quick to
  // reference verbally — "doc 7A3F-B1C9" — and fully implied by
  // the full hash so it doesn't add a separate verification claim.
  let docID = $derived.by(() => {
    if (!signatureHash) return '…';
    const h = signatureHash.slice(0, 8).toUpperCase();
    return `${h.slice(0, 4)}-${h.slice(4, 8)}`;
  });

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
      if (m === 'standard' || m === 'certificate' || m === 'report' || m === 'letterhead' || m === 'memo') mode = m;
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

  // German / English string table for the certificate template.
  // Keep keys descriptive and short; the structure mirrors the cert
  // body in render order so a translator can verify coverage by
  // reading top-to-bottom.
  const CERT_STRINGS = {
    en: {
      issuer: 'Granit · Document',
      eyebrow: 'This document is issued under',
      titleSuffix: '',
      bodyEyebrow: 'Subject',
      issuedOn: 'Issued on',
      signedBy: 'Signed by',
      noSigner: 'Unsigned',
      authenticityLabel: 'Authenticity Stamp',
      sigGenerated: 'Signed at',
      sigSource: 'Source path',
      sigLength: 'Content length',
      sigAlgo: 'Hash algorithm',
      sigTool: 'Tool',
      verifyHeading: 'To verify integrity',
      verifyBody: 'copy the body of this document into a file, run',
      verifyLinuxLabel: '(Linux / macOS)',
      verifyOr: 'or',
      verifyWindowsLabel: '(Windows)',
      verifyMatch: 'and compare the output to the fingerprint above. A match proves the content has not changed since this signature was applied at',
      verifyChange: 'A single character difference produces an entirely different hash.',
      disclaimer: 'This is a content-integrity stamp, not a legally binding e-signature. The hash proves the document hasn\'t been altered since generation; the signer field is a self-attested claim with no third-party verification authority.',
      lengthWords: 'words',
      lengthChars: 'characters',
      lengthLines: 'lines'
    },
    de: {
      issuer: 'Granit · Urkunde',
      eyebrow: 'Diese Urkunde wird ausgestellt unter',
      titleSuffix: '',
      bodyEyebrow: 'Gegenstand',
      issuedOn: 'Ausgestellt am',
      signedBy: 'Unterzeichnet von',
      noSigner: 'Ohne Unterzeichner',
      authenticityLabel: 'Echtheitsstempel',
      sigGenerated: 'Unterzeichnet am',
      sigSource: 'Quelldatei',
      sigLength: 'Inhaltsumfang',
      sigAlgo: 'Hash-Algorithmus',
      sigTool: 'Werkzeug',
      verifyHeading: 'Zur Echtheitsprüfung',
      verifyBody: 'Kopieren Sie den Dokumenttext in eine Datei und führen Sie aus:',
      verifyLinuxLabel: '(Linux / macOS)',
      verifyOr: 'oder',
      verifyWindowsLabel: '(Windows)',
      verifyMatch: 'und vergleichen Sie die Ausgabe mit dem oben angegebenen Fingerabdruck. Stimmen sie überein, ist der Inhalt unverändert seit der Unterzeichnung am',
      verifyChange: 'Bereits ein einzelnes verändertes Zeichen erzeugt einen vollständig anderen Hash.',
      disclaimer: 'Dies ist ein Echtheitsstempel des Inhalts und keine rechtsverbindliche elektronische Signatur. Der Hash beweist, dass das Dokument seit der Erstellung nicht verändert wurde; das Unterzeichnerfeld ist eine eigene Erklärung ohne externe Verifizierungsstelle.',
      lengthWords: 'Wörter',
      lengthChars: 'Zeichen',
      lengthLines: 'Zeilen'
    }
  } as const;
  let str = $derived(CERT_STRINGS[certLang]);

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
  async function computeHash(text: string): Promise<string> {
    if (typeof crypto === 'undefined' || !crypto.subtle) return '';
    const enc = new TextEncoder().encode(text);
    const buf = await crypto.subtle.digest('SHA-256', enc);
    return Array.from(new Uint8Array(buf))
      .map((b) => b.toString(16).padStart(2, '0'))
      .join('');
  }
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

  // Word + char counts for the signature block — small but
  // useful integrity datapoints alongside the hash.
  let docWords = $derived(body.trim() ? body.trim().split(/\s+/).length : 0);
  let docChars = $derived(body.length);
  let docLines = $derived(body ? body.split('\n').length : 0);

  // User agent + locale captured at sign time — small extra
  // provenance datapoints. Not load-bearing, but they help a reader
  // understand the context the signature was generated in.
  let signedFrom = $derived.by(() => {
    if (typeof navigator === 'undefined') return '';
    const ua = navigator.userAgent;
    if (/Firefox\//.test(ua)) return 'Firefox';
    if (/Edg\//.test(ua)) return 'Edge';
    if (/Chrome\//.test(ua)) return 'Chrome';
    if (/Safari\//.test(ua)) return 'Safari';
    return 'Browser';
  });

  function fmtTimestamp(iso: string): string {
    if (!iso) return '';
    try {
      // Locale follows the certificate-language toggle so a German
      // certificate gets a German timestamp ("7. Mai 2026, 14:32:18")
      // and the default English locale uses "May 7, 2026, 14:32:18".
      // hour12:false forces 24-hour format regardless of OS locale —
      // user explicitly asked for 24h on the signature footer.
      const locale = certLang === 'de' ? 'de-AT' : undefined;
      return new Date(iso).toLocaleString(locale, {
        year: 'numeric', month: 'short', day: 'numeric',
        hour: '2-digit', minute: '2-digit', second: '2-digit',
        hour12: false
      });
    } catch {
      return iso;
    }
  }
  function shortHash(h: string): string {
    if (!h) return '…';
    // Display as four 8-char groups for readability — same shape
    // signed-PDF viewers use for fingerprint summaries. The full
    // hash is still in the title attribute for copy-paste.
    return `${h.slice(0, 8)} ${h.slice(8, 16)} … ${h.slice(-16, -8)} ${h.slice(-8)}`;
  }

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
    // Use the certificate language ONLY for certificate mode;
    // the other templates stay in the user's OS locale because
    // memos / letterheads are not language-tagged in the UI.
    const locale = mode === 'certificate' && certLang === 'de' ? 'de-AT' : undefined;
    return new Date().toLocaleDateString(locale, {
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
      <!-- Sign-document toggle. Independent from the mode selector
           because signing is an additive concern: any of the four
           templates can carry a signature footer. The visual state
           is a chip — pressed = on. Available on every viewport
           because it's a structural document property, not a tweak. -->
      <button
        onclick={() => {
          signatureOn = !signatureOn;
          // Auto-open the configure panel the first time the user
          // turns on signing, so the Signer field is immediately
          // visible. Without this, users would toggle 🔏 on, see
          // "No signer specified" in the rendered footer, and have
          // no clue where to set it.
          if (signatureOn) configOpen = true;
        }}
        class="tb-btn {signatureOn ? 'tb-active' : ''}"
        title={signatureOn ? 'Document is signed (SHA-256 footer added)' : 'Add signature footer (SHA-256 + timestamp)'}
      >🔏 {signatureOn ? 'Signed' : 'Sign'}</button>
      {#if signatureOn}
        <!-- Inline signer field. Lives in the toolbar (not buried in
             the configure panel) when signing is on, because "who
             signed this" is the single most-important field of a
             signature — it deserves immediate, one-click access.
             Mirrors the same `signer` state as the configure-panel
             input below; both edit the same value. -->
        <input
          bind:value={signer}
          placeholder="Signer name (e.g. Raphael Lugmayr)"
          class="tb-signer-input"
          aria-label="Signer name"
        />
      {/if}
      <span class="tb-sep"></span>
      <div class="tb-modes">
        {#each [
          { id: 'standard', label: 'Standard' },
          { id: 'letterhead', label: 'Letterhead' },
          { id: 'memo', label: 'Memo' },
          { id: 'report', label: 'Report' },
          { id: 'certificate', label: 'Certificate' }
        ] as m}
          <button
            onclick={() => (mode = m.id as Mode)}
            class="tb-mode {mode === m.id ? 'tb-active' : ''}"
          >{m.label}</button>
        {/each}
      </div>
      {#if mode === 'certificate'}
        <!-- Certificate-only options: variant (size/density) and
             language (string table). Both surface only when the
             certificate template is selected, so the toolbar stays
             clean for other templates. Persisted to localStorage. -->
        <span class="tb-sep"></span>
        <div class="tb-modes" role="radiogroup" aria-label="Certificate variant">
          {#each [
            { id: 'standard', label: 'Standard', title: 'Full A4 certificate' },
            { id: 'compact',  label: 'Compact',  title: 'Half-page, denser body' }
          ] as v}
            <button
              onclick={() => (certVariant = v.id as CertVariant)}
              class="tb-mode {certVariant === v.id ? 'tb-active' : ''}"
              title={v.title}
            >{v.label}</button>
          {/each}
        </div>
        <div class="tb-modes" role="radiogroup" aria-label="Certificate language">
          {#each [
            { id: 'en', label: 'EN', title: 'English certificate' },
            { id: 'de', label: 'DE', title: 'Deutsch' }
          ] as l}
            <button
              onclick={() => (certLang = l.id as CertLang)}
              class="tb-mode {certLang === l.id ? 'tb-active' : ''}"
              title={l.title}
            >{l.label}</button>
          {/each}
        </div>
      {/if}
      <span class="tb-spacer"></span>
      <button onclick={doPrint} class="tb-btn tb-primary" title="Print (⌘P)">🖨 Print / Save as PDF</button>
    </header>

    {#if configOpen}
      <section class="config-panel">
        <div class="config-row">
          <label for="print-header">Header</label>
          {#if mode === 'letterhead'}
            <textarea
              id="print-header"
              bind:value={header}
              placeholder={`Acme Corp\n123 Main St\nVienna, AT`}
              class="config-input config-textarea"
              rows="3"
            ></textarea>
          {:else}
            <input
              id="print-header"
              bind:value={header}
              placeholder={mode === 'memo' ? 'Your name (FROM)' : 'e.g. ACME Corp — Internal'}
              class="config-input"
            />
          {/if}
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
        <!-- Signature claim fields. Only shown when the Sign toggle
             is on so the config panel doesn't fill with irrelevant
             options for users who never sign documents. Both are
             optional — a signature with neither still carries the
             SHA-256 / timestamp / source claim, which is the actual
             integrity stamp; signer / purpose are human-readable
             attestations on top. -->
        {#if signatureOn}
          <div class="config-row">
            <label for="print-signer">Signer</label>
            <input
              id="print-signer"
              bind:value={signer}
              placeholder="e.g. Raphael Lugmayr"
              class="config-input"
            />
          </div>
          <div class="config-row">
            <label for="print-purpose">Purpose</label>
            <input
              id="print-purpose"
              bind:value={purpose}
              placeholder="e.g. Q3 review · Internal use only"
              class="config-input"
            />
          </div>
        {/if}
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
    <main class="print-page" data-mode={mode} data-cert-variant={certVariant} data-cert-lang={certLang}>
      {#if mode === 'certificate'}
        <!-- Formal certificate template. The cert-frame is a real
             ornamented certificate frame with a double border and
             corner glyphs (kept simple — no external fonts, all
             inline SVG, so it survives PDF export and any CSP).
             Two sub-variants:
               • standard: full-A4, generous margins, large title
               • compact:  half-page, tighter margins, smaller title
             Two languages (en / de) swap the eyebrow / labels and
             pick the locale for the date format. The compact +
             German combo prints a small German Urkunde with the
             signature footer underneath. -->
        <div class="cert-frame">
          <span class="cert-corner cert-corner-tl">❖</span>
          <span class="cert-corner cert-corner-tr">❖</span>
          <span class="cert-corner cert-corner-bl">❖</span>
          <span class="cert-corner cert-corner-br">❖</span>
          <div class="cert-content">
            <div class="cert-issuer">{str.issuer}</div>
            <div class="cert-divider cert-divider--top">
              <span class="cert-divider-line"></span>
              <span class="cert-divider-mark">❦</span>
              <span class="cert-divider-line"></span>
            </div>
            <div class="cert-eyebrow">{str.eyebrow}</div>
            <h1 class="cert-title">{title}</h1>
            <div class="cert-divider">
              <span class="cert-divider-line"></span>
              <span class="cert-divider-mark">✦</span>
              <span class="cert-divider-line"></span>
            </div>
            <article class="cert-body">
              <MarkdownRenderer body={body} />
            </article>
            <div class="cert-meta">
              <div class="cert-meta-row">
                <span class="cert-meta-label">{str.issuedOn}</span>
                <span class="cert-meta-value">{todayHuman()}</span>
              </div>
              <div class="cert-meta-row">
                <span class="cert-meta-label">{str.signedBy}</span>
                <span class="cert-meta-value cert-signer-line">
                  {#if signer}{signer}{:else}<em class="cert-signer-empty">{str.noSigner}</em>{/if}
                </span>
              </div>
              {#if header}
                <div class="cert-meta-row">
                  <span class="cert-meta-value cert-meta-header">{header}</span>
                </div>
              {/if}
            </div>
          </div>
        </div>
      {:else if mode === 'letterhead'}
        <!-- Letterhead — formal corporate document. Header band at
             the top with sender block, narrow body for readability,
             contact strip footer. The sender block parses the
             configured header field as multi-line so a user can put
             "Acme Corp\n123 Main St\nVienna, AT" in the header
             input and have it render naturally. -->
        <header class="lh-header">
          <div class="lh-sender">
            {#each (header || 'Your Letterhead Here').split('\n') as line, i}
              <div class:lh-sender-name={i === 0}>{line}</div>
            {/each}
          </div>
          <div class="lh-rule"></div>
        </header>
        <article class="print-body lh-body">
          <div class="lh-date">{todayHuman()}</div>
          <h1 class="lh-title">{title}</h1>
          <MarkdownRenderer body={body} />
        </article>
        <footer class="lh-footer">
          <div class="lh-footer-rule"></div>
          <div class="lh-footer-text">
            {footer || `${sourcePath}  ·  Generated with Granit`}
          </div>
        </footer>
      {:else if mode === 'memo'}
        <!-- Memo — interoffice format. The classic TO/FROM/DATE/RE
             block at the top, plain body. Header field doubles as
             the FROM line; user can override via standard markdown
             at the top of the note. -->
        <header class="memo-header">
          <div class="memo-eyebrow">Memorandum</div>
          <table class="memo-meta">
            <tbody>
              <tr><th>To:</th><td>—</td></tr>
              <tr><th>From:</th><td>{header || '—'}</td></tr>
              <tr><th>Date:</th><td>{todayHuman()}</td></tr>
              <tr><th>Re:</th><td>{title}</td></tr>
            </tbody>
          </table>
        </header>
        <article class="print-body memo-body">
          <MarkdownRenderer body={body} />
        </article>
        <footer class="print-footer">
          <div class="print-footer-text">
            {footer || `${sourcePath}  ·  Generated with Granit`}
          </div>
        </footer>
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

      <!-- Document signature footer — the PDF-grade integrity stamp
           the user actually asked for. Toggleable on every mode
           via the toolbar's Sign button. Renders SHA-256 of the
           body, a generated-at timestamp, the source path, the
           Granit seal, and a small provenance line linking the
           open-source repo. Looks like the signature panel a
           PDF viewer surfaces for a signed document. -->
      {#if signatureOn}
        <aside class="doc-signature" aria-label="Document signature">
          <div class="doc-signature__inner">
            <div class="doc-signature__seal">
              <svg viewBox="0 0 120 120" width="64" height="64" aria-hidden="true">
                <defs>
                  <path id="sig-arc-top" d="M 60 60 m -44 0 a 44 44 0 0 1 88 0" fill="none"/>
                  <path id="sig-arc-bot" d="M 60 60 m 44 0 a 44 44 0 0 1 -88 0" fill="none"/>
                </defs>
                <circle cx="60" cy="60" r="54" fill="none" stroke="#5a7088" stroke-width="1.5"/>
                <circle cx="60" cy="60" r="48" fill="none" stroke="#5a7088" stroke-width="0.6"/>
                <circle cx="60" cy="60" r="30" fill="none" stroke="#5a7088" stroke-width="1"/>
                <text font-family="Georgia, serif" font-size="7" letter-spacing="1.5" fill="#5a7088">
                  <textPath href="#sig-arc-top" startOffset="50%" text-anchor="middle">GENERATED · WITH · GRANIT</textPath>
                </text>
                <text font-family="Georgia, serif" font-size="5" letter-spacing="0.8" fill="#5a7088">
                  <textPath href="#sig-arc-bot" startOffset="50%" text-anchor="middle">github.com/artaeon/granit</textPath>
                </text>
                <text x="60" y="56" font-family="Georgia, serif" font-size="14" font-weight="700" text-anchor="middle" fill="#5a7088" letter-spacing="2">G</text>
                <text x="60" y="68" font-family="Georgia, serif" font-size="5" letter-spacing="1.5" text-anchor="middle" fill="#5a7088">GRANIT</text>
              </svg>
            </div>
            <div class="doc-signature__body">
              <div class="doc-signature__head">
                <div class="doc-signature__eyebrow">{str.authenticityLabel}</div>
                <div class="doc-signature__signer-row">
                  {#if signer}
                    <span class="doc-signature__signer-label">{str.signedBy}</span>
                    <span class="doc-signature__signer-name">{signer}</span>
                  {:else}
                    <span class="doc-signature__signer-name doc-signature__signer-none">{str.noSigner}</span>
                  {/if}
                </div>
                <div class="doc-signature__docid">
                  Document ID <span class="doc-signature__docid-value">{docID}</span>
                </div>
              </div>
              <div class="doc-signature__lead">
                {#if purpose}<strong>{purpose}.</strong>{' '}{/if}{certLang === 'de'
                  ? 'Dieses Dokument wurde mit Granit (Open-Source-Notiz- und Wissenswerkzeug) erzeugt. Der unten angegebene kryptographische Fingerabdruck wird über den Inhalt berechnet und ändert sich, sobald ein einziges Zeichen verändert wird.'
                  : 'This document was generated through Granit (open-source notes & knowledge tool). The cryptographic fingerprint below is computed over the body and changes the moment any character is altered.'}
              </div>
              <div class="doc-signature__hashbox" title="SHA-256" role="presentation">
                <div class="doc-signature__hashlabel">SHA-256 {certLang === 'de' ? 'Fingerabdruck' : 'fingerprint'}</div>
                <div class="doc-signature__hashvalue">{signatureHash || '…'}</div>
              </div>
              <dl class="doc-signature__fields">
                <dt>{str.sigGenerated}</dt>
                <dd>{fmtTimestamp(signatureTimestamp)}</dd>
                <dt>{str.sigSource}</dt>
                <dd class="doc-signature__src">{sourcePath}</dd>
                <dt>{str.sigLength}</dt>
                <dd>{docWords} {str.lengthWords} · {docChars} {str.lengthChars} · {docLines} {str.lengthLines}</dd>
                <dt>{str.sigAlgo}</dt>
                <dd>SHA-256 (FIPS 180-4) · 32 bytes / 256 bits</dd>
                <dt>{certLang === 'de' ? 'Erstellt mit' : 'Signed from'}</dt>
                <dd>{signedFrom}</dd>
                <dt>{str.sigTool}</dt>
                <dd>
                  Granit ·
                  {certLang === 'de' ? 'quelloffen' : 'open-source'} ·
                  <a href="https://github.com/artaeon/granit">github.com/artaeon/granit</a>
                </dd>
              </dl>
              <p class="doc-signature__note">
                <strong>{str.verifyHeading}:</strong>
                {str.verifyBody}
                <code>sha256sum &lt;file&gt;</code> {str.verifyLinuxLabel}
                {str.verifyOr}
                <code>certutil -hashfile &lt;file&gt; SHA256</code> {str.verifyWindowsLabel},
                {str.verifyMatch} <strong>{fmtTimestamp(signatureTimestamp)}</strong>.
                {str.verifyChange}
              </p>
              <p class="doc-signature__disclaimer">
                <em>{str.disclaimer}</em>
              </p>
            </div>
          </div>
        </aside>
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
  .tb-signer-input {
    padding: 0.25rem 0.6rem;
    border: 1px solid var(--color-primary);
    border-radius: 0.25rem;
    background: var(--color-base);
    color: var(--color-text);
    font-size: 0.85rem;
    min-width: 16rem;
    outline: none;
  }
  .tb-signer-input::placeholder { color: var(--color-dim); }
  .tb-signer-input:focus { box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 30%, transparent); }
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
  .config-textarea {
    font-family: inherit;
    resize: vertical;
    min-height: 4.5rem;
    line-height: 1.4;
  }
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
    padding: 2.5cm 2cm;
    line-height: 1.65;
    color: #1a1a1a;
  }
  .formal-body {
    /* Slightly tighter measure than the screen body — printed
       formal documents read better in narrower columns. */
    max-width: 16.5cm;
    margin: 0 auto;
  }
  .formal-title {
    font-size: 22pt;
    font-weight: 700;
    margin: 0.5rem 0 0.25rem;
    color: #1a1a1a;
    letter-spacing: 0.005em;
  }
  .formal-meta {
    font-size: 9.5pt;
    color: #666;
    margin-bottom: 1.5rem;
    border-bottom: 1px solid #c8c8c8;
    padding-bottom: 0.6rem;
  }

  .print-page[data-mode="letterhead"] {
    font-family: Georgia, 'Iowan Old Style', 'Times New Roman', serif;
    font-size: 11pt;
    padding: 1.8cm 2cm;
    line-height: 1.6;
  }
  .print-page[data-mode="memo"] {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
    font-size: 11pt;
    padding: 2cm 2cm;
    line-height: 1.55;
  }

  /* ----- Letterhead template ----- */
  /* Top sender block (multi-line) → narrow body → contact strip
     footer. Mimics the corporate-letter feel of a printed cover
     sheet. The sender block accepts newlines in the header field
     so the user gets a 3-line address without resorting to HTML. */
  .lh-header {
    margin-bottom: 1.5rem;
  }
  .lh-sender {
    font-size: 10pt;
    color: #2a2a2a;
    line-height: 1.4;
  }
  .lh-sender-name {
    font-size: 16pt !important;
    font-weight: 600;
    letter-spacing: 0.02em;
    color: #1a1a1a;
    margin-bottom: 0.25rem;
  }
  .lh-rule {
    margin-top: 0.6rem;
    height: 2px;
    background: linear-gradient(to right, #1a1a1a 0%, #1a1a1a 60%, transparent 100%);
  }
  .lh-body {
    margin-top: 0;
  }
  .lh-date {
    font-size: 10pt;
    color: #555;
    margin-bottom: 1.2rem;
  }
  .lh-title {
    font-size: 18pt;
    font-weight: 600;
    margin: 0 0 1rem;
    color: #1a1a1a;
    line-height: 1.25;
  }
  .lh-footer {
    margin-top: 2rem;
    text-align: center;
  }
  .lh-footer-rule {
    height: 1px;
    background: #c0c0c0;
    margin-bottom: 0.5rem;
  }
  .lh-footer-text {
    font-size: 9pt;
    color: #666;
    letter-spacing: 0.02em;
  }

  /* ----- Memo template ----- */
  /* Classic interoffice memo: large "MEMORANDUM" eyebrow, four-row
     metadata table (TO/FROM/DATE/RE), then body. The labels are
     bold + right-aligned in a fixed-width column for that
     unmistakable "office memo" feel. */
  .memo-header {
    margin-bottom: 1.5rem;
    padding-bottom: 0.8rem;
    border-bottom: 2px solid #1a1a1a;
  }
  .memo-eyebrow {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
    font-size: 22pt;
    font-weight: 700;
    letter-spacing: 0.18em;
    text-transform: uppercase;
    color: #1a1a1a;
    margin-bottom: 1rem;
  }
  .memo-meta {
    border-collapse: collapse;
    width: 100%;
  }
  .memo-meta th {
    text-align: left;
    font-weight: 700;
    width: 4.5rem;
    padding: 0.15rem 0.5rem 0.15rem 0;
    color: #1a1a1a;
    font-size: 10.5pt;
    vertical-align: top;
  }
  .memo-meta td {
    padding: 0.15rem 0;
    color: #1a1a1a;
    font-size: 10.5pt;
    vertical-align: top;
  }
  .memo-body {
    /* Memo body is plain — the header band carries all the
       formal weight, body is conversational. */
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
    /* The triple inset gives the frame a "engraved" look without
       any external image — outer cream stripe, golden hairline,
       inner cream stripe. Reads as a real certificate border on
       printer output. */
    box-shadow:
      inset 0 0 0 2px #fbf7ee,
      inset 0 0 0 3px #c9a55c,
      inset 0 0 0 5px #fbf7ee;
    background: #fbf7ee;
    min-height: calc(29.7cm - 2.4cm);
    display: flex;
    flex-direction: column;
  }
  /* Compact variant: half-page footprint, denser body text. Picked
     for the case the user described — when the standard A4 frame
     is too much for a short Urkunde / certificate. The frame still
     uses A4 paper; the cert just doesn't fill the entire page. */
  .print-page[data-cert-variant="compact"] .cert-frame {
    margin: 0.8cm;
    padding: 1cm 1.2cm 1cm;
    min-height: 14cm;
  }
  .cert-corner {
    position: absolute;
    color: #8a6d3b;
    font-size: 18pt;
    line-height: 1;
  }
  .print-page[data-cert-variant="compact"] .cert-corner { font-size: 13pt; }
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
    font-size: 9.5pt;
    letter-spacing: 0.4em;
    text-transform: uppercase;
    color: #8a6d3b;
    font-weight: 700;
    margin-bottom: 0.6rem;
  }
  .print-page[data-cert-variant="compact"] .cert-issuer {
    font-size: 8.5pt;
    margin-bottom: 0.4rem;
  }
  .cert-eyebrow {
    font-style: italic;
    font-size: 11pt;
    color: #8a6d3b;
    margin: 0.8rem 0 0.4rem;
    letter-spacing: 0.05em;
  }
  .print-page[data-cert-variant="compact"] .cert-eyebrow {
    font-size: 9pt;
    margin: 0.4rem 0 0.2rem;
  }
  .cert-title {
    font-family: 'Iowan Old Style', Georgia, 'Times New Roman', serif;
    font-size: 30pt;
    font-weight: 700;
    margin: 0.4rem 0 0.6rem;
    color: #2a2419;
    letter-spacing: 0.012em;
    line-height: 1.18;
  }
  .print-page[data-cert-variant="compact"] .cert-title {
    font-size: 18pt;
    margin: 0.2rem 0 0.4rem;
  }
  .cert-divider {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    margin: 0.5rem 0 1.2rem;
    color: #8a6d3b;
  }
  .cert-divider--top {
    margin: 0.4rem 0 0.6rem;
  }
  .print-page[data-cert-variant="compact"] .cert-divider {
    margin: 0.3rem 0 0.6rem;
  }
  .cert-divider-line {
    flex: 0 1 25%;
    height: 1px;
    background: linear-gradient(to right, transparent, #8a6d3b, transparent);
  }
  .cert-divider-mark {
    font-size: 14pt;
  }
  .print-page[data-cert-variant="compact"] .cert-divider-mark {
    font-size: 11pt;
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
  .print-page[data-cert-variant="compact"] .cert-body {
    font-size: 10pt;
    line-height: 1.5;
    max-width: 14cm;
  }
  /* Meta block at the bottom of the cert: Issued on / Signed by /
     Header. Two-column-ish layout with a label / value alignment
     that reads like the back of a real certificate. */
  .cert-meta {
    margin-top: 1.5rem;
    padding-top: 1rem;
    border-top: 1px solid #c9a55c;
    display: grid;
    grid-template-columns: max-content 1fr;
    column-gap: 1rem;
    row-gap: 0.4rem;
    font-size: 10pt;
    color: #2a2419;
    text-align: left;
  }
  .print-page[data-cert-variant="compact"] .cert-meta {
    margin-top: 0.8rem;
    padding-top: 0.5rem;
    font-size: 8.5pt;
    row-gap: 0.2rem;
  }
  .cert-meta-row {
    display: contents;
  }
  .cert-meta-label {
    font-size: 8.5pt;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: #8a6d3b;
    font-weight: 700;
    align-self: baseline;
  }
  .cert-meta-value {
    color: #2a2419;
    font-family: Georgia, 'Iowan Old Style', serif;
  }
  .cert-meta-header {
    font-style: italic;
    color: #5a4a30;
    grid-column: 1 / -1;
    text-align: center;
    border-top: 1px dashed #c9a55c;
    padding-top: 0.4rem;
    margin-top: 0.2rem;
  }
  .cert-signer-line {
    border-bottom: 1px solid #2a2419;
    padding-bottom: 1px;
    min-width: 4cm;
    display: inline-block;
  }
  .cert-signer-empty {
    color: #8a6d3b;
    font-style: italic;
    border-bottom: none;
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

  /* ----- Document Signature footer ----- */
  /* Styled like the integrity panel a PDF reader (Acrobat,
     Preview) shows for a signed document — not gaudy, not
     decorative, just a clean evidentiary block: the round seal on
     the left, hash + timestamp + provenance on the right, a
     subtle border + tonal background that says "this is a
     control surface, not body content". Stays attached to the
     bottom of the printed page; renders crisply at any zoom
     because the seal is inline SVG. */
  .doc-signature {
    margin-top: 1.5cm;
    page-break-inside: avoid;
    break-inside: avoid;
  }
  .doc-signature__inner {
    display: flex;
    gap: 1rem;
    padding: 0.85rem 1rem;
    border: 1px solid #b9c4d0;
    border-radius: 0.25rem;
    background: #f5f8fb;
    color: #2a3340;
  }
  .doc-signature__seal {
    flex-shrink: 0;
    align-self: center;
  }
  .doc-signature__body {
    flex: 1;
    min-width: 0;
  }
  /* New layout: head is a vertical stack — eyebrow, signer row,
     docID. The previous version was a single horizontal flex row
     where the signer was buried in the dl below; the new layout
     promotes the signer to a prominent, scannable header line. */
  .doc-signature__head {
    margin-bottom: 0.6rem;
    padding-bottom: 0.5rem;
    border-bottom: 1px solid #b9c4d0;
  }
  .doc-signature__eyebrow {
    font-size: 7.5pt;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.15em;
    color: #8090a4;
    margin-bottom: 0.3rem;
  }
  .doc-signature__signer-row {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    margin-bottom: 0.25rem;
  }
  .doc-signature__signer-label {
    font-size: 8pt;
    color: #5a7088;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-weight: 600;
    flex-shrink: 0;
  }
  .doc-signature__signer-name {
    font-size: 13pt;
    font-weight: 700;
    color: #1a1a1a;
    letter-spacing: 0.01em;
    font-family: Georgia, 'Times New Roman', serif;
  }
  .doc-signature__signer-none {
    font-style: italic;
    color: #8090a4;
    font-weight: 500;
    font-size: 11pt;
  }
  .doc-signature__docid {
    font-size: 8pt;
    color: #5a7088;
    font-variant-numeric: tabular-nums;
  }
  .doc-signature__docid-value {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-weight: 700;
    color: #1a4fb3;
    letter-spacing: 0.04em;
  }
  /* Hash callout — full SHA-256 on its own row, monospace, with a
     subtle ruled border so it reads as the load-bearing claim
     rather than just another row in the metadata table. */
  .doc-signature__hashbox {
    margin: 0.5rem 0;
    padding: 0.4rem 0.6rem;
    background: #ffffff;
    border: 1px solid #c5d1de;
    border-radius: 0.2rem;
  }
  .doc-signature__hashlabel {
    font-size: 7.5pt;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: #5a7088;
    margin-bottom: 0.15rem;
  }
  .doc-signature__hashvalue {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 8pt;
    color: #1a4fb3;
    word-break: break-all;
    letter-spacing: 0.02em;
    line-height: 1.4;
  }
  .doc-signature__signer {
    font-weight: 600;
    color: #1a1a1a !important;
  }
  .doc-signature__lead {
    font-size: 8.5pt;
    line-height: 1.5;
    color: #44546a;
    margin-bottom: 0.6rem;
  }
  .doc-signature__fields {
    display: grid;
    grid-template-columns: 5.5rem 1fr;
    column-gap: 0.75rem;
    row-gap: 0.15rem;
    margin: 0 0 0.6rem;
    font-size: 8.5pt;
    color: #2a3340;
  }
  .doc-signature__fields dt {
    color: #5a7088;
    font-weight: 600;
    font-size: 8pt;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .doc-signature__fields dd {
    margin: 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    overflow-wrap: anywhere;
  }
  .doc-signature__hash {
    /* Ample letter-spacing for the grouped-hash readout, like a
       PDF viewer's fingerprint summary. */
    letter-spacing: 0.06em;
    color: #1a4fb3;
  }
  .doc-signature__src {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  .doc-signature__note {
    font-size: 7.5pt;
    line-height: 1.5;
    color: #5a7088;
    margin: 0;
    padding-top: 0.5rem;
    border-top: 1px dashed #c5d1de;
  }
  /* Disclaimer is the smallest, mutest piece — sets reader
     expectations about the integrity-vs-legal-signature distinction
     without competing with the verification instructions above. */
  .doc-signature__disclaimer {
    font-size: 7pt;
    line-height: 1.5;
    color: #8090a4;
    margin: 0.4rem 0 0;
    padding-top: 0.4rem;
  }
  .doc-signature__body a {
    color: #1a4fb3;
    text-decoration: none;
  }

  /* Long-content print pagination. The previous version printed
     only the first page because:
       1. .print-page was display:flex (column) — flex containers
          handle overflow oddly under print, and the browser may
          not break flex items across pages cleanly.
       2. .print-overlay was position:fixed in screen mode, which
          some browsers carry into print — fixed elements typically
          render only on the first page per the CSS spec.
     The @media print block below resets both: the overlay becomes
     position:static, the .print-page becomes display:block, and
     content flows naturally past the page boundary so the printer
     auto-paginates. page-break-inside:avoid on the doc-signature
     keeps the integrity stamp from being split across pages. */

  /* THE actual print rules. We hide the overlay's chrome (toolbar,
     config panel, page shadow), reset margins so the OS @page
     handles them, and let the .print-page content flow at native
     dimensions onto the printer. */
  @media print {
    :global(html), :global(body) {
      background: white !important;
      margin: 0 !important;
      padding: 0 !important;
      height: auto !important;
      overflow: visible !important;
    }
    /* Why this isolation pattern: SvelteKit wraps everything in
       <div style="display: contents">%sveltekit.body%</div> in
       app.html (see /web/src/app.html). The previous rule used
       `body > *:not(.print-overlay) { display: none }` — but
       .print-overlay is NOT a direct child of body; it's nested
       inside the SvelteKit wrapper. So the rule hid the WRAPPER,
       which hid everything including the print-overlay, and Chrome's
       print engine fell back to whatever it could render on page 1.

       The visibility-based recipe below works regardless of
       ancestor structure: hide every descendant of body, then make
       the overlay + its subtree visible again, then reposition the
       overlay to the document origin so it doesn't render after
       the hidden-but-still-laid-out screen content. */
    :global(body *) { visibility: hidden !important; }
    :global(.print-overlay), :global(.print-overlay *) {
      visibility: visible !important;
    }
    /* position:absolute (NOT fixed) because position:fixed blocks
       only render on the first page in print spec, while absolute-
       positioned subtrees DO paginate in modern Chrome / Firefox.
       This is the load-bearing line for multi-page output. */
    .print-overlay {
      position: absolute !important;
      left: 0 !important;
      top: 0 !important;
      right: 0 !important;
      width: 100% !important;
      height: auto !important;
      overflow: visible !important;
      display: block !important;
      background: white !important;
      z-index: 0 !important;
    }
    .print-toolbar, .config-panel { display: none !important; }
    .print-page {
      width: 100% !important;
      min-height: 0 !important;
      max-height: none !important;
      height: auto !important;
      margin: 0 !important;
      padding: 0 !important;
      box-shadow: none !important;
      display: block !important;
      overflow: visible !important;
    }
    /* CRITICAL for multi-page output: every flex / grid container
       inside the printed page must be reset to plain block layout,
       and every fixed min-height / max-height removed. Flex chains
       are what was breaking pagination — a `flex: 1` body inside a
       `display: flex; min-height: 29.7cm` parent keeps content
       visually constrained to one viewport-page even when the
       printer would happily paginate a block.
       The cert-frame / cert-content / cert-body chain was the
       worst offender (cert was the user's primary failure mode);
       all four templates have similar issues. We reset them
       wholesale. */
    .cert-frame, .cert-content, .cert-body,
    .formal-body, .lh-body, .lh-header, .memo-body, .memo-header,
    .print-body, .doc-signature, .doc-signature__inner {
      display: block !important;
      flex: none !important;
      min-height: 0 !important;
      max-height: none !important;
      height: auto !important;
      overflow: visible !important;
    }
    .formal-body, .lh-body, .memo-body, .cert-body, .print-body {
      max-width: none !important;
      margin: 0 !important;
    }
    /* The cert-frame has a decorative border + corner ornaments. In
       print we keep the border (it renders on every page break,
       which is fine — actually looks coherent), but we must NOT
       carry the inset double-border + min-height from screen mode
       or content can't break. Corners are positioned absolutely so
       they pin to the first page only — that's accepted; trying to
       repeat them on every page would require page-counting. */
    .cert-frame {
      margin: 0 !important;
      padding: 1.2cm 1cm !important;
      box-shadow: inset 0 0 0 1px #fff, inset 0 0 0 2px #d4b87a !important;
    }
    /* Tighten orphan/widow control so a heading's content doesn't
       get stranded at the bottom of one page with its subtext on
       the next. Most browsers honor these defaults but explicit is
       safer for print. */
    :global(.print-page p),
    :global(.print-page li),
    :global(.print-page blockquote) {
      orphans: 3;
      widows: 3;
    }
    /* Keep code blocks and tables intact when possible — splitting
       a code fence mid-line is unreadable; splitting a table mid-
       row strands the header from the body. */
    :global(.print-page pre),
    :global(.print-page table) {
      page-break-inside: avoid;
      break-inside: avoid;
    }
    /* Document signature already has page-break-inside:avoid in
       its own rule, which is the load-bearing one — the
       authenticity stamp must NEVER be split across pages. */
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
