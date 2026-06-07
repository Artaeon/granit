<!--
  HabitCategoryChip — small "● Category" chip plus inline-edit popover.

  Reads/writes through a CategoryEditController instance owned by the
  parent route. When closed it renders a chip (or a "+ category"
  affordance when unset); when this habit owns the open editor, it
  swaps in a free-text input + datalist of known categories.

  Visual language matches the existing habit-card metadata row: text-xs,
  rounded-full, 1px border tinted by a deterministic hash of the
  category name so the same string always gets the same accent across
  the page. No new palette — the hash picks one of the existing
  Catppuccin-flavoured CSS vars the rest of the app already uses.
-->
<script lang="ts">
  import type { CategoryEditController } from '$lib/habits/habitsCategoryEdit.svelte';
  import { focusOnMount } from '$lib/util/focusOnMount';

  type Props = {
    habitName: string;
    category: string | undefined;
    ctl: CategoryEditController;
  };
  let { habitName, category, ctl }: Props = $props();

  // Deterministic colour-of-category. Picks one of seven semantic CSS
  // vars based on a simple djb2 hash so the same category always
  // tints the same. Keeps the chip glanceable without ballooning the
  // palette.
  const ACCENTS = [
    'primary',
    'secondary',
    'accent',
    'info',
    'success',
    'warning',
    'error'
  ];
  function accentVar(name: string): string {
    let h = 5381;
    for (let i = 0; i < name.length; i++) h = ((h << 5) + h + name.charCodeAt(i)) | 0;
    const idx = Math.abs(h) % ACCENTS.length;
    return `var(--color-${ACCENTS[idx]})`;
  }

  let isOpen = $derived(ctl.openFor === habitName);

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      void ctl.commit(habitName);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      ctl.cancel();
    }
  }
</script>

{#if isOpen}
  <span class="inline-flex items-center gap-1 text-[11px]">
    <input
      bind:value={ctl.draft}
      use:focusOnMount
      onkeydown={onKeydown}
      list="habit-category-suggestions"
      placeholder="category…"
      class="px-1.5 py-0.5 bg-base border border-surface2 rounded text-text text-[11px] w-32 focus:outline-none focus:border-primary"
    />
    <datalist id="habit-category-suggestions">
      {#each ctl.categories as c (c)}
        <option value={c}></option>
      {/each}
    </datalist>
    <button
      type="button"
      onclick={() => void ctl.commit(habitName)}
      disabled={ctl.busy}
      class="px-1.5 py-0.5 bg-primary text-on-primary rounded text-[11px] disabled:opacity-50"
    >save</button>
    <button
      type="button"
      onclick={() => ctl.cancel()}
      class="px-1.5 py-0.5 text-dim hover:text-text text-[11px]"
    >cancel</button>
  </span>
{:else if category}
  <button
    type="button"
    onclick={() => ctl.open(habitName, category)}
    class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-full text-[10px] uppercase tracking-wider border bg-surface0 hover:bg-surface1"
    style="color: {accentVar(category)}; border-color: {accentVar(category)};"
    title="category — click to edit"
  >
    <span class="w-1.5 h-1.5 rounded-full" style="background: {accentVar(category)};"></span>
    {category}
  </button>
{:else}
  <button
    type="button"
    onclick={() => ctl.open(habitName, '')}
    class="inline-flex items-center px-1.5 py-0.5 rounded-full text-[10px] uppercase tracking-wider border bg-surface1 text-dim border-surface2 hover:text-text"
    title="set a category"
  >+ category</button>
{/if}
