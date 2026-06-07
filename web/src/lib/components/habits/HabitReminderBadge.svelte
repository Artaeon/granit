<script lang="ts">
  // ⏰ Reminder badge — small clickable chip showing the habit's
  // reminderTime (HH:MM) or a "+ rmd" affordance when none is set.
  // Click opens an inline <input type="time"> right on the card —
  // no popover, no modal. Enter saves; Esc / blur cancels.
  //
  // Empty state deliberately shows a low-contrast "+ rmd" instead
  // of nothing so a power user discovers the surface; first-time
  // users on dense cards skim past it.

  import { focusOnMount } from '$lib/util/focusOnMount';
  import type { ReminderEditController } from '$lib/habits/habitsReminderEdit.svelte';

  let {
    name,
    reminderTime,
    ctl
  }: {
    name: string;
    reminderTime: string | undefined;
    ctl: ReminderEditController;
  } = $props();

  let open = $derived(ctl.editing === name);

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      ctl.cancel();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      void ctl.submit(name);
    }
  }
</script>

{#if open}
  <span class="inline-flex items-center gap-1 text-xs">
    <span class="text-dim" aria-hidden="true">⏰</span>
    <input
      type="time"
      bind:value={ctl.draft}
      use:focusOnMount
      onkeydown={onKeydown}
      onblur={() => void ctl.submit(name)}
      disabled={ctl.busy}
      class="px-1 py-0.5 bg-base border border-primary rounded text-text text-xs font-mono disabled:opacity-50"
      aria-label="reminder time for {name}"
    />
    <button
      type="button"
      onmousedown={(e) => { e.preventDefault(); ctl.cancel(); }}
      class="px-1 text-dim hover:text-text text-[11px]"
      title="cancel"
      aria-label="cancel reminder edit"
    >×</button>
    {#if ctl.error}
      <span class="text-[10px] text-error">{ctl.error}</span>
    {/if}
  </span>
{:else if reminderTime}
  <button
    type="button"
    onclick={() => ctl.start(name, reminderTime)}
    class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] border border-surface2 bg-surface0 text-subtext hover:text-text hover:bg-surface1"
    title="reminder time — click to edit"
  >
    <span aria-hidden="true">⏰</span>
    <span class="font-mono">{reminderTime}</span>
  </button>
{:else}
  <button
    type="button"
    onclick={() => ctl.start(name, '')}
    class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border bg-surface1 text-dim border-surface2 hover:text-text"
    title="set a daily reminder time"
  >+ rmd</button>
{/if}
