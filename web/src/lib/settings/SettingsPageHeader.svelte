<script lang="ts">
  // Slim header for /settings. Mirrors the TasksPageHeader pattern:
  // page title on the left, segmented icon+label tab strip on the
  // right. No subtitle — each tab makes its own purpose obvious.
  // Sticky so the tabs stay reachable while a long section scrolls.
  export type SettingsTab = 'general' | 'ai' | 'sync' | 'vault';

  type Props = {
    tab: SettingsTab;
    onSelect: (t: SettingsTab) => void;
  };

  let { tab, onSelect }: Props = $props();

  // Icon glyphs are intentionally minimal — one stroke each so they
  // read as hints, not illustrations. Order keeps the most-touched
  // tabs first (General > AI > Sync > Vault).
  const TABS: { id: SettingsTab; label: string; title: string; icon: string }[] = [
    {
      id: 'general',
      label: 'General',
      title: 'Theme, profile, daily notes, editor behavior',
      // Cog wheel — generic settings.
      icon: 'M12 8a4 4 0 1 1 0 8 4 4 0 0 1 0-8z M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1 1.55V21a2 2 0 0 1-4 0v-.09A1.7 1.7 0 0 0 9 19.4a1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-1.55-1H3a2 2 0 0 1 0-4h.09A1.7 1.7 0 0 0 4.6 9a1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-1.55V3a2 2 0 0 1 4 0v.09A1.7 1.7 0 0 0 15 4.6a1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.7 1.7 0 0 0 19.4 9a1.7 1.7 0 0 0 1.55 1H21a2 2 0 0 1 0 4h-.09a1.7 1.7 0 0 0-1.55 1z'
    },
    {
      id: 'ai',
      label: 'AI',
      title: 'AI features, provider, audit, web research',
      // Spark — AI.
      icon: 'M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5z M19 16l.7 2 .7-2 .7 2 M5 16l.7 2 .7-2 .7 2'
    },
    {
      id: 'sync',
      label: 'Sync',
      title: 'Reminders, git auto-sync, devices, integrations',
      // Circular arrows — sync.
      icon: 'M21 12a9 9 0 1 1-3-6.7 M21 4v5h-5'
    },
    {
      id: 'vault',
      label: 'Vault',
      title: 'Vault info + security',
      // Padlock — vault/security.
      icon: 'M5 11h14v10H5z M8 11V7a4 4 0 0 1 8 0v4'
    }
  ];
</script>

<div class="sticky top-0 z-10 flex items-center gap-2 px-3 py-2 border-b border-surface1 bg-mantle">
  <h1 class="text-base sm:text-lg font-semibold text-text leading-none">Settings</h1>
  <span class="flex-1"></span>

  <!-- Desktop: icon + label segmented control -->
  <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden">
    {#each TABS as t (t.id)}
      <button
        type="button"
        onclick={() => onSelect(t.id)}
        title={t.title}
        aria-label={t.label}
        aria-pressed={tab === t.id}
        class="px-2.5 py-1.5 inline-flex items-center gap-1.5 text-xs {tab === t.id ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d={t.icon} />
        </svg>
        <span class="hidden md:inline">{t.label}</span>
      </button>
    {/each}
  </div>

  <!-- Mobile: native select fallback (same 4 options) -->
  <select
    class="sm:hidden bg-surface0 border border-surface1 rounded px-2 py-1 text-xs text-text"
    value={tab}
    onchange={(e) => onSelect((e.currentTarget as HTMLSelectElement).value as SettingsTab)}
    aria-label="settings tab"
  >
    {#each TABS as t (t.id)}
      <option value={t.id}>{t.label}</option>
    {/each}
  </select>
</div>
