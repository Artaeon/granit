// Thin typed wrapper around the browser SpeechRecognition API.
//
// `webkitSpeechRecognition` is a vendor-prefixed legacy interface
// that TypeScript's lib.dom.d.ts doesn't include in older versions
// — and even when it does, the constructor signature lives behind a
// `(window as any)` cast at every call site. AIOverlay and the
// dashboard's QuickCaptureWidget both grew their own untyped
// SpeechRecognition handling; consolidate here so future surfaces
// don't repeat the dance.
//
// The exported `createSpeechRecognition()` returns null when the
// API isn't available (Firefox desktop has no support; Safari
// requires user gesture). Callers branch on the return value to
// hide the voice button rather than rendering one that errors on
// click.

/** One alternative transcription from the engine. The Web Speech
 *  API returns multiple alternatives per result; we use only the
 *  highest-confidence one ([0]). */
export interface SpeechRecognitionAlternative {
  transcript: string;
  confidence: number;
}

/** A single recognition result. The result is BOTH array-like (with
 *  alternatives at numeric indices) AND has an `isFinal` flag — the
 *  combined shape matches the Web Speech API's `SpeechRecognitionResult`. */
export interface SpeechRecognitionResultLike extends ArrayLike<SpeechRecognitionAlternative> {
  /** Final = the engine has stopped revising this segment. */
  readonly isFinal: boolean;
}

export interface SpeechRecognitionLike {
  /** When true, the engine keeps listening across phrase boundaries. */
  continuous: boolean;
  /** When true, fires onresult with isFinal=false during a phrase. */
  interimResults: boolean;
  /** BCP-47 tag, e.g. "en-US". Defaults to navigator.language. */
  lang: string;
  start(): void;
  stop(): void;
  /** Fired on every recognition update (interim + final). */
  onresult: ((event: { resultIndex: number; results: ArrayLike<SpeechRecognitionResultLike> }) => void) | null;
  onerror: ((event: { error: string }) => void) | null;
  onend: (() => void) | null;
}

interface SpeechRecognitionConstructor {
  new (): SpeechRecognitionLike;
}

/** Returns true when the browser exposes SpeechRecognition. */
export function isSpeechRecognitionSupported(): boolean {
  if (typeof window === 'undefined') return false;
  const w = window as unknown as {
    SpeechRecognition?: SpeechRecognitionConstructor;
    webkitSpeechRecognition?: SpeechRecognitionConstructor;
  };
  return !!(w.SpeechRecognition || w.webkitSpeechRecognition);
}

/** Construct a fresh SpeechRecognition instance, or null when
 *  unavailable. The caller wires up onresult / onerror / onend
 *  and calls start()/stop() as needed. */
export function createSpeechRecognition(): SpeechRecognitionLike | null {
  if (typeof window === 'undefined') return null;
  const w = window as unknown as {
    SpeechRecognition?: SpeechRecognitionConstructor;
    webkitSpeechRecognition?: SpeechRecognitionConstructor;
  };
  const SR = w.SpeechRecognition || w.webkitSpeechRecognition;
  if (!SR) return null;
  return new SR();
}
