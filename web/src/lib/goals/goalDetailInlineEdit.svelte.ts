// Inline-edit buffers + draft autosave for GoalDetail.
//
// Three editable text fields:
//   • title         — single-line; the goal's name
//   • description   — multiline; the goal's "what + why"
//   • notes         — long-form; the goal's running journal —
//                     highest loss risk on a reload, so draft
//                     autosave matters most here.
//
// Same KISS pattern as projectInlineEdit: buffer-to-localStorage on
// change, restore on re-entry to edit, clear on successful commit.
// Drafts keyed per-goal so switching goals in the drawer doesn't
// cross-contaminate.
//
// The "cancel-then-save" bug: Esc handlers used to flip
// editingDesc=false; that unmounts the textarea, the browser fires
// blur, commitDesc runs, and the just-Esc'd text gets patched
// anyway. The cancelling flags below are checked before each commit
// fires to short-circuit that path — same shape as
// projectInlineEdit, which had this fixed first.

import type { Goal } from '$lib/api';
import {
  loadDraft,
  clearDraft,
  makeDraftWriter
} from '$lib/util/draftAutosave';

export interface GoalDetailInlineEditController {
  editingTitle: boolean;
  editingDesc: boolean;
  editingNotes: boolean;
  titleBuf: string;
  descBuf: string;
  notesBuf: string;

  startEditTitle(): void;
  startEditDesc(): void;
  startEditNotes(): void;

  commitTitle(): Promise<void>;
  commitDesc(): Promise<void>;
  commitNotes(): Promise<void>;

  /** Esc handler for each editor — set the cancel flag BEFORE
   *  flipping editing=false so the blur-on-unmount that follows
   *  short-circuits the commit instead of silently persisting.
   *  Also clears the draft + cancels the pending writer. */
  cancelEditTitle(): void;
  cancelEditDesc(): void;
  cancelEditNotes(): void;

  /** Goal-switch reset — closes any open editor + cancels pending
   *  draft writers so the OLD goal's buffer doesn't bleed into the
   *  NEW goal's localStorage key. */
  reset(): void;
}

export interface GoalDetailInlineEditDeps {
  getGoal: () => Goal | null;
  patch: (p: Partial<Goal>) => Promise<boolean>;
}

export function createGoalDetailInlineEdit(
  deps: GoalDetailInlineEditDeps
): GoalDetailInlineEditController {
  let editingTitle = $state(false);
  let editingDesc = $state(false);
  let editingNotes = $state(false);
  let titleBuf = $state('');
  let descBuf = $state('');
  let notesBuf = $state('');
  // Cancel sentinels — set by cancelEdit*() before flipping
  // editing=false, checked by commit*() so a blur fired by DOM
  // unmount doesn't silently persist text the user just Esc'd.
  let cancellingTitle = false;
  let cancellingDesc = false;
  let cancellingNotes = false;

  const titleDraftWriter = makeDraftWriter(400);
  const descDraftWriter = makeDraftWriter(400);
  const notesDraftWriter = makeDraftWriter(400);

  function titleDraftKey() {
    const goal = deps.getGoal();
    return goal ? `goal.title.${goal.id}` : '';
  }
  function descDraftKey() {
    const goal = deps.getGoal();
    return goal ? `goal.description.${goal.id}` : '';
  }
  function notesDraftKey() {
    const goal = deps.getGoal();
    return goal ? `goal.notes.${goal.id}` : '';
  }

  $effect(() => {
    if (editingTitle && titleDraftKey()) titleDraftWriter.save(titleDraftKey(), titleBuf);
  });
  $effect(() => {
    if (editingDesc && descDraftKey()) descDraftWriter.save(descDraftKey(), descBuf);
  });
  $effect(() => {
    if (editingNotes && notesDraftKey()) notesDraftWriter.save(notesDraftKey(), notesBuf);
  });

  function startEditTitle() {
    const goal = deps.getGoal();
    if (!goal) return;
    titleBuf = loadDraft<string>(titleDraftKey(), goal.title);
    editingTitle = true;
  }
  function startEditDesc() {
    const goal = deps.getGoal();
    if (!goal) return;
    descBuf = loadDraft<string>(descDraftKey(), goal.description ?? '');
    editingDesc = true;
  }
  function startEditNotes() {
    const goal = deps.getGoal();
    if (!goal) return;
    notesBuf = loadDraft<string>(notesDraftKey(), goal.notes ?? '');
    editingNotes = true;
  }

  async function commitTitle() {
    if (cancellingTitle) { cancellingTitle = false; return; }
    editingTitle = false;
    const goal = deps.getGoal();
    if (goal && titleBuf.trim() && titleBuf !== goal.title) {
      await deps.patch({ title: titleBuf.trim() });
    }
    clearDraft(titleDraftKey());
    titleDraftWriter.cancel();
  }
  async function commitDesc() {
    if (cancellingDesc) { cancellingDesc = false; return; }
    editingDesc = false;
    const goal = deps.getGoal();
    if (goal && descBuf !== (goal.description ?? '')) {
      await deps.patch({ description: descBuf });
    }
    clearDraft(descDraftKey());
    descDraftWriter.cancel();
  }
  async function commitNotes() {
    if (cancellingNotes) { cancellingNotes = false; return; }
    editingNotes = false;
    const goal = deps.getGoal();
    if (goal && notesBuf !== (goal.notes ?? '')) {
      await deps.patch({ notes: notesBuf });
    }
    clearDraft(notesDraftKey());
    notesDraftWriter.cancel();
  }

  function cancelEditTitle() {
    cancellingTitle = true;
    editingTitle = false;
    clearDraft(titleDraftKey());
    titleDraftWriter.cancel();
  }
  function cancelEditDesc() {
    cancellingDesc = true;
    editingDesc = false;
    clearDraft(descDraftKey());
    descDraftWriter.cancel();
  }
  function cancelEditNotes() {
    cancellingNotes = true;
    editingNotes = false;
    clearDraft(notesDraftKey());
    notesDraftWriter.cancel();
  }

  function reset() {
    editingTitle = false;
    editingDesc = false;
    editingNotes = false;
    cancellingTitle = false;
    cancellingDesc = false;
    cancellingNotes = false;
    // flushNow (not cancel) — when the user switches goals mid-
    // typing (within the 400ms debounce), the last keystrokes are
    // still pending. cancel() would silently discard them; flushNow
    // commits them to localStorage so re-opening the old goal
    // restores the same buffer. Notes is the highest-loss-risk
    // field, where this matters most.
    titleDraftWriter.flushNow();
    descDraftWriter.flushNow();
    notesDraftWriter.flushNow();
  }

  return {
    get editingTitle() { return editingTitle; },
    set editingTitle(v) { editingTitle = v; },
    get editingDesc() { return editingDesc; },
    set editingDesc(v) { editingDesc = v; },
    get editingNotes() { return editingNotes; },
    set editingNotes(v) { editingNotes = v; },
    get titleBuf() { return titleBuf; },
    set titleBuf(v) { titleBuf = v; },
    get descBuf() { return descBuf; },
    set descBuf(v) { descBuf = v; },
    get notesBuf() { return notesBuf; },
    set notesBuf(v) { notesBuf = v; },
    startEditTitle,
    startEditDesc,
    startEditNotes,
    commitTitle,
    commitDesc,
    commitNotes,
    cancelEditTitle,
    cancelEditDesc,
    cancelEditNotes,
    reset
  };
}
