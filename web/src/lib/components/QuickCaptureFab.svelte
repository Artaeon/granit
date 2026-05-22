<script lang="ts">
  import { api, todayISO, type Project } from '$lib/api';
  import { onMount } from 'svelte';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { invalidateTitleCache } from '$lib/editor/wikilinks';
  import { goto } from '$app/navigation';
  import { slugifyTitle } from '$lib/util/slug';
  import VoiceNoteModal from '$lib/components/VoiceNoteModal.svelte';

  // QuickCaptureFab — global capture surface. Single keystroke
  // (Mod-Shift-N) opens a tabbed modal that can capture a task,
  // freeform note, calendar event, or jot from anywhere.
  //
  // Task → today's daily note `## Tasks` section
  // Note → vault note at <folder>/<slug>.md (Notes/ default)
  // Event → events.json (one-off date + optional time)
  // Jot → bullet appended to today's daily note `## Jots` section
  //
  // Mode persists across reopens so the user comes back to the
  // same surface they last used.

  type CaptureMode = 'task' | 'note' | 'event' | 'jot';

  let open = $state(false);
  let voiceOpen = $state(false);
  let mode = $state<CaptureMode>('task');

  // Shared text input — used as the task line, note title, event
  // title, or jot body depending on mode.
  let text = $state('');
  let body = $state('');
  let saving = $state(false);

  // Task fields
  let priority = $state(0);
  let dueDate = $state('');
  let recurrence = $state<'' | 'daily' | 'weekly' | 'monthly' | '3x-week'>('');
  let projectId = $state('');
  let tags = $state(''); // comma separated, applied to task or note

  // Note fields
  let folder = $state('Notes');

  // Event fields
  let eventDate = $state(todayISO());
  let allDay = $state(true);
  let startTime = $state('09:00');
  let endTime = $state('10:00');

  let projects = $state<Project[]>([]);
  let projectsLoaded = $state(false);

  let inputEl: HTMLInputElement | undefined = $state();

  function show() {
    open = true;
    if (!projectsLoaded) void loadProjects();
    queueMicrotask(() => inputEl?.focus());
  }
  function hide() {
    open = false;
    text = '';
    body = '';
    priority = 0;
    dueDate = '';
    recurrence = '';
    projectId = '';
    tags = '';
    folder = 'Notes';
    eventDate = todayISO();
    allDay = true;
    startTime = '09:00';
    endTime = '10:00';
  }

  async function loadProjects() {
    try {
      const res = await api.listProjects();
      projects = res.projects.filter((p) => p.status !== 'archived');
    } catch {
      // best-effort — the picker just stays empty
    } finally {
      projectsLoaded = true;
    }
  }

  onMount(() => {
    function onKey(e: KeyboardEvent) {
      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.shiftKey && (e.key === 'N' || e.key === 'n')) {
        e.preventDefault();
        if (open) hide();
        else show();
      } else if (mod && e.shiftKey && (e.key === 'V' || e.key === 'v')) {
        const target = e.target as HTMLElement | null;
        const inField = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable);
        if (!inField) {
          e.preventDefault();
          voiceOpen = true;
        }
      } else if (open && e.key === 'Escape') {
        hide();
      } else if (open && mod && (e.key === 'Enter' || e.key === 'Return')) {
        // Mod-Enter submits from any field — useful when focus is
        // in the multi-line body textarea.
        e.preventDefault();
        void submit();
      } else if (open && !e.metaKey && !e.ctrlKey && !e.altKey) {
        const target = e.target as HTMLElement | null;
        const inField = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA');
        if (inField) return;
        if (e.key === '1') { mode = 'task'; e.preventDefault(); }
        else if (e.key === '2') { mode = 'note'; e.preventDefault(); }
        else if (e.key === '3') { mode = 'event'; e.preventDefault(); }
        else if (e.key === '4') { mode = 'jot'; e.preventDefault(); }
      }
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  });

  function dailyPath(): string {
    return `Daily/${todayISO()}.md`;
  }

  function parseTags(): string[] {
    return tags
      .split(/[,\s]+/)
      .map((t) => t.trim().replace(/^#/, ''))
      .filter(Boolean);
  }

  async function submit(e?: Event) {
    e?.preventDefault();
    if (!text.trim()) return;
    saving = true;
    try {
      if (mode === 'task') {
        await api.createTask({
          notePath: dailyPath(),
          text: text.trim(),
          priority: priority || undefined,
          dueDate: dueDate || undefined,
          tags: parseTags().length ? parseTags() : undefined,
          section: '## Tasks',
          // Bundled into the create so the new task lands fully-formed
          // in one round-trip; the prior two-step (create + patch)
          // shape produced a brief flicker when WS broadcast the create
          // before the patch arrived.
          projectId: projectId || undefined,
          recurrence: recurrence || undefined
        });
        toast.success('task captured');
        invalidateTitleCache();
      } else if (mode === 'note') {
        const title = text.trim();
        const slug = slugifyTitle(title);
        const cleanFolder = folder.trim().replace(/^\/+|\/+$/g, '') || 'Notes';
        const path = `${cleanFolder}/${slug}.md`;
        const fm: Record<string, unknown> = { title };
        const tagList = parseTags();
        if (tagList.length) fm.tags = tagList;
        await api.createNote({ path, frontmatter: fm, body: body.trim() });
        toast.success('note created');
        invalidateTitleCache();
        hide();
        void goto(`/notes/${encodeURIComponent(path)}`);
        return;
      } else if (mode === 'event') {
        const payload: Record<string, string> = {
          title: text.trim(),
          date: eventDate
        };
        if (!allDay) {
          payload.start_time = startTime;
          payload.end_time = endTime;
        }
        await api.createEvent(payload);
        toast.success('event captured');
      } else if (mode === 'jot') {
        // Append a bullet line to today's daily note's `## Jots`
        // section. The server-side daily handler materialises the
        // section + file as needed.
        const line = `- ${text.trim()}`;
        try {
          const path = dailyPath();
          let existing = '';
          try {
            const note = await api.getNote(path);
            existing = note.body || '';
          } catch {
            existing = '';
          }
          let next: string;
          const jotIdx = existing.search(/^## Jots\b/m);
          if (jotIdx >= 0) {
            const after = existing.slice(jotIdx);
            const headerEnd = after.indexOf('\n');
            const insertAt = jotIdx + (headerEnd >= 0 ? headerEnd + 1 : after.length);
            next = existing.slice(0, insertAt) + line + '\n' + existing.slice(insertAt);
          } else {
            const sep = existing && !existing.endsWith('\n') ? '\n\n' : '\n';
            next = existing + sep + '## Jots\n' + line + '\n';
          }
          await api.putNote(path, { body: next });
          toast.success('jot captured');
        } catch (err) {
          toast.error('jot failed: ' + errorMessage(err));
          return;
        }
      }
      hide();
    } catch (err) {
      toast.error('capture failed: ' + errorMessage(err));
    } finally {
      saving = false;
    }
  }

  const TABS: { id: CaptureMode; label: string; hint: string }[] = [
    { id: 'task', label: 'Task', hint: '1' },
    { id: 'note', label: 'Note', hint: '2' },
    { id: 'event', label: 'Event', hint: '3' },
    { id: 'jot', label: 'Jot', hint: '4' }
  ];

  const PLACEHOLDERS: Record<CaptureMode, string> = {
    task: 'what needs doing?',
    note: 'note title',
    event: 'event title',
    jot: 'quick thought, link, or one-liner'
  };
</script>

<!-- Capture cluster: primary "+" plus a smaller voice satellite.
     Both desktop-only (mobile gets the bottom-nav). -->
<div class="hidden md:flex flex-col items-end gap-2 fixed bottom-5 right-5 z-30">
  <button
    type="button"
    onclick={() => (voiceOpen = true)}
    aria-label="voice note (Ctrl+Shift+V)"
    title="voice note (Ctrl+Shift+V)"
    class="w-10 h-10 flex items-center justify-center rounded-full bg-mantle border border-surface1 text-subtext shadow hover:bg-surface0 hover:text-primary transition-colors"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
      <rect x="9" y="3" width="6" height="12" rx="3"/>
      <path d="M5 11a7 7 0 0014 0M12 18v3" stroke-linecap="round"/>
    </svg>
  </button>
  <button
    type="button"
    onclick={show}
    aria-label="quick capture (Ctrl+Shift+N)"
    title="quick capture (Ctrl+Shift+N)"
    class="w-12 h-12 flex items-center justify-center rounded-full bg-primary text-on-primary shadow-lg hover:opacity-90 transition-colors"
  >
    <svg viewBox="0 0 24 24" class="w-6 h-6" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
      <path d="M12 5v14M5 12h14"/>
    </svg>
  </button>
</div>

<VoiceNoteModal bind:open={voiceOpen} />

{#if open}
  <!-- Backdrop. Solid 60% dark — no blur. -->
  <div
    class="fixed inset-0 z-50 bg-black/60 flex items-end sm:items-start sm:pt-[12vh] justify-center sm:p-4"
    role="dialog"
    aria-label="quick capture"
    onclick={hide}
    onkeydown={(e) => { if (e.key === 'Escape') hide(); }}
    tabindex="-1"
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      class="w-full max-w-xl bg-mantle border border-surface1 rounded-t-lg sm:rounded-lg shadow-2xl overflow-hidden"
      role="document"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
    >
      <!-- Tab strip — mode is the headline UX choice; everything
           else adapts to it. Bold solid pill for the active tab. -->
      <div class="flex items-center border-b border-surface1 bg-base">
        {#each TABS as t (t.id)}
          <button
            type="button"
            onclick={() => { mode = t.id; queueMicrotask(() => inputEl?.focus()); }}
            class="flex-1 px-3 py-2.5 text-sm font-medium transition-colors {mode === t.id ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface0 hover:text-text'}"
            aria-pressed={mode === t.id}
          >
            <span>{t.label}</span>
            <span class="ml-1.5 text-[10px] {mode === t.id ? 'opacity-80' : 'text-dim'}">{t.hint}</span>
          </button>
        {/each}
        <button
          type="button"
          onclick={hide}
          aria-label="close"
          class="px-3 py-2.5 text-dim hover:text-text"
        >×</button>
      </div>

      <form onsubmit={submit} class="p-4 space-y-3">
        <input
          bind:this={inputEl}
          bind:value={text}
          required
          placeholder={PLACEHOLDERS[mode]}
          class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary"
        />

        {#if mode === 'note'}
          <textarea
            bind:value={body}
            placeholder="optional body — markdown supported"
            rows="6"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y font-mono leading-relaxed"
          ></textarea>
          <div class="flex flex-wrap items-center gap-2 text-xs">
            <label class="text-dim text-[11px] flex items-center gap-1.5">
              <span>Folder</span>
              <input
                bind:value={folder}
                placeholder="Notes"
                class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm w-32"
              />
            </label>
            <label class="text-dim text-[11px] flex items-center gap-1.5 flex-1 min-w-[10rem]">
              <span>Tags</span>
              <input
                bind:value={tags}
                placeholder="tag1, tag2"
                class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm flex-1"
              />
            </label>
          </div>
        {:else if mode === 'task'}
          <div class="grid grid-cols-2 sm:grid-cols-3 gap-2 text-xs">
            <select
              bind:value={priority}
              class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm"
              aria-label="priority"
            >
              <option value={0}>no priority</option>
              <option value={1}>!1 high</option>
              <option value={2}>!2 medium</option>
              <option value={3}>!3 low</option>
            </select>
            <input
              type="date"
              bind:value={dueDate}
              class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm"
              aria-label="due date"
            />
            <select
              bind:value={recurrence}
              class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm"
              aria-label="recurrence"
            >
              <option value="">no repeat</option>
              <option value="daily">daily</option>
              <option value="weekly">weekly</option>
              <option value="monthly">monthly</option>
              <option value="3x-week">3×/week</option>
            </select>
            <select
              bind:value={projectId}
              class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm col-span-2 sm:col-span-1"
              aria-label="project"
            >
              <option value="">no project</option>
              {#each projects as p (p.name)}
                <option value={p.name}>{p.name}</option>
              {/each}
            </select>
            <input
              bind:value={tags}
              placeholder="#tag1 #tag2"
              class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm col-span-2"
              aria-label="tags"
            />
          </div>
        {:else if mode === 'event'}
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs items-center">
            <input
              type="date"
              bind:value={eventDate}
              class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm col-span-2"
              aria-label="event date"
            />
            <label class="flex items-center gap-1.5 text-subtext col-span-2 sm:col-span-1">
              <input type="checkbox" bind:checked={allDay} class="w-3.5 h-3.5 accent-primary"/>
              <span>all-day</span>
            </label>
            {#if !allDay}
              <input
                type="time"
                bind:value={startTime}
                class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm"
                aria-label="start time"
              />
              <input
                type="time"
                bind:value={endTime}
                class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-text text-sm"
                aria-label="end time"
              />
            {/if}
          </div>
        {/if}

        <div class="flex items-center gap-2 pt-1">
          <span class="text-[11px] text-dim flex-1">
            {#if mode === 'task'}
              Saved to today's daily note
            {:else if mode === 'note'}
              New note in vault
            {:else if mode === 'event'}
              Added to calendar
            {:else}
              Appended to daily Jots
            {/if}
            <kbd class="ml-1 text-[10px] font-mono px-1.5 py-0.5 bg-surface0 border border-surface1 rounded">Ctrl+Enter</kbd>
          </span>
          <button
            type="button"
            onclick={hide}
            class="px-3 py-1.5 text-sm text-subtext hover:text-text"
          >Cancel</button>
          <button
            type="submit"
            disabled={!text.trim() || saving}
            class="px-4 py-1.5 bg-primary text-on-primary rounded font-medium text-sm disabled:opacity-50 hover:opacity-90 transition-colors"
          >
            {saving ? '…' : 'Capture'}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}
