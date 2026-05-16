<script lang="ts">
  import '../app.css';
  import { onMount, untrack } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api } from '$lib/api';
  import { page } from '$app/stores';
  import Drawer from '$lib/components/Drawer.svelte';
  import CommandPalette from '$lib/components/CommandPalette.svelte';
  import BottomNav from '$lib/components/BottomNav.svelte';
  import Toaster from '$lib/components/Toaster.svelte';
  import OfflineBanner from '$lib/components/OfflineBanner.svelte';
  import InstallPrompt from '$lib/components/InstallPrompt.svelte';
  import RunningTimer from '$lib/components/RunningTimer.svelte';
  import NavIcon from '$lib/components/NavIcon.svelte';
  import Logo from '$lib/components/Logo.svelte';
  import QuickCaptureFab from '$lib/components/QuickCaptureFab.svelte';
  import PomodoroPill from '$lib/components/PomodoroPill.svelte';
  import AIOverlay from '$lib/components/AIOverlay.svelte';
  import NoteTray from '$lib/components/NoteTray.svelte';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { findMode, currentModeId } from '$lib/ai/agents';
  import { sidebarPins, togglePin } from '$lib/stores/sidebar-pins';
  import { lastOpenNote, trayEnabled } from '$lib/stores/open-note';
  import { get } from 'svelte/store';

  // Sidebar quick-action chips. Each one opens the AI overlay
  // pre-filled with a prompt and (when send=true) fires it
  // immediately. Keeps the most-used AI surfaces one click away
  // instead of two (open overlay → type / pick action). The
  // chips defer to the Sabbath check by going through openAIOverlay
  // which won't bypass the server-side Sabbath gate even if it
  // opens the panel.
  type AIQuick = { id: string; label: string; glyph: string; modeId?: string; text: string; send: boolean; title: string };
  const aiQuickActions: AIQuick[] = [
    { id: 'briefing', label: 'Briefing', glyph: '☀', text: 'Give me a short morning briefing — top three things I should focus on today and one thing I might be forgetting.', send: true, title: 'Morning briefing — top 3 + one thing you might forget' },
    { id: 'triage', label: 'Triage', glyph: '⚖', text: 'Help me triage my open tasks — which 3 should I do today, and what should I defer or delete?', send: true, title: 'Inbox / task triage — pick 3, defer the rest' },
    { id: 'free', label: 'Find time', glyph: '⏱', modeId: 'analyst', text: 'Find me 60 minutes for deep work in the next 3 days. List 3 candidate slots.', send: false, title: 'Find a free slot for deep work' }
  ];
  function runQuickAction(q: AIQuick) {
    openAIOverlay({ modeId: q.modeId, text: q.text, send: q.send });
    drawerOpen = false;
  }
  import { connect, disconnect, wsConnected, onWsEvent } from '$lib/ws';
  import { theme, nextTheme, themeLabel } from '$lib/stores/theme';
  import { modulesStore } from '$lib/stores/modules';
  import { sabbath, SABBATH_HIDE_MODULES } from '$lib/stores/sabbath';
  import { goto } from '$app/navigation';
  import { toast } from '$lib/components/toast';
  import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';

  let palette: { show: () => void } | undefined = $state();

  let { children } = $props();

  // Manage the websocket lifecycle alongside auth
  $effect(() => {
    if ($auth) connect();
    else disconnect();
  });

  // Auto-reload when the service worker activates a new build. The SW
  // posts {type:'sw-updated'} after clients.claim(). Previously we
  // waited for the next tab-hide so a mid-edit refresh wouldn't drop
  // unsaved work — but mobile users often keep one tab open all day,
  // and silently waiting meant the user thought we'd shipped nothing
  // (cache-stuck on yesterday's bundle for hours after a deploy).
  // New behaviour: hidden tab → reload immediately; visible tab →
  // surface a toast with a "Reload" action so the user gets agency
  // without being yanked mid-keystroke. The note drafts module
  // already preserves every keystroke to localStorage so even a
  // reload-during-edit can't lose data.
  let updateAvailable = $state(false);
  onMount(() => {
    // Pre-hydration splash hide. Setting data-app-ready on <html>
    // triggers the `[data-app-ready] #app-splash { display: none }`
    // rule in app.html. Done in the root onMount so EVERY child page
    // benefits — even ones that have their own loading states only
    // start showing them after this fires. Wrapped in a defensive
    // try so an SSR or test environment without document doesn't
    // crash.
    try {
      document.documentElement.setAttribute('data-app-ready', '1');
    } catch (_) {
      // No document — ignore.
    }
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;
    const onMessage = (event: MessageEvent) => {
      if (event?.data?.type !== 'sw-updated') return;
      if (document.visibilityState === 'hidden') {
        location.reload();
      } else {
        updateAvailable = true;
      }
    };
    navigator.serviceWorker.addEventListener('message', onMessage);
    return () => navigator.serviceWorker.removeEventListener('message', onMessage);
  });

  // ── Sidebar live counts ────────────────────────────────────────────
  // overdueTasks: open tasks with a dueDate strictly before today (YYYY-MM-DD).
  // todayEvents: calendar feed entries (events.json + ICS subscriptions +
  // scheduled tasks) whose date / start lands on today. Both refresh
  // on mount, on auth gain, and on relevant WS events so the badges
  // stay in sync after a TUI edit or a tab returning from background
  // without manual reload. Errors swallow silently — a stale or
  // missing badge is fine; an alert spam isn't.
  let overdueTaskCount = $state<number>(0);
  let todayEventCount = $state<number>(0);

  function todayISO(): string {
    const d = new Date();
    const m = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${d.getFullYear()}-${m}-${day}`;
  }

  async function refreshOverdueTasks() {
    try {
      const today = todayISO();
      const res = await api.listTasks({ status: 'open', due_before: today });
      overdueTaskCount = res.tasks.filter((t) => !t.done && !!t.dueDate && t.dueDate < today).length;
    } catch {
      // leave previous count in place
    }
  }

  async function refreshTodayEvents() {
    try {
      const today = todayISO();
      const feed = await api.calendar(today, today);
      const isToday = (ev: { date?: string; start?: string }) => {
        if (ev.date) return ev.date === today;
        if (ev.start) return ev.start.slice(0, 10) === today;
        return false;
      };
      todayEventCount = feed.events.filter(isToday).length;
    } catch {
      // leave previous count in place
    }
  }

  // Trigger fetches once auth is known. Re-runs when $auth flips
  // false→true so a fresh login populates badges immediately.
  $effect(() => {
    if (!$auth) {
      overdueTaskCount = 0;
      todayEventCount = 0;
      return;
    }
    refreshOverdueTasks();
    refreshTodayEvents();
  });

  // Listen to WS task/event mutations to keep badges live without
  // polling. We debounce lightly via microtask so a burst (e.g. plan
  // apply that flips many tasks) collapses into one refetch.
  onMount(() => {
    let pendingTasks = false;
    let pendingEvents = false;
    const off = onWsEvent((ev) => {
      if (ev.type === 'task.changed' || ev.type === 'vault.rescanned') {
        if (pendingTasks) return;
        pendingTasks = true;
        queueMicrotask(() => { pendingTasks = false; refreshOverdueTasks(); });
      }
      if (ev.type === 'event.changed' || ev.type === 'event.removed' || ev.type === 'task.changed' || ev.type === 'vault.rescanned') {
        if (pendingEvents) return;
        pendingEvents = true;
        queueMicrotask(() => { pendingEvents = false; refreshTodayEvents(); });
      }
    });
    return off;
  });

  // moduleId gates the entry against the modules store. Entries without
  // a moduleId stay visible unconditionally.
  type NavItem = { href: string; label: string; icon: string; moduleId?: string };

  // Grouped nav. Section ID is the persistence key for collapsed state
  // and the header label. The Today entry sits above all groups (no
  // header) because it's the always-on home — sections start where
  // organisation begins to help.
  const today: NavItem = { href: '/', label: 'Today', icon: 'today' };

  type NavSection = { id: string; label: string; items: NavItem[] };
  const sections: NavSection[] = [
    {
      id: 'daily',
      label: 'Daily',
      items: [
        { href: '/morning', label: 'Morning', icon: 'morning', moduleId: 'morning' },
        { href: '/tasks', label: 'Tasks', icon: 'tasks' },
        { href: '/calendar', label: 'Calendar', icon: 'calendar' },
        { href: '/jots', label: 'Jots', icon: 'jots', moduleId: 'jots' },
        { href: '/habits', label: 'Habits', icon: 'habits', moduleId: 'habit_tracker' },
        { href: '/examen', label: 'Examen', icon: 'examen', moduleId: 'examen' }
      ]
    },
    {
      id: 'plan',
      label: 'Plan',
      items: [
        { href: '/vision', label: 'Vision', icon: 'vision', moduleId: 'vision' },
        { href: '/review', label: 'Review', icon: 'review', moduleId: 'weekly_review' },
        { href: '/goals', label: 'Goals', icon: 'goals', moduleId: 'goals' },
        { href: '/deadlines', label: 'Deadlines', icon: 'deadline', moduleId: 'deadlines' },
        { href: '/projects', label: 'Projects', icon: 'projects', moduleId: 'projects' },
        { href: '/ventures', label: 'Ventures', icon: 'ventures', moduleId: 'ventures' }
      ]
    },
    {
      id: 'life',
      label: 'Life',
      items: [
        { href: '/finance', label: 'Finance', icon: 'finance', moduleId: 'finance' },
        { href: '/shopping', label: 'Shopping', icon: 'shopping', moduleId: 'shopping' },
        { href: '/hub', label: 'Hub', icon: 'hub', moduleId: 'hub' },
        { href: '/people', label: 'People', icon: 'people', moduleId: 'people' },
        { href: '/measurements', label: 'Metrics', icon: 'measurements', moduleId: 'measurements' },
        { href: '/prayer', label: 'Prayer', icon: 'prayer', moduleId: 'prayer' },
        { href: '/scripture', label: 'Scripture', icon: 'scripture', moduleId: 'scripture' },
        { href: '/roots', label: 'Roots', icon: 'roots', moduleId: 'roots' }
      ]
    },
    {
      id: 'knowledge',
      label: 'Knowledge',
      items: [
        { href: '/notes', label: 'Notes', icon: 'notes' },
        { href: '/search', label: 'Search', icon: 'search' },
        { href: '/books', label: 'Books', icon: 'books', moduleId: 'books' },
        { href: '/templates', label: 'Templates', icon: 'templates' },
        { href: '/objects', label: 'Objects', icon: 'objects', moduleId: 'objects' },
        { href: '/tags', label: 'Tags', icon: 'tags' }
      ]
    },
    {
      id: 'ai',
      label: 'AI',
      items: [
        { href: '/agents', label: 'Agents', icon: 'agents', moduleId: 'agents' },
        { href: '/chat', label: 'Chat', icon: 'chat', moduleId: 'chat' }
      ]
    }
  ];

  // Settings stays in the footer rail next to theme + sign-out, not as
  // a section item — it's a meta destination.
  const settingsItem: NavItem = { href: '/settings', label: 'Settings', icon: 'settings' };

  // Flat nav list — used for: route guard match, mobile back-to-section
  // header, modules filter parity. Includes Today + every section item +
  // settings so route resolution covers the full surface.
  const nav: NavItem[] = [today, ...sections.flatMap((s) => s.items), settingsItem];

  // Pinned items — resolve hrefs from $sidebarPins against the flat nav
  // so the user's pins survive across module-config changes (a pin to a
  // route that's been disabled simply drops out instead of throwing).
  // Filter pinned-but-hidden against the same modules+sabbath logic
  // that gates the section bodies, so a Sabbath user doesn't see work
  // items at the top of their rail.
  let pinnedItems = $derived.by(() => {
    void $modulesStore;
    void $sabbath;
    if ($sidebarPins.length === 0) return [] as NavItem[];
    const byHref = new Map(nav.map((n) => [n.href, n]));
    return $sidebarPins
      .map((h) => byHref.get(h))
      .filter((it): it is NavItem => {
        if (!it) return false;
        if (it.moduleId) {
          if (!modulesStore.isEnabled(it.moduleId)) return false;
          if ($sabbath && SABBATH_HIDE_MODULES.includes(it.moduleId)) return false;
        }
        return true;
      });
  });

  // Per-section visible items (after module filter + sabbath overlay).
  // Sections with no visible items collapse out of the rendered list
  // entirely so the user doesn't see an empty header. Sabbath mode
  // hides work modules on top of the user's persistent module config
  // — it's a temporal overlay, not a config edit.
  let visibleSections = $derived.by(() => {
    void $modulesStore;
    void $sabbath;
    return sections
      .map((s) => ({
        ...s,
        items: s.items.filter((item) => {
          if (item.moduleId && !modulesStore.isEnabled(item.moduleId)) return false;
          if ($sabbath && item.moduleId && SABBATH_HIDE_MODULES.includes(item.moduleId)) return false;
          return true;
        })
      }))
      .filter((s) => s.items.length > 0);
  });

  // ── Sidebar UX state ──────────────────────────────────────────────
  // Collapsed sections + compact mode are both per-device localStorage.
  // collapsedSections is a record of section.id → true to keep the
  // wire format tiny (only collapsed sections are stored).
  const COLLAPSED_KEY = 'granit.sidebar.collapsed';
  const COMPACT_KEY = 'granit.sidebar.compact';
  // Default-collapse everything except Daily. The original default
  // was "all expanded", which surfaced 25+ items at once and made
  // the sidebar feel like a phonebook. Daily stays expanded because
  // it's the morning/tasks/calendar cluster every user lives in. The
  // others get expanded as needed and the choice persists. Existing
  // users keep whatever they had.
  const DEFAULT_COLLAPSED: Record<string, boolean> = {
    plan: true,
    life: true,
    knowledge: true,
    ai: true
  };
  function loadCollapsed(): Record<string, boolean> {
    return loadStored<Record<string, boolean>>(COLLAPSED_KEY, { ...DEFAULT_COLLAPSED });
  }
  let collapsedSections = $state<Record<string, boolean>>(loadCollapsed());
  function toggleSection(id: string) {
    const next = { ...collapsedSections };
    if (next[id]) delete next[id];
    else next[id] = true;
    collapsedSections = next;
    saveStored(COLLAPSED_KEY, next);
  }
  // Auto-expand the section containing the active route. Without
  // this the user can land on /goals (collapsed-by-default Plan
  // section) and the sidebar misleads them about where they are.
  // We mutate collapsedSections without persisting so closing it
  // again — and going elsewhere — restores the user's preference.
  //
  // The collapsedSections read MUST be in untrack — otherwise the
  // user can never collapse a section containing the active route,
  // because the toggleSection write retriggers this effect, which
  // re-expands it immediately. Same Svelte 5 untrack re-seed pattern
  // we hit in the email-signatures editor.
  $effect(() => {
    const path = $page.url.pathname;
    if (path === '/') return;
    untrack(() => {
      for (const s of sections) {
        const inSection = s.items.some((it) => path === it.href || path.startsWith(it.href + '/'));
        if (inSection && collapsedSections[s.id]) {
          collapsedSections = { ...collapsedSections, [s.id]: false };
        }
      }
    });
  });

  let compact = $state<boolean>(loadStoredString(COMPACT_KEY, '0') === '1');
  function toggleCompact() {
    compact = !compact;
    saveStoredString(COMPACT_KEY, compact ? '1' : '0');
  }

  // Route guard: if the user lands on a path whose module is disabled
  // (deep link, bookmark, stale tab), bounce to home. We use a tiny
  // delay-via-effect rather than a load function because layout.ts is
  // SSR-only — by the time any code runs the SPA is already mounted.
  // Also bounces sabbath-hidden modules — typing /tasks during
  // Sabbath should redirect to home with a soft message rather than
  // letting the user back-door past the discipline.
  $effect(() => {
    void $modulesStore; // re-run when modules state arrives/changes
    void $sabbath;
    const path = $page.url.pathname;
    const match = nav.find(
      (n) => n.href !== '/' && (path === n.href || path.startsWith(n.href + '/'))
    );
    if (match?.moduleId && !modulesStore.isEnabled(match.moduleId)) {
      toast.info(`${match.label} is disabled — enable it in Settings → Modules`);
      goto('/', { replaceState: true });
      return;
    }
    if ($sabbath && match?.moduleId && SABBATH_HIDE_MODULES.includes(match.moduleId)) {
      toast.info(`${match.label} is hidden during Sabbath — exit Sabbath mode to access.`);
      goto('/', { replaceState: true });
    }
  });

  let drawerOpen = $state(false);

  // Close drawer on route change
  $effect(() => {
    void $page.url.pathname;
    drawerOpen = false;
  });

  // Mod-Shift-O — jump to whatever the open-note tray remembers.
  // Mirrors the user's "ein system tray" framing: one keystroke
  // returns you to the last note from anywhere in the app. The
  // shortcut intentionally fires even when the user is typing into
  // an input/textarea, mirroring Mod+J (Ask AI) and Mod+K (palette):
  // these are global app-shell shortcuts, not text editing ones.
  // Skips when the tray is disabled (settings opt-out), when there's
  // nothing remembered, or when the user is already on that note.
  //
  // get() is intentional: we don't want this $effect to re-subscribe
  // (and re-register the listener) every time the tray store ticks.
  // The listener is registered once and reads fresh values at
  // keypress time.
  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || !e.shiftKey || e.altKey) return;
      if (e.key.toLowerCase() !== 'o') return;
      if (!get(trayEnabled)) return;
      const entry = get(lastOpenNote);
      if (!entry) return;
      const targetPath = '/notes/' + encodeURIComponent(entry.path);
      // Already there → suppress (the chip would be hidden anyway).
      const here = get(page).url.pathname;
      if (here === targetPath) return;
      e.preventDefault();
      goto(targetPath);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  let activeNav = $derived.by(() => {
    return nav.find((n) => n.href === $page.url.pathname || (n.href !== '/' && $page.url.pathname.startsWith(n.href)));
  });
  let title = $derived(activeNav?.label ?? 'everything');
  let showBackToSection = $derived(
    !!activeNav && activeNav.href !== '/' && $page.url.pathname !== activeNav.href
  );

  // Browser tab title. Empty home stays as just 'Granit'; deep
  // pages get 'Granit · Pagename' so multiple open tabs are
  // distinguishable. Single source of truth (the same activeNav
  // that drives the mobile header) means new routes pick this up
  // automatically when added to the nav array.
  let tabTitle = $derived.by(() => {
    if (!activeNav || activeNav.href === '/') return 'Granit';
    return `Granit · ${activeNav.label}`;
  });

  function NavLinks() {}
</script>

<svelte:head>
  <title>{tabTitle}</title>
</svelte:head>

{#snippet navItem(item: NavItem, isCompact: boolean, opts: { showPinAction?: boolean } = {})}
  {@const active = $page.url.pathname === item.href || (item.href !== '/' && $page.url.pathname.startsWith(item.href))}
  {@const badge = item.href === '/tasks' && overdueTaskCount > 0
    ? { count: overdueTaskCount, tone: 'error' as const, label: `${overdueTaskCount} overdue` }
    : item.href === '/calendar' && todayEventCount > 0
      ? { count: todayEventCount, tone: 'subtle' as const, label: `${todayEventCount} today` }
      : null}
  {@const pinned = $sidebarPins.includes(item.href)}
  {@const canPin = opts.showPinAction !== false && item.href !== '/' && item.href !== '/settings' && !isCompact}
  <a
    href={item.href}
    onclick={() => (drawerOpen = false)}
    title={isCompact ? (badge ? `${item.label} — ${badge.label}` : item.label) : undefined}
    aria-label={badge ? `${item.label}, ${badge.label}` : item.label}
    class="group relative flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-1.5'} rounded text-sm transition-colors
      {active
        ? 'text-primary bg-surface1 font-medium'
        : 'text-subtext hover:bg-surface0 hover:text-text focus-visible:bg-surface0 focus-visible:text-text focus-visible:outline-none'}"
  >
    <!-- Active rail: a 3px accent strip on the left edge replaces
         the heavier full-row fill, so scanning down the sidebar
         lands on the active item without the eye getting pulled.
         3px reads cleanly on every density / DPR — 2px disappeared
         under blur on some displays. -->
    {#if active}
      <span class="absolute left-0 top-1.5 bottom-1.5 w-[3px] rounded-full bg-primary" aria-hidden="true"></span>
    {/if}
    <span class="relative flex-shrink-0">
      <NavIcon name={item.icon} class="w-5 h-5 flex-shrink-0" />
      {#if isCompact && badge}
        <!-- Compact-mode badge sits as a corner overlay on the icon
             so the rail can still surface alerts without labels. The
             error tone shows the digit; the subtle tone collapses to
             a dot since the count is informational, not urgent. -->
        {#if badge.tone === 'error'}
          <span
            class="absolute -top-1.5 -right-1.5 min-w-[16px] h-4 px-1 rounded-full bg-error text-on-primary text-[9px] font-bold leading-4 text-center"
            aria-hidden="true"
          >{badge.count > 9 ? '9+' : badge.count}</span>
        {:else}
          <span class="absolute -top-0.5 -right-0.5 w-1.5 h-1.5 rounded-full bg-primary" aria-hidden="true"></span>
        {/if}
      {/if}
    </span>
    {#if !isCompact}
      <span class="truncate flex-1">{item.label}</span>
      {#if badge}
        {#if badge.tone === 'error'}
          <span
            class="ml-auto inline-flex items-center justify-center min-w-[18px] h-[18px] px-1.5 rounded-full bg-error text-on-primary text-[10px] font-semibold leading-none"
            aria-hidden="true"
          >{badge.count > 99 ? '99+' : badge.count}</span>
        {:else}
          <span
            class="ml-auto inline-flex items-center justify-center min-w-[18px] h-[18px] px-1.5 rounded-full bg-surface1 text-subtext text-[10px] font-medium leading-none"
            aria-hidden="true"
          >{badge.count > 99 ? '99+' : badge.count}</span>
        {/if}
      {/if}
      {#if canPin}
        <!-- Pin toggle. Always visible when pinned (so the user can
             unpin without hovering); revealed on hover otherwise. The
             button uses stopPropagation so clicking the star doesn't
             also navigate. Reads as a star to match the universal pin
             metaphor. -->
        <button
          type="button"
          onclick={(e) => { e.preventDefault(); e.stopPropagation(); togglePin(item.href); }}
          title={pinned ? 'unpin from sidebar top' : 'pin to sidebar top'}
          aria-label={pinned ? `Unpin ${item.label}` : `Pin ${item.label}`}
          class="ml-1 p-0.5 rounded transition-opacity {pinned ? 'text-warning opacity-100' : 'text-dim opacity-0 group-hover:opacity-70 hover:!opacity-100 hover:text-warning'}"
        >
          <svg viewBox="0 0 16 16" class="w-3.5 h-3.5" fill={pinned ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="1.4" stroke-linejoin="round">
            <path d="M8 1.5l1.85 4.05L14 6.2l-3.1 2.85L11.7 13 8 10.85 4.3 13l.8-3.95L2 6.2l4.15-.65z"/>
          </svg>
        </button>
      {/if}
    {/if}
  </a>
{/snippet}

{#snippet navContent(isCompact: boolean)}
  <div class="flex flex-col h-full">
    <!-- Brand area collapses to icon-only in compact mode so the rail
         doesn't blow up to full width on narrow desktops. The 'e'
         monogram + accent dot reads as a logo without needing the
         full text. -->
    <div class="border-b border-surface1 {isCompact ? 'px-2 py-3 flex justify-center' : 'px-4 py-3'}">
      {#if isCompact}
        <div class="w-9 h-9 rounded bg-surface1 text-primary flex items-center justify-center" aria-label="Granit">
          <Logo class="w-5 h-5" label="" />
        </div>
      {:else}
        <div class="flex items-center gap-2">
          <div class="w-7 h-7 rounded bg-surface1 text-primary flex items-center justify-center flex-shrink-0">
            <Logo class="w-4 h-4" label="" />
          </div>
          <div class="min-w-0">
            <div class="text-sm font-semibold text-text leading-tight">Granit</div>
            <div class="text-[10px] text-dim leading-tight mt-0.5">your vault, anywhere</div>
          </div>
        </div>
      {/if}
    </div>

    <nav class="flex-1 overflow-y-auto {isCompact ? 'px-1.5 py-3' : 'px-2 py-3'} space-y-1">
      <!-- Quick jump — compact form drops the kbd hint + label,
           keeps the icon as the click target. -->
      <button
        onclick={() => { palette?.show(); drawerOpen = false; }}
        title={isCompact ? 'Quick jump (⌘K)' : undefined}
        class="w-full flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-1.5'} rounded text-sm text-subtext hover:bg-surface0 hover:text-text transition-colors"
      >
        <NavIcon name="search" class="w-5 h-5 flex-shrink-0" />
        {#if !isCompact}
          <span class="flex-1 text-left">Quick jump</span>
          <kbd class="text-[10px] text-dim font-mono px-1.5 py-0.5 bg-surface0 border border-surface1 rounded">⌘K</kbd>
        {/if}
      </button>

      <!-- Ask AI — opens the global AI overlay. Subtle gradient
           border + sparkle icon distinguish it from regular nav so
           the user notices an "intelligence" surface without it
           dominating the rail. Mod+J also works from anywhere; this
           button is for discovery + click-first users.

           When Sabbath mode is on, AI calls are server-side gated; we
           dim the sparkle and surface a paused dot so the user
           understands the click will be silenced before they make it. -->
      <button
        onclick={() => { openAIOverlay(); drawerOpen = false; }}
        title={isCompact ? ($sabbath ? 'AI paused — Sabbath' : 'Ask AI (⌘J)') : undefined}
        class="w-full flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-2'} rounded text-sm mb-2 transition-colors {$sabbath ? 'bg-surface0 text-dim' : 'bg-primary text-on-primary hover:opacity-90 font-medium'}"
      >
        <span class="relative flex-shrink-0">
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"/>
            <circle cx="12" cy="12" r="3.5" fill="currentColor"/>
          </svg>
          {#if $sabbath}
            <span class="absolute -top-0.5 -right-0.5 w-2 h-2 rounded-full bg-warning" aria-hidden="true"></span>
          {/if}
        </span>
        {#if !isCompact}
          {#if $sabbath}
            <span class="flex-1 text-left">AI paused</span>
            <span class="text-[10px] text-warning font-medium">Sabbath</span>
          {:else}
            <span class="flex-1 text-left">Ask AI</span>
            <kbd class="text-[10px] font-mono px-1.5 py-0.5 rounded border border-on-primary text-on-primary opacity-70">⌘J</kbd>
          {/if}
        {/if}
      </button>

      <!-- AI sub-rail. Mode pill + quick-action chips. Hidden in
           Sabbath mode (the parent button already says AI is paused;
           dimming the chips would just be visual noise) and in
           compact mode (icon rail has no horizontal space for the
           chip row — the user can still hit Mod+J). The mode pill
           is informational, click-to-open: tells the user which
           agent posture is currently selected so they read at a
           glance "I'm in Coach mode" without opening the overlay. -->
      {#if !isCompact && !$sabbath}
        {@const cur = findMode($currentModeId)}
        <div class="px-3 -mt-1 mb-2 space-y-1.5">
          <button
            type="button"
            onclick={() => { openAIOverlay(); drawerOpen = false; }}
            title={`current AI mode — ${cur.tagline}. Click to switch.`}
            class="w-full flex items-center gap-1.5 text-[10px] text-dim hover:text-text transition-colors"
          >
            <span aria-hidden="true">{cur.glyph}</span>
            <span class="uppercase tracking-wider">Mode: {cur.label}</span>
            <span class="ml-auto opacity-60">change</span>
          </button>
          <div class="flex flex-wrap gap-1">
            {#each aiQuickActions as q (q.id)}
              <button
                type="button"
                onclick={() => runQuickAction(q)}
                title={q.title}
                class="text-[11px] px-2 py-1 rounded inline-flex items-center gap-1 bg-surface0 text-subtext hover:bg-surface1 hover:text-text transition-colors"
              >
                <span aria-hidden="true">{q.glyph}</span>
                <span>{q.label}</span>
              </button>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Pinned items — user-curated rail above Today. Hidden when
           empty so first-time users don't see a phantom group. The
           pin star inside each navItem is the only entry point;
           there's no separate manage screen because the action
           model is "see it in nav, hover to pin/unpin". In compact
           mode the items render without their group header (parity
           with the section dividers below). -->
      {#if pinnedItems.length > 0}
        {#if isCompact}
          {#each pinnedItems as item (item.href)}
            {@render navItem(item, true)}
          {/each}
          <div class="my-1.5 flex items-center justify-center gap-1" aria-hidden="true">
            <span class="h-px w-2 bg-surface1"></span>
            <span class="w-1 h-1 rounded-full bg-surface1"></span>
            <span class="h-px w-2 bg-surface1"></span>
          </div>
        {:else}
          <div class="pb-1 mb-1 border-b border-surface1">
            <div class="px-3 pb-0.5 pt-0.5 text-[10px] uppercase tracking-wider text-dim flex items-center gap-1">
              <svg viewBox="0 0 16 16" class="w-3 h-3" fill="currentColor" aria-hidden="true">
                <path d="M8 1.5l1.85 4.05L14 6.2l-3.1 2.85L11.7 13 8 10.85 4.3 13l.8-3.95L2 6.2l4.15-.65z"/>
              </svg>
              <span>Pinned</span>
            </div>
            <div class="space-y-0.5">
              {#each pinnedItems as item (item.href)}
                {@render navItem(item, false)}
              {/each}
            </div>
          </div>
        {/if}
      {/if}

      <!-- Today sits above all groups, no header, since it's home. -->
      {@render navItem(today, isCompact, { showPinAction: false })}

      <!-- Sections. In compact mode the section header collapses to a
           thin separator line so the visual rhythm of grouping is
           preserved without the labels. -->
      {#each visibleSections as section}
        {@const isCollapsed = !!collapsedSections[section.id] && !isCompact}
        {#if isCompact}
          <!-- Compact section divider: a short centered rule + a tiny
               pip so the visual rhythm of grouping survives icon-only
               mode without forcing the user to remember which icon
               belongs to which section. Title surfaces the section
               label on hover for orientation. -->
          <div
            class="my-2.5 flex items-center justify-center gap-1"
            aria-hidden="true"
            title={section.label}
          >
            <span class="h-px w-2 bg-surface1"></span>
            <span class="w-1 h-1 rounded-full bg-surface1"></span>
            <span class="h-px w-2 bg-surface1"></span>
          </div>
          {#each section.items as item}
            {@render navItem(item, true)}
          {/each}
        {:else}
          <div class="pt-1">
            <button
              type="button"
              onclick={() => toggleSection(section.id)}
              aria-expanded={!isCollapsed}
              class="w-full flex items-center gap-1.5 px-3 py-1 text-[10px] uppercase tracking-wider text-dim hover:text-subtext transition-colors"
            >
              <span class="flex-1 text-left">{section.label}</span>
              <svg viewBox="0 0 24 24" class="w-3 h-3 transition-transform {isCollapsed ? '-rotate-90' : ''}" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="6 9 12 15 18 9" />
              </svg>
            </button>
            {#if !isCollapsed}
              <div class="space-y-0.5 mt-0.5">
                {#each section.items as item}
                  {@render navItem(item, false)}
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      {/each}
    </nav>

    <!-- Footer rail. Settings, theme, compact toggle, sign out. -->
    <div class="border-t border-surface1 {isCompact ? 'px-1.5 py-2 space-y-1' : 'px-2 py-3 space-y-1'}">
      {@render navItem(settingsItem, isCompact)}

      <button
        onclick={() => theme.set(nextTheme($theme))}
        title={isCompact ? `Theme: ${themeLabel($theme)} — tap to cycle` : undefined}
        class="w-full flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-1.5'} rounded text-sm text-subtext hover:bg-surface0 hover:text-text transition-colors"
      >
        <svg viewBox="0 0 24 24" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          {#if $theme === 'dark'}
            <path d="M21 12.79A9 9 0 1 1 11.21 3a7 7 0 0 0 9.79 9.79z"/>
          {:else if $theme === 'light'}
            <circle cx="12" cy="12" r="4"/>
            <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/>
          {:else}
            <circle cx="12" cy="12" r="9"/>
            <path d="M12 3a9 9 0 0 0 0 18z" fill="currentColor"/>
          {/if}
        </svg>
        {#if !isCompact}
          <span class="flex-1 text-left">Theme: {themeLabel($theme)}</span>
          <span class="text-[10px] text-dim">cycle</span>
        {/if}
      </button>

      <!-- Sabbath row. Mark 2:27: "the sabbath was made for man." A
           split layout: the icon+label opens the /sabbath landing
           (verse, time-remaining, schedule); the "→" pill toggles
           sabbath state in place. Two distinct intents, one row.
           Compact mode collapses both into a single icon-button
           that just toggles, since hover-tooltips do most of the
           explaining and a side-by-side button row doesn't fit.
           Auto-clears at midnight via a read-time check in the
           store, so a forgotten 'on' state recovers the next
           morning by itself. -->
      {#if isCompact}
        <button
          onclick={() => sabbath.toggle()}
          title={$sabbath ? 'Sabbath mode is on — tap to exit' : 'Enter sabbath mode (hides work modules for today)'}
          class="w-full flex justify-center items-center px-2 py-2 rounded text-sm transition-colors {$sabbath ? 'bg-success text-on-primary hover:opacity-90' : 'text-dim hover:bg-surface0 hover:text-text'}"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            {#if $sabbath}
              <path d="M12 2l1.5 4.5L18 8l-4.5 1.5L12 14l-1.5-4.5L6 8l4.5-1.5L12 2zM12 14v8M9 22h6"/>
            {:else}
              <path d="M12 2l2 5h5l-4 3 1.5 5L12 12l-4.5 3L9 10 5 7h5z"/>
            {/if}
          </svg>
        </button>
      {:else}
        <div class="flex items-stretch gap-1 rounded {$sabbath ? 'bg-success text-on-primary' : ''}">
          <a
            href="/sabbath"
            onclick={() => (drawerOpen = false)}
            class="flex-1 flex items-center gap-3 px-3 py-2 rounded-l transition-colors {$sabbath ? 'hover:opacity-90' : 'text-dim hover:bg-surface0 hover:text-text'}"
          >
            <svg viewBox="0 0 24 24" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
              {#if $sabbath}
                <path d="M12 2l1.5 4.5L18 8l-4.5 1.5L12 14l-1.5-4.5L6 8l4.5-1.5L12 2zM12 14v8M9 22h6"/>
              {:else}
                <path d="M12 2l2 5h5l-4 3 1.5 5L12 12l-4.5 3L9 10 5 7h5z"/>
              {/if}
            </svg>
            <span class="flex-1 text-left">{$sabbath ? 'Sabbath on' : 'Sabbath'}</span>
            <span class="text-[10px] {$sabbath ? 'opacity-80' : 'text-dim'}">open</span>
          </a>
          <button
            onclick={() => sabbath.toggle()}
            title={$sabbath ? 'tap to exit sabbath' : 'enter sabbath now'}
            aria-label={$sabbath ? 'exit sabbath' : 'enter sabbath'}
            class="px-2.5 py-2 rounded-r transition-colors {$sabbath ? 'hover:opacity-90' : 'text-dim hover:bg-surface0 hover:text-text'}"
          >
            <span class="text-base">{$sabbath ? '×' : '→'}</span>
          </button>
        </div>
      {/if}

      <!-- Desktop-only compact toggle. Hidden on mobile because the
           drawer is already an icon-poor experience and a compact
           toggle in a temporary panel doesn't save anything. -->
      <button
        onclick={toggleCompact}
        title={isCompact ? 'Expand sidebar' : 'Collapse to icons'}
        class="hidden md:flex w-full items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-2'} rounded text-sm text-dim hover:bg-surface0 hover:text-text transition-colors"
      >
        <svg viewBox="0 0 24 24" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          {#if isCompact}
            <polyline points="9 18 15 12 9 6" />
          {:else}
            <polyline points="15 18 9 12 15 6" />
          {/if}
        </svg>
        {#if !isCompact}<span class="flex-1 text-left">Collapse</span>{/if}
      </button>

      {#if !isCompact}
        <div class="flex items-center justify-between px-3 pt-1">
          <button
            onclick={async () => { try { await api.authLogout(); } catch {} auth.clear(); }}
            class="text-xs text-dim hover:text-error transition-colors"
          >
            sign out
          </button>
          <div class="flex items-center gap-1.5" title={$wsConnected ? 'live' : 'offline'}>
            <span class="w-2 h-2 rounded-full {$wsConnected ? 'bg-success' : 'bg-dim'}"></span>
            <span class="text-[10px] text-dim font-mono">v0.0.1</span>
          </div>
        </div>
      {:else}
        <!-- Compact connection pip lives at the very bottom, on its
             own line, so the rail still surfaces live/offline state. -->
        <div class="flex justify-center pt-1" title={$wsConnected ? 'live' : 'offline'}>
          <span class="w-2 h-2 rounded-full {$wsConnected ? 'bg-success' : 'bg-dim'}"></span>
        </div>
      {/if}
    </div>
  </div>
{/snippet}

<!--
  h-dvh (dynamic viewport height) instead of h-screen (100vh) so the
  shell actually shrinks when the on-screen keyboard opens on mobile.
  100vh on iOS Safari stays at the full screen height regardless of
  the keyboard — that's the whole reason child surfaces (chat, ai
  overlay) need their own visualViewport hacks. h-dvh fixes this at
  the root, and falls back to 100vh on browsers that don't support
  dvh (auto via Tailwind's fallback chain).
-->
<div class="h-dvh flex flex-col md:flex-row overflow-hidden">
  {#if $auth}
    <!-- Mobile top bar — back (when in subpage) · title · search.
         Settings used to live here too but it's already in the More
         drawer now, so the redundancy is gone. The bottom nav is the
         primary nav surface; this top bar is just contextual. -->
    <header class="md:hidden flex items-center gap-1 px-3 h-12 border-b border-surface1 bg-mantle sticky top-0 z-30 flex-shrink-0">
      {#if showBackToSection && activeNav}
        <a
          href={activeNav.href}
          aria-label="back to {activeNav.label}"
          class="w-10 h-10 -ml-2 flex items-center justify-center text-subtext hover:text-primary rounded"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </a>
      {/if}
      <h1 class="text-base font-medium text-text flex-1 truncate">{title}</h1>
      <button
        onclick={() => palette?.show()}
        aria-label="search"
        class="w-10 h-10 flex items-center justify-center text-subtext hover:text-primary rounded"
      >
        <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="8" /><path d="m21 21-4.3-4.3" stroke-linecap="round" />
        </svg>
      </button>
    </header>

    <!-- Desktop sidebar — expand/compact width is driven by the
         compact toggle in the footer rail. Both states animate via
         the transition class so the resize feels intentional. -->
    <aside
      class="hidden md:flex bg-mantle border-r border-surface1 flex-shrink-0 transition-[width] duration-150 {compact ? 'md:w-14' : 'md:w-56 lg:w-60'}"
    >
      {@render navContent(compact)}
    </aside>

    <!-- Mobile "More" drawer always renders the full (non-compact)
         nav — a temporary panel doesn't benefit from icon-only mode. -->
    <Drawer bind:open={drawerOpen} side="left">
      {@render navContent(false)}
    </Drawer>
  {/if}

  <!-- NoteTray sets --note-tray-h on <html> when visible. The
       tray-reserve class stacks that var on top of the existing
       pb-bottomnav (mobile) / pb-0 (desktop) base so the bottom
       28px of editable content isn't clipped behind the tray. -->
  <main
    class="flex-1 min-h-0 min-w-0 overflow-hidden flex flex-col pb-bottomnav md:pb-0 main-with-tray"
    style="padding-right: var(--ai-pinned-w, 0px);"
  >
    <!-- Sabbath ribbon. Visible from every authed page so the state
         is unmissable; the mode auto-clears at midnight. Click to
         exit. Z-index sits below the running-timer pill so they
         don't clash. -->
    {#if $auth && $sabbath}
      <button
        type="button"
        onclick={() => sabbath.disable()}
        class="flex-shrink-0 px-4 py-1.5 bg-success text-on-primary text-xs text-center hover:opacity-90 transition-colors"
        title="Tap to exit sabbath mode"
      >
        Sabbath mode is on — work modules hidden until midnight. Tap to exit.
      </button>
    {/if}
    <div class="flex-1 min-h-0 overflow-hidden">
      {@render children()}
    </div>
  </main>

  {#if $auth}
    <BottomNav onMore={() => (drawerOpen = true)} />
  {/if}
</div>

{#if $auth}
  <CommandPalette bind:this={palette} />
  <OfflineBanner />
  <!-- Floating top-right pill that's only visible while a clock-in
       is running. Position keeps it out of the way of the editor and
       the offline banner; on mobile the component itself hides
       below the sm breakpoint to avoid crowding the bottom nav. -->
  <div class="fixed top-3 right-3 z-30">
    <RunningTimer />
  </div>
  <!-- One-time PWA install hint. The component is self-gating: it only
       renders when the browser has fired beforeinstallprompt (Chromium)
       or when we detect iOS Safari, and only when the user hasn't
       already installed or dismissed it. Auth-gated because pre-login
       is too early to be useful. -->
  <InstallPrompt />
  <!-- Global quick-capture FAB. Single keystroke (Ctrl+Shift+N) opens
       a small task-capture modal from anywhere in the app. Auth-gated
       since pre-login captures don't have a daily note to write to.
       Hidden during Sabbath — capturing a task is the work the day
       is supposed to be free of. -->
  {#if !$sabbath}
    <QuickCaptureFab />
    <!-- Pomodoro pill — focus-session timer that survives navigation
         + tab close (state in localStorage). Hidden during Sabbath
         (the day's not for focus sessions). The pill itself self-
         hides when idle + no recent finish, so the bottom-right
         stays clean for users who don't use it. -->
    <PomodoroPill />
    <!-- Mobile AI FAB — Mod+J doesn't exist on a phone keyboard,
         and the sidebar "Ask AI" button is hidden behind a drawer
         on mobile. This sparkle button sits above the bottom nav
         (with iOS safe-area padding) so the AI overlay is one tap
         away. Desktop hides this since the sidebar pill covers it.
         Sabbath skip matches the rule applied to QuickCaptureFab. -->
    <button
      type="button"
      onclick={() => openAIOverlay()}
      aria-label="Ask AI"
      title="Ask AI"
      class="md:hidden fixed right-5 z-30 w-12 h-12 flex items-center justify-center rounded-full bg-primary text-on-primary shadow-lg hover:opacity-90 transition-all active:scale-95"
      style="bottom: calc(3.5rem + env(safe-area-inset-bottom, 0px) + 0.75rem);"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"/>
        <circle cx="12" cy="12" r="3.5" fill="currentColor" fill-opacity="0.25"/>
      </svg>
    </button>
  {/if}
  <!-- Global AI overlay. Listens for Mod+J on its own — no
       external trigger needed. Auth-gated since pre-login the
       configured-LLM lookup would 401. -->
  <AIOverlay />

  <!-- Persistent "open note" tray. Slim bottom bar (desktop) /
       chip above the bottom nav (mobile) that surfaces the last
       opened note + any tray-pinned notes so the user can jump
       back from anywhere. The component self-gates on its own
       settings toggle + visibility rules (hidden when on the
       active note, hidden when nothing stored). Auth-gated since
       a pre-login user has no vault to remember from. -->
  <NoteTray />
{/if}
<Toaster />

<!-- "New version available" banner. Sits ABOVE the bottom nav (z-40 vs
     bottom-nav's z-30) and uses sm:bottom-3 on desktop so it doesn't
     collide with the nav rail edge. Kept dismissable because the user
     might want to finish a thought before we yank them — but the action
     button reloads on tap so they don't have to hunt for "clear cache". -->
{#if updateAvailable}
  <div
    role="status"
    class="fixed inset-x-3 z-40 bottom-[calc(3.5rem+env(safe-area-inset-bottom,0px)+0.75rem)] md:bottom-3 md:left-auto md:right-3 md:max-w-sm bg-mantle border border-primary rounded-lg shadow-2xl p-3 flex items-center gap-3"
  >
    <div class="flex-1 min-w-0">
      <div class="text-sm font-medium text-text">Update available</div>
      <div class="text-xs text-dim mt-0.5">Reload to pick up the latest build.</div>
    </div>
    <button
      onclick={() => location.reload()}
      class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 flex-shrink-0"
    >Reload</button>
    <button
      onclick={() => (updateAvailable = false)}
      aria-label="dismiss"
      class="text-dim hover:text-text flex-shrink-0 px-1"
    >×</button>
  </div>
{/if}
