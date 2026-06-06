// Map raw AI provider error messages to actionable UI hints. Provider
// errors land here from /api/v1/chat, /api/v1/agents/run, and the
// plan-day-schedule endpoint. The raw text is too noisy for end-users
// ("ollama: 404 {\"error\":\"model 'qwen2.5:0.5b' not found\"}") — this
// classifier picks the most specific match and produces a one-line
// headline plus an optional CTA so the user can fix the underlying
// cause (almost always a /settings tweak) instead of guessing.
//
// Match order is deliberate: connection issues win over generic 404
// because a refused dial means Ollama isn't running, not "model not
// found". The model-not-found branch comes after so a key error on a
// hosted provider doesn't get misread as an Ollama pull problem.

/** True when this Error is an AbortError — fired by AbortController
 *  when the user stops an in-flight AI stream. chatStream's onError
 *  is invoked with both real errors AND AbortErrors; controllers
 *  should filter aborts so the "Stop" button doesn't pollute the
 *  error surface with "aborted" red text after the user's own
 *  action. */
export function isAbortError(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false;
  const e = err as { name?: unknown; message?: unknown };
  if (e.name === 'AbortError') return true;
  // Some fetch implementations surface the abort as a DOMException
  // with no .name set or as an Error whose message contains "abort".
  // Belt + braces.
  if (typeof e.message === 'string' && /\babort(ed)?\b/i.test(e.message)) {
    return true;
  }
  return false;
}

export interface AiErrorHint {
  /** One-line user-facing summary. Safe to render in a toast. */
  headline: string;
  /** Optional click-through to the page where the user can fix it. */
  cta?: { label: string; href: string };
  /** Untouched original message — kept for the "details" expand and
   *  console logging. Never shown by default. */
  raw: string;
}

export function classifyAiError(message: string): AiErrorHint {
  const m = message.toLowerCase();

  // Auth / key issues. OpenAI uses "api key", "unauthorized", and
  // 401/403; Anthropic surfaces similar strings. We match before model
  // checks because a bad key on a hosted provider also returns a 404
  // for "model not found", and we want the more useful headline.
  if (
    m.includes('api key') ||
    m.includes('unauthorized') ||
    m.includes('401') ||
    m.includes('403')
  ) {
    return {
      headline: 'AI provider key is invalid or missing',
      cta: { label: 'Open Settings', href: '/settings' },
      raw: message
    };
  }

  // Ollama not running / wrong URL. "connection refused" is the
  // canonical Linux/macOS message; "no such host" is what Go's net
  // package returns for an unresolvable hostname. Either way the fix
  // is in /settings (toggle provider or correct ollama_url).
  if (
    m.includes('connection refused') ||
    m.includes('no such host') ||
    (m.includes('ollama') && m.includes('dial'))
  ) {
    return {
      headline: "Can't reach Ollama — is `ollama serve` running?",
      cta: { label: 'Open Settings', href: '/settings' },
      raw: message
    };
  }

  // Model not pulled / wrong name. Ollama returns 404 with a body
  // like {"error":"model 'foo' not found"}; OpenAI returns a 404 if
  // the model id doesn't match a snapshot. Both want a /settings tweak.
  if (m.includes('model') && (m.includes('not found') || m.includes('404'))) {
    return {
      headline: 'AI model not found — pull it or pick a different one',
      cta: { label: 'Open Settings', href: '/settings' },
      raw: message
    };
  }

  return { headline: 'AI request failed', raw: message };
}
