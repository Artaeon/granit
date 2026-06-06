// Inline-edit controller for the TaskDetail drawer's two long-form
// buffers: the task title (click to edit, Enter / blur to commit, Esc
// to cancel) and the free-form notes textarea (always-on, commit on
// blur).
//
// Both buffers round-trip through draftAutosave so a closed tab / OS
// suspend mid-edit doesn't lose paragraphs of work. Drafts are keyed
// per-task-id so switching tasks doesn't cross-contaminate. The
// install() helper hooks up the two persistence $effects; call it
// from the component's script body so the effects bind to its
// lifecycle.
//
// Gating: the notes textarea is always rendered (no edit toggle) so
// the draft-write $effect must gate on "actually differs from server
// state". Without it every task open would write a stale-equal draft
// and accumulate localStorage entries forever. The title draft is
// only written while titleEditing is true.
//
// initFor(task) is called by the parent's per-task $effect on each
// fresh task target — loads any saved notes draft (preferring it over
// the canonical task.notes since it's the most recent intent) and
// resets the title editor.

import type { Task } from '$lib/api';
import { cleanTaskText } from '$lib/util/taskParse';
import { clearDraft, loadDraft, makeDraftWriter } from '$lib/util/draftAutosave';

export interface TaskDetailInlineEditController {
  notesBuf: string;
  titleBuf: string;
  titleEditing: boolean;

  /** Reseed buffers for a fresh task target. Loads the notes draft
   *  preferentially, falling back to task.notes; resets the title
   *  editor closed with the canonical task.text. */
  initFor(task: Task): void;
  /** Install the two draft-write $effects. Call from a component's
   *  script body so the $effect registrations bind to its lifecycle. */
  install(): void;

  /** Open the title editor; prefers a stale draft over the canonical
   *  title text so an interrupted rename can be picked back up. */
  startTitleEdit(): void;
  /** Commit the title buffer through `patch` if it actually changed,
   *  then close the editor and clear the draft. */
  commitTitle(): Promise<void>;
  /** Close the title editor without saving and clear the draft. */
  cancelTitleEdit(): void;
  /** Commit the notes buffer through `patch` if it differs from
   *  server state, then clear the draft. */
  commitNotes(): Promise<void>;
}

export type TaskDetailInlineEditDeps = {
  getTask: () => Task | null;
  patch: (p: { text?: string; notes?: string }) => Promise<void>;
};

export function createTaskDetailInlineEdit(deps: TaskDetailInlineEditDeps): TaskDetailInlineEditController {
  let notesBuf = $state('');
  let titleBuf = $state('');
  let titleEditing = $state(false);

  const notesDraftWriter = makeDraftWriter(400);
  const titleDraftWriter = makeDraftWriter(400);
  const notesDraftKey = () => {
    const t = deps.getTask();
    return t ? `task.notes.${t.id}` : '';
  };
  const titleDraftKey = () => {
    const t = deps.getTask();
    return t ? `task.title.${t.id}` : '';
  };

  function initFor(task: Task) {
    // Prefer a saved draft for notes — that's the most recent intent.
    // Title draft is loaded only when the user actually opens the
    // title editor (titleEditing toggle); otherwise the displayed
    // title comes from the canonical task.text.
    const notesDraft = loadDraft<string | null>(`task.notes.${task.id}`, null);
    notesBuf = (notesDraft !== null && notesDraft !== '') ? notesDraft : (task.notes ?? '');
    titleEditing = false;
    titleBuf = cleanTaskText(task.text);
  }

  function install() {
    // Notes textarea is always rendered (no edit toggle), so gate the
    // draft write on "actually differs from server state". Without
    // this gate every task open would write a stale-equal draft and
    // accumulate localStorage entries forever.
    $effect(() => {
      const t = deps.getTask();
      const k = notesDraftKey();
      if (!t || !k) return;
      if (notesBuf === (t.notes ?? '')) return;
      notesDraftWriter.save(k, notesBuf);
    });
    $effect(() => {
      const t = deps.getTask();
      const k = titleDraftKey();
      if (!t || !titleEditing || !k) return;
      if (titleBuf === cleanTaskText(t.text)) return;
      titleDraftWriter.save(k, titleBuf);
    });
  }

  function startTitleEdit() {
    const task = deps.getTask();
    if (!task) return;
    const draft = loadDraft<string | null>(titleDraftKey(), null);
    titleBuf = (draft !== null && draft !== '') ? draft : cleanTaskText(task.text);
    titleEditing = true;
  }

  async function commitTitle() {
    const task = deps.getTask();
    if (!task) { titleEditing = false; return; }
    const next = titleBuf.trim();
    // cleanTaskText strips inline markers (!1 / due:.. / #tag) for
    // display; we round-trip the user's edit as the new task text and
    // let the parser re-extract markers from the new line on the next
    // read. Empty title is a no-op.
    if (!next || next === cleanTaskText(task.text)) {
      titleEditing = false;
      clearDraft(titleDraftKey());
      titleDraftWriter.cancel();
      return;
    }
    titleEditing = false;
    await deps.patch({ text: next });
    clearDraft(titleDraftKey());
    titleDraftWriter.cancel();
  }

  function cancelTitleEdit() {
    const task = deps.getTask();
    if (task) titleBuf = cleanTaskText(task.text);
    titleEditing = false;
    clearDraft(titleDraftKey());
    titleDraftWriter.cancel();
  }

  async function commitNotes() {
    const task = deps.getTask();
    if (!task) return;
    if (notesBuf === (task.notes ?? '')) {
      // No change to persist — but drop the draft so it doesn't
      // resurrect on next mount with stale-equal content.
      clearDraft(notesDraftKey());
      notesDraftWriter.cancel();
      return;
    }
    await deps.patch({ notes: notesBuf });
    clearDraft(notesDraftKey());
    notesDraftWriter.cancel();
  }

  return {
    get notesBuf() { return notesBuf; },
    set notesBuf(v) { notesBuf = v; },
    get titleBuf() { return titleBuf; },
    set titleBuf(v) { titleBuf = v; },
    get titleEditing() { return titleEditing; },
    set titleEditing(v) { titleEditing = v; },
    initFor,
    install,
    startTitleEdit,
    commitTitle,
    cancelTitleEdit,
    commitNotes
  };
}
