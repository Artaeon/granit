<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, ApiError, type DashboardConfig, type DashboardWidget, type VaultInfo } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { widgetRegistry, widgetMeta } from '$lib/dashboard/registry';
  import AuthScreen from '$lib/components/AuthScreen.svelte';

  // New widget types we ship in this build that the server's defaults
  // (internal/serveapi/handlers_dashboard.go) doesn't know about yet. We
  // inject them into the user's saved config locally so they appear in
  // customize-mode and can render. Order matters — these are the slots
  // we want the new widgets to occupy by default.
  // Widgets we inject into a user's saved config because the server
  // default didn't ship them. Most go in DISABLED so the dashboard
  // stays focused — the user opts in via Customize. Only widgets
  // that materially change "what is today?" land enabled.
  //
  // Tightened during the "professional cleanup" pass: the previous
  // default lit up 21 widgets on first launch (server's 15 + 6 new),
  // which read as "a wall of tiles" rather than a workspace. Tier-1
  // here: today-stream + today-focus, both of which directly answer
  // the user's first morning question. Everything else is opt-in.
  const NEW_WIDGETS: { id: string; type: import('$lib/api').DashboardWidgetType; afterId: string; enabled: boolean }[] = [
    // Today stream sits at the very top after greeting — the
    // headline "what's happening now + what's next" panel. Merges
    // today's events, scheduled tasks, due tasks, and deadlines
    // into one chronological feed plus a tomorrow + day-after
    // preview. Single source of truth for "shape of today" so the
    // user doesn't have to triangulate four separate today-* tiles.
    { id: 'w-today-stream', type: 'today-stream', afterId: 'w-greeting', enabled: true },
    // Today focus — the AI-suggested #1 thing for the day. Anchors
    // intention right under the stream.
    { id: 'w-today-focus', type: 'today-focus', afterId: 'w-today-stream', enabled: true },
    // Vision anchors the morning re-read but isn't critical for
    // tactical day-of work — opt-in.
    { id: 'w-vision', type: 'vision', afterId: 'w-today-focus', enabled: false },
    // One-thing: weekly-plan commitment. Opt-in for users who do
    // a Sunday planning ritual.
    { id: 'w-one-thing', type: 'one-thing', afterId: 'w-vision', enabled: false },
    // top-deadlines: deadline pressure tile. Opt-in — at-a-glance
    // already shows a 7-day deadline count and that's enough for
    // most days.
    { id: 'w-top-deadlines', type: 'top-deadlines', afterId: 'w-now', enabled: false },
    // Quick links — hub favorites. Opt-in.
    { id: 'w-quick-links', type: 'quick-links', afterId: 'w-top-deadlines', enabled: false },
    // Verse-for-mood — secondary scripture surface. Opt-in.
    { id: 'w-verse-for-mood', type: 'verse-for-mood', afterId: 'w-scripture', enabled: false }
  ];

  // The auth surface (setup / login / token paste) lives in the
  // AuthScreen component so this file stays focused on the dashboard
  // grid. When !$auth, +page just renders <AuthScreen />; on success
  // AuthScreen writes the token to the auth store and the dashboard
  // branch below takes over.

  let vault = $state<VaultInfo | null>(null);
  let config = $state<DashboardConfig | null>(null);
  let editing = $state(false);
  let loadError = $state('');

  // First paint: if we already have a token, verify it. If it works,
  // the load() effect below pulls vault + config. If not, auth.clear()
  // unsets $auth and the AuthScreen branch renders.
  onMount(async () => {
    if ($auth) {
      try {
        await api.vault();
      } catch {
        auth.clear();
      }
    }
  });

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
        // 401 in the load path means our token went bad mid-session.
        // Clearing it bounces us to <AuthScreen />, which runs its own
        // /auth/status fetch on mount — no need to duplicate here.
        auth.clear();
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
  // Curated set: greeting (date anchor), at-a-glance (today's counts),
  // today-stream (the chronological feed — covers events + scheduled
  // + due + deadlines in one), today-focus (the morning commitment),
  // today-tasks (action surface for due/overdue rows), top-deadlines
  // (by-when pressure). Six tiles, no scrolling on a typical desktop.
  // calendar-week was dropped because today-stream's tomorrow/day-
  // after preview covers the same ground for the focus-mode use.
  const FOCUS_ESSENTIALS = new Set<import('$lib/api').DashboardWidgetType>([
    'greeting',
    'at-a-glance',
    'today-stream',
    'today-focus',
    'today-tasks',
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
  <AuthScreen />
{:else}
  <div class="h-full overflow-y-auto">
    <!-- Tighter padding + max-width: power-UI density beats breathing
         room when the user wants everything on one screen. -->
    <div class="p-3 sm:p-4 lg:p-6 max-w-7xl mx-auto">
      {#if loadError}<div class="text-sm text-error mb-4">{loadError}</div>{/if}

      {#if aiNotConfigured}
        <div class="mb-4 p-4 bg-surface0 border border-warning rounded-lg flex items-start gap-3">
          <div class="w-8 h-8 rounded-full bg-surface0 flex items-center justify-center flex-shrink-0">
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
            {focus ? 'bg-surface1 text-primary border border-surface2' : 'bg-surface0 border border-surface1 text-subtext hover:border-primary'}"
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
          class="text-xs px-3 py-1.5 bg-surface0 border border-surface1 rounded inline-flex items-center gap-1.5 {editing ? 'text-primary border-primary' : 'text-subtext hover:border-primary'}"
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="3"/>
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33h0a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82v0a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
          </svg>
          {editing ? 'Done' : 'Customize'}
        </button>
      </div>

      {#if editing && config}
        <section class="mb-4 bg-mantle border border-surface1 rounded-lg p-4 space-y-3">
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
                    {dragOverId === w.id && dragId !== w.id ? 'bg-surface1 border border-surface2' : 'border border-transparent'}"
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
                  <button onclick={() => moveUp(w.id)} disabled={i === 0} aria-label="move up" class="w-11 h-11 sm:w-7 sm:h-7 inline-flex items-center justify-center text-dim hover:text-text disabled:opacity-30 rounded">
                    <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="18 15 12 9 6 15"/></svg>
                  </button>
                  <button onclick={() => moveDown(w.id)} disabled={i === config.widgets.length - 1} aria-label="move down" class="w-11 h-11 sm:w-7 sm:h-7 inline-flex items-center justify-center text-dim hover:text-text disabled:opacity-30 rounded">
                    <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
                  </button>
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
        <section class="mb-4 bg-mantle border border-surface1 rounded-lg p-4 space-y-3">
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
                <li class="flex items-center gap-2 px-2.5 py-1.5 rounded {active ? 'bg-surface1 border border-surface2' : 'border border-transparent hover:bg-surface0'}">
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
        <div class="mb-4 p-4 bg-mantle border border-surface1 rounded-lg text-sm">
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
        <!-- grid-auto-flow: dense lets the browser slot smaller cards
             into gaps left by tall ones in earlier rows, so the layout
             auto-fills empty space instead of leaving a phonebook-style
             waterfall. items-start keeps each widget at its natural
             height (no ugly empty padding inside short widgets). -->
        <div class="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-2 sm:gap-3 items-start" style="grid-auto-flow: dense;">
          {#each activeWidgets as { widget, meta } (widget.id)}
            <!-- Each widget chunk is loaded lazily via meta.load();
                 the registry's loader is memoised so re-renders await
                 the same resolved promise instead of refetching.
                 The skeleton placeholder reserves a small height so
                 the grid doesn't reflow once each chunk lands. -->
            <div class={meta.span === 2 ? 'lg:col-span-2 xl:col-span-3' : ''}>
              {#await meta.load()}
                <div class="bg-surface0 border border-surface1 rounded-lg p-3 animate-pulse h-24"></div>
              {:then Widget}
                <Widget vaultPath={vault?.root ?? ''} />
              {:catch err}
                <div class="bg-surface0 border border-error text-error rounded-lg p-3 text-xs">
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
