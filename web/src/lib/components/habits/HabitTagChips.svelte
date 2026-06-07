<!--
  HabitTagChips — row of "#tag ×" chips plus a "+ tag" adder.

  Reads/writes through a TagEditController instance owned by the
  parent route. Two modes:

   • Closed: renders each existing tag as a static "#tag" chip plus
     a "+ tag" button. Click the button to open the editor.
   • Open  (ctl.openFor === habitName): renders the working draft
     as chips with × buttons, an autocomplete input for the next
     tag, and Save/Cancel.

  Visual language: even smaller than category chips — text-[10px] —
  to keep dense rows scannable. No coloured borders; tags are
  many-per-habit so accent noise would hurt more than help.
-->
<script lang="ts">
  import type { TagEditController } from '$lib/habits/habitsTagEdit.svelte';
  import { focusOnMount } from '$lib/util/focusOnMount';

  type Props = {
    habitName: string;
    tags: string[] | undefined;
    ctl: TagEditController;
  };
  let { habitName, tags, ctl }: Props = $props();

  let isOpen = $derived(ctl.openFor === habitName);
  let displayTags = $derived(tags ?? []);

  function onAddKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      const t = ctl.addText.trim();
      if (t) {
        ctl.addTag(t);
      } else {
        // Empty input + Enter commits the whole draft. Saves a click
        // when the user just removed chips and is done.
        void ctl.commit(habitName);
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      ctl.cancel();
    } else if (e.key === 'Backspace' && ctl.addText === '' && ctl.draft.length > 0) {
      // Standard chip-input affordance: backspace on an empty field
      // pops the last chip so the user can edit it in.
      e.preventDefault();
      const last = ctl.draft[ctl.draft.length - 1];
      ctl.removeTag(last);
      ctl.addText = last;
    }
  }
</script>

{#if isOpen}
  <span class="inline-flex items-center flex-wrap gap-1 text-[10px]">
    {#each ctl.draft as t (t)}
      <span class="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded-full bg-surface1 text-text border border-surface2">
        #{t}
        <button
          type="button"
          onclick={() => ctl.removeTag(t)}
          class="text-dim hover:text-error leading-none"
          aria-label="remove tag"
        >×</button>
      </span>
    {/each}
    <input
      bind:value={ctl.addText}
      use:focusOnMount
      onkeydown={onAddKeydown}
      list="habit-tag-suggestions"
      placeholder="add tag…"
      class="px-1.5 py-0.5 bg-base border border-surface2 rounded text-text text-[10px] w-24 focus:outline-none focus:border-primary"
    />
    <datalist id="habit-tag-suggestions">
      {#each ctl.tags as t (t)}
        <option value={t}></option>
      {/each}
    </datalist>
    <button
      type="button"
      onclick={() => void ctl.commit(habitName)}
      disabled={ctl.busy}
      class="px-1.5 py-0.5 bg-primary text-on-primary rounded text-[10px] disabled:opacity-50"
    >save</button>
    <button
      type="button"
      onclick={() => ctl.cancel()}
      class="px-1.5 py-0.5 text-dim hover:text-text text-[10px]"
    >cancel</button>
  </span>
{:else}
  <span class="inline-flex items-center flex-wrap gap-1 text-[10px]">
    {#each displayTags as t (t)}
      <span
        class="inline-flex items-center px-1.5 py-0.5 rounded-full bg-surface1 text-subtext border border-surface2"
        title="tag"
      >#{t}</span>
    {/each}
    <button
      type="button"
      onclick={() => ctl.open(habitName, displayTags)}
      class="inline-flex items-center px-1.5 py-0.5 rounded-full border bg-surface1 text-dim border-surface2 hover:text-text uppercase tracking-wider"
      title="add / edit tags"
    >+ tag</button>
  </span>
{/if}
