<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import { api, todayISO, type Note } from '$lib/api';
  import { parseTaskInput } from '$lib/util/taskParse';
  import { slugifyTitle } from '$lib/util/slug';
  import { onLocalMidnight } from '$lib/util/midnightTick';
  import {
    createSpeechRecognition,
    isSpeechRecognitionSupported,
    type SpeechRecognitionLike
  } from '$lib/util/speechRecognition';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { goto } from '$app/navigation';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';

  // Quick capture — "type and forget" surface. Three modes (task /
  // jot / note) share a single input row, smart parsing extracts
  // markers from task text, voice-to-text via the browser's
  // SpeechRecognition API. The whole widget is one row plus an
  // optional parsed-chips strip — no header, no recent-captures
  // section. The recent-captures strip was removed in the 2026-05-23
  // pass after the user reported it as never-used clutter; undo of
  // a wrong capture now happens via the toast (and the underlying
  // edit history of the daily note / inbox).
  //
  // Data shapes per mode:
  //   task → api.createTask in today's daily note's `## Tasks`
  //   jot  → put back today's daily note with the line appended under
  //          `## Jots` (created if missing).
  //   note → api.createNote at Inbox/YYYY-MM-DD-<slug>.md, then jump
  //          straight into the editor for elaboration.

  type Mode = 'task' | 'jot' | 'note';
  const MODES: { id: Mode; label: string; glyph: string; placeholder: string; tone: string }[] = [
    { id: 'task', label: 'Task', glyph: '◉', placeholder: 'task — try "review PR !1 morgen #work"', tone: 'text-primary' },
    { id: 'jot', label: 'Jot', glyph: '✎', placeholder: 'jot — a thought to land in today\'s daily note', tone: 'text-secondary' },
    { id: 'note', label: 'Note', glyph: '◇', placeholder: 'note title — opens the editor for the body', tone: 'text-warning' }
  ];

  let mode = $state<Mode>('task');
  let raw = $state('');
  let busy = $state(false);
  let inputEl: HTMLInputElement | undefined = $state();
  let daily = $state<Note | null>(null);
  let dailyError = $state(false);

  // Persist last-used mode so the user's preferred default sticks.
  const MODE_KEY = 'granit.dashboard.qc.mode';
  let stopMidnight: (() => void) | null = null;
  onMount(() => {
    const v = loadStoredString(MODE_KEY, '');
    if (v === 'task' || v === 'jot' || v === 'note') mode = v;
    void loadDaily();
    voiceSupported = isSpeechRecognitionSupported();
    // Re-pull the daily reference at midnight so the next capture
    // goes to the new day's daily note. Otherwise a dashboard left
    // open overnight would dump tomorrow morning's first task into
    // yesterday's note.
    stopMidnight = onLocalMidnight(() => { void loadDaily(); });
  });
  onDestroy(() => { if (stopMidnight) stopMidnight(); });
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
      toast.error('failed: ' + errorMessage(err));
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
    await api.createTask({
      notePath: daily.path,
      text: p.text,
      priority: p.priority || undefined,
      dueDate: p.dueDate || undefined,
      tags: p.tags.length ? p.tags : undefined,
      section: '## Tasks'
    });
    // Toast carries the parsed result so the user can sanity-check
    // that smart parsing matched their intent — replaces the recent-
    // captures strip's role of "what did I just save?".
    const bits = [
      p.text.length > 40 ? p.text.slice(0, 40) + '…' : p.text,
      p.priority > 0 ? `P${p.priority}` : null,
      p.dueDate ? `📅 ${p.dueDate}` : null,
      ...p.tags.map((t) => `#${t}`)
    ].filter(Boolean);
    toast.success('task added — ' + bits.join(' · '));
  }

  async function captureJot(text: string) {
    if (!daily) {
      toast.error('today\'s daily note isn\'t loaded yet — try again in a moment');
      return;
    }
    // Append `- HH:MM <text>` under `## Jots` (create the section if
    // missing). One getNote + putNote — keeps jots in the same daily
    // note as everything else.
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

  let activeMode = $derived(MODES.find((m) => m.id === mode)!);
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-2.5">
  <form
    onsubmit={submit}
    class="flex items-center gap-1.5 bg-mantle border border-surface1 rounded-lg pl-1 pr-1.5 py-1 focus-within:border-primary transition-colors"
  >
    <!-- Mode segmented control — compact. The active mode shows
         glyph + label; inactive ones show just the glyph. Tab cycles
         when the input is empty. -->
    <div class="flex bg-surface0 rounded text-[11px] overflow-hidden flex-shrink-0">
      {#each MODES as m (m.id)}
        <button
          type="button"
          onclick={() => { mode = m.id; tick().then(() => inputEl?.focus()); }}
          class="inline-flex items-center gap-1 transition-colors leading-none
            {mode === m.id ? 'bg-primary text-on-primary px-2 py-1' : 'text-dim hover:text-text hover:bg-surface1 px-1.5 py-1'}"
          title={`Switch to ${m.label} mode (Tab)`}
          aria-pressed={mode === m.id}
          aria-label={`${m.label} mode`}
        >
          <span aria-hidden="true">{m.glyph}</span>
          {#if mode === m.id}
            <span class="font-medium">{m.label}</span>
          {/if}
        </button>
      {/each}
    </div>

    <input
      bind:this={inputEl}
      bind:value={raw}
      onkeydown={onKey}
      placeholder={activeMode.placeholder}
      disabled={busy}
      class="flex-1 min-w-0 bg-transparent text-sm sm:text-[15px] text-text placeholder-dim focus:outline-none px-1"
    />

    <!-- Voice toggle. Hidden when not supported. -->
    {#if voiceSupported}
      <button
        type="button"
        onclick={toggleVoice}
        title={recording ? 'Stop dictation' : 'Dictate'}
        aria-label={recording ? 'Stop dictation' : 'Start dictation'}
        class="w-7 h-7 inline-flex items-center justify-center rounded {recording ? 'bg-surface0 text-error' : 'text-dim hover:text-text hover:bg-surface1'} flex-shrink-0"
      >
        {#if recording}
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 animate-pulse" fill="currentColor"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>
        {:else}
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M12 1.5a3 3 0 0 0-3 3v6a3 3 0 0 0 6 0v-6a3 3 0 0 0-3-3zM5 10v1a7 7 0 0 0 14 0v-1M12 18v3"/>
          </svg>
        {/if}
      </button>
    {/if}

    <button
      type="submit"
      disabled={busy || !raw.trim() || dailyError}
      aria-label="Submit capture"
      class="px-2.5 py-1 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50 flex-shrink-0 inline-flex items-center gap-1"
    >
      {busy ? '…' : 'add'}
      <kbd class="hidden sm:inline text-[10px] opacity-70">↵</kbd>
    </button>
  </form>

  <!-- Daily-note load failure inline notice. Without this, the submit
       button silently disables and the user has no idea why their
       capture isn't accepting input. The retry pulls the note again;
       most failures here are transient (server warming up, ws
       reconnecting). -->
  {#if dailyError}
    <div class="flex items-center gap-2 mt-1.5 text-[11px] text-warning">
      <span>Today's daily note didn't load — capture is paused.</span>
      <button
        type="button"
        onclick={loadDaily}
        class="text-secondary hover:underline"
      >Retry</button>
    </div>
  {/if}

  <!-- Parsed-chips strip. Renders only when the task parser actually
       extracted something — empty input or jot/note mode collapses
       the widget to a single-row footprint. -->
  {#if mode === 'task' && hasMarkers && parsedTask}
    <div class="flex flex-wrap items-center gap-1 mt-1.5 text-[11px]">
      <span class="text-dim uppercase tracking-wider mr-1">parsed</span>
      {#if parsedTask.priority > 0}
        {@const pc = parsedTask.priority === 1
          ? 'text-error border-error'
          : parsedTask.priority === 2
            ? 'text-warning border-warning'
            : 'text-info border-info'}
        <span class="px-1.5 py-0.5 rounded border bg-surface0 {pc}">P{parsedTask.priority}</span>
      {/if}
      {#if parsedTask.dueDate}
        <span class="px-1.5 py-0.5 rounded bg-surface1 text-secondary">📅 {parsedTask.dueDate}</span>
      {/if}
      {#each parsedTask.tags as t}
        <span class="px-1.5 py-0.5 rounded bg-surface1 text-accent">#{t}</span>
      {/each}
    </div>
  {/if}
</section>
