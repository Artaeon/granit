<script lang="ts">
  // Settings page — state container + tab router. The four tabs
  // (general / ai / sync / vault) live in $lib/settings/Settings*Tab
  // and consume the slices of state they need via props. The script
  // here owns load + persistence + side-effect handlers; the
  // children are mostly markup.
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type OpenAIModelOption, type CalendarSource } from '$lib/api';
  import { onWsEvent, wsConnected } from '$lib/ws';
  import { errorMessage } from '$lib/util/errorMessage';
  import { relativeTime } from '$lib/util/relativeTime';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';
  import { profilesStore } from '$lib/stores/profiles';
  import { hiddenSections } from '$lib/stores/sidebar-ui';
  import { toast } from '$lib/components/toast';

  import SettingsPageHeader, { type SettingsTab } from '$lib/settings/SettingsPageHeader.svelte';
  import SettingsGeneralTab from '$lib/settings/SettingsGeneralTab.svelte';
  import SettingsAITab from '$lib/settings/SettingsAITab.svelte';
  import SettingsSyncTab from '$lib/settings/SettingsSyncTab.svelte';
  import SettingsVaultTab from '$lib/settings/SettingsVaultTab.svelte';

  import type { AppConfig, AppConfigPatch } from '$lib/api';

  // ── Tab state ───────────────────────────────────────────────────
  const TAB_KEY = 'granit.settings.tab';
  function loadTab(): SettingsTab {
    const v = loadStoredString(TAB_KEY, 'general');
    if (v === 'general' || v === 'ai' || v === 'sync' || v === 'vault') return v;
    return 'general';
  }
  let tab = $state<SettingsTab>(loadTab());
  $effect(() => saveStoredString(TAB_KEY, tab));

  // ── Core load state ─────────────────────────────────────────────
  type SyncStatus = {
    enabled: boolean;
    interval?: string;
    lastPull?: string;
    lastPush?: string;
    pulls?: number;
    pushes?: number;
    lastErr?: string;
  };

  let vault = $state<{ root: string; notes: number } | null>(null);
  let sync = $state<SyncStatus | null>(null);
  let syncBusy = $state(false);
  let authStatus = $state<{ hasPassword: boolean; sessionCount?: number; setupAt?: string } | null>(null);
  let devices = $state<import('$lib/api').Device[]>([]);
  let revokeBusy = $state<string | null>(null);

  // Curated config from /api/v1/config — same file the TUI reads.
  let appCfg = $state<AppConfig | null>(null);
  let configBusy = $state(false);
  let openAIKeyBuf = $state('');
  let recurringTasksBuf = $state('');

  // Curated OpenAI model picker; lazy-loaded the first time the AI
  // provider settings is rendered (saves a request on tabs that
  // never open it).
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

  async function patchConfig(patch: AppConfigPatch) {
    if (!appCfg) return;
    configBusy = true;
    try {
      appCfg = await api.patchConfig(patch);
    } catch (e) {
      pwError = errorMessage(e);
    } finally {
      configBusy = false;
    }
  }

  async function commitOpenAIKey() {
    if (!openAIKeyBuf.trim()) return;
    await patchConfig({ openai_key: openAIKeyBuf.trim() });
    openAIKeyBuf = '';
  }
  async function clearOpenAIKey() {
    // No native confirm — the ConfirmButton on the AI tab arms
    // itself before calling this.
    await patchConfig({ openai_key: '' });
    openAIKeyBuf = '';
  }

  function syncRecurringBuf(c: AppConfig) {
    recurringTasksBuf = (c.daily_recurring_tasks ?? []).join('\n');
  }
  async function commitRecurringTasks() {
    const list = recurringTasksBuf.split('\n').map((s) => s.trim()).filter(Boolean);
    await patchConfig({ daily_recurring_tasks: list });
  }

  // Calendar sources — list lives on the calendar page; here we
  // just refresh on WS events so any badge / count stays accurate.
  let calSources = $state<CalendarSource[]>([]);
  async function loadCalSources() {
    try {
      const r = await api.listCalendarSources();
      calSources = r.sources;
    } catch {
      calSources = [];
    }
  }

  // Password-change panel state — kept in the parent because the
  // form is small and the success path needs to clear auth.
  let pwOpen = $state(false);
  let pwOld = $state('');
  let pwNew = $state('');
  let pwConfirm = $state('');
  let pwBusy = $state(false);
  let pwError = $state('');
  let pwSuccess = $state('');
  function setPwOpen(v: boolean) { pwOpen = v; }
  function resetPwForm() {
    pwError = ''; pwOld = ''; pwNew = ''; pwConfirm = '';
  }

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

  async function syncNow() {
    if (syncBusy) return;
    syncBusy = true;
    try {
      await api.req('/sync', { method: 'POST' });
      toast.success('sync triggered');
      setTimeout(async () => {
        try { sync = await api.req<SyncStatus>('/sync'); } catch {}
      }, 1500);
    } catch (e) {
      toast.error('sync failed: ' + errorMessage(e));
    } finally {
      syncBusy = false;
    }
  }

  async function revokeDevice(id: string) {
    revokeBusy = id;
    try {
      await api.revokeDevice(id);
      devices = devices.filter((d) => d.id !== id);
    } catch (e) {
      pwError = errorMessage(e);
    } finally {
      revokeBusy = null;
    }
  }

  const fmtRelative = (iso: string) =>
    relativeTime(iso, {
      dateThresholdDays: 7,
      dateFormatter: (d) => d.toLocaleDateString()
    }) || iso;

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
      setTimeout(() => { auth.clear(); }, 1500);
    } catch (e) {
      pwError = errorMessage(e);
    } finally {
      pwBusy = false;
    }
  }

  async function revokeAllSessions() {
    try {
      await api.authRevokeAll();
      auth.clear();
    } catch (e) {
      pwError = errorMessage(e);
    }
  }

  // Profile activation — store owns the API call; this wrapper
  // keeps the toast + busy indicator local to settings.
  let profileBusyId = $state<string | null>(null);
  let hiddenSectionsCount = $derived.by(() => {
    void $hiddenSections;
    return Object.values($hiddenSections).filter(Boolean).length;
  });
  async function activateProfile(id: string) {
    if (id === $profilesStore.activeId) return;
    profileBusyId = id;
    try {
      await profilesStore.activate(id);
      const name = $profilesStore.profiles.find((p) => p.id === id)?.name ?? id;
      toast.success(`Profile switched to ${name}`);
    } catch (e) {
      toast.error("Couldn't switch profile: " + errorMessage(e));
    } finally {
      profileBusyId = null;
    }
  }

  // ── AI prefs / audit / snapshot / status ─────────────────────────
  let aiPrefs = $state<{
    features: Record<string, { enabled: boolean; provider?: string }>;
    redaction_enabled: boolean;
    disabled_redaction?: string[];
    default_provider?: string;
  }>({
    features: {},
    redaction_enabled: true,
    default_provider: 'ollama'
  });
  let aiPrefsSaveTimer: ReturnType<typeof setTimeout> | null = null;
  let aiAudit = $state<{
    timestamp: string;
    feature: string;
    provider?: string;
    model?: string;
    prompt_size_bytes: number;
    response_size_bytes?: number;
    prompt_tokens?: number;
    completion_tokens?: number;
    cost_micro_cents?: number;
    redactions?: { name: string; count: number }[];
    error?: string;
  }[]>([]);
  let aiSnapshotJSON = $state('');
  let aiStatus = $state<{
    sabbath_active: boolean;
    global_provider: string;
    global_model: string;
    redaction: boolean;
    default_provider?: string;
    features: Record<string, { enabled: boolean; provider: string; model: string; source: string }>;
  } | null>(null);

  async function loadAIPrefs() {
    try {
      const r = await api.getAIPrefs();
      if (r.prefs) aiPrefs = r.prefs;
    } catch {}
  }
  function saveAIPrefs() {
    if (aiPrefsSaveTimer) clearTimeout(aiPrefsSaveTimer);
    aiPrefsSaveTimer = setTimeout(async () => {
      try {
        await api.putAIPrefs(aiPrefs);
      } catch (err) {
        toast.error('Save failed: ' + errorMessage(err));
      }
    }, 400);
  }
  async function toggleAIFeature(id: string, enabled: boolean) {
    if (!aiPrefs.features[id]) aiPrefs.features[id] = { enabled };
    else aiPrefs.features[id] = { ...aiPrefs.features[id], enabled };
    saveAIPrefs();
    setTimeout(() => { void loadAIStatus(); }, 600);
  }
  async function loadAIStatus() {
    try { aiStatus = await api.getAIStatus(); } catch {}
  }
  async function loadAIAudit() {
    try {
      const r = await api.getAIAudit();
      aiAudit = r.entries ?? [];
    } catch {}
  }
  async function clearAIAudit() {
    try {
      await api.clearAIAudit();
      aiAudit = [];
      toast.success('AI history cleared');
    } catch (err) {
      toast.error('Clear failed: ' + errorMessage(err));
    }
  }
  async function loadAISnapshot() {
    try {
      const r = await api.getAISnapshot();
      aiSnapshotJSON = JSON.stringify(r.snapshot, null, 2);
    } catch (err) {
      aiSnapshotJSON = `// Failed to load snapshot: ${errorMessage(err)}`;
    }
  }

  // ── Web search config ───────────────────────────────────────────
  let webSearchCfg = $state<{
    provider: string;
    brave_key_set: boolean;
    max_results: number;
  }>({ provider: 'duckduckgo', brave_key_set: false, max_results: 5 });
  let webSearchBraveKeyBuf = $state('');
  let webSearchBusy = $state(false);
  async function loadWebSearchCfg() {
    try { webSearchCfg = await api.getWebSearchConfig(); } catch {}
  }
  async function patchWebSearch(p: Partial<{ provider: string; brave_key: string; max_results: number }>) {
    webSearchBusy = true;
    try {
      webSearchCfg = await api.patchWebSearchConfig(p);
    } catch (err) {
      toast.error('Save failed: ' + errorMessage(err));
    } finally {
      webSearchBusy = false;
    }
  }
  async function commitBraveKey() {
    if (!webSearchBraveKeyBuf.trim()) return;
    await patchWebSearch({ brave_key: webSearchBraveKeyBuf.trim() });
    webSearchBraveKeyBuf = '';
  }
  async function clearBraveKey() {
    // Confirmation happens in the ConfirmButton on the AI tab.
    await patchWebSearch({ brave_key: '' });
    webSearchBraveKeyBuf = '';
  }

  // ── Notification preferences + push status ──────────────────────
  let prefs = $state<{
    calendar: { enabled: boolean };
    tasks: { enabled: boolean; due_today_time: string };
    deadlines: { enabled: boolean; days_before: number[]; at_time: string };
    quiet_hours: { enabled: boolean; start: string; end: string };
    default_event_reminder: number;
  }>({
    calendar: { enabled: true },
    tasks: { enabled: true, due_today_time: '09:00' },
    deadlines: { enabled: true, days_before: [7, 3, 1, 0], at_time: '09:00' },
    quiet_hours: { enabled: false, start: '22:00', end: '07:00' },
    default_event_reminder: 15
  });
  let prefsSaveTimer: ReturnType<typeof setTimeout> | null = null;
  async function loadPrefs() {
    try {
      const r = await api.getNotificationPrefs();
      if (r.prefs) prefs = r.prefs;
    } catch {}
  }
  function savePrefs() {
    if (prefsSaveTimer) clearTimeout(prefsSaveTimer);
    prefsSaveTimer = setTimeout(async () => {
      try {
        await api.putNotificationPrefs(prefs);
      } catch (err) {
        toast.error('Save failed: ' + errorMessage(err));
      }
    }, 400);
  }
  function toggleDeadlineOffset(off: number) {
    const list = prefs.deadlines.days_before;
    const i = list.indexOf(off);
    if (i >= 0) prefs.deadlines.days_before = list.filter((d) => d !== off);
    else prefs.deadlines.days_before = [...list, off].sort((a, b) => b - a);
  }

  let pushStatus = $state<{ supported: boolean; permission: NotificationPermission; subscribed: boolean; paused?: boolean }>({
    supported: false,
    permission: 'default',
    subscribed: false,
    paused: false
  });
  let pushBusy = $state(false);
  async function loadPush() {
    try {
      const m = await import('$lib/notifications');
      pushStatus = await m.getStatus();
    } catch {}
  }
  async function enablePush() {
    pushBusy = true;
    try {
      const m = await import('$lib/notifications');
      pushStatus = await m.subscribe();
    } catch (err) {
      toast.error('Subscribe failed: ' + errorMessage(err));
    } finally {
      pushBusy = false;
    }
  }
  async function pausePush() {
    pushBusy = true;
    try {
      const m = await import('$lib/notifications');
      await m.setPaused(true);
      pushStatus = await m.getStatus();
      toast.success('Notifications paused');
    } catch (err) {
      toast.error('Pause failed: ' + errorMessage(err));
    } finally {
      pushBusy = false;
    }
  }
  async function resumePush() {
    pushBusy = true;
    try {
      const m = await import('$lib/notifications');
      await m.setPaused(false);
      pushStatus = await m.getStatus();
      toast.success('Notifications resumed');
    } catch (err) {
      toast.error('Resume failed: ' + errorMessage(err));
    } finally {
      pushBusy = false;
    }
  }
  async function disablePush() {
    pushBusy = true;
    try {
      const m = await import('$lib/notifications');
      await m.unsubscribe();
      pushStatus = await m.getStatus();
    } catch (err) {
      toast.error('Unsubscribe failed: ' + errorMessage(err));
    } finally {
      pushBusy = false;
    }
  }
  async function testPush() {
    pushBusy = true;
    try {
      const m = await import('$lib/notifications');
      const r = await m.sendTest();
      if (r.sent > 0) toast.success(`Test sent to ${r.sent} device${r.sent === 1 ? '' : 's'}`);
      else toast.warning('No devices subscribed.');
    } catch (err) {
      toast.error('Test failed: ' + errorMessage(err));
    } finally {
      pushBusy = false;
    }
  }

  // ── Autocommit ──────────────────────────────────────────────────
  let autocommit = $state<{ enabled: boolean; isGitRepo: boolean }>({ enabled: false, isGitRepo: false });
  let autocommitSaving = $state(false);
  async function loadAutocommit() {
    try { autocommit = await api.getAutocommit(); } catch {}
  }
  async function toggleAutocommit(enabled: boolean) {
    autocommitSaving = true;
    try {
      autocommit = await api.putAutocommit(enabled);
    } catch {
      autocommit = { ...autocommit };
    } finally {
      autocommitSaving = false;
    }
  }

  // ── Stoicera integration ────────────────────────────────────────
  let stoiceraSettings = $state<{
    enabled: boolean;
    venture_name: string;
    token_masked: string;
    has_token: boolean;
  }>({ enabled: false, venture_name: '', token_masked: '', has_token: false });
  let stoiceraSaving = $state(false);
  let stoiceraVentureBuf = $state('');
  let stoiceraTokenRevealed = $state<string | null>(null);
  function setStoiceraTokenRevealed(v: string | null) { stoiceraTokenRevealed = v; }

  async function loadStoiceraSettings() {
    try {
      stoiceraSettings = await api.getStoiceraSettings();
      stoiceraVentureBuf = stoiceraSettings.venture_name;
    } catch {}
  }
  async function toggleStoicera(enabled: boolean) {
    stoiceraSaving = true;
    try {
      stoiceraSettings = await api.patchStoiceraSettings({ enabled });
      stoiceraTokenRevealed = null;
    } catch (e) {
      toast.error('Stoicera: ' + errorMessage(e));
    } finally {
      stoiceraSaving = false;
    }
  }
  async function commitStoiceraVenture() {
    if (stoiceraVentureBuf === stoiceraSettings.venture_name) return;
    stoiceraSaving = true;
    try {
      stoiceraSettings = await api.patchStoiceraSettings({ venture_name: stoiceraVentureBuf });
    } catch (e) {
      toast.error('Stoicera: ' + errorMessage(e));
      stoiceraVentureBuf = stoiceraSettings.venture_name;
    } finally {
      stoiceraSaving = false;
    }
  }
  async function regenerateStoiceraToken() {
    // ConfirmButton on the Sync tab arms before invoking.
    stoiceraSaving = true;
    try {
      stoiceraSettings = await api.patchStoiceraSettings({ regenerate: true });
      stoiceraTokenRevealed = null;
    } catch (e) {
      toast.error('Regenerate: ' + errorMessage(e));
    } finally {
      stoiceraSaving = false;
    }
  }
  async function revealStoiceraToken() {
    try {
      const r = await api.getStoiceraToken();
      stoiceraTokenRevealed = r.token;
    } catch (e) {
      toast.error('Show token: ' + errorMessage(e));
    }
  }
  async function copyStoiceraToken() {
    try {
      const r = await api.getStoiceraToken();
      await navigator.clipboard.writeText(r.token);
      toast.success('Token copied to clipboard');
    } catch (e) {
      toast.error('Copy: ' + errorMessage(e));
    }
  }

  // ── Format helper used by the Sync tab ──────────────────────────
  function fmtTime(s?: string): string {
    if (!s || s.startsWith('0001-')) return '—';
    return new Date(s).toLocaleString();
  }

  // ── Mount + WS plumbing ─────────────────────────────────────────
  onMount(() => {
    load();
    void profilesStore.ensureLoaded();
    void loadCalSources();
    void loadAutocommit();
    void loadStoiceraSettings();
    void loadPush();
    void loadPrefs();
    void loadAIPrefs();
    void loadAIStatus();
    void loadWebSearchCfg();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path?.startsWith('calendars/')) {
        void loadCalSources();
      }
      load();
    });
  });
</script>

<div class="h-full overflow-y-auto bg-base">
  <SettingsPageHeader {tab} onSelect={(t) => (tab = t)} />

  <div class="p-3 sm:p-4 lg:p-6 max-w-3xl mx-auto">
    {#if tab === 'general'}
      <SettingsGeneralTab
        {appCfg}
        {configBusy}
        bind:recurringTasksBuf
        {hiddenSectionsCount}
        {profileBusyId}
        {patchConfig}
        {commitRecurringTasks}
        {activateProfile}
      />
    {:else if tab === 'ai'}
      <SettingsAITab
        {appCfg}
        {configBusy}
        bind:openAIKeyBuf
        {openAIModels}
        bind:aiPrefs
        {aiStatus}
        {aiAudit}
        {aiSnapshotJSON}
        {webSearchCfg}
        bind:webSearchBraveKeyBuf
        {webSearchBusy}
        {patchConfig}
        {commitOpenAIKey}
        {clearOpenAIKey}
        {ensureOpenAIModels}
        {saveAIPrefs}
        {toggleAIFeature}
        {loadAIStatus}
        {loadAIAudit}
        {clearAIAudit}
        {loadAISnapshot}
        {patchWebSearch}
        {commitBraveKey}
        {clearBraveKey}
      />
    {:else if tab === 'sync'}
      <SettingsSyncTab
        {sync}
        {syncBusy}
        {pushStatus}
        {pushBusy}
        bind:prefs
        {autocommit}
        {autocommitSaving}
        {stoiceraSettings}
        {stoiceraSaving}
        bind:stoiceraVentureBuf
        {stoiceraTokenRevealed}
        {devices}
        {revokeBusy}
        {syncNow}
        {enablePush}
        {pausePush}
        {resumePush}
        {disablePush}
        {testPush}
        {savePrefs}
        {toggleDeadlineOffset}
        {toggleAutocommit}
        {toggleStoicera}
        {commitStoiceraVenture}
        {regenerateStoiceraToken}
        {revealStoiceraToken}
        {copyStoiceraToken}
        {setStoiceraTokenRevealed}
        {revokeDevice}
        {fmtTime}
        {fmtRelative}
      />
    {:else if tab === 'vault'}
      <SettingsVaultTab
        {vault}
        wsConnected={$wsConnected}
        {authStatus}
        {pwOpen}
        bind:pwOld
        bind:pwNew
        bind:pwConfirm
        {pwBusy}
        {pwError}
        {pwSuccess}
        {setPwOpen}
        {resetPwForm}
        {changePassword}
        {revokeAllSessions}
      />
    {/if}
  </div>
</div>
