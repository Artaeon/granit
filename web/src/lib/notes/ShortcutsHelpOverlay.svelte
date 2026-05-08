<script lang="ts">
  // Discoverable cheat-sheet for the editor's power-user keymap.
  // Triggered by `?` or Mod-/ from the note view; closes on Esc or
  // backdrop click. Pure presentation — no editor coupling, just a
  // static table that the host page wires to a `bind:open` flag.
  //
  // The shortcut list mirrors what's actually bound in the editor
  // (see $lib/editor/{markdown-shortcuts, checkbox-shortcuts,
  // extract-note}.ts) — when adding a new keymap, add a row here too
  // or the cheat sheet drifts. Manageable trade-off for keeping the
  // overlay decoupled from the editor's keymap data structures.

  interface Props {
    open: boolean;
    onClose: () => void;
  }
  let { open = $bindable(false), onClose }: Props = $props();

  // Mac users see ⌘ in the chord display; everyone else sees Ctrl.
  // The keybindings themselves use CodeMirror's `Mod-` which already
  // resolves to the right one — this is purely cosmetic.
  let modKey = $state('Ctrl');
  $effect(() => {
    if (typeof navigator === 'undefined') return;
    if (/Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent)) {
      modKey = '⌘';
    }
  });

  type Row = { keys: string; label: string };
  type Group = { title: string; rows: Row[] };

  let groups = $derived<Group[]>([
    {
      title: 'Formatting',
      rows: [
        { keys: `${modKey}+B`, label: 'Bold the selection' },
        { keys: `${modKey}+I`, label: 'Italic the selection' },
        { keys: `${modKey}+Shift+I`, label: 'Italic (underscore form)' },
        { keys: `${modKey}+K`, label: 'Wrap selection as a markdown link' },
        { keys: `${modKey}+\``, label: 'Inline code (toggle)' }
      ]
    },
    {
      title: 'Headings & Blocks',
      rows: [
        { keys: `${modKey}+Alt+1..6`, label: 'Set the line to a heading of that level' },
        { keys: `${modKey}+Alt+0`, label: 'Strip heading / list / quote prefix' },
        { keys: `${modKey}+Shift+8`, label: 'Toggle bullet list on the line' },
        { keys: `${modKey}+Shift+9`, label: 'Toggle blockquote on the line' }
      ]
    },
    {
      title: 'Tasks & Lists',
      rows: [
        { keys: `${modKey}+Shift+Enter`, label: 'Insert a checkbox at line start' },
        { keys: `${modKey}+Enter`, label: 'Toggle the checkbox on this line' }
      ]
    },
    {
      title: 'Linking & Capture',
      rows: [
        { keys: `[[`, label: 'Wikilink picker — autocomplete to any note' },
        { keys: `#`, label: 'Tag picker — autocomplete from your tag set' },
        { keys: `/`, label: 'Slash picker — snippets + /h1 /code /divider /table /note /tip /warning /danger …' },
        { keys: `${modKey}+Shift+X`, label: 'Extract selection to a new note (auto-link)' },
        { keys: `${modKey}+Shift+A`, label: 'Ask AI about the selection (summarise / improve / translate)' },
        { keys: `${modKey}+Alt+Space`, label: 'Continue writing — AI streams a continuation as ghost text. Tab to accept, Esc to reject.' },
        { keys: `${modKey}+click`, label: 'Open a wikilink in place' }
      ]
    },
    {
      title: 'Navigation',
      rows: [
        { keys: `${modKey}+S`, label: 'Save now (auto-save runs anyway)' },
        { keys: `${modKey}+F`, label: 'Find within the current note' },
        { keys: `${modKey}+G`, label: 'Find next' },
        { keys: `${modKey}+P`, label: 'Browser print (use Export PDF for branded output)' }
      ]
    },
    {
      title: 'View',
      rows: [
        { keys: `?`, label: 'Open this cheat sheet' },
        { keys: 'Esc', label: 'Close any modal or dropdown' }
      ]
    }
  ]);

  function dismiss() {
    onClose();
  }

  $effect(() => {
    if (!open) return;
    const h = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        dismiss();
      }
    };
    window.addEventListener('keydown', h);
    return () => window.removeEventListener('keydown', h);
  });
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 flex items-start justify-center pt-12 sm:pt-20 px-4 bg-mantle/70 backdrop-blur-sm"
    onclick={dismiss}
    role="presentation"
  >
    <section
      class="w-full max-w-2xl bg-base border border-surface1 rounded-lg shadow-xl max-h-[80vh] flex flex-col"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-label="Keyboard shortcuts"
    >
      <header class="px-4 py-3 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-sm font-semibold text-text flex-1">Keyboard shortcuts</h2>
        <span class="text-[11px] text-dim">Press <kbd class="kbd-inline">?</kbd> any time</span>
        <button
          onclick={dismiss}
          aria-label="close"
          class="text-dim hover:text-text text-lg leading-none ml-2"
        >×</button>
      </header>
      <div class="flex-1 overflow-y-auto p-4 space-y-5">
        {#each groups as g}
          <div>
            <h3 class="text-[11px] uppercase tracking-wider text-dim font-medium mb-2">{g.title}</h3>
            <ul class="space-y-1">
              {#each g.rows as r}
                <li class="flex items-baseline gap-3 py-0.5">
                  <kbd class="kbd-chord flex-shrink-0">{r.keys}</kbd>
                  <span class="text-sm text-text">{r.label}</span>
                </li>
              {/each}
            </ul>
          </div>
        {/each}
      </div>
      <footer class="px-4 py-2.5 border-t border-surface1 text-[11px] text-dim flex items-center gap-2">
        <span class="flex-1">Tip: keep one foot on the keyboard. Half of these are why this editor was built.</span>
      </footer>
    </section>
  </div>
{/if}

<style>
  .kbd-chord {
    display: inline-block;
    min-width: 3rem;
    text-align: center;
    padding: 0.125rem 0.5rem;
    background: var(--color-surface0);
    border: 1px solid var(--color-surface1);
    border-bottom-width: 2px;
    border-radius: 0.25rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.7rem;
    color: var(--color-text);
    white-space: nowrap;
  }
  .kbd-inline {
    display: inline-block;
    padding: 0 0.3em;
    background: var(--color-surface0);
    border: 1px solid var(--color-surface1);
    border-radius: 0.2rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.7rem;
  }
</style>
