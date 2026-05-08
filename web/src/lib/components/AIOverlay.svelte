<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { page } from '$app/stores';
  import { api, type ChatMessage } from '$lib/api';
  import { sabbath } from '$lib/stores/sabbath';
  import { aiOverlayOpen } from '$lib/stores/ai-overlay';
  import { toast } from '$lib/components/toast';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import { AGENT_MODES, findMode, loadModeId, persistModeId } from '$lib/ai/agents';

  // AIOverlay — global AI panel. Slides in from the right on
  // desktop, becomes a bottom sheet on mobile. Triggered with
  // Mod+J from anywhere (and Esc to close). The body is split
  // into two modes:
  //   1. Quick actions  — four buttons that dispatch the existing
  //      Tier 1 features (briefing / triage / deadlines /
  //      synopsis). Result renders inline as markdown or a JSON
  //      block.
  //   2. Chat — a streaming conversation against the configured
  //      LLM via /api/v1/chat/stream. History is in-memory only
  //      so the overlay stays disposable; the dedicated /chat page
  //      is the place for long-running threads + saving.
  //
  // The component listens for its own keyboard shortcut so the
  // layout doesn't have to know it exists — drop a single
  // <AIOverlay /> in +layout.svelte and you're done.

  // open is a $derived view of the global store so any UI surface
  // (sidebar button, command palette, future mobile entry) can flip
  // the overlay without prop-drilling. We write back via store
  // setters when the user closes / Mod+J-toggles, keeping the
  // store as the single source of truth.
  const open = $derived($aiOverlayOpen);
  let panelEl: HTMLDivElement | undefined = $state();
  let inputEl: HTMLTextAreaElement | undefined = $state();
  let scrollEl: HTMLDivElement | undefined = $state();

  let busy = $state(false);
  let abort: AbortController | null = null;

  // Status pill — what model the chat / actions will route to.
  let statusInfo = $state<{ provider: string; model: string; sabbath: boolean } | null>(null);

  // Quick-action result. Cleared every time the user fires a new
  // action OR sends a chat message (chat takes over the body).
  let quickTitle = $state('');
  let quickResult = $state('');

  // Chat history — persisted to sessionStorage so closing the
  // overlay (Esc / outside-click / Mod+J) doesn't lose the
  // thread. Survives navigation within the tab; cleared on tab
  // close or explicit reset. The full /chat page is still the
  // place for save-as-note and long-running multi-day threads;
  // this layer keeps a quick question alive long enough to come
  // back to it after a tangent.
  const HISTORY_KEY = 'granit.ai.overlay.messages';
  function loadHistory(): ChatMessage[] {
    if (typeof sessionStorage === 'undefined') return [];
    try {
      const raw = sessionStorage.getItem(HISTORY_KEY);
      if (!raw) return [];
      const parsed = JSON.parse(raw);
      if (!Array.isArray(parsed)) return [];
      return parsed.filter(
        (m): m is ChatMessage =>
          m && typeof m === 'object' && typeof m.role === 'string' && typeof m.content === 'string'
      );
    } catch {
      return [];
    }
  }
  function persistHistory(list: ChatMessage[]) {
    if (typeof sessionStorage === 'undefined') return;
    try {
      // Cap to ~30 messages to keep sessionStorage tidy. Older
      // turns drop quietly; the user is unlikely to want a
      // 100-turn quick-overlay thread (that's what /chat is for).
      const trimmed = list.length > 30 ? list.slice(-30) : list;
      sessionStorage.setItem(HISTORY_KEY, JSON.stringify(trimmed));
    } catch {}
  }
  let messages = $state<ChatMessage[]>(loadHistory());
  let input = $state('');
  $effect(() => {
    void messages.length;
    persistHistory(messages);
  });

  // ── Page-aware context ──────────────────────────────────────────
  // Two attach modes, mutually exclusive depending on the route:
  //
  //   /notes/<path>  → attachNote  (server expands the note body
  //                    into a system prompt via chatStream's
  //                    notePath param — see handlers_chat.go).
  //   anywhere else  → attachSnapshot  (fetches the Context Engine
  //                    snapshot and prepends a system message
  //                    with today's events + open tasks +
  //                    recent notes + active goals + deadlines).
  //
  // Mutual exclusion keeps the prompt clean: a notes page already
  // has a primary doc; non-note surfaces benefit from the broader
  // "what's going on right now" view that the snapshot provides.
  let attachSnapshot = $state(true);

  // ── Agent modes + RAG ──────────────────────────────────────────
  // Mode = posture (system prompt). RAG = grounding (retrieved
  // notes prepended as context). They're independent: a Writer mode
  // user might want vault retrieval for facts; a Research mode user
  // might want bare LLM if working with a paper they pasted in.
  // The mode picker is the headline UX; RAG is a secondary toggle
  // that defaults from the mode's preference but the user overrides.
  let modeId = $state<string>(loadModeId());
  let mode = $derived(findMode(modeId));
  // Persist mode change + reset RAG default when user picks a new
  // mode, but DON'T reset on every render (that would clobber the
  // user's explicit override). Only seed when the user actively
  // changes mode.
  function selectMode(id: string) {
    if (id === modeId) return;
    modeId = id;
    persistModeId(id);
    rag = findMode(id).ragDefault;
  }
  // Initial seed: read the loaded mode's RAG default. We use the
  // module helper rather than `modeId` (which Svelte's analyzer
  // flags as a non-reactive read) so the warning stays clean. The
  // user's later mode-changes flow through selectMode() above.
  let rag = $state(findMode(loadModeId()).ragDefault);
  let modePickerOpen = $state(false);
  // Cached list of vault notes (path + title) for the RAG retrieval.
  // Loaded lazily on first send when rag=true; refresh on note
  // events. Per-tab — small enough that holding all titles is fine
  // even on 5k-note vaults.
  let ragIndex = $state<{ path: string; title: string; modTime: string }[]>([]);
  let ragIndexLoaded = $state(false);
  // Last retrieval result for transparency: 'AI saw notes A, B, C'.
  // Cleared on every send so the user sees fresh attribution per
  // turn rather than stale.
  type RagHit = { path: string; title: string; excerpt: string; score: number };
  let lastRagHits = $state<RagHit[]>([]);
  let snapshotLoading = $state(false);
  // Use unknown so we don't lock the consumer into the snapshot
  // shape — the backend evolves it independently.
  let snapshotData = $state<unknown>(null);

  async function loadSnapshot() {
    if (snapshotLoading) return;
    snapshotLoading = true;
    try {
      const r = await api.getAISnapshot();
      snapshotData = r.snapshot ?? null;
    } catch {
      snapshotData = null;
    } finally {
      snapshotLoading = false;
    }
  }

  // Note-aware chat. When the overlay opens on a /notes/<path>
  // page, we offer to attach that note as context to the chat
  // request (chatStream's notePath parameter — server expands it
  // into the system prompt). Default ON when opening on a note
  // page; once opened the user owns the toggle, so manual changes
  // stick. We deliberately don't drive this from a $effect because
  // attachNote being a dependency of its own auto-enable causes
  // the toggle to immediately re-enable when the user un-checks it
  // (the effect re-fires the moment attachNote flips false). The
  // open-transition is the right moment to make the call.
  let attachNote = $state(false);
  // $derived view of the current path so the chip + outgoing
  // chatStream call always reflect the page the user is on, even
  // if they navigate while the overlay is open.
  const currentNotePath = $derived.by(() => {
    const p = $page.url.pathname;
    if (!p.startsWith('/notes/')) return '';
    return decodeURIComponent(p.slice('/notes/'.length));
  });

  function close() {
    abort?.abort();
    if (recording) stopVoice();
    aiOverlayOpen.set(false);
  }
  function toggle() {
    aiOverlayOpen.update((v) => !v);
    // The $effect below handles focus + status + note-attach on
    // open-transitions, so no duplication here.
  }
  // Also handle external opens (sidebar button, etc.) — load
  // status + focus input when the store flips us to true. Two
  // tracking rules apply here:
  //
  //   - DON'T read attachNote: doing so would put it in the
  //     effect's deps and the user un-checking it would re-fire
  //     this effect, which would re-enable it (regression of the
  //     earlier flicker bug). Just write unconditionally.
  //
  //   - DON'T track currentNotePath either: navigating while the
  //     overlay is open would re-fire this effect and yank focus
  //     into the chat input, even if the user was typing in the
  //     destination page. untrack reads the path without
  //     subscribing, so the effect only re-fires on open changes.
  $effect(() => {
    if (open) {
      untrack(() => {
        // On note pages, prefer attachNote (the page has a
        // primary doc the AI should anchor to). Elsewhere,
        // pre-fetch the vault snapshot so the chat can route
        // through general "what's going on" context. Both
        // toggles can be flipped by the user once open.
        if (currentNotePath) {
          attachNote = true;
        } else if (attachSnapshot && !snapshotData) {
          void loadSnapshot();
        }
      });
      void loadStatus();
      tick().then(() => inputEl?.focus());
    }
  });

  // Global Mod+J shortcut + Esc to close. Fires from anywhere
  // including inside text inputs / contentEditable editors —
  // "ask AI about the note I'm currently writing" is the killer
  // use case, so we deliberately steal the keystroke from
  // editors. Mod+J has no strong default in inputs (browsers use
  // it for downloads, which we override the same way Mod+P
  // overrides print).
  function onKey(e: KeyboardEvent) {
    if (open && e.key === 'Escape') {
      e.preventDefault();
      close();
      return;
    }
    if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'j') {
      e.preventDefault();
      toggle();
    }
  }

  onMount(() => {
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  async function loadStatus() {
    try {
      const s = await api.getAIStatus();
      statusInfo = {
        provider: s.global_provider,
        model: s.global_model,
        sabbath: !!s.sabbath_active
      };
    } catch {
      statusInfo = null;
    }
  }

  // ── Quick actions ──────────────────────────────────────────────
  // Each one: cancel any in-flight call, fire the API, render
  // markdown (briefing / synopsis) or a JSON block of proposals
  // (triage / deadlines). Proposals are NOT applied from here —
  // the dedicated tasks page is the place for that flow because
  // it has the full task context. The overlay just shows the
  // model's suggestions so the user can decide whether to navigate
  // there. Keeps the overlay simple.
  async function runBriefing() {
    await runQuick('Daily briefing', async (s) => {
      const r = await api.aiDailyBriefing(s);
      return r.markdown;
    });
  }
  async function runSynopsis() {
    await runQuick('Weekly synopsis', async (s) => {
      const r = await api.aiWeeklyReview(s);
      return r.markdown;
    });
  }
  async function runTriage() {
    await runQuick('Inbox triage', async (s) => {
      const r = await api.aiInboxTriage(s);
      const props = r.proposals ?? [];
      if (props.length === 0) return '_No untriaged tasks to review._';
      const lines = props.map(
        (p) =>
          `- **${p.priority === 0 ? 'drop' : `P${p.priority}`}** · ${p.schedule} · ${p.rationale} _(${p.id})_`
      );
      return `${lines.length} suggestion${lines.length === 1 ? '' : 's'} — open /tasks → inbox to apply:\n\n${lines.join('\n')}`;
    });
  }
  async function runDeadlines() {
    await runQuick('Detect deadlines', async (s) => {
      const r = await api.aiDeadlineDetect(s);
      const props = r.proposals ?? [];
      if (props.length === 0) return '_No clear deadlines detected._';
      const lines = props.map((p) => `- **${p.due_date}** · ${p.rationale} _(${p.id})_`);
      return `${lines.length} deadline${lines.length === 1 ? '' : 's'} detected — open /tasks → inbox to apply:\n\n${lines.join('\n')}`;
    });
  }

  async function runQuick(title: string, fn: (signal: AbortSignal) => Promise<string>) {
    if (busy) return;
    abort?.abort();
    abort = new AbortController();
    busy = true;
    quickTitle = title;
    quickResult = '_running…_';
    messages = []; // chat clears when a quick action runs
    try {
      quickResult = await fn(abort.signal);
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        quickResult = '_cancelled_';
      } else {
        const msg = err instanceof Error ? err.message : String(err);
        quickResult = /disabled in AI preferences/i.test(msg)
          ? `_${msg}_  \n\n[Open settings →](/settings)`
          : `_failed:_ ${msg}`;
      }
    } finally {
      busy = false;
      abort = null;
    }
  }

  // ── Chat ──────────────────────────────────────────────────────
  // Streaming via /api/v1/chat/stream so the user sees tokens
  // arriving — important on slow local LLMs where a 30s wait
  // with no signal feels broken. Cancel button aborts mid-stream.
  // ── Slash commands ──────────────────────────────────────────────
  // Type-driven shortcuts that bypass the chat round-trip when
  // possible. Power-user surface; the buttons above the chat
  // pane stay for click-first users. Recognised commands:
  //
  //   /help              show this list
  //   /clear             reset the conversation
  //   /briefing          fire the daily briefing (same as button)
  //   /synopsis          fire the weekly synopsis
  //   /triage            run inbox triage
  //   /deadlines         detect deadlines
  //   /detach            drop the snapshot/note attach for next turn
  //
  // A leading slash that doesn't match falls through to normal
  // chat — so a user pasting code with a leading "/" doesn't get
  // accidentally intercepted unless the first word is a real cmd.
  const SLASH_HELP = `**Modes** (top-left in this panel)

  - **General** — balanced help across writing, planning, questions
  - **Research** — grounded answers, named sources, no invention (RAG on)
  - **Writer** — drafting partner, matches your voice
  - **Coach** — Socratic, questions over answers (RAG on)
  - **Analyst** — evidence-first, what would falsify the claim (RAG on)
  - **Architect** — trade-offs + recommendations for system design

  Toggle **RAG** to search the vault for relevant notes per question.

**Shortcuts**

  - <kbd>Mod+J</kbd> — toggle this panel
  - <kbd>Mod+1..6</kbd> — switch agent mode (General → Architect)
  - **🎤 mic** in the input row — voice dictation (browser STT)
  - **save** in the header — write the thread to \`chat-history/\` as a note

**Slash commands**

  - \`/help\` — show this list
  - \`/clear\` — reset the conversation
  - \`/briefing\` — daily briefing (today's events + tasks)
  - \`/synopsis\` — weekly synopsis (Wins / Setbacks / Learned / Next)
  - \`/triage\` — inbox triage proposals
  - \`/deadlines\` — detect deadlines in untimed tasks
  - \`/detach\` — drop the attached snapshot or note for the next message

**Where AI lives in granit**

  - **Note editor** — \`Mod-Shift-A\` ask about selection · \`Mod-Shift-/\` ask about section · \`Mod-Alt-Space\` continue writing · link suggester in the right rail
  - **/morning** — "Suggest from tasks" picks today's #1 focus
  - **/tasks** — "Top 3" focus picker · inbox triage · deadline detect
  - **/calendar** — "Plan my week" agent
  - **/goals** — "Suggest milestones" on goal detail
  - **/projects** — AI summary on project detail
  - **/vision** — "Harden vision" critic
  - **/examen** — gentle reflection prompts per section
  - **/people** — "Suggest 3" reach-outs based on cadence + notes
  - **/habits** — pattern insights from last 30 days

  Press <kbd>Mod+J</kbd> to toggle this panel anywhere in granit.`;

  function handleSlashCommand(raw: string): boolean {
    const cmd = raw.trim().toLowerCase().split(/\s+/)[0];
    switch (cmd) {
      case '/help':
        // Render help inline as an assistant message — keeps the
        // result in the persisted thread so a follow-up ("ok now
        // briefing") still sees the user's prior context.
        messages = [
          ...messages,
          { role: 'user', content: raw },
          { role: 'assistant', content: SLASH_HELP }
        ];
        input = '';
        return true;
      case '/clear':
        clearChat();
        input = '';
        return true;
      case '/briefing':
        input = '';
        void runBriefing();
        return true;
      case '/synopsis':
        input = '';
        void runSynopsis();
        return true;
      case '/triage':
        input = '';
        void runTriage();
        return true;
      case '/deadlines':
        input = '';
        void runDeadlines();
        return true;
      case '/detach':
        attachNote = false;
        attachSnapshot = false;
        input = '';
        toast.success('Context detached for the next message.');
        return true;
      default:
        return false;
    }
  }

  async function loadRagIndex() {
    if (ragIndexLoaded) return;
    try {
      const r = await api.listNotes({ limit: 5000 });
      ragIndex = r.notes.map((n) => ({
        path: n.path,
        title: n.title || n.path.replace(/\.md$/, ''),
        modTime: n.modTime
      }));
    } finally {
      ragIndexLoaded = true;
    }
  }

  // Retrieve top-K notes for the user's query. Two-stage:
  //   1. Title-token match: every note whose title contains any of
  //      the query tokens scores 2 per token. Cheap, exact, no I/O.
  //   2. For the top ~12 by title score, fetch their bodies and
  //      add 1 per body-token match (simple substring count).
  // Recency bumps the final score slightly so a note touched
  // yesterday wins over one untouched in 2024 when titles tie. We
  // cap at 3 hits + clip each excerpt to 800 chars so the prompt
  // stays bounded on a 5k-note vault. Strict in-process retrieval —
  // no embeddings, no extra service. Future: swap in a real
  // embedding lookup at the same call site.
  async function retrieveForRag(query: string, currentNote?: string): Promise<RagHit[]> {
    if (!ragIndexLoaded) await loadRagIndex();
    const tokens = Array.from(
      new Set(
        query
          .toLowerCase()
          .replace(/[^\w\s/-]/g, ' ')
          .split(/\s+/)
          .filter((t) => t.length >= 3 && !STOPWORDS.has(t))
      )
    );
    if (tokens.length === 0) return [];
    const now = Date.now();
    const titleScored = ragIndex
      .map((n) => {
        if (n.path === currentNote) return null; // exclude the current note from RAG
        let s = 0;
        const title = n.title.toLowerCase();
        for (const t of tokens) {
          if (title.includes(t)) s += 2;
        }
        // Recency tiebreaker: +0..0.5 based on age vs 30-day window.
        const age = now - new Date(n.modTime).getTime();
        const recency = Math.max(0, Math.min(0.5, 0.5 - age / (30 * 86_400_000)));
        return s > 0 ? { ...n, score: s + recency } : null;
      })
      .filter((x): x is { path: string; title: string; modTime: string; score: number } => !!x)
      .sort((a, b) => b.score - a.score)
      .slice(0, 12);
    if (titleScored.length === 0) return [];
    // Body fetch top 12 in parallel; score each body match.
    const bodies = await Promise.all(
      titleScored.map((n) => api.getNote(n.path).catch(() => null))
    );
    const final: RagHit[] = [];
    for (let i = 0; i < titleScored.length; i++) {
      const meta = titleScored[i];
      const body = bodies[i]?.body ?? '';
      let bodyScore = 0;
      const lc = body.toLowerCase();
      for (const t of tokens) {
        // Count occurrences (capped at 5 per token to avoid one
        // word-spam note dominating).
        let count = 0;
        let idx = 0;
        while ((idx = lc.indexOf(t, idx)) >= 0 && count < 5) {
          count++;
          idx += t.length;
        }
        bodyScore += count;
      }
      const totalScore = meta.score + bodyScore;
      if (totalScore <= 0) continue;
      // Excerpt: find the first body line that mentions any token,
      // ±200 chars. Falls back to the start of the body.
      let excerpt = body.slice(0, 800);
      for (const t of tokens) {
        const at = lc.indexOf(t);
        if (at >= 0) {
          const start = Math.max(0, at - 200);
          excerpt = body.slice(start, start + 800);
          if (start > 0) excerpt = '…' + excerpt;
          break;
        }
      }
      final.push({ path: meta.path, title: meta.title, excerpt: excerpt.trim(), score: totalScore });
    }
    final.sort((a, b) => b.score - a.score);
    return final.slice(0, 3);
  }

  // Token-cleanup stopwords. Tiny English set — RAG queries are
  // typically short, and dropping these lets the score reflect
  // content words rather than 'the', 'a', etc.
  const STOPWORDS = new Set([
    'the', 'a', 'an', 'of', 'to', 'in', 'for', 'on', 'and', 'or', 'is', 'it', 'be',
    'are', 'was', 'were', 'this', 'that', 'with', 'from', 'as', 'by', 'at', 'but',
    'not', 'if', 'so', 'do', 'does', 'did', 'have', 'has', 'had', 'can', 'will',
    'what', 'when', 'how', 'why', 'who', 'where', 'should', 'would', 'could',
    'about', 'into', 'over', 'than', 'then', 'them', 'they', 'their'
  ]);

  // ── Voice input ────────────────────────────────────────────────
  // Click the mic, the browser's SpeechRecognition fills the input
  // as you speak. Same Web Speech API used by the voice-note modal;
  // graceful fallback when unsupported (Firefox).
  type RecognitionCtor = new () => SpeechRecognition;
  interface SpeechRecognition extends EventTarget {
    continuous: boolean;
    interimResults: boolean;
    lang: string;
    onresult: ((this: SpeechRecognition, ev: SpeechRecognitionEvent) => unknown) | null;
    onerror: ((this: SpeechRecognition, ev: Event) => unknown) | null;
    onend: ((this: SpeechRecognition, ev: Event) => unknown) | null;
    start: () => void;
    stop: () => void;
    abort: () => void;
  }
  interface SpeechRecognitionEvent extends Event {
    resultIndex: number;
    results: {
      length: number;
      [i: number]: { isFinal: boolean; [j: number]: { transcript: string } };
    };
  }
  function getRecognitionCtor(): RecognitionCtor | null {
    if (typeof window === 'undefined') return null;
    const w = window as unknown as { SpeechRecognition?: RecognitionCtor; webkitSpeechRecognition?: RecognitionCtor };
    return w.SpeechRecognition ?? w.webkitSpeechRecognition ?? null;
  }
  let voiceSupported = $derived(typeof window !== 'undefined' && getRecognitionCtor() !== null);
  let recording = $state(false);
  let recognition: SpeechRecognition | null = null;
  let voiceBaseline = ''; // input value when recording started — finals append to this

  function startVoice() {
    const Ctor = getRecognitionCtor();
    if (!Ctor || recording) return;
    voiceBaseline = input.endsWith(' ') || input.length === 0 ? input : input + ' ';
    recognition = new Ctor();
    recognition.continuous = true;
    recognition.interimResults = true;
    recognition.lang = navigator.language || 'en-US';
    recognition.onresult = (ev) => {
      let interim = '';
      let final = '';
      for (let i = ev.resultIndex; i < ev.results.length; i++) {
        const res = ev.results[i];
        const text = res[0].transcript;
        if (res.isFinal) final += text + ' ';
        else interim += text;
      }
      if (final) voiceBaseline += final;
      input = (voiceBaseline + interim).replace(/\s+/g, ' ').trim();
    };
    recognition.onerror = () => {};
    recognition.onend = () => {
      // Chrome auto-ends on silence — restart while we're still
      // in recording mode so a long thought continues.
      if (recording && recognition) {
        try { recognition.start(); } catch {}
      }
    };
    try {
      recognition.start();
      recording = true;
    } catch {}
  }
  function stopVoice() {
    recording = false;
    try { recognition?.stop(); } catch {}
    recognition = null;
  }
  function toggleVoice() {
    if (recording) stopVoice();
    else startVoice();
  }

  // ── Save thread as note ────────────────────────────────────────
  // Persists the current overlay conversation as a markdown note
  // under chat-history/YYYY-MM-DD-HHmm-<slug>.md. Useful when a
  // chat lands on a real insight worth keeping; the dedicated
  // /chat page is for long-running threads, this is the quick
  // 'this was a good answer, save it' move from any page.
  let saving = $state(false);
  function slugify(s: string): string {
    return s
      .toLowerCase()
      .replace(/[^\w\s-]/g, '')
      .replace(/\s+/g, '-')
      .slice(0, 60)
      .replace(/^-+|-+$/g, '');
  }
  async function saveThreadAsNote() {
    if (saving) return;
    if (messages.length === 0 && !quickResult) {
      toast.info('Nothing to save yet.');
      return;
    }
    saving = true;
    const now = new Date();
    const yyyy = now.getFullYear();
    const mm = String(now.getMonth() + 1).padStart(2, '0');
    const dd = String(now.getDate()).padStart(2, '0');
    const hh = String(now.getHours()).padStart(2, '0');
    const mi = String(now.getMinutes()).padStart(2, '0');
    const firstUser = messages.find((m) => m.role === 'user')?.content ?? quickTitle ?? 'chat';
    const slug = slugify(firstUser) || 'chat';
    const path = `chat-history/${yyyy}-${mm}-${dd}-${hh}${mi}-${slug}.md`;
    // Body: human-readable transcript with mode + RAG metadata.
    const lines: string[] = [
      '# ' + (firstUser.length > 80 ? firstUser.slice(0, 80) + '…' : firstUser),
      '',
      `> mode: **${mode.label}** · ${rag ? 'RAG on' : 'RAG off'} · captured ${now.toLocaleString()}`,
      ''
    ];
    if (quickResult) {
      lines.push('## ' + (quickTitle || 'Quick result'), '', quickResult, '');
    }
    for (const m of messages) {
      lines.push(m.role === 'user' ? '## You' : '## Assistant', '', m.content, '');
    }
    if (lastRagHits.length > 0) {
      lines.push('## Sources retrieved', '');
      for (const h of lastRagHits) lines.push(`- [[${h.path}|${h.title}]]`);
    }
    try {
      await api.createNote({
        path,
        frontmatter: {
          type: 'chat',
          mode: mode.id,
          rag,
          captured_at: now.toISOString(),
          tags: ['chat', mode.id]
        },
        body: lines.join('\n')
      });
      toast.success('Saved · ' + path);
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      saving = false;
    }
  }

  async function send(e?: Event) {
    e?.preventDefault();
    const text = input.trim();
    if (!text || busy) return;
    if (text.startsWith('/') && handleSlashCommand(text)) return;
    quickTitle = '';
    quickResult = '';
    busy = true;
    abort?.abort();
    abort = new AbortController();
    const userMsg: ChatMessage = { role: 'user', content: text };
    // Build the prelude — a system message containing the
    // active agent mode's posture, optionally the vault snapshot
    // (on non-note routes when attached), optionally retrieved
    // RAG hits (when rag=true). Posture stays for the whole
    // thread; snapshot/RAG inject on the first turn only since
    // re-injecting on every turn burns tokens for facts the
    // assistant has already seen.
    const prelude: ChatMessage[] = [];
    const isFirstTurn = messages.length === 0;
    // Mode posture — every turn (cheap; one paragraph). Keeps the
    // mode active even after history is long.
    prelude.push({ role: 'system', content: mode.system });
    if (isFirstTurn && attachSnapshot && snapshotData && !currentNotePath) {
      prelude.push({
        role: 'system',
        content:
          "Here's a snapshot of the user's vault — today's events, " +
          'open tasks, recent notes, active goals, and deadlines. ' +
          'Refer to it when relevant; do not invent content beyond it.\n\n' +
          '```json\n' + JSON.stringify(snapshotData, null, 2) + '\n```'
      });
    }
    // RAG — runs on every turn the toggle is on, so a follow-up
    // question about a different topic retrieves different notes.
    // We pass currentNotePath so retrieveForRag skips it (no point
    // re-injecting the note already on the prompt via notePath).
    // Composing both: attachNote=true (current note in system) +
    // rag=true (related notes in system) is supported and useful
    // for 'explain this concept using my other notes too'.
    lastRagHits = [];
    if (rag) {
      try {
        const hits = await retrieveForRag(text, currentNotePath);
        if (hits.length > 0) {
          lastRagHits = hits;
          const formatted = hits
            .map((h, i) => `### Note ${i + 1}: ${h.title}\nPath: \`${h.path}\`\n\n${h.excerpt}`)
            .join('\n\n---\n\n');
          prelude.push({
            role: 'system',
            content:
              `RAG retrieved ${hits.length} note(s) from the user's vault that match this query. Quote from these when relevant; cite the note title in your reply. Do NOT invent content beyond what's here. If they don't actually answer the question, say so plainly.\n\n${formatted}`
          });
        }
      } catch {
        // Retrieval failure shouldn't block the chat — fall through
        // and let the model answer without RAG context.
      }
    }
    const history = [...prelude, ...messages, userMsg];
    messages = [...messages, userMsg, { role: 'assistant', content: '' }];
    input = '';
    let acc = '';
    const idx = messages.length - 1;
    try {
      await api.chatStream(
        history,
        attachNote && currentNotePath ? currentNotePath : undefined,
        {
          onChunk: (c) => {
            acc += c;
            // Reassign through map so $state picks up the change.
            messages = messages.map((m, i) => (i === idx ? { ...m, content: acc } : m));
          },
          onError: (err) => {
            messages = messages.map((m, i) =>
              i === idx ? { ...m, content: `_error:_ ${err.message}` } : m
            );
          }
        },
        abort.signal
      );
    } finally {
      busy = false;
      abort = null;
      tick().then(() => {
        if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
      });
    }
  }

  function cancelInflight() {
    abort?.abort();
  }

  function clearChat() {
    if (messages.length === 0) return;
    messages = [];
    quickTitle = '';
    quickResult = '';
  }

  $effect(() => {
    void messages.length;
    void quickResult;
    tick().then(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  });

  function onInputKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      void send();
    }
  }

  // Mode quick-switch: Mod+1..6 picks the matching mode without
  // opening the picker. Power-user shortcut; only fires while the
  // overlay is open + the user isn't typing into the chat input
  // (numbers there should land as numbers, not mode jumps).
  $effect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod || e.shiftKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      if (target instanceof HTMLTextAreaElement || target instanceof HTMLInputElement) return;
      const idx = parseInt(e.key, 10);
      if (Number.isNaN(idx) || idx < 1 || idx > AGENT_MODES.length) return;
      e.preventDefault();
      selectMode(AGENT_MODES[idx - 1].id);
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
</script>

{#if open}
  <!-- Backdrop. Click-to-close on mobile; on desktop the panel
       sits next to content rather than over it, so the backdrop
       is hidden by md:hidden — desktop users dismiss with Esc or
       the close button. -->
  <button
    type="button"
    aria-label="close AI overlay"
    onclick={close}
    class="md:hidden fixed inset-0 z-40 bg-black/40 backdrop-blur-sm"
  ></button>

  <div
    bind:this={panelEl}
    data-ai-overlay
    role="dialog"
    aria-label="AI assistant"
    class="fixed z-50 flex flex-col bg-base border-surface1 shadow-2xl
           inset-x-0 bottom-0 max-h-[85vh] rounded-t-xl border-t
           md:inset-y-0 md:right-0 md:left-auto md:bottom-auto md:top-0 md:h-full md:w-[420px] md:max-h-none md:rounded-none md:border-l md:border-t-0"
  >
    <!-- Header. Mobile gets a drag-handle visual hint at the very
         top; both layouts get title + status pill + close. -->
    <div class="md:hidden flex justify-center pt-2 pb-1">
      <span class="block w-10 h-1 rounded-full bg-surface2"></span>
    </div>
    <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
      <!-- Mode picker — replaces the static '✨ AI assistant'
           heading. Click to open a popover of agent modes, each
           with a one-line tagline. Mode is the headline UX choice
           in the overlay; status pill + cancel + close pack to
           the right. -->
      <div class="relative flex-shrink-0">
        <button
          type="button"
          onclick={() => (modePickerOpen = !modePickerOpen)}
          aria-haspopup="listbox"
          aria-expanded={modePickerOpen}
          class="inline-flex items-center gap-1.5 px-2 py-1 rounded hover:bg-surface0 text-text"
          title={`Mode: ${mode.label} — ${mode.tagline}`}
        >
          <span class="text-base leading-none">{mode.glyph}</span>
          <span class="text-sm font-semibold">{mode.label}</span>
          <svg viewBox="0 0 24 24" class="w-3 h-3 opacity-60" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="6 9 12 15 18 9" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </button>
        {#if modePickerOpen}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div
            role="presentation"
            class="fixed inset-0 z-40"
            onclick={() => (modePickerOpen = false)}
          ></div>
          <div
            role="listbox"
            class="absolute left-0 top-full mt-1 w-72 bg-mantle border border-surface1 rounded-lg shadow-xl z-50 py-1"
          >
            {#each AGENT_MODES as m (m.id)}
              <button
                type="button"
                role="option"
                aria-selected={m.id === modeId}
                onclick={() => { selectMode(m.id); modePickerOpen = false; }}
                class="w-full flex items-start gap-2 px-3 py-2 hover:bg-surface0 text-left {m.id === modeId ? 'bg-primary/10' : ''}"
              >
                <span class="text-base leading-tight flex-shrink-0">{m.glyph}</span>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium text-text">{m.label}</div>
                  <div class="text-[11px] text-dim leading-snug">{m.tagline}</div>
                </div>
                {#if m.id === modeId}
                  <span class="text-primary text-xs flex-shrink-0">✓</span>
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      </div>
      {#if statusInfo}
        <span
          class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-surface1 text-subtext truncate hidden sm:inline-block"
          title="Default backend (per-feature overrides apply individually)"
        >{statusInfo.provider} · {statusInfo.model}</span>
      {/if}
      <span class="flex-1"></span>
      {#if busy}
        <button
          onclick={cancelInflight}
          class="px-2 py-1 text-[11px] text-warning hover:underline"
          title="Cancel the in-flight request"
        >cancel</button>
      {/if}
      <button
        onclick={close}
        aria-label="close"
        class="text-dim hover:text-text px-2 py-1 text-lg leading-none"
      >×</button>
    </header>

    {#if statusInfo?.sabbath || $sabbath}
      <div class="mx-4 mt-3 px-3 py-2 text-[11px] bg-warning/10 border border-warning/30 rounded text-warning">
        🕯️ Sabbath mode — AI requests are paused today.
      </div>
    {/if}

    <!-- Quick actions row. Wraps on small viewports so it never
         pushes the body off-screen. -->
    <div class="px-4 py-3 border-b border-surface1 flex flex-wrap gap-1.5 flex-shrink-0">
      <button
        onclick={runBriefing}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Briefing</button>
      <button
        onclick={runSynopsis}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Weekly synopsis</button>
      <button
        onclick={runTriage}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Triage</button>
      <button
        onclick={runDeadlines}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Deadlines</button>
      <span class="flex-1"></span>
      {#if messages.length > 0 || quickResult}
        <button
          onclick={() => void saveThreadAsNote()}
          disabled={saving}
          class="px-2 py-1 text-[11px] text-secondary hover:underline disabled:opacity-50 inline-flex items-center gap-1"
          title="Save this thread as a markdown note under chat-history/"
        >
          <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M5 4h11l3 3v13H5z"/>
            <path d="M9 4v5h6V4M8 14h8M8 18h6" stroke-linecap="round"/>
          </svg>
          {saving ? 'saving…' : 'save'}
        </button>
        <button
          onclick={clearChat}
          class="px-2 py-1 text-[11px] text-dim hover:text-error"
          title="Clear the overlay"
        >clear</button>
      {/if}
    </div>

    <!-- Body — quick-action result OR chat thread. Mutually
         exclusive: firing a quick action clears the chat, sending
         a chat message clears the quick result. Keeps the overlay
         single-purpose at any moment. -->
    <div bind:this={scrollEl} class="flex-1 overflow-y-auto px-4 py-3">
      {#if quickResult}
        <div class="text-[10px] uppercase tracking-wider text-secondary mb-2">{quickTitle}</div>
        <div class="prose prose-sm max-w-none">
          <MarkdownRenderer body={quickResult} />
        </div>
      {:else if messages.length > 0}
        <ul class="space-y-3">
          {#each messages as m, i (i)}
            <li>
              <div class="text-[10px] uppercase tracking-wider {m.role === 'user' ? 'text-secondary' : 'text-primary'} mb-0.5">
                {m.role === 'user' ? 'you' : 'assistant'}
              </div>
              {#if m.role === 'user'}
                <div class="text-sm text-text whitespace-pre-wrap">{m.content}</div>
              {:else}
                <div class="prose prose-sm max-w-none">
                  <MarkdownRenderer body={m.content || '_…_'} />
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {:else}
        <div class="text-xs text-dim leading-relaxed">
          <p class="mb-2">Quick actions above run the configured AI features. Or type a question below.</p>
          <p class="text-[11px]">Press <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">Mod+J</kbd> anywhere to open this. <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">Esc</kbd> to close.</p>
        </div>
      {/if}
    </div>

    {#if lastRagHits.length > 0}
      <!-- RAG attribution strip — shows which vault notes the
           assistant saw on the last turn so the user can verify
           grounding. Click any to open the actual note. Compact
           by default; line-truncates on mobile. -->
      <div class="border-t border-surface1 px-4 py-1.5 flex items-center gap-1.5 flex-wrap text-[11px] flex-shrink-0 bg-mantle/40">
        <span class="text-dim">retrieved:</span>
        {#each lastRagHits as h (h.path)}
          <a
            href="/notes/{encodeURIComponent(h.path)}"
            class="text-secondary hover:underline truncate max-w-[12rem]"
            title={h.path}
          >{h.title}</a>
        {/each}
      </div>
    {/if}

    {#if currentNotePath}
      <!-- Note-context chip. Lets the user toggle whether the
           current note is attached to the next chat message. The
           server-side notePath expander on /chat/stream injects
           the note's body into the system prompt; we only show
           the path here so the user knows what we're sending. -->
      <div class="border-t border-surface1 px-4 py-2 flex items-center gap-2 flex-shrink-0 text-[11px] flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer flex-1 min-w-[10rem]">
          <input
            type="checkbox"
            bind:checked={attachNote}
            class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0"
          />
          <span class="text-dim flex-shrink-0">attach</span>
          <span class="text-subtext font-mono truncate" title={currentNotePath}>{currentNotePath}</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer flex-shrink-0" title="Search the vault for relevant notes and include their excerpts as grounding context">
          <input
            type="checkbox"
            bind:checked={rag}
            class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0"
          />
          <span class="text-dim">RAG</span>
        </label>
      </div>
    {:else}
      <!-- Snapshot-context chip. On non-note routes the AI gets
           the Context Engine's snapshot — events, tasks, recent
           notes, goals, deadlines — so freeform questions like
           "what should I do next?" have actual data to lean on
           rather than guesses. Only injected on the first turn
           of a thread (subsequent turns lean on the model's own
           reply context to avoid burning tokens). RAG is the
           sibling toggle: search the full vault per turn and
           prepend the top matching notes' excerpts so cross-vault
           questions get grounded answers. -->
      <div class="border-t border-surface1 px-4 py-2 flex items-center gap-3 flex-shrink-0 text-[11px] flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer flex-1 min-w-[10rem]">
          <input
            type="checkbox"
            bind:checked={attachSnapshot}
            disabled={snapshotLoading}
            class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0 disabled:opacity-50"
          />
          <span class="text-dim flex-shrink-0">snapshot</span>
          <span class="text-subtext font-mono truncate">
            {#if snapshotLoading}
              loading…
            {:else if snapshotData}
              today's vault
            {:else}
              unavailable
            {/if}
          </span>
          {#if !snapshotLoading && !snapshotData}
            <button
              type="button"
              onclick={(e) => { e.preventDefault(); void loadSnapshot(); }}
              class="text-secondary hover:underline ml-1"
            >retry</button>
          {/if}
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer flex-shrink-0" title="Search the vault for relevant notes per question and include their excerpts as grounding context">
          <input
            type="checkbox"
            bind:checked={rag}
            class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0"
          />
          <span class="text-dim">RAG</span>
        </label>
      </div>
    {/if}

    <!-- Chat input. Sits at the bottom, growable up to a few rows.
         Enter sends, Shift+Enter inserts a newline. Disabled
         during Sabbath since the request would just be refused. -->
    <form
      onsubmit={send}
      class="border-t border-surface1 px-4 py-3 flex items-end gap-2 flex-shrink-0"
    >
      <textarea
        bind:this={inputEl}
        bind:value={input}
        onkeydown={onInputKey}
        rows="2"
        placeholder={$sabbath ? 'Sabbath active — AI paused' : recording ? 'Listening… speak freely' : 'Ask anything, or type /help for slash commands'}
        disabled={busy || $sabbath}
        class="flex-1 bg-surface0 border border-surface1 rounded px-3 py-2 text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-none disabled:opacity-60 {recording ? 'border-error' : ''}"
      ></textarea>
      {#if voiceSupported}
        <!-- Voice input: tap to start, tap again to stop. Live
             transcript fills the input as the user speaks. Same
             SpeechRecognition shape as the voice-note modal. -->
        <button
          type="button"
          onclick={toggleVoice}
          disabled={busy || $sabbath}
          aria-pressed={recording}
          class="px-3 py-2 text-sm rounded font-medium disabled:opacity-40 inline-flex items-center justify-center transition-colors {recording ? 'bg-error text-white animate-pulse' : 'bg-surface0 border border-surface1 text-subtext hover:border-primary'}"
          title={recording ? 'Stop dictating' : 'Dictate (browser speech-to-text)'}
          aria-label={recording ? 'Stop dictating' : 'Dictate'}
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="9" y="3" width="6" height="12" rx="3"/>
            <path d="M5 11a7 7 0 0014 0M12 18v3" stroke-linecap="round"/>
          </svg>
        </button>
      {/if}
      <button
        type="submit"
        disabled={busy || !input.trim() || $sabbath}
        class="px-3 py-2 text-sm bg-primary text-on-primary rounded font-medium disabled:opacity-40"
      >Send</button>
    </form>
  </div>
{/if}
