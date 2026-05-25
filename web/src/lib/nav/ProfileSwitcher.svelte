<script lang="ts">
  import { onMount } from 'svelte';
  import { profilesStore } from '$lib/stores/profiles';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';

  // Profile switcher chip for the nav footer. Renders only when the
  // store has loaded AND at least 2 profiles exist — a vault with just
  // "Classic" should see no switcher noise. Click opens an inline menu
  // listing every profile with the active one marked; tap a name to
  // activate.

  interface Props {
    /** When true, the parent nav is in compact (icon-only) mode and we
     *  render an icon chip with the profile initial. When false, full
     *  label + chevron. */
    isCompact?: boolean;
  }
  let { isCompact = false }: Props = $props();

  let menuOpen = $state(false);
  let busyId = $state<string | null>(null);

  onMount(() => {
    void profilesStore.ensureLoaded();
  });

  let active = $derived.by(() => {
    if (!$profilesStore.loaded) return null;
    return $profilesStore.profiles.find((p) => p.id === $profilesStore.activeId) ?? null;
  });

  // Hide when nothing to switch to. The store fallback returns an empty
  // list on fetch failure, so this also gracefully hides on API error.
  let shouldShow = $derived($profilesStore.loaded && $profilesStore.profiles.length > 1);

  async function activate(id: string) {
    if (id === $profilesStore.activeId) {
      menuOpen = false;
      return;
    }
    busyId = id;
    try {
      await profilesStore.activate(id);
      const name = $profilesStore.profiles.find((p) => p.id === id)?.name ?? id;
      toast.success(`Profile switched to ${name}`);
      menuOpen = false;
    } catch (e) {
      toast.error('Couldn\'t switch profile: ' + errorMessage(e));
    } finally {
      busyId = null;
    }
  }

  // Click-outside + Escape handlers. Inlined so the component doesn't
  // need a separate util import — single use case here.
  let rootEl: HTMLDivElement | undefined = $state();
  let triggerEl: HTMLButtonElement | undefined = $state();
  function onDocClick(e: MouseEvent) {
    if (!menuOpen) return;
    if (rootEl && !rootEl.contains(e.target as Node)) menuOpen = false;
  }
  function onDocKeydown(e: KeyboardEvent) {
    if (!menuOpen) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      menuOpen = false;
      // Return focus to the trigger so keyboard users land back where
      // they opened the menu from.
      triggerEl?.focus();
    }
  }
  onMount(() => {
    document.addEventListener('mousedown', onDocClick);
    document.addEventListener('keydown', onDocKeydown);
    return () => {
      document.removeEventListener('mousedown', onDocClick);
      document.removeEventListener('keydown', onDocKeydown);
    };
  });

  // Active profile initial — first letter of the profile name, used in
  // compact mode so the chip still communicates which profile is on.
  let activeInitial = $derived(active?.name?.charAt(0)?.toUpperCase() ?? '·');
</script>

{#if shouldShow}
  <div bind:this={rootEl} class="relative">
    {#if isCompact}
      <button
        bind:this={triggerEl}
        type="button"
        onclick={() => (menuOpen = !menuOpen)}
        title={active ? `Profile: ${active.name} — tap to switch` : 'Switch profile'}
        aria-label="switch profile"
        aria-expanded={menuOpen}
        aria-haspopup="menu"
        aria-controls="profile-switcher-menu"
        class="w-full flex justify-center items-center px-2 py-2 rounded text-sm font-mono text-primary hover:bg-surface0 transition-colors"
      >
        <span class="w-5 h-5 inline-flex items-center justify-center rounded-full border border-primary text-[11px]">{activeInitial}</span>
      </button>
    {:else}
      <button
        bind:this={triggerEl}
        type="button"
        onclick={() => (menuOpen = !menuOpen)}
        aria-expanded={menuOpen}
        aria-haspopup="menu"
        aria-controls="profile-switcher-menu"
        class="w-full flex items-center gap-3 px-3 py-1.5 rounded text-xs text-dim hover:bg-surface0 hover:text-subtext transition-colors"
      >
        <span class="w-4 h-4 inline-flex items-center justify-center rounded-full border border-dim text-[10px] font-mono flex-shrink-0">{activeInitial}</span>
        <span class="flex-1 text-left">
          <span class="block text-[10px] uppercase tracking-wider text-dim/70 leading-none">Profile</span>
          <span class="block text-sm text-subtext leading-tight truncate">{active?.name ?? '—'}</span>
        </span>
        <svg viewBox="0 0 24 24" class="w-3 h-3 flex-shrink-0 transition-transform {menuOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="6 9 12 15 18 9"/>
        </svg>
      </button>
    {/if}

    {#if menuOpen}
      <!-- Popover. Positioned absolutely so it doesn't push the rest of
           the nav around when it opens. Right-side anchored in compact
           mode so it doesn't clip off the left edge. -->
      <div
        id="profile-switcher-menu"
        class="absolute bottom-full mb-1 {isCompact ? 'left-full ml-2' : 'left-0 right-0'} bg-mantle border border-surface1 rounded-lg shadow-xl z-50 overflow-hidden"
        role="menu"
        aria-label="profiles"
      >
        <div class="px-3 py-1.5 text-[10px] uppercase tracking-wider text-dim border-b border-surface1">
          Switch profile
        </div>
        <ul class="max-h-72 overflow-y-auto">
          {#each $profilesStore.profiles as p (p.id)}
            {@const isActive = p.id === $profilesStore.activeId}
            {@const isBusy = busyId === p.id}
            <li>
              <button
                type="button"
                role="menuitem"
                onclick={() => activate(p.id)}
                disabled={isBusy}
                class="w-full flex items-start gap-2 px-3 py-2 text-left text-sm hover:bg-surface0 disabled:opacity-50 transition-colors {isActive ? 'bg-surface0' : ''}"
              >
                <span class="w-1 self-stretch rounded {isActive ? 'bg-primary' : 'bg-transparent'} flex-shrink-0"></span>
                <span class="flex-1 min-w-0">
                  <span class="block font-medium text-text">
                    {p.name}
                    {#if isActive}<span class="ml-1 text-[10px] uppercase tracking-wider text-primary">active</span>{/if}
                    {#if !p.builtIn}<span class="ml-1 text-[10px] text-dim">custom</span>{/if}
                  </span>
                  {#if p.description}
                    <span class="block text-[11px] text-dim leading-snug">{p.description}</span>
                  {/if}
                </span>
                {#if isBusy}
                  <span class="text-[11px] text-dim">…</span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
        <a
          href="/settings"
          onclick={() => (menuOpen = false)}
          class="block px-3 py-1.5 text-[11px] text-secondary hover:bg-surface0 border-t border-surface1"
        >Manage profiles in Settings →</a>
      </div>
    {/if}
  </div>
{/if}
