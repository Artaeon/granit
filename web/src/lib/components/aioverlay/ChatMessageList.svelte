<script lang="ts">
  import type { ChatMessage } from '$lib/api';
  import type { RagHit } from '$lib/chat/rag';
  import {
    parseFollowups,
    parseActions,
    stripStructuredBlocks,
    actionKey,
    type ParsedAction
  } from '$lib/chat/actionParser';
  import { hasActiveEditor, insertAtCursor } from '$lib/stores/active-editor';
  import { focusOnMount } from '$lib/util/focusOnMount';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  // ChatMessageList — the streaming-conversation render: user
  // bubbles, assistant bubbles, action chips, follow-up suggestions,
  // per-turn RAG sources, plus all the per-message buttons (edit,
  // regen, branch, save-as-note, copy, pin, insert-at-cursor) and
  // the inline save-to-library / edit-user-message forms.
  //
  // Extracted from AIOverlay so the message list can be rendered
  // identically by the standalone /chat page and any future thread
  // viewer. The component owns layout + per-message UX; the parent
  // owns mutation (regen / branch / pin / save / commit-action all
  // close over thread state and the abort lifecycle). This component
  // talks to the parent via narrow callback props.
  //
  // Bindable values flow back for the inline forms that the parent
  // also writes (cancelling closes them from outside the list):
  //   - savingLibraryLabel (the library-form text input)
  //   - editingUserDraft (the edit-user textarea)
  //   - expandedSources (per-turn details<open> state)

  interface Props {
    messages: ChatMessage[];
    busy: boolean;
    pinnedIndex: Record<number, boolean>;
    perTurnRagHits: Record<number, RagHit[]>;
    expandedSources: Record<number, boolean>;
    committedActions: Record<string, boolean>;
    savingLibraryIdx: number | null;
    savingLibraryLabel: string;
    savingLibraryBusy: boolean;
    editingUserIdx: number | null;
    editingUserDraft: string;
    copiedMessageIdx: number | null;
    savingMessageIdx: number | null;
    currentProjectName: string | null;
    onOpenSaveLibrary: (idx: number, content: string) => void;
    onCancelSaveLibrary: () => void;
    onConfirmSaveLibrary: (content: string) => void;
    onRegen: (idx: number) => void;
    onBranch: (idx: number) => void;
    onSaveAsNote: (idx: number) => void;
    onCopy: (content: string, idx: number) => void;
    onPin: (idx: number) => void;
    onStartEditUser: (idx: number) => void;
    onCancelEditUser: () => void;
    onSubmitEditUser: () => void;
    onCommitAction: (msgIdx: number, a: ParsedAction) => void;
    onSendFollowup: (prompt: string) => void;
  }

  let {
    messages,
    busy,
    pinnedIndex,
    perTurnRagHits,
    expandedSources = $bindable(),
    committedActions,
    savingLibraryIdx,
    savingLibraryLabel = $bindable(),
    savingLibraryBusy,
    editingUserIdx,
    editingUserDraft = $bindable(),
    copiedMessageIdx,
    savingMessageIdx,
    currentProjectName,
    onOpenSaveLibrary,
    onCancelSaveLibrary,
    onConfirmSaveLibrary,
    onRegen,
    onBranch,
    onSaveAsNote,
    onCopy,
    onPin,
    onStartEditUser,
    onCancelEditUser,
    onSubmitEditUser,
    onCommitAction,
    onSendFollowup
  }: Props = $props();
</script>

<ul class="space-y-3">
  {#each messages as m, i (i)}
    <li>
      <div class="text-[10px] uppercase tracking-wider {m.role === 'user' ? 'text-secondary' : 'text-primary'} mb-0.5 flex items-center gap-2">
        <span>{m.role === 'user' ? 'you' : 'assistant'}</span>
        {#if m.role === 'user' && m.content && !busy && savingLibraryIdx !== i}
          <!-- Save this user prompt to the library so it becomes a
               one-click entry in the inline AI menu too. Opens an
               inline label input below; saves through api.putAIPrompts
               which is a full upsert so we GET, append, PUT. -->
          <button
            type="button"
            onclick={() => onOpenSaveLibrary(i, m.content)}
            class="tap-target inline-flex items-center justify-center w-6 h-6 rounded text-dim hover:text-secondary hover:bg-surface0 leading-none transition-colors text-[10px]"
            aria-label="Save this prompt to your library"
            title="Save this prompt to your AI library — one click to reuse from any surface"
          >+</button>
        {/if}
        {#if m.role === 'assistant' && m.content && !busy}
          <span class="ml-auto inline-flex items-center gap-1">
            <!-- Regenerate — re-run the same user prompt to get a
                 different answer. Truncates the thread at the
                 preceding user message and re-fires send(), so RAG /
                 mentions / snapshot all re-resolve on the fresh
                 attempt. -->
            <button
              type="button"
              onclick={() => onRegen(i)}
              class="tap-target inline-flex items-center justify-center w-7 h-7 rounded text-dim hover:text-primary hover:bg-surface0 active:bg-surface1 leading-none transition-colors"
              aria-label="Regenerate this reply"
              title="Re-run the prompt to get a different answer"
            >
              <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M3 12a9 9 0 0 1 15-6.7L21 8"/>
                <path d="M21 3v5h-5"/>
                <path d="M21 12a9 9 0 0 1-15 6.7L3 16"/>
                <path d="M3 21v-5h5"/>
              </svg>
            </button>
            <!-- Branch — fork the conversation up to and including
                 this message into a new thread. Original stays in
                 history. -->
            <button
              type="button"
              onclick={() => onBranch(i)}
              class="tap-target inline-flex items-center justify-center w-7 h-7 rounded text-dim hover:text-secondary hover:bg-surface0 active:bg-surface1 leading-none transition-colors"
              aria-label="Branch from here"
              title="Fork the thread from this message into a new conversation"
            >
              <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="6" cy="6" r="2"/>
                <circle cx="18" cy="6" r="2"/>
                <circle cx="6" cy="18" r="2"/>
                <path d="M6 8v8M6 12h6a4 4 0 004-4V8" stroke-linecap="round"/>
              </svg>
            </button>
            <!-- Save as note — cuts the draft → copy → new note loop.
                 PM-drafted briefs / status reports file under
                 Projects/<name>/ when in project scope, Drafts/
                 otherwise. The toast surfaces the resulting path
                 with an Open action. -->
            <button
              type="button"
              onclick={() => onSaveAsNote(i)}
              disabled={savingMessageIdx !== null}
              class="tap-target inline-flex items-center justify-center w-7 h-7 rounded text-dim hover:text-success hover:bg-surface0 active:bg-surface1 leading-none transition-colors disabled:opacity-50"
              aria-label="Save this reply as a vault note"
              title={currentProjectName
                ? `Save under Projects/${currentProjectName}/`
                : 'Save under Drafts/'}
            >
              {#if savingMessageIdx === i}
                <span class="text-[10px]">…</span>
              {:else}
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M5 4h11l3 3v13H5z"/>
                  <path d="M9 4v5h6V4M8 14h8M8 18h6"/>
                </svg>
              {/if}
            </button>
            <!-- Copy — drops the assistant reply's content straight
                 to clipboard so the user can paste elsewhere without
                 first saving it as a vault note. Falls back silently
                 when the Clipboard API isn't available; toast
                 confirms success or hints at the failure mode. -->
            <button
              type="button"
              onclick={() => onCopy(m.content, i)}
              class="tap-target inline-flex items-center justify-center w-7 h-7 rounded leading-none hover:bg-surface0 active:bg-surface1 transition-colors {copiedMessageIdx === i ? 'text-success' : 'text-dim hover:text-primary'}"
              aria-label="Copy this reply to clipboard"
              title={copiedMessageIdx === i ? 'Copied!' : 'Copy reply to clipboard'}
            >
              {#if copiedMessageIdx === i}
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="20 6 9 17 4 12"/>
                </svg>
              {:else}
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="9" y="9" width="11" height="11" rx="2"/>
                  <path d="M5 15V5a2 2 0 0 1 2-2h10"/>
                </svg>
              {/if}
            </button>
            <!-- Pin star — toggles a per-message pin so the reply
                 can be retrieved from the Pinned tab. Snapshots
                 content at click time so a future re-roll / thread
                 prune doesn't lose the text. -->
            <button
              type="button"
              onclick={() => onPin(i)}
              class="tap-target inline-flex items-center justify-center w-7 h-7 rounded text-base leading-none hover:bg-surface0 active:bg-surface1 transition-colors {pinnedIndex[i] ? 'text-warning' : 'text-dim hover:text-warning'}"
              aria-pressed={!!pinnedIndex[i]}
              title={pinnedIndex[i] ? 'Unpin this reply' : 'Pin this reply (find it under History → Pinned)'}
            >
              {#if pinnedIndex[i]}★{:else}☆{/if}
            </button>
            <!-- Insert at cursor — only when a note editor is
                 actively mounted (notes/[...path] page). Drops this
                 reply's text where the cursor is, exactly like a
                 paste; replaces any selection. Lets the user drag a
                 chat answer into the doc without leaving the
                 conversation. -->
            {#if $hasActiveEditor}
              <button
                type="button"
                onclick={() => insertAtCursor(m.content)}
                class="tap-target inline-flex items-center justify-center w-7 h-7 rounded text-dim hover:text-primary hover:bg-surface0 active:bg-surface1 leading-none transition-colors"
                aria-label="Insert this reply at the editor's cursor"
                title="Insert this reply at the current cursor position in the open note"
              >
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M5 12h11"/>
                  <path d="M12 5l7 7-7 7"/>
                </svg>
              </button>
            {/if}
          </span>
        {/if}
      </div>
      {#if m.role === 'user' && savingLibraryIdx === i}
        <!-- Inline save-to-library form: small input for a label,
             submit pushes the prompt into the user's library so it
             surfaces in the inline AI menu's Library section. The
             prompt body is the message content verbatim. Esc closes
             without saving. -->
        <form
          onsubmit={(e) => { e.preventDefault(); onConfirmSaveLibrary(m.content); }}
          class="mt-1 flex items-center gap-1.5"
        >
          <input
            type="text"
            bind:value={savingLibraryLabel}
            placeholder="short name (e.g. 'tighten', 'my voice')"
            onkeydown={(e) => { if (e.key === 'Escape') { e.preventDefault(); onCancelSaveLibrary(); } }}
            class="flex-1 px-2 py-1 text-xs bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-secondary"
            use:focusOnMount
            disabled={savingLibraryBusy}
          />
          <button type="submit" disabled={savingLibraryBusy || !savingLibraryLabel.trim()} class="text-xs px-2 py-1 bg-secondary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50">save</button>
          <button type="button" onclick={onCancelSaveLibrary} class="text-xs text-dim hover:text-text px-1">cancel</button>
        </form>
      {/if}
      {#if m.role === 'user'}
        {#if editingUserIdx === i}
          <!-- Inline-edit a user message. Save truncates everything
               after this turn, resubmits with the edited content;
               cancel restores the original. Mod-Enter / Enter
               (without shift) submits; Esc cancels. Auto-grows up
               to ~10 lines, then scrolls inside the textarea. -->
          <div class="mt-1">
            <textarea
              bind:value={editingUserDraft}
              class="w-full bg-surface0 border border-primary rounded p-2 text-base md:text-sm text-text resize-none focus:outline-none focus:border-primary"
              rows={Math.min(10, Math.max(2, (editingUserDraft.match(/\n/g)?.length ?? 0) + 1))}
              onkeydown={(e) => {
                if (e.key === 'Escape') { e.preventDefault(); onCancelEditUser(); return; }
                if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
                  e.preventDefault();
                  onSubmitEditUser();
                }
              }}
              use:focusOnMount
            ></textarea>
            <div class="mt-1.5 flex items-center gap-2 text-[11px]">
              <button
                type="button"
                onclick={onSubmitEditUser}
                class="tap-target px-2.5 py-1 rounded bg-primary text-on-primary font-medium hover:opacity-90"
              >Save & resubmit</button>
              <button
                type="button"
                onclick={onCancelEditUser}
                class="tap-target px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:bg-surface1"
              >Cancel</button>
              <span class="text-dim">Enter to submit · Esc to cancel</span>
            </div>
          </div>
        {:else}
          <div class="group flex items-start gap-1.5">
            <div class="flex-1 text-sm text-text whitespace-pre-wrap">{m.content}</div>
            {#if !busy}
              <!-- Edit pencil — visible on hover (desktop) and
                   always visible on touch (no hover) so mobile users
                   can still discover the affordance. Resubmits the
                   edited message, truncating everything after this
                   turn. -->
              <button
                type="button"
                onclick={() => onStartEditUser(i)}
                class="tap-target opacity-0 group-hover:opacity-100 focus:opacity-100 [@media(hover:none)]:opacity-100 inline-flex items-center justify-center w-7 h-7 rounded text-dim hover:text-text hover:bg-surface0 active:bg-surface1 transition-opacity"
                aria-label="Edit and resubmit"
                title="Edit this message and resubmit"
              >
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M12 20h9"/>
                  <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5z"/>
                </svg>
              </button>
            {/if}
          </div>
        {/if}
      {:else}
        {@const cleaned = stripStructuredBlocks(m.content || '')}
        {@const followups = busy && i === messages.length - 1 ? [] : parseFollowups(m.content || '')}
        {@const actions = busy && i === messages.length - 1 ? [] : parseActions(m.content || '')}
        {@const inflight = busy && i === messages.length - 1}
        <div class="prose prose-sm max-w-none">
          <MarkdownRenderer body={cleaned || '_…_'} />
          {#if inflight}
            <!-- Streaming caret. Inserted after the markdown so the
                 user has a continuous "still writing" signal between
                 chunks. Blinks at 1.06s so it reads as live
                 composition rather than a stuck cursor. -->
            <span class="ai-streaming-caret text-primary" aria-hidden="true"></span>
          {/if}
        </div>
        {#if actions.length > 0}
          <!-- Vault action chips proposed by the assistant — one tap
               creates the task / event / note / memory entry, with a
               confirmation toast. Each chip de-dupes itself after
               click via committedActions so a regen with the same
               proposal doesn't re-fire. -->
          <div class="mt-2 flex flex-wrap gap-1.5">
            {#each actions as a, ai (actionKey(i, a) + ai)}
              {@const k = actionKey(i, a)}
              {@const committed = !!committedActions[k]}
              <button
                type="button"
                onclick={() => onCommitAction(i, a)}
                disabled={committed}
                class="tap-target inline-flex items-center gap-1 px-2 py-1 rounded text-[11px] transition-colors {committed
                  ? 'bg-success text-on-primary cursor-default'
                  : 'bg-surface0 text-text hover:bg-surface1'}"
                title={committed ? 'Already committed' : 'Click to commit this action'}
              >
                {#if committed}✓{:else}+{/if}
                {#if a.type === 'task'}
                  Task: {a.text}{a.dueDate ? ` (${a.dueDate})` : ''}
                {:else if a.type === 'event'}
                  Event: {a.title} ({a.start.slice(11, 16)})
                {:else if a.type === 'note'}
                  Note: {a.title}
                {:else if a.type === 'remember'}
                  Remember: {a.content}
                {/if}
              </button>
            {/each}
          </div>
        {/if}
        {#if followups.length > 0}
          <!-- Suggested follow-ups — one tap dispatches the prompt
               through send(). Cheap to render: pure text parsing of
               the assistant's reply suffix. -->
          <div class="mt-2 flex flex-wrap gap-1.5">
            {#each followups as fu, fi (i + ':fu:' + fi)}
              <button
                type="button"
                onclick={() => onSendFollowup(fu)}
                class="tap-target inline-flex items-center gap-1 px-2 py-1 rounded text-[11px] bg-surface0 text-subtext hover:text-text hover:bg-surface1 transition-colors"
                title="Send as next message"
              >
                <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                  <path d="M5 12h14M13 5l7 7-7 7"/>
                </svg>
                {fu}
              </button>
            {/each}
          </div>
        {/if}
        {#if perTurnRagHits[i]?.length}
          <!-- Inline Sources for this turn — a collapsible strip
               below the assistant reply. The bottom attribution
               strip shows the most recent set; this lets the user
               scroll back through a long thread and see exactly
               which notes grounded each answer. -->
          <details
            open={!!expandedSources[i]}
            class="mt-2 text-[11px]"
            ontoggle={(e) => { expandedSources = { ...expandedSources, [i]: (e.currentTarget as HTMLDetailsElement).open }; }}
          >
            <summary class="cursor-pointer text-dim hover:text-text inline-flex items-center gap-1">
              <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M4 4h12l4 4v12H4z"/>
                <path d="M16 4v4h4M8 12h8M8 16h6" stroke-linecap="round"/>
              </svg>
              <span>Sources · {perTurnRagHits[i].length}</span>
            </summary>
            <ul class="mt-1.5 space-y-1.5 pl-4 border-l border-surface1">
              {#each perTurnRagHits[i] as h (h.path)}
                <li>
                  <a
                    href="/notes/{encodeURIComponent(h.path)}"
                    class="text-secondary hover:underline font-medium"
                    title={h.path}
                  >{h.title}</a>
                  <span class="text-dim font-mono ml-1.5 text-[10px]">{h.path}</span>
                  <p class="text-dim leading-snug mt-0.5 line-clamp-2">{h.excerpt}</p>
                </li>
              {/each}
            </ul>
          </details>
        {/if}
      {/if}
    </li>
  {/each}
</ul>
