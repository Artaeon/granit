// Link-suggester glue for the notes editor's right-rail panel.
//
// Two affordances surfaced via NoteInfoRail:
//
//   • Tag chip — appends a suggested tag to frontmatter.tags
//     (de-duplicated, via the existing saveFrontmatter pipeline).
//
//   • Link chip — inserts a wikilink at the editor cursor, or
//     appends it to the body when the editor is unmounted (preview
//     view).
//
// The pure helpers live in $lib/notes/frontmatterTagOps; this
// controller owns the route-side deps wiring (current note, current
// editor handle, current body, saveFrontmatter) so the rail panel
// can stay free of route state.

import type { Note } from '$lib/api';
import {
  parseTagsField,
  addSuggestedTag,
  insertSuggestedLink
} from '$lib/notes/frontmatterTagOps';

export interface NoteLinkSuggester {
  /** Tags already declared on this note — drives "already added"
   *  highlighting on the suggester chips. */
  readonly existingTagList: string[];
  addSuggestedTag: (tag: string) => Promise<void>;
  insertSuggestedLink: (markup: string) => void;
}

export interface NoteLinkSuggesterOpts {
  getNote: () => Note | null;
  saveFrontmatter: (next: Record<string, unknown>) => Promise<boolean>;
  /** Live editor handle for cursor-insert. Null when unmounted —
   *  insertSuggestedLink falls through to appendToBody. */
  getInsertAtCursor: () => ((text: string) => void) | undefined;
  /** Append fallback for preview-view link inserts. The controller
   *  mutates body via the pipe proxy passed in by the page. */
  appendToBody: (markup: string) => void;
}

export function createNoteLinkSuggester(
  opts: NoteLinkSuggesterOpts
): NoteLinkSuggester {
  const existingTagList = $derived(
    parseTagsField(
      opts.getNote()?.frontmatter as Record<string, unknown> | undefined
    )
  );

  async function addSuggestedTagWrapped(tag: string): Promise<void> {
    const note = opts.getNote();
    if (!note) return;
    await addSuggestedTag(tag, { note, saveFrontmatter: opts.saveFrontmatter });
  }

  function insertSuggestedLinkWrapped(markup: string): void {
    insertSuggestedLink(markup, {
      insertAtCursor: opts.getInsertAtCursor(),
      appendToBody: opts.appendToBody
    });
  }

  return {
    get existingTagList() { return existingTagList; },
    addSuggestedTag: addSuggestedTagWrapped,
    insertSuggestedLink: insertSuggestedLinkWrapped
  };
}
