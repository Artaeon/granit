<script lang="ts">
  // The printable paper surface. Switches between four templates
  // (standard / report / letterhead / memo) via the data-mode
  // attribute — actual layout differences are CSS, this component
  // just emits the right markup tree per mode.
  //
  // All styling lives in the parent PrintPreview.svelte's <style>
  // block (.print-page, .lh-*, .memo-*, .print-header etc.) so the
  // @media print rules stay centralised.
  import MarkdownRenderer from '../MarkdownRenderer.svelte';
  import SignatureFooter from './SignatureFooter.svelte';
  import type { Mode, CertLang, CertVariant, CertStrings } from './strings';
  import { todayHuman } from './strings';

  let {
    mode,
    title,
    body,
    sourcePath,
    header,
    footer,
    signatureOn,
    certVariant,
    certLang,
    str,
    signer,
    purpose,
    signatureHash,
    signatureTimestamp,
    docID,
    docWords,
    docChars,
    docLines
  }: {
    mode: Mode;
    title: string;
    body: string;
    sourcePath: string;
    header: string;
    footer: string;
    signatureOn: boolean;
    certVariant: CertVariant;
    certLang: CertLang;
    str: CertStrings;
    signer: string;
    purpose: string;
    signatureHash: string;
    signatureTimestamp: string;
    docID: string;
    docWords: number;
    docChars: number;
    docLines: number;
  } = $props();

  let today = $derived(todayHuman(certLang));
</script>

<!-- The printable surface. data-mode toggles the layout.
     data-sig-variant / data-sig-lang drive the signature
     footer's compact-vs-standard rendering and language. -->
<main class="print-page" data-mode={mode} data-sig-variant={certVariant} data-sig-lang={certLang}>
  {#if mode === 'letterhead'}
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
      <div class="lh-date">{today}</div>
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
          <tr><th>Date:</th><td>{today}</td></tr>
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
      <div class="doc-meta">{sourcePath} · {today}</div>
      <MarkdownRenderer body={body} />
    </article>
    <footer class="print-footer">
      <div class="print-footer-text">
        {footer || today}
      </div>
    </footer>
  {/if}

  <!-- Document signature footer — the PDF-grade integrity stamp
       the user actually asked for. Toggleable on every mode via
       the toolbar's Sign button. -->
  {#if signatureOn}
    <SignatureFooter
      variant={certVariant}
      lang={certLang}
      {str}
      {signer}
      {purpose}
      {signatureHash}
      {signatureTimestamp}
      {docID}
      {sourcePath}
      {docWords}
      {docChars}
      {docLines}
    />
  {/if}
</main>
