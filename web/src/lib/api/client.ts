// Minimal API client. Persists the bearer token in localStorage.

const TOKEN_KEY = 'everything.token';

export function getToken(): string | null {
  if (typeof localStorage === 'undefined') return null;
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(tok: string): void {
  localStorage.setItem(TOKEN_KEY, tok);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

// The single fetch + auth + error path the rest of the file rides
// on. Returns both the parsed body and the response ETag so callers
// that participate in optimistic-concurrency (notes editor's load
// → save → conflict-detect loop) can round-trip the If-Match header;
// callers that don't care use req() and ignore the etag.
export async function reqWithEtag<T>(path: string, init: RequestInit = {}): Promise<{ data: T; etag: string | null }> {
  const headers = new Headers(init.headers);
  const tok = getToken();
  if (tok) headers.set('Authorization', `Bearer ${tok}`);
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  const res = await fetch(`/api/v1${path}`, { ...init, headers });
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {}
    throw new ApiError(res.status, msg);
  }
  const etag = res.headers.get('ETag');
  if (res.status === 204) return { data: undefined as T, etag };
  return { data: (await res.json()) as T, etag };
}

// Convenience wrapper for callers that don't care about the ETag.
// Same throw-on-error semantics — this is the workhorse for the
// rest of the API surface; only the notes editor uses reqWithEtag
// directly.
export async function req<T>(path: string, init: RequestInit = {}): Promise<T> {
  const { data } = await reqWithEtag<T>(path, init);
  return data;
}

