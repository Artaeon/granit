// Inline-edit buffers + draft autosave for ProjectDetail.
//
// Three editable text fields:
//
//   • description  — multiline; onblur commits; draft autosave so a
//                    reload while the textarea has focus doesn't lose
//                    typed content.
//   • next_action  — single-line; same draft autosave.
//   • name         — single-line; no draft (rename is short + rare).
//
// The "cancel-then-save" bug: Esc handlers used to flip
// editingDescription=false; that unmounts the textarea, the browser
// fires blur, commitDescription runs, and the just-Esc'd text gets
// patched anyway. The cancelling flags in this controller are
// checked before commit fires to short-circuit that path.
//
// Project-switch reset: when the parent swaps the project prop without
// unmounting (master-detail list-click), the new project's draft key
// is different from the old one. The reset() method closes any open
// editor + cancels pending draft writers so the OLD project's buffer
// doesn't bleed into the NEW project's localStorage key.

import type { Project } from '$lib/api';
import {
  loadDraft,
  clearDraft,
  makeDraftWriter
} from '$lib/util/draftAutosave';

export interface ProjectInlineEditController {
  editingDescription: boolean;
  editingNextAction: boolean;
  editingName: boolean;
  descBuf: string;
  nextActionBuf: string;
  nameBuf: string;
  // cancellingDesc / cancellingNextAction sentinels are internal —
  // owned by cancelEditDescription() + cancelEditNextAction() and
  // read by the commit*() functions. Not on the interface; no
  // external caller has business writing them.

  /** Open the description editor seeded with the current value (or
   *  the saved draft if present). */
  startEditDescription(): void;
  /** Open the next-action editor seeded with the current value (or
   *  the saved draft if present). */
  startEditNextAction(): void;
  /** Open the name editor seeded with the current value. */
  startEditName(): void;

  commitDescription(): Promise<void>;
  commitNextAction(): Promise<void>;
  commitName(): Promise<void>;

  /** Esc handler for the description textarea — set the cancel flag
   *  BEFORE flipping editingDescription so the blur-on-unmount that
   *  follows short-circuits commitDescription instead of silently
   *  persisting. Also clears the draft + cancels the pending writer. */
  cancelEditDescription(): void;
  cancelEditNextAction(): void;

  /** Project-switch reset — closes any open editor + cancels pending
   *  draft writers. Call from the prop-watch $effect in the parent. */
  reset(): void;
}

export interface ProjectInlineEditDeps {
  getProject: () => Project;
  /** Per-field patch hook — usually `patch({ field: value })` from
   *  the parent. Returns whether the save succeeded. */
  patch: (p: Partial<Project>) => Promise<boolean>;
}

export function createProjectInlineEdit(deps: ProjectInlineEditDeps): ProjectInlineEditController {
  let editingDescription = $state(false);
  let editingNextAction = $state(false);
  let editingName = $state(false);
  let descBuf = $state('');
  let nextActionBuf = $state('');
  let nameBuf = $state('');
  // Plain `let` not $state — set + read both happen synchronously
  // inside the cancelEdit -> blur -> commit chain, no reactive
  // consumer needs to track them.
  let cancellingDesc = false;
  let cancellingNextAction = false;

  const descDraftWriter = makeDraftWriter(400);
  const nextActionDraftWriter = makeDraftWriter(400);

  // Drafts are keyed by project.name so switching projects doesn't
  // cross-contaminate.
  function descDraftKey(): string {
    return `project.description.${deps.getProject().name}`;
  }
  function nextActionDraftKey(): string {
    return `project.nextAction.${deps.getProject().name}`;
  }

  $effect(() => {
    if (editingDescription) descDraftWriter.save(descDraftKey(), descBuf);
  });
  $effect(() => {
    if (editingNextAction) nextActionDraftWriter.save(nextActionDraftKey(), nextActionBuf);
  });

  function startEditDescription() {
    const project = deps.getProject();
    descBuf = loadDraft<string>(descDraftKey(), project.description ?? '');
    editingDescription = true;
  }

  function startEditNextAction() {
    const project = deps.getProject();
    nextActionBuf = loadDraft<string>(nextActionDraftKey(), project.next_action ?? '');
    editingNextAction = true;
  }

  function startEditName() {
    const project = deps.getProject();
    nameBuf = project.name;
    editingName = true;
  }

  async function commitDescription() {
    if (cancellingDesc) { cancellingDesc = false; return; }
    editingDescription = false;
    const project = deps.getProject();
    if (descBuf !== (project.description ?? '')) {
      await deps.patch({ description: descBuf });
    }
    // Whether the patch fired or not, the user closed the editor —
    // the in-buffer text is no longer "in-flight", clear the draft.
    clearDraft(descDraftKey());
    descDraftWriter.cancel();
  }

  async function commitNextAction() {
    if (cancellingNextAction) { cancellingNextAction = false; return; }
    editingNextAction = false;
    const project = deps.getProject();
    if (nextActionBuf !== (project.next_action ?? '')) {
      await deps.patch({ next_action: nextActionBuf });
    }
    clearDraft(nextActionDraftKey());
    nextActionDraftWriter.cancel();
  }

  async function commitName() {
    editingName = false;
    const project = deps.getProject();
    if (nameBuf && nameBuf !== project.name) {
      await deps.patch({ name: nameBuf });
    }
  }

  function cancelEditDescription() {
    cancellingDesc = true;
    editingDescription = false;
    clearDraft(descDraftKey());
    descDraftWriter.cancel();
  }

  function cancelEditNextAction() {
    cancellingNextAction = true;
    editingNextAction = false;
    clearDraft(nextActionDraftKey());
    nextActionDraftWriter.cancel();
  }

  function reset() {
    editingDescription = false;
    editingNextAction = false;
    editingName = false;
    cancellingDesc = false;
    cancellingNextAction = false;
    descDraftWriter.cancel();
    nextActionDraftWriter.cancel();
  }

  return {
    get editingDescription() { return editingDescription; },
    set editingDescription(v) { editingDescription = v; },
    get editingNextAction() { return editingNextAction; },
    set editingNextAction(v) { editingNextAction = v; },
    get editingName() { return editingName; },
    set editingName(v) { editingName = v; },
    get descBuf() { return descBuf; },
    set descBuf(v) { descBuf = v; },
    get nextActionBuf() { return nextActionBuf; },
    set nextActionBuf(v) { nextActionBuf = v; },
    get nameBuf() { return nameBuf; },
    set nameBuf(v) { nameBuf = v; },
    startEditDescription,
    startEditNextAction,
    startEditName,
    commitDescription,
    commitNextAction,
    commitName,
    cancelEditDescription,
    cancelEditNextAction,
    reset
  };
}
