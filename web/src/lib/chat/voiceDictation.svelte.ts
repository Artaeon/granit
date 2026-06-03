// Voice dictation harness for the chat composer.
//
// Wraps $lib/util/speechRecognition into a small Svelte 5 rune-state
// store: start / stop / toggle plus a reactive `recording` flag. The
// AIOverlay used to wire this inline (60 LOC of recognition lifecycle
// plus a "baseline + interim/final" string-merging effect); centralised
// here so a future ChatComposer extraction (or the dashboard's quick
// capture, which already uses the same SpeechRecognition wrapper)
// can share the exact same restart-on-silence + baseline-merge
// semantics.
//
// Exported as a factory so the consumer hands in (a) a getter for the
// current input text and (b) a setter the harness calls with the merged
// transcript. Reactive flag uses Svelte 5 runes via the `.svelte.ts`
// extension — calling code reads `dictation.recording` like a regular
// prop and SvelteKit rewires it on change.

import {
  createSpeechRecognition,
  isSpeechRecognitionSupported,
  type SpeechRecognitionLike
} from '$lib/util/speechRecognition';

export interface VoiceDictation {
  /** Whether STT is available in the current browser. Stable across
   *  the lifetime of the page; safe to read once at mount. */
  readonly supported: boolean;
  /** Live "are we listening" flag. Track from a $derived / template. */
  readonly recording: boolean;
  start(): void;
  stop(): void;
  toggle(): void;
}

export interface VoiceDictationOptions {
  /** Current value of the input text. Read at start() to seed the
   *  baseline that finals append to (so transcription extends what
   *  the user already typed rather than replacing it). */
  getInput(): string;
  /** Called with the merged transcript on every result event. The
   *  consumer's $state setter writes it back into the composer. */
  setInput(next: string): void;
}

export function createVoiceDictation(opts: VoiceDictationOptions): VoiceDictation {
  // The recognition handle is plain state (not reactive) — only
  // `recording` is read from templates. Avoids reactivity churn
  // every chunk.
  let recognition: SpeechRecognitionLike | null = null;
  let voiceBaseline = ''; // input value when recording started — finals append to this
  let recording = $state(false);
  const supported = isSpeechRecognitionSupported();

  function start() {
    if (recording) return;
    const r = createSpeechRecognition();
    if (!r) return;
    const cur = opts.getInput();
    voiceBaseline = cur.endsWith(' ') || cur.length === 0 ? cur : cur + ' ';
    r.continuous = true;
    r.interimResults = true;
    r.lang = navigator.language || 'en-US';
    r.onresult = (ev) => {
      let interim = '';
      let final = '';
      for (let i = ev.resultIndex; i < ev.results.length; i++) {
        const res = ev.results[i];
        if (!res || !res[0]) continue;
        const text = res[0].transcript;
        if (res.isFinal) final += text + ' ';
        else interim += text;
      }
      if (final) voiceBaseline += final;
      opts.setInput((voiceBaseline + interim).replace(/\s+/g, ' ').trim());
    };
    r.onerror = () => {};
    r.onend = () => {
      // Chrome auto-ends on silence — restart while we're still in
      // recording mode so a long thought continues to capture
      // without the user re-clicking.
      if (recording && recognition) {
        try { recognition.start(); } catch {}
      }
    };
    try {
      r.start();
      // Only adopt the recognition instance after start() succeeds.
      // If permission is denied / device busy / etc., start() throws
      // — keeping the assignment inside the try means a failed start
      // doesn't leave us holding a dangling recognition reference
      // that a future onend handler could still see and try to restart.
      recognition = r;
      recording = true;
    } catch {
      // Permission denied / device busy / etc. — drop silently;
      // recording stays false so the button label doesn't get
      // stuck mid-toggle. recognition stays null so stop() / onend
      // are no-ops.
    }
  }

  function stop() {
    recording = false;
    try { recognition?.stop(); } catch {}
    recognition = null;
  }

  function toggle() {
    if (recording) stop();
    else start();
  }

  return {
    supported,
    get recording() {
      return recording;
    },
    start,
    stop,
    toggle
  };
}
