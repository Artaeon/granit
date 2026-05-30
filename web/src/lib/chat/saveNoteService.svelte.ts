// Save-as-note / save-to-library / copy service for AIOverlay.
//
// Cuts the "draft a reply → manually copy → create a note" loop into
// one button. Three flows handled here:
//
//   1. saveThreadAsNote   — the full conversation gets written under
//      chat-history/. Used by the toolbar "save" button.
//   2. saveAssistantAsNote — one assistant reply gets written under
//      Projects/<name>/ (if in project scope), Goals/<title>/ (goal
//      scope), Calendar/ (calendar route), or Drafts/ otherwise.
//      Used by the per-message disk icon.
//   3. confirmSaveLibrary — a user prompt becomes a library entry,
//      reusable from the inline AI menu's Library section. Used by
//      the per-user-message "+" icon.
//
// Plus copyAssistantMessage — sibling concern (drop content to
// clipboard). Lives here because it shares stripStructuredBlocks
// with saveAssistantAsNote and the "this is what we'd save / copy"
// content shape.
//
// State convention: parent owns the reactive slots (saving /
// savingMessageIdx / copiedMessageIdx / savingLibraryIdx / etc.)
// because the message list and toolbar both render them. The
// service reads / writes via the refs object; the timer handle for
// the copy flash is owned internally (it's per-instance, not UI).

import { api } from '$lib/api';
import type { ChatMessage } from '$lib/api';
import type { RagHit } from './rag';
import {
  buildSaveThreadPayload,
  buildAssistantNotePayload,
  buildAssistantNoteRetryPath
} from './saveToNote';
import { deriveDraftTitle } from '$lib/ai/draftTitle';
import { stripStructuredBlocks } from './actionParser';
import { deriveLibraryLabel } from './history';
import { todayISO } from '$lib/util/date';
import { errorMessage } from '$lib/util/errorMessage';
import { toast } from '$lib/components/toast';

export interface SaveNoteRefs {
  /** Whether the save-thread toolbar action is in flight. */
  saving: boolean;
  /** Idx of the assistant message currently being saved (null when
   *  idle). The "save as note" icon swaps to a spinner for that row. */
  savingMessageIdx: number | null;
  /** Idx of the assistant message that just got copied. Triggers a
   *  checkmark flash for ~1.2s. Reset to null on timer expiry. */
  copiedMessageIdx: number | null;
  /** Idx of the user message whose "save to library" form is open
   *  (null when no form is open). */
  savingLibraryIdx: number | null;
  /** Bindable label input. Seeded from deriveLibraryLabel on open. */
  savingLibraryLabel: string;
  /** Whether a library PUT is in flight. Disables the form during. */
  savingLibraryBusy: boolean;
  /** Full thread — read for saveThreadAsNote payload. */
  messages: ChatMessage[];
  /** Quick-action result, also captured by saveThreadAsNote when
   *  no chat messages exist. */
  quickTitle: string;
  quickResult: string;
  /** Mode metadata — captured in frontmatter for both save flows. */
  modeId: string;
  modeLabel: string;
  /** RAG state at the time of save — captured in thread payload. */
  rag: boolean;
  lastRagHits: RagHit[];
  /** Routing context — drives folder for saveAssistantAsNote. */
  currentProjectName: string;
  currentGoalId: string;
  onCalendarPage: boolean;
}

export interface SaveNoteServiceOptions {
  refs: SaveNoteRefs;
}

export interface SaveNoteService {
  saveThreadAsNote(): Promise<void>;
  saveAssistantAsNote(idx: number): Promise<void>;
  copyAssistantMessage(content: string, idx: number): Promise<void>;
  openSaveLibrary(idx: number, content: string): void;
  cancelSaveLibrary(): void;
  confirmSaveLibrary(promptContent: string): Promise<void>;
}

export function createSaveNoteService(opts: SaveNoteServiceOptions): SaveNoteService {
  // Internal — per-instance flash timer.
  let copyResetTimer: ReturnType<typeof setTimeout> | null = null;

  async function saveThreadAsNote() {
    if (opts.refs.saving) return;
    const payload = buildSaveThreadPayload({
      messages: opts.refs.messages,
      quickTitle: opts.refs.quickTitle,
      quickResult: opts.refs.quickResult,
      modeId: opts.refs.modeId,
      modeLabel: opts.refs.modeLabel,
      rag: opts.refs.rag,
      lastRagHits: opts.refs.lastRagHits,
      now: new Date()
    });
    if (!payload) {
      toast.info('Nothing to save yet.');
      return;
    }
    opts.refs.saving = true;
    try {
      await api.createNote(payload);
      toast.success('Saved · ' + payload.path);
    } catch (e) {
      toast.error('save failed: ' + errorMessage(e));
    } finally {
      opts.refs.saving = false;
    }
  }

  async function copyAssistantMessage(content: string, idx: number): Promise<void> {
    // Drop action / suggestion blocks so the clipboard contains only
    // the human-readable reply, not the JSON the assistant emitted.
    const cleaned = stripStructuredBlocks(content || '').trim();
    if (!cleaned) {
      toast.info('Nothing to copy.');
      return;
    }
    try {
      if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(cleaned);
      } else {
        // Fallback for non-secure contexts / older browsers: temporary
        // textarea + execCommand. Deprecated but still works on iOS
        // Safari served over plain HTTP (rare for Granit but possible
        // on a LAN).
        const ta = document.createElement('textarea');
        ta.value = cleaned;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
      }
      opts.refs.copiedMessageIdx = idx;
      if (copyResetTimer) clearTimeout(copyResetTimer);
      copyResetTimer = setTimeout(() => {
        opts.refs.copiedMessageIdx = null;
      }, 1200);
    } catch {
      toast.error('Copy failed — your browser blocked clipboard access.');
    }
  }

  async function saveAssistantAsNote(idx: number) {
    const m = opts.refs.messages[idx];
    if (!m || m.role !== 'assistant') return;
    if (opts.refs.savingMessageIdx !== null) return;
    const cleaned = stripStructuredBlocks(m.content || '').trim();
    if (!cleaned) {
      toast.info('Nothing to save in this reply.');
      return;
    }
    opts.refs.savingMessageIdx = idx;
    const title = deriveDraftTitle(cleaned, todayISO());
    const { basePath, folder, baseSlug, frontmatter } = buildAssistantNotePayload({
      cleanedContent: cleaned,
      title,
      modeId: opts.refs.modeId,
      currentProjectName: opts.refs.currentProjectName,
      currentGoalId: opts.refs.currentGoalId,
      onCalendarPage: opts.refs.onCalendarPage
    });
    try {
      let finalPath = basePath;
      try {
        await api.createNote({ path: basePath, frontmatter, body: cleaned });
      } catch (err) {
        // 409 Conflict — file exists. Retry with a time suffix.
        // Any other error rethrows to the outer toast handler.
        const msg = errorMessage(err);
        if (!/already exists|409/i.test(msg)) throw err;
        finalPath = buildAssistantNoteRetryPath(folder, baseSlug, new Date());
        await api.createNote({ path: finalPath, frontmatter, body: cleaned });
      }
      toast.success(`Saved · ${finalPath}`, {
        action: { label: 'Open', href: `/notes/${encodeURIComponent(finalPath)}` }
      });
    } catch (e) {
      toast.error('Save failed: ' + errorMessage(e));
    } finally {
      opts.refs.savingMessageIdx = null;
    }
  }

  function openSaveLibrary(idx: number, content: string) {
    opts.refs.savingLibraryIdx = idx;
    // Seed label with the first few words so the user has something
    // to edit rather than starting from blank — most labels are a
    // quick tweak of the seed.
    opts.refs.savingLibraryLabel = deriveLibraryLabel(content);
  }

  function cancelSaveLibrary() {
    opts.refs.savingLibraryIdx = null;
    opts.refs.savingLibraryLabel = '';
  }

  async function confirmSaveLibrary(promptContent: string) {
    const label = opts.refs.savingLibraryLabel.trim();
    if (!label || opts.refs.savingLibraryBusy) return;
    opts.refs.savingLibraryBusy = true;
    try {
      const cur = await api.getAIPrompts();
      // 'either' as the default scope — the user can edit scope later
      // if they want to constrain when the entry surfaces. Most chat
      // prompts apply equally to selection + cursor surfaces.
      const next = {
        entries: [
          ...(cur.entries ?? []),
          { label, prompt: promptContent.trim(), scope: 'either' as const }
        ]
      };
      await api.putAIPrompts(next);
      toast.success(`Saved "${label}" to library`);
      cancelSaveLibrary();
    } catch (err) {
      toast.error('save failed: ' + errorMessage(err));
    } finally {
      opts.refs.savingLibraryBusy = false;
    }
  }

  return {
    saveThreadAsNote,
    saveAssistantAsNote,
    copyAssistantMessage,
    openSaveLibrary,
    cancelSaveLibrary,
    confirmSaveLibrary
  };
}
