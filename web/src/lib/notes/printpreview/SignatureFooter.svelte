<script lang="ts">
  // The integrity-stamp footer appended to the rendered document
  // when the user toggles "Sign". Two variants:
  //   • compact  — single-row trust stamp + hash, fits at the foot
  //                of any A4 page without pushing content over.
  //   • standard — full panel with verification commands and a
  //                disclaimer block.
  //
  // Styling lives in PrintPreview.svelte's <style> block (.doc-signature
  // selectors) so the print-CSS stays in one place. This component is
  // pure markup + i18n wiring.
  import type { CertLang, CertVariant, CertStrings } from './strings';
  import { fmtTimestamp } from './strings';

  let {
    variant,
    lang,
    str,
    signer,
    purpose,
    signatureHash,
    signatureTimestamp,
    docID,
    sourcePath,
    docWords,
    docChars,
    docLines
  }: {
    variant: CertVariant;
    lang: CertLang;
    str: CertStrings;
    signer: string;
    purpose: string;
    signatureHash: string;
    signatureTimestamp: string;
    docID: string;
    sourcePath: string;
    docWords: number;
    docChars: number;
    docLines: number;
  } = $props();
</script>

<aside class="doc-signature" aria-label={str.authenticityLabel}>
  {#if variant === 'compact'}
    <!-- Compact variant: a single-row trust stamp. The seal is
         small; the body is one line of trust text plus a tight
         2-row grid of meta. The hash sits on its own line so a
         verifier can copy it. -->
    <div class="doc-signature__compact">
      <svg viewBox="0 0 60 60" class="doc-signature__seal-sm" aria-hidden="true">
        <circle cx="30" cy="30" r="27" fill="none" stroke="#5a7088" stroke-width="1.2"/>
        <circle cx="30" cy="30" r="22" fill="none" stroke="#5a7088" stroke-width="0.4"/>
        <text x="30" y="36" font-family="Georgia, serif" font-size="20" font-weight="700" text-anchor="middle" fill="#5a7088">G</text>
      </svg>
      <div class="doc-signature__compact-body">
        <div class="doc-signature__compact-headline">
          <strong>
            {lang === 'de'
              ? 'Mit Granit erzeugt · Open-Source · keine Schadsoftware'
              : 'Generated with Granit · Open-source · No harmful software'}
          </strong>
          {#if signer}<span class="doc-signature__compact-signer"> · {str.signedBy} {signer}</span>{/if}
        </div>
        <div class="doc-signature__compact-meta">
          <span><strong>{str.sigGenerated}:</strong> {fmtTimestamp(signatureTimestamp, lang)}</span>
          <span class="doc-signature__compact-sep">·</span>
          <span>Doc <span class="doc-signature__docid-value">{docID}</span></span>
          <span class="doc-signature__compact-sep">·</span>
          <span>{lang === 'de' ? 'Werkzeug' : 'Tool'}: <a href="https://github.com/artaeon/granit">github.com/artaeon/granit</a></span>
        </div>
        <div class="doc-signature__compact-hash" title="Full SHA-256">
          SHA-256: <span>{signatureHash || '…'}</span>
        </div>
      </div>
    </div>
  {:else}
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
          {#if purpose}<strong>{purpose}.</strong>{' '}{/if}{lang === 'de'
            ? 'Dieses Dokument wurde mit Granit (Open-Source-Notiz- und Wissenswerkzeug, keine Schadsoftware) erzeugt. Der kryptographische Fingerabdruck wird über den Inhalt berechnet und ändert sich, sobald ein einziges Zeichen verändert wird.'
            : 'This document was generated with Granit (open-source notes & knowledge tool — no harmful software). The cryptographic fingerprint is computed over the content and changes the moment any character is altered.'}
        </div>
        <div class="doc-signature__hashbox" title="SHA-256" role="presentation">
          <div class="doc-signature__hashlabel">SHA-256 {lang === 'de' ? 'Fingerabdruck' : 'fingerprint'}</div>
          <div class="doc-signature__hashvalue">{signatureHash || '…'}</div>
        </div>
        <dl class="doc-signature__fields">
          <dt>{str.sigGenerated}</dt>
          <dd>{fmtTimestamp(signatureTimestamp, lang)}</dd>
          <dt>{str.sigSource}</dt>
          <dd class="doc-signature__src">{sourcePath}</dd>
          <dt>{str.sigLength}</dt>
          <dd>{docWords} {str.lengthWords} · {docChars} {str.lengthChars} · {docLines} {str.lengthLines}</dd>
          <dt>{str.sigAlgo}</dt>
          <dd>SHA-256 (FIPS 180-4)</dd>
          <dt>{str.sigTool}</dt>
          <dd>
            Granit ·
            {lang === 'de' ? 'quelloffen' : 'open-source'} ·
            <a href="https://github.com/artaeon/granit">github.com/artaeon/granit</a>
          </dd>
        </dl>
        <p class="doc-signature__note">
          <strong>{str.verifyHeading}:</strong>
          {str.verifyBody}
          <code>sha256sum &lt;file&gt;</code> {str.verifyLinuxLabel}
          {str.verifyOr}
          <code>certutil -hashfile &lt;file&gt; SHA256</code> {str.verifyWindowsLabel},
          {str.verifyMatch} <strong>{fmtTimestamp(signatureTimestamp, lang)}</strong>.
        </p>
        <p class="doc-signature__disclaimer">
          <em>{str.disclaimer}</em>
        </p>
      </div>
    </div>
  {/if}
</aside>
