// Shared types + i18n strings + small helpers for the PrintPreview
// subtree. Lives outside the Svelte components so the templates can
// share the same labels without prop-drilling a giant table.

export type Mode = 'standard' | 'report' | 'letterhead' | 'memo';
export type CertVariant = 'standard' | 'compact';
export type CertLang = 'en' | 'de';

// German / English string table for the certificate template.
// Keep keys descriptive and short; the structure mirrors the cert
// body in render order so a translator can verify coverage by
// reading top-to-bottom.
export const CERT_STRINGS = {
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
    disclaimer: "This is a content-integrity stamp, not a legally binding e-signature. The hash proves the document hasn't been altered since generation; the signer field is a self-attested claim with no third-party verification authority.",
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

export type CertStrings = (typeof CERT_STRINGS)[CertLang];

// Format an ISO timestamp following the cert language toggle so a
// German certificate gets a German timestamp. hour12:false forces
// 24-hour format regardless of OS locale.
export function fmtTimestamp(iso: string, lang: CertLang): string {
  if (!iso) return '';
  try {
    const locale = lang === 'de' ? 'de-AT' : undefined;
    return new Date(iso).toLocaleString(locale, {
      year: 'numeric', month: 'short', day: 'numeric',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
      hour12: false
    });
  } catch {
    return iso;
  }
}

// Today's date in the user's locale, used as a default placeholder
// when the footer field is empty.
export function todayHuman(lang: CertLang): string {
  const locale = lang === 'de' ? 'de-AT' : undefined;
  return new Date().toLocaleDateString(locale, {
    year: 'numeric', month: 'long', day: 'numeric'
  });
}

// SHA-256 hex digest of an arbitrary string. Returns '' if the
// SubtleCrypto API isn't available (SSR / very old browsers).
export async function computeHash(text: string): Promise<string> {
  if (typeof crypto === 'undefined' || !crypto.subtle) return '';
  const enc = new TextEncoder().encode(text);
  const buf = await crypto.subtle.digest('SHA-256', enc);
  return Array.from(new Uint8Array(buf))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}
