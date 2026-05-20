<script lang="ts">
  import { onMount } from 'svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { modulesStore } from '$lib/stores/modules';
  import { sections as navSections, type NavItem } from '$lib/nav/config';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';

  // /settings/features — feature toggle page that mirrors the
  // sidebar's structure so the user can ask "what's in my Daily
  // section?" rather than scanning a flat module list. Backed by the
  // same modulesStore + /api/v1/modules endpoint the old tab used;
  // logic lifted verbatim so behaviour stays identical.

  let pendingModulePatch: Record<string, boolean> = $state({});
  let moduleSaveTimer: ReturnType<typeof setTimeout> | null = null;
  let moduleSaving = $state(false);

  onMount(() => {
    void modulesStore.ensureLoaded();
    return () => {
      // Flush any unsent toggles on unmount so the user's last click
      // doesn't get lost when they navigate away within the 350ms
      // debounce window.
      if (moduleSaveTimer) {
        clearTimeout(moduleSaveTimer);
        void commitModulePatch();
      }
    };
  });

  function queueModuleToggle(id: string, enabled: boolean) {
    pendingModulePatch[id] = enabled;
    pendingModulePatch = { ...pendingModulePatch };
    if (moduleSaveTimer) clearTimeout(moduleSaveTimer);
    moduleSaveTimer = setTimeout(commitModulePatch, 350);
  }

  async function commitModulePatch() {
    if (Object.keys(pendingModulePatch).length === 0) return;
    const patch = pendingModulePatch;
    pendingModulePatch = {};
    moduleSaving = true;
    try {
      await modulesStore.set(patch);
      toast.success('Features updated');
    } catch (e) {
      toast.error(errorMessage(e));
      void modulesStore.refresh();
    } finally {
      moduleSaving = false;
    }
  }

  // Build a (sectionLabel, items[]) view of the nav so we can render
  // the toggles in the same order the user reads them in the sidebar.
  // Items without a moduleId are "always on" (core surfaces — Tasks,
  // Calendar, Notes, etc.) and rendered with a lock indicator.
  type Row = {
    item: NavItem;
    modEntry: { id: string; name: string; description: string; enabled: boolean } | null;
    isCore: boolean;
  };
  type SectionView = { id: string; label: string; rows: Row[] };

  let view = $derived.by<SectionView[]>(() => {
    const moduleById = new Map($modulesStore.modules.map((m) => [m.id, m]));
    return navSections.map((s) => ({
      id: s.id,
      label: s.label,
      rows: s.items.map((item): Row => {
        if (!item.moduleId) {
          return { item, modEntry: null, isCore: true };
        }
        const m = moduleById.get(item.moduleId);
        if (!m) {
          // Unknown module id — treat as core so the user doesn't
          // see a broken-looking entry. Logged for debugging.
          return { item, modEntry: null, isCore: true };
        }
        return { item, modEntry: m, isCore: false };
      })
    }));
  });

  // Bulk actions per section. "Enable all" / "Disable all" matters
  // because the user typically wants to ramp a whole section at once
  // (e.g. "I don't use the Life section at all"). Iterates through
  // toggleable items only — cores stay on.
  function bulkSetSection(s: SectionView, enabled: boolean) {
    for (const r of s.rows) {
      if (r.modEntry && r.modEntry.enabled !== enabled) {
        queueModuleToggle(r.modEntry.id, enabled);
      }
    }
  }

  function enabledCount(s: SectionView): { enabled: number; total: number } {
    let enabled = 0;
    let total = 0;
    for (const r of s.rows) {
      if (r.isCore) continue;
      total++;
      const queued = r.modEntry ? pendingModulePatch[r.modEntry.id] : undefined;
      const isOn = queued !== undefined ? queued : (r.modEntry?.enabled ?? false);
      if (isOn) enabled++;
    }
    return { enabled, total };
  }
</script>

<svelte:head>
  <title>Features · Granit</title>
</svelte:head>

<div class="max-w-3xl mx-auto p-3 sm:p-6">
  <PageHeader
    title="Features"
    subtitle="Hide what you don't use. Disabled features stay on disk — re-enable any time."
  />

  <div class="mb-4 flex items-baseline gap-3 text-sm">
    <a href="/settings" class="text-secondary hover:underline">← Back to Settings</a>
    {#if moduleSaving}
      <span class="text-[11px] uppercase tracking-wider text-dim">saving…</span>
    {/if}
  </div>

  {#if !$modulesStore.loaded}
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 space-y-2">
      <Skeleton class="h-4 w-1/3 mb-2" />
      <Skeleton class="h-4 w-full" />
      <Skeleton class="h-4 w-3/4" />
    </section>
  {:else}
    {#each view as section (section.id)}
      {@const counts = enabledCount(section)}
      <section class="bg-surface0 border border-surface1 rounded-lg p-3 sm:p-4 mb-3">
        <header class="flex items-baseline gap-3 mb-3">
          <h2 class="text-sm font-semibold text-text uppercase tracking-wider">{section.label}</h2>
          {#if counts.total > 0}
            <span class="text-[11px] text-dim font-mono tabular-nums">{counts.enabled}/{counts.total} on</span>
          {/if}
          <span class="flex-1"></span>
          {#if counts.total > 0}
            <!-- Bulk row toggles. Only meaningful when the section
                 has at least one toggleable item; pure-core sections
                 (Today on its own would be one) show nothing. -->
            <button
              type="button"
              onclick={() => bulkSetSection(section, true)}
              disabled={counts.enabled === counts.total}
              class="text-[11px] text-secondary hover:underline disabled:opacity-40 disabled:no-underline disabled:cursor-default"
            >all on</button>
            <button
              type="button"
              onclick={() => bulkSetSection(section, false)}
              disabled={counts.enabled === 0}
              class="text-[11px] text-warning hover:underline disabled:opacity-40 disabled:no-underline disabled:cursor-default"
            >all off</button>
          {/if}
        </header>

        <ul class="space-y-1">
          {#each section.rows as row (row.item.href)}
            {@const queued = row.modEntry ? pendingModulePatch[row.modEntry.id] : undefined}
            {@const checked = row.isCore ? true : (queued !== undefined ? queued : (row.modEntry?.enabled ?? false))}
            <li>
              <label
                class="flex items-start gap-3 py-2 px-1 rounded transition-colors {row.isCore ? 'opacity-70 cursor-not-allowed' : 'cursor-pointer hover:bg-mantle/50'}"
              >
                <input
                  type="checkbox"
                  {checked}
                  disabled={row.isCore}
                  onchange={(e) => {
                    if (row.modEntry) {
                      queueModuleToggle(row.modEntry.id, (e.target as HTMLInputElement).checked);
                    }
                  }}
                  class="w-4 h-4 mt-0.5 accent-primary {row.isCore ? 'cursor-not-allowed' : 'cursor-pointer'}"
                />
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2 text-sm text-text">
                    <span class="font-medium">{row.item.label}</span>
                    <code class="text-[10px] text-dim font-mono">{row.item.href}</code>
                    {#if row.isCore}
                      <span class="text-[10px]" title="Always on — core surface">🔒</span>
                    {/if}
                  </div>
                  {#if row.modEntry}
                    <div class="text-[11px] text-dim leading-snug mt-0.5">{row.modEntry.description}</div>
                  {:else if row.isCore}
                    <div class="text-[11px] text-dim leading-snug mt-0.5">Always on — can't disable.</div>
                  {/if}
                </div>
              </label>
            </li>
          {/each}
        </ul>
      </section>
    {/each}

    <!-- Catch-all for modules that don't map to a sidebar section
         (e.g. a backend-only module, or a future module registered
         after a release). Without this, a new module appearing in
         the API response would be invisible to the user. -->
    {@const navModuleIds = new Set(navSections.flatMap((s) => s.items.map((i) => i.moduleId).filter(Boolean)))}
    {@const orphans = $modulesStore.modules.filter((m) => !navModuleIds.has(m.id))}
    {#if orphans.length > 0}
      <section class="bg-surface0 border border-surface1 rounded-lg p-3 sm:p-4 mb-3">
        <header class="mb-3">
          <h2 class="text-sm font-semibold text-text uppercase tracking-wider">Other</h2>
          <p class="text-[11px] text-dim mt-1">Backend modules not tied to a sidebar entry.</p>
        </header>
        <ul class="space-y-1">
          {#each orphans as m (m.id)}
            {@const queued = pendingModulePatch[m.id]}
            {@const checked = queued !== undefined ? queued : m.enabled}
            <li>
              <label class="flex items-start gap-3 py-2 px-1 rounded cursor-pointer hover:bg-mantle/50">
                <input
                  type="checkbox"
                  {checked}
                  onchange={(e) => queueModuleToggle(m.id, (e.target as HTMLInputElement).checked)}
                  class="w-4 h-4 mt-0.5 accent-primary cursor-pointer"
                />
                <div class="flex-1 min-w-0">
                  <div class="text-sm text-text font-medium">{m.name}</div>
                  <div class="text-[11px] text-dim leading-snug mt-0.5">{m.description}</div>
                </div>
              </label>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
</div>
