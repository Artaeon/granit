// Slugify a title into a vault-safe filename body (no extension).
// Used wherever a string becomes part of a path: Inbox/<date>-<slug>.md,
// extract-to-note, AI thread save-as-note, project-linked new note.
//
// Five files were doing variants of this inline; consolidating here
// gives every "+ New note" surface the same path shape.
//
// Behaviour: lowercase, NFKD-normalise then strip combining
// diacriticals (so "café" becomes "cafe"), collapse non-alphanumerics
// to a single dash, trim leading/trailing dashes, and clamp to 80
// chars. Empty / all-symbols input returns "" — callers fall back to
// "untitled" or similar.

export function slugifyTitle(title: string): string {
  return title
    .toLowerCase()
    .normalize('NFKD')
    // Strip combining diacritical marks (U+0300..U+036F).
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80);
}
