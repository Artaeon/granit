<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type OpenAIModelOption, type CalendarSource } from '$lib/api';
  import { onWsEvent, wsConnected } from '$lib/ws';
  import { theme, themeIcon, themeLabel, type Theme } from '$lib/stores/theme';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import RecurringEditor from '$lib/components/RecurringEditor.svelte';
  import { modulesStore } from '$lib/stores/modules';
  import { toast } from '$lib/components/toast';

  // Curated OpenAI model picker — refreshed against
  // developers.openai.com/api/docs/pricing periodically. Server is the
  // source of truth (internal/agentruntime.RecommendedOpenAIModels);
  // the web just renders. Lazy-loaded so we don't fetch on settings
  // pages where the user never opens the AI section.
  let openAIModels = $state<OpenAIModelOption[] | null>(null);
  async function ensureOpenAIModels() {
    if (openAIModels) return;
    try {
      const r = await api.listOpenAIModels();
      openAIModels = r.models;
    } catch {
      openAIModels = [];
    }
  }

  type SyncStatus = {
    enabled: boolean;
    interval?: string;
    lastPull?: string;
    lastPush?: string;
    pulls?: number;
    pushes?: number;
    lastErr?: string;
  };

  import type { AppConfig, AppConfigPatch } from '$lib/api';

  let vault = $state<{ root: string; notes: number } | null>(null);
  let sync = $state<SyncStatus | null>(null);
  let authStatus = $state<{ hasPassword: boolean; sessionCount?: number; setupAt?: string } | null>(null);
  let devices = $state<import('$lib/api').Device[]>([]);
  let revokeBusy = $state<string | null>(null);

  // Curated config from /api/v1/config — same file the TUI reads.
  let appCfg = $state<AppConfig | null>(null);
  let configBusy = $state(false);
  // Inline-edit buffers for keys (we don't echo the secret back from
  // the server — only a "set/unset" flag — so the input starts empty
  // each load). Empty string + the user clicking save clears the key
  // server-side; non-empty string sets it.
  let openAIKeyBuf = $state('');
  let recurringTasksBuf = $state('');

  async function patchConfig(patch: AppConfigPatch) {
    if (!appCfg) return;
    configBusy = true;
    try {
      appCfg = await api.patchConfig(patch);
    } catch (e) {
      pwError = e instanceof Error ? e.message : String(e);
    } finally {
      configBusy = false;
    }
  }

  async function commitOpenAIKey() {
    // Empty input + currently-set key → no-op (avoid accidental clears).
    // Empty input + no key set → no-op. Non-empty → patch.
    if (!openAIKeyBuf.trim()) return;
    await patchConfig({ openai_key: openAIKeyBuf.trim() });
    openAIKeyBuf = '';
  }
  async function clearOpenAIKey() {
    if (!confirm('Clear the OpenAI API key?')) return;
    await patchConfig({ openai_key: '' });
    openAIKeyBuf = '';
  }
  // Anthropic key commit/clear handlers omitted — the dropdown no
  // longer offers Anthropic and the runtime can't reach the Messages
  // API yet. See TODO in internal/agentruntime/llm.go.

  // Recurring-tasks: list <textarea> with one item per line. Easier
  // mental model than a chip-add UI for a one-time setup screen.
  function syncRecurringBuf(c: AppConfig) {
    recurringTasksBuf = (c.daily_recurring_tasks ?? []).join('\n');
  }
  async function commitRecurringTasks() {
    const list = recurringTasksBuf.split('\n').map((s) => s.trim()).filter(Boolean);
    await patchConfig({ daily_recurring_tasks: list });
  }

  // Local calendars — writable .ics files under <vault>/calendars/.
  // Settings shows the list + a "+ New calendar" button; everything
  // else (event CRUD) lives on the calendar page.
  let calSources = $state<CalendarSource[]>([]);
  let calBusy = $state(false);
  async function loadCalSources() {
    try {
      const r = await api.listCalendarSources();
      calSources = r.sources;
    } catch {
      calSources = [];
    }
  }
  async function newCalendar() {
    const name = prompt('Calendar name (a-z, 0-9, -, _, space; max 64 chars):');
    if (!name) return;
    calBusy = true;
    try {
      await api.createCalendar({ name: name.trim() });
      await loadCalSources();
    } catch (e) {
      pwError = e instanceof Error ? e.message : String(e);
    } finally {
      calBusy = false;
    }
  }

  // Change-password panel state — hidden until the user opens it.
  let pwOpen = $state(false);
  let pwOld = $state('');
  let pwNew = $state('');
  let pwConfirm = $state('');
  let pwBusy = $state(false);
  let pwError = $state('');
  let pwSuccess = $state('');

  async function load() {
    if (!$auth) return;
    try {
      const [v, s, a, d, c] = await Promise.all([
        api.vault(),
        api.req<SyncStatus>('/sync'),
        api.authStatus(),
        api.listDevices().catch(() => ({ devices: [] })),
        api.getConfig().catch(() => null)
      ]);
      vault = v;
      sync = s;
      authStatus = a;
      devices = d.devices;
      if (c) {
        appCfg = c;
        syncRecurringBuf(c);
      }
    } catch {}
  }

  async function revokeDevice(id: string) {
    revokeBusy = id;
    try {
      await api.revokeDevice(id);
      devices = devices.filter((d) => d.id !== id);
    } catch (e) {
      pwError = e instanceof Error ? e.message : String(e);
    } finally {
      revokeBusy = null;
    }
  }

  function fmtRelative(iso: string): string {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    const mins = Math.round((Date.now() - d.getTime()) / 60_000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins} min ago`;
    if (mins < 24 * 60) return `${Math.round(mins / 60)}h ago`;
    const days = Math.round(mins / (24 * 60));
    if (days < 7) return `${days}d ago`;
    return d.toLocaleDateString();
  }

  async function changePassword(e: Event) {
    e.preventDefault();
    pwError = ''; pwSuccess = '';
    if (pwNew.length < 6) { pwError = 'new password must be at least 6 characters'; return; }
    if (pwNew !== pwConfirm) { pwError = 'passwords do not match'; return; }
    pwBusy = true;
    try {
      await api.authChangePassword(pwOld, pwNew);
      pwSuccess = 'password changed — all sessions revoked, please sign in again';
      pwOld = ''; pwNew = ''; pwConfirm = '';
      // Server invalidates the current token on password change. Clear
      // local auth so the user routes back to login.
      setTimeout(() => { auth.clear(); }, 1500);
    } catch (e) {
      pwError = e instanceof Error ? e.message : String(e);
    } finally {
      pwBusy = false;
    }
  }

  async function revokeAllSessions() {
    if (!confirm('Sign out everywhere? You will need to log in again on every device.')) return;
    try {
      await api.authRevokeAll();
      auth.clear();
    } catch (e) {
      pwError = e instanceof Error ? e.message : String(e);
    }
  }
  onMount(() => {
    load();
    void modulesStore.ensureLoaded();
    void loadCalSources();
    return onWsEvent((ev) => {
      // Watch for ICS file mutations so the calendars list refreshes
      // when an event is created from another tab.
      if (ev.type === 'state.changed' && ev.path?.startsWith('calendars/')) {
        void loadCalSources();
      }
      load();
    });
  });

  // Module toggle UX. We debounce so a user rapid-firing checkboxes
  // doesn't fire a PUT per click (the server batches anyway, but the
  // round-trip still costs). Pending edits coalesce into one PUT after
  // a quiet period; toast on success/failure.
  let pendingModulePatch: Record<string, boolean> = $state({});
  let moduleSaveTimer: ReturnType<typeof setTimeout> | null = null;
  let moduleSaving = $state(false);

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
      toast.success('Modules updated');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
      // Roll back the optimistic store state so checkboxes match
      // server truth on the next render tick.
      void modulesStore.refresh();
    } finally {
      moduleSaving = false;
    }
  }

  const themeOptions: Theme[] = ['system', 'light', 'dark'];

  // Keyboard shortcuts list (mirrors what's actually wired)
  const shortcuts: { keys: string; what: string }[] = [
    { keys: '⌘ K  /  Ctrl+K', what: 'Open command palette / search' },
    { keys: '⌘ S  /  Ctrl+S', what: 'Save the current note' },
    { keys: '⌘ F  /  Ctrl+F', what: 'Find in editor' },
    { keys: '⌘ Z  /  Ctrl+Z', what: 'Undo' },
    { keys: '↵',              what: 'Submit (in any form)' },
    { keys: 'Esc',            what: 'Close modal / palette' }
  ];

  function fmtTime(s?: string): string {
    if (!s || s.startsWith('0001-')) return '—';
    return new Date(s).toLocaleString();
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-3xl mx-auto">
    <PageHeader title="Settings" subtitle="Theme, keyboard shortcuts, vault info, sync status" />

    <!-- Theme -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Theme</h2>
      <div class="grid grid-cols-3 gap-2">
        {#each themeOptions as t}
          {@const active = $theme === t}
          <button
            onclick={() => theme.set(t)}
            class="px-3 py-3 rounded-lg border-2 flex flex-col items-center gap-1
              {active ? 'border-primary bg-primary/10 text-primary' : 'border-surface1 bg-mantle text-subtext hover:border-primary/40'}"
          >
            <span class="text-2xl">{themeIcon(t)}</span>
            <span class="text-xs font-medium">{themeLabel(t)}</span>
          </button>
        {/each}
      </div>
      <p class="text-xs text-dim mt-2 leading-relaxed">
        System follows your OS setting and updates live. Stored in <code class="text-[10px]">localStorage</code>.
      </p>
    </section>

    <!-- Modules — toggle which surfaces appear in the sidebar / are
         routable. Backed by .granit/modules.json (same file the TUI
         registry persists to). Core surfaces (notes, tasks, calendar,
         settings) are always-on and rendered with a lock icon. -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <header class="flex items-baseline justify-between mb-3">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Modules</h2>
        {#if moduleSaving}
          <span class="text-[10px] uppercase tracking-wider text-dim">saving…</span>
        {/if}
      </header>
      <p class="text-xs text-dim mb-3">
        Disable a module to hide its sidebar entry, dashboard widgets, and route. Re-enable any time — your data stays on disk.
      </p>
      {#if !$modulesStore.loaded}
        <Skeleton class="h-4 w-full mb-2" />
        <Skeleton class="h-4 w-3/4" />
      {:else}
        <div class="space-y-1.5">
          <!-- Always-on core. Rendered first with a lock icon so the
               user understands these can't be disabled. -->
          {#each $modulesStore.coreIds as core (core.id)}
            <label class="flex items-start gap-3 py-1 opacity-70 cursor-not-allowed">
              <input type="checkbox" checked disabled class="w-4 h-4 mt-0.5 accent-primary" />
              <div class="flex-1">
                <div class="text-sm text-text flex items-center gap-1.5">
                  <span>{core.name}</span>
                  <span class="text-[10px]" title="Always on — core surface">🔒</span>
                </div>
                <div class="text-[11px] text-dim">Always on, can't disable.</div>
              </div>
            </label>
          {/each}

          <div class="border-t border-surface1 my-2"></div>

          {#each $modulesStore.modules as m (m.id)}
            {@const queued = pendingModulePatch[m.id]}
            {@const checked = queued !== undefined ? queued : m.enabled}
            <label class="flex items-start gap-3 cursor-pointer py-1">
              <input
                type="checkbox"
                {checked}
                onchange={(e) => queueModuleToggle(m.id, (e.target as HTMLInputElement).checked)}
                class="w-4 h-4 mt-0.5 accent-primary cursor-pointer"
              />
              <div class="flex-1">
                <div class="text-sm text-text">{m.name}</div>
                <div class="text-[11px] text-dim">{m.description}</div>
              </div>
            </label>
          {/each}
        </div>
      {/if}
    </section>

    <!-- AI provider — same config the TUI reads. Setting up either
         surface is enough; both pick up changes automatically. -->
    <section class="bg-surface0 border-2 border-primary/30 rounded-lg p-4 mb-4">
      <header class="flex items-baseline gap-3 mb-3">
        <h2 class="text-base font-semibold text-text">AI provider</h2>
        {#if appCfg}
          {@const provider = appCfg.ai_provider || 'ollama'}
          {@const ready = (provider === 'openai' && appCfg.openai_key_set)
            || (provider === 'ollama' || provider === 'local' || provider === '')}
          <span
            class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium"
            style="color: var(--color-{ready ? 'success' : 'warning'}); background: color-mix(in srgb, var(--color-{ready ? 'success' : 'warning'}) 14%, transparent);"
          >{ready ? 'configured' : 'needs API key'}</span>
        {/if}
      </header>
      <p class="text-xs text-dim mb-3 -mt-1">
        Powers chat, agent runs (Plan my day, deep research, summarize, reflect), morning AI focus suggestion, and any future AI feature. Same config as <code class="text-text">granit tui</code>.
      </p>
      {#if !appCfg}
        <Skeleton class="h-4 w-1/3 mb-2" />
        <Skeleton class="h-4 w-1/2" />
      {:else}
        <div class="space-y-3 text-sm">
          <div>
            <label for="ai-provider" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Provider</label>
            <!-- Anthropic intentionally absent: the runtime
                 (internal/agentruntime/llm.go) doesn't implement the
                 Messages API yet. AppConfig keeps anthropic_* fields so
                 a TUI-set value isn't truncated on save from the web. -->
            <select
              id="ai-provider"
              value={appCfg.ai_provider || 'ollama'}
              onchange={(e) => patchConfig({ ai_provider: (e.target as HTMLSelectElement).value })}
              disabled={configBusy}
              class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text"
            >
              <option value="openai">OpenAI (cloud — gpt-4o, gpt-5)</option>
              <option value="ollama">Ollama (local — free, private)</option>
            </select>
            <p class="text-[11px] text-dim mt-1">
              {#if (appCfg.ai_provider || 'ollama') === 'openai'}
                Need a key? <a href="https://platform.openai.com/api-keys" target="_blank" rel="noopener" class="text-secondary hover:underline">platform.openai.com/api-keys</a> · ~$0.0001 per chat call on gpt-4o-mini.
              {:else}
                Run <code class="text-text">ollama serve</code> locally. Free, private, slower than cloud.
              {/if}
            </p>
          </div>

          {#if appCfg.ai_provider === 'openai' || (!appCfg.ai_provider && appCfg.openai_key_set)}
            {void ensureOpenAIModels()}
            <div>
              <label for="openai-model" class="block text-[11px] uppercase tracking-wider text-dim mb-1">
                OpenAI model
                {#if appCfg.openai_model}<span class="text-dim normal-case ml-1 font-mono">· {appCfg.openai_model}</span>{/if}
              </label>
              {#if openAIModels === null}
                <Skeleton class="h-9 w-full" />
              {:else if openAIModels.length === 0}
                <input
                  id="openai-model"
                  value={appCfg.openai_model}
                  onblur={(e) => patchConfig({ openai_model: (e.target as HTMLInputElement).value })}
                  placeholder="gpt-4o-mini"
                  class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
                />
              {:else}
                <!-- Curated picker. Prices in the option label so the
                     user picks knowingly. Custom model still allowed via
                     the override input below. -->
                <select
                  id="openai-model"
                  value={appCfg.openai_model}
                  onchange={(e) => patchConfig({ openai_model: (e.target as HTMLSelectElement).value })}
                  disabled={configBusy}
                  class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
                >
                  {#if appCfg.openai_model}
                    {#if !openAIModels.find((m) => m.id === appCfg!.openai_model)}
                      <option value={appCfg.openai_model}>{appCfg.openai_model} (custom)</option>
                    {/if}
                  {/if}
                  {#each openAIModels as m}
                    <option value={m.id}>
                      {m.id} — in {m.input_per_m} / out {m.output_per_m} per 1M tokens{m.note ? ` · ${m.note}` : ''}
                    </option>
                  {/each}
                </select>
                <details class="mt-1.5">
                  <summary class="text-[11px] text-dim cursor-pointer hover:text-text">use a model not in the list</summary>
                  <input
                    type="text"
                    placeholder="gpt-5.5-pro / o3-mini / dated snapshot ID"
                    onblur={(e) => {
                      const v = (e.target as HTMLInputElement).value.trim();
                      if (v) patchConfig({ openai_model: v });
                    }}
                    class="mt-1.5 w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
                  />
                </details>
                <p class="text-[11px] text-dim mt-1.5">
                  Recommended for agents: <code>gpt-5.4-mini</code> (workhorse) or <code>gpt-5.4-nano</code> (cheap).
                  Pricing reflects May 2026 rates from <a href="https://platform.openai.com/docs/pricing" target="_blank" rel="noopener" class="text-secondary hover:underline">platform.openai.com</a>.
                </p>
              {/if}
            </div>
            <div>
              <label for="openai-key" class="block text-[11px] uppercase tracking-wider text-dim mb-1">
                OpenAI API key
                {#if appCfg.openai_key_set}<span class="text-success normal-case ml-1">· set</span>{/if}
              </label>
              <div class="flex gap-2">
                <input
                  id="openai-key"
                  type="password"
                  bind:value={openAIKeyBuf}
                  placeholder={appCfg.openai_key_set ? '••••• (hidden — type to replace)' : 'sk-…'}
                  class="flex-1 px-3 py-2 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
                />
                <button onclick={commitOpenAIKey} disabled={!openAIKeyBuf.trim() || configBusy} class="px-3 py-2 text-xs bg-primary text-on-primary rounded disabled:opacity-50">save</button>
                {#if appCfg.openai_key_set}
                  <button onclick={clearOpenAIKey} class="px-3 py-2 text-xs text-error hover:bg-error/10 rounded border border-error/30">clear</button>
                {/if}
              </div>
              <p class="text-[11px] text-dim mt-1">Stored in <code>~/.config/granit/config.json</code>. Never echoed back to the browser after save.</p>
            </div>
          {/if}

          <!-- Anthropic key/model inputs intentionally hidden — the
               runtime can't talk to the Messages API yet. See the
               TODO in internal/agentruntime/llm.go. -->

          {#if appCfg.ai_provider === 'ollama' || appCfg.ai_provider === 'local' || appCfg.ai_provider === ''}
            <div>
              <label for="ollama-url" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Ollama URL</label>
              <input
                id="ollama-url"
                value={appCfg.ollama_url}
                onblur={(e) => patchConfig({ ollama_url: (e.target as HTMLInputElement).value })}
                placeholder="http://localhost:11434"
                class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
              />
            </div>
            <div>
              <label for="ollama-model" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Ollama model</label>
              <input
                id="ollama-model"
                value={appCfg.ollama_model}
                onblur={(e) => patchConfig({ ollama_model: (e.target as HTMLInputElement).value })}
                placeholder="llama3.2"
                class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
              />
              <p class="text-[11px] text-dim mt-1">Pull first with <code>ollama pull {appCfg.ollama_model || 'llama3.2'}</code>.</p>
            </div>
          {/if}
        </div>
      {/if}
    </section>

    <!-- Daily / weekly notes -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Daily notes</h2>
      {#if !appCfg}
        <Skeleton class="h-4 w-1/2" />
      {:else}
        <div class="space-y-3 text-sm">
          <div>
            <label for="daily-folder" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Daily notes folder</label>
            <input
              id="daily-folder"
              value={appCfg.daily_notes_folder}
              onblur={(e) => patchConfig({ daily_notes_folder: (e.target as HTMLInputElement).value })}
              placeholder="Jots"
              class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
            />
            <p class="text-[11px] text-dim mt-1">Empty = vault root. New daily notes land at <code>{appCfg.daily_notes_folder || ''}/{'{YYYY-MM-DD}'}.md</code>.</p>
          </div>
          <div>
            <label for="weekly-folder" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Weekly notes folder</label>
            <input
              id="weekly-folder"
              value={appCfg.weekly_notes_folder}
              onblur={(e) => patchConfig({ weekly_notes_folder: (e.target as HTMLInputElement).value })}
              placeholder="Weeklies"
              class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
            />
          </div>
          <div>
            <label for="recurring-tasks" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Daily habits / recurring tasks</label>
            <textarea
              id="recurring-tasks"
              bind:value={recurringTasksBuf}
              onblur={commitRecurringTasks}
              rows="5"
              placeholder="Workout&#10;Read&#10;Meditate"
              class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text text-sm"
            ></textarea>
            <p class="text-[11px] text-dim mt-1">One per line. Renders as a habits checklist on every daily note. Tick by writing <code>- [x] {'{habit}'}</code> in the daily.</p>
          </div>
        </div>
      {/if}
    </section>

    <!-- Recurring tasks -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Recurring tasks</h2>
      <RecurringEditor />
    </section>

    <!-- Editor / behavior -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Editor & behavior</h2>
      {#if !appCfg}
        <Skeleton class="h-4 w-1/2" />
      {:else}
        <div class="space-y-2 text-sm">
          {#each [
            { key: 'auto_save', label: 'Auto-save', help: 'Save 2s after the last keystroke.' },
            { key: 'line_numbers', label: 'Line numbers', help: 'Show line gutter in the editor.' },
            { key: 'word_wrap', label: 'Word wrap', help: 'Wrap long lines instead of horizontal scroll.' },
            { key: 'auto_close_brackets', label: 'Auto-close brackets', help: 'Insert matching ), ], } and quotes as you type.' },
            { key: 'highlight_current_line', label: 'Highlight current line', help: 'Tint the editor row your cursor is on.' },
            { key: 'editor_insert_tabs', label: 'Insert tab character', help: 'Use a real tab on Tab. Off = spaces (width set below).' },
            { key: 'editor_auto_indent', label: 'Auto-indent', help: 'Match the previous line indent on Enter.' },
            { key: 'auto_dark_mode', label: 'Auto dark mode', help: 'Follow OS preference (overrides theme picker).' },
            { key: 'auto_daily_note', label: 'Auto-create daily note', help: 'Open or create today daily on app launch.' },
            { key: 'task_exclude_done', label: 'Hide done tasks by default', help: 'Tasks page opens with only open items.' },
            { key: 'search_content_by_default', label: 'Search note content by default', help: 'Search bar matches body text, not just titles.' },
            { key: 'auto_tag', label: 'AI auto-tag on save', help: 'Suggest tags from note content (requires AI provider).' },
            { key: 'background_bots', label: 'AI background bots', help: 'Auto-analyze notes on save (e.g. summary, action items).' },
            { key: 'semantic_search_enabled', label: 'AI semantic search index', help: 'Background embedding index enables fuzzy meaning search.' },
            { key: 'ghost_writer', label: 'AI ghost writer', help: 'Inline writing suggestions while you type.' },
            { key: 'ai_auto_apply_edits', label: 'Auto-apply AI edits', help: 'Skip the BEFORE/AFTER preview on inline AI edits.' }
          ] as opt}
            <label class="flex items-start gap-3 cursor-pointer py-1">
              <input
                type="checkbox"
                checked={(appCfg as unknown as Record<string, boolean>)[opt.key]}
                onchange={(e) => patchConfig({ [opt.key]: (e.target as HTMLInputElement).checked } as AppConfigPatch)}
                disabled={configBusy}
                class="w-4 h-4 mt-0.5 accent-primary cursor-pointer"
              />
              <div class="flex-1">
                <div class="text-sm text-text">{opt.label}</div>
                <div class="text-[11px] text-dim">{opt.help}</div>
              </div>
            </label>
          {/each}

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3 border-t border-surface1">
            <div>
              <label for="tab-size" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Tab size</label>
              <input
                id="tab-size"
                type="number"
                min="1"
                max="16"
                value={appCfg.editor_tab_size || 4}
                onblur={(e) => patchConfig({ editor_tab_size: Number((e.target as HTMLInputElement).value) || 4 })}
                class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text text-sm"
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
                class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text text-sm"
              />
            </div>
            <div class="sm:col-span-2">
              <label for="weekly-template" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Weekly note template path</label>
              <input
                id="weekly-template"
                value={appCfg.weekly_note_template ?? ''}
                onblur={(e) => patchConfig({ weekly_note_template: (e.target as HTMLInputElement).value })}
                placeholder="Templates/weekly.md"
                class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
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
                class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
              />
              <p class="text-[11px] text-dim mt-1">Comma-separated. Tasks under these folders are hidden from the Tasks page.</p>
            </div>
          </div>
        </div>
      {/if}
    </section>

    <!-- Security -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Security</h2>
      {#if !authStatus}
        <Skeleton class="h-4 w-1/2" />
      {:else}
        <dl class="space-y-1.5 text-sm mb-3">
          <div class="flex gap-3">
            <dt class="text-dim w-28 flex-shrink-0">Password</dt>
            <dd class="text-text">{authStatus.hasPassword ? 'set' : 'not set — paste-token mode'}</dd>
          </div>
          {#if authStatus.setupAt}
            <div class="flex gap-3">
              <dt class="text-dim w-28 flex-shrink-0">Created</dt>
              <dd class="text-text">{new Date(authStatus.setupAt).toLocaleDateString()}</dd>
            </div>
          {/if}
          <div class="flex gap-3">
            <dt class="text-dim w-28 flex-shrink-0">Active sessions</dt>
            <dd class="text-text">{authStatus.sessionCount ?? 0}</dd>
          </div>
        </dl>

        {#if authStatus.hasPassword}
          {#if !pwOpen}
            <div class="flex gap-2 flex-wrap">
              <button onclick={() => (pwOpen = true)} class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded hover:border-primary text-text">
                Change password
              </button>
              <button onclick={revokeAllSessions} class="px-3 py-1.5 text-xs text-error hover:bg-error/10 rounded border border-error/30">
                Sign out everywhere
              </button>
            </div>
          {:else}
            <form onsubmit={changePassword} class="space-y-2 mt-2">
              <input type="password" bind:value={pwOld} placeholder="current password" autocomplete="current-password" required class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text" />
              <input type="password" bind:value={pwNew} placeholder="new password" autocomplete="new-password" required class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text" />
              <input type="password" bind:value={pwConfirm} placeholder="confirm new password" autocomplete="new-password" required class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text" />
              {#if pwError}<div class="text-sm text-error">{pwError}</div>{/if}
              {#if pwSuccess}<div class="text-sm text-success">{pwSuccess}</div>{/if}
              <div class="flex gap-2">
                <button type="submit" disabled={pwBusy} class="px-3 py-1.5 text-xs bg-primary text-on-primary rounded disabled:opacity-50">
                  {pwBusy ? 'changing…' : 'Change password'}
                </button>
                <button type="button" onclick={() => { pwOpen = false; pwError = ''; pwOld = pwNew = pwConfirm = ''; }} class="px-3 py-1.5 text-xs text-dim hover:text-text">cancel</button>
              </div>
            </form>
          {/if}
        {:else}
          <p class="text-xs text-dim italic">Sign in once via the home page to set a password.</p>
        {/if}
      {/if}
    </section>

    <!-- Devices — every browser/laptop with an active session. The
         current device is highlighted. Each row can be revoked, which
         signs that device out without touching the password. -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <div class="flex items-baseline justify-between mb-3">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Devices</h2>
        <span class="text-xs text-dim">{devices.length} active</span>
      </div>
      {#if devices.length === 0}
        <p class="text-sm text-dim italic">No active sessions. (You can't see this — you're logged in.)</p>
      {:else}
        <ul class="divide-y divide-surface1">
          {#each devices as d (d.id)}
            <li class="py-2.5 flex items-center gap-3">
              <!-- Per-device-type icon. The label was set by the browser
                   on login (Web/iOS/macOS/Linux/Windows) — best-effort
                   identifier the user can recognize. -->
              <div class="w-8 h-8 rounded bg-surface1 flex items-center justify-center text-subtext flex-shrink-0">
                {#if d.label === 'iOS' || d.label === 'Android'}
                  <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="2" width="12" height="20" rx="2"/><path d="M11 18h2"/></svg>
                {:else}
                  <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="3" y="5" width="18" height="11" rx="1"/><path d="M2 20h20"/></svg>
                {/if}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-baseline gap-2">
                  <span class="text-sm text-text font-medium">{d.label || 'Unknown device'}</span>
                  {#if d.current}
                    <span class="text-[10px] uppercase px-1.5 py-0.5 rounded bg-success/15 text-success">this device</span>
                  {/if}
                  <code class="text-[10px] text-dim font-mono">{d.id}</code>
                </div>
                <div class="text-xs text-dim">
                  active {fmtRelative(d.lastUsed)} · created {fmtRelative(d.createdAt)}
                </div>
              </div>
              {#if !d.current}
                <button
                  onclick={() => revokeDevice(d.id)}
                  disabled={revokeBusy === d.id}
                  class="px-2.5 py-1 text-xs text-error hover:bg-error/10 rounded border border-error/30 disabled:opacity-50 flex-shrink-0"
                >{revokeBusy === d.id ? 'revoking…' : 'revoke'}</button>
              {/if}
            </li>
          {/each}
        </ul>
        <p class="text-[11px] text-dim italic mt-3">
          A new row appears each time you log in from a different browser, phone, or laptop.
          Revoking signs that device out without affecting the others.
        </p>
      {/if}
    </section>

    <!-- Vault info -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Vault</h2>
      {#if !vault}
        <Skeleton class="h-4 w-2/3 mb-2" />
        <Skeleton class="h-4 w-1/2" />
      {:else}
        <dl class="space-y-1.5 text-sm">
          <div class="flex gap-3">
            <dt class="text-dim w-24 flex-shrink-0">Root</dt>
            <dd class="text-text font-mono text-xs break-all">{vault.root}</dd>
          </div>
          <div class="flex gap-3">
            <dt class="text-dim w-24 flex-shrink-0">Notes</dt>
            <dd class="text-text">{vault.notes}</dd>
          </div>
          <div class="flex gap-3">
            <dt class="text-dim w-24 flex-shrink-0">Live</dt>
            <dd class="flex items-center gap-2">
              <span class="w-2 h-2 rounded-full {$wsConnected ? 'bg-success' : 'bg-dim'}"></span>
              <span class="text-text">{$wsConnected ? 'connected' : 'offline'}</span>
            </dd>
          </div>
        </dl>
      {/if}
    </section>

    <!-- Sync status -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Git auto-sync</h2>
      {#if !sync}
        <Skeleton class="h-4 w-1/2" />
      {:else if !sync.enabled}
        <p class="text-sm text-dim leading-relaxed">
          Disabled. Pass <code class="text-xs">--sync</code> to <code class="text-xs">granit web</code> to enable periodic
          <code class="text-xs">git pull</code> + auto-commit/push when this server hosts a git-backed vault.
        </p>
      {:else}
        <dl class="space-y-1.5 text-sm">
          <div class="flex gap-3">
            <dt class="text-dim w-28 flex-shrink-0">Interval</dt>
            <dd class="text-text">{sync.interval ?? '—'}</dd>
          </div>
          <div class="flex gap-3">
            <dt class="text-dim w-28 flex-shrink-0">Last pull</dt>
            <dd class="text-text">{fmtTime(sync.lastPull)} ({sync.pulls ?? 0} total)</dd>
          </div>
          <div class="flex gap-3">
            <dt class="text-dim w-28 flex-shrink-0">Last push</dt>
            <dd class="text-text">{fmtTime(sync.lastPush)} ({sync.pushes ?? 0} total)</dd>
          </div>
          {#if sync.lastErr}
            <div class="flex gap-3">
              <dt class="text-dim w-28 flex-shrink-0">Error</dt>
              <dd class="text-error text-xs">{sync.lastErr}</dd>
            </div>
          {/if}
        </dl>
      {/if}
    </section>

    <!-- Keyboard shortcuts -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Keyboard shortcuts</h2>
      <ul class="space-y-1.5 text-sm">
        {#each shortcuts as s}
          <li class="flex items-baseline gap-3">
            <kbd class="text-xs font-mono px-2 py-0.5 bg-mantle border border-surface1 rounded text-text whitespace-nowrap">{s.keys}</kbd>
            <span class="text-subtext">{s.what}</span>
          </li>
        {/each}
      </ul>
    </section>

    <!-- About -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">About</h2>
      <p class="text-sm text-subtext leading-relaxed">
        Granit web — your vault, anywhere. Powered by the same data layer as the granit TUI;
        learn more at <a href="https://github.com/artaeon/granit" rel="noreferrer" target="_blank">github.com/artaeon/granit</a>.
      </p>
      <div class="mt-3">
        <button
          onclick={() => auth.clear()}
          class="text-xs text-error hover:underline"
        >
          Sign out (clears the bearer token from this device)
        </button>
      </div>
    </section>
  </div>
</div>
