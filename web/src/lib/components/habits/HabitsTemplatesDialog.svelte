<script lang="ts">
  // Templates dialog. Minimal list-of-templates with a click-to-
  // preview that reveals the habits inside, plus "Add all" to fan
  // out createHabit calls in parallel. No card grid eye-candy —
  // just text rows, in line with POWERUI.
  //
  // Modal framing copied from EditModal.svelte (backdrop + Esc on
  // window + click-outside dismissal). We don't reuse the EditModal
  // component itself because the consumer-style children snippet
  // would force the apply button into the slot, which complicates
  // the disabled-state wiring during applying. Inlining a thin
  // backdrop is cheaper than the props churn.

  import { onMount } from 'svelte';
  import type { TemplatesDialogController } from '$lib/habits/habitsTemplatesDialog.svelte';

  type Props = {
    ctl: TemplatesDialogController;
  };

  let { ctl }: Props = $props();

  // Esc-to-close. Listens on the window so the key still fires when
  // focus lives inside a focusable preview row (the row is a
  // button — Esc on it would be swallowed by some browsers
  // otherwise).
  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!ctl.open) return;
      if (e.key === 'Escape') {
        e.preventDefault();
        ctl.close();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
</script>

{#if ctl.open}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={() => ctl.close()}
    role="dialog"
    aria-modal="true"
    aria-label="Habit templates"
    tabindex="-1"
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      role="document"
      class="w-full max-w-md bg-mantle border border-surface1 rounded-lg shadow-xl max-h-[80vh] flex flex-col"
    >
      <header class="px-4 py-2.5 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-base font-semibold text-text">Habit templates</h2>
        <span class="text-[11px] text-dim">curated starter sets</span>
        <span class="flex-1"></span>
        <button
          type="button"
          onclick={() => ctl.close()}
          class="text-dim hover:text-text text-sm"
          aria-label="close templates dialog"
        >×</button>
      </header>

      <div class="overflow-y-auto flex-1">
        <ul class="divide-y divide-surface1">
          {#each ctl.templates as tpl (tpl.id)}
            {@const isPreview = ctl.previewId === tpl.id}
            <li>
              <button
                type="button"
                onclick={() => ctl.selectPreview(tpl.id)}
                class="w-full text-left px-4 py-2.5 hover:bg-surface0 transition-colors"
                aria-expanded={isPreview}
              >
                <div class="flex items-baseline gap-2">
                  <span class="text-sm font-medium text-text">{tpl.name}</span>
                  <span class="text-[11px] text-dim">{tpl.habits.length} habits</span>
                  <span class="flex-1"></span>
                  <span class="text-[11px] text-dim">
                    {isPreview ? '▾' : '▸'}
                  </span>
                </div>
                <p class="text-[11px] text-dim mt-0.5 leading-snug">
                  {tpl.description}
                </p>
              </button>
              {#if isPreview}
                <div class="px-4 pb-3 bg-surface0/60">
                  <ul class="space-y-1 mt-1">
                    {#each tpl.habits as item (item.name)}
                      <li class="text-[12px] text-text flex items-center gap-2">
                        <span class="text-dim">·</span>
                        <span class="flex-1">{item.name}</span>
                        {#if item.category}
                          <span class="text-[10px] uppercase tracking-wider text-secondary/80">
                            {item.category}
                          </span>
                        {/if}
                        {#if item.frequency}
                          <span class="text-[10px] text-dim font-mono">
                            {item.frequency}
                          </span>
                        {/if}
                      </li>
                    {/each}
                  </ul>
                  <div class="mt-3 flex items-center gap-2">
                    <button
                      type="button"
                      onclick={() => ctl.applyTemplate(tpl.id)}
                      disabled={ctl.applying}
                      class="px-3 py-1.5 bg-primary text-on-primary rounded text-xs font-medium disabled:opacity-50"
                    >
                      {ctl.applying ? 'creating…' : `Add all ${tpl.habits.length}`}
                    </button>
                    <span class="text-[11px] text-dim">
                      adds every habit above; you can archive any after
                    </span>
                  </div>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      </div>

      <footer class="px-4 py-2 border-t border-surface1 flex items-center justify-end gap-2">
        <button
          type="button"
          onclick={() => ctl.close()}
          class="px-3 py-1 text-sm text-dim hover:text-text"
        >Close</button>
      </footer>
    </div>
  </div>
{/if}
