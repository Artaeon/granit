<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, ApiError, type DashboardConfig, type DashboardWidget, type VaultInfo } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { widgetRegistry, widgetMeta } from '$lib/dashboard/registry';

  // New widget types we ship in this build that the server's defaults
  // (internal/serveapi/handlers_dashboard.go) doesn't know about yet. We
  // inject them into the user's saved config locally so they appear in
  // customize-mode and can render. Order matters — these are the slots
  // we want the new widgets to occupy by default.
  const NEW_WIDGETS: { id: string; type: import('$lib/api').DashboardWidgetType; afterId: string; enabled: boolean }[] = [
    // Vision lands at the very top — anchor for the morning re-read.
    { id: 'w-vision', type: 'vision', afterId: 'w-greeting', enabled: true },
    // One-thing shows the commitment from the latest weekly review,
    // sitting between vision and today's focus so the week's
    // intention bridges the season's vision and the day's tactics.
    { id: 'w-one-thing', type: 'one-thing', afterId: 'w-vision', enabled: true },
    { id: 'w-today-focus', type: 'today-focus', afterId: 'w-one-thing', enabled: true },
    { id: 'w-top-deadlines', type: 'top-deadlines', afterId: 'w-now', enabled: true },
    // Pairs naturally with top-deadlines so the two by-when widgets
    // sit side-by-side on the wide dashboard grid.
    { id: 'w-top-goals', type: 'top-goals', afterId: 'w-top-deadlines', enabled: true },
    // Quick links — hub favorites surfaced next to the by-when
    // widgets so the morning view answers both "when" and "where"
    // at a glance.
    { id: 'w-quick-links', type: 'quick-links', afterId: 'w-top-goals', enabled: true }
  ];

  // Auth state machine on the landing page:
  //   loading      → checking /auth/status
  //   setup        → no password yet → set one
  //   login        → password is set → enter it
  //   advanced     → user wants to paste a CLI bearer token instead
  type AuthScreen = 'loading' | 'setup' | 'login' | 'advanced';
  let authScreen = $state<AuthScreen>('loading');
  let setupAt = $state<string | null>(null);
  let password = $state('');
  let passwordConfirm = $state('');
  let tokenInput = $state('');
  let signingIn = $state(false);
  let signInError = $state('');

  let vault = $state<VaultInfo | null>(null);
  let config = $state<DashboardConfig | null>(null);
  let editing = $state(false);
  let loadError = $state('');

  // First paint: if we already have a token, try it. If it works, render
  // dashboard. If not, fetch /auth/status and show setup or login.
  onMount(async () => {
    if ($auth) {
      try {
        await api.vault();
        return; // valid — load() runs via the $effect below
      } catch {
        auth.clear();
      }
    }
    await refreshAuthScreen();
  });

  async function refreshAuthScreen() {
    try {
      const r = await api.authStatus();
      authScreen = r.hasPassword ? 'login' : 'setup';
      setupAt = r.setupAt ?? null;
    } catch {
      // Server unreachable — fall back to advanced (token paste) so the
      // user can at least diagnose. Better than a dead form.
      authScreen = 'advanced';
    }
  }

  $effect(() => {
    if ($auth) load();
    else {
      vault = null;
      config = null;
    }
  });

  async function load() {
    loadError = '';
    try {
      const [v, c] = await Promise.all([api.vault(), api.getDashboard()]);
      vault = v;
      config = injectNewWidgets(c);
      // If we added widgets the server didn't know about, persist so the
      // toggle states travel across devices on next load.
      if (config.widgets.length !== c.widgets.length) {
        await api.putDashboard(config).catch(() => {});
      }
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        auth.clear();
        await refreshAuthScreen();
      } else loadError = e instanceof Error ? e.message : String(e);
    }
  }

  // Splice in any NEW_WIDGETS the saved config doesn't have, anchored
  // after the slot we want them to follow. Idempotent — re-running on a
  // config that already has the new widget is a no-op.
  function injectNewWidgets(c: DashboardConfig): DashboardConfig {
    const have = new Set(c.widgets.map((w) => w.id));
    let widgets = [...c.widgets];
    for (const nw of NEW_WIDGETS) {
      if (have.has(nw.id)) continue;
      const anchor = widgets.findIndex((w) => w.id === nw.afterId);
      const entry = { id: nw.id, type: nw.type, enabled: nw.enabled };
      if (anchor === -1) widgets.push(entry);
      else widgets = [...widgets.slice(0, anchor + 1), entry, ...widgets.slice(anchor + 1)];
    }
    return { ...c, widgets };
  }

  function deviceLabel(): string {
    if (typeof navigator === 'undefined') return '';
    // Compact UA hint — saved alongside the session so the user can
    // identify it later when revoking. No fingerprinting beyond UA.
    const ua = navigator.userAgent;
    if (/iPhone|iPad/.test(ua)) return 'iOS';
    if (/Android/.test(ua)) return 'Android';
    if (/Mac OS X/.test(ua)) return 'macOS';
    if (/Linux/.test(ua)) return 'Linux';
    if (/Windows/.test(ua)) return 'Windows';
    return 'Web';
  }

  async function setupPassword(e: Event) {
    e.preventDefault();
    if (password.length < 6) { signInError = 'password must be at least 6 characters'; return; }
    if (password !== passwordConfirm) { signInError = 'passwords do not match'; return; }
    signingIn = true;
    signInError = '';
    try {
      const r = await api.authSetup(password, deviceLabel());
      auth.setToken(r.token);
      password = ''; passwordConfirm = '';
    } catch (e) {
      signInError = e instanceof Error ? e.message : String(e);
    } finally {
      signingIn = false;
    }
  }

  async function login(e: Event) {
    e.preventDefault();
    if (!password) return;
    signingIn = true;
    signInError = '';
    try {
      const r = await api.authLogin(password, deviceLabel());
      auth.setToken(r.token);
      password = '';
    } catch (e) {
      signInError = e instanceof Error ? e.message : 'invalid password';
    } finally {
      signingIn = false;
    }
  }

  async function signInWithToken(e: Event) {
    e.preventDefault();
    if (!tokenInput.trim()) return;
    signingIn = true;
    signInError = '';
    auth.setToken(tokenInput.trim());
    try {
      await api.vault();
      tokenInput = '';
    } catch {
      auth.clear();
      signInError = 'invalid token';
    } finally {
      signingIn = false;
    }
  }

  async function persist() {
    if (!config) return;
    try {
      const saved = await api.putDashboard(config);
      config = saved;
    } catch (e) {
      console.error(e);
    }
  }

  // ----- Layout presets -----
  //
  // Save / activate / delete each return the full updated config so we
  // swap state in one round trip rather than re-fetching. Failures
  // surface via toast — we don't try to roll back optimistic changes
  // because the server's response IS the new state of truth.

  let savingLayout = $state(false);
  async function saveCurrentLayout() {
    if (!config) return;
    const name = prompt('Save current arrangement as preset:', config.active || '');
    if (!name) return;
    const trimmed = name.trim();
    if (!trimmed) return;
    savingLayout = true;
    try {
      const saved = await api.saveDashboardLayout(trimmed);
      config = saved;
    } catch (e) {
      console.error('saveLayout', e);
    } finally {
      savingLayout = false;
    }
  }

  async function activateLayout(name: string) {
    if (!config || config.active === name) return;
    try {
      const saved = await api.activateDashboardLayout(name);
      config = saved;
    } catch (e) {
      console.error('activateLayout', e);
    }
  }

  async function deleteLayout(name: string) {
    if (!config) return;
    if (!confirm(`Delete the "${name}" preset? The widgets stay where they are.`)) return;
    try {
      const saved = await api.deleteDashboardLayout(name);
      config = saved;
    } catch (e) {
      console.error('deleteLayout', e);
    }
  }

  function toggleWidget(id: string) {
    if (!config) return;
    config = {
      ...config,
      widgets: config.widgets.map((w) => (w.id === id ? { ...w, enabled: !w.enabled } : w))
    };
    persist();
  }

  function moveUp(id: string) {
    if (!config) return;
    const i = config.widgets.findIndex((w) => w.id === id);
    if (i <= 0) return;
    const ws = [...config.widgets];
    [ws[i - 1], ws[i]] = [ws[i], ws[i - 1]];
    config = { ...config, widgets: ws };
    persist();
  }
  function moveDown(id: string) {
    if (!config) return;
    const i = config.widgets.findIndex((w) => w.id === id);
    if (i < 0 || i >= config.widgets.length - 1) return;
    const ws = [...config.widgets];
    [ws[i + 1], ws[i]] = [ws[i], ws[i + 1]];
    config = { ...config, widgets: ws };
    persist();
  }

  // ----- Drag-and-drop reorder (customize-mode only) -----
  //
  // Uses native HTML5 DnD instead of pointer events so we don't conflict
  // with the calendar's plan-mode pointer drag (which lives on a totally
  // different surface). The moveUp/moveDown buttons stay as a fallback
  // for keyboard / touch users who can't easily hold-drag.
  let dragId = $state<string | null>(null);
  let dragOverId = $state<string | null>(null);

  function onDragStart(id: string, ev: DragEvent) {
    if (!editing) return;
    dragId = id;
    if (ev.dataTransfer) {
      ev.dataTransfer.effectAllowed = 'move';
      // Required on Firefox — without setData() the drag never starts.
      try { ev.dataTransfer.setData('text/plain', id); } catch {}
    }
  }

  function onDragOver(id: string, ev: DragEvent) {
    if (!editing || !dragId || dragId === id) return;
    ev.preventDefault();
    if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'move';
    dragOverId = id;
  }

  function onDragLeave(id: string) {
    if (dragOverId === id) dragOverId = null;
  }

  function onDrop(targetId: string, ev: DragEvent) {
    if (!editing || !config || !dragId || dragId === targetId) {
      dragId = null;
      dragOverId = null;
      return;
    }
    ev.preventDefault();
    const ws = [...config.widgets];
    const fromIdx = ws.findIndex((w) => w.id === dragId);
    const toIdx = ws.findIndex((w) => w.id === targetId);
    if (fromIdx < 0 || toIdx < 0) {
      dragId = null;
      dragOverId = null;
      return;
    }
    const [moved] = ws.splice(fromIdx, 1);
    ws.splice(toIdx, 0, moved);
    config = { ...config, widgets: ws };
    dragId = null;
    dragOverId = null;
    persist();
  }

  function onDragEnd() {
    dragId = null;
    dragOverId = null;
  }

  // Focus mode — temporarily hides everything except the essentials
  // for a quiet "what matters today" view. Not a saved preset; just
  // a render-time filter so the user's preset/layout choices are
  // untouched. Toggle is at the top of the page.
  //
  // The essentials list is curated, not user-configurable: greeting
  // (date anchor), at-a-glance (today's counts), today-focus (the
  // morning commitment), today-tasks (the working list), calendar-
  // week (what's coming), top-deadlines (the by-when pressure).
  // Six tiles, one screen, no scrolling on a typical desktop.
  const FOCUS_ESSENTIALS = new Set<import('$lib/api').DashboardWidgetType>([
    'greeting',
    'at-a-glance',
    'today-focus',
    'today-tasks',
    'calendar-week',
    'top-deadlines'
  ]);
  const FOCUS_KEY = 'granit.dashboard.focus';
  let focus = $state<boolean>(
    typeof localStorage !== 'undefined' && localStorage.getItem(FOCUS_KEY) === '1'
  );
  function toggleFocus() {
    focus = !focus;
    try { localStorage.setItem(FOCUS_KEY, focus ? '1' : '0'); } catch {}
  }

  let activeWidgets = $derived.by(() => {
    if (!config) return [];
    return config.widgets
      .filter((w) => w.enabled && (!focus || FOCUS_ESSENTIALS.has(w.type)))
      .map((w) => ({ widget: w, meta: widgetMeta(w.type) }))
      .filter((x): x is { widget: DashboardWidget; meta: NonNullable<ReturnType<typeof widgetMeta>> } => !!x.meta);
  });

  // AI setup hint. Shown until the user has either configured a cloud
  // provider key OR explicitly dismissed it. Detects the common
  // first-launch state where the user has all the AI features in the
  // UI (Plan my day / Reflect / Chat / Agents) but no provider that
  // can actually run them — and points at /settings instead of letting
  // those features error out cryptically.
  let appCfg = $state<import('$lib/api').AppConfig | null>(null);
  let aiHintDismissed = $state(false);
  if (typeof localStorage !== 'undefined') {
    aiHintDismissed = localStorage.getItem('granit.ai.hint.dismissed') === '1';
  }
  $effect(() => {
    if ($auth && !appCfg) {
      api.getConfig().then((c) => (appCfg = c)).catch(() => {});
    }
  });
  let aiNotConfigured = $derived.by(() => {
    if (!appCfg || aiHintDismissed) return false;
    const p = appCfg.ai_provider || 'local';
    if (p === 'openai') return !appCfg.openai_key_set;
    if (p === 'anthropic') return !appCfg.anthropic_key_set;
    // Ollama / local: we can't tell from config whether the daemon is
    // reachable, but the default model (qwen2.5:0.5b) is rarely pulled.
    // Show the hint when no cloud provider is set at all — covers the
    // most common "AI features just don't work" state.
    return !appCfg.openai_key_set && !appCfg.anthropic_key_set;
  });
  function dismissAiHint() {
    aiHintDismissed = true;
    try { localStorage.setItem('granit.ai.hint.dismissed', '1'); } catch {}
  }
</script>

{#if !$auth}
  <div class="h-full overflow-y-auto flex items-center justify-center p-4 sm:p-8">
    <div class="w-full max-w-sm bg-mantle border border-surface1 rounded-lg p-5 sm:p-6 space-y-4">
      <div>
        <h1 class="text-lg font-semibold text-text">everything</h1>
        {#if authScreen === 'setup'}
          <p class="text-sm text-dim mt-1">First launch — set a password to secure your vault.</p>
        {:else if authScreen === 'login'}
          <p class="text-sm text-dim mt-1">Sign in with your password</p>
          {#if setupAt}<p class="text-[11px] text-dim/70 mt-0.5">Account created {new Date(setupAt).toLocaleDateString()}</p>{/if}
        {:else if authScreen === 'advanced'}
          <p class="text-sm text-dim mt-1">Paste your bearer token</p>
        {:else}
          <p class="text-sm text-dim mt-1">Checking…</p>
        {/if}
      </div>

      {#if authScreen === 'setup'}
        <form onsubmit={setupPassword} class="space-y-3">
          <div>
            <label for="pw" class="block text-xs uppercase tracking-wider text-dim mb-1">New password</label>
            <input
              id="pw"
              type="password"
              autocomplete="new-password"
              bind:value={password}
              placeholder="at least 6 characters"
              required
              class="w-full px-3 py-3 bg-surface0 border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary"
            />
          </div>
          <div>
            <label for="pwc" class="block text-xs uppercase tracking-wider text-dim mb-1">Confirm password</label>
            <input
              id="pwc"
              type="password"
              autocomplete="new-password"
              bind:value={passwordConfirm}
              required
              class="w-full px-3 py-3 bg-surface0 border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary"
            />
          </div>
          {#if signInError}<div class="text-sm text-error">{signInError}</div>{/if}
          <button type="submit" disabled={signingIn || !password} class="w-full px-3 py-3 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90 disabled:opacity-50">
            {signingIn ? 'creating…' : 'Set password & sign in'}
          </button>
          <p class="text-[11px] text-dim text-center">
            Stored as an argon2id hash in <code>.granit/web-auth.json</code>.
            Tip: use a passphrase you can remember — there's no recovery flow.
          </p>
        </form>
      {:else if authScreen === 'login'}
        <form onsubmit={login} class="space-y-3">
          <input
            type="password"
            autocomplete="current-password"
            bind:value={password}
            placeholder="password"
            required
            autofocus
            class="w-full px-3 py-3 bg-surface0 border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary"
          />
          {#if signInError}<div class="text-sm text-error">{signInError}</div>{/if}
          <button type="submit" disabled={signingIn || !password} class="w-full px-3 py-3 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90 disabled:opacity-50">
            {signingIn ? 'signing in…' : 'Sign in'}
          </button>
          <button type="button" onclick={() => { authScreen = 'advanced'; signInError = ''; }} class="w-full text-xs text-dim hover:text-text">
            use bearer token instead
          </button>
        </form>
      {:else if authScreen === 'advanced'}
        <form onsubmit={signInWithToken} class="space-y-3">
          <input
            type="password"
            bind:value={tokenInput}
            placeholder="bearer token (CLI)"
            class="w-full px-3 py-3 bg-surface0 border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary font-mono"
          />
          {#if signInError}<div class="text-sm text-error">{signInError}</div>{/if}
          <button type="submit" disabled={signingIn || !tokenInput.trim()} class="w-full px-3 py-3 bg-surface0 border border-surface1 text-text rounded text-sm font-medium hover:border-primary disabled:opacity-50">
            {signingIn ? 'signing in…' : 'Sign in with token'}
          </button>
          <button type="button" onclick={refreshAuthScreen} class="w-full text-xs text-dim hover:text-text">
            ← back to password
          </button>
          <p class="text-[11px] text-dim break-words">
            Token is stored in <code>.granit/everything-token</code> on the server. Use this for CLI scripts only.
          </p>
        </form>
      {:else}
        <div class="text-sm text-dim text-center py-4">…</div>
      {/if}
    </div>
  </div>
{:else}
  <div class="h-full overflow-y-auto">
    <div class="p-4 sm:p-6 lg:p-8 max-w-7xl mx-auto">
      {#if loadError}<div class="text-sm text-error mb-4">{loadError}</div>{/if}

      {#if aiNotConfigured}
        <div class="mb-4 p-4 bg-warning/10 border border-warning/30 rounded-lg flex items-start gap-3">
          <div class="w-8 h-8 rounded-full bg-warning/20 flex items-center justify-center flex-shrink-0">
            <svg viewBox="0 0 24 24" class="w-4 h-4 text-warning" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 9v4M12 17h.01" stroke-linecap="round"/>
              <circle cx="12" cy="12" r="9"/>
            </svg>
          </div>
          <div class="flex-1 min-w-0">
            <p class="text-sm text-text font-medium">AI provider not configured</p>
            <p class="text-xs text-dim mt-0.5">
              Plan my day, Chat, Reflect, deep research, morning AI suggestion — none of these will work until you set an API key.
            </p>
            <div class="flex items-center gap-3 mt-2">
              <a href="/settings" class="px-3 py-1.5 text-xs bg-warning text-mantle rounded font-medium">Open Settings</a>
              <button onclick={dismissAiHint} class="text-xs text-dim hover:text-text">dismiss</button>
            </div>
          </div>
        </div>
      {/if}

      <div class="flex items-center justify-end gap-2 mb-4">
        <!-- Focus toggle — render-time filter that strips the dashboard
             to its 6 essentials so a noisy widget list doesn't
             overwhelm the user when they just want today's view.
             Doesn't touch saved presets or widget config. -->
        <button
          type="button"
          onclick={toggleFocus}
          aria-pressed={focus}
          class="text-xs px-3 py-1.5 rounded inline-flex items-center gap-1.5 transition-colors
            {focus ? 'bg-primary/15 text-primary border border-primary/40' : 'bg-surface0 border border-surface1 text-subtext hover:border-primary/40'}"
          title={focus ? 'Show all enabled widgets' : 'Hide everything except today essentials'}
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="9"/>
            <circle cx="12" cy="12" r="3" fill="currentColor"/>
          </svg>
          {focus ? 'Focus on' : 'Focus'}
        </button>
        <!-- Active preset chip + quick switcher. Shown only when the
             user has at least one saved layout, so the row stays tidy
             until presets become useful. -->
        {#if config && (config.layouts?.length ?? 0) > 0}
          <select
            value={config.active ?? ''}
            onchange={(e) => {
              const next = (e.target as HTMLSelectElement).value;
              if (next) activateLayout(next);
            }}
            class="text-xs px-2 py-1.5 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary"
            aria-label="active dashboard layout"
            title="switch dashboard layout"
          >
            {#if !config.active}
              <option value="">— ad-hoc —</option>
            {/if}
            {#each config.layouts ?? [] as l (l.name)}
              <option value={l.name}>{l.name}</option>
            {/each}
          </select>
        {/if}
        <button
          onclick={() => (editing = !editing)}
          class="text-xs px-3 py-1.5 bg-surface0 border border-surface1 rounded {editing ? 'text-primary border-primary' : 'text-subtext hover:border-primary'}"
        >
          {editing ? 'done editing' : '⚙ customize'}
        </button>
      </div>

      {#if editing && config}
        <section class="mb-6 bg-mantle/50 border border-surface1 rounded-lg p-4 space-y-3">
          <h2 class="text-sm font-medium text-text">Widgets</h2>
          <ul class="space-y-1.5">
            {#each config.widgets as w, i (w.id)}
              {@const meta = widgetMeta(w.type)}
              {#if meta}
                <li
                  draggable="true"
                  ondragstart={(ev) => onDragStart(w.id, ev)}
                  ondragover={(ev) => onDragOver(w.id, ev)}
                  ondragleave={() => onDragLeave(w.id)}
                  ondrop={(ev) => onDrop(w.id, ev)}
                  ondragend={onDragEnd}
                  class="flex items-center gap-2 py-1.5 px-2 rounded transition-colors cursor-grab active:cursor-grabbing
                    {dragId === w.id ? 'opacity-40' : ''}
                    {dragOverId === w.id && dragId !== w.id ? 'bg-primary/10 border border-primary/40' : 'border border-transparent'}"
                >
                  <span aria-hidden="true" class="text-dim/60 select-none flex-shrink-0" title="drag to reorder">⋮⋮</span>
                  <button
                    onclick={() => toggleWidget(w.id)}
                    aria-label="toggle"
                    class="w-9 h-9 sm:w-6 sm:h-6 rounded flex items-center justify-center flex-shrink-0 hover:bg-surface0"
                  >
                    <span
                      class="w-4 h-4 rounded border flex items-center justify-center
                        {w.enabled ? 'bg-success border-success' : 'border-surface2'}"
                    >
                      {#if w.enabled}
                        <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                      {/if}
                    </span>
                  </button>
                  <div class="flex-1 min-w-0">
                    <div class="text-sm text-text">{meta.label}</div>
                    <div class="text-xs text-dim truncate">{meta.description}</div>
                  </div>
                  <!-- Reorder buttons grow to a 44x44 hit-area on touch
                       devices since drag-to-reorder isn't reachable from
                       a phone — these chevrons are the actual touch UI. -->
                  <button onclick={() => moveUp(w.id)} disabled={i === 0} aria-label="move up" class="w-11 h-11 sm:w-7 sm:h-7 text-dim hover:text-text disabled:opacity-30 rounded">↑</button>
                  <button onclick={() => moveDown(w.id)} disabled={i === config.widgets.length - 1} aria-label="move down" class="w-11 h-11 sm:w-7 sm:h-7 text-dim hover:text-text disabled:opacity-30 rounded">↓</button>
                </li>
              {/if}
            {/each}
          </ul>
          <p class="text-xs text-dim pt-2 border-t border-surface1">
            drag rows to reorder · saved to <code class="text-[10px]">.granit/everything-dashboard.json</code> · syncs across devices
          </p>
        </section>

        <!-- Layout presets — switch between named arrangements like
             focus / morning / shutdown. Each preset captures the
             complete widget list (order + enabled state); switching
             swaps them in. Save snapshots whatever's currently
             arranged. -->
        <section class="mb-6 bg-mantle/50 border border-surface1 rounded-lg p-4 space-y-3">
          <div class="flex items-baseline justify-between">
            <h2 class="text-sm font-medium text-text">Layout presets</h2>
            <button
              onclick={saveCurrentLayout}
              disabled={savingLayout}
              class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50"
            >
              + save current as preset
            </button>
          </div>
          {#if (config?.layouts?.length ?? 0) === 0}
            <p class="text-xs text-dim italic">
              No presets yet. Arrange your widgets above, then save as
              <em>focus</em> / <em>morning</em> / <em>shutdown</em> — switch from the dropdown next to "customize".
            </p>
          {:else}
            <ul class="space-y-1">
              {#each config?.layouts ?? [] as l (l.name)}
                {@const active = config?.active === l.name}
                <li class="flex items-center gap-2 px-2.5 py-1.5 rounded {active ? 'bg-primary/10 border border-primary/40' : 'border border-transparent hover:bg-surface0'}">
                  <span class="text-sm flex-1 truncate {active ? 'text-primary font-medium' : 'text-text'}">
                    {l.name}
                  </span>
                  <span class="text-[11px] text-dim">{l.widgets.filter((w) => w.enabled).length} enabled</span>
                  {#if !active}
                    <button
                      onclick={() => activateLayout(l.name)}
                      class="text-xs px-2 py-0.5 text-secondary hover:underline"
                    >activate</button>
                  {:else}
                    <span class="text-[10px] uppercase tracking-wider text-primary">active</span>
                  {/if}
                  <button
                    onclick={() => deleteLayout(l.name)}
                    aria-label="delete {l.name}"
                    title="delete preset"
                    class="text-xs text-dim hover:text-error w-6 h-6 flex items-center justify-center rounded"
                  >×</button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/if}

      {#if focus && config && activeWidgets.length === 0}
        <!-- Focus on but the user has none of the essentials enabled.
             Tell them rather than render an empty page. -->
        <div class="mb-4 p-4 bg-mantle/60 border border-surface1 rounded-lg text-sm">
          <div class="text-text font-medium mb-1">Focus mode is on, but no essential widgets are enabled.</div>
          <p class="text-xs text-dim mb-3">
            Focus shows: greeting, at-a-glance, today's focus, today's tasks, calendar week, top deadlines.
            Enable any of these in customize, or turn focus off.
          </p>
          <div class="flex items-center gap-2">
            <button onclick={toggleFocus} class="px-3 py-1.5 text-xs rounded bg-primary text-on-primary font-medium">Turn off focus</button>
            <button onclick={() => (editing = true)} class="px-3 py-1.5 text-xs rounded bg-surface0 border border-surface1 text-subtext hover:border-primary">Customize widgets</button>
          </div>
        </div>
      {/if}
      {#if config}
        <!-- Three-column grid above 1280px: span-2 widgets become
             full-width strips, span-1 widgets pack 3 per row so wide
             displays don't leave half-empty rows. items-start keeps
             each widget at its natural content height — without it,
             a short widget paired with a tall one stretches and the
             card looks half-empty inside. -->
        <div class="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-3 sm:gap-4 items-start">
          {#each activeWidgets as { widget, meta } (widget.id)}
            <!-- Each widget chunk is loaded lazily via meta.load();
                 the registry's loader is memoised so re-renders await
                 the same resolved promise instead of refetching.
                 The skeleton placeholder reserves a small height so
                 the grid doesn't reflow once each chunk lands. -->
            <div class={meta.span === 2 ? 'lg:col-span-2 xl:col-span-3' : ''}>
              {#await meta.load()}
                <div class="bg-surface0 border border-surface1 rounded-lg p-4 animate-pulse h-24"></div>
              {:then Widget}
                <Widget vaultPath={vault?.root ?? ''} />
              {:catch err}
                <div class="bg-error/10 border border-error/30 text-error rounded-lg p-3 text-xs">
                  Widget {meta.label} failed to load: {err?.message ?? err}
                </div>
              {/await}
            </div>
          {/each}
        </div>
      {:else}
        <div class="text-sm text-dim">loading dashboard…</div>
      {/if}
    </div>
  </div>
{/if}
