<script lang="ts">
  import { onMount } from 'svelte';
  import { goto, beforeNavigate } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Editor from '$lib/editor/Editor.svelte';
  import NotesTree from '$lib/notes/NotesTree.svelte';
  import Outline from '$lib/notes/Outline.svelte';
  import BacklinksPanel from '$lib/notes/BacklinksPanel.svelte';
  import FrontmatterEditor from '$lib/notes/FrontmatterEditor.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import DailyQuickAdd from '$lib/notes/DailyQuickAdd.svelte';
  import DailyContext from '$lib/notes/DailyContext.svelte';
  import NoteDeadlinesStrip from '$lib/deadlines/NoteDeadlinesStrip.svelte';
  import Drawer from '$lib/components/Drawer.svelte';
  import { toast } from '$lib/components/toast';
  import { getDraft, setDraft, clearDraft, draftDivergesFromServer } from '$lib/notes/drafts';

  type ViewMode = 'edit' | 'preview' | 'split';
  const VIEW_KEY = 'granit.note.viewMode';
  let viewMode = $state<ViewMode>('edit');
  // Restore preference once at mount.
  onMount(() => {
    try {
      const v = localStorage.getItem(VIEW_KEY);
      if (v === 'edit' || v === 'preview' || v === 'split') viewMode = v;
    } catch {}
  });
  function setViewMode(m: ViewMode) {
    viewMode = m;
    try { localStorage.setItem(VIEW_KEY, m); } catch {}
  }

  let note = $state<Note | null>(null);
  let body = $state('');
  let saving = $state(false);
  let dirty = $state(false);
  let error = $state('');
  let lastLoadedPath = $state('');
  let editor: { scrollToLine: (n: number) => void } | undefined = $state();

  let treeDrawerOpen = $state(false);
  let infoDrawerOpen = $state(false);

  let pinned = $state<Set<string>>(new Set());
  let pinBusy = $state(false);

  async function loadPinned() {
    try {
      const r = await api.listPinned();
      pinned = new Set(r.pinned.map((p) => p.path));
    } catch {}
  }

  async function togglePin() {
    if (!note) return;
    pinBusy = true;
    try {
      const want = !pinned.has(note.path);
      const r = await api.setPinned(note.path, want);
      pinned = new Set(r.pinned.map((p) => p.path));
    } finally {
      pinBusy = false;
    }
  }

  $effect(() => {
    const path = $page.params.path;
    if (path) load(decodeURIComponent(path));
  });

  let draftRestored = $state(false);

  async function load(p: string) {
    error = '';
    draftRestored = false;
    if (lastLoadedPath === p) return;
    lastLoadedPath = p;
    try {
      const fresh = await api.getNote(p);
      const serverBody = fresh.body ?? '';

      // Restore a local draft if it diverges AND the server hasn't moved
      // forward since the draft was made. If the server is newer, the user
      // (or another device, or the TUI) has saved newer content — restoring
      // the local draft would silently overwrite that. Discard the stale
      // draft and prefer the server in that case.
      const draft = getDraft(p);
      if (draft && draftDivergesFromServer(draft, serverBody)) {
        const serverNewer = new Date(fresh.modTime) > new Date(draft.baseModTime);
        if (serverNewer) {
          clearDraft(p);
          toast.warning('stale local draft discarded — server has newer content');
        } else {
          prev = draft.body;
          body = draft.body;
          note = fresh;
          dirty = true;
          draftRestored = true;
          treeDrawerOpen = false;
          infoDrawerOpen = false;
          toast.info('restored unsaved draft');
          save({ silent: true });
          return;
        }
      } else if (draft) {
        // Draft matches server — stale, clean up.
        clearDraft(p);
      }

      note = fresh;
      body = serverBody;
      prev = body;
      dirty = false;
      treeDrawerOpen = false;
      infoDrawerOpen = false;
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      // If we have a local draft, surface it instead of an error so an
      // offline reload doesn't lose work.
      const draft = getDraft(p);
      if (draft) {
        prev = draft.body;
        body = draft.body;
        note = {
          path: p,
          title: p.split('/').pop()!.replace(/\.md$/, ''),
          modTime: new Date().toISOString(),
          size: draft.body.length,
          frontmatter: {},
          body: draft.body
        } as Note;
        dirty = true;
        draftRestored = true;
        toast.warning('offline — showing your local draft');
        return;
      }
      error = msg;
      note = null;
      body = '';
      prev = '';
      dirty = false;
      // Critical: drop the dedupe guard so a refetch of the SAME path
      // is allowed. Without this, the user lands on a 404/network-error
      // note, the page renders the error banner, and any subsequent
      // navigation back to that URL (browser back, retry click,
      // sidebar re-click) silently no-ops because `lastLoadedPath ===
      // p` returns early. The user concludes the page is frozen and
      // hits reload.
      lastLoadedPath = '';
    }
  }

  let lastSavedAt = $state<number | null>(null);
  let nowTick = $state(Date.now());
  let saveFailed = $state(false);

  // Tick once per second so "saved Ns ago" stays accurate.
  $effect(() => {
    const t = setInterval(() => (nowTick = Date.now()), 1000);
    return () => clearInterval(t);
  });

  async function save(opts: { silent?: boolean } = {}): Promise<boolean> {
    if (!note || !dirty || saving) return !dirty;
    saving = true;
    error = '';
    try {
      const updated = await api.putNote(note.path, { frontmatter: note.frontmatter as Record<string, unknown>, body });
      note = updated;
      prev = body;
      dirty = false;
      lastSavedAt = Date.now();
      saveFailed = false;
      clearDraft(updated.path);
      draftRestored = false;
      if (!opts.silent) toast.success('saved');
      return true;
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      error = msg;
      saveFailed = true;
      // Even silent auto-saves toast the failure once — we'd rather warn
      // the user than have them assume their work is safe.
      if (!opts.silent || !lastShownSilentError) {
        toast.error(`save failed: ${msg}`);
        lastShownSilentError = true;
      }
      return false;
    } finally {
      saving = false;
    }
  }
  let lastShownSilentError = $state(false);
  // Reset the "silent error already shown" gate when the user manages a
  // successful interaction (so a second outage shows a toast again).
  $effect(() => {
    if (!saveFailed) lastShownSilentError = false;
  });

  let prev = $state('');
  $effect(() => {
    if (body !== prev) {
      dirty = true;
      prev = body;
    }
  });

  // Auto-save: debounce 2s after last edit. If save fails, the next edit
  // re-triggers the timer so we keep retrying as the user continues typing.
  $effect(() => {
    void body;
    if (!dirty || saving || !note) return;
    const t = setTimeout(() => {
      if (dirty && !saving && note) save({ silent: true });
    }, 2000);
    return () => clearTimeout(t);
  });

  // Persist the body to localStorage on every change (debounced 600ms).
  // Survives tab close, offline reload, and brief power-loss / browser-crash
  // scenarios that auto-save's 2s window doesn't cover.
  $effect(() => {
    void body;
    if (!note || !dirty) return;
    const t = setTimeout(() => {
      if (note) setDraft(note.path, body, note.modTime);
    }, 600);
    return () => clearTimeout(t);
  });

  // When the network comes back, retry any pending save.
  $effect(() => {
    const onOnline = () => {
      if (saveFailed && dirty && !saving) save({ silent: true });
    };
    window.addEventListener('online', onOnline);
    return () => window.removeEventListener('online', onOnline);
  });

  // Best-effort flush on SPA navigation. We can't synchronously block
  // the navigation (await isn't honored by browser navigations), but the
  // draft layer protects against data loss on the worst case — this just
  // tries to push edits over the wire faster than the 2s debounce would.
  beforeNavigate(() => {
    if (dirty && !saving && note) {
      // Body is already in localStorage via setDraft (debounce 600ms).
      // Fire-and-forget the save; it'll race the navigation but either
      // outcome is safe (draft still on disk).
      void save({ silent: true });
    }
  });

  // Save status label that updates with the live tick.
  let saveStatus = $derived.by(() => {
    void nowTick; // keep it reactive
    if (saving) return 'saving…';
    if (saveFailed && dirty) return 'retry?';
    if (dirty) return 'unsaved';
    if (!lastSavedAt) return 'saved';
    const ago = Math.floor((Date.now() - lastSavedAt) / 1000);
    if (ago < 4) return 'saved';
    if (ago < 60) return `saved ${ago}s ago`;
    if (ago < 3600) return `saved ${Math.floor(ago / 60)}m ago`;
    return 'saved';
  });

  async function navigateWikilink(title: string) {
    // Best-effort flush of any pending edit. We never block navigation on
    // the save result — the localStorage draft (setDraft, debounce 600ms)
    // already preserves the body, and beforeNavigate flushes again. If the
    // user is offline, save will fail; the draft is still on disk and gets
    // retried automatically when 'online' fires.
    if (dirty) void save({ silent: true });
    try {
      const list = await api.listNotes({ q: title, limit: 5 });
      const exact = list.notes.find((n) => n.title.toLowerCase() === title.toLowerCase());
      const target = exact ?? list.notes[0];
      if (target) {
        goto(`/notes/${encodeURIComponent(target.path)}`);
        return;
      }
    } catch {}
    goto(`/notes/${encodeURIComponent(title + '.md')}`);
  }

  $effect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (dirty) {
        e.preventDefault();
        e.returnValue = '';
      }
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });

  // Pinned set — refresh on mount + on WS changes.
  onMount(() => {
    loadPinned();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') loadPinned();
    });
  });

  // Live-reload current note from WS, but never clobber unsaved edits.
  onMount(() =>
    onWsEvent((ev) => {
      if (ev.type !== 'note.changed') return;
      if (!note || ev.path !== note.path) return;
      if (dirty || saving) return;
      lastLoadedPath = '';
      load(note.path);
    })
  );

  let wordCount = $derived.by(() => {
    const t = body.trim();
    return t ? t.split(/\s+/).length : 0;
  });

  function jumpToLine(lineNum: number) {
    editor?.scrollToLine(lineNum);
    infoDrawerOpen = false;
  }

  async function saveFrontmatter(next: Record<string, unknown>) {
    if (!note) return;
    try {
      const updated = await api.putNote(note.path, { frontmatter: next, body });
      note = updated;
      prev = body;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  // ----- Daily-note navigation -----
  // A note is "daily" when its basename is YYYY-MM-DD.md OR its frontmatter
  // has type=daily. When daily, expose prev/next-day jumps in the header.
  const dailyDateRe = /(\d{4}-\d{2}-\d{2})\.md$/;
  let dailyDate = $derived.by(() => {
    if (!note) return null;
    const m = note.path.match(dailyDateRe);
    if (m) return m[1];
    if (note.frontmatter && (note.frontmatter as Record<string, unknown>).type === 'daily') {
      const d = (note.frontmatter as Record<string, unknown>).date;
      if (typeof d === 'string') return d.slice(0, 10);
    }
    return null;
  });
  let isDaily = $derived(dailyDate !== null);

  function shiftDate(iso: string, days: number): string {
    const [y, m, d] = iso.split('-').map(Number);
    const dt = new Date(y, m - 1, d);
    dt.setDate(dt.getDate() + days);
    const yy = dt.getFullYear();
    const mm = String(dt.getMonth() + 1).padStart(2, '0');
    const dd = String(dt.getDate()).padStart(2, '0');
    return `${yy}-${mm}-${dd}`;
  }
  async function gotoDaily(date: string) {
    if (dirty) void save({ silent: true });
    try {
      // /api/v1/daily/<date> creates today's note if missing; for past/future
      // dates it just returns the existing note (we won't auto-materialize
      // an empty file for arbitrary historical dates).
      const n = await api.daily(date);
      goto(`/notes/${encodeURIComponent(n.path)}`);
    } catch {
      // If no existing daily for that date, just try the canonical path.
      goto(`/notes/${encodeURIComponent(date + '.md')}`);
    }
  }

  let dailyLabel = $derived.by(() => {
    if (!dailyDate) return '';
    const today = new Date();
    const todayIso = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`;
    const yesterday = shiftDate(todayIso, -1);
    const tomorrow = shiftDate(todayIso, 1);
    if (dailyDate === todayIso) return 'today';
    if (dailyDate === yesterday) return 'yesterday';
    if (dailyDate === tomorrow) return 'tomorrow';
    const [y, m, d] = dailyDate.split('-').map(Number);
    const dt = new Date(y, m - 1, d);
    return dt.toLocaleDateString(undefined, { weekday: 'long' });
  });
</script>

{#snippet treeContent()}
  <div class="px-2 pt-2 pb-1 text-xs uppercase tracking-wider text-dim flex-shrink-0">Vault</div>
  <NotesTree currentPath={note?.path} onSelect={() => (treeDrawerOpen = false)} />
{/snippet}

{#snippet infoContent()}
  <div class="p-3 space-y-4 overflow-y-auto h-full">
    <section>
      <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Outline</h3>
      <Outline body={body} onJump={jumpToLine} />
    </section>
    {#if note}
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Backlinks</h3>
        <BacklinksPanel path={note.path} onNavigate={navigateWikilink} />
      </section>
    {/if}
    {#if note}
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim mb-2">Properties</h3>
        <FrontmatterEditor frontmatter={note.frontmatter ?? {}} onChange={saveFrontmatter} />
      </section>
    {/if}
  </div>
{/snippet}

<div class="h-full flex">
  <!-- Tree (desktop only) -->
  <aside class="hidden lg:flex lg:flex-col lg:w-64 xl:w-72 border-r border-surface1 bg-mantle/40 flex-shrink-0">
    {@render treeContent()}
  </aside>

  <!-- Tree drawer (mobile + tablet) -->
  <Drawer bind:open={treeDrawerOpen} side="left">
    <div class="h-full flex flex-col">
      {@render treeContent()}
    </div>
  </Drawer>

  <!-- Center: editor -->
  <div class="flex-1 flex flex-col min-w-0">
    {#if error && !note}
      <!-- Stuck-on-error escape header. When the load failed and we
           have no note to render, the normal header below is hidden
           too — without this the user has no UI to navigate away
           except a full page reload. Keep it minimal: just a back
           link and the error message. -->
      <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0">
        <a
          href="/notes"
          aria-label="back to notes"
          class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </a>
        <button
          onclick={() => (treeDrawerOpen = true)}
          aria-label="vault tree"
          class="lg:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M3 6h18M3 12h18M3 18h18" stroke-linecap="round" />
          </svg>
        </button>
        <h1 class="text-base font-semibold text-text flex-1 truncate">Couldn't open note</h1>
        <button
          onclick={() => { lastLoadedPath = ''; load(decodeURIComponent($page.params.path ?? '')); }}
          class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded hover:border-primary text-text"
        >Retry</button>
      </header>
      <div class="p-6 text-sm text-error">{error}</div>
    {:else if error}
      <div class="px-4 py-2 text-sm text-error border-b border-error/30 bg-error/10 flex-shrink-0">{error}</div>
    {/if}
    {#if note}
      <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0">
        <a
          href="/notes"
          aria-label="back to notes"
          class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </a>
        <button
          onclick={() => (treeDrawerOpen = true)}
          aria-label="vault tree"
          title="vault tree"
          class="lg:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M3 6h18M3 12h18M3 18h18" stroke-linecap="round" />
          </svg>
        </button>
        {#if isDaily && dailyDate}
          <button
            onclick={() => gotoDaily(shiftDate(dailyDate, -1))}
            aria-label="previous day"
            title="previous day"
            class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
          >‹</button>
        {/if}
        <div class="min-w-0 flex-1">
          <h1 class="text-base sm:text-lg font-semibold text-text truncate">
            {note.title}
            {#if dailyLabel}
              <span class="ml-2 text-xs font-normal text-dim uppercase tracking-wider">{dailyLabel}</span>
            {/if}
          </h1>
          <!-- Folder breadcrumbs — each segment is a clickable filter
               link back into the notes index. Plus tag chips pulled
               from the frontmatter when present. -->
          <div class="text-[11px] text-dim flex items-center gap-1 truncate">
            <a href="/notes" class="hover:text-primary">vault</a>
            {#each note.path.split('/').slice(0, -1) as seg, i}
              <span class="text-dim/60">/</span>
              <a href="/notes?folder={encodeURIComponent(note.path.split('/').slice(0, i + 1).join('/'))}" class="hover:text-primary truncate font-mono">{seg}</a>
            {/each}
            {#if (note.frontmatter as Record<string, unknown>)?.tags && Array.isArray((note.frontmatter as Record<string, unknown>).tags)}
              <span class="ml-2 flex items-center gap-1 flex-wrap">
                {#each ((note.frontmatter as Record<string, unknown>).tags as string[]).slice(0, 6) as t}
                  <a
                    href="/notes?tag={encodeURIComponent(t)}"
                    class="px-1.5 py-0.5 rounded text-[10px] hover:bg-surface1"
                    style="background: color-mix(in srgb, var(--color-secondary) 14%, transparent); color: var(--color-secondary);"
                  >#{t}</a>
                {/each}
              </span>
            {/if}
          </div>
        </div>
        {#if isDaily && dailyDate}
          <button
            onclick={() => gotoDaily(shiftDate(dailyDate, 1))}
            aria-label="next day"
            title="next day"
            class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
          >›</button>
          <button
            onclick={() => gotoDaily('today')}
            aria-label="today"
            title="jump to today"
            class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-primary hidden sm:inline-flex flex-shrink-0"
          >today</button>
        {/if}
        <button
          onclick={togglePin}
          disabled={pinBusy}
          aria-label={pinned.has(note.path) ? 'unpin' : 'pin'}
          title={pinned.has(note.path) ? 'unpin from dashboard' : 'pin to dashboard'}
          class="w-9 h-9 flex items-center justify-center rounded text-lg disabled:opacity-50
            {pinned.has(note.path) ? 'text-warning' : 'text-dim hover:text-warning'}"
        >
          {pinned.has(note.path) ? '★' : '☆'}
        </button>
        <span class="text-xs text-dim hidden sm:inline">{wordCount} words</span>
        <!-- view-mode toggle -->
        <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs">
          {#each [{m: 'edit', l: 'edit', i: '✎'}, {m: 'split', l: 'split', i: '⊟'}, {m: 'preview', l: 'preview', i: '👁'}] as v}
            <button
              onclick={() => setViewMode(v.m as ViewMode)}
              class="px-2.5 py-1.5 {viewMode === v.m ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
              title={v.l}
            >
              <span class="text-[11px]">{v.i}</span>
            </button>
          {/each}
        </div>
        <!-- mobile: 2-mode toggle (edit/preview only) -->
        <button
          onclick={() => setViewMode(viewMode === 'preview' ? 'edit' : 'preview')}
          aria-label={viewMode === 'preview' ? 'edit source' : 'show preview'}
          class="sm:hidden w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-base"
        >
          {viewMode === 'preview' ? '✎' : '👁'}
        </button>
        <button
          onclick={() => save()}
          disabled={(!dirty && !saveFailed) || saving}
          title={saveStatus}
          class="px-3 sm:px-4 py-2 rounded text-sm font-medium disabled:opacity-60
            {saveFailed ? 'bg-error text-mantle' : dirty || saving ? 'bg-primary text-mantle' : 'bg-surface1 text-subtext'}"
        >
          {saveStatus}
        </button>
        <button
          onclick={() => (infoDrawerOpen = true)}
          aria-label="outline & backlinks"
          class="xl:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M4 6h16M4 10h10M4 14h16M4 18h10" stroke-linecap="round" />
          </svg>
        </button>
      </header>
      {#if isDaily && note}
        {@const np = note.path}
        <!-- Carryover (yesterday's open) + habit checklist render
             above the quick-add bar so they're the first thing the
             user sees on the daily. Both collapse to a header line
             when the user wants the editor max-screen. -->
        <DailyContext onChanged={async () => { lastLoadedPath = ''; await load(np); }} />
        <DailyQuickAdd notePath={np} dailyDate={dailyDate} onAdded={async () => { lastLoadedPath = ''; await load(np); }} />
      {/if}
      <!-- Deadline strip — surfaces project/goal-linked deadlines for
           this note. Renders nothing when frontmatter has neither
           field, or none of the deadlines match. -->
      <NoteDeadlinesStrip frontmatter={note.frontmatter ?? null} />
      <div class="flex-1 min-h-0 p-2 sm:p-3">
        {#if viewMode === 'edit'}
          <Editor bind:value={body} bind:this={editor} onSave={save} onNavigate={navigateWikilink} />
        {:else if viewMode === 'preview'}
          <div class="h-full overflow-y-auto bg-surface0 border border-surface1 rounded px-4 sm:px-6 py-4">
            <div class="max-w-3xl mx-auto">
              <MarkdownRenderer body={body} onWikilink={navigateWikilink} />
            </div>
          </div>
        {:else}
          <!-- split (desktop only) -->
          <div class="h-full grid grid-cols-1 lg:grid-cols-2 gap-2">
            <Editor bind:value={body} bind:this={editor} onSave={save} onNavigate={navigateWikilink} />
            <div class="h-full overflow-y-auto bg-surface0 border border-surface1 rounded px-4 sm:px-6 py-4 hidden lg:block">
              <MarkdownRenderer body={body} onWikilink={navigateWikilink} />
            </div>
          </div>
        {/if}
      </div>
      <footer class="md:hidden px-3 py-1.5 border-t border-surface1 text-[11px] text-dim flex items-center justify-between">
        <span>{wordCount} words</span>
        <span class="opacity-60">[[ for autocomplete · ⌘-click links</span>
      </footer>
    {:else}
      <div class="p-6 text-sm text-dim">loading…</div>
    {/if}
  </div>

  <!-- Right info panel (desktop xl+) -->
  <aside class="hidden xl:flex xl:flex-col xl:w-72 border-l border-surface1 bg-mantle/40 flex-shrink-0">
    {@render infoContent()}
  </aside>

  <!-- Info drawer (mobile + tablet + desktop without xl) -->
  <Drawer bind:open={infoDrawerOpen} side="right">
    {@render infoContent()}
  </Drawer>
</div>
