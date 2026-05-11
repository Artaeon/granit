<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type OpenAIModelOption, type CalendarSource } from '$lib/api';
  import { onWsEvent, wsConnected } from '$lib/ws';
  import { theme, themeLabel, type Theme } from '$lib/stores/theme';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import RecurringEditor from '$lib/components/RecurringEditor.svelte';
  import { modulesStore } from '$lib/stores/modules';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { relativeTime } from '$lib/util/relativeTime';

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
  let syncBusy = $state(false);
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
      pwError = errorMessage(e);
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
      pwError = errorMessage(e);
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

  // Manual one-off sync — POST /api/v1/sync triggers the same cycle
  // the daemon runs every interval (pull → commit local changes →
  // push). Only available when --sync was passed at startup; the
  // button hides itself when sync.enabled is false.
  // Refreshes the status fields a moment later so the user sees the
  // updated last-pull / last-push timestamps.
  async function syncNow() {
    if (syncBusy) return;
    syncBusy = true;
    try {
      await api.req('/sync', { method: 'POST' });
      toast.success('sync triggered');
      // Give the background goroutine a beat to finish before we
      // re-read status. 1.5s covers the typical pull+push round-trip
      // on a small vault; longer-running syncs land later via the
      // user's next visit / refresh.
      setTimeout(async () => {
        try {
          sync = await api.req<SyncStatus>('/sync');
        } catch {}
      }, 1500);
    } catch (e) {
      const msg = errorMessage(e);
      toast.error('sync failed: ' + msg);
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

  // Falls back to the calendar date past 7 days; the locale full
  // date format used here is fine for the audit-log "last activity"
  // surface — older entries don't need precision.
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
      // Server invalidates the current token on password change. Clear
      // local auth so the user routes back to login.
      setTimeout(() => { auth.clear(); }, 1500);
    } catch (e) {
      pwError = errorMessage(e);
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
      pwError = errorMessage(e);
    }
  }
  onMount(() => {
    load();
    void modulesStore.ensureLoaded();
    void loadCalSources();
    void loadAutocommit();
    void loadPush();
    void loadPrefs();
    void loadAIPrefs();
    void loadAIStatus();
    return onWsEvent((ev) => {
      // Watch for ICS file mutations so the calendars list refreshes
      // when an event is created from another tab.
      if (ev.type === 'state.changed' && ev.path?.startsWith('calendars/')) {
        void loadCalSources();
      }
      load();
    });
  });

  // AI feature preferences + audit log + snapshot peek. The
  // foundation pieces (Context Engine, redaction, audit) are
  // always-on; this is the user-facing layer.
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
  let aiAuditOpen = $state(false);
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
  let aiSnapshotOpen = $state(false);
  let aiSnapshotJSON = $state('');
  // /ai/status — what each feature would actually run with right
  // now. Populates the small pill next to each toggle so the user
  // sees "via Ollama (llama3.2)" without having to fire a request.
  let aiStatus = $state<{
    sabbath_active: boolean;
    global_provider: string;
    global_model: string;
    redaction: boolean;
    default_provider?: string;
    features: Record<string, { enabled: boolean; provider: string; model: string; source: string }>;
  } | null>(null);
  async function loadAIStatus() {
    try { aiStatus = await api.getAIStatus(); } catch {}
  }

  // Usage rollup over the audit list. Pure derivation — no extra
  // wire calls. Today + last 7 days count requests, bytes, errors;
  // gives the user a "what did this week cost" answer without us
  // having to write a totals endpoint. Bytes are an honest stand-in
  // for tokens (we don't store actual token counts on the server),
  // so we deliberately don't try to estimate dollar cost — bytes →
  // tokens is provider-specific and a wrong number is worse than
  // none.
  function formatBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / 1024 / 1024).toFixed(1)} MB`;
  }
  // Render micro-cents as $X.XXXX with trailing-zero trim. Mirrors
  // agentruntime.FormatCents on the Go side so the UI agrees with
  // any future debug logs. Returns "—" for the not-priced case
  // (Ollama, unknown OpenAI model snapshots).
  function formatCost(microCents?: number): string {
    if (!microCents || microCents <= 0) return '—';
    const dollars = microCents / 1_000_000 / 100; // µcents → cents → dollars
    if (dollars < 0.0001) return '<$0.0001';
    // Show up to 4 decimals, trim trailing zeros.
    let s = dollars.toFixed(4);
    s = s.replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '');
    return '$' + s;
  }
  function formatTokens(n: number): string {
    if (n < 1000) return `${n}`;
    return `${(n / 1000).toFixed(1)}k`;
  }
  // Per-feature usage breakdown — same window as the headline
  // rollup tiles (today + last 7 days) but bucketed by feature so
  // the user can see "Daily briefing burned $0.012 today, Inbox
  // triage $0.003" without scrolling the whole audit list. Sorted
  // by cost desc so the expensive features land at the top.
  const aiUsageByFeature = $derived.by(() => {
    const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
    const buckets = new Map<string, { count: number; tokens: number; cost: number }>();
    for (const e of aiAudit) {
      const t = new Date(e.timestamp).getTime();
      if (t < sevenDaysAgo) continue;
      const key = e.feature || 'unknown';
      const b = buckets.get(key) ?? { count: 0, tokens: 0, cost: 0 };
      b.count++;
      b.tokens += (e.prompt_tokens ?? 0) + (e.completion_tokens ?? 0);
      b.cost += e.cost_micro_cents ?? 0;
      buckets.set(key, b);
    }
    return Array.from(buckets.entries())
      .map(([feature, v]) => ({ feature, ...v }))
      .sort((a, b) => b.cost - a.cost || b.count - a.count);
  });

  const aiUsage = $derived.by(() => {
    const now = Date.now();
    const todayStart = new Date(); todayStart.setHours(0, 0, 0, 0);
    const sevenDaysAgo = now - 7 * 24 * 60 * 60 * 1000;
    let todayN = 0, todayIn = 0, todayOut = 0, todayErr = 0, todayPT = 0, todayCT = 0, todayCost = 0;
    let weekN = 0, weekIn = 0, weekOut = 0, weekErr = 0, weekPT = 0, weekCT = 0, weekCost = 0;
    for (const e of aiAudit) {
      const t = new Date(e.timestamp).getTime();
      const inB = e.prompt_size_bytes ?? 0;
      const outB = e.response_size_bytes ?? 0;
      const pT = e.prompt_tokens ?? 0;
      const cT = e.completion_tokens ?? 0;
      const cost = e.cost_micro_cents ?? 0;
      if (t >= todayStart.getTime()) {
        todayN++;
        todayIn += inB;
        todayOut += outB;
        todayPT += pT;
        todayCT += cT;
        todayCost += cost;
        if (e.error) todayErr++;
      }
      if (t >= sevenDaysAgo) {
        weekN++;
        weekIn += inB;
        weekOut += outB;
        weekPT += pT;
        weekCT += cT;
        weekCost += cost;
        if (e.error) weekErr++;
      }
    }
    return {
      todayN, todayIn, todayOut, todayErr, todayPT, todayCT, todayCost,
      weekN, weekIn, weekOut, weekErr, weekPT, weekCT, weekCost
    };
  });

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
        const t = await import('$lib/components/toast');
        t.toast.error('Save failed: ' + (errorMessage(err)));
      }
    }, 400);
  }
  async function toggleAIFeature(id: string, enabled: boolean) {
    if (!aiPrefs.features[id]) aiPrefs.features[id] = { enabled };
    else aiPrefs.features[id] = { ...aiPrefs.features[id], enabled };
    saveAIPrefs();
    // Refresh runtime status after a short delay so the resolved
    // provider/model pill updates without a page reload. Long enough
    // to let the debounced PUT land first.
    setTimeout(() => { void loadAIStatus(); }, 600);
  }
  async function loadAIAudit() {
    try {
      const r = await api.getAIAudit();
      aiAudit = r.entries ?? [];
    } catch {}
  }
  async function clearAIAudit() {
    if (!confirm('Clear the AI audit log? This permanently deletes the record of every AI request.')) return;
    try {
      await api.clearAIAudit();
      aiAudit = [];
      const t = await import('$lib/components/toast');
      t.toast.success('AI history cleared');
    } catch (err) {
      const t = await import('$lib/components/toast');
      t.toast.error('Clear failed: ' + (errorMessage(err)));
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

  // Notification preferences. Per-category toggles + quiet
  // hours + defaults, mirrored from .granit/notifications.json.
  // Saved-on-change with a 400ms debounce so dragging a time
  // slider doesn't fire a PUT per movement.
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
        const t = await import('$lib/components/toast');
        t.toast.error('Save failed: ' + (errorMessage(err)));
      }
    }, 400);
  }
  function toggleDeadlineOffset(off: number) {
    const list = prefs.deadlines.days_before;
    const i = list.indexOf(off);
    if (i >= 0) prefs.deadlines.days_before = list.filter((d) => d !== off);
    else prefs.deadlines.days_before = [...list, off].sort((a, b) => b - a);
  }

  // Push notifications state. Mirrors the SW + browser
  // PushManager state plus a 'subscribed' flag the server has
  // recorded. enablePush / disablePush wrap the helper from
  // $lib/notifications which handles permission + subscribe call
  // against the server's VAPID key.
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
      const m = errorMessage(err);
      const t = await import('$lib/components/toast');
      t.toast.error('Subscribe failed: ' + m);
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
      const t = await import('$lib/components/toast');
      t.toast.success('Notifications paused');
    } catch (err) {
      const m = errorMessage(err);
      const t = await import('$lib/components/toast');
      t.toast.error('Pause failed: ' + m);
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
      const t = await import('$lib/components/toast');
      t.toast.success('Notifications resumed');
    } catch (err) {
      const m = errorMessage(err);
      const t = await import('$lib/components/toast');
      t.toast.error('Resume failed: ' + m);
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
      const m = errorMessage(err);
      const t = await import('$lib/components/toast');
      t.toast.error('Unsubscribe failed: ' + m);
    } finally {
      pushBusy = false;
    }
  }
  async function testPush() {
    pushBusy = true;
    try {
      const m = await import('$lib/notifications');
      const r = await m.sendTest();
      const t = await import('$lib/components/toast');
      if (r.sent > 0) t.toast.success(`Test sent to ${r.sent} device${r.sent === 1 ? '' : 's'}`);
      else t.toast.warning('No devices subscribed.');
    } catch (err) {
      const e = errorMessage(err);
      const t = await import('$lib/components/toast');
      t.toast.error('Test failed: ' + e);
    } finally {
      pushBusy = false;
    }
  }

  // Autocommit setting state. Loaded from /api/v1/autocommit on
  // mount; toggle saves immediately (no debounce since this is a
  // single boolean, not a per-click flurry like Modules).
  let autocommit = $state<{ enabled: boolean; isGitRepo: boolean }>({
    enabled: false,
    isGitRepo: false
  });
  let autocommitSaving = $state(false);
  async function loadAutocommit() {
    try {
      autocommit = await api.getAutocommit();
    } catch {}
  }
  async function toggleAutocommit(enabled: boolean) {
    autocommitSaving = true;
    try {
      autocommit = await api.putAutocommit(enabled);
    } catch (e) {
      // Revert on error so the checkbox reflects reality.
      autocommit = { ...autocommit };
    } finally {
      autocommitSaving = false;
    }
  }

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
      toast.error(errorMessage(e));
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

    <!-- Theme — two simple modes plus system-follow. Granit is
         a strict monochrome surface; this control picks the side
         of black/white you read against. -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Theme</h2>
      <div class="grid grid-cols-3 gap-2">
        {#each themeOptions as t}
          {@const active = $theme === t}
          <button
            onclick={() => theme.set(t)}
            class="px-3 py-3 rounded-lg border flex flex-col items-center gap-1.5 transition-colors
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
      <p class="text-xs text-dim mt-2 leading-relaxed">
        System follows your OS setting and updates live.
      </p>
    </section>

    <!-- Push notifications. The most-asked feature for any
         self-hosted calendar tool: reminders that fire when the
         tab is closed. Opt-in because the subscribe flow needs
         permission + a stored endpoint. -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <header class="flex items-baseline justify-between mb-2">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Reminders</h2>
        {#if pushBusy}
          <span class="text-[10px] uppercase tracking-wider text-dim">working…</span>
        {/if}
      </header>
      {#if !pushStatus.supported}
        <p class="text-sm text-dim">
          Push notifications aren't supported in this browser. On iOS this works only in an installed PWA on iOS 16.4+.
        </p>
      {:else if pushStatus.permission === 'denied'}
        <p class="text-sm text-warning">
          Notifications are blocked at the browser level. Enable them in your browser's site settings, then return here.
        </p>
      {:else if !pushStatus.subscribed}
        <p class="text-sm text-dim mb-3">
          Reminds you about upcoming events even when the tab is closed. Set a "Remind me N min before" on any event to fire a push.
        </p>
        <button
          onclick={() => void enablePush()}
          disabled={pushBusy}
          class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
        >Enable mobile reminders</button>
      {:else}
        <!-- Subscribed. Two states:
             • active   → "Pause" button (keeps sub, server stops pushing)
             • paused   → "Resume" button (re-enables without re-permission)
             Plus "Send test" to verify the endpoint actually delivers,
             and "Unsubscribe this device" as a small secondary option
             for permanent removal. -->
        {#if pushStatus.paused}
          <p class="text-sm text-warning mb-3">
            ⏸ Notifications paused on this device. Subscription is still active — resume any time without re-granting permission.
          </p>
        {:else}
          <p class="text-sm text-success mb-3">
            ✓ Subscribed on this device. Set a reminder on any event in the calendar to receive a push.
          </p>
        {/if}
        <div class="flex flex-wrap items-center gap-2">
          {#if pushStatus.paused}
            <button
              onclick={() => void resumePush()}
              disabled={pushBusy}
              class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
            >Resume notifications</button>
          {:else}
            <button
              onclick={() => void pausePush()}
              disabled={pushBusy}
              class="px-3 py-1.5 bg-surface0 text-warning rounded text-sm hover:bg-surface0 disabled:opacity-50"
              title="Stop receiving notifications without unsubscribing"
            >Pause notifications</button>
          {/if}
          <button
            onclick={() => void testPush()}
            disabled={pushBusy || pushStatus.paused}
            class="px-3 py-1.5 bg-surface1 text-subtext rounded text-sm hover:bg-surface2 disabled:opacity-50"
            title={pushStatus.paused ? 'Resume first to send a test' : 'Send a test push to all subscribed devices'}
          >Send test</button>
          <span class="flex-1"></span>
          <button
            onclick={() => void disablePush()}
            disabled={pushBusy}
            class="px-3 py-1.5 text-dim hover:text-error text-sm"
            title="Permanently remove this device's subscription. Re-enabling will require granting permission again."
          >Unsubscribe</button>
        </div>
      {/if}

      <!-- Per-category preferences. Hidden until at least one
           device is subscribed — toggles aren't useful when no
           push can fire anyway. The whole panel is one form so
           a flurry of changes coalesces into one PUT. -->
      {#if pushStatus.subscribed}
        <div class="mt-5 pt-4 border-t border-surface1 space-y-3">
          <h3 class="text-[10px] uppercase tracking-wider text-dim font-semibold">What to remind me about</h3>

          <!-- Calendar events. Master toggle + the default
               reminder offset that pre-fills the create form. -->
          <div class="flex items-start gap-3">
            <input
              type="checkbox"
              bind:checked={prefs.calendar.enabled}
              onchange={() => void savePrefs()}
              class="mt-1 w-4 h-4 accent-primary"
            />
            <div class="flex-1 min-w-0">
              <div class="text-sm text-text">📅 Calendar events</div>
              <div class="text-[11px] text-dim">Reminders fire at the configured "remind me N min before" on each event.</div>
              <label class="mt-2 flex items-center gap-2 text-[11px] text-dim">
                Default reminder offset
                <select
                  bind:value={prefs.default_event_reminder}
                  onchange={() => void savePrefs()}
                  class="bg-surface0 border border-surface1 rounded px-2 py-1 text-text text-xs"
                >
                  <option value={0}>off</option>
                  <option value={5}>5 minutes before</option>
                  <option value={15}>15 minutes before</option>
                  <option value={30}>30 minutes before</option>
                  <option value={60}>1 hour before</option>
                  <option value={1440}>1 day before</option>
                </select>
              </label>
            </div>
          </div>

          <!-- Tasks. Master toggle + the time-of-day for the
               daily "tasks due today" summary push. -->
          <div class="flex items-start gap-3">
            <input
              type="checkbox"
              bind:checked={prefs.tasks.enabled}
              onchange={() => void savePrefs()}
              class="mt-1 w-4 h-4 accent-primary"
            />
            <div class="flex-1 min-w-0">
              <div class="text-sm text-text">✓ Tasks due today</div>
              <div class="text-[11px] text-dim">One morning summary listing tasks whose due date is today.</div>
              <label class="mt-2 flex items-center gap-2 text-[11px] text-dim">
                Reminder time
                <input
                  type="time"
                  bind:value={prefs.tasks.due_today_time}
                  onchange={() => void savePrefs()}
                  class="bg-surface0 border border-surface1 rounded px-2 py-1 text-text text-xs font-mono tabular-nums"
                />
              </label>
            </div>
          </div>

          <!-- Deadlines. Master toggle + days-before list +
               time-of-day. The days-before list is rendered as
               a row of toggle pills so the user sees + edits the
               offsets at a glance. -->
          <div class="flex items-start gap-3">
            <input
              type="checkbox"
              bind:checked={prefs.deadlines.enabled}
              onchange={() => void savePrefs()}
              class="mt-1 w-4 h-4 accent-primary"
            />
            <div class="flex-1 min-w-0">
              <div class="text-sm text-text">⏰ Deadlines</div>
              <div class="text-[11px] text-dim">Fire at each chosen offset before a deadline (one push per offset).</div>
              <div class="mt-2 flex items-center gap-1 flex-wrap">
                {#each [14, 7, 3, 1, 0] as off}
                  {@const active = prefs.deadlines.days_before.includes(off)}
                  <button
                    type="button"
                    onclick={() => { toggleDeadlineOffset(off); void savePrefs(); }}
                    class="px-2 py-1 text-[11px] rounded border transition-colors
                      {active ? 'bg-surface1 border-primary text-primary' : 'bg-surface0 border-surface1 text-dim hover:border-primary'}"
                  >{off === 0 ? 'day-of' : `${off}d`}</button>
                {/each}
              </div>
              <label class="mt-2 flex items-center gap-2 text-[11px] text-dim">
                Reminder time
                <input
                  type="time"
                  bind:value={prefs.deadlines.at_time}
                  onchange={() => void savePrefs()}
                  class="bg-surface0 border border-surface1 rounded px-2 py-1 text-text text-xs font-mono tabular-nums"
                />
              </label>
            </div>
          </div>

          <!-- Quiet hours. Suppresses ALL pushes during the
               window (any category). Wrap-around (e.g. 22:00 →
               07:00) handled server-side. -->
          <div class="flex items-start gap-3 pt-2 border-t border-surface1">
            <input
              type="checkbox"
              bind:checked={prefs.quiet_hours.enabled}
              onchange={() => void savePrefs()}
              class="mt-1 w-4 h-4 accent-primary"
            />
            <div class="flex-1 min-w-0">
              <div class="text-sm text-text">🌙 Quiet hours</div>
              <div class="text-[11px] text-dim">No pushes between these times — across all categories.</div>
              <div class="mt-2 flex items-center gap-2 text-[11px] text-dim">
                From
                <input
                  type="time"
                  bind:value={prefs.quiet_hours.start}
                  onchange={() => void savePrefs()}
                  class="bg-surface0 border border-surface1 rounded px-2 py-1 text-text text-xs font-mono tabular-nums"
                />
                to
                <input
                  type="time"
                  bind:value={prefs.quiet_hours.end}
                  onchange={() => void savePrefs()}
                  class="bg-surface0 border border-surface1 rounded px-2 py-1 text-text text-xs font-mono tabular-nums"
                />
              </div>
            </div>
          </div>
        </div>
      {/if}
    </section>

    <!-- AI features. Per-feature opt-in toggles + audit log + a
         "what AI sees" peek so the user has perfect transparency
         into what data MIGHT leave the device. Foundation pieces
         (Context Engine, redaction, audit) are always-on; the
         features themselves are opt-in via the toggles below. -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <header class="flex items-baseline justify-between mb-3">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium">AI features</h2>
        <span class="text-[10px] text-dim">opt-in · redacted · audited</span>
      </header>
      <p class="text-xs text-dim mb-3 leading-relaxed">
        Each feature checks the toggle before doing any work. Prompts are passed through a PII-redaction pass before they leave the device. Every request is recorded to an audit log you can inspect or clear below.
      </p>

      {#if aiStatus?.sabbath_active}
        <div class="mb-3 px-3 py-2 text-[11px] bg-surface0 border border-warning rounded text-warning">
          🕯️ Sabbath mode active — AI requests are paused today. Toggling features here is fine; calls just won't fire until Sabbath ends.
        </div>
      {/if}
      {#if aiStatus}
        <div class="mb-3 text-[11px] text-dim">
          Default backend: <span class="text-subtext font-mono">{aiStatus.global_provider} · {aiStatus.global_model}</span>
        </div>
      {/if}
      <div class="space-y-2">
        {#each [
          { id: 'daily_briefing',  label: 'Daily briefing',  desc: 'Morning summary: today\'s events + urgent tasks + 1 deadline.' },
          { id: 'weekly_review',   label: 'Weekly review',   desc: 'Friday/Sunday: drafts a Wins / Setbacks / Learned / Next-week review.' },
          { id: 'inbox_triage',    label: 'Inbox triage',    desc: 'Suggests priority + schedule for untriaged tasks.' },
          { id: 'deadline_detect', label: 'Deadline detect', desc: 'Reads open tasks without due dates and proposes one when the title carries a clear deadline signal.' },
          { id: 'annotate_note',   label: 'Annotate note',   desc: 'Reads a note and proposes 3-5 margin notes — questions, counter-arguments, "this matters" markers — anchored to specific lines. Review and accept each from the editor right rail.' },
          { id: 'summarise',       label: 'Summarise (existing)',  desc: 'In-editor "summarise selection / whole note" — already shipping.' },
          { id: 'extract_tasks',   label: 'Extract tasks (existing)', desc: 'In-editor "extract tasks from this note" — already shipping.' },
          { id: 'suggest_tags',    label: 'Suggest tags (existing)', desc: 'In-editor "suggest tags for this note" — already shipping.' },
          { id: 'rewrite',         label: 'Rewrite / improve (existing)', desc: 'In-editor selection rewriter — already shipping.' },
          { id: 'chat',            label: 'Chat (existing)', desc: 'The /chat page. Toggle off to disable entirely.' }
        ] as f}
          {@const cfg = aiPrefs.features[f.id] ?? { enabled: false }}
          {@const st = aiStatus?.features[f.id]}
          <label class="flex items-start gap-3 py-1.5 cursor-pointer">
            <input
              type="checkbox"
              checked={cfg.enabled}
              onchange={(e) => { void toggleAIFeature(f.id, (e.target as HTMLInputElement).checked); }}
              class="mt-1 w-4 h-4 accent-primary cursor-pointer"
            />
            <div class="flex-1 min-w-0">
              <div class="flex items-baseline gap-2 flex-wrap">
                <span class="text-sm text-text">{f.label}</span>
                {#if cfg.enabled && st}
                  <!-- Resolved provider+model pill. Tooltip explains
                       which override path took effect (feature / default /
                       global) so the user can audit the routing without
                       reading our config code. -->
                  <span
                    class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-surface1 text-subtext"
                    title={st.source === 'feature' ? 'Per-feature override' : st.source === 'default' ? 'Prefs default_provider' : 'Global ai_provider from config.json'}
                  >via {st.provider} · {st.model}</span>
                {/if}
                {#if cfg.enabled}
                  <!-- Per-feature provider override. Empty value
                       means "use the prefs default / global config" —
                       resolveLLMConfig honors that fallback chain.
                       We list the providers granit's NewLLM actually
                       implements; Anthropic is in the config schema
                       but has no LLM driver yet, so omitted here. -->
                  <select
                    value={cfg.provider ?? ''}
                    onchange={(e) => {
                      const v = (e.target as HTMLSelectElement).value;
                      aiPrefs.features[f.id] = { ...cfg, provider: v || undefined };
                      saveAIPrefs();
                      setTimeout(() => { void loadAIStatus(); }, 600);
                    }}
                    class="text-[10px] bg-surface1 border border-surface1 rounded px-1 py-0.5 text-subtext hover:border-primary cursor-pointer"
                    onclick={(e) => e.stopPropagation()}
                    title="Override the provider for this feature only"
                  >
                    <option value="">(default)</option>
                    <option value="ollama">ollama</option>
                    <option value="openai">openai</option>
                  </select>
                {/if}
              </div>
              <div class="text-[11px] text-dim">{f.desc}</div>
            </div>
          </label>
        {/each}
      </div>

      <div class="mt-4 pt-3 border-t border-surface1 flex items-center gap-3">
        <label class="flex items-center gap-2 text-xs text-subtext flex-1">
          <input
            type="checkbox"
            checked={aiPrefs.redaction_enabled}
            onchange={(e) => { aiPrefs.redaction_enabled = (e.target as HTMLInputElement).checked; void saveAIPrefs(); }}
            class="w-4 h-4 accent-primary"
          />
          Redact PII (emails, phone, IBAN, cards, IPs) before prompts leave the device
        </label>
      </div>

      <div class="mt-4 pt-3 border-t border-surface1 flex items-center gap-2 flex-wrap">
        <button
          onclick={() => { aiAuditOpen = !aiAuditOpen; if (aiAuditOpen) void loadAIAudit(); }}
          class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary"
        >{aiAuditOpen ? 'Hide audit log' : 'View audit log'}</button>
        <button
          onclick={() => { aiSnapshotOpen = !aiSnapshotOpen; if (aiSnapshotOpen) void loadAISnapshot(); }}
          class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary"
        >{aiSnapshotOpen ? 'Hide snapshot' : 'What AI sees'}</button>
        <span class="flex-1"></span>
        <button
          onclick={() => void clearAIAudit()}
          class="px-3 py-1.5 text-xs text-dim hover:text-error"
          title="Permanently delete the AI audit log (GDPR right-to-erasure for the on-device portion)"
        >Clear AI history</button>
      </div>

      {#if aiAuditOpen}
        <div class="mt-3 pt-3 border-t border-surface1">
          {#if aiAudit.length === 0}
            <p class="text-xs text-dim italic">No AI requests recorded yet.</p>
          {:else}
            <!-- Usage rollup. Today first since "what did I just
                 burn" is the more common question; 7-day below for
                 weekly context. Errors split out separately because
                 they're free (no provider charges on a request that
                 failed before the chat call) but worth flagging. -->
            <div class="mb-3 grid grid-cols-2 gap-2 text-[11px]">
              <div class="px-2 py-1.5 bg-mantle border border-surface1 rounded">
                <div class="text-[10px] uppercase tracking-wider text-dim">Today</div>
                <div class="text-text">{aiUsage.todayN} request{aiUsage.todayN === 1 ? '' : 's'}</div>
                {#if aiUsage.todayPT + aiUsage.todayCT > 0}
                  <div class="text-dim font-mono" title="real token counts from the provider">{formatTokens(aiUsage.todayPT)} + {formatTokens(aiUsage.todayCT)} tokens</div>
                {/if}
                <div class="text-dim font-mono">{formatBytes(aiUsage.todayIn)} in / {formatBytes(aiUsage.todayOut)} out</div>
                {#if aiUsage.todayCost > 0}
                  <div class="text-secondary font-mono" title="OpenAI pricing — Ollama is free">{formatCost(aiUsage.todayCost)}</div>
                {/if}
                {#if aiUsage.todayErr > 0}
                  <div class="text-error">{aiUsage.todayErr} error{aiUsage.todayErr === 1 ? '' : 's'}</div>
                {/if}
              </div>
              <div class="px-2 py-1.5 bg-mantle border border-surface1 rounded">
                <div class="text-[10px] uppercase tracking-wider text-dim">Last 7 days</div>
                <div class="text-text">{aiUsage.weekN} request{aiUsage.weekN === 1 ? '' : 's'}</div>
                {#if aiUsage.weekPT + aiUsage.weekCT > 0}
                  <div class="text-dim font-mono" title="real token counts from the provider">{formatTokens(aiUsage.weekPT)} + {formatTokens(aiUsage.weekCT)} tokens</div>
                {/if}
                <div class="text-dim font-mono">{formatBytes(aiUsage.weekIn)} in / {formatBytes(aiUsage.weekOut)} out</div>
                {#if aiUsage.weekCost > 0}
                  <div class="text-secondary font-mono" title="OpenAI pricing — Ollama is free">{formatCost(aiUsage.weekCost)}</div>
                {/if}
                {#if aiUsage.weekErr > 0}
                  <div class="text-error">{aiUsage.weekErr} error{aiUsage.weekErr === 1 ? '' : 's'}</div>
                {/if}
              </div>
            </div>

            {#if aiUsageByFeature.length > 1}
              <!-- Per-feature breakdown over the same 7-day window
                   as the headline tile. Surfaces "which feature is
                   eating my budget" without forcing the user to
                   scroll the per-request audit list. Ordered by
                   cost desc, fall-through to count when costs tie
                   (Ollama entries have $0 across the board). -->
              <!-- overflow-x-auto so a long feature name plus its
                   four numeric columns can scroll horizontally on a
                   narrow phone instead of overflowing the page. -->
              <div class="mb-3 px-2 py-1.5 bg-mantle border border-surface1 rounded overflow-x-auto">
                <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Last 7 days · by feature</div>
                <table class="w-full text-[11px]">
                  <tbody>
                    {#each aiUsageByFeature as f}
                      <tr>
                        <td class="text-text py-0.5 pr-2 whitespace-nowrap">{f.feature}</td>
                        <td class="text-dim font-mono py-0.5 pr-2 tabular-nums text-right whitespace-nowrap">{f.count}×</td>
                        <td class="text-dim font-mono py-0.5 pr-2 tabular-nums text-right whitespace-nowrap">{formatTokens(f.tokens)} tok</td>
                        <td class="text-secondary font-mono py-0.5 tabular-nums text-right whitespace-nowrap">{f.cost > 0 ? formatCost(f.cost) : '—'}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {/if}
          {/if}
          {#if aiAudit.length > 0}
            <ul class="space-y-1 text-[11px] font-mono max-h-72 overflow-y-auto">
              {#each aiAudit as e (e.timestamp + e.feature)}
                <li class="px-2 py-1.5 bg-mantle border border-surface1 rounded">
                  <div class="flex items-baseline gap-2 flex-wrap">
                    <span class="text-dim">{new Date(e.timestamp).toLocaleString()}</span>
                    <span class="text-text font-semibold">{e.feature}</span>
                    {#if e.provider}<span class="text-secondary">via {e.provider}{e.model ? ` · ${e.model}` : ''}</span>{/if}
                    <span class="text-dim ml-auto">
                      {#if (e.prompt_tokens ?? 0) + (e.completion_tokens ?? 0) > 0}
                        {e.prompt_tokens ?? 0}+{e.completion_tokens ?? 0} tok
                      {:else}
                        {e.prompt_size_bytes}B / {e.response_size_bytes ?? 0}B
                      {/if}
                      {#if e.cost_micro_cents && e.cost_micro_cents > 0}
                        <span class="text-secondary ml-1">{formatCost(e.cost_micro_cents)}</span>
                      {/if}
                    </span>
                  </div>
                  {#if e.redactions && e.redactions.length > 0}
                    <div class="text-dim mt-0.5">
                      Redacted:
                      {#each e.redactions as r, i}
                        <span class="ml-1">{r.count}× {r.name}{i < (e.redactions?.length ?? 0) - 1 ? ',' : ''}</span>
                      {/each}
                    </div>
                  {/if}
                  {#if e.error}
                    <div class="text-error mt-0.5">{e.error}</div>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}

      {#if aiSnapshotOpen}
        <div class="mt-3 pt-3 border-t border-surface1">
          <p class="text-[11px] text-dim mb-2">
            This is the JSON shape AI features pass to your chosen provider. Note bodies and email contents are <strong>not</strong> included by default.
          </p>
          <pre class="max-h-72 overflow-auto bg-mantle border border-surface1 rounded p-2 text-[10px] font-mono text-subtext">{aiSnapshotJSON}</pre>
        </div>
      {/if}
    </section>

    <!-- Autocommit — debounced git-commit-on-save. Opt-in because
         not every vault is a git repo and surprising commits would
         be hostile. The status line tells the user whether the
         vault is actually a git repo so they don't toggle on
         expecting magic in a non-repo directory. -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <header class="flex items-baseline justify-between mb-2">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Git autocommit</h2>
        {#if autocommitSaving}
          <span class="text-[10px] uppercase tracking-wider text-dim">saving…</span>
        {/if}
      </header>
      <label class="flex items-center gap-3 cursor-pointer">
        <input
          type="checkbox"
          checked={autocommit.enabled}
          onchange={(e) => void toggleAutocommit((e.target as HTMLInputElement).checked)}
          disabled={autocommitSaving}
          class="w-4 h-4 accent-primary cursor-pointer"
        />
        <div class="flex-1 min-w-0">
          <div class="text-sm text-text">Auto-commit changes to git</div>
          <div class="text-[11px] text-dim mt-0.5">
            {#if autocommit.isGitRepo}
              Coalesced commit ~30s after the last save. Single tidy commit per work session.
            {:else}
              <span class="text-warning">Vault is not a git repository — toggle does nothing until you run <code class="text-[10px]">git init</code> in the vault.</span>
            {/if}
          </div>
        </div>
      </label>
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
    <section class="bg-surface0 border-2 border-surface2 rounded-lg p-4 mb-4">
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
                  <button onclick={clearOpenAIKey} class="px-3 py-2 text-xs text-error hover:bg-surface0 rounded border border-error">clear</button>
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
              <button onclick={revokeAllSessions} class="px-3 py-1.5 text-xs text-error hover:bg-surface0 rounded border border-error">
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
                    <span class="text-[10px] uppercase px-1.5 py-0.5 rounded bg-surface0 text-success">this device</span>
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
                  class="px-2.5 py-1 text-xs text-error hover:bg-surface0 rounded border border-error disabled:opacity-50 flex-shrink-0"
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
      <div class="flex items-baseline justify-between mb-3">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Git auto-sync</h2>
        {#if sync?.enabled}
          <button
            type="button"
            onclick={syncNow}
            disabled={syncBusy}
            class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50"
            title="trigger a one-off pull + commit + push"
          >{syncBusy ? '…' : '↻ Sync now'}</button>
        {/if}
      </div>
      {#if !sync}
        <Skeleton class="h-4 w-1/2" />
      {:else if !sync.enabled}
        <div class="text-sm text-dim leading-relaxed space-y-2">
          <p>
            Auto-sync is off. To enable periodic <code class="text-xs">git pull</code> + auto-commit/push,
            edit your <code class="text-xs">docker-compose.yml</code> on the server and add
            <code class="text-xs">--sync --sync-interval 60s</code> to the granit
            <code class="text-xs">command:</code> line, then
            <code class="text-xs">docker compose up -d</code>.
          </p>
          <p>
            To commit + push the vault manually right now (no daemon needed),
            SSH into your server and run inside the vault directory:
          </p>
          <pre class="text-xs font-mono px-3 py-2 bg-mantle border border-surface1 rounded overflow-x-auto"><code>cd /srv/granit-vault
git add -A
git commit -m "manual sync $(date +%F)"
git push</code></pre>
        </div>
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
