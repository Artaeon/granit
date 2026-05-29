<script lang="ts">
  import { tick } from 'svelte';
  import SlashCommandPicker from '$lib/components/SlashCommandPicker.svelte';
  import MentionPicker, { type MentionRef } from '$lib/components/MentionPicker.svelte';
  import type { VoiceDictation } from '$lib/chat/voiceDictation.svelte';

  // Composer — ChatGPT-style multiline input wrapped in a rounded
  // container, with mic + send anchored in the bottom-right corner
  // plus slash-command and mention pickers anchored below the input.
  //
  // Extracted from AIOverlay so the standalone /chat page and any
  // future surface (inline note menu, dashboard quick-capture) can
  // reuse the exact same input ergonomics — autosize-on-type, iOS
  // 16px-min font-size workaround, pickerKey routing, voice baseline
  // merging — without copying.
  //
  // Voice instance is owned by the parent (createVoiceDictation lives
  // there because empty-state starter buttons outside the composer
  // also dictate via the same `voice.toggle()`). The parent passes
  // the instance in; the composer reads `voice.recording` for the
  // button state and calls `voice.toggle()` on click.

  interface Props {
    /** Live composer text. Two-way bound; voice transcript and slash
     *  picks write directly into this. */
    input: string;
    /** Textarea DOM ref. Bound so the parent can refocus after
     *  starter-button clicks and other chrome interactions. */
    inputEl: HTMLTextAreaElement | undefined;
    /** Outer panel element — read for autosize cap (input grows up to
     *  ~50% of the panel's height before scrolling internally). */
    panelEl: HTMLElement | undefined;
    /** Slash picker open flag — bound so parent's Esc handler can
     *  close it without reaching into the composer. */
    slashPickerOpen: boolean;
    /** Mention picker open flag — same as above. */
    mentionPickerOpen: boolean;
    /** Slash picker component instance — bound so parent can call
     *  handleKey / detectTrigger / etc. from outside the composer. */
    slashPickerRef: SlashCommandPicker | undefined;
    /** Mention picker component instance — same as above, plus the
     *  empty-state "Reference an item" starter calls detectTrigger
     *  after writing '@' into input. */
    mentionPickerRef: MentionPicker | undefined;
    /** Voice dictation harness. Owned by parent so non-composer
     *  surfaces (empty-state starter strip) can read .supported and
     *  call .toggle() against the same instance. */
    voice: VoiceDictation;
    /** True while a chat send / quick action is in flight; disables
     *  the textarea + send button so the user can't double-fire. */
    busy: boolean;
    /** True when the Sabbath module is locking AI access. Disables
     *  send + shows a "paused" placeholder. */
    sabbathActive: boolean;
    /** Called when the form submits (Enter without Shift, or send
     *  button click). Parent owns the actual send() implementation. */
    onSubmit: () => void;
    /** Called when the mention picker finishes a pick. Parent
     *  appends the ref to its mentionedRefs $state. */
    onMentionPick: (ref: MentionRef) => void;
  }

  let {
    input = $bindable(),
    inputEl = $bindable(),
    panelEl,
    slashPickerOpen = $bindable(),
    mentionPickerOpen = $bindable(),
    slashPickerRef = $bindable(),
    mentionPickerRef = $bindable(),
    voice,
    busy,
    sabbathActive,
    onSubmit,
    onMentionPick
  }: Props = $props();

  // ── Composer keydown / change handlers ─────────────────────────
  // Mention + slash pickers swallow arrow/enter/tab while open so
  // the user navigates the popup before falling through to send-on-
  // enter. handleKey returns true when the picker swallows the
  // event; the slash picker also returns false on the exact-match-
  // Enter case so this fall-through still calls send().
  function onInputKey(e: KeyboardEvent) {
    if (mentionPickerRef?.handleKey(e)) return;
    if (slashPickerRef?.handleKey(e)) return;
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      onSubmit();
    }
  }

  function onInputChange() {
    slashPickerRef?.detectTrigger();
    mentionPickerRef?.detectTrigger();
    autosizeInput();
  }

  // Caret moved without typing — re-evaluate mention/slash context.
  function onInputClick() {
    slashPickerRef?.detectTrigger();
    mentionPickerRef?.detectTrigger();
  }

  // ── Autosize ──────────────────────────────────────────────────
  // textarea[rows=2] is the resting height; as the user types
  // newlines (or pastes a multi-line prompt) we expand the element
  // up to ~50% of the panel's height before falling back to
  // internal scrolling. Reset to height:auto so scrollHeight reads
  // the natural content height, then clamp + write back. Cheap
  // (one layout read per keystroke).
  function autosizeInput() {
    if (!inputEl) return;
    const ta = inputEl;
    const panel = panelEl?.getBoundingClientRect().height ?? window.innerHeight;
    const cap = Math.max(120, Math.floor(panel * 0.5));
    ta.style.height = 'auto';
    const next = Math.min(cap, ta.scrollHeight);
    ta.style.height = next + 'px';
    ta.style.overflowY = ta.scrollHeight > cap ? 'auto' : 'hidden';
  }

  // Re-run autosize on every input mutation (typing, voice, slash
  // pick, mention pick). $effect tracks `input` as a dep so any
  // programmatic write — voice transcript, mention insert, /help —
  // also triggers a resize without relying on individual call sites.
  $effect(() => {
    void input;
    tick().then(() => autosizeInput());
  });
</script>

<!-- Chat input. ChatGPT-style composer: textarea wraps the full
     width, mic + send sit as icon buttons inside the bottom-right
     corner. Disabled during Sabbath. -->
<form
  onsubmit={(e) => { e.preventDefault(); onSubmit(); }}
  class="border-t border-surface1 px-3 py-3 flex-shrink-0"
>
  <div
    class="relative bg-surface0 border rounded-2xl px-3 py-2 transition-colors {voice.recording ? 'border-error' : 'border-surface1 focus-within:border-primary'}"
  >
    <!-- font-size: 16px on mobile is CRITICAL. iOS Safari auto-
         zooms any focused input with font-size < 16px and, while
         zooming, scrolls the page to centre the input — dragging
         fixed-positioned ancestors (the AI panel) up with it.
         text-base md:text-sm gives 16px on mobile (no iOS zoom)
         + 14px on desktop (matches the rest of the chat). -->
    <textarea
      bind:this={inputEl}
      bind:value={input}
      onkeydown={onInputKey}
      oninput={onInputChange}
      onclick={onInputClick}
      rows="2"
      placeholder={sabbathActive ? 'Sabbath active — AI paused' : voice.recording ? 'Listening… speak freely' : 'Ask anything, /help for commands, @ to reference…'}
      disabled={busy || sabbathActive}
      class="w-full bg-transparent border-0 text-base md:text-sm text-text placeholder-dim focus:outline-none resize-none disabled:opacity-60 pr-20"
      style="min-height: 2.5rem; max-height: 12rem;"
    ></textarea>
    <!-- Bottom-right action cluster — mic (optional) + send.
         Anchored inside the textarea wrapper so the input grows
         vertically while the buttons stay pinned to its corner. -->
    <div class="absolute right-2 bottom-2 flex items-center gap-1">
      {#if voice.supported}
        <button
          type="button"
          onclick={() => voice.toggle()}
          disabled={busy || sabbathActive}
          aria-pressed={voice.recording}
          class="w-8 h-8 inline-flex items-center justify-center rounded-full disabled:opacity-40 transition-colors {voice.recording ? 'bg-error text-white animate-pulse' : 'text-subtext hover:bg-surface1 hover:text-text'}"
          title={voice.recording ? 'Stop dictating' : 'Dictate'}
          aria-label={voice.recording ? 'Stop dictating' : 'Dictate'}
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="9" y="3" width="6" height="12" rx="3"/>
            <path d="M5 11a7 7 0 0014 0M12 18v3"/>
          </svg>
        </button>
      {/if}
      <button
        type="submit"
        disabled={busy || !input.trim() || sabbathActive}
        aria-label="Send"
        title="Send (Enter)"
        class="w-8 h-8 inline-flex items-center justify-center rounded-full bg-primary text-on-primary disabled:opacity-30 hover:opacity-90 active:opacity-80 transition-opacity"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 19V5M5 12l7-7 7 7"/>
        </svg>
      </button>
    </div>
    <!-- Slash-command + mention pickers. Rendered as children of
         the wrapper so their popovers anchor to the input box. -->
    <SlashCommandPicker
      bind:this={slashPickerRef}
      bind:value={input}
      bind:open={slashPickerOpen}
      {inputEl}
      onSubmit={onSubmit}
    />
    {#if !slashPickerOpen}
      <MentionPicker
        bind:this={mentionPickerRef}
        bind:value={input}
        bind:open={mentionPickerOpen}
        {inputEl}
        onPick={onMentionPick}
      />
    {/if}
  </div>
</form>
