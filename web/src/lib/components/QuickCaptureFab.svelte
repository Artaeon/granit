<script lang="ts">
  import { api, todayISO } from '$lib/api';
  import { onMount } from 'svelte';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { invalidateTitleCache } from '$lib/editor/wikilinks';
  import VoiceNoteModal from '$lib/components/VoiceNoteModal.svelte';

  // QuickCaptureFab — global floating-action-button rendered from
  // +layout.svelte once the user is authed. Single keyboard shortcut
  // (Mod-Shift-N) toggles a small modal that captures a task into
  // today's daily note.
  //
  // Why a FAB instead of inline forms scattered everywhere: capture
  // should be one keystroke from anywhere in the app, with no context
  // switch — the user is on /goals when an idea hits, presses
  // Mod-Shift-N, types, and is back where they were a second later.
  // Mounted at the layout level once so every authed page gets it.
  //
  // Today this captures a task to today's daily note. A "jot" mode
  // (free-text bullet append, no parser) was scoped out for this
  // commit because the server has no /jots POST endpoint yet — adding
  // one is a clean separate change. For now, jots happen in the daily
  // note's editor directly.

  let open = $state(false);
  let voiceOpen = $state(false);
  let text = $state('');
  let priority = $state(0);
  let dueDate = $state('');
  let recurrence = $state<'' | 'daily' | 'weekly' | 'monthly' | '3x-week'>('');
  let saving = $state(false);
  let inputEl: HTMLInputElement | undefined = $state();

  function show() {
    open = true;
    // Defer focus to next tick so the input is mounted in the DOM.
    queueMicrotask(() => inputEl?.focus());
  }
  function hide() {
    open = false;
    // Reset transient form state but keep the mode preference so
    // re-opening lands where the user last was.
    text = '';
    priority = 0;
    dueDate = '';
    recurrence = '';
  }

  // Global keybind. We attach to the document so the shortcut works
  // even when no input is focused. Mod-Shift-N = "new" with a
  // discriminator so we don't collide with Mod-N (browser → new
  // window/tab) or Mod-Shift-T (browser → reopen tab).
  onMount(() => {
    function onKey(e: KeyboardEvent) {
      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.shiftKey && (e.key === 'N' || e.key === 'n')) {
        e.preventDefault();
        if (open) hide();
        else show();
      } else if (mod && e.shiftKey && (e.key === 'V' || e.key === 'v')) {
        // Mod-Shift-V — voice note. Same chord shape as the
        // capture sibling so it's discoverable; V for voice. The
        // browser's "paste without formatting" is also Mod-Shift-V
        // but only inside a contenteditable — we preventDefault so
        // it doesn't compete in plain page chrome. Inside text
        // inputs the chord still produces a paste because input
        // elements consume the keydown before our document listener
        // sees it.
        const target = e.target as HTMLElement | null;
        const inField = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable);
        if (!inField) {
          e.preventDefault();
          voiceOpen = true;
        }
      } else if (open && e.key === 'Escape') {
        hide();
      }
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  });

  // Daily note path (`Daily/YYYY-MM-DD.md`) — server-side handler
  // creates the file if missing. We don't pre-fetch the daily here;
  // the createTask + jot endpoints both auto-materialise.
  function dailyPath(): string {
    return `Daily/${todayISO()}.md`;
  }

  async function submit(e: Event) {
    e.preventDefault();
    if (!text.trim()) return;
    saving = true;
    try {
      const created = await api.createTask({
        notePath: dailyPath(),
        text: text.trim(),
        priority: priority || undefined,
        dueDate: dueDate || undefined,
        section: '## Tasks'
      });
      // Recurrence isn't part of the create payload (server applies it
      // as a marker via PATCH). One round-trip if the user picked a
      // recurrence, zero otherwise. Best-effort — the task itself
      // exists either way and the user can always set recurrence from
      // the task detail panel.
      if (recurrence) {
        try {
          await api.patchTask(created.id, { recurrence });
        } catch {
          /* ignore — primary capture succeeded */
        }
      }
      toast.success('task captured');
      // The daily note may have been auto-created by the createTask
      // path. Drop the title cache so wikilink + autolink suggestions
      // pick it up on the next typed phrase.
      invalidateTitleCache();
      hide();
    } catch (err) {
      toast.error('capture failed: ' + (errorMessage(err)));
    } finally {
      saving = false;
    }
  }
</script>

<!-- Floating button. Hidden at sm breakpoint and below by default
     because the mobile bottom-nav already crowds the bottom-right; on
     mobile users can still trigger via the keyboard shortcut on a
     paired keyboard, or we surface it differently in a follow-up.
     The button itself sits one nav-row above the bottom edge so the
     FAB doesn't collide with system home-bar gestures. -->
<!-- Capture cluster: primary "+" plus a smaller voice "🎤" satellite.
     Both desktop-only (mobile gets the bottom-nav). The voice button
     sits a row above the primary so the cluster reads as related but
     the primary is unambiguous. -->
<div class="hidden md:flex flex-col items-end gap-2 fixed bottom-5 right-5 z-30">
  <button
    type="button"
    onclick={() => (voiceOpen = true)}
    aria-label="voice note (Ctrl+Shift+V)"
    title="voice note (Ctrl+Shift+V)"
    class="w-10 h-10 flex items-center justify-center rounded-full bg-mantle border border-surface1 text-subtext shadow hover:bg-surface0 hover:text-primary transition-all hover:scale-105"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
      <rect x="9" y="3" width="6" height="12" rx="3"/>
      <path d="M5 11a7 7 0 0014 0M12 18v3" stroke-linecap="round"/>
    </svg>
  </button>
  <button
    type="button"
    onclick={show}
    aria-label="quick capture (Ctrl+Shift+N)"
    title="quick capture (Ctrl+Shift+N)"
    class="w-12 h-12 flex items-center justify-center rounded-full bg-primary text-on-primary shadow-lg hover:opacity-90 transition-all hover:scale-105"
  >
    <svg viewBox="0 0 24 24" class="w-6 h-6" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
      <path d="M12 5v14M5 12h14"/>
    </svg>
  </button>
</div>

<VoiceNoteModal bind:open={voiceOpen} />

{#if open}
  <!-- Backdrop. Click anywhere outside the panel closes. -->
  <div
    class="fixed inset-0 z-50 bg-black/50 flex items-end sm:items-start sm:pt-[15vh] justify-center sm:p-4"
    role="dialog"
    aria-label="quick capture"
    onclick={hide}
    onkeydown={(e) => { if (e.key === 'Escape') hide(); }}
    tabindex="-1"
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      class="w-full max-w-lg bg-mantle border border-surface1 rounded-t-lg sm:rounded-lg shadow-2xl overflow-hidden"
      role="document"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
    >
      <header class="flex items-center border-b border-surface1 px-4 py-2">
        <h2 class="text-sm font-medium text-text flex-1">Quick capture</h2>
        <button
          type="button"
          onclick={hide}
          aria-label="close"
          class="text-dim hover:text-text"
        >×</button>
      </header>

      <form onsubmit={submit} class="p-4 space-y-3">
        <input
          bind:this={inputEl}
          bind:value={text}
          required
          placeholder="what needs doing?"
          class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary"
        />

        <div class="flex flex-wrap items-center gap-2 text-xs">
          <select
            bind:value={priority}
            class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm"
          >
            <option value={0}>no priority</option>
            <option value={1}>!1 high</option>
            <option value={2}>!2 med</option>
            <option value={3}>!3 low</option>
          </select>
          <input
            type="date"
            bind:value={dueDate}
            class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm"
            aria-label="due date"
          />
          <select
            bind:value={recurrence}
            class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm"
            aria-label="recurrence"
          >
            <option value="">no repeat</option>
            <option value="daily">🔁 daily</option>
            <option value="weekly">🔁 weekly</option>
            <option value="monthly">🔁 monthly</option>
            <option value="3x-week">🔁 3×/week</option>
          </select>
        </div>

        <div class="flex items-center gap-2 pt-1">
          <span class="text-[11px] text-dim flex-1">
            Saves to today's daily note
            <kbd class="ml-1 text-[10px] font-mono px-1.5 py-0.5 bg-surface0 border border-surface1 rounded">Ctrl+Shift+N</kbd>
          </span>
          <button
            type="button"
            onclick={hide}
            class="px-3 py-1.5 text-sm text-subtext hover:text-text"
          >Cancel</button>
          <button
            type="submit"
            disabled={!text.trim() || saving}
            class="px-4 py-1.5 bg-primary text-on-primary rounded font-medium text-sm disabled:opacity-50"
          >
            {saving ? '…' : 'Capture'}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}
