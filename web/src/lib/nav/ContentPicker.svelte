<script lang="ts">
  // Dropdown picker for the right-pane's 10 content options. Owns the
  // open/close state, click-outside + Escape handling, and focus
  // return to the trigger button on select. Lifted out of RightPane
  // so the shell stays a layout file; the picker is its own concern.
  //
  // The current chord is rendered on the right of each menu row,
  // resolved at render-time via findBinding so chord drift in the
  // registry surfaces here immediately. `isMac` is computed once on
  // mount — the platform doesn't change mid-session.
  //
  // The trigger button + menu live inside a single relatively
  // positioned wrapper so the menu's `absolute left-0 top-full` lands
  // directly below the trigger regardless of header layout above.
  import { onMount, tick } from 'svelte';
  import { findBinding } from '$lib/keybindings/registry';
  import type { RightPaneContent } from '$lib/stores/rightPane';
  import {
    RIGHT_PANE_OPTIONS,
    displayChord,
    type RightPaneContentOption
  } from './rightPaneContentMeta';

  let {
    current,
    onSelect
  }: {
    current: RightPaneContent;
    onSelect: (id: RightPaneContent) => void;
  } = $props();

  let open = $state(false);
  let buttonEl: HTMLButtonElement | undefined = $state();
  let menuEl: HTMLDivElement | undefined = $state();

  let isMac = $state(false);
  onMount(() => {
    isMac =
      typeof navigator !== 'undefined' &&
      /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent);
  });

  let currentOption = $derived.by<RightPaneContentOption>(
    () => RIGHT_PANE_OPTIONS.find((o) => o.id === current) ?? RIGHT_PANE_OPTIONS[0]
  );

  function toggle() {
    open = !open;
  }
  async function pick(c: RightPaneContent) {
    onSelect(c);
    open = false;
    // Tick before refocusing so the menu collapse doesn't compete
    // with focus restoration on the trigger.
    await tick();
    buttonEl?.focus();
  }

  // Outside-click + Escape close the menu. Only attached while open so
  // a closed picker doesn't keep stray document listeners alive.
  $effect(() => {
    if (!open) return;
    function onDoc(ev: MouseEvent) {
      const t = ev.target as Node | null;
      if (!t) return;
      if (menuEl && menuEl.contains(t)) return;
      if (buttonEl && buttonEl.contains(t)) return;
      open = false;
    }
    function onKey(ev: KeyboardEvent) {
      if (ev.key === 'Escape') {
        open = false;
        buttonEl?.focus();
      }
    }
    document.addEventListener('mousedown', onDoc);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDoc);
      document.removeEventListener('keydown', onKey);
    };
  });
</script>

<div class="relative">
  <button
    bind:this={buttonEl}
    type="button"
    onclick={toggle}
    aria-haspopup="menu"
    aria-expanded={open}
    title={currentOption.title}
    class="flex items-center gap-1.5 px-2 py-1 rounded text-sm text-text hover:bg-surface0 transition-colors"
  >
    <svg viewBox="0 0 24 24" class="w-4 h-4 text-primary" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      {@html currentOption.iconPath}
    </svg>
    <span>{currentOption.label}</span>
    <svg viewBox="0 0 24 24" class="w-3 h-3 text-dim" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <polyline points="6 9 12 15 18 9"/>
    </svg>
  </button>

  {#if open}
    <div
      bind:this={menuEl}
      role="menu"
      class="absolute left-0 top-full mt-1 z-20 min-w-[14rem] bg-surface0 border border-surface1 rounded shadow-lg py-1"
    >
      {#each RIGHT_PANE_OPTIONS as opt (opt.id)}
        {@const binding = findBinding(opt.bindingId)}
        {@const active = current === opt.id}
        <button
          role="menuitem"
          type="button"
          onclick={() => pick(opt.id)}
          title={opt.title}
          class="flex items-center gap-2 w-full px-2 py-1.5 text-left text-sm transition-colors
            {active ? 'bg-surface1 text-primary' : 'text-text hover:bg-surface1'}"
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0 {active ? 'text-primary' : 'text-dim'}" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            {@html opt.iconPath}
          </svg>
          <span class="flex-1 truncate">{opt.label}</span>
          {#if binding}
            <span class="text-[10px] text-dim tabular-nums flex-shrink-0">{displayChord(binding.keys, isMac)}</span>
          {/if}
        </button>
      {/each}
    </div>
  {/if}
</div>
