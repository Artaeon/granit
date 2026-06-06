<script lang="ts">
  // Scrolling results body for the command palette.
  //
  // Renders the grouped CmdItem list plus the loading / empty state.
  // Pulled out of CommandPalette.svelte so the shell shows the dialog
  // chrome and lets this component own the per-row layout (icon /
  // label / hint / detail) + the hover-tracks-selection wiring.
  //
  // Selection is two-way bound — the parent owns the cursor index so
  // the keyboard install can move it from anywhere; this component
  // both reads it (to highlight + position scroll) and writes it on
  // mouseenter so hover and arrow-key cursors stay in sync.
  //
  // Header offset is computed inline because grouped items already
  // come pre-sorted; turning it into a $derived would add a hop with
  // no measurable benefit on 80-row lists.

  import NavIcon from '../NavIcon.svelte';
  import type { CmdItem, Group } from './paletteTypes';

  interface Props {
    grouped: { group: Group; items: CmdItem[] }[];
    items: CmdItem[];
    selected: number;
    dataLoaded: boolean;
    onSelect: (idx: number) => void;
    onInvoke: (item: CmdItem) => void;
  }

  let {
    grouped,
    items,
    selected,
    dataLoaded,
    onSelect,
    onInvoke
  }: Props = $props();

  // Flat-index offset for the first row of group `gIdx`. Reads
  // grouped from the prop on each call — cheap, and avoids a
  // $derived that would re-fire for every selected change.
  function offset(gIdx: number): number {
    let s = 0;
    for (let i = 0; i < gIdx; i++) s += grouped[i].items.length;
    return s;
  }
</script>

{#if items.length === 0}
  <div class="px-4 py-6 text-sm text-dim">
    {dataLoaded ? 'no matches' : 'loading…'}
  </div>
{:else}
  {#each grouped as g, gIdx (g.group)}
    <div class="px-4 pt-2 pb-0.5 text-[10px] uppercase tracking-wider text-dim flex items-center gap-1.5">
      <span>{g.group}</span>
      <!-- Hit count — at-a-glance density signal. The user
           reads "PAGES 32 · CONTENT 8" and knows whether to
           keep typing or Tab into a denser bucket. -->
      <span class="text-dim/70 font-mono normal-case">({g.items.length})</span>
    </div>
    <ul>
      {#each g.items as it, iIdx (it.id)}
        {@const flat = offset(gIdx) + iIdx}
        <li>
          <button
            data-cmd-idx={flat}
            onclick={() => onInvoke(it)}
            onmouseenter={() => onSelect(flat)}
            class="w-full text-left px-4 py-1.5 flex items-baseline gap-2.5 {selected === flat ? 'bg-surface1' : ''}"
          >
            <span class="w-5 h-5 flex items-center justify-center text-dim flex-shrink-0">
              <NavIcon name={it.icon} class="w-4 h-4" />
            </span>
            <span class="flex-1 min-w-0 truncate text-text text-sm">{it.label}</span>
            {#if it.hint}
              <kbd class="text-[10px] text-dim font-mono px-1.5 py-0.5 bg-surface0 border border-surface1 rounded flex-shrink-0">{it.hint}</kbd>
            {/if}
            {#if it.detail}
              <span class="hidden sm:inline text-xs text-dim font-mono truncate max-w-[40%]">{it.detail}</span>
            {/if}
          </button>
        </li>
      {/each}
    </ul>
  {/each}
{/if}
