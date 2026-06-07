<script lang="ts">
  // Sticky footer bar visible while bulk-select mode is active.
  // Renders four text buttons: Archive N · Set category… · Add tag… ·
  // Cancel. Each "…" button reveals a tiny inline input the user
  // commits with Enter. Stays on a single ~40px-tall row on
  // desktop; wraps on phones.
  //
  // Patch fan-out lives on the controller — this component only
  // owns the inline input drafts (category / tag) and the
  // open-which-input toggle, which would clutter the controller
  // if hoisted.

  import type { BulkSelectController } from '$lib/habits/habitsBulkSelect.svelte';

  type Props = {
    ctl: BulkSelectController;
  };

  let { ctl }: Props = $props();

  // Which inline input is open. Only one at a time so the bar
  // doesn't grow vertically when both are revealed.
  let openInput = $state<'category' | 'tag' | null>(null);
  let categoryDraft = $state('');
  let tagDraft = $state('');

  async function commitCategory(e?: Event) {
    e?.preventDefault();
    const v = categoryDraft.trim();
    if (!v) return;
    await ctl.setCategoryForSelected(v);
    categoryDraft = '';
    openInput = null;
  }
  async function commitTag(e?: Event) {
    e?.preventDefault();
    const v = tagDraft.trim();
    if (!v) return;
    await ctl.addTagToSelected(v);
    tagDraft = '';
    openInput = null;
  }
</script>

{#if ctl.active}
  <!-- Sticky footer — z-index above the page content but below
       modal dialogs (z-50). Tinted surface so the bar reads as
       chrome distinct from habit cards. -->
  <div
    class="sticky bottom-0 left-0 right-0 z-40 bg-mantle/95 backdrop-blur border-t border-surface2 px-4 py-2 -mx-4 sm:-mx-6 lg:-mx-8 mt-6"
    role="toolbar"
    aria-label="bulk habit actions"
  >
    <div class="flex flex-wrap items-center gap-2 max-w-5xl mx-auto">
      <span class="text-xs text-dim font-mono tabular-nums">
        {ctl.count} selected
      </span>
      <span class="flex-1 min-w-0"></span>

      <button
        type="button"
        onclick={() => ctl.archiveSelected()}
        disabled={ctl.busy || ctl.count === 0}
        class="px-2 py-1 text-xs rounded border border-surface2 text-text hover:bg-surface1 disabled:opacity-50"
        title="archive every selected habit"
      >Archive {ctl.count}</button>

      {#if openInput === 'category'}
        <form onsubmit={commitCategory} class="flex items-center gap-1">
          <input
            bind:value={categoryDraft}
            placeholder="category…"
            class="px-2 py-1 text-xs bg-base border border-surface2 rounded text-text w-32 focus:outline-none focus:border-primary"
          />
          <button
            type="submit"
            disabled={ctl.busy || !categoryDraft.trim()}
            class="px-2 py-1 text-xs bg-primary text-on-primary rounded disabled:opacity-50"
          >set</button>
          <button
            type="button"
            onclick={() => { openInput = null; categoryDraft = ''; }}
            class="px-1.5 py-1 text-xs text-dim hover:text-text"
          >×</button>
        </form>
      {:else}
        <button
          type="button"
          onclick={() => { openInput = 'category'; categoryDraft = ''; }}
          disabled={ctl.busy || ctl.count === 0}
          class="px-2 py-1 text-xs rounded border border-surface2 text-text hover:bg-surface1 disabled:opacity-50"
        >Set category…</button>
      {/if}

      {#if openInput === 'tag'}
        <form onsubmit={commitTag} class="flex items-center gap-1">
          <input
            bind:value={tagDraft}
            placeholder="tag…"
            class="px-2 py-1 text-xs bg-base border border-surface2 rounded text-text w-32 focus:outline-none focus:border-primary"
          />
          <button
            type="submit"
            disabled={ctl.busy || !tagDraft.trim()}
            class="px-2 py-1 text-xs bg-primary text-on-primary rounded disabled:opacity-50"
          >add</button>
          <button
            type="button"
            onclick={() => { openInput = null; tagDraft = ''; }}
            class="px-1.5 py-1 text-xs text-dim hover:text-text"
          >×</button>
        </form>
      {:else}
        <button
          type="button"
          onclick={() => { openInput = 'tag'; tagDraft = ''; }}
          disabled={ctl.busy || ctl.count === 0}
          class="px-2 py-1 text-xs rounded border border-surface2 text-text hover:bg-surface1 disabled:opacity-50"
        >Add tag…</button>
      {/if}

      <button
        type="button"
        onclick={() => ctl.cancel()}
        class="px-2 py-1 text-xs rounded text-dim hover:text-text"
        title="leave bulk-select mode"
      >Cancel</button>
    </div>
  </div>
{/if}
