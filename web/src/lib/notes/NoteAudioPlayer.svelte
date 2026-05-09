<!--
  NoteAudioPlayer — read the current note aloud via the browser's
  SpeechSynthesis API. No backend dependency — costs nothing, works
  offline, no audit gating because no AI is involved (it's just
  text-to-speech, the same as a screen reader).

  Use case: walk-and-listen to your own notes. Hands-busy review.
  Eyes tired but brain still wants the content. Anywhere the user
  would prefer ears to eyes.

  UX shape — header strip with:
    - Play / pause toggle.
    - Stop / reset.
    - Speed slider (0.75x — 1.75x; SpeechSynthesis caps at 10x but
      anything above 1.8x is unintelligible for normal prose).
    - Voice picker (collapsed by default; expands on click).
    - Progress label (sentence N of M).
    - Close button to hide the player entirely.

  We chunk the body into "speakable" segments — sentence-ish pieces
  ending at . ! ? — and queue them sequentially. Chunking matters
  because some browsers (Chromium) clip an utterance to ~32KB; long
  notes would silently truncate. Sentence boundaries also let us
  show progress, jump forward/back, and survive an unexpected error
  on one sentence (skip + continue) instead of taking the whole
  reading down.

  Cleans up on unmount + on note path change so a half-spoken note
  doesn't keep talking when the user navigates away.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';

  let {
    body,
    title,
    onClose
  }: { body: string; title: string; onClose: () => void } = $props();

  // Strip the parts of the markdown that read poorly aloud:
  //   - frontmatter (`---` block)
  //   - fenced code blocks (a TTS reading line by line of code is
  //     pointless and slow)
  //   - heading hashes (just say the heading text)
  //   - link/wikilink markup (read the visible label, not the URL)
  //   - tag prefix (`#` is silent)
  //   - bold/italic markers
  // Footnote refs are kept as "footnote 1" inline because the body
  // gets chunked AFTER stripping; saying "see footnote N" mid-
  // sentence is more useful than dropping a number.
  function speakablePlainText(src: string): string {
    let s = src;
    // Frontmatter.
    if (s.startsWith('---')) {
      const end = s.indexOf('\n---', 3);
      if (end !== -1) s = s.slice(end + 4).replace(/^\r?\n/, '');
    }
    // Fenced code → "(code block)"  spoken once instead of read line by line.
    s = s.replace(/```[\s\S]*?```/g, '\n(code block)\n');
    s = s.replace(/~~~[\s\S]*?~~~/g, '\n(code block)\n');
    // Inline code → keep the text; backticks are silent.
    s = s.replace(/`([^`\n]+)`/g, '$1');
    // Wikilinks: `[[X|Y]]` → Y; `[[X]]` → X.
    s = s.replace(/!?\[\[([^\]\n|]+)(?:\|([^\]\n]+))?\]\]/g, (_m, a, b) => (b ?? a).trim());
    // Markdown links: `[label](url)` → label.
    s = s.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1');
    // Bare URLs → "(link)" so the TTS doesn't spell out the host.
    s = s.replace(/\bhttps?:\/\/[^\s)]+/g, '(link)');
    // Footnote refs: `[^id]` → "footnote {id}".
    s = s.replace(/\[\^([^\]\s]+)\]/g, 'footnote $1');
    // Highlights: `==text==` → just the text, with a pause.
    s = s.replace(/==([^=\n][^=]*?)==/g, '$1.');
    // Headings: drop the leading `#`s, append a period if missing
    // so the TTS pauses naturally between sections.
    s = s.replace(/^#{1,6}\s+(.+?)\s*#*$/gm, (_, t: string) => {
      const txt = t.trim();
      return txt.endsWith('.') || txt.endsWith('!') || txt.endsWith('?') ? txt : txt + '.';
    });
    // Bold/italic markers — strip surrounding **/* but keep inner text.
    s = s.replace(/\*\*([^*]+)\*\*/g, '$1');
    s = s.replace(/\*([^*]+)\*/g, '$1');
    s = s.replace(/__([^_]+)__/g, '$1');
    s = s.replace(/_([^_]+)_/g, '$1');
    // Tags: `#foo` → "foo" so we don't say "hash foo".
    s = s.replace(/(^|\s)#([\p{L}\p{N}_/-]+)/gu, '$1$2');
    // List bullets — drop leading "- ", "* ", "+ ", "1. ".
    s = s.replace(/^\s*[-*+]\s+/gm, '');
    s = s.replace(/^\s*\d+\.\s+/gm, '');
    // Block quotes — drop leading "> ".
    s = s.replace(/^\s*>\s?/gm, '');
    // Horizontal rules.
    s = s.replace(/^\s*([-*_]\s*){3,}\s*$/gm, '');
    // Collapse triple+ blank lines.
    s = s.replace(/\n{3,}/g, '\n\n');
    return s.trim();
  }

  // Split into "sentences" — period/question/exclamation followed by
  // whitespace OR end of paragraph. We over-split rather than under-
  // split because the cost of an extra chunk is one Utterance object;
  // the cost of an under-split is silent truncation on long notes.
  function chunkIntoSentences(src: string): string[] {
    if (!src.trim()) return [];
    const out: string[] = [];
    // Split on paragraph boundaries first so a paragraph never
    // straddles two utterances (the TTS resumes mid-paragraph would
    // sound bizarre).
    const paragraphs = src.split(/\n\s*\n+/);
    for (const para of paragraphs) {
      const trimmed = para.trim();
      if (!trimmed) continue;
      // Sentence-ish split. Look for [.?!] followed by space and
      // an uppercase letter or a digit (catches "... and 12 cars
      // came." but doesn't split on "U.S.A.").
      const parts = trimmed.split(/(?<=[.!?])\s+(?=[A-Z0-9])/);
      for (const p of parts) {
        const t = p.trim();
        if (!t) continue;
        // Cap any one chunk at ~600 chars — Chromium clips around
        // 32KB but anything over a few hundred chars per utterance
        // is too long for stable progress reporting (the user
        // can't seek into the middle of a 30-second sentence).
        if (t.length > 600) {
          for (let i = 0; i < t.length; i += 600) {
            out.push(t.slice(i, i + 600));
          }
        } else {
          out.push(t);
        }
      }
    }
    return out;
  }

  let sentences = $derived.by(() => chunkIntoSentences(speakablePlainText(body)));
  let total = $derived(sentences.length);
  let position = $state(0);
  let playing = $state(false);
  let paused = $state(false);
  let rate = $state(1);
  // Voices: lazy-load on click. Voices populate asynchronously in
  // some browsers; we listen for `voiceschanged` once and refresh.
  let voices = $state<SpeechSynthesisVoice[]>([]);
  let voiceURI = $state<string>('');
  let voicePickerOpen = $state(false);

  const VOICE_KEY = 'granit.note.audio.voice';
  const RATE_KEY = 'granit.note.audio.rate';

  // Hydrate persisted prefs on mount. Voice URI is matched
  // case-insensitively to a real voice on each load — voice list
  // can change between sessions.
  $effect(() => {
    const r = parseFloat(loadStoredString(RATE_KEY, ''));
    if (Number.isFinite(r) && r >= 0.5 && r <= 2.5) rate = r;
    const v = loadStoredString(VOICE_KEY, '');
    if (v) voiceURI = v;
  });

  function loadVoices() {
    if (typeof speechSynthesis === 'undefined') return;
    const list = speechSynthesis.getVoices();
    if (list.length > 0) {
      // Prefer English voices at the top; everything else after
      // alphabetically. Within English, prefer voices whose lang
      // starts with the doc lang (en-US > en-GB) — we don't have
      // a doc-lang frontmatter convention yet so fall back to
      // browser default lang.
      const docLang = (document.documentElement.lang || navigator.language || 'en-US').toLowerCase();
      const sorted = [...list].sort((a, b) => {
        const aMatch = a.lang.toLowerCase().startsWith(docLang.slice(0, 2));
        const bMatch = b.lang.toLowerCase().startsWith(docLang.slice(0, 2));
        if (aMatch && !bMatch) return -1;
        if (!aMatch && bMatch) return 1;
        return a.name.localeCompare(b.name);
      });
      voices = sorted;
      if (!voiceURI && sorted.length > 0) voiceURI = sorted[0].voiceURI;
    }
  }

  $effect(() => {
    if (typeof speechSynthesis === 'undefined') return;
    loadVoices();
    const onChange = () => loadVoices();
    speechSynthesis.addEventListener('voiceschanged', onChange);
    return () => speechSynthesis.removeEventListener('voiceschanged', onChange);
  });

  // Stop reading when the body / sentences change so a doc edit
  // mid-read doesn't keep speaking stale text. Re-play would have
  // to be a deliberate user action.
  $effect(() => {
    void sentences;
    if (typeof speechSynthesis === 'undefined') return;
    if (playing) {
      speechSynthesis.cancel();
      playing = false;
      paused = false;
      position = 0;
    }
  });

  let currentUtterance: SpeechSynthesisUtterance | null = null;

  function speakAt(idx: number) {
    if (typeof speechSynthesis === 'undefined') return;
    if (idx >= sentences.length) {
      // Reached the end — reset.
      playing = false;
      paused = false;
      position = 0;
      return;
    }
    const u = new SpeechSynthesisUtterance(sentences[idx]);
    u.rate = rate;
    const voice = voices.find((v) => v.voiceURI === voiceURI);
    if (voice) {
      u.voice = voice;
      u.lang = voice.lang;
    }
    u.onend = () => {
      // If user paused or stopped between fire and end, don't auto-
      // advance — they're driving now.
      if (!playing) return;
      position = idx + 1;
      speakAt(idx + 1);
    };
    u.onerror = (ev) => {
      // Skip to next sentence on a single-utterance error rather
      // than stopping the whole reading.
      if (!playing) return;
      // 'canceled' / 'interrupted' aren't actual failures — stop()
      // and pause() both fire them. Treat as expected.
      const reason = (ev as SpeechSynthesisErrorEvent).error;
      if (reason === 'canceled' || reason === 'interrupted') return;
      console.warn('TTS error', reason, 'at sentence', idx);
      position = idx + 1;
      speakAt(idx + 1);
    };
    currentUtterance = u;
    speechSynthesis.speak(u);
  }

  function play() {
    if (typeof speechSynthesis === 'undefined') return;
    if (sentences.length === 0) return;
    if (paused) {
      speechSynthesis.resume();
      paused = false;
      playing = true;
      return;
    }
    speechSynthesis.cancel();
    playing = true;
    paused = false;
    speakAt(position);
  }

  function pause() {
    if (typeof speechSynthesis === 'undefined') return;
    speechSynthesis.pause();
    paused = true;
    playing = false;
  }

  function stop() {
    if (typeof speechSynthesis === 'undefined') return;
    speechSynthesis.cancel();
    playing = false;
    paused = false;
    position = 0;
    currentUtterance = null;
  }

  function next() {
    if (sentences.length === 0) return;
    const wasPlaying = playing || paused;
    speechSynthesis.cancel();
    position = Math.min(sentences.length - 1, position + 1);
    if (wasPlaying) {
      speechSynthesis.cancel();
      playing = true;
      paused = false;
      speakAt(position);
    }
  }
  function prev() {
    if (sentences.length === 0) return;
    const wasPlaying = playing || paused;
    speechSynthesis.cancel();
    position = Math.max(0, position - 1);
    if (wasPlaying) {
      speechSynthesis.cancel();
      playing = true;
      paused = false;
      speakAt(position);
    }
  }

  function setRate(v: number) {
    rate = Math.max(0.5, Math.min(2.5, v));
    saveStoredString(RATE_KEY, String(rate));
    // Apply on next utterance — Chromium needs us to cancel and
    // re-speak from current position to pick up a rate change
    // mid-utterance. Doing so for every slider tick would be
    // jittery; we accept that the change takes effect on the next
    // sentence instead.
  }

  function setVoice(uri: string) {
    voiceURI = uri;
    saveStoredString(VOICE_KEY, uri);
    voicePickerOpen = false;
    // Re-speak from current position with the new voice if active.
    if (playing) {
      speechSynthesis.cancel();
      speakAt(position);
    }
  }

  // Cleanup on destroy — otherwise a navigation away keeps talking.
  onDestroy(() => {
    if (typeof speechSynthesis !== 'undefined') {
      speechSynthesis.cancel();
    }
    playing = false;
    paused = false;
  });

  let supported = $derived(typeof window !== 'undefined' && 'speechSynthesis' in window);
  let activeVoice = $derived(voices.find((v) => v.voiceURI === voiceURI) ?? null);
</script>

<div class="audio-player border-t border-b border-secondary/20 bg-secondary/5 px-3 py-1.5 flex items-center gap-2 text-xs flex-wrap">
  {#if !supported}
    <span class="text-dim italic">browser doesn't support speech synthesis</span>
    <button
      type="button"
      onclick={onClose}
      class="ml-auto text-dim hover:text-text"
      aria-label="close audio player"
    >×</button>
  {:else if total === 0}
    <span class="text-dim italic">nothing speakable in this note</span>
    <button
      type="button"
      onclick={onClose}
      class="ml-auto text-dim hover:text-text"
      aria-label="close audio player"
    >×</button>
  {:else}
    <!-- Play / pause toggle. Wide click target since this is the
         primary control for the whole player. -->
    {#if playing}
      <button
        type="button"
        onclick={pause}
        aria-label="pause"
        title="Pause"
        class="w-7 h-7 flex items-center justify-center rounded bg-secondary text-mantle hover:opacity-90"
      >❚❚</button>
    {:else}
      <button
        type="button"
        onclick={play}
        aria-label={paused ? 'resume' : 'play'}
        title={paused ? 'Resume' : 'Play'}
        class="w-7 h-7 flex items-center justify-center rounded bg-secondary text-mantle hover:opacity-90"
      >▶</button>
    {/if}
    <button
      type="button"
      onclick={stop}
      disabled={!playing && !paused && position === 0}
      aria-label="stop"
      title="Stop and reset"
      class="w-6 h-6 flex items-center justify-center text-subtext hover:text-error disabled:opacity-30"
    >■</button>
    <button
      type="button"
      onclick={prev}
      aria-label="previous sentence"
      title="Previous sentence"
      class="w-6 h-6 flex items-center justify-center text-subtext hover:text-secondary"
    >‹</button>
    <button
      type="button"
      onclick={next}
      aria-label="next sentence"
      title="Next sentence"
      class="w-6 h-6 flex items-center justify-center text-subtext hover:text-secondary"
    >›</button>

    <!-- Progress label. Replaces a giant scrubber bar (which would
         require a meaningful per-sentence duration we don't have)
         with a sentence-counter that's accurate cheap. -->
    <span class="text-dim font-mono tabular-nums" title={title}>
      {position + 1} / {total}
    </span>

    <span class="flex-1"></span>

    <!-- Speed slider. 0.75x is "easier-to-follow" speed; 1.0x is
         normal; ~1.5x is the speedrun crowd's sweet spot. -->
    <label class="inline-flex items-center gap-1 text-dim">
      <span class="text-[10px] uppercase tracking-wider">speed</span>
      <input
        type="range"
        min="0.75"
        max="1.75"
        step="0.05"
        value={rate}
        oninput={(e) => setRate(parseFloat((e.target as HTMLInputElement).value))}
        class="w-20"
        aria-label="reading speed"
      />
      <span class="font-mono text-[10px] w-7 tabular-nums">{rate.toFixed(2)}x</span>
    </label>

    <!-- Voice picker — collapsed by default; click opens a small
         dropdown. We don't render the full list inline because some
         browsers expose 30+ voices and it's a wall of noise. -->
    <div class="relative">
      <button
        type="button"
        onclick={() => (voicePickerOpen = !voicePickerOpen)}
        class="text-secondary hover:underline text-[11px]"
        title="Pick a voice"
      >
        {activeVoice ? activeVoice.name.slice(0, 18) : 'voice'} ▾
      </button>
      {#if voicePickerOpen}
        <div
          class="absolute right-0 top-full mt-1 z-10 max-h-64 overflow-y-auto bg-base border border-surface1 rounded shadow-lg min-w-[14rem]"
          role="listbox"
          aria-label="voice"
        >
          {#each voices as v}
            <button
              type="button"
              onclick={() => setVoice(v.voiceURI)}
              class="w-full text-left px-2 py-1 text-[11px] hover:bg-surface0 truncate {v.voiceURI === voiceURI ? 'bg-surface0 text-primary' : 'text-text'}"
              title={`${v.name} (${v.lang})`}
            >
              <span class="font-mono text-[10px] text-dim mr-1">{v.lang}</span>
              {v.name}
            </button>
          {/each}
        </div>
      {/if}
    </div>

    <button
      type="button"
      onclick={onClose}
      class="text-dim hover:text-text text-base leading-none"
      aria-label="close audio player"
      title="Close audio player"
    >×</button>
  {/if}
</div>
