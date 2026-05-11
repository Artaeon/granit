<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import { api, todayISO, type Note } from '$lib/api';
  import { parseTaskInput } from '$lib/util/taskParse';
  import { slugifyTitle } from '$lib/util/slug';
  import {
    createSpeechRecognition,
    isSpeechRecognitionSupported,
    type SpeechRecognitionLike
  } from '$lib/util/speechRecognition';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { goto } from '$app/navigation';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';

  // Quick capture is the dashboard's "type and forget" surface — three
  // modes (task / jot / note) sharing a single input row, smart
  // parsing on tasks, voice-to-text via the browser's
  // SpeechRecognition API, and a recent-captures strip with one-click
  // undo. The entire widget is keyboard-driven: Tab cycles modes,
  // Enter submits, Esc clears.
  //
  // Data shapes per mode:
  //   task → api.createTask in today's daily note's `## Tasks`
  //   jot  → put back today's daily note with the line appended under
  //          `## Jots` (created if missing). Single round-trip via
  //          getNote + putNote.
  //   note → api.createNote at Inbox/YYYY-MM-DD-<slug>.md, then jump
  //          straight into the editor for elaboration.

  type Mode = 'task' | 'jot' | 'note';
  const MODES: { id: Mode; label: string; glyph: string; placeholder: string; tone: string }[] = [
    { id: 'task', label: 'Task', glyph: '◉', placeholder: '+ task — try "review PR !1 due:2026-05-15 #work"', tone: 'text-primary' },
    { id: 'jot', label: 'Jot', glyph: '✎', placeholder: '+ jot — a thought, observation, or quick note', tone: 'text-secondary' },
    { id: 'note', label: 'Note', glyph: '◇', placeholder: '+ note title — opens the editor for the body', tone: 'text-warning' }
  ];

  let mode = $state<Mode>('task');
  let raw = $state('');
  let busy = $state(false);
  let inputEl: HTMLInputElement | undefined = $state();
  let daily = $state<Note | null>(null);
  let dailyError = $state(false);

  // Persist last-used mode so the user's preferred default sticks.
  const MODE_KEY = 'granit.dashboard.qc.mode';
  onMount(() => {
    const v = loadStoredString(MODE_KEY, '');
    if (v === 'task' || v === 'jot' || v === 'note') mode = v;
    void loadDaily();
  });
  $effect(() => saveStoredString(MODE_KEY, mode));

  async function loadDaily() {
    try {
      daily = await api.daily('today');
      dailyError = false;
    } catch {
      dailyError = true;
    }
  }

  // Smart parsing only applies to task mode — jots and notes take the
  // raw text. Showing the parsed chips on jot/note would be visual
  // noise the user can't act on.
  let parsedTask = $derived(mode === 'task' ? parseTaskInput(raw) : null);
  let hasMarkers = $derived(
    parsedTask !== null && (parsedTask.priority > 0 || parsedTask.dueDate !== '' || parsedTask.tags.length > 0)
  );

  // ── Recent captures strip ─────────────────────────────────────────
  // In-memory only; cleared on tab close. The 'undo' link reverses the
  // capture (delete task / strip jot line / delete note). Capped at 3
  // so the strip stays glanceable.
  type Recent = {
    id: string; // synthetic; for keyed iteration
    label: string;
    mode: Mode;
    /** Reverse the capture. Each mode has its own undo path. */
    undo: () => Promise<void>;
    capturedAt: number;
  };
  let recents = $state<Recent[]>([]);

  function pushRecent(r: Omit<Recent, 'id' | 'capturedAt'>) {
    recents = [{ ...r, id: crypto.randomUUID().slice(0, 8), capturedAt: Date.now() }, ...recents].slice(0, 3);
  }

  async function runUndo(r: Recent) {
    try {
      await r.undo();
      recents = recents.filter((x) => x.id !== r.id);
      toast.success(`undone — ${r.label}`);
    } catch (e) {
      toast.error('couldn\'t undo: ' + (errorMessage(e)));
    }
  }

  // ── Submit per mode ───────────────────────────────────────────────
  async function submit(e?: Event) {
    e?.preventDefault();
    const text = raw.trim();
    if (!text || busy) return;
    busy = true;
    try {
      if (mode === 'task') {
        await captureTask(text);
      } else if (mode === 'jot') {
        await captureJot(text);
      } else {
        await captureNote(text);
      }
      raw = '';
      await tick();
      inputEl?.focus();
    } catch (err) {
      toast.error('failed: ' + (errorMessage(err)));
    } finally {
      busy = false;
    }
  }

  async function captureTask(text: string) {
    if (!daily) {
      toast.error('today\'s daily note isn\'t loaded yet — try again in a moment');
      return;
    }
    const p = parseTaskInput(text);
    if (!p.text) return;
    const created = await api.createTask({
      notePath: daily.path,
      text: p.text,
      priority: p.priority || undefined,
      dueDate: p.dueDate || undefined,
      tags: p.tags.length ? p.tags : undefined,
      section: '## Tasks'
    });
    pushRecent({
      label: p.text.length > 40 ? p.text.slice(0, 40) + '…' : p.text,
      mode: 'task',
      undo: async () => { await api.deleteTask(created.id); }
    });
    toast.success('task added');
  }

  async function captureJot(text: string) {
    if (!daily) {
      toast.error('today\'s daily note isn\'t loaded yet — try again in a moment');
      return;
    }
    // Append `- HH:MM <text>` under `## Jots` (create the section if
    // missing). One getNote + putNote — simpler than racing a
    // dedicated endpoint, and keeps jots in the same daily note as
    // everything else.
    const fresh = await api.getNote(daily.path);
    const body = fresh.body ?? '';
    const time = new Date().toTimeString().slice(0, 5);
    const line = `- ${time} ${text}`;
    let next: string;
    if (/^##\s+Jots\b/m.test(body)) {
      next = body.replace(/(^##\s+Jots\b[^\n]*\n)/m, `$1${line}\n`);
    } else {
      const sep = body.endsWith('\n') ? '' : '\n';
      next = body + sep + `\n## Jots\n${line}\n`;
    }
    await api.putNote(fresh.path, { frontmatter: fresh.frontmatter ?? {}, body: next });
    pushRecent({
      label: text.length > 40 ? text.slice(0, 40) + '…' : text,
      mode: 'jot',
      undo: async () => {
        // Remove the exact line we just added. Re-fetch so we don't
        // clobber concurrent edits.
        const cur = await api.getNote(daily!.path);
        const stripped = (cur.body ?? '').replace(line + '\n', '');
        await api.putNote(cur.path, { frontmatter: cur.frontmatter ?? {}, body: stripped });
      }
    });
    daily = fresh; // keep our copy current for the next capture
    toast.success('jot added');
  }

  async function captureNote(title: string) {
    const today = todayISO();
    const path = `Inbox/${today}-${slugifyTitle(title) || 'untitled'}.md`;
    const created = await api.createNote({
      path,
      frontmatter: { title, created: new Date().toISOString() },
      body: ''
    });
    pushRecent({
      label: title.length > 40 ? title.slice(0, 40) + '…' : title,
      mode: 'note',
      undo: async () => { await api.deleteNote(created.path); }
    });
    toast.success('note created');
    void goto(`/notes/${encodeURIComponent(created.path)}`);
  }

  // ── Voice input ───────────────────────────────────────────────────
  // Browser SpeechRecognition via the typed wrapper from
  // $lib/util/speechRecognition. Transcript appends to whatever's
  // already in the input so the user can dictate, edit, then submit.
  let recording = $state(false);
  let voiceSupported = $state(false);
  let recognition: SpeechRecognitionLike | null = null;
  let voiceBaseline = '';

  onMount(() => {
    voiceSupported = isSpeechRecognitionSupported();
  });

  function startVoice() {
    const r = createSpeechRecognition();
    if (!r) return;
    voiceBaseline = raw.endsWith(' ') || raw.length === 0 ? raw : raw + ' ';
    recognition = r;
    r.continuous = true;
    r.interimResults = true;
    r.lang = navigator.language || 'en-US';
    r.onresult = (e) => {
      let interim = '';
      let final = '';
      for (let i = e.resultIndex; i < e.results.length; i++) {
        const result = e.results[i];
        if (result[0]) {
          if (result.isFinal) final += result[0].transcript;
          else interim += result[0].transcript;
        }
      }
      if (final) voiceBaseline += final;
      raw = (voiceBaseline + interim).replace(/\s+/g, ' ').trimStart();
    };
    r.onerror = () => stopVoice();
    r.onend = () => { recording = false; };
    r.start();
    recording = true;
  }
  function stopVoice() {
    try { recognition?.stop(); } catch {}
    recognition = null;
    recording = false;
  }
  function toggleVoice() {
    if (recording) stopVoice(); else startVoice();
  }
  onDestroy(stopVoice);

  // ── Keyboard ──────────────────────────────────────────────────────
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Tab' && !e.shiftKey) {
      // Cycle mode; only when the input is empty so a tab inside text
      // can still indent / behave normally.
      if (raw === '') {
        e.preventDefault();
        const idx = MODES.findIndex((m) => m.id === mode);
        mode = MODES[(idx + 1) % MODES.length].id;
      }
    } else if (e.key === 'Escape') {
      raw = '';
      stopVoice();
    }
  }

  function fmtAgo(t: number): string {
    const s = Math.max(0, Math.floor((Date.now() - t) / 1000));
    if (s < 60) return `${s}s ago`;
    const m = Math.floor(s / 60);
    if (m < 60) return `${m}m ago`;
    return `${Math.floor(m / 60)}h ago`;
  }

  let activeMode = $derived(MODES.find((m) => m.id === mode)!);
  // Tick the recents strip's relative timestamps every 30s without
  // doing anything else expensive.
  let _now = $state(Date.now());
  let agoTick: ReturnType<typeof setInterval> | null = null;
  onMount(() => {
    agoTick = setInterval(() => { _now = Date.now(); }, 30_000);
  });
  onDestroy(() => { if (agoTick) clearInterval(agoTick); });
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <header class="flex items-center gap-2 mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Quick capture</h2>
    <span class="flex-1"></span>
    <!-- Mode strip — three pills, single tap to switch. Tab also
         cycles when the input is empty. The active mode picks up its
         tone class from MODES so the user can ambient-read which
         mode is selected without reading the label. -->
    <div class="flex bg-mantle border border-surface1 rounded text-[11px] overflow-hidden">
      {#each MODES as m (m.id)}
        <button
          type="button"
          onclick={() => { mode = m.id; tick().then(() => inputEl?.focus()); }}
          class="px-2 py-1 inline-flex items-center gap-1 transition-colors
            {mode === m.id ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          title={`Switch to ${m.label} mode`}
        >
          <span aria-hidden="true">{m.glyph}</span>
          <span>{m.label}</span>
        </button>
      {/each}
    </div>
  </header>

  <form onsubmit={submit} class="flex items-center gap-2 bg-mantle border border-surface1 rounded-lg px-3 py-2 focus-within:border-primary transition-colors">
    <span class="text-base leading-none flex-shrink-0 {activeMode.tone}" aria-hidden="true">{activeMode.glyph}</span>
    <input
      bind:this={inputEl}
      bind:value={raw}
      onkeydown={onKey}
      placeholder={activeMode.placeholder}
      disabled={busy}
      class="flex-1 min-w-0 bg-transparent text-sm sm:text-[15px] text-text placeholder-dim focus:outline-none"
    />

    <!-- Voice toggle. Hidden when not supported (e.g. Firefox) so
         the button doesn't read as broken. -->
    {#if voiceSupported}
      <button
        type="button"
        onclick={toggleVoice}
        title={recording ? 'Stop dictation' : 'Dictate'}
        class="w-8 h-8 inline-flex items-center justify-center rounded {recording ? 'bg-surface0 text-error' : 'text-dim hover:text-text hover:bg-surface1'} flex-shrink-0"
      >
        {#if recording}
          <svg viewBox="0 0 24 24" class="w-4 h-4 animate-pulse" fill="currentColor"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>
        {:else}
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M12 1.5a3 3 0 0 0-3 3v6a3 3 0 0 0 6 0v-6a3 3 0 0 0-3-3zM5 10v1a7 7 0 0 0 14 0v-1M12 18v3"/>
          </svg>
        {/if}
      </button>
    {/if}

    <button
      type="submit"
      disabled={busy || !raw.trim() || dailyError}
      class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50 flex-shrink-0 inline-flex items-center gap-1"
    >
      {busy ? '…' : 'add'}
      <kbd class="hidden sm:inline text-[10px] opacity-70">↵</kbd>
    </button>
  </form>

  <!-- Smart-parse chips (tasks only). -->
  {#if mode === 'task' && hasMarkers && parsedTask}
    <div class="flex flex-wrap items-center gap-1.5 mt-2 text-xs">
      <span class="text-dim text-[10px] uppercase tracking-wider">parsed</span>
      {#if parsedTask.priority > 0}
        {@const pc = parsedTask.priority === 1
          ? 'bg-surface0 text-error border-error'
          : parsedTask.priority === 2
            ? 'bg-surface0 text-warning border-warning'
            : 'bg-surface0 text-info border-info'}
        <span class="px-2 py-0.5 rounded border {pc}">P{parsedTask.priority}</span>
      {/if}
      {#if parsedTask.dueDate}
        <span class="px-2 py-0.5 rounded bg-surface1 text-secondary">📅 {parsedTask.dueDate}</span>
      {/if}
      {#each parsedTask.tags as t}
        <span class="px-2 py-0.5 rounded bg-surface1 text-accent">#{t}</span>
      {/each}
    </div>
  {/if}

  <!-- Hint row — only on empty input, calmer than always-on. -->
  {#if !raw.trim() && !hasMarkers}
    <p class="mt-2 text-[11px] text-dim">
      {#if mode === 'task'}
        <code class="text-error">!1</code>/<code class="text-warning">!2</code>/<code class="text-info">!3</code> priority ·
        <code class="text-secondary">due:YYYY-MM-DD</code> ·
        <code class="text-accent">#tag</code>
      {:else if mode === 'jot'}
        Lands in today's daily note under <code>## Jots</code> with the time stamped.
      {:else}
        Creates <code>Inbox/{todayISO()}-…md</code> and opens it.
      {/if}
      <span class="opacity-60"> · Tab cycles modes</span>
    </p>
  {/if}

  <!-- Recent captures strip. Only renders when there's something to
       show; otherwise the widget stays compact. Each row links to the
       captured item where applicable, plus a tiny "undo" chip. -->
  {#if recents.length > 0}
    {void _now}
    <div class="mt-3 pt-3 border-t border-surface1 space-y-1">
      {#each recents as r (r.id)}
        {@const m = MODES.find((x) => x.id === r.mode)!}
        <div class="flex items-baseline gap-2 text-xs">
          <span class="flex-shrink-0 {m.tone}" aria-hidden="true">{m.glyph}</span>
          <span class="flex-1 text-subtext truncate">{r.label}</span>
          <span class="text-[10px] text-dim flex-shrink-0">{fmtAgo(r.capturedAt)}</span>
          <button
            type="button"
            onclick={() => void runUndo(r)}
            class="text-[10px] text-dim hover:text-warning underline-offset-2 hover:underline"
            title="Undo this capture"
          >undo</button>
        </div>
      {/each}
    </div>
  {/if}
</section>
