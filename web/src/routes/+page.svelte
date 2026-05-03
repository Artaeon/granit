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
    { id: 'w-today-focus', type: 'today-focus', afterId: 'w-greeting', enabled: true },
    { id: 'w-top-deadlines', type: 'top-deadlines', afterId: 'w-now', enabled: true }
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

  let activeWidgets = $derived.by(() => {
    if (!config) return [];
    return config.widgets
      .filter((w) => w.enabled)
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
          <button type="submit" disabled={signingIn || !password} class="w-full px-3 py-3 bg-primary text-mantle rounded text-sm font-medium hover:opacity-90 disabled:opacity-50">
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
          <button type="submit" disabled={signingIn || !password} class="w-full px-3 py-3 bg-primary text-mantle rounded text-sm font-medium hover:opacity-90 disabled:opacity-50">
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
    <div class="p-4 sm:p-6 lg:p-8 max-w-6xl mx-auto">
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

      <div class="flex items-center justify-end mb-4">
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
                    class="w-4 h-4 rounded border flex items-center justify-center flex-shrink-0
                      {w.enabled ? 'bg-success border-success' : 'border-surface2'}"
                  >
                    {#if w.enabled}
                      <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                    {/if}
                  </button>
                  <div class="flex-1 min-w-0">
                    <div class="text-sm text-text">{meta.label}</div>
                    <div class="text-xs text-dim truncate">{meta.description}</div>
                  </div>
                  <button onclick={() => moveUp(w.id)} disabled={i === 0} class="w-7 h-7 text-dim hover:text-text disabled:opacity-30">↑</button>
                  <button onclick={() => moveDown(w.id)} disabled={i === config.widgets.length - 1} class="w-7 h-7 text-dim hover:text-text disabled:opacity-30">↓</button>
                </li>
              {/if}
            {/each}
          </ul>
          <p class="text-xs text-dim pt-2 border-t border-surface1">
            drag rows to reorder · saved to <code class="text-[10px]">.granit/everything-dashboard.json</code> · syncs across devices
          </p>
        </section>
      {/if}

      {#if config}
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
          {#each activeWidgets as { widget, meta } (widget.id)}
            {@const Widget = meta.component}
            <div class={meta.span === 2 ? 'lg:col-span-2' : ''}>
              <Widget vaultPath={vault?.root ?? ''} />
            </div>
          {/each}
        </div>
      {:else}
        <div class="text-sm text-dim">loading dashboard…</div>
      {/if}
    </div>
  </div>
{/if}
