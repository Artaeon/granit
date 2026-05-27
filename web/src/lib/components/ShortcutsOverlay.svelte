<script lang="ts">
  // Global-shortcuts cheat sheet. Pulls rows from the central
  // KEYBINDINGS registry so it can't drift — adding a chord there
  // surfaces it here automatically. The note-specific overlay
  // ($lib/notes/ShortcutsHelpOverlay) stays for editor-only keymaps
  // (markdown formatting, completion picker etc.) that don't make
  // sense outside the editor surface.
  //
  // Triggered via `?` from +layout.svelte. Esc / backdrop click
  // closes. Skips the listener while focus is inside an input so
  // `?` in a text field types a question mark instead of opening
  // the overlay (the layout's keydown handler is what enforces
  // that — this component just reacts to its `open` prop).

  import { KEYBINDINGS, bindingMatchesRoute, type KeyBinding } from '$lib/keybindings/registry';
  import { page } from '$app/stores';
  import { fly, fade } from 'svelte/transition';

  interface Props {
    open: boolean;
    onClose: () => void;
  }
  let { open = $bindable(false), onClose }: Props = $props();

  // Display the chord with the right modifier glyph per platform.
  // Mac users see ⌘ + ⇧, everyone else sees Ctrl / Shift.
  let isMac = $state(false);
  $effect(() => {
    if (typeof navigator === 'undefined') return;
    isMac = /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent);
  });

  function displayKeys(keys: string): string[] {
    return keys.split('+').map((p) => {
      const t = p.trim();
      if (t === 'Mod') return isMac ? '⌘' : 'Ctrl';
      if (t === 'Shift') return isMac ? '⇧' : 'Shift';
      if (t === 'Alt') return isMac ? '⌥' : 'Alt';
      if (t === 'Ctrl') return isMac ? '⌃' : 'Ctrl';
      return t;
    });
  }

  // Group bindings into three buckets: per-page (matches active
  // route), global (fires everywhere including inputs), and app-shell
  // (everywhere except inside text inputs). Bindings with a `route`
  // filter only ever appear in the "Current page" section so a chord
  // like `s` (snooze cursor task on /tasks) doesn't confuse the user
  // when they're on /notes.
  let pathname = $derived($page.url.pathname);
  let groups = $derived(() => {
    const pageRows: KeyBinding[] = [];
    const global: KeyBinding[] = [];
    const app: KeyBinding[] = [];
    for (const b of KEYBINDINGS) {
      if (b.route) {
        if (bindingMatchesRoute(b, pathname)) pageRows.push(b);
        continue;
      }
      if (b.scope === 'global') global.push(b);
      else app.push(b);
    }
    return [
      { title: 'Current page', rows: pageRows },
      { title: 'Global · works while typing too', rows: global },
      { title: 'App-shell · pauses while inside an input', rows: app }
    ];
  });

  $effect(() => {
    if (!open) return;
    const h = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener('keydown', h);
    return () => window.removeEventListener('keydown', h);
  });
</script>

{#if open}
  <div
    transition:fade={{ duration: 120 }}
    class="fixed inset-0 z-50 flex items-end sm:items-start justify-center sm:pt-12 md:pt-20 sm:px-4 bg-black/60"
    onclick={onClose}
    role="presentation"
  >
    <div
      transition:fly={{ y: 16, duration: 180 }}
      class="w-full sm:max-w-xl bg-base border border-surface1 rounded-t-xl sm:rounded-lg shadow-xl max-h-[88dvh] sm:max-h-[80dvh] flex flex-col"
      role="dialog"
      aria-modal="true"
      aria-label="Keyboard shortcuts"
      tabindex="-1"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
    >
      <header class="px-3 py-2 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-sm font-semibold text-text flex-1">Keyboard shortcuts</h2>
        <span class="hidden sm:inline text-[11px] text-dim">
          <kbd class="px-1 py-0.5 bg-surface1 border border-surface2 rounded text-[10px] font-mono">?</kbd>
          opens this anytime
        </span>
        <button
          onclick={onClose}
          aria-label="close"
          class="text-dim hover:text-text text-lg leading-none px-1"
        >×</button>
      </header>
      <div class="flex-1 overflow-y-auto p-3 space-y-4">
        {#each groups() as g (g.title)}
          {#if g.rows.length > 0}
            <section>
              <h3 class="text-[10px] uppercase tracking-wider text-dim font-medium mb-2">{g.title}</h3>
              <ul class="divide-y divide-surface1/50">
                {#each g.rows as row (row.id)}
                  <li class="flex items-baseline gap-3 py-1.5">
                    <span class="flex items-center gap-1 flex-shrink-0">
                      {#each displayKeys(row.keys) as k, i (i)}
                        {#if i > 0}
                          <span class="text-dim text-[10px]">+</span>
                        {/if}
                        <kbd class="px-1.5 py-0.5 bg-surface1 border border-surface2 rounded text-[11px] font-mono text-text">{k}</kbd>
                      {/each}
                    </span>
                    <span class="flex-1 min-w-0">
                      <span class="text-sm text-text">{row.label}</span>
                      {#if row.description}
                        <span class="block text-[11px] text-dim leading-snug mt-0.5">{row.description}</span>
                      {/if}
                    </span>
                  </li>
                {/each}
              </ul>
            </section>
          {/if}
        {/each}
        <p class="text-[11px] text-dim pt-2 border-t border-surface1">
          Editor-specific shortcuts (markdown formatting, slash picker,
          wikilink autocomplete) live in the note-view cheat sheet —
          open a note and press <kbd class="px-1 py-0.5 bg-surface1 border border-surface2 rounded text-[10px] font-mono">?</kbd>
          there.
        </p>
      </div>
    </div>
  </div>
{/if}
