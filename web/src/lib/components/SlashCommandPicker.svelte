<script lang="ts" module>
  // Slash-command spec — exported so consumers can describe new commands
  // (none for now, but the type is part of the public surface). The
  // master command list lives inside the component so the picker UI is
  // the single source of truth.
  export interface SlashSpec {
    cmd: string;
    desc: string;
    /** When true, the picker keeps the picker open after typing the
     *  command so the user can supply an argument. */
    hasArg?: boolean;
  }
</script>

<script lang="ts">
  import { tick } from 'svelte';

  // SlashCommandPicker — the dropdown that appears when the user types
  // a leading "/" in the AI overlay's composer. Owns the filter logic,
  // the keyboard navigation, and the picker UI. Hands the dispatch
  // decision back to the parent via onSubmit() (fired when an argless
  // command is picked / Enter-confirmed) — the parent then routes to
  // its actual send()/handleSlashCommand() pipeline.
  //
  // Extracted from AIOverlay.svelte. The keydown chain there is:
  //   mention picker (highest precedence) → slash picker → fall-through
  //     (Enter sends, etc.). handleKey() below preserves that contract:
  // returns true when the picker swallowed the event, false when the
  // parent should keep handling it. For the exact-match-on-Enter case
  // (user finished typing the command name themselves) we close the
  // picker AND return false so the parent's send() runs and routes
  // through handleSlashCommand() as before.

  interface Props {
    /** Two-way bound to the parent composer's textarea value. */
    value: string;
    /** Two-way bound to the parent's open flag so Esc-from-parent and
     *  outside-click integrations can both flip it. */
    open: boolean;
    /** The actual <textarea> element — needed to read selection start
     *  and to refocus after a pick. */
    inputEl: HTMLTextAreaElement | undefined;
    /** Fired when the picker resolves to a "send this now" decision
     *  (argless command picked from the list). The parent's send()
     *  pipeline then runs the value through handleSlashCommand(). */
    onSubmit: () => void;
  }
  let {
    value = $bindable(),
    open = $bindable(),
    inputEl,
    onSubmit
  }: Props = $props();

  const COMMANDS: SlashSpec[] = [
    { cmd: '/help', desc: 'Show all slash commands + AI surfaces' },
    { cmd: '/clear', desc: 'Reset the current conversation (saves to history first)' },
    { cmd: '/new', desc: 'Start a new thread (current one is saved)' },
    { cmd: '/save', desc: 'Save the current thread as a markdown note' },
    { cmd: '/briefing', desc: 'Daily briefing — today\'s events + tasks' },
    { cmd: '/synopsis', desc: 'Weekly synopsis — Wins / Setbacks / Learned / Next' },
    { cmd: '/triage', desc: 'Run inbox triage on untriaged tasks' },
    { cmd: '/deadlines', desc: 'Detect deadlines in untimed tasks' },
    { cmd: '/mode', desc: 'Switch agent mode (general, research, writer, coach, analyst, architect)', hasArg: true },
    { cmd: '/persona', desc: 'Switch persona (lewis, aurelius, socrates, chrysostom, founder, magister, examen)', hasArg: true },
    { cmd: '/rag', desc: 'Toggle RAG retrieval for the next turn' },
    { cmd: '/forget', desc: 'Drop snapshot/note attachment + queued mentions' },
    { cmd: '/detach', desc: 'Drop the snapshot/note attachment (legacy alias of /forget)' }
  ];
  let filtered = $state<SlashSpec[]>([]);
  let selectedIdx = $state(0);

  // Trigger when the input starts with "/" and the caret is somewhere
  // in the first whitespace-free token. Subsequent tokens (the argument
  // to /mode / /persona) close the picker. Exposed so the parent can
  // call this from oninput AND from onclick (caret moved without
  // typing).
  export function detectTrigger() {
    if (!inputEl) return;
    const v = value;
    if (!v.startsWith('/')) {
      open = false;
      return;
    }
    const caret = inputEl.selectionStart ?? v.length;
    const firstSpace = v.indexOf(' ');
    if (firstSpace !== -1 && caret > firstSpace) {
      open = false;
      return;
    }
    const token = firstSpace === -1 ? v : v.slice(0, firstSpace);
    const tl = token.toLowerCase();
    filtered = COMMANDS.filter((s) => s.cmd.startsWith(tl));
    if (filtered.length === 0) {
      open = false;
      return;
    }
    open = true;
    selectedIdx = Math.min(selectedIdx, filtered.length - 1);
  }

  function pick(s: SlashSpec) {
    value = s.cmd + (s.hasArg ? ' ' : '');
    open = false;
    if (!s.hasArg) {
      // Fire immediately for argless commands so the picker doubles as
      // a "type slash, click command" power-tool.
      tick().then(() => onSubmit());
    } else {
      // Leave the picker dismissed; let the user type the arg.
      tick().then(() => {
        if (inputEl) {
          inputEl.focus();
          const pos = value.length;
          inputEl.setSelectionRange(pos, pos);
        }
      });
    }
  }

  // Keyboard handler. Returns true when the event was swallowed by the
  // picker; false when the parent should keep processing (notably: the
  // exact-match-on-Enter case, where the user has already typed the
  // full command name themselves and the parent's send() should run).
  export function handleKey(e: KeyboardEvent): boolean {
    if (!open || filtered.length === 0) return false;
    // Enter — special-case for fully-typed commands so a trailing
    // Enter doesn't autocomplete the user's own typing. Mirrors the
    // pre-extraction in-overlay behaviour exactly.
    if (e.key === 'Enter' && !e.shiftKey) {
      const sel = filtered[selectedIdx];
      if (sel && sel.cmd === value.trim().split(/\s+/)[0].toLowerCase()) {
        // Exact match — close the picker and let the parent's send()
        // route through handleSlashCommand().
        open = false;
        return false;
      }
      if (sel) {
        e.preventDefault();
        pick(sel);
        return true;
      }
      return false;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selectedIdx = (selectedIdx + 1) % filtered.length;
      return true;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      selectedIdx = (selectedIdx - 1 + filtered.length) % filtered.length;
      return true;
    }
    if (e.key === 'Tab') {
      const s = filtered[selectedIdx];
      if (s) {
        e.preventDefault();
        // Tab autocompletes the command WITHOUT firing — useful for
        // /mode and /persona where the user still needs an arg.
        value = s.cmd + (s.hasArg ? ' ' : '');
        open = false;
        tick().then(() => {
          if (inputEl) {
            inputEl.focus();
            const pos = value.length;
            inputEl.setSelectionRange(pos, pos);
          }
        });
        return true;
      }
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      open = false;
      return true;
    }
    return false;
  }
</script>

{#if open}
  <!-- Slash-command picker. Triggers when input starts with "/" and the
       caret is in the first token. Same nav UX as the mention picker.
       Picker is mutually exclusive with the mention picker (slash always
       wins because the input must start with /). -->
  <div
    role="listbox"
    class="absolute left-0 right-0 bottom-full mb-1 bg-mantle border border-surface1 rounded-lg shadow-xl z-30 max-h-64 overflow-y-auto"
  >
    {#each filtered as s, i (s.cmd)}
      <button
        type="button"
        role="option"
        aria-selected={i === selectedIdx}
        onmousedown={(e) => { e.preventDefault(); pick(s); }}
        onmouseenter={() => (selectedIdx = i)}
        class="w-full flex items-baseline gap-2 px-3 py-1.5 text-left hover:bg-surface0 {i === selectedIdx ? 'bg-surface0' : ''}"
      >
        <span class="text-xs font-mono text-primary flex-shrink-0">{s.cmd}</span>
        {#if s.hasArg}
          <span class="text-[10px] text-secondary">+arg</span>
        {/if}
        <span class="text-[11px] text-dim truncate flex-1">{s.desc}</span>
      </button>
    {/each}
  </div>
{/if}
