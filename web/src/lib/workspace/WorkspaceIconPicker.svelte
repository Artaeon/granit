<!--
  WorkspaceIconPicker — small popover used by the WorkspacePill to
  let the user pick the active workspace's identity glyph. Renders a
  curated grid of NavIcons (the same set used across the BottomNav +
  pane registry) so a workspace's icon reads the same vocabulary as
  the panes inside it.

  Curated: ~24 icons cover the common workspace themes (workspace /
  daily routine / project work / planning / writing / money / spirit).
  Showing the full NavIcon catalog (50+ entries) felt overwhelming;
  any non-covered glyph is still applyable via the API by passing
  the name through a future command-palette entry.
-->
<script lang="ts">
  import NavIcon from '$lib/components/NavIcon.svelte';

  type Props = {
    /** Currently-selected NavIcon name. Highlighted in the grid. */
    current: string;
    onPick: (name: string) => void;
  };

  let { current, onPick }: Props = $props();

  const ICONS: string[] = [
    'workspace',
    'today',
    'tasks',
    'calendar',
    'goals',
    'notes',
    'habits',
    'deadline',
    'projects',
    'finance',
    'chat',
    'plans',
    'vision',
    'review',
    'scripture',
    'prayer',
    'virtues',
    'stats',
    'hub',
    'roots',
    'books',
    'jots',
    'ventures',
    'people'
  ];

  let open = $state(false);
  let rootEl: HTMLElement | null = $state(null);

  function pick(name: string) {
    open = false;
    onPick(name);
  }

  $effect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (rootEl && !rootEl.contains(e.target as Node)) open = false;
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') open = false;
    };
    window.addEventListener('mousedown', onDown);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  });
</script>

<span bind:this={rootEl} class="relative inline-flex items-center">
  <button
    type="button"
    onclick={() => (open = !open)}
    aria-haspopup="menu"
    aria-expanded={open}
    title="Change workspace icon"
    aria-label="Change workspace icon"
    class="inline-flex items-center justify-center px-1.5 h-full hover:bg-on-primary/10 transition-colors"
  >
    <NavIcon name={current} class="w-3.5 h-3.5" />
  </button>
  {#if open}
    <!-- Pops UP since the StatusBar lives at the bottom of the
         viewport. Right-anchored so a narrow viewport doesn't push
         the menu off the right edge when the workspace pills row
         is horizontally scrolled. -->
    <div
      role="menu"
      aria-label="Workspace icon"
      class="absolute right-0 bottom-full mb-1 w-56 bg-mantle border border-surface1 rounded-lg shadow-xl p-2 z-30"
    >
      <div class="text-[10px] uppercase tracking-wider text-dim font-mono mb-1.5 px-1">Icon</div>
      <div class="grid grid-cols-6 gap-1">
        {#each ICONS as name (name)}
          {@const active = current === name}
          <button
            type="button"
            role="menuitem"
            onclick={() => pick(name)}
            aria-current={active ? 'true' : undefined}
            title={name}
            aria-label={`Icon: ${name}`}
            class="inline-flex items-center justify-center w-7 h-7 rounded transition-colors
              {active ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface0 hover:text-text'}"
          >
            <NavIcon {name} class="w-4 h-4" />
          </button>
        {/each}
      </div>
    </div>
  {/if}
</span>
