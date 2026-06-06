<!--
  AIOverlayEmptyBody — shown when there's no chat thread, no
  quick-action result, just a fresh overlay. Surfaces three power-
  user shortcuts so a new user knows the surface is denser than it
  looks (slash commands, @-references, voice dictation), then a
  keyboard-shortcut footer.

  Tappable starter chips pre-fill the composer + nudge the user
  toward the right next action:

    • See what's here — `/help`, fires immediately so the help text
                        renders in the chat list.
    • Reference an item — `@`, triggers the MentionPicker.
    • Dictate          — only when the device supports SpeechRecognition.
-->
<script lang="ts">
  type Props = {
    voiceSupported: boolean;
    onSendHelp: () => void;
    onStartMention: () => void;
    onToggleVoice: () => void;
  };

  let { voiceSupported, onSendHelp, onStartMention, onToggleVoice }: Props = $props();
</script>

<div class="text-xs text-dim leading-relaxed">
  <p class="mb-3 text-text">Ask anything, or pick a starter.</p>
  <div class="flex flex-wrap gap-1.5 mb-3">
    <button
      type="button"
      onclick={onSendHelp}
      class="tap-target px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary text-[11px] transition-colors"
    >See what's here</button>
    <button
      type="button"
      onclick={onStartMention}
      class="tap-target px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary text-[11px] transition-colors"
    >Reference an item</button>
    {#if voiceSupported}
      <button
        type="button"
        onclick={onToggleVoice}
        class="tap-target px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary text-[11px] transition-colors"
      >Dictate</button>
    {/if}
  </div>
  <p class="text-[11px] leading-relaxed">
    <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">Mod+J</kbd> toggle ·
    <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">/</kbd> commands ·
    <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">@</kbd> entities ·
    <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">Mod+1..9</kbd> mode
  </p>
</div>
