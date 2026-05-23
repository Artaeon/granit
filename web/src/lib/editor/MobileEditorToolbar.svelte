<script lang="ts">
  import { onMount } from 'svelte';

  // MobileEditorToolbar — phone-only formatting bar that floats above
  // the on-screen keyboard while the editor is focused. Solves three
  // mobile pain points that the desktop SelectionToolbar can't:
  //
  //  1. Text selection on phones is fiddly (long-press + drag handles).
  //     A user mid-typing wants to insert a heading or checkbox at the
  //     cursor without selecting anything first.
  //  2. Mobile keyboards don't expose Mod / Alt chords — power-user
  //     shortcuts like Mod-Shift-8 (bullet) aren't reachable at all
  //     without an external keyboard. This bar surfaces them as taps.
  //  3. The SelectionToolbar is positioned above the selection, but
  //     on mobile the selection often sits just above the keyboard
  //     anyway — the toolbar would render BEHIND the keyboard. A
  //     bottom-anchored bar above `--kb-h` (set globally by
  //     +layout.svelte's visualViewport listener) sits in the visible
  //     strip every time.
  //
  // Architecture: the bar dispatches chords through the same
  // `dispatchChord` path SelectionToolbar uses, so behaviour stays
  // identical to the keymap. No parallel command implementations.
  //
  // Visibility: editor focus drives `focused`. We attach focus / blur
  // listeners to the editor's contentDOM (CodeMirror 6 uses a
  // contentEditable div, not a textarea) and gate the render on
  // mobile breakpoint via Tailwind's `md:hidden`.

  interface Props {
    /** The editor's CodeMirror contentDOM, from Editor.getDOM().
     *  Optional because the host page reads it after the editor
     *  mounts — we tolerate undefined and re-bind via $effect when
     *  it lands. */
    contentDOM: HTMLElement | undefined;
    /** Dispatch a chord into the editor. Same wiring as
     *  SelectionToolbar — synthesises a KeyboardEvent that the
     *  editor's keymap handles, so toolbar buttons take the same
     *  code path as desktop shortcuts. */
    onCommand: (chord: string) => void;
    /** Insert a literal string at the cursor. Used by the snippet
     *  buttons (checkbox, wikilink, tag) that aren't backed by a
     *  keymap chord. Mirrors the Editor's insertAtCursor signature. */
    onInsert: (text: string) => void;
  }

  let { contentDOM, onCommand, onInsert }: Props = $props();

  // Focus state — driven by listeners on the editor's contentDOM. We
  // re-attach whenever the prop lands or changes (HMR / editor remount).
  let focused = $state(false);

  $effect(() => {
    if (!contentDOM) return;
    const onFocus = () => { focused = true; };
    const onBlur = () => { focused = false; };
    contentDOM.addEventListener('focus', onFocus);
    contentDOM.addEventListener('blur', onBlur);
    // Bootstrap from current state in case the editor was already
    // focused when this effect runs.
    focused = document.activeElement === contentDOM || contentDOM.contains(document.activeElement);
    return () => {
      contentDOM.removeEventListener('focus', onFocus);
      contentDOM.removeEventListener('blur', onBlur);
    };
  });

  // Buttons. Two flavours:
  //   - chord: dispatches an existing keymap chord (bold/italic/etc.)
  //   - insert: literal text to slot at cursor (checkbox/wikilink/tag)
  // Order optimised for mobile thumb-reach: most-used left of less-used.
  // Glyphs are kept single-character so the row fits ~8 buttons on a
  // 360px viewport without horizontal scroll; the row is scrollable
  // anyway as a fallback (no ellipsis surprise).
  interface ChordAction { kind: 'chord'; label: string; chord: string; title: string }
  interface InsertAction { kind: 'insert'; label: string; insert: string; title: string }
  type Action = ChordAction | InsertAction;

  const ACTIONS: Action[] = [
    { kind: 'chord',  label: 'B',  chord: 'mod+b',         title: 'Bold' },
    { kind: 'chord',  label: 'I',  chord: 'mod+i',         title: 'Italic' },
    // Checkbox is the highest-value mobile insert — task capture is
    // the single most common phone use of the editor.
    { kind: 'insert', label: '☐',  insert: '- [ ] ',       title: 'Checkbox task' },
    // Wikilink: the [[ trigger opens the link picker on its own, so
    // tapping this button just types the two brackets and lets the
    // existing autocomplete take over.
    { kind: 'insert', label: '[[', insert: '[[',           title: 'Wikilink' },
    { kind: 'insert', label: '#',  insert: '#',            title: 'Tag' },
    { kind: 'chord',  label: 'H',  chord: 'mod+alt+2',     title: 'Heading 2' },
    { kind: 'chord',  label: '•',  chord: 'mod+shift+8',   title: 'Bullet list' },
    { kind: 'chord',  label: '"',  chord: 'mod+shift+q',   title: 'Blockquote' },
    { kind: 'chord',  label: '<>', chord: 'mod+`',         title: 'Inline code' },
    { kind: 'chord',  label: '🔗', chord: 'mod+k',         title: 'Link' },
    { kind: 'chord',  label: '✨', chord: 'mod+shift+a',   title: 'Ask AI about selection' }
  ];

  function run(a: Action) {
    if (a.kind === 'chord') onCommand(a.chord);
    else onInsert(a.insert);
  }
</script>

<!-- The bar mounts only on mobile (md:hidden) and only while the
     editor is focused. position: fixed; bottom: var(--kb-h, 0px)
     anchors it to just-above-the-keyboard — the same --kb-h variable
     +layout.svelte updates from the visualViewport API. When the
     keyboard is closed, --kb-h is 0 and the bar would sit at the
     viewport bottom, which is fine; but we gate the render on
     `focused` anyway so the bar isn't visible when not editing. -->
{#if focused}
  <div
    role="toolbar"
    aria-label="Editor formatting"
    class="md:hidden fixed left-0 right-0 z-40 bg-mantle border-t border-surface1 px-1.5 py-1.5 flex items-center gap-1 overflow-x-auto"
    style="bottom: var(--kb-h, 0px); padding-bottom: max(env(safe-area-inset-bottom), 0.375rem);"
  >
    {#each ACTIONS as a (a.label)}
      <button
        type="button"
        title={a.title}
        aria-label={a.title}
        onpointerdown={(e) => {
          // pointerdown (not click) so the button fires BEFORE the
          // editor loses focus. Clicking outside the contentDOM
          // briefly blurs it on iOS — by the time `click` lands, the
          // chord-dispatch would target an unfocused editor and the
          // keymap would no-op. preventDefault keeps focus where it is.
          e.preventDefault();
          run(a);
        }}
        onkeydown={(e) => {
          // Hardware-keyboard fallback (rare on phones but reachable
          // via an external Bluetooth keyboard on iPad / Android
          // tablets). Tab-focus + Enter / Space should still work.
          // pointerdown doesn't fire on keyboard activation, so without
          // this handler the button would visually highlight but do
          // nothing.
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            run(a);
          }
        }}
        class="w-10 h-9 flex-shrink-0 inline-flex items-center justify-center rounded text-sm text-subtext hover:text-text hover:bg-surface0 active:bg-surface1 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary transition-colors font-medium"
      >{a.label}</button>
    {/each}
  </div>
{/if}
