<script lang="ts">
  // General tab — theme, profile, sidebar visibility, daily/weekly
  // note paths, editor behavior, note tray, keyboard shortcuts, about.
  // Editor behavior (16 toggles + 4 inputs) lives behind a "Show
  // advanced" toggle so the default view stays one screen.
  import { theme, themeLabel, type Theme } from '$lib/stores/theme';
  import { profilesStore } from '$lib/stores/profiles';
  import { hiddenSections, setSectionHidden } from '$lib/stores/sidebar-ui';
  import { sections as navSections } from '$lib/nav/config';
  import { trayEnabled, clearOpenNote, pinnedTrayNotes } from '$lib/stores/open-note';
  import { auth } from '$lib/stores/auth';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import RecurringEditor from '$lib/components/RecurringEditor.svelte';
  import SettingsSection from './SettingsSection.svelte';
  import SettingsRow from './SettingsRow.svelte';
  import type { AppConfig, AppConfigPatch } from '$lib/api';

  type Props = {
    appCfg: AppConfig | null;
    configBusy: boolean;
    recurringTasksBuf: string;
    hiddenSectionsCount: number;
    profileBusyId: string | null;
    patchConfig: (patch: AppConfigPatch) => Promise<void>;
    commitRecurringTasks: () => Promise<void>;
    activateProfile: (id: string) => Promise<void>;
  };

  let {
    appCfg,
    configBusy,
    recurringTasksBuf = $bindable(),
    hiddenSectionsCount,
    profileBusyId,
    patchConfig,
    commitRecurringTasks,
    activateProfile
  }: Props = $props();

  const themeOptions: Theme[] = ['system', 'light', 'dark'];

  // Editor-behavior boolean toggles. The labels are user-facing, the
  // keys map to AppConfigPatch fields the TUI also writes.
  const EDITOR_TOGGLES = [
    { key: 'auto_save', label: 'Auto-save', help: 'Save 2s after the last keystroke.' },
    { key: 'line_numbers', label: 'Line numbers', help: 'Show line gutter in the editor.' },
    { key: 'word_wrap', label: 'Word wrap', help: 'Wrap long lines instead of horizontal scroll.' },
    { key: 'auto_close_brackets', label: 'Auto-close brackets', help: 'Insert matching ), ], } and quotes as you type.' },
    { key: 'highlight_current_line', label: 'Highlight current line', help: 'Tint the editor row your cursor is on.' },
    { key: 'editor_insert_tabs', label: 'Insert tab character', help: 'Use a real tab on Tab. Off = spaces.' },
    { key: 'editor_auto_indent', label: 'Auto-indent', help: 'Match the previous line indent on Enter.' },
    { key: 'auto_dark_mode', label: 'Auto dark mode', help: 'Follow OS preference (overrides theme picker).' },
    { key: 'auto_daily_note', label: 'Auto-create daily note', help: 'Open or create today on app launch.' },
    { key: 'task_exclude_done', label: 'Hide done tasks by default', help: 'Tasks page opens with only open items.' },
    { key: 'search_content_by_default', label: 'Search note content by default', help: 'Search matches body, not just titles.' },
    { key: 'auto_tag', label: 'AI auto-tag on save', help: 'Suggest tags from note content (requires AI provider).' },
    { key: 'background_bots', label: 'AI background bots', help: 'Auto-analyze notes on save (summary, action items).' },
    { key: 'semantic_search_enabled', label: 'AI semantic search index', help: 'Background embedding index enables fuzzy meaning search.' },
    { key: 'ghost_writer', label: 'AI ghost writer', help: 'Inline writing suggestions while you type.' },
    { key: 'ai_auto_apply_edits', label: 'Auto-apply AI edits', help: 'Skip BEFORE/AFTER preview on inline AI edits.' }
  ] as const;

  // Keyboard shortcuts — mirrors what's actually wired.
  const shortcuts: { keys: string; what: string }[] = [
    { keys: 'Cmd K / Ctrl+K', what: 'Open command palette / search' },
    { keys: 'Cmd S / Ctrl+S', what: 'Save the current note' },
    { keys: 'Cmd F / Ctrl+F', what: 'Find in editor' },
    { keys: 'Cmd Z / Ctrl+Z', what: 'Undo' },
    { keys: 'Cmd Shift O / Ctrl+Shift+O', what: 'Jump back to last opened note' },
    { keys: 'Enter', what: 'Submit (in any form)' },
    { keys: 'Esc', what: 'Close modal / palette' }
  ];
</script>

<!-- Theme — three-up picker. Most-touched control on this tab, so it
     stays at the top with the visual chip layout the user already
     recognised. -->
<SettingsSection title="Theme">
  {#snippet children()}
    <div class="grid grid-cols-3 gap-2 py-1">
      {#each themeOptions as t (t)}
        {@const active = $theme === t}
        <button
          type="button"
          onclick={() => theme.set(t)}
          class="px-3 py-2.5 rounded-lg border flex flex-col items-center gap-1.5 transition-colors
            {active ? 'border-primary bg-surface1 text-text' : 'border-surface1 bg-mantle text-subtext hover:bg-surface1 hover:text-text'}"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            {#if t === 'dark'}
              <path d="M21 12.79A9 9 0 1 1 11.21 3a7 7 0 0 0 9.79 9.79z"/>
            {:else if t === 'light'}
              <circle cx="12" cy="12" r="4"/>
              <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/>
            {:else}
              <circle cx="12" cy="12" r="9"/>
              <path d="M12 3a9 9 0 0 0 0 18z" fill="currentColor"/>
            {/if}
          </svg>
          <span class="text-xs font-medium">{themeLabel(t)}</span>
        </button>
      {/each}
    </div>
    <p class="text-[11px] text-dim mt-1">System follows your OS setting and updates live.</p>
  {/snippet}
</SettingsSection>

<!-- Profile — picker reads from the store. Each entry one-click
     activates. Built-in profiles + user-authored "custom" profiles
     are tagged so the user can spot which survive a granit-update. -->
<SettingsSection
  title="Profile"
  status={$profilesStore.loaded && $profilesStore.activeId
    ? `active: ${$profilesStore.profiles.find((p) => p.id === $profilesStore.activeId)?.name ?? $profilesStore.activeId}`
    : undefined}
>
  {#snippet children()}
    {#if !$profilesStore.loaded}
      <Skeleton class="h-4 w-1/2 mb-1" />
      <Skeleton class="h-4 w-1/3" />
    {:else if $profilesStore.profiles.length === 0}
      <p class="text-xs text-dim italic py-1">No profiles registered.</p>
    {:else}
      <ul class="space-y-0.5">
        {#each $profilesStore.profiles as p (p.id)}
          {@const isActive = p.id === $profilesStore.activeId}
          <li>
            <button
              type="button"
              onclick={() => activateProfile(p.id)}
              disabled={profileBusyId === p.id || isActive}
              class="w-full text-left px-2 py-1.5 rounded transition-colors flex items-start gap-2 {isActive ? 'bg-surface1' : 'hover:bg-surface0'}"
            >
              <span class="w-0.5 self-stretch rounded {isActive ? 'bg-primary' : 'bg-transparent'} flex-shrink-0"></span>
              <span class="flex-1 min-w-0">
                <span class="block text-sm font-medium text-text">
                  {p.name}
                  {#if isActive}<span class="ml-1.5 text-[10px] uppercase tracking-wider text-primary">active</span>{/if}
                  {#if !p.builtIn}<span class="ml-1.5 text-[10px] text-dim">custom</span>{/if}
                </span>
                {#if p.description}
                  <span class="block text-[11px] text-dim mt-0.5 leading-snug">{p.description}</span>
                {/if}
              </span>
              {#if profileBusyId === p.id}
                <span class="text-[11px] text-dim flex-shrink-0">…</span>
              {:else if !isActive}
                <span class="text-[11px] text-secondary flex-shrink-0">activate →</span>
              {/if}
            </button>
          </li>
        {/each}
      </ul>
      <p class="text-[11px] text-dim mt-2 leading-snug">
        Activating changes the active pointer only. Module visibility stays where you set it in Features below.
      </p>
    {/if}
  {/snippet}
</SettingsSection>

<!-- Sidebar Views — per-device hide/show for nav sections. -->
<SettingsSection
  title="Sidebar Views"
  status={hiddenSectionsCount > 0 ? `${hiddenSectionsCount} hidden` : undefined}
>
  {#snippet children()}
    <ul class="space-y-0.5">
      {#each navSections as section (section.id)}
        {@const visible = !$hiddenSections[section.id]}
        <li class="flex items-center gap-2 px-1 py-1">
          <button
            type="button"
            onclick={() => setSectionHidden(section.id, visible)}
            aria-pressed={visible}
            aria-label="{visible ? 'hide' : 'show'} {section.label}"
            class="w-9 h-5 rounded-full relative transition-colors flex-shrink-0 {visible ? 'bg-primary' : 'bg-surface1'}"
          >
            <span class="absolute top-0.5 w-4 h-4 rounded-full bg-base transition-all {visible ? 'left-4' : 'left-0.5'}"></span>
          </button>
          <span class="flex-1 text-sm text-text">{section.label}</span>
          <span class="text-[10px] text-dim tabular-nums">{section.items.length}</span>
        </li>
      {/each}
    </ul>
    <p class="text-[11px] text-dim mt-2 leading-snug">
      Routes still work via the command palette + direct URLs. This only affects the sidebar rail.
    </p>
  {/snippet}
</SettingsSection>

<!-- Features link — entry point to /settings/features. -->
<a
  href="/settings/features"
  class="block bg-surface0 border border-surface1 rounded-lg p-3 mb-2.5 hover:border-primary transition-colors group"
>
  <div class="flex items-baseline gap-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium group-hover:text-text transition-colors">Features</h2>
    <span class="flex-1"></span>
    <span class="text-secondary text-sm group-hover:underline">configure →</span>
  </div>
  <p class="text-xs text-dim mt-1.5 leading-snug">
    Toggle which features show in the sidebar — Morning, Habits, Goals, Examen, and the rest. Hide anything you don't use; data stays on disk.
  </p>
</a>

<!-- Daily notes — daily/weekly folders + recurring habit list. -->
<SettingsSection title="Daily notes">
  {#snippet children()}
    {#if !appCfg}
      <Skeleton class="h-4 w-1/2" />
    {:else}
      <SettingsRow label="Daily notes folder" help="Empty = vault root. New dailies land at {appCfg.daily_notes_folder || ''}/{'{YYYY-MM-DD}'}.md">
        {#snippet control()}
          <input
            value={appCfg.daily_notes_folder}
            onblur={(e) => patchConfig({ daily_notes_folder: (e.target as HTMLInputElement).value })}
            placeholder="Jots"
            class="w-full sm:w-56 px-2 py-1 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
          />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Weekly notes folder">
        {#snippet control()}
          <input
            value={appCfg.weekly_notes_folder}
            onblur={(e) => patchConfig({ weekly_notes_folder: (e.target as HTMLInputElement).value })}
            placeholder="Weeklies"
            class="w-full sm:w-56 px-2 py-1 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
          />
        {/snippet}
      </SettingsRow>
      <div class="pt-2">
        <div class="text-sm text-text mb-1">Daily habits / recurring tasks</div>
        <textarea
          bind:value={recurringTasksBuf}
          onblur={commitRecurringTasks}
          rows="4"
          placeholder="Workout&#10;Read&#10;Meditate"
          class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-text text-sm"
        ></textarea>
        <p class="text-[11px] text-dim mt-1 leading-snug">
          One per line. Renders as a checklist on every daily note. Tick with <code>- [x] habit</code>.
        </p>
      </div>
    {/if}
  {/snippet}
</SettingsSection>

<!-- Recurring tasks — full editor lives in its own component. -->
<SettingsSection title="Recurring tasks">
  {#snippet children()}
    <div class="py-1">
      <RecurringEditor />
    </div>
  {/snippet}
</SettingsSection>

<!-- Editor & behavior — 16 boolean toggles plus 4 inputs. Lives
     behind an advanced collapse: power users open it, casual users
     never see the noise. Tab size + max search results live with
     the booleans so the whole "how does the editor feel" surface
     is one expandable region. -->
<SettingsSection title="Editor & behavior" advancedLabel="Show editor preferences">
  {#snippet children()}
    {#if !appCfg}
      <Skeleton class="h-4 w-1/2" />
    {:else}
      <p class="text-[11px] text-dim py-1">
        Editor preferences ({EDITOR_TOGGLES.length} toggles + 4 inputs) are kept under "Show editor preferences" to keep this screen scannable.
      </p>
    {/if}
  {/snippet}
  {#snippet advanced()}
    {#if appCfg}
      <div class="grid sm:grid-cols-2 gap-x-4 gap-y-1">
        {#each EDITOR_TOGGLES as opt (opt.key)}
          <label class="flex items-start gap-2.5 cursor-pointer py-1">
            <input
              type="checkbox"
              checked={(appCfg as unknown as Record<string, boolean>)[opt.key]}
              onchange={(e) => patchConfig({ [opt.key]: (e.target as HTMLInputElement).checked } as AppConfigPatch)}
              disabled={configBusy}
              class="w-4 h-4 mt-0.5 accent-primary cursor-pointer"
            />
            <div class="flex-1 min-w-0">
              <div class="text-[13px] text-text leading-snug">{opt.label}</div>
              <div class="text-[10.5px] text-dim leading-snug">{opt.help}</div>
            </div>
          </label>
        {/each}
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3 mt-3 border-t border-surface1">
        <div>
          <label for="tab-size" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Tab size</label>
          <input
            id="tab-size"
            type="number"
            min="1"
            max="16"
            value={appCfg.editor_tab_size || 4}
            onblur={(e) => patchConfig({ editor_tab_size: Number((e.target as HTMLInputElement).value) || 4 })}
            class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-text text-sm"
          />
        </div>
        <div>
          <label for="max-search" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Max search results</label>
          <input
            id="max-search"
            type="number"
            min="10"
            max="1000"
            step="10"
            value={appCfg.max_search_results || 100}
            onblur={(e) => patchConfig({ max_search_results: Number((e.target as HTMLInputElement).value) || 100 })}
            class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-text text-sm"
          />
        </div>
        <div class="sm:col-span-2">
          <label for="weekly-template" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Weekly note template path</label>
          <input
            id="weekly-template"
            value={appCfg.weekly_note_template ?? ''}
            onblur={(e) => patchConfig({ weekly_note_template: (e.target as HTMLInputElement).value })}
            placeholder="Templates/weekly.md"
            class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
          />
          <p class="text-[11px] text-dim mt-1">Path inside the vault. Empty = built-in fallback layout.</p>
        </div>
        <div class="sm:col-span-2">
          <label for="exclude-folders" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Task: exclude folders</label>
          <input
            id="exclude-folders"
            value={(appCfg.task_exclude_folders ?? []).join(', ')}
            onblur={(e) => {
              const list = (e.target as HTMLInputElement).value
                .split(',')
                .map((s) => s.trim())
                .filter(Boolean);
              patchConfig({ task_exclude_folders: list });
            }}
            placeholder="Archive/, Templates/"
            class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
          />
          <p class="text-[11px] text-dim mt-1">Comma-separated. Hidden from Tasks page.</p>
        </div>
      </div>
    {/if}
  {/snippet}
</SettingsSection>

<!-- Note tray — bottom bar with last-opened-note jump-back. -->
<SettingsSection title="Note tray">
  {#snippet children()}
    <label class="flex items-start gap-2.5 cursor-pointer py-1">
      <input
        type="checkbox"
        checked={$trayEnabled}
        onchange={(e) => trayEnabled.set((e.target as HTMLInputElement).checked)}
        class="w-4 h-4 mt-0.5 accent-primary cursor-pointer"
      />
      <div class="flex-1">
        <div class="text-sm text-text">Show note tray</div>
        <div class="text-[11px] text-dim leading-snug">Slim bar at the bottom with a one-click jump back to your last opened note. <code>Mod-Shift-O</code> also opens it.</div>
      </div>
    </label>
    {#if $pinnedTrayNotes.length > 0}
      <div class="mt-2 pt-2 border-t border-surface1">
        <div class="text-[11px] uppercase tracking-wider text-dim mb-1">Pinned ({$pinnedTrayNotes.length})</div>
        <ul class="text-xs text-subtext space-y-0.5 font-mono">
          {#each $pinnedTrayNotes as p (p.path)}
            <li class="truncate" title={p.path}>{p.title || p.path}</li>
          {/each}
        </ul>
      </div>
    {/if}
    <div class="mt-2 pt-2 border-t border-surface1">
      <button
        type="button"
        onclick={() => clearOpenNote()}
        class="text-[11px] text-dim hover:text-text px-2 py-1 rounded hover:bg-surface1"
      >Clear remembered note</button>
    </div>
  {/snippet}
</SettingsSection>

<!-- Keyboard shortcuts -->
<SettingsSection title="Keyboard shortcuts">
  {#snippet children()}
    <ul class="space-y-1 py-1">
      {#each shortcuts as s (s.keys)}
        <li class="flex items-baseline gap-3">
          <kbd class="text-[11px] font-mono px-1.5 py-0.5 bg-mantle border border-surface1 rounded text-text whitespace-nowrap">{s.keys}</kbd>
          <span class="text-subtext text-sm">{s.what}</span>
        </li>
      {/each}
    </ul>
  {/snippet}
</SettingsSection>

<!-- About + sign-out -->
<SettingsSection title="About">
  {#snippet children()}
    <p class="text-sm text-subtext leading-snug py-1">
      Granit web — your vault, anywhere. Powered by the same data layer as the granit TUI.
      Learn more at <a href="https://github.com/artaeon/granit" rel="noreferrer" target="_blank" class="text-secondary hover:underline">github.com/artaeon/granit</a>.
    </p>
    <div class="pt-2">
      <button
        type="button"
        onclick={() => auth.clear()}
        class="text-xs text-error hover:underline"
      >Sign out (clears the bearer token from this device)</button>
    </div>
  {/snippet}
</SettingsSection>
